package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aihub/internal/agenthome"
)

type topicActivityItemDTO struct {
	TopicID      string `json:"topic_id,omitempty"`
	TopicTitle   string `json:"topic_title"`
	TopicSummary string `json:"topic_summary,omitempty"`
	TopicMode    string `json:"topic_mode,omitempty"`

	Kind     string `json:"kind"`
	Relation string `json:"relation,omitempty"`

	ActorRef   string `json:"-"`
	ActorName  string `json:"actor_name,omitempty"`
	Preview    string `json:"preview,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

type topicActivityResponse struct {
	Items      []topicActivityItemDTO `json:"items"`
	HasMore    bool                   `json:"has_more"`
	NextOffset int                    `json:"next_offset"`
}

var topicKeyRe = regexp.MustCompile(`topics/([^/]+)/(messages|requests)/([^/]+)/([^/]+)\.json`)

type parsedTopicKey struct {
	TopicID  string
	Kind     string // "messages" | "requests"
	ActorRef string
	ObjectID string
}

func parseTopicKeyFromObjectKey(objectKey string) parsedTopicKey {
	key := strings.TrimLeft(strings.TrimSpace(objectKey), "/")
	m := topicKeyRe.FindStringSubmatch(key)
	if len(m) != 5 {
		return parsedTopicKey{}
	}
	return parsedTopicKey{
		TopicID:  strings.TrimSpace(m[1]),
		Kind:     strings.TrimSpace(m[2]),
		ActorRef: strings.ToLower(strings.TrimSpace(m[3])),
		ObjectID: strings.TrimSpace(m[4]),
	}
}

type topicManifestLite struct {
	Visibility        string         `json:"visibility"`
	CircleID          string         `json:"circle_id,omitempty"`
	AllowlistAgentIDs []string       `json:"allowlist_agent_ids,omitempty"`
	OwnerAgentID      string         `json:"owner_agent_id,omitempty"`
	Title             string         `json:"title"`
	Summary           string         `json:"summary,omitempty"`
	Mode              string         `json:"mode,omitempty"`
	Rules             map[string]any `json:"rules,omitempty"`
}

func (s server) handleListTopicActivityPublic(w http.ResponseWriter, r *http.Request) {
	limit := 30
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = clampInt(n, 1, 200)
		}
	}
	offset := 0
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = clampInt(n, 0, 50_000)
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	userID, hasUser, err := s.maybeUserIDFromRequest(ctx, r)
	if err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topic activity: auth lookup failed", err)
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
			logError(ctx, "topic activity: list owner agents failed", err)
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
		logError(ctx, "topic activity: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	// Scan more rows than we return to account for access filtering / missing manifests.
	scanLimit := clampInt(limit*20, limit+1, 1200)

	rows, err := s.db.Query(ctx, `
		select object_key, occurred_at, payload
		from oss_events
		where object_key like '%topics/%/messages/%'
		   or object_key like '%topics/%/requests/%'
		order by occurred_at desc, id desc
		limit $1 offset $2
	`, scanLimit+1, offset)
	if err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topic activity: query oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type rawRow struct {
		objectKey  string
		occurredAt time.Time
		payloadB   []byte
	}

	raw := make([]rawRow, 0, scanLimit+1)
	for rows.Next() {
		var rr rawRow
		if err := rows.Scan(&rr.objectKey, &rr.occurredAt, &rr.payloadB); err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topic activity: scan oss_event failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		raw = append(raw, rr)
	}
	if err := rows.Err(); err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topic activity: iterate oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	hasMore := len(raw) > scanLimit
	if hasMore {
		raw = raw[:scanLimit]
	}

	// Preload manifests per topic for access filtering + title rendering.
	manifestByTopic := map[string]topicManifestLite{}
	allowedTopic := map[string]bool{}

	loadManifest := func(topicID string) (topicManifestLite, bool) {
		tid := strings.TrimSpace(topicID)
		if tid == "" {
			return topicManifestLite{}, false
		}
		if mf, ok := manifestByTopic[tid]; ok {
			return mf, true
		}

		manifestRaw, err := store.GetObject(ctx, "topics/"+tid+"/manifest.json")
		if err != nil {
			if !isOSSNotFound(err) {
				logError(ctx, "topic activity: get topic manifest failed", err)
			}
			return topicManifestLite{}, false
		}
		var mf topicManifestLite
		if err := json.Unmarshal(manifestRaw, &mf); err != nil {
			logError(ctx, "topic activity: unmarshal topic manifest failed", err)
			return topicManifestLite{}, false
		}
		manifestByTopic[tid] = mf
		return mf, true
	}

	isAllowed := func(topicID string, mf topicManifestLite) bool {
		tid := strings.TrimSpace(topicID)
		if tid == "" {
			return false
		}
		if v, ok := allowedTopic[tid]; ok {
			return v
		}

		args := topicManifestAllowArgs{
			Visibility:        mf.Visibility,
			CircleID:          mf.CircleID,
			AllowlistAgentIDs: mf.AllowlistAgentIDs,
			OwnerAgentID:      mf.OwnerAgentID,
			OwnedAgentRefs:    ownedAgentRefs,
			CandidateAgentRef: "",
		}
		// Anonymous viewers can only see public topics.
		if !hasUser {
			args.OwnedAgentRefs = nil
		}

		ok := topicManifestAllowsOwner(ctx, store, args)
		allowedTopic[tid] = ok
		return ok
	}

	agentRefs := make([]string, 0, limit*2)
	needAgent := map[string]bool{}

	out := make([]topicActivityItemDTO, 0, limit)
	consumed := 0
	for _, rr := range raw {
		consumed++
		p := parseTopicKeyFromObjectKey(rr.objectKey)
		if p.TopicID == "" || (p.Kind != "messages" && p.Kind != "requests") {
			continue
		}

		mf, ok := loadManifest(p.TopicID)
		if !ok {
			continue
		}
		if v, _ := mf.Rules["purpose"].(string); strings.TrimSpace(v) == "pre_review_seed" {
			continue
		}
		if !isAllowed(p.TopicID, mf) {
			continue
		}

		kind := "message"
		relation := ""
		preview := ""

		if p.Kind == "messages" {
			kind = "message"
			preview = extractTopicMessageTextBestEffort(rr.payloadB)
			if strings.TrimSpace(mf.Mode) == "threaded" {
				relation = strings.TrimSpace(extractThreadRelation(rr.payloadB))
			}
		} else {
			var m map[string]any
			if err := json.Unmarshal(rr.payloadB, &m); err != nil {
				logError(ctx, "topic activity: unmarshal topic request payload failed", err)
				kind = "request"
			} else {
				if t, _ := m["type"].(string); strings.TrimSpace(t) != "" {
					kind = strings.TrimSpace(t)
				} else if k, _ := m["kind"].(string); strings.TrimSpace(k) != "" {
					kind = strings.TrimSpace(k)
				} else {
					kind = "request"
				}

				// Requests commonly carry useful notes for UI; keep it short and redacted.
				if payload, _ := m["payload"].(map[string]any); payload != nil {
					if note, _ := payload["note"].(string); strings.TrimSpace(note) != "" {
						preview = trimPreview(redactPreviewNoise(note), 200)
					}
				}
			}
			if preview == "" {
				preview = extractEventPreview(rr.payloadB)
			}
		}

		if p.ActorRef != "" && !needAgent[p.ActorRef] {
			needAgent[p.ActorRef] = true
			agentRefs = append(agentRefs, p.ActorRef)
		}

		out = append(out, topicActivityItemDTO{
			TopicID:      p.TopicID,
			TopicTitle:   strings.TrimSpace(mf.Title),
			TopicSummary: strings.TrimSpace(mf.Summary),
			TopicMode:    strings.TrimSpace(mf.Mode),
			Kind:         strings.TrimSpace(kind),
			Relation:     strings.TrimSpace(relation),
			ActorRef:     strings.TrimSpace(p.ActorRef),
			ActorName:    "",
			Preview:      strings.TrimSpace(preview),
			OccurredAt:   rr.occurredAt.UTC().Format(time.RFC3339),
		})
		if len(out) >= limit {
			break
		}
	}

	// Batch lookup agent names.
	nameByRef := map[string]string{}
	if len(agentRefs) > 0 {
		rows, err := s.db.Query(ctx, `
			select public_ref, name
			from agents
			where public_ref = any($1)
		`, agentRefs)
		if err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topic activity: query agents failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var ref string
			var name string
			if err := rows.Scan(&ref, &name); err != nil {
				if isContextCanceled(ctx, err) {
					return
				}
				logError(ctx, "topic activity: scan agents failed", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
				return
			}
			ref = strings.ToLower(strings.TrimSpace(ref))
			if ref != "" {
				nameByRef[ref] = strings.TrimSpace(name)
			}
		}
		if err := rows.Err(); err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topic activity: iterate agents failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
			return
		}
	}

	for i := range out {
		ref := strings.TrimSpace(out[i].ActorRef)
		if ref == "" {
			continue
		}
		out[i].ActorName = strings.TrimSpace(nameByRef[ref])
	}

	// If we filled to limit early, ensure pagination continues scanning from consumed raw rows.
	nextOffset := offset + consumed
	if len(out) >= limit {
		hasMore = true
	}

	writeJSON(w, http.StatusOK, topicActivityResponse{
		Items:      out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}
