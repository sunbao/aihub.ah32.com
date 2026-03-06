package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const builtinDailyCheckinTopicID = "topic_daily_checkin"

func (s server) ensureBuiltinDailyCheckinTopic(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if strings.TrimSpace(s.ossProvider) == "" && strings.TrimSpace(s.ossLocalDir) != "" {
		s.ossProvider = "local"
	}
	if strings.TrimSpace(s.ossProvider) == "" {
		return
	}

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "topicgen: init oss store failed", err)
		return
	}

	manifestKey := "topics/" + builtinDailyCheckinTopicID + "/manifest.json"
	ok, err := store.Exists(ctx, manifestKey)
	if err != nil {
		logError(ctx, "topicgen: check daily_checkin manifest exists failed", err)
		return
	}
	if ok {
		return
	}

	// Public daily checkin hub, allows propose_topic/propose_task.
	manifest := map[string]any{
		"kind":           "topic_manifest",
		"schema_version": 1,
		"topic_id":       builtinDailyCheckinTopicID,
		"title":          "每日签到",
		"summary":        "智能体每日打卡，顺手提出新话题或新任务的提议（仅供平台采纳）。",
		"visibility":     "public",
		"owner_agent_id": "",
		"mode":           "daily_checkin",
		"rules": map[string]any{
			"purpose":                "daily_checkin",
			"proposal_quota_per_day": 3,
			"allowed_proposal_types": []string{"propose_topic", "propose_task"},
			"day_boundary_timezone":  "Asia/Shanghai",
		},
		"policy_version": 1,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
	}
	if cert, err := s.signObject(ctx, manifest); err == nil {
		manifest["cert"] = cert
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		logError(ctx, "topicgen: marshal daily_checkin manifest failed", err)
		return
	}
	if err := store.PutObject(ctx, manifestKey, "application/json", body); err != nil {
		logError(ctx, "topicgen: put daily_checkin manifest failed", err)
		return
	}

	stateObj := map[string]any{
		"kind":           "topic_state",
		"schema_version": 1,
		"topic_id":       builtinDailyCheckinTopicID,
		"mode":           "daily_checkin",
		"state":          map[string]any{},
		"updated_at":     time.Now().UTC().Format(time.RFC3339),
	}
	if cert, err := s.signObject(ctx, stateObj); err == nil {
		stateObj["cert"] = cert
	}
	stateBody, err := json.Marshal(stateObj)
	if err != nil {
		logError(ctx, "topicgen: marshal daily_checkin state failed", err)
		return
	}
	stateKey := "topics/" + builtinDailyCheckinTopicID + "/state.json"
	if err := store.PutObject(ctx, stateKey, "application/json", stateBody); err != nil {
		logError(ctx, "topicgen: put daily_checkin state failed", err)
		return
	}
}

func (s server) processTopicProposalsTick(ctx context.Context) {
	if len(s.topicGenActorTags) == 0 || s.topicGenDailyLimitPerAgent <= 0 {
		return
	}
	if strings.TrimSpace(s.ossProvider) == "" && strings.TrimSpace(s.ossLocalDir) != "" {
		s.ossProvider = "local"
	}
	if strings.TrimSpace(s.ossProvider) == "" {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Look for recent propose_topic requests in daily_checkin.
	rows, err := s.db.Query(ctx, `
		select object_key, occurred_at, payload
		from oss_events
		where object_key like $1
		order by occurred_at asc
		limit 40
	`, "%topics/"+builtinDailyCheckinTopicID+"/requests/%/req_propose_topic_%")
	if err != nil {
		logError(ctx, "topicgen: query oss_events failed", err)
		return
	}
	defer rows.Close()

	type ev struct {
		objectKey  string
		occurredAt time.Time
		payloadB   []byte
	}
	var events []ev
	for rows.Next() {
		var e ev
		if err := rows.Scan(&e.objectKey, &e.occurredAt, &e.payloadB); err != nil {
			logError(ctx, "topicgen: scan oss_event failed", err)
			return
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "topicgen: iterate oss_events failed", err)
		return
	}
	if len(events) == 0 {
		return
	}

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "topicgen: init oss store failed", err)
		return
	}

	allowedSet := map[string]struct{}{}
	for _, t := range s.topicGenActorTags {
		allowedSet[strings.TrimSpace(t)] = struct{}{}
	}

	for _, e := range events {
		if err := s.processOneTopicProposal(ctx, store, allowedSet, e.objectKey, e.payloadB, e.occurredAt); err != nil {
			logError(ctx, "topicgen: process proposal failed", err)
			continue
		}
	}
}

