package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	preReviewEvaluationTTL       = 7 * 24 * time.Hour
	preReviewEvaluationMaxPerDay = 20
)

type adminEvaluationJudgeDTO struct {
	AgentID        string `json:"agent_id"`
	Name           string `json:"name"`
	Enabled        bool   `json:"enabled"`
	Status         string `json:"status"`
	AdmittedStatus string `json:"admitted_status"`
}

func (s server) handleAdminListEvaluationJudges(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select j.agent_id, a.name, j.enabled, a.status, a.admitted_status
		from evaluation_judge_agents j
		join agents a on a.id = j.agent_id
		order by j.enabled desc, a.updated_at desc
	`)
	if err != nil {
		logError(ctx, "admin list evaluation judges: query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]adminEvaluationJudgeDTO, 0)
	for rows.Next() {
		var (
			agentID        uuid.UUID
			name           string
			enabled        bool
			status         string
			admittedStatus string
		)
		if err := rows.Scan(&agentID, &name, &enabled, &status, &admittedStatus); err != nil {
			logError(ctx, "admin list evaluation judges: scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		out = append(out, adminEvaluationJudgeDTO{
			AgentID:        agentID.String(),
			Name:           strings.TrimSpace(name),
			Enabled:        enabled,
			Status:         strings.TrimSpace(status),
			AdmittedStatus: strings.TrimSpace(admittedStatus),
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "admin list evaluation judges: iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type adminSetEvaluationJudgesRequest struct {
	AgentIDs []string `json:"agent_ids"`
}

func (s server) handleAdminSetEvaluationJudges(w http.ResponseWriter, r *http.Request) {
	userID, ok := userIDFromCtx(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req adminSetEvaluationJudgesRequest
	if !readJSONLimited(w, r, &req, 32*1024) {
		return
	}

	if len(req.AgentIDs) > 50 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many agent ids"})
		return
	}

	seen := map[uuid.UUID]bool{}
	agentIDs := make([]uuid.UUID, 0, len(req.AgentIDs))
	for _, raw := range req.AgentIDs {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid agent id"})
			return
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		agentIDs = append(agentIDs, id)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "admin set evaluation judges: db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	// Validate agent existence up-front.
	for _, id := range agentIDs {
		var ok bool
		if err := tx.QueryRow(ctx, `select true from agents where id=$1`, id).Scan(&ok); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent not found"})
				return
			}
			logError(ctx, "admin set evaluation judges: agent lookup failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
			return
		}
	}

	// Replace-set semantics:
	// - ensure provided ids are enabled (upsert)
	// - remove any others
	if len(agentIDs) == 0 {
		if _, err := tx.Exec(ctx, `delete from evaluation_judge_agents`); err != nil {
			logError(ctx, "admin set evaluation judges: delete all failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
			return
		}
	} else {
		if _, err := tx.Exec(ctx, `delete from evaluation_judge_agents where agent_id <> all($1)`, agentIDs); err != nil {
			logError(ctx, "admin set evaluation judges: delete others failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
			return
		}
		for _, id := range agentIDs {
			if _, err := tx.Exec(ctx, `
				insert into evaluation_judge_agents (agent_id, enabled)
				values ($1, true)
				on conflict (agent_id) do update set enabled = true, updated_at = now()
			`, id); err != nil {
				logError(ctx, "admin set evaluation judges: upsert failed", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
				return
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "admin set evaluation judges: commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "admin", userID, "evaluation_judges_set", map[string]any{"agent_ids": req.AgentIDs})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

type createPreReviewEvaluationRequest struct {
	Topic string `json:"topic"`
}

type preReviewEvaluationDTO struct {
	EvaluationID string `json:"evaluation_id"`
	AgentID      string `json:"agent_id"`
	RunID        string `json:"run_id"`
	Topic        string `json:"topic"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
	ExpiresAt    string `json:"expires_at"`
}

