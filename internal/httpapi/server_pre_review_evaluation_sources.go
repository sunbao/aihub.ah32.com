package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/google/uuid"
)

type recentTopicForEvaluationDTO struct {
	TopicID            string `json:"topic_id"`
	Title              string `json:"title"`
	Summary            string `json:"summary,omitempty"`
	Mode               string `json:"mode,omitempty"`
	OpeningQuestion    string `json:"opening_question,omitempty"`
	LastMessagePreview string `json:"last_message_preview,omitempty"`
	LastMessageAt      string `json:"last_message_at,omitempty"`
}

type listRecentTopicsForEvaluationResponse struct {
	Items []recentTopicForEvaluationDTO `json:"items"`
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

func (s server) handleOwnerListRecentTopicsForEvaluation(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	limit := clampInt(int64Query(r, "limit", 10), 1, 50)
	candidateAgentID := strings.TrimSpace(r.URL.Query().Get("candidate_agent_id"))

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if strings.TrimSpace(s.ossProvider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}
	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "recent topics: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	ownedAgentIDs, err := s.listOwnerAgentIDs(ctx, userID, 50)
	if err != nil {
		logError(ctx, "recent topics: list owner agents failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	var candidateAgentUUID uuid.UUID
	if candidateAgentID != "" {
		id, err := uuid.Parse(candidateAgentID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid candidate_agent_id"})
			return
		}
		candidateAgentUUID = id
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
			continue
		}
		if !topicManifestAllowsOwner(ctx, store, topicManifestAllowArgs{
			Visibility:        mf.Visibility,
			CircleID:          mf.CircleID,
			AllowlistAgentIDs: mf.AllowlistAgentIDs,
			OwnerAgentID:      mf.OwnerAgentID,
			OwnedAgentIDs:     ownedAgentIDs,
			CandidateAgentID:  candidateAgentUUID,
		}) {
			continue
		}

		opening := ""
		if mf.Rules != nil {
			if v, ok := mf.Rules["opening_question"].(string); ok {
				opening = strings.TrimSpace(v)
			}
		}

		row := seen[topicID]
		out = append(out, recentTopicForEvaluationDTO{
			TopicID:            topicID,
			Title:              strings.TrimSpace(mf.Title),
			Summary:            strings.TrimSpace(mf.Summary),
			Mode:               strings.TrimSpace(mf.Mode),
			OpeningQuestion:    opening,
			LastMessagePreview: extractEventPreview(row.payloadB),
			LastMessageAt:      row.occurredAt.UTC().Format(time.RFC3339),
		})
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
	OwnedAgentIDs     []uuid.UUID
	CandidateAgentID  uuid.UUID
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
		for _, id := range args.OwnedAgentIDs {
			if id.String() == ownerAgent {
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
		if args.CandidateAgentID != uuid.Nil && allow[args.CandidateAgentID.String()] {
			return true
		}
		for _, id := range args.OwnedAgentIDs {
			if allow[id.String()] {
				return true
			}
		}
		return false
	case "circle":
		cid := strings.TrimSpace(args.CircleID)
		if cid == "" {
			return false
		}
		for _, id := range args.OwnedAgentIDs {
			ok, err := store.Exists(ctx, "circles/"+cid+"/members/"+id.String()+".json")
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
