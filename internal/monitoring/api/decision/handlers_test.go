package decision

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/strategy_techniques"
)

func mp(value, changePct float64) marketdata.MacroDataPoint {
	return marketdata.MacroDataPoint{Symbol: "TEST", Value: value, ChangePct: changePct}
}

//go:fix inline
func float64Ptr(v float64) *float64 { return new(v) }

func TestResolveSymbolName(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"known with .TW", "2330.TW", "台積電"},
		{"known without .TW", "2330", "台積電"},
		{"unknown symbol", "ZZZZ.TW", "ZZZZ.TW"},
		{"unknown without .TW", "ZZZZ", "ZZZZ"},
		{"financial stock", "2881.TW", "富邦金"},
		{"etf", "0050.TW", "元大台灣50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSymbolName(tt.input)
			if got != tt.expect {
				t.Errorf("resolveSymbolName(%q) = %q, want %q", tt.input, got, tt.expect)
			}
		})
	}
}

func TestBuildMarketNarrativeData_Nil(t *testing.T) {
	got := buildMarketNarrativeData(nil)
	if got != (narrative.MarketNarrativeData{}) {
		t.Errorf("expected zero-value MarketNarrativeData, got %+v", got)
	}
}

func TestBuildMarketNarrativeData_Valid(t *testing.T) {
	snap := &marketdata.MacroDataSnapshot{
		US10Y:             mp(4.5, 0.025),
		DXY:               mp(104, -0.3),
		VIX:               mp(18.5, -5.0),
		USD_TWD:           mp(32.5, 0.1),
		Oil:               mp(78, -1.5),
		Gold:              mp(2350, 0.5),
		JPY:               mp(150.5, 0.2),
		CPIYoY:            mp(2.5, 0),
		Bdi:               mp(1800, -2.0),
		Copper:            mp(4.5, 1.0),
		ExportElectronics: mp(0, 5.0),
		SOXIndex:          mp(5000, 2.0),
		DRAMSpotPrice:     mp(1.5, -0.5),
		SPXIndex:          mp(5500, 1.0),
		NDXIndex:          mp(19000, 1.5),
		DJIIndex:          mp(42000, 0.5),
		TSMADR:            mp(180, 3.0),
	}
	got := buildMarketNarrativeData(snap)
	if got.US10YChangeBps != 2.5 {
		t.Errorf("US10YChangeBps = %v, want 2.5", got.US10YChangeBps)
	}
	if got.VIXLevel != 18.5 {
		t.Errorf("VIXLevel = %v, want 18.5", got.VIXLevel)
	}
	if got.TSMADRChangePct != 3.0 {
		t.Errorf("TSMADRChangePct = %v, want 3.0", got.TSMADRChangePct)
	}
	if got.GoldLevel != 2350 {
		t.Errorf("GoldLevel = %v, want 2350", got.GoldLevel)
	}
}

