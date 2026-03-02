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

type adminAgentGatewayHealthDTO struct {
	AgentID        string `json:"agent_id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	AdmittedStatus string `json:"admitted_status"`
	PendingOffers  int    `json:"pending_offers"`
	ActiveClaims   int    `json:"active_claims"`
	LastPollAt     string `json:"last_poll_at,omitempty"`
	LastClaimAt    string `json:"last_claim_at,omitempty"`
	LastCompleteAt string `json:"last_complete_at,omitempty"`
}

type adminListAgentGatewayHealthResponse struct {
	Items      []adminAgentGatewayHealthDTO `json:"items"`
	HasMore    bool                         `json:"has_more"`
	NextOffset int                          `json:"next_offset"`
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

func (s server) handleAdminListAgentGatewayHealth(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	terms := splitSearchTerms(q)

	idsRaw := strings.TrimSpace(r.URL.Query().Get("agent_ids"))
	idParts := []string{}
	if idsRaw != "" {
		idParts = splitSearchTerms(strings.ReplaceAll(idsRaw, ",", " "))
	}

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

	args := make([]any, 0, 32)
	where := make([]string, 0, 16)
	argN := 1

	if len(idParts) > 0 {
		if len(idParts) > 50 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many agent ids"})
			return
		}
		ids := make([]uuid.UUID, 0, len(idParts))
		seen := map[uuid.UUID]bool{}
		for _, p := range idParts {
			id, err := uuid.Parse(strings.TrimSpace(p))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
				return
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			where = append(where, "a.id = any($"+strconv.Itoa(argN)+")")
			args = append(args, ids)
			argN++
		}
	}

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
		select
			a.id,
			coalesce(a.name, ''),
			coalesce(a.status, ''),
			coalesce(a.admitted_status, ''),
			coalesce(po.pending_offers, 0) as pending_offers,
			coalesce(ac.active_claims, 0) as active_claims,
			coalesce(lp.last_poll_at, null) as last_poll_at,
			coalesce(lc.last_claim_at, null) as last_claim_at,
			coalesce(lk.last_complete_at, null) as last_complete_at
		from agents a
		left join (
			select o.agent_id, count(*)::int as pending_offers
			from work_item_offers o
			join work_items wi on wi.id = o.work_item_id
			where wi.status = 'offered'
			group by o.agent_id
		) po on po.agent_id = a.id
		left join (
			select l.agent_id, count(*)::int as active_claims
			from work_item_leases l
			join work_items wi on wi.id = l.work_item_id
			where wi.status = 'claimed' and l.lease_expires_at > now()
			group by l.agent_id
		) ac on ac.agent_id = a.id
		left join (
			select actor_id, max(created_at) as last_poll_at
			from audit_logs
			where actor_type = 'agent' and action = 'gateway_poll'
			group by actor_id
		) lp on lp.actor_id = a.id
		left join (
			select actor_id, max(created_at) as last_claim_at
			from audit_logs
			where actor_type = 'agent' and action = 'work_item_claimed'
			group by actor_id
		) lc on lc.actor_id = a.id
		left join (
			select actor_id, max(created_at) as last_complete_at
			from audit_logs
			where actor_type = 'agent' and action = 'work_item_completed'
			group by actor_id
		) lk on lk.actor_id = a.id
	`
	if len(where) > 0 {
		sql += " where " + strings.Join(where, " and ")
	}
	sql += " order by a.updated_at desc limit $" + strconv.Itoa(argN) + " offset $" + strconv.Itoa(argN+1)
	args = append(args, limit+1, offset)

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		logError(ctx, "admin list agent gateway health query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]adminAgentGatewayHealthDTO, 0, limit)
	for rows.Next() {
		var (
			agentID        uuid.UUID
			name           string
			status         string
			admittedStatus string
			pendingOffers  int
			activeClaims   int
			lastPollAt     *time.Time
			lastClaimAt    *time.Time
			lastCompleteAt *time.Time
		)
		if err := rows.Scan(&agentID, &name, &status, &admittedStatus, &pendingOffers, &activeClaims, &lastPollAt, &lastClaimAt, &lastCompleteAt); err != nil {
			logError(ctx, "admin list agent gateway health scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		dto := adminAgentGatewayHealthDTO{
			AgentID:        agentID.String(),
			Name:           strings.TrimSpace(name),
			Status:         strings.TrimSpace(status),
			AdmittedStatus: strings.TrimSpace(admittedStatus),
			PendingOffers:  pendingOffers,
			ActiveClaims:   activeClaims,
		}
		if lastPollAt != nil {
			dto.LastPollAt = lastPollAt.UTC().Format(time.RFC3339)
		}
		if lastClaimAt != nil {
			dto.LastClaimAt = lastClaimAt.UTC().Format(time.RFC3339)
		}
		if lastCompleteAt != nil {
			dto.LastCompleteAt = lastCompleteAt.UTC().Format(time.RFC3339)
		}
		out = append(out, dto)
		if len(out) >= limit+1 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "admin list agent gateway health iterate failed", err)
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

	writeJSON(w, http.StatusOK, adminListAgentGatewayHealthResponse{
		Items:      out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}
