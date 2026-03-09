package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/go-chi/chi/v5"
)

type adminCleanupTopicMessagesRequest struct {
	DryRun     bool `json:"dry_run"`
	SinceHours int  `json:"since_hours"`
	MaxScan    int  `json:"max_scan"`
	MaxDelete  int  `json:"max_delete"`
}

type adminCleanupTopicMessagesItem struct {
	ObjectKey   string `json:"object_key"`
	OccurredAt  string `json:"occurred_at"`
	Reason      string `json:"reason"`
	TextPreview string `json:"text_preview,omitempty"`
}

type adminCleanupTopicMessagesResponse struct {
	TopicID  string                          `json:"topic_id"`
	DryRun   bool                            `json:"dry_run"`
	Scanned  int                             `json:"scanned"`
	Matched  int                             `json:"matched"`
	Deleted  int                             `json:"deleted"`
	Items    []adminCleanupTopicMessagesItem `json:"items"`
	Warnings []string                        `json:"warnings,omitempty"`
}

func (s server) handleAdminCleanupTopicMessages(w http.ResponseWriter, r *http.Request) {
	topicID := strings.TrimSpace(chi.URLParam(r, "topicID"))
	if topicID == "" || len(topicID) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid topic_id"})
		return
	}

	var req adminCleanupTopicMessagesRequest
	if !readJSONLimited(w, r, &req, 32*1024) {
		return
	}
	if req.SinceHours <= 0 {
		req.SinceHours = 72
	}
	req.SinceHours = clampInt(req.SinceHours, 1, 24*90)
	if req.MaxScan <= 0 {
		req.MaxScan = 2000
	}
	req.MaxScan = clampInt(req.MaxScan, 20, 50_000)
	if req.MaxDelete <= 0 {
		req.MaxDelete = 200
	}
	req.MaxDelete = clampInt(req.MaxDelete, 1, 5000)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
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
		logError(ctx, "cleanup topic messages: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	pat1 := "topics/" + topicID + "/messages/%"
	pat2 := pat1
	if basePrefix != "" {
		pat2 = basePrefix + "/" + pat1
	}

	cutoff := time.Now().UTC().Add(-time.Duration(req.SinceHours) * time.Hour)

	rows, err := s.db.Query(ctx, `
		select object_key, occurred_at, payload
		from oss_events
		where (object_key like $1 or object_key like $2)
		  and occurred_at >= $3
		order by occurred_at desc, id desc
		limit $4
	`, pat1, pat2, cutoff, req.MaxScan)
	if err != nil {
		logError(ctx, "cleanup topic messages: query oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type cand struct {
		objectKey  string
		occurredAt time.Time
		payloadB   []byte
		reason     string
		preview    string
	}

	scanned := 0
	matched := 0
	cands := make([]cand, 0, 128)

	for rows.Next() {
		scanned++
		var objectKey string
		var occurredAt time.Time
		var payloadB []byte
		if err := rows.Scan(&objectKey, &occurredAt, &payloadB); err != nil {
			logError(ctx, "cleanup topic messages: scan oss_event failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		p := parseTopicKeyFromObjectKey(objectKey)
		if strings.TrimSpace(p.TopicID) != topicID || strings.TrimSpace(p.Kind) != "messages" {
			continue
		}
		reason, preview := badTopicMessageReason(payloadB)
		if reason == "" {
			continue
		}
		matched++
		cands = append(cands, cand{
			objectKey:  strings.TrimLeft(strings.TrimSpace(objectKey), "/"),
			occurredAt: occurredAt.UTC(),
			payloadB:   payloadB,
			reason:     reason,
			preview:    preview,
		})
		if len(cands) >= req.MaxDelete && !req.DryRun {
			// Keep the request bounded on apply to avoid long-running admin calls.
			break
		}
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "cleanup topic messages: iterate oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	resp := adminCleanupTopicMessagesResponse{
		TopicID: topicID,
		DryRun:  req.DryRun,
		Scanned: scanned,
		Matched: matched,
		Deleted: 0,
		Items:   make([]adminCleanupTopicMessagesItem, 0, len(cands)),
	}

	for _, c := range cands {
		resp.Items = append(resp.Items, adminCleanupTopicMessagesItem{
			ObjectKey:   c.objectKey,
			OccurredAt:  c.occurredAt.Format(time.RFC3339),
			Reason:      c.reason,
			TextPreview: c.preview,
		})
	}

	if req.DryRun {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	deleted := 0
	for _, c := range cands {
		if deleted >= req.MaxDelete {
			break
		}

		fullKey := strings.TrimLeft(strings.TrimSpace(c.objectKey), "/")
		stripped := strings.TrimLeft(stripBasePrefix(fullKey, basePrefix), "/")
		prefixed := agenthome.JoinKey(basePrefix, stripped)

		if _, err := s.db.Exec(ctx, `
			delete from oss_events
			where object_key = $1 or object_key = $2 or object_key = $3
		`, fullKey, stripped, prefixed); err != nil {
			logError(ctx, "cleanup topic messages: delete oss_events failed", err)
			resp.Warnings = append(resp.Warnings, "db delete failed for "+fullKey)
			continue
		}

		if _, err := store.DeletePrefix(ctx, stripped); err != nil {
			logError(ctx, "cleanup topic messages: delete oss object failed", err)
			resp.Warnings = append(resp.Warnings, "oss delete failed for "+stripped)
			// Keep going; DB is already cleaned.
		}

		deleted++
		s.audit(ctx, "user", userID, "cleanup_topic_message", map[string]any{
			"topic_id":    topicID,
			"object_key":  fullKey,
			"reason":      c.reason,
			"occurred_at": c.occurredAt.Format(time.RFC3339),
		})
	}

	resp.Deleted = deleted
	writeJSON(w, http.StatusOK, resp)
}

func badTopicMessageReason(payloadB []byte) (reason string, preview string) {
	preview = strings.TrimSpace(extractTopicMessageTextBestEffort(payloadB))
	preview = truncateRunes(preview, 200)
	t := strings.TrimSpace(preview)
	if t == "" {
		return "empty_text", ""
	}
	if t == "{}" || t == "[]" || t == "null" {
		return "empty_object", t
	}
	if strings.Contains(t, "\uFFFD") {
		return "replacement_char", t
	}
	if hasRuneRun(t, '?', 4) {
		return "question_marks", t
	}
	if looksLikeJSONWrappedText(t) {
		return "json_wrapped", t
	}
	return "", t
}

func hasRuneRun(s string, r rune, minRun int) bool {
	if minRun <= 1 {
		minRun = 1
	}
	run := 0
	for _, ch := range s {
		if ch == r {
			run++
			if run >= minRun {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func looksLikeJSONWrappedText(t string) bool {
	v := strings.TrimSpace(t)
	if len(v) < 8 {
		return false
	}
	if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") && strings.Contains(v, "\"text\"") {
		return true
	}
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") && strings.Contains(v, "\"text\"") {
		return true
	}
	return false
}
