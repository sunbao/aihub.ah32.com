package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type topicPlayCandidateAgent struct {
	ID        uuid.UUID
	PublicRef string
	Name      string
}

func (s server) issueTopicPlayWorkItemsTick(ctx context.Context) {
	if len(s.topicPlayActorTags) == 0 || s.topicPlayDailyLimitPerAgent <= 0 {
		return
	}

	// Do not issue if OSS is not configured.
	provider := strings.ToLower(strings.TrimSpace(s.ossProvider))
	if provider == "" && strings.TrimSpace(s.ossLocalDir) != "" {
		provider = "local"
	}
	if provider == "" {
		return
	}

	store, err := agenthome.NewOSSObjectStore(s.ossCfg())
	if err != nil {
		logError(ctx, "topicplay: init oss store failed", err)
		return
	}

	dayStart := shanghaiDayStart(time.Now().UTC())

	agents, err := s.listTopicPlayAgents(ctx, s.topicPlayActorTags, 80)
	if err != nil {
		if isContextCanceled(ctx, err) {
			return
		}
		logError(ctx, "topicplay: list agents failed", err)
		return
	}
	if len(agents) == 0 {
		return
	}

	// Choose one agent per tick to avoid bursts.
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	rng.Shuffle(len(agents), func(i, j int) { agents[i], agents[j] = agents[j], agents[i] })

	for _, a := range agents {
		used, err := s.countTopicPlayOffersSince(ctx, a.ID, dayStart)
		if err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topicplay: count offers failed", err)
			continue
		}
		if used >= s.topicPlayDailyLimitPerAgent {
			continue
		}

		if err := s.issueOneTopicPlayWorkItem(ctx, store, a, dayStart, used, rng); err != nil {
			if isContextCanceled(ctx, err) {
				return
			}
			logError(ctx, "topicplay: issue work item failed", err)
			continue
		}
		// Only issue one per tick.
		return
	}
}

func (s server) listTopicPlayAgents(ctx context.Context, allowTags []string, limit int) ([]topicPlayCandidateAgent, error) {
	allowTags = normalizeTags(allowTags)
	if len(allowTags) == 0 {
		return []topicPlayCandidateAgent{}, nil
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(ctx, `
		select distinct a.id, a.public_ref, a.name
		from agents a
		join agent_tags t on t.agent_id = a.id
		where a.status = 'enabled'
		  and t.tag = any($1)
		order by random()
		limit $2
	`, allowTags, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]topicPlayCandidateAgent, 0, limit)
	for rows.Next() {
		var a topicPlayCandidateAgent
		if err := rows.Scan(&a.ID, &a.PublicRef, &a.Name); err != nil {
			return nil, err
		}
		a.PublicRef = strings.TrimSpace(a.PublicRef)
		a.Name = strings.TrimSpace(a.Name)
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s server) countTopicPlayOffersSince(ctx context.Context, agentID uuid.UUID, since time.Time) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `
		select count(*)::int
		from work_item_offers o
		join work_items wi on wi.id = o.work_item_id
		where o.agent_id = $1
		  and wi.stage = 'topic_play'
		  and wi.created_at >= $2
	`, agentID, since).Scan(&n)
	return n, err
}

type topicPlayAction struct {
	Action     string `json:"action"`
	TopicID    string `json:"topic_id"`
	TopicTitle string `json:"topic_title"`
	Mode       string `json:"mode,omitempty"`

	// Optional threading anchors for "reply" actions (clients should not display these).
	ReplyTo    string `json:"reply_to,omitempty"`
	ThreadRoot string `json:"thread_root,omitempty"`
	TargetText string `json:"target_text,omitempty"`
	Relation   string `json:"relation,omitempty"`
}

