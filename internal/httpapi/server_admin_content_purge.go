package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/google/uuid"
)

const adminPurgeContentConfirm = "DELETE_ALL_CONTENT"

type adminPurgeContentRequest struct {
	// Default is dry-run for safety. Set dry_run=false + confirm to actually delete.
	DryRun  *bool  `json:"dry_run,omitempty"`
	Confirm string `json:"confirm,omitempty"`

	// Optional toggles; defaults to true when omitted.
	PurgeRuns   *bool `json:"purge_runs,omitempty"`
	PurgeAgents *bool `json:"purge_agents,omitempty"`
	PurgeTopics *bool `json:"purge_topics,omitempty"`

	// If true, re-create built-in seed data after purge (pre-review seed topics + daily checkin topic).
	// Default true.
	Reseed *bool `json:"reseed,omitempty"`
}

type adminPurgeContentCounts struct {
	Agents      int `json:"agents"`
	Runs        int `json:"runs"`
	TopicEvents int `json:"topic_events"`
}

type adminPurgeContentResult struct {
	RunsDeleted           int  `json:"runs_deleted,omitempty"`
	AgentsDeleted         int  `json:"agents_deleted,omitempty"`
	TopicOSObjectsDeleted int  `json:"topic_oss_objects_deleted,omitempty"`
	TopicEventsDeleted    int  `json:"topic_events_deleted,omitempty"`
	Reseeded              bool `json:"reseeded,omitempty"`
}

type adminPurgeContentResponse struct {
	OK      bool   `json:"ok"`
	DryRun  bool   `json:"dry_run"`
	Confirm string `json:"confirm,omitempty"`

	Counts adminPurgeContentCounts  `json:"counts"`
	Plan   []string                 `json:"plan,omitempty"`
	Result *adminPurgeContentResult `json:"result,omitempty"`
}

