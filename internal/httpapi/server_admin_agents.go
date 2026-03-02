package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type adminAgentDTO struct {
	AgentID        string `json:"agent_id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	AdmittedStatus string `json:"admitted_status"`
	UpdatedAt      string `json:"updated_at"`
}

type adminListAgentsResponse struct {
	Items      []adminAgentDTO `json:"items"`
	HasMore    bool            `json:"has_more"`
	NextOffset int             `json:"next_offset"`
}

func (s server) handleAdminListAgents(w http.ResponseWriter, r *http.Request) {
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
			"a.name ilike $" + strconv.Itoa(argN),
			"a.id::text ilike $" + strconv.Itoa(argN),
		}
		where = append(where, "("+strings.Join(parts, " or ")+")")
		args = append(args, pat)
		argN++
	}

	sql := `
		select a.id, a.name, a.status, a.admitted_status, a.updated_at
		from agents a
	`
	if len(where) > 0 {
		sql += " where " + strings.Join(where, " and ")
	}
	sql += " order by a.updated_at desc limit $" + strconv.Itoa(argN) + " offset $" + strconv.Itoa(argN+1)
	args = append(args, limit+1, offset)

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		logError(ctx, "admin list agents query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]adminAgentDTO, 0, limit)
	for rows.Next() {
		var (
			agentID        uuid.UUID
			name           string
			status         string
			admittedStatus string
			updatedAt      time.Time
		)
		if err := rows.Scan(&agentID, &name, &status, &admittedStatus, &updatedAt); err != nil {
			logError(ctx, "admin list agents scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		out = append(out, adminAgentDTO{
			AgentID:        agentID.String(),
			Name:           strings.TrimSpace(name),
			Status:         strings.TrimSpace(status),
			AdmittedStatus: strings.TrimSpace(admittedStatus),
			UpdatedAt:      updatedAt.UTC().Format(time.RFC3339),
		})
		if len(out) >= limit+1 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "admin list agents iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	hasMore := false
	nextOffset := offset
	if len(out) > limit {
		hasMore = true
		out = out[:limit]
		nextOffset = offset + limit
	}

	writeJSON(w, http.StatusOK, adminListAgentsResponse{
		Items:      out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}
