package httpapi

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

//go:embed data/agent-card-catalogs.v1.json
var embeddedAgentCardCatalogsV1 []byte

type agentCardCatalogs struct {
	CatalogVersion     string                    `json:"catalog_version"`
	PersonalityPresets []catalogPersonalityPreset `json:"personality_presets,omitempty"`
	Interests          []catalogLabeledItem       `json:"interests,omitempty"`
	Capabilities       []catalogLabeledItem       `json:"capabilities,omitempty"`
	BioTemplates       []catalogTextTemplate      `json:"bio_templates,omitempty"`
	GreetingTemplates  []catalogTextTemplate      `json:"greeting_templates,omitempty"`
	// Optional fields (UI may use them, server validation doesn't depend on them).
	NameTemplates  []map[string]any `json:"name_templates,omitempty"`
	AvatarOptions  []map[string]any `json:"avatar_options,omitempty"`
}

type catalogLabeledItem struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Category string   `json:"category,omitempty"`
	Keywords []string `json:"keywords,omitempty"`
}

type catalogTextTemplate struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Template string `json:"template"`
	MinChars int    `json:"min_chars,omitempty"`
	MaxChars int    `json:"max_chars,omitempty"`
}

type catalogPersonalityPreset struct {
	ID          string         `json:"id"`
	Label       string         `json:"label"`
	Description string         `json:"description,omitempty"`
	Values      personalityDTO `json:"values"`
}

var (
	agentCardCatalogsOnce sync.Once
	agentCardCatalogsVal  *agentCardCatalogs
	agentCardCatalogsErr  error
)

func loadAgentCardCatalogs() (*agentCardCatalogs, error) {
	agentCardCatalogsOnce.Do(func() {
		var c agentCardCatalogs
		if err := json.Unmarshal(embeddedAgentCardCatalogsV1, &c); err != nil {
			agentCardCatalogsErr = err
			return
		}
		c.CatalogVersion = strings.TrimSpace(c.CatalogVersion)
		agentCardCatalogsVal = &c
	})
	return agentCardCatalogsVal, agentCardCatalogsErr
}

type agentCardCatalogSets struct {
	interestLabels    map[string]struct{}
	capabilityLabels  map[string]struct{}
	bioTemplateIDs    map[string]catalogTextTemplate
	greetingTemplateIDs map[string]catalogTextTemplate
}

func (c *agentCardCatalogs) sets() agentCardCatalogSets {
	s := agentCardCatalogSets{
		interestLabels:      map[string]struct{}{},
		capabilityLabels:    map[string]struct{}{},
		bioTemplateIDs:      map[string]catalogTextTemplate{},
		greetingTemplateIDs: map[string]catalogTextTemplate{},
	}
	for _, it := range c.Interests {
		lbl := strings.TrimSpace(it.Label)
		if lbl != "" {
			s.interestLabels[lbl] = struct{}{}
		}
	}
	for _, it := range c.Capabilities {
		lbl := strings.TrimSpace(it.Label)
		if lbl != "" {
			s.capabilityLabels[lbl] = struct{}{}
		}
	}
	for _, t := range c.BioTemplates {
		if strings.TrimSpace(t.ID) == "" {
			continue
		}
		s.bioTemplateIDs[strings.TrimSpace(t.ID)] = t
	}
	for _, t := range c.GreetingTemplates {
		if strings.TrimSpace(t.ID) == "" {
			continue
		}
		s.greetingTemplateIDs[strings.TrimSpace(t.ID)] = t
	}
	return s
}

func renderCatalogTemplate(tmpl string, name string, interests []string, capabilities []string) string {
	out := tmpl
	out = strings.ReplaceAll(out, "{name}", strings.TrimSpace(name))
	out = strings.ReplaceAll(out, "{interests}", strings.Join(interests, "、"))
	out = strings.ReplaceAll(out, "{capabilities}", strings.Join(capabilities, "、"))
	return strings.TrimSpace(out)
}

func matchesRenderedTemplate(text string, templates []catalogTextTemplate, name string, interests []string, capabilities []string) bool {
	want := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if want == "" {
		return false
	}
	for _, t := range templates {
		got := renderCatalogTemplate(t.Template, name, interests, capabilities)
		got = strings.Join(strings.Fields(strings.TrimSpace(got)), " ")
		if got != "" && got == want {
			return true
		}
	}
	return false
}

func isPureCatalogCard(c *agentCardCatalogs, personaTemplateID string, interests []string, capabilities []string, bio string, greeting string, name string) bool {
	// NOTE: personaTemplateID is already validated elsewhere (must be approved if set).
	// Missing persona is allowed and does not force review.
	_ = strings.TrimSpace(personaTemplateID)

	sets := c.sets()
	for _, v := range interests {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := sets.interestLabels[v]; !ok {
			return false
		}
	}
	for _, v := range capabilities {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := sets.capabilityLabels[v]; !ok {
			return false
		}
	}

	// To be auto-approved, bio/greeting must be present and match a catalog template rendering.
	if !matchesRenderedTemplate(bio, c.BioTemplates, name, interests, capabilities) {
		return false
	}
	if !matchesRenderedTemplate(greeting, c.GreetingTemplates, name, interests, capabilities) {
		return false
	}
	return true
}

