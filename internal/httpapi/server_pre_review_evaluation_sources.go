package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"aihub/internal/agenthome"
)

type recentTopicForEvaluationDTO struct {
	TopicID            string `json:"topic_id"`
	Title              string `json:"title"`
	Summary            string `json:"summary,omitempty"`
	Mode               string `json:"mode,omitempty"`
	OpeningQuestion    string `json:"opening_question,omitempty"`
	Category           string `json:"category,omitempty"`
	LastMessagePreview string `json:"last_message_preview,omitempty"`
	LastMessageAt      string `json:"last_message_at,omitempty"`
}

type listRecentTopicsForEvaluationResponse struct {
	Items []recentTopicForEvaluationDTO `json:"items"`
}

type recentRunForEvaluationDTO struct {
	RunRef    string `json:"run_ref"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at,omitempty"`
}

type listRecentRunsForEvaluationResponse struct {
	Items []recentRunForEvaluationDTO `json:"items"`
}

var topicIDFromObjectKeyRe = regexp.MustCompile(`topics/([^/]+)/messages/`)

func parseTopicIDFromObjectKey(objectKey string) string {
	objectKey = strings.TrimSpace(objectKey)
	m := topicIDFromObjectKeyRe.FindStringSubmatch(objectKey)
	if len(m) != 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}

