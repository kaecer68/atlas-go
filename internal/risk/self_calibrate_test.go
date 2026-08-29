package risk

import (
	"context"
	"strings"
	"sync"
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
	for i := range 100 {
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

func TestReplayWithThresholds_UsesNotionalNotReturn(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	cfg.Risk.MaxPositionSize.Value = 0.10
	cfg.Risk.MaxDailyLossPct.Value = 0.03

	results := []replayResult{
		{OrderPrice: 1000, Quantity: 1000, ForwardReturn: -0.05},
		{OrderPrice: 100, Quantity: 10, ForwardReturn: -0.05},
	}
	adjusted := replayWithThresholds(results, cfg)

	if !adjusted[0].WouldHaveBlocked {
		t.Fatal("expected notional=1M blocked at 10%% cap on 3M ref")
	}
	if adjusted[1].WouldHaveBlocked {
		t.Fatal("expected notional=1000 NOT blocked at 10%% cap on 3M ref")
	}
}

func TestApplyCalibrationChange_RecordsSetParameterError(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	ie := config.NewInferenceEngine(cfg)
	report := &CalibrationReport{}

	applyCalibrationChange(ie, "unknown_param_xyz", 0.5, report)

	if len(report.Errors) != 1 {
		t.Fatalf("expected 1 error recorded, got %d", len(report.Errors))
	}
	if !strings.Contains(report.Errors[0], "unknown_param_xyz") {
		t.Fatalf("expected error to mention param name, got: %s", report.Errors[0])
	}
}

func TestApplyCalibrationChange_NoErrorOnSuccess(t *testing.T) {
	cfg := config.DefaultParametersConfig()
	ie := config.NewInferenceEngine(cfg)
	report := &CalibrationReport{}

	applyCalibrationChange(ie, "risk_max_position_size", 0.15, report)

	if len(report.Errors) != 0 {
		t.Fatalf("expected no errors on valid param, got %v", report.Errors)
	}
}

func TestDetectOscillation_AlternatingReturnsTrue(t *testing.T) {
	if !detectOscillation([]float64{0.05, 0.10, 0.06, 0.11}) {
		t.Fatal("expected oscillation detected for alternating up/down sequence")
	}
}

func TestDetectOscillation_MonotonicReturnsFalse(t *testing.T) {
	if detectOscillation([]float64{0.05, 0.06, 0.07, 0.08}) {
		t.Fatal("expected no oscillation for monotonic increasing sequence")
	}
	if detectOscillation([]float64{0.10, 0.09, 0.08, 0.07}) {
		t.Fatal("expected no oscillation for monotonic decreasing sequence")
	}
}

func TestDetectOscillation_ShortHistoryReturnsFalse(t *testing.T) {
	if detectOscillation([]float64{0.05}) {
		t.Fatal("expected no oscillation with single value")
	}
	if detectOscillation(nil) {
		t.Fatal("expected no oscillation with empty history")
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

// TestSelfCalibrateBounds verifies the post-optimize unit guard rejects values
// that fall outside the documented [val*0.3, val*3.0] bounds. The bug class is
// the optimizer returning near-zero values (e.g. 0.000181, 0.000906) that pass
// through to persistence and corrupt risk thresholds.
func TestSelfCalibrateBounds(t *testing.T) {
	tests := []struct {
		name        string
		current     float64
		proposed    float64
		wantAccept  bool
		description string
	}{
		{
			name:        "accepts value within bounds",
			current:     0.12,
			proposed:    0.10,
			wantAccept:  true,
			description: "0.10 is within [0.036, 0.36]",
		},
		{
			name:        "accepts upper bound",
			current:     0.12,
			proposed:    0.35,
			wantAccept:  true,
			description: "0.35 is within [0.036, 0.36]",
		},
		{
			name:        "accepts lower bound",
			current:     0.12,
			proposed:    0.04,
			wantAccept:  true,
			description: "0.04 is within [0.036, 0.36]",
		},
		{
			name:        "rejects value below lower bound (current bug value)",
			current:     0.12,
			proposed:    0.0009069926399999993,
			wantAccept:  false,
			description: "0.000906 is far below [0.036, 0.36] — 20x below lower bound",
		},
		{
			name:        "rejects daily loss bug value",
			current:     0.03,
			proposed:    0.00018139852799999994,
			wantAccept:  false,
			description: "0.000181 is far below [0.009, 0.09] — 50x below lower bound",
		},
		{
			name:        "rejects value above upper bound",
			current:     0.10,
			proposed:    0.50,
			wantAccept:  false,
			description: "0.50 is above [0.03, 0.30]",
		},
		{
			name:        "accepts identical value",
			current:     0.05,
			proposed:    0.05,
			wantAccept:  true,
			description: "no-op should pass",
		},
		{
			name:        "handles zero current gracefully",
			current:     0,
			proposed:    0.05,
			wantAccept:  true,
			description: "zero current falls back to absolute check, accepts sane value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accept := validateCalibrationBounds(tt.current, tt.proposed)
			if accept != tt.wantAccept {
				t.Errorf("validateCalibrationBounds(current=%v, proposed=%v) = %v, want %v (%s)",
					tt.current, tt.proposed, accept, tt.wantAccept, tt.description)
			}
		})
	}
}

// TestSelfCalibrateBoundsCurrentBuggyValues is the regression test for the
// exact bug class observed in production: max_position_size=0.000906 and
// max_daily_loss_pct=0.000181 surviving the optimizer and being persisted.
func TestSelfCalibrateBoundsCurrentBuggyValues(t *testing.T) {
	buggyValues := []struct {
		name  string
		value float64
	}{
		{"max_position_size", 0.0009069926399999993},
		{"max_daily_loss_pct", 0.00018139852799999994},
	}

	// These are the historical "sane" values that should have been the
	// floor for the bounds check: max_position_size=0.12, max_daily_loss_pct=0.03.
	saneValues := map[string]float64{
		"max_position_size":  0.12,
		"max_daily_loss_pct": 0.03,
	}

	for _, v := range buggyValues {
		current := saneValues[v.name]
		if validateCalibrationBounds(current, v.value) {
			t.Errorf("BUG: %s proposed value %v accepted, but it falls 20x+ outside [val*0.3, val*3.0]",
				v.name, v.value)
		}
	}
}

// TestSelfCalibrateBounds_AbsoluteFloorPreventsDrift covers the production
// drift: even when `current` has already shrunk to an absurd value (e.g.
// max_position_size=9.14e-6 from a prior buggy round), a proposed value that
// stays below the absolute floor (0.005) must be rejected — relative bounds
// alone (current*0.3 ~ *3.0) would otherwise accept the drift forever.
func TestSelfCalibrateBounds_AbsoluteFloorPreventsDrift(t *testing.T) {
	cases := []struct {
		name     string
		current  float64
		proposed float64
		want     bool
	}{
		{"drifted current, smaller proposed rejected", 9.14e-6, 5.48e-6, false},
		{"drifted current, still pathological rejected", 9.14e-6, 0.001, false},
		{"drifted current, 0.006 still below floor rejected", 9.14e-6, 0.006, false},
		{"sane current, sane proposed accepted", 0.15, 0.10, true},
		{"sane current, above floor within bounds accepted", 0.15, 0.30, true},
		{"sane current, below relative lower rejected", 0.15, 0.04, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateCalibrationBounds(c.current, c.proposed)
			if got != c.want {
				t.Errorf("validateCalibrationBounds(current=%v, proposed=%v) = %v, want %v",
					c.current, c.proposed, got, c.want)
			}
		})
	}
}

func TestSelfCalibrate_Concurrent(t *testing.T) {
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

	const N = 8
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for range N {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := g.SelfCalibrate(context.Background(), provider, 5); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent SelfCalibrate error: %v", err)
	}
}
