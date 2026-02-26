package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type workItemsProgressDTO struct {
	Total     int `json:"total"`
	Offered   int `json:"offered"`
	Claimed   int `json:"claimed"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Scheduled int `json:"scheduled"`
}

type activityItemDTO struct {
	RunID     string `json:"run_id"`
	RunGoal   string `json:"run_goal"`
	RunStatus string `json:"run_status"`

	Seq       int64          `json:"seq"`
	Kind      string         `json:"kind"`
	Persona   string         `json:"persona"`
	Payload   map[string]any `json:"payload"`
	CreatedAt string         `json:"created_at"`

	WorkItems workItemsProgressDTO `json:"work_items"`
}

type activityResponse struct {
	Items      []activityItemDTO `json:"items"`
	HasMore    bool              `json:"has_more"`
	NextOffset int               `json:"next_offset"`
}

func (s server) handleListActivityPublic(w http.ResponseWriter, r *http.Request) {
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

	includeSystem := false
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("include_system"))) {
	case "1", "true", "yes", "y":
		includeSystem = true
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	args := make([]any, 0, 8)
	where := make([]string, 0, 8)
	argN := 1

	platformArg := argN
	args = append(args, platformUserID)
	argN++

	if !includeSystem {
		where = append(where, "r.publisher_user_id <> $"+strconv.Itoa(platformArg))
	}

	where = append(where, "r.review_status in ('pending','approved')")
	where = append(where, "e.review_status in ('pending','approved')")
	where = append(where, "e.is_key_node = true")

	limitPlusOne := limit + 1

	sql := `
		select
			e.run_id, e.seq, e.kind, e.persona, e.payload, e.created_at,
			left(r.goal, 400) as run_goal,
			r.status,
			coalesce(wi.total, 0) as wi_total,
			coalesce(wi.offered, 0) as wi_offered,
			coalesce(wi.claimed, 0) as wi_claimed,
			coalesce(wi.completed, 0) as wi_completed,
			coalesce(wi.failed, 0) as wi_failed,
			coalesce(wi.scheduled, 0) as wi_scheduled
		from events e
		join runs r on r.id = e.run_id
		left join lateral (
			select
				count(*)::int as total,
				count(*) filter (where status = 'offered')::int as offered,
				count(*) filter (where status = 'claimed')::int as claimed,
				count(*) filter (where status = 'completed')::int as completed,
				count(*) filter (where status = 'failed')::int as failed,
				count(*) filter (where status = 'scheduled')::int as scheduled
			from work_items
			where run_id = r.id
		) wi on true
	`
	if len(where) > 0 {
		sql += " where " + strings.Join(where, " and ")
	}
	sql += " order by e.created_at desc, e.seq desc limit $" + strconv.Itoa(argN) + " offset $" + strconv.Itoa(argN+1)
	args = append(args, limitPlusOne, offset)

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		logError(ctx, "list activity query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]activityItemDTO, 0, limit)
	for rows.Next() {
		var (
			runID     uuid.UUID
			seq       int64
			kind      string
			persona   string
			payloadB  []byte
			createdAt time.Time
			runGoal   string
			runStatus string
			wiTotal   int
			wiOffered int
			wiClaimed int
			wiDone    int
			wiFailed  int
			wiSched   int
		)
		if err := rows.Scan(
			&runID, &seq, &kind, &persona, &payloadB, &createdAt,
			&runGoal, &runStatus,
			&wiTotal, &wiOffered, &wiClaimed, &wiDone, &wiFailed, &wiSched,
		); err != nil {
			logError(ctx, "list activity scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(payloadB, &payload); err != nil {
			logError(ctx, "list activity payload unmarshal failed", err)
			payload = map[string]any{"text": "（事件内容解析失败）", "_decode_error": true}
		}

		out = append(out, activityItemDTO{
			RunID:     runID.String(),
			RunGoal:   strings.TrimSpace(runGoal),
			RunStatus: strings.TrimSpace(runStatus),
			Seq:       seq,
			Kind:      strings.TrimSpace(kind),
			Persona:   strings.TrimSpace(persona),
			Payload:   payload,
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
			WorkItems: workItemsProgressDTO{
				Total:     wiTotal,
				Offered:   wiOffered,
				Claimed:   wiClaimed,
				Completed: wiDone,
				Failed:    wiFailed,
				Scheduled: wiSched,
			},
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "list activity iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
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

	writeJSON(w, http.StatusOK, activityResponse{
		Items:      out,
		HasMore:    hasMore,
		NextOffset: nextOffset,
	})
}

