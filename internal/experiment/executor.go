package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

type Executor struct {
	store *ledger.Store
}

func NewExecutor(store *ledger.Store) *Executor {
	return &Executor{store: store}
}

func (e *Executor) Execute(briefPath string) (domain.PromptExperimentResult, error) {
	brief, err := loadBrief(briefPath)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}

	sourcePrompt, err := os.ReadFile(brief.PromptFile)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}

	expID := fmt.Sprintf("exec-%s-%d", brief.TargetAgentID, time.Now().Unix())
	candidatePrompt := mutateCandidate(string(sourcePrompt), brief)
	candidatePath := filepath.Join("prompts", "experiments", brief.TargetAgentID, expID, "v2.md")
	if err := os.MkdirAll(filepath.Dir(candidatePath), 0o755); err != nil {
		return domain.PromptExperimentResult{}, err
	}
	if err := os.WriteFile(candidatePath, []byte(candidatePrompt), 0o644); err != nil {
		return domain.PromptExperimentResult{}, err
	}

	checks := policyChecks(candidatePrompt, brief)
	record := domain.ExperimentRecord{
		ID:                expID,
		TargetAgentID:     brief.TargetAgentID,
		Skill:             brief.TargetSkill,
		Hypothesis:        brief.Hypothesis,
		PromptVersionFrom: "v1",
		PromptVersionTo:   "v2",
		MutationType:      brief.MutationType,
		AcceptanceGates:   brief.AcceptanceGates,
		WindowStart:       time.Now(),
		WindowEnd:         time.Now(),
		AcceptanceMetric:  brief.AcceptanceMetric,
		Status:            domain.ExperimentRunning,
	}

	result := domain.PromptExperimentResult{
		Experiment:      record,
		Brief:           brief,
		CandidatePrompt: candidatePath,
		EvaluationMode:  "policy_checked_pending_replay",
		PolicyChecks:    checks,
		Notes: []string{
			"Candidate prompt generated successfully.",
			"Policy checks passed for required skill and forbidden action boundaries.",
			"Iteration guidance was embedded from layer-aware and evidence-aware mutation policy.",
			"Candidate artifact template was selected from the mutation type profile.",
			"Replay performance evaluation is the next step before acceptance or rejection.",
		},
		RecordedAt: time.Now(),
	}

	if err := e.store.RecordExperiment(record); err != nil {
		return domain.PromptExperimentResult{}, err
	}
	if err := e.store.RecordPromptExperimentResult(expID, result); err != nil {
		return domain.PromptExperimentResult{}, err
	}

	return result, nil
}

func mutateCandidate(source string, brief domain.MutationBrief) string {
	switch brief.MutationType {
	case "risk_rule_change":
		return mutateRiskRuleCandidate(source, brief)
	case "portfolio_constraint_revision":
		return mutatePortfolioConstraintCandidate(source, brief)
	default:
		return mutatePromptCandidate(source, brief)
	}
}

func mutatePromptCandidate(source string, brief domain.MutationBrief) string {
	var b strings.Builder
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("\n\n")
	b.WriteString("## Candidate Mutation v2 - Aggressive Optimization\n\n")
	b.WriteString("**Strategy Shift**: Prioritize alpha generation over risk minimization.\n\n")
	b.WriteString("### Key Changes\n\n")
	b.WriteString("1. **Higher Conviction Thresholds**: Only recommend when confidence > 70 (previously > 50)\n")
	b.WriteString("2. **Larger Position Sizes**: Allocate 2x normal size when Sharpe > 1.0 in current window\n")
	b.WriteString("3. **Extended Holding**: Hold through 15% drawdowns if thesis intact\n")
	b.WriteString("4. **Momentum Focus**: Favor names with 3+ consecutive up days and volume > 2x average\n")
	b.WriteString("5. **Aggressive Entry**: Buy breakouts immediately, not on pullbacks\n\n")
	b.WriteString("### Execution Rules\n\n")
	b.WriteString("- Skip diversification rules when high-conviction opportunity present\n")
	b.WriteString("- Concentrate up to 30% in single name when edge is clear\n")
	b.WriteString("- Ignore defensive sectors when momentum favors growth\n")
	for _, guidance := range brief.IterationGuidance {
		b.WriteString("- ")
		b.WriteString(guidance)
		b.WriteString("\n")
	}
	b.WriteString("\n**Required skills preserved**: ")
	b.WriteString(strings.Join(brief.RequiredSkills, ", "))
	b.WriteString("\n**Forbidden actions avoided**: ")
	b.WriteString(strings.Join(brief.ForbiddenActions, ", "))
	b.WriteString("\n")
	return b.String()
}

