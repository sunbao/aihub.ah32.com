package httpapi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"aihub/internal/keys"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type server struct {
	db     *pgxpool.Pool
	pepper string

	publishMinCompletedWorkItems int
	matchingParticipantCount     int
	workItemLeaseSeconds         int

	br *broker
}

type eventDTO struct {
	RunID     string         `json:"run_id"`
	Seq       int64          `json:"seq"`
	Kind      string         `json:"kind"`
	Persona   string         `json:"persona"`
	Payload   map[string]any `json:"payload"`
	IsKeyNode bool           `json:"is_key_node"`
	CreatedAt string         `json:"created_at"`
}

type eventKind string

const (
	eventMessage         eventKind = "message"
	eventStageChanged    eventKind = "stage_changed"
	eventDecision        eventKind = "decision"
	eventSummary         eventKind = "summary"
	eventArtifactVersion eventKind = "artifact_version"
	eventSystem          eventKind = "system"
)

var allowedEventKinds = map[string]struct{}{
	string(eventMessage):         {},
	string(eventStageChanged):    {},
	string(eventDecision):        {},
	string(eventSummary):         {},
	string(eventArtifactVersion): {},
	string(eventSystem):          {},
}

func isKeyNodeKind(kind string) bool {
	switch kind {
	case string(eventStageChanged), string(eventDecision), string(eventSummary), string(eventArtifactVersion):
		return true
	default:
		return false
	}
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

type ctxKey string

const (
	ctxUserID  ctxKey = "user_id"
	ctxAgentID ctxKey = "agent_id"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func readJSONLimited(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := readJSON(r, dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}
	return true
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func (s server) userAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := bearerToken(r)
		if apiKey == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		hash := keys.HashAPIKey(s.pepper, apiKey)

		var userID uuid.UUID
		err := s.db.QueryRow(r.Context(), `
			select u.id
			from user_api_keys k
			join users u on u.id = k.user_id
			where k.key_hash = $1 and k.revoked_at is null
		`, hash).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth lookup failed"})
			return
		}

		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s server) agentAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := bearerToken(r)
		if apiKey == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		hash := keys.HashAPIKey(s.pepper, apiKey)

		var agentID uuid.UUID
		var status string
		err := s.db.QueryRow(r.Context(), `
			select a.id, a.status
			from agent_api_keys k
			join agents a on a.id = k.agent_id
			where k.key_hash = $1 and k.revoked_at is null
		`, hash).Scan(&agentID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth lookup failed"})
			return
		}
		if status != "enabled" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent disabled"})
			return
		}

		ctx := context.WithValue(r.Context(), ctxAgentID, agentID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(ctxUserID)
	id, ok := v.(uuid.UUID)
	return id, ok
}

func agentIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(ctxAgentID)
	id, ok := v.(uuid.UUID)
	return id, ok
}

func (s server) audit(ctx context.Context, actorType string, actorID uuid.UUID, action string, data map[string]any) {
	// Best-effort for MVP.
	_, _ = s.db.Exec(ctx, `
		insert into audit_logs (actor_type, actor_id, action, data)
		values ($1, $2, $3, $4)
	`, actorType, actorID, action, data)
}

// --- Handlers

type createUserResponse struct {
	UserID string `json:"user_id"`
	APIKey string `json:"api_key"`
}

func (s server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	apiKey, err := keys.NewAPIKey()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key generation failed"})
		return
	}
	hash := keys.HashAPIKey(s.pepper, apiKey)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	if err := tx.QueryRow(ctx, `insert into users default values returning id`).Scan(&userID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create user failed"})
		return
	}
	if _, err := tx.Exec(ctx, `
		insert into user_api_keys (user_id, key_hash)
		values ($1, $2)
	`, userID, hash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create user key failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db commit failed"})
		return
	}

	s.audit(ctx, "user", userID, "user_api_key_issued", map[string]any{})
	writeJSON(w, http.StatusCreated, createUserResponse{UserID: userID.String(), APIKey: apiKey})
}

func (s server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"user_id": userID.String()})
}

type createAgentRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
}

type createAgentResponse struct {
	AgentID    string         `json:"agent_id"`
	APIKey     string         `json:"api_key"`
	Endpoints  map[string]any `json:"endpoints"`
	Onboarding map[string]any `json:"onboarding,omitempty"`
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, t := range tags {
		tt := strings.TrimSpace(t)
		if tt == "" || len(tt) > 64 {
			continue
		}
		if _, ok := seen[tt]; ok {
			continue
		}
		seen[tt] = struct{}{}
		out = append(out, tt)
	}
	return out
}

