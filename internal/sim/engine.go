package sim

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/reflexivity"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/tax"
)

// TraceWriter records simulation pipeline trace events.
// SimTraceWriter in internal/orchestrator satisfies this interface.
type TraceWriter interface {
	Record(step int, layer, status string, meta map[string]any)
}

// RotationFunc evaluates held positions against BUY candidates and returns SELL
// recommendations for the weakest holding(s) to make room for new entries.
type RotationFunc func(
	positions []domain.Position,
	recs []domain.Recommendation,
	quotes map[string]domain.Quote,
	maxOpenPositions int,
) []domain.Recommendation

type Engine struct {
	constraints domain.SimulationConstraints
	// reserveCashFractionOverride, when non-nil, replaces
	// constraints.ReserveCashFraction for the effective reserve calculation.
	// Used by CharterMode period-driven cash control (C2).
	reserveCashFractionOverride *float64
	// convictionFloorAdjustment, when non-nil, adds percentage points on top
	// of constraints.MinRecommendationConviction for the effective conviction
	// floor. Used by CharterMode period-driven conviction gating (C3).
	convictionFloorAdjustment *int
	optimizer                 *portfolio.Optimizer
	useOptimizer              bool
	reflexRules               []reflexivity.Rule
	ctx                       context.Context
	slippageModel             *SlippageModel
	marketImpactModel         *MarketImpactModel
	taxCalc                   *tax.TaiwanTaxCalculator
	dividends                 map[string]float64
	thresholdEngine           *DynamicThresholdEngine
	preTradeGate              *risk.PreTradeGate
	traceWriter               TraceWriter
	rotationFunc              RotationFunc
	riskCalculator            RiskCalculator
}

// RiskCalculator computes real VaR from portfolio state.
type RiskCalculator interface {
	ComputePortfolioVaR(totalValue float64, positions map[string]float64) float64
}

type sellDetail struct {
	Symbol    string
	Quantity  int
	AvgCost   float64
	ExecPrice float64
	Reason    string
}

func NewEngine(constraints domain.SimulationConstraints) *Engine {
	return &Engine{
		constraints: constraints,
		ctx:         context.Background(),
		taxCalc:     tax.NewTaiwanTaxCalculator(domain.DefaultTaiwanTaxConfig()),
	}
}

// WithContext sets the root context for optimizer calls.
func (e *Engine) WithContext(ctx context.Context) *Engine {
	e.ctx = ctx
	return e
}

// WithOptimizer enables portfolio-optimizer-driven order generation.
func (e *Engine) WithOptimizer(o *portfolio.Optimizer) *Engine {
	e.optimizer = o
	e.useOptimizer = true
	return e
}

// Optimizer returns the attached portfolio optimizer, or nil.
func (e *Engine) Optimizer() *portfolio.Optimizer {
	return e.optimizer
}

// WithReflexivityRules attaches reflexivity feedback rules to the engine.
func (e *Engine) WithReflexivityRules(rules ...reflexivity.Rule) *Engine {
	e.reflexRules = append(e.reflexRules, rules...)
	return e
}

// WithSlippageModel attaches a dynamic slippage model to the engine.
// When set, the engine uses liquidity-based slippage instead of fixed SlippageBPS.
func (e *Engine) WithSlippageModel(sm *SlippageModel) *Engine {
	e.slippageModel = sm
	return e
}

// WithMarketImpactModel attaches a market impact model to the engine.
// When set, large orders relative to ADV incur additional execution cost.
func (e *Engine) WithMarketImpactModel(m *MarketImpactModel) *Engine {
	e.marketImpactModel = m
	return e
}

// WithTaxCalculator attaches a tax calculator for post-simulation tax adjustment.
func (e *Engine) WithTaxCalculator(tc *tax.TaiwanTaxCalculator) *Engine {
	e.taxCalc = tc
	return e
}

// WithDividends sets the dividend amounts per symbol for tax computation.
func (e *Engine) WithDividends(divs map[string]float64) *Engine {
	e.dividends = divs
	return e
}

// WithThresholdEngine attaches a dynamic threshold engine for correlation-based filtering.
func (e *Engine) WithThresholdEngine(te *DynamicThresholdEngine) *Engine {
	e.thresholdEngine = te
	return e
}

func (e *Engine) WithPreTradeGate(g *risk.PreTradeGate) *Engine {
	e.preTradeGate = g
	return e
}

// WithRiskCalculator attaches a risk calculator used by filterByPreTradeGate.
func (e *Engine) WithRiskCalculator(rc RiskCalculator) *Engine {
	e.riskCalculator = rc
	return e
}

