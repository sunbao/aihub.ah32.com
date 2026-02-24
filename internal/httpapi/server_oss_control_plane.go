package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type adminCreateCircleRequest struct {
	CircleID     string `json:"circle_id,omitempty"`
	Title        string `json:"title"`
	Summary      string `json:"summary,omitempty"`
	Visibility   string `json:"visibility,omitempty"` // public|invite|circle (meaning in-registry, not anonymous web)
	OwnerAgentID string `json:"owner_agent_id"`
}

func (s server) handleAdminCreateCircle(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	var req adminCreateCircleRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}

	circleID := strings.TrimSpace(req.CircleID)
	if circleID == "" {
		circleID = "circle_" + uuid.New().String()
	}
	if len(circleID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "circle_id too long"})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid title"})
		return
	}
	summary := strings.TrimSpace(req.Summary)
	if len(summary) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "summary too long"})
		return
	}

	ownerAgentID := strings.TrimSpace(req.OwnerAgentID)
	if _, err := uuid.Parse(ownerAgentID); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid owner_agent_id"})
		return
	}

	visibility := strings.TrimSpace(req.Visibility)
	if visibility == "" {
		visibility = "circle"
	}
	switch visibility {
	case "public", "invite", "circle":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid visibility"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	obj := map[string]any{
		"kind":          "circle_manifest",
		"schema_version": 1,
		"circle_id":     circleID,
		"title":         title,
		"summary":       summary,
		"visibility":    visibility,
		"owner_agent_id": ownerAgentID,
		"policy_version": 1,
		"created_at":    time.Now().UTC().Format(time.RFC3339),
	}
	cert, err := s.signObject(ctx, obj)
	if err != nil {
		logError(ctx, "sign circle manifest failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
		return
	}
	obj["cert"] = cert

	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "marshal circle manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	manifestKey := "circles/" + circleID + "/manifest.json"
	if err := store.PutObject(ctx, manifestKey, "application/json", body); err != nil {
		logError(ctx, "put circle manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	// Ensure owner is a member.
	memberObj := map[string]any{
		"kind":           "circle_member",
		"schema_version": 1,
		"circle_id":      circleID,
		"agent_id":       ownerAgentID,
		"joined_at":      time.Now().UTC().Format(time.RFC3339),
	}
	memberCert, err := s.signObject(ctx, memberObj)
	if err != nil {
		logError(ctx, "sign circle member failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
		return
	}
	memberObj["cert"] = memberCert
	memberBody, err := json.Marshal(memberObj)
	if err != nil {
		logError(ctx, "marshal circle member failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	memberKey := "circles/" + circleID + "/members/" + ownerAgentID + ".json"
	if err := store.PutObject(ctx, memberKey, "application/json", memberBody); err != nil {
		logError(ctx, "put circle owner member failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"circle_id": circleID,
		"manifest_key": agenthome.JoinKey(s.ossBasePrefix, manifestKey),
	})
}

func (s server) handleAdminProcessCircleJoins(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}
	circleID := strings.TrimSpace(chi.URLParam(r, "circleID"))
	if circleID == "" || len(circleID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid circle id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	manifestRaw, err := store.GetObject(ctx, "circles/"+circleID+"/manifest.json")
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
	if owner == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "circle has no owner"})
		return
	}

	joinPrefix := "circles/" + circleID + "/join_requests/"
	keys, err := store.ListObjects(ctx, joinPrefix, 1000)
	if err != nil {
		logError(ctx, "list join_requests failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss list failed"})
		return
	}

	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	basePrefixSlash := ""
	if basePrefix != "" {
		basePrefixSlash = basePrefix + "/"
	}

	added := 0
	for _, fullKey := range keys {
		key := strings.TrimPrefix(fullKey, basePrefixSlash)
		if !strings.HasPrefix(key, joinPrefix) {
			continue
		}
		name := strings.TrimPrefix(key, joinPrefix)
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		requestAgentID := strings.TrimSuffix(name, ".json")
		if requestAgentID == "" {
			continue
		}

		memberKey := "circles/" + circleID + "/members/" + requestAgentID + ".json"
		exists, err := store.Exists(ctx, memberKey)
		if err != nil {
			logError(ctx, "check member exists failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
			return
		}
		if exists {
			continue
		}

		approvalKey := "circles/" + circleID + "/join_approvals/" + requestAgentID + "/" + owner + ".json"
		approved, err := store.Exists(ctx, approvalKey)
		if err != nil {
			logError(ctx, "check approval exists failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss check failed"})
			return
		}
		if !approved {
			continue
		}

		memberObj := map[string]any{
			"kind":           "circle_member",
			"schema_version": 1,
			"circle_id":      circleID,
			"agent_id":       requestAgentID,
			"joined_at":      time.Now().UTC().Format(time.RFC3339),
			"approved_by":    owner,
		}
		cert, err := s.signObject(ctx, memberObj)
		if err != nil {
			logError(ctx, "sign circle member failed", err)
			writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
			return
		}
		memberObj["cert"] = cert
		body, err := json.Marshal(memberObj)
		if err != nil {
			logError(ctx, "marshal circle member failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
			return
		}
		if err := store.PutObject(ctx, memberKey, "application/json", body); err != nil {
			logError(ctx, "put circle member failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
			return
		}
		added++
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "added": added})
}

type adminCreateTaskManifestRequest struct {
	TaskID         string   `json:"task_id,omitempty"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary,omitempty"`
	Visibility     string   `json:"visibility"` // public|circle|invite|owner-only
	CircleID       string   `json:"circle_id,omitempty"`
	InviteAgentIDs []string `json:"invite_agent_ids,omitempty"`
	OwnerAgentID   string   `json:"owner_agent_id,omitempty"`
}

func (s server) handleAdminCreateTaskManifest(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	var req adminCreateTaskManifestRequest
	if !readJSONLimited(w, r, &req, 128*1024) {
		return
	}

	taskID := strings.TrimSpace(req.TaskID)
	if taskID == "" {
		taskID = "task_" + uuid.New().String()
	}
	if len(taskID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task_id too long"})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid title"})
		return
	}
	summary := strings.TrimSpace(req.Summary)
	if len(summary) > 4000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "summary too long"})
		return
	}

	visibility := strings.TrimSpace(req.Visibility)
	switch visibility {
	case "public", "circle", "invite", "owner-only", "owner_only":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid visibility"})
		return
	}

	if visibility == "circle" && strings.TrimSpace(req.CircleID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing circle_id"})
		return
	}
	if visibility == "invite" && len(req.InviteAgentIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing invite_agent_ids"})
		return
	}
	if (visibility == "owner-only" || visibility == "owner_only") && strings.TrimSpace(req.OwnerAgentID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing owner_agent_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	obj := map[string]any{
		"kind":           "task_manifest",
		"schema_version": 1,
		"task_id":        taskID,
		"title":          title,
		"summary":        summary,
		"visibility":     visibility,
		"circle_id":      strings.TrimSpace(req.CircleID),
		"invite_agent_ids": req.InviteAgentIDs,
		"owner_agent_id": strings.TrimSpace(req.OwnerAgentID),
		"policy_version": 1,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
	}
	cert, err := s.signObject(ctx, obj)
	if err != nil {
		logError(ctx, "sign task manifest failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
		return
	}
	obj["cert"] = cert

	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "marshal task manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	manifestKey := "tasks/" + taskID + "/manifest.json"
	if err := store.PutObject(ctx, manifestKey, "application/json", body); err != nil {
		logError(ctx, "put task manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"task_id":      taskID,
		"manifest_key": agenthome.JoinKey(s.ossBasePrefix, manifestKey),
	})
}

type adminCreateTopicManifestRequest struct {
	TopicID           string         `json:"topic_id,omitempty"`
	Title             string         `json:"title"`
	Summary           string         `json:"summary,omitempty"`
	Visibility        string         `json:"visibility"` // public|circle|invite|owner-only
	CircleID          string         `json:"circle_id,omitempty"`
	AllowlistAgentIDs []string       `json:"allowlist_agent_ids,omitempty"`
	OwnerAgentID      string         `json:"owner_agent_id,omitempty"`
	Mode              string         `json:"mode"` // intro_once|daily_checkin|freeform|threaded|turn_queue|limited_slots|debate|collab_roles|roast_banter|crosstalk|skit_chain|drum_pass|idiom_chain|poetry_duel
	Rules             map[string]any `json:"rules,omitempty"`
	InitialState      map[string]any `json:"initial_state,omitempty"`
}

func (s server) handleAdminCreateTopicManifest(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	var req adminCreateTopicManifestRequest
	if !readJSONLimited(w, r, &req, 256*1024) {
		return
	}

	topicID := strings.TrimSpace(req.TopicID)
	if topicID == "" {
		topicID = "topic_" + uuid.New().String()
	}
	if len(topicID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "topic_id too long"})
		return
	}

	title := strings.TrimSpace(req.Title)
	if title == "" || len(title) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid title"})
		return
	}
	summary := strings.TrimSpace(req.Summary)
	if len(summary) > 4000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "summary too long"})
		return
	}

	visibility := strings.TrimSpace(req.Visibility)
	switch visibility {
	case "public", "circle", "invite", "owner-only", "owner_only":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid visibility"})
		return
	}
	if visibility == "circle" && strings.TrimSpace(req.CircleID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing circle_id"})
		return
	}
	if visibility == "invite" && len(req.AllowlistAgentIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing allowlist_agent_ids"})
		return
	}
	if (visibility == "owner-only" || visibility == "owner_only") && strings.TrimSpace(req.OwnerAgentID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing owner_agent_id"})
		return
	}

	mode := strings.TrimSpace(req.Mode)
	switch mode {
	case "intro_once", "daily_checkin", "freeform", "threaded", "turn_queue", "limited_slots", "debate", "collab_roles",
		"roast_banter", "crosstalk", "skit_chain", "drum_pass", "idiom_chain", "poetry_duel":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid mode"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	manifest := map[string]any{
		"kind":            "topic_manifest",
		"schema_version":  1,
		"topic_id":        topicID,
		"title":           title,
		"summary":         summary,
		"visibility":      visibility,
		"circle_id":       strings.TrimSpace(req.CircleID),
		"allowlist_agent_ids": req.AllowlistAgentIDs,
		"owner_agent_id":  strings.TrimSpace(req.OwnerAgentID),
		"mode":            mode,
		"rules":           req.Rules,
		"policy_version":  1,
		"created_at":      time.Now().UTC().Format(time.RFC3339),
	}
	cert, err := s.signObject(ctx, manifest)
	if err != nil {
		logError(ctx, "sign topic manifest failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
		return
	}
	manifest["cert"] = cert

	body, err := json.Marshal(manifest)
	if err != nil {
		logError(ctx, "marshal topic manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	manifestKey := "topics/" + topicID + "/manifest.json"
	if err := store.PutObject(ctx, manifestKey, "application/json", body); err != nil {
		logError(ctx, "put topic manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	// Optional initial state for coordination modes.
	state := req.InitialState
	if state == nil {
		state = map[string]any{}
	}
	stateObj := map[string]any{
		"kind":           "topic_state",
		"schema_version": 1,
		"topic_id":       topicID,
		"mode":           mode,
		"state":          state,
		"updated_at":     time.Now().UTC().Format(time.RFC3339),
	}
	stateCert, err := s.signObject(ctx, stateObj)
	if err != nil {
		logError(ctx, "sign topic state failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
		return
	}
	stateObj["cert"] = stateCert
	stateBody, err := json.Marshal(stateObj)
	if err != nil {
		logError(ctx, "marshal topic state failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	stateKey := "topics/" + topicID + "/state.json"
	if err := store.PutObject(ctx, stateKey, "application/json", stateBody); err != nil {
		logError(ctx, "put topic state failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"topic_id":      topicID,
		"manifest_key":  agenthome.JoinKey(s.ossBasePrefix, manifestKey),
		"state_key":     agenthome.JoinKey(s.ossBasePrefix, stateKey),
	})
}

type adminUpdateTopicStateRequest struct {
	State map[string]any `json:"state"`
}

func (s server) handleAdminUpdateTopicState(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}
	topicID := strings.TrimSpace(chi.URLParam(r, "topicID"))
	if topicID == "" || len(topicID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid topic id"})
		return
	}

	var req adminUpdateTopicStateRequest
	if !readJSONLimited(w, r, &req, 256*1024) {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	manifestRaw, err := store.GetObject(ctx, "topics/"+topicID+"/manifest.json")
	if err != nil {
		logError(ctx, "get topic manifest failed", err)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
		return
	}
	var mf struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(manifestRaw, &mf); err != nil {
		logError(ctx, "unmarshal topic manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
		return
	}
	mode := strings.TrimSpace(mf.Mode)
	if mode == "" {
		mode = "freeform"
	}

	stateObj := map[string]any{
		"kind":           "topic_state",
		"schema_version": 1,
		"topic_id":       topicID,
		"mode":           mode,
		"state":          req.State,
		"updated_at":     time.Now().UTC().Format(time.RFC3339),
	}
	cert, err := s.signObject(ctx, stateObj)
	if err != nil {
		logError(ctx, "sign topic state failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
		return
	}
	stateObj["cert"] = cert

	body, err := json.Marshal(stateObj)
	if err != nil {
		logError(ctx, "marshal topic state failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	stateKey := "topics/" + topicID + "/state.json"
	if err := store.PutObject(ctx, stateKey, "application/json", body); err != nil {
		logError(ctx, "put topic state failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "state_key": agenthome.JoinKey(s.ossBasePrefix, stateKey)})
}
