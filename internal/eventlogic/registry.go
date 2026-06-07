package eventlogic

import (
	"fmt"
	"sync"
	"time"
)

// RuleRegistry stores and manages all event rules with thread-safe access.
type RuleRegistry struct {
	mu    sync.RWMutex
	rules map[string]*EventRule
}

// NewRegistry creates a new RuleRegistry pre-seeded with 6 built-in rules.
func NewRegistry() *RuleRegistry {
	reg := &RuleRegistry{
		rules: make(map[string]*EventRule),
	}
	reg.seedRules()
	return reg
}

// List returns all rules in the registry.
func (r *RuleRegistry) List() []*EventRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*EventRule, 0, len(r.rules))
	for _, rule := range r.rules {
		result = append(result, rule)
	}
	return result
}

// GetByID returns a rule by its ID and whether it was found.
func (r *RuleRegistry) GetByID(id string) (*EventRule, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rule, ok := r.rules[id]
	return rule, ok
}

// ListActive returns only rules with Status == "active".
func (r *RuleRegistry) ListActive() []*EventRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*EventRule
	for _, rule := range r.rules {
		if rule.Status == StatusActive {
			result = append(result, rule)
		}
	}
	return result
}

// ListExpired returns only rules with Status == "expired".
func (r *RuleRegistry) ListExpired() []*EventRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*EventRule
	for _, rule := range r.rules {
		if rule.Status == StatusExpired {
			result = append(result, rule)
		}
	}
	return result
}

// Add inserts a new rule into the registry.
// Returns an error if a rule with the same ID already exists.
func (r *RuleRegistry) Add(rule *EventRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rules[rule.ID]; exists {
		return fmt.Errorf("eventlogic: rule %s already exists", rule.ID)
	}
	r.rules[rule.ID] = rule
	return nil
}

// Update modifies an existing rule in the registry.
// Sets the UpdatedAt timestamp to now. Returns an error if the rule is not found.
func (r *RuleRegistry) Update(rule *EventRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rules[rule.ID]; !exists {
		return fmt.Errorf("eventlogic: rule %s not found", rule.ID)
	}
	rule.UpdatedAt = time.Now()
	r.rules[rule.ID] = rule
	return nil
}

// Delete removes a rule from the registry by ID.
// Returns an error if the rule is not found.
func (r *RuleRegistry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rules[id]; !exists {
		return fmt.Errorf("eventlogic: rule %s not found", id)
	}
	delete(r.rules, id)
	return nil
}

// Count returns the total number of rules in the registry.
func (r *RuleRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.rules)
}

// CountActive returns the number of active rules in the registry.
func (r *RuleRegistry) CountActive() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, rule := range r.rules {
		if rule.Status == StatusActive {
			count++
		}
	}
	return count
}

// seedRules registers the 6 built-in event rules.
func (r *RuleRegistry) seedRules() {
	seeds := []*EventRule{
		{
			ID:      "sox-foreignflow-semiconductor",
			Pattern: "SOX index > +3% AND foreign capital consecutive buy >= 3 days → semiconductor up",
			Conditions: []Condition{
				{Field: "SOXIndex.ChangePct", Operator: "gt", Value: 3.0},
				{Field: "ForeignInvestorNet.ConsecutiveDays", Operator: "gte", Value: 3},
			},
			AffectedSectors:  []string{"semiconductor"},
			Direction:        DirUp,
			HitRate:          0.5,
			ConfidenceSource: SourceManual,
			Status:           StatusActive,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:      "usmarket-taiwan-lag",
			Pattern: "US market close direction → TW market open direction (timezone arbitrage)",
			Conditions: []Condition{
				{Field: "USMarketClose.Direction", Operator: "eq", Value: 1},
			},
			AffectedSectors:  []string{"semiconductor", "ai_supply_chain", "electronics"},
			Direction:        DirUp,
			HitRate:          0.5,
			ConfidenceSource: SourceManual,
			Status:           StatusActive,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:      "dxy-strong-export-boost",
			Pattern: "DXY strengthening + BDI rising → shipping benefits",
			Conditions: []Condition{
				{Field: "DXY.ChangePct", Operator: "gt", Value: 0.5},
				{Field: "Bdi.ChangePct", Operator: "gt", Value: 5.0},
			},
			AffectedSectors:  []string{"shipping"},
			Direction:        DirUp,
			HitRate:          0.5,
			ConfidenceSource: SourceManual,
			Status:           StatusActive,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:      "foreign-outflow-bearish",
			Pattern: "Foreign capital consecutive sell >= 5 days → all sectors bearish",
			Conditions: []Condition{
				{Field: "ForeignInvestorNet.ConsecutiveDays", Operator: "gte", Value: 5},
				{Field: "ForeignInvestorNet.Direction", Operator: "eq", Value: -1},
			},
			AffectedSectors:  []string{"*"},
			Direction:        DirDown,
			HitRate:          0.5,
			ConfidenceSource: SourceManual,
			Status:           StatusActive,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:      "nvidia-earnings-ai-chain",
			Pattern: "NVIDIA earnings beat → AI supply chain linkage",
			Conditions: []Condition{
				{Field: "NarrativeTheme", Operator: "eq", StringValue: "AI_capex_surge"},
			},
			AffectedSectors:  []string{"ai_supply_chain", "semiconductor", "electronics"},
			Direction:        DirUp,
			HitRate:          0.5,
			ConfidenceSource: SourceManual,
			Status:           StatusActive,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
		{
			ID:      "usd-twd-managed-float",
			Pattern: "USD/TWD breaks 32.0 → export stocks benefit (TWD depreciation good for exports)",
			Conditions: []Condition{
				{Field: "USD_TWD.Value", Operator: "gt", Value: 32.0},
			},
			AffectedSectors:  []string{"electronics", "semiconductor", "shipping"},
			Direction:        DirUp,
			HitRate:          0.5,
			ConfidenceSource: SourceManual,
			Status:           StatusActive,
			CreatedAt:        time.Now(),
			UpdatedAt:        time.Now(),
		},
	}

	for _, rule := range seeds {
		r.rules[rule.ID] = rule
	}
}
