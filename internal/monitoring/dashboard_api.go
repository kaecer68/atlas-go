package monitoring

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/experiment"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/replay"
	"github.com/kaecer68/atlas-go/internal/risk"
)

type DashboardAPI struct {
	workDir            string
	ledgerDir          string
	baselinePath       string
	narrativeEngine    *narrative.NarrativeEngine
	macroIngestor      *narrative.MacroIngestor
	geoProvider        narrative.GeopoliticalRiskProvider
	taiwanGeoProvider  *narrative.CompositeTaiwanGeopoliticalProvider
	taiwanStressCalc   *narrative.TaiwanStressCalculator
	reportGenerator    *narrative.ReportGenerator
	pool               *pgxpool.Pool
	backtestMu         sync.Mutex
	backtestRunning    bool
	backtestStatus     map[string]interface{}
	metricsCollector   *MetricsCollector
	metricsHistory     *MetricsHistory
	industryClassifier *industry.ClassificationTree
	seasonalEngine     *industry.SeasonalEngine
	cycleTracker       *industry.CycleTracker
	linkageAnalyzer    *industry.LinkageAnalyzer
	riskMonitor        *industry.RiskMonitor
	healthManager      *portfolio.AgentHealthManager
	dataQualityChecker *DataQualityChecker
	janusEngine        *janus.Engine
}

type MacroRadarResponse struct {
	SessionID     string                    `json:"session_id"`
	Regime        domain.Regime             `json:"regime"`
	GuardOutcomes []domain.GuardOutcome     `json:"guard_outcomes"`
	BrokerRuntime domain.BrokerRuntimeAudit `json:"broker_runtime"`
	RecordedAt    time.Time                 `json:"recorded_at"`
}

type AgentObservatoryResponse struct {
	SessionID              string                    `json:"session_id"`
	NextExperimentAgentID  string                    `json:"next_experiment_agent_id"`
	WeakestAgentScorecards []domain.Scorecard        `json:"weakest_agent_scorecards"`
	BrokerRuntime          domain.BrokerRuntimeAudit `json:"broker_runtime"`
	RecordedAt             time.Time                 `json:"recorded_at"`
}

// PortfolioStateResponse is the response for GET /api/dashboard/portfolio-state.
type PortfolioStateResponse struct {
	SnapshotTime     time.Time          `json:"snapshot_time"`
	Cash             float64            `json:"cash"`
	PortfolioValue   float64            `json:"portfolio_value"`
	CumulativePnL    float64            `json:"cumulative_pnl"`
	CumulativePnLPct float64            `json:"cumulative_pnl_pct"`
	CurrentDrawdown  float64            `json:"current_drawdown"`
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
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

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

type ForecastVsRealityItem struct {
	ExperimentID   string                  `json:"experiment_id"`
	ProposalID     string                  `json:"proposal_id"`
	CommitID       string                  `json:"commit_id"`
	ApprovalID     string                  `json:"approval_id"`
	TargetAgentID  string                  `json:"target_agent_id"`
	Skill          string                  `json:"skill"`
	MutationType   string                  `json:"mutation_type"`
	Status         domain.ExperimentStatus `json:"status"`
	BaselineValue  float64                 `json:"baseline_value"`
	CandidateValue float64                 `json:"candidate_value"`
	RecordedAt     time.Time               `json:"recorded_at"`
}

type ForecastVsRealityResponse struct {
	Items         []ForecastVsRealityItem   `json:"items"`
	BrokerRuntime domain.BrokerRuntimeAudit `json:"broker_runtime"`
}

func NewDashboardAPI(workDir, ledgerDir string, metricsCollector *MetricsCollector) *DashboardAPI {
	provider := marketdata.NewCompositeMacroProvider(
		marketdata.NewYahooFinanceMacroProvider(),
		marketdata.NewTWSECapitalFlowProvider(filepath.Join(workDir, "data/state/capital_flow")),
		marketdata.NewTWSEBalanceProvider(filepath.Join(workDir, "data/state/margin")),
		marketdata.NewExportStatisticsProvider(filepath.Join(workDir, "data/state/export")),
		marketdata.NewTSMCRevenueProvider(filepath.Join(workDir, "data/state/tsmc_revenue")),
	)
	geoProvider := narrative.NewCompositeGeopoliticalProvider(
		narrative.NewRSSGeopoliticalProvider(),
		narrative.NewGDELTGeopoliticalProvider(),
	)
	taiwanGeoProvider := narrative.NewCompositeTaiwanGeopoliticalProvider(
		narrative.NewTaiwanRSSGeopoliticalProvider(),
	)
	if metricsCollector == nil {
		metricsCollector = NewMetricsCollector()
	}
	return &DashboardAPI{
		workDir:            workDir,
		ledgerDir:          ledgerDir,
		baselinePath:       filepath.Join(workDir, "data/state/baseline_policy.json"),
		narrativeEngine:    narrative.NewNarrativeEngine(),
		macroIngestor:      narrative.NewMacroIngestor(provider, filepath.Join(workDir, "data/state/macro")),
		geoProvider:        geoProvider,
		taiwanGeoProvider:  taiwanGeoProvider,
		taiwanStressCalc:   narrative.NewTaiwanStressCalculator(geoProvider),
		reportGenerator:    narrative.NewReportGenerator(),
		metricsCollector:   metricsCollector,
		metricsHistory:     NewMetricsHistory(1000),
		industryClassifier: industry.DefaultClassification(),
		seasonalEngine:     industry.NewSeasonalEngine(),
		cycleTracker:       industry.NewCycleTracker(),
		linkageAnalyzer:    industry.NewLinkageAnalyzer(),
		riskMonitor:        industry.NewRiskMonitor(),
		healthManager:      portfolio.NewAgentHealthManager(),
		dataQualityChecker: NewDataQualityChecker(workDir, ledgerDir),
	}
}

func (a *DashboardAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/macro-radar", a.handleMacroRadar)
	mux.HandleFunc("/api/dashboard/agent-observatory", a.handleAgentObservatory)
	mux.HandleFunc("/api/dashboard/forecast-vs-reality", a.handleForecastVsReality)
	mux.HandleFunc("/api/dashboard/experiment-inbox", a.handleExperimentInbox)
	mux.HandleFunc("/api/dashboard/system-health", a.handleSystemHealth)
	mux.HandleFunc("/api/dashboard/recommendation-pipeline", a.handleRecommendationPipeline)
	mux.HandleFunc("/api/dashboard/universe-overlap", a.handleUniverseOverlap)
	mux.HandleFunc("/api/dashboard/data-channels", a.handleDataChannels)
	mux.HandleFunc("/api/channels/ingest", a.handleChannelsIngest)
	mux.HandleFunc("/health", a.handleHealth)
	mux.HandleFunc("/api/dashboard/sessions", a.handleSessions)
	mux.HandleFunc("/api/report/latest", a.handleLatestReport)
	mux.HandleFunc("/api/report/list", a.handleReportList)
	mux.HandleFunc("/api/dashboard/risk", a.handleRiskMetrics)
	mux.HandleFunc("/api/dashboard/daily-summary", a.handleDailySummary)
	mux.HandleFunc("/api/dashboard/retail-sentiment", a.handleRetailSentiment)
	mux.HandleFunc("/api/dashboard/capital-phase", a.handleCapitalPhase)
	mux.HandleFunc("/api/dashboard/tax-snapshot", a.handleTaxSnapshot)
	mux.HandleFunc("/api/dashboard/metrics", a.handleMetrics)
	mux.HandleFunc("/api/dashboard/metrics/trend", a.handleMetricsTrend)
	mux.HandleFunc("/api/dashboard/data-quality", a.handleDataQuality)
}

func (a *DashboardAPI) RegisterIndustryRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/industry/classification", a.handleIndustryClassification)
	mux.HandleFunc("/api/industry/seasonality", a.handleIndustrySeasonality)
	mux.HandleFunc("/api/industry/seasonality/calendar", a.handleIndustrySeasonalityCalendar)
	mux.HandleFunc("/api/industry/cycle", a.handleIndustryCycle)
	mux.HandleFunc("/api/industry/linkage", a.handleIndustryLinkage)
	mux.HandleFunc("/api/industry/risk", a.handleIndustryRisk)
	mux.HandleFunc("/api/industry/overview", a.handleIndustryOverview)
	mux.HandleFunc("/api/industry/shock-simulation", a.handleShockSimulation)
	mux.HandleFunc("/api/industry/graph", a.handleIndustryGraph)
}

// RegisterSwaggerRoutes mounts Swagger UI and the OpenAPI spec.
func (a *DashboardAPI) RegisterSwaggerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/docs", a.handleSwaggerUI)
	mux.HandleFunc("/api/docs/swagger.json", a.handleSwaggerJSON)
}

// RegisterNarrativeRoutes mounts narrative analysis endpoints and the narrative dashboard.
func (a *DashboardAPI) RegisterNarrativeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/narrative/events", a.handleNarrativeEvents)
	mux.HandleFunc("/api/narrative/chains", a.handleNarrativeChains)
	mux.HandleFunc("/api/narrative/models", a.handleNarrativeModels)
	mux.HandleFunc("/api/narrative/templates", a.handleNarrativeTemplates)
	mux.HandleFunc("/api/narrative/seasonal", a.handleSeasonalAnalysis)
}

// RegisterControlRoutes mounts human intervention control endpoints.
func (a *DashboardAPI) RegisterControlRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/control/pause-agent", a.handlePauseAgent)
	mux.HandleFunc("/api/control/resume-agent", a.handleResumeAgent)
	mux.HandleFunc("/api/control/set-model-weight", a.handleSetModelWeight)
	mux.HandleFunc("/api/control/sector-ban", a.handleSectorBan)
	mux.HandleFunc("/api/control/approve-recommendation", a.handleApproveRecommendation)
	mux.HandleFunc("/api/control/reject-recommendation", a.handleRejectRecommendation)
	mux.HandleFunc("/api/control/audit-log", a.handleAuditLog)
	mux.HandleFunc("/api/control/active-overrides", a.handleActiveOverrides)
	mux.HandleFunc("/api/agents/health", a.handleAgentHealth)
}

// RegisterMacroRoutes mounts macro data snapshot endpoints.
func (a *DashboardAPI) RegisterMacroRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/macro/ingest", a.handleMacroIngest)
	mux.HandleFunc("/api/macro/snapshot/latest", a.handleMacroSnapshotLatest)
	mux.HandleFunc("/api/macro/snapshot/history", a.handleMacroSnapshotHistory)
	mux.HandleFunc("/api/macro/capital-flow/latest", a.handleCapitalFlowLatest)
	mux.HandleFunc("/api/taiwan/stress-index", a.handleTaiwanStressIndex)
}

// RegisterPhase3Routes mounts Phase 3 advanced systems observability endpoints.
func (a *DashboardAPI) RegisterPhase3Routes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/phase3-status", a.handlePhase3Status)
}

func (a *DashboardAPI) RegisterLiveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/live-status", a.handleLiveStatus)
	mux.HandleFunc("/api/dashboard/portfolio-state", a.handlePortfolioState)
	mux.HandleFunc("/api/dashboard/pnl-attribution", a.handlePnLAttribution)
	mux.HandleFunc("/api/dashboard/risk-exposure", a.handleRiskExposure)
}

// RegisterExperimentRoutes mounts experiment lifecycle endpoints.
func (a *DashboardAPI) RegisterExperimentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/experiment/promote", a.handleExperimentPromote)
	mux.HandleFunc("/api/experiment/revert", a.handleExperimentRevert)
	mux.HandleFunc("/api/experiment/history", a.handleExperimentHistory)
	mux.HandleFunc("/api/experiment/judge", a.handleJudgeExperiment)
	mux.HandleFunc("/api/experiment/diff", a.handleExperimentDiff)
}

// RegisterBacktestRoutes mounts backtest execution endpoints.
func (a *DashboardAPI) RegisterBacktestRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/backtest/run", a.handleBacktestRun)
	mux.HandleFunc("/api/backtest/status", a.handleBacktestStatus)
}

// SetPool injects an optional database pool for DB-backed channel health.
func (a *DashboardAPI) SetPool(pool *pgxpool.Pool) {
	a.pool = pool
}

// SetHealthManager injects an optional agent health manager.
func (a *DashboardAPI) SetHealthManager(m *portfolio.AgentHealthManager) {
	a.healthManager = m
}

// SetJanusEngine injects an optional JANUS engine for regime health monitoring.
func (a *DashboardAPI) SetJanusEngine(e *janus.Engine) {
	a.janusEngine = e
}

func (a *DashboardAPI) handleLiveStatus(w http.ResponseWriter, r *http.Request) {
	status := a.loadLiveStatus()
	writeJSON(w, http.StatusOK, status)
}

func (a *DashboardAPI) handlePortfolioState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	liveBasePath := filepath.Join(a.workDir, live.DefaultLiveStateBasePath)

	portfolio, err := live.LoadLastPortfolioState(liveBasePath)
	if err != nil {
		log.Printf("[DashboardAPI] warn: failed to load portfolio state: %v", err)
		writeJSON(w, http.StatusOK, PortfolioStateResponse{})
		return
	}

	posMap, err := live.LoadLastPositions(liveBasePath)
	if err != nil {
		log.Printf("[DashboardAPI] warn: failed to load positions: %v", err)
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

	equityCurve, _ := a.buildEquityCurve()

	resp := PortfolioStateResponse{
		SnapshotTime:     portfolio.LastUpdated,
		Cash:             portfolio.Cash,
		PortfolioValue:   portfolio.Cash + totalMarketValue,
		CumulativePnL:    portfolio.RealizedPnL + portfolio.UnrealizedPnL,
		CumulativePnLPct: 0,
		CurrentDrawdown:  0,
		PositionsCount:   len(positions),
		Positions:        positions,
		EquityCurve:      equityCurve,
	}
	if portfolio.Cash > 0 {
		resp.CumulativePnLPct = resp.CumulativePnL / portfolio.Cash
	}

	writeJSON(w, http.StatusOK, resp)
}

