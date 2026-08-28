// Tests for StockpickerWinrateExecutor (PR 2d-executor).
//
// Coverage contract (from the task): Supports match/non-match, recommend
// happy path (mock winrate + flow pass), eligible gate reject, win-rate
// gate rejects, flow gate reject, DB error silent, nil dependencies →
// default behavior (real read-only ledger + real flow file).
package orchestrator

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// ── mocks ─────────────────────────────────────────────────────────────

// mockWinRateStore is a scriptable WinRateStoreReader.
type mockWinRateStore struct {
	summary stockpicker.StockWinRateSummary
	found   bool
	err     error
}

func (m *mockWinRateStore) LoadWinRate(ctx context.Context, symbol, source, window string) (stockpicker.StockWinRateSummary, bool, error) {
	if m.err != nil {
		return stockpicker.StockWinRateSummary{}, false, m.err
	}
	return m.summary, m.found, nil
}

// mockFlowSource is a scriptable FlowSource.
type mockFlowSource struct {
	net float64
	ok  bool
}

func (m mockFlowSource) LatestForeignNet(symbol string) (float64, bool) {
	return m.net, m.ok
}

// ── fixtures ──────────────────────────────────────────────────────────

func stockpickerWinrateAgent() domain.AgentSpec {
	return domain.AgentSpec{
		ID:      "stockpicker-winrate-01",
		Name:    "個股勝率選股",
		Layer:   domain.LayerStyle,
		Skill:   StockpickerWinrateSkill,
		Enabled: true,
	}
}

func stockpickerWinrateQuote() domain.Quote {
	return domain.Quote{Symbol: "2330.TW", Last: 500, IsTradable: true}
}

func eligibleWinRateSummary() stockpicker.StockWinRateSummary {
	return stockpicker.StockWinRateSummary{
		Symbol:            "2330",
		Source:            "stockpicker-foreign-3d-net-buy",
		Window:            "120d",
		Observations:      40,
		Hits:              26,
		WinRate:           0.65,
		WilsonLower:       0.50,
		WilsonUpper:       0.78,
		Confidence:        0.95,
		CalibrationStatus: stockpicker.CalibrationEligible,
		NetCostRate:       0.00585,
		AvgForwardReturn:  0.015,
		UpdatedAt:         "2026-08-27T12:00:00Z",
	}
}

// testFlowGateway returns a hermetic gateway with the documented foreign
// threshold (min_abs_net = 0.1 億股) — independent of the config singleton.
func testFlowGateway() *stockpicker.FlowGateway {
	return stockpicker.NewFlowGateway(stockpicker.FlowGatewayParameters{
		Foreign:                  stockpicker.ForeignThreshold{MinAbsNet: 0.1},
		FailClosedWhenAllMissing: true,
	})
}

// ── Supports ──────────────────────────────────────────────────────────

func TestStockpickerWinrateSupports(t *testing.T) {
	e := StockpickerWinrateExecutor{}
	agent := stockpickerWinrateAgent()

	if !e.Supports(agent) {
		t.Fatal("Supports(eligible spec) = false, want true")
	}

	// Enabled-ness is collectRecommendations' job — an executor matches by
	// skill alone so the registry guard can resolve enabled agents.
	disabled := agent
	disabled.Enabled = false
	if !e.Supports(disabled) {
		t.Fatal("Supports(disabled spec) = false, want true (enabled is the collector's concern)")
	}

	for _, skill := range []string{"semiconductor_desk", "etf_rotation_desk", "stockpicker_flow", ""} {
		other := agent
		other.Skill = skill
		if e.Supports(other) {
			t.Errorf("Supports(skill %q) = true, want false", skill)
		}
	}
}

// ── Recommend happy path ──────────────────────────────────────────────