// WithTraceWriter attaches a trace writer for recording pre-trade gate and
// optimizer fallback events during simulation.
func (e *Engine) WithTraceWriter(tw TraceWriter) *Engine {
	e.traceWriter = tw
	return e
}

// WithReserveCashFraction overrides the reserve cash fraction for subsequent
// runs (CharterMode period-driven cash control). The fraction is in [0,1] —
// e.g. 0.45 means 45% of cash stays reserved and only 55% is deployable.
// Passing a negative value clears the override and restores the base
// constraint value (Phase A behavior).
func (e *Engine) WithReserveCashFraction(f float64) *Engine {
	if f < 0 {
		e.reserveCashFractionOverride = nil
		return e
	}
	e.reserveCashFractionOverride = &f
	return e
}

// effectiveReserveCashFraction returns the reserve fraction in effect for
// the current run: the per-run override when set, otherwise the base
// constraint value.
func (e *Engine) effectiveReserveCashFraction() float64 {
	if e.reserveCashFractionOverride != nil {
		return *e.reserveCashFractionOverride
	}
	return e.constraints.ReserveCashFraction
}

// WithConvictionFloorAdjustment adds delta percentage points on top of the
// base MinRecommendationConviction for subsequent runs (CharterMode periodized
// conviction floor, C3). Passing 0 clears the adjustment and restores the base
// constraint value (Phase A behavior). The delta is the periodized floor
// premium: RISK_OFF +10 / black_swan +20 (charter §C14/C17).
func (e *Engine) WithConvictionFloorAdjustment(delta int) *Engine {
	if delta <= 0 {
		e.convictionFloorAdjustment = nil
		return e
	}
	e.convictionFloorAdjustment = &delta
	return e
}

// effectiveConvictionFloor returns the minimum recommendation conviction in
// effect for the current run: the base constraint plus the periodized
// adjustment when set, otherwise the base constraint value.
func (e *Engine) effectiveConvictionFloor() int {
	if e.convictionFloorAdjustment != nil {
		return e.constraints.MinRecommendationConviction + *e.convictionFloorAdjustment
	}
	return e.constraints.MinRecommendationConviction
}

// buildPortfolioState assembles the risk.PortfolioState used by the gate.
// Extracted from filterByPreTradeGate for testability.
func (e *Engine) buildPortfolioState(cash float64, positions []domain.Position) risk.PortfolioState {
	totalValue := cash
	posMap := make(map[string]float64, len(positions))
	for _, p := range positions {
		totalValue += p.MarketValue
		posMap[p.Symbol] = p.MarketValue
	}
	if totalValue <= 0 {
		totalValue = 3_000_000
	}
	var95 := 0.0
	if e.riskCalculator != nil {
		var95 = e.riskCalculator.ComputePortfolioVaR(totalValue, posMap)
	}
	return risk.PortfolioState{
		TotalValue:     totalValue,
		Cash:           cash,
		Var95:          var95,
		SectorExposure: make(map[string]float64),
		Positions:      posMap,
	}
}

func (e *Engine) filterByPreTradeGate(
	cash float64,
	positions []domain.Position,
	recs []domain.Recommendation,
) []domain.Recommendation {
	pf := e.buildPortfolioState(cash, positions)

	var filtered []domain.Recommendation
	var blocked int
	for _, rec := range recs {
		order := e.buildOrderIntent(rec, pf.TotalValue)
		decision, err := e.preTradeGate.Check(context.TODO(), order, pf, "NORMAL")
		if err != nil {
			logging.Warn("sim", "pre_trade_check_failed", "symbol", rec.Symbol, "err", err)
			filtered = append(filtered, rec)
			continue
		}
		if decision.Verdict == risk.VerdictBlock || decision.Verdict == risk.VerdictHalt {
			// Auto-size: when blocked by position concentration, reduce order
			// to fit within the limit instead of rejecting entirely.
			if strings.Contains(decision.Reason, "position") && strings.Contains(decision.Reason, "would be") {
				currentPct := pf.Positions[rec.Symbol] / pf.TotalValue
				limit := e.preTradeGate.MaxPositionPct()
				if currentPct < limit {
					maxNotional := (limit - currentPct) * pf.TotalValue
					if maxNotional > pf.TotalValue*0.01 {
						order.Notional = maxNotional
						decision2, err2 := e.preTradeGate.Check(context.TODO(), order, pf, "NORMAL")
						if err2 == nil && decision2.Verdict != risk.VerdictBlock && decision2.Verdict != risk.VerdictHalt {
							filtered = append(filtered, rec)
							pf = applyOrderToState(pf, order)
							continue
						}
					}
				}
			}
			logging.Info("sim", "pre_trade_blocked",
				"symbol", rec.Symbol,
				"reason", decision.Reason)
			blocked++
			continue
		}
		filtered = append(filtered, rec)
		pf = applyOrderToState(pf, order)
	}

	if e.traceWriter != nil {
		passed := len(filtered)
		status := "OK"
		if blocked > 0 && passed == 0 {
			status = "WARN"
		}
		e.traceWriter.Record(6, "pre_trade_gate", status, map[string]any{
			"passed":  passed,
			"blocked": blocked,
			"total":   len(recs),
		})
	}

	return filtered
}