func (s server) handleOwnerListRecentRunsForEvaluation(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	limit := clampInt(int64Query(r, "limit", 10), 1, 50)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select public_ref, goal, created_at
		from runs
		where publisher_user_id = $1
		order by created_at desc
		limit $2
	`, userID, limit)
	if err != nil {
		logError(ctx, "recent runs: query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]recentRunForEvaluationDTO, 0, limit)
	for rows.Next() {
		var (
			runRef    string
			goal      string
			createdAt time.Time
		)
		if err := rows.Scan(&runRef, &goal, &createdAt); err != nil {
			logError(ctx, "recent runs: scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		out = append(out, recentRunForEvaluationDTO{
			RunRef:    strings.TrimSpace(runRef),
			Title:     strings.TrimSpace(goal),
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "recent runs: iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, listRecentRunsForEvaluationResponse{Items: out})
}

func (s server) handleOwnerListRecentTopicsForEvaluation(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	limit := clampInt(int64Query(r, "limit", 10), 1, 50)
	candidateAgentRefRaw := strings.TrimSpace(r.URL.Query().Get("candidate_agent_ref"))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

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
		logError(ctx, "recent topics: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	ownedAgentRefs, err := s.listOwnerAgentRefs(ctx, userID, 50)
	if err != nil {
		logError(ctx, "recent topics: list owner agents failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	candidateAgentRef := ""
	if candidateAgentRefRaw != "" {
		ref, err := parseAgentRef(candidateAgentRefRaw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid candidate_agent_ref"})
			return
		}
		// Validate ownership so the UI can't probe arbitrary refs.
		if _, err := s.lookupOwnerAgentIDByRef(ctx, userID, ref); err != nil {
			logError(ctx, "recent topics: lookup candidate agent by agent_ref failed", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid candidate_agent_ref"})
			return
		}
		candidateAgentRef = ref
	}

	type evRow struct {
		objectKey  string
		occurredAt time.Time
		payloadB   []byte
	}

	rows, err := s.db.Query(ctx, `
		select object_key, occurred_at, payload
		from oss_events
		where object_key like '%topics/%/messages/%'
		order by occurred_at desc
		limit 300
	`)
	if err != nil {
		logError(ctx, "recent topics: query oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	seen := map[string]evRow{}
	order := make([]string, 0, limit*2)
	for rows.Next() {
		var (
			objectKey  string
			occurredAt time.Time
			payloadB   []byte
		)
		if err := rows.Scan(&objectKey, &occurredAt, &payloadB); err != nil {
			logError(ctx, "recent topics: scan oss_event failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		topicID := parseTopicIDFromObjectKey(objectKey)
		if topicID == "" {
			continue
		}
		if _, ok := seen[topicID]; ok {
			continue
		}
		seen[topicID] = evRow{objectKey: objectKey, occurredAt: occurredAt, payloadB: payloadB}
		order = append(order, topicID)
		if len(order) >= limitPlus(limit, 20) {
			break
		}
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "recent topics: iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	out := make([]recentTopicForEvaluationDTO, 0, limit)
	for _, topicID := range order {
		if len(out) >= limit {
			break
		}

		manifestRaw, err := store.GetObject(ctx, "topics/"+topicID+"/manifest.json")
		if err != nil {
			if !isOSSNotFound(err) {
				logError(ctx, "recent topics: get manifest failed", err)
			}
			continue
		}

		var mf struct {
			Visibility        string         `json:"visibility"`
			CircleID          string         `json:"circle_id,omitempty"`
			AllowlistAgentIDs []string       `json:"allowlist_agent_ids,omitempty"`
			OwnerAgentID      string         `json:"owner_agent_id,omitempty"`
			Title             string         `json:"title"`
			Summary           string         `json:"summary,omitempty"`
			Mode              string         `json:"mode,omitempty"`
			Rules             map[string]any `json:"rules,omitempty"`
		}
		if err := json.Unmarshal(manifestRaw, &mf); err != nil {
			logError(ctx, "recent topics: unmarshal manifest failed", err)
			continue
		}
		if !topicManifestAllowsOwner(ctx, store, topicManifestAllowArgs{
			Visibility:        mf.Visibility,
			CircleID:          mf.CircleID,
			AllowlistAgentIDs: mf.AllowlistAgentIDs,
			OwnerAgentID:      mf.OwnerAgentID,
			OwnedAgentRefs:    ownedAgentRefs,
			CandidateAgentRef: candidateAgentRef,
		}) {
			continue
		}

		opening := ""
		category := ""
		if mf.Rules != nil {
			if v, ok := mf.Rules["opening_question"].(string); ok {
				opening = strings.TrimSpace(v)
			}
			if v, ok := mf.Rules["seed_category"].(string); ok {
				category = strings.TrimSpace(v)
			}
		}

		row := seen[topicID]
		out = append(out, recentTopicForEvaluationDTO{
			TopicID:            topicID,
			Title:              strings.TrimSpace(mf.Title),
			Summary:            strings.TrimSpace(mf.Summary),
			Mode:               strings.TrimSpace(mf.Mode),
			OpeningQuestion:    opening,
			Category:           category,
			LastMessagePreview: extractEventPreview(row.payloadB),
			LastMessageAt:      row.occurredAt.UTC().Format(time.RFC3339),
		})
	}

	// If OSS event ingest isn't configured (or no recent message events exist yet),
	// fall back to listing topic manifests so the UI still has selectable topics.
	if len(out) < limit {
		keys, err := store.ListObjects(ctx, "topics/", 3000)
		if err != nil {
			logError(ctx, "recent topics: list objects failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}

		basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
		stripBasePrefix := func(key string) string {
			k := strings.TrimLeft(strings.TrimSpace(key), "/")
			if basePrefix == "" {
				return k
			}
			p := basePrefix + "/"
			if strings.HasPrefix(k, p) {
				return strings.TrimPrefix(k, p)
			}
			return k
		}

		type manifestCandidate struct {
			topicID   string
			createdAt time.Time
			dto       recentTopicForEvaluationDTO
		}

		seenTopicID := map[string]bool{}
		for _, t := range order {
			seenTopicID[t] = true
		}
		for _, it := range out {
			seenTopicID[strings.TrimSpace(it.TopicID)] = true
		}

		cands := make([]manifestCandidate, 0, 64)
		for _, key := range keys {
			k := stripBasePrefix(key)
			if !strings.HasPrefix(k, "topics/") || !strings.HasSuffix(k, "/manifest.json") {
				continue
			}

			parts := strings.Split(k, "/")
			if len(parts) < 3 {
				continue
			}
			topicID := strings.TrimSpace(parts[1])
			if topicID == "" || seenTopicID[topicID] {
				continue
			}

			manifestRaw, err := store.GetObject(ctx, "topics/"+topicID+"/manifest.json")
			if err != nil {
				if !isOSSNotFound(err) {
					logError(ctx, "recent topics: get manifest failed", err)
				}
				continue
			}

			var mf struct {
				Visibility        string         `json:"visibility"`
				CircleID          string         `json:"circle_id,omitempty"`
				AllowlistAgentIDs []string       `json:"allowlist_agent_ids,omitempty"`
				OwnerAgentID      string         `json:"owner_agent_id,omitempty"`
				Title             string         `json:"title"`
				Summary           string         `json:"summary,omitempty"`
				Mode              string         `json:"mode,omitempty"`
				Rules             map[string]any `json:"rules,omitempty"`
				CreatedAt         string         `json:"created_at,omitempty"`
			}
			if err := json.Unmarshal(manifestRaw, &mf); err != nil {
				logError(ctx, "recent topics: unmarshal manifest failed", err)
				continue
			}
			if !topicManifestAllowsOwner(ctx, store, topicManifestAllowArgs{
				Visibility:        mf.Visibility,
				CircleID:          mf.CircleID,
				AllowlistAgentIDs: mf.AllowlistAgentIDs,
				OwnerAgentID:      mf.OwnerAgentID,
				OwnedAgentRefs:    ownedAgentRefs,
				CandidateAgentRef: candidateAgentRef,
			}) {
				continue
			}

			opening := ""
			category := ""
			if mf.Rules != nil {
				if v, ok := mf.Rules["opening_question"].(string); ok {
					opening = strings.TrimSpace(v)
				}
				if v, ok := mf.Rules["seed_category"].(string); ok {
					category = strings.TrimSpace(v)
				}
			}

			var createdAt time.Time
			if strings.TrimSpace(mf.CreatedAt) != "" {
				if t, err := time.Parse(time.RFC3339, strings.TrimSpace(mf.CreatedAt)); err == nil {
					createdAt = t
				}
			}

			cands = append(cands, manifestCandidate{
				topicID:   topicID,
				createdAt: createdAt,
				dto: recentTopicForEvaluationDTO{
					TopicID:         topicID,
					Title:           strings.TrimSpace(mf.Title),
					Summary:         strings.TrimSpace(mf.Summary),
					Mode:            strings.TrimSpace(mf.Mode),
					OpeningQuestion: opening,
					Category:        category,
				},
			})
		}

		sort.Slice(cands, func(i, j int) bool {
			return cands[i].createdAt.After(cands[j].createdAt)
		})
		for _, c := range cands {
			if len(out) >= limit {
				break
			}
			out = append(out, c.dto)
		}
	}

	writeJSON(w, http.StatusOK, listRecentTopicsForEvaluationResponse{Items: out})
}

func limitPlus(a, b int) int {
	if a <= 0 {
		return b
	}
	return a + b
}

type topicManifestAllowArgs struct {
	Visibility        string
	CircleID          string
	AllowlistAgentIDs []string
	OwnerAgentID      string
	OwnedAgentRefs    []string
	CandidateAgentRef string
}

func topicManifestAllowsOwner(ctx context.Context, store agenthome.OSSObjectStore, args topicManifestAllowArgs) bool {
	vis := strings.TrimSpace(args.Visibility)
	if vis == "" {
		vis = "public"
	}
	switch vis {
	case "public":
		return true
	case "owner-only", "owner_only":
		ownerAgent := strings.TrimSpace(args.OwnerAgentID)
		if ownerAgent == "" {
			return false
		}
		for _, ref := range args.OwnedAgentRefs {
			if strings.TrimSpace(ref) == ownerAgent {
				return true
			}
		}
		return false
	case "invite":
		allow := map[string]bool{}
		for _, v := range args.AllowlistAgentIDs {
			v = strings.TrimSpace(v)
			if v != "" {
				allow[v] = true
			}
		}
		if strings.TrimSpace(args.CandidateAgentRef) != "" && allow[strings.TrimSpace(args.CandidateAgentRef)] {
			return true
		}
		for _, ref := range args.OwnedAgentRefs {
			if allow[strings.TrimSpace(ref)] {
				return true
			}
		}
		return false
	case "circle":
		cid := strings.TrimSpace(args.CircleID)
		if cid == "" {
			return false
		}
		for _, ref := range args.OwnedAgentRefs {
			ref = strings.TrimSpace(ref)
			if ref == "" {
				continue
			}
			ok, err := store.Exists(ctx, "circles/"+cid+"/members/"+ref+".json")
			if err != nil {
				logError(ctx, "recent topics: check circle member failed", err)
				continue
			}
			if ok {
				return true
			}
		}
		return false
	default:
		return false
	}
}
