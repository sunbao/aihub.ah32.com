package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type taskgenCheckinArtifact struct {
	Proposal any `json:"proposal"`
}

type taskgenProposalTask struct {
	Type            string   `json:"type"`
	Title           string   `json:"title"`
	Summary         string   `json:"summary,omitempty"`
	Visibility      string   `json:"visibility,omitempty"` // public|unlisted
	Tags            []string `json:"tags,omitempty"`       // used as run required_tags
	TimeboxHours    int      `json:"timebox_hours,omitempty"`
	ExpectedOutputs any      `json:"expected_outputs,omitempty"`
}

func (s server) maybeTaskgenFromFinalArtifact(agentID uuid.UUID, runID uuid.UUID, artifactID uuid.UUID, content string) {
	if len(s.taskGenActorTags) == 0 || s.taskGenDailyLimitPerAgent <= 0 {
		return
	}
	// Cheap guardrail: taskgen only triggers on JSON-shaped content.
	c := strings.TrimSpace(content)
	if c == "" || (!strings.HasPrefix(c, "{") && !strings.HasPrefix(c, "[")) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		logError(ctx, "taskgen: db begin failed", err)
		return
	}
	defer tx.Rollback(ctx)

	// Determine whether the submitting agent is completing a claimed checkin-stage item.
	var (
		stage    string
		ownerID  uuid.UUID
		agentRef string
	)
	err = tx.QueryRow(ctx, `
		select wi.stage, a.owner_id, a.public_ref
		from work_item_leases l
		join work_items wi on wi.id = l.work_item_id
		join agents a on a.id = l.agent_id
		where l.agent_id = $1
		  and wi.run_id = $2
		  and wi.status = 'claimed'
		limit 1
	`, agentID, runID).Scan(&stage, &ownerID, &agentRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		logError(ctx, "taskgen: lookup stage/owner failed", err)
		return
	}
	if strings.TrimSpace(stage) != "checkin" {
		return
	}

	// Idempotency: create a processing row first. If it already exists, we're done.
	proposalObj := map[string]any{}
	if err := json.Unmarshal([]byte(c), &proposalObj); err != nil {
		// Record the decode error once so we don't keep retrying on the same artifact.
		payloadJSON, mErr := marshalJSONB(map[string]any{"error": "invalid_json"})
		if mErr != nil {
			logError(ctx, "taskgen: marshal invalid_json payload failed", mErr)
			return
		}
		if _, insErr := tx.Exec(ctx, `
			insert into agent_taskgen_runs (
				source_artifact_id, source_run_id, proposer_agent_id, owner_id,
				proposal_type, proposal, outcome, reason_code
			) values ($1, $2, $3, $4, $5, $6, 'rejected', 'invalid_json')
			on conflict (source_artifact_id) do nothing
		`, artifactID, runID, agentID, ownerID, "unknown", payloadJSON); insErr != nil {
			logError(ctx, "taskgen: insert invalid_json audit failed", insErr)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit invalid_json audit failed", err)
		}
		return
	}

	proposalJSON, err := marshalJSONB(proposalObj)
	if err != nil {
		logError(ctx, "taskgen: marshal proposal failed", err)
		return
	}

	ct, err := tx.Exec(ctx, `
		insert into agent_taskgen_runs (
			source_artifact_id, source_run_id, proposer_agent_id, owner_id,
			proposal_type, proposal, outcome, reason_code
		) values ($1, $2, $3, $4, $5, $6, 'processing', 'processing')
		on conflict (source_artifact_id) do nothing
	`, artifactID, runID, agentID, ownerID, "unknown", proposalJSON)
	if err != nil {
		logError(ctx, "taskgen: insert processing row failed", err)
		return
	}
	if ct.RowsAffected() == 0 {
		// Already processed.
		return
	}

	// Actor allowlist: agent must carry at least one configured tag.
	tags, err := s.listAgentTags(ctx, tx, agentID, 200)
	if err != nil {
		logError(ctx, "taskgen: list agent tags failed", err)
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "error", "tag_lookup_failed", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}
	allowed := false
	allowedSet := map[string]struct{}{}
	for _, t := range s.taskGenActorTags {
		allowedSet[strings.TrimSpace(t)] = struct{}{}
	}
	for _, t := range tags {
		if _, ok := allowedSet[t]; ok {
			allowed = true
			break
		}
	}
	if !allowed {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "not_eligible", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}

	// Extract "proposal" from the checkin artifact payload.
	var env taskgenCheckinArtifact
	if err := json.Unmarshal([]byte(c), &env); err != nil {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "invalid_schema", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}
	if env.Proposal == nil {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "no_proposal", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}

	propB, err := marshalJSONB(env.Proposal)
	if err != nil {
		logError(ctx, "taskgen: marshal proposal field failed", err)
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "error", "proposal_encode_failed", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}
	var prop taskgenProposalTask
	if err := json.Unmarshal(propB, &prop); err != nil {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "invalid_schema", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}
	if strings.TrimSpace(prop.Type) != "propose_task" {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "unsupported_proposal_type", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}
	title := strings.TrimSpace(prop.Title)
	if title == "" || len(title) > 200 {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "invalid_title", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}
	summary := strings.TrimSpace(prop.Summary)
	if len(summary) > 4000 {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "summary_too_long", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}

	// Quota: accepted per agent per day (Asia/Shanghai boundary).
	dayStart := shanghaiDayStart(time.Now().UTC())
	var acceptedToday int
	if err := tx.QueryRow(ctx, `
		select count(1)
		from agent_taskgen_runs
		where proposer_agent_id = $1
		  and outcome = 'accepted'
		  and created_at >= $2
	`, agentID, dayStart).Scan(&acceptedToday); err != nil {
		logError(ctx, "taskgen: quota query failed", err)
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "error", "quota_query_failed", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}
	if acceptedToday >= s.taskGenDailyLimitPerAgent {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "quota_exceeded", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}

	// Required tags: use proposal.tags, filtered by allowed prefixes (case-insensitive).
	required := normalizeTags(prop.Tags)
	required = filterTagsByPrefixes(required, s.taskGenAllowedTagPrefixes)
	if len(required) == 0 {
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "rejected", "missing_required_tags", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}
	// Always include a stable marker tag for audit/debug.
	required = normalizeTags(append(required, "taskgen", "taskgen-from-"+safeTagSuffix(agentRef)))

	visibility := strings.ToLower(strings.TrimSpace(prop.Visibility))
	isPublic := true
	if visibility == "unlisted" {
		isPublic = false
	}

	constraints := buildTaskgenConstraints(summary, prop.TimeboxHours, prop.ExpectedOutputs, agentRef)

	runID2, runRef2, workItemID, err := s.createRunInTx(ctx, tx, ownerID, title, constraints, required, nil, isPublic)
	if err != nil {
		logError(ctx, "taskgen: create run failed", err)
		if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "error", "create_run_failed", nil, ""); err != nil {
			logError(ctx, "taskgen: update outcome failed", err)
		}
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}

	if err := s.updateTaskgenOutcome(ctx, tx, artifactID, "accepted", "accepted", &runID2, runRef2); err != nil {
		logError(ctx, "taskgen: update outcome failed", err)
		if err := tx.Commit(ctx); err != nil {
			logError(ctx, "taskgen: commit failed", err)
		}
		return
	}

	if err := tx.Commit(ctx); err != nil {
		logError(ctx, "taskgen: commit failed", err)
		return
	}

	s.audit(ctx, "agent", agentID, "taskgen_run_created", map[string]any{
		"source_run_id":        runID.String(),
		"source_artifact_id":   artifactID.String(),
		"created_run_id":       runID2.String(),
		"created_run_ref":      runRef2,
		"initial_work_item_id": workItemID.String(),
	})
}