func (e *Engine) buildOrderIntent(rec domain.Recommendation, totalValue float64) risk.OrderIntent {
	notional := totalValue * 0.05
	if rec.TargetPrice > 0 {
		notional = float64(rec.Conviction) / 100.0 * totalValue * 0.1
	}
	side := string(rec.Side)
	if side == "" {
		side = "buy"
	}
	return risk.OrderIntent{
		Symbol:     rec.Symbol,
		Side:       side,
		Notional:   notional,
		AgentID:    rec.Agent,
		Conviction: rec.Conviction,
	}
}

func applyOrderToState(pf risk.PortfolioState, o risk.OrderIntent) risk.PortfolioState {
	pf.Cash -= o.Notional
	pf.Positions[o.Symbol] += o.Notional
	return pf
}

// GetThresholdEngine returns the attached dynamic threshold engine.
func (e *Engine) GetThresholdEngine() *DynamicThresholdEngine {
	return e.thresholdEngine
}

func (e *Engine) Run(regime domain.Regime, quotes []domain.Quote, recs []domain.Recommendation) domain.SimulationResult {
	state := domain.NewSimulationState(e.constraints.StartingCash)
	return e.RunWithState(&state, regime, quotes, recs)
}

// RunWithState executes a simulation using an existing state, enabling multi-day backtests.
func (e *Engine) RunWithState(state *domain.SimulationState, regime domain.Regime, quotes []domain.Quote, recs []domain.Recommendation) domain.SimulationResult {
	day := deriveSimDay(quotes)
	dayResult := e.RunDay(state, day, regime, quotes, recs)
	result := domain.SimulationResult{
		Regime:         regime,
		Orders:         dayResult.Orders,
		Trades:         dayResult.Trades,
		Positions:      state.Positions,
		EndingCash:     state.Cash,
		PortfolioValue: dayResult.PortfolioValue,
		GuardOutcomes:  nil,
		FallbackEvents: dayResult.FallbackEvents,
	}
	if e.taxCalc != nil {
		e.computeTaxAdjustedResults(&result, state)
	} else {
		logging.Warn("sim", "tax_fallback", "reason", "nil_calculator", "action", "skip")
		result.FallbackEvents = append(result.FallbackEvents, "tax: nil calculator, skipping")
	}
	return result
}