func (a *DashboardAPI) handlePnLAttribution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "read sessions")
		return
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
			continue
		}
		allSummaries = append(allSummaries, s)
		if s.SessionID > latestSession {
			latestSession = s.SessionID
			latestSummary = s
		}
	}

	if latestSession == "" {
		writeJSON(w, http.StatusOK, PnLAttributionResponse{})
		return
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

	outcomes, _ := a.loadRecommendationOutcomes(latestSession)
	symSectorMap := a.buildSymbolSectorMap()
	var (
		agentMap                                    = make(map[string]*AgentAttribution)
		sectorMap                                   = make(map[string]*SectorAttribution)
		symbolMap                                   = make(map[string]*SymbolAttribution)
		fMomentum, fValue, fQuality, fAgent, fTotal float64
		fCount                                      int
	)
	sectorLabelMap := map[string]string{
		"semiconductor":   "半導體",
		"ai_supply_chain": "AI供應鏈",
		"robotics":        "機器人",
		"financials":      "金融",
		"shipping":        "航運",
		"energy":          "能源",
		"electronics":     "電子",
		"consumer":        "消費",
		"industrial":      "工業",
		"other":           "其他",
	}
	agentLayerMap := map[string]string{
		"taiwan-macro-01":       "macro",
		"foreign-flow-01":       "macro",
		"semi-desk-01":          "sector",
		"ai-desk-01":            "sector",
		"growth-momentum-01":    "style",
		"value-yield-01":        "style",
		"technical-breakout-01": "style",
		"earnings-quality-01":   "style",
		"shipping-desk-01":      "sector",
		"financials-desk-01":    "sector",
	}

	for _, oc := range outcomes {
		if !oc.PassedGuards || oc.ForwardReturn == 0 {
			continue
		}
		if oc.AgentID == "" || oc.Symbol == "" {
			continue
		}

		if agentMap[oc.AgentID] == nil {
			agentMap[oc.AgentID] = &AgentAttribution{AgentID: oc.AgentID, Layer: agentLayerMap[oc.AgentID]}
		}
		agentMap[oc.AgentID].TotalReturn += oc.ForwardReturn
		agentMap[oc.AgentID].Count++

		sector := getSymbolSector(oc.Symbol, symSectorMap)
		if sectorMap[sector] == nil {
			sectorMap[sector] = &SectorAttribution{Sector: sector, SectorLabel: sectorLabelMap[sector]}
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

	writeJSON(w, http.StatusOK, PnLAttributionResponse{
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
	})
}

func (a *DashboardAPI) handleRiskExposure(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "read sessions")
		return
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

	dailyReturns := make([]float64, 0, len(portfolioValues)-1)
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			dailyReturns = append(dailyReturns, (portfolioValues[i]-portfolioValues[i-1])/portfolioValues[i-1])
		}
	}

	var snap domain.RiskSnapshot
	var insufficient bool
	if len(dailyReturns) >= 30 {
		snap = risk.ComputeRiskSnapshot(dailyReturns, portfolioValues)
	} else {
		insufficient = true
	}

	liveBasePath := filepath.Join(a.workDir, live.DefaultLiveStateBasePath)
	portfolio, _ := live.LoadLastPortfolioState(liveBasePath)
	positions, _ := live.LoadLastPositions(liveBasePath)

	var totalMV float64
	for _, p := range positions {
		totalMV += p.MarketValue
	}
	portfolioValue := portfolio.Cash + totalMV
	var cashRatio float64
	if portfolioValue > 0 {
		cashRatio = portfolio.Cash / portfolioValue
	}

	outcomes, _ := a.loadRecommendationOutcomes("")
	symSectorMap := a.buildSymbolSectorMap()
	sectorWeights, factorExp := computeSectorFactorExposure(outcomes, portfolioValue, symSectorMap)

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

	writeJSON(w, http.StatusOK, RiskExposureResponse{
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
	})
}

func (a *DashboardAPI) loadRecommendationOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
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
	}
	path := filepath.Join(sessionsDir, sessionID, "recommendation_outcomes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var outcomes []domain.RecommendationOutcome
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var oc domain.RecommendationOutcome
		if err := json.Unmarshal([]byte(line), &oc); err != nil {
			continue
		}
		outcomes = append(outcomes, oc)
	}
	return outcomes, scanner.Err()
}

func (a *DashboardAPI) buildSymbolSectorMap() map[string]string {
	m := make(map[string]string)
	if a.industryClassifier == nil {
		return m
	}
	for _, seg := range a.industryClassifier.GetAllSegments() {
		for _, sym := range seg.RepresentativeStocks {
			m[sym] = seg.ID
		}
	}
	return m
}

func getSymbolSector(symbol string, symMap map[string]string) string {
	if s, ok := symMap[symbol]; ok {
		return s
	}
	return "other"
}

func computeSectorFactorExposure(outcomes []domain.RecommendationOutcome, portfolioValue float64, symSectorMap map[string]string) ([]SectorExposure, FactorExposureInline) {
	sectorLabelMap := map[string]string{
		"semiconductor":   "半導體",
		"ai_supply_chain": "AI供應鏈",
		"robotics":        "機器人",
		"financials":      "金融",
		"shipping":        "航運",
		"energy":          "能源",
		"electronics":     "電子",
		"consumer":        "消費",
		"industrial":      "工業",
		"other":           "其他",
	}

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
			SectorLabel: sectorLabelMap[sec],
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

// buildEquityCurve constructs an equity curve from all session summaries,
// sorted by session trading date ascending.
func (a *DashboardAPI) buildEquityCurve() ([]EquityCurvePoint, error) {
	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no sessions directory")
		}
		return nil, err
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
		return nil, fmt.Errorf("no session data")
	}

	// Sort by date ascending for chart rendering
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
	return curve, nil
}

func (a *DashboardAPI) loadLiveStatus() map[string]interface{} {
	// Load circuit breaker state from persisted file
	cbState := struct {
		State          string    `json:"state"`
		StateChangedAt time.Time `json:"state_changed_at"`
		ConsecutiveSL  int       `json:"consecutive_sl"`
		CooldownUntil  time.Time `json:"cooldown_until"`
		IntradayPeak   float64   `json:"intraday_peak"`
		DayStartValue  float64   `json:"day_start_value"`
	}{
		State: "unknown",
	}
	if data, err := os.ReadFile(filepath.Join(a.workDir, live.DefaultCircuitBreakerStatePath)); err == nil {
		if err := json.Unmarshal(data, &cbState); err != nil {
			log.Printf("[DashboardAPI] warn: failed to unmarshal circuit breaker state: %v", err)
		}
	}

	// Load portfolio from live state store JSONL (last line = latest)
	portfolio := struct {
		Cash          float64 `json:"cash"`
		TotalExposure float64 `json:"total_exposure"`
		AvailableCash float64 `json:"available_cash"`
		DayPnL        float64 `json:"day_pnl"`
		UnrealizedPnL float64 `json:"unrealized_pnl"`
	}{}
	liveBasePath := filepath.Join(a.workDir, live.DefaultLiveStateBasePath)
	if p, err := live.LoadLastPortfolioState(liveBasePath); err == nil {
		portfolio.Cash = p.Cash
		portfolio.TotalExposure = p.TotalExposure
		portfolio.AvailableCash = p.AvailableCash
		portfolio.DayPnL = p.DayPnL
		portfolio.UnrealizedPnL = p.UnrealizedPnL
	} else {
		log.Printf("[DashboardAPI] warn: failed to read portfolio state: %v", err)
	}

	// Load positions count from live state store JSONL
	positions := make(map[string]interface{})
	if posMap, err := live.LoadLastPositions(liveBasePath); err == nil {
		positions = make(map[string]interface{}, len(posMap))
		for k := range posMap {
			positions[k] = posMap[k]
		}
	} else {
		log.Printf("[DashboardAPI] warn: failed to read positions state: %v", err)
	}

	return map[string]interface{}{
		"circuit_breaker": map[string]interface{}{
			"state":            cbState.State,
			"state_changed_at": cbState.StateChangedAt,
			"consecutive_sl":   cbState.ConsecutiveSL,
			"cooldown_until":   cbState.CooldownUntil,
			"intraday_peak":    cbState.IntradayPeak,
			"day_start_value":  cbState.DayStartValue,
		},
		"portfolio": map[string]interface{}{
			"cash":            portfolio.Cash,
			"available_cash":  portfolio.AvailableCash,
			"total_exposure":  portfolio.TotalExposure,
			"day_pnl":         portfolio.DayPnL,
			"unrealized_pnl":  portfolio.UnrealizedPnL,
			"positions_count": len(positions),
		},
		"timestamp": time.Now().UTC(),
	}
}

func (a *DashboardAPI) handleExperimentPromote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ResultPath string `json:"result_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.ResultPath == "" {
		writeJSONError(w, http.StatusBadRequest, "result_path required")
		return
	}
	mgr := baseline.NewManager(a.baselinePath)
	policy, err := mgr.PromoteResult(req.ResultPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("promote failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "version": policy.Version})
}

func (a *DashboardAPI) handleExperimentRevert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Type         string `json:"type"`
		Version      int    `json:"version"`
		ExperimentID string `json:"experiment_id"`
		Reason       string `json:"reason"`
		DryRun       bool   `json:"dry_run"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	target := baseline.RevertTarget{Type: baseline.RevertType(req.Type), Version: req.Version, ExperimentID: req.ExperimentID}
	mgr := baseline.NewManager(a.baselinePath)
	result, err := mgr.Revert(target, req.Reason, req.DryRun)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("revert failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func promotionHistoryToAPI(history []baseline.PromotionRecordWithVersion) []map[string]any {
	result := make([]map[string]any, len(history))
	for i, h := range history {
		result[i] = map[string]any{
			"experiment_id":   h.ExperimentID,
			"target_agent_id": h.TargetAgentID,
			"target_skill":    h.TargetSkill,
			"mutation_type":   h.MutationType,
			"candidate_path":  h.CandidatePath,
			"promoted_at":     h.PromotedAt,
			"status":          h.Status,
			"version_after":   h.VersionAfter,
			"version":         h.Version,
		}
	}
	return result
}

func (a *DashboardAPI) handleExperimentHistory(w http.ResponseWriter, r *http.Request) {
	mgr := baseline.NewManager(a.baselinePath)
	history, err := mgr.GetPromotionHistory()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load history: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": promotionHistoryToAPI(history)})
}