func TestStockpickerWinrateRecommendHappyPath(t *testing.T) {
	e := StockpickerWinrateExecutor{
		WinRateStore: &mockWinRateStore{summary: eligibleWinRateSummary(), found: true},
		FlowSource:   mockFlowSource{net: 50000, ok: true}, // 0.5 億股 > 0.1
		Gateway:      testFlowGateway(),
	}

	rec, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil)
	if !ok {
		t.Fatal("Recommend = (_, false), want (rec, true)")
	}
	if rec.Agent != "stockpicker-winrate-01" {
		t.Errorf("rec.Agent = %q, want stockpicker-winrate-01", rec.Agent)
	}
	if rec.Skill != StockpickerWinrateSkill {
		t.Errorf("rec.Skill = %q, want %q", rec.Skill, StockpickerWinrateSkill)
	}
	if rec.Symbol != "2330.TW" {
		t.Errorf("rec.Symbol = %q, want 2330.TW", rec.Symbol)
	}
	if rec.Side != domain.SideBuy {
		t.Errorf("rec.Side = %q, want BUY", rec.Side)
	}
	// 55 base + 10 (win_rate 0.65 >= 0.60) + 10 (wilson_lower 0.50 >= 0.50),
	// no large-sample bonus (40 < 2×30).
	if rec.Conviction != 75 {
		t.Errorf("rec.Conviction = %d, want 75", rec.Conviction)
	}
	for _, want := range []string{"win_rate=0.650", "observations=40", "wilson_lower=0.500", "外資層通過"} {
		if !strings.Contains(rec.Reason, want) {
			t.Errorf("rec.Reason = %q, want substring %q", rec.Reason, want)
		}
	}
}

// ── Gate rejects ──────────────────────────────────────────────────────

func TestStockpickerWinrateRecommendCalibrationGate(t *testing.T) {
	for _, status := range []stockpicker.CalibrationStatus{
		stockpicker.CalibrationCalibrating,
		stockpicker.CalibrationDegraded,
	} {
		s := eligibleWinRateSummary()
		s.CalibrationStatus = status
		e := StockpickerWinrateExecutor{
			WinRateStore: &mockWinRateStore{summary: s, found: true},
			FlowSource:   mockFlowSource{net: 50000, ok: true},
			Gateway:      testFlowGateway(),
		}
		if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
			t.Errorf("Recommend with calibration_status %q = true, want false (observation-only)", status)
		}
	}
}

func TestStockpickerWinrateRecommendWinRateGates(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*stockpicker.StockWinRateSummary)
		wantReason string
	}{
		{name: "too few observations", mutate: func(s *stockpicker.StockWinRateSummary) { s.Observations = 29 }, wantReason: "observations"},
		{name: "win rate below threshold", mutate: func(s *stockpicker.StockWinRateSummary) { s.WinRate = 0.54 }, wantReason: "win_rate"},
		{name: "wilson lower below threshold", mutate: func(s *stockpicker.StockWinRateSummary) { s.WilsonLower = 0.44 }, wantReason: "wilson_lower"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := eligibleWinRateSummary()
			tc.mutate(&s)
			e := StockpickerWinrateExecutor{
				WinRateStore: &mockWinRateStore{summary: s, found: true},
				FlowSource:   mockFlowSource{net: 50000, ok: true},
				Gateway:      testFlowGateway(),
			}
			if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
				t.Errorf("Recommend(%s) = true, want false", tc.name)
			}
		})
	}
}

