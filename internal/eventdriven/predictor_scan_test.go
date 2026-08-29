package eventdriven

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- scanSeverityWeight ----------------------------------------------

func TestScanSeverityWeight(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{
		{"critical", 1.0},
		{"high", 0.7},
		{"medium", 0.4},
		{"low", 0.1},
		{"CRITICAL", 1.0}, // case-insensitive
		{"  high  ", 0.7}, // whitespace-tolerant
		{"unknown", 0.0},
		{"", 0.0},
	}
	for _, tc := range tests {
		got := scanSeverityWeight(tc.in)
		if got != tc.want {
			t.Errorf("scanSeverityWeight(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// --- scanThemeDirection ----------------------------------------------

func TestScanThemeDirection(t *testing.T) {
	tests := []struct {
		theme string
		want  float64
	}{
		// Bullish keywords (English)
		{"ai_rally", 1.0},
		{"semiconductor_growth", 1.0},
		{"tech_bull_phase", 1.0},
		{"range_breakout", 1.0},
		{"credit_expansion", 1.0},
		// Bullish keywords (Chinese)
		{"半導體成長", 1.0},
		{"指數突破", 1.0},
		{"經濟復甦", 1.0},
		// Bearish keywords (English)
		{"earnings_decline", -1.0},
		{"margin_fall", -1.0},
		{"fx_breakdown", -1.0},
		{"credit_contraction", -1.0},
		{"foreign_capital_flight", -1.0},
		// Bearish keywords (Chinese)
		{"景氣衰退", -1.0},
		{"資金外逃", -1.0},
		// Neutral / unknown themes
		{"earnings_release", 0.0},
		{"market_open", 0.0},
		{"data_update", 0.0},
		{"", 0.0},
		// Case insensitive
		{"AI_RALLY", 1.0},
		{"BULL_MARKET", 1.0},
	}
	for _, tc := range tests {
		got := scanThemeDirection(tc.theme)
		if got != tc.want {
			t.Errorf("scanThemeDirection(%q) = %v, want %v", tc.theme, got, tc.want)
		}
	}
}

// When both bullish and bearish keywords are present in the same theme,
// the bullish table is checked first and wins (this is intentional —
// allows themes like "growth_after_decline" to be classified bullish).
func TestScanThemeDirection_BullishPrecedence(t *testing.T) {
	got := scanThemeDirection("growth_after_decline")
	if got != 1.0 {
		t.Errorf("expected bullish precedence, got %v", got)
	}
}

// --- applyScanThemes --------------------------------------------------

// fakeScanStore implements DetectorScanStore for tests.
type fakeScanStore struct {
	scans []ScanResult
	err   error
}

func (f *fakeScanStore) LoadRecentScans(_ context.Context, _ int) ([]ScanResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.scans, nil
}

func mkTime(base time.Time, h int) time.Time { return base.Add(time.Duration(h) * time.Hour) }

func TestApplyScanThemes_NilStore(t *testing.T) {
	p := &Predictor{}
	tilt, drivers := p.applyScanThemes(context.Background(), time.Now())
	if tilt != 0 || drivers != nil {
		t.Errorf("nil store should return (0, nil), got (%v, %v)", tilt, drivers)
	}
}

func TestApplyScanThemes_StoreError(t *testing.T) {
	p := &Predictor{
		scanStore: &fakeScanStore{err: errors.New("db locked")},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), time.Now())
	if tilt != 0 || drivers != nil {
		t.Errorf("error should be swallowed, got (%v, %v)", tilt, drivers)
	}
}

func TestApplyScanThemes_EmptyStore(t *testing.T) {
	p := &Predictor{
		scanStore: &fakeScanStore{scans: nil},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), time.Now())
	if tilt != 0 || drivers != nil {
		t.Errorf("empty store should return (0, nil), got (%v, %v)", tilt, drivers)
	}
}

func TestApplyScanThemes_BullishScan(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	p := &Predictor{
		scanStore: &fakeScanStore{scans: []ScanResult{
			{Theme: "ai_rally", Severity: "critical", Confidence: 0.9, DetectedAt: mkTime(now, -2)},
		}},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), now)
	if tilt <= 0 {
		t.Errorf("expected positive tilt for bullish critical scan, got %v", tilt)
	}
	if len(drivers) != 1 || drivers[0] != "ai_rally" {
		t.Errorf("expected drivers=[ai_rally], got %v", drivers)
	}
	// Max tilt is bounded to scanThemeTiltDampener (0.5).
	if tilt > scanThemeTiltDampener+1e-9 {
		t.Errorf("tilt must not exceed dampener %v, got %v", scanThemeTiltDampener, tilt)
	}
}

func TestApplyScanThemes_BearishScan(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	p := &Predictor{
		scanStore: &fakeScanStore{scans: []ScanResult{
			{Theme: "foreign_capital_flight", Severity: "critical", Confidence: 0.85, DetectedAt: mkTime(now, -5)},
		}},
	}
	tilt, _ := p.applyScanThemes(context.Background(), now)
	if tilt >= 0 {
		t.Errorf("expected negative tilt for bearish critical scan, got %v", tilt)
	}
	if tilt < -scanThemeTiltDampener-1e-9 {
		t.Errorf("tilt must not exceed -dampener %v, got %v", scanThemeTiltDampener, tilt)
	}
}

func TestApplyScanThemes_FiltersOutOfWindow(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	// 48h old scan → outside 24h lookback.
	p := &Predictor{
		scanStore: &fakeScanStore{scans: []ScanResult{
			{Theme: "ai_rally", Severity: "critical", Confidence: 0.9, DetectedAt: mkTime(now, -48)},
		}},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), now)
	if tilt != 0 || drivers != nil {
		t.Errorf("old scan must be filtered, got (%v, %v)", tilt, drivers)
	}
}

