package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type gatewayCreateRunRequest struct {
	Goal         string   `json:"goal"`
	Constraints  string   `json:"constraints"`
	RequiredTags []string `json:"required_tags"`
	Visibility   string   `json:"visibility,omitempty"` // public|unlisted
}

type gatewayCreateRunResponse struct {
	RunRef string `json:"run_ref"`
}

func (s server) handleGatewayCreateRun(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if len(s.taskGenActorTags) == 0 || s.taskGenDailyLimitPerAgent <= 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "taskgen disabled"})
		return
	}

	var req gatewayCreateRunRequest
	if !readJSONLimited(w, r, &req, 128*1024) {
		return
	}
	req.Goal = strings.TrimSpace(req.Goal)
	req.Constraints = strings.TrimSpace(req.Constraints)
	req.RequiredTags = normalizeTags(req.RequiredTags)
	req.Visibility = strings.ToLower(strings.TrimSpace(req.Visibility))

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
	if len(req.RequiredTags) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing required_tags"})
		return
	}
	req.RequiredTags = filterTagsByPrefixes(req.RequiredTags, s.taskGenAllowedTagPrefixes)
	if len(req.RequiredTags) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "required_tags not allowed"})
		return
	}
	isPublic := true
	if req.Visibility == "unlisted" {
		isPublic = false
	} else if req.Visibility != "" && req.Visibility != "public" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid visibility"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "gateway create run: db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	// Eligibility: agent must have at least one configured taskgen actor tag.
	tags, err := listAgentTagsInTx(ctx, tx, agentID, 200)
	if err != nil {
		logError(ctx, "gateway create run: list agent tags failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tags query failed"})
		return
	}
	allowedSet := map[string]struct{}{}
	for _, t := range s.taskGenActorTags {
		allowedSet[strings.TrimSpace(t)] = struct{}{}
	}
	eligible := false
	for _, t := range tags {
		if _, ok := allowedSet[strings.TrimSpace(t)]; ok {
			eligible = true
			break
		}
	}
	if !eligible {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not eligible"})
		return
	}

	// Quota: created runs per agent per day (Asia/Shanghai boundary).
	dayStart := shanghaiDayStart(time.Now().UTC())
	var usedToday int
	if err := tx.QueryRow(ctx, `
		select count(1)
		from audit_logs
		where actor_type = 'agent'
		  and actor_id = $1
		  and action = 'gateway_run_created'
		  and created_at >= $2
	`, agentID, dayStart).Scan(&usedToday); err != nil {
		logError(ctx, "gateway create run: quota query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "quota query failed"})
		return
	}
	if usedToday >= s.taskGenDailyLimitPerAgent {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "quota exceeded"})
		return
	}

	// Publisher user = this agent's owner.
	var ownerID uuid.UUID
	var agentRef string
	if err := tx.QueryRow(ctx, `select owner_id, public_ref from agents where id = $1`, agentID).Scan(&ownerID, &agentRef); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "unknown agent"})
			return
		}
		logError(ctx, "gateway create run: owner lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "owner lookup failed"})
		return
	}

	// Always include a stable marker tag to make provenance queryable.
	req.RequiredTags = normalizeTags(append(req.RequiredTags, "taskgen", "taskgen-by-"+safeTagSuffix(agentRef)))

	runID, runRef, workItemID, err := s.createRunInTx(ctx, tx, ownerID, req.Goal, req.Constraints, req.RequiredTags, nil, isPublic)
	if err != nil {
		logError(ctx, "gateway create run: create failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create run failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "gateway create run: commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "agent", agentID, "gateway_run_created", map[string]any{
		"run_id":               runID.String(),
		"run_ref":              runRef,
		"initial_work_item_id": workItemID.String(),
	})
	writeJSON(w, http.StatusCreated, gatewayCreateRunResponse{RunRef: runRef})
}