func (a *DashboardAPI) handlePhase3Status(w http.ResponseWriter, r *http.Request) {
	metrics, err := orchestrator.LoadPhase3Metrics("")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load phase3 metrics: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (a *DashboardAPI) handleSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Atlas-Go API Docs</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>
SwaggerUIBundle({
  url: './docs/swagger.json',
  dom_id: '#swagger-ui',
  presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.presets.standalone]
});
</script>
</body>
</html>`))
}

func (a *DashboardAPI) handleSwaggerJSON(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(a.workDir, "docs/swagger.json"))
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "swagger spec not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (a *DashboardAPI) handleMacroRadar(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	summary, err := a.loadSessionSummary(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load macro radar data: %v", err))
		return
	}
	if summary == nil {
		writeJSON(w, http.StatusOK, MacroRadarResponse{})
		return
	}

	resp := MacroRadarResponse{
		SessionID:     summary.SessionID,
		Regime:        summary.Regime,
		GuardOutcomes: append([]domain.GuardOutcome(nil), summary.GuardOutcomes...),
		BrokerRuntime: summary.BrokerRuntime,
		RecordedAt:    summary.RecordedAt,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *DashboardAPI) handleAgentObservatory(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	limit, err := parseLimit(r, 5, 50)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	summary, err := a.loadSessionSummary(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load agent observatory summary: %v", err))
		return
	}

	store := ledger.NewStore(a.ledgerDir)
	outcomes, err := store.LoadOutcomes()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load recommendation outcomes: %v", err))
		return
	}
	scorecards := ledger.BuildScorecards(outcomes)
	if len(scorecards) > limit {
		scorecards = scorecards[:limit]
	}

	resp := AgentObservatoryResponse{
		WeakestAgentScorecards: scorecards,
	}
	if summary != nil {
		resp.SessionID = summary.SessionID
		resp.NextExperimentAgentID = summary.NextExperimentAgentID
		resp.BrokerRuntime = summary.BrokerRuntime
		resp.RecordedAt = summary.RecordedAt
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *DashboardAPI) handleForecastVsReality(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r, 20, 100)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))

	items, err := a.loadForecastVsRealityItems(agentID, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load forecast-vs-reality data: %v", err))
		return
	}
	resp := ForecastVsRealityResponse{Items: items}
	summary, err := a.loadSessionSummary("")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load forecast-vs-reality summary context: %v", err))
		return
	}
	if summary != nil {
		resp.BrokerRuntime = summary.BrokerRuntime
	}
	writeJSON(w, http.StatusOK, resp)
}

// sessionDateFromID extracts the trading date from a session ID like "session-20260410-daily".
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

func (a *DashboardAPI) loadSessionSummary(sessionID string) (*domain.SessionSummary, error) {
	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	summaries := make([]domain.SessionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if sessionID != "" && entry.Name() != sessionID {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	if sessionID != "" {
		selected := summaries[0]
		return &selected, nil
	}

	// Sort by session trading date (derived from SessionID) descending,
	// then by RecordedAt as a tiebreaker.
	slices.SortFunc(summaries, func(a, b domain.SessionSummary) int {
		aDate := sessionDateFromID(a.SessionID)
		bDate := sessionDateFromID(b.SessionID)
		switch {
		case aDate.After(bDate):
			return -1
		case aDate.Before(bDate):
			return 1
		case a.RecordedAt.After(b.RecordedAt):
			return -1
		case a.RecordedAt.Before(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})
	latest := summaries[0]
	return &latest, nil
}

func (a *DashboardAPI) loadForecastVsRealityItems(agentID string, limit int) ([]ForecastVsRealityItem, error) {
	experimentsDir := filepath.Join(a.ledgerDir, "experiments")
	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]ForecastVsRealityItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(experimentsDir, entry.Name())
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var result domain.PromptExperimentResult
		if err := json.Unmarshal(bytes, &result); err != nil {
			return nil, err
		}
		if agentID != "" && result.Experiment.TargetAgentID != agentID {
			continue
		}
		items = append(items, ForecastVsRealityItem{
			ExperimentID:   result.Experiment.ID,
			ProposalID:     result.Experiment.ProposalID,
			CommitID:       result.Experiment.CommitID,
			ApprovalID:     result.Experiment.ApprovalID,
			TargetAgentID:  result.Experiment.TargetAgentID,
			Skill:          result.Experiment.Skill,
			MutationType:   result.Experiment.MutationType,
			Status:         result.Experiment.Status,
			BaselineValue:  result.Experiment.BaselineValue,
			CandidateValue: result.Experiment.CandidateValue,
			RecordedAt:     result.RecordedAt,
		})
	}

	slices.SortFunc(items, func(a, b ForecastVsRealityItem) int {
		switch {
		case a.RecordedAt.After(b.RecordedAt):
			return -1
		case a.RecordedAt.Before(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})

	if len(items) > limit {
		return items[:limit], nil
	}
	return items, nil
}

// buildMutationSummary extracts a concise parameter delta from baseline vs candidate prompt controls.
func buildMutationSummary(policy baseline.Policy, result domain.PromptExperimentResult) string {
	baselinePrompt := baseline.ResolvePromptOverride(policy, result.Experiment.TargetAgentID, result.Experiment.Skill)
	if baselinePrompt == "" {
		sourcePrompt, err := os.ReadFile(result.Brief.PromptFile)
		if err == nil {
			baselinePrompt = string(sourcePrompt)
		}
	}

	baselineCtrl, _ := domain.ExtractPromptControl(baselinePrompt)
	candidateBytes, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		return result.Experiment.MutationType
	}
	candidateCtrl, _ := domain.ExtractPromptControl(string(candidateBytes))

	parts := make([]string, 0, 4)
	add := func(name string, base, cand int64) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %d→%d", name, base, cand))
		}
	}
	addInt := func(name string, base, cand int) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %d→%d", name, base, cand))
		}
	}
	addBool := func(name string, base, cand bool) {
		if base != cand {
			parts = append(parts, fmt.Sprintf("%s: %t→%t", name, base, cand))
		}
	}

	add("volume_floor", baselineCtrl.VolumeFloor, candidateCtrl.VolumeFloor)
	addInt("volume_downgrade", baselineCtrl.VolumeDowngrade, candidateCtrl.VolumeDowngrade)
	addInt("close_strength_boost", baselineCtrl.CloseStrengthBoost, candidateCtrl.CloseStrengthBoost)
	add("hard_reject_volume", baselineCtrl.HardRejectVolume, candidateCtrl.HardRejectVolume)
	addInt("conviction_floor", baselineCtrl.ConvictionFloor, candidateCtrl.ConvictionFloor)
	addInt("volume_boost", baselineCtrl.VolumeBoost, candidateCtrl.VolumeBoost)
	addInt("neutral_penalty_reduction", baselineCtrl.NeutralPenaltyReduction, candidateCtrl.NeutralPenaltyReduction)
	addBool("require_trend", baselineCtrl.RequireTrend, candidateCtrl.RequireTrend)

	if len(parts) == 0 {
		return result.Experiment.MutationType
	}
	return strings.Join(parts, ", ")
}
func parseLimit(r *http.Request, defaultValue, maxValue int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return defaultValue, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: must be integer")
	}
	if v <= 0 {
		return 0, fmt.Errorf("invalid limit: must be > 0")
	}
	if v > maxValue {
		return maxValue, nil
	}
	return v, nil
}

func (a *DashboardAPI) handleReportList(w http.ResponseWriter, r *http.Request) {
	reportDir := filepath.Join(a.workDir, "reports")
	entries, err := os.ReadDir(reportDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"reports": []any{}})
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read reports dir: %v", err))
		return
	}

	reports := make([]map[string]any, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "backtest_") || !strings.HasSuffix(name, ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		reports = append(reports, map[string]any{
			"filename":   name,
			"path":       "/api/report/latest?file=" + name,
			"updated_at": info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	// Sort by updated_at descending
	slices.SortFunc(reports, func(a, b map[string]any) int {
		aTime, _ := time.Parse(time.RFC3339, a["updated_at"].(string))
		bTime, _ := time.Parse(time.RFC3339, b["updated_at"].(string))
		switch {
		case aTime.After(bTime):
			return -1
		case aTime.Before(bTime):
			return 1
		default:
			return 0
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

func (a *DashboardAPI) handleRiskMetrics(w http.ResponseWriter, r *http.Request) {
	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"message": "no sessions available"})
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read sessions: %v", err))
		return
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
			continue
		}
		sessions = append(sessions, sessionEntry{name: entry.Name(), value: summary.PortfolioValue})
	}

	// Sort chronologically by session directory name to ensure correct daily return ordering
	slices.SortFunc(sessions, func(a, b sessionEntry) int {
		return strings.Compare(a.name, b.name)
	})

	portfolioValues := make([]float64, len(sessions))
	for i, s := range sessions {
		portfolioValues[i] = s.value
	}

	dailyReturns := make([]float64, 0, len(portfolioValues)-1)
	for i := 1; i < len(portfolioValues); i++ {
		if portfolioValues[i-1] > 0 {
			dailyReturns = append(dailyReturns, (portfolioValues[i]-portfolioValues[i-1])/portfolioValues[i-1])
		}
	}

	var snap map[string]float64
	if len(dailyReturns) >= 30 {
		computed := risk.ComputeRiskSnapshot(dailyReturns, portfolioValues)
		snap = map[string]float64{
			"var_95":           computed.VaR95,
			"var_99":           computed.VaR99,
			"cvar_95":          computed.CVaR95,
			"max_drawdown_pct": computed.MaxDrawdownPct,
			"data_points":      float64(len(dailyReturns)),
		}
	} else {
		snap = map[string]float64{
			"var_95":            0,
			"var_99":            0,
			"cvar_95":           0,
			"max_drawdown_pct":  0,
			"data_points":       float64(len(dailyReturns)),
			"insufficient_data": 1,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"risk_snapshot": snap,
		"session_count": len(portfolioValues),
	})
}

// UniverseOverlapResponse exposes agent universe coverage and pairwise overlaps.
type UniverseOverlapResponse struct {
	Agents   []AgentUniverseView       `json:"agents"`
	Matrix   map[string]map[string]int `json:"matrix"`
	Warnings []string                  `json:"warnings"`
}

// AgentUniverseView shows a single agent's universe coverage.
type AgentUniverseView struct {
	AgentID           string                   `json:"agent_id"`
	Name              string                   `json:"name"`
	Layer             string                   `json:"layer"`
	Universe          []string                 `json:"universe"`
	ScreeningCriteria domain.ScreeningCriteria `json:"screening_criteria"`
}

func (a *DashboardAPI) handleUniverseOverlap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	registryPath := filepath.Join(a.workDir, "configs/agents.json")
	registry, err := orchestrator.LoadRegistry(registryPath)
	if err != nil {
		registry = orchestrator.SeedRegistry()
	}

	agents := make([]AgentUniverseView, 0)
	byAgent := make(map[string]map[string]struct{})
	warnings := make([]string, 0)

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		universe := make([]string, len(agent.Universe))
		copy(universe, agent.Universe)
		if len(universe) == 0 {
			universe = orchestrator.DefaultSymbols()
		}
		agents = append(agents, AgentUniverseView{
			AgentID:           agent.ID,
			Name:              agent.Name,
			Layer:             string(agent.Layer),
			Universe:          universe,
			ScreeningCriteria: agent.ScreeningCriteria,
		})
		set := make(map[string]struct{}, len(universe))
		for _, s := range universe {
			set[s] = struct{}{}
		}
		byAgent[agent.ID] = set
		// Only stock-picking layers are expected to have a dedicated universe.
		// Context and control layers falling back to defaults is by design, not a warning.
		if len(agent.Universe) == 0 && isStockPickingLayer(string(agent.Layer)) {
			warnings = append(warnings, fmt.Sprintf("%s 未設定標的池（fallback 至預設值）", agent.ID))
		}
		if len(universe) < 3 {
			warnings = append(warnings, fmt.Sprintf("%s 標的池僅有 %d 檔標的", agent.ID, len(universe)))
		}
	}

	// Track which agents use a fallback universe so we can exclude them from
	// meaningful overlap calculations (their wide coverage creates noise).
	fallbackAgents := make(map[string]bool)
	for _, v := range agents {
		// In our registry, an empty Universe means fallback to DefaultSymbols.
		// We look up the original agent config to confirm.
		for _, agent := range registry.Agents {
			if agent.ID == v.AgentID {
				fallbackAgents[v.AgentID] = len(agent.Universe) == 0
				break
			}
		}
	}

	matrix := make(map[string]map[string]int)
	for idA, setA := range byAgent {
		matrix[idA] = make(map[string]int)
		for idB, setB := range byAgent {
			if idA == idB {
				continue
			}
			overlap := 0
			for s := range setA {
				if _, ok := setB[s]; ok {
					overlap++
				}
			}
			matrix[idA][idB] = overlap
			// Crowding warnings are only meaningful among stock-picking layers.
			// Also skip if either agent uses a fallback universe (wide default coverage creates noise).
			if overlap >= 3 && isStockPickingLayerByID(idA, agents) && isStockPickingLayerByID(idB, agents) && !fallbackAgents[idA] && !fallbackAgents[idB] {
				warnings = append(warnings, fmt.Sprintf("標的池重疊過高：%s ↔ %s（%d 檔）", idA, idB, overlap))
			}
		}
	}

	writeJSON(w, http.StatusOK, UniverseOverlapResponse{
		Agents:   agents,
		Matrix:   matrix,
		Warnings: warnings,
	})
}

// isStockPickingLayer returns true for layers that are expected to originate
// symbol-specific recommendations and therefore have a meaningful universe.
func isStockPickingLayer(layer string) bool {
	return layer == "sector" || layer == "style" || layer == "superinvestor"
}

// isStockPickingLayerByID looks up the layer for an agent ID in the provided views.
func isStockPickingLayerByID(agentID string, views []AgentUniverseView) bool {
	for _, v := range views {
		if v.AgentID == agentID {
			return isStockPickingLayer(v.Layer)
		}
	}
	return false
}

func (a *DashboardAPI) handleLatestReport(w http.ResponseWriter, r *http.Request) {
	reportDir := filepath.Join(a.workDir, "reports")
	entries, err := os.ReadDir(reportDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "no reports directory found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read reports dir: %v", err))
		return
	}

	var latestFile string
	var latestTime time.Time
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "backtest_") || !strings.HasSuffix(name, ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if latestFile == "" || info.ModTime().After(latestTime) {
			latestFile = name
			latestTime = info.ModTime()
		}
	}

	if latestFile == "" {
		writeJSONError(w, http.StatusNotFound, "no backtest report found")
		return
	}

	path := filepath.Join(reportDir, latestFile)
	content, err := os.ReadFile(path)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read report: %v", err))
		return
	}

	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (a *DashboardAPI) handleNarrativeEvents(w http.ResponseWriter, r *http.Request) {
	data := narrative.MarketNarrativeData{
		US10YChangeBps:                parseFloatQuery(r, "us10y_change_bps", 15),
		DXYChangePct:                  parseFloatQuery(r, "dxy_change_pct", 2.0),
		VIXLevel:                      parseFloatQuery(r, "vix_level", 30),
		USD_TWD_ChangePct:             parseFloatQuery(r, "usd_twd_change_pct", 0),
		OilChangePct:                  parseFloatQuery(r, "oil_change_pct", 6.0),
		GoldChangePct:                 parseFloatQuery(r, "gold_change_pct", 2.5),
		JPY_ChangePct:                 parseFloatQuery(r, "jpy_change_pct", 3.0),
		AICapexSentiment:              parseFloatQuery(r, "ai_capex_sentiment", 0.8),
		GeopoliticalGPR:               parseFloatQuery(r, "geopolitical_gpr", 160),
		RetailInstitutionalDivergence: parseFloatQuery(r, "retail_divergence", 0),
		MarginZScore:                  parseFloatQuery(r, "margin_zscore", 0),
	}
	events := a.narrativeEngine.DetectEvents(data)
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (a *DashboardAPI) handleNarrativeChains(w http.ResponseWriter, r *http.Request) {
	data := narrative.MarketNarrativeData{
		US10YChangeBps:    parseFloatQuery(r, "us10y_change_bps", 15),
		DXYChangePct:      parseFloatQuery(r, "dxy_change_pct", 2.0),
		VIXLevel:          parseFloatQuery(r, "vix_level", 30),
		USD_TWD_ChangePct: parseFloatQuery(r, "usd_twd_change_pct", 0),
		OilChangePct:      parseFloatQuery(r, "oil_change_pct", 6.0),
		GoldChangePct:     parseFloatQuery(r, "gold_change_pct", 2.5),
		JPY_ChangePct:     parseFloatQuery(r, "jpy_change_pct", 3.0),
		AICapexSentiment:  parseFloatQuery(r, "ai_capex_sentiment", 0.8),
		GeopoliticalGPR:   parseFloatQuery(r, "geopolitical_gpr", 160),
	}
	events := a.narrativeEngine.DetectEvents(data)
	chains := a.narrativeEngine.MatchChains(events)
	writeJSON(w, http.StatusOK, map[string]any{"chains": chains})
}

func (a *DashboardAPI) handleNarrativeModels(w http.ResponseWriter, r *http.Request) {
	data := narrative.MarketNarrativeData{
		US10YChangeBps:    parseFloatQuery(r, "us10y_change_bps", 15),
		DXYChangePct:      parseFloatQuery(r, "dxy_change_pct", 2.0),
		VIXLevel:          parseFloatQuery(r, "vix_level", 30),
		USD_TWD_ChangePct: parseFloatQuery(r, "usd_twd_change_pct", 0),
		OilChangePct:      parseFloatQuery(r, "oil_change_pct", 6.0),
		GoldChangePct:     parseFloatQuery(r, "gold_change_pct", 2.5),
		JPY_ChangePct:     parseFloatQuery(r, "jpy_change_pct", 3.0),
		AICapexSentiment:  parseFloatQuery(r, "ai_capex_sentiment", 0.8),
		GeopoliticalGPR:   parseFloatQuery(r, "geopolitical_gpr", 160),
	}
	events := a.narrativeEngine.DetectEvents(data)
	themes := make([]string, len(events))
	for i, e := range events {
		themes[i] = e.Theme
	}

	// Evaluate model prediction errors against replay data so weights are live.
	replayPath := filepath.Join(a.workDir, "data/replay/tw_extended_90days.csv")
	if err := a.narrativeEngine.EvaluateModels(replayPath); err != nil {
		log.Printf("[DashboardAPI] EvaluateModels warning: %v", err)
	}

	models := a.narrativeEngine.ActiveModels(themes)
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (a *DashboardAPI) handleNarrativeTemplates(w http.ResponseWriter, r *http.Request) {
	kb := narrative.NewKnowledgeBase()
	templates := kb.ListTemplates()
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates})
}

func parseFloatQuery(r *http.Request, key string, defaultValue float64) float64 {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return defaultValue
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return defaultValue
	}
	return v
}

func (a *DashboardAPI) handlePauseAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AgentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	store := ledger.NewStore(a.ledgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-pause-%s-%d", req.AgentID, time.Now().UnixNano()),
		Type:          "pause_agent",
		TargetAgentID: req.AgentID,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (a *DashboardAPI) handleResumeAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AgentID == "" {
		writeJSONError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	store := ledger.NewStore(a.ledgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-resume-%s-%d", req.AgentID, time.Now().UnixNano()),
		Type:          "resume_agent",
		TargetAgentID: req.AgentID,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (a *DashboardAPI) handleAgentHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if a.healthManager == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"agents":      []*portfolio.AgentHealth{},
			"total":       0,
			"muted_count": 0,
		})
		return
	}

	registryPath := filepath.Join(a.workDir, "configs/agents.json")
	registry, err := orchestrator.LoadRegistry(registryPath)
	if err != nil {
		registry = orchestrator.SeedRegistry()
	}

	agents := make([]*portfolio.AgentHealth, 0)
	mutedCount := 0

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		h := a.healthManager.GetHealth(agent.ID)
		if h == nil {
			h = &portfolio.AgentHealth{
				AgentID: agent.ID,
				Status:  portfolio.HealthStatusHealthy,
			}
		}
		agents = append(agents, h)
		if h.Status == portfolio.HealthStatusMuted {
			mutedCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"agents":      agents,
		"total":       len(agents),
		"muted_count": mutedCount,
	})
}

func (a *DashboardAPI) handleSetModelWeight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ModelID  string  `json:"model_id"`
		Weight   float64 `json:"weight"`
		Reason   string  `json:"reason"`
		Operator string  `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.ModelID == "" {
		writeJSONError(w, http.StatusBadRequest, "model_id required")
		return
	}
	store := ledger.NewStore(a.ledgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-model-%s-%d", req.ModelID, time.Now().UnixNano()),
		Type:          "set_model_weight",
		TargetModelID: req.ModelID,
		Value:         req.Weight,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (a *DashboardAPI) handleSectorBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Sector   string `json:"sector"`
		Banned   bool   `json:"banned"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Sector == "" {
		writeJSONError(w, http.StatusBadRequest, "sector required")
		return
	}
	store := ledger.NewStore(a.ledgerDir)
	interventionType := "sector_unban"
	if req.Banned {
		interventionType = "sector_ban"
	}
	intervention := domain.HumanIntervention{
		ID:           fmt.Sprintf("int-sector-%s-%d", req.Sector, time.Now().UnixNano()),
		Type:         interventionType,
		TargetSector: req.Sector,
		Reason:       req.Reason,
		Operator:     req.Operator,
		RecordedAt:   time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (a *DashboardAPI) handleApproveRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Symbol   string `json:"symbol"`
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	store := ledger.NewStore(a.ledgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-approve-%s-%d", req.Symbol, time.Now().UnixNano()),
		Type:          "approve_rec",
		TargetSymbol:  req.Symbol,
		TargetAgentID: req.AgentID,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (a *DashboardAPI) handleRejectRecommendation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Symbol   string `json:"symbol"`
		AgentID  string `json:"agent_id"`
		Reason   string `json:"reason"`
		Operator string `json:"operator"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	store := ledger.NewStore(a.ledgerDir)
	intervention := domain.HumanIntervention{
		ID:            fmt.Sprintf("int-reject-%s-%d", req.Symbol, time.Now().UnixNano()),
		Type:          "reject_rec",
		TargetSymbol:  req.Symbol,
		TargetAgentID: req.AgentID,
		Reason:        req.Reason,
		Operator:      req.Operator,
		RecordedAt:    time.Now().UTC(),
	}
	if err := store.RecordHumanIntervention(intervention); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("record intervention: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "intervention": intervention})
}

func (a *DashboardAPI) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	store := ledger.NewStore(a.ledgerDir)
	interventions, err := store.LoadHumanInterventions()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load interventions: %v", err))
		return
	}
	// Reverse to show newest first.
	slices.Reverse(interventions)
	writeJSON(w, http.StatusOK, map[string]any{"interventions": interventions})
}