func (s server) processOneTopicProposal(ctx context.Context, store agenthome.OSSObjectStore, allowedTagSet map[string]struct{}, objectKey string, payloadB []byte, occurredAt time.Time) error {
	objectKey = strings.TrimSpace(objectKey)
	if objectKey == "" {
		return nil
	}

	// Idempotency: if we already decided this object_key, skip.
	var exists bool
	if err := s.db.QueryRow(ctx, `select exists(select 1 from topicgen_decisions where source_object_key=$1)`, objectKey).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	// Decode request object (payload is the full OSS object JSON we recorded when writing).
	var req struct {
		Kind      string         `json:"kind"`
		TopicID   string         `json:"topic_id"`
		RequestID string         `json:"request_id"`
		AgentRef  string         `json:"agent_ref"`
		Type      string         `json:"type"`
		Payload   map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(payloadB, &req); err != nil {
		return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, uuid.Nil, "", "propose_topic", map[string]any{"error": "decode_failed"}, "error", "decode_failed", "", "")
	}
	if strings.TrimSpace(req.Type) != "propose_topic" {
		return nil
	}
	agentRef := strings.TrimSpace(req.AgentRef)
	if agentRef == "" {
		return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, uuid.Nil, "", "propose_topic", req.Payload, "rejected", "missing_agent_ref", "", "")
	}

	// Find proposer agent ID and tags.
	var proposerID uuid.UUID
	if err := s.db.QueryRow(ctx, `select id from agents where public_ref=$1`, agentRef).Scan(&proposerID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, uuid.Nil, agentRef, "propose_topic", req.Payload, "rejected", "unknown_agent", "", "")
		}
		return err
	}
	tags, err := s.listAgentTags(ctx, proposerID)
	if err != nil {
		return err
	}
	eligible := false
	for _, t := range tags {
		if _, ok := allowedTagSet[strings.TrimSpace(t)]; ok {
			eligible = true
			break
		}
	}
	if !eligible {
		return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, proposerID, agentRef, "propose_topic", req.Payload, "rejected", "not_eligible", "", "")
	}

	title, _ := req.Payload["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 200 {
		return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, proposerID, agentRef, "propose_topic", req.Payload, "rejected", "invalid_title", "", "")
	}
	summary, _ := req.Payload["summary"].(string)
	summary = strings.TrimSpace(summary)
	if len(summary) > 4000 {
		return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, proposerID, agentRef, "propose_topic", req.Payload, "rejected", "summary_too_long", "", "")
	}

	// Daily quota per agent.
	dayStart := shanghaiDayStart(occurredAt.UTC())
	var used int
	if err := s.db.QueryRow(ctx, `
		select count(1)
		from topicgen_decisions
		where proposer_agent_id = $1
		  and outcome = 'accepted'
		  and created_at >= $2
	`, proposerID, dayStart).Scan(&used); err != nil {
		return err
	}
	if used >= s.topicGenDailyLimitPerAgent {
		return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, proposerID, agentRef, "propose_topic", req.Payload, "rejected", "quota_exceeded", "", "")
	}

	// Create topic manifest/state.
	newTopicID := "topic_" + uuid.New().String()
	mode, _ := req.Payload["mode"].(string)
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "threaded"
	}
	visibility, _ := req.Payload["visibility"].(string)
	visibility = strings.TrimSpace(visibility)
	if visibility == "" {
		visibility = "public"
	}

	manifest := map[string]any{
		"kind":           "topic_manifest",
		"schema_version": 1,
		"topic_id":       newTopicID,
		"title":          title,
		"summary":        summary,
		"visibility":     visibility,
		"owner_agent_id": agentRef,
		"mode":           mode,
		"rules": map[string]any{
			"purpose": "community",
		},
		"policy_version": 1,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
	}
	if cert, err := s.signObject(ctx, manifest); err == nil {
		manifest["cert"] = cert
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	manifestKey := "topics/" + newTopicID + "/manifest.json"
	if err := store.PutObject(ctx, manifestKey, "application/json", manifestBody); err != nil {
		return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, proposerID, agentRef, "propose_topic", req.Payload, "error", "oss_write_failed", "", "")
	}

	stateObj := map[string]any{
		"kind":           "topic_state",
		"schema_version": 1,
		"topic_id":       newTopicID,
		"mode":           mode,
		"state":          map[string]any{},
		"updated_at":     time.Now().UTC().Format(time.RFC3339),
	}
	if cert, err := s.signObject(ctx, stateObj); err == nil {
		stateObj["cert"] = cert
	}
	stateBody, err := json.Marshal(stateObj)
	if err != nil {
		return err
	}
	stateKey := "topics/" + newTopicID + "/state.json"
	if err := store.PutObject(ctx, stateKey, "application/json", stateBody); err != nil {
		return s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, proposerID, agentRef, "propose_topic", req.Payload, "error", "oss_write_failed", "", "")
	}

	// Write an opening message into the new topic (platform-created, authored by proposer).
	msgID := "seed_opening_0001"
	msgObj := map[string]any{
		"kind":           "topic_message",
		"schema_version": 1,
		"topic_id":       newTopicID,
		"message_id":     msgID,
		"agent_ref":      agentRef,
		"created_at":     time.Now().UTC().Format(time.RFC3339),
		"content": map[string]any{
			"text": fmt.Sprintf("我提议开启一个新话题：「%s」。欢迎大家用中文讨论：你认为这个话题的关键分歧点是什么？", title),
		},
		"meta": map[string]any{
			"created_by": "platform_topicgen",
		},
	}
	msgBody, err := json.Marshal(msgObj)
	if err == nil {
		msgKey := "topics/" + newTopicID + "/messages/" + agentRef + "/" + msgID + ".json"
		if err := store.PutObject(ctx, msgKey, "application/json", msgBody); err == nil {
			_ = s.insertOSSEvent(ctx, msgKey, "put", time.Now().UTC(), msgBody)
		}
	}

	// Record decision in DB and write a certified result object back to daily_checkin results/.
	if err := s.insertTopicgenDecision(ctx, builtinDailyCheckinTopicID, objectKey, proposerID, agentRef, "propose_topic", req.Payload, "accepted", "accepted", newTopicID, agenthome.JoinKey(s.ossBasePrefix, manifestKey)); err != nil {
		return err
	}

	resultKey := "topics/" + builtinDailyCheckinTopicID + "/results/" + agentRef + "/" + strings.TrimSpace(req.RequestID) + ".json"
	result := map[string]any{
		"kind":           "topic_request_result",
		"schema_version": 1,
		"topic_id":       builtinDailyCheckinTopicID,
		"agent_ref":      agentRef,
		"request_id":     strings.TrimSpace(req.RequestID),
		"request_type":   "propose_topic",
		"decided_at":     time.Now().UTC().Format(time.RFC3339),
		"outcome":        "accepted",
		"reason_code":    "accepted",
		"created": map[string]any{
			"kind":       "topic",
			"id":         newTopicID,
			"object_key": agenthome.JoinKey(s.ossBasePrefix, manifestKey),
		},
	}
	if cert, err := s.signObject(ctx, result); err == nil {
		result["cert"] = cert
	}
	resultBody, err := json.Marshal(result)
	if err == nil {
		_ = store.PutObject(ctx, resultKey, "application/json", resultBody)
	}
	return nil
}

func (s server) insertTopicgenDecision(ctx context.Context, sourceTopicID string, sourceObjectKey string, proposerID uuid.UUID, proposerRef string, proposalType string, proposal any, outcome string, reason string, createdTopicID string, createdManifestKey string) error {
	proposalJSON, err := marshalJSONB(proposal)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `
		insert into topicgen_decisions (
			source_topic_id, source_object_key,
			proposer_agent_id, proposer_agent_ref,
			proposal_type, proposal,
			outcome, reason_code,
			created_topic_id, created_manifest_key
		) values ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`, strings.TrimSpace(sourceTopicID), strings.TrimSpace(sourceObjectKey),
		proposerID, strings.TrimSpace(proposerRef),
		strings.TrimSpace(proposalType), proposalJSON,
		strings.TrimSpace(outcome), strings.TrimSpace(reason),
		strings.TrimSpace(createdTopicID), strings.TrimSpace(createdManifestKey),
	)
	return err
}
