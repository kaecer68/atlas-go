package eventlogic

import (
	"fmt"
	"sync"
)

type ValidationContext struct {
	NumericFields map[string]float64
	StringFields  map[string]string
}
type RuleValidator struct {
	registry *RuleRegistry
	mu       sync.RWMutex
}

func NewValidator(r *RuleRegistry) *RuleValidator { return &RuleValidator{registry: r} }
func (v *RuleValidator) EvaluateCondition(c Condition, ctx *ValidationContext) bool {
	switch c.Operator {
	case "eq":
		if c.StringValue != "" {
			a, ok := ctx.StringFields[c.Field]
			return ok && a == c.StringValue
		}
		a, ok := ctx.NumericFields[c.Field]
		return ok && a == c.Value
	case "neq":
		if c.StringValue != "" {
			a, ok := ctx.StringFields[c.Field]
			return !ok || a != c.StringValue
		}
		a, ok := ctx.NumericFields[c.Field]
		return !ok || a != c.Value
	case "gt":
		a, ok := ctx.NumericFields[c.Field]
		return ok && a > c.Value
	case "lt":
		a, ok := ctx.NumericFields[c.Field]
		return ok && a < c.Value
	case "gte":
		a, ok := ctx.NumericFields[c.Field]
		return ok && a >= c.Value
	case "lte":
		a, ok := ctx.NumericFields[c.Field]
		return ok && a <= c.Value
	default:
		return false
	}
}

func (v *RuleValidator) EvaluateRule(r *EventRule, ctx *ValidationContext) bool {
	if len(r.Conditions) == 0 {
		return false
	}
	for _, c := range r.Conditions {
		if !v.EvaluateCondition(c, ctx) {
			return false
		}
	}
	return true
}

func (v *RuleValidator) RecordOutcome(id string, hit bool) error {
	r, ok := v.registry.GetByID(id)
	if !ok {
		return fmt.Errorf("eventlogic: rule %s not found", id)
	}
	v.mu.Lock()
	r.TotalTests++
	if hit {
		r.TotalHits++
	}
	if r.TotalTests > 0 {
		r.HitRate = float64(r.TotalHits) / float64(r.TotalTests)
	}
	v.mu.Unlock()
	return v.registry.Update(r)
}

func (v *RuleValidator) ValidateAll(ctx *ValidationContext, dir string) []ValidationResult {
	rules := v.registry.ListActive()
	rs := make([]ValidationResult, 0, len(rules))
	for _, r := range rules {
		if !v.EvaluateRule(r, ctx) {
			continue
		}
		hit := r.Direction == dir
		_ = v.RecordOutcome(r.ID, hit)
		rs = append(rs, ValidationResult{RuleID: r.ID, Fired: true, WasHit: hit})
	}
	return rs
}

type ValidationResult struct {
	RuleID string `json:"rule_id"`
	Fired  bool   `json:"fired"`
	WasHit bool   `json:"was_hit"`
	Error  string `json:"error,omitempty"`
}