func (a *DashboardAPI) handleActiveOverrides(w http.ResponseWriter, r *http.Request) {
	store := ledger.NewStore(a.ledgerDir)
	interventions, err := store.LoadHumanInterventions()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load interventions: %v", err))
		return
	}

	pausedAgents := make(map[string]bool)
	bannedSectors := make(map[string]bool)
	modelWeights := make(map[string]float64)

	for _, iv := range interventions {
		switch iv.Type {
		case "pause_agent":
			pausedAgents[iv.TargetAgentID] = true
		case "resume_agent":
			delete(pausedAgents, iv.TargetAgentID)
		case "sector_ban":
			bannedSectors[iv.TargetSector] = true
		case "sector_unban":
			delete(bannedSectors, iv.TargetSector)
		case "set_model_weight":
			modelWeights[iv.TargetModelID] = iv.Value
		default:
			// Ignore unknown intervention types.
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"paused_agents":  mapKeys(pausedAgents),
		"banned_sectors": mapKeys(bannedSectors),
		"model_weights":  modelWeights,
	})
}

func (a *DashboardAPI) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *DashboardAPI) handleDailySummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	events := a.loadNarrativeEventsForDate(date)
	recs := a.loadRecommendationsForDate(date)
	risk := a.loadRiskSnapshot()

	report := a.reportGenerator.GenerateDailySummary(date, events, recs, risk)
	writeJSON(w, http.StatusOK, report)
}

func (a *DashboardAPI) loadNarrativeEventsForDate(date string) []narrative.NarrativeEvent {
	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDate := sessionDateFromID(entry.Name())
		if sessionDate.IsZero() {
			continue
		}
		if sessionDate.Format("2006-01-02") != date {
			continue
		}

		eventsPath := filepath.Join(sessionsDir, entry.Name(), "narrative_events.json")
		data, err := os.ReadFile(eventsPath)
		if err != nil {
			return nil
		}

		var events []narrative.NarrativeEvent
		if err := json.Unmarshal(data, &events); err != nil {
			return nil
		}
		return events
	}

	return nil
}

func (a *DashboardAPI) loadRiskSnapshot() *domain.RiskSnapshot {
	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
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

	if len(dailyReturns) >= 30 {
		computed := risk.ComputeRiskSnapshot(dailyReturns, portfolioValues)
		return &domain.RiskSnapshot{
			VaR95:          computed.VaR95,
			VaR99:          computed.VaR99,
			CVaR95:         computed.CVaR95,
			MaxDrawdownPct: computed.MaxDrawdownPct,
		}
	}

	return nil
}

func (a *DashboardAPI) loadRecommendationsForDate(date string) []domain.Recommendation {
	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionDate := sessionDateFromID(entry.Name())
		if sessionDate.IsZero() {
			continue
		}
		if sessionDate.Format("2006-01-02") != date {
			continue
		}

		outcomesPath := filepath.Join(sessionsDir, entry.Name(), "recommendation_outcomes.jsonl")
		data, err := os.ReadFile(outcomesPath)
		if err != nil {
			return nil
		}

		var recs []domain.Recommendation
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var outcome struct {
				AgentID             string                      `json:"AgentID"`
				Skill               string                      `json:"Skill"`
				Layer               string                      `json:"Layer"`
				Symbol              string                      `json:"Symbol"`
				Side                string                      `json:"Side"`
				Conviction          int                         `json:"Conviction"`
				TargetPrice         float64                     `json:"TargetPrice"`
				StopLossPrice       float64                     `json:"StopLossPrice"`
				Reason              string                      `json:"Reason"`
				FactorScores        domain.FactorScores         `json:"factor_scores,omitempty"`
				ConvictionBreakdown *domain.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
			}
			if err := json.Unmarshal([]byte(line), &outcome); err != nil {
				continue
			}
			recs = append(recs, domain.Recommendation{
				Agent:               outcome.AgentID,
				Skill:               outcome.Skill,
				Layer:               domain.AgentLayer(outcome.Layer),
				Symbol:              outcome.Symbol,
				Side:                domain.Side(outcome.Side),
				Conviction:          outcome.Conviction,
				TargetPrice:         outcome.TargetPrice,
				StopLossPrice:       outcome.StopLossPrice,
				Reason:              outcome.Reason,
				FactorScores:        outcome.FactorScores,
				ConvictionBreakdown: outcome.ConvictionBreakdown,
			})
		}
		return recs
	}

	return nil
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func (a *DashboardAPI) handleMacroIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	events, snap, err := a.macroIngestor.Ingest(r.Context())
	stateDir := filepath.Join(a.workDir, "data/state")
	if err != nil {
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("us_yahoo", "error", err.Error())
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("jpy_yahoo", "error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("ingest failed: %v", err))
		return
	}
	NewChannelHealthStoreWithPool(stateDir, a.pool).Record("us_yahoo", "ok", "")
	NewChannelHealthStoreWithPool(stateDir, a.pool).Record("jpy_yahoo", "ok", "")
	writeJSON(w, http.StatusOK, map[string]any{
		"events":   events,
		"snapshot": snap,
	})
}

func (a *DashboardAPI) handleChannelsIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stateDir := filepath.Join(a.workDir, "data/state")
	var wg sync.WaitGroup
	var macroErr, geoErr, capFlowErr, exportErr, tsmcErr, twGeoErr, janusErr, tejErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		events, snap, err := a.macroIngestor.Ingest(r.Context())
		if err != nil {
			macroErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("us_yahoo", "error", err.Error())
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("jpy_yahoo", "error", err.Error())
			log.Printf("[handleChannelsIngest] macro ingest failed: %v", err)
			return
		}
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("us_yahoo", "ok", "")
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("jpy_yahoo", "ok", "")
		log.Printf("[handleChannelsIngest] macro ingest succeeded: %d events, recorded_at=%d", len(events), snap.RecordedAt)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		score, err := a.geoProvider.FetchScore(r.Context())
		if err != nil {
			geoErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("geopolitical", "error", err.Error())
			log.Printf("[handleChannelsIngest] geo ingest failed: %v", err)
			return
		}
		store := narrative.NewGeopoliticalStore(filepath.Join(stateDir, "geopolitical"))
		if err := store.Save(score); err != nil {
			geoErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("geopolitical", "error", err.Error())
			log.Printf("[handleChannelsIngest] geo save failed: %v", err)
			return
		}
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("geopolitical", "ok", "")
		log.Printf("[handleChannelsIngest] geo ingest succeeded: intensity=%.2f", score.Intensity)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		capFlowProvider := marketdata.NewTWSECapitalFlowProvider(filepath.Join(stateDir, "capital_flow"))
		_, err := capFlowProvider.FetchSnapshot(r.Context())
		if err != nil {
			capFlowErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("twse_capital_flow", "error", err.Error())
			log.Printf("[handleChannelsIngest] capital flow ingest failed: %v", err)
			return
		}
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("twse_capital_flow", "ok", "")
		log.Printf("[handleChannelsIngest] capital flow ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		exportProvider := marketdata.NewExportStatisticsProvider(filepath.Join(stateDir, "export"))
		_, err := exportProvider.FetchSnapshot(r.Context())
		if err != nil {
			exportErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("export_statistics", "error", err.Error())
			log.Printf("[handleChannelsIngest] export statistics ingest failed: %v", err)
			return
		}
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("export_statistics", "ok", "")
		log.Printf("[handleChannelsIngest] export statistics ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tsmcProvider := marketdata.NewTSMCRevenueProvider(filepath.Join(stateDir, "tsmc_revenue"))
		_, err := tsmcProvider.FetchSnapshot(r.Context())
		if err != nil {
			tsmcErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("tsmc_revenue", "error", err.Error())
			log.Printf("[handleChannelsIngest] TSMC revenue ingest failed: %v", err)
			return
		}
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("tsmc_revenue", "ok", "")
		log.Printf("[handleChannelsIngest] TSMC revenue ingest succeeded")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		twGeoScore, err := a.taiwanGeoProvider.FetchScore(r.Context())
		if err != nil {
			twGeoErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("geopolitical_taiwan", "error", err.Error())
			log.Printf("[handleChannelsIngest] Taiwan geopolitical ingest failed: %v", err)
			return
		}
		twStore := narrative.NewGeopoliticalStore(filepath.Join(stateDir, "geopolitical", "taiwan"))
		if err := twStore.Save(twGeoScore); err != nil {
			twGeoErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("geopolitical_taiwan", "error", err.Error())
			log.Printf("[handleChannelsIngest] Taiwan geopolitical save failed: %v", err)
			return
		}
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("geopolitical_taiwan", "ok", "")
		log.Printf("[handleChannelsIngest] Taiwan geopolitical ingest succeeded: intensity=%.2f", twGeoScore.Intensity)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if a.janusEngine == nil {
			janusErr = fmt.Errorf("JANUS engine not initialized")
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("janus_regime", "error", janusErr.Error())
			log.Printf("[handleChannelsIngest] JANUS regime ingest skipped: engine not initialized")
			return
		}
		a.janusEngine.Update()
		status := a.janusEngine.GetStatus()
		if status.LastUpdated.IsZero() {
			janusErr = fmt.Errorf("JANUS engine has no data after update")
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("janus_regime", "error", janusErr.Error())
			log.Printf("[handleChannelsIngest] JANUS regime ingest failed: %v", janusErr)
			return
		}
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("janus_regime", "ok", "")
		log.Printf("[handleChannelsIngest] JANUS regime ingest succeeded: class=%s", status.Classification)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		tejKey := os.Getenv("TEJ_API_KEY")
		if tejKey == "" {
			tejErr = fmt.Errorf("TEJ_API_KEY not set")
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("tej", "error", tejErr.Error())
			log.Printf("[handleChannelsIngest] TEJ ingest skipped: TEJ_API_KEY not set")
			return
		}
		tejClient := marketdata.NewTEJClient(tejKey)
		if err := tejClient.Ping(r.Context()); err != nil {
			tejErr = err
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("tej", "error", err.Error())
			log.Printf("[handleChannelsIngest] TEJ ingest failed: %v", err)
			return
		}
		NewChannelHealthStoreWithPool(stateDir, a.pool).Record("tej", "ok", "")
		log.Printf("[handleChannelsIngest] TEJ ingest succeeded")
	}()

	wg.Wait()

	result := map[string]any{
		"macro_ok":    macroErr == nil,
		"geo_ok":      geoErr == nil,
		"cap_flow_ok": capFlowErr == nil,
		"export_ok":   exportErr == nil,
		"tsmc_ok":     tsmcErr == nil,
		"tw_geo_ok":   twGeoErr == nil,
		"janus_ok":    janusErr == nil,
		"tej_ok":      tejErr == nil,
	}
	if macroErr != nil {
		result["macro_error"] = macroErr.Error()
	}
	if geoErr != nil {
		result["geo_error"] = geoErr.Error()
	}
	if capFlowErr != nil {
		result["cap_flow_error"] = capFlowErr.Error()
	}
	if exportErr != nil {
		result["export_error"] = exportErr.Error()
	}
	if tsmcErr != nil {
		result["tsmc_error"] = tsmcErr.Error()
	}
	if twGeoErr != nil {
		result["tw_geo_error"] = twGeoErr.Error()
	}
	if janusErr != nil {
		result["janus_error"] = janusErr.Error()
	}
	if tejErr != nil {
		result["tej_error"] = tejErr.Error()
	}

	if macroErr != nil && geoErr != nil && capFlowErr != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("all core ingests failed: macro=%v, geo=%v, cap_flow=%v", macroErr, geoErr, capFlowErr))
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (a *DashboardAPI) handleMacroSnapshotLatest(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(a.macroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "no macro snapshot available")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read snapshot: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (a *DashboardAPI) handleMacroSnapshotHistory(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		writeJSONError(w, http.StatusBadRequest, "date query param required (YYYY-MM-DD)")
		return
	}
	path := filepath.Join(a.macroIngestor.SnapshotDir(), date+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "snapshot not found for date")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read snapshot: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (a *DashboardAPI) handleCapitalFlowLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := filepath.Join(a.macroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "no macro snapshot available")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read snapshot: %v", err))
		return
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("decode snapshot: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"foreign_investor_net": snap.ForeignInvestorNet,
		"domestic_fund_net":    snap.DomesticFundNet,
		"dealer_net":           snap.DealerNet,
		"recorded_at":          snap.RecordedAt,
	})
}

func (a *DashboardAPI) handleTaiwanStressIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// Try to load latest snapshot first; if missing, trigger ingest.
	var snap marketdata.MacroDataSnapshot
	path := filepath.Join(a.macroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			_, snap, err = a.macroIngestor.Ingest(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("ingest failed: %v", err))
				return
			}
		} else {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read snapshot: %v", err))
			return
		}
	} else {
		if err := json.Unmarshal(data, &snap); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("decode snapshot: %v", err))
			return
		}
	}

	geoStore := narrative.NewGeopoliticalStore(filepath.Join(a.workDir, "data/state/geopolitical"))
	index, err := a.taiwanStressCalc.CalculateFromSnapshotWithStore(r.Context(), snap, geoStore)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("calculate stress index: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, index)
}

// ExperimentInboxItem represents a single experiment in the inbox.
type ExperimentInboxItem struct {
	ExperimentID    string                  `json:"experiment_id"`
	TargetAgentID   string                  `json:"target_agent_id"`
	Skill           string                  `json:"skill"`
	MutationType    string                  `json:"mutation_type"`
	MutationSummary string                  `json:"mutation_summary,omitempty"`
	Status          domain.ExperimentStatus `json:"status"`
	BaselineValue   float64                 `json:"baseline_value"`
	CandidateValue  float64                 `json:"candidate_value"`
	CandidatePath   string                  `json:"candidate_path"`
	RejectReason    string                  `json:"reject_reason,omitempty"`
	RecordedAt      time.Time               `json:"recorded_at"`
}

// ExperimentInboxResponse groups experiments by actionable state.
type ExperimentInboxResponse struct {
	PendingJudges   []ExperimentInboxItem `json:"pending_judges"`
	PendingPromotes []ExperimentInboxItem `json:"pending_promotes"`
	RecentHistory   []ExperimentInboxItem `json:"recent_history"`
	BaselineVersion int                   `json:"baseline_version"`
}

