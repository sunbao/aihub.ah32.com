package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type adminPreReviewEvaluationDTO struct {
	EvaluationID string                        `json:"evaluation_id"`
	AgentRef     string                        `json:"agent_ref"`
	AgentName    string                        `json:"agent_name"`
	RunRef       string                        `json:"run_ref"`
	Topic        string                        `json:"topic"`
	TopicID      string                        `json:"topic_id,omitempty"`
	SourceRunRef string                        `json:"source_run_ref,omitempty"`
	Source       *preReviewEvaluationSourceDTO `json:"source,omitempty"`
	RunStatus    string                        `json:"run_status"`
	CreatedAt    string                        `json:"created_at"`
	ExpiresAt    string                        `json:"expires_at"`
}

type adminListPreReviewEvaluationsResponse struct {
	Items      []adminPreReviewEvaluationDTO `json:"items"`
	HasMore    bool                          `json:"has_more"`
	NextOffset int                           `json:"next_offset"`
}

func (s server) handleAdminListPreReviewEvaluations(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	terms := splitSearchTerms(q)

	limit := 50
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

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	args := make([]any, 0, 16)
	where := make([]string, 0, 8)
	argN := 1

	for _, t := range terms {
		pat := "%" + t + "%"
		parts := []string{
			"e.id::text ilike $" + strconv.Itoa(argN),
			"r.public_ref ilike $" + strconv.Itoa(argN),
			"a.public_ref ilike $" + strconv.Itoa(argN),
			"e.topic ilike $" + strconv.Itoa(argN),
			"coalesce(e.source_topic_id, '') ilike $" + strconv.Itoa(argN),
			"coalesce(sr.public_ref, '') ilike $" + strconv.Itoa(argN),
			"a.name ilike $" + strconv.Itoa(argN),
		}
		where = append(where, "("+strings.Join(parts, " or ")+")")
		args = append(args, pat)
		argN++
	}

	limitPlusOne := limit + 1

	sql := `
		select
			e.id, a.public_ref, coalesce(a.name, '') as agent_name,
			r.public_ref, e.topic, coalesce(sr.public_ref, '') as source_run_ref, e.source_topic_id, e.source_snapshot,
			coalesce(r.status, '') as run_status,
			e.created_at, e.expires_at
		from agent_pre_review_evaluations e
		join runs r on r.id = e.run_id
		join agents a on a.id = e.agent_id
		left join runs sr on sr.id = e.source_run_id
	`
	if len(where) > 0 {
		sql += " where " + strings.Join(where, " and ")
	}
	sql += " order by e.created_at desc limit $" + strconv.Itoa(argN) + " offset $" + strconv.Itoa(argN+1)
	args = append(args, limitPlusOne, offset)

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		logError(ctx, "admin list pre-review evaluations query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]adminPreReviewEvaluationDTO, 0, limit)
	for rows.Next() {
		var (
			evalID          uuid.UUID
			agentRef        string
			agentName       string
			runRef          string
			topic           string
			sourceRunRef    string
			sourceTopic     *string
			sourceSnapshotB []byte
			runStatus       string
			createdAt       time.Time
			expiresAt       time.Time
		)
		if err := rows.Scan(&evalID, &agentRef, &agentName, &runRef, &topic, &sourceRunRef, &sourceTopic, &sourceSnapshotB, &runStatus, &createdAt, &expiresAt); err != nil {
			logError(ctx, "admin list pre-review evaluations scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		dto := adminPreReviewEvaluationDTO{
			EvaluationID: evalID.String(),
			AgentRef:     strings.TrimSpace(agentRef),
			AgentName:    strings.TrimSpace(agentName),
			RunRef:       strings.TrimSpace(runRef),
			Topic:        strings.TrimSpace(topic),
			RunStatus:    strings.TrimSpace(runStatus),
			CreatedAt:    createdAt.UTC().Format(time.RFC3339),
			ExpiresAt:    expiresAt.UTC().Format(time.RFC3339),
		}
		if strings.TrimSpace(sourceRunRef) != "" {
			dto.SourceRunRef = strings.TrimSpace(sourceRunRef)
		}
		if sourceTopic != nil {
			dto.TopicID = strings.TrimSpace(*sourceTopic)
		}
		dto.Source = preReviewSourceFromSnapshot(sourceSnapshotB)
		out = append(out, dto)
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "admin list pre-review evaluations iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	hasMore := false
	if len(out) > limit {
		hasMore = true
		out = out[:limit]
	}
	nextOffset := offset + len(out)
	if hasMore {
		nextOffset = offset + limit
	}

	writeJSON(w, http.StatusOK, adminListPreReviewEvaluationsResponse{
		Items:      out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}

func (s server) handleAdminDeletePreReviewEvaluation(w http.ResponseWriter, r *http.Request) {
	adminID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	evaluationID, err := uuid.Parse(chi.URLParam(r, "evaluationID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid evaluation id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		runID   uuid.UUID
		ownerID uuid.UUID
		agentID uuid.UUID
	)
	err = s.db.QueryRow(ctx, `
		select run_id, owner_id, agent_id
		from agent_pre_review_evaluations
		where id = $1
	`, evaluationID).Scan(&runID, &ownerID, &agentID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "admin delete pre-review evaluation lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	// Delete run to cascade-delete work_items/events/artifacts/evaluation row.
	ct, err := s.db.Exec(ctx, `delete from runs where id = $1`, runID)
	if err != nil {
		logError(ctx, "admin delete pre-review evaluation delete run failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	s.audit(ctx, "admin", adminID, "pre_review_evaluation_deleted_admin", map[string]any{
		"evaluation_id": evaluationID.String(),
		"run_id":        runID.String(),
		"owner_id":      ownerID.String(),
		"agent_id":      agentID.String(),
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}
