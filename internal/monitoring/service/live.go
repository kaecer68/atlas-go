package service

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/ledger"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// symbolNameMap resolves common TW stock symbols to Chinese names.
// Keep in sync with shared_web/static/js/names.js STOCK_NAME_MAP.
var symbolNameMap = map[string]string{
	"0050.TW":  "元大台灣50",
	"0056.TW":  "元大高股息",
	"00878.TW": "國泰永續高股息",
	"1101.TW":  "台泥",
	"1216.TW":  "統一",
	"1301.TW":  "台塑",
	"1303.TW":  "南亞",
	"1326.TW":  "台化",
	"1402.TW":  "遠東新",
	"2002.TW":  "中鋼",
	"2105.TW":  "正新",
	"2207.TW":  "和泰車",
	"2303.TW":  "聯電",
	"2308.TW":  "台達電",
	"2317.TW":  "鴻海",
	"2327.TW":  "國巨",
	"2330.TW":  "台積電",
	"2357.TW":  "華碩",
	"2379.TW":  "瑞昱",
	"2382.TW":  "廣達",
	"2395.TW":  "研華",
	"2408.TW":  "南亞科",
	"2412.TW":  "中華電",
	"2454.TW":  "聯發科",
	"2880.TW":  "華南金",
	"2881.TW":  "富邦金",
	"2882.TW":  "國泰金",
	"2883.TW":  "開發金",
	"2884.TW":  "玉山金",
	"2885.TW":  "元大金",
	"2886.TW":  "兆豐金",
	"2887.TW":  "台新金",
	"2891.TW":  "中信金",
	"2892.TW":  "第一金",
	"3008.TW":  "大立光",
	"3045.TW":  "台灣大",
	"3711.TW":  "日月光投控",
	"4904.TW":  "遠傳",
	"5880.TW":  "台灣金控",
	"6505.TW":  "台塑化",
}

func resolveSymbolName(symbol string) string {
	if name, ok := symbolNameMap[symbol]; ok {
		return name
	}
	if !strings.HasSuffix(symbol, ".TW") {
		if name, ok := symbolNameMap[symbol+".TW"]; ok {
			return name
		}
	}
	return symbol
}

// LiveService provides live trading status and portfolio state operations.
type LiveService struct {
	WorkDir    string
	LedgerDir  string
	Classifier *industry.ClassificationTree

	// TradeStore backs LoadTradeHistory (the source of the portfolio
	// trade_count KPI). When nil the legacy JSONL *Store (LedgerDir) is
	// used; production wiring injects the same PG-first SSoT store that
	// backs the performance report so the portfolio card and the report
	// share one backend semantics (see RegisterLiveRoutes).
	TradeStore ledger.OutcomeStore
}

// NewLiveService creates a new LiveService.
func NewLiveService(workDir, ledgerDir string) *LiveService {
	return &LiveService{
		WorkDir:   workDir,
		LedgerDir: ledgerDir,
	}
}

// WithTradeStore attaches the ledger store used for trade-history reads.
func (s *LiveService) WithTradeStore(store ledger.OutcomeStore) *LiveService {
	s.TradeStore = store
	return s
}

// LiveStatusResponse represents the live trading status response.
type LiveStatusResponse struct {
	CircuitBreaker CircuitBreakerStatus `json:"circuit_breaker"`
	Portfolio      PortfolioSummary     `json:"portfolio"`
	Timestamp      time.Time            `json:"timestamp"`
}

// CircuitBreakerStatus holds circuit breaker state information.
type CircuitBreakerStatus struct {
	State          string    `json:"state"`
	StateChangedAt time.Time `json:"state_changed_at"`
	ConsecutiveSL  int       `json:"consecutive_sl"`
	CooldownUntil  time.Time `json:"cooldown_until"`
	IntradayPeak   float64   `json:"intraday_peak"`
	DayStartValue  float64   `json:"day_start_value"`
}

// PortfolioSummary holds a summary of portfolio state.
type PortfolioSummary struct {
	Cash           float64 `json:"cash"`
	AvailableCash  float64 `json:"available_cash"`
	TotalExposure  float64 `json:"total_exposure"`
	DayPnL         float64 `json:"day_pnl"`
	UnrealizedPnL  float64 `json:"unrealized_pnl"`
	PositionsCount int     `json:"positions_count"`
}