// RunDay executes a single trading day, updating state in-place.
func (e *Engine) RunDay(
	state *domain.SimulationState,
	day time.Time,
	regime domain.Regime,
	quotes []domain.Quote,
	recs []domain.Recommendation,
) domain.DayResult {
	quoteBySymbol := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteBySymbol[q.Symbol] = q
	}

	orders := make([]domain.Order, 0)
	trades := make([]domain.TradeRecord, 0)
	var fallbackEvents []string

	// 0. Apply reflexivity rules before any trading
	for _, rule := range e.reflexRules {
		recs = rule.Apply(recs, *state, quoteBySymbol)
	}

	// 1. Mark existing positions to market
	for i := range state.Positions {
		if q, ok := quoteBySymbol[state.Positions[i].Symbol]; ok {
			state.Positions[i].CurrentPrice = q.Last
			state.Positions[i].MarketValue = float64(state.Positions[i].Quantity) * q.Last
			state.Positions[i].UnrealizedPnL = float64(state.Positions[i].Quantity) * (q.Last - state.Positions[i].AverageCost)
		}
	}

	// 1.5. Rotation: evaluate held positions and generate SELL signals to
	// make room for BUY candidates. Uses live in-simulation portfolio state.
	if e.rotationFunc != nil && len(state.Positions) > 0 && len(recs) > 0 && e.constraints.MaxOpenPositions > 0 {
		sellRecs := e.rotationFunc(state.Positions, recs, quoteBySymbol, e.constraints.MaxOpenPositions)
		recs = append(recs, sellRecs...)
	}

	// 2. Sell logic
	if e.constraints.SellLogicEnabled() {
		sellOrders, sellDetails := e.executeSells(state, quoteBySymbol, recs, &fallbackEvents, day)
		orders = append(orders, sellOrders...)
		for _, sd := range sellDetails {
			state.RealizedPnL += float64(sd.Quantity) * (sd.ExecPrice - sd.AvgCost)
			trades = append(trades, domain.TradeRecord{
				Symbol:   sd.Symbol,
				Side:     domain.SideSell,
				Quantity: sd.Quantity,
				Price:    sd.ExecPrice,
				Amount:   float64(sd.Quantity) * sd.ExecPrice,
				Reason:   sd.Reason,
			})
		}
	}

	// 2.5. Rebalancing: trim positions exceeding the pre-trade max position limit.
	// This runs after sell logic (stop-loss/take-profit) and before buy logic,
	// freeing capital and enforcing diversification proactively.
	if e.preTradeGate != nil && e.preTradeGate.MaxPositionPct() > 0 {
		totalValue := state.PortfolioValue()
		limit := e.preTradeGate.MaxPositionPct()
		for i := range state.Positions {
			if state.Positions[i].Quantity <= 0 {
				continue
			}
			q, ok := quoteBySymbol[state.Positions[i].Symbol]
			if !ok || !q.IsTradable {
				continue
			}
			pct := state.Positions[i].MarketValue / totalValue
			if pct > limit {
				excessValue := state.Positions[i].MarketValue - totalValue*limit
				reduceQty := int(excessValue / q.Last)
				if reduceQty > 0 && reduceQty < state.Positions[i].Quantity {
					slippageBPS := e.getSlippageBPS(state.Positions[i].Symbol, quoteBySymbol, &fallbackEvents)
					proceeds := float64(reduceQty) * q.Last
					impactBPS := e.getImpactBPS(proceeds, q)
					price := applyBPS(q.Last, -(slippageBPS + e.transactionCostBPS(proceeds) + impactBPS))
					proceeds = float64(reduceQty) * price
					e.creditCashWithTPlus2Lock(state, proceeds, day)
					state.Positions[i].Quantity -= reduceQty
					state.Positions[i].MarketValue = float64(state.Positions[i].Quantity) * price
					state.RealizedPnL += float64(reduceQty) * (price - state.Positions[i].AverageCost)
					orders = append(orders, domain.Order{
						Symbol:   state.Positions[i].Symbol,
						Side:     domain.SideSell,
						Quantity: reduceQty,
						Price:    price,
						Reason:   fmt.Sprintf("rebalance: %.1f%% > %.0f%%", pct*100, limit*100),
					})
					trades = append(trades, domain.TradeRecord{
						Symbol:   state.Positions[i].Symbol,
						Side:     domain.SideSell,
						Quantity: reduceQty,
						Price:    price,
						Amount:   proceeds,
						Reason:   "rebalance_trim",
					})
				}
			}
		}
	}

	if e.constraints.MaxHoldingDays > 0 {
		kept := make([]domain.Position, 0, len(state.Positions))
		for i := range state.Positions {
			pos := &state.Positions[i]
			if pos.Quantity <= 0 || pos.EntryDate.IsZero() {
				kept = append(kept, *pos)
				continue
			}
			heldDays := int(day.Sub(pos.EntryDate).Hours() / 24)
			if heldDays <= e.constraints.MaxHoldingDays {
				kept = append(kept, *pos)
				continue
			}
			q, ok := quoteBySymbol[pos.Symbol]
			if !ok || !q.IsTradable {
				kept = append(kept, *pos)
				continue
			}
			slippageBPS := e.getSlippageBPS(pos.Symbol, quoteBySymbol, &fallbackEvents)
			price := applyBPS(q.Last, -(slippageBPS + e.transactionCostBPS(float64(pos.Quantity)*q.Last)))
			proceeds := float64(pos.Quantity) * price
			e.creditCashWithTPlus2Lock(state, proceeds, day)
			state.RealizedPnL += float64(pos.Quantity) * (price - pos.AverageCost)
			orders = append(orders, domain.Order{
				Symbol: pos.Symbol, Side: domain.SideSell, Quantity: pos.Quantity, Price: price, Reason: "max_holding_days",
			})
			trades = append(trades, domain.TradeRecord{
				Symbol: pos.Symbol, Side: domain.SideSell, Quantity: pos.Quantity, Price: price, Amount: proceeds, Reason: "max_holding_days",
			})
		}
		state.Positions = kept
	}

	// 3. Buy logic
	buyOrders, newPositions := e.executeBuys(state.AvailableCash(day), state.Positions, quoteBySymbol, recs, regime, &fallbackEvents, day)
	orders = append(orders, buyOrders...)
	for _, o := range buyOrders {
		trades = append(trades, domain.TradeRecord{
			Symbol:   o.Symbol,
			Side:     domain.SideBuy,
			Quantity: o.Quantity,
			Price:    o.Price,
			Amount:   float64(o.Quantity) * o.Price,
			Reason:   o.Reason,
		})
	}
	state.Positions = newPositions
	state.Cash -= totalCost(buyOrders)

	// 4. Record daily metrics
	portfolioValue := state.PortfolioValue()
	state.EquityCurve = append(state.EquityCurve, portfolioValue)
	prevValue := state.PreviousValues["_portfolio_"]
	if prevValue > 0 {
		state.DailyReturns = append(state.DailyReturns, (portfolioValue-prevValue)/prevValue)
	}
	state.PreviousValues["_portfolio_"] = portfolioValue
	if portfolioValue > state.MaxEquity {
		state.MaxEquity = portfolioValue
	}
	if state.MaxEquity > 0 {
		dd := (state.MaxEquity - portfolioValue) / state.MaxEquity
		if dd > state.CurrentDrawdown {
			state.CurrentDrawdown = dd
		}
	}

	dailyPnL := portfolioValue - prevValue
	if prevValue == 0 {
		dailyPnL = portfolioValue - e.constraints.StartingCash
	}

	return domain.DayResult{
		Regime:         regime,
		Orders:         orders,
		Trades:         trades,
		Positions:      clonePositions(state.Positions),
		Cash:           state.Cash,
		PortfolioValue: portfolioValue,
		DailyPnL:       dailyPnL,
		FallbackEvents: fallbackEvents,
	}
}

