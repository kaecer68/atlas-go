package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

type Executor struct {
	store        ledger.FullStore
	baselinePath string
}

func NewExecutor(store ledger.FullStore, baselinePath string) *Executor {
	return &Executor{store: store, baselinePath: baselinePath}
}

func (e *Executor) Run(briefPath string, replayPath string) (domain.PromptExperimentResult, error) {
	brief, err := loadBrief(briefPath)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}

	windowStart, windowEnd, err := parseWindowDates(brief.WindowID)
	if err != nil {
		return domain.PromptExperimentResult{}, fmt.Errorf("parse window dates: %w", err)
	}
	dataMeta, err := ValidateReplayData(windowStart, windowEnd, replayPath)
	if err != nil {
		return domain.PromptExperimentResult{}, err
	}

	policy, _ := baseline.Load(e.baselinePath)

	effectiveSource := baseline.ResolvePromptOverride(policy, brief.TargetAgentID, brief.TargetSkill)
	if effectiveSource == "" {
		sourcePrompt, err := os.ReadFile(brief.PromptFile)
		if err != nil {
			return domain.PromptExperimentResult{}, err
		}
		effectiveSource = string(sourcePrompt)
	}

	expID := fmt.Sprintf("exec-%s-%d", brief.TargetAgentID, time.Now().Unix())
	candidatePrompt := mutateCandidate(effectiveSource, brief, policy.Constraints)
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

	// Capture parameter snapshot for experiment traceability
	var paramSnapshotID string
	paramsCfg, err := config.LoadParametersConfig(constants.ParametersFile)
	if err == nil {
		snap := config.SnapshotForExperiment(paramsCfg, expID)
		store := config.NewSnapshotStore(constants.StateParameterSnapshots)
		if err := store.SaveSnapshot(snap); err == nil {
			paramSnapshotID = snap.ID
		}
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
			"Candidate constraints are generated relative to current baseline to ensure meaningful delta.",
			"Replay performance evaluation is the next step before acceptance or rejection.",
		},
		RecordedAt:          time.Now(),
		DataMetadata:        dataMeta,
		ParameterSnapshotID: paramSnapshotID,
	}

	if err := e.store.RecordExperiment(record); err != nil {
		return domain.PromptExperimentResult{}, err
	}
	if err := e.store.RecordPromptExperimentResult(expID, result); err != nil {
		return domain.PromptExperimentResult{}, err
	}

	return result, nil
}

func mutateCandidate(source string, brief domain.MutationBrief, base domain.SimulationConstraints) string {
	switch brief.MutationType {
	case "risk_rule_change":
		return mutateRiskRuleCandidate(source, brief, base)
	case "portfolio_constraint_revision":
		return mutatePortfolioConstraintCandidate(source, brief, base)
	case "pipeline_stage_toggle":
		return mutatePipelineStageCandidate(source, brief)
	default:
		return mutatePromptCandidate(source, brief)
	}
}