func (s server) listActiveEvaluationJudgeAgents(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := s.db.Query(ctx, `
		select a.id
		from evaluation_judge_agents j
		join agents a on a.id = j.agent_id
		where j.enabled = true
		  and a.status = 'enabled'
		  and a.admitted_status = 'admitted'
		order by a.updated_at desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]uuid.UUID, 0, 8)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s server) handleOwnerCreatePreReviewEvaluation(w http.ResponseWriter, r *http.Request) {
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
	var req createPreReviewEvaluationRequest
	if !readJSONLimited(w, r, &req, 16*1024) {
		return
	}
	req.Topic = strings.TrimSpace(req.Topic)
	if len(req.Topic) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "topic too long"})
		return
	}
	if req.Topic == "" {
		req.Topic = "随机话题"
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	if err := s.requireOwnerAgent(ctx, userID, agentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "create pre-review evaluation: owner check failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	// Basic rate limit: per-owner per-agent per-day.
	startOfDayUTC := time.Now().UTC().Truncate(24 * time.Hour)
	var cnt int
	if err := s.db.QueryRow(ctx, `
		select count(*)
		from agent_pre_review_evaluations
		where owner_id = $1 and agent_id = $2 and created_at >= $3
	`, userID, agentID, startOfDayUTC).Scan(&cnt); err != nil {
		logError(ctx, "create pre-review evaluation: rate limit query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if cnt >= preReviewEvaluationMaxPerDay {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "evaluation limit reached"})
		return
	}

	judgeCtx, cancelJudges := context.WithTimeout(ctx, 5*time.Second)
	judgeIDs, err := s.listActiveEvaluationJudgeAgents(judgeCtx)
	cancelJudges()
	if err != nil {
		logError(ctx, "create pre-review evaluation: list judges failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	if len(judgeIDs) == 0 {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "no evaluation judges configured"})
		return
	}

	// Snapshot candidate card basics (best-effort: keep JSON small).
	var (
		name         string
		description  string
		promptView   string
		personality  []byte
		interests    []byte
		capabilities []byte
		bio          string
		greeting     string
		persona      []byte
		cardVersion  int
		cardReview   string
	)
	if err := s.db.QueryRow(ctx, `
		select name, description, prompt_view, personality, interests, capabilities, bio, greeting, coalesce(persona, '{}'::jsonb), card_version, card_review_status
		from agents
		where id = $1 and owner_id = $2
	`, agentID, userID).Scan(&name, &description, &promptView, &personality, &interests, &capabilities, &bio, &greeting, &persona, &cardVersion, &cardReview); err != nil {
		logError(ctx, "create pre-review evaluation: snapshot query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	var (
		personalityObj  map[string]any
		interestsArr    []string
		capabilitiesArr []string
		personaObj      map[string]any
	)
	if err := unmarshalJSONNullable(personality, &personalityObj); err != nil {
		logError(ctx, "create pre-review evaluation: personality decode failed", err)
		personalityObj = map[string]any{}
	}
	if err := unmarshalJSONNullable(interests, &interestsArr); err != nil {
		logError(ctx, "create pre-review evaluation: interests decode failed", err)
		interestsArr = []string{}
	}
	if err := unmarshalJSONNullable(capabilities, &capabilitiesArr); err != nil {
		logError(ctx, "create pre-review evaluation: capabilities decode failed", err)
		capabilitiesArr = []string{}
	}
	if err := unmarshalJSONNullable(persona, &personaObj); err != nil {
		logError(ctx, "create pre-review evaluation: persona decode failed", err)
		personaObj = map[string]any{}
	}

	now := time.Now().UTC()
	expiresAt := now.Add(preReviewEvaluationTTL)

	runGoal := "提审前测评：" + strings.TrimSpace(name)
	if runGoal == "提审前测评：" {
		runGoal = "提审前测评"
	}
	runConstraints := strings.TrimSpace(`
你是“测评智能体（裁判）”。请基于候选智能体的设定，完成一次“提审前测评”并给出可执行的修改建议。

要求：
1) 先以“候选智能体”的身份，针对话题给出一段真实可交付的回复（这是主人想看到的效果）。
2) 再以“测评智能体”的身份，指出优点/风险点/不符合平台规范的地方，并给出可执行的修改建议（可列清单）。
3) 禁止冒充真实世界的在世名人/具体身份；只能做“表达风格参考”，不得自称为真实人物。
4) 输出用中文，避免无意义的 UUID/英文噪音。
5) 输出格式：Markdown，包含两个标题：## 候选智能体回复、## 测评与建议
`)

	// Create an unlisted run offered only to judge agents.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "create pre-review evaluation: db begin failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db begin failed"})
		return
	}
	defer tx.Rollback(ctx)

	var runID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into runs (publisher_user_id, goal, constraints, status, review_status, is_public)
		values ($1, $2, $3, 'created', 'pending', false)
		returning id
	`, userID, runGoal, runConstraints).Scan(&runID); err != nil {
		logError(ctx, "create pre-review evaluation: create run failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create run failed"})
		return
	}

	skills := s.skillsGatewayWhitelist
	if skills == nil {
		skills = []string{}
	}
	stageContext := s.stageContextForStage("review", skills)
	stageContext["pre_review_evaluation"] = map[string]any{
		"topic": req.Topic,
		"candidate_agent": map[string]any{
			"agent_id":           agentID.String(),
			"name":               strings.TrimSpace(name),
			"description":        strings.TrimSpace(description),
			"prompt_view":        strings.TrimSpace(promptView),
			"personality":        personalityObj,
			"interests":          interestsArr,
			"capabilities":       capabilitiesArr,
			"bio":                strings.TrimSpace(bio),
			"greeting":           strings.TrimSpace(greeting),
			"persona":            personaObj,
			"card_version":       cardVersion,
			"card_review_status": strings.TrimSpace(cardReview),
		},
		"output_rules": map[string]any{
			"language":         "zh",
			"no_uuid":          true,
			"no_english_noise": true,
		},
	}

	availableSkillsJSON, err := json.Marshal(skills)
	if err != nil {
		logError(ctx, "create pre-review evaluation: marshal available_skills failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}
	stageContextJSON, err := json.Marshal(stageContext)
	if err != nil {
		logError(ctx, "create pre-review evaluation: marshal stage_context failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encode failed"})
		return
	}

	var workItemID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into work_items (run_id, stage, kind, status, context, available_skills)
		values ($1, 'review', 'draft', 'offered', $2, $3)
		returning id
	`, runID, stageContextJSON, availableSkillsJSON).Scan(&workItemID); err != nil {
		logError(ctx, "create pre-review evaluation: create work item failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create work item failed"})
		return
	}

	for _, jid := range judgeIDs {
		if _, err := tx.Exec(ctx, `
			insert into work_item_offers (work_item_id, agent_id) values ($1, $2)
			on conflict do nothing
		`, workItemID, jid); err != nil {
			logError(ctx, "create pre-review evaluation: create offers failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create offers failed"})
			return
		}
	}

	var evaluationID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into agent_pre_review_evaluations (owner_id, agent_id, run_id, topic, expires_at)
		values ($1, $2, $3, $4, $5)
		returning id
	`, userID, agentID, runID, req.Topic, expiresAt).Scan(&evaluationID); err != nil {
		logError(ctx, "create pre-review evaluation: insert evaluation failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "create pre-review evaluation: commit failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "commit failed"})
		return
	}

	s.audit(ctx, "user", userID, "pre_review_evaluation_created", map[string]any{
		"agent_id":      agentID.String(),
		"evaluation_id": evaluationID.String(),
		"run_id":        runID.String(),
		"judge_agent_ids": func() []string {
			out := make([]string, 0, len(judgeIDs))
			for _, id := range judgeIDs {
				out = append(out, id.String())
			}
			return out
		}(),
	})

	writeJSON(w, http.StatusCreated, map[string]any{
		"evaluation_id": evaluationID.String(),
		"run_id":        runID.String(),
		"expires_at":    expiresAt.Format(time.RFC3339),
	})
}

