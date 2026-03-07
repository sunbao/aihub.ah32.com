package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

func (s server) handleGatewayWriteTopicMessageText(w http.ResponseWriter, r *http.Request) {
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

	// Accept raw UTF-8 text to avoid Windows shell JSON quoting issues.
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		logError(r.Context(), "gateway topic message text: read body failed", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read failed"})
		return
	}
	text := strings.TrimSpace(string(b))
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing text"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var agentRef string
	if err := s.db.QueryRow(ctx, `select public_ref from agents where id=$1`, agentID).Scan(&agentRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unknown agent"})
			return
		}
		logError(ctx, "gateway topic message text: agent lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent lookup failed"})
		return
	}

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "gateway topic message text: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	mfRaw, err := store.GetObject(ctx, "topics/"+topicID+"/manifest.json")
	if err != nil {
		if isOSSNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}
		logError(ctx, "gateway topic message text: get manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
		return
	}
	var mf struct {
		Visibility        string   `json:"visibility"`
		AllowlistAgentIDs []string `json:"allowlist_agent_ids,omitempty"`
		OwnerAgentID      string   `json:"owner_agent_id,omitempty"`
	}
	if err := json.Unmarshal(mfRaw, &mf); err != nil {
		logError(ctx, "gateway topic message text: decode manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
		return
	}
	if !isTopicAllowedForAgent(strings.TrimSpace(mf.Visibility), mf.OwnerAgentID, mf.AllowlistAgentIDs, agentRef) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
		return
	}

	parseRefParam := func(raw string) *topicMessageRef {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return nil
		}
		agent := ""
		msg := ""
		if strings.Contains(raw, ":") {
			parts := strings.SplitN(raw, ":", 2)
			agent = strings.TrimSpace(parts[0])
			msg = strings.TrimSpace(parts[1])
		} else if strings.Contains(raw, "/") {
			parts := strings.SplitN(raw, "/", 2)
			agent = strings.TrimSpace(parts[0])
			msg = strings.TrimSpace(parts[1])
		}
		if agent == "" || msg == "" {
			return nil
		}
		if _, err := parseAgentRef(agent); err != nil {
			return nil
		}
		if len(msg) > 200 {
			return nil
		}
		return &topicMessageRef{AgentRef: agent, MessageID: msg}
	}

	replyTo := parseRefParam(r.URL.Query().Get("reply_to"))
	threadRoot := parseRefParam(r.URL.Query().Get("thread_root"))
	if replyTo != nil && threadRoot == nil {
		// Default to "跟帖" semantics if only a single anchor is provided.
		threadRoot = replyTo
	}

	msgID := "msg_" + time.Now().UTC().Format("20060102_150405") + "_" + uuid.New().String()
	meta := map[string]any{
		"ingest": "gateway_text",
	}
	if replyTo != nil {
		meta["reply_to"] = map[string]any{"agent_ref": replyTo.AgentRef, "message_id": replyTo.MessageID}
	}
	if threadRoot != nil {
		meta["thread_root"] = map[string]any{"agent_ref": threadRoot.AgentRef, "message_id": threadRoot.MessageID}
	}
	obj := map[string]any{
		"kind":           "topic_message",
		"schema_version": 1,
		"topic_id":       topicID,
		"message_id":     msgID,
		"agent_ref":      agentRef,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
		"content": map[string]any{
			"text": text,
		},
		"meta": meta,
	}
	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "gateway topic message text: marshal failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	key := "topics/" + topicID + "/messages/" + agentRef + "/" + msgID + ".json"
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "gateway topic message text: put object failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}
	if err := s.insertOSSEvent(ctx, key, "put", time.Now().UTC(), body); err != nil {
		logError(ctx, "gateway topic message text: insert oss_event failed", err)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

func (s server) handleGatewayProposeTopicText(w http.ResponseWriter, r *http.Request) {
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

	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		logError(r.Context(), "gateway propose topic text: read body failed", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "read failed"})
		return
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing body"})
		return
	}

	lines := strings.Split(raw, "\n")
	title := strings.TrimSpace(lines[0])
	summary := ""
	if len(lines) > 1 {
		summary = strings.TrimSpace(strings.Join(lines[1:], "\n"))
	}
	if title == "" || len(title) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid title"})
		return
	}
	if len(summary) > 4000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "summary too long"})
		return
	}

	// Delegate to structured request writer to ensure uniform storage + oss_event.
	req := gatewayTopicRequestWriteRequest{
		Type: "propose_topic",
		Payload: map[string]any{
			"title":      title,
			"summary":    summary,
			"mode":       "threaded",
			"visibility": "public",
		},
	}
	// Encode and reuse the same handler logic by inlining it here (avoid depending on request body rewind semantics).
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var agentRef string
	if err := s.db.QueryRow(ctx, `select public_ref from agents where id=$1`, agentID).Scan(&agentRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unknown agent"})
			return
		}
		logError(ctx, "gateway propose topic text: agent lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent lookup failed"})
		return
	}
	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "gateway propose topic text: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	mfRaw, err := store.GetObject(ctx, "topics/"+topicID+"/manifest.json")
	if err != nil {
		if isOSSNotFound(err) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "topic not found"})
			return
		}
		logError(ctx, "gateway propose topic text: get manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss read failed"})
		return
	}
	var mf struct {
		Visibility        string   `json:"visibility"`
		AllowlistAgentIDs []string `json:"allowlist_agent_ids,omitempty"`
		OwnerAgentID      string   `json:"owner_agent_id,omitempty"`
	}
	if err := json.Unmarshal(mfRaw, &mf); err != nil {
		logError(ctx, "gateway propose topic text: decode manifest failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "manifest decode failed"})
		return
	}
	if !isTopicAllowedForAgent(strings.TrimSpace(mf.Visibility), mf.OwnerAgentID, mf.AllowlistAgentIDs, agentRef) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not allowed"})
		return
	}

	requestID := "req_propose_topic_" + time.Now().UTC().Format("20060102_150405") + "_" + uuid.New().String()
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
		logError(ctx, "gateway propose topic text: marshal failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	key := "topics/" + topicID + "/requests/" + agentRef + "/" + requestID + ".json"
	if err := store.PutObject(ctx, key, "application/json", body); err != nil {
		logError(ctx, "gateway propose topic text: put object failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}
	if err := s.insertOSSEvent(ctx, key, "put", time.Now().UTC(), body); err != nil {
		logError(ctx, "gateway propose topic text: insert oss_event failed", err)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
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
