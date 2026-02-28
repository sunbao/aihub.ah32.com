package httpapi

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed data/agent-card-catalogs.v1.json
var embeddedAgentCardCatalogsV1 []byte

var agentCardCatalogsETag = func() string {
	sum := sha256.Sum256(embeddedAgentCardCatalogsV1)
	return fmt.Sprintf("\"agent-card-catalogs-%x\"", sum)
}()

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
	ID         string   `json:"id"`
	Label      string   `json:"label"`
	LabelEn    string   `json:"label_en,omitempty"`
	Category   string   `json:"category,omitempty"`
	CategoryEn string   `json:"category_en,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
	KeywordsEn []string `json:"keywords_en,omitempty"`
}

type catalogTextTemplate struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	LabelEn    string `json:"label_en,omitempty"`
	Template   string `json:"template"`
	TemplateEn string `json:"template_en,omitempty"`
	MinChars   int    `json:"min_chars,omitempty"`
	MaxChars   int    `json:"max_chars,omitempty"`
}

type catalogPersonalityPreset struct {
	ID            string         `json:"id"`
	Label         string         `json:"label"`
	LabelEn       string         `json:"label_en,omitempty"`
	Description   string         `json:"description,omitempty"`
	DescriptionEn string         `json:"description_en,omitempty"`
	Values        personalityDTO `json:"values"`
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
	interestLabels     map[string]struct{}
	capabilityLabels   map[string]struct{}
	interestLabelToEn  map[string]string
	capabilityLabelToEn map[string]string
	bioTemplateIDs     map[string]catalogTextTemplate
	greetingTemplateIDs map[string]catalogTextTemplate
}

func (c *agentCardCatalogs) sets() agentCardCatalogSets {
	s := agentCardCatalogSets{
		interestLabels:       map[string]struct{}{},
		capabilityLabels:     map[string]struct{}{},
		interestLabelToEn:    map[string]string{},
		capabilityLabelToEn:  map[string]string{},
		bioTemplateIDs:       map[string]catalogTextTemplate{},
		greetingTemplateIDs:  map[string]catalogTextTemplate{},
	}
	for _, it := range c.Interests {
		lbl := strings.TrimSpace(it.Label)
		if lbl != "" {
			s.interestLabels[lbl] = struct{}{}
			if strings.TrimSpace(it.LabelEn) != "" {
				s.interestLabelToEn[lbl] = strings.TrimSpace(it.LabelEn)
			}
		}
	}
	for _, it := range c.Capabilities {
		lbl := strings.TrimSpace(it.Label)
		if lbl != "" {
			s.capabilityLabels[lbl] = struct{}{}
			if strings.TrimSpace(it.LabelEn) != "" {
				s.capabilityLabelToEn[lbl] = strings.TrimSpace(it.LabelEn)
			}
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

func cleanCatalogList(list []string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		out = append(out, v)
	}
	return out
}

func mapCatalogList(list []string, m map[string]string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if mapped := strings.TrimSpace(m[v]); mapped != "" {
			out = append(out, mapped)
		} else {
			out = append(out, v)
		}
	}
	return out
}

func renderCatalogTemplate(tmpl string, name string, interests []string, capabilities []string, joiner string) string {
	out := tmpl
	out = strings.ReplaceAll(out, "{name}", strings.TrimSpace(name))
	out = strings.ReplaceAll(out, "{interests}", strings.Join(cleanCatalogList(interests), joiner))
	out = strings.ReplaceAll(out, "{capabilities}", strings.Join(cleanCatalogList(capabilities), joiner))
	return strings.TrimSpace(out)
}

type catalogRenderVars struct {
	name           string
	interestsZh    []string
	capabilitiesZh []string
	interestsEn    []string
	capabilitiesEn []string
}

func matchesRenderedTemplate(text string, templates []catalogTextTemplate, vars catalogRenderVars) bool {
	want := strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if want == "" {
		return false
	}
	for _, t := range templates {
		if strings.TrimSpace(t.Template) != "" {
			got := renderCatalogTemplate(t.Template, vars.name, vars.interestsZh, vars.capabilitiesZh, "、")
			got = strings.Join(strings.Fields(strings.TrimSpace(got)), " ")
			if got != "" && got == want {
				return true
			}
		}
		if strings.TrimSpace(t.TemplateEn) != "" {
			got := renderCatalogTemplate(t.TemplateEn, vars.name, vars.interestsEn, vars.capabilitiesEn, ", ")
			got = strings.Join(strings.Fields(strings.TrimSpace(got)), " ")
			if got != "" && got == want {
				return true
			}
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
	vars := catalogRenderVars{
		name:           name,
		interestsZh:    interests,
		capabilitiesZh: capabilities,
		interestsEn:    mapCatalogList(interests, sets.interestLabelToEn),
		capabilitiesEn: mapCatalogList(capabilities, sets.capabilityLabelToEn),
	}
	if !matchesRenderedTemplate(bio, c.BioTemplates, vars) {
		return false
	}
	if !matchesRenderedTemplate(greeting, c.GreetingTemplates, vars) {
		return false
	}
	return true
}