func (s server) handleAdminPurgeContent(w http.ResponseWriter, r *http.Request) {
	var req adminPurgeContentRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}

	dryRun := true
	if req.DryRun != nil {
		dryRun = bool(*req.DryRun)
	}
	confirm := strings.TrimSpace(req.Confirm)
	if !dryRun && confirm != adminPurgeContentConfirm {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing confirm (set confirm=" + adminPurgeContentConfirm + ")"})
		return
	}

	purgeRuns := true
	if req.PurgeRuns != nil {
		purgeRuns = bool(*req.PurgeRuns)
	}
	purgeAgents := true
	if req.PurgeAgents != nil {
		purgeAgents = bool(*req.PurgeAgents)
	}
	purgeTopics := true
	if req.PurgeTopics != nil {
		purgeTopics = bool(*req.PurgeTopics)
	}
	reseed := true
	if req.Reseed != nil {
		reseed = bool(*req.Reseed)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	var agentCount int
	if err := s.db.QueryRow(ctx, `select count(*) from agents`).Scan(&agentCount); err != nil {
		logError(ctx, "admin purge content: count agents failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	var runCount int
	if err := s.db.QueryRow(ctx, `select count(*) from runs`).Scan(&runCount); err != nil {
		logError(ctx, "admin purge content: count runs failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	var topicEventCount int
	if err := s.db.QueryRow(ctx, `select count(*) from oss_events where object_key like 'topics/%'`).Scan(&topicEventCount); err != nil {
		logError(ctx, "admin purge content: count topic oss_events failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	plan := []string{}
	if purgeTopics {
		plan = append(plan, "delete OSS objects under prefix topics/ (all topics)")
		plan = append(plan, "delete oss_events where object_key like topics/%")
	}
	if purgeRuns {
		plan = append(plan, "delete all runs (cascades to events/artifacts/work_items/offers)")
	}
	if purgeAgents {
		plan = append(plan, "delete all agents (cascades to agent keys/tags/evaluation judges, etc.)")
	}
	if reseed {
		plan = append(plan, "reseed pre-review seed topics + built-in daily checkin topic")
	}

	resp := adminPurgeContentResponse{
		OK:      true,
		DryRun:  dryRun,
		Confirm: confirm,
		Counts: adminPurgeContentCounts{
			Agents:      agentCount,
			Runs:        runCount,
			TopicEvents: topicEventCount,
		},
		Plan: plan,
	}

	if dryRun {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	var store agenthome.OSSObjectStore
	if purgeTopics || reseed {
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
		st, err := agenthome.NewOSSObjectStore(ossCfg)
		if err != nil {
			logError(ctx, "admin purge content: init oss store failed", err)
			writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "oss not configured"})
			return
		}
		store = st
	}

	out := adminPurgeContentResult{}

	if purgeTopics {
		deleted, err := store.DeletePrefix(ctx, "topics/")
		if err != nil {
			logError(ctx, "admin purge content: oss delete topics prefix failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "oss delete failed"})
			return
		}
		out.TopicOSObjectsDeleted = deleted

		tag, err := s.db.Exec(ctx, `delete from oss_events where object_key like 'topics/%'`)
		if err != nil {
			logError(ctx, "admin purge content: delete topic oss_events failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db delete failed"})
			return
		}
		out.TopicEventsDeleted = int(tag.RowsAffected())
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "admin purge content: db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	if purgeRuns {
		tag, err := tx.Exec(ctx, `delete from runs`)
		if err != nil {
			logError(ctx, "admin purge content: delete runs failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db delete failed"})
			return
		}
		out.RunsDeleted = int(tag.RowsAffected())
	}

	if purgeAgents {
		tag, err := tx.Exec(ctx, `delete from agents`)
		if err != nil {
			logError(ctx, "admin purge content: delete agents failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db delete failed"})
			return
		}
		out.AgentsDeleted = int(tag.RowsAffected())
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "admin purge content: db commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	if reseed {
		s.ensurePreReviewSeedData(ctx)
		s.ensureBuiltinDailyCheckinTopic(ctx)

		// Verify reseed produced core manifests so a failure isn't silently masked by best-effort seeders.
		if store == nil {
			logError(ctx, "admin purge content: reseed store missing", errors.New("oss store is nil"))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reseed failed"})
			return
		}

		ok, err := store.Exists(ctx, "topics/"+builtinDailyCheckinTopicID+"/manifest.json")
		if err != nil {
			logError(ctx, "admin purge content: reseed daily_checkin verify failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reseed failed"})
			return
		}
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reseed failed: daily_checkin missing"})
			return
		}

		seedOK := false
		for i, tid := range preReviewSeedTopicIDs {
			// Bound checks; we only need a signal that seed topics exist.
			if i >= 6 {
				break
			}
			tid = strings.TrimSpace(tid)
			if tid == "" {
				continue
			}
			ok, err := store.Exists(ctx, "topics/"+tid+"/manifest.json")
			if err != nil {
				logError(ctx, "admin purge content: reseed pre-review verify failed", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reseed failed"})
				return
			}
			if ok {
				seedOK = true
				break
			}
		}
		if !seedOK {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "reseed failed: pre-review seed topics missing"})
			return
		}

		out.Reseeded = true
	}

	actorID, ok := userIDFromCtx(r.Context())
	if !ok {
		actorID = uuid.Nil
	}
	s.audit(ctx, "admin", actorID, "content_purged", map[string]any{
		"purge_topics": purgeTopics,
		"purge_runs":   purgeRuns,
		"purge_agents": purgeAgents,
		"reseed":       reseed,
		"result": map[string]any{
			"runs_deleted":              out.RunsDeleted,
			"agents_deleted":            out.AgentsDeleted,
			"topic_oss_objects_deleted": out.TopicOSObjectsDeleted,
			"topic_events_deleted":      out.TopicEventsDeleted,
		},
	})

	resp.Result = &out
	writeJSON(w, http.StatusOK, resp)
}
