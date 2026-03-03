package httpapi

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	agentRefRe = regexp.MustCompile(`(?i)^a_[0-9a-f]{16}$`)
	runRefRe   = regexp.MustCompile(`(?i)^r_[0-9a-f]{16}$`)
)

func normalizePublicRef(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func parseAgentRef(raw string) (string, error) {
	ref := normalizePublicRef(raw)
	if !agentRefRe.MatchString(ref) {
		return "", errors.New("invalid agent_ref")
	}
	return ref, nil
}

func parseRunRef(raw string) (string, error) {
	ref := normalizePublicRef(raw)
	if !runRefRe.MatchString(ref) {
		return "", errors.New("invalid run_ref")
	}
	return ref, nil
}

func (s server) lookupAgentIDByRef(ctx context.Context, agentRef string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `select id from agents where public_ref=$1`, agentRef).Scan(&id)
	return id, err
}

func (s server) lookupRunIDByRef(ctx context.Context, runRef string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `select id from runs where public_ref=$1`, runRef).Scan(&id)
	return id, err
}

func (s server) lookupOwnerAgentIDByRef(ctx context.Context, ownerID uuid.UUID, agentRef string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.db.QueryRow(ctx, `select id from agents where public_ref=$1 and owner_id=$2`, agentRef, ownerID).Scan(&id)
	return id, err
}

func requireAgentRefParam(w http.ResponseWriter, r *http.Request, paramName string) (string, bool) {
	raw := chi.URLParam(r, paramName)
	agentRef, err := parseAgentRef(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_ref"})
		return "", false
	}
	return agentRef, true
}

func requireRunRefParam(w http.ResponseWriter, r *http.Request, paramName string) (string, bool) {
	raw := chi.URLParam(r, paramName)
	runRef, err := parseRunRef(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run_ref"})
		return "", false
	}
	return runRef, true
}

func (s server) requireAgentFromURLRef(w http.ResponseWriter, r *http.Request, paramName string) (uuid.UUID, string, bool) {
	raw := chi.URLParam(r, paramName)
	agentRef, err := parseAgentRef(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_ref"})
		return uuid.Nil, "", false
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	agentID, err := s.lookupAgentIDByRef(ctx, agentRef)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return uuid.Nil, "", false
	}
	if err != nil {
		logError(ctx, "lookup agent by agent_ref failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return uuid.Nil, "", false
	}

	return agentID, agentRef, true
}

func (s server) requireRunFromURLRef(w http.ResponseWriter, r *http.Request, paramName string) (uuid.UUID, string, bool) {
	raw := chi.URLParam(r, paramName)
	runRef, err := parseRunRef(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run_ref"})
		return uuid.Nil, "", false
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	runID, err := s.lookupRunIDByRef(ctx, runRef)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return uuid.Nil, "", false
	}
	if err != nil {
		logError(ctx, "lookup run by run_ref failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return uuid.Nil, "", false
	}

	return runID, runRef, true
}