func (a *DashboardAPI) handleExperimentInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	policy, err := baseline.Load(a.baselinePath)
	if err != nil {
		policy = baseline.DefaultPolicy()
	}

	experimentsDir := filepath.Join(a.ledgerDir, "experiments")
	// Auto-expire stale planned/running experiments before building the inbox.
	if _, err := experiment.ExpireOldExperiments(experimentsDir, experiment.DefaultExperimentTTL); err != nil {
		log.Printf("[DashboardAPI] warn: experiment TTL cleanup failed: %v", err)
	}

	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, ExperimentInboxResponse{BaselineVersion: policy.Version})
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read experiments dir: %v", err))
		return
	}

	pendingJudges := make([]ExperimentInboxItem, 0)
	pendingPromotes := make([]ExperimentInboxItem, 0)
	recentHistory := make([]ExperimentInboxItem, 0)

	promotedIDs := make(map[string]bool)
	for _, pr := range policy.Promotions {
		promotedIDs[pr.ExperimentID] = true
	}

	// Collect all accepted items first so we can deduplicate by agent.
	var allAccepted []ExperimentInboxItem
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(experimentsDir, entry.Name())
		bytes, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var result domain.PromptExperimentResult
		if err := json.Unmarshal(bytes, &result); err != nil {
			continue
		}

		item := ExperimentInboxItem{
			ExperimentID:    result.Experiment.ID,
			TargetAgentID:   result.Experiment.TargetAgentID,
			Skill:           result.Experiment.Skill,
			MutationType:    result.Experiment.MutationType,
			MutationSummary: buildMutationSummary(policy, result),
			Status:          result.Experiment.Status,
			BaselineValue:   result.Experiment.BaselineValue,
			CandidateValue:  result.Experiment.CandidateValue,
			CandidatePath:   result.CandidatePrompt,
			RejectReason:    result.Experiment.RevertReason,
			RecordedAt:      result.RecordedAt,
		}

		switch result.Experiment.Status {
		case domain.ExperimentRunning, domain.ExperimentPlanned:
			pendingJudges = append(pendingJudges, item)
		case domain.ExperimentAccepted:
			allAccepted = append(allAccepted, item)
		default:
			recentHistory = append(recentHistory, item)
		}
	}

	// For each agent, keep only the latest accepted experiment as pending promote;
	// older accepted experiments and already-promoted ones go to history.
	latestByAgent := make(map[string]ExperimentInboxItem)
	for _, item := range allAccepted {
		existing, ok := latestByAgent[item.TargetAgentID]
		if !ok || item.RecordedAt.After(existing.RecordedAt) {
			latestByAgent[item.TargetAgentID] = item
		}
	}
	for _, item := range allAccepted {
		latest := latestByAgent[item.TargetAgentID]
		if promotedIDs[item.ExperimentID] || item.ExperimentID != latest.ExperimentID {
			recentHistory = append(recentHistory, item)
		} else {
			pendingPromotes = append(pendingPromotes, item)
		}
	}

	slices.SortFunc(pendingJudges, func(a, b ExperimentInboxItem) int {
		if a.RecordedAt.After(b.RecordedAt) {
			return -1
		}
		return 1
	})
	slices.SortFunc(pendingPromotes, func(a, b ExperimentInboxItem) int {
		if a.RecordedAt.After(b.RecordedAt) {
			return -1
		}
		return 1
	})
	slices.SortFunc(recentHistory, func(a, b ExperimentInboxItem) int {
		if a.RecordedAt.After(b.RecordedAt) {
			return -1
		}
		return 1
	})
	if len(recentHistory) > 10 {
		recentHistory = recentHistory[:10]
	}

	writeJSON(w, http.StatusOK, ExperimentInboxResponse{
		PendingJudges:   pendingJudges,
		PendingPromotes: pendingPromotes,
		RecentHistory:   recentHistory,
		BaselineVersion: policy.Version,
	})
}

func (a *DashboardAPI) handleJudgeExperiment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ExperimentID string `json:"experiment_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.ExperimentID == "" {
		writeJSONError(w, http.StatusBadRequest, "experiment_id required")
		return
	}

	resultPath := filepath.Join(a.ledgerDir, "experiments", req.ExperimentID+".json")
	if _, err := os.Stat(resultPath); err != nil {
		writeJSONError(w, http.StatusNotFound, "experiment result not found")
		return
	}

	replayPath := filepath.Join(a.workDir, "data/replay/tw_extended_90days.csv")
	judge := experiment.NewJudge(ledger.NewStore(a.ledgerDir), replayPath, a.baselinePath)
	result, err := judge.Evaluate(resultPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("judge failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"status":     result.Experiment.Status,
		"baseline":   result.Experiment.BaselineValue,
		"candidate":  result.Experiment.CandidateValue,
		"experiment": result.Experiment,
	})
}

func (a *DashboardAPI) handleExperimentDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	experimentID := strings.TrimSpace(r.URL.Query().Get("experiment_id"))
	if experimentID == "" {
		writeJSONError(w, http.StatusBadRequest, "experiment_id required")
		return
	}

	resultPath := filepath.Join(a.ledgerDir, "experiments", experimentID+".json")
	bytes, err := os.ReadFile(resultPath)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "experiment result not found")
		return
	}
	var result domain.PromptExperimentResult
	if err := json.Unmarshal(bytes, &result); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "invalid experiment result")
		return
	}

	// Always use the original agent prompt file as the baseline for diff.
	promptFile := result.Brief.PromptFile
	if !filepath.IsAbs(promptFile) {
		promptFile = filepath.Join(a.workDir, promptFile)
	}
	baselineBytes, err := os.ReadFile(promptFile)
	baselinePrompt := ""
	if err == nil {
		baselinePrompt = string(baselineBytes)
	}

	candidateBytes, err := os.ReadFile(result.CandidatePrompt)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "cannot read candidate prompt")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"baseline_prompt":  baselinePrompt,
		"candidate_prompt": string(candidateBytes),
		"target_agent_id":  result.Experiment.TargetAgentID,
		"skill":            result.Experiment.Skill,
	})
}

func (a *DashboardAPI) handleBacktestRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Start == "" || req.End == "" {
		writeJSONError(w, http.StatusBadRequest, "start and end dates required")
		return
	}
	startDate, err := time.Parse("2006-01-02", req.Start)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid start date format (YYYY-MM-DD)")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.End)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid end date format (YYYY-MM-DD)")
		return
	}

	a.backtestMu.Lock()
	if a.backtestRunning {
		a.backtestMu.Unlock()
		writeJSONError(w, http.StatusConflict, "backtest already running")
		return
	}
	a.backtestRunning = true
	a.backtestStatus = map[string]interface{}{
		"running":    true,
		"started_at": time.Now().UTC(),
		"start":      req.Start,
		"end":        req.End,
	}
	a.backtestMu.Unlock()

	go func() {
		cfg := config.Load()
		if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
			cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
		}
		runner := backtest.NewRunner(cfg)
		summary, err := runner.Run(startDate, endDate)
		if err == nil {
			if rerr := runner.GenerateReport(summary); rerr != nil {
				log.Printf("[DashboardAPI] backtest report generation failed: %v", rerr)
			}
		}

		a.backtestMu.Lock()
		a.backtestRunning = false
		a.backtestStatus["running"] = false
		a.backtestStatus["finished_at"] = time.Now().UTC()
		if err != nil {
			a.backtestStatus["error"] = err.Error()
		} else {
			a.backtestStatus["window_id"] = summary.WindowID
			a.backtestStatus["sessions"] = summary.SessionCount
			a.backtestStatus["outcomes"] = summary.OutcomeCount
			a.backtestStatus["worst_agent"] = summary.WorstAgentID
		}
		a.backtestMu.Unlock()
	}()

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"running":      true,
		"check_status": "/api/backtest/status",
		"start":        req.Start,
		"end":          req.End,
	})
}

func (a *DashboardAPI) handleBacktestStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	a.backtestMu.Lock()
	status := make(map[string]interface{}, len(a.backtestStatus))
	for k, v := range a.backtestStatus {
		status[k] = v
	}
	a.backtestMu.Unlock()
	writeJSON(w, http.StatusOK, status)
}

// SystemHealthResponse exposes config consistency and freshness.
type SystemHealthResponse struct {
	BaselineVersion       string            `json:"baseline_version"`
	ReplayDataLatestDate  string            `json:"replay_data_latest_date"`
	ReplayDataPathOK      bool              `json:"replay_data_path_ok"`
	LastWindowID          string            `json:"last_window_id"`
	LastWindowGeneratedAt time.Time         `json:"last_window_generated_at"`
	Warnings              []string          `json:"warnings"`
	Regime                domain.Regime     `json:"regime"`
	DataChannels          []DataChannelInfo `json:"data_channels,omitempty"`
}

type DataChannelInfo struct {
	ChannelID  string `json:"channel_id"`
	Label      string `json:"label"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	UpdatedAt  string `json:"updated_at"`
}

func (a *DashboardAPI) handleSystemHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	warnings := make([]string, 0)

	policy, err := baseline.Load(a.baselinePath)
	baselineVersion := "未知"
	if err != nil {
		warnings = append(warnings, "基線策略未載入")
	} else {
		baselineVersion = fmt.Sprintf("v%d", policy.Version)
	}

	replayPath := filepath.Join(a.workDir, "data/replay/tw_extended_90days.csv")
	replayOK := true
	latestReplayDate := ""
	ds, err := replay.LoadTWSEOpenDataCSV(replayPath)
	if err != nil {
		replayOK = false
		warnings = append(warnings, "replay 資料無法讀取："+err.Error())
	} else if len(ds.Dates) > 0 {
		latestReplayDate = ds.Dates[len(ds.Dates)-1].Format("2006-01-02")
	}

	lastWindow := ""
	var lastWindowTime time.Time
	windowsDir := filepath.Join(a.ledgerDir, "windows")
	if entries, err := os.ReadDir(windowsDir); err == nil {
		var latest time.Time
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") || strings.Contains(e.Name(), "mutation-brief") {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latest) {
				latest = info.ModTime()
				lastWindow = strings.TrimSuffix(e.Name(), ".json")
				lastWindowTime = info.ModTime()
			}
		}
	}

	if baselineVersion != "未知" && lastWindowTime.IsZero() {
		warnings = append(warnings, "找不到回測窗口摘要")
	}

	// Crowding check from latest session outcomes
	latestSummary, _ := a.loadSessionSummary("")
	if latestSummary != nil {
		store := ledger.NewStore(a.ledgerDir)
		outcomes, _ := store.LoadSessionOutcomes(latestSummary.SessionID)
		symbolAgents := make(map[string]map[string]struct{})
		for _, outcome := range outcomes {
			if symbolAgents[outcome.Symbol] == nil {
				symbolAgents[outcome.Symbol] = make(map[string]struct{})
			}
			symbolAgents[outcome.Symbol][outcome.AgentID] = struct{}{}
		}
		for symbol, agents := range symbolAgents {
			count := len(agents)
			if count >= 4 {
				warnings = append(warnings, fmt.Sprintf("重疊過高：%s（%d 個 AI）", symbol, count))
			} else if count >= 3 {
				warnings = append(warnings, fmt.Sprintf("擁擠交易：%s（%d 個 AI）", symbol, count))
			}
		}
	}
	regime := domain.RegimeNeutral
	if summary, err := a.loadSessionSummary(""); err == nil && summary != nil {
		regime = summary.Regime
	}

	now := time.Now()
	channels := []DataChannelInfo{
		a.buildChannelInfo("us_yahoo", "Yahoo Finance Macro", a.checkMacroHealth, filepath.Join(a.workDir, "data/state/macro/latest.json"), now),
		a.buildChannelInfo("twse_capital_flow", "TWSE 三大法人", a.checkCapitalFlowHealth, filepath.Join(a.workDir, "data/state/capital_flow"), now),
		a.buildChannelInfo("geopolitical", "地緣政治風險", a.checkGeopoliticalHealth, filepath.Join(a.workDir, "data/state/geopolitical/latest.json"), now),
		a.buildChannelInfo("twse_replay", "TWSE Replay", a.checkReplayHealth, filepath.Join(a.workDir, "data/replay/tw_extended_90days.csv"), now),
	}

	writeJSON(w, http.StatusOK, SystemHealthResponse{
		BaselineVersion:       baselineVersion,
		ReplayDataLatestDate:  latestReplayDate,
		ReplayDataPathOK:      replayOK,
		LastWindowID:          lastWindow,
		LastWindowGeneratedAt: lastWindowTime,
		Warnings:              warnings,
		Regime:                regime,
		DataChannels:          channels,
	})
}

func (a *DashboardAPI) buildChannelInfo(id, label string, checker func(string, time.Time) (string, string), path string, now time.Time) DataChannelInfo {
	status, updated := checker(path, now)
	return DataChannelInfo{
		ChannelID:  id,
		Label:      label,
		Status:     status,
		StatusText: statusText(status),
		UpdatedAt:  updated,
	}
}

// PipelineItem represents a single recommendation through the guard pipeline.
type PipelineItem struct {
	Symbol              string                      `json:"symbol"`
	AgentID             string                      `json:"agent_id"`
	Skill               string                      `json:"skill"`
	Layer               string                      `json:"layer"`
	Side                string                      `json:"side"`
	Conviction          int                         `json:"conviction"`
	TargetPrice         float64                     `json:"target_price"`
	StopLossPrice       float64                     `json:"stop_loss_price"`
	ForwardReturn       float64                     `json:"forward_return"`
	Hit                 bool                        `json:"hit"`
	Reason              string                      `json:"reason"`
	Price               float64                     `json:"price"`
	PassedGuards        bool                        `json:"passed_guards"`
	GuardReason         string                      `json:"guard_reason"`
	Tags                []string                    `json:"tags"`
	RecordedAt          time.Time                   `json:"recorded_at"`
	FactorScores        domain.FactorScores         `json:"factor_scores,omitempty"`
	ConvictionBreakdown *domain.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
}

// RecommendationPipelineResponse returns the latest session pipeline.
type RecommendationPipelineResponse struct {
	SessionID     string                   `json:"session_id"`
	Regime        domain.Regime            `json:"regime"`
	Items         []PipelineItem           `json:"items"`
	GuardOutcomes []domain.GuardOutcome    `json:"guard_outcomes"`
	ScreenedItems []domain.ScreeningReject `json:"screened_items"`
	RecordedAt    time.Time                `json:"recorded_at"`
}

func computePipelineTags(ds *replay.Dataset, symbol string, date time.Time) []string {
	if ds == nil {
		return nil
	}
	dateKey := date.Format("2006-01-02")
	bar, ok := ds.ByDate[dateKey][symbol]
	if !ok {
		return nil
	}
	var prevBar domain.DailyBar
	var hasPrev bool
	for i, d := range ds.Dates {
		if d.Format("2006-01-02") == dateKey && i > 0 {
			prevBar = ds.ByDate[ds.Dates[i-1].Format("2006-01-02")][symbol]
			hasPrev = prevBar.Close > 0
			break
		}
	}

	tags := make([]string, 0, 3)
	changePct := 0.0
	if bar.Open > 0 {
		changePct = (bar.Close - bar.Open) / bar.Open
	}
	if changePct > 0.035 {
		tags = append(tags, "長紅")
	} else if changePct < -0.035 {
		tags = append(tags, "長黑")
	}
	if hasPrev && prevBar.Volume > 0 && bar.Volume > int64(float64(prevBar.Volume)*1.5) {
		tags = append(tags, "放量")
	}

	high5 := bar.Close
	low5 := bar.Close
	for i, d := range ds.Dates {
		if d.Format("2006-01-02") == dateKey {
			start := i - 4
			if start < 0 {
				start = 0
			}
			for _, pd := range ds.Dates[start : i+1] {
				b := ds.ByDate[pd.Format("2006-01-02")][symbol]
				if b.Close > high5 {
					high5 = b.Close
				}
				if b.Close > 0 && (low5 == 0 || b.Close < low5) {
					low5 = b.Close
				}
			}
			break
		}
	}
	if bar.Close > 0 && bar.Close == high5 {
		tags = append(tags, "創5日高")
	}
	if bar.Close > 0 && low5 > 0 && bar.Close == low5 {
		tags = append(tags, "創5日低")
	}
	return tags
}

