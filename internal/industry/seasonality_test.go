package industry

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestDefaultSeasonalPatterns(t *testing.T) {
	patterns := DefaultSeasonalPatterns()
	if len(patterns) != 9 {
		t.Errorf("expected 9 seasonal patterns (7 canonical + 2 election), got %d", len(patterns))
	}

	// Check specific pattern
	var springFound bool
	for _, p := range patterns {
		if p.ID == "spring_festival" {
			springFound = true
			if p.StartMonth != 1 || p.StartDay != 15 {
				t.Errorf("expected spring festival start 1/15, got %d/%d", p.StartMonth, p.StartDay)
			}
			if p.EndMonth != 2 || p.EndDay != 15 {
				t.Errorf("expected spring festival end 2/15, got %d/%d", p.EndMonth, p.EndDay)
			}
			if p.HistoricalAccuracy != 0.70 {
				t.Errorf("expected accuracy 0.70, got %f", p.HistoricalAccuracy)
			}
			if len(p.FavoredIndustries) == 0 {
				t.Error("expected favored industries for spring festival")
			}
		}
	}
	if !springFound {
		t.Error("spring_festival pattern not found")
	}
}

func TestDetectCurrentPatterns(t *testing.T) {
	engine := NewSeasonalEngine()

	// Test spring festival (Jan 20) — overlaps with election_presidential
	springDate := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	patterns := engine.DetectCurrentPatterns(springDate)
	if len(patterns) != 2 {
		t.Errorf("expected 2 active patterns on Jan 20 (spring_festival + election_presidential), got %d", len(patterns))
	}
	var foundSpring bool
	for _, p := range patterns {
		if p.ID == "spring_festival" {
			foundSpring = true
			break
		}
	}
	if !foundSpring {
		t.Error("expected spring_festival in active patterns")
	}

	// Test tech peak season (Aug 1) - overlaps with summer_electricity
	techDate := time.Date(2024, 8, 1, 0, 0, 0, 0, time.UTC)
	patterns = engine.DetectCurrentPatterns(techDate)
	if len(patterns) != 2 {
		t.Errorf("expected 2 active patterns on Aug 1 (tech_peak + summer_electricity), got %d", len(patterns))
	}
	var hasTechPeak bool
	for _, p := range patterns {
		if p.ID == "tech_peak_season" {
			hasTechPeak = true
			break
		}
	}
	if !hasTechPeak {
		t.Error("expected tech_peak_season in active patterns")
	}

	// Test no pattern (Mar 20 - between patterns)
	noPatternDate := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	patterns = engine.DetectCurrentPatterns(noPatternDate)
	if len(patterns) != 1 { // earnings_window is 3/1-4/15
		t.Errorf("expected 1 active pattern on Mar 20, got %d", len(patterns))
	}
}

func TestGetPatternAdjustment(t *testing.T) {
	engine := NewSeasonalEngine()

	// Test favored industry during spring festival
	springDate := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	adj := engine.GetPatternAdjustment("financials", springDate)
	if adj <= 1.0 {
		t.Errorf("expected adjustment > 1.0 for favored industry, got %f", adj)
	}

	// Test avoided industry during spring festival
	adj = engine.GetPatternAdjustment("semiconductor", springDate)
	if adj >= 1.0 {
		t.Errorf("expected adjustment < 1.0 for avoided industry, got %f", adj)
	}

	// Test neutral industry
	adj = engine.GetPatternAdjustment("shipping", springDate)
	if adj != 1.0 {
		t.Errorf("expected adjustment 1.0 for neutral industry, got %f", adj)
	}
}