func (e *Engine) getSlippageBPS(symbol string, quotes map[string]domain.Quote, fallbackEvents *[]string) float64 {
	if e.slippageModel != nil {
		return e.slippageModel.CalculateSlippageBPS(symbol, quotes, fallbackEvents)
	}
	return e.constraints.SlippageBPS
}

// getImpactBPS estimates additional market impact cost for an order.
// Uses today's volume as a rough ADV proxy when no historical ADV is available.
func (e *Engine) getImpactBPS(orderNotional float64, quote domain.Quote) float64 {
	if e.marketImpactModel == nil {
		return 0
	}
	adv := quote.Volume
	if adv <= 0 {
		adv = e.marketImpactModel.DefaultADV
	}
	// Default 20% annualized volatility estimate when no vol surface is available.
	impact := e.marketImpactModel.Estimate(orderNotional, adv, quote.Last, 0.20)
	return impact.TotalImpactBPS
}

// transactionCostBPS returns the applicable commission rate in basis points.
// Uses the discounted rate for orders meeting the notional threshold.
func (e *Engine) transactionCostBPS(notional float64) float64 {
	if IsEligibleForDiscountedCommission(notional, e.constraints.CommissionDiscountThreshold) {
		return e.constraints.DiscountedCommissionBps
	}
	return e.constraints.TransactionCostBPS
}

func (e *Engine) creditCashWithTPlus2Lock(state *domain.SimulationState, amount float64, day time.Time) {
	state.Cash += amount
	state.LockedCash = append(state.LockedCash, domain.LockedCashEntry{
		UnlockDay: day.AddDate(0, 0, 2),
		Amount:    amount,
	})
}

func (e *Engine) executeSells(
	state *domain.SimulationState,
	quoteBySymbol map[string]domain.Quote,
	recs []domain.Recommendation,
	fallbackEvents *[]string,
	day time.Time,
) ([]domain.Order, []sellDetail) {
	remaining := make([]domain.Position, 0, len(state.Positions))
	var orders []domain.Order
	var details []sellDetail

	for _, pos := range state.Positions {
		quote, ok := quoteBySymbol[pos.Symbol]
		if !ok || !quote.IsTradable {
			remaining = append(remaining, pos)
			continue
		}

		shouldSell, reason := e.shouldSellPosition(pos, quote, recs)
		if shouldSell {
			slippageBPS := e.getSlippageBPS(pos.Symbol, quoteBySymbol, fallbackEvents)
			notional := float64(pos.Quantity) * quote.Last
			impactBPS := e.getImpactBPS(notional, quote)
			price := applyBPS(quote.Last, -(slippageBPS + e.transactionCostBPS(notional) + impactBPS))
			proceeds := float64(pos.Quantity) * price
			e.creditCashWithTPlus2Lock(state, proceeds, day)
			orders = append(orders, domain.Order{
				Symbol:   pos.Symbol,
				Side:     domain.SideSell,
				Quantity: pos.Quantity,
				Price:    price,
				Reason:   reason,
			})
			details = append(details, sellDetail{Symbol: pos.Symbol, Quantity: pos.Quantity, AvgCost: pos.AverageCost, ExecPrice: price, Reason: reason})
			continue
		}
		remaining = append(remaining, pos)
	}
	state.Positions = remaining
	return orders, details
}