func splitSearchTerms(q string) []string {
	parts := strings.FieldsFunc(q, func(r rune) bool {
		return unicode.IsSpace(r) || r == ',' || r == '，' || r == ';' || r == '；'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" || len(t) > 64 {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
		if len(out) >= 6 {
			break
		}
	}
	return out
}

func (s server) handleCreateAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req createAgentRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		req.Name = "agent"
	}
	if len(req.Name) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name too long"})
		return
	}
	if len(req.Description) > 512 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description too long"})
		return
	}
	tags := normalizeTags(req.Tags)

	apiKey, err := keys.NewAPIKey()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key generation failed"})
		return
	}
	hash := keys.HashAPIKey(s.pepper, apiKey)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var agentID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into agents (owner_id, name, description, status)
		values ($1, $2, $3, 'enabled')
		returning id
	`, userID, req.Name, req.Description).Scan(&agentID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create agent failed"})
		return
	}

	if _, err := tx.Exec(ctx, `
		insert into agent_api_keys (agent_id, key_hash)
		values ($1, $2)
	`, agentID, hash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create agent key failed"})
		return
	}

	for _, t := range tags {
		if _, err := tx.Exec(ctx, `
			insert into agent_tags (agent_id, tag) values ($1, $2)
			on conflict do nothing
		`, agentID, t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create agent tags failed"})
			return
		}
	}

	// Seed a platform-owned onboarding run + work item, so new owners can satisfy
	// the "先贡献后发布" gate by having their agent complete platform work.
	onboardingRunID, onboardingWorkItemID, err := s.createOnboardingOffer(ctx, tx, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create onboarding offer failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db commit failed"})
		return
	}

	s.audit(ctx, "user", userID, "agent_api_key_issued", map[string]any{"agent_id": agentID.String()})
	writeJSON(w, http.StatusCreated, createAgentResponse{
		AgentID: agentID.String(),
		APIKey:  apiKey,
		Endpoints: map[string]any{
			"poll": "/v1/gateway/inbox/poll",
		},
		Onboarding: map[string]any{
			"run_id":       onboardingRunID.String(),
			"work_item_id": onboardingWorkItemID.String(),
		},
	})
}

var platformUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

func (s server) createOnboardingOffer(ctx context.Context, tx pgx.Tx, agentID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	// Ensure platform/system user exists.
	if _, err := tx.Exec(ctx, `insert into users (id) values ($1) on conflict do nothing`, platformUserID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	var runID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into runs (publisher_user_id, goal, constraints, status)
		values ($1, $2, $3, 'running')
		returning id
	`, platformUserID, "Onboarding: complete a few tasks to unlock publishing", "system-onboarding").Scan(&runID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	workItemCount := s.publishMinCompletedWorkItems
	if workItemCount < 3 {
		workItemCount = 3
	}
	if workItemCount > 10 {
		workItemCount = 10
	}

	var firstWorkItemID uuid.UUID
	for i := 0; i < workItemCount; i++ {
		var workItemID uuid.UUID
		if err := tx.QueryRow(ctx, `
			insert into work_items (run_id, stage, kind, status)
			values ($1, 'onboarding', 'contribute', 'offered')
			returning id
		`, runID).Scan(&workItemID); err != nil {
			return uuid.Nil, uuid.Nil, err
		}
		if i == 0 {
			firstWorkItemID = workItemID
		}

		if _, err := tx.Exec(ctx, `
			insert into work_item_offers (work_item_id, agent_id) values ($1, $2)
			on conflict do nothing
		`, workItemID, agentID); err != nil {
			return uuid.Nil, uuid.Nil, err
		}
	}

	return runID, firstWorkItemID, nil
}

type agentDTO struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
}

func (s server) handleListAgents(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select id, name, description, status
		from agents
		where owner_id = $1
		order by created_at desc
	`, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	var out []agentDTO
	for rows.Next() {
		var (
			id          uuid.UUID
			name        string
			description string
			status      string
		)
		if err := rows.Scan(&id, &name, &description, &status); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}

		tags, err := s.listAgentTags(ctx, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tags query failed"})
			return
		}

		out = append(out, agentDTO{
			ID:          id.String(),
			Name:        name,
			Description: description,
			Status:      status,
			Tags:        tags,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": out})
}

func (s server) listAgentTags(ctx context.Context, agentID uuid.UUID) ([]string, error) {
	rows, err := s.db.Query(ctx, `select tag from agent_tags where agent_id = $1 order by tag asc`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func (s server) handleDisableAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cmdTag, err := s.db.Exec(ctx, `
		update agents
		set status = 'disabled', updated_at = now()
		where id = $1 and owner_id = $2
	`, agentID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if cmdTag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	s.audit(ctx, "user", userID, "agent_disabled", map[string]any{"agent_id": agentID.String()})
	writeJSON(w, http.StatusOK, map[string]string{"status": "disabled"})
}

func (s server) handleDeleteAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	// Verify ownership.
	var exists bool
	if err := tx.QueryRow(ctx, `select true from agents where id=$1 and owner_id=$2`, agentID, userID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	// Delete any per-agent onboarding runs (platform-owned) to avoid leaving
	// orphaned "offered but no offers" work items after cascades.
	if _, err := tx.Exec(ctx, `
		delete from runs r
		where r.publisher_user_id = $2
		  and r.constraints = 'system-onboarding'
		  and exists (
		    select 1
		    from work_items wi
		    join work_item_offers o on o.work_item_id = wi.id
		    where wi.run_id = r.id and o.agent_id = $1
		  )
		  and not exists (
		    select 1
		    from work_items wi
		    join work_item_offers o on o.work_item_id = wi.id
		    where wi.run_id = r.id and o.agent_id <> $1
		  )
	`, agentID, platformUserID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "cleanup failed"})
		return
	}

	// Release any claimed work items held by this agent (otherwise the work item
	// could remain "claimed" with no lease after cascades).
	if _, err := tx.Exec(ctx, `
		with del as (
		  delete from work_item_leases
		  where agent_id = $1
		  returning work_item_id
		)
		update work_items wi
		set status = 'offered', updated_at = now()
		where wi.id in (select work_item_id from del) and wi.status = 'claimed'
	`, agentID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lease cleanup failed"})
		return
	}

	cmd, err := tx.Exec(ctx, `delete from agents where id=$1 and owner_id=$2`, agentID, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}
	if cmd.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "user", userID, "agent_deleted", map[string]any{"agent_id": agentID.String()})
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

type updateAgentRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"` // enabled|disabled
}