func (a *DashboardAPI) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSON(w, http.StatusOK, map[string]any{"sessions": []any{}})
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read sessions dir: %v", err))
		return
	}

	type sessionMeta struct {
		SessionID    string    `json:"session_id"`
		RecordedAt   time.Time `json:"recorded_at"`
		Regime       string    `json:"regime"`
		OutcomeCount int       `json:"outcome_count"`
	}

	sessions := make([]sessionMeta, 0, len(entries))
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
		sessions = append(sessions, sessionMeta{
			SessionID:    summary.SessionID,
			RecordedAt:   summary.RecordedAt,
			Regime:       string(summary.Regime),
			OutcomeCount: summary.OutcomeCount,
		})
	}

	// Sort by session trading date descending, then RecordedAt tiebreaker.
	slices.SortFunc(sessions, func(a, b sessionMeta) int {
		aDate := sessionDateFromID(a.SessionID)
		bDate := sessionDateFromID(b.SessionID)
		switch {
		case aDate.After(bDate):
			return -1
		case aDate.Before(bDate):
			return 1
		case a.RecordedAt.After(b.RecordedAt):
			return -1
		case a.RecordedAt.Before(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})

	writeJSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

func (a *DashboardAPI) handleRecommendationPipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	showAll := r.URL.Query().Get("show_all") == "true"
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))

	summary, err := a.loadSessionSummary(sessionID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load recommendation pipeline summary: %v", err))
		return
	}
	if summary == nil {
		writeJSON(w, http.StatusOK, RecommendationPipelineResponse{})
		return
	}

	var ds *replay.Dataset
	cfg := config.Load()
	replayPath := cfg.ReplayDataPath
	if replayPath == "samples/replay/twse_stock_day_all_sample.csv" {
		replayPath = "data/replay/tw_extended_90days.csv"
	}
	if replayPath != "" {
		if !filepath.IsAbs(replayPath) {
			replayPath = filepath.Join(a.workDir, replayPath)
		}
		if dsTmp, err := replay.LoadTWSEOpenDataCSV(replayPath); err == nil {
			ds = dsTmp
		}
	}

	sessionsDir := filepath.Join(a.ledgerDir, "sessions")
	outcomesPath := filepath.Join(sessionsDir, summary.SessionID, "recommendation_outcomes.jsonl")
	items := make([]PipelineItem, 0)
	if data, err := os.ReadFile(outcomesPath); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var outcome struct {
				AgentID             string                      `json:"AgentID"`
				Skill               string                      `json:"Skill"`
				Layer               string                      `json:"Layer"`
				Symbol              string                      `json:"Symbol"`
				Side                string                      `json:"Side"`
				Conviction          int                         `json:"Conviction"`
				TargetPrice         float64                     `json:"TargetPrice"`
				StopLossPrice       float64                     `json:"StopLossPrice"`
				ForwardReturn       float64                     `json:"ForwardReturn"`
				Hit                 bool                        `json:"Hit"`
				Reason              string                      `json:"Reason"`
				Price               float64                     `json:"Price"`
				PassedGuards        bool                        `json:"PassedGuards"`
				GuardReason         string                      `json:"GuardReason"`
				RecordedAt          time.Time                   `json:"RecordedAt"`
				FactorScores        domain.FactorScores         `json:"factor_scores,omitempty"`
				ConvictionBreakdown *domain.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
			}
			if err := json.Unmarshal([]byte(line), &outcome); err != nil {
				continue
			}
			fr := outcome.ForwardReturn
			price := outcome.Price
			side := outcome.Side
			passedGuards := outcome.PassedGuards
			// Legacy sessions (generated before PassedGuards field existed)
			// should default to true to preserve backward-compatible display.
			if !passedGuards && !strings.Contains(line, `"PassedGuards"`) {
				passedGuards = true
			}
			if ds != nil && !outcome.RecordedAt.IsZero() {
				if fr == 0 {
					if recalculated, ok := ds.ForwardReturn(outcome.Symbol, outcome.RecordedAt, 1); ok {
						fr = recalculated
					}
				}
				if price == 0 {
					if bar, ok := ds.ByDate[outcome.RecordedAt.Format("2006-01-02")][outcome.Symbol]; ok {
						price = bar.Close
					}
				}
			}
			if side == "" {
				side = string(domain.SideBuy)
			}
			tp := outcome.TargetPrice
			slp := outcome.StopLossPrice
			if tp == 0 && slp == 0 && price > 0 {
				tp, slp = fallbackPriceTargets(outcome.Skill, price)
			}
			if !showAll && !passedGuards {
				continue
			}
			tags := computePipelineTags(ds, outcome.Symbol, outcome.RecordedAt)
			items = append(items, PipelineItem{
				Symbol:              outcome.Symbol,
				AgentID:             outcome.AgentID,
				Skill:               outcome.Skill,
				Layer:               outcome.Layer,
				Side:                side,
				Conviction:          outcome.Conviction,
				TargetPrice:         tp,
				StopLossPrice:       slp,
				ForwardReturn:       fr,
				Hit:                 fr > 0,
				Reason:              outcome.Reason,
				Price:               price,
				PassedGuards:        passedGuards,
				GuardReason:         outcome.GuardReason,
				Tags:                tags,
				RecordedAt:          outcome.RecordedAt,
				FactorScores:        outcome.FactorScores,
				ConvictionBreakdown: outcome.ConvictionBreakdown,
			})
		}
	}

	guards := make([]domain.GuardOutcome, 0, len(summary.GuardOutcomes))
	for _, g := range summary.GuardOutcomes {
		guards = append(guards, domain.GuardOutcome{
			GuardID:     g.GuardID,
			GuardSkill:  g.GuardSkill,
			Severity:    g.Severity,
			Passed:      g.Passed,
			Reason:      g.Reason,
			InputCount:  g.InputCount,
			OutputCount: g.OutputCount,
		})
	}

	store := ledger.NewStore(a.ledgerDir)
	screened, err := store.LoadSessionScreeningRejects(summary.SessionID)
	if err != nil {
		log.Printf("LoadSessionScreeningRejects %s: %v", summary.SessionID, err)
	}

	writeJSON(w, http.StatusOK, RecommendationPipelineResponse{
		SessionID:     summary.SessionID,
		Regime:        summary.Regime,
		Items:         items,
		GuardOutcomes: guards,
		ScreenedItems: screened,
		RecordedAt:    summary.RecordedAt,
	})
}

// fallbackPriceTargets returns reasonable target/stop-loss multipliers for legacy
// sessions that did not persist TargetPrice/StopLossPrice in outcomes.
// Multipliers are aligned with the orchestrator executor definitions.
func fallbackPriceTargets(skill string, price float64) (float64, float64) {
	var targetMult, stopLossMult float64
	switch skill {
	case "semiconductor_desk":
		targetMult, stopLossMult = 1.06, 0.95
	case "ai_supply_chain_desk":
		targetMult, stopLossMult = 1.08, 0.95
	case "etf_rotation_desk":
		targetMult, stopLossMult = 1.04, 0.97
	case "financials_desk":
		targetMult, stopLossMult = 1.05, 0.96
	case "shipping_desk":
		targetMult, stopLossMult = 1.07, 0.94
	case "growth_momentum":
		targetMult, stopLossMult = 1.08, 0.95
	case "value_yield":
		targetMult, stopLossMult = 1.05, 0.96
	case "earnings_quality":
		targetMult, stopLossMult = 1.06, 0.95
	case "technical_breakout":
		targetMult, stopLossMult = 1.10, 0.94
	case "alpha_discovery":
		targetMult, stopLossMult = 1.06, 0.95
	default:
		targetMult, stopLossMult = 1.05, 0.95
	}
	return price * targetMult, price * stopLossMult
}