// LoadLiveStatus returns the current live trading status.
func (s *LiveService) LoadLiveStatus() LiveStatusResponse {
	cbState := CircuitBreakerStatus{
		State: "unknown",
	}
	if data, err := os.ReadFile(filepath.Join(s.WorkDir, livestore.DefaultCircuitBreakerStatePath)); err == nil {
		if err := json.Unmarshal(data, &cbState); err != nil {
			logging.Warn("liveservice", "unmarshal_circuit_breaker_failed", "err", err.Error())
		}
	}

	portfolio := PortfolioSummary{}
	liveBasePath := filepath.Join(s.WorkDir, livestore.DefaultLiveStateBasePath)
	if p, err := livestore.LoadLastPortfolioState(liveBasePath); err == nil {
		portfolio.Cash = p.Cash
		portfolio.TotalExposure = p.TotalExposure
		portfolio.AvailableCash = p.AvailableCash
		portfolio.DayPnL = p.DayPnL
		portfolio.UnrealizedPnL = p.UnrealizedPnL
	} else {
		logging.Warn("liveservice", "read_portfolio_state_failed", "err", err.Error())
	}

	positionsCount := 0
	if posMap, err := livestore.LoadLastPositions(liveBasePath); err == nil {
		positionsCount = len(posMap)
	} else {
		logging.Warn("liveservice", "read_positions_state_failed", "err", err.Error())
	}
	portfolio.PositionsCount = positionsCount

	return LiveStatusResponse{
		CircuitBreaker: cbState,
		Portfolio:      portfolio,
		Timestamp:      time.Now().UTC(),
	}
}

// PortfolioStateResponse is the response for portfolio state endpoint.
type PortfolioStateResponse struct {
	SnapshotTime       time.Time          `json:"snapshot_time"`
	Cash               float64            `json:"cash"`
	StartingCash       float64            `json:"starting_cash,omitempty"`
	PortfolioValue     float64            `json:"portfolio_value"`
	RealizedPnL        float64            `json:"realized_pnl,omitempty"`
	CumulativePnL      float64            `json:"cumulative_pnl"`
	CumulativePnLPct   float64            `json:"cumulative_pnl_pct"`
	CurrentDrawdown    float64            `json:"current_drawdown"`
	MaxDrawdown        float64            `json:"max_drawdown"`
	UnrealizedPnLTotal float64            `json:"unrealized_pnl_total,omitempty"`
	ConcentrationRatio float64            `json:"concentration_ratio,omitempty"`
	TradeCount         int                `json:"trade_count,omitempty"`
	PositionsCount     int                `json:"positions_count"`
	Positions          []PositionDTO      `json:"positions"`
	EquityCurve        []EquityCurvePoint `json:"equity_curve"`
	CrossFootPnL       CrossFootCheck     `json:"cross_foot_pnl"`
}

