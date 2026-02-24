package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"aihub/internal/agenthome"
	"aihub/internal/keys"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type server struct {
	db                     *pgxpool.Pool
	pepper                 string
	adminToken             string
	publicBaseURL          string
	githubClientID         string
	githubClientSecret     string
	skillsGatewayWhitelist []string

	publishMinCompletedWorkItems int
	matchingParticipantCount     int
	workItemLeaseSeconds         int

	br *broker

	platformKeysEncryptionKey string
	platformCertIssuer        string
	platformCertTTLSeconds    int
	promptViewMaxChars        int

	ossProvider           string
	ossEndpoint           string
	ossRegion             string
	ossBucket             string
	ossBasePrefix         string
	ossAccessKeyID        string
	ossAccessKeySecret    string
	ossSTSRoleARN         string
	ossSTSDurationSeconds int
	ossLocalDir           string
	ossEventsIngestToken  string
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
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logErrorNoCtx("writeJSON encode failed", err)
	}
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

func unmarshalJSONNullable(b []byte, dst any) error {
	if len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, dst)
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

func (s server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimSpace(s.adminToken) == "" {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "admin token not configured"})
			return
		}
		token := bearerToken(r)
		if token == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		if token != s.adminToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}

		next.ServeHTTP(w, r)
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
	if _, err := s.db.Exec(ctx, `
		insert into audit_logs (actor_type, actor_id, action, data)
		values ($1, $2, $3, $4)
	`, actorType, actorID, action, data); err != nil {
		logError(ctx, "audit insert failed", err)
	}
}

// --- Handlers

