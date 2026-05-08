package narrative

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestGenerateDailySummary_DateMatches(t *testing.T) {
	rg := NewReportGenerator()
	date := "2026-04-20"

	report := rg.GenerateDailySummary(date, nil, nil, nil)

	if report.Date != date {
		t.Errorf("expected date %q, got %q", date, report.Date)
	}
}

func TestGenerateDailySummary_SectionsNonEmpty(t *testing.T) {
	rg := NewReportGenerator()

	report := rg.GenerateDailySummary("2026-04-20", nil, nil, nil)

	if len(report.Sections) == 0 {
		t.Fatal("expected at least one section")
	}
	for i, s := range report.Sections {
		if s.Content == "" {
			t.Errorf("section %d (%q) has empty content", i, s.Title)
		}
	}
}

func TestGenerateDailySummary_NarrativeEventsMentioned(t *testing.T) {
	rg := NewReportGenerator()
	events := []NarrativeEvent{
		{
			ID:         "evt-test-1",
			Theme:      "US_rates_up",
			Region:     "US",
			Sentiment:  -0.6,
			Confidence: 0.75,
			HitRate:    0.72,
			TimeWindow: "1_week",
			Timestamp:  time.Now().UTC(),
		},
		{
			ID:         "evt-test-2",
			Theme:      "AI_capex_surge",
			Region:     "US",
			Sentiment:  0.8,
			Confidence: 0.70,
			HitRate:    0.81,
			TimeWindow: "1_month",
			Timestamp:  time.Now().UTC(),
		},
	}

	report := rg.GenerateDailySummary("2026-04-20", events, nil, nil)

	if report.NarrativeCount != 2 {
		t.Errorf("expected NarrativeCount=2, got %d", report.NarrativeCount)
	}

	summarySection := findSection(report, "巨觀事件摘要")
	if summarySection == nil {
		t.Fatal("missing 巨觀事件摘要 section")
	}
	if !contains(summarySection.Content, "US_rates_up") {
		t.Error("narrative summary should mention US_rates_up")
	}
	if !contains(summarySection.Content, "AI_capex_surge") {
		t.Error("narrative summary should mention AI_capex_surge")
	}
}

func TestGenerateDailySummary_TopPicks(t *testing.T) {
	rg := NewReportGenerator()
	recs := []domain.Recommendation{
		{Agent: "agent-a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 8},
		{Agent: "agent-b", Symbol: "2881.TW", Side: domain.SideBuy, Conviction: 5},
		{Agent: "agent-c", Symbol: "2317.TW", Side: domain.SideSell, Conviction: 3},
	}

	report := rg.GenerateDailySummary("2026-04-20", nil, recs, nil)

	if len(report.TopPicks) != 3 {
		t.Errorf("expected 3 top picks, got %d", len(report.TopPicks))
	}
	if report.TopPicks[0].Symbol != "2330.TW" {
		t.Errorf("expected highest conviction pick to be 2330.TW, got %s", report.TopPicks[0].Symbol)
	}
}

func TestGenerateDailySummary_TopPicksCapped(t *testing.T) {
	rg := NewReportGenerator()
	var recs []domain.Recommendation
	for i := range 10 {
		recs = append(recs, domain.Recommendation{
			Agent:      "agent",
			Symbol:     "2330.TW",
			Side:       domain.SideBuy,
			Conviction: 10 - i,
		})
	}

	report := rg.GenerateDailySummary("2026-04-20", nil, recs, nil)

	if len(report.TopPicks) > 5 {
		t.Errorf("expected at most 5 top picks, got %d", len(report.TopPicks))
	}
}

func TestGenerateDailySummary_RiskLevel(t *testing.T) {
	rg := NewReportGenerator()

	tests := []struct {
		name      string
		risk      *domain.RiskSnapshot
		wantLevel string
	}{
		{"nil risk", nil, "未知"},
		{"low risk", &domain.RiskSnapshot{VaR95: 0.5, MaxDrawdownPct: 2.0}, "低"},
		{"medium risk", &domain.RiskSnapshot{VaR95: 2.0, MaxDrawdownPct: 6.0}, "中"},
		{"high risk", &domain.RiskSnapshot{VaR95: 4.0, MaxDrawdownPct: 12.0}, "高"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := rg.GenerateDailySummary("2026-04-20", nil, nil, tt.risk)
			if report.RiskLevel != tt.wantLevel {
				t.Errorf("expected risk level %q, got %q", tt.wantLevel, report.RiskLevel)
			}
		})
	}
}

