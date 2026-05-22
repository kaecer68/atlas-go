package risk

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestBuildPortfolioState(t *testing.T) {
	s := SessionOutcome{
		PortfolioValue:  2_000_000,
		EndingCash:      500_000,
		SectorExposures: map[string]float64{"semiconductor": 600_000},
		PositionValues:  map[string]float64{"2330": 200_000},
	}
	pf := buildPortfolioState(s)
	if pf.TotalValue != 2_000_000 {
		t.Errorf("expected 2M, got %.0f", pf.TotalValue)
	}
	if pf.Cash != 500_000 {
		t.Errorf("expected 500k cash, got %.0f", pf.Cash)
	}
	if pf.SectorExposure["semiconductor"] != 600_000 {
		t.Errorf("expected semiconductor 600k, got %.0f", pf.SectorExposure["semiconductor"])
	}
}

func TestBuildPortfolioStateZeroValue(t *testing.T) {
	s := SessionOutcome{}
	pf := buildPortfolioState(s)
	if pf.TotalValue <= 0 {
		t.Errorf("expected fallback total value, got %.0f", pf.TotalValue)
	}
}

func TestApplyOrderToState(t *testing.T) {
	pf := PortfolioState{
		TotalValue:     1_000_000,
		Cash:           200_000,
		Positions:      map[string]float64{"2330": 100_000},
		SectorExposure: map[string]float64{"semiconductor": 300_000},
	}
	o := OrderIntent{Symbol: "2330", Sector: "semiconductor", Notional: 50_000}
	pf = applyOrderToState(pf, o)
	if pf.Cash != 150_000 {
		t.Errorf("cash should drop to 150k, got %.0f", pf.Cash)
	}
	if pf.Positions["2330"] != 150_000 {
		t.Errorf("position should grow to 150k, got %.0f", pf.Positions["2330"])
	}
	if pf.SectorExposure["semiconductor"] != 350_000 {
		t.Errorf("sector exposure should grow to 350k, got %.0f", pf.SectorExposure["semiconductor"])
	}
}

func TestScoreThresholdsEmpty(t *testing.T) {
	s := scoreThresholds(nil)
	if s != 0 {
		t.Errorf("expected 0 for empty, got %.4f", s)
	}
}

func TestScoreThresholdsAllCorrect(t *testing.T) {
	results := []replayResult{
		{ForwardReturn: -0.05, WouldHaveBlocked: true},
		{ForwardReturn: 0.03, WouldHaveBlocked: false},
	}
	s := scoreThresholds(results)
	if s <= 0 {
		t.Errorf("expected positive score for correct decisions, got %.4f", s)
	}
}

func TestScoreThresholdsAllWrong(t *testing.T) {
	results := []replayResult{
		{ForwardReturn: -0.05, WouldHaveBlocked: false},
		{ForwardReturn: 0.03, WouldHaveBlocked: true},
	}
	s := scoreThresholds(results)
	if s >= 0 {
		t.Errorf("expected negative score for wrong decisions, got %.4f", s)
	}
}

func TestScoreThresholdsMixed(t *testing.T) {
	results := []replayResult{
		{ForwardReturn: -0.10, WouldHaveBlocked: true},
		{ForwardReturn: -0.08, WouldHaveBlocked: false},
		{ForwardReturn: 0.05, WouldHaveBlocked: false},
		{ForwardReturn: 0.02, WouldHaveBlocked: true},
	}
	s := scoreThresholds(results)
	if s == 0 {
		t.Errorf("expected non-zero score, got %.4f", s)
	}
}

func TestScoreThresholdsHighInterceptPenalty(t *testing.T) {
	results := make([]replayResult, 100)
	for i := 0; i < 100; i++ {
		results[i] = replayResult{
			ForwardReturn:    -0.01,
			WouldHaveBlocked: true,
		}
	}
	s := scoreThresholds(results)
	if s > -1 {
		t.Errorf("expected <-1 penalty for >50%% intercept, got %.4f", s)
	}
}

func TestReplayWithThresholds(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	cfg.Risk.MaxPositionSize.Value = 0.10
	cfg.Risk.MaxDailyLossPct.Value = 0.03

	results := []replayResult{
		{ForwardReturn: -0.05},
		{ForwardReturn: 0.02},
	}
	adjusted := replayWithThresholds(results, cfg)
	if len(adjusted) != 2 {
		t.Fatalf("expected 2 results, got %d", len(adjusted))
	}
}

func TestClassifyDelta(t *testing.T) {
	if c := classifyDelta(10, 50); c != "high" {
		t.Errorf("expected high for 10%% delta with 50 sessions, got %s", c)
	}
	if c := classifyDelta(3, 20); c != "medium" {
		t.Errorf("expected medium for 3%% delta with 20 sessions, got %s", c)
	}
	if c := classifyDelta(1, 5); c != "low" {
		t.Errorf("expected low for 1%% delta with 5 sessions, got %s", c)
	}
}

type mockCalibrationProvider struct {
	Sessions []SessionOutcome
	Err      error
}

func (m *mockCalibrationProvider) RecentSessions(_ context.Context, _ int) ([]SessionOutcome, error) {
	return m.Sessions, m.Err
}

func TestSelfCalibrateEmpty(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	provider := &mockCalibrationProvider{Sessions: nil}
	_, err := g.SelfCalibrate(context.Background(), provider, 10)
	if err == nil {
		t.Fatal("expected error for no sessions")
	}
}

func TestSelfCalibrateWithOrders(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	now := time.Now()
	outcomes := []SessionOutcome{
		{
			SessionID:      "session-20260501-daily",
			PortfolioValue: 1_000_000,
			EndingCash:     200_000,
			Orders: []HistoricOrder{
				{Symbol: "2330", Side: "buy", Notional: 50_000, ForwardReturn: 0.03, Hit: true},
				{Symbol: "2303", Side: "buy", Notional: 30_000, ForwardReturn: -0.08, Hit: false},
				{Symbol: "2317", Side: "buy", Notional: 40_000, ForwardReturn: -0.12, Hit: false},
			},
			Timestamp: now,
		},
	}
	provider := &mockCalibrationProvider{Sessions: outcomes}
	report, err := g.SelfCalibrate(context.Background(), provider, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Evaluated == 0 {
		t.Fatal("expected at least 1 evaluated order")
	}
	if report.SessionSpan == "" {
		t.Fatal("expected non-empty session span")
	}
	_ = g.LastCalibrationReport()
}

func TestLastCalibrationReportNil(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	r := g.LastCalibrationReport()
	if r != nil {
		t.Fatal("expected nil when no calibration run")
	}
}

func TestSetLastCalibration(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	report := &CalibrationReport{
		Verdict:   "stable",
		Summary:   "thresholds optimal",
		Evaluated: 50,
	}
	g.SetLastCalibration(report)
	r := g.LastCalibrationReport()
	if r == nil {
		t.Fatal("expected non-nil report")
	}
	if r.Verdict != "stable" {
		t.Errorf("expected stable, got %s", r.Verdict)
	}
	// Verify immutability of returned copy
	r.Verdict = "modified"
	if g.LastCalibrationReport().Verdict != "stable" {
		t.Error("SetLastCalibration should return a copy, not the original")
	}
}
