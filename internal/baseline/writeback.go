// Charter A/B writeback (Phase C4) — the evolution loop.
//
// cmd/experimental/charter-ab produces one CharterDelta per A/B step
// (internal/charter/delta.go). WritebackCharter records those deltas into
// the baseline policy so the constitution becomes a verifiable, auditable
// evolution mechanism:
//
//	significant_enable → the delta's switch is mapped onto
//	                     Constraints/ExecutionPolicy and recorded as a
//	                     charter_constraint promotion (status=promoted).
//	directional_watch  → runtime values are NOT overridden; the delta is
//	                     recorded as a charter_delta_recorded promotion
//	                     (status=watch) with full evidence (p value, BCa CI,
//	                     window).
//	inert / degenerate  → recorded as a charter_delta_recorded finding
//	                     (status=recorded); no constraints are written.
//
// Every appended record bumps Version exactly once. This keeps the package
// invariant version == len(promotions)+1 that rollback.go's RevertLast /
// GetPromotionHistory rely on (each promotion maps to one version step), so
// a findings-only writeback must still bump to stay revertable.
//
// The writeback is idempotent: a delta whose ExperimentID
// (charter-ab-step-<n>) is already present in Promotions is skipped, so
// re-running a writeback (or re-running the A/B and writing the fresh
// reports back) never duplicates records.
package baseline

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/charter"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
)

// Mutation types and statuses used by charter writeback records.
const (
	MutationTypeCharterConstraint = "charter_constraint"     // significant_enable → runtime constraints
	MutationTypeCharterDelta      = "charter_delta_recorded" // watch / finding record
	StatusPromoted                = "promoted"               // significant_enable
	StatusWatch                   = "watch"                  // directional_watch (recorded, not enforced)
	StatusRecorded                = "recorded"               // inert / degenerate finding
)

// WritebackCharter applies charter A/B deltas to the baseline policy at
// m.path. See the package comment for the per-verdict semantics.
//
// The returned Policy is the policy as written (or, when every delta was
// already recorded, the policy as loaded — the file is not touched).
func (m *Manager) WritebackCharter(deltas []charter.CharterDelta, sourceDir string) (Policy, error) {
	if m.path == "" {
		return Policy{}, fmt.Errorf("baseline policy path must not be empty")
	}
	policy, err := LoadStrict(m.path)
	if err != nil {
		return Policy{}, fmt.Errorf("load policy for charter writeback: %w", err)
	}
	next := policy
	if next.Promotions == nil {
		next.Promotions = []PromotionRecord{}
	}

	appended := 0
	for i := range deltas {
		d := deltas[i]
		// Idempotency: a delta already recorded must not be written twice.
		if hasExperimentID(next.Promotions, d.ExperimentID()) {
			continue
		}
		record, err := buildCharterRecord(&next, d, sourceDir)
		if err != nil {
			return Policy{}, fmt.Errorf("build charter record for step %d: %w", d.Step, err)
		}
		next.Version++
		record.VersionAfter = next.Version
		next.Promotions = append(next.Promotions, record)
		appended++
	}

	if appended == 0 {
		return next, nil // nothing new — do not touch the file
	}
	if err := SaveWithLock(m.path, next); err != nil {
		return Policy{}, fmt.Errorf("save policy after charter writeback: %w", err)
	}
	return next, nil
}

// buildCharterRecord converts one delta into a PromotionRecord following the
// writeback semantics. For significant_enable it mutates policy in place
// (constraint patch) and snapshots the resulting constraints.
func buildCharterRecord(policy *Policy, d charter.CharterDelta, sourceDir string) (PromotionRecord, error) {
	evidence, err := json.Marshal(d)
	if err != nil {
		return PromotionRecord{}, fmt.Errorf("marshal delta evidence: %w", err)
	}
	record := PromotionRecord{
		ExperimentID:   d.ExperimentID(),
		TargetAgentID:  "charter", // the charter layer, not an agent
		TargetSkill:    d.Switch,
		CandidatePath:  filepath.Join(sourceDir, fmt.Sprintf("step-%d.json", d.Step)),
		PromotedAt:     time.Now(),
		PromptSnapshot: string(evidence),
	}

	switch d.Verdict {
	case charter.VerdictSignificantEnable:
		applyCharterConstraint(policy, d)
		snapshot := policy.Constraints
		record.MutationType = MutationTypeCharterConstraint
		record.Status = StatusPromoted
		record.ConstraintsSnapshot = &snapshot
	case charter.VerdictDirectionalWatch:
		record.MutationType = MutationTypeCharterDelta
		record.Status = StatusWatch
	default: // inert / degenerate findings
		record.MutationType = MutationTypeCharterDelta
		record.Status = StatusRecorded
	}
	return record, nil
}

// applyCharterConstraint maps a significant_enable delta's switch onto its
// baseline policy fields. Switches whose behavior lives in methodology-level
// config (PeriodOnly / StrategyFilter / MacroFlow) have no direct policy
// field; their promotion record carries the evidence instead and no runtime
// value changes.
func applyCharterConstraint(policy *Policy, d charter.CharterDelta) {
	switch d.Switch {
	case "CashReserve":
		// The charter's cash reserve is period-dependent (methodology_rules
		// cash_reserve_pct). Promote the strictest period reserve as the
		// static floor so the protection applies across all periods.
		rules := config.TryLoadMethodologyRules("configs/methodology_rules.yaml")
		maxReserve := 0.0
		for _, p := range allPeriods() {
			if r := rules.GetCashReserve(string(p)); r > maxReserve {
				maxReserve = r
			}
		}
		if maxReserve > 0 {
			policy.Constraints.ReserveCashFraction = maxReserve / 100.0
		}
	case "ConvictionFloor":
		// The charter adds up to +20 conviction points (black_swan) on top of
		// the base floor (charter.ConvictionFloorDelta). Promote the strictest
		// delta as the static floor.
		policy.Constraints.MinRecommendationConviction += charter.ConvictionFloorDelta(domain.PeriodBlackSwan)
		policy.ExecutionPolicy = ExecutionPolicyFromConstraints(policy.Constraints)
	}
}

func allPeriods() []domain.MarketPeriod {
	return []domain.MarketPeriod{
		domain.PeriodDownturn,
		domain.PeriodTurnaroundUp,
		domain.PeriodBull,
		domain.PeriodPlateau,
		domain.PeriodConsolidation,
		domain.PeriodTurnaroundDown,
		domain.PeriodBlackSwan,
	}
}

func hasExperimentID(promotions []PromotionRecord, id string) bool {
	for _, p := range promotions {
		if p.ExperimentID == id {
			return true
		}
	}
	return false
}
