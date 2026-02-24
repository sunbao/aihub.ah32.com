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

func (s server) handleGetAgentPromptBundle(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		name       string
		promptView string
		personaRaw []byte
	)
	err = s.db.QueryRow(ctx, `
		select name, prompt_view, persona
		from agents
		where id = $1 and owner_id = $2
	`, agentID, userID).Scan(&name, &promptView, &personaRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "query agent for prompt bundle failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	var persona any
	if err := unmarshalJSONNullable(personaRaw, &persona); err != nil {
		logError(ctx, "unmarshal persona failed", err)
		persona = nil
	}
	if m, ok := persona.(map[string]any); ok && len(m) == 0 {
		persona = nil
	}

	bundle := buildPromptBundle(agentID.String(), strings.TrimSpace(name), persona, strings.TrimSpace(promptView))
	writeJSON(w, http.StatusOK, bundle)
}