func TestBuildPremarketData_Nil(t *testing.T) {
	got := buildPremarketData(nil)
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestBuildPremarketData_Valid(t *testing.T) {
	snap := &marketdata.MacroDataSnapshot{
		SOXIndex:           mp(5000, 2.5),
		ForeignInvestorNet: mp(5000, 0),
		USD_TWD:            mp(32.5, 0.1),
		Bdi:                mp(1800, -2.0),
		VIX:                mp(18, -5.0),
		DXY:                mp(104.5, -0.3),
		Oil:                mp(78, -1.5),
	}
	got := buildPremarketData(snap)
	if got == nil {
		t.Fatal("expected non-nil PremarketData")
	}
	if got.USMarket["sox_pct"] != 2.5 {
		t.Errorf("sox_pct = %v, want 2.5", got.USMarket["sox_pct"])
	}
	if got.ForeignFlow["net_buy_twd"] != 5000.0 {
		t.Errorf("net_buy_twd = %v, want 5000", got.ForeignFlow["net_buy_twd"])
	}
	if got.FX["usd_twd"] != 32.5 {
		t.Errorf("usd_twd = %v, want 32.5", got.FX["usd_twd"])
	}
}

func TestBuildPremarketData_SetsStressIndex(t *testing.T) {
	snap := &marketdata.MacroDataSnapshot{
		DXY: mp(104.5, -0.3),
		Oil: mp(78, -1.5),
		VIX: mp(18, -5.0),
	}
	got := buildPremarketData(snap)
	if got == nil || got.StressIndex == nil {
		t.Fatal("expected non-nil StressIndex")
	}
	if got.StressIndex["dxy"] != 104.5 {
		t.Errorf("dxy = %v, want 104.5", got.StressIndex["dxy"])
	}
	if got.StressIndex["vix_level"] != 18.0 {
		t.Errorf("vix_level = %v, want 18", got.StressIndex["vix_level"])
	}
}

func TestBuildCoreIndicators_Nil(t *testing.T) {
	h := &Handlers{}
	got := h.buildCoreIndicators(nil)
	if got != nil {
		t.Errorf("expected nil CoreIndicators, got %+v", got)
	}
}

func TestBuildCoreIndicators_Valid(t *testing.T) {
	snap := &marketdata.MacroDataSnapshot{
		ForeignInvestorNet: mp(10000, 0),
		TSMADR:             mp(180, 3.5),
		NVDA:               mp(120, 2.0),
		DXY:                mp(104, -0.3),
	}
	h := &Handlers{}
	got := h.buildCoreIndicators(snap)
	if got == nil {
		t.Fatal("expected non-nil CoreIndicators")
	}
	if got.ForeignCapitalNetTWD == nil || *got.ForeignCapitalNetTWD != 10000 {
		t.Errorf("ForeignCapitalNetTWD = %v, want 10000", got.ForeignCapitalNetTWD)
	}
	if got.TSMADRpct == nil || *got.TSMADRpct != 3.5 {
		t.Errorf("TSMADRpct = %v, want 3.5", got.TSMADRpct)
	}
	if got.NVDApct == nil || *got.NVDApct != 2.0 {
		t.Errorf("NVDApct = %v, want 2.0", got.NVDApct)
	}
	if got.DXYpct == nil || *got.DXYpct != -0.3 {
		t.Errorf("DXYpct = %v, want -0.3", got.DXYpct)
	}
}

func newTestRegistry(t *testing.T) *strategy_techniques.Registry {
	t.Helper()
	const seeds = `[
  {"id":"alpha","name":"alpha","layer":"L1","summary":"us rate","conditions":[{"field":"DXY.ChangePct","operator":"lt","value":0,"timeframe":"1D","source":"us_yahoo"}],
   "direction":"up","risk":"medium","source":"backtest","status":"active","attribution_mode":"rule_based"},
  {"id":"beta","name":"beta","layer":"L2","summary":"foreign","conditions":[{"field":"ForeignInvestorNet.Value","operator":"gt","value":0,"timeframe":"1D","source":"twse_capital_flow"}],
   "direction":"up","risk":"low","source":"backtest","status":"active","attribution_mode":"rule_based"},
  {"id":"gamma","name":"gamma","layer":"L3","summary":"nvidia","conditions":[{"field":"NVDA.ChangePct","operator":"gt","value":0.5,"timeframe":"1D","source":"us_nvda"}],
   "direction":"up","risk":"low","source":"backtest","status":"degraded","attribution_mode":"rule_based","attribution":["regime_shift"]},
  {"id":"delta","name":"delta","layer":"L5","summary":"geopolitical","conditions":[{"field":"DXY.ChangePct","operator":"gt","value":0.5,"timeframe":"1D","source":"us_yahoo"}],
   "direction":"volatile","risk":"high","source":"manual","status":"active","attribution_mode":"llm_annotated"},
  {"id":"epsilon","name":"epsilon","layer":"L5","summary":"expired","conditions":[{"field":"VIX.Value","operator":"gt","value":30,"timeframe":"1D","source":"us_yahoo"}],
   "direction":"down","risk":"high","source":"manual","status":"expired","attribution_mode":"llm_annotated","attribution":["event_resolved"]}
]`
	reg, err := strategy_techniques.LoadFromBytes([]byte(seeds))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	return reg
}

func TestBuildStrategiesSummary_NilRegistry(t *testing.T) {
	h := &Handlers{}
	got := h.buildStrategiesSummary()
	if got != nil {
		t.Errorf("expected nil from nil registry, got %+v", got)
	}
}

func TestBuildStrategiesSummary_ActiveOnly(t *testing.T) {
	reg := newTestRegistry(t)
	h := &Handlers{StrategyRegistry: reg}
	got := h.buildStrategiesSummary()
	if len(got) != 3 {
		t.Errorf("got %d active strategies, want 3 (alpha+beta+delta)", len(got))
	}
	for i, s := range got {
		if s.Status != "active" {
			t.Errorf("strategy[%d] status = %q, want active", i, s.Status)
		}
	}
}

func TestBuildStrategiesSummary_IncludesFields(t *testing.T) {
	reg := newTestRegistry(t)
	h := &Handlers{StrategyRegistry: reg}
	got := h.buildStrategiesSummary()
	if len(got) == 0 {
		t.Fatal("expected at least one strategy")
	}
	first := got[0]
	if first.ID != "alpha" {
		t.Errorf("ID = %q, want alpha", first.ID)
	}
	if first.Layer != "L1" {
		t.Errorf("Layer = %q, want L1", first.Layer)
	}
}

func TestHandleDecisionChain_NilDependencies(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/decision-chain", nil)
	status, body := h.HandleDecisionChain(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body is %T, want map[string]any", body)
	}
	for _, key := range []string{"events", "sector_heatmap", "recommendations", "exit_alerts", "strategies"} {
		if _, exists := m[key]; !exists {
			t.Errorf("missing key %q in response", key)
		}
	}
	strategies := m["strategies"]
	if s, ok := strategies.([]StrategyFrameSummary); !ok || len(s) != 0 {
		t.Errorf("strategies = %v, want nil or empty", strategies)
	}
}

func TestHandleDecisionChain_WithStrategies(t *testing.T) {
	reg := newTestRegistry(t)
	h := &Handlers{StrategyRegistry: reg}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/decision-chain", nil)
	status, body := h.HandleDecisionChain(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m, ok := body.(map[string]any)
	if !ok {
		t.Fatalf("body is %T", body)
	}
	summaries, ok := m["strategies"].([]StrategyFrameSummary)
	if !ok {
		t.Fatalf("strategies is %T, want []StrategyFrameSummary", m["strategies"])
	}
	if len(summaries) != 3 {
		t.Errorf("strategies count = %d, want 3 (active only)", len(summaries))
	}
}

type stubMacroProvider struct {
	snap marketdata.MacroDataSnapshot
	err  error
}

func (s *stubMacroProvider) Name() string { return "stub" }
func (s *stubMacroProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	if s.err != nil {
		return marketdata.MacroDataSnapshot{}, s.err
	}
	return s.snap, nil
}

func TestHandleDecisionChain_WithMacroProvider(t *testing.T) {
	reg := newTestRegistry(t)
	mockSnap := marketdata.MacroDataSnapshot{
		DXY: mp(104.5, -0.3),
		VIX: mp(18.5, -5.0),
	}
	h := &Handlers{
		StrategyRegistry: reg,
		MacroProvider:    &stubMacroProvider{snap: mockSnap},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/decision-chain", nil)
	status, body := h.HandleDecisionChain(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m, _ := body.(map[string]any)
	events, ok := m["events"].(EventBlock)
	if !ok {
		t.Fatalf("events is %T", m["events"])
	}
	if events.Premarket == nil {
		t.Error("Premarket should not be nil when macro snapshot available")
	}
	indicators, ok := m["core_indicators"].(*CoreIndicators)
	if !ok {
		t.Fatalf("core_indicators is %T", m["core_indicators"])
	}
	if indicators == nil || indicators.DXYpct == nil || *indicators.DXYpct != -0.3 {
		t.Errorf("DXYpct = %v, want -0.3", indicators)
	}
}

type errProvider struct{}

func (e errProvider) Name() string { return "err" }
func (e errProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	return marketdata.MacroDataSnapshot{}, &stubError{"timeout"}
}

type stubError struct{ msg string }

func (e *stubError) Error() string { return e.msg }

func TestHandleDecisionChain_MacroProviderError(t *testing.T) {
	reg := newTestRegistry(t)
	h := &Handlers{
		StrategyRegistry: reg,
		MacroProvider:    errProvider{},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/decision-chain", nil)
	status, body := h.HandleDecisionChain(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 (graceful degradation)", status)
	}
	m, _ := body.(map[string]any)
	events, _ := m["events"].(EventBlock)
	if events.Premarket != nil {
		t.Error("Premarket should be nil when macro fetch fails")
	}
	indicators, _ := m["core_indicators"].(CoreIndicators)
	if indicators != (CoreIndicators{}) {
		t.Errorf("core_indicators should be zero when macro fetch fails, got %+v", indicators)
	}
}

func TestHandleDecisionChain_VariousCombinations(t *testing.T) {
	// Verify handler gracefully handles nil services while producing valid response
	reg := newTestRegistry(t)
	h := &Handlers{StrategyRegistry: reg}
	// IndustrySvc, PipelineSvc, NarrativeEng are nil — handler skips them via nil checks.
	// MacroProvider is nil — handler skips macro fetch.
	// WorkDir is empty — computeExitAlerts returns empty.
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/decision-chain", nil)
	status, body := h.HandleDecisionChain(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m, _ := body.(map[string]any)

	// Events should be empty (no narrative engine, no macro)
	events, _ := m["events"].(EventBlock)
	if len(events.Today) != 0 || len(events.Recent) != 0 {
		t.Errorf("events should be empty with nil narrative engine, got today=%d recent=%d", len(events.Today), len(events.Recent))
	}
	if events.Premarket != nil {
		t.Error("Premarket should be nil without macro provider")
	}

	// Heatmap should be empty (no industry service)
	hm, ok := m["sector_heatmap"]
	if !ok {
		t.Error("missing sector_heatmap")
	} else if list, ok := hm.([]any); ok && len(list) != 0 {
		t.Errorf("heatmap should be empty, got %d entries", len(list))
	}

	// Recommendations should be empty (no pipeline service)
	recs, _ := m["recommendations"].([]RecEntry)
	if len(recs) != 0 {
		t.Errorf("recommendations should be empty, got %d", len(recs))
	}
}

func TestComputeExitAlerts_NoWorkDir(t *testing.T) {
	h := &Handlers{}
	alerts := h.computeExitAlerts()
	if len(alerts) != 0 {
		t.Errorf("expected 0 exit alerts when WorkDir empty, got %d", len(alerts))
	}
}

func TestComputeExitAlerts_NoPositions(t *testing.T) {
	dir := t.TempDir()
	h := &Handlers{WorkDir: dir, LedgerDir: dir}
	alerts := h.computeExitAlerts()
	if len(alerts) != 0 {
		t.Errorf("expected 0 exit alerts with empty dir, got %d", len(alerts))
	}
}

func TestComputeExitAlerts_WithPositions(t *testing.T) {
	workDir := t.TempDir()
	ledgerDir := t.TempDir()

	liveStateDir := filepath.Join(workDir, "data/state/live/state")
	os.MkdirAll(liveStateDir, 0o755)

	portState := map[string]any{"cash": 500000, "available_cash": 400000}
	portData, _ := json.Marshal(portState)
	os.WriteFile(filepath.Join(liveStateDir, "portfolio_state.json"), portData, 0o644)

	// PnlPct (ratio) = UnrealizedPnL / (Qty * AvgCost) → ×100 = percentage points
	// 2330.TW: cost=600, pnl=204 → ratio 0.34 → 34.0% → 強烈建議獲利了結
	// 2454.TW: cost=1200, pnl=132 → ratio 0.11 → 11.0% → 部分獲利了結
	positions := []map[string]any{
		{
			"symbol": "2330.TW", "quantity": 1, "average_cost": 600.0,
			"current_price": 804.0, "market_value": 804, "unrealized_pnl": 204,
		},
		{
			"symbol": "2454.TW", "quantity": 1, "average_cost": 1200.0,
			"current_price": 1332.0, "market_value": 1332, "unrealized_pnl": 132,
		},
	}
	posData, _ := json.Marshal(positions)
	os.WriteFile(filepath.Join(liveStateDir, "positions_current.json"), posData, 0o644)

	h := &Handlers{WorkDir: workDir, LedgerDir: ledgerDir}
	alerts := h.computeExitAlerts()
	if len(alerts) != 2 {
		t.Fatalf("expected 2 exit alerts, got %d", len(alerts))
	}
	if alerts[0].Symbol != "2330.TW" {
		t.Errorf("alerts[0].Symbol = %q, want 2330.TW", alerts[0].Symbol)
	}
	if alerts[0].Suggestion != "強烈建議獲利了結" {
		t.Errorf("alerts[0].Suggestion = %q, want 強烈建議獲利了結 (PnlPct=34)", alerts[0].Suggestion)
	}
	if alerts[1].Symbol != "2454.TW" {
		t.Errorf("alerts[1].Symbol = %q, want 2454.TW", alerts[1].Symbol)
	}
	if alerts[1].Suggestion != "部分獲利了結" {
		t.Errorf("alerts[1].Suggestion = %q, want 部分獲利了結 (PnlPct=11)", alerts[1].Suggestion)
	}
}

func TestComputeExitAlerts_PositionsBelowThreshold(t *testing.T) {
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	liveStateDir := filepath.Join(workDir, "data/state/live/state")
	os.MkdirAll(liveStateDir, 0o755)

	portState := map[string]any{"cash": 100000, "available_cash": 100000}
	portData, _ := json.Marshal(portState)
	os.WriteFile(filepath.Join(liveStateDir, "portfolio_state.json"), portData, 0o644)

	// PnlPct (ratio) = 18/600 = 0.03 → 3.0% < 5.0% → filtered out
	positions := []map[string]any{
		{
			"symbol": "2330.TW", "quantity": 1, "average_cost": 600.0,
			"current_price": 618.0, "market_value": 618, "unrealized_pnl": 18,
		},
	}
	posData, _ := json.Marshal(positions)
	os.WriteFile(filepath.Join(liveStateDir, "positions_current.json"), posData, 0o644)

	h := &Handlers{WorkDir: workDir, LedgerDir: ledgerDir}
	alerts := h.computeExitAlerts()
	if len(alerts) != 0 {
		t.Errorf("expected 0 exit alerts for PnlPct=3.0 below threshold, got %d", len(alerts))
	}
}

func TestComputeExitAlerts_NegativePnl(t *testing.T) {
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	liveStateDir := filepath.Join(workDir, "data/state/live/state")
	os.MkdirAll(liveStateDir, 0o755)

	portState := map[string]any{"cash": 100000}
	portData, _ := json.Marshal(portState)
	os.WriteFile(filepath.Join(liveStateDir, "portfolio_state.json"), portData, 0o644)

	// PnlPct (ratio) = -72/600 = -0.12 → -12.0% → absPnl=12 > 5, <= -10 → 建議評估停損
	positions := []map[string]any{
		{
			"symbol": "2330.TW", "quantity": 1, "average_cost": 600.0,
			"current_price": 528.0, "market_value": 528, "unrealized_pnl": -72,
		},
	}
	posData, _ := json.Marshal(positions)
	os.WriteFile(filepath.Join(liveStateDir, "positions_current.json"), posData, 0o644)

	h := &Handlers{WorkDir: workDir, LedgerDir: ledgerDir}
	alerts := h.computeExitAlerts()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 exit alert, got %d", len(alerts))
	}
	if alerts[0].Suggestion != "建議評估停損" {
		t.Errorf("Suggestion = %q, want 建議評估停損 (PnlPct=-12)", alerts[0].Suggestion)
	}
}

func TestHandleDecisionChain_WithExitAlerts(t *testing.T) {
	workDir := t.TempDir()
	ledgerDir := t.TempDir()
	liveStateDir := filepath.Join(workDir, "data/state/live/state")
	os.MkdirAll(liveStateDir, 0o755)

	portState := map[string]any{"cash": 500000, "available_cash": 400000}
	portData, _ := json.Marshal(portState)
	os.WriteFile(filepath.Join(liveStateDir, "portfolio_state.json"), portData, 0o644)

	// PnlPct (ratio) = 300/600 = 0.5 → 50.0% → 強烈建議獲利了結
	positions := []map[string]any{
		{
			"symbol": "2330.TW", "quantity": 1, "average_cost": 600.0,
			"current_price": 900.0, "market_value": 900, "unrealized_pnl": 300,
		},
	}
	posData, _ := json.Marshal(positions)
	os.WriteFile(filepath.Join(liveStateDir, "positions_current.json"), posData, 0o644)

	reg := newTestRegistry(t)
	h := &Handlers{
		WorkDir:          workDir,
		LedgerDir:        ledgerDir,
		StrategyRegistry: reg,
	}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/decision-chain", nil)
	status, body := h.HandleDecisionChain(req)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	m, _ := body.(map[string]any)
	exitAlerts, ok := m["exit_alerts"].([]ExitAlert)
	if !ok {
		t.Fatalf("exit_alerts is %T", m["exit_alerts"])
	}
	if len(exitAlerts) != 1 {
		t.Errorf("expected 1 exit alert in full handler, got %d", len(exitAlerts))
	}
}

func TestStrategyFrameSummary_JSONTags(t *testing.T) {
	s := StrategyFrameSummary{
		ID: "test-id", Name: "test", Layer: "L3", Summary: "summary text",
		Themes: []string{"tech"}, Direction: "up", Risk: "medium",
		HitRate: 0.75, Status: "active",
		Attribution: []string{"regime_shift"}, AffectedSectors: []string{"semiconductor"},
	}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	str := string(b)
	for _, want := range []string{`"id":"test-id"`, `"layer":"L3"`, `"themes":["tech"]`, `"hit_rate":0.75`, `"affected_sectors"`} {
		if !containsStr(str, want) {
			t.Errorf("JSON missing %q in %s", want, str)
		}
	}
}

func TestExitAlert_JSONTags(t *testing.T) {
	ea := ExitAlert{Symbol: "2330.TW", Name: "台積電", DaysHeld: 10, PnlPct: new(15.0), Suggestion: "部分獲利了結"}
	b, err := json.Marshal(ea)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	str := string(b)
	for _, want := range []string{`"symbol":"2330.TW"`, `"name":"台積電"`, `"pnl_pct":15`} {
		if !containsStr(str, want) {
			t.Errorf("JSON missing %q in %s", want, str)
		}
	}
}

func TestCoreIndicators_JSONTags(t *testing.T) {
	ci := CoreIndicators{
		ForeignCapitalNetTWD: float64Ptr(5000),
		TSMADRpct:            new(2.5),
		NVDApct:              new(1.5),
		DXYpct:               new(-0.3),
	}
	b, err := json.Marshal(ci)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	str := string(b)
	for _, want := range []string{`"foreign_capital_net_twd":5000`, `"tsm_adr_pct":2.5`, `"nvda_pct":1.5`, `"dxy_pct":-0.3`} {
		if !containsStr(str, want) {
			t.Errorf("JSON missing %q in %s", want, str)
		}
	}
}

func TestRegisterRoutes(t *testing.T) {
	h := &Handlers{}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/decision-chain", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code == 0 {
		t.Error("route /api/dashboard/decision-chain not registered")
	}

	reqPost := httptest.NewRequest(http.MethodPost, "/api/dashboard/decision-chain", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, reqPost)
	if w2.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want %d", w2.Code, http.StatusMethodNotAllowed)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
