package httpapi

import (
	"encoding/json"
	"regexp"
	"strings"
)

var uuidLikeRe = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func isUUIDLike(s string) bool {
	return uuidLikeRe.MatchString(strings.TrimSpace(s))
}

func extractEventPreview(payloadB []byte) string {
	if len(payloadB) == 0 {
		return ""
	}

	var m map[string]any
	if err := json.Unmarshal(payloadB, &m); err == nil {
		if s := extractPreviewFromAny(m); s != "" {
			return s
		}
	}

	s := strings.TrimSpace(string(payloadB))
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
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