// TestStockpickerWinrateRecommendBoundaries pins the >=/> semantics at the
// exact thresholds: observations=30, win_rate=0.55, wilson_lower=0.45 all
// pass (>=), while foreign flow = 10000 千股 = exactly 0.1 億股 fails
// (validator uses strict >).
func TestStockpickerWinrateRecommendBoundaries(t *testing.T) {
	base := eligibleWinRateSummary()
	cases := []struct {
		name   string
		mutate func(*stockpicker.StockWinRateSummary)
		wantOK bool
	}{
		{name: "observations exactly 30 passes", mutate: func(s *stockpicker.StockWinRateSummary) { s.Observations = 30 }, wantOK: true},
		{name: "win_rate exactly 0.55 passes", mutate: func(s *stockpicker.StockWinRateSummary) { s.WinRate = 0.55 }, wantOK: true},
		{name: "wilson_lower exactly 0.45 passes", mutate: func(s *stockpicker.StockWinRateSummary) { s.WilsonLower = 0.45 }, wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.mutate(&s)
			e := StockpickerWinrateExecutor{
				WinRateStore: &mockWinRateStore{summary: s, found: true},
				FlowSource:   mockFlowSource{net: 50000, ok: true},
				Gateway:      testFlowGateway(),
			}
			_, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil)
			if ok != tc.wantOK {
				t.Errorf("Recommend = ok=%v, want %v", ok, tc.wantOK)
			}
		})
	}

	// Foreign flow exactly at the gate: 10000 千股 = 0.1 億股. The foreign
	// layer uses strict > (abs(億股) > min_abs_net), so exactly 0.1 fails.
	t.Run("flow exactly 0.1 億股 fails (strict >)", func(t *testing.T) {
		e := StockpickerWinrateExecutor{
			WinRateStore: &mockWinRateStore{summary: eligibleWinRateSummary(), found: true},
			FlowSource:   mockFlowSource{net: 10000, ok: true},
			Gateway:      testFlowGateway(),
		}
		if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
			t.Fatal("Recommend with flow exactly 0.1 億股 = true, want false (strict >)")
		}
	})
	// Just above the gate passes: 10001 千股 = 0.10001 億股 > 0.1.
	t.Run("flow just above 0.1 億股 passes", func(t *testing.T) {
		e := StockpickerWinrateExecutor{
			WinRateStore: &mockWinRateStore{summary: eligibleWinRateSummary(), found: true},
			FlowSource:   mockFlowSource{net: 10001, ok: true},
			Gateway:      testFlowGateway(),
		}
		if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); !ok {
			t.Fatal("Recommend with flow just above 0.1 億股 = false, want true")
		}
	})
}

func TestStockpickerWinrateRecommendFlowGateReject(t *testing.T) {
	// 500 千股 = 0.005 億股 ≤ min_abs_net 0.1 → foreign layer fails.
	e := StockpickerWinrateExecutor{
		WinRateStore: &mockWinRateStore{summary: eligibleWinRateSummary(), found: true},
		FlowSource:   mockFlowSource{net: 500, ok: true},
		Gateway:      testFlowGateway(),
	}
	if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
		t.Fatal("Recommend with sub-threshold flow = true, want false (flow gate reject)")
	}
}

func TestStockpickerWinrateRecommendFlowMissing(t *testing.T) {
	// Missing flow data → fail closed (不誤殺也不亂推).
	e := StockpickerWinrateExecutor{
		WinRateStore: &mockWinRateStore{summary: eligibleWinRateSummary(), found: true},
		FlowSource:   mockFlowSource{net: 0, ok: false},
		Gateway:      testFlowGateway(),
	}
	if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
		t.Fatal("Recommend with missing flow = true, want false (fail closed)")
	}
}

func TestStockpickerWinrateRecommendNotFound(t *testing.T) {
	e := StockpickerWinrateExecutor{
		WinRateStore: &mockWinRateStore{summary: stockpicker.StockWinRateSummary{}, found: false},
		FlowSource:   mockFlowSource{net: 50000, ok: true},
		Gateway:      testFlowGateway(),
	}
	if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
		t.Fatal("Recommend with no stored win rate = true, want false")
	}
}

func TestStockpickerWinrateRecommendDBError(t *testing.T) {
	e := StockpickerWinrateExecutor{
		WinRateStore: &mockWinRateStore{err: context.DeadlineExceeded},
		FlowSource:   mockFlowSource{net: 50000, ok: true},
		Gateway:      testFlowGateway(),
	}
	if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
		t.Fatal("Recommend with DB error = true, want false (silent failure)")
	}
}

func TestStockpickerWinrateRecommendUnsupportedSkill(t *testing.T) {
	agent := stockpickerWinrateAgent()
	agent.Skill = "semiconductor_desk"
	e := StockpickerWinrateExecutor{
		WinRateStore: &mockWinRateStore{summary: eligibleWinRateSummary(), found: true},
		FlowSource:   mockFlowSource{net: 50000, ok: true},
		Gateway:      testFlowGateway(),
	}
	if _, ok := e.Recommend(agent, stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
		t.Fatal("Recommend with non-matching skill = true, want false")
	}
}