func TestExplainFactorScores_MentionsFactorNames(t *testing.T) {
	rg := NewReportGenerator()
	rec := domain.Recommendation{
		Symbol: "2330.TW",
		FactorScores: domain.FactorScores{
			Momentum: 0.75,
			Value:    0.40,
			Quality:  0.85,
			Total:    0.67,
		},
	}

	explanation := rg.ExplainFactorScores(rec)

	if !contains(explanation, "2330.TW") {
		t.Error("explanation should mention symbol")
	}
	if !contains(explanation, "動能") {
		t.Error("explanation should mention 動能 factor")
	}
	if !contains(explanation, "價值") {
		t.Error("explanation should mention 價值 factor")
	}
	if !contains(explanation, "品質") {
		t.Error("explanation should mention 品質 factor")
	}
}

func TestExplainFactorScores_WithBreakdown(t *testing.T) {
	rg := NewReportGenerator()
	rec := domain.Recommendation{
		Symbol: "2881.TW",
		FactorScores: domain.FactorScores{
			Momentum: 0.60,
			Value:    0.80,
			Quality:  0.70,
			Total:    0.70,
			Breakdown: &domain.FactorScoreBreakdown{
				Momentum: domain.FactorScoreItem{
					Score:     0.60,
					Formula:   "momentum_20d",
					RawInputs: map[string]float64{"return_20d": 0.05},
				},
				Value: domain.FactorScoreItem{
					Score:     0.80,
					Formula:   "pe_inverse",
					RawInputs: map[string]float64{"pe_ratio": 12.5},
				},
				Quality: domain.FactorScoreItem{
					Score:     0.70,
					Formula:   "roe_stability",
					RawInputs: map[string]float64{"roe": 0.15},
				},
			},
		},
	}

	explanation := rg.ExplainFactorScores(rec)

	if !contains(explanation, "momentum_20d") {
		t.Error("explanation should mention momentum formula")
	}
	if !contains(explanation, "pe_inverse") {
		t.Error("explanation should mention value formula")
	}
	if !contains(explanation, "roe_stability") {
		t.Error("explanation should mention quality formula")
	}
}

func TestExplainFactorScores_WithoutBreakdown(t *testing.T) {
	rg := NewReportGenerator()
	rec := domain.Recommendation{
		Symbol: "0050.TW",
		FactorScores: domain.FactorScores{
			Momentum: 0.50,
			Value:    0.50,
			Quality:  0.50,
			Total:    0.50,
		},
	}

	explanation := rg.ExplainFactorScores(rec)

	if !contains(explanation, "0050.TW") {
		t.Error("explanation should mention symbol")
	}
	if !contains(explanation, "0.50") {
		t.Error("explanation should mention factor scores")
	}
}

func TestClassifyRisk(t *testing.T) {
	tests := []struct {
		name string
		risk *domain.RiskSnapshot
		want string
	}{
		{"nil", nil, "未知"},
		{"low", &domain.RiskSnapshot{VaR95: 0.5, MaxDrawdownPct: 1.0}, "低"},
		{"medium var", &domain.RiskSnapshot{VaR95: 2.0, MaxDrawdownPct: 1.0}, "中"},
		{"medium dd", &domain.RiskSnapshot{VaR95: 0.5, MaxDrawdownPct: 6.0}, "中"},
		{"high var", &domain.RiskSnapshot{VaR95: 4.0, MaxDrawdownPct: 1.0}, "高"},
		{"high dd", &domain.RiskSnapshot{VaR95: 0.5, MaxDrawdownPct: 15.0}, "高"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRisk(tt.risk)
			if got != tt.want {
				t.Errorf("classifyRisk(%v) = %q, want %q", tt.risk, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"long", "這是一個很長的測試字串", 6, "這是一個很…"},
		{"at period", "第一句。第二句。", 8, "第一句。…"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if len(got) > tt.maxLen+3 {
				t.Errorf("truncate(%q, %d) = %q (len=%d), expected len <= %d",
					tt.input, tt.maxLen, got, len(got), tt.maxLen+3)
			}
		})
	}
}

func findSection(report *domain.DailySummaryReport, title string) *domain.ReportSection {
	for i := range report.Sections {
		if report.Sections[i].Title == title {
			return &report.Sections[i]
		}
	}
	return nil
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && searchSubstring(s, substr))
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
