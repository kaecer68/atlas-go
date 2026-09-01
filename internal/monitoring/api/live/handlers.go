package live

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/risk"
)

// Handlers holds the dependencies for live trading handlers.
type Handlers struct {
	LedgerDir     string
	WorkDir       string
	Svc           *service.LiveService
	Classifier    *industry.ClassificationTree
	AgentLayerMap map[string]string
}

// sectorLabel returns the Chinese label for a sector ID, sourced from the
// industry ClassificationTree when available.
func (h *Handlers) sectorLabel(sec string) string {
	if h.Classifier != nil {
		if seg, ok := h.Classifier.GetSegment(sec); ok {
			return seg.Name
		}
	}
	switch sec {
	case "other":
		return "其他"
	default:
		return sec
	}
}

// agentLayer returns the execution layer for a given agent ID, sourced from
// the AgentLayerMap (built from configs/agents.json).
func (h *Handlers) agentLayer(agentID string) string {
	if h.AgentLayerMap != nil {
		if layer, ok := h.AgentLayerMap[agentID]; ok {
			return layer
		}
	}
	return ""
}

// BuildAgentLayerMap loads the agent registry from configs/agents.json and
// builds a map from agent ID to its execution layer string.
func BuildAgentLayerMap(workDir string) map[string]string {
	m := make(map[string]string)
	path := filepath.Join(workDir, constants.AgentsConfigPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	var reg struct {
		Agents []struct {
			ID    string `json:"id"`
			Layer string `json:"layer"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &reg); err != nil {
		return m
	}
	for _, a := range reg.Agents {
		m[a.ID] = a.Layer
	}
	return m
}

func (h *Handlers) getService() *service.LiveService {
	if h.Svc != nil {
		if h.Svc.Classifier == nil {
			h.Svc.Classifier = h.Classifier
		}
		return h.Svc
	}
	svc := service.NewLiveService(h.WorkDir, h.LedgerDir)
	svc.Classifier = h.Classifier
	return svc
}

// RegisterRoutes mounts live trading endpoints.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/pnl-attribution", shared.Get(h.HandlePnLAttribution))
	mux.Handle("GET /api/dashboard/risk-exposure", shared.Get(h.HandleRiskExposure))
	mux.Handle("GET /api/dashboard/live-status", shared.Get(h.HandleLiveStatus))
	mux.Handle("GET /api/dashboard/portfolio-state", shared.Get(h.HandlePortfolioState))
	mux.Handle("GET /api/dashboard/trade-history", shared.Get(h.HandleTradeHistory))
	mux.Handle("GET /api/dashboard/benchmark-comparison", shared.Get(h.HandleBenchmarkComparison))
}

func getSymbolSector(symbol string, symMap map[string]string) string {
	if s, ok := symMap[symbol]; ok {
		return s
	}
	return "other"
}

func (h *Handlers) computeSectorFactorExposure(outcomes []domain.RecommendationOutcome, portfolioValue float64, symSectorMap map[string]string) ([]SectorExposure, FactorExposureInline) {
	type secAgg struct {
		count                        int
		absReturn                    float64
		avgM, avgV, avgQ, avgA, avgT float64
	}
	secMap := make(map[string]*secAgg)

	var totalM, totalV, totalQ, totalA, totalT float64
	var totalAbsReturn float64
	var cnt int

	for _, oc := range outcomes {
		if !oc.PassedGuards || oc.Symbol == "" {
			continue
		}
		sec := getSymbolSector(oc.Symbol, symSectorMap)
		if secMap[sec] == nil {
			secMap[sec] = &secAgg{}
		}
		s := secMap[sec]
		s.count++
		s.absReturn += math.Abs(oc.ForwardReturn)
		totalAbsReturn += math.Abs(oc.ForwardReturn)
		s.avgM += oc.FactorScores.Momentum
		s.avgV += oc.FactorScores.Value
		s.avgQ += oc.FactorScores.Quality
		s.avgA += oc.FactorScores.Agent
		s.avgT += oc.FactorScores.Total

		totalM += oc.FactorScores.Momentum
		totalV += oc.FactorScores.Value
		totalQ += oc.FactorScores.Quality
		totalA += oc.FactorScores.Agent
		totalT += oc.FactorScores.Total
		cnt++
	}

	var sectorExp []SectorExposure
	for sec, s := range secMap {
		weight := 0.0
		if totalAbsReturn > 0 {
			weight = s.absReturn / totalAbsReturn
		}
		sectorExp = append(sectorExp, SectorExposure{
			Sector:      sec,
			SectorLabel: h.sectorLabel(sec),
			Weight:      weight,
			EstValue:    weight * portfolioValue,
		})
	}

	var fe FactorExposureInline
	if cnt > 0 {
		fe = FactorExposureInline{
			Momentum: totalM / float64(cnt),
			Value:    totalV / float64(cnt),
			Quality:  totalQ / float64(cnt),
			Agent:    totalA / float64(cnt),
			Total:    totalT / float64(cnt),
		}
	}

	return sectorExp, fe
}

// PnLAttributionResponse is the response for GET /api/dashboard/pnl-attribution.
type PnLAttributionResponse struct {
	SnapshotTime      time.Time           `json:"snapshot_time"`
	SessionID         string              `json:"session_id"`
	StartingValue     float64             `json:"starting_value"`
	CurrentValue      float64             `json:"current_value"`
	CumulativePnL     float64             `json:"cumulative_pnl"`
	CumulativeRetPct  float64             `json:"cumulative_return_pct"`
	AgentAttribution  []AgentAttribution  `json:"agent_attribution"`
	SectorAttribution []SectorAttribution `json:"sector_attribution"`
	FactorAttribution FactorAttribution   `json:"factor_attribution"`
	SymbolAttribution []SymbolAttribution `json:"symbol_attribution"`
}

type AgentAttribution struct {
	AgentID     string  `json:"agent_id"`
	AgentName   string  `json:"agent_name"`
	Layer       string  `json:"layer"`
	TotalReturn float64 `json:"total_return"`
	Count       int     `json:"count"`
	AvgReturn   float64 `json:"avg_return"`
}

type SectorAttribution struct {
	Sector      string  `json:"sector"`
	SectorLabel string  `json:"sector_label"`
	TotalReturn float64 `json:"total_return"`
	Count       int     `json:"count"`
	AvgReturn   float64 `json:"avg_return"`
}

type FactorAttribution struct {
	Momentum FactorDetail `json:"momentum"`
	Value    FactorDetail `json:"value"`
	Quality  FactorDetail `json:"quality"`
	Agent    FactorDetail `json:"agent"`
	Total    FactorDetail `json:"total"`
}

type FactorDetail struct {
	AvgScore     float64 `json:"avg_score"`
	AvgReturn    float64 `json:"avg_return"`
	Contribution float64 `json:"contribution"`
}

type SymbolAttribution struct {
	Symbol      string  `json:"symbol"`
	TotalReturn float64 `json:"total_return"`
	Count       int     `json:"count"`
	AvgReturn   float64 `json:"avg_return"`
	Side        string  `json:"side"`
}

type RiskExposureResponse struct {
	SnapshotTime     time.Time               `json:"snapshot_time"`
	VaR95            float64                 `json:"var_95"`
	VaR99            float64                 `json:"var_99"`
	CVaR95           float64                 `json:"cvar_95"`
	MaxDrawdownPct   float64                 `json:"max_drawdown_pct"`
	PortfolioValue   float64                 `json:"portfolio_value"`
	CashRatio        float64                 `json:"cash_ratio"`
	PositionCount    int                     `json:"position_count"`
	SectorExposure   []SectorExposure        `json:"sector_exposure"`
	FactorExposure   FactorExposureInline    `json:"factor_exposure"`
	Concentration    []PositionConcentration `json:"concentration"`
	DataPoints       int                     `json:"data_points"`
	InsufficientData bool                    `json:"insufficient_data"`
	// VarAvailable reports whether the return history meets VaR's minimum
	// observation bar (risk.MinObservationsForVaR). Below it, VaR/CVaR are
	// zero-valued placeholders and the UI must show the observation period
	// instead (#1785-B: 182 points rendered as "VaR 0.0" was misleading).
	VarAvailable bool `json:"var_available"`
}

type SectorExposure struct {
	Sector      string  `json:"sector"`
	SectorLabel string  `json:"sector_label"`
	Weight      float64 `json:"weight"`
	EstValue    float64 `json:"est_value"`
}

type FactorExposureInline struct {
	Momentum float64 `json:"momentum"`
	Value    float64 `json:"value"`
	Quality  float64 `json:"quality"`
	Agent    float64 `json:"agent"`
	Total    float64 `json:"total"`
}

type PositionConcentration struct {
	Symbol      string  `json:"symbol"`
	MarketValue float64 `json:"market_value"`
	Weight      float64 `json:"weight"`
}

// HandlePnLAttribution returns P&L attribution breakdown by agent, sector, symbol, and factor.
func (h *Handlers) HandlePnLAttribution(r *http.Request) (int, any) {
	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return http.StatusOK, PnLAttributionResponse{}
		}
		logging.Warn("live_handler", "sessions_dir_unreadable", logging.Err(err))
		return http.StatusOK, PnLAttributionResponse{}
	}

	latestSession := ""
	var latestSummary domain.SessionSummary
	var allSummaries []domain.SessionSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var s domain.SessionSummary
		if err := json.Unmarshal(bytes, &s); err != nil {
			logging.Warn("live_handler", "corrupted_summary_skipped", logging.Err(err))
			continue
		}
		allSummaries = append(allSummaries, s)
		if s.SessionID > latestSession {
			latestSession = s.SessionID
			latestSummary = s
		}
	}

	if latestSession == "" {
		return http.StatusOK, PnLAttributionResponse{}
	}

	slices.SortFunc(allSummaries, func(a, b domain.SessionSummary) int {
		return strings.Compare(a.SessionID, b.SessionID)
	})

	var startingValue, currentValue float64
	if len(allSummaries) >= 2 {
		startingValue = allSummaries[0].PortfolioValue
	}
	currentValue = latestSummary.PortfolioValue
	cumulativePnL := currentValue - startingValue
	var cumulativeRetPct float64
	if startingValue > 0 {
		cumulativeRetPct = cumulativePnL / startingValue
	}

	outcomes, _ := loadRecommendationOutcomes(h.LedgerDir, latestSession)
	symSectorMap := buildSymbolSectorMap(h.Classifier)
	var (
		agentMap                                    = make(map[string]*AgentAttribution)
		sectorMap                                   = make(map[string]*SectorAttribution)
		symbolMap                                   = make(map[string]*SymbolAttribution)
		fMomentum, fValue, fQuality, fAgent, fTotal float64
		fCount                                      int
	)

	for _, oc := range outcomes {
		if !oc.PassedGuards || oc.ForwardReturn == 0 {
			continue
		}
		if oc.AgentID == "" || oc.Symbol == "" {
			continue
		}

		if agentMap[oc.AgentID] == nil {
			agentMap[oc.AgentID] = &AgentAttribution{AgentID: oc.AgentID, Layer: h.agentLayer(oc.AgentID)}
		}
		agentMap[oc.AgentID].TotalReturn += oc.ForwardReturn
		agentMap[oc.AgentID].Count++

		sector := getSymbolSector(oc.Symbol, symSectorMap)
		if sectorMap[sector] == nil {
			sectorMap[sector] = &SectorAttribution{Sector: sector, SectorLabel: h.sectorLabel(sector)}
		}
		sectorMap[sector].TotalReturn += oc.ForwardReturn
		sectorMap[sector].Count++

		if symbolMap[oc.Symbol] == nil {
			symbolMap[oc.Symbol] = &SymbolAttribution{Symbol: oc.Symbol, Side: string(oc.Side)}
		}
		symbolMap[oc.Symbol].TotalReturn += oc.ForwardReturn
		symbolMap[oc.Symbol].Count++

		fMomentum += oc.FactorScores.Momentum
		fValue += oc.FactorScores.Value
		fQuality += oc.FactorScores.Quality
		fAgent += oc.FactorScores.Agent
		fTotal += oc.FactorScores.Total
		fCount++
	}

	var agentAttr []AgentAttribution
	for _, a := range agentMap {
		if a.Count > 0 {
			a.AvgReturn = a.TotalReturn / float64(a.Count)
			a.AgentName = a.AgentID
		}
		agentAttr = append(agentAttr, *a)
	}
	var sectorAttr []SectorAttribution
	for _, s := range sectorMap {
		if s.Count > 0 {
			s.AvgReturn = s.TotalReturn / float64(s.Count)
		}
		sectorAttr = append(sectorAttr, *s)
	}
	var symbolAttr []SymbolAttribution
	for _, s := range symbolMap {
		if s.Count > 0 {
			s.AvgReturn = s.TotalReturn / float64(s.Count)
		}
		symbolAttr = append(symbolAttr, *s)
	}

	var factorAttr FactorAttribution
	if fCount > 0 {
		avgM, avgV, avgQ, avgA, avgT := fMomentum/float64(fCount), fValue/float64(fCount), fQuality/float64(fCount), fAgent/float64(fCount), fTotal/float64(fCount)
		avgRet := float64(0)
		if len(outcomes) > 0 {
			var sumRet float64
			for _, oc := range outcomes {
				if oc.PassedGuards {
					sumRet += oc.ForwardReturn
				}
			}
			avgRet = sumRet / float64(len(outcomes))
		}
		factorAttr = FactorAttribution{
			Momentum: FactorDetail{AvgScore: avgM, AvgReturn: avgRet, Contribution: avgM * avgRet},
			Value:    FactorDetail{AvgScore: avgV, AvgReturn: avgRet, Contribution: avgV * avgRet},
			Quality:  FactorDetail{AvgScore: avgQ, AvgReturn: avgRet, Contribution: avgQ * avgRet},
			Agent:    FactorDetail{AvgScore: avgA, AvgReturn: avgRet, Contribution: avgA * avgRet},
			Total:    FactorDetail{AvgScore: avgT, AvgReturn: avgRet, Contribution: avgT * avgRet},
		}
	}

	return http.StatusOK, PnLAttributionResponse{
		SnapshotTime:      latestSummary.RecordedAt,
		SessionID:         latestSession,
		StartingValue:     startingValue,
		CurrentValue:      currentValue,
		CumulativePnL:     cumulativePnL,
		CumulativeRetPct:  cumulativeRetPct,
		AgentAttribution:  agentAttr,
		SectorAttribution: sectorAttr,
		FactorAttribution: factorAttr,
		SymbolAttribution: symbolAttr,
	}
}

// HandleRiskExposure returns risk metrics including VaR, CVaR, max drawdown, and sector/factor exposure.
func (h *Handlers) HandleRiskExposure(r *http.Request) (int, any) {
	sessionsDir := filepath.Join(h.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return http.StatusOK, RiskExposureResponse{
				SnapshotTime:     time.Now(),
				InsufficientData: true,
			}
		}
		logging.Warn("live_handler", "sessions_dir_unreadable", logging.Err(err))
		return http.StatusOK, RiskExposureResponse{
			SnapshotTime:     time.Now(),
			InsufficientData: true,
		}
	}

	type sessionEntry struct {
		name  string
		value float64
	}
	sessions := make([]sessionEntry, 0, len(entries))
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
			logging.Warn("live_handler", "corrupted_summary_skipped", logging.Err(err))
			continue
		}
		sessions = append(sessions, sessionEntry{name: entry.Name(), value: summary.PortfolioValue})
	}

	slices.SortFunc(sessions, func(a, b sessionEntry) int {
		return strings.Compare(a.name, b.name)
	})

	portfolioValues := make([]float64, len(sessions))
	for i, s := range sessions {
		portfolioValues[i] = s.value
	}

	dailyReturns := make([]float64, 0, max(0, len(portfolioValues)-1))
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			dailyReturns = append(dailyReturns, (portfolioValues[i]-portfolioValues[i-1])/portfolioValues[i-1])
		}
	}

	var snap domain.RiskSnapshot
	var insufficient bool
	var varAvailable bool
	if len(dailyReturns) >= 30 {
		snap = risk.ComputeRiskSnapshot(dailyReturns, portfolioValues)
		// VaR/CVaR are zero placeholders below 252 observations — surface the
		// gap explicitly so the UI can show the observation period (#1785-B).
		varAvailable = len(dailyReturns) >= risk.MinObservationsForVaR
	} else {
		insufficient = true
	}

	liveBasePath := filepath.Join(h.WorkDir, livestore.DefaultLiveStateBasePath)
	portfolio, err := livestore.LoadLastPortfolioState(liveBasePath)
	if err != nil {
		logging.Warn("live_handler", "load_portfolio_state_failed", logging.Err(err))
	}
	positions, err := livestore.LoadLastPositions(liveBasePath)
	if err != nil {
		logging.Warn("live_handler", "load_positions_failed", logging.Err(err))
	}

	var totalMV float64
	for _, p := range positions {
		totalMV += p.MarketValue
	}
	portfolioValue := portfolio.Cash + totalMV
	var cashRatio float64
	if portfolioValue > 0 {
		cashRatio = portfolio.Cash / portfolioValue
	}

	outcomes, _ := loadRecommendationOutcomes(h.LedgerDir, "")
	symSectorMap := buildSymbolSectorMap(h.Classifier)
	sectorWeights, factorExp := h.computeSectorFactorExposure(outcomes, portfolioValue, symSectorMap)

	var concentration []PositionConcentration
	posList := make([]domain.Position, 0, len(positions))
	for _, p := range positions {
		posList = append(posList, p)
	}
	slices.SortFunc(posList, func(a, b domain.Position) int {
		if b.MarketValue == a.MarketValue {
			return 0
		}
		if b.MarketValue > a.MarketValue {
			return 1
		}
		return -1
	})
	for i := 0; i < len(posList) && i < 5; i++ {
		p := posList[i]
		w := float64(0)
		if portfolioValue > 0 {
			w = p.MarketValue / portfolioValue
		}
		concentration = append(concentration, PositionConcentration{
			Symbol:      p.Symbol,
			MarketValue: p.MarketValue,
			Weight:      w,
		})
	}

	return http.StatusOK, RiskExposureResponse{
		SnapshotTime:     time.Now(),
		VaR95:            snap.VaR95,
		VaR99:            snap.VaR99,
		CVaR95:           snap.CVaR95,
		MaxDrawdownPct:   snap.MaxDrawdownPct,
		PortfolioValue:   portfolioValue,
		CashRatio:        cashRatio,
		PositionCount:    len(positions),
		SectorExposure:   sectorWeights,
		FactorExposure:   factorExp,
		Concentration:    concentration,
		DataPoints:       len(dailyReturns),
		InsufficientData: insufficient,
		VarAvailable:     varAvailable,
	}
}

// HandleLiveStatus returns the current live trading status.
func (h *Handlers) HandleLiveStatus(r *http.Request) (int, any) {
	status := h.getService().LoadLiveStatus()
	return http.StatusOK, status
}

// HandlePortfolioState returns the current portfolio state with positions.
func (h *Handlers) HandlePortfolioState(r *http.Request) (int, any) {
	state := h.getService().LoadPortfolioState()
	return http.StatusOK, state
}

func (h *Handlers) HandleTradeHistory(r *http.Request) (int, any) {
	trades := h.getService().LoadTradeHistory()
	if trades == nil {
		trades = []domain.TradeRecord{}
	}
	return http.StatusOK, trades
}

func loadRecommendationOutcomes(ledgerDir, sessionID string) ([]domain.RecommendationOutcome, error) {
	sessionsDir := filepath.Join(ledgerDir, "sessions")
	if sessionID == "" {
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			return nil, err
		}
		var latest string
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() > latest {
				latest = entry.Name()
			}
		}
		sessionID = latest
	} else {
		if err := shared.ValidateSessionID(sessionID); err != nil {
			return nil, err
		}
		entries, err := os.ReadDir(sessionsDir)
		if err != nil {
			return nil, err
		}
		found := false
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() == sessionID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
	}
	path := filepath.Join(sessionsDir, sessionID, "recommendation_outcomes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var outcomes []domain.RecommendationOutcome
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var oc domain.RecommendationOutcome
		if err := json.Unmarshal([]byte(line), &oc); err != nil {
			logging.Warn("live_handler", "corrupted_outcome_skipped", logging.Err(err))
			continue
		}
		outcomes = append(outcomes, oc)
	}
	return outcomes, scanner.Err()
}

func buildSymbolSectorMap(classifier *industry.ClassificationTree) map[string]string {
	m := make(map[string]string)
	if classifier == nil {
		return m
	}
	for _, seg := range classifier.GetAllSegments() {
		for _, sym := range seg.RepresentativeStocks {
			m[sym] = seg.ID
		}
	}
	return m
}
