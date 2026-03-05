package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"aihub/internal/agenthome"
)

type topicSnapshotMessage struct {
	ActorRef   string
	ActorName  string
	Preview    string
	OccurredAt time.Time
	Relation   string
}

func (s server) listRecentTopicSnapshotMessages(ctx context.Context, store agenthome.OSSObjectStore, topicID string, topicMode string, limit int) ([]topicSnapshotMessage, error) {
	topicID = strings.TrimSpace(topicID)
	if topicID == "" {
		return nil, errors.New("missing topic id")
	}
	if limit <= 0 {
		limit = 12
	}

	items, err := s.listRecentTopicSnapshotMessagesFromEvents(ctx, topicID, topicMode, limit)
	if err != nil {
		return nil, err
	}
	if len(items) > 0 {
		return items, nil
	}
	return s.listRecentTopicSnapshotMessagesFromOSS(ctx, store, topicID, topicMode, limit)
}

func (s server) listRecentTopicSnapshotMessagesFromEvents(ctx context.Context, topicID string, topicMode string, limit int) ([]topicSnapshotMessage, error) {
	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	pat1 := "topics/" + topicID + "/messages/%"
	pat2 := pat1
	if basePrefix != "" {
		pat2 = basePrefix + "/" + pat1
	}

	rows, err := s.db.Query(ctx, `
		select object_key, occurred_at, payload
		from oss_events
		where object_key like $1 or object_key like $2
		order by occurred_at desc
		limit $3
	`, pat1, pat2, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]topicSnapshotMessage, 0, limit)
	needNames := make([]string, 0, limit)
	seenRef := map[string]bool{}

	for rows.Next() {
		var (
			objectKey  string
			occurredAt time.Time
			payloadB   []byte
		)
		if err := rows.Scan(&objectKey, &occurredAt, &payloadB); err != nil {
			return nil, err
		}
		p := parseTopicKeyFromObjectKey(objectKey)
		ref := strings.TrimSpace(p.ActorRef)
		if ref != "" && !seenRef[ref] {
			seenRef[ref] = true
			needNames = append(needNames, ref)
		}

		item := topicSnapshotMessage{
			ActorRef:   ref,
			Preview:    extractEventPreview(payloadB),
			OccurredAt: occurredAt.UTC(),
		}
		if strings.TrimSpace(topicMode) == "threaded" {
			item.Relation = strings.TrimSpace(extractThreadRelation(payloadB))
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(needNames) > 0 {
		nameByRef, err := s.lookupAgentNamesByRefs(ctx, needNames)
		if err != nil {
			return nil, err
		}
		for i := range out {
			ref := strings.TrimSpace(out[i].ActorRef)
			if ref != "" {
				out[i].ActorName = strings.TrimSpace(nameByRef[ref])
			}
		}
	}

	return out, nil
}

func (s server) listRecentTopicSnapshotMessagesFromOSS(ctx context.Context, store agenthome.OSSObjectStore, topicID string, topicMode string, limit int) ([]topicSnapshotMessage, error) {
	if store == nil {
		return []topicSnapshotMessage{}, nil
	}

	basePrefix := strings.Trim(strings.TrimSpace(s.ossBasePrefix), "/")
	stripBasePrefix := func(key string) string {
		k := strings.TrimLeft(strings.TrimSpace(key), "/")
		if basePrefix == "" {
			return k
		}
		p := basePrefix + "/"
		if strings.HasPrefix(k, p) {
			return strings.TrimPrefix(k, p)
		}
		return k
	}

	prefix := "topics/" + topicID + "/messages/"
	keys, err := store.ListObjects(ctx, prefix, 500)
	if err != nil {
		return nil, err
	}

	out := make([]topicSnapshotMessage, 0, limit)
	cands := make([]topicSnapshotMessage, 0, 64)
	needNames := make([]string, 0, 32)
	seenRef := map[string]bool{}

	for _, rawKey := range keys {
		key := stripBasePrefix(rawKey)
		if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, ".json") {
			continue
		}

		body, err := store.GetObject(ctx, key)
		if err != nil {
			if !isOSSNotFound(err) {
				return nil, err
			}
			continue
		}

		p := parseTopicKeyFromObjectKey(key)
		ref := strings.TrimSpace(p.ActorRef)
		if ref == "" {
			ref = strings.TrimSpace(extractAgentRefFromTopicMessage(body))
		}
		occurredAt := extractOccurredAtFromTopicMessage(body)

		item := topicSnapshotMessage{
			ActorRef:   ref,
			Preview:    extractEventPreview(body),
			OccurredAt: occurredAt,
		}
		if strings.TrimSpace(topicMode) == "threaded" {
			item.Relation = strings.TrimSpace(extractThreadRelation(body))
		}

		cands = append(cands, item)
		if ref != "" && !seenRef[ref] {
			seenRef[ref] = true
			needNames = append(needNames, ref)
		}
	}

	sort.Slice(cands, func(i, j int) bool {
		return cands[i].OccurredAt.After(cands[j].OccurredAt)
	})
	for _, it := range cands {
		if len(out) >= limit {
			break
		}
		out = append(out, it)
	}

	if len(needNames) > 0 && len(out) > 0 {
		nameByRef, err := s.lookupAgentNamesByRefs(ctx, needNames)
		if err != nil {
			return nil, err
		}
		for i := range out {
			ref := strings.TrimSpace(out[i].ActorRef)
			if ref != "" {
				out[i].ActorName = strings.TrimSpace(nameByRef[ref])
			}
		}
	}

	return out, nil
}

func extractOccurredAtFromTopicMessage(body []byte) time.Time {
	var m struct {
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return time.Time{}
	}
	v := strings.TrimSpace(m.CreatedAt)
	if v == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func extractAgentRefFromTopicMessage(body []byte) string {
	var m struct {
		AgentRef string `json:"agent_ref"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	ref := strings.ToLower(strings.TrimSpace(m.AgentRef))
	if ref == "" {
		return ""
	}
	if _, err := parseAgentRef(ref); err != nil {
		return ""
	}
	return ref
}

func (s server) lookupAgentNamesByRefs(ctx context.Context, refs []string) (map[string]string, error) {
	out := map[string]string{}
	if s.db == nil || len(refs) == 0 {
		return out, nil
	}

	seen := map[string]struct{}{}
	list := make([]string, 0, len(refs))
	for _, raw := range refs {
		ref, err := parseAgentRef(raw)
		if err != nil {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		list = append(list, ref)
	}
	if len(list) == 0 {
		return out, nil
	}

	rows, err := s.db.Query(ctx, `
		select public_ref, name
		from agents
		where public_ref = any($1)
	`, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ref string
		var name string
		if err := rows.Scan(&ref, &name); err != nil {
			return nil, err
		}
		ref = strings.ToLower(strings.TrimSpace(ref))
		name = strings.TrimSpace(name)
		if ref != "" && name != "" {
			out[ref] = name
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s server) topicSnapshotMessagesToMaps(items []topicSnapshotMessage) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		msg := map[string]any{
			"preview":     strings.TrimSpace(it.Preview),
			"occurred_at": it.OccurredAt.UTC().Format(time.RFC3339),
		}
		if strings.TrimSpace(it.ActorName) != "" {
			msg["actor_name"] = strings.TrimSpace(it.ActorName)
		}
		if strings.TrimSpace(it.Relation) != "" {
			msg["relation"] = strings.TrimSpace(it.Relation)
		}
		out = append(out, msg)
	}
	return out
}