func (s server) handleOwnerListPreReviewEvaluations(w http.ResponseWriter, r *http.Request) {
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

	limit := clampInt(int64Query(r, "limit", 20), 1, 100)

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := s.requireOwnerAgent(ctx, userID, agentID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		logError(ctx, "list pre-review evaluations: owner check failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	rows, err := s.db.Query(ctx, `
		select e.id, e.run_id, e.topic, r.status, e.created_at, e.expires_at
		from agent_pre_review_evaluations e
		join runs r on r.id = e.run_id
		where e.owner_id = $1 and e.agent_id = $2
		order by e.created_at desc
		limit $3
	`, userID, agentID, limit)
	if err != nil {
		logError(ctx, "list pre-review evaluations: query failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	out := make([]preReviewEvaluationDTO, 0, limit)
	for rows.Next() {
		var (
			evalID    uuid.UUID
			runID     uuid.UUID
			topic     string
			status    string
			createdAt time.Time
			expiresAt time.Time
		)
		if err := rows.Scan(&evalID, &runID, &topic, &status, &createdAt, &expiresAt); err != nil {
			logError(ctx, "list pre-review evaluations: scan failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		out = append(out, preReviewEvaluationDTO{
			EvaluationID: evalID.String(),
			AgentID:      agentID.String(),
			RunID:        runID.String(),
			Topic:        strings.TrimSpace(topic),
			Status:       strings.TrimSpace(status),
			CreatedAt:    createdAt.UTC().Format(time.RFC3339),
			ExpiresAt:    expiresAt.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "list pre-review evaluations: iterate failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (s server) handleOwnerDeletePreReviewEvaluation(w http.ResponseWriter, r *http.Request) {
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
	evaluationID, err := uuid.Parse(chi.URLParam(r, "evaluationID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid evaluation id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var runID uuid.UUID
	err = s.db.QueryRow(ctx, `
		select e.run_id
		from agent_pre_review_evaluations e
		join runs r on r.id = e.run_id
		where e.id = $1
		  and e.owner_id = $2
		  and e.agent_id = $3
		  and r.publisher_user_id = e.owner_id
	`, evaluationID, userID, agentID).Scan(&runID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if err != nil {
		logError(ctx, "delete pre-review evaluation: lookup failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}

	ct, err := s.db.Exec(ctx, `delete from runs where id = $1 and publisher_user_id = $2`, runID, userID)
	if err != nil {
		logError(ctx, "delete pre-review evaluation: delete run failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	s.audit(ctx, "user", userID, "pre_review_evaluation_deleted", map[string]any{
		"agent_id":      agentID.String(),
		"evaluation_id": evaluationID.String(),
		"run_id":        runID.String(),
	})
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s server) cleanupExpiredPreReviewEvaluations(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.db.Exec(ctx, `
		with doomed as (
			select run_id
			from agent_pre_review_evaluations
			where expires_at <= now()
			limit 200
		)
		delete from runs
		where id in (select run_id from doomed)
	`)
	if err != nil {
		logError(ctx, "cleanup expired pre-review evaluations failed", err)
	}
}
