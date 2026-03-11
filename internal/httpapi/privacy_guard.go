package httpapi

import (
	"context"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

type privacyKind string

const (
	privacyEmail      privacyKind = "email"
	privacyPhone      privacyKind = "phone"
	privacyIDNumber   privacyKind = "id_number"
	privacySecret     privacyKind = "secret"
	privacyPrivateKey privacyKind = "private_key"
)

type privacyFinding struct {
	Kind  privacyKind
	Field string // JSON-ish path like "content.text" / "payload.title"
}

type privacyViolationError struct {
	Findings []privacyFinding
}

func (e privacyViolationError) Error() string {
	kinds := privacyKinds(e.Findings)
	if len(kinds) == 0 {
		return "privacy violation"
	}
	return "privacy violation: " + strings.Join(kinds, ",")
}

func privacyKinds(findings []privacyFinding) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		k := string(f.Kind)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var (
	privacyEmailRe = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
	// Go regexp doesn't support lookbehind; use non-digit boundaries.
	privacyPhoneCNRe   = regexp.MustCompile(`(?:^|[^0-9])1[3-9][0-9]{9}(?:$|[^0-9])`)
	privacyPhoneE164Re = regexp.MustCompile(`\+[1-9][0-9]{7,14}`)
	privacyID18Re      = regexp.MustCompile(`(?:^|[^0-9])[0-9]{17}[0-9Xx](?:$|[^0-9])`)
	privacyID15Re      = regexp.MustCompile(`(?:^|[^0-9])[0-9]{15}(?:$|[^0-9])`)

	privacyPEMKeyRe = regexp.MustCompile(`-----BEGIN (?:RSA |OPENSSH )?PRIVATE KEY-----`)

	privacyOpenAIKeyRe = regexp.MustCompile(`\bsk-[A-Za-z0-9]{16,}\b`)
	privacyGitHubKeyRe = regexp.MustCompile(`\bghp_[A-Za-z0-9]{36}\b`)
	privacyAWSKeyRe    = regexp.MustCompile(`\b(?:AKIA|ASIA)[0-9A-Z]{16}\b`)
	privacyBearerRe    = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._\-]{20,}\b`)
)

func detectPrivacyInString(s string) []privacyKind {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// Keep checks cheap; avoid scanning extremely large blobs.
	const maxBytes = 220_000
	if len(s) > maxBytes {
		s = s[:maxBytes]
	}

	kinds := []privacyKind{}
	add := func(k privacyKind) {
		for _, it := range kinds {
			if it == k {
				return
			}
		}
		kinds = append(kinds, k)
	}

	if privacyEmailRe.FindStringIndex(s) != nil {
		add(privacyEmail)
	}
	if privacyPhoneE164Re.FindStringIndex(s) != nil || privacyPhoneCNRe.FindStringIndex(s) != nil {
		add(privacyPhone)
	}
	if privacyID18Re.FindStringIndex(s) != nil || privacyID15Re.FindStringIndex(s) != nil {
		add(privacyIDNumber)
	}
	if privacyPEMKeyRe.FindStringIndex(s) != nil {
		add(privacyPrivateKey)
	}
	if privacyOpenAIKeyRe.FindStringIndex(s) != nil ||
		privacyGitHubKeyRe.FindStringIndex(s) != nil ||
		privacyAWSKeyRe.FindStringIndex(s) != nil ||
		privacyBearerRe.FindStringIndex(s) != nil {
		add(privacySecret)
	}
	return kinds
}

func detectPrivacyFindings(v any, rootField string) []privacyFinding {
	const (
		maxDepth    = 10
		maxFindings = 24
	)
	findings := []privacyFinding{}
	seen := map[string]struct{}{}

	var scan func(node any, field string, depth int)
	scan = func(node any, field string, depth int) {
		if depth > maxDepth {
			return
		}
		if len(findings) >= maxFindings {
			return
		}

		switch t := node.(type) {
		case string:
			for _, k := range detectPrivacyInString(t) {
				key := string(k) + "|" + field
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				findings = append(findings, privacyFinding{Kind: k, Field: field})
				if len(findings) >= maxFindings {
					return
				}
			}
		case map[string]any:
			// Deterministic order to keep tests stable.
			keys := make([]string, 0, len(t))
			for k := range t {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				nextField := k
				if field != "" {
					nextField = field + "." + k
				}
				scan(t[k], nextField, depth+1)
				if len(findings) >= maxFindings {
					return
				}
			}
		case []any:
			for _, it := range t {
				nextField := field
				if nextField == "" {
					nextField = "[]"
				} else {
					nextField = nextField + "[]"
				}
				scan(it, nextField, depth+1)
				if len(findings) >= maxFindings {
					return
				}
			}
		default:
			// Ignore non-string leaf nodes.
		}
	}

	scan(v, strings.TrimSpace(rootField), 0)
	return findings
}

func rejectIfPrivacyViolation(ctx context.Context, w http.ResponseWriter, v any, rootField string, logMsgPrefix string) bool {
	findings := detectPrivacyFindings(v, rootField)
	if len(findings) == 0 {
		return false
	}

	err := privacyViolationError{Findings: findings}
	if logMsgPrefix == "" {
		logMsgPrefix = "privacy violation"
	}
	// Do not log raw content; only log kinds + fields.
	logError(ctx, logMsgPrefix, err)
	writeJSON(w, http.StatusBadRequest, map[string]any{
		"error": "privacy_violation",
		"kinds": privacyKinds(findings),
	})
	return true
}
