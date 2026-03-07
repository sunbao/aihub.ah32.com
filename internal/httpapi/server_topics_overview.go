package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"aihub/internal/agenthome"
)

type topicOverviewHighlightDTO struct {
	ActorName  string `json:"actor_name,omitempty"`
	Preview    string `json:"preview,omitempty"`
	Relation   string `json:"relation,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

type topicOverviewItemDTO struct {
	TopicID string `json:"topic_id"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
	Mode    string `json:"mode,omitempty"`

	LastKind       string `json:"last_kind,omitempty"`
	LastRelation   string `json:"last_relation,omitempty"`
	LastPreview    string `json:"last_preview,omitempty"`
	LastActorName  string `json:"last_actor_name,omitempty"`
	LastOccurredAt string `json:"last_occurred_at"`

	Highlights []topicOverviewHighlightDTO `json:"highlights,omitempty"`

	// Internal only (never returned).
	lastActorRef string
	hiActorRefs  []string
}

type topicsOverviewResponse struct {
	Items      []topicOverviewItemDTO `json:"items"`
	HasMore    bool                   `json:"has_more"`
	NextOffset int                    `json:"next_offset"`
}

func (s server) handleListTopicsOverviewPublic(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = clampInt(n, 1, 60)
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
		logError(ctx, "topics overview: auth lookup failed", err)
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
			logError(ctx, "topics overview: list owner agents failed", err)
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
		logError(ctx, "topics overview: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

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
		raw, err := store.GetObject(ctx, "topics/"+tid+"/manifest.json")
		if err != nil {
			if !isOSSNotFound(err) {
				logError(ctx, "topics overview: get manifest failed", err)
			}
			return topicManifestLite{}, false
		}
		var mf topicManifestLite
		if err := json.Unmarshal(raw, &mf); err != nil {
			logError(ctx, "topics overview: decode manifest failed", err)
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

	// Scan more rows than we return to account for access filtering + manifests missing.
	scanLimit := clampInt(limit*250, limit+1, 5000)
	rows, err := s.db.Query(ctx, `
		select object_key, occurred_at, payload
		from oss_events
		where object_key like '%topics/%/messages/%'
		order by occurred_at desc, id desc
		limit $1 offset $2
	`, scanLimit+1, offset)
	if err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topics overview: query oss_events failed", err)
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
			logError(ctx, "topics overview: scan oss_event failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		raw = append(raw, rr)
	}
	if err := rows.Err(); err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topics overview: iterate oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	seenTopic := map[string]int{} // topic_id -> index in out
	out := make([]topicOverviewItemDTO, 0, limit)

	needRefs := map[string]bool{}
	refs := make([]string, 0, limit*6)

	consumed := 0
	for _, rr := range raw {
		consumed++
		p := parseTopicKeyFromObjectKey(rr.objectKey)
		if p.TopicID == "" || p.Kind != "messages" {
			continue
		}
		tid := strings.TrimSpace(p.TopicID)
		mf, ok := loadManifest(tid)
		if !ok {
			continue
		}
		if v, _ := mf.Rules["purpose"].(string); strings.TrimSpace(v) == "pre_review_seed" {
			continue
		}
		if !isAllowed(tid, mf) {
			continue
		}

		preview := extractEventPreview(rr.payloadB)
		relation := ""
		if strings.TrimSpace(mf.Mode) == "threaded" {
			relation = strings.TrimSpace(extractThreadRelation(rr.payloadB))
		}

		idx, exists := seenTopic[tid]
		if !exists {
			item := topicOverviewItemDTO{
				TopicID:        tid,
				Title:          strings.TrimSpace(mf.Title),
				Summary:        strings.TrimSpace(mf.Summary),
				Mode:           strings.TrimSpace(mf.Mode),
				LastKind:       "message",
				LastRelation:   relation,
				LastPreview:    strings.TrimSpace(preview),
				LastOccurredAt: rr.occurredAt.UTC().Format(time.RFC3339),
				lastActorRef:   strings.TrimSpace(p.ActorRef),
				Highlights:     []topicOverviewHighlightDTO{},
				hiActorRefs:    []string{},
				LastActorName:  "",
			}

			if strings.TrimSpace(p.ActorRef) != "" {
				ref := strings.TrimSpace(p.ActorRef)
				if !needRefs[ref] {
					needRefs[ref] = true
					refs = append(refs, ref)
				}
			}

			// Seed highlights with the most recent message.
			item.Highlights = append(item.Highlights, topicOverviewHighlightDTO{
				Preview:    strings.TrimSpace(preview),
				Relation:   relation,
				OccurredAt: rr.occurredAt.UTC().Format(time.RFC3339),
			})
			item.hiActorRefs = append(item.hiActorRefs, strings.TrimSpace(p.ActorRef))

			seenTopic[tid] = len(out)
			out = append(out, item)
			if len(out) >= limit && consumed >= limit*12 {
				break
			}
			continue
		}

		// Fill highlights: keep a few latest messages to show "essence" on the first screen.
		if idx >= 0 && idx < len(out) && len(out[idx].Highlights) < 3 {
			out[idx].Highlights = append(out[idx].Highlights, topicOverviewHighlightDTO{
				Preview:    strings.TrimSpace(preview),
				Relation:   relation,
				OccurredAt: rr.occurredAt.UTC().Format(time.RFC3339),
			})
			out[idx].hiActorRefs = append(out[idx].hiActorRefs, strings.TrimSpace(p.ActorRef))
			ref := strings.TrimSpace(p.ActorRef)
			if ref != "" && !needRefs[ref] {
				needRefs[ref] = true
				refs = append(refs, ref)
			}
		}
	}

	nameByRef := map[string]string{}
	if len(refs) > 0 {
		m, err := s.lookupAgentNamesByRefs(ctx, refs)
		if err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topics overview: lookup agent names failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		nameByRef = m
	}

	for i := range out {
		ref := strings.TrimSpace(out[i].lastActorRef)
		if ref != "" {
			out[i].LastActorName = strings.TrimSpace(nameByRef[ref])
		}
		// Fill highlight actor names (best-effort).
		if len(out[i].Highlights) > 0 && len(out[i].hiActorRefs) == len(out[i].Highlights) {
			for j := range out[i].Highlights {
				rref := strings.TrimSpace(out[i].hiActorRefs[j])
				if rref != "" {
					out[i].Highlights[j].ActorName = strings.TrimSpace(nameByRef[rref])
				}
			}
		}
		// Clear internal refs.
		out[i].lastActorRef = ""
		out[i].hiActorRefs = nil
	}

	nextOffset := offset + consumed
	hasMore := false
	if len(out) >= limit {
		hasMore = true
	}

	writeJSON(w, http.StatusOK, topicsOverviewResponse{
		Items:      out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}
