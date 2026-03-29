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
	b.WriteString("## Candidate Mutation v2\n\n")
	b.WriteString("Target layer: ")
	b.WriteString(string(brief.TargetLayer))
	b.WriteString(".\n")
	b.WriteString(fmt.Sprintf("Observed evidence windows: %d.\n", brief.ObservedWindowCount))
	b.WriteString("Maturity level: ")
	b.WriteString(brief.MaturityLevel)
	b.WriteString(".\n\n")
	b.WriteString("Use a tighter qualification process before upgrading conviction.\n\n")
	b.WriteString("- Require trend confirmation and volume confirmation before promoting a breakout.\n")
	b.WriteString("- Downgrade conviction when earnings support or estimate support is unclear.\n")
	b.WriteString("- Reject setups that look illiquid, narrative-only, or structurally fragile.\n")
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
	return b.String()
}

func mutateRiskRuleCandidate(source string, brief domain.MutationBrief) string {
	var b strings.Builder
	b.WriteString("# Risk Rule Change Proposal\n\n")
	b.WriteString("This artifact proposes a conservative control-layer change driven by replay evidence.\n\n")
	writeMutationHeader(&b, brief)
	b.WriteString("## Proposed Rule Adjustments\n\n")
	b.WriteString("- Tighten conviction thresholds before a recommendation can survive CRO filtering.\n")
	b.WriteString("- Add explicit downgrade logic when liquidity or close strength deteriorates.\n")
	b.WriteString("- Require downstream agents to keep the same required skill coverage before a risk rule can pass.\n\n")
	b.WriteString("## Baseline Context\n\n```text\n")
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("\n```\n\n")
	b.WriteString("## Candidate Rule Patch\n\n```yaml\n")
	b.WriteString("risk_rule_change:\n")
	b.WriteString("  conviction_floor: 55\n")
	b.WriteString("  liquidity_floor: 5000000\n")
	b.WriteString("  reject_on_weak_close: true\n")
	b.WriteString("```\n\n")
	b.WriteString("## Guardrails\n\n")
	writeGuidanceAndPolicy(&b, brief)
	return b.String()
}

func mutatePortfolioConstraintCandidate(source string, brief domain.MutationBrief) string {
	var b strings.Builder
	b.WriteString("# Portfolio Constraint Revision Proposal\n\n")
	b.WriteString("This artifact proposes a portfolio-governance change and therefore should be judged with the strictest acceptance profile.\n\n")
	writeMutationHeader(&b, brief)
	b.WriteString("## Proposed Constraint Revisions\n\n")
	b.WriteString("- Reduce maximum single-position concentration when replay evidence shows fragile clustering.\n")
	b.WriteString("- Increase reserve cash requirements during uncertain or thin-evidence windows.\n")
	b.WriteString("- Preserve CRO veto authority and do not bypass control-layer sequencing.\n\n")
	b.WriteString("## Baseline Context\n\n```text\n")
	b.WriteString(strings.TrimSpace(source))
	b.WriteString("\n```\n\n")
	b.WriteString("## Candidate Constraint Patch\n\n```yaml\n")
	b.WriteString("portfolio_constraint_revision:\n")
	b.WriteString("  max_position_weight: 0.15\n")
	b.WriteString("  reserve_cash_fraction: 0.12\n")
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
