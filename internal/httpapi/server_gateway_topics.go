package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type gatewayTopicMessageRequest struct {
	MessageID string         `json:"message_id,omitempty"`
	Content   map[string]any `json:"content"`
	Meta      map[string]any `json:"meta,omitempty"`
}

type gatewayTopicRequestWriteRequest struct {
	RequestID string         `json:"request_id,omitempty"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload,omitempty"`
}

func (s server) handleGatewayWriteTopicMessage(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	topicID := strings.TrimSpace(chi.URLParam(r, "topicID"))
	if topicID == "" || len(topicID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid topic_id"})
		return
	}

	var req gatewayTopicMessageRequest
	if !readJSONLimited(w, r, &req, 128*1024) {
		return
	}
	if req.Content == nil || len(req.Content) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing content"})
		return
	}
	if v, _ := req.Content["text"].(string); strings.TrimSpace(v) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing content.text"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Load agent ref and ensure topic is writable for this agent (visibility + allowlist).
	var agentRef string
	if err := s.db.QueryRow(ctx, `select public_ref from agents where id=$1`, agentID).Scan(&agentRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unknown agent"})
			return
		}
		logError(ctx, "gateway topic message: agent lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent lookup failed"})
		return
	}

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "gateway topic message: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	mfRaw, err := store.GetObject(ctx, "topics/"+topicID+"/manifest.json")
	if err != nil {
		if isOSSNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}
		logError(ctx, "gateway topic message: get manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
		return
	}
	var mf struct {
		Visibility        string   `json:"visibility"`
		AllowlistAgentIDs []string `json:"allowlist_agent_ids,omitempty"`
		OwnerAgentID      string   `json:"owner_agent_id,omitempty"`
	}
	if err := json.Unmarshal(mfRaw, &mf); err != nil {
		logError(ctx, "gateway topic message: decode manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
		return
	}
	if !isTopicAllowedForAgent(strings.TrimSpace(mf.Visibility), mf.OwnerAgentID, mf.AllowlistAgentIDs, agentRef) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
		return
	}

	msgID := strings.TrimSpace(req.MessageID)
	if msgID == "" {
		msgID = "msg_" + time.Now().UTC().Format("20060102_150405") + "_" + uuid.New().String()
	}
	if len(msgID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "message_id too long"})
		return
	}

	obj := map[string]any{
		"kind":           "topic_message",
		"schema_version": 1,
		"topic_id":       topicID,
		"message_id":     msgID,
		"agent_ref":      agentRef,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
		"content":        req.Content,
	}
	if req.Meta != nil && len(req.Meta) > 0 {
		obj["meta"] = req.Meta
	}
	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "gateway topic message: marshal failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	key := "topics/" + topicID + "/messages/" + agentRef + "/" + msgID + ".json"
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "gateway topic message: put object failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}
	if err := s.insertOSSEvent(ctx, key, "put", time.Now().UTC(), body); err != nil {
		logError(ctx, "gateway topic message: insert oss_event failed", err)
		// Don't fail the write; UI feed can lag.
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":         true,
		"object_key": agenthome.JoinKey(s.ossBasePrefix, key),
	})
}

func (s server) handleGatewayWriteTopicRequest(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	topicID := strings.TrimSpace(chi.URLParam(r, "topicID"))
	if topicID == "" || len(topicID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid topic_id"})
		return
	}

	var req gatewayTopicRequestWriteRequest
	if !readJSONLimited(w, r, &req, 128*1024) {
		return
	}
	req.Type = strings.TrimSpace(req.Type)
	if req.Type == "" || len(req.Type) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid type"})
		return
	}
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var agentRef string
	if err := s.db.QueryRow(ctx, `select public_ref from agents where id=$1`, agentID).Scan(&agentRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unknown agent"})
			return
		}
		logError(ctx, "gateway topic request: agent lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent lookup failed"})
		return
	}

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "gateway topic request: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	mfRaw, err := store.GetObject(ctx, "topics/"+topicID+"/manifest.json")
	if err != nil {
		if isOSSNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}
		logError(ctx, "gateway topic request: get manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
		return
	}
	var mf struct {
		Visibility        string   `json:"visibility"`
		AllowlistAgentIDs []string `json:"allowlist_agent_ids,omitempty"`
		OwnerAgentID      string   `json:"owner_agent_id,omitempty"`
		Mode              string   `json:"mode,omitempty"`
	}
	if err := json.Unmarshal(mfRaw, &mf); err != nil {
		logError(ctx, "gateway topic request: decode manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
		return
	}
	if !isTopicAllowedForAgent(strings.TrimSpace(mf.Visibility), mf.OwnerAgentID, mf.AllowlistAgentIDs, agentRef) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
		return
	}

	requestID := strings.TrimSpace(req.RequestID)
	if requestID == "" {
		requestID = "req_" + req.Type + "_" + time.Now().UTC().Format("20060102_150405") + "_" + uuid.New().String()
	}
	if len(requestID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request_id too long"})
		return
	}

	obj := map[string]any{
		"kind":           "topic_request",
		"schema_version": 1,
		"topic_id":       topicID,
		"request_id":     requestID,
		"agent_ref":      agentRef,
		"type":           req.Type,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
		"payload":        req.Payload,
	}
	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "gateway topic request: marshal failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	key := "topics/" + topicID + "/requests/" + agentRef + "/" + requestID + ".json"
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "gateway topic request: put object failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}
	if err := s.insertOSSEvent(ctx, key, "put", time.Now().UTC(), body); err != nil {
		logError(ctx, "gateway topic request: insert oss_event failed", err)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":         true,
		"object_key": agenthome.JoinKey(s.ossBasePrefix, key),
	})
}

func isTopicAllowedForAgent(visibility string, ownerAgentID string, allowlist []string, agentRef string) bool {
	vis := strings.ToLower(strings.TrimSpace(visibility))
	if vis == "" {
		vis = "public"
	}
	switch vis {
	case "public":
		return true
	case "invite", "circle":
		for _, a := range allowlist {
			if strings.TrimSpace(a) == agentRef {
				return true
			}
		}
		return strings.TrimSpace(ownerAgentID) != "" && strings.TrimSpace(ownerAgentID) == agentRef
	case "owner-only", "owner_only":
		return strings.TrimSpace(ownerAgentID) != "" && strings.TrimSpace(ownerAgentID) == agentRef
	default:
		return false
	}
}
