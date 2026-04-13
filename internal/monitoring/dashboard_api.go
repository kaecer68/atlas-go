package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

type DashboardAPI struct {
	ledgerDir        string
	narrativeEngine  *narrative.NarrativeEngine
	macroIngestor    *narrative.MacroIngestor
	taiwanStressCalc *narrative.TaiwanStressCalculator
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

func NewDashboardAPI(ledgerDir string) *DashboardAPI {
	provider := marketdata.NewCompositeMacroProvider(
		marketdata.NewYahooFinanceMacroProvider(),
		marketdata.NewTWSECapitalFlowProvider("data/state/capital_flow"),
	)
	geoProvider := narrative.NewCompositeGeopoliticalProvider(
		narrative.NewRSSGeopoliticalProvider(),
		narrative.NewGDELTGeopoliticalProvider(),
	)
	return &DashboardAPI{
		ledgerDir:        ledgerDir,
		narrativeEngine:  narrative.NewNarrativeEngine(),
		macroIngestor:    narrative.NewMacroIngestor(provider, "data/state/macro"),
		taiwanStressCalc: narrative.NewTaiwanStressCalculator(geoProvider),
	}
}

func (a *DashboardAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/macro-radar", a.handleMacroRadar)
	mux.HandleFunc("/api/dashboard/agent-observatory", a.handleAgentObservatory)
	mux.HandleFunc("/api/dashboard/forecast-vs-reality", a.handleForecastVsReality)
	mux.HandleFunc("/api/report/latest", a.handleLatestReport)
	mux.HandleFunc("/api/report/list", a.handleReportList)
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
	mux.HandleFunc("/dashboard/narrative", a.handleNarrativeDashboard)
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
}

func (a *DashboardAPI) handleLiveStatus(w http.ResponseWriter, r *http.Request) {
	status := a.loadLiveStatus()
	writeJSON(w, http.StatusOK, status)
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
	if data, err := os.ReadFile("data/state/circuit_breaker_state.json"); err == nil {
		_ = json.Unmarshal(data, &cbState)
	}

	// Load portfolio from live state store JSONL (last line = latest)
	portfolio := struct {
		Cash          float64   `json:"cash"`
		TotalExposure float64   `json:"total_exposure"`
		AvailableCash float64   `json:"available_cash"`
		DayPnL        float64   `json:"day_pnl"`
		UnrealizedPnL float64   `json:"unrealized_pnl"`
	}{}
	if data, err := os.ReadFile("data/state/live/state/portfolio_state.jsonl"); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) > 0 {
			_ = json.Unmarshal([]byte(lines[len(lines)-1]), &portfolio)
		}
	}

	// Load positions count from live state store JSONL
	positions := make(map[string]interface{})
	if data, err := os.ReadFile("data/state/live/state/positions_current.jsonl"); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(lines) > 0 {
			_ = json.Unmarshal([]byte(lines[len(lines)-1]), &positions)
		}
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

func (a *DashboardAPI) handleNarrativeDashboard(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("web/static/narrative-dashboard.html")
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "narrative dashboard not found")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
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
	data, err := os.ReadFile("docs/swagger.json")
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

	slices.SortFunc(summaries, func(a, b domain.SessionSummary) int {
		switch {
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
	reportDir := "reports"
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

func (a *DashboardAPI) handleLatestReport(w http.ResponseWriter, r *http.Request) {
	reportDir := "reports"
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
		ID:           fmt.Sprintf("int-approve-%s-%d", req.Symbol, time.Now().UnixNano()),
		Type:         "approve_rec",
		TargetSymbol: req.Symbol,
		TargetAgentID: req.AgentID,
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
		ID:           fmt.Sprintf("int-reject-%s-%d", req.Symbol, time.Now().UnixNano()),
		Type:         "reject_rec",
		TargetSymbol: req.Symbol,
		TargetAgentID: req.AgentID,
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
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"paused_agents":  mapKeys(pausedAgents),
		"banned_sectors": mapKeys(bannedSectors),
		"model_weights":  modelWeights,
	})
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
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("ingest failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":   events,
		"snapshot": snap,
	})
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

	index, err := a.taiwanStressCalc.CalculateFromSnapshot(r.Context(), snap)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("calculate stress index: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, index)
}
