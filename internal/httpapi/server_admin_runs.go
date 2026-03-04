package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func isSmokeLikeRunGoal(goal string) bool {
	g := strings.TrimSpace(goal)
	if g == "" {
		return false
	}
	// Smoke scripts use "Smoke:" and "Smoke moderation:" prefixes.
	if strings.HasPrefix(g, "Smoke:") || strings.HasPrefix(g, "Smoke moderation:") {
		return true
	}
	return false
}

func (s server) handleAdminDeleteRun(w http.ResponseWriter, r *http.Request) {
	adminID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	runRefRaw := chi.URLParam(r, "runRef")
	runRef, err := parseRunRef(runRefRaw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run_ref"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		runID      uuid.UUID
		publisher  uuid.UUID
		goal       string
		createdAt  time.Time
	)
	err = s.db.QueryRow(ctx, `
		select id, publisher_user_id, goal, created_at
		from runs
		where public_ref = $1
	`, runRef).Scan(&runID, &publisher, &goal, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "admin delete run lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	// Production safety: only allow deleting runs owned by this admin, or runs that look like smoke runs.
	if publisher != adminID && !isSmokeLikeRunGoal(goal) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not authorized"})
		return
	}

	ct, err := s.db.Exec(ctx, `delete from runs where id = $1`, runID)
	if err != nil {
		logError(ctx, "admin delete run failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	s.audit(ctx, "admin", adminID, "run_deleted_admin", map[string]any{
		"run_id":      runID.String(),
		"run_ref":     runRef,
		"publisher_id": publisher.String(),
		"goal":        strings.TrimSpace(goal),
		"created_at":  createdAt.UTC().Format(time.RFC3339),
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

