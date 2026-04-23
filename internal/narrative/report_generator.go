package narrative

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ReportGenerator produces daily summary reports from narrative events,
// recommendations, and risk data.
type ReportGenerator struct {
	engine *NarrativeEngine
}

// NewReportGenerator creates a ReportGenerator backed by the default NarrativeEngine.
func NewReportGenerator() *ReportGenerator {
	return &ReportGenerator{
		engine: NewNarrativeEngine(),
	}
}

// GenerateDailySummary builds a DailySummaryReport for the given date.
func (rg *ReportGenerator) GenerateDailySummary(
	date string,
	events []NarrativeEvent,
	recommendations []domain.Recommendation,
	risk *domain.RiskSnapshot,
) *domain.DailySummaryReport {
	report := &domain.DailySummaryReport{
		Date:           date,
		NarrativeCount: len(events),
	}

	sections := make([]domain.ReportSection, 0, 5)

	sections = append(sections, rg.marketOverviewSection(date, events, risk))
	sections = append(sections, rg.narrativeSummarySection(events))
	sections = append(sections, rg.topPicksSection(recommendations))
	sections = append(sections, rg.riskWarningSection(risk))

	report.Sections = sections
	report.TopPicks = topPicks(recommendations, 5)
	report.RiskLevel = classifyRisk(risk)

	return report
}

func (rg *ReportGenerator) marketOverviewSection(date string, events []NarrativeEvent, risk *domain.RiskSnapshot) domain.ReportSection {
	regime := "NEUTRAL"
	if len(events) > 0 {
		avgSentiment := 0.0
		for _, e := range events {
			avgSentiment += e.Sentiment
		}
		avgSentiment /= float64(len(events))
		if avgSentiment > 0.2 {
			regime = "RISK_ON"
		} else if avgSentiment < -0.2 {
			regime = "RISK_OFF"
		}
	}

	riskLevel := classifyRisk(risk)

	content := fmt.Sprintf(TemplateMarketOverview, date, regime, riskLevel)
	return domain.ReportSection{
		Title:    "市場概況",
		Content:  content,
		Priority: 1,
	}
}

func (rg *ReportGenerator) narrativeSummarySection(events []NarrativeEvent) domain.ReportSection {
	var parts []string
	for _, evt := range events {
		tmpl, ok := rg.engine.kb.GetTemplate(evt.Theme)
		rationale := evt.Theme
		if ok {
			rationale = truncate(tmpl.Rationale, 120)
		}
		parts = append(parts, fmt.Sprintf(TemplateNarrativeSummary,
			evt.Theme, evt.Confidence, evt.HitRate, rationale))
	}

	content := strings.Join(parts, "\n\n")
	if content == "" {
		content = "今日未偵測到顯著巨觀事件。"
	}

	return domain.ReportSection{
		Title:    "巨觀事件摘要",
		Content:  content,
		Priority: 2,
	}
}

func (rg *ReportGenerator) topPicksSection(recs []domain.Recommendation) domain.ReportSection {
	top := topPicks(recs, 5)
	var parts []string
	for _, r := range top {
		parts = append(parts, fmt.Sprintf(TemplateTopPickIntro,
			r.Symbol, r.Side, r.Conviction, r.Agent))
	}

	content := strings.Join(parts, "\n\n")
	if content == "" {
		content = "今日無推薦標的。"
	}

	return domain.ReportSection{
		Title:    "重點推薦",
		Content:  content,
		Priority: 3,
	}
}

func (rg *ReportGenerator) riskWarningSection(risk *domain.RiskSnapshot) domain.ReportSection {
	riskLevel := classifyRisk(risk)
	var95 := 0.0
	maxDD := 0.0
	if risk != nil {
		var95 = risk.VaR95
		maxDD = risk.MaxDrawdownPct
	}

	content := fmt.Sprintf(TemplateRiskWarning, riskLevel, var95, maxDD)
	return domain.ReportSection{
		Title:    "風險警示",
		Content:  content,
		Priority: 4,
	}
}

// ExplainFactorScores returns a Chinese explanation for a recommendation's factor scores.
func (rg *ReportGenerator) ExplainFactorScores(rec domain.Recommendation) string {
	fs := rec.FactorScores
	content := fmt.Sprintf(TemplateFactorExplanation,
		rec.Symbol, fs.Momentum, fs.Value, fs.Quality, fs.Total)

	var details []string
	if fs.Breakdown != nil {
		b := fs.Breakdown
		if b.Momentum.Formula != "" {
			details = append(details, fmt.Sprintf("• 動能：公式 = %s，原始輸入 = %v", b.Momentum.Formula, b.Momentum.RawInputs))
		}
		if b.Value.Formula != "" {
			details = append(details, fmt.Sprintf("• 價值：公式 = %s，原始輸入 = %v", b.Value.Formula, b.Value.RawInputs))
		}
		if b.Quality.Formula != "" {
			details = append(details, fmt.Sprintf("• 品質：公式 = %s，原始輸入 = %v", b.Quality.Formula, b.Quality.RawInputs))
		}
	}

	if len(details) > 0 {
		content += "\n\n" + strings.Join(details, "\n")
	}

	return content
}

func topPicks(recs []domain.Recommendation, n int) []domain.Recommendation {
	sorted := make([]domain.Recommendation, len(recs))
	copy(sorted, recs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Conviction > sorted[j].Conviction
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

func classifyRisk(risk *domain.RiskSnapshot) string {
	if risk == nil {
		return "未知"
	}
	if risk.VaR95 > 3.0 || risk.MaxDrawdownPct > 10.0 {
		return "高"
	}
	if risk.VaR95 > 1.5 || risk.MaxDrawdownPct > 5.0 {
		return "中"
	}
	return "低"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	for i, r := range s {
		if i > maxLen {
			break
		}
		if r == '\n' || r == '。' || r == '，' {
			return s[:i+1] + "…"
		}
	}
	return s[:maxLen] + "…"
}
