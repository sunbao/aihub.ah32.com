package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type agentFullDTO struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Status        string         `json:"status"`
	Tags          []string       `json:"tags"`
	AvatarURL     string         `json:"avatar_url"`
	Personality   personalityDTO `json:"personality"`
	Interests     []string       `json:"interests"`
	Capabilities  []string       `json:"capabilities"`
	Bio           string         `json:"bio"`
	Greeting      string         `json:"greeting"`
	Persona       any            `json:"persona,omitempty"`
	PromptView    string         `json:"prompt_view"`
	CardVersion   int            `json:"card_version"`
	CardCert      any            `json:"card_cert,omitempty"`
	CardReview    string         `json:"card_review_status"`
	AgentPublicKey string        `json:"agent_public_key"`
	Admission     map[string]any `json:"admission"`
	Discovery     discoveryDTO   `json:"discovery"`
	Autonomous    autonomousDTO  `json:"autonomous"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

func (s server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		name           string
		description    string
		status         string
		avatarURL      string
		personalityRaw []byte
		interestsRaw   []byte
		capabilitiesRaw []byte
		bio            string
		greeting       string
		discoveryRaw   []byte
		autonomousRaw  []byte
		personaRaw     []byte
		promptView     string
		cardVersion    int
		cardCertRaw    []byte
		cardReview     string
		agentPubKey    string
		admittedStatus string
		admittedAt     *time.Time
		createdAt      time.Time
		updatedAt      time.Time
	)

	err = s.db.QueryRow(ctx, `
		select
			name,
			description,
			status,
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
			agent_public_key,
			admitted_status,
			admitted_at,
			created_at,
			updated_at
		from agents
		where id = $1 and owner_id = $2
	`, agentID, userID).Scan(
		&name,
		&description,
		&status,
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
		&agentPubKey,
		&admittedStatus,
		&admittedAt,
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

	admission := map[string]any{"status": admittedStatus}
	if admittedAt != nil {
		admission["admitted_at"] = admittedAt.UTC().Format(time.RFC3339)
	}

	writeJSON(w, http.StatusOK, agentFullDTO{
		ID:             agentID.String(),
		Name:           strings.TrimSpace(name),
		Description:    strings.TrimSpace(description),
		Status:         strings.TrimSpace(status),
		Tags:           tags,
		AvatarURL:      strings.TrimSpace(avatarURL),
		Personality:    personality,
		Interests:      interests,
		Capabilities:   capabilities,
		Bio:            strings.TrimSpace(bio),
		Greeting:       strings.TrimSpace(greeting),
		Persona:        persona,
		PromptView:     strings.TrimSpace(promptView),
		CardVersion:    cardVersion,
		CardCert:       cardCert,
		CardReview:     strings.TrimSpace(cardReview),
		AgentPublicKey: strings.TrimSpace(agentPubKey),
		Admission:      admission,
		Discovery:      discovery,
		Autonomous:     autonomous,
		CreatedAt:      createdAt.UTC().Format(time.RFC3339),
		UpdatedAt:      updatedAt.UTC().Format(time.RFC3339),
	})
}

type discoverAgentItemDTO struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	AvatarURL   string   `json:"avatar_url"`
	PromptView  string   `json:"prompt_view"`
	Interests   []string `json:"interests,omitempty"`
	Personality *personalityDTO `json:"personality,omitempty"`
	MatchScore  float64  `json:"match_score"`
}

func (s server) handleDiscoverAgents(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q too long"})
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)

	interestQuery := normalizeStringList(strings.Split(strings.TrimSpace(r.URL.Query().Get("interests")), ","), 6, 64)

	var target personalityDTO
	targetProvided := false
	if v := strings.TrimSpace(r.URL.Query().Get("p_extrovert")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			target.Extrovert = f
			targetProvided = true
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("p_curious")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			target.Curious = f
			targetProvided = true
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("p_creative")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			target.Creative = f
			targetProvided = true
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("p_stable")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			target.Stable = f
			targetProvided = true
		}
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
		`admitted_status = 'admitted'`,
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
		select id, name, description, avatar_url, prompt_view, interests, personality
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
		id          uuid.UUID
		name        string
		description string
		avatarURL   string
		promptView  string
		interestsRaw []byte
		personalityRaw []byte
	}
	var items []rowItem
	for rows.Next() {
		var it rowItem
		if err := rows.Scan(&it.id, &it.name, &it.description, &it.avatarURL, &it.promptView, &it.interestsRaw, &it.personalityRaw); err != nil {
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
				ID:          it.id.String(),
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
			return scoredItems[i].dto.ID < scoredItems[j].dto.ID
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
	RunID     string `json:"run_id"`
	Goal      string `json:"goal"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type discoverAgentDetailDTO struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	AvatarURL    string                 `json:"avatar_url"`
	Bio          string                 `json:"bio"`
	Greeting     string                 `json:"greeting"`
	PromptView   string                 `json:"prompt_view"`
	Persona      any                    `json:"persona,omitempty"`
	Interests    []string               `json:"interests,omitempty"`
	Capabilities []string               `json:"capabilities,omitempty"`
	Personality  personalityDTO         `json:"personality,omitempty"`
	RecentRuns   []discoverAgentRecentRunDTO `json:"recent_runs,omitempty"`
}

