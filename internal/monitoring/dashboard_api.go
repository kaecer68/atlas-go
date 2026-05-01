package monitoring

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/live"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	apibacktest "github.com/kaecer68/atlas-go/internal/monitoring/api/backtest"
	apicontrol "github.com/kaecer68/atlas-go/internal/monitoring/api/control"
	apidata "github.com/kaecer68/atlas-go/internal/monitoring/api/data"
	apiexperiment "github.com/kaecer68/atlas-go/internal/monitoring/api/experiment"
	apihealth "github.com/kaecer68/atlas-go/internal/monitoring/api/health"
	apiindustry "github.com/kaecer68/atlas-go/internal/monitoring/api/industry"
	apilive "github.com/kaecer68/atlas-go/internal/monitoring/api/live"
	apimacro "github.com/kaecer68/atlas-go/internal/monitoring/api/macro"
	apimarketdata "github.com/kaecer68/atlas-go/internal/monitoring/api/marketdata"
	apimetrics "github.com/kaecer68/atlas-go/internal/monitoring/api/metrics"
	apinarrative "github.com/kaecer68/atlas-go/internal/monitoring/api/narrative"
	apipipeline "github.com/kaecer68/atlas-go/internal/monitoring/api/pipeline"
	apireport "github.com/kaecer68/atlas-go/internal/monitoring/api/report"
	apirisk "github.com/kaecer68/atlas-go/internal/monitoring/api/risk"
	apiswagger "github.com/kaecer68/atlas-go/internal/monitoring/api/swagger"
	apisystem "github.com/kaecer68/atlas-go/internal/monitoring/api/system"
	apitax "github.com/kaecer68/atlas-go/internal/monitoring/api/tax"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
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
	industryService    *service.IndustryService
	metricsCollector   *MetricsCollector
	metricsHistory     *MetricsHistory
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
		industryService:    service.NewIndustryService(industry.DefaultClassification(), industry.NewSeasonalEngine(), industry.NewCycleTracker(), industry.NewLinkageAnalyzer(), industry.NewRiskMonitor()),
		metricsCollector:   metricsCollector,
		metricsHistory:     NewMetricsHistory(1000),
		healthManager:      portfolio.NewAgentHealthManager(),
		dataQualityChecker: NewDataQualityChecker(workDir, ledgerDir),
	}
}