// DataChannel represents a single data source configuration and health status.
type DataChannel struct {
	Country    string `json:"country"`
	Platform   string `json:"platform"`
	APIFormat  string `json:"api_format"`
	Path       string `json:"path"`
	Storage    string `json:"storage"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	UpdatedAt  string `json:"updated_at"`
	LastError  string `json:"last_error,omitempty"`
	ChannelID  string `json:"channel_id"`
}

func (a *DashboardAPI) handleDataChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	channels := make([]DataChannel, 0)
	healthStore := NewChannelHealthStoreWithPool(filepath.Join(a.workDir, "data/state"), a.pool)

	// 1. Yahoo Finance Macro (US + Global)
	macroPath := filepath.Join(a.workDir, "data/state/macro/latest.json")
	macroStatus, macroUpdated := a.checkMacroHealth(macroPath, now)
	macroRec := healthStore.Get("us_yahoo")
	if macroRec != nil && macroRec.Status != "" {
		macroStatus = macroRec.Status
		if macroRec.LastError != "" {
			macroUpdated = "上次失敗: " + macroRec.LastError
		} else {
			macroUpdated = "上次抓取: " + macroRec.LastFetchAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "us_yahoo",
		Country:    "美國",
		Platform:   "Yahoo Finance",
		APIFormat:  "REST JSON",
		Path:       "query1.finance.yahoo.com/v8/finance/chart",
		Storage:    "data/state/macro/latest.json",
		Status:     macroStatus,
		StatusText: statusText(macroStatus),
		UpdatedAt:  macroUpdated,
		LastError: func() string {
			if macroRec != nil {
				return macroRec.LastError
			}
			return ""
		}(),
	})

	// 2. TWSE OpenAPI / T86 - Replay data
	replayPath := filepath.Join(a.workDir, "data/replay/tw_extended_90days.csv")
	replayStatus, replayUpdated := a.checkReplayHealth(replayPath, now)
	replayRec := healthStore.Get("twse_replay")
	if replayRec != nil && replayRec.Status != "" {
		replayStatus = replayRec.Status
		if replayRec.LastError != "" {
			replayUpdated = "上次失敗: " + replayRec.LastError
		} else if replayRec.LastSuccessAt != "" {
			replayUpdated = "上次成功: " + replayRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "twse_replay",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "OpenAPI / CSV",
		Path:       "openapi.twse.com.tw / www.twse.com.tw",
		Storage:    "data/replay/tw_extended_90days.csv",
		Status:     replayStatus,
		StatusText: statusText(replayStatus),
		UpdatedAt:  replayUpdated,
		LastError: func() string {
			if replayRec != nil {
				return replayRec.LastError
			}
			return ""
		}(),
	})

	// 3. TWSE Capital Flow
	capFlowDir := filepath.Join(a.workDir, "data/state/capital_flow")
	capStatus, capUpdated := a.checkCapitalFlowHealth(capFlowDir, now)
	channels = append(channels, DataChannel{
		ChannelID:  "twse_capital_flow",
		Country:    "台灣",
		Platform:   "TWSE 三大法人",
		APIFormat:  "T86 JSON",
		Path:       "www.twse.com.tw/rwd/zh/fund/T86",
		Storage:    "data/state/capital_flow/*.json",
		Status:     capStatus,
		StatusText: statusText(capStatus),
		UpdatedAt:  capUpdated,
	})

	// 4. Fugle (optional/live)
	fugleKey := os.Getenv("FUGLE_API_KEY")
	fugleStatus := "inactive"
	fugleUpdated := "-"
	fugleLastError := ""
	if fugleKey != "" {
		fugleClient := marketdata.NewFugleClient(fugleKey)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err := fugleClient.GetQuote(ctx, "0050")
		cancel()
		if err != nil {
			fugleStatus = "error"
			fugleUpdated = "API 連線失敗"
			fugleLastError = err.Error()
		} else {
			fugleStatus = "ok"
			fugleUpdated = "API 連線正常"
		}
	} else {
		fugleUpdated = "未設定 API Key"
	}
	channels = append(channels, DataChannel{
		ChannelID:  "fugle",
		Country:    "台灣",
		Platform:   "Fugle 富果",
		APIFormat:  "REST JSON",
		Path:       "api.fugle.tw",
		Storage:    "(live cache / memory)",
		Status:     fugleStatus,
		StatusText: statusText(fugleStatus),
		UpdatedAt:  fugleUpdated,
		LastError:  fugleLastError,
	})

	// 5. JPY via Yahoo (Japan indicator, same endpoint as US)
	jpyStatus, jpyUpdated := a.checkJPYHealth(macroPath, now)
	jpyRec := healthStore.Get("jpy_yahoo")
	if jpyRec != nil && jpyRec.Status != "" {
		jpyStatus = jpyRec.Status
		if jpyRec.LastError != "" {
			jpyUpdated = "上次失敗: " + jpyRec.LastError
		} else {
			jpyUpdated = "上次抓取: " + jpyRec.LastFetchAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "jpy_yahoo",
		Country:    "日本",
		Platform:   "Yahoo Finance (JPY)",
		APIFormat:  "REST JSON",
		Path:       "query1.finance.yahoo.com/v8/finance/chart",
		Storage:    "data/state/macro/latest.json",
		Status:     jpyStatus,
		StatusText: statusText(jpyStatus),
		UpdatedAt:  jpyUpdated,
		LastError: func() string {
			if jpyRec != nil {
				return jpyRec.LastError
			}
			return ""
		}(),
	})

	// 6. Geopolitical Risk (RSS + GDELT)
	geoPath := filepath.Join(a.workDir, "data/state/geopolitical/latest.json")
	geoStatus, geoUpdated := a.checkGeopoliticalHealth(geoPath, now)
	geoRec := healthStore.Get("geopolitical")
	if geoRec != nil && geoRec.Status != "" {
		geoStatus = geoRec.Status
		if geoRec.LastError != "" {
			geoUpdated = "上次失敗: " + geoRec.LastError
		} else {
			geoUpdated = "上次抓取: " + geoRec.LastFetchAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "geopolitical",
		Country:    "中東/全球",
		Platform:   "RSS + GDELT",
		APIFormat:  "RSS / REST JSON",
		Path:       "feeds.bbci.co.uk / api.gdeltproject.org",
		Storage:    "data/state/geopolitical/latest.json",
		Status:     geoStatus,
		StatusText: statusText(geoStatus),
		UpdatedAt:  geoUpdated,
		LastError: func() string {
			if geoRec != nil {
				return geoRec.LastError
			}
			return ""
		}(),
	})

	// 7. TWSE Margin (Retail Leverage — reverse indicator)
	marginDir := filepath.Join(a.workDir, "data/state/margin")
	marginStatus, marginUpdated := a.checkCapitalFlowHealth(marginDir, now)
	marginRec := healthStore.Get("twse_margin")
	if marginRec != nil && marginRec.Status != "" {
		marginStatus = marginRec.Status
		if marginRec.LastError != "" {
			marginUpdated = "上次失敗: " + marginRec.LastError
		} else if marginRec.LastSuccessAt != "" {
			marginUpdated = "上次成功: " + marginRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "twse_margin",
		Country:    "台灣",
		Platform:   "TWSE 融資融券",
		APIFormat:  "Miantane JSON",
		Path:       "www.twse.com.tw/rwd/zh/marginTradingMiantane",
		Storage:    "data/state/margin/*_margin.json",
		Status:     marginStatus,
		StatusText: statusText(marginStatus),
		UpdatedAt:  marginUpdated,
		LastError: func() string {
			if marginRec != nil {
				return marginRec.LastError
			}
			return ""
		}(),
	})

	// 8. Export Statistics (Electronics export proxy for tech sector health)
	exportDir := filepath.Join(a.workDir, "data/state/export")
	exportStatus, exportUpdated := a.checkCapitalFlowHealth(exportDir, now)
	exportRec := healthStore.Get("export_statistics")
	if exportRec != nil && exportRec.Status != "" {
		exportStatus = exportRec.Status
		if exportRec.LastError != "" {
			exportUpdated = "上次失敗: " + exportRec.LastError
		} else if exportRec.LastSuccessAt != "" {
			exportUpdated = "上次成功: " + exportRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "export_statistics",
		Country:    "台灣",
		Platform:   "TWSE 出口統計",
		APIFormat:  "FAS210 JSON",
		Path:       "www.twse.com.tw/rwd/zh/exchangeReport/FAS210",
		Storage:    "data/state/export/*_export.json",
		Status:     exportStatus,
		StatusText: statusText(exportStatus),
		UpdatedAt:  exportUpdated,
		LastError: func() string {
			if exportRec != nil {
				return exportRec.LastError
			}
			return ""
		}(),
	})

	// 9. TSMC Revenue (AI capex sentiment proxy)
	tsmcDir := filepath.Join(a.workDir, "data/state/tsmc_revenue")
	tsmcStatus, tsmcUpdated := a.checkCapitalFlowHealth(tsmcDir, now)
	tsmcRec := healthStore.Get("tsmc_revenue")
	if tsmcRec != nil && tsmcRec.Status != "" {
		tsmcStatus = tsmcRec.Status
		if tsmcRec.LastError != "" {
			tsmcUpdated = "上次失敗: " + tsmcRec.LastError
		} else if tsmcRec.LastSuccessAt != "" {
			tsmcUpdated = "上次成功: " + tsmcRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "tsmc_revenue",
		Country:    "台灣",
		Platform:   "TWSE 台積電月營收",
		APIFormat:  "TWT49U JSON",
		Path:       "www.twse.com.tw/rwd/zh/fund/TWT49U",
		Storage:    "data/state/tsmc_revenue/*_revenue.json",
		Status:     tsmcStatus,
		StatusText: statusText(tsmcStatus),
		UpdatedAt:  tsmcUpdated,
		LastError: func() string {
			if tsmcRec != nil {
				return tsmcRec.LastError
			}
			return ""
		}(),
	})

	// 10. Taiwan Geopolitical Risk (RSS + Cross-Strait monitoring)
	twGeoDir := filepath.Join(a.workDir, "data/state/geopolitical/taiwan")
	twGeoStatus, twGeoUpdated := a.checkCapitalFlowHealth(twGeoDir, now)
	twGeoRec := healthStore.Get("geopolitical_taiwan")
	if twGeoRec != nil && twGeoRec.Status != "" {
		twGeoStatus = twGeoRec.Status
		if twGeoRec.LastError != "" {
			twGeoUpdated = "上次失敗: " + twGeoRec.LastError
		} else if twGeoRec.LastSuccessAt != "" {
			twGeoUpdated = "上次成功: " + twGeoRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "geopolitical_taiwan",
		Country:    "台灣",
		Platform:   "CNA / 自由時報 / TVBS RSS",
		APIFormat:  "RSS XML",
		Path:       "www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw",
		Storage:    "data/state/geopolitical/taiwan/latest.json",
		Status:     twGeoStatus,
		StatusText: statusText(twGeoStatus),
		UpdatedAt:  twGeoUpdated,
		LastError: func() string {
			if twGeoRec != nil {
				return twGeoRec.LastError
			}
			return ""
		}(),
	})

	// 11. JANUS Regime (Meta-layer regime detection)
	janusStatus, janusUpdated := a.checkJanusHealth(now)
	janusRec := healthStore.Get("janus_regime")
	if janusRec != nil && janusRec.Status != "" {
		janusStatus = janusRec.Status
		if janusRec.LastError != "" {
			janusUpdated = "上次失敗: " + janusRec.LastError
		} else if janusRec.LastSuccessAt != "" {
			janusUpdated = "上次成功: " + janusRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "janus_regime",
		Country:    "全域",
		Platform:   "JANUS Engine",
		APIFormat:  "Internal",
		Path:       "internal/janus",
		Storage:    "(in-memory state)",
		Status:     janusStatus,
		StatusText: statusText(janusStatus),
		UpdatedAt:  janusUpdated,
		LastError: func() string {
			if janusRec != nil {
				return janusRec.LastError
			}
			return ""
		}(),
	})

	// 12. TEJ (Taiwan Economic Journal - premium financial data)
	tejStatus := "unknown"
	tejUpdated := ""
	tejRec := healthStore.Get("tej")
	if tejRec != nil && tejRec.Status != "" {
		tejStatus = tejRec.Status
		if tejRec.LastError != "" {
			tejUpdated = "上次失敗: " + tejRec.LastError
		} else if tejRec.LastSuccessAt != "" {
			tejUpdated = "上次成功: " + tejRec.LastSuccessAt
		}
	}
	channels = append(channels, DataChannel{
		ChannelID:  "tej",
		Country:    "台灣",
		Platform:   "TEJ 台灣經濟新報",
		APIFormat:  "REST JSON",
		Path:       "TEJ API (premium)",
		Storage:    "N/A (live query)",
		Status:     tejStatus,
		StatusText: statusText(tejStatus),
		UpdatedAt:  tejUpdated,
		LastError: func() string {
			if tejRec != nil {
				return tejRec.LastError
			}
			return ""
		}(),
	})

	// Build alerts list for overview card
	alerts := healthStore.Alerts()
	// Also add age-based alerts if health store doesn't cover them
	for _, c := range channels {
		if c.Status == "error" {
			found := false
			for _, a := range alerts {
				if a.ChannelID == c.ChannelID {
					found = true
					break
				}
			}
			if !found {
				alerts = append(alerts, ChannelAlert{
					ChannelID: c.ChannelID,
					Status:    c.Status,
					Error:     c.LastError,
				})
			}
		}
	}
	if alerts == nil {
		alerts = []ChannelAlert{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels":  channels,
		"alerts":    alerts,
		"generated": now.Format("2006-01-02 15:04:05"),
	})
}

func (a *DashboardAPI) checkMacroHealth(path string, now time.Time) (string, string) {
	info, err := os.Stat(path)
	if err != nil {
		return "error", "檔案不存在"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "error", "無法讀取"
	}
	var snap struct {
		RecordedAt int64 `json:"recorded_at"`
		DXY        struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"dxy"`
		Oil struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"oil"`
	}
	_ = json.Unmarshal(data, &snap)

	latest := info.ModTime()
	if snap.RecordedAt > 0 {
		latest = time.Unix(snap.RecordedAt, 0)
	}
	if snap.DXY.Timestamp > 0 {
		dxyTime := time.Unix(snap.DXY.Timestamp, 0)
		if dxyTime.After(latest) {
			latest = dxyTime
		}
	}
	if snap.Oil.Timestamp > 0 {
		oilTime := time.Unix(snap.Oil.Timestamp, 0)
		if oilTime.After(latest) {
			latest = oilTime
		}
	}

	age := now.Sub(latest)
	if age < 24*time.Hour {
		return "ok", latest.Format("2006-01-02 15:04:05")
	}
	if age < 7*24*time.Hour {
		return "warn", latest.Format("2006-01-02 15:04:05")
	}
	return "error", latest.Format("2006-01-02 15:04:05")
}

func (a *DashboardAPI) checkJPYHealth(path string, now time.Time) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "error", "檔案不存在"
	}
	var snap struct {
		JPY struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"jpy"`
	}
	_ = json.Unmarshal(data, &snap)
	if snap.JPY.Timestamp == 0 {
		return "error", "無 JPY 資料"
	}
	t := time.Unix(snap.JPY.Timestamp, 0)
	age := now.Sub(t)
	if age < 24*time.Hour {
		return "ok", t.Format("2006-01-02 15:04:05")
	}
	if age < 7*24*time.Hour {
		return "warn", t.Format("2006-01-02 15:04:05")
	}
	return "error", t.Format("2006-01-02 15:04:05")
}

func (a *DashboardAPI) checkGeopoliticalHealth(path string, now time.Time) (string, string) {
	info, err := os.Stat(path)
	if err != nil {
		return "error", "檔案不存在"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "error", "無法讀取"
	}
	var score struct {
		Timestamp time.Time `json:"timestamp"`
	}
	_ = json.Unmarshal(data, &score)
	var latest time.Time
	if !score.Timestamp.IsZero() {
		latest = score.Timestamp
	} else {
		latest = info.ModTime()
	}
	age := now.Sub(latest)
	if age < 24*time.Hour {
		return "ok", latest.Format("2006-01-02 15:04:05")
	}
	if age < 7*24*time.Hour {
		return "warn", latest.Format("2006-01-02 15:04:05")
	}
	return "error", latest.Format("2006-01-02 15:04:05")
}

func (a *DashboardAPI) checkJanusHealth(now time.Time) (string, string) {
	if a.janusEngine == nil {
		return "inactive", "JANUS engine 未啟用"
	}
	status := a.janusEngine.GetStatus()
	if status.LastUpdated.IsZero() {
		return "warn", "JANUS 已載入但尚未更新"
	}
	age := now.Sub(status.LastUpdated)
	if age < 7*24*time.Hour {
		return "ok", status.LastUpdated.Format("2006-01-02 15:04:05")
	}
	if age < 30*24*time.Hour {
		return "warn", status.LastUpdated.Format("2006-01-02 15:04:05")
	}
	return "error", status.LastUpdated.Format("2006-01-02 15:04:05")
}

func (a *DashboardAPI) checkReplayHealth(path string, now time.Time) (string, string) {
	f, err := os.Open(path)
	if err != nil {
		return "error", "檔案不存在"
	}
	defer f.Close()

	// Read all lines to find the last two trading dates and compute zero-change ratio
	var lastLine string
	scanner := bufio.NewScanner(f)
	lines := make([]string, 0)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		lastLine = line
	}
	if lastLine == "" {
		return "error", "空檔案"
	}
	parts := strings.Split(lastLine, ",")
	if len(parts) == 0 {
		return "error", "格式錯誤"
	}
	latestDate := strings.TrimSpace(parts[0])
	t, err := time.Parse("2006-01-02", latestDate)
	if err != nil {
		return "error", "日期解析失敗"
	}

	// Check zero-change ratio for last two dates
	if len(lines) > 1 {
		prevCloseByCode := make(map[string]float64)
		lastCloseByCode := make(map[string]float64)
		var prevDate string
		for i := len(lines) - 1; i >= 0; i-- {
			row := strings.Split(lines[i], ",")
			if len(row) < 9 || row[0] == "Date" {
				continue
			}
			date := row[0]
			if date != latestDate && prevDate == "" {
				prevDate = date
			}
			if date == latestDate && len(row) >= 9 {
				closeVal, _ := strconv.ParseFloat(strings.TrimSpace(row[8]), 64)
				lastCloseByCode[row[1]] = closeVal
			}
			if date == prevDate && len(row) >= 9 {
				closeVal, _ := strconv.ParseFloat(strings.TrimSpace(row[8]), 64)
				prevCloseByCode[row[1]] = closeVal
			}
		}
		zeroChange := 0
		compared := 0
		for code, lastClose := range lastCloseByCode {
			if prevClose, ok := prevCloseByCode[code]; ok && prevClose > 0 {
				compared++
				if lastClose == prevClose {
					zeroChange++
				}
			}
		}
		if compared > 0 {
			ratio := float64(zeroChange) / float64(compared)
			if ratio > 0.3 {
				return "warn", fmt.Sprintf("%s (%.0f%% 標的隔日收盤價無變動，請檢查 backfill 資料)", latestDate, ratio*100)
			}
		}
	}

	age := now.Sub(t)
	if age < 3*24*time.Hour {
		return "ok", latestDate
	}
	if age < 14*24*time.Hour {
		return "warn", latestDate
	}
	return "error", latestDate
}

func (a *DashboardAPI) checkCapitalFlowHealth(dir string, now time.Time) (string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return "error", "無資料"
	}
	var latestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}
	if latestFile == "" {
		return "error", "無有效檔案"
	}
	dateStr := strings.TrimSuffix(latestFile, ".json")

	var dataTs time.Time
	data, err := os.ReadFile(filepath.Join(dir, latestFile))
	if err == nil {
		var flow struct {
			Date string `json:"date"`
		}
		if json.Unmarshal(data, &flow) == nil && flow.Date != "" {
			if parsed, err := time.ParseInLocation("20060102", flow.Date, time.FixedZone("CST", 8*60*60)); err == nil {
				dataTs = parsed
			}
		}
	}

	var t time.Time
	if !dataTs.IsZero() {
		t = dataTs
	} else {
		parsed, err := time.Parse("20060102", dateStr)
		if err != nil {
			return "error", "日期解析失敗"
		}
		t = parsed
	}

	age := now.Sub(t)
	if age < 24*time.Hour {
		return "ok", dateStr
	}
	if age < 7*24*time.Hour {
		return "warn", dateStr
	}
	return "error", dateStr
}

func statusText(status string) string {
	switch status {
	case "ok":
		return "正常"
	case "warn":
		return "延遲"
	case "error":
		return "異常"
	case "inactive":
		return "未啟用"
	default:
		return "未知"
	}
}

func (a *DashboardAPI) handleRetailSentiment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	provider := marketdata.NewTWSERetailSentimentProvider(a.workDir)
	snap, err := provider.FetchSnapshot(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("fetch retail sentiment: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"margin_balance":    snap.MarginBalance,
		"margin_change_pct": snap.MarginChangePct,
		"day_trading_ratio": snap.DayTradingRatio,
		"margin_percentile": snap.MarginPercentile,
		"sentiment_score":   snap.CalculateSentimentScore(),
		"extreme_reading":   snap.ExtremeReading(),
		"timestamp":         snap.Timestamp,
	})
}

func (a *DashboardAPI) handleCapitalPhase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	config := domain.DefaultCapitalPhaseConfig()
	snap := domain.CapitalSnapshot{
		Phase:           config.CurrentPhase,
		PhaseStartDate:  config.PhaseStartDate,
		DaysInPhase:     int(time.Since(config.PhaseStartDate).Hours() / 24),
		TotalCapital:    1000000,
		DeployedCapital: 350000,
		ReserveCash:     650000,
		RollingSharpe:   1.25,
		MaxDrawdown:     0.05,
		CanAdvance:      true,
		AdvanceReason:   "all criteria met",
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"phase":            snap.Phase,
		"phase_start_date": snap.PhaseStartDate,
		"days_in_phase":    snap.DaysInPhase,
		"rolling_sharpe":   snap.RollingSharpe,
		"max_drawdown":     snap.MaxDrawdown,
		"can_advance":      snap.CanAdvance,
		"advance_reason":   snap.AdvanceReason,
		"capital_limit":    config.CapitalLimits[string(snap.Phase)],
		"total_capital":    snap.TotalCapital,
		"deployed_capital": snap.DeployedCapital,
		"reserve_cash":     snap.ReserveCash,
		"is_simulated":     true,
		"note":             "此為示範資料，非真實交易記錄。實際資金階段需透過 live trading 或模擬執行後計算。",
	})
}