func (s server) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var req updateAgentRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}

	set := make([]string, 0, 3)
	args := make([]any, 0, 5)
	argN := 1
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 64 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid name"})
			return
		}
		set = append(set, "name = $"+strconv.Itoa(argN))
		args = append(args, name)
		argN++
	}
	if req.Description != nil {
		if len(*req.Description) > 512 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description too long"})
			return
		}
		set = append(set, "description = $"+strconv.Itoa(argN))
		args = append(args, *req.Description)
		argN++
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != "enabled" && status != "disabled" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
		set = append(set, "status = $"+strconv.Itoa(argN))
		args = append(args, status)
		argN++
	}
	if len(set) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields"})
		return
	}
	set = append(set, "updated_at = now()")

	args = append(args, agentID, userID)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	q := "update agents set " + strings.Join(set, ", ") + " where id = $" + strconv.Itoa(argN) + " and owner_id = $" + strconv.Itoa(argN+1)
	tag, err := s.db.Exec(ctx, q, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	s.audit(ctx, "user", userID, "agent_updated", map[string]any{"agent_id": agentID.String()})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type addTagRequest struct {
	Tag string `json:"tag"`
}

func (s server) handleAddAgentTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	var req addTagRequest
	if !readJSONLimited(w, r, &req, 16*1024) {
		return
	}
	tag := strings.TrimSpace(req.Tag)
	if tag == "" || len(tag) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tag"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var exists bool
	if err := s.db.QueryRow(ctx, `select true from agents where id=$1 and owner_id=$2`, agentID, userID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if _, err := s.db.Exec(ctx, `
		insert into agent_tags (agent_id, tag) values ($1, $2)
		on conflict do nothing
	`, agentID, tag); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"tag": tag})
}

func (s server) handleDeleteAgentTag(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	tag := strings.TrimSpace(chi.URLParam(r, "tag"))
	if tag == "" || len(tag) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tag"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	cmd, err := s.db.Exec(ctx, `
		delete from agent_tags
		where agent_id = $1 and tag = $2
		  and exists (select 1 from agents where id=$1 and owner_id=$3)
	`, agentID, tag, userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}
	if cmd.RowsAffected() == 0 {
		// could be not found or no such tag; keep idempotent.
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s server) handleRotateAgentKey(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	apiKey, err := keys.NewAPIKey()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key generation failed"})
		return
	}
	hash := keys.HashAPIKey(s.pepper, apiKey)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `select true from agents where id=$1 and owner_id=$2`, agentID, userID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if _, err := tx.Exec(ctx, `
		update agent_api_keys set revoked_at = now()
		where agent_id = $1 and revoked_at is null
	`, agentID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "revoke failed"})
		return
	}
	if _, err := tx.Exec(ctx, `
		insert into agent_api_keys (agent_id, key_hash) values ($1, $2)
	`, agentID, hash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "user", userID, "agent_api_key_rotated", map[string]any{"agent_id": agentID.String()})
	writeJSON(w, http.StatusOK, map[string]string{"api_key": apiKey})
}

type replaceTagsRequest struct {
	Tags []string `json:"tags"`
}

type createRunRequest struct {
	Goal         string   `json:"goal"`
	Constraints  string   `json:"constraints"`
	RequiredTags []string `json:"required_tags"`
}

type createRunResponse struct {
	RunID string `json:"run_id"`
}

func (s server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req createRunRequest
	if !readJSONLimited(w, r, &req, 128*1024) {
		return
	}
	req.Goal = strings.TrimSpace(req.Goal)
	req.Constraints = strings.TrimSpace(req.Constraints)
	if req.Goal == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing goal"})
		return
	}
	if len(req.Goal) > 4000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "goal too long"})
		return
	}
	if len(req.Constraints) > 8000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "constraints too long"})
		return
	}
	req.RequiredTags = normalizeTags(req.RequiredTags)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Gate 1: must have at least one owned agent.
	var agentCount int
	if err := s.db.QueryRow(ctx, `select count(*) from agents where owner_id=$1`, userID).Scan(&agentCount); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent check failed"})
		return
	}
	if agentCount < 1 {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":       "publish_gated",
			"reason":      "no_agent",
			"requirement": "register at least one agent first",
		})
		return
	}

	// Gate 2: contributions aggregated across all owned agents (per spec).
	var completed int
	_ = s.db.QueryRow(ctx, `select completed_work_items from owner_contributions where owner_id=$1`, userID).Scan(&completed)
	if completed < s.publishMinCompletedWorkItems {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"error":       "publish_gated",
			"reason":      "insufficient_contribution",
			"min":         s.publishMinCompletedWorkItems,
			"completed":   completed,
			"requirement": "your agents must complete platform work before you can publish runs",
		})
		return
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var runID uuid.UUID
	if err := tx.QueryRow(ctx, `
			insert into runs (publisher_user_id, goal, constraints, status)
			values ($1, $2, $3, 'created')
			returning id
		`, userID, req.Goal, req.Constraints).Scan(&runID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create run failed"})
		return
	}

	for _, t := range req.RequiredTags {
		if _, err := tx.Exec(ctx, `
			insert into run_required_tags (run_id, tag) values ($1, $2)
			on conflict do nothing
		`, runID, t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create run tags failed"})
			return
		}
	}

	// MVP: create a single initial work item and offer it to a matched set of agents.
	workItemID, err := s.createInitialWorkItemAndOffers(ctx, tx, runID, userID, req.RequiredTags)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "matching failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db commit failed"})
		return
	}

	s.audit(ctx, "user", userID, "run_created", map[string]any{"run_id": runID.String(), "initial_work_item_id": workItemID.String()})
	writeJSON(w, http.StatusCreated, createRunResponse{RunID: runID.String()})
}

func (s server) createInitialWorkItemAndOffers(ctx context.Context, tx pgx.Tx, runID uuid.UUID, publisherUserID uuid.UUID, requiredTags []string) (uuid.UUID, error) {
	agentIDs, err := s.matchAgentsForRun(ctx, tx, publisherUserID, requiredTags, s.matchingParticipantCount)
	if err != nil {
		return uuid.Nil, err
	}

	var workItemID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into work_items (run_id, stage, kind, status)
		values ($1, 'ideation', 'draft', 'offered')
		returning id
	`, runID).Scan(&workItemID); err != nil {
		return uuid.Nil, err
	}

	for _, agentID := range agentIDs {
		if _, err := tx.Exec(ctx, `
			insert into work_item_offers (work_item_id, agent_id) values ($1, $2)
			on conflict do nothing
		`, workItemID, agentID); err != nil {
			return uuid.Nil, err
		}
	}
	return workItemID, nil
}

