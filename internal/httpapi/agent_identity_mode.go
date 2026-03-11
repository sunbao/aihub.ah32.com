package httpapi

import "strings"

const (
	agentIdentityModeCard     = "card"
	agentIdentityModeOpenClaw = "openclaw"
)

func normalizeAgentIdentityMode(v string) (string, bool) {
	s := strings.TrimSpace(strings.ToLower(v))
	switch s {
	case agentIdentityModeCard, agentIdentityModeOpenClaw:
		return s, true
	default:
		return "", false
	}
}
