package httpapi

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s server) handleRunStreamSSE(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}
	if !s.requireRunPublicOrOwner(w, r, runID) {
		return
	}

	afterSeq := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("after_seq")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after_seq"})
			return
		}
		afterSeq = n
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()

	bw := bufio.NewWriterSize(w, 16*1024)
	defer func() {
		if err := bw.Flush(); err != nil {
			logError(ctx, "sse flush failed", err)
		}
	}()

	// Backfill.
	events, err := s.fetchEvents(ctx, runID, afterSeq, 500)
	if err != nil {
		logError(ctx, "sse backfill fetch failed", err)
		if err := writeSSE(bw, "error", map[string]string{"error": "backfill failed"}); err != nil {
			logError(ctx, "sse write failed", err)
			return
		}
		if err := bw.Flush(); err != nil {
			logError(ctx, "sse flush failed", err)
		}
		return
	}
	for _, ev := range events {
		if err := writeSSE(bw, "event", ev); err != nil {
			logError(ctx, "sse write failed", err)
			return
		}
	}
	if err := bw.Flush(); err != nil {
		logError(ctx, "sse flush failed", err)
		return
	}
	flusher.Flush()

	// Subscribe for live events.
	ch := s.br.subscribe(runID)
	defer s.br.unsubscribe(runID, ch)

	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			if ev.Seq <= afterSeq {
				continue
			}
			afterSeq = ev.Seq
			if err := writeSSE(bw, "event", ev); err != nil {
				logError(ctx, "sse write failed", err)
				return
			}
			if err := bw.Flush(); err != nil {
				logError(ctx, "sse flush failed", err)
				return
			}
			flusher.Flush()
		case <-keepAlive.C:
			if _, err := bw.WriteString(": keepalive\n\n"); err != nil {
				logError(ctx, "sse keepalive write failed", err)
				return
			}
			if err := bw.Flush(); err != nil {
				logError(ctx, "sse flush failed", err)
				return
			}
			flusher.Flush()
		}
	}
}

func (s server) handleRunReplay(w http.ResponseWriter, r *http.Request) {
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}
	if !s.requireRunPublicOrOwner(w, r, runID) {
		return
	}

	afterSeq := int64(0)
	if v := strings.TrimSpace(r.URL.Query().Get("after_seq")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after_seq"})
			return
		}
		afterSeq = n
	}
	limit := clampInt(int64Query(r, "limit", 200), 1, 500)

	events, err := s.fetchEvents(r.Context(), runID, afterSeq, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	var keyNodes []eventDTO
	for _, ev := range events {
		if ev.IsKeyNode {
			keyNodes = append(keyNodes, ev)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":    runID.String(),
		"events":    events,
		"key_nodes": keyNodes,
		"after_seq": afterSeq,
		"limit":     limit,
	})
}

