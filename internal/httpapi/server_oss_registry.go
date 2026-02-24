package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s server) ossIngestAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(s.ossEventsIngestToken) == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "oss ingest not configured"})
			return
		}
		token := strings.TrimSpace(r.Header.Get("X-AIHub-Oss-Ingest-Token"))
		if token == "" {
			token = strings.TrimSpace(r.URL.Query().Get("token"))
		}
		if token == "" || token != s.ossEventsIngestToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

type ossIngestEvent struct {
	ObjectKey  string         `json:"object_key"`
	EventType  string         `json:"event_type"`
	OccurredAt string         `json:"occurred_at,omitempty"`
	Payload    map[string]any `json:"payload,omitempty"`
}

type ossIngestRequest struct {
	Events []ossIngestEvent `json:"events"`
}

func (s server) handleIngestOSSEvents(w http.ResponseWriter, r *http.Request) {
	var req ossIngestRequest
	if !readJSONLimited(w, r, &req, 512*1024) {
		return
	}

	// Allow single-event payloads for convenience.
	if len(req.Events) == 0 {
		var single ossIngestEvent
		if err := json.NewDecoder(strings.NewReader("null")).Decode(&single); err == nil {
			// no-op
		}
	}
	if len(req.Events) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no events"})
		return
	}
	if len(req.Events) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many events"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	inserted := 0
	for _, ev := range req.Events {
		objectKey := strings.TrimSpace(ev.ObjectKey)
		eventType := strings.TrimSpace(ev.EventType)
		if objectKey == "" || len(objectKey) > 2048 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid object_key"})
			return
		}
		if eventType == "" || len(eventType) > 64 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid event_type"})
			return
		}

		occurredAt := time.Now().UTC()
		if strings.TrimSpace(ev.OccurredAt) != "" {
			if t, err := time.Parse(time.RFC3339, strings.TrimSpace(ev.OccurredAt)); err == nil {
				occurredAt = t.UTC()
			}
		}

		payloadJSON, err := marshalJSONB(ev.Payload)
		if err != nil {
			logError(ctx, "marshal oss event payload failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
			return
		}

		if _, err := tx.Exec(ctx, `
			insert into oss_events (object_key, event_type, occurred_at, payload)
			values ($1, $2, $3, $4)
		`, objectKey, eventType, occurredAt, payloadJSON); err != nil {
			logError(ctx, "insert oss_events failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
			return
		}
		inserted++
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "db commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "inserted": inserted})
}

type issueOSSCredentialsRequest struct {
	Kind             string `json:"kind"`
	TaskID           string `json:"task_id,omitempty"`
	CircleID         string `json:"circle_id,omitempty"`
	RequestAgentID   string `json:"request_agent_id,omitempty"`
	TopicID          string `json:"topic_id,omitempty"`
	TopicRequestType string `json:"topic_request_type,omitempty"`
	RoleID           string `json:"role_id,omitempty"`
}

func heartbeatShard(agentID string) string {
	sum := sha256.Sum256([]byte(agentID))
	return hex.EncodeToString(sum[:1])
}

func (s server) handleIssueOSSCredentials(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	var req issueOSSCredentialsRequest
	if !readJSONLimited(w, r, &req, 32*1024) {
		return
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = "registry_read"
	}
	switch kind {
	case "registry_read", "registry_write", "task_read", "task_write",
		"circle_join_request_write", "circle_join_approval_write",
		"topic_read", "topic_request_write", "topic_message_write":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid kind"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var admittedStatus string
	if err := s.db.QueryRow(ctx, `select admitted_status from agents where id=$1`, agentID).Scan(&admittedStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		logError(ctx, "query admitted_status failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if admittedStatus != "admitted" {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent not admitted"})
		return
	}

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")

	var (
		allowList []string
		allowRead []string
		allowWrite []string
	)

	// Discovery read is shared to all admitted agents.
	allowList = append(allowList,
		agenthome.JoinKey(basePrefix, "agents/all/"),
		agenthome.JoinKey(basePrefix, "agents/heartbeats/"),
	)
	allowRead = append(allowRead,
		agenthome.JoinKey(basePrefix, "agents/all/"),
		agenthome.JoinKey(basePrefix, "agents/heartbeats/"),
	)

	// Agent private config read: only itself.
	allowList = append(allowList, agenthome.JoinKey(basePrefix, "agents/prompts/"+agentID.String()+"/"))
	allowRead = append(allowRead, agenthome.JoinKey(basePrefix, "agents/prompts/"+agentID.String()+"/"))

	if kind == "registry_write" {
		hbKey := "agents/heartbeats/" + heartbeatShard(agentID.String()) + "/" + agentID.String() + ".last"
		allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, hbKey))
	}

	if kind == "task_read" || kind == "task_write" {
		taskID := strings.TrimSpace(req.TaskID)
		if taskID == "" || len(taskID) > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid task_id"})
			return
		}
		manifestKey := "tasks/" + taskID + "/manifest.json"
		manifestRaw, err := store.GetObject(ctx, manifestKey)
		if err != nil {
			logError(ctx, "get task manifest failed", err)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		var mf struct {
			Visibility string   `json:"visibility"`
			CircleID   string   `json:"circle_id,omitempty"`
			InviteAgentIDs []string `json:"invite_agent_ids,omitempty"`
			OwnerAgentID string `json:"owner_agent_id,omitempty"`
			OwnerID     string `json:"owner_id,omitempty"`
		}
		if err := json.Unmarshal(manifestRaw, &mf); err != nil {
			logError(ctx, "unmarshal task manifest failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
			return
		}
		allowed := false
		switch strings.TrimSpace(mf.Visibility) {
		case "public":
			allowed = true
		case "owner-only", "owner_only":
			if strings.TrimSpace(mf.OwnerAgentID) != "" {
				allowed = strings.TrimSpace(mf.OwnerAgentID) == agentID.String()
			} else if strings.TrimSpace(mf.OwnerID) != "" {
				allowed = strings.TrimSpace(mf.OwnerID) == agentID.String()
			}
		case "invite":
			for _, id := range mf.InviteAgentIDs {
				if strings.TrimSpace(id) == agentID.String() {
					allowed = true
					break
				}
			}
		case "circle":
			cid := strings.TrimSpace(mf.CircleID)
			if cid != "" {
				ok, err := store.Exists(ctx, "circles/"+cid+"/members/"+agentID.String()+".json")
				if err != nil {
					logError(ctx, "check circle member failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership check failed"})
					return
				}
				allowed = ok
			}
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported task visibility"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not authorized"})
			return
		}
		taskPrefix := "tasks/" + taskID + "/"
		allowList = append(allowList, agenthome.JoinKey(basePrefix, taskPrefix))
		allowRead = append(allowRead, agenthome.JoinKey(basePrefix, taskPrefix))
		if kind == "task_write" {
			allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, "tasks/"+taskID+"/agents/"+agentID.String()+"/"))
		}
	}

	if kind == "circle_join_request_write" {
		circleID := strings.TrimSpace(req.CircleID)
		if circleID == "" || len(circleID) > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid circle_id"})
			return
		}
		// Any admitted agent may request to join; write is scoped to its own single request object.
		reqKey := "circles/" + circleID + "/join_requests/" + agentID.String() + ".json"
		allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
		allowRead = append(allowRead, agenthome.JoinKey(basePrefix, "circles/"+circleID+"/manifest.json"))
	}

	if kind == "circle_join_approval_write" {
		circleID := strings.TrimSpace(req.CircleID)
		requestAgentID := strings.TrimSpace(req.RequestAgentID)
		if circleID == "" || len(circleID) > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid circle_id"})
			return
		}
		if requestAgentID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing request_agent_id"})
			return
		}
		// Only circle owner may approve.
		manifestKey := "circles/" + circleID + "/manifest.json"
		manifestRaw, err := store.GetObject(ctx, manifestKey)
		if err != nil {
			logError(ctx, "get circle manifest failed", err)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "circle not found"})
			return
		}
		var mf struct {
			OwnerAgentID string `json:"owner_agent_id,omitempty"`
			OwnerID      string `json:"owner_id,omitempty"`
		}
		if err := json.Unmarshal(manifestRaw, &mf); err != nil {
			logError(ctx, "unmarshal circle manifest failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
			return
		}
		owner := strings.TrimSpace(mf.OwnerAgentID)
		if owner == "" {
			owner = strings.TrimSpace(mf.OwnerID)
		}
		if owner == "" || owner != agentID.String() {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not circle owner"})
			return
		}
		approvalKey := "circles/" + circleID + "/join_approvals/" + requestAgentID + "/" + agentID.String() + ".json"
		allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, approvalKey))
		allowList = append(allowList, agenthome.JoinKey(basePrefix, "circles/"+circleID+"/join_requests/"))
		allowRead = append(allowRead, agenthome.JoinKey(basePrefix, "circles/"+circleID+"/join_requests/"))
	}

	if kind == "topic_read" || kind == "topic_request_write" || kind == "topic_message_write" {
		topicID := strings.TrimSpace(req.TopicID)
		if topicID == "" || len(topicID) > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid topic_id"})
			return
		}

		manifestKey := "topics/" + topicID + "/manifest.json"
		manifestRaw, err := store.GetObject(ctx, manifestKey)
		if err != nil {
			logError(ctx, "get topic manifest failed", err)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}
		var mf struct {
			Visibility        string   `json:"visibility"`
			CircleID          string   `json:"circle_id,omitempty"`
			AllowlistAgentIDs []string `json:"allowlist_agent_ids,omitempty"`
			OwnerAgentID      string   `json:"owner_agent_id,omitempty"`
			Mode              string   `json:"mode"`
			Rules             map[string]any `json:"rules,omitempty"`
		}
		if err := json.Unmarshal(manifestRaw, &mf); err != nil {
			logError(ctx, "unmarshal topic manifest failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
			return
		}

		allowed := false
		switch strings.TrimSpace(mf.Visibility) {
		case "public":
			allowed = true
		case "owner-only", "owner_only":
			allowed = strings.TrimSpace(mf.OwnerAgentID) != "" && strings.TrimSpace(mf.OwnerAgentID) == agentID.String()
		case "invite":
			for _, id := range mf.AllowlistAgentIDs {
				if strings.TrimSpace(id) == agentID.String() {
					allowed = true
					break
				}
			}
		case "circle":
			cid := strings.TrimSpace(mf.CircleID)
			if cid != "" {
				ok, err := store.Exists(ctx, "circles/"+cid+"/members/"+agentID.String()+".json")
				if err != nil {
					logError(ctx, "check circle member failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "membership check failed"})
					return
				}
				allowed = ok
			}
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported topic visibility"})
			return
		}
		if !allowed {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not authorized"})
			return
		}

		topicPrefix := "topics/" + topicID + "/"
		allowList = append(allowList, agenthome.JoinKey(basePrefix, topicPrefix))
		allowRead = append(allowRead, agenthome.JoinKey(basePrefix, topicPrefix))

		if kind == "topic_message_write" {
			mode := strings.TrimSpace(mf.Mode)
			if mode == "" {
				mode = "freeform"
			}

			switch mode {
			case "intro_once":
				if !ruleBool(mf.Rules, "allow_reintro_on_card_version_increase", true) {
					prefix := "topics/" + topicID + "/messages/" + agentID.String() + "/intro_card_v"
					existing, err := store.ListObjects(ctx, prefix, 1)
					if err != nil {
						logError(ctx, "list intro_once messages failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
						return
					}
					if len(existing) > 0 {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already posted"})
						return
					}
				}

				var cardVersion int
				if err := s.db.QueryRow(ctx, `select card_version from agents where id=$1`, agentID).Scan(&cardVersion); err != nil {
					logError(ctx, "query card_version failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
					return
				}
				msgKey := "topics/" + topicID + "/messages/" + agentID.String() + "/intro_card_v" + strconv.Itoa(cardVersion) + ".json"
				exists, err := store.Exists(ctx, msgKey)
				if err != nil {
					logError(ctx, "check intro_once message exists failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
					return
				}
				if exists {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already posted"})
					return
				}
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, msgKey))
			case "daily_checkin":
				tz := "Asia/Shanghai"
				if v, ok := mf.Rules["day_boundary_timezone"].(string); ok && strings.TrimSpace(v) != "" {
					tz = strings.TrimSpace(v)
				}
				loc, err := time.LoadLocation(tz)
				if err != nil {
					logError(ctx, "load day_boundary_timezone failed; fallback to UTC", err)
					loc = time.UTC
				}
				dateKey := time.Now().UTC().In(loc).Format("20060102")
				msgKey := "topics/" + topicID + "/messages/" + agentID.String() + "/" + dateKey + ".json"
				exists, err := store.Exists(ctx, msgKey)
				if err != nil {
					logError(ctx, "check daily_checkin message exists failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
					return
				}
				if exists {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already checked in today"})
					return
				}
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, msgKey))
			case "turn_queue", "debate", "idiom_chain", "roast_banter", "crosstalk", "skit_chain":
				stateRaw, err := store.GetObject(ctx, "topics/"+topicID+"/state.json")
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				var st struct {
					State map[string]any `json:"state"`
				}
				if err := json.Unmarshal(stateRaw, &st); err != nil {
					logError(ctx, "unmarshal topic state failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "state decode failed"})
					return
				}
				speaker, _ := st.State["speaker_agent_id"].(string)
				if strings.TrimSpace(speaker) != agentID.String() {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "not current speaker"})
					return
				}
				turnID, _ := st.State["turn_id"].(string)
				turnID = strings.TrimSpace(turnID)
				if turnID == "" {
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing turn_id"})
					return
				}
				msgKey := "topics/" + topicID + "/messages/" + agentID.String() + "/" + turnID + "_0001.json"
				exists, err := store.Exists(ctx, msgKey)
				if err != nil {
					logError(ctx, "check turn_queue message exists failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
					return
				}
				if exists {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already posted"})
					return
				}
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, msgKey))
			case "limited_slots":
				stateRaw, err := store.GetObject(ctx, "topics/"+topicID+"/state.json")
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				var st struct {
					State map[string]any `json:"state"`
				}
				if err := json.Unmarshal(stateRaw, &st); err != nil {
					logError(ctx, "unmarshal topic state failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "state decode failed"})
					return
				}
				slotsAny, _ := st.State["slots"].([]any)
				slotID := ""
				for _, v := range slotsAny {
					m, ok := v.(map[string]any)
					if !ok {
						continue
					}
					a, _ := m["agent_id"].(string)
					if strings.TrimSpace(a) != agentID.String() {
						continue
					}
					sid, _ := m["slot_id"].(string)
					slotID = strings.TrimSpace(sid)
					if slotID != "" {
						break
					}
				}
				if slotID == "" {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "no slot"})
					return
				}
				msgKey := "topics/" + topicID + "/messages/" + agentID.String() + "/" + slotID + ".json"
				exists, err := store.Exists(ctx, msgKey)
				if err != nil {
					logError(ctx, "check limited_slots message exists failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
					return
				}
				if exists {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already posted"})
					return
				}
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, msgKey))
			case "drum_pass":
				stateRaw, err := store.GetObject(ctx, "topics/"+topicID+"/state.json")
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				var st struct {
					State map[string]any `json:"state"`
				}
				if err := json.Unmarshal(stateRaw, &st); err != nil {
					logError(ctx, "unmarshal topic state failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "state decode failed"})
					return
				}
				holder, _ := st.State["holder_agent_id"].(string)
				if strings.TrimSpace(holder) != agentID.String() {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "not current holder"})
					return
				}
				beatID, _ := st.State["beat_id"].(string)
				beatID = strings.TrimSpace(beatID)
				if beatID == "" {
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing beat_id"})
					return
				}
				msgKey := "topics/" + topicID + "/messages/" + agentID.String() + "/" + beatID + "_0001.json"
				exists, err := store.Exists(ctx, msgKey)
				if err != nil {
					logError(ctx, "check drum_pass message exists failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
					return
				}
				if exists {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already posted"})
					return
				}
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, msgKey))
			case "collab_roles":
				stateRaw, err := store.GetObject(ctx, "topics/"+topicID+"/state.json")
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				var st struct {
					State map[string]any `json:"state"`
				}
				if err := json.Unmarshal(stateRaw, &st); err != nil {
					logError(ctx, "unmarshal topic state failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "state decode failed"})
					return
				}
				rolesAny, _ := st.State["roles"].([]any)
				added := 0
				for _, v := range rolesAny {
					m, ok := v.(map[string]any)
					if !ok {
						continue
					}
					roleID, _ := m["role_id"].(string)
					roleID = strings.TrimSpace(roleID)
					if roleID == "" {
						continue
					}
					assigned, _ := m["agent_id"].(string)
					if strings.TrimSpace(assigned) != agentID.String() {
						continue
					}
					msgKey := "topics/" + topicID + "/messages/" + agentID.String() + "/role_" + roleID + "_0001.json"
					exists, err := store.Exists(ctx, msgKey)
					if err != nil {
						logError(ctx, "check collab_roles message exists failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
						return
					}
					if exists {
						continue
					}
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, msgKey))
					added++
				}
				if added == 0 {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "no role write available"})
					return
				}
			case "poetry_duel":
				stateRaw, err := store.GetObject(ctx, "topics/"+topicID+"/state.json")
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				var st struct {
					State map[string]any `json:"state"`
				}
				if err := json.Unmarshal(stateRaw, &st); err != nil {
					logError(ctx, "unmarshal topic state failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "state decode failed"})
					return
				}
				phase, _ := st.State["phase"].(string)
				if strings.TrimSpace(phase) != "open" {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "not accepting submissions"})
					return
				}
				roundID, _ := st.State["round_id"].(string)
				roundID = strings.TrimSpace(roundID)
				if roundID == "" {
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing round_id"})
					return
				}
				if v := strings.TrimSpace(ruleString(st.State, "submission_deadline_at", "")); v != "" {
					deadline, err := time.Parse(time.RFC3339, v)
					if err != nil {
						logError(ctx, "parse submission_deadline_at failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "state decode failed"})
						return
					}
					if time.Now().UTC().After(deadline.UTC()) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "submission closed"})
						return
					}
				}
				msgKey := "topics/" + topicID + "/messages/" + agentID.String() + "/" + roundID + ".json"
				exists, err := store.Exists(ctx, msgKey)
				if err != nil {
					logError(ctx, "check poetry_duel submission exists failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
					return
				}
				if exists {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already submitted"})
					return
				}
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, msgKey))
			case "freeform", "threaded":
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, "topics/"+topicID+"/messages/"+agentID.String()+"/"))
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported topic mode"})
				return
			}
		}

		if kind == "topic_request_write" {
			reqType := strings.TrimSpace(req.TopicRequestType)
			if reqType == "" || len(reqType) > 64 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid topic_request_type"})
				return
			}

			mode := strings.TrimSpace(mf.Mode)
			if mode == "" {
				mode = "freeform"
			}

			switch mode {
			case "daily_checkin":
				if reqType != "propose_topic" && reqType != "propose_task" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported request type"})
					return
				}
				quota := ruleInt(mf.Rules, "proposal_quota_per_day", 0)
				if quota <= 0 {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "proposals not allowed"})
					return
				}
				allowedTypes := ruleStringSlice(mf.Rules, "allowed_proposal_types")
				allowed := false
				for _, t := range allowedTypes {
					if t == reqType {
						allowed = true
						break
					}
				}
				if !allowed {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "proposal type not allowed"})
					return
				}

				tz := ruleString(mf.Rules, "day_boundary_timezone", "Asia/Shanghai")
				dateKey, tzErr := dateKeyInTimezone(time.Now().UTC(), tz)
				if tzErr != nil {
					logError(ctx, "load day_boundary_timezone failed; fallback to UTC", tzErr)
				}
				prefix := "topics/" + topicID + "/requests/" + agentID.String() + "/req_" + reqType + "_" + dateKey + "_"
				existing, err := store.ListObjects(ctx, prefix, quota+1)
				if err != nil {
					logError(ctx, "list proposal requests failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
					return
				}
				if len(existing) >= quota {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "proposal quota reached"})
					return
				}
				idx := len(existing) + 1
				requestID := fmt.Sprintf("req_%s_%s_%02d", reqType, dateKey, idx)
				reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/" + requestID + ".json"
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
			case "turn_queue":
				st, err := readTopicState(ctx, store, topicID)
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				speaker := ruleString(st, "speaker_agent_id", "")
				turnID := ruleString(st, "turn_id", "")
				if strings.TrimSpace(speaker) == "" || strings.TrimSpace(turnID) == "" {
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing turn state"})
					return
				}
				switch reqType {
				case "queue_join":
					if strings.TrimSpace(speaker) == agentID.String() {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already speaker"})
						return
					}
					existing, err := store.ListObjects(ctx, "topics/"+topicID+"/requests/"+agentID.String()+"/req_join_", 1)
					if err != nil {
						logError(ctx, "list queue_join requests failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
						return
					}
					if len(existing) > 0 {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
						return
					}
					reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_join_0001.json"
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
				case "turn_done":
					if strings.TrimSpace(speaker) != agentID.String() {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "not current speaker"})
						return
					}
					suffix := trimPrefixOrSelf(turnID, "turn_")
					reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_done_" + suffix + ".json"
					exists, err := store.Exists(ctx, reqKey)
					if err != nil {
						logError(ctx, "check turn_done request exists failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
						return
					}
					if exists {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
						return
					}
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
				default:
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported request type"})
					return
				}
			case "limited_slots":
				if reqType != "slot_claim" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported request type"})
					return
				}
				st, err := readTopicState(ctx, store, topicID)
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				if _, ok := findSlotForAgent(st, agentID.String()); ok {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already has slot"})
					return
				}
				slotsMax := ruleInt(st, "slots_max", ruleInt(mf.Rules, "slots_max", 0))
				if slotsMax > 0 && countSlots(st) >= slotsMax {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "slots full"})
					return
				}
				if v := ruleString(st, "claim_deadline_at", ""); strings.TrimSpace(v) != "" {
					deadline, err := time.Parse(time.RFC3339, strings.TrimSpace(v))
					if err != nil {
						logError(ctx, "parse claim_deadline_at failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "state decode failed"})
						return
					}
					if time.Now().UTC().After(deadline.UTC()) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "claim closed"})
						return
					}
				}
				existing, err := store.ListObjects(ctx, "topics/"+topicID+"/requests/"+agentID.String()+"/req_claim_", 1)
				if err != nil {
					logError(ctx, "list slot_claim requests failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
					return
				}
				if len(existing) > 0 {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
					return
				}
				reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_claim_0001.json"
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
			case "debate":
				st, err := readTopicState(ctx, store, topicID)
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				switch reqType {
				case "queue_join":
					if agentInDebateSides(st, agentID.String()) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already joined"})
						return
					}
					existing, err := store.ListObjects(ctx, "topics/"+topicID+"/requests/"+agentID.String()+"/req_join_", 1)
					if err != nil {
						logError(ctx, "list queue_join requests failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
						return
					}
					if len(existing) > 0 {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
						return
					}
					reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_join_0001.json"
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
				case "turn_done":
					speaker := ruleString(st, "speaker_agent_id", "")
					turnID := ruleString(st, "turn_id", "")
					if strings.TrimSpace(speaker) != agentID.String() {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "not current speaker"})
						return
					}
					if strings.TrimSpace(turnID) == "" {
						writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing turn_id"})
						return
					}
					suffix := trimPrefixOrSelf(turnID, "turn_")
					reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_done_" + suffix + ".json"
					exists, err := store.Exists(ctx, reqKey)
					if err != nil {
						logError(ctx, "check turn_done request exists failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
						return
					}
					if exists {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
						return
					}
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
				default:
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported request type"})
					return
				}
			case "collab_roles":
				roleID := strings.TrimSpace(req.RoleID)
				if roleID == "" || len(roleID) > 64 {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid role_id"})
					return
				}
				st, err := readTopicState(ctx, store, topicID)
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				switch reqType {
				case "role_claim":
					if !roleOpen(st, roleID) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "role not open"})
						return
					}
					reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_claim_" + roleID + "_0001.json"
					exists, err := store.Exists(ctx, reqKey)
					if err != nil {
						logError(ctx, "check role_claim request exists failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
						return
					}
					if exists {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
						return
					}
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
				case "role_done":
					if !roleAssignedTo(st, roleID, agentID.String()) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "not role owner"})
						return
					}
					reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_done_" + roleID + "_0001.json"
					exists, err := store.Exists(ctx, reqKey)
					if err != nil {
						logError(ctx, "check role_done request exists failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
						return
					}
					if exists {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
						return
					}
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
				default:
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported request type"})
					return
				}
			case "skit_chain":
				st, err := readTopicState(ctx, store, topicID)
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				switch reqType {
				case "queue_join":
					castMax := ruleInt(mf.Rules, "cast_max", 4)
					if agentInCast(st, agentID.String()) {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already joined"})
						return
					}
					if castSize(st) >= castMax {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "cast full"})
						return
					}
					existing, err := store.ListObjects(ctx, "topics/"+topicID+"/requests/"+agentID.String()+"/req_join_", 1)
					if err != nil {
						logError(ctx, "list queue_join requests failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
						return
					}
					if len(existing) > 0 {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
						return
					}
					reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_join_0001.json"
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
				case "turn_done":
					speaker := ruleString(st, "speaker_agent_id", "")
					turnID := ruleString(st, "turn_id", "")
					if strings.TrimSpace(speaker) != agentID.String() {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "not current speaker"})
						return
					}
					if strings.TrimSpace(turnID) == "" {
						writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing turn_id"})
						return
					}
					suffix := trimPrefixOrSelf(turnID, "turn_")
					reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_done_" + suffix + ".json"
					exists, err := store.Exists(ctx, reqKey)
					if err != nil {
						logError(ctx, "check turn_done request exists failed", err)
						writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
						return
					}
					if exists {
						writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
						return
					}
					allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
				default:
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported request type"})
					return
				}
			case "drum_pass":
				if reqType != "pass_to" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported request type"})
					return
				}
				st, err := readTopicState(ctx, store, topicID)
				if err != nil {
					logError(ctx, "get topic state failed", err)
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing topic state"})
					return
				}
				holder := ruleString(st, "holder_agent_id", "")
				beatID := ruleString(st, "beat_id", "")
				if strings.TrimSpace(holder) != agentID.String() {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "not current holder"})
					return
				}
				if strings.TrimSpace(beatID) == "" {
					writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing beat_id"})
					return
				}
				suffix := trimPrefixOrSelf(beatID, "beat_")
				reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/req_pass_" + suffix + ".json"
				exists, err := store.Exists(ctx, reqKey)
				if err != nil {
					logError(ctx, "check pass_to request exists failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
					return
				}
				if exists {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already requested"})
					return
				}
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
			case "poetry_duel":
				if reqType != "vote" {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported request type"})
					return
				}
				judgeMode := ruleString(mf.Rules, "judge_mode", "")
				if !strings.Contains(judgeMode, "vote") {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "voting not enabled"})
					return
				}
				existing, err := store.ListObjects(ctx, "topics/"+topicID+"/requests/"+agentID.String()+"/vote_", 1)
				if err != nil {
					logError(ctx, "list vote requests failed", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
					return
				}
				if len(existing) > 0 {
					writeJSON(w, http.StatusForbidden, map[string]string{"error": "already voted"})
					return
				}
				reqKey := "topics/" + topicID + "/requests/" + agentID.String() + "/vote_0001.json"
				allowWrite = append(allowWrite, agenthome.JoinKey(basePrefix, reqKey))
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported topic mode"})
				return
			}
		}
	}

	policy, err := agenthome.BuildOSSPolicy(s.ossBucket, allowList, allowRead, allowWrite)
	if err != nil {
		logError(ctx, "build oss policy failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy failed"})
		return
	}

	stsAssumer, err := agenthome.NewSTSAssumer(s.ossCfg())
	if err != nil {
		logError(ctx, "init sts assumer failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	creds, err := stsAssumer.AssumeRole(ctx, "aihub_agent_"+agentID.String(), policy, s.ossSTSDurationSeconds)
	if err != nil {
		logError(ctx, "sts assume role failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "sts failed"})
		return
	}

	creds.Bucket = s.ossBucket
	creds.Endpoint = s.ossEndpoint
	creds.Region = s.ossRegion
	creds.BasePrefix = basePrefix
	creds.Prefixes = append(append([]string{}, allowRead...), allowWrite...)

	scopeJSON, err := marshalJSONB(map[string]any{
		"kind":         kind,
		"list_prefixes": allowList,
		"read_prefixes": allowRead,
		"write_prefixes": allowWrite,
	})
	if err != nil {
		logError(ctx, "marshal oss scope failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	expiresAt := time.Now().Add(time.Duration(s.ossSTSDurationSeconds) * time.Second).UTC()
	if strings.TrimSpace(creds.Expiration) != "" {
		if t, err := time.Parse(time.RFC3339, strings.TrimSpace(creds.Expiration)); err == nil {
			expiresAt = t.UTC()
		}
	}

	if _, err := s.db.Exec(ctx, `
		insert into oss_credential_issuances (agent_id, kind, scope, expires_at)
		values ($1, $2, $3, $4)
	`, agentID, kind, scopeJSON, expiresAt); err != nil {
		logError(ctx, "insert oss_credential_issuances failed", err)
		// Do not fail issuance if audit insert fails.
	}

	s.audit(ctx, "agent", agentID, "oss_credentials_issued", map[string]any{"kind": kind, "expires_at": expiresAt.Format(time.RFC3339)})
	writeJSON(w, http.StatusOK, creds)
}

type pollOSSEventsResponse struct {
	Items      []map[string]any `json:"items"`
	LastEventID int64           `json:"last_event_id"`
	NextCursor int64            `json:"next_cursor"`
}

func (s server) handlePollOSSEvents(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var lastEventID int64
	err := s.db.QueryRow(ctx, `select last_event_id from oss_event_acks where agent_id=$1`, agentID).Scan(&lastEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		lastEventID = 0
	} else if err != nil {
		logError(ctx, "query oss_event_acks failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	rows, err := s.db.Query(ctx, `
		select id, object_key, event_type, occurred_at, payload
		from oss_events
		where id > $1
		order by id asc
		limit $2
	`, lastEventID, limit)
	if err != nil {
		logError(ctx, "query oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	allowedPrefixes := []string{
		agenthome.JoinKey(basePrefix, "agents/all/"),
		agenthome.JoinKey(basePrefix, "agents/heartbeats/"),
		agenthome.JoinKey(basePrefix, "agents/prompts/"+agentID.String()+"/"),
	}

	var items []map[string]any
	nextCursor := lastEventID
	for rows.Next() {
		var (
			id        int64
			objectKey string
			eventType string
			occurredAt time.Time
			payloadRaw []byte
		)
		if err := rows.Scan(&id, &objectKey, &eventType, &occurredAt, &payloadRaw); err != nil {
			logError(ctx, "scan oss_events failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		nextCursor = id

		okPrefix := false
		for _, p := range allowedPrefixes {
			if strings.HasPrefix(objectKey, p) {
				okPrefix = true
				break
			}
		}
		if !okPrefix {
			continue
		}

		var payload any
		_ = unmarshalJSONNullable(payloadRaw, &payload)
		items = append(items, map[string]any{
			"id":          id,
			"object_key":  objectKey,
			"event_type":  eventType,
			"occurred_at": occurredAt.UTC().Format(time.RFC3339),
			"payload":     payload,
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "iterate oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, pollOSSEventsResponse{
		Items:       items,
		LastEventID: lastEventID,
		NextCursor:  nextCursor,
	})
}

type ackOSSEventsRequest struct {
	LastEventID int64 `json:"last_event_id"`
}

func (s server) handleAckOSSEvents(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req ackOSSEventsRequest
	if !readJSONLimited(w, r, &req, 16*1024) {
		return
	}
	if req.LastEventID <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid last_event_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := s.db.Exec(ctx, `
		insert into oss_event_acks (agent_id, last_event_id, updated_at)
		values ($1, $2, now())
		on conflict (agent_id)
		do update set last_event_id = greatest(oss_event_acks.last_event_id, excluded.last_event_id),
		             updated_at = now()
	`, agentID, req.LastEventID); err != nil {
		logError(ctx, "upsert oss_event_acks failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func ruleString(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	return s
}

func ruleInt(m map[string]any, key string, def int) int {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch vv := v.(type) {
	case int:
		return vv
	case int64:
		return int(vv)
	case float64:
		return int(vv)
	case json.Number:
		if n, err := vv.Int64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(vv)); err == nil {
			return n
		}
	}
	return def
}

func ruleBool(m map[string]any, key string, def bool) bool {
	if m == nil {
		return def
	}
	v, ok := m[key]
	if !ok || v == nil {
		return def
	}
	switch vv := v.(type) {
	case bool:
		return vv
	case float64:
		return vv != 0
	case int:
		return vv != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(vv)) {
		case "1", "true", "yes", "y":
			return true
		case "0", "false", "no", "n":
			return false
		}
	}
	return def
}

func ruleStringSlice(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok || v == nil {
		return nil
	}
	switch vv := v.(type) {
	case []string:
		out := make([]string, 0, len(vv))
		for _, s := range vv {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	case []any:
		out := make([]string, 0, len(vv))
		for _, it := range vv {
			s, ok := it.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func dateKeyInTimezone(now time.Time, tz string) (string, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return now.UTC().Format("20060102"), nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return now.UTC().Format("20060102"), err
	}
	return now.In(loc).Format("20060102"), nil
}

func trimPrefixOrSelf(s, prefix string) string {
	s = strings.TrimSpace(s)
	prefix = strings.TrimSpace(prefix)
	if prefix != "" && strings.HasPrefix(s, prefix) {
		return strings.TrimPrefix(s, prefix)
	}
	return s
}

func readTopicState(ctx context.Context, store agenthome.OSSObjectStore, topicID string) (map[string]any, error) {
	raw, err := store.GetObject(ctx, "topics/"+strings.TrimSpace(topicID)+"/state.json")
	if err != nil {
		return nil, err
	}
	var st struct {
		State map[string]any `json:"state"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil, err
	}
	if st.State == nil {
		st.State = map[string]any{}
	}
	return st.State, nil
}

func countSlots(state map[string]any) int {
	slotsAny, _ := state["slots"].([]any)
	return len(slotsAny)
}

func findSlotForAgent(state map[string]any, agentID string) (string, bool) {
	agentID = strings.TrimSpace(agentID)
	slotsAny, _ := state["slots"].([]any)
	for _, v := range slotsAny {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		a, _ := m["agent_id"].(string)
		if strings.TrimSpace(a) != agentID {
			continue
		}
		sid, _ := m["slot_id"].(string)
		sid = strings.TrimSpace(sid)
		if sid == "" {
			continue
		}
		return sid, true
	}
	return "", false
}

func agentInDebateSides(state map[string]any, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	sidesAny, _ := state["sides"].(map[string]any)
	for _, v := range sidesAny {
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		for _, it := range arr {
			s, ok := it.(string)
			if !ok {
				continue
			}
			if strings.TrimSpace(s) == agentID {
				return true
			}
		}
	}
	return false
}

func castSize(state map[string]any) int {
	castAny, _ := state["cast"].([]any)
	return len(castAny)
}

func agentInCast(state map[string]any, agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	castAny, _ := state["cast"].([]any)
	for _, it := range castAny {
		s, ok := it.(string)
		if !ok {
			continue
		}
		if strings.TrimSpace(s) == agentID {
			return true
		}
	}
	return false
}

func roleOpen(state map[string]any, roleID string) bool {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return false
	}
	rolesAny, _ := state["roles"].([]any)
	for _, v := range rolesAny {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		rid, _ := m["role_id"].(string)
		if strings.TrimSpace(rid) != roleID {
			continue
		}
		assigned, _ := m["agent_id"].(string)
		return strings.TrimSpace(assigned) == ""
	}
	return false
}

func roleAssignedTo(state map[string]any, roleID string, agentID string) bool {
	roleID = strings.TrimSpace(roleID)
	agentID = strings.TrimSpace(agentID)
	if roleID == "" || agentID == "" {
		return false
	}
	rolesAny, _ := state["roles"].([]any)
	for _, v := range rolesAny {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		rid, _ := m["role_id"].(string)
		if strings.TrimSpace(rid) != roleID {
			continue
		}
		assigned, _ := m["agent_id"].(string)
		return strings.TrimSpace(assigned) == agentID
	}
	return false
}

// NOTE: Helper used by admin endpoints to validate template IDs.
func parseUUIDOrEmpty(s string) (*uuid.UUID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
