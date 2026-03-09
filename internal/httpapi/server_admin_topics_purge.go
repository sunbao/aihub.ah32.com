package httpapi

import (
	"context"
	"net/http"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/google/uuid"
)

type adminPurgeTopicsRequest struct {
	// Default is dry-run for safety. Set dry_run=false + confirm to actually delete.
	DryRun  *bool  `json:"dry_run,omitempty"`
	Confirm string `json:"confirm,omitempty"`

	// KeepTopicIDs are never deleted. If empty, topic_daily_checkin is always kept.
	KeepTopicIDs []string `json:"keep_topic_ids,omitempty"`

	// MaxScan limits how many distinct topics we consider (default 5000).
	MaxScan int `json:"max_scan,omitempty"`

	// MaxDelete limits how many topics can be deleted in one call (default 200).
	MaxDelete int `json:"max_delete,omitempty"`
}

type adminPurgeTopicsItem struct {
	TopicID    string `json:"topic_id"`
	OSDeleted  int    `json:"oss_deleted,omitempty"`
	DBDeleted  int    `json:"db_deleted,omitempty"`
	Skipped    bool   `json:"skipped,omitempty"`
	SkipReason string `json:"skip_reason,omitempty"`
}

type adminPurgeTopicsResponse struct {
	OK      bool   `json:"ok"`
	DryRun  bool   `json:"dry_run"`
	Confirm string `json:"confirm,omitempty"`

	ScannedDistinctTopics int `json:"scanned_distinct_topics"`
	MatchedTopics         int `json:"matched_topics"`
	DeletedTopics         int `json:"deleted_topics"`

	KeepTopicIDs []string               `json:"keep_topic_ids,omitempty"`
	Items        []adminPurgeTopicsItem `json:"items,omitempty"`
}

func (s server) handleAdminPurgeTopics(w http.ResponseWriter, r *http.Request) {
	var req adminPurgeTopicsRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}

	keep := map[string]bool{}
	for _, k := range req.KeepTopicIDs {
		tid := strings.TrimSpace(k)
		if tid != "" {
			keep[tid] = true
		}
	}
	keep[builtinDailyCheckinTopicID] = true
	keepList := make([]string, 0, len(keep))
	for tid := range keep {
		keepList = append(keepList, tid)
	}

	maxScan := clampInt(req.MaxScan, 1, 200_000)
	if req.MaxScan == 0 {
		maxScan = 5000
	}
	maxDelete := clampInt(req.MaxDelete, 1, 20_000)
	if req.MaxDelete == 0 {
		maxDelete = 200
	}

	dryRun := true
	if req.DryRun != nil {
		dryRun = bool(*req.DryRun)
	}
	confirm := strings.TrimSpace(req.Confirm)
	if !dryRun {
		if confirm != "DELETE_ALL_TOPICS" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing confirm (set confirm=DELETE_ALL_TOPICS)"})
			return
		}
	}

	cfg := s.ossCfg()
	if strings.TrimSpace(cfg.Provider) == "" && strings.TrimSpace(cfg.LocalDir) != "" {
		cfg.Provider = "local"
	}
	if strings.TrimSpace(cfg.Provider) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()

	// List candidate topic IDs from oss_events. Topics are OSS-backed; oss_events is our index.
	rows, err := s.db.Query(ctx, `
		select distinct split_part(object_key, '/', 2) as topic_id
		from oss_events
		where object_key like 'topics/%/messages/%'
		   or object_key like 'topics/%/requests/%'
		limit $1
	`, maxScan)
	if err != nil {
		logError(ctx, "admin purge topics: query oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	candidates := make([]string, 0, 200)
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			logError(ctx, "admin purge topics: scan row failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		tid = strings.TrimSpace(tid)
		if tid == "" || len(tid) > 200 {
			continue
		}
		candidates = append(candidates, tid)
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "admin purge topics: iterate rows failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	store, err := agenthome.NewOSSObjectStore(cfg)
	if err != nil {
		logError(ctx, "admin purge topics: init oss store failed", err)
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
		return
	}

	out := adminPurgeTopicsResponse{
		OK:                    true,
		DryRun:                dryRun,
		Confirm:               confirm,
		ScannedDistinctTopics: len(candidates),
		KeepTopicIDs:          keepList,
		Items:                 []adminPurgeTopicsItem{},
	}

	deletedCount := 0
	for _, tid := range candidates {
		if keep[tid] {
			out.Items = append(out.Items, adminPurgeTopicsItem{TopicID: tid, Skipped: true, SkipReason: "keep"})
			continue
		}
		out.MatchedTopics++
		if deletedCount >= maxDelete {
			out.Items = append(out.Items, adminPurgeTopicsItem{TopicID: tid, Skipped: true, SkipReason: "max_delete_reached"})
			continue
		}

		if dryRun {
			out.Items = append(out.Items, adminPurgeTopicsItem{TopicID: tid})
			continue
		}

		prefix := "topics/" + tid + "/"
		osDeleted, err := store.DeletePrefix(ctx, prefix)
		if err != nil {
			logError(ctx, "admin purge topics: oss delete prefix failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss delete failed"})
			return
		}

		tag, err := s.db.Exec(ctx, `delete from oss_events where object_key like $1`, prefix+"%")
		if err != nil {
			logError(ctx, "admin purge topics: delete oss_events failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db delete failed"})
			return
		}
		dbDeleted := int(tag.RowsAffected())

		out.Items = append(out.Items, adminPurgeTopicsItem{TopicID: tid, OSDeleted: osDeleted, DBDeleted: dbDeleted})
		deletedCount++
		out.DeletedTopics++
	}

	s.audit(ctx, "admin", uuid.Nil, "oss_topics_purged", map[string]any{
		"dry_run":    dryRun,
		"matched":    out.MatchedTopics,
		"deleted":    out.DeletedTopics,
		"max_scan":   maxScan,
		"max_delete": maxDelete,
	})

	writeJSON(w, http.StatusOK, out)
}
