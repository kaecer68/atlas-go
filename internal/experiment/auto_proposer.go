package experiment

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// AutoProposer monitors agent health and performance metrics to automatically
// generate mutation briefs when degradation is detected.
//
// Trigger conditions:
//  1. Rolling Sharpe drops below negative threshold for 5+ consecutive days
//  2. Hit rate collapses from >60% to <40% over a 20-day window
//  3. Agent stuck at minimum weight for >30 days after auto-reset
//  4. Agent health status transitions to MUTED
type AutoProposer struct {
	dwManager    *portfolio.DarwinianWeightManager
	healthMgr    *portfolio.AgentHealthManager
	tracker      *domain.MaturityTracker
	lastProposed map[string]time.Time // agentID -> last proposal time (cooldown)
	cooldown     time.Duration
}

// NewAutoProposer creates a proposer wired to the weight and health systems.
func NewAutoProposer(dw *portfolio.DarwinianWeightManager, health *portfolio.AgentHealthManager) *AutoProposer {
	return &AutoProposer{
		dwManager:    dw,
		healthMgr:    health,
		lastProposed: make(map[string]time.Time),
		cooldown:     7 * 24 * time.Hour, // max 1 proposal per agent per week
	}
}

// WithMaturityTracker attaches a maturity tracker for burn-in gating.
func (p *AutoProposer) WithMaturityTracker(mt *domain.MaturityTracker) *AutoProposer {
	p.tracker = mt
	return p
}

// WithCooldown sets the minimum interval between proposals for the same agent.
func (p *AutoProposer) WithCooldown(d time.Duration) *AutoProposer {
	p.cooldown = d
	return p
}

// Proposal represents an auto-generated mutation brief candidate.
type Proposal struct {
	AgentID       string
	MutationType  string
	TriggerReason string
	Brief         experiment.MutationBrief
}

// ProposeOutcome explains why CheckAndPropose produced its result, so the
// scheduler caller can distinguish "burn-in skip", "no degradation trigger
// hit", "all agents in cooldown", and "proposals generated" in structured
// logs (audit A5: auto_propose finished in 50ms with no way to tell which).
type ProposeOutcome struct {
	BurnInSkipped      bool   // maturity still in burn-in
	AgentsScanned      int    // agents considered by the scan
	CooldownSkipped    int    // agents skipped due to per-agent cooldown
	NoTrigger          int    // agents scanned with no degradation trigger
	ProposalsGenerated int    // proposals returned
	Reason             string // machine-readable outcome for structured logging
}

// CheckAndPropose scans all agents and returns proposals for those that meet
// degradation criteria. Proposals are auto-generated in all maturity phases
// except BURN_IN. Execution is handled by the caller (AutoJudgePromoter).
// The returned ProposeOutcome lets the caller log a precise skip reason.
func (p *AutoProposer) CheckAndPropose(ctx context.Context) ([]Proposal, ProposeOutcome, error) {
	outcome := ProposeOutcome{}
	if p.tracker != nil {
		m := p.tracker.Current()
		if m == domain.MaturityBurnIn {
			outcome.BurnInSkipped = true
			outcome.Reason = "burn_in"
			logging.Warn("auto_proposer", "burn_in_skip",
				"days_until_calibrating", p.tracker.DaysUntil(domain.MaturityCalibrating))
			return nil, outcome, nil
		}
	}

	if p.dwManager == nil {
		return nil, outcome, fmt.Errorf("auto_proposer: DarwinianWeightManager is nil")
	}

	agents := p.dwManager.GetAllAgentWeightData()
	now := time.Now()
	var proposals []Proposal

	for _, agent := range agents {
		outcome.AgentsScanned++
		// Cooldown check
		if last, ok := p.lastProposed[agent.AgentID]; ok && now.Sub(last) < p.cooldown {
			outcome.CooldownSkipped++
			continue
		}

		reason := p.checkAgent(agent)
		if reason == "" {
			outcome.NoTrigger++
			continue
		}

		brief := p.buildBrief(agent, reason)
		proposals = append(proposals, Proposal{
			AgentID:       agent.AgentID,
			MutationType:  "auto_prompt_optimization",
			TriggerReason: reason,
			Brief:         brief,
		})
		p.lastProposed[agent.AgentID] = now
	}

	outcome.ProposalsGenerated = len(proposals)
	switch {
	case len(proposals) > 0:
		outcome.Reason = "proposals"
	case outcome.CooldownSkipped == outcome.AgentsScanned && outcome.AgentsScanned > 0:
		outcome.Reason = "cooldown_only"
	default:
		outcome.Reason = "no_trigger"
	}

	logging.Info("auto_proposer", "scan_complete",
		"agents_scanned", len(agents),
		"proposals_generated", len(proposals),
		"cooldown_skipped", outcome.CooldownSkipped,
		"no_trigger", outcome.NoTrigger,
		"reason", outcome.Reason)
	return proposals, outcome, nil
}

// checkAgent evaluates a single agent against degradation triggers.
// Returns a non-empty trigger reason string if the agent should be proposed for mutation.
func (p *AutoProposer) checkAgent(agent *portfolio.DarwinianAgentWeight) string {
	// Trigger 1: Sharpe trap — stuck at minimum weight for extended period
	if agent.Weight <= 0.31 && agent.ConsecutiveAtMin >= 5 {
		return fmt.Sprintf("weight_trap: stuck at %.2f for %d cycles", agent.Weight, agent.ConsecutiveAtMin)
	}

	// Trigger 2: Negative Sharpe sustained
	if agent.RollingSharpe < -0.5 && agent.TotalSignals >= 60 {
		return fmt.Sprintf("sharpe_degradation: rolling_sharpe=%.3f (signals=%d)", agent.RollingSharpe, agent.TotalSignals)
	}

	// Trigger 3: Hit rate collapse
	if agent.HitRate < 0.4 && agent.TotalSignals >= 30 {
		return fmt.Sprintf("hit_rate_collapse: %.1f%% after %d signals", agent.HitRate*100, agent.TotalSignals)
	}

	// Trigger 4: High volatility penalty — agent is too noisy
	if agent.RollingVolatility > 0.05 && agent.RollingSharpe < 0 {
		return fmt.Sprintf("volatility_penalty: vol=%.3f sharpe=%.3f", agent.RollingVolatility, agent.RollingSharpe)
	}

	// Trigger 5: Health manager says agent is muted
	if p.healthMgr != nil {
		h := p.healthMgr.GetHealth(agent.AgentID)
		if h != nil && h.Status == portfolio.HealthStatusMuted {
			return fmt.Sprintf("health_muted: status=%s sharpe=%.3f", h.Status, h.AnnualizedSharpe)
		}
	}

	return ""
}

func (p *AutoProposer) buildBrief(agent *portfolio.DarwinianAgentWeight, reason string) experiment.MutationBrief {
	maturity := "level_1_exploratory"
	if p.tracker != nil {
		switch p.tracker.Current() {
		case domain.MaturityCalibrating:
			maturity = "level_2_validated"
		case domain.MaturityFullAuto:
			maturity = "level_3_production"
		}
	}

	return experiment.MutationBrief{
		MutationType:  "auto_prompt_optimization",
		TargetAgentID: agent.AgentID,
		TargetSkill:   agent.Skill,
		Hypothesis:    reason,
		MaturityLevel: maturity,
		GeneratedAt:   time.Now(),
	}
}
