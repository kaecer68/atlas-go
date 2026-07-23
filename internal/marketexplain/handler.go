// Package marketexplain provides the "為什麼漲跌" (Why Did the Market Move)
// compose endpoint that generates a plain-Chinese market explanation for
// retail investors. It aggregates macro data, capital flow, and regime
// signals into a structured narrative.
//
// Design principle (per product-positioning.md): always return a usable
// explanation. The rule-based path is the backbone; LLM enhancement is
// optional and degrades gracefully when unavailable.
package marketexplain

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// Explanation is the structured response for GET /api/market/explain.
type Explanation struct {
	GeneratedAt time.Time `json:"generated_at"`
	Source      string    `json:"source"` // "rule_based" | "llm_enhanced"
	Headline    string    `json:"headline"`
	Detail      string    `json:"detail"`
	Sections    []Section `json:"sections"`
}

// Section is a named block of explanation text.
type Section struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Handler serves the market explain endpoint.
type Handler struct {
	provider marketdata.MacroDataProvider
	cf       *capitalflow.Service
}

// NewHandler creates a market explain handler.
func NewHandler(provider marketdata.MacroDataProvider, cf *capitalflow.Service) *Handler {
	return &Handler{provider: provider, cf: cf}
}

// HandleExplain responds to GET /api/market/explain.
func (h *Handler) HandleExplain(r *http.Request) (int, any) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// Fetch market data.
	snap, err := h.provider.FetchSnapshot(ctx)
	if err != nil {
		logging.Warn("marketexplain", "snapshot_failed", "err", err.Error())
		return http.StatusServiceUnavailable, map[string]string{
			"error": "無法取得市場資料，請稍後再試",
		}
	}

	// Fetch capital flow summary.
	cfSummary, cfErr := h.cf.Summary(ctx)
	if cfErr != nil {
		logging.Warn("marketexplain", "capitalflow_failed", "err", cfErr.Error())
	}

	explanation := compose(snap, cfSummary)
	return http.StatusOK, explanation
}

// compose builds a rule-based explanation from available data.
func compose(snap marketdata.MacroDataSnapshot, cfSummary capitalflow.SummaryReport) Explanation {
	sections := make([]Section, 0, 4)

	// 1. 大盤表現
	taiexSection := buildTAIEXSection(snap)
	sections = append(sections, taiexSection)

	// 2. 資金面
	capitalSection := buildCapitalSection(cfSummary)
	if capitalSection.Body != "" {
		sections = append(sections, capitalSection)
	}

	// 3. 國際環境
	globalSection := buildGlobalSection(snap)
	if globalSection.Body != "" {
		sections = append(sections, globalSection)
	}

	// 4. 風險提示
	riskSection := buildRiskSection(snap, cfSummary)
	if riskSection.Body != "" {
		sections = append(sections, riskSection)
	}

	// Headline: one-line summary from TAIEX + capital direction.
	headline := buildHeadline(snap, cfSummary)

	return Explanation{
		GeneratedAt: time.Now(),
		Source:      "rule_based",
		Headline:    headline,
		Detail:      strings.Join(sectionBodies(sections), "\n\n"),
		Sections:    sections,
	}
}

func sectionBodies(secs []Section) []string {
	out := make([]string, len(secs))
	for i, s := range secs {
		out[i] = s.Body
	}
	return out
}

// ---------------------------------------------------------------------------
// Section builders
// ---------------------------------------------------------------------------