func (e *Engine) shouldSellPosition(pos domain.Position, quote domain.Quote, recs []domain.Recommendation) (bool, string) {
	// Look up any recommendation for this symbol to check target/stop-loss prices.
	var rec *domain.Recommendation
	for i := range recs {
		if recs[i].Symbol == pos.Symbol {
			rec = &recs[i]
			break
		}
	}

	if rec != nil && rec.StopLossPrice > 0 && quote.Last <= rec.StopLossPrice {
		return true, "stop_loss_price"
	}
	if rec != nil && rec.TargetPrice > 0 && quote.Last >= rec.TargetPrice {
		return true, "target_price"
	}
	if e.constraints.StopLossPct > 0 && quote.Last <= pos.AverageCost*(1-e.constraints.StopLossPct) {
		return true, "stop_loss"
	}
	if e.constraints.TakeProfitPct > 0 && quote.Last >= pos.AverageCost*(1+e.constraints.TakeProfitPct) {
		return true, "take_profit"
	}
	for _, r := range recs {
		if r.Symbol == pos.Symbol && r.Side == domain.SideSell {
			return true, "conviction_reversal"
		}
	}
	return false, ""
}

// RunMultiDay runs a sequential multi-day simulation using the provided regime.
func (e *Engine) RunMultiDay(
	regime domain.Regime,
	quotesByDate map[string][]domain.Quote,
	recsByDate map[string][]domain.Recommendation,
	dates []time.Time,
) domain.SimulationReport {
	state := domain.NewSimulationState(e.constraints.StartingCash)
	var firstDate, lastDate time.Time
	if len(dates) > 0 {
		firstDate = dates[0]
		lastDate = dates[len(dates)-1]
	}

	var totalTrades int
	var allFallbackEvents []string
	for _, date := range dates {
		key := date.Format("2006-01-02")
		quotes := quotesByDate[key]
		recs := recsByDate[key]
		result := e.RunDay(&state, date, regime, quotes, recs)
		totalTrades += len(result.Orders)
		allFallbackEvents = append(allFallbackEvents, result.FallbackEvents...)
	}

	report := domain.SimulationReport{
		TotalReturn:    0,
		SharpeRatio:    0,
		MaxDrawdown:    state.CurrentDrawdown,
		EquityCurve:    append([]float64(nil), state.EquityCurve...),
		AgentHitRates:  make(map[string]float64),
		TradeCount:     totalTrades,
		StartDate:      firstDate,
		EndDate:        lastDate,
		FallbackEvents: allFallbackEvents,
	}

	if len(state.EquityCurve) > 0 && state.EquityCurve[0] > 0 {
		report.TotalReturn = (state.EquityCurve[len(state.EquityCurve)-1] - state.EquityCurve[0]) / state.EquityCurve[0]
	}
	if len(state.DailyReturns) > 1 {
		report.SharpeRatio = calculateSharpe(state.DailyReturns)
	}

	return report
}

func (e *Engine) executeBuys(
	cash float64,
	existingPositions []domain.Position,
	quoteBySymbol map[string]domain.Quote,
	recs []domain.Recommendation,
	regime domain.Regime,
	fallbackEvents *[]string,
	day time.Time,
) ([]domain.Order, []domain.Position) {
	if e.preTradeGate != nil {
		recs = e.filterByPreTradeGate(cash, existingPositions, recs)
	}
	if e.useOptimizer && e.optimizer != nil {
		return e.executeOptimizerBuys(cash, existingPositions, quoteBySymbol, recs, regime, fallbackEvents, day)
	}
	return e.executeLegacyBuys(cash, existingPositions, quoteBySymbol, recs, regime, fallbackEvents, day)
}

