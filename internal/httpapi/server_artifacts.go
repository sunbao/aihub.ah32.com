package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type runOutputDTO struct {
	RunID   string `json:"run_id"`
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Author  string `json:"author"`
	Content string `json:"content"`
}

type submitArtifactRequest struct {
	Kind           string `json:"kind"` // draft|final
	Content        string `json:"content"`
	LinkedEventSeq *int64 `json:"linked_event_seq"`
}

type submitArtifactResponse struct {
	RunID      string `json:"run_id"`
	Version    int    `json:"version"`
	Kind       string `json:"kind"`
	ArtifactID string `json:"artifact_id,omitempty"`
}

func (s server) handleGatewaySubmitArtifact(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	var req submitArtifactRequest
	if !readJSONLimited(w, r, &req, 256*1024) {
		return
	}
	req.Kind = strings.TrimSpace(req.Kind)
	if req.Kind == "" {
		req.Kind = "final"
	}
	if req.Kind != "draft" && req.Kind != "final" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid kind"})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing content"})
		return
	}
	if len(req.Content) > 200_000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content too large"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Agent must be a participant in the run.
	var participant bool
	err = s.db.QueryRow(ctx, `
		select true
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		where o.agent_id = $1 and wi.run_id = $2
		limit 1
	`, agentID, runID).Scan(&participant)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a participant"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "participant check failed"})
		return
	}

	// Review work items should emit feedback as events, not submit new artifacts (to avoid overriding run output).
	var onReviewLease bool
	if err := s.db.QueryRow(ctx, `
		select exists(
			select 1
			from work_item_leases l
			join work_items wi on wi.id = l.work_item_id
			where l.agent_id = $1
			  and wi.run_id = $2
			  and wi.kind = 'review'
			  and wi.status = 'claimed'
		)
	`, agentID, runID).Scan(&onReviewLease); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lease check failed"})
		return
	}
	if onReviewLease {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "review_work_item_no_artifacts"})
		return
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	// Lock run to serialize version allocation.
	if _, err := tx.Exec(ctx, `select 1 from runs where id=$1 for update`, runID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run lock failed"})
		return
	}

	var nextVersion int
	if err := tx.QueryRow(ctx, `select coalesce(max(version), 0) + 1 from artifacts where run_id=$1`, runID).Scan(&nextVersion); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "version allocation failed"})
		return
	}

	var linkedSeq any
	if req.LinkedEventSeq != nil && *req.LinkedEventSeq > 0 {
		linkedSeq = *req.LinkedEventSeq
	} else {
		linkedSeq = nil
	}

	var artifactID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into artifacts (run_id, version, kind, content, linked_event_seq)
		values ($1, $2, $3, $4, $5)
		returning id
	`, runID, nextVersion, req.Kind, req.Content, linkedSeq).Scan(&artifactID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	// Best-effort: automatically create a review work item for final artifacts when there is at least one other eligible agent.
	if req.Kind == "final" {
		if err := s.maybeCreateReviewWorkItem(ctx, runID, artifactID, agentID); err != nil {
			logError(ctx, "create review work item failed", err)
		}
	}

	s.audit(ctx, "agent", agentID, "artifact_submitted", map[string]any{"run_id": runID.String(), "version": nextVersion, "kind": req.Kind, "artifact_id": artifactID.String()})
	writeJSON(w, http.StatusCreated, submitArtifactResponse{RunID: runID.String(), Version: nextVersion, Kind: req.Kind, ArtifactID: artifactID.String()})
}

func (s server) maybeCreateReviewWorkItem(ctx context.Context, runID uuid.UUID, artifactID uuid.UUID, authorAgentID uuid.UUID) error {
	// Avoid duplicate review items for the same target artifact.
	var exists bool
	if err := s.db.QueryRow(ctx, `
		select exists(
			select 1
			from work_items
			where run_id = $1
			  and kind = 'review'
			  and review_context->>'target_artifact_id' = $2
		)
	`, runID, artifactID.String()).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	// Pick any other enabled participant as reviewer.
	rows, err := s.db.Query(ctx, `
		select distinct o.agent_id
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		join agents a on a.id = o.agent_id
		where wi.run_id = $1
		  and o.agent_id <> $2
		  and a.status = 'enabled'
	`, runID, authorAgentID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var candidates []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		candidates = append(candidates, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	shuffleUUIDs(ctx, candidates)
	reviewerID := candidates[0]

	authorTag := ""
	if tag, err := s.personaForAgentInRun(ctx, runID, authorAgentID); err == nil {
		authorTag = tag
	} else {
		logError(ctx, "persona lookup failed for review work item", err)
	}
	reviewContext := map[string]any{
		"target_artifact_id": artifactID.String(),
		"target_author_tag":  authorTag,
		"review_criteria":    []string{"creativity", "logic", "readability"},
	}
	reviewContextJSON, err := json.Marshal(reviewContext)
	if err != nil {
		logError(ctx, "marshal review_context failed", err)
		return err
	}

	skills := s.skillsGatewayWhitelist
	if skills == nil {
		skills = []string{}
	}
	availableSkillsJSON, err := json.Marshal(skills)
	if err != nil {
		logError(ctx, "marshal available_skills failed", err)
		return err
	}
	stageContextJSON, err := json.Marshal(s.stageContextForStage("review", skills))
	if err != nil {
		logError(ctx, "marshal stage_context failed", err)
		return err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var workItemID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into work_items (run_id, stage, kind, status, context, available_skills, review_context)
		values ($1, 'review', 'review', 'offered', $2, $3, $4)
		returning id
	`, runID, stageContextJSON, availableSkillsJSON, reviewContextJSON).Scan(&workItemID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		insert into work_item_offers (work_item_id, agent_id) values ($1, $2)
		on conflict do nothing
	`, workItemID, reviewerID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.audit(ctx, "system", platformUserID, "review_work_item_created", map[string]any{
		"run_id":             runID.String(),
		"work_item_id":       workItemID.String(),
		"reviewer_agent_id":  reviewerID.String(),
		"target_artifact_id": artifactID.String(),
	})
	return nil
}

func (s server) ownerForAgent(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	var ownerID uuid.UUID
	if err := s.db.QueryRow(ctx, `select owner_id from agents where id=$1`, agentID).Scan(&ownerID); err != nil {
		return uuid.Nil, err
	}
	return ownerID, nil
}

func (s server) handleGetRunOutputPublic(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var (
		version      int
		kind         string
		content      string
		reviewStatus string
	)
	err = s.db.QueryRow(ctx, `
		select version, kind, content, review_status
		from artifacts
		where run_id = $1
		order by version desc
		limit 1
	`, runID).Scan(&version, &kind, &content, &reviewStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no output"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if reviewStatus == "rejected" {
		content = "该作品已被管理员审核后屏蔽"
	}

	// Best-effort: find who submitted this artifact (by audit logs).
	author := ""
	var submitter uuid.UUID
	err = s.db.QueryRow(ctx, `
		select actor_id
		from audit_logs
		where actor_type = 'agent'
		  and action = 'artifact_submitted'
		  and data->>'run_id' = $1
		  and (data->>'version')::int = $2
		order by created_at desc
		limit 1
	`, runID.String(), version).Scan(&submitter)
	if err == nil {
		if p, err := s.personaForAgentInRun(ctx, runID, submitter); err == nil {
			author = p
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		logError(ctx, "audit log author lookup failed", err)
	}

	writeJSON(w, http.StatusOK, runOutputDTO{
		RunID:   runID.String(),
		Version: version,
		Kind:    kind,
		Author:  author,
		Content: content,
	})
}

func (s server) handleGetRunArtifactPublic(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}
	version, err := strconv.Atoi(strings.TrimSpace(chi.URLParam(r, "version")))
	if err != nil || version < 1 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid version"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var (
		kind           string
		content        string
		linkedEventSeq *int64
		createdAt      time.Time
		reviewStatus   string
	)
	err = s.db.QueryRow(ctx, `
		select kind, content, linked_event_seq, created_at, review_status
		from artifacts
		where run_id=$1 and version=$2
	`, runID, version).Scan(&kind, &content, &linkedEventSeq, &createdAt, &reviewStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if reviewStatus == "rejected" {
		content = "该作品已被管理员审核后屏蔽"
	}

	// Provide jump info for key nodes: if linked_event_seq is present, clients can start replay near it.
	resp := map[string]any{
		"run_id":     runID.String(),
		"version":    version,
		"kind":       kind,
		"content":    content,
		"created_at": createdAt.UTC().Format(time.RFC3339),
		"linked_seq": linkedEventSeq,
		"replay_url": "/v1/runs/" + runID.String() + "/replay",
	}
	writeJSON(w, http.StatusOK, resp)
}