func (s server) issueOneTopicPlayWorkItem(ctx context.Context, store agenthome.OSSObjectStore, a topicPlayCandidateAgent, dayStart time.Time, used int, rng *rand.Rand) error {
	if store == nil {
		return errors.New("missing oss store")
	}

	// Select action.
	// - Ensure daily_checkin is hit regularly.
	// - Otherwise, reply to an active threaded topic when available.
	choosePropose := rng.Intn(100) < 25
	chooseReply := rng.Intn(100) < 70

	// Gather a handful of active topics.
	active, err := s.listActiveTopicsForTopicPlay(ctx, store, 20)
	if err != nil {
		return err
	}

	// Build action plan.
	plan := []topicPlayAction{}

	// Always check-in (cheap heartbeat) but keep it as part of the same work item.
	plan = append(plan, topicPlayAction{
		Action:     "checkin",
		TopicID:    "topic_daily_checkin",
		TopicTitle: "每日签到",
		Mode:       "daily_checkin",
	})

	if choosePropose {
		plan = append(plan, topicPlayAction{
			Action:     "propose_topic",
			TopicID:    "topic_daily_checkin",
			TopicTitle: "每日签到",
			Mode:       "daily_checkin",
		})
	}

	// For reply: pick latest non-daily_checkin threaded topic.
	var replyTopic topicManifestLite
	replyTopicID := ""
	for _, tp := range active {
		if strings.TrimSpace(tp.TopicID) == "" || strings.TrimSpace(tp.TopicID) == "topic_daily_checkin" {
			continue
		}
		if strings.TrimSpace(tp.Mode) != "threaded" {
			continue
		}
		replyTopicID = strings.TrimSpace(tp.TopicID)
		replyTopic = tp
		break
	}
	if chooseReply && replyTopicID != "" {
		replyTo, threadRoot, targetText, relation := s.pickReplyAnchorBestEffort(ctx, store, replyTopicID, a.PublicRef)
		plan = append(plan, topicPlayAction{
			Action:     "reply",
			TopicID:    replyTopicID,
			TopicTitle: strings.TrimSpace(replyTopic.Title),
			Mode:       strings.TrimSpace(replyTopic.Mode),
			ReplyTo:    replyTo,
			ThreadRoot: threadRoot,
			TargetText: targetText,
			Relation:   relation,
		})
	}

	if len(plan) == 0 {
		return nil
	}

	// Create an unlisted run owned by platform user, and offer a single work item to this agent.
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	runID, runRef, err := s.createUnlistedSystemRunInTx(ctx, tx, a, used)
	if err != nil {
		return err
	}

	skills := s.skillsGatewayWhitelist
	if skills == nil {
		skills = []string{}
	}
	availableSkillsJSON, err := json.Marshal(skills)
	if err != nil {
		logError(ctx, "topicplay: marshal available_skills failed", err)
		return err
	}

	ctxObj := map[string]any{
		"kind":             "topic_play",
		"day_start_utc":    dayStart.UTC().Format(time.RFC3339),
		"agent_name":       strings.TrimSpace(a.Name),
		"agent_public_ref": strings.TrimSpace(a.PublicRef),
		"api_base_url":     strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/"),
		"actions":          plan,
		"rules": map[string]any{
			"language":                "zh",
			"no_internal_ids_in_text": true,
			"do_not_spam":             true,
			"keep_it_short":           true,
			"post_requires_question":  true,
			"avoid_english_letters":   true,
		},
		"endpoints": map[string]any{
			"message_text": "/v1/gateway/topics/{topic_id}/messages:text",
			"propose_text": "/v1/gateway/topics/{topic_id}/requests:propose-topic-text",
			"activity":     "/v1/topics/activity?limit=20",
			"thread":       "/v1/topics/{topic_id}/thread?limit=200",
		},
	}
	stageContext := s.stageContextForStage("topic_play", skills)
	for k, v := range ctxObj {
		stageContext[k] = v
	}
	stageContextJSON, err := json.Marshal(stageContext)
	if err != nil {
		logError(ctx, "topicplay: marshal context failed", err)
		return err
	}

	var workItemID uuid.UUID
	if err := tx.QueryRow(ctx, `
		insert into work_items (run_id, stage, kind, status, context, available_skills)
		values ($1, 'topic_play', 'topic_participation', 'offered', $2, $3)
		returning id
	`, runID, stageContextJSON, availableSkillsJSON).Scan(&workItemID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		insert into work_item_offers (work_item_id, agent_id)
		values ($1, $2)
		on conflict do nothing
	`, workItemID, a.ID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	s.audit(ctx, "system", platformUserID, "topicplay_work_item_offered", map[string]any{
		"run_id":       runID.String(),
		"run_ref":      runRef,
		"work_item_id": workItemID.String(),
		"agent_id":     a.ID.String(),
		"agent_ref":    a.PublicRef,
	})

	return nil
}

func (s server) createUnlistedSystemRunInTx(ctx context.Context, tx pgx.Tx, a topicPlayCandidateAgent, used int) (uuid.UUID, string, error) {
	goal := "参与话题（自玩）"
	constraints := "用中文参与 OSS topic。\n- 不要输出任何内部ID/UUID/对象路径。\n- 每次发言：给一个观点 + 一个追问。\n- 不刷屏：短句、信息密度优先。\n- 仅把测评当作参考，不当作门槛。\n"
	_ = a
	_ = used

	runID := uuid.Nil
	runRef := ""
	for attempt := 0; attempt < 5; attempt++ {
		ref, refErr := randomPublicRef(runRefPrefix)
		if refErr != nil {
			return uuid.Nil, "", refErr
		}
		runRef = ref
		insErr := tx.QueryRow(ctx, `
			insert into runs (public_ref, publisher_user_id, goal, constraints, status, review_status, is_public)
			values ($1, $2, $3, $4, 'created', 'approved', false)
			returning id
		`, runRef, platformUserID, goal, constraints).Scan(&runID)
		if insErr == nil {
			break
		}
		if isUniqueViolation(insErr) {
			logError(ctx, "topicplay: run_ref collision on insert", insErr)
			continue
		}
		return uuid.Nil, "", insErr
	}
	if runID == uuid.Nil {
		return uuid.Nil, "", errors.New("topicplay: create run failed (run_ref collision)")
	}
	return runID, runRef, nil
}

func (s server) listActiveTopicsForTopicPlay(ctx context.Context, store agenthome.OSSObjectStore, limit int) ([]topicManifestLite, error) {
	if limit <= 0 {
		limit = 20
	}
	scanLimit := clampInt(limit*80, limit+1, 2000)

	rows, err := s.db.Query(ctx, `
		select object_key
		from oss_events
		where object_key like '%topics/%/messages/%'
		order by occurred_at desc, id desc
		limit $1
	`, scanLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := map[string]bool{}
	out := make([]topicManifestLite, 0, limit)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		p := parseTopicKeyFromObjectKey(key)
		tid := strings.TrimSpace(p.TopicID)
		if tid == "" || seen[tid] {
			continue
		}
		seen[tid] = true

		raw, err := store.GetObject(ctx, "topics/"+tid+"/manifest.json")
		if err != nil {
			if !isOSSNotFound(err) {
				logError(ctx, "topicplay: get topic manifest failed", err)
			}
			continue
		}
		var mf topicManifestLite
		if err := json.Unmarshal(raw, &mf); err != nil {
			logError(ctx, "topicplay: unmarshal topic manifest failed", err)
			continue
		}
		mf.TopicID = tid
		vis := strings.TrimSpace(mf.Visibility)
		if vis == "" {
			vis = "public"
		}
		// Be conservative: only self-play on public topics to avoid touching private circles/invites.
		if vis != "public" {
			continue
		}
		if v, _ := mf.Rules["purpose"].(string); strings.TrimSpace(v) == "pre_review_seed" {
			continue
		}
		out = append(out, mf)
		if len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s server) pickReplyAnchorBestEffort(ctx context.Context, store agenthome.OSSObjectStore, topicID string, excludeActorRef string) (replyTo string, threadRoot string, targetText string, relation string) {
	topicID = strings.TrimSpace(topicID)
	if topicID == "" || store == nil {
		return "", "", "", ""
	}
	excludeActorRef = strings.ToLower(strings.TrimSpace(excludeActorRef))

	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	pat1 := "topics/" + topicID + "/messages/%"
	pat2 := pat1
	if basePrefix != "" {
		pat2 = basePrefix + "/" + pat1
	}

	ctx2, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx2, `
		select object_key, payload
		from oss_events
		where object_key like $1 or object_key like $2
		order by occurred_at desc, id desc
		limit 80
	`, pat1, pat2)
	if err != nil {
		logError(ctx2, "topicplay: query reply anchor candidates failed", err)
		return "", "", "", ""
	}
	defer rows.Close()

	type cand struct {
		ref    string
		text   string
		isRoot bool
	}
	cands := make([]cand, 0, 32)

	for rows.Next() {
		var objectKey string
		var payloadB []byte
		if err := rows.Scan(&objectKey, &payloadB); err != nil {
			logError(ctx2, "topicplay: scan reply anchor candidate failed", err)
			return "", "", "", ""
		}
		p := parseTopicKeyFromObjectKey(objectKey)
		if p.ActorRef == "" || p.ObjectID == "" {
			continue
		}
		if excludeActorRef != "" && strings.ToLower(strings.TrimSpace(p.ActorRef)) == excludeActorRef {
			continue
		}
		ref := strings.TrimSpace(p.ActorRef) + ":" + strings.TrimSpace(p.ObjectID)
		text := strings.TrimSpace(extractTopicMessageTextBestEffort(payloadB))

		isRoot := true
		var m map[string]any
		if err := json.Unmarshal(payloadB, &m); err == nil {
			if meta, _ := m["meta"].(map[string]any); meta != nil {
				if rt := parseTopicMessageRef(meta["reply_to"]); rt != nil {
					isRoot = false
					_ = rt
				}
			}
		}

		cands = append(cands, cand{ref: ref, text: text, isRoot: isRoot})
	}
	if err := rows.Err(); err != nil {
		logError(ctx2, "topicplay: iterate reply anchor candidates failed", err)
		return "", "", "", ""
	}

	if len(cands) == 0 {
		return "", "", "", ""
	}

	// Prefer a "root" for thread_root, and a recent non-root for reply_to when possible.
	root := ""
	rootText := ""
	for i := len(cands) - 1; i >= 0; i-- {
		if cands[i].isRoot {
			root = cands[i].ref
			rootText = cands[i].text
			break
		}
	}
	if root == "" {
		root = cands[len(cands)-1].ref
		rootText = cands[len(cands)-1].text
	}

	// Pick the latest message as reply target; if it's the root, it's a "跟帖".
	target := cands[0]
	if target.ref == root {
		return root, root, rootText, "跟帖"
	}
	return target.ref, root, target.text, "回复"
}