func (a *DashboardAPI) handleTaxSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	snapshots := []domain.TaxSnapshot{
		{
			Symbol:             "2330.TW",
			DividendTaxRate:    0.28,
			TransactionTaxRate: 0.003,
			DividendTax:        0,
			TransactionTax:     150,
			TotalTax:           150,
			AfterTaxPnL:        4850,
		},
	}

	var beforeTaxPnL, afterTaxPnL, totalTax float64
	for _, s := range snapshots {
		beforeTaxPnL += s.AfterTaxPnL + s.TotalTax
		afterTaxPnL += s.AfterTaxPnL
		totalTax += s.TotalTax
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"snapshots":      snapshots,
		"before_tax_pnl": beforeTaxPnL,
		"after_tax_pnl":  afterTaxPnL,
		"total_tax_paid": totalTax,
		"is_simulated":   true,
		"note":           "此為示範資料（僅 2330.TW 單一筆），非真實交易記錄。實際稅務資料需從 ledger outcomes 計算。",
	})
}

func (a *DashboardAPI) handleSeasonalAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	month := now.Month()
	day := now.Day()

	var expectations []map[string]any
	var activeEvents []map[string]any

	if (month == 1 && day >= 15) || (month == 2 && day <= 15) {
		expectations = append(expectations, map[string]any{
			"theme":                 "spring_festival_season",
			"historical_avg_return": 0.05,
			"current_return":        0.02,
			"expectation_gap":       0.03,
			"already_priced_in":     false,
			"surprise_potential":    0.6,
			"confidence":            0.7,
		})
		activeEvents = append(activeEvents, map[string]any{
			"theme":      "spring_festival_season",
			"confidence": 0.65,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"expectations":           expectations,
		"active_seasonal_events": activeEvents,
	})
}

// handleMetrics 處理指標查詢請求
func (a *DashboardAPI) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 從查詢參數獲取指標類型
	metricType := r.URL.Query().Get("type")

	var response interface{}

	switch metricType {
	case "screening":
		response = map[string]interface{}{
			"screening_rate":   a.metricsCollector.GetScreeningRate(),
			"screening_total":  a.metricsCollector.GetMetricsSnapshot().ScreeningTotal,
			"screening_passed": a.metricsCollector.GetMetricsSnapshot().ScreeningPassed,
		}
	case "alerts":
		snapshot := a.metricsCollector.GetMetricsSnapshot()
		response = map[string]interface{}{
			"alerts_triggered":    snapshot.AlertsTriggered,
			"alerts_acknowledged": snapshot.AlertsAcknowledged,
			"alerts_by_type":      snapshot.AlertsByType,
		}
	case "all":
		response = a.metricsCollector.GetMetricsSnapshot()
	default:
		response = a.metricsCollector.GetMetricsSnapshot()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (a *DashboardAPI) handleIndustryClassification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	tree := a.industryClassifier
	segments := tree.GetAllSegments()

	var result []map[string]interface{}
	for _, seg := range segments {
		if seg.ParentID == "" {
			children := tree.GetChildren(seg.ID)
			var childList []map[string]interface{}
			for _, child := range children {
				grandchildren := tree.GetChildren(child.ID)
				var grandchildList []map[string]interface{}
				for _, gc := range grandchildren {
					grandchildList = append(grandchildList, map[string]interface{}{
						"id":          gc.ID,
						"name":        gc.Name,
						"weight":      gc.Weight,
						"description": gc.Description,
					})
				}
				childList = append(childList, map[string]interface{}{
					"id":          child.ID,
					"name":        child.Name,
					"weight":      child.Weight,
					"description": child.Description,
					"children":    grandchildList,
				})
			}
			result = append(result, map[string]interface{}{
				"id":          seg.ID,
				"name":        seg.Name,
				"weight":      seg.Weight,
				"description": seg.Description,
				"children":    childList,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"industries": result,
		"count":      len(result),
	})
}

func (a *DashboardAPI) handleIndustrySeasonality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	now := time.Now()

	patterns := a.seasonalEngine.DetectCurrentPatterns(now)
	var activePatterns []map[string]interface{}
	for _, p := range patterns {
		if industryID == "" || p.IsRelevantForIndustry(industryID) {
			activePatterns = append(activePatterns, map[string]interface{}{
				"id":                  p.ID,
				"name":                p.Name,
				"description":         p.Description,
				"start_month":         p.StartMonth,
				"start_day":           p.StartDay,
				"end_month":           p.EndMonth,
				"end_day":             p.EndDay,
				"historical_accuracy": p.HistoricalAccuracy,
				"typical_return":      p.TypicalReturn(),
				"affected_industries": p.AffectedIndustries,
			})
		}
	}

	var adjustment float64
	if industryID != "" {
		adjustment = a.seasonalEngine.GetPatternAdjustment(industryID, now)
	}

	allPatterns := a.seasonalEngine.GetAllPatterns()
	var historicalPatterns []map[string]interface{}
	for _, p := range allPatterns {
		if industryID == "" || p.IsRelevantForIndustry(industryID) {
			historicalPatterns = append(historicalPatterns, map[string]interface{}{
				"id":                  p.ID,
				"name":                p.Name,
				"name_en":             p.NameEN,
				"description":         p.Description,
				"start_month":         p.StartMonth,
				"start_day":           p.StartDay,
				"end_month":           p.EndMonth,
				"end_day":             p.EndDay,
				"historical_accuracy": p.HistoricalAccuracy,
				"typical_return":      p.TypicalReturn(),
				"adjustment_factor":   p.AdjustmentFactor,
				"favored_industries":  p.FavoredIndustries,
				"avoided_industries":  p.AvoidedIndustries,
				"impact": func() string {
					if industryID == "" {
						return ""
					}
					impact, _ := a.seasonalEngine.GetIndustryImpact(p.ID, industryID)
					return impact
				}(),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"current_date":        now.Format("2006-01-02"),
		"active_patterns":     activePatterns,
		"pattern_count":       len(activePatterns),
		"adjustment":          adjustment,
		"all_patterns":        historicalPatterns,
		"total_pattern_count": len(historicalPatterns),
	})
}

func (a *DashboardAPI) handleIndustrySeasonalityCalendar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	now := time.Now()
	calendar := a.seasonalEngine.GenerateCalendar(now.Year())

	var months []map[string]interface{}
	for m := 1; m <= 12; m++ {
		monthPatterns := calendar.ByMonth[m]
		var relevantPatterns []map[string]interface{}
		for _, p := range monthPatterns {
			if industryID == "" || p.IsRelevantForIndustry(industryID) {
				relevantPatterns = append(relevantPatterns, map[string]interface{}{
					"id":                  p.ID,
					"name":                p.Name,
					"historical_accuracy": p.HistoricalAccuracy,
					"typical_return":      p.TypicalReturn(),
					"adjustment_factor":   p.AdjustmentFactor,
				})
			}
		}
		months = append(months, map[string]interface{}{
			"month":    m,
			"patterns": relevantPatterns,
			"count":    len(relevantPatterns),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"year":     calendar.Year,
		"industry": industryID,
		"months":   months,
	})
}

func (a *DashboardAPI) handleIndustryCycle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")

	if industryID == "" {
		var allPositions []map[string]interface{}
		for _, seg := range a.industryClassifier.GetAllSegments() {
			if seg.ParentID != "" {
				continue
			}
			if pos, ok := a.cycleTracker.GetPosition(seg.ID); ok {
				allPositions = append(allPositions, map[string]interface{}{
					"industry":        seg.ID,
					"name":            seg.Name,
					"business_cycle":  pos.BusinessCycle,
					"inventory_cycle": pos.InventoryCycle,
					"capex_cycle":     pos.CapexCycle,
					"confidence":      pos.Confidence,
					"updated_at":      pos.UpdatedAt,
					"is_favorable":    pos.IsFavorable(),
					"phase_score":     pos.GetPhaseScore(),
					"trend":           pos.GetTrend(),
				})
			}
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"industries": allPositions,
			"count":      len(allPositions),
		})
		return
	}

	position, ok := a.cycleTracker.GetPosition(industryID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "industry not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"industry":        industryID,
		"business_cycle":  position.BusinessCycle,
		"inventory_cycle": position.InventoryCycle,
		"capex_cycle":     position.CapexCycle,
		"confidence":      position.Confidence,
		"updated_at":      position.UpdatedAt,
		"is_favorable":    position.IsFavorable(),
		"phase_score":     position.GetPhaseScore(),
		"trend":           position.GetTrend(),
	})
}

func (a *DashboardAPI) handleIndustryLinkage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	industryID := r.URL.Query().Get("industry")
	if industryID == "" {
		writeJSONError(w, http.StatusBadRequest, "industry parameter required")
		return
	}

	graph := a.linkageAnalyzer.GetSupplyChainGraph()
	upstream := graph.GetUpstream(industryID)
	downstream := graph.GetDownstream(industryID)

	correlations := a.linkageAnalyzer.GetCorrelationMatrix().GetCorrelatedIndustries(industryID, 0.0)
	var correlationList []map[string]interface{}
	for otherIndustry, correlation := range correlations {
		strength := "low"
		if math.Abs(correlation) > 0.7 {
			strength = "high"
		} else if math.Abs(correlation) > 0.4 {
			strength = "medium"
		}
		correlationList = append(correlationList, map[string]interface{}{
			"industry":    otherIndustry,
			"correlation": correlation,
			"strength":    strength,
		})
	}

	score := a.linkageAnalyzer.CalculateLinkageScore(industryID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"industry":      industryID,
		"upstream":      upstream,
		"downstream":    downstream,
		"correlations":  correlationList,
		"linkage_score": score,
	})
}

func (a *DashboardAPI) handleIndustryRisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	symbol := r.URL.Query().Get("symbol")
	industryID := r.URL.Query().Get("industry")
	if symbol == "" && industryID == "" {
		writeJSONError(w, http.StatusBadRequest, "symbol or industry parameter required")
		return
	}

	var risks []industry.RiskEvent
	if symbol == "" && industryID != "" {
		risks = a.riskMonitor.GetAllRisks("ALL", industryID, 0, 0)
	} else {
		risks = a.riskMonitor.GetAllRisks(symbol, industryID, 0, 0)
	}
	var riskList []map[string]interface{}
	for _, risk := range risks {
		riskList = append(riskList, map[string]interface{}{
			"id":                 risk.ID,
			"type":               risk.Type,
			"severity":           risk.Severity,
			"description":        risk.Description,
			"impact_estimate":    risk.ImpactEstimate,
			"confidence":         risk.Confidence,
			"detected_at":        risk.DetectedAt,
			"recommended_action": risk.RecommendedAction,
		})
	}

	highest := a.riskMonitor.GetHighestRisk(risks)
	var highestRisk map[string]interface{}
	if highest != nil {
		highestRisk = map[string]interface{}{
			"id":          highest.ID,
			"type":        highest.Type,
			"severity":    highest.Severity,
			"description": highest.Description,
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"symbol":       symbol,
		"industry":     industryID,
		"risk_count":   len(riskList),
		"risks":        riskList,
		"highest_risk": highestRisk,
	})
}

func (a *DashboardAPI) handleIndustryOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	now := time.Now()
	tree := a.industryClassifier
	segments := tree.GetAllSegments()

	var industries []map[string]interface{}
	for _, seg := range segments {
		if seg.ParentID != "" {
			continue
		}

		cyclePos, ok := a.cycleTracker.GetPosition(seg.ID)
		if !ok {
			continue
		}

		patterns := a.seasonalEngine.DetectCurrentPatterns(now)
		var activePatternNames []string
		for _, p := range patterns {
			if p.IsRelevantForIndustry(seg.ID) {
				activePatternNames = append(activePatternNames, p.Name)
			}
		}

		linkageScore := a.linkageAnalyzer.CalculateLinkageScore(seg.ID)

		industries = append(industries, map[string]interface{}{
			"id":                seg.ID,
			"name":              seg.Name,
			"cycle_phase":       cyclePos.BusinessCycle,
			"inventory_cycle":   cyclePos.InventoryCycle,
			"capex_cycle":       cyclePos.CapexCycle,
			"cycle_confidence":  cyclePos.Confidence,
			"is_favorable":      cyclePos.IsFavorable(),
			"seasonal_patterns": activePatternNames,
			"linkage_score":     linkageScore,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"industries": industries,
		"count":      len(industries),
		"updated_at": now,
	})
}

func (a *DashboardAPI) handleShockSimulation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		SourceIndustry string  `json:"source_industry"`
		ShockMagnitude float64 `json:"shock_magnitude"`
		MaxDepth       int     `json:"max_depth"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json: "+err.Error())
		return
	}

	if req.SourceIndustry == "" {
		writeJSONError(w, http.StatusBadRequest, "source_industry required")
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = 3
	}

	impacts := a.linkageAnalyzer.PropagateShock(req.SourceIndustry, req.ShockMagnitude, req.MaxDepth)

	var impactList []map[string]interface{}
	for industry, impact := range impacts {
		impactList = append(impactList, map[string]interface{}{
			"industry": industry,
			"impact":   impact,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"source":       req.SourceIndustry,
		"shock":        req.ShockMagnitude,
		"max_depth":    req.MaxDepth,
		"impacts":      impactList,
		"impact_count": len(impactList),
	})
}

func (a *DashboardAPI) handleIndustryGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cm := a.linkageAnalyzer.GetCorrelationMatrix()

	var nodes []map[string]interface{}
	var edges []map[string]interface{}
	nodeSet := make(map[string]bool)

	allCorrelations := cm.GetAllCorrelations()
	for industryA, correlations := range allCorrelations {
		if !nodeSet[industryA] {
			nodeSet[industryA] = true
			score := a.linkageAnalyzer.CalculateLinkageScore(industryA)
			nodes = append(nodes, map[string]interface{}{
				"id":                  industryA,
				"systemic_importance": score.SystemicImportance,
				"upstream_count":      score.UpstreamCount,
				"downstream_count":    score.DownstreamCount,
			})
		}

		for industryB, correlation := range correlations {
			if industryA >= industryB {
				continue
			}
			if !nodeSet[industryB] {
				nodeSet[industryB] = true
				score := a.linkageAnalyzer.CalculateLinkageScore(industryB)
				nodes = append(nodes, map[string]interface{}{
					"id":                  industryB,
					"systemic_importance": score.SystemicImportance,
					"upstream_count":      score.UpstreamCount,
					"downstream_count":    score.DownstreamCount,
				})
			}

			strength := "low"
			if math.Abs(correlation) > 0.7 {
				strength = "high"
			} else if math.Abs(correlation) > 0.4 {
				strength = "medium"
			}

			edges = append(edges, map[string]interface{}{
				"source":      industryA,
				"target":      industryB,
				"correlation": correlation,
				"strength":    strength,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"nodes": nodes,
		"edges": edges,
	})
}

func (a *DashboardAPI) handleMetricsTrend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metric := r.URL.Query().Get("metric")
	period := r.URL.Query().Get("period")

	if metric == "" {
		metric = "screening_rate"
	}
	if period == "" {
		period = "24h"
	}

	var duration time.Duration
	switch period {
	case "24h":
		duration = 24 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	default:
		duration = 24 * time.Hour
	}

	snapshot := a.metricsCollector.GetMetricsSnapshot()
	trend := a.metricsHistory.GetTrend(metric)

	var filteredTrend []TrendPoint
	cutoff := time.Now().Add(-duration)
	for _, point := range trend {
		if point.Timestamp.After(cutoff) {
			filteredTrend = append(filteredTrend, point)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"metric":      metric,
		"period":      period,
		"duration":    duration.String(),
		"current":     snapshot,
		"trend":       filteredTrend,
		"data_points": len(filteredTrend),
	})
}

func (a *DashboardAPI) handleDataQuality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	report := a.dataQualityChecker.RunAll(ctx)

	writeJSON(w, http.StatusOK, report)
}
