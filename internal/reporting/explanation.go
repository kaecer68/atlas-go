package reporting

import (
	"fmt"
	"strings"

	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// ExplainRecommendation converts a Recommendation with FactorScores and ConvictionBreakdown
// into a human-readable Chinese explanation.
// Format: "推薦 {symbol}（信念度 {conviction}）；{factor breakdown}；{conviction breakdown}"
func ExplainRecommendation(rec recommendation.Recommendation) string {
	var parts []string

	// Header: recommendation with conviction
	parts = append(parts, fmt.Sprintf("推薦 %s（信念度 %d）", rec.Symbol, rec.Conviction))

	// Factor breakdown
	if rec.FactorScores.Breakdown != nil {
		parts = append(parts, explainFactorBreakdown(rec.FactorScores.Breakdown))
	} else {
		// Fallback to aggregate scores
		parts = append(parts, fmt.Sprintf("因子總分 %.0f/100", rec.FactorScores.Total))
	}

	// Conviction breakdown
	if rec.ConvictionBreakdown != nil {
		parts = append(parts, explainConvictionBreakdown(rec.ConvictionBreakdown))
	}

	return strings.Join(parts, "；")
}

// explainConvictionBreakdown converts a ConvictionBreakdown into Chinese explanation.
// Format: "信念結構：基礎 %d → 地板 %d → 最終 %d；{steps}"
func explainConvictionBreakdown(cb *shared.ConvictionBreakdown) string {
	if cb == nil {
		return ""
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("信念結構：基礎 %d → 地板 %d → 最終 %d", cb.Base, cb.Floor, cb.Final))

	if len(cb.Steps) > 0 {
		var steps []string
		for _, step := range cb.Steps {
			deltaStr := ""
			if step.Delta > 0 {
				deltaStr = fmt.Sprintf("+%d", step.Delta)
			} else {
				deltaStr = fmt.Sprintf("%d", step.Delta)
			}
			steps = append(steps, fmt.Sprintf("%s（%s）%s", step.Rule, deltaStr, step.Reason))
		}
		parts = append(parts, strings.Join(steps, "，"))
	}

	return strings.Join(parts, "；")
}

// explainFactorBreakdown converts FactorScoreBreakdown into Chinese explanation.
// Format: "動能因子 85/100（ret20 12%），價值因子 45/100（pe 15%）[備援]"
func explainFactorBreakdown(bd *shared.FactorScoreBreakdown) string {
	if bd == nil {
		return ""
	}

	var factors []string

	// Momentum
	factors = append(factors, explainFactorItem("動能因子", bd.Momentum))

	// Value
	factors = append(factors, explainFactorItem("價值因子", bd.Value))

	// Quality
	factors = append(factors, explainFactorItem("品質因子", bd.Quality))

	// Agent
	factors = append(factors, explainFactorItem("代理因子", bd.Agent))

	// Institutional Sentiment
	factors = append(factors, explainFactorItem("機構情緒因子", bd.InstitutionalSentiment))

	// Liquidity
	factors = append(factors, explainFactorItem("流動性因子", bd.Liquidity))

	// Filter out empty strings
	var nonEmpty []string
	for _, f := range factors {
		if f != "" {
			nonEmpty = append(nonEmpty, f)
		}
	}

	return strings.Join(nonEmpty, "，")
}

// explainFactorItem formats a single FactorScoreItem into Chinese.
func explainFactorItem(name string, item shared.FactorScoreItem) string {
	if item.Score == 0 && !item.IsFallback {
		// Score of 0 with no fallback likely means not computed
		return ""
	}

	// Build raw inputs string if available
	var rawStr string
	if len(item.RawInputs) > 0 {
		var inputs []string
		for k, v := range item.RawInputs {
			inputs = append(inputs, fmt.Sprintf("%s %.0f", k, v*100))
		}
		rawStr = fmt.Sprintf("（%s）", strings.Join(inputs, "，"))
	}

	// Add fallback indicator
	fallbackStr := ""
	if item.IsFallback {
		fallbackStr = "【備援】"
	}

	return fmt.Sprintf("%s %.0f/100%s%s", name, item.Score, rawStr, fallbackStr)
}

// ExplainTrace converts a ReasoningTrace into a human-readable Chinese explanation.
// Format: "[{phase label}] {component}: {reasoning} | 信心度 {pct}% | [備援估算 if fallback]"
func ExplainTrace(trace orchestrator.ReasoningTrace) string {
	phaseLabel := getPhaseLabel(trace.Phase)

	var parts []string
	parts = append(parts, fmt.Sprintf("[%s] %s: %s", phaseLabel, trace.Component, trace.Reasoning))
	parts = append(parts, fmt.Sprintf("信心度 %.0f%%", trace.Confidence*100))

	if trace.IsFallback {
		parts = append(parts, "備援估算")
	}

	return strings.Join(parts, " | ")
}

// getPhaseLabel returns Chinese label for phase constant.
func getPhaseLabel(phase string) string {
	switch phase {
	case orchestrator.PhaseRegimeDetection:
		return "盤勢偵測"
	case orchestrator.PhaseAgentRecommendation:
		return "代理推薦"
	case orchestrator.PhaseControlFilter:
		return "控制過濾"
	case orchestrator.PhasePortfolioBuild:
		return "組合建構"
	default:
		return phase
	}
}
