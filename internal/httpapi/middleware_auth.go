package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"aihub/internal/keys"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ctxKey string

const (
	ctxUserID  ctxKey = "user_id"
	ctxAgentID ctxKey = "agent_id"
)

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func (s server) userAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := bearerToken(r)
		if apiKey == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		hash := keys.HashAPIKey(s.pepper, apiKey)

		var userID uuid.UUID
		err := s.db.QueryRow(r.Context(), `
			select u.id
			from user_api_keys k
			join users u on u.id = k.user_id
			where k.key_hash = $1 and k.revoked_at is null
		`, hash).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		if err != nil {
			logError(r.Context(), "admin auth lookup failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth lookup failed"})
			return
		}

		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s server) agentAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := bearerToken(r)
		if apiKey == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		hash := keys.HashAPIKey(s.pepper, apiKey)

		var agentID uuid.UUID
		var status string
		err := s.db.QueryRow(r.Context(), `
			select a.id, a.status
			from agent_api_keys k
			join agents a on a.id = k.agent_id
			where k.key_hash = $1 and k.revoked_at is null
		`, hash).Scan(&agentID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth lookup failed"})
			return
		}
		if status != "enabled" {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "agent disabled"})
			return
		}

		ctx := context.WithValue(r.Context(), ctxAgentID, agentID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s server) adminAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := bearerToken(r)
		if apiKey == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing bearer token"})
			return
		}
		hash := keys.HashAPIKey(s.pepper, apiKey)

		var userID uuid.UUID
		var isAdmin bool
		err := s.db.QueryRow(r.Context(), `
			select u.id, u.is_admin
			from user_api_keys k
			join users u on u.id = k.user_id
			where k.key_hash = $1 and k.revoked_at is null
		`, hash).Scan(&userID, &isAdmin)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth lookup failed"})
			return
		}
		if !isAdmin {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
			return
		}

		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func userIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(ctxUserID)
	id, ok := v.(uuid.UUID)
	return id, ok
}

func agentIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	v := ctx.Value(ctxAgentID)
	id, ok := v.(uuid.UUID)
	return id, ok
}