func (a *DashboardAPI) RegisterRoutes(mux *http.ServeMux) {
	pipelineSvc := service.NewPipelineService(a.workDir, a.ledgerDir)
	pipelineHandlers := apipipeline.NewHandlers(pipelineSvc)
	pipelineHandlers.RegisterRoutes(mux)

	reportSvc := service.NewReportService(a.workDir, a.ledgerDir)
	reportHandlers := apireport.NewHandlers(reportSvc)
	reportHandlers.RegisterRoutes(mux)

	metricsSvc := service.NewMetricsService(
		&service.MetricsCollectorAdapter{
			GetScreeningRateFunc: a.metricsCollector.GetScreeningRate,
			GetMetricsSnapshotFunc: func() service.MetricsSnapshot {
				snap := a.metricsCollector.GetMetricsSnapshot()
				return service.MetricsSnapshot{
					ScreeningTotal:     snap.ScreeningTotal,
					ScreeningPassed:    snap.ScreeningPassed,
					ScreeningRate:      snap.ScreeningRate,
					AlertsTriggered:    snap.AlertsTriggered,
					AlertsAcknowledged: snap.AlertsAcknowledged,
					AlertsByType:       snap.AlertsByType,
					Timestamp:          snap.Timestamp,
				}
			},
		},
		&service.MetricsHistoryAdapter{
			GetTrendFunc: func(metric string) []service.TrendPoint {
				points := a.metricsHistory.GetTrend(metric)
				result := make([]service.TrendPoint, len(points))
				for i, p := range points {
					result[i] = service.TrendPoint{
						Timestamp: p.Timestamp,
						Value:     p.Value,
					}
				}
				return result
			},
		},
	)
	metricsHandlers := apimetrics.NewHandlers(metricsSvc)
	metricsHandlers.RegisterRoutes(mux)

	systemSvc := service.NewSystemService(a.workDir, a.ledgerDir, a.baselinePath)
	systemHandlers := apisystem.NewHandlers(systemSvc)
	systemHandlers.RegisterRoutes(mux)

	handlers := &apihealth.Handlers{}
	handlers.RegisterRoutes(mux)

	dataHandlers := apidata.NewHandlers(a.workDir, a.pool, a.macroIngestor, a.geoProvider, a.taiwanGeoProvider, a.janusEngine, &channelHealthAdapter{store: NewChannelHealthStoreWithPool(filepath.Join(a.workDir, "data/state"), a.pool)})
	dataHandlers.RegisterRoutes(mux)

	riskHandlers := apirisk.NewHandlers(a.ledgerDir)
	riskHandlers.RegisterRoutes(mux)

	marketdataHandlers := apimarketdata.NewHandlers(a.workDir)
	marketdataHandlers.RegisterRoutes(mux)

	taxHandlers := apitax.NewHandlers()
	taxHandlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterIndustryRoutes(mux *http.ServeMux) {
	handlers := &apiindustry.Handlers{
		Svc: a.industryService,
	}
	handlers.RegisterRoutes(mux)
}

// RegisterSwaggerRoutes mounts Swagger UI and the OpenAPI spec.
func (a *DashboardAPI) RegisterSwaggerRoutes(mux *http.ServeMux) {
	swaggerHandlers := apiswagger.NewHandlers(a.workDir)
	swaggerHandlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterNarrativeRoutes(mux *http.ServeMux) {
	svc := service.NewNarrativeService(a.workDir, a.narrativeEngine, a.reportGenerator)
	handlers := &apinarrative.Handlers{
		Svc: svc,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterControlRoutes(mux *http.ServeMux) {
	svc := service.NewControlService(a.workDir, a.ledgerDir, a.healthManager)
	handlers := &apicontrol.Handlers{
		Svc: svc,
	}
	handlers.RegisterRoutes(mux)
}

// RegisterMacroRoutes mounts macro data snapshot endpoints.
func (a *DashboardAPI) RegisterMacroRoutes(mux *http.ServeMux) {
	svc := service.NewMacroService(a.workDir, a.macroIngestor, a.taiwanStressCalc)
	handlers := &apimacro.Handlers{
		Service: svc,
	}
	handlers.RegisterRoutes(mux)
}

func (a *DashboardAPI) RegisterPhase3Routes(mux *http.ServeMux) {
}

func (a *DashboardAPI) RegisterLiveRoutes(mux *http.ServeMux) {
	svc := service.NewLiveService(a.workDir, a.ledgerDir)
	handlers := &apilive.Handlers{
		LedgerDir: a.ledgerDir,
		WorkDir:   a.workDir,
		Svc:       svc,
	}
	handlers.RegisterRoutes(mux)
}

// RegisterExperimentRoutes mounts experiment lifecycle endpoints.
func (a *DashboardAPI) RegisterExperimentRoutes(mux *http.ServeMux) {
	handlers := &apiexperiment.Handlers{
		BaselinePath: a.baselinePath,
		LedgerDir:    a.ledgerDir,
		WorkDir:      a.workDir,
	}
	handlers.RegisterRoutes(mux)
}

// RegisterBacktestRoutes mounts backtest execution endpoints.
func (a *DashboardAPI) RegisterBacktestRoutes(mux *http.ServeMux) {
	svc := service.NewBacktestService()
	handlers := apibacktest.NewHandlers(svc)
	handlers.RegisterRoutes(mux)
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

func (a *DashboardAPI) handlePhase3Status(w http.ResponseWriter, r *http.Request) {
	metrics, err := orchestrator.LoadPhase3Metrics("")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load phase3 metrics: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, metrics)
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
	latestSummary, _ := LoadSessionSummary(a.ledgerDir, "")
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
	if summary, err := LoadSessionSummary(a.ledgerDir, ""); err == nil && summary != nil {
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
	summary, err := LoadSessionSummary(a.ledgerDir, sessionID)
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

	summary, err := LoadSessionSummary(a.ledgerDir, sessionID)
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
	summary, err := LoadSessionSummary(a.ledgerDir, "")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load forecast-vs-reality summary context: %v", err))
		return
	}
	if summary != nil {
		resp.BrokerRuntime = summary.BrokerRuntime
	}
	writeJSON(w, http.StatusOK, resp)
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

	// Export statistics (customs open data — replaced TWSE FAS210)
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
			NewChannelHealthStoreWithPool(stateDir, a.pool).Record("tej", "inactive", "TEJ_API_KEY not set")
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

	summary, err := LoadSessionSummary(a.ledgerDir, sessionID)
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
	// TWSE FAS210 decommissioned; replaced with customs open data (data.gov.tw dataset 6053).
	exportDir := filepath.Join(a.workDir, "data/state/export")
	exportStatus, exportUpdated := a.checkExportHealth(exportDir, now)
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
		Platform:   "海關進出口統計 (data.gov.tw)",
		APIFormat:  "CSV",
		Path:       "opendata.customs.gov.tw/data/6053/csv.csv",
		Storage:    "data/state/export/*_export.json",
		Status:     exportStatus,
		StatusText: statusText(exportStatus),
		UpdatedAt:  exportUpdated,
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
	janusLastError := ""
	janusRec := healthStore.Get("janus_regime")
	if janusRec != nil {
		if janusRec.LastError != "" {
			janusLastError = janusRec.LastError
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
		LastError:  janusLastError,
	})

	// 12. TEJ (Taiwan Economic Journal - premium financial data)
	tejStatus := "inactive"
	tejUpdated := "TEJ_API_KEY not configured"
	tejKey := os.Getenv("TEJ_API_KEY")
	if tejKey != "" {
		tejStatus = "ok"
		tejUpdated = "TEJ API key configured"
		tejRec := healthStore.Get("tej")
		if tejRec != nil && tejRec.Status != "" {
			tejStatus = tejRec.Status
			if tejRec.LastError != "" {
				tejUpdated = "上次失敗: " + tejRec.LastError
			} else if tejRec.LastSuccessAt != "" {
				tejUpdated = "上次成功: " + tejRec.LastSuccessAt
			}
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
		LastError:  "",
	})

	knownInactive := map[string]bool{
		"fubon": true,
	}

	var freshAlerts []ChannelAlert
	for _, c := range channels {
		if c.Status == "error" || c.Status == "warn" {
			if knownInactive[c.ChannelID] {
				continue
			}
			freshAlerts = append(freshAlerts, ChannelAlert{
				ChannelID: c.ChannelID,
				Status:    c.Status,
				Error:     c.LastError,
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"channels":  channels,
		"alerts":    freshAlerts,
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

func (a *DashboardAPI) checkExportHealth(dir string, now time.Time) (string, string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		return "error", "無資料"
	}
	var latestFile string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), "_export.json") {
			continue
		}
		if e.Name() > latestFile {
			latestFile = e.Name()
		}
	}
	if latestFile == "" {
		return "error", "無有效檔案"
	}
	dateStr := strings.TrimSuffix(latestFile, "_export.json")

	var dataTs time.Time
	data, err := os.ReadFile(filepath.Join(dir, latestFile))
	if err == nil {
		var exp struct {
			Year  int `json:"year"`
			Month int `json:"month"`
		}
		if json.Unmarshal(data, &exp) == nil && exp.Year > 0 && exp.Month >= 1 {
			dataTs = time.Date(exp.Year+1911, time.Month(exp.Month), 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
		}
	}

	var t time.Time
	if !dataTs.IsZero() {
		t = dataTs
	} else {
		if len(dateStr) != 5 {
			return "error", "日期解析失敗"
		}
		rocYear, err1 := strconv.Atoi(dateStr[:3])
		month, err2 := strconv.Atoi(dateStr[3:])
		if err1 != nil || err2 != nil || month < 1 || month > 12 {
			return "error", "日期解析失敗"
		}
		t = time.Date(rocYear+1911, time.Month(month), 1, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	}

	age := now.Sub(t)
	if age < 45*24*time.Hour {
		return "ok", dateStr
	}
	if age < 90*24*time.Hour {
		return "warn", dateStr
	}
	return "error", dateStr
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
		TotalCapital:    0,
		DeployedCapital: 0,
		ReserveCash:     0,
		RollingSharpe:   0,
		MaxDrawdown:     0,
		CanAdvance:      false,
		AdvanceReason:   "no live trading data available",
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
		"note":             "no live trading data — capital phase requires orchestrator.System in live mode",
	})
}

func (a *DashboardAPI) handleTaxSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"snapshots":      []domain.TaxSnapshot{},
		"before_tax_pnl": 0,
		"after_tax_pnl":  0,
		"total_tax_paid": 0,
		"is_simulated":   true,
		"note":           "no trading sessions recorded — tax snapshots computed from ledger outcomes",
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

// channelHealthAdapter adapts monitoring.ChannelHealthStore to data.ChannelHealthRecorder.
type channelHealthAdapter struct {
	store *ChannelHealthStore
}

func (a *channelHealthAdapter) Record(channelID, status, errMsg string) error {
	return a.store.Record(channelID, status, errMsg)
}

func (a *channelHealthAdapter) Get(channelID string) *apidata.ChannelHealthRecord {
	rec := a.store.Get(channelID)
	if rec == nil {
		return nil
	}
	return &apidata.ChannelHealthRecord{
		Status:        rec.Status,
		LastFetchAt:   rec.LastFetchAt,
		LastError:     rec.LastError,
		LastSuccessAt: rec.LastSuccessAt,
	}
}

func (a *channelHealthAdapter) Alerts() []apidata.ChannelAlert {
	alerts := a.store.Alerts()
	result := make([]apidata.ChannelAlert, len(alerts))
	for i, al := range alerts {
		result[i] = apidata.ChannelAlert{
			ChannelID: al.ChannelID,
			Status:    al.Status,
			Error:     al.Error,
			FetchAt:   al.FetchAt,
		}
	}
	return result
}