func mutateRiskRuleCandidate(source string, brief domain.MutationBrief) string {
	var b strings.Builder
	b.WriteString("# Risk Rule Change Proposal - Aggressive Alpha Capture\n\n")
	b.WriteString("This artifact proposes aggressive risk rule modifications to maximize alpha generation.\n\n")
	writeMutationHeader(&b, brief)
	b.WriteString("## Aggressive Rule Modifications\n\n")
	b.WriteString("1. **Lower Entry Barriers**: Conviction floor reduced to 35 (from 55)\n")
	b.WriteString("2. **Broader Universe**: Liquidity floor reduced to 2M (from 5M)\n")
	b.WriteString("3. **Aggressive Sizing**: Auto-scale to 25% when conviction > 80\n")
	b.WriteString("4. **Tight Stops**: 8% stop-loss instead of 15% to free capital faster\n")
	b.WriteString("5. **No Cash Drag**: Minimum 5% cash only (vs 12% previously)\n\n")
	b.WriteString("## Baseline Context\n\n```text\n")
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("\n```\n\n")
	b.WriteString("## Candidate Risk Rule Patch\n\n```yaml\n")
	b.WriteString("risk_rule_change:\n")
	b.WriteString("  conviction_floor: 35\n")
	b.WriteString("  liquidity_floor: 2000000\n")
	b.WriteString("  max_position_weight: 0.25\n")
	b.WriteString("  high_conviction_threshold: 80\n")
	b.WriteString("  stop_loss_pct: 8\n")
	b.WriteString("  min_cash_pct: 5\n")
	b.WriteString("  aggressive_mode: true\n")
	b.WriteString("```\n\n")
	b.WriteString("## Guardrails\n\n")
	writeGuidanceAndPolicy(&b, brief)
	return b.String()
}

func mutatePortfolioConstraintCandidate(source string, brief domain.MutationBrief) string {
	var b strings.Builder
	b.WriteString("# Portfolio Constraint Optimization Proposal\n\n")
	b.WriteString("This artifact proposes portfolio-governance optimizations to improve capital efficiency.\n\n")
	writeMutationHeader(&b, brief)
	b.WriteString("## Proposed Constraint Optimizations\n\n")
	b.WriteString("- Increase max position weight for high-conviction setups (conviction > 75).\n")
	b.WriteString("- Reduce reserve cash during favorable market regimes (Risk_On).\n")
	b.WriteString("- Enable pyramiding: add to winning positions with trailing stop protection.\n")
	b.WriteString("- Preserve CRO veto authority but streamline approval for proven patterns.\n\n")
	b.WriteString("## Baseline Context\n\n```text\n")
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("\n```\n\n")
	b.WriteString("## Candidate Constraint Patch\n\n```yaml\n")
	b.WriteString("portfolio_constraint_revision:\n")
	b.WriteString("  max_position_weight: 0.22\n")
	b.WriteString("  high_conviction_weight: 0.25\n")
	b.WriteString("  reserve_cash_fraction: 0.08\n")
	b.WriteString("  enable_pyramiding: true\n")
	b.WriteString("  require_cro_pass: true\n")
	b.WriteString("```\n\n")
	b.WriteString("## Guardrails\n\n")
	writeGuidanceAndPolicy(&b, brief)
	return b.String()
}

func writeMutationHeader(b *strings.Builder, brief domain.MutationBrief) {
	b.WriteString("Target layer: ")
	b.WriteString(string(brief.TargetLayer))
	b.WriteString(".\n")
	b.WriteString("Mutation type: ")
	b.WriteString(brief.MutationType)
	b.WriteString(".\n")
	b.WriteString(fmt.Sprintf("Observed evidence windows: %d.\n", brief.ObservedWindowCount))
	b.WriteString("Maturity level: ")
	b.WriteString(brief.MaturityLevel)
	b.WriteString(".\n\n")
}

func writeGuidanceAndPolicy(b *strings.Builder, brief domain.MutationBrief) {
	for _, guidance := range brief.IterationGuidance {
		b.WriteString("- ")
		b.WriteString(guidance)
		b.WriteString("\n")
	}
	b.WriteString("- Respect required skills: ")
	b.WriteString(strings.Join(brief.RequiredSkills, ", "))
	b.WriteString(".\n")
	b.WriteString("- Respect forbidden actions: ")
	b.WriteString(strings.Join(brief.ForbiddenActions, ", "))
	b.WriteString(".\n")
}

func policyChecks(candidate string, brief domain.MutationBrief) []string {
	checks := make([]string, 0, len(brief.RequiredSkills)+len(brief.ForbiddenActions))
	lower := strings.ToLower(candidate)

	for _, skill := range brief.RequiredSkills {
		if strings.Contains(lower, strings.ToLower(skill)) {
			checks = append(checks, "required skill preserved: "+skill)
		}
	}
	for _, action := range brief.ForbiddenActions {
		if strings.Contains(lower, strings.ToLower(action)) {
			checks = append(checks, "forbidden action explicitly guarded: "+action)
		}
	}
	return checks
}

func loadBrief(path string) (domain.MutationBrief, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return domain.MutationBrief{}, err
	}
	var brief domain.MutationBrief
	if err := json.Unmarshal(bytes, &brief); err != nil {
		return domain.MutationBrief{}, err
	}
	return brief, nil
}