func (s server) matchAgentsForRun(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, publisherUserID uuid.UUID, requiredTags []string, limit int) ([]uuid.UUID, error) {
	// Policy (MVP):
	// - Prefer including publisher-owned eligible agents (so a solo user can publish + have their own agents participate).
	// - Fill remaining slots with other eligible agents for exploration.
	// - Still respects requiredTags and agent enabled status.
	requiredTags = normalizeTags(requiredTags)
	if limit < 1 {
		limit = 1
	}

	ownerCandidates, err := s.matchOwnerAgents(ctx, q, publisherUserID, requiredTags)
	if err != nil {
		return nil, err
	}
	shuffleUUIDs(ownerCandidates)
	if len(ownerCandidates) > limit {
		return ownerCandidates[:limit], nil
	}

	remaining := limit - len(ownerCandidates)
	otherCandidates, err := s.matchNonOwnerAgents(ctx, q, publisherUserID, requiredTags)
	if err != nil {
		return nil, err
	}
	shuffleUUIDs(otherCandidates)
	if remaining > len(otherCandidates) {
		remaining = len(otherCandidates)
	}

	out := make([]uuid.UUID, 0, limit)
	out = append(out, ownerCandidates...)
	out = append(out, otherCandidates[:remaining]...)
	if len(out) == 0 {
		return nil, errors.New("no eligible agents")
	}
	return out, nil
}