func buildTAIEXSection(snap marketdata.MacroDataSnapshot) Section {
	chg := snap.TAIEX.ChangePct
	taiwanSemi := snap.TaiwanSemiIndex
	dir := "上漲"
	emoji := "📈"
	if chg < 0 {
		dir = "下跌"
		emoji = "📉"
	}
	absChg := chg
	if absChg < 0 {
		absChg = -absChg
	}

	body := fmt.Sprintf("今日加權指數%s %.2f%%，%s", dir, chg, emoji)

	if absChg >= 2.0 {
		body += "漲跌幅較大，市場情緒明顯波動"
	} else if absChg >= 1.0 {
		body += "有明顯方向性變化"
	} else if absChg >= 0.5 {
		body += "小幅波動，屬正常範圍"
	} else {
		body += "幾乎持平，市場觀望氣氛濃厚"
	}

	// Add semiconductor context if available.
	if taiwanSemi.ChangePct != 0 {
		semiDir := "上漲"
		if taiwanSemi.ChangePct < 0 {
			semiDir = "下跌"
		}
		body += fmt.Sprintf("。半導體指數%s %.2f%%，", semiDir, taiwanSemi.ChangePct)
		if (chg > 0 && taiwanSemi.ChangePct > 0) || (chg < 0 && taiwanSemi.ChangePct < 0) {
			body += "與大盤方向一致"
		} else {
			body += "與大盤方向分歧，資金在類股間輪動"
		}
	}

	body += "。"
	return Section{Title: "大盤表現", Body: body}
}

// buildCapitalSection generates the capital flow explanation.
func buildCapitalSection(cfSummary capitalflow.SummaryReport) Section {
	if cfSummary.Summary == "" {
		return Section{}
	}
	body := cfSummary.Summary

	// Add dominant force detail if available.
	if len(cfSummary.Forces) > 0 {
		body += "\n\n各勢力動態："
		for _, f := range cfSummary.Forces {
			if f.Role != "subject" {
				continue
			}
			label := forceLabel(f.Force)
			zSuffix := ""
			if f.ZScore > 1.5 {
				zSuffix = "（顯著偏多）"
			} else if f.ZScore < -1.5 {
				zSuffix = "（顯著偏空）"
			} else if f.ZScore > 0.5 {
				zSuffix = "（偏多）"
			} else if f.ZScore < -0.5 {
				zSuffix = "（偏空）"
			}
			rawVal := f.RawValue
			if rawVal < 0 {
				rawVal = -rawVal
			}
			body += fmt.Sprintf("\n• %s：%s %.1f 億%s", label, directionText(f.ZScore), rawVal, zSuffix)
		}
	}

	return Section{Title: "資金面", Body: body}
}

func directionText(z float64) string {
	if z > 0.5 {
		return "買超"
	} else if z < -0.5 {
		return "賣超"
	}
	return "買賣超"
}

func forceLabel(f capitalflow.ForceName) string {
	switch f {
	case capitalflow.ForceForeign:
		return "外資"
	case capitalflow.ForceInstitutional:
		return "投信"
	case capitalflow.ForceDealer:
		return "自營商"
	case capitalflow.ForceGovernment:
		return "公股行庫"
	case capitalflow.ForceRetail:
		return "散戶"
	default:
		return string(f)
	}
}

func buildGlobalSection(snap marketdata.MacroDataSnapshot) Section {
	var parts []string

	if snap.VIX.ChangePct != 0 {
		vixDir := "上升"
		if snap.VIX.ChangePct < 0 {
			vixDir = "下降"
		}
		parts = append(parts, fmt.Sprintf("VIX 恐慌指數%.2f（%s），", snap.VIX.Value, vixDir))
		if snap.VIX.Value > 25 {
			parts = append(parts, "市場恐慌情緒偏高")
		} else if snap.VIX.Value < 15 {
			parts = append(parts, "市場情緒平穩")
		}
	}

	if snap.USD_TWD.ChangePct != 0 {
		twdDir := "貶值"
		if snap.USD_TWD.ChangePct < 0 {
			twdDir = "升值"
		}
		parts = append(parts, fmt.Sprintf("新台幣%s（%.4f）", twdDir, snap.USD_TWD.Value))
	}

	if snap.US10Y.ChangePct != 0 {
		parts = append(parts, fmt.Sprintf("美債 10 年期殖利率 %.2f%%", snap.US10Y.Value))
	}

	if snap.DXY.ChangePct != 0 {
		dxyDir := "走強"
		if snap.DXY.ChangePct < 0 {
			dxyDir = "走弱"
		}
		parts = append(parts, fmt.Sprintf("美元指數%s（%.2f）", dxyDir, snap.DXY.Value))
	}

	if len(parts) == 0 {
		return Section{}
	}

	body := "國際環境：" + strings.Join(parts, "；") + "。"
	return Section{Title: "國際環境", Body: body}
}