func (s server) handleDiscoverAgentDetail(w http.ResponseWriter, r *http.Request) {
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		name           string
		description    string
		avatarURL      string
		promptView     string
		bio            string
		greeting       string
		personaRaw     []byte
		interestsRaw   []byte
		capabilitiesRaw []byte
		personalityRaw []byte
		cardReview     string
	)
	err = s.db.QueryRow(ctx, `
		select name, description, avatar_url, prompt_view, bio, greeting,
		       coalesce(persona, '{}'::jsonb),
		       interests, capabilities, personality, card_review_status
		from agents
		where id = $1
		  and status = 'enabled'
		  and admitted_status = 'admitted'
		  and coalesce(discovery->>'public','false') = 'true'
		  and card_review_status = 'approved'
	`, agentID).Scan(&name, &description, &avatarURL, &promptView, &bio, &greeting, &personaRaw, &interestsRaw, &capabilitiesRaw, &personalityRaw, &cardReview)
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
		select r.id, r.goal, r.status, r.created_at
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
				runID     uuid.UUID
				goal      string
				status    string
				createdAt time.Time
			)
			if err := rows.Scan(&runID, &goal, &status, &createdAt); err != nil {
				logError(ctx, "discover agent recent runs scan failed", err)
				break
			}
			recent = append(recent, discoverAgentRecentRunDTO{
				RunID:     runID.String(),
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
		ID:           agentID.String(),
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

func (s server) handleSyncAgentToOSS(w http.ResponseWriter, r *http.Request) {
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
	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	var (
		name           string
		description    string
		avatarURL      string
		personalityRaw []byte
		interestsRaw   []byte
		capabilitiesRaw []byte
		bio            string
		greeting       string
		discoveryRaw   []byte
		autonomousRaw  []byte
		personaRaw     []byte
		promptView     string
		cardVersion    int
		cardReview     string
		agentPubKey    string
	)
	err = s.db.QueryRow(ctx, `
		select name, description, avatar_url, personality, interests, capabilities, bio, greeting, discovery, autonomous,
		       coalesce(persona, '{}'::jsonb), prompt_view, card_version, card_review_status, agent_public_key
		from agents
		where id = $1 and owner_id = $2
	`, agentID, userID).Scan(
		&name, &description, &avatarURL, &personalityRaw, &interestsRaw, &capabilitiesRaw, &bio, &greeting, &discoveryRaw, &autonomousRaw,
		&personaRaw, &promptView, &cardVersion, &cardReview, &agentPubKey,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "query agent for sync failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if strings.TrimSpace(cardReview) != "approved" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "agent card not approved"})
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
		logError(ctx, "sync agent to oss personality unmarshal failed", err)
		personality = defaultPersonality()
	} else if personality.Validate() != nil {
		logError(ctx, "sync agent to oss personality invalid", errors.New("invalid personality"))
		personality = defaultPersonality()
	}
	var interests []string
	if err := unmarshalJSONNullable(interestsRaw, &interests); err != nil {
		logError(ctx, "sync agent to oss interests unmarshal failed", err)
	}
	var capabilities []string
	if err := unmarshalJSONNullable(capabilitiesRaw, &capabilities); err != nil {
		logError(ctx, "sync agent to oss capabilities unmarshal failed", err)
	}
	var discovery discoveryDTO
	if err := unmarshalJSONNullable(discoveryRaw, &discovery); err != nil {
		logError(ctx, "sync agent to oss discovery unmarshal failed", err)
	}
	var autonomous autonomousDTO
	if err := unmarshalJSONNullable(autonomousRaw, &autonomous); err != nil {
		logError(ctx, "sync agent to oss autonomous unmarshal failed", err)
		autonomous = defaultAutonomous()
	} else if autonomous.Validate() != nil {
		logError(ctx, "sync agent to oss autonomous invalid", errors.New("invalid autonomous"))
		autonomous = defaultAutonomous()
	}
	var persona any
	if err := unmarshalJSONNullable(personaRaw, &persona); err != nil {
		logError(ctx, "sync agent to oss persona unmarshal failed", err)
	}
	if m, ok := persona.(map[string]any); ok && len(m) == 0 {
		persona = nil
	}

	if strings.TrimSpace(promptView) == "" {
		promptView = promptViewFromFields(name, persona, personality, interests, capabilities, bio)
	}
	if len([]rune(promptView)) > s.promptViewMaxChars {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt_view too long"})
		return
	}

	obj := map[string]any{
		"kind":          "agent_card",
		"schema_version": 1,
		"agent_id":      agentID.String(),
		"card_version":  cardVersion,
		"name":          strings.TrimSpace(name),
		"description":   strings.TrimSpace(description),
		"avatar_url":    strings.TrimSpace(avatarURL),
		"personality":   personality,
		"interests":     interests,
		"capabilities":  capabilities,
		"bio":           strings.TrimSpace(bio),
		"greeting":      strings.TrimSpace(greeting),
		"persona":       persona,
		"prompt_view":   promptView,
		"agent_public_key": strings.TrimSpace(agentPubKey),
		"discovery": map[string]any{
			"public": discovery.Public,
			"tags":   tags,
		},
		"autonomous": autonomous,
	}

	cert, err := s.signObject(ctx, obj)
	if err != nil {
		logError(ctx, "sign agent card failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
		return
	}
	obj["cert"] = cert

	body, err := json.Marshal(obj)
	if err != nil {
		logError(ctx, "marshal agent card failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	objectKey := "agents/all/" + agentID.String() + ".json"
	if err := store.PutObject(ctx, objectKey, "application/json", body); err != nil {
		logError(ctx, "put agent card failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	ossEndpoint := "oss://" + strings.TrimSpace(s.ossBucket) + "/" + agenthome.JoinKey(s.ossBasePrefix, objectKey)

	// Also publish a signed prompt bundle (agent-private).
	promptBundle := buildPromptBundle(agentID.String(), name, persona, promptView)
	bundleCert, err := s.signObject(ctx, promptBundle)
	if err != nil {
		logError(ctx, "sign prompt bundle failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "platform signing not configured"})
		return
	}
	promptBundle["cert"] = bundleCert
	bundleBody, err := json.Marshal(promptBundle)
	if err != nil {
		logError(ctx, "marshal prompt bundle failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	bundleKey := "agents/prompts/" + agentID.String() + "/bundle.json"
	if err := store.PutObject(ctx, bundleKey, "application/json", bundleBody); err != nil {
		logError(ctx, "put prompt bundle failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss write failed"})
		return
	}

	// Update discovery + cert in DB (best-effort; do not roll back OSS write).
	discovery.OSSEndpoint = ossEndpoint
	discovery.LastSyncedAt = nowRFC3339()
	discoveryJSON, err := marshalJSONB(discovery)
	if err != nil {
		logError(ctx, "marshal discovery failed", err)
	} else {
		certJSON, err := marshalJSONB(cert)
		if err != nil {
			logError(ctx, "marshal cert failed", err)
		} else {
			if _, err := s.db.Exec(ctx, `
				update agents
				set discovery = $1,
				    prompt_view = $2,
				    card_cert = $3,
				    updated_at = now()
				where id = $4 and owner_id = $5
			`, discoveryJSON, promptView, certJSON, agentID, userID); err != nil {
				logError(ctx, "update agent sync metadata failed", err)
			}
		}
	}

	s.audit(ctx, "user", userID, "agent_synced_to_oss", map[string]any{
		"agent_id":      agentID.String(),
		"object_key":    objectKey,
		"bundle_version": promptBundle["bundle_version"],
		"ab_group":      promptBundle["ab_group"],
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"oss_endpoint": ossEndpoint,
		"card_version": cardVersion,
	})
}

type admissionStartResponse struct {
	Challenge  string `json:"challenge"`
	ExpiresAt  string `json:"expires_at"`
}

func (s server) handleAdmissionStart(w http.ResponseWriter, r *http.Request) {
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var pubKey string
	if err := s.db.QueryRow(ctx, `select agent_public_key from agents where id=$1 and owner_id=$2`, agentID, userID).Scan(&pubKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "query agent_public_key failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if strings.TrimSpace(pubKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing agent_public_key"})
		return
	}

	chal, err := agenthome.NewRandomChallenge()
	if err != nil {
		logError(ctx, "generate admission challenge failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "challenge generation failed"})
		return
	}
	expiresAt := time.Now().Add(10 * time.Minute).UTC()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		insert into agent_admission_challenges (agent_id, challenge, expires_at)
		values ($1, $2, $3)
	`, agentID, chal, expiresAt); err != nil {
		logError(ctx, "insert admission challenge failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}
	if _, err := tx.Exec(ctx, `
		update agents
		set admitted_status = 'pending', updated_at = now()
		where id=$1 and owner_id=$2
	`, agentID, userID); err != nil {
		logError(ctx, "update admitted_status pending failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "db commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "user", userID, "agent_admission_started", map[string]any{"agent_id": agentID.String()})
	writeJSON(w, http.StatusOK, admissionStartResponse{Challenge: chal, ExpiresAt: expiresAt.Format(time.RFC3339)})
}

func (s server) handleAdmissionChallenge(w http.ResponseWriter, r *http.Request) {
	agentIDFromAuth, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	if agentID != agentIDFromAuth {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var (
		chal      string
		expiresAt time.Time
	)
	err = s.db.QueryRow(ctx, `
		select challenge, expires_at
		from agent_admission_challenges
		where agent_id = $1 and consumed_at is null and expires_at > now()
		order by created_at desc
		limit 1
	`, agentID).Scan(&chal, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no active challenge"})
		return
	}
	if err != nil {
		logError(ctx, "query admission challenge failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	writeJSON(w, http.StatusOK, admissionStartResponse{Challenge: chal, ExpiresAt: expiresAt.UTC().Format(time.RFC3339)})
}

type admissionCompleteRequest struct {
	Signature string `json:"signature"`
}

func (s server) handleAdmissionComplete(w http.ResponseWriter, r *http.Request) {
	agentIDFromAuth, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}
	if agentID != agentIDFromAuth {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req admissionCompleteRequest
	if !readJSONLimited(w, r, &req, 16*1024) {
		return
	}
	req.Signature = strings.TrimSpace(req.Signature)
	if req.Signature == "" || len(req.Signature) > 1024 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid signature"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		pubKeyStr string
		chalID    uuid.UUID
		chal      string
		expiresAt time.Time
	)
	err = s.db.QueryRow(ctx, `
		select a.agent_public_key, c.id, c.challenge, c.expires_at
		from agents a
		join agent_admission_challenges c on c.agent_id = a.id
		where a.id = $1
		  and c.consumed_at is null
		  and c.expires_at > now()
		order by c.created_at desc
		limit 1
	`, agentID).Scan(&pubKeyStr, &chalID, &chal, &expiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no active challenge"})
		return
	}
	if err != nil {
		logError(ctx, "query admission context failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	pubKey, err := agenthome.ParseEd25519PublicKey(pubKeyStr)
	if err != nil {
		logError(ctx, "parse agent_public_key failed", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_public_key"})
		return
	}

	okSig, err := agenthome.VerifyEd25519Base64(pubKey, []byte(chal), req.Signature)
	if err != nil {
		logError(ctx, "verify admission signature failed", err)
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid signature"})
		return
	}
	if !okSig {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "signature verify failed"})
		return
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `update agent_admission_challenges set consumed_at = now() where id = $1`, chalID); err != nil {
		logError(ctx, "consume admission challenge failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if _, err := tx.Exec(ctx, `
		update agents
		set admitted_status = 'admitted', admitted_at = now(), updated_at = now()
		where id = $1
	`, agentID); err != nil {
		logError(ctx, "update admitted status failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "db commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "agent", agentID, "agent_admitted", map[string]any{"agent_id": agentID.String(), "expires_at": expiresAt.UTC().Format(time.RFC3339)})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "admitted"})
}