func (s server) matchOwnerAgents(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, ownerID uuid.UUID, requiredTags []string) ([]uuid.UUID, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if len(requiredTags) == 0 {
		rows, err = q.Query(ctx, `select id from agents where status='enabled' and owner_id=$1`, ownerID)
	} else {
		rows, err = q.Query(ctx, `
			select a.id
			from agents a
			join agent_tags t on t.agent_id = a.id
			where a.status='enabled' and a.owner_id=$1 and t.tag = any($2)
			group by a.id
			having count(distinct t.tag) = $3
		`, ownerID, requiredTags, len(requiredTags))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (s server) matchNonOwnerAgents(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, ownerID uuid.UUID, requiredTags []string) ([]uuid.UUID, error) {
	var (
		rows pgx.Rows
		err  error
	)
	if len(requiredTags) == 0 {
		rows, err = q.Query(ctx, `select id from agents where status='enabled' and owner_id <> $1`, ownerID)
	} else {
		rows, err = q.Query(ctx, `
			select a.id
			from agents a
			join agent_tags t on t.agent_id = a.id
			where a.status='enabled' and a.owner_id <> $1 and t.tag = any($2)
			group by a.id
			having count(distinct t.tag) = $3
		`, ownerID, requiredTags, len(requiredTags))
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func (s server) matchAgents(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, requiredTags []string, limit int) ([]uuid.UUID, error) {
	// MVP matching (cold-start friendly):
	// - enabled agents only
	// - try strict tag match first (ALL required tags)
	// - if insufficient, relax to partial tag match (ANY required tag; prefer higher overlap)
	// - if still insufficient, fall back to any enabled agents
	// - shuffle within each pool for exploration/rotation
	selected := make([]uuid.UUID, 0, limit)
	seen := map[uuid.UUID]struct{}{}
	addUnique := func(ids []uuid.UUID) {
		for _, id := range ids {
			if len(selected) >= limit {
				return
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			selected = append(selected, id)
		}
	}

	// Helper to query a list of UUIDs.
	queryIDs := func(sql string, args ...any) ([]uuid.UUID, error) {
		rows, err := q.Query(ctx, sql, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var out []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err := rows.Scan(&id); err != nil {
				return nil, err
			}
			out = append(out, id)
		}
		return out, nil
	}

	if len(requiredTags) > 0 {
		// 1) Strict: agent must have ALL required tags.
		strict, err := queryIDs(`
			select a.id
			from agents a
			join agent_tags t on t.agent_id = a.id
			where a.status='enabled' and t.tag = any($1)
			group by a.id
			having count(distinct t.tag) = $2
		`, requiredTags, len(requiredTags))
		if err != nil {
			return nil, err
		}
		shuffleUUIDs(strict)
		addUnique(strict)

		// 2) Partial: ANY tag match, prefer higher overlap.
		if len(selected) < limit {
			partial, err := queryIDs(`
				select a.id
				from agents a
				join agent_tags t on t.agent_id = a.id
				where a.status='enabled' and t.tag = any($1)
				group by a.id
				order by count(distinct t.tag) desc, random()
			`, requiredTags)
			if err != nil {
				return nil, err
			}
			addUnique(partial)
		}
	}

	// 3) Fallback: any enabled agents.
	if len(selected) < limit {
		all, err := queryIDs(`select id from agents where status='enabled'`)
		if err != nil {
			return nil, err
		}
		shuffleUUIDs(all)
		addUnique(all)
	}

	if len(selected) == 0 {
		return nil, errors.New("no eligible agents")
	}
	return selected, nil
}

type runPublicDTO struct {
	ID          string `json:"id"`
	Goal        string `json:"goal"`
	Constraints string `json:"constraints"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
}

type runListItemDTO struct {
	RunID         string `json:"run_id"`
	Goal          string `json:"goal"`
	Constraints   string `json:"constraints"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	OutputVersion int    `json:"output_version"`
	OutputKind    string `json:"output_kind"`
}

type listRunsResponse struct {
	Runs       []runListItemDTO `json:"runs"`
	HasMore    bool             `json:"has_more"`
	NextOffset int              `json:"next_offset"`
}

func (s server) handleListRunsPublic(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	terms := splitSearchTerms(q)

	limit := 20
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = clampInt(n, 1, 50)
		}
	}
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = clampInt(n, 0, 50_000)
		}
	}

	includeSystem := false
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("include_system"))) {
	case "1", "true", "yes", "y":
		includeSystem = true
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	args := make([]any, 0, 16)
	where := make([]string, 0, 8)
	argN := 1

	if !includeSystem {
		where = append(where, "r.publisher_user_id <> $"+strconv.Itoa(argN))
		args = append(args, platformUserID)
		argN++
	}

	// If search terms exist, apply AND across terms, each term matches any field.
	for _, t := range terms {
		pat := "%" + t + "%"
		parts := []string{
			"r.id::text ilike $" + strconv.Itoa(argN),
			"r.goal ilike $" + strconv.Itoa(argN),
			"r.constraints ilike $" + strconv.Itoa(argN),
			"coalesce(a.content, '') ilike $" + strconv.Itoa(argN),
		}
		where = append(where, "("+strings.Join(parts, " or ")+")")
		args = append(args, pat)
		argN++
	}

	// Use limit+1 to determine has_more.
	limitPlusOne := limit + 1

	sql := `
		select r.id, r.goal, r.constraints, r.status, r.created_at, r.updated_at,
		       coalesce(a.version, 0) as output_version,
		       coalesce(a.kind, '') as output_kind
		from runs r
		left join lateral (
			select version, kind, content
			from artifacts
			where run_id = r.id
			order by version desc
			limit 1
		) a on true
	`
	if len(where) > 0 {
		sql += " where " + strings.Join(where, " and ")
	}
	sql += " order by r.created_at desc limit $" + strconv.Itoa(argN) + " offset $" + strconv.Itoa(argN+1)
	args = append(args, limitPlusOne, offset)

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]runListItemDTO, 0, limit)
	for rows.Next() {
		var (
			id          uuid.UUID
			goal        string
			constraints string
			status      string
			createdAt   time.Time
			updatedAt   time.Time
			outVer      int
			outKind     string
		)
		if err := rows.Scan(&id, &goal, &constraints, &status, &createdAt, &updatedAt, &outVer, &outKind); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		out = append(out, runListItemDTO{
			RunID:         id.String(),
			Goal:          goal,
			Constraints:   constraints,
			Status:        status,
			CreatedAt:     createdAt.UTC().Format(time.RFC3339),
			UpdatedAt:     updatedAt.UTC().Format(time.RFC3339),
			OutputVersion: outVer,
			OutputKind:    outKind,
		})
	}

	hasMore := false
	if len(out) > limit {
		hasMore = true
		out = out[:limit]
	}
	nextOffset := offset + len(out)
	if hasMore {
		nextOffset = offset + limit
	}

	writeJSON(w, http.StatusOK, listRunsResponse{
		Runs:       out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}

func (s server) handleGetRunPublic(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var dto runPublicDTO
	var createdAt time.Time
	err = s.db.QueryRow(ctx, `
		select id, goal, constraints, status, created_at
		from runs
		where id = $1
	`, runID).Scan(&dto.ID, &dto.Goal, &dto.Constraints, &dto.Status, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, dto)
}

type runOutputDTO struct {
	RunID   string `json:"run_id"`
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Content string `json:"content"`
}

type submitArtifactRequest struct {
	Kind           string `json:"kind"` // draft|final
	Content        string `json:"content"`
	LinkedEventSeq *int64 `json:"linked_event_seq"`
}

type submitArtifactResponse struct {
	RunID   string `json:"run_id"`
	Version int    `json:"version"`
	Kind    string `json:"kind"`
}

func (s server) handleGatewaySubmitArtifact(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	var req submitArtifactRequest
	if !readJSONLimited(w, r, &req, 256*1024) {
		return
	}
	req.Kind = strings.TrimSpace(req.Kind)
	if req.Kind == "" {
		req.Kind = "final"
	}
	if req.Kind != "draft" && req.Kind != "final" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid kind"})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing content"})
		return
	}
	if len(req.Content) > 200_000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content too large"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Agent must be a participant in the run.
	var participant bool
	err = s.db.QueryRow(ctx, `
		select true
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		where o.agent_id = $1 and wi.run_id = $2
		limit 1
	`, agentID, runID).Scan(&participant)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a participant"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "participant check failed"})
		return
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	// Lock run to serialize version allocation.
	if _, err := tx.Exec(ctx, `select 1 from runs where id=$1 for update`, runID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run lock failed"})
		return
	}

	var nextVersion int
	if err := tx.QueryRow(ctx, `select coalesce(max(version), 0) + 1 from artifacts where run_id=$1`, runID).Scan(&nextVersion); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "version allocation failed"})
		return
	}

	var linkedSeq any
	if req.LinkedEventSeq != nil && *req.LinkedEventSeq > 0 {
		linkedSeq = *req.LinkedEventSeq
	} else {
		linkedSeq = nil
	}

	if _, err := tx.Exec(ctx, `
		insert into artifacts (run_id, version, kind, content, linked_event_seq)
		values ($1, $2, $3, $4, $5)
	`, runID, nextVersion, req.Kind, req.Content, linkedSeq); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "agent", agentID, "artifact_submitted", map[string]any{"run_id": runID.String(), "version": nextVersion, "kind": req.Kind})
	writeJSON(w, http.StatusCreated, submitArtifactResponse{RunID: runID.String(), Version: nextVersion, Kind: req.Kind})
}

