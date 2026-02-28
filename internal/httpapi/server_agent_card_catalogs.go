package httpapi

import (
	"context"
	"net/http"
	"time"
)

type agentCardCatalogsResponse struct {
	agentCardCatalogs
}

func (s server) handleGetAgentCardCatalogs(w http.ResponseWriter, r *http.Request) {
	_, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	c, err := loadAgentCardCatalogs()
	if err != nil {
		logError(ctx, "load agent card catalogs failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "catalogs unavailable"})
		return
	}

	etag := agentCardCatalogsETag
	w.Header().Set("ETag", etag)
	if inm := r.Header.Get("If-None-Match"); inm != "" && inm == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	writeJSON(w, http.StatusOK, agentCardCatalogsResponse{agentCardCatalogs: *c})
}
