// Package risk provides risk evaluation: VaR / CVaR, drawdown, capital phase,
// portfolio concentration, and industry cycle risk.
//
// Core types:
//
//	VaRCalculator             — VaR / CVaR / max drawdown computation
//	MacroAwareDrawdownEngine  — Multi-tier decision: none → monitor → reduce → halt
//	CapitalPhaseController    — Capital phase management (file-persisted)
//	PortfolioRiskProvider     — Portfolio concentration / exposure interface
//	IndustryRiskProvider      — Industry cycle risk interface
//	ApprovalWorkflow          — Human-in-the-loop approval workflow
//
// Decision flow (only the first step is wired as of FU-20261016-32):
//
//	MacroAwareDrawdownEngine.Evaluate(macroAssessment, structuralAssessment)
//	  → MacroAwareDrawdownDecision (Action, Percentage, MaxExposure, Rationale)
//	  → decision.Action is consumed by StrategyEvolver.determineState
//	  → GetPositionSizeAdjustment(decision) → float64   [DECLARED ONLY]
//	  → ShouldHaltTrading(decision) → bool              [duplicate of the inline check]
//
// What is enforced today: the Action. StrategyEvolver suspends the strategy layer
// when Action >= DrawdownSevere (orchestrator/strategy_evolver.go:200), which is
// the same predicate ShouldHaltTrading encodes.
//
// What is NOT enforced: the exposure cap. MaxExposure is read only by
// GetPositionSizeAdjustment and by CalculatePortfolioAdjustment, and neither has
// a caller; EvaluateWithPortfolio, which does fill FinalMaxExposure in its
// breakdown, is only reachable from cmd/stress-test. So do not treat
// DrawdownLevel.MaxExposure (1.0 / 0.85 / 0.65 / 0.40 / 0.10) as a live cap.
//
// MaxExposure is a multiplier: 0.5 means "halve all positions" — it does
// NOT include cash. To cap total exposure, use
// CapitalPhaseController.GetCapitalLimit() instead.
//
// Cautions:
//   - MacroAwareDrawdownEngine.escalateAction() can silently escalate monitor
//     to halt when multiple risk dimensions agree
//   - Default VaR/CVaR assume normal returns, which underestimates tail risk
//     on Taiwan's fat-tailed market. Use HistoricalVaR for non-parametric VaR
//   - A nil PortfolioRiskProvider returns PortfolioRiskAssessment with all
//     zero fields, no error
//   - CapitalPhaseController uses file persistence; concurrent access is
//     last-write-wins
//   - Industry weights come from a map[string]float64; mismatched weights
//     cause biased evaluation
//   - The drawdown logic is NOT linked to internal/apigateway.CircuitBreaker
//
// Configuration: risk uses the global config.GetParametersConfig()
// exclusively. Per-module config files are forbidden — see
// docs/audit/2026-06-20-risk-orphan-config.md. New fields go into
// ParametersConfig.Risk with ParameterMetadata[T] and defaults in
// parameters_defaults.go. LockedSaveWithRollback handles atomic writes;
// splitting files causes split-brain and is rejected for live trading.
//
// Maturity: stable
package risk