func int64Query(r *http.Request, key string, fallback int) int {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func writeSSE(w *bufio.Writer, eventName string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := w.WriteString("event: " + eventName + "\n"); err != nil {
		return err
	}
	if _, err := w.WriteString("data: "); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	if _, err := w.WriteString("\n\n"); err != nil {
		return err
	}
	return nil
}

func (s server) fetchEvents(ctx context.Context, runID uuid.UUID, afterSeq int64, limit int) ([]eventDTO, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select seq, kind, persona, payload, is_key_node, created_at, review_status
		from events
		where run_id = $1 and seq > $2
		order by seq asc
		limit $3
	`, runID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []eventDTO
	for rows.Next() {
		var (
			seq          int64
			kind         string
			persona      string
			payloadB     []byte
			isKeyNode    bool
			createdAt    time.Time
			reviewStatus string
		)
		if err := rows.Scan(&seq, &kind, &persona, &payloadB, &isKeyNode, &createdAt, &reviewStatus); err != nil {
			return nil, err
		}
		var payload map[string]any
		if err := unmarshalJSONNullable(payloadB, &payload); err != nil {
			logError(ctx, "unmarshal event payload failed", err)
			payload = map[string]any{"text": "（事件内容解析失败）", "_decode_error": true}
		}
		if reviewStatus == "rejected" {
			payload = map[string]any{"text": "该内容已被管理员审核后屏蔽", "_redacted": true}
		}
		out = append(out, eventDTO{
			RunID:     runID.String(),
			Seq:       seq,
			Kind:      kind,
			Persona:   persona,
			Payload:   payload,
			IsKeyNode: isKeyNode,
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

func (s server) handleReplaceAgentTags(w http.ResponseWriter, r *http.Request) {
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

	var req replaceTagsRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	tags := normalizeTags(req.Tags)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var exists bool
	if err := tx.QueryRow(ctx, `select true from agents where id=$1 and owner_id=$2`, agentID, userID).Scan(&exists); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	if _, err := tx.Exec(ctx, `delete from agent_tags where agent_id=$1`, agentID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete tags failed"})
		return
	}
	for _, t := range tags {
		if _, err := tx.Exec(ctx, `
			insert into agent_tags (agent_id, tag) values ($1, $2)
			on conflict do nothing
		`, agentID, t); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert tags failed"})
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

func (s server) handleGatewayPoll(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	s.audit(ctx, "agent", agentID, "gateway_poll", map[string]any{})

	rows, err := s.db.Query(ctx, `
		select wi.id, wi.run_id, wi.stage, wi.kind, wi.status, wi.context, wi.available_skills, wi.review_context
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		left join work_item_leases l on l.work_item_id = wi.id
		where o.agent_id = $1
		  and (
		    wi.status = 'offered'
		    or (wi.status = 'claimed' and l.agent_id = $1 and l.lease_expires_at > now())
		  )
		order by wi.created_at asc
		limit 50
	`, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	type offerDTO struct {
		WorkItemID      string         `json:"work_item_id"`
		RunID           string         `json:"run_id"`
		Stage           string         `json:"stage"`
		Kind            string         `json:"kind"`
		Status          string         `json:"status"`
		Goal            string         `json:"goal"`
		Constraints     string         `json:"constraints"`
		StageContext    map[string]any `json:"stage_context,omitempty"`
		AvailableSkills []string       `json:"available_skills,omitempty"`
		ReviewContext   map[string]any `json:"review_context,omitempty"`
	}
	offers := make([]offerDTO, 0)
	artifactRefsCache := map[uuid.UUID][]artifactRefDTO{}
	for rows.Next() {
		var (
			workItemID      uuid.UUID
			runID           uuid.UUID
			stage           string
			kind            string
			status          string
			context         []byte
			availableSkills []byte
			reviewContext   []byte
		)
		if err := rows.Scan(&workItemID, &runID, &stage, &kind, &status, &context, &availableSkills, &reviewContext); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}

		var goal, constraints string
		if err := s.db.QueryRow(ctx, `select goal, constraints from runs where id=$1`, runID).Scan(&goal, &constraints); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run lookup failed"})
			return
		}

		// Parse JSON fields
		var stageContext map[string]any
		var skills []string
		var revCtx map[string]any
		if err := unmarshalJSONNullable(context, &stageContext); err != nil {
			logError(ctx, "unmarshal work item context failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "context decode failed"})
			return
		}
		if err := unmarshalJSONNullable(availableSkills, &skills); err != nil {
			logError(ctx, "unmarshal work item available_skills failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "available skills decode failed"})
			return
		}
		if err := unmarshalJSONNullable(reviewContext, &revCtx); err != nil {
			logError(ctx, "unmarshal work item review_context failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "review context decode failed"})
			return
		}

		refs, ok := artifactRefsCache[runID]
		if !ok {
			refs, err = s.listArtifactRefs(ctx, runID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "artifact refs lookup failed"})
				return
			}
			artifactRefsCache[runID] = refs
		}
		if stageContext == nil {
			stageContext = map[string]any{}
		}
		stageContext["available_skills"] = skills
		stageContext["previous_artifacts"] = refs

		offers = append(offers, offerDTO{
			WorkItemID:      workItemID.String(),
			RunID:           runID.String(),
			Stage:           stage,
			Kind:            kind,
			Status:          status,
			Goal:            goal,
			Constraints:     constraints,
			StageContext:    stageContext,
			AvailableSkills: skills,
			ReviewContext:   revCtx,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agent_id": agentID.String(), "offers": offers})
}

type gatewayTaskDTO struct {
	RunID      string   `json:"run_id"`
	WorkItemID string   `json:"work_item_id"`
	Goal       string   `json:"goal"`
	Tags       []string `json:"tags,omitempty"`
	Reward     string   `json:"reward"`
}

func (s server) handleGatewayTasks(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	limit, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("limit")))
	limit = clampInt(limit, 1, 200)

	tags := normalizeTags(strings.Split(strings.TrimSpace(r.URL.Query().Get("tags")), ","))

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	s.audit(ctx, "agent", agentID, "gateway_tasks_list", map[string]any{"tags": tags, "limit": limit})

	args := []any{agentID, limit}
	where := ""
	if len(tags) > 0 {
		where = "and exists (select 1 from run_required_tags t2 where t2.run_id = wi.run_id and t2.tag = any($3))"
		args = append(args, tags)
	}

	rows, err := s.db.Query(ctx, `
		select
			wi.run_id,
			wi.id,
			r.goal,
			coalesce(array_agg(distinct t.tag) filter (where t.tag is not null), '{}'::text[]) as tags
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		join runs r on r.id = wi.run_id
		left join run_required_tags t on t.run_id = wi.run_id
		where o.agent_id = $1
		  and wi.status = 'offered'
		  `+where+`
		group by wi.run_id, wi.id, r.goal
		order by wi.created_at asc
		limit $2
	`, args...)
	if err != nil {
		logError(ctx, "query gateway tasks failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]gatewayTaskDTO, 0)
	for rows.Next() {
		var (
			runID      uuid.UUID
			workItemID uuid.UUID
			goal       string
			taskTags   []string
		)
		if err := rows.Scan(&runID, &workItemID, &goal, &taskTags); err != nil {
			logError(ctx, "scan gateway tasks failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		out = append(out, gatewayTaskDTO{
			RunID:      runID.String(),
			WorkItemID: workItemID.String(),
			Goal:       strings.TrimSpace(goal),
			Tags:       taskTags,
			Reward:     "contribution_points",
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "iterate gateway tasks failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"tasks": out})
}

type workItemDetailDTO struct {
	WorkItemID      string         `json:"work_item_id"`
	RunID           string         `json:"run_id"`
	Stage           string         `json:"stage"`
	Kind            string         `json:"kind"`
	Status          string         `json:"status"`
	Goal            string         `json:"goal"`
	Constraints     string         `json:"constraints"`
	StageContext    map[string]any `json:"stage_context,omitempty"`
	AvailableSkills []string       `json:"available_skills,omitempty"`
	ReviewContext   map[string]any `json:"review_context,omitempty"`
	ScheduledAt     *time.Time     `json:"scheduled_at,omitempty"`
	CreatedAt       string         `json:"created_at"`
	UpdatedAt       string         `json:"updated_at"`
}

func (s server) handleGatewayGetWorkItem(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work_item_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Must be offered to this agent.
	var offered bool
	if err := s.db.QueryRow(ctx, `select true from work_item_offers where work_item_id=$1 and agent_id=$2`, workItemID, agentID).Scan(&offered); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not offered"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "offer check failed"})
		return
	}

	var (
		runID           uuid.UUID
		stage           string
		kind            string
		status          string
		createdAt       time.Time
		updatedAt       time.Time
		goal            string
		constraints     string
		context         []byte
		availableSkills []byte
		reviewContext   []byte
		scheduledAt     *time.Time
	)
	err = s.db.QueryRow(ctx, `
		select wi.run_id, wi.stage, wi.kind, wi.status, wi.created_at, wi.updated_at, r.goal, r.constraints, wi.context, wi.available_skills, wi.review_context, wi.scheduled_at
		from work_items wi
		join runs r on r.id = wi.run_id
		where wi.id = $1
	`, workItemID).Scan(&runID, &stage, &kind, &status, &createdAt, &updatedAt, &goal, &constraints, &context, &availableSkills, &reviewContext, &scheduledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	// Parse JSON fields
	var stageContext map[string]any
	var skills []string
	var revCtx map[string]any
	if err := unmarshalJSONNullable(context, &stageContext); err != nil {
		logError(ctx, "unmarshal work item context failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "context decode failed"})
		return
	}
	if err := unmarshalJSONNullable(availableSkills, &skills); err != nil {
		logError(ctx, "unmarshal work item available_skills failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "available skills decode failed"})
		return
	}
	if err := unmarshalJSONNullable(reviewContext, &revCtx); err != nil {
		logError(ctx, "unmarshal work item review_context failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "review context decode failed"})
		return
	}

	refs, err := s.listArtifactRefs(ctx, runID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "artifact refs lookup failed"})
		return
	}
	if stageContext == nil {
		stageContext = map[string]any{}
	}
	stageContext["available_skills"] = skills
	stageContext["previous_artifacts"] = refs

	s.audit(ctx, "agent", agentID, "work_item_read", map[string]any{"work_item_id": workItemID.String(), "run_id": runID.String()})
	writeJSON(w, http.StatusOK, workItemDetailDTO{
		WorkItemID:      workItemID.String(),
		RunID:           runID.String(),
		Stage:           stage,
		Kind:            kind,
		Status:          status,
		Goal:            goal,
		Constraints:     constraints,
		StageContext:    stageContext,
		AvailableSkills: skills,
		ReviewContext:   revCtx,
		ScheduledAt:     scheduledAt,
		CreatedAt:       createdAt.UTC().Format(time.RFC3339),
		UpdatedAt:       updatedAt.UTC().Format(time.RFC3339),
	})
}

type workItemSkillsResponse struct {
	WorkItemID string     `json:"work_item_id"`
	RunID      string     `json:"run_id"`
	Skills     []skillDTO `json:"skills"`
}

type skillDTO struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

func (s server) handleGatewayWorkItemSkills(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work_item_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Must be offered to this agent.
	var offered bool
	if err := s.db.QueryRow(ctx, `select true from work_item_offers where work_item_id=$1 and agent_id=$2`, workItemID, agentID).Scan(&offered); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not offered"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "offer check failed"})
		return
	}

	var (
		runID           uuid.UUID
		availableSkills []byte
	)
	if err := s.db.QueryRow(ctx, `select run_id, available_skills from work_items where id=$1`, workItemID).Scan(&runID, &availableSkills); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	var skills []string
	if err := unmarshalJSONNullable(availableSkills, &skills); err != nil {
		logError(ctx, "unmarshal work item available_skills failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "available skills decode failed"})
		return
	}
	out := make([]skillDTO, 0, len(skills))
	for _, name := range skills {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, skillDTO{
			Name:        name,
			Description: "",
			Parameters:  map[string]any{},
		})
	}

	writeJSON(w, http.StatusOK, workItemSkillsResponse{
		WorkItemID: workItemID.String(),
		RunID:      runID.String(),
		Skills:     out,
	})
}

type artifactRefDTO struct {
	Version   int    `json:"version"`
	Kind      string `json:"kind"`
	URL       string `json:"url"`
	CreatedAt string `json:"created_at"`
}

func (s server) listArtifactRefs(ctx context.Context, runID uuid.UUID) ([]artifactRefDTO, error) {
	rows, err := s.db.Query(ctx, `
		select version, kind, created_at
		from artifacts
		where run_id = $1 and review_status <> 'rejected'
		order by version asc
	`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]artifactRefDTO, 0)
	for rows.Next() {
		var (
			version   int
			kind      string
			createdAt time.Time
		)
		if err := rows.Scan(&version, &kind, &createdAt); err != nil {
			return nil, err
		}
		out = append(out, artifactRefDTO{
			Version:   version,
			Kind:      kind,
			URL:       "/v1/runs/" + runID.String() + "/artifacts/" + strconv.Itoa(version),
			CreatedAt: createdAt.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}

type claimResponse struct {
	WorkItemID      string         `json:"work_item_id"`
	RunID           string         `json:"run_id"`
	Stage           string         `json:"stage"`
	Kind            string         `json:"kind"`
	Status          string         `json:"status"`
	Goal            string         `json:"goal"`
	Constraints     string         `json:"constraints"`
	StageContext    map[string]any `json:"stage_context,omitempty"`
	AvailableSkills []string       `json:"available_skills,omitempty"`
	ReviewContext   map[string]any `json:"review_context,omitempty"`
	LeaseExpiresAt  string         `json:"lease_expires_at"`
}

func (s server) enrichStageContextForOffer(ctx context.Context, runID uuid.UUID, stageContext map[string]any, skills []string) (map[string]any, error) {
	if stageContext == nil {
		stageContext = map[string]any{}
	}
	refs, err := s.listArtifactRefs(ctx, runID)
	if err != nil {
		return nil, err
	}
	stageContext["available_skills"] = skills
	stageContext["previous_artifacts"] = refs
	return stageContext, nil
}

func (s server) buildClaimResponse(ctx context.Context, workItemID uuid.UUID, leaseExpiresAt time.Time) (claimResponse, error) {
	var (
		runID           uuid.UUID
		stage           string
		kind            string
		goal            string
		constraints     string
		contextB        []byte
		availableSkills []byte
		reviewContextB  []byte
	)
	if err := s.db.QueryRow(ctx, `
		select wi.run_id, wi.stage, wi.kind, r.goal, r.constraints, wi.context, wi.available_skills, wi.review_context
		from work_items wi
		join runs r on r.id = wi.run_id
		where wi.id = $1
	`, workItemID).Scan(&runID, &stage, &kind, &goal, &constraints, &contextB, &availableSkills, &reviewContextB); err != nil {
		return claimResponse{}, err
	}

	var stageContext map[string]any
	var skills []string
	var revCtx map[string]any
	if err := unmarshalJSONNullable(contextB, &stageContext); err != nil {
		return claimResponse{}, err
	}
	if err := unmarshalJSONNullable(availableSkills, &skills); err != nil {
		return claimResponse{}, err
	}
	if err := unmarshalJSONNullable(reviewContextB, &revCtx); err != nil {
		return claimResponse{}, err
	}
	stageContext, err := s.enrichStageContextForOffer(ctx, runID, stageContext, skills)
	if err != nil {
		return claimResponse{}, err
	}

	return claimResponse{
		WorkItemID:      workItemID.String(),
		RunID:           runID.String(),
		Stage:           strings.TrimSpace(stage),
		Kind:            strings.TrimSpace(kind),
		Status:          "claimed",
		Goal:            strings.TrimSpace(goal),
		Constraints:     strings.TrimSpace(constraints),
		StageContext:    stageContext,
		AvailableSkills: skills,
		ReviewContext:   revCtx,
		LeaseExpiresAt:  leaseExpiresAt.Format(time.RFC3339),
	}, nil
}

func (s server) handleGatewayClaimWorkItem(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work_item_id"})
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

	// Must be offered to this agent.
	var offered bool
	if err := tx.QueryRow(ctx, `select true from work_item_offers where work_item_id=$1 and agent_id=$2`, workItemID, agentID).Scan(&offered); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not offered"})
			return
		}
		logError(ctx, "gateway claim: offer check failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "offer check failed"})
		return
	}

	// Only allow claiming if currently offered.
	var status string
	if err := tx.QueryRow(ctx, `select status from work_items where id=$1 for update`, workItemID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "gateway claim: work item lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "work item lookup failed"})
		return
	}
	if status != "offered" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "not claimable"})
		return
	}

	expiresAt := time.Now().UTC().Add(time.Duration(s.workItemLeaseSeconds) * time.Second)
	if _, err := tx.Exec(ctx, `
		insert into work_item_leases (work_item_id, agent_id, lease_expires_at)
		values ($1, $2, $3)
	`, workItemID, agentID, expiresAt); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already claimed"})
		return
	}
	if _, err := tx.Exec(ctx, `update work_items set status='claimed', updated_at=now() where id=$1`, workItemID); err != nil {
		logError(ctx, "gateway claim: update work item failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "gateway claim: commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "agent", agentID, "work_item_claimed", map[string]any{"work_item_id": workItemID.String(), "lease_expires_at": expiresAt.Format(time.RFC3339)})
	resp, err := s.buildClaimResponse(ctx, workItemID, expiresAt)
	if err != nil {
		logError(ctx, "gateway claim: build response failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "response build failed"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s server) handleGatewayClaimNextWorkItem(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "gateway claim-next: db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var workItemID uuid.UUID
	err = tx.QueryRow(ctx, `
		select wi.id
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		where o.agent_id = $1
		  and wi.status = 'offered'
		order by wi.created_at asc
		limit 1
		for update of wi skip locked
	`, agentID).Scan(&workItemID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no offers"})
		return
	}
	if err != nil {
		logError(ctx, "gateway claim-next: select offer failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	expiresAt := time.Now().UTC().Add(time.Duration(s.workItemLeaseSeconds) * time.Second)
	if _, err := tx.Exec(ctx, `
		insert into work_item_leases (work_item_id, agent_id, lease_expires_at)
		values ($1, $2, $3)
	`, workItemID, agentID, expiresAt); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "already claimed"})
		return
	}
	if _, err := tx.Exec(ctx, `update work_items set status='claimed', updated_at=now() where id=$1`, workItemID); err != nil {
		logError(ctx, "gateway claim-next: update work item failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "gateway claim-next: commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "agent", agentID, "work_item_claimed", map[string]any{"work_item_id": workItemID.String(), "lease_expires_at": expiresAt.Format(time.RFC3339)})
	resp, err := s.buildClaimResponse(ctx, workItemID, expiresAt)
	if err != nil {
		logError(ctx, "gateway claim-next: build response failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "response build failed"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s server) handleGatewayCompleteWorkItem(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	workItemID, err := uuid.Parse(chi.URLParam(r, "workItemID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid work_item_id"})
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

	var leaseAgent uuid.UUID
	var leaseExpires time.Time
	err = tx.QueryRow(ctx, `
		select agent_id, lease_expires_at
		from work_item_leases
		where work_item_id = $1
	`, workItemID).Scan(&leaseAgent, &leaseExpires)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "not leased"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lease lookup failed"})
		return
	}
	if leaseAgent != agentID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not lease holder"})
		return
	}
	if time.Now().UTC().After(leaseExpires) {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "lease expired"})
		return
	}

	if _, err := tx.Exec(ctx, `update work_items set status='completed', updated_at=now() where id=$1`, workItemID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if _, err := tx.Exec(ctx, `delete from work_item_leases where work_item_id=$1`, workItemID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "lease delete failed"})
		return
	}

	// Update owner aggregated contribution counter.
	var ownerID uuid.UUID
	if err := tx.QueryRow(ctx, `select owner_id from agents where id=$1`, agentID).Scan(&ownerID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "owner lookup failed"})
		return
	}
	if _, err := tx.Exec(ctx, `
		insert into owner_contributions (owner_id, completed_work_items, updated_at)
		values ($1, 1, now())
		on conflict (owner_id) do update
		set completed_work_items = owner_contributions.completed_work_items + 1,
		    updated_at = now()
	`, ownerID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "contribution update failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}
	s.audit(ctx, "agent", agentID, "work_item_completed", map[string]any{"work_item_id": workItemID.String(), "owner_id": ownerID.String()})
	writeJSON(w, http.StatusOK, map[string]string{"status": "completed"})
}

func shuffleUUIDs(ctx context.Context, ids []uuid.UUID) {
	for i := len(ids) - 1; i > 0; i-- {
		nBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			logError(ctx, "shuffleUUIDs rand failed", err)
			return
		}
		j := int(nBig.Int64())
		ids[i], ids[j] = ids[j], ids[i]
	}
}

type emitEventRequest struct {
	Kind    string         `json:"kind"`
	Payload map[string]any `json:"payload"`
}

func (s server) handleGatewayEmitEvent(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	runID, err := uuid.Parse(chi.URLParam(r, "runID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run id"})
		return
	}

	var req emitEventRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	req.Kind = strings.TrimSpace(req.Kind)
	if _, ok := allowedEventKinds[req.Kind]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid kind"})
		return
	}
	if req.Payload == nil {
		req.Payload = map[string]any{}
	}
	// Basic payload size guardrail.
	payloadJSON, err := json.Marshal(req.Payload)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
		return
	}
	if len(payloadJSON) > 16*1024 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "payload too large"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Agent must be a participant: it must have been offered a work item in this run.
	var participant bool
	err = s.db.QueryRow(ctx, `
		select true
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		where o.agent_id = $1 and wi.run_id = $2
		limit 1
	`, agentID, runID).Scan(&participant)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a participant"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "participant check failed"})
		return
	}

	persona, err := s.personaForAgentInRun(ctx, runID, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "persona lookup failed"})
		return
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	// Lock run row to serialize seq allocation per run.
	if _, err := tx.Exec(ctx, `select 1 from runs where id=$1 for update`, runID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "run lock failed"})
		return
	}

	var nextSeq int64
	if err := tx.QueryRow(ctx, `select coalesce(max(seq), 0) + 1 from events where run_id=$1`, runID).Scan(&nextSeq); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "seq allocation failed"})
		return
	}
	isKey := isKeyNodeKind(req.Kind)

	createdAt := time.Now().UTC()
	if _, err := tx.Exec(ctx, `
		insert into events (run_id, seq, kind, persona, payload, is_key_node, created_at)
		values ($1, $2, $3, $4, $5, $6, $7)
	`, runID, nextSeq, req.Kind, persona, payloadJSON, isKey, createdAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	var payloadMap map[string]any
	if err := unmarshalJSONNullable(payloadJSON, &payloadMap); err != nil {
		logError(ctx, "unmarshal emitted payload failed", err)
		payloadMap = map[string]any{"_decode_error": true}
	}
	ev := eventDTO{
		RunID:     runID.String(),
		Seq:       nextSeq,
		Kind:      req.Kind,
		Persona:   persona,
		Payload:   payloadMap,
		IsKeyNode: isKey,
		CreatedAt: createdAt.Format(time.RFC3339),
	}
	s.br.publish(runID, ev)
	writeJSON(w, http.StatusCreated, ev)

	s.audit(ctx, "agent", agentID, "event_emitted", map[string]any{"run_id": runID.String(), "seq": nextSeq, "kind": req.Kind, "is_key_node": isKey})
}

func (s server) personaForAgent(ctx context.Context, agentID uuid.UUID) (string, error) {
	// Public persona is derived from tags (not identity), and must not expose owner.
	rows, err := s.db.Query(ctx, `select tag from agent_tags where agent_id=$1 order by tag asc`, agentID)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return "", err
		}
		t = strings.TrimSpace(t)
		if t != "" {
			tags = append(tags, t)
		}
		if len(tags) >= 2 {
			break
		}
	}
	if len(tags) == 0 {
		return "智能体", nil
	}
	return strings.Join(tags, " / "), nil
}

func (s server) personaForAgentInRun(ctx context.Context, runID uuid.UUID, agentID uuid.UUID) (string, error) {
	base, err := s.personaForAgent(ctx, agentID)
	if err != nil {
		return "", err
	}

	// Prefer the owner-provided agent display name, so viewers can distinguish participants.
	var name string
	if err := s.db.QueryRow(ctx, `select name from agents where id=$1`, agentID).Scan(&name); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		logError(ctx, "agent name lookup failed", err)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return base, nil
	}
	return name, nil
}

type invokeToolRequest struct {
	RunID string         `json:"run_id"`
	Tool  string         `json:"tool"`
	Input map[string]any `json:"input"`
}

func (s server) handleGatewayInvokeTool(w http.ResponseWriter, r *http.Request) {
	agentID, ok := agentIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req invokeToolRequest
	if !readJSONLimited(w, r, &req, 64*1024) {
		return
	}
	req.Tool = strings.TrimSpace(req.Tool)
	if req.Tool == "" || len(req.Tool) > 128 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tool"})
		return
	}
	runID, err := uuid.Parse(strings.TrimSpace(req.RunID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid run_id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	allowed, err := s.isToolAllowed(ctx, agentID, runID, req.Tool)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "policy check failed"})
		return
	}
	if !allowed {
		s.audit(ctx, "agent", agentID, "tool_denied", map[string]any{"run_id": runID.String(), "tool": req.Tool})
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "tool_denied"})
		return
	}

	s.audit(ctx, "agent", agentID, "tool_allowed_but_not_implemented", map[string]any{"run_id": runID.String(), "tool": req.Tool})
	writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "tool_not_implemented"})
}

func (s server) isToolAllowed(ctx context.Context, agentID uuid.UUID, runID uuid.UUID, tool string) (bool, error) {
	// Default deny: tool must be explicitly allowed for both agent and run.
	var agentAllowed bool
	if err := s.db.QueryRow(ctx, `select exists(select 1 from agent_allowed_tools where agent_id=$1 and tool=$2)`, agentID, tool).Scan(&agentAllowed); err != nil {
		return false, err
	}
	if !agentAllowed {
		return false, nil
	}
	var runAllowed bool
	if err := s.db.QueryRow(ctx, `select exists(select 1 from run_allowed_tools where run_id=$1 and tool=$2)`, runID, tool).Scan(&runAllowed); err != nil {
		return false, err
	}
	return runAllowed, nil
}
