package httpapi

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"aihub/internal/keys"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// --- Admin moderation (post-review)

type adminIssueUserKeyResponse struct {
	UserID string `json:"user_id"`
	APIKey string `json:"api_key"`
}

func (s server) handleAdminIssueUserKey(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	apiKey, err := keys.NewAPIKey()
	if err != nil {
		logError(ctx, "admin issue user key: key generation failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key generation failed"})
		return
	}
	hash := keys.HashAPIKey(s.pepper, apiKey)

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "admin issue user key: db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	if err := tx.QueryRow(ctx, `insert into users default values returning id`).Scan(&userID); err != nil {
		logError(ctx, "admin issue user key: create user failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create user failed"})
		return
	}
	if _, err := tx.Exec(ctx, `
		insert into user_api_keys (user_id, key_hash)
		values ($1, $2)
	`, userID, hash); err != nil {
		logError(ctx, "admin issue user key: create user key failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create user key failed"})
		return
	}
	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "admin issue user key: db commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db commit failed"})
		return
	}

	s.audit(ctx, "admin", uuid.Nil, "admin_user_api_key_issued", map[string]any{"user_id": userID.String()})
	writeJSON(w, http.StatusCreated, adminIssueUserKeyResponse{UserID: userID.String(), APIKey: apiKey})
}

type moderationActionRequest struct {
	Reason string `json:"reason"`
}

type adminModerationQueueItemDTO struct {
	TargetType   string `json:"target_type"`
	ID           string `json:"id"`
	RunID        string `json:"run_id,omitempty"`
	Seq          *int64 `json:"seq,omitempty"`
	Version      *int   `json:"version,omitempty"`
	Kind         string `json:"kind,omitempty"`
	Persona      string `json:"persona,omitempty"`
	ReviewStatus string `json:"review_status"`
	Summary      string `json:"summary"`
	CreatedAt    string `json:"created_at"`
}

type adminModerationQueueResponse struct {
	Items      []adminModerationQueueItemDTO `json:"items"`
	HasMore    bool                          `json:"has_more"`
	NextOffset int                           `json:"next_offset"`
}

func isValidReviewStatus(s string) bool {
	switch s {
	case "pending", "approved", "rejected":
		return true
	default:
		return false
	}
}

