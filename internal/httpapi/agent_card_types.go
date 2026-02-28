package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type personalityDTO struct {
	Extrovert float64 `json:"extrovert"`
	Curious   float64 `json:"curious"`
	Creative  float64 `json:"creative"`
	Stable    float64 `json:"stable"`
}

func (p personalityDTO) Validate() error {
	if p.Extrovert < 0 || p.Extrovert > 1 {
		return errors.New("personality.extrovert out of range")
	}
	if p.Curious < 0 || p.Curious > 1 {
		return errors.New("personality.curious out of range")
	}
	if p.Creative < 0 || p.Creative > 1 {
		return errors.New("personality.creative out of range")
	}
	if p.Stable < 0 || p.Stable > 1 {
		return errors.New("personality.stable out of range")
	}
	return nil
}

func defaultPersonality() personalityDTO {
	return personalityDTO{Extrovert: 0.5, Curious: 0.5, Creative: 0.5, Stable: 0.5}
}

type discoveryDTO struct {
	Public       bool   `json:"public"`
	OSSEndpoint  string `json:"oss_endpoint,omitempty"`
	LastSyncedAt string `json:"last_synced_at,omitempty"`
}

type autonomousDTO struct {
	Enabled             bool `json:"enabled"`
	PollIntervalSeconds int  `json:"poll_interval_seconds"`
	AutoAcceptMatching  bool `json:"auto_accept_matching"`
}

func defaultAutonomous() autonomousDTO {
	return autonomousDTO{Enabled: false, PollIntervalSeconds: 60, AutoAcceptMatching: false}
}

func (a autonomousDTO) Validate() error {
	if a.PollIntervalSeconds < 10 || a.PollIntervalSeconds > 86400 {
		return errors.New("autonomous.poll_interval_seconds out of range")
	}
	return nil
}

func normalizeStringList(in []string, maxItems int, maxLen int) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if len(v) > maxLen {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
		if len(out) >= maxItems {
			break
		}
	}
	return out
}

func marshalJSONB(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return []byte("null"), nil
	}
	return b, nil
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func safeTrim(s string, max int) string {
	s = strings.TrimSpace(s)
	if max > 0 && len(s) > max {
		return s[:max]
	}
	return s
}

func validateNonEmptyOrDefault(s string, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	return s
}

func promptViewFromFields(name string, persona any, p personalityDTO, interests, capabilities []string, bio string) string {
	// Keep this short and prompt-safe; agent-side prompt bundles can embed this directly.
	// NOTE: enforced max length at call site.
	var sb strings.Builder
	sb.WriteString("名字：")
	sb.WriteString(strings.TrimSpace(name))

	if m, ok := persona.(map[string]any); ok && len(m) == 0 {
		persona = nil
	}
	if persona != nil {
		lbl := personaDisplayLabel(persona)
		sb.WriteString("；人设：")
		if lbl != "" {
			sb.WriteString(lbl)
			sb.WriteString("（风格参考）")
		} else {
			sb.WriteString("已启用（风格参考）")
		}
		sb.WriteString("；禁止冒充/自称原型身份")
	}

	sb.WriteString(fmt.Sprintf("；性格：外向%.2f/好奇%.2f/创意%.2f/稳定%.2f", p.Extrovert, p.Curious, p.Creative, p.Stable))

	if len(interests) > 0 {
		sb.WriteString("；兴趣：")
		sb.WriteString(strings.Join(interests, "、"))
	}
	if len(capabilities) > 0 {
		sb.WriteString("；能力：")
		sb.WriteString(strings.Join(capabilities, "、"))
	}
	bio = strings.TrimSpace(bio)
	if bio != "" {
		sb.WriteString("；简介：")
		sb.WriteString(bio)
	}
	return sb.String()
}

func personaDisplayLabel(persona any) string {
	m, ok := persona.(map[string]any)
	if !ok {
		return ""
	}
	cands := []string{
		strings.TrimSpace(asString(m["label"])),
		strings.TrimSpace(asString(m["name"])),
		strings.TrimSpace(asString(m["title"])),
		strings.TrimSpace(asString(m["display_name"])),
	}
	for _, c := range cands {
		if c != "" && !strings.HasPrefix(c, "persona_") && !strings.HasPrefix(c, "custom_") {
			return c
		}
	}
	if insp, ok := m["inspiration"].(map[string]any); ok {
		ref := strings.TrimSpace(asString(insp["reference"]))
		if ref != "" && !strings.HasPrefix(ref, "persona_") && !strings.HasPrefix(ref, "custom_") {
			return ref
		}
	}
	return ""
}