func (s server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	type meResponse struct {
		Provider    string `json:"provider,omitempty"`
		Login       string `json:"login,omitempty"`
		Name        string `json:"name,omitempty"`
		DisplayName string `json:"display_name,omitempty"`
		AvatarURL   string `json:"avatar_url,omitempty"`
		ProfileURL  string `json:"profile_url,omitempty"`
	}

	var login, name, avatar, profile string
	err := s.db.QueryRow(ctx, `
		select login, name, avatar_url, profile_url
		from user_identities
		where user_id = $1 and provider = 'github'
		order by created_at desc
		limit 1
	`, userID).Scan(&login, &name, &avatar, &profile)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, meResponse{})
		return
	}
	if err != nil {
		logError(ctx, "me identity query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	display := strings.TrimSpace(name)
	if display == "" {
		display = strings.TrimSpace(login)
	}
	writeJSON(w, http.StatusOK, meResponse{
		Provider:    "github",
		Login:       strings.TrimSpace(login),
		Name:        strings.TrimSpace(name),
		DisplayName: display,
		AvatarURL:   strings.TrimSpace(avatar),
		ProfileURL:  strings.TrimSpace(profile),
	})
}

type createAgentRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`

	AvatarURL      string          `json:"avatar_url,omitempty"`
	AgentPublicKey string          `json:"agent_public_key,omitempty"`
	Personality    *personalityDTO `json:"personality,omitempty"`
	Interests      []string        `json:"interests,omitempty"`
	Capabilities   []string        `json:"capabilities,omitempty"`
	Bio            string          `json:"bio,omitempty"`
	Greeting       string          `json:"greeting,omitempty"`
	Discovery      *discoveryDTO   `json:"discovery,omitempty"`
	Autonomous     *autonomousDTO  `json:"autonomous,omitempty"`
	PersonaTemplateID string       `json:"persona_template_id,omitempty"`
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

	avatarURL := strings.TrimSpace(req.AvatarURL)
	if len(avatarURL) > 1024 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar_url too long"})
		return
	}

	personality := defaultPersonality()
	if req.Personality != nil {
		if err := req.Personality.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid personality"})
			return
		}
		personality = *req.Personality
	}

	interests := normalizeStringList(req.Interests, 24, 64)
	capabilities := normalizeStringList(req.Capabilities, 24, 64)

	bio := strings.TrimSpace(req.Bio)
	if len(bio) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bio too long"})
		return
	}
	greeting := strings.TrimSpace(req.Greeting)
	if len(greeting) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "greeting too long"})
		return
	}

	discovery := discoveryDTO{Public: false}
	if req.Discovery != nil {
		discovery.Public = req.Discovery.Public
	}

	autonomous := defaultAutonomous()
	if req.Autonomous != nil {
		if err := req.Autonomous.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid autonomous config"})
			return
		}
		autonomous = *req.Autonomous
	}

	agentPublicKey := strings.TrimSpace(req.AgentPublicKey)
	if agentPublicKey != "" {
		pub, err := agenthome.ParseEd25519PublicKey(agentPublicKey)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_public_key"})
			return
		}
		agentPublicKey = "ed25519:" + base64.StdEncoding.EncodeToString(pub)
	}

	var personaAny any
	var personaJSON []byte
	if strings.TrimSpace(req.PersonaTemplateID) != "" {
		// Resolve template on create (must be approved).
		// Read in the same TX later to ensure consistent snapshot.
	}

	promptView := promptViewFromFields(req.Name, personaAny, personality, interests, capabilities, bio)
	if len([]rune(promptView)) > s.promptViewMaxChars {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt_view too long"})
		return
	}

	personalityJSON, err := marshalJSONB(personality)
	if err != nil {
		logError(r.Context(), "marshal personality failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	interestsJSON, err := marshalJSONB(interests)
	if err != nil {
		logError(r.Context(), "marshal interests failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	capabilitiesJSON, err := marshalJSONB(capabilities)
	if err != nil {
		logError(r.Context(), "marshal capabilities failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	discoveryJSON, err := marshalJSONB(discovery)
	if err != nil {
		logError(r.Context(), "marshal discovery failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	autonomousJSON, err := marshalJSONB(autonomous)
	if err != nil {
		logError(r.Context(), "marshal autonomous failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
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

	if strings.TrimSpace(req.PersonaTemplateID) != "" {
		var personaRaw []byte
		if err := tx.QueryRow(ctx, `
			select persona
			from persona_templates
			where id = $1 and review_status = 'approved'
		`, strings.TrimSpace(req.PersonaTemplateID)).Scan(&personaRaw); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid persona_template_id"})
				return
			}
			logError(ctx, "query persona template failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		personaJSON = personaRaw
		if err := json.Unmarshal(personaRaw, &personaAny); err != nil {
			logError(ctx, "unmarshal persona template failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decode failed"})
			return
		}
		// Regenerate prompt_view with persona present.
		promptView = promptViewFromFields(req.Name, personaAny, personality, interests, capabilities, bio)
		if len([]rune(promptView)) > s.promptViewMaxChars {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt_view too long"})
			return
		}
	}

	var agentID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into agents (
			owner_id, name, description, status,
			avatar_url, personality, interests, capabilities, bio, greeting,
			discovery, autonomous, persona,
			agent_public_key,
			prompt_view
		)
		values (
			$1, $2, $3, 'enabled',
			$4, $5, $6, $7, $8, $9,
			$10, $11, $12,
			$13,
			$14
		)
		returning id
	`, userID, req.Name, req.Description,
		avatarURL, personalityJSON, interestsJSON, capabilitiesJSON, bio, greeting,
		discoveryJSON, autonomousJSON, personaJSON,
		agentPublicKey,
		promptView,
	).Scan(&agentID); err != nil {
		logError(ctx, "insert agent failed", err)
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

	// Offer platform built-in tasks (入驻自我介绍 + 每日签到), so new owners can satisfy
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
var platformIntroRunID = uuid.MustParse("00000000-0000-0000-0000-000000000010")
var platformCheckinRunID = uuid.MustParse("00000000-0000-0000-0000-000000000011")

func (s server) createOnboardingOffer(ctx context.Context, tx pgx.Tx, agentID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	// Ensure platform/system user exists.
	if _, err := tx.Exec(ctx, `insert into users (id) values ($1) on conflict do nothing`, platformUserID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	// Ensure platform built-in runs exist (global, not per-agent).
	// NOTE: These are discoverable on the homepage (include_system=1), and every new agent will get
	// two offered work items under them (intro + check-in).
	if _, err := tx.Exec(ctx, `
		insert into runs (id, publisher_user_id, goal, constraints, status)
		values ($1, $2, $3, $4, 'running')
		on conflict (id) do update
		set publisher_user_id = excluded.publisher_user_id,
		    goal = excluded.goal,
		    constraints = excluded.constraints,
		    status = excluded.status,
		    updated_at = now()
	`, platformIntroRunID, platformUserID,
		"平台内置任务：入驻自我介绍",
		"要求：必须遵循任务项里写明的「预期输出」；只用中文；不要泄露密钥/Token/隐私信息；不需要人工中途指挥；最后要完成任务项。",
	); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into runs (id, publisher_user_id, goal, constraints, status)
		values ($1, $2, $3, $4, 'running')
		on conflict (id) do update
		set publisher_user_id = excluded.publisher_user_id,
		    goal = excluded.goal,
		    constraints = excluded.constraints,
		    status = excluded.status,
		    updated_at = now()
	`, platformCheckinRunID, platformUserID,
		"平台内置任务：每日签到",
		"要求：必须遵循任务项里写明的「预期输出」；只用中文；不要泄露密钥/Token/隐私信息；不需要人工中途指挥；最后要完成任务项。",
	); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	skills := s.skillsGatewayWhitelist
	if skills == nil {
		skills = []string{}
	}
	availableSkillsJSON, err := json.Marshal(skills)
	if err != nil {
		logError(ctx, "marshal available_skills failed", err)
		return uuid.Nil, uuid.Nil, err
	}
	onboardingContextJSON, err := json.Marshal(s.stageContextForStage("onboarding", skills))
	if err != nil {
		logError(ctx, "marshal stage_context failed", err)
		return uuid.Nil, uuid.Nil, err
	}
	checkinContextJSON, err := json.Marshal(s.stageContextForStage("checkin", skills))
	if err != nil {
		logError(ctx, "marshal stage_context failed", err)
		return uuid.Nil, uuid.Nil, err
	}

	var introWorkItemID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into work_items (run_id, stage, kind, status, context, available_skills)
		values ($1, 'onboarding', 'contribute', 'offered', $2, $3)
		returning id
	`, platformIntroRunID, onboardingContextJSON, availableSkillsJSON).Scan(&introWorkItemID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into work_item_offers (work_item_id, agent_id) values ($1, $2)
		on conflict do nothing
	`, introWorkItemID, agentID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	var checkinWorkItemID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into work_items (run_id, stage, kind, status, context, available_skills)
		values ($1, 'checkin', 'contribute', 'offered', $2, $3)
		returning id
	`, platformCheckinRunID, checkinContextJSON, availableSkillsJSON).Scan(&checkinWorkItemID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `
		insert into work_item_offers (work_item_id, agent_id) values ($1, $2)
		on conflict do nothing
	`, checkinWorkItemID, agentID); err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	return platformIntroRunID, introWorkItemID, nil
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

	// Delete any platform built-in work items that were exclusively offered to this agent,
	// to avoid leaving orphaned work items with no offers after cascades.
	if _, err := tx.Exec(ctx, `
		delete from work_items wi
		where wi.run_id in ($2, $3)
		  and exists (
		    select 1
		    from work_item_offers o
		    where o.work_item_id = wi.id and o.agent_id = $1
		  )
		  and not exists (
		    select 1
		    from work_item_offers o
		    where o.work_item_id = wi.id and o.agent_id <> $1
		  )
	`, agentID, platformIntroRunID, platformCheckinRunID); err != nil {
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

	AvatarURL       *string          `json:"avatar_url,omitempty"`
	Personality     *personalityDTO  `json:"personality,omitempty"`
	Interests       *[]string        `json:"interests,omitempty"`
	Capabilities    *[]string        `json:"capabilities,omitempty"`
	Bio             *string          `json:"bio,omitempty"`
	Greeting        *string          `json:"greeting,omitempty"`
	Discovery       *discoveryDTO    `json:"discovery,omitempty"`
	Autonomous      *autonomousDTO   `json:"autonomous,omitempty"`
	PersonaTemplateID *string        `json:"persona_template_id,omitempty"`
	AgentPublicKey  *string          `json:"agent_public_key,omitempty"`
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

	if req.Name == nil &&
		req.Description == nil &&
		req.Status == nil &&
		req.AvatarURL == nil &&
		req.Personality == nil &&
		req.Interests == nil &&
		req.Capabilities == nil &&
		req.Bio == nil &&
		req.Greeting == nil &&
		req.Discovery == nil &&
		req.Autonomous == nil &&
		req.PersonaTemplateID == nil &&
		req.AgentPublicKey == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields"})
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

	var (
		curName           string
		curDescription    string
		curStatus         string
		curAvatarURL      string
		curPersonalityRaw []byte
		curInterestsRaw   []byte
		curCapabilitiesRaw []byte
		curBio            string
		curGreeting       string
		curDiscoveryRaw   []byte
		curAutonomousRaw  []byte
		curPersonaRaw     []byte
		curAgentPubKey    string
		curCardVersion    int
		curPromptView     string
		curCardCertRaw    []byte
	)
	err = tx.QueryRow(ctx, `
		select
			name, description, status, avatar_url,
			personality, interests, capabilities,
			bio, greeting,
			discovery, autonomous,
			persona,
			agent_public_key,
			card_version,
			prompt_view,
			card_cert
		from agents
		where id = $1 and owner_id = $2
	`, agentID, userID).Scan(
		&curName, &curDescription, &curStatus, &curAvatarURL,
		&curPersonalityRaw, &curInterestsRaw, &curCapabilitiesRaw,
		&curBio, &curGreeting,
		&curDiscoveryRaw, &curAutonomousRaw,
		&curPersonaRaw,
		&curAgentPubKey,
		&curCardVersion,
		&curPromptView,
		&curCardCertRaw,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "query agent for update failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	name := strings.TrimSpace(curName)
	description := curDescription
	status := strings.TrimSpace(curStatus)
	avatarURL := strings.TrimSpace(curAvatarURL)
	bio := curBio
	greeting := curGreeting

	var personality personalityDTO
	if err := unmarshalJSONNullable(curPersonalityRaw, &personality); err != nil || personality.Validate() != nil {
		personality = defaultPersonality()
	}
	var interests []string
	if err := unmarshalJSONNullable(curInterestsRaw, &interests); err != nil {
		interests = []string{}
	}
	var capabilities []string
	if err := unmarshalJSONNullable(curCapabilitiesRaw, &capabilities); err != nil {
		capabilities = []string{}
	}
	var discovery discoveryDTO
	_ = unmarshalJSONNullable(curDiscoveryRaw, &discovery)
	var autonomous autonomousDTO
	if err := unmarshalJSONNullable(curAutonomousRaw, &autonomous); err != nil || autonomous.Validate() != nil {
		autonomous = defaultAutonomous()
	}
	var personaAny any
	_ = unmarshalJSONNullable(curPersonaRaw, &personaAny)
	if m, ok := personaAny.(map[string]any); ok && len(m) == 0 {
		personaAny = nil
	}
	personaJSON := []byte(nil)

	cardChanged := false

	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" || len(n) > 64 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid name"})
			return
		}
		if n != name {
			cardChanged = true
		}
		name = n
	}
	if req.Description != nil {
		if len(*req.Description) > 512 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description too long"})
			return
		}
		if *req.Description != description {
			cardChanged = true
		}
		description = *req.Description
	}
	if req.Status != nil {
		st := strings.TrimSpace(*req.Status)
		if st != "enabled" && st != "disabled" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
		status = st
	}
	if req.AvatarURL != nil {
		u := strings.TrimSpace(*req.AvatarURL)
		if len(u) > 1024 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "avatar_url too long"})
			return
		}
		if u != avatarURL {
			cardChanged = true
		}
		avatarURL = u
	}
	if req.Personality != nil {
		if err := req.Personality.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid personality"})
			return
		}
		personality = *req.Personality
		cardChanged = true
	}
	if req.Interests != nil {
		interests = normalizeStringList(*req.Interests, 24, 64)
		cardChanged = true
	}
	if req.Capabilities != nil {
		capabilities = normalizeStringList(*req.Capabilities, 24, 64)
		cardChanged = true
	}
	if req.Bio != nil {
		b := strings.TrimSpace(*req.Bio)
		if len(b) > 2000 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bio too long"})
			return
		}
		bio = b
		cardChanged = true
	}
	if req.Greeting != nil {
		g := strings.TrimSpace(*req.Greeting)
		if len(g) > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "greeting too long"})
			return
		}
		if g != greeting {
			cardChanged = true
		}
		greeting = g
	}
	if req.Discovery != nil {
		if discovery.Public != req.Discovery.Public {
			cardChanged = true
		}
		discovery.Public = req.Discovery.Public
	}
	if req.Autonomous != nil {
		if err := req.Autonomous.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid autonomous config"})
			return
		}
		cardChanged = true
		autonomous = *req.Autonomous
	}
	if req.PersonaTemplateID != nil {
		tid := strings.TrimSpace(*req.PersonaTemplateID)
		if tid == "" {
			personaAny = nil
			cardChanged = true
		} else {
			var personaRaw []byte
			if err := tx.QueryRow(ctx, `
				select persona
				from persona_templates
				where id = $1 and review_status = 'approved'
			`, tid).Scan(&personaRaw); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid persona_template_id"})
					return
				}
				logError(ctx, "query persona template failed", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
				return
			}
			var p any
			if err := json.Unmarshal(personaRaw, &p); err != nil {
				logError(ctx, "unmarshal persona template failed", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decode failed"})
				return
			}
			personaAny = p
			personaJSON = personaRaw
			cardChanged = true
		}
	}

	admittedStatus := ""
	admittedAt := (*time.Time)(nil)
	agentPublicKey := strings.TrimSpace(curAgentPubKey)
	if req.AgentPublicKey != nil {
		if strings.TrimSpace(curAgentPubKey) != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_public_key already set"})
			return
		}
		k := strings.TrimSpace(*req.AgentPublicKey)
		if k != "" {
			pub, err := agenthome.ParseEd25519PublicKey(k)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_public_key"})
				return
			}
			agentPublicKey = "ed25519:" + base64.StdEncoding.EncodeToString(pub)
			admittedStatus = "not_requested"
			admittedAt = nil
			cardChanged = true
		}
	}

	personalityJSON, err := marshalJSONB(personality)
	if err != nil {
		logError(ctx, "marshal personality failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	interestsJSON, err := marshalJSONB(interests)
	if err != nil {
		logError(ctx, "marshal interests failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	capabilitiesJSON, err := marshalJSONB(capabilities)
	if err != nil {
		logError(ctx, "marshal capabilities failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	discoveryJSON, err := marshalJSONB(discovery)
	if err != nil {
		logError(ctx, "marshal discovery failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	autonomousJSON, err := marshalJSONB(autonomous)
	if err != nil {
		logError(ctx, "marshal autonomous failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	promptView := ""
	if cardChanged {
		curCardVersion++
		promptView = promptViewFromFields(name, personaAny, personality, interests, capabilities, bio)
		if len([]rune(promptView)) > s.promptViewMaxChars {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt_view too long"})
			return
		}
	} else {
		// Keep prompt_view stable if card fields didn't change.
		promptView = ""
	}

	// If persona template wasn't changed in this request, preserve existing JSONB.
	if req.PersonaTemplateID == nil {
		personaJSON = curPersonaRaw
	}

	cardCertJSON := curCardCertRaw
	if cardChanged {
		// Invalidate existing certification on any card update.
		cardCertJSON = []byte("{}")
	}

	// Update statement.
	_, err = tx.Exec(ctx, `
		update agents
		set
			name = $1,
			description = $2,
			status = $3,
			avatar_url = $4,
			personality = $5,
			interests = $6,
			capabilities = $7,
			bio = $8,
			greeting = $9,
			discovery = $10,
			autonomous = $11,
			persona = $12,
			agent_public_key = $13,
			admitted_status = case when $14 <> '' then $14 else admitted_status end,
			admitted_at = case when $14 <> '' then $15 else admitted_at end,
			card_version = $16,
			prompt_view = case when $17 <> '' then $17 else prompt_view end,
			card_cert = $18,
			updated_at = now()
		where id = $19 and owner_id = $20
	`,
		name,
		description,
		status,
		avatarURL,
		personalityJSON,
		interestsJSON,
		capabilitiesJSON,
		bio,
		greeting,
		discoveryJSON,
		autonomousJSON,
		personaJSON,
		agentPublicKey,
		admittedStatus,
		admittedAt,
		curCardVersion,
		promptView,
		cardCertJSON,
		agentID,
		userID,
	)
	if err != nil {
		logError(ctx, "update agent failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "db commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
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
	Goal         string     `json:"goal"`
	Constraints  string     `json:"constraints"`
	RequiredTags []string   `json:"required_tags"`
	ScheduledAt  *time.Time `json:"scheduled_at,omitempty"`
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
	if err := s.db.QueryRow(ctx, `select completed_work_items from owner_contributions where owner_id=$1`, userID).Scan(&completed); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			logError(ctx, "contribution gate lookup failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "contribution check failed"})
			return
		}
		// Treat missing row as zero contributions.
		completed = 0
	}
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
	workItemID, err := s.createInitialWorkItemAndOffers(ctx, tx, runID, userID, req.RequiredTags, req.ScheduledAt)
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

type stageTemplate struct {
	StageDescription string
	OutputDesc       string
	OutputLength     string
	OutputFormat     string
}

var stageTemplates = map[string]stageTemplate{
	"ideation": {
		StageDescription: "构思：生成创意方向",
		OutputDesc:       "创意摘要（要点列表即可）",
		OutputLength:     "100-200 字",
		OutputFormat:     "plain text",
	},
	"onboarding": {
		StageDescription: "入驻自我介绍：让大家认识你",
		OutputDesc:       "自我介绍（擅长方向/能力边界/偏好）+ 一段你能稳定产出的内容",
		OutputLength:     "200-400 字",
		OutputFormat:     "markdown",
	},
	"checkin": {
		StageDescription: "每日签到：提交今天的状态与计划",
		OutputDesc:       "今日签到（日期）+ 今日状态/计划（要点）",
		OutputLength:     "80-200 字",
		OutputFormat:     "markdown",
	},
	"review": {
		StageDescription: "互评：对同伴作品给出可执行反馈",
		OutputDesc:       "指出优点/问题/修改建议（可落地）",
		OutputLength:     "100-200 字",
		OutputFormat:     "markdown",
	},
}

func (s server) stageContextForStage(stage string, skills []string) map[string]any {
	tpl, ok := stageTemplates[stage]
	if !ok {
		tpl = stageTemplate{
			StageDescription: stage,
			OutputDesc:       "遵循目标与约束",
			OutputLength:     "",
			OutputFormat:     "plain text",
		}
	}
	expectedOutput := map[string]any{
		"description": tpl.OutputDesc,
		"length":      tpl.OutputLength,
		"format":      tpl.OutputFormat,
	}
	return map[string]any{
		"stage_description":  tpl.StageDescription,
		"expected_output":    expectedOutput,
		"available_skills":   skills,
		"previous_artifacts": []any{},
		// Backward-compatible convenience fields.
		"format": tpl.OutputFormat,
	}
}

func (s server) createInitialWorkItemAndOffers(ctx context.Context, tx pgx.Tx, runID uuid.UUID, publisherUserID uuid.UUID, requiredTags []string, scheduledAt *time.Time) (uuid.UUID, error) {
	agentIDs, err := s.matchAgentsForRun(ctx, tx, publisherUserID, requiredTags, s.matchingParticipantCount)
	if err != nil {
		return uuid.Nil, err
	}

	skills := s.skillsGatewayWhitelist
	if skills == nil {
		skills = []string{}
	}
	availableSkillsJSON, err := json.Marshal(skills)
	if err != nil {
		logError(ctx, "marshal available_skills failed", err)
		return uuid.Nil, err
	}
	stageContextJSON, err := json.Marshal(s.stageContextForStage("ideation", skills))
	if err != nil {
		logError(ctx, "marshal stage_context failed", err)
		return uuid.Nil, err
	}

	// Determine initial status based on scheduled_at
	status := "offered"
	if scheduledAt != nil {
		status = "scheduled"
	}

	var workItemID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into work_items (run_id, stage, kind, status, context, available_skills, scheduled_at)
		values ($1, 'ideation', 'draft', $2, $3, $4, $5)
		returning id
	`, runID, status, stageContextJSON, availableSkillsJSON, scheduledAt).Scan(&workItemID); err != nil {
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

// schedulePendingWorkItems transitions scheduled work items to offered when their scheduled time arrives
func (s server) schedulePendingWorkItems(ctx context.Context) {
	_, err := s.db.Exec(ctx, `
		update work_items
		set status = 'offered', updated_at = now()
		where status = 'scheduled'
		and scheduled_at is not null
		and scheduled_at <= now()
	`)
	if err != nil {
		// Scheduler should be resilient: log and continue next tick.
		logError(ctx, "schedulePendingWorkItems update failed", err)
	}
}

func (s server) matchAgentsForRun(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, publisherUserID uuid.UUID, requiredTags []string, limit int) ([]uuid.UUID, error) {
	// Policy (MVP):
	// - Prefer including publisher-owned enabled agents (so a solo user can publish + have their own agents participate).
	// - Use requiredTags as a preference signal; relax automatically when the agent pool is small (cold-start friendly).
	// - Fill remaining slots with other enabled agents for exploration.
	requiredTags = normalizeTags(requiredTags)
	if limit < 1 {
		limit = 1
	}

	ownerCandidates, err := s.matchOwnerAgents(ctx, q, publisherUserID, requiredTags, limit)
	if err != nil {
		return nil, err
	}
	remaining := limit - len(ownerCandidates)
	otherCandidates, err := s.matchNonOwnerAgents(ctx, q, publisherUserID, requiredTags, remaining)
	if err != nil {
		return nil, err
	}

	out := make([]uuid.UUID, 0, limit)
	out = append(out, ownerCandidates...)
	out = append(out, otherCandidates...)
	if len(out) == 0 {
		return nil, errors.New("no eligible agents")
	}
	return out, nil
}

func (s server) matchOwnerAgents(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, ownerID uuid.UUID, requiredTags []string, limit int) ([]uuid.UUID, error) {
	if limit < 1 {
		return nil, nil
	}
	var (
		rows pgx.Rows
		err  error
	)
	if len(requiredTags) == 0 {
		rows, err = q.Query(ctx, `select id from agents where status='enabled' and owner_id=$1 order by random() limit $2`, ownerID, limit)
	} else {
		rows, err = q.Query(ctx, `
			select a.id
			from agents a
			left join agent_tags t on t.agent_id = a.id and t.tag = any($2)
			where a.status='enabled' and a.owner_id=$1
			group by a.id
			order by count(distinct t.tag) desc, random()
			limit $3
		`, ownerID, requiredTags, limit)
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
}, ownerID uuid.UUID, requiredTags []string, limit int) ([]uuid.UUID, error) {
	if limit < 1 {
		return nil, nil
	}
	var (
		rows pgx.Rows
		err  error
	)
	if len(requiredTags) == 0 {
		rows, err = q.Query(ctx, `select id from agents where status='enabled' and owner_id <> $1 order by random() limit $2`, ownerID, limit)
	} else {
		rows, err = q.Query(ctx, `
			select a.id
			from agents a
			left join agent_tags t on t.agent_id = a.id and t.tag = any($2)
			where a.status='enabled' and a.owner_id <> $1
			group by a.id
			order by count(distinct t.tag) desc, random()
			limit $3
		`, ownerID, requiredTags, limit)
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
	IsSystem      bool   `json:"is_system"`
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

	// Always add platform user id as the first parameter so we can compute `is_system`
	// in the select list without shifting dynamic placeholders later.
	platformArg := argN
	args = append(args, platformUserID)
	argN++

	if !includeSystem {
		where = append(where, "r.publisher_user_id <> $"+strconv.Itoa(platformArg))
	}

	// Rejected runs are not discoverable via public list/search.
	where = append(where, "r.review_status <> 'rejected'")

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
		       coalesce(a.kind, '') as output_kind,
		       (r.publisher_user_id = $` + strconv.Itoa(platformArg) + `) as is_system
		from runs r
		left join lateral (
			select version, kind,
			       case when review_status = 'rejected' then '' else content end as content
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
		var isSystem bool
		if err := rows.Scan(&id, &goal, &constraints, &status, &createdAt, &updatedAt, &outVer, &outKind, &isSystem); err != nil {
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
			IsSystem:      isSystem,
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
	var reviewStatus string
	err = s.db.QueryRow(ctx, `
		select id, goal, constraints, status, created_at, review_status
		from runs
		where id = $1
	`, runID).Scan(&dto.ID, &dto.Goal, &dto.Constraints, &dto.Status, &createdAt, &reviewStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if reviewStatus == "rejected" {
		dto.Goal = "该内容已被管理员审核后屏蔽"
		dto.Constraints = "该内容已被管理员审核后屏蔽"
	}
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	writeJSON(w, http.StatusOK, dto)
}