func TestApplyScanThemes_FiltersFutureScan(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	// 48h in the future → outside upper bound (day+24h).
	p := &Predictor{
		scanStore: &fakeScanStore{scans: []ScanResult{
			{Theme: "ai_rally", Severity: "critical", Confidence: 0.9, DetectedAt: mkTime(now, 48)},
		}},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), now)
	if tilt != 0 || drivers != nil {
		t.Errorf("future scan must be filtered, got (%v, %v)", tilt, drivers)
	}
}

func TestApplyScanThemes_FiltersLowConfidence(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	p := &Predictor{
		scanStore: &fakeScanStore{scans: []ScanResult{
			{Theme: "ai_rally", Severity: "critical", Confidence: 0.3, DetectedAt: mkTime(now, -2)},
		}},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), now)
	if tilt != 0 || drivers != nil {
		t.Errorf("low confidence must be filtered, got (%v, %v)", tilt, drivers)
	}
}

func TestApplyScanThemes_FiltersUnknownSeverity(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	p := &Predictor{
		scanStore: &fakeScanStore{scans: []ScanResult{
			{Theme: "ai_rally", Severity: "weird_level", Confidence: 0.9, DetectedAt: mkTime(now, -2)},
		}},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), now)
	if tilt != 0 || drivers != nil {
		t.Errorf("unknown severity must be filtered, got (%v, %v)", tilt, drivers)
	}
}

func TestApplyScanThemes_FiltersNeutralTheme(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	p := &Predictor{
		scanStore: &fakeScanStore{scans: []ScanResult{
			{Theme: "earnings_release", Severity: "critical", Confidence: 0.9, DetectedAt: mkTime(now, -2)},
		}},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), now)
	if tilt != 0 || drivers != nil {
		t.Errorf("neutral theme must be filtered, got (%v, %v)", tilt, drivers)
	}
}

func TestApplyScanThemes_AggregatesMixed(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	p := &Predictor{
		scanStore: &fakeScanStore{scans: []ScanResult{
			// Bullish critical scan at 0.9 confidence
			{Theme: "ai_rally", Severity: "critical", Confidence: 0.9, DetectedAt: mkTime(now, -2)},
			// Bearish high scan at 0.7 confidence — partial offset
			{Theme: "foreign_capital_flight", Severity: "high", Confidence: 0.7, DetectedAt: mkTime(now, -3)},
			// Neutral — filtered
			{Theme: "earnings_release", Severity: "critical", Confidence: 0.9, DetectedAt: mkTime(now, -2)},
			// Old — filtered
			{Theme: "ai_rally", Severity: "critical", Confidence: 0.9, DetectedAt: mkTime(now, -48)},
		}},
	}
	tilt, drivers := p.applyScanThemes(context.Background(), now)
	// Net: +1.0*0.9 - 0.7*0.7 = 0.9 - 0.49 = 0.41 → dampened to 0.205
	if tilt <= 0 {
		t.Errorf("expected net bullish tilt, got %v", tilt)
	}
	if len(drivers) != 2 {
		t.Errorf("expected 2 drivers (cap=5), got %d: %v", len(drivers), drivers)
	}
}

func TestApplyScanThemes_BoundsTiltAtPlusMinusOne(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	scans := make([]ScanResult, 0, 20)
	for range 20 {
		scans = append(scans, ScanResult{
			Theme:      "ai_rally",
			Severity:   "critical",
			Confidence: 1.0,
			DetectedAt: mkTime(now, -1),
		})
	}
	p := &Predictor{scanStore: &fakeScanStore{scans: scans}}
	tilt, _ := p.applyScanThemes(context.Background(), now)
	// Even with 20x critical bullish scans, tilt must be bounded to ±dampener
	if tilt != scanThemeTiltDampener {
		t.Errorf("tilt must be bounded to dampener %v, got %v", scanThemeTiltDampener, tilt)
	}
}

func TestApplyScanThemes_DriverCapAtFive(t *testing.T) {
	now := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	scans := make([]ScanResult, 0, 10)
	for range 10 {
		scans = append(scans, ScanResult{
			Theme:      "ai_rally",
			Severity:   "critical",
			Confidence: 0.9,
			DetectedAt: mkTime(now, -1),
		})
	}
	p := &Predictor{scanStore: &fakeScanStore{scans: scans}}
	_, drivers := p.applyScanThemes(context.Background(), now)
	if len(drivers) != scanThemeMaxDriversKept {
		t.Errorf("expected driver cap %d, got %d", scanThemeMaxDriversKept, len(drivers))
	}
}
