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
// Decision flow:
//
//	MacroAwareDrawdownEngine.Evaluate(riskSnapshot, regime, narrativeEvents)
//	  → DrawdownDecision (action, rationale, positionScale)
//	  → GetPositionSizeAdjustment(decision) → float64
//	  → ShouldHaltTrading(decision) → bool
//
// positionScale is a multiplier: 0.5 means "halve all positions" — it does
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
