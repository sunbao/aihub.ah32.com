package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/go-chi/chi/v5"
)

type topicThreadMessageDTO struct {
	Text      string `json:"text"`
	ActorName string `json:"actor_name,omitempty"`
	Relation  string `json:"relation,omitempty"`
	CreatedAt string `json:"created_at"`

	// For client-side threading (must not be displayed to end users).
	ReplyTo     *topicMessageRef `json:"reply_to,omitempty"`
	ThreadRoot  *topicMessageRef `json:"thread_root,omitempty"`
	MessageID   string           `json:"message_id"`
	ActorRef    string           `json:"actor_ref,omitempty"`
	OccurredAt  string           `json:"occurred_at,omitempty"`
	ObjectKey   string           `json:"-"`
	InternalRef *topicMessageRef `json:"-"`
}

type topicThreadTopicDTO struct {
	TopicID    string `json:"topic_id"`
	Title      string `json:"title"`
	Summary    string `json:"summary,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

type topicThreadResponse struct {
	Topic    topicThreadTopicDTO     `json:"topic"`
	Messages []topicThreadMessageDTO `json:"messages"`
}

func (s server) handleGetTopicThreadPublic(w http.ResponseWriter, r *http.Request) {
	topicID := strings.TrimSpace(chi.URLParam(r, "topicID"))
	if topicID == "" || len(topicID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid topic_id"})
		return
	}

	limit := 200
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = clampInt(n, 1, 500)
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	userID, hasUser, err := s.maybeUserIDFromRequest(ctx, r)
	if err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topic thread: auth lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth lookup failed"})
		return
	}

	ownedAgentRefs := []string{}
	if hasUser {
		refs, err := s.listOwnerAgentRefs(ctx, userID, 80)
		if err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topic thread: list owner agents failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		ownedAgentRefs = refs
	}

	provider := strings.ToLower(strings.TrimSpace(s.ossProvider))
	if provider == "" && strings.TrimSpace(s.ossLocalDir) != "" {
		provider = "local"
	}
	if provider == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	ossCfg := s.ossCfg()
	ossCfg.Provider = provider
	store, err := agenthome.NewOSSObjectStore(ossCfg)
	if err != nil {
		logError(ctx, "topic thread: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	manifestRaw, err := store.GetObject(ctx, "topics/"+topicID+"/manifest.json")
	if err != nil {
		if isOSSNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}
		logError(ctx, "topic thread: get manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
		return
	}
	var mf topicManifestLite
	if err := json.Unmarshal(manifestRaw, &mf); err != nil {
		logError(ctx, "topic thread: decode manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
		return
	}

	args := topicManifestAllowArgs{
		Visibility:        mf.Visibility,
		CircleID:          mf.CircleID,
		AllowlistAgentIDs: mf.AllowlistAgentIDs,
		OwnerAgentID:      mf.OwnerAgentID,
		OwnedAgentRefs:    ownedAgentRefs,
		CandidateAgentRef: "",
	}
	if !hasUser {
		args.OwnedAgentRefs = nil
	}
	if !topicManifestAllowsOwner(ctx, store, args) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
		return
	}

	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	pat1 := "topics/" + topicID + "/messages/%"
	pat2 := pat1
	if basePrefix != "" {
		pat2 = basePrefix + "/" + pat1
	}

	rows, err := s.db.Query(ctx, `
		select object_key, occurred_at, payload
		from oss_events
		where object_key like $1 or object_key like $2
		order by occurred_at desc, id desc
		limit $3
	`, pat1, pat2, limit)
	if err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topic thread: query oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type rawMsg struct {
		occurredAt time.Time
		objectKey  string
		payloadB   []byte
	}
	rawMsgs := make([]rawMsg, 0, limit)
	for rows.Next() {
		var rm rawMsg
		if err := rows.Scan(&rm.objectKey, &rm.occurredAt, &rm.payloadB); err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topic thread: scan oss_event failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		rawMsgs = append(rawMsgs, rm)
	}
	if err := rows.Err(); err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topic thread: iterate oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	// Reverse to chronological (older -> newer) for tree building + reading.
	for i, j := 0, len(rawMsgs)-1; i < j; i, j = i+1, j-1 {
		rawMsgs[i], rawMsgs[j] = rawMsgs[j], rawMsgs[i]
	}

	msgs := make([]topicThreadMessageDTO, 0, len(rawMsgs))
	needRefs := map[string]bool{}
	refs := make([]string, 0, 32)

	for _, rm := range rawMsgs {
		var m map[string]any
		if err := json.Unmarshal(rm.payloadB, &m); err != nil {
			logError(ctx, "topic thread: decode message payload failed", err)
			continue
		}

		agentRef, _ := m["agent_ref"].(string)
		agentRef = strings.ToLower(strings.TrimSpace(agentRef))
		msgID, _ := m["message_id"].(string)
		msgID = strings.TrimSpace(msgID)
		createdAt, _ := m["created_at"].(string)
		createdAt = strings.TrimSpace(createdAt)

		text := strings.TrimSpace(extractTopicMessageTextBestEffort(rm.payloadB))

		var replyTo *topicMessageRef
		var threadRoot *topicMessageRef
		relation := ""
		if strings.TrimSpace(mf.Mode) == "threaded" {
			relation = strings.TrimSpace(extractThreadRelation(rm.payloadB))
		}
		if meta, _ := m["meta"].(map[string]any); meta != nil {
			replyTo = parseTopicMessageRef(meta["reply_to"])
			threadRoot = parseTopicMessageRef(meta["thread_root"])
		}

		if agentRef != "" && !needRefs[agentRef] {
			needRefs[agentRef] = true
			refs = append(refs, agentRef)
		}

		msgs = append(msgs, topicThreadMessageDTO{
			Text:       text,
			ActorName:  "",
			Relation:   relation,
			CreatedAt:  createdAt,
			ReplyTo:    replyTo,
			ThreadRoot: threadRoot,
			MessageID:  msgID,
			ActorRef:   agentRef,
			OccurredAt: rm.occurredAt.UTC().Format(time.RFC3339),
		})
	}

	nameByRef := map[string]string{}
	if len(refs) > 0 {
		m, err := s.lookupAgentNamesByRefs(ctx, refs)
		if err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topic thread: lookup agent names failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		nameByRef = m
	}
	for i := range msgs {
		ref := strings.TrimSpace(msgs[i].ActorRef)
		if ref != "" {
			msgs[i].ActorName = strings.TrimSpace(nameByRef[ref])
		}
		// Keep ActorRef for client threading keys; UI must not render it.
	}

	writeJSON(w, http.StatusOK, topicThreadResponse{
		Topic: topicThreadTopicDTO{
			TopicID:    topicID,
			Title:      strings.TrimSpace(mf.Title),
			Summary:    strings.TrimSpace(mf.Summary),
			Mode:       strings.TrimSpace(mf.Mode),
			Visibility: strings.TrimSpace(mf.Visibility),
		},
		Messages: msgs,
	})
}
