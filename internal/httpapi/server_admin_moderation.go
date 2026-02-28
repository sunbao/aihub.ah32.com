package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
			  and r.is_public = true
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
			join runs r on r.id = e.run_id
			where e.review_status = $1
			  and r.is_public = true
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
			join runs r on r.id = a.run_id
			where a.review_status = $1
			  and r.is_public = true
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
		logError(ctx, "admin moderation queue: query failed", err)
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
			logError(ctx, "admin moderation queue: scan failed", err)
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
	if err := rows.Err(); err != nil {
		logError(ctx, "admin moderation queue: iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
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
			logError(ctx, "admin moderation get: query run failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}

		var tags []string
		rows, err := s.db.Query(ctx, `select tag from run_required_tags where run_id=$1 order by tag asc`, runID)
		if err != nil {
			logError(ctx, "admin moderation get: query run_required_tags failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		for rows.Next() {
			var t string
			if err := rows.Scan(&t); err != nil {
				rows.Close()
				logError(ctx, "admin moderation get: scan run_required_tags failed", err)
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
			logError(ctx, "admin moderation get: iterate run_required_tags failed", err)
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
			logError(ctx, "admin moderation get: query event failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
		var payloadMap map[string]any
		if err := unmarshalJSONNullable(payload, &payloadMap); err != nil {
			logError(ctx, "admin moderation get: unmarshal event payload failed", err)
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
			logError(ctx, "admin moderation get: query artifact failed", err)
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
		logError(ctx, "admin moderation get: query moderation_actions failed", err)
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
			logError(ctx, "admin moderation get: scan moderation_actions failed", err)
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
		logError(ctx, "admin moderation get: iterate moderation_actions failed", err)
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
		logError(ctx, "admin moderation set status: db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var updated bool
	switch targetType {
	case "run":
		ct, err := tx.Exec(ctx, `update runs set review_status=$1, updated_at=now() where id=$2`, desiredStatus, id)
		if err != nil {
			logError(ctx, "admin moderation set status: update run failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
			return
		}
		updated = ct.RowsAffected() > 0
	case "event":
		ct, err := tx.Exec(ctx, `update events set review_status=$1 where id=$2`, desiredStatus, id)
		if err != nil {
			logError(ctx, "admin moderation set status: update event failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
			return
		}
		updated = ct.RowsAffected() > 0
	case "artifact":
		ct, err := tx.Exec(ctx, `update artifacts set review_status=$1 where id=$2`, desiredStatus, id)
		if err != nil {
			logError(ctx, "admin moderation set status: update artifact failed", err)
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
		logError(ctx, "admin moderation set status: insert moderation_actions failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "admin moderation set status: commit failed", err)
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