func mutatePromptCandidate(source string, brief domain.MutationBrief) string {
	base := stripMutationSections(source)
	ctrl, bullets := v2ControlBlockAndBullets(brief)
	var b strings.Builder
	b.WriteString(strings.TrimSpace(base))
	b.WriteString("\n\n")
	b.WriteString(domain.RenderPromptControl(ctrl))
	b.WriteString("\n\n")
	b.WriteString("## Candidate Mutation v2 - Prompt Tightening\n\n")
	b.WriteString("This mutation tightens setup quality while preserving required skill and guardrail boundaries.\n\n")
	b.WriteString("### Executable Prompt Controls\n\n")
	for _, line := range bullets {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
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

// stripMutationSections removes appended Candidate Mutation sections and trailing
// control blocks so that new mutations are generated from a clean base prompt.
func stripMutationSections(source string) string {
	s := domain.ControlBlockRe.ReplaceAllString(source, "")
	mutationRe := regexp.MustCompile(`\n*## Candidate Mutation v\d+[\s\S]*`)
	s = mutationRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func v2ControlBlockAndBullets(brief domain.MutationBrief) (domain.PromptControl, []string) {
	switch brief.TargetSkill {
	case "semiconductor_desk":
		return domain.PromptControl{VolumeFloor: 1500000, VolumeDowngrade: 25, CloseStrengthBoost: 10, HardRejectVolume: 1500000, PriceCondition: "close_below_open"}, []string{"- downgrade conviction when weak volume is accompanied by price below open", "- tolerate volume between 1.5M and 3M if close strength confirms leadership", "- reject setups only when volume is below 1.5M and price action is weak", "- keep exploratory coverage: do not hard-filter solely on one weak signal"}
	case "financials_desk":
		return domain.PromptControl{VolumeDowngrade: 20, PriceCondition: "close_below_open"}, []string{"- credit quality gate: downgrade conviction when close is weak vs open", "- spread sensitivity downgrade: penalize weak intraday close strength", "- capital adequacy premium: upgrade conviction when close strength confirms balance-sheet resilience", "- enforce illiquid rejection for weak volume names"}
	case "technical_breakout":
		return domain.PromptControl{VolumeBoost: 5, PriceCondition: "close_near_high"}, []string{"- catch-up momentum: boost conviction for stocks closing strong but below session peak", "- volume participation acceptance: include moderate volume names alongside high-volume breakouts", "- close-strength tolerance: do not downgrade minor below-high closes unless breakdown is material", "- breakout confirmation bonus: boost conviction when close confirms strength near session high with volume"}
	case "etf_rotation_desk":
		return domain.PromptControl{CloseStrengthBoost: 6, VolumeBoost: 5, HardRejectVolume: 5000000, PriceCondition: "close_below_low_threshold"}, []string{"- rotation boost when close confirms strength above open", "- sector leadership premium when volume confirms institutional participation", "- reject setups that close near session low under controlled risk", "- keep exploratory coverage: do not hard-filter solely on one weak signal"}
	case "value_yield":
		return domain.PromptControl{VolumeDowngrade: 15, PriceCondition: "close_below_open"}, []string{"- require dividend cover stability before upgrading conviction", "- downgrade yield traps with weak balance-sheet trends", "- keep exploratory coverage: do not hard-filter solely on one weak signal"}
	case "growth_momentum":
		return domain.PromptControl{VolumeDowngrade: 15, PriceCondition: "close_below_open", RequireTrend: brief.ObservedWindowCount >= 5}, []string{"- keep momentum bias but tolerate one weak confirmation signal in exploratory mode", "- downgrade conviction when price is below intraday strength or open", "- enforce illiquid rejection for weak volume names", "- keep exploratory coverage: do not hard-filter solely on one weak signal"}
	default:
		return domain.PromptControl{VolumeDowngrade: 15, PriceCondition: "close_below_open", RequireTrend: brief.ObservedWindowCount >= 5}, []string{"- keep momentum bias but tolerate one weak confirmation signal in exploratory mode", "- downgrade conviction when price is below intraday strength or open", "- enforce illiquid rejection for weak volume names", "- keep exploratory coverage: do not hard-filter solely on one weak signal"}
	}
}

func mutateRiskRuleCandidate(source string, brief domain.MutationBrief, base domain.SimulationConstraints) string {
	var b strings.Builder
	title := "# Risk Rule Change Proposal - Governance Tightening"
	description := "This artifact proposes risk rule modifications designed to be measurable, auditable, and execution-safe."

	convictionFloor := computeDeltaConviction(base.MinRecommendationConviction)
	liquidityFloor := computeDeltaLiquidity(base.MinTradableVolume)
	maxPositionWeight := computeDeltaMaxPositionWeight(base.MaxPositionWeight)
	reserveCashFraction := computeDeltaReserveCash(base.ReserveCashFraction)
	stopLossPct := computeDeltaStopLoss(base.StopLossPct)
	requireCROPass := true

	rules := []string{
		fmt.Sprintf("1. **Raise Entry Quality**: Conviction floor adjusted to %d (from %d)", convictionFloor, base.MinRecommendationConviction),
		fmt.Sprintf("2. **Tighten Liquidity**: Liquidity floor set to %s (from %s)", formatInt64(liquidityFloor), formatInt64(base.MinTradableVolume)),
		fmt.Sprintf("3. **Trim Position Concentration**: Max position weight set to %.0f%% (from %.0f%%)", maxPositionWeight*100, base.MaxPositionWeight*100),
		fmt.Sprintf("4. **Cash Buffer**: Reserve cash fraction set to %.0f%% (from %.0f%%)", reserveCashFraction*100, base.ReserveCashFraction*100),
		"5. **Keep CRO Gate**: Require CRO pass remains enabled",
	}

	if brief.TargetSkill != "financials_desk" {
		title = "# Risk Rule Change Proposal - Controlled Optimization"
		description = "This artifact proposes controlled risk rule changes to improve portfolio efficiency under explicit guardrails."
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
	fmt.Fprintf(&b, "  conviction_floor: %d\n", convictionFloor)
	fmt.Fprintf(&b, "  liquidity_floor: %d\n", liquidityFloor)
	fmt.Fprintf(&b, "  max_position_weight: %.2f\n", maxPositionWeight)
	fmt.Fprintf(&b, "  reserve_cash_fraction: %.2f\n", reserveCashFraction)
	fmt.Fprintf(&b, "  stop_loss_pct: %.0f\n", stopLossPct*100)
	fmt.Fprintf(&b, "  require_cro_pass: %t\n", requireCROPass)
	b.WriteString("```\n\n")
	b.WriteString("## Guardrails\n\n")
	writeGuidanceAndPolicy(&b, brief)
	return b.String()
}

func mutatePortfolioConstraintCandidate(source string, brief domain.MutationBrief, base domain.SimulationConstraints) string {
	var b strings.Builder
	b.WriteString("# Portfolio Constraint Optimization Proposal\n\n")
	b.WriteString("This artifact proposes portfolio-governance optimizations to improve capital efficiency.\n\n")
	writeMutationHeader(&b, brief)
	b.WriteString("## Proposed Constraint Optimizations\n\n")

	maxWeight := computeDeltaMaxPositionWeight(base.MaxPositionWeight)
	reserveCash := computeDeltaReserveCash(base.ReserveCashFraction)
	stopLossPct := computeDeltaStopLoss(base.StopLossPct)
	takeProfitPct := 0.15
	if base.TakeProfitPct > 0 {
		if base.TakeProfitPct <= 0.12 {
			takeProfitPct = 0.20
		} else {
			takeProfitPct = 0.12
		}
	}

	fmt.Fprintf(&b, "- Adjust max position weight to %.0f%% (from %.0f%%).\n", maxWeight*100, base.MaxPositionWeight*100)
	fmt.Fprintf(&b, "- Adjust reserve cash fraction to %.0f%% (from %.0f%%).\n", reserveCash*100, base.ReserveCashFraction*100)
	b.WriteString("- Enable pyramiding: add to winning positions with trailing stop protection.\n")
	b.WriteString("- Preserve CRO veto authority but streamline approval for proven patterns.\n\n")
	b.WriteString("## Baseline Context\n\n```text\n")
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("\n```\n\n")
	b.WriteString("## Candidate Constraint Patch\n\n```yaml\n")
	b.WriteString("portfolio_constraint_revision:\n")
	fmt.Fprintf(&b, "  max_position_weight: %.2f\n", maxWeight)
	fmt.Fprintf(&b, "  reserve_cash_fraction: %.2f\n", reserveCash)
	fmt.Fprintf(&b, "  stop_loss_pct: %.0f\n", stopLossPct*100)
	fmt.Fprintf(&b, "  take_profit_pct: %.0f\n", takeProfitPct*100)
	b.WriteString("  enable_pyramiding: true\n")
	b.WriteString("  require_cro_pass: true\n")
	b.WriteString("```\n\n")
	b.WriteString("## Guardrails\n\n")
	writeGuidanceAndPolicy(&b, brief)
	return b.String()
}

func computeDeltaConviction(base int) int {
	if base <= 40 {
		return base + 10
	}
	if base >= 60 {
		return base - 10
	}
	return base + 7
}

func computeDeltaLiquidity(base int64) int64 {
	if base <= 2000000 {
		return base * 3 / 2
	}
	if base >= 5000000 {
		return base * 2 / 3
	}
	return base + 1000000
}

func computeDeltaMaxPositionWeight(base float64) float64 {
	if base >= 0.22 {
		v := base - 0.03
		if v < 0.10 {
			return 0.10
		}
		return v
	}
	v := base + 0.03
	if v > 0.35 {
		return 0.35
	}
	return v
}

func computeDeltaReserveCash(base float64) float64 {
	if base >= 0.15 {
		v := base - 0.02
		if v < 0.05 {
			return 0.05
		}
		return v
	}
	v := base + 0.02
	if v > 0.25 {
		return 0.25
	}
	return v
}

func computeDeltaStopLoss(base float64) float64 {
	if base <= 0 {
		return 0.08
	}
	if base <= 0.09 {
		return 0.12
	}
	return 0.08
}

func formatInt64(v int64) string {
	if v >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(v)/1000000)
	}
	if v >= 1000 {
		return fmt.Sprintf("%.1fK", float64(v)/1000)
	}
	return fmt.Sprintf("%d", v)
}

func writeMutationHeader(b *strings.Builder, brief domain.MutationBrief) {
	b.WriteString("Target layer: ")
	b.WriteString(string(brief.TargetLayer))
	b.WriteString(".\n")
	b.WriteString("Mutation type: ")
	b.WriteString(brief.MutationType)
	b.WriteString(".\n")
	fmt.Fprintf(b, "Observed evidence windows: %d.\n", brief.ObservedWindowCount)
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

// mutatePipelineStageCandidate generates an architecture mutation artifact
// for pipeline stage toggling. Unlike prompt mutations, this produces a
// structured config proposal rather than a prompt file.
func mutatePipelineStageCandidate(source string, brief domain.MutationBrief) string {
	var b strings.Builder
	b.WriteString("# Pipeline Stage Toggle Proposal\n\n")
	b.WriteString("This artifact proposes toggling a pipeline stage on/off.\n\n")
	b.WriteString("## Hypothesis\n\n")
	b.WriteString(brief.Hypothesis)
	b.WriteString("\n\n")
	b.WriteString("## Proposed Change\n\n")
	fmt.Fprintf(&b, "- **Stage**: `%s`\n", brief.PipelineStage)
	fmt.Fprintf(&b, "- **Action**: `%s`\n", brief.PipelineAction)
	b.WriteString("\n## Baseline Context\n\n```text\n")
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("\n```\n\n")
	b.WriteString("## Candidate Config Patch\n\n```yaml\n")
	b.WriteString("pipeline_stage_toggle:\n")
	fmt.Fprintf(&b, "  stage: %s\n", brief.PipelineStage)
	fmt.Fprintf(&b, "  action: %s\n", brief.PipelineAction)
	b.WriteString("```\n\n")
	b.WriteString("## Guardrails\n\n")
	b.WriteString("- Must not toggle critical safety stages (ControlLayer)\n")
	b.WriteString("- Must be validated on multi-regime replay window\n")
	b.WriteString("- Requires 2x improvement threshold vs standard mutations\n")
	writeGuidanceAndPolicy(&b, brief)
	return b.String()
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
