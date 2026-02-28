package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// requireRunPublicOrOwner restricts access to unlisted runs (is_public=false).
// Public runs remain accessible without authentication.
func (s server) requireRunPublicOrOwner(w http.ResponseWriter, r *http.Request, runID uuid.UUID) bool {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var (
		publisherUserID uuid.UUID
		isPublic        bool
	)
	if err := s.db.QueryRow(ctx, `select publisher_user_id, is_public from runs where id=$1`, runID).Scan(&publisherUserID, &isPublic); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return false
		}
		logError(ctx, "run access lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return false
	}

	if isPublic {
		return true
	}

	userID, ok, err := s.maybeUserIDFromRequest(ctx, r)
	if err != nil {
		logError(ctx, "run access auth lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth lookup failed"})
		return false
	}
	if !ok || userID != publisherUserID {
		// Hide existence for unlisted runs unless you own it.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return false
	}
	return true
}