func (e *Engine) executeOptimizerBuys(
	cash float64,
	existingPositions []domain.Position,
	quoteBySymbol map[string]domain.Quote,
	recs []domain.Recommendation,
	regime domain.Regime,
	fallbackEvents *[]string,
	day time.Time,
) ([]domain.Order, []domain.Position) {
	orders, err := e.optimizer.OptimizeToOrders(e.ctx, recs, quoteBySymbol, cash)
	if err != nil {
		logging.Warn("sim", "optimizer_fallback", "reason", "optimizer_failed", logging.Err(err))
		if fallbackEvents != nil {
			*fallbackEvents = append(*fallbackEvents, "optimizer: fallback to legacy buys")
		}
		if e.traceWriter != nil {
			e.traceWriter.Record(6, "optimizer", "WARN", map[string]any{
				"event":   "fallback_to_legacy",
				"reason":  err.Error(),
				"rec_cnt": len(recs),
			})
		}
		return e.executeLegacyBuys(cash, existingPositions, quoteBySymbol, recs, regime, fallbackEvents, day)
	}

	positions := clonePositions(existingPositions)
	var filteredOrders []domain.Order
	coverage := make(map[string]int)
	for _, rec := range recs {
		if rec.Conviction >= e.effectiveConvictionFloor() {
			coverage[rec.Symbol]++
		}
	}

	maxDeployableCash := cash * (1 - e.effectiveReserveCashFraction())
	maxPositionWeight := e.constraints.MaxPositionWeight
	if regime == domain.RegimeNeutral {
		maxPositionWeight = maxPositionWeight * config.GetParametersConfig().Engine.Simulation.NeutralRegimeSizingFactor.Value
	}
	maxPerPosition := maxDeployableCash * maxPositionWeight

	for _, order := range orders {
		if len(positions) >= e.constraints.MaxOpenPositions {
			break
		}
		if order.Side != domain.SideBuy {
			continue
		}
		if coverage[order.Symbol] == 0 {
			continue
		}
		quote, ok := quoteBySymbol[order.Symbol]
		if !ok || !quote.IsTradable || quote.Volume < e.constraints.MinTradableVolume {
			continue
		}

		slippageBPS := e.getSlippageBPS(order.Symbol, quoteBySymbol, fallbackEvents)
		notional := float64(order.Quantity) * quote.Last
		impactBPS := e.getImpactBPS(notional, quote)
		price := applyBPS(quote.Last, slippageBPS+e.transactionCostBPS(notional)+impactBPS)
		quantity := order.Quantity
		if quantity <= 0 {
			continue
		}
		positionValue := float64(quantity) * price
		if positionValue > maxPerPosition {
			quantity = int(math.Floor(maxPerPosition/price/100.0) * 100)
			if quantity <= 0 {
				continue
			}
		}
		cost := float64(quantity) * price
		if cost > cash {
			continue
		}
		cash -= cost
		filteredOrders = append(filteredOrders, domain.Order{
			Symbol:   order.Symbol,
			Side:     domain.SideBuy,
			Quantity: quantity,
			Price:    price,
			Reason:   order.Reason,
		})
		positions = appendOrUpdatePosition(positions, domain.Position{
			Symbol:        order.Symbol,
			Quantity:      quantity,
			AverageCost:   price,
			CurrentPrice:  quote.Last,
			MarketValue:   float64(quantity) * quote.Last,
			UnrealizedPnL: 0,
		}, day)
	}

	return filteredOrders, positions
}

