package httpapi

import (
	"regexp"
	"strings"
)

var allowedTopicCategories = []string{
	"政史",
	"当今实事",
	"明星",
	"热点",
}

var topicCategorySet = func() map[string]bool {
	m := map[string]bool{}
	for _, c := range allowedTopicCategories {
		m[strings.TrimSpace(c)] = true
	}
	return m
}()

var topicTitleCategoryRe = regexp.MustCompile(`^\s*(?:\[(?P<bracket>[^\]]+)\]|【(?P<corner>[^】]+)】|(?P<plain>[^:：|｜]{1,12})[:：|｜])\s*(?P<title>.+?)\s*$`)

func normalizeTopicCategory(s string) string {
	c := strings.TrimSpace(s)
	if c == "" {
		return ""
	}
	if topicCategorySet[c] {
		return c
	}
	return ""
}

// extractCategoryFromTitleLine supports:
// - [政史] 标题
// - 【政史】标题
// - 政史: 标题 / 政史｜标题
// Only the four allowed categories are recognized; otherwise it returns ("", originalTitle).
func extractCategoryFromTitleLine(titleLine string) (category string, title string) {
	raw := strings.TrimSpace(titleLine)
	if raw == "" {
		return "", ""
	}
	m := topicTitleCategoryRe.FindStringSubmatch(raw)
	if len(m) == 0 {
		return "", raw
	}
	names := topicTitleCategoryRe.SubexpNames()
	g := map[string]string{}
	for i, n := range names {
		if i == 0 || strings.TrimSpace(n) == "" {
			continue
		}
		g[n] = strings.TrimSpace(m[i])
	}
	cat := g["bracket"]
	if cat == "" {
		cat = g["corner"]
	}
	if cat == "" {
		cat = g["plain"]
	}
	cat = normalizeTopicCategory(cat)
	if cat == "" {
		return "", raw
	}
	t := strings.TrimSpace(g["title"])
	if t == "" {
		return "", raw
	}
	return cat, t
}
