package service

import (
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"bufio"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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
	if data, err := os.ReadFile(filepath.Join(s.WorkDir, livestore.livestore.DefaultCircuitBreakerStatePath)); err == nil {
		if err := json.Unmarshal(data, &cbState); err != nil {
			log.Printf("[LiveService] warn: failed to unmarshal circuit breaker state: %v", err)
		}
	}

	portfolio := PortfolioSummary{}
	liveBasePath := filepath.Join(s.WorkDir, livestore.livestore.DefaultLiveStateBasePath)
	if p, err := livestore.LoadLastlivestore.PortfolioState(liveBasePath); err == nil {
		portfolio.Cash = p.Cash
		portfolio.TotalExposure = p.TotalExposure
		portfolio.AvailableCash = p.AvailableCash
		portfolio.DayPnL = p.DayPnL
		portfolio.UnrealizedPnL = p.UnrealizedPnL
	} else {
		log.Printf("[LiveService] warn: failed to read portfolio state: %v", err)
	}

	positionsCount := 0
	if posMap, err := livestore.livestore.LoadLastPositions(liveBasePath); err == nil {
		positionsCount = len(posMap)
	} else {
		log.Printf("[LiveService] warn: failed to read positions state: %v", err)
	}
	portfolio.PositionsCount = positionsCount

	return LiveStatusResponse{
		CircuitBreaker: cbState,
		Portfolio:      portfolio,
		Timestamp:      time.Now().UTC(),
	}
}

// livestore.PortfolioStateResponse is the response for portfolio state endpoint.
type livestore.PortfolioStateResponse struct {
	SnapshotTime     time.Time          `json:"snapshot_time"`
	Cash             float64            `json:"cash"`
	PortfolioValue   float64            `json:"portfolio_value"`
	CumulativePnL    float64            `json:"cumulative_pnl"`
	CumulativePnLPct float64            `json:"cumulative_pnl_pct"`
	CurrentDrawdown  float64            `json:"current_drawdown"`
	PositionsCount   int                `json:"positions_count"`
	Positions        []PositionDTO      `json:"positions"`
	EquityCurve      []EquityCurvePoint `json:"equity_curve"`
	Regime           string             `json:"regime"`
	RegimeLabel      string             `json:"regime_label"`
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
	AgentID       string  `json:"agent_id"`
	AgentName     string  `json:"agent_name"`
	Conviction    float64 `json:"conviction"`
}

