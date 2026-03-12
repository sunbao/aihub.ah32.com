package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type agentFullDTO struct {
	AgentRef     string         `json:"agent_ref"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Status       string         `json:"status"`
	IdentityMode string         `json:"identity_mode"`
	Tags         []string       `json:"tags"`
	AvatarURL    string         `json:"avatar_url"`
	Personality  personalityDTO `json:"personality"`
	Interests    []string       `json:"interests"`
	Capabilities []string       `json:"capabilities"`
	Bio          string         `json:"bio"`
	Greeting     string         `json:"greeting"`
	Persona      any            `json:"persona,omitempty"`
	PromptView   string         `json:"prompt_view"`
	CardVersion  int            `json:"card_version"`
	CardCert     any            `json:"card_cert,omitempty"`
	CardReview   string         `json:"card_review_status"`
	Discovery    discoveryDTO   `json:"discovery"`
	Autonomous   autonomousDTO  `json:"autonomous"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

func (s server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentRef, ok := requireAgentRefParam(w, r, "agentRef")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var agentID uuid.UUID
	var (
		name            string
		description     string
		status          string
		identityMode    string
		avatarURL       string
		personalityRaw  []byte
		interestsRaw    []byte
		capabilitiesRaw []byte
		bio             string
		greeting        string
		discoveryRaw    []byte
		autonomousRaw   []byte
		personaRaw      []byte
		promptView      string
		cardVersion     int
		cardCertRaw     []byte
		cardReview      string
		createdAt       time.Time
		updatedAt       time.Time
	)

	err := s.db.QueryRow(ctx, `
		select
			id,
			name,
			description,
			status,
			identity_mode,
			avatar_url,
			personality,
			interests,
			capabilities,
			bio,
			greeting,
			discovery,
			autonomous,
			coalesce(persona, '{}'::jsonb),
			prompt_view,
			card_version,
			card_cert,
			card_review_status,
			created_at,
			updated_at
		from agents
		where public_ref = $1 and owner_id = $2
	`, agentRef, userID).Scan(
		&agentID,
		&name,
		&description,
		&status,
		&identityMode,
		&avatarURL,
		&personalityRaw,
		&interestsRaw,
		&capabilitiesRaw,
		&bio,
		&greeting,
		&discoveryRaw,
		&autonomousRaw,
		&personaRaw,
		&promptView,
		&cardVersion,
		&cardCertRaw,
		&cardReview,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "query agent failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	tags, err := s.listAgentTags(ctx, agentID)
	if err != nil {
		logError(ctx, "list agent tags failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	var personality personalityDTO
	if err := unmarshalJSONNullable(personalityRaw, &personality); err != nil {
		logError(ctx, "unmarshal personality failed", err)
		personality = defaultPersonality()
	}

	var interests []string
	if err := unmarshalJSONNullable(interestsRaw, &interests); err != nil {
		logError(ctx, "unmarshal interests failed", err)
		interests = []string{}
	}

	var capabilities []string
	if err := unmarshalJSONNullable(capabilitiesRaw, &capabilities); err != nil {
		logError(ctx, "unmarshal capabilities failed", err)
		capabilities = []string{}
	}

	var discovery discoveryDTO
	if err := unmarshalJSONNullable(discoveryRaw, &discovery); err != nil {
		logError(ctx, "unmarshal discovery failed", err)
	}

	var autonomous autonomousDTO
	if err := unmarshalJSONNullable(autonomousRaw, &autonomous); err != nil {
		logError(ctx, "unmarshal autonomous failed", err)
		autonomous = defaultAutonomous()
	}

	var persona any
	if err := unmarshalJSONNullable(personaRaw, &persona); err != nil {
		logError(ctx, "unmarshal persona failed", err)
		persona = nil
	}
	// Treat empty object as unset.
	if m, ok := persona.(map[string]any); ok && len(m) == 0 {
		persona = nil
	}

	var cardCert any
	if err := unmarshalJSONNullable(cardCertRaw, &cardCert); err != nil {
		logError(ctx, "unmarshal card_cert failed", err)
		cardCert = nil
	}

	writeJSON(w, http.StatusOK, agentFullDTO{
		AgentRef:     agentRef,
		Name:         strings.TrimSpace(name),
		Description:  strings.TrimSpace(description),
		Status:       strings.TrimSpace(status),
		IdentityMode: strings.TrimSpace(identityMode),
		Tags:         tags,
		AvatarURL:    strings.TrimSpace(avatarURL),
		Personality:  personality,
		Interests:    interests,
		Capabilities: capabilities,
		Bio:          strings.TrimSpace(bio),
		Greeting:     strings.TrimSpace(greeting),
		Persona:      persona,
		PromptView:   strings.TrimSpace(promptView),
		CardVersion:  cardVersion,
		CardCert:     cardCert,
		CardReview:   strings.TrimSpace(cardReview),
		Discovery:    discovery,
		Autonomous:   autonomous,
		CreatedAt:    createdAt.UTC().Format(time.RFC3339),
		UpdatedAt:    updatedAt.UTC().Format(time.RFC3339),
	})
}

type discoverAgentItemDTO struct {
	AgentRef    string          `json:"agent_ref"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	AvatarURL   string          `json:"avatar_url"`
	PromptView  string          `json:"prompt_view"`
	Interests   []string        `json:"interests,omitempty"`
	Personality *personalityDTO `json:"personality,omitempty"`
	MatchScore  float64         `json:"match_score"`
}

func (s server) handleDiscoverAgents(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q too long"})
		return
	}

	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil {
			logError(r.Context(), "parse discover limit failed", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		limit = parsed
	}
	limit = clampInt(limit, 1, 200)

	interestQuery := normalizeStringList(strings.Split(strings.TrimSpace(r.URL.Query().Get("interests")), ","), 6, 64)

	var target personalityDTO
	targetProvided := false
	if v := strings.TrimSpace(r.URL.Query().Get("p_extrovert")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			logError(r.Context(), "parse discover p_extrovert failed", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid p_extrovert"})
			return
		}
		target.Extrovert = f
		targetProvided = true
	}
	if v := strings.TrimSpace(r.URL.Query().Get("p_curious")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			logError(r.Context(), "parse discover p_curious failed", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid p_curious"})
			return
		}
		target.Curious = f
		targetProvided = true
	}
	if v := strings.TrimSpace(r.URL.Query().Get("p_creative")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			logError(r.Context(), "parse discover p_creative failed", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid p_creative"})
			return
		}
		target.Creative = f
		targetProvided = true
	}
	if v := strings.TrimSpace(r.URL.Query().Get("p_stable")); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			logError(r.Context(), "parse discover p_stable failed", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid p_stable"})
			return
		}
		target.Stable = f
		targetProvided = true
	}
	if targetProvided {
		if err := target.Validate(); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid personality target"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	args := make([]any, 0, 2)
	where := []string{
		`status = 'enabled'`,
		`coalesce(discovery->>'public','false') = 'true'`,
		`card_review_status = 'approved'`,
	}
	argN := 1
	if q != "" {
		where = append(where, `(name ilike '%' || $`+strconv.Itoa(argN)+` || '%' or description ilike '%' || $`+strconv.Itoa(argN)+` || '%')`)
		args = append(args, q)
		argN++
	}

	query := `
		select id, public_ref, name, description, avatar_url, prompt_view, interests, personality
		from agents
		where ` + strings.Join(where, " and ") + `
		order by updated_at desc
		limit ` + strconv.Itoa(limit)

	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		logError(ctx, "discover query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type rowItem struct {
		id             uuid.UUID
		agentRef       string
		name           string
		description    string
		avatarURL      string
		promptView     string
		interestsRaw   []byte
		personalityRaw []byte
	}
	var items []rowItem
	for rows.Next() {
		var it rowItem
		if err := rows.Scan(&it.id, &it.agentRef, &it.name, &it.description, &it.avatarURL, &it.promptView, &it.interestsRaw, &it.personalityRaw); err != nil {
			logError(ctx, "discover scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "discover iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	type scored struct {
		dto   discoverAgentItemDTO
		score float64
	}
	scoredItems := make([]scored, 0, len(items))
	for _, it := range items {
		var interests []string
		if err := unmarshalJSONNullable(it.interestsRaw, &interests); err != nil {
			logError(ctx, "discover interests unmarshal failed", err)
		}
		var personality personalityDTO
		if err := unmarshalJSONNullable(it.personalityRaw, &personality); err != nil {
			logError(ctx, "discover personality unmarshal failed", err)
		}
		p := personality

		score := 0.0
		if len(interestQuery) > 0 {
			set := map[string]struct{}{}
			for _, v := range interests {
				set[v] = struct{}{}
			}
			hit := 0
			for _, qv := range interestQuery {
				if _, ok := set[qv]; ok {
					hit++
				}
			}
			score += float64(hit) / float64(len(interestQuery))
		}
		if targetProvided {
			// Simple similarity: 1 - mean absolute error.
			mae := (abs(personality.Extrovert-target.Extrovert) + abs(personality.Curious-target.Curious) + abs(personality.Creative-target.Creative) + abs(personality.Stable-target.Stable)) / 4.0
			score += (1.0 - mae)
		}

		scoredItems = append(scoredItems, scored{
			dto: discoverAgentItemDTO{
				AgentRef:    it.agentRef,
				Name:        strings.TrimSpace(it.name),
				Description: strings.TrimSpace(it.description),
				AvatarURL:   strings.TrimSpace(it.avatarURL),
				PromptView:  strings.TrimSpace(it.promptView),
				Interests:   interests,
				Personality: &p,
				MatchScore:  score,
			},
			score: score,
		})
	}

	sort.SliceStable(scoredItems, func(i, j int) bool {
		if scoredItems[i].score == scoredItems[j].score {
			return scoredItems[i].dto.AgentRef < scoredItems[j].dto.AgentRef
		}
		return scoredItems[i].score > scoredItems[j].score
	})

	out := make([]discoverAgentItemDTO, 0, len(scoredItems))
	for _, it := range scoredItems {
		out = append(out, it.dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type discoverAgentRecentRunDTO struct {
	RunRef    string `json:"run_ref"`
	Goal      string `json:"goal"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type discoverAgentDetailDTO struct {
	AgentRef     string                      `json:"agent_ref"`
	Name         string                      `json:"name"`
	Description  string                      `json:"description"`
	AvatarURL    string                      `json:"avatar_url"`
	Bio          string                      `json:"bio"`
	Greeting     string                      `json:"greeting"`
	PromptView   string                      `json:"prompt_view"`
	Persona      any                         `json:"persona,omitempty"`
	Interests    []string                    `json:"interests,omitempty"`
	Capabilities []string                    `json:"capabilities,omitempty"`
	Personality  personalityDTO              `json:"personality,omitempty"`
	RecentRuns   []discoverAgentRecentRunDTO `json:"recent_runs,omitempty"`
}

func (s server) handleDiscoverAgentDetail(w http.ResponseWriter, r *http.Request) {
	agentRef, ok := requireAgentRefParam(w, r, "agentRef")
	if !ok {
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var agentID uuid.UUID
	var (
		name            string
		description     string
		avatarURL       string
		promptView      string
		bio             string
		greeting        string
		personaRaw      []byte
		interestsRaw    []byte
		capabilitiesRaw []byte
		personalityRaw  []byte
		cardReview      string
	)
	err := s.db.QueryRow(ctx, `
		select id, name, description, avatar_url, prompt_view, bio, greeting,
		       coalesce(persona, '{}'::jsonb),
		       interests, capabilities, personality, card_review_status
		from agents
		where public_ref = $1
		  and status = 'enabled'
		  and coalesce(discovery->>'public','false') = 'true'
		  and card_review_status = 'approved'
	`, agentRef).Scan(&agentID, &name, &description, &avatarURL, &promptView, &bio, &greeting, &personaRaw, &interestsRaw, &capabilitiesRaw, &personalityRaw, &cardReview)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "discover agent detail query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	_ = cardReview

	var interests []string
	if err := unmarshalJSONNullable(interestsRaw, &interests); err != nil {
		logError(ctx, "discover agent detail interests unmarshal failed", err)
	}
	var capabilities []string
	if err := unmarshalJSONNullable(capabilitiesRaw, &capabilities); err != nil {
		logError(ctx, "discover agent detail capabilities unmarshal failed", err)
	}
	var personality personalityDTO
	if err := unmarshalJSONNullable(personalityRaw, &personality); err != nil {
		logError(ctx, "discover agent detail personality unmarshal failed", err)
	}
	var persona any
	if err := unmarshalJSONNullable(personaRaw, &persona); err != nil {
		logError(ctx, "discover agent detail persona unmarshal failed", err)
	}
	if m, ok := persona.(map[string]any); ok && len(m) == 0 {
		persona = nil
	}

	// Best-effort: recent runs where this agent submitted artifacts.
	var recent []discoverAgentRecentRunDTO
	rows, err := s.db.Query(ctx, `
		with contributed as (
			select (data->>'run_id') as run_id, max(created_at) as last_at
			from audit_logs
			where actor_type = 'agent'
			  and actor_id = $1
			  and action = 'artifact_submitted'
			  and coalesce(data->>'run_id','') <> ''
			group by (data->>'run_id')
		)
		select r.public_ref, r.goal, r.status, r.created_at
		from contributed c
		join runs r on r.id::text = c.run_id
		where r.review_status <> 'rejected'
		order by c.last_at desc
		limit 8
	`, agentID)
	if err != nil {
		logError(ctx, "discover agent recent runs query failed", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var (
				runRef    string
				goal      string
				status    string
				createdAt time.Time
			)
			if err := rows.Scan(&runRef, &goal, &status, &createdAt); err != nil {
				logError(ctx, "discover agent recent runs scan failed", err)
				break
			}
			recent = append(recent, discoverAgentRecentRunDTO{
				RunRef:    runRef,
				Goal:      goal,
				Status:    status,
				CreatedAt: createdAt.UTC().Format(time.RFC3339),
			})
		}
		if err := rows.Err(); err != nil {
			logError(ctx, "discover agent recent runs iterate failed", err)
		}
	}

	writeJSON(w, http.StatusOK, discoverAgentDetailDTO{
		AgentRef:     agentRef,
		Name:         strings.TrimSpace(name),
		Description:  strings.TrimSpace(description),
		AvatarURL:    strings.TrimSpace(avatarURL),
		Bio:          strings.TrimSpace(bio),
		Greeting:     strings.TrimSpace(greeting),
		PromptView:   strings.TrimSpace(promptView),
		Persona:      persona,
		Interests:    interests,
		Capabilities: capabilities,
		Personality:  personality,
		RecentRuns:   recent,
	})
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func (s server) ossCfg() agenthome.OSSConfig {
	return agenthome.OSSConfig{
		Provider:           s.ossProvider,
		Endpoint:           s.ossEndpoint,
		Region:             s.ossRegion,
		Bucket:             s.ossBucket,
		BasePrefix:         s.ossBasePrefix,
		AccessKeyID:        s.ossAccessKeyID,
		AccessKeySecret:    s.ossAccessKeySecret,
		STSRoleARN:         s.ossSTSRoleARN,
		STSDurationSeconds: s.ossSTSDurationSeconds,
		LocalDir:           s.ossLocalDir,
	}
}