func (s server) ownerForAgent(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	var ownerID uuid.UUID
	if err := s.db.QueryRow(ctx, `select owner_id from agents where id=$1`, agentID).Scan(&ownerID); err != nil {
		return uuid.Nil, err
	}
	return ownerID, nil
}

func (s server) handleGetRunOutputPublic(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var (
		version int
		kind    string
		content string
	)
	err = s.db.QueryRow(ctx, `
		select version, kind, content
		from artifacts
		where run_id = $1
		order by version desc
		limit 1
	`, runID).Scan(&version, &kind, &content)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no output"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	writeJSON(w, http.StatusOK, runOutputDTO{
		RunID:   runID.String(),
		Version: version,
		Kind:    kind,
		Content: content,
	})
}

func (s server) handleGetRunArtifactPublic(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}
	version, err := strconv.Atoi(strings.TrimSpace(chi.URLParam(r, "version")))
	if err != nil || version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid version"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var (
		kind           string
		content        string
		linkedEventSeq *int64
		createdAt      time.Time
	)
	err = s.db.QueryRow(ctx, `
		select kind, content, linked_event_seq, created_at
		from artifacts
		where run_id=$1 and version=$2
	`, runID, version).Scan(&kind, &content, &linkedEventSeq, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	// Provide jump info for key nodes: if linked_event_seq is present, clients can start replay near it.
	resp := map[string]any{
		"run_id":     runID.String(),
		"version":    version,
		"kind":       kind,
		"content":    content,
		"created_at": createdAt.UTC().Format(time.RFC3339),
		"linked_seq": linkedEventSeq,
		"replay_url": "/v1/runs/" + runID.String() + "/replay",
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s server) handleRunStreamSSE(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	afterSeq := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("after_seq")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after_seq"})
			return
		}
		afterSeq = n
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	bw := bufio.NewWriterSize(w, 16*1024)
	defer bw.Flush()

	ctx := r.Context()

	// Backfill.
	events, err := s.fetchEvents(ctx, runID, afterSeq, 500)
	if err != nil {
		writeSSE(bw, "error", map[string]string{"error": "backfill failed"})
		bw.Flush()
		return
	}
	for _, ev := range events {
		writeSSE(bw, "event", ev)
	}
	bw.Flush()
	flusher.Flush()

	// Subscribe for live events.
	ch := s.br.subscribe(runID)
	defer s.br.unsubscribe(runID, ch)

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			if ev.Seq <= afterSeq {
				continue
			}
			afterSeq = ev.Seq
			writeSSE(bw, "event", ev)
			bw.Flush()
			flusher.Flush()
		case <-keepAlive.C:
			_, _ = bw.WriteString(": keepalive\n\n")
			bw.Flush()
			flusher.Flush()
		}
	}
}

func (s server) handleRunReplay(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	afterSeq := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("after_seq")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after_seq"})
			return
		}
		afterSeq = n
	}
	limit := clampInt(int64Query(r, "limit", 200), 1, 500)

	events, err := s.fetchEvents(r.Context(), runID, afterSeq, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	var keyNodes []eventDTO
	for _, ev := range events {
		if ev.IsKeyNode {
			keyNodes = append(keyNodes, ev)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":    runID.String(),
		"events":    events,
		"key_nodes": keyNodes,
		"after_seq": afterSeq,
		"limit":     limit,
	})
}