func (s server) listAgentTags(ctx context.Context, tx pgx.Tx, agentID uuid.UUID, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := tx.Query(ctx, `select tag from agent_tags where agent_id = $1 order by tag asc limit $2`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, strings.TrimSpace(t))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s server) updateTaskgenOutcome(ctx context.Context, tx pgx.Tx, artifactID uuid.UUID, outcome string, reason string, createdRunID *uuid.UUID, createdRunRef string) error {
	var runID any
	if createdRunID != nil && *createdRunID != uuid.Nil {
		runID = *createdRunID
	} else {
		runID = nil
		createdRunRef = ""
	}
	_, err := tx.Exec(ctx, `
		update agent_taskgen_runs
		set outcome = $2,
		    reason_code = $3,
		    created_run_id = $4,
		    created_run_ref = $5
		where source_artifact_id = $1
	`, artifactID, strings.TrimSpace(outcome), strings.TrimSpace(reason), runID, strings.TrimSpace(createdRunRef))
	return err
}

func filterTagsByPrefixes(tags []string, prefixes []string) []string {
	if len(prefixes) == 0 {
		return tags
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		lt := strings.ToLower(strings.TrimSpace(t))
		if lt == "" {
			continue
		}
		for _, p := range prefixes {
			pp := strings.ToLower(strings.TrimSpace(p))
			if pp == "" {
				continue
			}
			if strings.HasPrefix(lt, pp) {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

func safeTagSuffix(agentRef string) string {
	// Keep this stable + safe for tag usage: "a_xxx" -> "a_xxx".
	s := strings.TrimSpace(agentRef)
	if s == "" {
		return "unknown"
	}
	if len(s) > 32 {
		s = s[:32]
	}
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '_' || r == '-':
			return r
		default:
			return '-'
		}
	}, s)
	return strings.Trim(s, "-")
}

func buildTaskgenConstraints(summary string, timeboxHours int, expectedOutputs any, proposerAgentRef string) string {
	var sb strings.Builder
	if summary != "" {
		sb.WriteString("摘要：")
		sb.WriteString(summary)
		sb.WriteString("\n\n")
	}
	if timeboxHours > 0 {
		sb.WriteString("时间盒：")
		sb.WriteString(strconvItoaClamp(timeboxHours, 1, 168))
		sb.WriteString(" 小时\n\n")
	}
	if expectedOutputs != nil {
		if b, err := json.Marshal(expectedOutputs); err == nil && len(b) > 0 && len(b) <= 8000 {
			sb.WriteString("期望输出（结构化）：\n")
			sb.WriteString(string(b))
			sb.WriteString("\n\n")
		}
	}
	if strings.TrimSpace(proposerAgentRef) != "" {
		sb.WriteString("提议来源：")
		sb.WriteString(strings.TrimSpace(proposerAgentRef))
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func shanghaiDayStart(nowUTC time.Time) time.Time {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return nowUTC.Truncate(24 * time.Hour)
	}
	t := nowUTC.In(loc)
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc).UTC()
}

func strconvItoaClamp(v int, min int, max int) string {
	if v < min {
		v = min
	}
	if v > max {
		v = max
	}
	return strconv.Itoa(v)
}
