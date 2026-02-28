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

type ownerRunWorkItemDTO struct {
	WorkItemID       string `json:"work_item_id"`
	Stage            string `json:"stage"`
	Kind             string `json:"kind"`
	Status           string `json:"status"`
	StageDescription string `json:"stage_description,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type ownerListRunWorkItemsResponse struct {
	RunID     string                `json:"run_id"`
	RunGoal   string                `json:"run_goal"`
	RunStatus string                `json:"run_status"`
	Items     []ownerRunWorkItemDTO `json:"items"`
}

func (s server) handleOwnerListRunWorkItems(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	limit := clampInt(int64Query(r, "limit", 50), 1, 200)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		runGoal         string
		runStatus       string
		isPublic        bool
		publisherUserID uuid.UUID
	)
	if err := s.db.QueryRow(ctx, `
		select goal, status, is_public, publisher_user_id
		from runs
		where id = $1
	`, runID).Scan(&runGoal, &runStatus, &isPublic, &publisherUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "owner list run work items: query run failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if !isPublic && publisherUserID != userID {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	rows, err := s.db.Query(ctx, `
		select id, stage, kind, status, context, created_at
		from work_items
		where run_id = $1
		order by created_at desc
		limit $2
	`, runID, limit)
	if err != nil {
		logError(ctx, "owner list run work items: query work_items failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]ownerRunWorkItemDTO, 0, limit)
	for rows.Next() {
		var (
			workItemID uuid.UUID
			stage      string
			kind       string
			status     string
			contextB   []byte
			createdAt  time.Time
		)
		if err := rows.Scan(&workItemID, &stage, &kind, &status, &contextB, &createdAt); err != nil {
			logError(ctx, "owner list run work items: scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}

		stageDescription := ""
		if len(contextB) > 0 {
			var m map[string]any
			if err := json.Unmarshal(contextB, &m); err != nil {
				logError(ctx, "owner list run work items: context decode failed", err)
			} else if v, ok := m["stage_description"].(string); ok {
				stageDescription = strings.TrimSpace(v)
			}
		}

		out = append(out, ownerRunWorkItemDTO{
			WorkItemID:       workItemID.String(),
			Stage:            strings.TrimSpace(stage),
			Kind:             strings.TrimSpace(kind),
			Status:           strings.TrimSpace(status),
			StageDescription: stageDescription,
			CreatedAt:        createdAt.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "owner list run work items: iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, ownerListRunWorkItemsResponse{
		RunID:     runID.String(),
		RunGoal:   strings.TrimSpace(runGoal),
		RunStatus: strings.TrimSpace(runStatus),
		Items:     out,
	})
}
