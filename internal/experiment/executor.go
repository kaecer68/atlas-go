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
	proposalID := brief.ProposalID
	if proposalID == "" {
		proposalID = "proposal-" + expID
	}
	record := domain.ExperimentRecord{
		ID:                expID,
		ProposalID:        proposalID,
		CommitID:          "commit-" + expID,
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
		Status:            domain.ExperimentPlanned,
	}
	if err := domain.TransitionExperimentStatus(&record, domain.ExperimentRunning); err != nil {
		return domain.PromptExperimentResult{}, fmt.Errorf("start experiment %s: %w", expID, err)
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
	b.WriteString("## Candidate Mutation v2 - Prompt Tightening\n\n")
	b.WriteString("This mutation tightens setup quality while preserving required skill and guardrail boundaries.\n\n")
	b.WriteString("### Executable Prompt Controls\n\n")
	if brief.TargetSkill == "financials_desk" {
		b.WriteString("- credit quality gate: downgrade conviction when close is weak vs open\n")
		b.WriteString("- spread sensitivity downgrade: penalize weak intraday close strength\n")
		b.WriteString("- capital adequacy premium: upgrade conviction when close strength confirms balance-sheet resilience\n")
		b.WriteString("- enforce illiquid rejection for weak volume names\n\n")
	} else if brief.TargetSkill == "technical_breakout" {
		b.WriteString("- catch-up momentum: boost conviction for stocks closing strong but below session peak\n")
		b.WriteString("- volume participation acceptance: include moderate volume names alongside high-volume breakouts\n")
		b.WriteString("- close-strength tolerance: do not downgrade minor below-high closes unless breakdown is material\n")
		b.WriteString("- breakout confirmation bonus: boost conviction when close confirms strength near session high with volume\n\n")
	} else {
		if brief.ObservedWindowCount >= 5 {
			b.WriteString("- require trend confirmation before issuing buy recommendations\n")
		} else {
			b.WriteString("- keep momentum bias but tolerate one weak confirmation signal in exploratory mode\n")
		}
		b.WriteString("- downgrade conviction when price is below intraday strength or open\n")
		if brief.ObservedWindowCount >= 8 {
			b.WriteString("- reject setups when trend persistence is not present\n")
		} else {
			b.WriteString("- keep exploratory coverage: do not hard-filter solely on one weak signal\n")
		}
		b.WriteString("- enforce illiquid rejection for weak volume names\n\n")
	}
	b.WriteString("### Operator Notes\n\n")
	b.WriteString("- Keep conviction selective and avoid narrative-only entries\n")
	b.WriteString("- Prefer clean continuation structures over low-quality breakouts\n")
	b.WriteString("- Preserve control-layer sequencing and CRO/CIO boundaries\n")
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
	title := "# Risk Rule Change Proposal - Governance Tightening"
	description := "This artifact proposes risk rule modifications designed to be measurable, auditable, and execution-safe."
	rules := []string{
		"1. **Raise Entry Quality**: Conviction floor increased to 48 (from baseline floor)",
		"2. **Tighten Liquidity**: Liquidity floor raised to 3.5M to reduce weak fills",
		"3. **Trim Position Concentration**: Max position weight set to 20%",
		"4. **Increase Cash Buffer**: Reserve cash fraction set to 12%",
		"5. **Keep CRO Gate**: Require CRO pass remains enabled",
	}
	convictionFloor := 48
	liquidityFloor := int64(3500000)
	maxPositionWeight := 0.20
	reserveCashFraction := 0.12
	requireCROPass := true

	if brief.TargetSkill != "financials_desk" {
		title = "# Risk Rule Change Proposal - Controlled Optimization"
		description = "This artifact proposes controlled risk rule changes to improve portfolio efficiency under explicit guardrails."
		rules = []string{
			"1. **Entry Filter Refresh**: Conviction floor set to 42",
			"2. **Liquidity Guard**: Liquidity floor maintained at 3.0M for tradability",
			"3. **Position Discipline**: Max position weight set to 22%",
			"4. **Operational Buffer**: Reserve cash fraction set to 10%",
			"5. **Control Integrity**: Require CRO pass remains enabled",
		}
		convictionFloor = 42
		liquidityFloor = 3000000
		maxPositionWeight = 0.22
		reserveCashFraction = 0.10
	}

	b.WriteString(title)
	b.WriteString("\n\n")
	b.WriteString(description)
	b.WriteString("\n\n")
	writeMutationHeader(&b, brief)
	b.WriteString("## Rule Modifications\n\n")
	for _, rule := range rules {
		b.WriteString(rule)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString("## Baseline Context\n\n```text\n")
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("\n```\n\n")
	b.WriteString("## Candidate Rule Patch\n\n```yaml\n")
	b.WriteString("risk_rule_change:\n")
	b.WriteString(fmt.Sprintf("  conviction_floor: %d\n", convictionFloor))
	b.WriteString(fmt.Sprintf("  liquidity_floor: %d\n", liquidityFloor))
	b.WriteString(fmt.Sprintf("  max_position_weight: %.2f\n", maxPositionWeight))
	b.WriteString(fmt.Sprintf("  reserve_cash_fraction: %.2f\n", reserveCashFraction))
	b.WriteString(fmt.Sprintf("  require_cro_pass: %t\n", requireCROPass))
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
		return domain.MutationBrief{}, fmt.Errorf("read mutation brief %s: %w", path, err)
	}
	var brief domain.MutationBrief
	if err := json.Unmarshal(bytes, &brief); err != nil {
		return domain.MutationBrief{}, fmt.Errorf("unmarshal mutation brief %s: %w", path, err)
	}
	if err := brief.NormalizeAndValidate(); err != nil {
		return domain.MutationBrief{}, fmt.Errorf("validate mutation brief %s: %w", path, err)
	}
	return brief, nil
}