func (e *Engine) executeLegacyBuys(
	cash float64,
	existingPositions []domain.Position,
	quoteBySymbol map[string]domain.Quote,
	recs []domain.Recommendation,
	regime domain.Regime,
	fallbackEvents *[]string,
	day time.Time,
) ([]domain.Order, []domain.Position) {
	positions := clonePositions(existingPositions)
	orders := make([]domain.Order, 0)

	sortedRecs := make([]domain.Recommendation, len(recs))
	copy(sortedRecs, recs)
	sort.Slice(sortedRecs, func(i, j int) bool {
		if sortedRecs[i].Conviction != sortedRecs[j].Conviction {
			return sortedRecs[i].Conviction > sortedRecs[j].Conviction
		}
		if sortedRecs[i].Symbol != sortedRecs[j].Symbol {
			return sortedRecs[i].Symbol < sortedRecs[j].Symbol
		}
		if sortedRecs[i].Agent != sortedRecs[j].Agent {
			return sortedRecs[i].Agent < sortedRecs[j].Agent
		}
		return sortedRecs[i].Reason < sortedRecs[j].Reason
	})

	maxDeployableCash := cash * (1 - e.effectiveReserveCashFraction())
	maxPositionWeight := e.constraints.MaxPositionWeight
	if regime == domain.RegimeNeutral {
		maxPositionWeight = maxPositionWeight * config.GetParametersConfig().Engine.Simulation.NeutralRegimeSizingFactor.Value
	}
	maxPerPosition := maxDeployableCash * maxPositionWeight

	for _, rec := range sortedRecs {
		if len(positions) >= e.constraints.MaxOpenPositions {
			break
		}
		if rec.Side != domain.SideBuy {
			continue
		}
		if rec.Conviction < e.effectiveConvictionFloor() {
			continue
		}

		quote, ok := quoteBySymbol[rec.Symbol]
		if !ok || !quote.IsTradable || quote.Volume < e.constraints.MinTradableVolume {
			continue
		}

		slippageBPS := e.getSlippageBPS(rec.Symbol, quoteBySymbol, fallbackEvents)
		notional := maxPerPosition
		impactBPS := e.getImpactBPS(notional, quote)
		price := applyBPS(quote.Last, slippageBPS+e.transactionCostBPS(notional)+impactBPS)
		quantity := int(math.Floor(maxPerPosition/price/100.0) * 100)
		if quantity <= 0 {
			continue
		}

		cost := float64(quantity) * price
		if cost > cash {
			continue
		}

		cash -= cost
		orders = append(orders, domain.Order{
			Symbol:   rec.Symbol,
			Side:     domain.SideBuy,
			Quantity: quantity,
			Price:    price,
			Reason:   rec.Reason,
		})
		positions = appendOrUpdatePosition(positions, domain.Position{
			Symbol:        rec.Symbol,
			Quantity:      quantity,
			AverageCost:   price,
			CurrentPrice:  quote.Last,
			MarketValue:   float64(quantity) * quote.Last,
			UnrealizedPnL: 0,
		}, day)
	}

	return orders, positions
}

func appendOrUpdatePosition(positions []domain.Position, newPos domain.Position, day time.Time) []domain.Position {
	for i := range positions {
		if positions[i].Symbol == newPos.Symbol {
			totalQty := positions[i].Quantity + newPos.Quantity
			totalCost := positions[i].AverageCost*float64(positions[i].Quantity) + newPos.AverageCost*float64(newPos.Quantity)
			positions[i].Quantity = totalQty
			positions[i].AverageCost = totalCost / float64(totalQty)
			positions[i].CurrentPrice = newPos.CurrentPrice
			positions[i].MarketValue = float64(totalQty) * newPos.CurrentPrice
			positions[i].UnrealizedPnL = float64(totalQty) * (newPos.CurrentPrice - positions[i].AverageCost)
			return positions
		}
	}
	newPos.EntryDate = day
	return append(positions, newPos)
}

func clonePositions(src []domain.Position) []domain.Position {
	out := make([]domain.Position, len(src))
	copy(out, src)
	return out
}

func totalCost(orders []domain.Order) float64 {
	total := 0.0
	for _, o := range orders {
		if o.Side == domain.SideBuy {
			total += float64(o.Quantity) * o.Price
		}
	}
	return total
}

func calculateSharpe(returns []float64) float64 {
	return portfolio.ComputeSharpe(returns, portfolio.SharpeConfig{
		Frequency:  portfolio.FrequencyPerDay,
		MinSamples: 60,
	})
}

// deriveSimDay extracts the trading day from the first quote's AsOf field.
// Falls back to time.Time{} when quotes is empty (preserving pre-fix behavior).
func deriveSimDay(quotes []domain.Quote) time.Time {
	for _, q := range quotes {
		if !q.AsOf.IsZero() {
			return q.AsOf
		}
	}
	return time.Time{}
}

func applyBPS(price, bps float64) float64 {
	return price * (1 + bps/10000.0)
}

func (e *Engine) computeTaxAdjustedResults(result *domain.SimulationResult, state *domain.SimulationState) {
	if e.taxCalc == nil {
		logging.Warn("sim", "tax_fallback", "reason", "nil_calculator", "action", "skip")
		return
	}

	sellPrices := make(map[string]float64, len(state.Positions))
	for _, pos := range state.Positions {
		sellPrices[pos.Symbol] = pos.CurrentPrice
	}

	taxSnapshots := e.taxCalc.CalculatePortfolioTax(state.Positions, sellPrices, e.dividends)

	var totalTax float64
	for _, snap := range taxSnapshots {
		totalTax += snap.TotalTax
	}

	beforeTaxPnL := result.PortfolioValue - e.constraints.StartingCash
	afterTaxPnL := beforeTaxPnL - totalTax

	result.TaxSnapshots = taxSnapshots
	result.BeforeTaxPnL = beforeTaxPnL
	result.AfterTaxPnL = afterTaxPnL
	result.TotalTaxPaid = totalTax
}
