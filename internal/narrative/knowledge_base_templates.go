package narrative

// PR2 (sub-issue-3): KnowledgeBase template management extracted from
// knowledge_base.go. The KnowledgeBase type is a small, focused
// component that handles causal template registration, lookup, and
// chain matching. It is used by both NarrativeEngine.MatchChains and
// the Bundle API endpoint.
//
// Per internal/narrative/AGENTS.md:
//   - MatchChains output includes FavoredSectors + AvoidedSectors
//     (directionality), derived via classifySectorsByImpact.
//   - collectAffectedSectors + classifySectorsByImpact are unexported
//     helpers; they stay co-located with MatchChains for clarity.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"sync"
)

// sha256Hex returns the lowercase hex SHA-256 digest of data.
func sha256Hex(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// TemplateAuditHook is called when a template is registered (added or updated).
// The templateID identifies the template; contentHash is a SHA-256 hex digest
// of the canonical JSON representation.
type TemplateAuditHook func(templateID, contentHash string)

// KnowledgeBase holds causal templates and produces instantiated chains.
type KnowledgeBase struct {
	mu        sync.RWMutex
	templates map[string]CausalTemplate
	auditHook TemplateAuditHook
}

// SetTemplateAuditHook wires a mutation audit callback (Stage 2.3a).
func (kb *KnowledgeBase) SetTemplateAuditHook(hook TemplateAuditHook) {
	kb.mu.Lock()
	kb.auditHook = hook
	kb.mu.Unlock()
}

// NewKnowledgeBase creates a knowledge base preloaded with default templates.
func NewKnowledgeBase() *KnowledgeBase {
	kb := &KnowledgeBase{
		templates: make(map[string]CausalTemplate),
	}
	for _, t := range DefaultTemplates() {
		kb.templates[t.ID] = t
	}
	return kb
}

// RegisterTemplate adds or replaces a template.
func (kb *KnowledgeBase) RegisterTemplate(t CausalTemplate) {
	kb.mu.Lock()
	kb.templates[t.ID] = t
	hook := kb.auditHook
	kb.mu.Unlock()

	// Stage 2.3a: fire template change audit hook outside the lock.
	if hook != nil {
		// SHA-256 hex of canonical JSON repr of the template.
		raw, _ := json.Marshal(t)
		hash := sha256Hex(raw)
		hook(t.ID, hash)
	}
}

// GetTemplate returns a template by ID.
func (kb *KnowledgeBase) GetTemplate(id string) (CausalTemplate, bool) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	t, ok := kb.templates[id]
	return t, ok
}

// GetTemplateByTheme returns the first template matching the trigger theme.
func (kb *KnowledgeBase) GetTemplateByTheme(theme string) (CausalTemplate, bool) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	for _, t := range kb.templates {
		if strings.EqualFold(t.TriggerTheme, theme) {
			return t, true
		}
	}
	return CausalTemplate{}, false
}

// ListTemplates returns all registered templates.
func (kb *KnowledgeBase) ListTemplates() []CausalTemplate {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	out := make([]CausalTemplate, 0, len(kb.templates))
	for _, t := range kb.templates {
		out = append(out, t)
	}
	return out
}

// MatchChains finds all causal templates that match a given event and instantiates chains.
func (kb *KnowledgeBase) MatchChains(event NarrativeEvent) []CausalChain {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	var chains []CausalChain
	for _, tmpl := range kb.templates {
		if !strings.EqualFold(tmpl.TriggerTheme, event.Theme) {
			continue
		}
		if tmpl.RequiredRegion != "" && !strings.EqualFold(tmpl.RequiredRegion, event.Region) {
			continue
		}

		score := event.Confidence * tmpl.HistoricalHitRate
		steps := make([]CausalStep, len(tmpl.Steps))
		copy(steps, tmpl.Steps)

		affected := collectAffectedSectors(tmpl.Steps)
		favored, avoided := classifySectorsByImpact(tmpl.Steps)

		chains = append(chains, CausalChain{
			EventID:         event.ID,
			TemplateID:      tmpl.ID,
			TriggerTheme:    tmpl.TriggerTheme,
			AffectedSectors: affected,
			FavoredSectors:  favored,
			AvoidedSectors:  avoided,
			Steps:           steps,
			Score:           score,
			DetectedAt:      event.Timestamp,
		})
	}
	return chains
}

// collectAffectedSectors aggregates unique affected sectors from all causal steps.
func collectAffectedSectors(steps []CausalStep) []string {
	seen := make(map[string]struct{})
	for _, step := range steps {
		for _, s := range step.Affected {
			seen[s] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	return out
}

// classifySectorsByImpact groups affected sectors into favored (net positive
// impact) and avoided (net negative impact) based on the cumulative Impact
// across all steps that mention each sector.
func classifySectorsByImpact(steps []CausalStep) (favored, avoided []string) {
	netImpact := make(map[string]float64)
	for _, step := range steps {
		for _, s := range step.Affected {
			netImpact[s] += step.Impact
		}
	}
	for s, impact := range netImpact {
		if impact > 0 {
			favored = append(favored, s)
		} else if impact < 0 {
			avoided = append(avoided, s)
		}
	}
	return favored, avoided
}