// PositionDTO represents a single position with computed P&L percentage.
type PositionDTO struct {
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	Quantity      int     `json:"quantity"`
	AverageCost   float64 `json:"average_cost"`
	CurrentPrice  float64 `json:"current_price"`
	MarketValue   float64 `json:"market_value"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
	PnlPct        float64 `json:"pnl_pct"`
	Sector        string  `json:"sector,omitempty"`
}

// CrossFootCheck represents the cross-footing verification between
// the portfolio-level UnrealizedPnL and the sum of individual position P&Ls.
type CrossFootCheck struct {
	IsBalanced   bool    `json:"is_balanced"`
	Portfolio    float64 `json:"portfolio_unrealized"`
	SumPositions float64 `json:"sum_positions_unrealized"`
	Difference   float64 `json:"difference"`
}

// EquityCurvePoint is a single point on the equity curve.
type EquityCurvePoint struct {
	Label         string  `json:"label"`
	Value         float64 `json:"value"`
	Currency      string  `json:"currency,omitempty"`
	AfterTaxValue float64 `json:"after_tax_value,omitempty"`
	TaxPaid       float64 `json:"tax_paid,omitempty"`
}

// LoadPortfolioState returns the current portfolio state with positions and equity curve.
func (s *LiveService) LoadPortfolioState() PortfolioStateResponse {
	liveBasePath := filepath.Join(s.WorkDir, livestore.DefaultLiveStateBasePath)

	portfolio, err := livestore.LoadLastPortfolioState(liveBasePath)
	if err != nil {
		logging.Warn("liveservice", "load_portfolio_state_failed", "err", err.Error())
		return PortfolioStateResponse{}
	}

	posMap, err := livestore.LoadLastPositions(liveBasePath)
	if err != nil {
		logging.Warn("liveservice", "load_positions_failed", "err", err.Error())
	}

	positions := make([]PositionDTO, 0, len(posMap))
	var totalUnrealizedPnL float64
	var storedUnrealizedPnL float64
	for _, pos := range posMap {
		// Mark-to-market at read time. The live-store writers (simulation /
		// backtest → live-store sync, intraday orchestrator fills) persist
		// current_price but can leave unrealized_pnl / market_value stale
		// (e.g. 0) when a position's last update was a fill or trim that did
		// not re-run the mark (observed 2026-09-03 production: 00713.TW cost
		// 62.43 vs price 62.35 but pnl 0). Recompute both from current_price
		// so the dashboard never shows a pnl that contradicts the snapshot
		// price. Positions without a price (never quoted) keep the persisted
		// values, so a missing quote cannot masquerade as a total loss.
		marketValue := pos.MarketValue
		unrealized := pos.UnrealizedPnL
		if pos.CurrentPrice > 0 {
			marketValue = float64(pos.Quantity) * pos.CurrentPrice
			unrealized = float64(pos.Quantity) * (pos.CurrentPrice - pos.AverageCost)
		}
		pnlPct := 0.0
		if cost := float64(pos.Quantity) * pos.AverageCost; cost > 0 {
			pnlPct = unrealized / cost
		}
		storedUnrealizedPnL += pos.UnrealizedPnL
		totalUnrealizedPnL += unrealized
		positions = append(positions, PositionDTO{
			Symbol:        pos.Symbol,
			Name:          resolveSymbolName(pos.Symbol),
			Quantity:      pos.Quantity,
			AverageCost:   pos.AverageCost,
			CurrentPrice:  pos.CurrentPrice,
			MarketValue:   marketValue,
			UnrealizedPnL: unrealized,
			PnlPct:        pnlPct,
		})
	}
	slices.SortFunc(positions, func(a, b PositionDTO) int {
		return strings.Compare(a.Symbol, b.Symbol)
	})

	var totalMarketValue float64
	for _, p := range positions {
		totalMarketValue += p.MarketValue
	}

	// Compute HHI (Herfindahl-Hirschman Index) as concentration ratio [0, 1]
	var hhi float64
	if totalMarketValue > 0 {
		for _, p := range positions {
			weight := p.MarketValue / totalMarketValue
			hhi += weight * weight
		}
	}

	// Cross-footing verification: validates the persisted snapshot's own
	// internal consistency (portfolio_state.json unrealized_pnl vs the sum of
	// the persisted per-position unrealized_pnl values), i.e. whether the
	// writer kept the two state files in sync. It intentionally uses the
	// persisted (pre-mark-to-market) values: the KPI-level mark above
	// overrides stale writer output, so comparing the file's portfolio pnl
	// against the recomputed sum would flag every stale-but-consistent
	// snapshot and spam the log.
	crossFoot := CrossFootCheck{
		Portfolio:    portfolio.UnrealizedPnL,
		SumPositions: storedUnrealizedPnL,
		Difference:   portfolio.UnrealizedPnL - storedUnrealizedPnL,
		IsBalanced:   math.Abs(portfolio.UnrealizedPnL-storedUnrealizedPnL) < 0.01,
	}
	if !crossFoot.IsBalanced {
		logging.Warn("liveservice", "cross_footing_mismatch",
			"portfolio_unrealized", crossFoot.Portfolio,
			"sum_positions", crossFoot.SumPositions,
			"difference", crossFoot.Difference)
	}

	// Fill sector for each position from classifier
	symSectorMap := s.buildSymbolSectorMap()
	for i := range positions {
		positions[i].Sector = getSectorForSymbol(positions[i].Symbol, symSectorMap)
	}

	equityCurve := s.buildEquityCurve()
	tradeCount := len(s.LoadTradeHistory())
	startingCash := 0.0
	realizedPnL := 0.0
	currentDrawdown := 0.0
	if ps, err := sim.LoadPersistentState(s.LedgerDir); err == nil && ps != nil {
		startingCash = ps.StartingCash
		realizedPnL = ps.RealizedPnL
		currentDrawdown = ps.CurrentDrawdown
	}

	// Historical max drawdown from the equity curve (peak-to-trough decline).
	maxDrawdown := calculateMaxDrawdownFromEquityCurve(equityCurve)

	resp := PortfolioStateResponse{
		SnapshotTime:       portfolio.LastUpdated,
		Cash:               portfolio.Cash,
		StartingCash:       startingCash,
		PortfolioValue:     portfolio.Cash + totalMarketValue,
		RealizedPnL:        realizedPnL,
		CumulativePnL:      realizedPnL + totalUnrealizedPnL,
		CumulativePnLPct:   0,
		CurrentDrawdown:    currentDrawdown,
		MaxDrawdown:        maxDrawdown,
		UnrealizedPnLTotal: totalUnrealizedPnL,
		ConcentrationRatio: hhi,
		TradeCount:         tradeCount,
		PositionsCount:     len(positions),
		Positions:          positions,
		EquityCurve:        equityCurve,
		CrossFootPnL:       crossFoot,
	}
	if startingCash > 0 {
		resp.CumulativePnLPct = resp.CumulativePnL / startingCash
	}

	return resp
}

func (s *LiveService) LoadTradeHistory() []domain.TradeRecord {
	store := s.TradeStore
	if store == nil {
		store = ledger.NewStore(s.LedgerDir)
	}
	trades, err := store.LoadAllSessionTrades()
	if err != nil {
		logging.Warn("liveservice", "load_trade_history_failed", "err", err.Error())
		return nil
	}
	return trades
}

// buildEquityCurve constructs an equity curve from all session summaries.
func (s *LiveService) buildEquityCurve() []EquityCurvePoint {
	sessionsDir := filepath.Join(s.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return nil
	}

	type sessionPoint struct {
		date          time.Time
		label         string
		value         float64
		taxPaid       float64
		afterTaxValue float64
	}
	points := make([]sessionPoint, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			continue
		}
		if summary.PortfolioValue == 0 {
			continue
		}
		date := domain.SessionDateFromID(summary.SessionID)
		afterTaxValue := summary.PortfolioValue - summary.TotalTaxPaid
		points = append(points, sessionPoint{
			date:          date,
			label:         summary.SessionID,
			value:         summary.PortfolioValue,
			taxPaid:       summary.TotalTaxPaid,
			afterTaxValue: afterTaxValue,
		})
	}

	if len(points) == 0 {
		return nil
	}

	slices.SortFunc(points, func(a, b sessionPoint) int {
		return a.date.Compare(b.date)
	})

	curve := make([]EquityCurvePoint, len(points))
	for i, p := range points {
		curve[i] = EquityCurvePoint{
			Label:         p.label,
			Value:         p.value,
			Currency:      "TWD",
			AfterTaxValue: p.afterTaxValue,
			TaxPaid:       p.taxPaid,
		}
	}
	return curve
}

// calculateMaxDrawdownFromEquityCurve computes the peak-to-trough decline
// from a series of equity-curve values. Returns a positive magnitude
// (e.g., 0.20 = 20% drawdown). Empty curve returns 0.
func calculateMaxDrawdownFromEquityCurve(curve []EquityCurvePoint) float64 {
	if len(curve) == 0 {
		return 0.0
	}
	values := make([]float64, len(curve))
	for i, p := range curve {
		values[i] = p.Value
	}
	return risk.CalculateMaxDrawdown(values)
}

// buildSymbolSectorMap builds a symbol→sector mapping from the classifier.
func (s *LiveService) buildSymbolSectorMap() map[string]string {
	m := make(map[string]string)
	if s.Classifier == nil {
		return m
	}
	for _, seg := range s.Classifier.GetAllSegments() {
		for _, sym := range seg.RepresentativeStocks {
			m[sym] = seg.ID
		}
	}
	return m
}

// getSectorForSymbol looks up the sector for a symbol from the pre-built map.
func getSectorForSymbol(symbol string, symMap map[string]string) string {
	if s, ok := symMap[symbol]; ok {
		return s
	}
	return "other"
}
