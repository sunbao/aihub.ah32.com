package httpapi

import (
	"encoding/json"
	"strings"
)

// extractTopicMessageTextBestEffort tries to recover a human-readable message body
// from a stored topic_message payload. Some historical/test payloads may have
// double-encoded JSON or nested content shapes; this function normalizes them.
func extractTopicMessageTextBestEffort(payloadB []byte) string {
	if len(payloadB) == 0 {
		return ""
	}

	extractFromMap := func(m map[string]any) string {
		if m == nil {
			return ""
		}
		if v, _ := m["text"].(string); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		if c, ok := m["content"].(map[string]any); ok && c != nil {
			if v, _ := c["text"].(string); strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
			// Some buggy writers wrapped again: { content: { content: { text } } }.
			if cc, ok := c["content"].(map[string]any); ok && cc != nil {
				if v, _ := cc["text"].(string); strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
			// Or: { content: { content: "..." } }.
			if v, _ := c["content"].(string); strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		if v, _ := m["content"].(string); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return ""
	}

	var m map[string]any
	if err := json.Unmarshal(payloadB, &m); err == nil {
		if s := extractFromMap(m); s != "" {
			return s
		}
	}

	// Fallback: use generic preview extraction, then try to decode if it looks like JSON.
	s := strings.TrimSpace(extractEventPreview(payloadB))
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		var m2 map[string]any
		if err := json.Unmarshal([]byte(s), &m2); err == nil {
			if ss := extractFromMap(m2); ss != "" {
				return ss
			}
		}
	}
	return s
}
