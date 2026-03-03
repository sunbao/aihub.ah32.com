package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ownerGetPreReviewEvaluationResponse struct {
	EvaluationID   string                        `json:"evaluation_id"`
	AgentRef       string                        `json:"agent_ref"`
	RunRef         string                        `json:"run_ref"`
	Topic          string                        `json:"topic"`
	TopicID        string                        `json:"topic_id,omitempty"`
	WorkItemID     string                        `json:"work_item_id,omitempty"`
	SourceRunRef   string                        `json:"source_run_ref,omitempty"`
	Source         *preReviewEvaluationSourceDTO `json:"source,omitempty"`
	SourceSnapshot map[string]any                `json:"source_snapshot,omitempty"`
	Status         string                        `json:"status"`
	CreatedAt      string                        `json:"created_at"`
	ExpiresAt      string                        `json:"expires_at"`
}

func (s server) handleOwnerGetPreReviewEvaluation(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentRef, ok := requireAgentRefParam(w, r, "agentRef")
	if !ok {
		return
	}
	evaluationID, err := uuid.Parse(chi.URLParam(r, "evaluationID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid evaluation id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	agentID, err := s.lookupAgentIDByRef(ctx, agentRef)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "get pre-review evaluation: lookup agent by agent_ref failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if err := s.requireOwnerAgent(ctx, userID, agentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "get pre-review evaluation: owner check failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	var (
		runRef          string
		topic           string
		sourceRunRef    *string
		sourceTopic     *string
		sourceWorkItem  *uuid.UUID
		sourceSnapshotB []byte
		status          string
		createdAt       time.Time
		expiresAt       time.Time
	)
	if err := s.db.QueryRow(ctx, `
		select r.public_ref, e.topic, sr.public_ref, e.source_topic_id, e.source_work_item_id, e.source_snapshot,
		       r.status, e.created_at, e.expires_at
		from agent_pre_review_evaluations e
		join runs r on r.id = e.run_id
		left join runs sr on sr.id = e.source_run_id
		where e.id = $1 and e.owner_id = $2 and e.agent_id = $3
	`, evaluationID, userID, agentID).Scan(&runRef, &topic, &sourceRunRef, &sourceTopic, &sourceWorkItem, &sourceSnapshotB, &status, &createdAt, &expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "get pre-review evaluation: query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	resp := ownerGetPreReviewEvaluationResponse{
		EvaluationID: evaluationID.String(),
		AgentRef:     agentRef,
		RunRef:       runRef,
		Topic:        strings.TrimSpace(topic),
		Status:       strings.TrimSpace(status),
		CreatedAt:    createdAt.UTC().Format(time.RFC3339),
		ExpiresAt:    expiresAt.UTC().Format(time.RFC3339),
		Source:       preReviewSourceFromSnapshot(sourceSnapshotB),
	}
	if sourceRunRef != nil {
		resp.SourceRunRef = strings.TrimSpace(*sourceRunRef)
	}
	if sourceTopic != nil {
		resp.TopicID = strings.TrimSpace(*sourceTopic)
	}
	if sourceWorkItem != nil {
		resp.WorkItemID = sourceWorkItem.String()
	}
	if len(sourceSnapshotB) > 0 {
		var m map[string]any
		if err := json.Unmarshal(sourceSnapshotB, &m); err != nil {
			logError(ctx, "get pre-review evaluation: decode source_snapshot failed", err)
		} else if len(m) > 0 {
			resp.SourceSnapshot = m
		}
	}

	writeJSON(w, http.StatusOK, resp)
}
