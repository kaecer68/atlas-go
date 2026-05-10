package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// LiveService provides live trading status and portfolio state operations.
type LiveService struct {
	WorkDir   string
	LedgerDir string
}

// NewLiveService creates a new LiveService.
func NewLiveService(workDir, ledgerDir string) *LiveService {
	return &LiveService{
		WorkDir:   workDir,
		LedgerDir: ledgerDir,
	}
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
	SnapshotTime     time.Time          `json:"snapshot_time"`
	Cash             float64            `json:"cash"`
	StartingCash     float64            `json:"starting_cash,omitempty"`
	PortfolioValue   float64            `json:"portfolio_value"`
	RealizedPnL      float64            `json:"realized_pnl,omitempty"`
	CumulativePnL    float64            `json:"cumulative_pnl"`
	CumulativePnLPct float64            `json:"cumulative_pnl_pct"`
	CurrentDrawdown  float64            `json:"current_drawdown"`
	TradeCount       int                `json:"trade_count,omitempty"`
	PositionsCount   int                `json:"positions_count"`
	Positions        []PositionDTO      `json:"positions"`
	EquityCurve      []EquityCurvePoint `json:"equity_curve"`
}

// PositionDTO represents a single position with computed P&L percentage.
type PositionDTO struct {
	Symbol        string  `json:"symbol"`
	Quantity      int     `json:"quantity"`
	AverageCost   float64 `json:"average_cost"`
	CurrentPrice  float64 `json:"current_price"`
	MarketValue   float64 `json:"market_value"`
	UnrealizedPnL float64 `json:"unrealized_pnl"`
	PnlPct        float64 `json:"pnl_pct"`
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
	for _, pos := range posMap {
		pnlPct := 0.0
		if cost := float64(pos.Quantity) * pos.AverageCost; cost > 0 {
			pnlPct = pos.UnrealizedPnL / cost
		}
		positions = append(positions, PositionDTO{
			Symbol:        pos.Symbol,
			Quantity:      pos.Quantity,
			AverageCost:   pos.AverageCost,
			CurrentPrice:  pos.CurrentPrice,
			MarketValue:   pos.MarketValue,
			UnrealizedPnL: pos.UnrealizedPnL,
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

	equityCurve := s.buildEquityCurve()
	tradeCount := len(s.LoadTradeHistory())
	startingCash := 0.0
	if persistentState, err := sim.LoadPersistentState(s.LedgerDir); err == nil && persistentState != nil {
		startingCash = persistentState.StartingCash
	}

	resp := PortfolioStateResponse{
		SnapshotTime:     portfolio.LastUpdated,
		Cash:             portfolio.Cash,
		StartingCash:     startingCash,
		PortfolioValue:   portfolio.Cash + totalMarketValue,
		RealizedPnL:      portfolio.RealizedPnL,
		CumulativePnL:    portfolio.RealizedPnL + portfolio.UnrealizedPnL,
		CumulativePnLPct: 0,
		CurrentDrawdown:  0,
		TradeCount:       tradeCount,
		PositionsCount:   len(positions),
		Positions:        positions,
		EquityCurve:      equityCurve,
	}
	if portfolio.Cash > 0 {
		resp.CumulativePnLPct = resp.CumulativePnL / portfolio.Cash
	}

	return resp
}

func (s *LiveService) LoadTradeHistory() []domain.TradeRecord {
	store := ledger.NewStore(s.LedgerDir)
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
		date := sessionDateFromID(summary.SessionID)
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

// sessionDateFromID extracts the trading date from a session ID.
func sessionDateFromID(id string) time.Time {
	const prefix = "session-"
	if !strings.HasPrefix(id, prefix) {
		return time.Time{}
	}
	trimmed := strings.TrimPrefix(id, prefix)
	parts := strings.Split(trimmed, "-")
	if len(parts) < 1 {
		return time.Time{}
	}
	if d, err := time.Parse("20060102", parts[0]); err == nil {
		return d
	}
	return time.Time{}
}