func (s server) handleAdminModerationQueue(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "pending"
	}
	if !isValidReviewStatus(status) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}

	typesParam := strings.TrimSpace(r.URL.Query().Get("types"))
	includeRun, includeEvent, includeArtifact := true, true, true
	if typesParam != "" {
		includeRun, includeEvent, includeArtifact = false, false, false
		for _, t := range strings.Split(typesParam, ",") {
			switch strings.TrimSpace(t) {
			case "run":
				includeRun = true
			case "event":
				includeEvent = true
			case "artifact":
				includeArtifact = true
			case "":
				// ignore
			default:
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid type"})
				return
			}
		}
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	if offset < 0 {
		offset = 0
	}

	var selects []string
	if includeRun {
		selects = append(selects, `
			select
				'run'::text as target_type,
				r.id as id,
				r.id as run_id,
				null::bigint as seq,
				null::int as version,
				''::text as kind,
				''::text as persona,
				r.review_status as review_status,
				left(r.goal, 200) as summary,
				r.created_at as created_at
			from runs r
			where r.review_status = $1
		`)
	}
	if includeEvent {
		selects = append(selects, `
			select
				'event'::text as target_type,
				e.id as id,
				e.run_id as run_id,
				e.seq as seq,
				null::int as version,
				e.kind as kind,
				e.persona as persona,
				e.review_status as review_status,
				left(coalesce(e.payload->>'text', e.payload::text), 200) as summary,
				e.created_at as created_at
			from events e
			where e.review_status = $1
		`)
	}
	if includeArtifact {
		selects = append(selects, `
			select
				'artifact'::text as target_type,
				a.id as id,
				a.run_id as run_id,
				null::bigint as seq,
				a.version as version,
				a.kind as kind,
				''::text as persona,
				a.review_status as review_status,
				left(a.content, 200) as summary,
				a.created_at as created_at
			from artifacts a
			where a.review_status = $1
		`)
	}
	if len(selects) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no types selected"})
		return
	}

	query := `
		select *
		from (
	` + strings.Join(selects, "\nunion all\n") + `
		) q
		order by created_at desc
		limit $2 offset $3
	`

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, query, status, limit+1, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	var out []adminModerationQueueItemDTO
	for rows.Next() {
		var (
			targetType   string
			id           uuid.UUID
			runID        uuid.UUID
			seq          *int64
			version      *int
			kind         string
			persona      string
			reviewStatus string
			summary      string
			createdAt    time.Time
		)
		if err := rows.Scan(&targetType, &id, &runID, &seq, &version, &kind, &persona, &reviewStatus, &summary, &createdAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		out = append(out, adminModerationQueueItemDTO{
			TargetType:   targetType,
			ID:           id.String(),
			RunID:        runID.String(),
			Seq:          seq,
			Version:      version,
			Kind:         strings.TrimSpace(kind),
			Persona:      strings.TrimSpace(persona),
			ReviewStatus: reviewStatus,
			Summary:      summary,
			CreatedAt:    createdAt.UTC().Format(time.RFC3339),
		})
	}

	hasMore := false
	if len(out) > limit {
		hasMore = true
		out = out[:limit]
	}
	nextOffset := offset + len(out)
	if hasMore {
		nextOffset = offset + limit
	}

	writeJSON(w, http.StatusOK, adminModerationQueueResponse{
		Items:      out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}

type moderationActionDTO struct {
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	ActorType string `json:"actor_type"`
	ActorID   string `json:"actor_id"`
	CreatedAt string `json:"created_at"`
}

func (s server) handleAdminModerationGet(w http.ResponseWriter, r *http.Request) {
	targetType := strings.TrimSpace(chi.URLParam(r, "targetType"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var detail map[string]any
	switch targetType {
	case "run":
		var (
			runID         uuid.UUID
			publisherUser uuid.UUID
			goal          string
			constraints   string
			status        string
			reviewStatus  string
			createdAt     time.Time
			updatedAt     time.Time
		)
		err := s.db.QueryRow(ctx, `
			select id, publisher_user_id, goal, constraints, status, review_status, created_at, updated_at
			from runs
			where id=$1
		`, id).Scan(&runID, &publisherUser, &goal, &constraints, &status, &reviewStatus, &createdAt, &updatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}

		var tags []string
		rows, err := s.db.Query(ctx, `select tag from run_required_tags where run_id=$1 order by tag asc`, runID)
		if err != nil {
			logError(ctx, "query run_required_tags failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err != nil {
				rows.Close()
				logError(ctx, "scan run_required_tags failed", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
				return
			}
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			logError(ctx, "iterate run_required_tags failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		rows.Close()

		detail = map[string]any{
			"id":                runID.String(),
			"publisher_user_id": publisherUser.String(),
			"goal":              goal,
			"constraints":       constraints,
			"status":            status,
			"review_status":     reviewStatus,
			"required_tags":     tags,
			"created_at":        createdAt.UTC().Format(time.RFC3339),
			"updated_at":        updatedAt.UTC().Format(time.RFC3339),
		}
	case "event":
		var (
			eventID      uuid.UUID
			runID        uuid.UUID
			seq          int64
			kind         string
			persona      string
			payload      []byte
			isKey        bool
			reviewStatus string
			createdAt    time.Time
		)
		err := s.db.QueryRow(ctx, `
			select id, run_id, seq, kind, persona, payload, is_key_node, review_status, created_at
			from events
			where id=$1
		`, id).Scan(&eventID, &runID, &seq, &kind, &persona, &payload, &isKey, &reviewStatus, &createdAt)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		var payloadMap map[string]any
		if err := unmarshalJSONNullable(payload, &payloadMap); err != nil {
			logError(ctx, "unmarshal moderation event payload failed", err)
			payloadMap = map[string]any{"text": "（事件内容解析失败）", "_decode_error": true}
		}
		detail = map[string]any{
			"id":            eventID.String(),
			"run_id":        runID.String(),
			"seq":           seq,
			"kind":          kind,
			"persona":       persona,
			"payload":       payloadMap,
			"is_key_node":   isKey,
			"review_status": reviewStatus,
			"created_at":    createdAt.UTC().Format(time.RFC3339),
		}
	case "artifact":
		var (
			artifactID   uuid.UUID
			runID        uuid.UUID
			version      int
			kind         string
			content      string
			linkedSeq    *int64
			reviewStatus string
			createdAt    time.Time
		)
		err := s.db.QueryRow(ctx, `
			select id, run_id, version, kind, content, linked_event_seq, review_status, created_at
			from artifacts
			where id=$1
		`, id).Scan(&artifactID, &runID, &version, &kind, &content, &linkedSeq, &reviewStatus, &createdAt)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		detail = map[string]any{
			"id":            artifactID.String(),
			"run_id":        runID.String(),
			"version":       version,
			"kind":          kind,
			"content":       content,
			"linked_seq":    linkedSeq,
			"review_status": reviewStatus,
			"created_at":    createdAt.UTC().Format(time.RFC3339),
		}
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target type"})
		return
	}

	var actions []moderationActionDTO
	rows, err := s.db.Query(ctx, `
		select action, reason, actor_type, actor_id, created_at
		from moderation_actions
		where target_type=$1 and target_id=$2
		order by created_at desc
		limit 100
	`, targetType, id)
	if err != nil {
		logError(ctx, "query moderation_actions failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var (
			action    string
			reason    string
			actorType string
			actorID   uuid.UUID
			createdAt time.Time
		)
		if err := rows.Scan(&action, &reason, &actorType, &actorID, &createdAt); err != nil {
			logError(ctx, "scan moderation_actions failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		actions = append(actions, moderationActionDTO{
			Action:    action,
			Reason:    reason,
			ActorType: actorType,
			ActorID:   actorID.String(),
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "iterate moderation_actions failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"target_type": targetType,
		"target_id":   id.String(),
		"detail":      detail,
		"actions":     actions,
	})
}

func (s server) handleAdminModerationApprove(w http.ResponseWriter, r *http.Request) {
	s.handleAdminModerationSetStatus(w, r, "approved", "approve")
}

func (s server) handleAdminModerationReject(w http.ResponseWriter, r *http.Request) {
	s.handleAdminModerationSetStatus(w, r, "rejected", "reject")
}

func (s server) handleAdminModerationUnreject(w http.ResponseWriter, r *http.Request) {
	// After an explicit admin action, default to approved.
	s.handleAdminModerationSetStatus(w, r, "approved", "unreject")
}

func (s server) handleAdminModerationSetStatus(w http.ResponseWriter, r *http.Request, desiredStatus, action string) {
	targetType := strings.TrimSpace(chi.URLParam(r, "targetType"))
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
		return
	}
	if !isValidReviewStatus(desiredStatus) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}

	var req moderationActionRequest
	if r.ContentLength > 0 {
		if !readJSONLimited(w, r, &req, 32*1024) {
			return
		}
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason too long"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var updated bool
	switch targetType {
	case "run":
		ct, err := tx.Exec(ctx, `update runs set review_status=$1, updated_at=now() where id=$2`, desiredStatus, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
			return
		}
		updated = ct.RowsAffected() > 0
	case "event":
		ct, err := tx.Exec(ctx, `update events set review_status=$1 where id=$2`, desiredStatus, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
			return
		}
		updated = ct.RowsAffected() > 0
	case "artifact":
		ct, err := tx.Exec(ctx, `update artifacts set review_status=$1 where id=$2`, desiredStatus, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
			return
		}
		updated = ct.RowsAffected() > 0
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid target type"})
		return
	}
	if !updated {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if _, err := tx.Exec(ctx, `
		insert into moderation_actions (actor_type, actor_id, target_type, target_id, action, reason)
		values ('admin', $1, $2, $3, $4, $5)
	`, uuid.Nil, targetType, id, action, reason); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "admin", uuid.Nil, "moderation_"+action, map[string]any{
		"target_type":   targetType,
		"target_id":     id.String(),
		"review_status": desiredStatus,
		"reason":        reason,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"target_type":   targetType,
		"target_id":     id.String(),
		"review_status": desiredStatus,
	})
}

// --- Admin work item assignment (break-glass)

type adminAgentListItemDTO struct {
	AgentID     string   `json:"agent_id"`
	OwnerID     string   `json:"owner_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      string   `json:"status"`
	Tags        []string `json:"tags"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type adminAgentListResponse struct {
	Items      []adminAgentListItemDTO `json:"items"`
	HasMore    bool                    `json:"has_more"`
	NextOffset int                     `json:"next_offset"`
}

func (s server) handleAdminListAgents(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	ownerIDStr := strings.TrimSpace(r.URL.Query().Get("owner_id"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	if offset < 0 {
		offset = 0
	}

	var ownerID uuid.UUID
	if ownerIDStr != "" {
		id, err := uuid.Parse(ownerIDStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid owner_id"})
			return
		}
		ownerID = id
	}

	if q != "" && len(q) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q too long"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	args := make([]any, 0, 8)
	where := make([]string, 0, 4)
	argN := 1

	if status != "" {
		where = append(where, "a.status = $"+strconv.Itoa(argN))
		args = append(args, status)
		argN++
	}
	if ownerIDStr != "" {
		where = append(where, "a.owner_id = $"+strconv.Itoa(argN))
		args = append(args, ownerID)
		argN++
	}
	if q != "" {
		where = append(where, `(a.name ilike '%' || $`+strconv.Itoa(argN)+` || '%' or a.description ilike '%' || $`+strconv.Itoa(argN)+` || '%' or exists (select 1 from agent_tags t where t.agent_id = a.id and t.tag ilike '%' || $`+strconv.Itoa(argN)+` || '%'))`)
		args = append(args, q)
		argN++
	}

	sql := `
		select
			a.id, a.owner_id, a.name, a.description, a.status, a.created_at, a.updated_at,
			coalesce((select json_agg(tag order by tag) from agent_tags t where t.agent_id = a.id), '[]'::json) as tags_json
		from agents a
	`
	if len(where) > 0 {
		sql += " where " + strings.Join(where, " and ")
	}
	sql += " order by a.created_at desc limit $" + strconv.Itoa(argN) + " offset $" + strconv.Itoa(argN+1)
	args = append(args, limit+1, offset)

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	items := make([]adminAgentListItemDTO, 0, limit+1)
	for rows.Next() {
		var (
			agentID     uuid.UUID
			ownerID     uuid.UUID
			name        string
			description string
			status      string
			createdAt   time.Time
			updatedAt   time.Time
			tagsJSON    []byte
		)
		if err := rows.Scan(&agentID, &ownerID, &name, &description, &status, &createdAt, &updatedAt, &tagsJSON); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		var tags []string
		if err := unmarshalJSONNullable(tagsJSON, &tags); err != nil {
			logError(ctx, "unmarshal admin agents tags failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tags decode failed"})
			return
		}
		items = append(items, adminAgentListItemDTO{
			AgentID:     agentID.String(),
			OwnerID:     ownerID.String(),
			Name:        name,
			Description: description,
			Status:      status,
			Tags:        tags,
			CreatedAt:   createdAt.UTC().Format(time.RFC3339),
			UpdatedAt:   updatedAt.UTC().Format(time.RFC3339),
		})
	}

	hasMore := false
	if len(items) > limit {
		items = items[:limit]
		hasMore = true
	}
	nextOffset := offset + len(items)

	writeJSON(w, http.StatusOK, adminAgentListResponse{
		Items:      items,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}

type adminAgentRefDTO struct {
	AgentID string `json:"agent_id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
}

type adminLeaseDTO struct {
	Agent     adminAgentRefDTO `json:"agent"`
	ExpiresAt string           `json:"expires_at"`
}

type adminWorkItemDTO struct {
	WorkItemID string             `json:"work_item_id"`
	RunID      string             `json:"run_id"`
	Stage      string             `json:"stage"`
	Kind       string             `json:"kind"`
	Status     string             `json:"status"`
	Offers     []adminAgentRefDTO `json:"offers"`
	Lease      *adminLeaseDTO     `json:"lease,omitempty"`
	CreatedAt  string             `json:"created_at"`
	UpdatedAt  string             `json:"updated_at"`
}

type adminWorkItemListItemDTO struct {
	adminWorkItemDTO
	RunGoal      string   `json:"run_goal"`
	RequiredTags []string `json:"required_tags"`
}

type adminListWorkItemsResponse struct {
	Items      []adminWorkItemListItemDTO `json:"items"`
	HasMore    bool                       `json:"has_more"`
	NextOffset int                        `json:"next_offset"`
}

func (s server) handleAdminListWorkItems(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	runIDStr := strings.TrimSpace(r.URL.Query().Get("run_id"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	if offset < 0 {
		offset = 0
	}

	var runID uuid.UUID
	runIDPrefix := ""
	if runIDStr != "" {
		id, err := uuid.Parse(runIDStr)
		if err != nil {
			// Allow short prefix like "b8b5..." for admin search.
			var b strings.Builder
			for _, r := range runIDStr {
				if unicode.Is(unicode.Hex_Digit, r) {
					b.WriteRune(unicode.ToLower(r))
				}
			}
			runIDPrefix = strings.TrimSpace(b.String())
			if runIDPrefix == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
				return
			}
			if len(runIDPrefix) > 32 {
				runIDPrefix = runIDPrefix[:32]
			}
		} else {
			runID = id
		}
	}

	if q != "" {
		if len(q) > 200 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q too long"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	args := make([]any, 0, 8)
	where := make([]string, 0, 4)
	argN := 1

	if status != "" {
		where = append(where, "wi.status = $"+strconv.Itoa(argN))
		args = append(args, status)
		argN++
	}
	if runIDStr != "" {
		if runIDPrefix != "" {
			where = append(where, "replace(wi.run_id::text, '-', '') like $"+strconv.Itoa(argN)+" || '%'")
			args = append(args, runIDPrefix)
			argN++
		} else {
			where = append(where, "wi.run_id = $"+strconv.Itoa(argN))
			args = append(args, runID)
			argN++
		}
	}
	if q != "" {
		where = append(where, "(r.goal ilike '%' || $"+strconv.Itoa(argN)+" || '%' or r.constraints ilike '%' || $"+strconv.Itoa(argN)+" || '%')")
		args = append(args, q)
		argN++
	}

	sql := `
		select
			wi.id, wi.run_id, wi.stage, wi.kind, wi.status, wi.created_at, wi.updated_at,
			l.agent_id as lease_agent_id,
			coalesce(la.name, '') as lease_agent_name,
			coalesce(la.status, '') as lease_agent_status,
			l.lease_expires_at as lease_expires_at,
			coalesce(r.goal, '') as run_goal,
			coalesce(
				(select json_agg(tag order by tag) from run_required_tags rt where rt.run_id = wi.run_id),
				'[]'::json
			) as required_tags_json,
			coalesce(
				(
					select json_agg(
						jsonb_build_object('agent_id', oa.id, 'name', coalesce(oa.name, ''), 'status', coalesce(oa.status, ''))
						order by coalesce(oa.name, ''), oa.id
					)
					from work_item_offers o
					join agents oa on oa.id = o.agent_id
					where o.work_item_id = wi.id
				),
				'[]'::json
			) as offers_json
		from work_items wi
		join runs r on r.id = wi.run_id
		left join work_item_leases l on l.work_item_id = wi.id
		left join agents la on la.id = l.agent_id
	`
	if len(where) > 0 {
		sql += " where " + strings.Join(where, " and ")
	}
	sql += " order by wi.created_at desc limit $" + strconv.Itoa(argN) + " offset $" + strconv.Itoa(argN+1)
	args = append(args, limit+1, offset)

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	var out []adminWorkItemListItemDTO
	for rows.Next() {
		var (
			workItemID uuid.UUID
			runID      uuid.UUID
			stage      string
			kind       string
			statusV    string
			createdAt  time.Time
			updatedAt  time.Time

			leaseAgentID     *uuid.UUID
			leaseAgentName   string
			leaseAgentStatus string
			leaseExpiresAt   *time.Time

			runGoal          string
			requiredTagsJSON []byte
			offersJSON       []byte
		)
		if err := rows.Scan(
			&workItemID, &runID, &stage, &kind, &statusV, &createdAt, &updatedAt,
			&leaseAgentID, &leaseAgentName, &leaseAgentStatus, &leaseExpiresAt,
			&runGoal, &requiredTagsJSON,
			&offersJSON,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}

		var offers []adminAgentRefDTO
		if err := unmarshalJSONNullable(offersJSON, &offers); err != nil {
			logError(ctx, "unmarshal admin work items offers failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "offers decode failed"})
			return
		}

		var requiredTags []string
		if err := unmarshalJSONNullable(requiredTagsJSON, &requiredTags); err != nil {
			logError(ctx, "unmarshal admin work items required_tags failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "required tags decode failed"})
			return
		}
		for i := range requiredTags {
			requiredTags[i] = strings.TrimSpace(requiredTags[i])
		}

		var lease *adminLeaseDTO
		if leaseAgentID != nil && leaseExpiresAt != nil {
			lease = &adminLeaseDTO{
				Agent: adminAgentRefDTO{
					AgentID: leaseAgentID.String(),
					Name:    leaseAgentName,
					Status:  leaseAgentStatus,
				},
				ExpiresAt: leaseExpiresAt.UTC().Format(time.RFC3339),
			}
		}

		out = append(out, adminWorkItemListItemDTO{
			adminWorkItemDTO: adminWorkItemDTO{
				WorkItemID: workItemID.String(),
				RunID:      runID.String(),
				Stage:      stage,
				Kind:       kind,
				Status:     statusV,
				Offers:     offers,
				Lease:      lease,
				CreatedAt:  createdAt.UTC().Format(time.RFC3339),
				UpdatedAt:  updatedAt.UTC().Format(time.RFC3339),
			},
			RunGoal:      runGoal,
			RequiredTags: requiredTags,
		})
	}

	hasMore := false
	if len(out) > limit {
		hasMore = true
		out = out[:limit]
	}
	nextOffset := offset + len(out)
	if hasMore {
		nextOffset = offset + limit
	}

	writeJSON(w, http.StatusOK, adminListWorkItemsResponse{
		Items:      out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}

type adminWorkItemDetailDTO struct {
	WorkItem       adminWorkItemDTO `json:"work_item"`
	RunGoal        string           `json:"run_goal"`
	RunConstraints string           `json:"run_constraints"`
	RequiredTags   []string         `json:"required_tags"`
}

func (s server) handleAdminGetWorkItem(w http.ResponseWriter, r *http.Request) {
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work item id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var (
		runID          uuid.UUID
		stage          string
		kind           string
		statusV        string
		createdAt      time.Time
		updatedAt      time.Time
		runGoal        string
		runConstraints string

		leaseAgentID     *uuid.UUID
		leaseAgentName   string
		leaseAgentStatus string
		leaseExpiresAt   *time.Time

		offersJSON []byte
	)

	err = s.db.QueryRow(ctx, `
		select
			wi.run_id, wi.stage, wi.kind, wi.status, wi.created_at, wi.updated_at,
			r.goal, r.constraints,
			l.agent_id as lease_agent_id,
			coalesce(la.name, '') as lease_agent_name,
			coalesce(la.status, '') as lease_agent_status,
			l.lease_expires_at as lease_expires_at,
			coalesce(
				json_agg(
					jsonb_build_object('agent_id', oa.id, 'name', coalesce(oa.name, ''), 'status', coalesce(oa.status, ''))
					order by coalesce(oa.name, ''), oa.id
				) filter (where oa.id is not null),
				'[]'::json
			) as offers_json
		from work_items wi
		join runs r on r.id = wi.run_id
		left join work_item_leases l on l.work_item_id = wi.id
		left join agents la on la.id = l.agent_id
		left join work_item_offers o on o.work_item_id = wi.id
		left join agents oa on oa.id = o.agent_id
		where wi.id = $1
		group by wi.id, r.id, l.agent_id, la.name, la.status, l.lease_expires_at
	`, workItemID).Scan(
		&runID, &stage, &kind, &statusV, &createdAt, &updatedAt,
		&runGoal, &runConstraints,
		&leaseAgentID, &leaseAgentName, &leaseAgentStatus, &leaseExpiresAt,
		&offersJSON,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	var offers []adminAgentRefDTO
	if err := unmarshalJSONNullable(offersJSON, &offers); err != nil {
		logError(ctx, "unmarshal admin work item offers failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "offers decode failed"})
		return
	}

	var lease *adminLeaseDTO
	if leaseAgentID != nil && leaseExpiresAt != nil {
		lease = &adminLeaseDTO{
			Agent: adminAgentRefDTO{
				AgentID: leaseAgentID.String(),
				Name:    leaseAgentName,
				Status:  leaseAgentStatus,
			},
			ExpiresAt: leaseExpiresAt.UTC().Format(time.RFC3339),
		}
	}

	var requiredTags []string
	rows, err := s.db.Query(ctx, `select tag from run_required_tags where run_id=$1 order by tag asc`, runID)
	if err != nil {
		logError(ctx, "query run_required_tags failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			logError(ctx, "scan run_required_tags failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		t = strings.TrimSpace(t)
		if t != "" {
			requiredTags = append(requiredTags, t)
		}
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "iterate run_required_tags failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	writeJSON(w, http.StatusOK, adminWorkItemDetailDTO{
		WorkItem: adminWorkItemDTO{
			WorkItemID: workItemID.String(),
			RunID:      runID.String(),
			Stage:      stage,
			Kind:       kind,
			Status:     statusV,
			Offers:     offers,
			Lease:      lease,
			CreatedAt:  createdAt.UTC().Format(time.RFC3339),
			UpdatedAt:  updatedAt.UTC().Format(time.RFC3339),
		},
		RunGoal:        runGoal,
		RunConstraints: runConstraints,
		RequiredTags:   requiredTags,
	})
}

type adminWorkItemCandidateDTO struct {
	AgentID     string   `json:"agent_id"`
	Name        string   `json:"name"`
	Tags        []string `json:"tags"`
	Hits        int      `json:"hits"`
	MatchedTags []string `json:"matched_tags"`
	MissingTags []string `json:"missing_tags"`
}

type adminWorkItemCandidatesResponse struct {
	WorkItemID   string                      `json:"work_item_id"`
	RunID        string                      `json:"run_id"`
	RequiredTags []string                    `json:"required_tags"`
	Matched      []adminWorkItemCandidateDTO `json:"matched"`
	Fallback     []adminWorkItemCandidateDTO `json:"fallback"`
}

func (s server) handleAdminWorkItemCandidates(w http.ResponseWriter, r *http.Request) {
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work item id"})
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 500)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var runID uuid.UUID
	if err := s.db.QueryRow(ctx, `select run_id from work_items where id=$1`, workItemID).Scan(&runID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	var requiredTags []string
	rows, err := s.db.Query(ctx, `select tag from run_required_tags where run_id=$1 order by tag asc`, runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		t = strings.TrimSpace(t)
		if t != "" {
			requiredTags = append(requiredTags, t)
		}
	}
	rows.Close()

	type rowDTO struct {
		id          uuid.UUID
		name        string
		status      string
		tags        []string
		matchedTags []string
		hits        int
	}

	var candidateRows []rowDTO
	if len(requiredTags) == 0 {
		rows, err := s.db.Query(ctx, `
			select a.id, a.name, a.status,
			       coalesce(array_agg(distinct t.tag) filter (where t.tag is not null), '{}'::text[]) as tags
			from agents a
			left join agent_tags t on t.agent_id = a.id
			where a.status='enabled'
			group by a.id
			order by random()
			limit $1
		`, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		defer rows.Close()
		for rows.Next() {
			var r rowDTO
			if err := rows.Scan(&r.id, &r.name, &r.status, &r.tags); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
				return
			}
			r.hits = 0
			candidateRows = append(candidateRows, r)
		}
	} else {
		rows, err := s.db.Query(ctx, `
			select
				a.id,
				a.name,
				a.status,
				coalesce(array_agg(distinct at.tag) filter (where at.tag is not null), '{}'::text[]) as tags,
				coalesce(array_agg(distinct mt.tag) filter (where mt.tag is not null), '{}'::text[]) as matched_tags,
				count(distinct mt.tag) as hits
			from agents a
			left join agent_tags at on at.agent_id = a.id
			left join agent_tags mt on mt.agent_id = a.id and mt.tag = any($1)
			where a.status='enabled'
			group by a.id
			order by hits desc, random()
			limit $2
		`, requiredTags, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		defer rows.Close()
		for rows.Next() {
			var r rowDTO
			if err := rows.Scan(&r.id, &r.name, &r.status, &r.tags, &r.matchedTags, &r.hits); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
				return
			}
			candidateRows = append(candidateRows, r)
		}
	}

	requiredSet := map[string]struct{}{}
	for _, t := range requiredTags {
		requiredSet[t] = struct{}{}
	}

	var matched []adminWorkItemCandidateDTO
	var fallback []adminWorkItemCandidateDTO
	for _, rrow := range candidateRows {
		sort.Strings(rrow.tags)
		sort.Strings(rrow.matchedTags)

		matchedSet := map[string]struct{}{}
		for _, t := range rrow.matchedTags {
			t = strings.TrimSpace(t)
			if t != "" {
				matchedSet[t] = struct{}{}
			}
		}
		var missing []string
		for t := range requiredSet {
			if _, ok := matchedSet[t]; !ok {
				missing = append(missing, t)
			}
		}
		sort.Strings(missing)

		dto := adminWorkItemCandidateDTO{
			AgentID:     rrow.id.String(),
			Name:        strings.TrimSpace(rrow.name),
			Tags:        rrow.tags,
			Hits:        rrow.hits,
			MatchedTags: rrow.matchedTags,
			MissingTags: missing,
		}
		if rrow.hits > 0 {
			matched = append(matched, dto)
		} else {
			fallback = append(fallback, dto)
		}
	}

	writeJSON(w, http.StatusOK, adminWorkItemCandidatesResponse{
		WorkItemID:   workItemID.String(),
		RunID:        runID.String(),
		RequiredTags: requiredTags,
		Matched:      matched,
		Fallback:     fallback,
	})
}

type adminAssignWorkItemRequest struct {
	AgentIDs []string `json:"agent_ids"`
	Mode     string   `json:"mode"` // add|force_reassign
	Reason   string   `json:"reason"`
}

func (s server) handleAdminAssignWorkItem(w http.ResponseWriter, r *http.Request) {
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work item id"})
		return
	}

	var req adminAssignWorkItemRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	mode := strings.TrimSpace(req.Mode)
	if mode == "" {
		mode = "add"
	}
	if mode != "add" && mode != "force_reassign" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid mode"})
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason too long"})
		return
	}
	if mode == "force_reassign" && reason == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason required for force_reassign"})
		return
	}

	seen := map[uuid.UUID]struct{}{}
	var agentIDs []uuid.UUID
	for _, sID := range req.AgentIDs {
		sID = strings.TrimSpace(sID)
		if sID == "" {
			continue
		}
		id, err := uuid.Parse(sID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
			return
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		agentIDs = append(agentIDs, id)
	}
	if len(agentIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing agent_ids"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var wiStatus string
	if err := tx.QueryRow(ctx, `select status from work_items where id=$1 for update`, workItemID).Scan(&wiStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if wiStatus == "completed" || wiStatus == "failed" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "work item is not assignable"})
		return
	}

	if mode == "force_reassign" {
		if _, err := tx.Exec(ctx, `delete from work_item_leases where work_item_id=$1`, workItemID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lease delete failed"})
			return
		}
		if _, err := tx.Exec(ctx, `update work_items set status='offered', updated_at=now() where id=$1`, workItemID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "status update failed"})
			return
		}
	}

	// Validate enabled agents (and avoid FK errors).
	rows, err := tx.Query(ctx, `select id from agents where status='enabled' and id = any($1)`, agentIDs)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent check failed"})
		return
	}
	found := map[uuid.UUID]struct{}{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "agent check failed"})
			return
		}
		found[id] = struct{}{}
	}
	rows.Close()
	if len(found) != len(agentIDs) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "some agents not found or disabled"})
		return
	}

	for _, agentID := range agentIDs {
		if _, err := tx.Exec(ctx, `
			insert into work_item_offers (work_item_id, agent_id) values ($1, $2)
			on conflict do nothing
		`, workItemID, agentID); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert offer failed"})
			return
		}
	}

	if _, err := tx.Exec(ctx, `
		insert into work_item_assignment_actions (actor_type, actor_id, work_item_id, action, mode, agent_ids, reason)
		values ('admin', $1, $2, 'assign', $3, $4, $5)
	`, uuid.Nil, workItemID, mode, agentIDs, reason); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert audit failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	var agentIDStrs []string
	for _, id := range agentIDs {
		agentIDStrs = append(agentIDStrs, id.String())
	}
	s.audit(ctx, "admin", uuid.Nil, "work_item_assigned", map[string]any{
		"work_item_id": workItemID.String(),
		"agent_ids":    agentIDStrs,
		"mode":         mode,
		"reason":       reason,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"work_item_id": workItemID.String(),
		"agent_ids":    agentIDStrs,
		"mode":         mode,
	})
}

type adminUnassignWorkItemRequest struct {
	AgentIDs []string `json:"agent_ids"`
	Reason   string   `json:"reason"`
}

func (s server) handleAdminUnassignWorkItem(w http.ResponseWriter, r *http.Request) {
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work item id"})
		return
	}

	var req adminUnassignWorkItemRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 2000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reason too long"})
		return
	}

	seen := map[uuid.UUID]struct{}{}
	var agentIDs []uuid.UUID
	for _, sID := range req.AgentIDs {
		sID = strings.TrimSpace(sID)
		if sID == "" {
			continue
		}
		id, err := uuid.Parse(sID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent_id"})
			return
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		agentIDs = append(agentIDs, id)
	}
	if len(agentIDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing agent_ids"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `select exists(select 1 from work_items where id=$1)`, workItemID).Scan(&exists); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if _, err := tx.Exec(ctx, `delete from work_item_offers where work_item_id=$1 and agent_id = any($2)`, workItemID, agentIDs); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete offer failed"})
		return
	}

	if _, err := tx.Exec(ctx, `
		insert into work_item_assignment_actions (actor_type, actor_id, work_item_id, action, mode, agent_ids, reason)
		values ('admin', $1, $2, 'unassign', 'remove', $3, $4)
	`, uuid.Nil, workItemID, agentIDs, reason); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert audit failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	var agentIDStrs []string
	for _, id := range agentIDs {
		agentIDStrs = append(agentIDStrs, id.String())
	}
	s.audit(ctx, "admin", uuid.Nil, "work_item_unassigned", map[string]any{
		"work_item_id": workItemID.String(),
		"agent_ids":    agentIDStrs,
		"reason":       reason,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"work_item_id": workItemID.String(),
		"agent_ids":    agentIDStrs,
	})
}