// EquityCurvePoint is a single point on the equity curve.
type EquityCurvePoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// Loadlivestore.PortfolioState returns the current portfolio state with positions and equity curve.
func (s *LiveService) Loadlivestore.PortfolioState() livestore.PortfolioStateResponse {
	liveBasePath := filepath.Join(s.WorkDir, livestore.livestore.DefaultLiveStateBasePath)

	portfolio, err := livestore.LoadLastlivestore.PortfolioState(liveBasePath)
	if err != nil {
		log.Printf("[LiveService] warn: failed to load portfolio state: %v", err)
		return livestore.PortfolioStateResponse{}
	}

	posMap, err := livestore.livestore.LoadLastPositions(liveBasePath)
	if err != nil {
		log.Printf("[LiveService] warn: failed to load positions: %v", err)
	}

	// Fallback: if live state has no positions, try loading from latest session
	if len(posMap) == 0 {
		posMap = s.loadSessionPositions()
	}

	// Load agent attribution from latest session outcomes
	symbolAgentMap := s.buildSymbolAgentMap()

	positions := make([]PositionDTO, 0, len(posMap))
	for _, pos := range posMap {
		pnlPct := 0.0
		if cost := float64(pos.Quantity) * pos.AverageCost; cost > 0 {
			pnlPct = pos.UnrealizedPnL / cost
		}
		agentID, agentName, conviction := "", "", 0.0
		if attr, ok := symbolAgentMap[pos.Symbol]; ok {
			agentID = attr.agentID
			agentName = attr.agentName
			conviction = attr.conviction
		}
		positions = append(positions, PositionDTO{
			Symbol:        pos.Symbol,
			Quantity:      pos.Quantity,
			AverageCost:   pos.AverageCost,
			CurrentPrice:  pos.CurrentPrice,
			MarketValue:   pos.MarketValue,
			UnrealizedPnL: pos.UnrealizedPnL,
			PnlPct:        pnlPct,
			AgentID:       agentID,
			AgentName:     agentName,
			Conviction:    conviction,
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
	drawdown := computeCurrentDrawdown(equityCurve, portfolio.Cash+totalMarketValue)

	// Load regime from latest session summary
	regime, regimeLabel := s.loadLatestRegime()

	resp := livestore.PortfolioStateResponse{
		SnapshotTime:     portfolio.LastUpdated,
		Cash:             portfolio.Cash,
		PortfolioValue:   portfolio.Cash + totalMarketValue,
		CumulativePnL:    portfolio.RealizedPnL + portfolio.UnrealizedPnL,
		CumulativePnLPct: 0,
		CurrentDrawdown:  drawdown,
		PositionsCount:   len(positions),
		Positions:        positions,
		EquityCurve:      equityCurve,
		Regime:           regime,
		RegimeLabel:      regimeLabel,
	}
	if portfolio.Cash > 0 {
		resp.CumulativePnLPct = resp.CumulativePnL / portfolio.Cash
	}

	return resp
}

// symbolAgentAttr holds agent attribution for a position.
type symbolAgentAttr struct {
	agentID    string
	agentName  string
	conviction float64
}

// buildSymbolAgentMap loads the latest session's recommendation outcomes
// and builds a map from symbol to agent attribution.
func (s *LiveService) buildSymbolAgentMap() map[string]symbolAgentAttr {
	result := make(map[string]symbolAgentAttr)

	sessionsDir := filepath.Join(s.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return result
	}

	// Find latest daily session
	var latest string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), "-daily") {
			continue
		}
		if e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return result
	}

	// Read recommendation outcomes JSONL
	outcomesPath := filepath.Join(sessionsDir, latest, "recommendation_outcomes.jsonl")
	f, err := os.Open(outcomesPath)
	if err != nil {
		return result
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Parse minimal fields - only need symbol, agent_id, skill, conviction
		var raw struct {
			Symbol     string  `json:"symbol"`
			AgentID    string  `json:"agent_id"`
			Skill      string  `json:"skill"`
			Conviction float64 `json:"conviction"`
			Passed     *bool   `json:"passed_guards"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		// Only attribute passed recommendations
		if raw.Passed != nil && !*raw.Passed {
			continue
		}
		// Take the first agent for each symbol (or highest conviction could be better)
		if _, exists := result[raw.Symbol]; !exists {
			result[raw.Symbol] = symbolAgentAttr{
				agentID:    raw.AgentID,
				agentName:  raw.Skill,
				conviction: raw.Conviction,
			}
		}
	}

	return result
}

// loadSessionPositions loads positions from the latest session's positions.json as fallback.
func (s *LiveService) loadSessionPositions() map[string]domain.Position {
	result := make(map[string]domain.Position)

	sessionsDir := filepath.Join(s.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return result
	}

	// Find the latest daily session that has a positions.json file
	type candidate struct {
		name string
		date time.Time
	}
	var latest candidate
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), "-daily") {
			continue
		}
		positionsPath := filepath.Join(sessionsDir, e.Name(), "positions.json")
		if _, err := os.Stat(positionsPath); err != nil {
			continue
		}
		// Extract date from session ID (session-YYYYMMDD-daily)
		dateStr := strings.TrimPrefix(e.Name(), "session-")
		dateStr = strings.TrimSuffix(dateStr, "-daily")
		if parsed, err := time.Parse("20060102", dateStr); err == nil {
			if parsed.After(latest.date) {
				latest = candidate{name: e.Name(), date: parsed}
			}
		}
	}
	if latest.name == "" {
		return result
	}

	positionsPath := filepath.Join(sessionsDir, latest.name, "positions.json")
	bytes, err := os.ReadFile(positionsPath)
	if err != nil {
		return result
	}

	var positions []domain.Position
	if err := json.Unmarshal(bytes, &positions); err != nil {
		log.Printf("[LiveService] warn: failed to parse session positions: %v", err)
		return result
	}

	for _, p := range positions {
		result[p.Symbol] = p
	}
	return result
}

// loadLatestRegime reads the regime from the latest session summary.
func (s *LiveService) loadLatestRegime() (string, string) {
	sessionsDir := filepath.Join(s.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", ""
	}

	var latest string
	for _, e := range entries {
		if !e.IsDir() || !strings.HasSuffix(e.Name(), "-daily") {
			continue
		}
		if e.Name() > latest {
			latest = e.Name()
		}
	}
	if latest == "" {
		return "", ""
	}

	summaryPath := filepath.Join(sessionsDir, latest, "summary.json")
	bytes, err := os.ReadFile(summaryPath)
	if err != nil {
		return "", ""
	}
	var summary domain.SessionSummary
	if err := json.Unmarshal(bytes, &summary); err != nil {
		return "", ""
	}

	regime := string(summary.Regime)
	label := regimeLabel(regime)
	return regime, label
}

// computeCurrentDrawdown calculates the current drawdown from equity curve peaks.
func computeCurrentDrawdown(curve []EquityCurvePoint, currentValue float64) float64 {
	if len(curve) == 0 {
		return 0
	}
	peak := curve[0].Value
	for _, p := range curve {
		if p.Value > peak {
			peak = p.Value
		}
	}
	if peak <= 0 {
		return 0
	}
	// Use currentValue if provided (live portfolio), otherwise last curve point
	cv := currentValue
	if cv <= 0 && len(curve) > 0 {
		cv = curve[len(curve)-1].Value
	}
	if cv >= peak {
		return 0
	}
	return (peak - cv) / peak
}

// regimeLabel returns a Chinese label for the regime.
func regimeLabel(regime string) string {
	switch regime {
	case "RISK_ON":
		return "多頭"
	case "RISK_OFF":
		return "空頭"
	case "NEUTRAL":
		return "盤整"
	default:
		return regime
	}
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
		date  time.Time
		label string
		value float64
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
		points = append(points, sessionPoint{
			date:  date,
			label: summary.SessionID,
			value: summary.PortfolioValue,
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
			Label: p.label,
			Value: p.value,
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
