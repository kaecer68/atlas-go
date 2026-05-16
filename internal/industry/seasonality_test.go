package industry

import (
	"testing"
	"time"
)

func TestDefaultSeasonalPatterns(t *testing.T) {
	patterns := DefaultSeasonalPatterns()
	if len(patterns) != 7 {
		t.Errorf("expected 7 seasonal patterns, got %d", len(patterns))
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

	// Test spring festival (Jan 20)
	springDate := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	patterns := engine.DetectCurrentPatterns(springDate)
	if len(patterns) != 1 {
		t.Errorf("expected 1 active pattern on Jan 20, got %d", len(patterns))
	}
	if len(patterns) > 0 && patterns[0].ID != "spring_festival" {
		t.Errorf("expected spring_festival, got %s", patterns[0].ID)
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

	// Test during spring festival
	springDate := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	accuracy := engine.GetHistoricalAccuracy(springDate)
	if accuracy != 0.70 {
		t.Errorf("expected accuracy 0.70 during spring festival, got %f", accuracy)
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

	if len(calendar.Patterns) != 7 {
		t.Errorf("expected 7 patterns, got %d", len(calendar.Patterns))
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
	engine := NewSeasonalEngine()
	now := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC) // tech_peak_season active

	// Without linkage/narrative/dynamic env — should just have direct_match
	bd := engine.GetAdjustmentBreakdown("semiconductor", now)
	if bd == nil {
		t.Fatal("expected non-nil breakdown")
	}
	if bd.DirectMatch <= 1.0 {
		t.Errorf("expected DirectMatch > 1.0 for semiconductor during tech_peak_season, got %.4f", bd.DirectMatch)
	}
	if bd.SupplyChain != 1.0 {
		t.Errorf("expected SupplyChain=1.0 (no graph), got %.4f", bd.SupplyChain)
	}
	if bd.Narrative != 1.0 {
		t.Errorf("expected Narrative=1.0 (no provider), got %.4f", bd.Narrative)
	}
	if bd.DynamicEnv != 1.0 {
		t.Errorf("expected DynamicEnv=1.0 (no modulator), got %.4f", bd.DynamicEnv)
	}
	if bd.Composite != bd.DirectMatch*bd.SupplyChain*bd.Narrative*bd.DynamicEnv {
		t.Errorf("Composite %.4f != product of layers %.4f", bd.Composite, bd.DirectMatch*bd.SupplyChain*bd.Narrative*bd.DynamicEnv)
	}

	// No active patterns for a neutral date
	neutral := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC) // no pattern
	bd2 := engine.GetAdjustmentBreakdown("semiconductor", neutral)
	if bd2 == nil {
		t.Fatal("expected non-nil breakdown")
	}
	if bd2.DirectMatch != 1.0 {
		t.Errorf("expected DirectMatch=1.0 for neutral date, got %.4f", bd2.DirectMatch)
	}
	if bd2.Composite != 1.0 {
		t.Errorf("expected Composite=1.0 for neutral date, got %.4f", bd2.Composite)
	}

	// Avoided industry during tech_peak_season
	bd3 := engine.GetAdjustmentBreakdown("consumer", now)
	if bd3 == nil {
		t.Fatal("expected non-nil breakdown")
	}
	if bd3.DirectMatch >= 1.0 {
		t.Errorf("expected DirectMatch < 1.0 for consumer avoided during tech_peak_season, got %.4f", bd3.DirectMatch)
	}
}
