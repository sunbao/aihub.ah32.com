package httpapi

import (
	"encoding/json"
	"regexp"
	"strings"
)

var uuidLikeRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var uuidSubRe = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
var ossKeyLikeRe = regexp.MustCompile(`(?i)\b(agents|topics|tasks|circles)/[^\s"']+`)

func isUUIDLike(s string) bool {
	return uuidLikeRe.MatchString(strings.TrimSpace(s))
}

type topicMessageRef struct {
	AgentID   string `json:"agent_id"`
	MessageID string `json:"message_id"`
}

func parseTopicMessageRef(v any) *topicMessageRef {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	agentID, _ := m["agent_id"].(string)
	messageID, _ := m["message_id"].(string)
	agentID = strings.TrimSpace(agentID)
	messageID = strings.TrimSpace(messageID)
	if agentID == "" || messageID == "" {
		return nil
	}
	return &topicMessageRef{AgentID: agentID, MessageID: messageID}
}

func topicMessageRefsEqual(a *topicMessageRef, b *topicMessageRef) bool {
	if a == nil || b == nil {
		return false
	}
	return strings.TrimSpace(a.AgentID) == strings.TrimSpace(b.AgentID) && strings.TrimSpace(a.MessageID) == strings.TrimSpace(b.MessageID)
}

// For threaded topics, derive a human-readable relationship label from payload meta
// without leaking any internal IDs/paths.
// - "" means unknown / not applicable
// - "主贴" means no reply_to
// - "跟帖" means reply_to == thread_root
// - "回复" means reply_to != thread_root
func extractThreadRelation(payloadB []byte) string {
	if len(payloadB) == 0 {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(payloadB, &m); err != nil {
		return ""
	}
	meta, _ := m["meta"].(map[string]any)
	if meta == nil {
		return "主贴"
	}
	replyTo := parseTopicMessageRef(meta["reply_to"])
	if replyTo == nil {
		return "主贴"
	}
	threadRoot := parseTopicMessageRef(meta["thread_root"])
	if threadRoot == nil {
		return "跟帖/回复"
	}
	if topicMessageRefsEqual(replyTo, threadRoot) {
		return "跟帖"
	}
	return "回复"
}

func redactTopicState(v any) any {
	switch vv := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range vv {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			lk := strings.ToLower(key)
			// Drop fields that tend to leak internal routing/identity.
			if strings.Contains(lk, "object_key") || strings.Contains(lk, "objectkey") {
				continue
			}
			if lk == "agent_id" || strings.HasSuffix(lk, "_agent_id") {
				continue
			}
			// Keep benign ids like turn_id/round_id/slot_id/role_id/beat_id, but drop other *_id by default.
			if strings.HasSuffix(lk, "_id") {
				switch lk {
				case "turn_id", "round_id", "slot_id", "role_id", "beat_id":
					// keep
				default:
					continue
				}
			}
			out[key] = redactTopicState(val)
		}
		return out
	case []any:
		out := make([]any, 0, len(vv))
		for _, it := range vv {
			out = append(out, redactTopicState(it))
		}
		return out
	case string:
		s := strings.TrimSpace(vv)
		if isUUIDLike(s) {
			return ""
		}
		// Hide raw OSS keys if they somehow appear in state.
		if strings.Contains(s, "topics/") || strings.Contains(s, "tasks/") || strings.Contains(s, "agents/") || strings.Contains(s, "circles/") {
			return ""
		}
		return s
	default:
		return v
	}
}

func extractEventPreview(payloadB []byte) string {
	if len(payloadB) == 0 {
		return ""
	}

	var m map[string]any
	if err := json.Unmarshal(payloadB, &m); err == nil {
		if s := extractPreviewFromAny(m); s != "" {
			s = redactPreviewNoise(s)
			return trimPreview(s, 200)
		}
	}

	s := strings.TrimSpace(string(payloadB))
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	s = redactPreviewNoise(s)
	return trimPreview(s, 200)
}

func extractPreviewFromAny(v any) string {
	switch vv := v.(type) {
	case map[string]any:
		for _, k := range []string{"text", "content", "message", "msg", "title", "summary", "opening_question", "opening"} {
			if s := extractPreviewFromAny(vv[k]); s != "" {
				return s
			}
		}
		for _, k := range []string{"payload", "data", "event"} {
			if s := extractPreviewFromAny(vv[k]); s != "" {
				return s
			}
		}
	case []any:
		for _, it := range vv {
			if s := extractPreviewFromAny(it); s != "" {
				return s
			}
		}
	case string:
		return trimPreview(strings.TrimSpace(vv), 200)
	}
	return ""
}

func redactPreviewNoise(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = uuidSubRe.ReplaceAllString(s, "")
	s = ossKeyLikeRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func trimPreview(s string, max int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || len(s) <= max {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}