// ── nil dependencies → production defaults ────────────────────────────

// TestStockpickerWinrateNilDependenciesDefaultBehavior wires nothing but
// WorkDir and exercises the whole default path end-to-end: the read-only
// ledger open (default path resolution → data/state/atlas.db), the default
// stocktools.OpenWinRateDB opener, the default file FlowSource, the
// default config-backed FlowGateway, and the default thresholds.
func TestStockpickerWinrateNilDependenciesDefaultBehavior(t *testing.T) {
	tmp := t.TempDir()
	writeStockpickerLedger(t, tmp)
	writeStockpickerFlowFile(t, tmp)

	// Hermetic env: the executor's default path resolution must not pick up
	// a developer's ATLAS_MCP_STOCKPICKER_DB / ATLAS_WORK_DIR.
	t.Setenv("ATLAS_MCP_STOCKPICKER_DB", "")
	t.Setenv("ATLAS_WORK_DIR", "")

	e := StockpickerWinrateExecutor{WorkDir: tmp}
	rec, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil)
	if !ok {
		t.Fatal("Recommend (nil deps, real ledger + flow file) = false, want true")
	}
	if rec.Symbol != "2330.TW" || rec.Side != domain.SideBuy {
		t.Fatalf("rec = %+v, want BUY 2330.TW", rec)
	}
	if rec.Conviction != 75 {
		t.Errorf("rec.Conviction = %d, want 75 (55 base + 10 + 10)", rec.Conviction)
	}
	for _, want := range []string{"win_rate=0.650", "observations=40", "wilson_lower=0.500", "外資層通過"} {
		if !strings.Contains(rec.Reason, want) {
			t.Errorf("rec.Reason = %q, want substring %q", rec.Reason, want)
		}
	}
}

// TestStockpickerWinrateNilDependenciesMissingLedger covers the silent
// failure contract through the REAL default opener: no ledger file on
// disk → (zero, false), no panic.
func TestStockpickerWinrateNilDependenciesMissingLedger(t *testing.T) {
	tmp := t.TempDir() // no data/state/atlas.db
	t.Setenv("ATLAS_MCP_STOCKPICKER_DB", "")
	t.Setenv("ATLAS_WORK_DIR", "")

	e := StockpickerWinrateExecutor{WorkDir: tmp}
	if _, ok := e.Recommend(stockpickerWinrateAgent(), stockpickerWinrateQuote(), "", domain.Regime(""), nil); ok {
		t.Fatal("Recommend with missing ledger = true, want false (silent DB failure)")
	}
}

// ── helpers ───────────────────────────────────────────────────────────

// writeStockpickerLedger creates a real read-only-able ledger at
// <tmp>/data/state/atlas.db with one eligible stock_win_rate row.
func writeStockpickerLedger(t *testing.T, tmp string) {
	t.Helper()
	dbPath := filepath.Join(tmp, "data", "state", "atlas.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir ledger dir: %v", err)
	}
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	if err := stockpicker.SaveWinRate(context.Background(), db, eligibleWinRateSummary()); err != nil {
		t.Fatalf("save win rate: %v", err)
	}
}

// writeStockpickerFlowFile creates <tmp>/data/state/stock_flows/2330.json
// with two ascending-dated flow points; the latest (50000 千股 = 0.5 億股)
// passes the default foreign threshold.
func writeStockpickerFlowFile(t *testing.T, tmp string) {
	t.Helper()
	dir := filepath.Join(tmp, "data", "state", "stock_flows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir flows dir: %v", err)
	}
	payload, err := json.Marshal(flowFile{
		Symbol: "2330",
		Flows: []stockpicker.FlowPoint{
			{Date: "2026-08-25", ForeignNet: 30000},
			{Date: "2026-08-26", ForeignNet: 50000},
		},
	})
	if err != nil {
		t.Fatalf("marshal flow file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "2330.json"), payload, 0o644); err != nil {
		t.Fatalf("write flow file: %v", err)
	}
}