func TestIsDateInRange(t *testing.T) {
	engine := NewSeasonalEngine()

	tests := []struct {
		name     string
		month    int
		day      int
		startM   int
		startD   int
		endM     int
		endD     int
		expected bool
	}{
		{"normal range inside", 8, 1, 7, 1, 9, 15, true},
		{"normal range before", 6, 1, 7, 1, 9, 15, false},
		{"normal range after", 10, 1, 7, 1, 9, 15, false},
		{"wrapped range inside", 1, 10, 12, 20, 1, 15, true},
		{"wrapped range dec", 12, 25, 12, 20, 1, 15, true},
		{"wrapped range outside", 2, 20, 12, 20, 1, 15, false},
		{"exact start", 7, 1, 7, 1, 9, 15, true},
		{"exact end", 9, 15, 7, 1, 9, 15, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.isDateInRange(tt.month, tt.day, tt.startM, tt.startD, tt.endM, tt.endD)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGetPatternByID(t *testing.T) {
	engine := NewSeasonalEngine()

	pattern, ok := engine.GetPatternByID("spring_festival")
	if !ok {
		t.Fatal("spring_festival pattern not found")
	}
	if pattern.Name != "春節行情" {
		t.Errorf("expected name '春節行情', got '%s'", pattern.Name)
	}

	_, ok = engine.GetPatternByID("nonexistent")
	if ok {
		t.Error("expected false for nonexistent pattern")
	}
}

func TestGetHistoricalAccuracy(t *testing.T) {
	engine := NewSeasonalEngine()

	// Test during spring festival (Jan 20 now has spring_festival + election_presidential)
	springDate := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	accuracy := engine.GetHistoricalAccuracy(springDate)
	expected := (0.70 + 0.55) / 2.0
	if accuracy != expected {
		t.Errorf("expected accuracy %f during spring festival+election, got %f", expected, accuracy)
	}

	// Test during no pattern
	noPatternDate := time.Date(2024, 3, 20, 0, 0, 0, 0, time.UTC)
	accuracy = engine.GetHistoricalAccuracy(noPatternDate)
	if accuracy != 0.55 { // earnings_window accuracy
		t.Errorf("expected accuracy 0.55 during earnings window, got %f", accuracy)
	}
}

func TestGenerateCalendar(t *testing.T) {
	engine := NewSeasonalEngine()
	calendar := engine.GenerateCalendar(2024)

	if calendar.Year != 2024 {
		t.Errorf("expected year 2024, got %d", calendar.Year)
	}

	if len(calendar.Patterns) != 9 {
		t.Errorf("expected 9 patterns, got %d", len(calendar.Patterns))
	}

	// Check January has patterns
	janPatterns := calendar.ByMonth[1]
	if len(janPatterns) == 0 {
		t.Error("expected patterns in January")
	}

	// Check July has tech peak season
	julPatterns := calendar.ByMonth[7]
	var hasTechPeak bool
	for _, p := range julPatterns {
		if p.ID == "tech_peak_season" {
			hasTechPeak = true
			break
		}
	}
	if !hasTechPeak {
		t.Error("expected tech_peak_season in July")
	}
}

func TestPatternString(t *testing.T) {
	p := SeasonalPattern{
		Name:               "春節行情",
		NameEN:             "Spring Festival Rally",
		StartMonth:         1,
		StartDay:           15,
		EndMonth:           2,
		EndDay:             15,
		HistoricalAccuracy: 0.70,
		AvgMarketReturn:    0.032,
	}

	s := p.String()
	expected := "春節行情 (Spring Festival Rally): 01/15-02/15, Accuracy: 70%, Avg Return: 3.2%"
	if s != expected {
		t.Errorf("expected '%s', got '%s'", expected, s)
	}
}

func TestGetAdjustmentBreakdown(t *testing.T) {
	se := NewSeasonalEngine()
	ab := se.GetAdjustmentBreakdown("semiconductor", time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC))
	if ab.Composite <= 0 {
		t.Errorf("expected positive composite adjustment factor, got %f", ab.Composite)
	}
	if ab.DirectMatch <= 0 {
		t.Errorf("expected positive direct match, got %f", ab.DirectMatch)
	}
}

func TestDetectThemeDirection_OilRising(t *testing.T) {
	se := NewSeasonalEngine()
	modulator := &DynamicEnvModulator{
		current:  marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 115}},
		baseline: marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 100}},
	}
	se.SetDynamicEnv(modulator)

	direction := se.detectThemeDirection("oil_price_shock")
	if direction != 1.0 {
		t.Fatalf("expected +1.0 for rising oil (deviation 0.15 > 0.05), got %f", direction)
	}
}

func TestDetectThemeDirection_OilFalling(t *testing.T) {
	se := NewSeasonalEngine()
	modulator := &DynamicEnvModulator{
		current:  marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 90}},
		baseline: marketdata.MacroDataSnapshot{Oil: marketdata.MacroDataPoint{Value: 100}},
	}
	se.SetDynamicEnv(modulator)

	direction := se.detectThemeDirection("oil_price_shock")
	if direction != -1.0 {
		t.Fatalf("expected -1.0 for falling oil (deviation -0.10 < -0.05), got %f", direction)
	}
}

func TestDetectThemeDirection_DollarStrong(t *testing.T) {
	se := NewSeasonalEngine()
	modulator := &DynamicEnvModulator{
		current:  marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{Value: 110}},
		baseline: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{Value: 100}},
	}
	se.SetDynamicEnv(modulator)

	direction := se.detectThemeDirection("US_rates_up")
	if direction != 1.0 {
		t.Fatalf("expected +1.0 for strong dollar (deviation 0.10 > 0.03), got %f", direction)
	}
}

func TestDetectThemeDirection_DollarWeak(t *testing.T) {
	se := NewSeasonalEngine()
	modulator := &DynamicEnvModulator{
		current:  marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{Value: 95}},
		baseline: marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{Value: 100}},
	}
	se.SetDynamicEnv(modulator)

	direction := se.detectThemeDirection("US_rates_up")
	if direction != -1.0 {
		t.Fatalf("expected -1.0 for weak dollar (deviation -0.05 < -0.03), got %f", direction)
	}
}

func TestDetectThemeDirection_NoDynamicEnv(t *testing.T) {
	se := NewSeasonalEngine()
	// No DynamicEnvModulator set

	direction := se.detectThemeDirection("oil_price_shock")
	if direction != 1.0 {
		t.Fatalf("expected +1.0 fallback when no dynamicEnv, got %f", direction)
	}

	direction = se.detectThemeDirection("JPY_carry_unwind")
	if direction != -1.0 {
		t.Fatalf("expected -1.0 fallback for JPY_carry_unwind, got %f", direction)
	}
}

func TestDetectThemeDirection_UnknownTheme(t *testing.T) {
	se := NewSeasonalEngine()
	direction := se.detectThemeDirection("nonexistent_theme")
	if direction != 1.0 {
		t.Fatalf("expected +1.0 for unknown theme, got %f", direction)
	}
}