func buildRiskSection(snap marketdata.MacroDataSnapshot, cfSummary capitalflow.SummaryReport) Section {
	var warnings []string

	// VIX threshold
	if snap.VIX.Value > 25 {
		warnings = append(warnings, "⚠️ VIX 高於 25，市場波動加劇，建議控制部位")
	}

	// Capital flow divergence: count opposing directions among official actors.
	divergent := countDivergent(cfSummary.Forces)
	if divergent >= 2 {
		warnings = append(warnings, "⚠️ 資金勢力分歧，三大法人方向不一致，短線震盪機率高")
	}

	// TAIEX large move
	absChg := snap.TAIEX.ChangePct
	if absChg < 0 {
		absChg = -absChg
	}
	if absChg >= 3.0 {
		warnings = append(warnings, "⚠️ 今日大盤波動超過 3%，請注意風險控管")
	}

	if len(warnings) == 0 {
		return Section{}
	}

	return Section{Title: "風險提示", Body: strings.Join(warnings, "\n")}
}

// countDivergent returns how many of the 3 official actors (foreign, institutional,
// dealer) have conflicting directions (some bullish, some bearish). Used to detect
// force divergence for risk warnings.
func countDivergent(forces []capitalflow.ForceScore) int {
	official := map[string]string{} // force → trend
	for _, f := range forces {
		if f.Role == "subject" && (f.Force == capitalflow.ForceForeign ||
			f.Force == capitalflow.ForceInstitutional ||
			f.Force == capitalflow.ForceDealer) {
			official[string(f.Force)] = f.Trend
		}
	}
	bullish := 0
	bearish := 0
	for _, t := range official {
		switch t {
		case "bullish":
			bullish++
		case "bearish":
			bearish++
		}
	}
	if bullish > 0 && bearish > 0 {
		return bullish + bearish
	}
	return 0
}

func buildHeadline(snap marketdata.MacroDataSnapshot, cfSummary capitalflow.SummaryReport) string {
	// Check if TAIEX data is available. D-01: the taiex_index channel
	// can fail independently, leaving snap.TAIEX as an empty struct
	// (Symbol="" → ChangePct=0.0). In that case "持平 0.00%" is a
	// misleading zero-value artifact, not a real market reading.
	if snap.TAIEX.Symbol == "" {
		headline := "今日台股指數資料待更新"
		if cfSummary.Summary != "" {
			firstSentence := cfSummary.Summary
			if idx := strings.Index(firstSentence, "。"); idx > 0 {
				firstSentence = firstSentence[:idx]
			}
			headline += "，" + firstSentence
		}
		headline += "。"
		return headline
	}

	chg := snap.TAIEX.ChangePct
	dir := "持平"
	if chg > 0 {
		dir = "上漲"
	} else if chg < 0 {
		dir = "下跌"
	}
	absChg := chg
	if absChg < 0 {
		absChg = -absChg
	}

	headline := fmt.Sprintf("今日台股%s %.2f%%", dir, chg)

	// Add capital context.
	if cfSummary.Summary != "" {
		// Extract first sentence.
		firstSentence := cfSummary.Summary
		if idx := strings.Index(firstSentence, "。"); idx > 0 {
			firstSentence = firstSentence[:idx]
		}
		headline += "，" + firstSentence
	}

	headline += "。"

	// Add VIX context.
	if snap.VIX.Value > 25 {
		headline += " 市場波動偏高，請謹慎操作。"
	} else if absChg >= 2 {
		headline += " 市場有明顯方向，建議關注資金持續性。"
	}

	return headline
}