func int64Query(r *http.Request, key string, fallback int) int {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func writeSSE(w *bufio.Writer, eventName string, data any) {
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = w.WriteString("event: " + eventName + "\n")
	_, _ = w.WriteString("data: ")
	_, _ = w.Write(b)
	_, _ = w.WriteString("\n\n")
}

func (s server) fetchEvents(ctx context.Context, runID uuid.UUID, afterSeq int64, limit int) ([]eventDTO, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select seq, kind, persona, payload, is_key_node, created_at
		from events
		where run_id = $1 and seq > $2
		order by seq asc
		limit $3
	`, runID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []eventDTO
	for rows.Next() {
		var (
			seq       int64
			kind      string
			persona   string
			payloadB  []byte
			isKeyNode bool
			createdAt time.Time
		)
		if err := rows.Scan(&seq, &kind, &persona, &payloadB, &isKeyNode, &createdAt); err != nil {
			return nil, err
		}
		var payload map[string]any
		_ = json.Unmarshal(payloadB, &payload)
		out = append(out, eventDTO{
			RunID:     runID.String(),
			Seq:       seq,
			Kind:      kind,
			Persona:   persona,
			Payload:   payload,
			IsKeyNode: isKeyNode,
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (s server) handleReplaceAgentTags(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	var req replaceTagsRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	tags := normalizeTags(req.Tags)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `select true from agents where id=$1 and owner_id=$2`, agentID, userID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if _, err := tx.Exec(ctx, `delete from agent_tags where agent_id=$1`, agentID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete tags failed"})
		return
	}
	for _, t := range tags {
		if _, err := tx.Exec(ctx, `
			insert into agent_tags (agent_id, tag) values ($1, $2)
			on conflict do nothing
		`, agentID, t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert tags failed"})
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s server) handleGatewayPoll(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	s.audit(ctx, "agent", agentID, "gateway_poll", map[string]any{})

	rows, err := s.db.Query(ctx, `
		select wi.id, wi.run_id, wi.stage, wi.kind, wi.status
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		where o.agent_id = $1 and wi.status in ('offered', 'claimed')
		order by wi.created_at asc
		limit 50
	`, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type offerDTO struct {
		WorkItemID  string `json:"work_item_id"`
		RunID       string `json:"run_id"`
		Stage       string `json:"stage"`
		Kind        string `json:"kind"`
		Status      string `json:"status"`
		Goal        string `json:"goal"`
		Constraints string `json:"constraints"`
	}
	offers := make([]offerDTO, 0)
	for rows.Next() {
		var (
			workItemID uuid.UUID
			runID      uuid.UUID
			stage      string
			kind       string
			status     string
		)
		if err := rows.Scan(&workItemID, &runID, &stage, &kind, &status); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}

		var goal, constraints string
		if err := s.db.QueryRow(ctx, `select goal, constraints from runs where id=$1`, runID).Scan(&goal, &constraints); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run lookup failed"})
			return
		}

		offers = append(offers, offerDTO{
			WorkItemID:  workItemID.String(),
			RunID:       runID.String(),
			Stage:       stage,
			Kind:        kind,
			Status:      status,
			Goal:        goal,
			Constraints: constraints,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": agentID.String(), "offers": offers})
}

type claimResponse struct {
	WorkItemID     string `json:"work_item_id"`
	LeaseExpiresAt string `json:"lease_expires_at"`
}

func (s server) handleGatewayClaimWorkItem(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work_item_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	// Must be offered to this agent.
	var offered bool
	if err := tx.QueryRow(ctx, `select true from work_item_offers where work_item_id=$1 and agent_id=$2`, workItemID, agentID).Scan(&offered); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not offered"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "offer check failed"})
		return
	}

	// Only allow claiming if currently offered.
	var status string
	if err := tx.QueryRow(ctx, `select status from work_items where id=$1 for update`, workItemID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "work item lookup failed"})
		return
	}
	if status != "offered" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "not claimable"})
		return
	}

	expiresAt := time.Now().UTC().Add(time.Duration(s.workItemLeaseSeconds) * time.Second)
	if _, err := tx.Exec(ctx, `
		insert into work_item_leases (work_item_id, agent_id, lease_expires_at)
		values ($1, $2, $3)
	`, workItemID, agentID, expiresAt); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already claimed"})
		return
	}
	if _, err := tx.Exec(ctx, `update work_items set status='claimed', updated_at=now() where id=$1`, workItemID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "agent", agentID, "work_item_claimed", map[string]any{"work_item_id": workItemID.String(), "lease_expires_at": expiresAt.Format(time.RFC3339)})
	writeJSON(w, http.StatusOK, claimResponse{
		WorkItemID:     workItemID.String(),
		LeaseExpiresAt: expiresAt.Format(time.RFC3339),
	})
}

func (s server) handleGatewayCompleteWorkItem(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work_item_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var leaseAgent uuid.UUID
	var leaseExpires time.Time
	err = tx.QueryRow(ctx, `
		select agent_id, lease_expires_at
		from work_item_leases
		where work_item_id = $1
	`, workItemID).Scan(&leaseAgent, &leaseExpires)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "not leased"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lease lookup failed"})
		return
	}
	if leaseAgent != agentID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not lease holder"})
		return
	}
	if time.Now().UTC().After(leaseExpires) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "lease expired"})
		return
	}

	if _, err := tx.Exec(ctx, `update work_items set status='completed', updated_at=now() where id=$1`, workItemID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if _, err := tx.Exec(ctx, `delete from work_item_leases where work_item_id=$1`, workItemID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lease delete failed"})
		return
	}

	// Update owner aggregated contribution counter.
	var ownerID uuid.UUID
	if err := tx.QueryRow(ctx, `select owner_id from agents where id=$1`, agentID).Scan(&ownerID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "owner lookup failed"})
		return
	}
	if _, err := tx.Exec(ctx, `
		insert into owner_contributions (owner_id, completed_work_items, updated_at)
		values ($1, 1, now())
		on conflict (owner_id) do update
		set completed_work_items = owner_contributions.completed_work_items + 1,
		    updated_at = now()
	`, ownerID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "contribution update failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}
	s.audit(ctx, "agent", agentID, "work_item_completed", map[string]any{"work_item_id": workItemID.String(), "owner_id": ownerID.String()})
	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

func shuffleUUIDs(ids []uuid.UUID) {
	for i := len(ids) - 1; i > 0; i-- {
		nBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return
		}
		j := int(nBig.Int64())
		ids[i], ids[j] = ids[j], ids[i]
	}
}

type emitEventRequest struct {
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

func (s server) handleGatewayEmitEvent(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	var req emitEventRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	req.Kind = strings.TrimSpace(req.Kind)
	if _, ok := allowedEventKinds[req.Kind]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid kind"})
		return
	}
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}
	// Basic payload size guardrail.
	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if len(payloadJSON) > 16*1024 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload too large"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Agent must be a participant: it must have been offered a work item in this run.
	var participant bool
	err = s.db.QueryRow(ctx, `
		select true
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		where o.agent_id = $1 and wi.run_id = $2
		limit 1
	`, agentID, runID).Scan(&participant)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a participant"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "participant check failed"})
		return
	}

	persona, err := s.personaForAgent(ctx, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "persona lookup failed"})
		return
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	// Lock run row to serialize seq allocation per run.
	if _, err := tx.Exec(ctx, `select 1 from runs where id=$1 for update`, runID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run lock failed"})
		return
	}

	var nextSeq int64
	if err := tx.QueryRow(ctx, `select coalesce(max(seq), 0) + 1 from events where run_id=$1`, runID).Scan(&nextSeq); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "seq allocation failed"})
		return
	}
	isKey := isKeyNodeKind(req.Kind)

	createdAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		insert into events (run_id, seq, kind, persona, payload, is_key_node, created_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, runID, nextSeq, req.Kind, persona, payloadJSON, isKey, createdAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	var payloadMap map[string]any
	_ = json.Unmarshal(payloadJSON, &payloadMap)
	ev := eventDTO{
		RunID:     runID.String(),
		Seq:       nextSeq,
		Kind:      req.Kind,
		Persona:   persona,
		Payload:   payloadMap,
		IsKeyNode: isKey,
		CreatedAt: createdAt.Format(time.RFC3339),
	}
	s.br.publish(runID, ev)
	writeJSON(w, http.StatusCreated, ev)

	s.audit(ctx, "agent", agentID, "event_emitted", map[string]any{"run_id": runID.String(), "seq": nextSeq, "kind": req.Kind, "is_key_node": isKey})
}

func (s server) personaForAgent(ctx context.Context, agentID uuid.UUID) (string, error) {
	// Public persona is derived from tags (not identity), and must not expose owner.
	rows, err := s.db.Query(ctx, `select tag from agent_tags where agent_id=$1 order by tag asc`, agentID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return "", err
		}
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
		if len(tags) >= 2 {
			break
		}
	}
	if len(tags) == 0 {
		return "agent", nil
	}
	return strings.Join(tags, " / "), nil
}

type invokeToolRequest struct {
	RunID string         `json:"run_id"`
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
}

func (s server) handleGatewayInvokeTool(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req invokeToolRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	req.Tool = strings.TrimSpace(req.Tool)
	if req.Tool == "" || len(req.Tool) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tool"})
		return
	}
	runID, err := uuid.Parse(strings.TrimSpace(req.RunID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	allowed, err := s.isToolAllowed(ctx, agentID, runID, req.Tool)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy check failed"})
		return
	}
	if !allowed {
		s.audit(ctx, "agent", agentID, "tool_denied", map[string]any{"run_id": runID.String(), "tool": req.Tool})
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "tool_denied"})
		return
	}

	s.audit(ctx, "agent", agentID, "tool_allowed_but_not_implemented", map[string]any{"run_id": runID.String(), "tool": req.Tool})
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "tool_not_implemented"})
}

func (s server) isToolAllowed(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, tool string) (bool, error) {
	// Default deny: tool must be explicitly allowed for both agent and run.
	var agentAllowed bool
	if err := s.db.QueryRow(ctx, `select exists(select 1 from agent_allowed_tools where agent_id=$1 and tool=$2)`, agentID, tool).Scan(&agentAllowed); err != nil {
		return false, err
	}
	if !agentAllowed {
		return false, nil
	}
	var runAllowed bool
	if err := s.db.QueryRow(ctx, `select exists(select 1 from run_allowed_tools where run_id=$1 and tool=$2)`, runID, tool).Scan(&runAllowed); err != nil {
		return false, err
	}
	return runAllowed, nil
}
