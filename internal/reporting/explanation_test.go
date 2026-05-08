package reporting

import (
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

func TestExplainRecommendation_FactorScores(t *testing.T) {
	rec := recommendation.Recommendation{
		Symbol:     "2330",
		Conviction: 75,
		FactorScores: shared.FactorScores{
			Momentum:               85,
			Value:                  45,
			Quality:                90,
			Agent:                  70,
			InstitutionalSentiment: 80,
			Liquidity:              60,
			Total:                  72,
			Breakdown: &shared.FactorScoreBreakdown{
				Momentum: shared.FactorScoreItem{
					Score:      85,
					Weight:     0.2,
					Formula:    "ret20",
					RawInputs:  map[string]float64{"ret20": 0.12},
					IsFallback: false,
				},
				Value: shared.FactorScoreItem{
					Score:      45,
					Weight:     0.15,
					Formula:    "pe_ratio",
					RawInputs:  map[string]float64{"pe": 15.0},
					IsFallback: false,
				},
				Quality: shared.FactorScoreItem{
					Score:      90,
					Weight:     0.25,
					Formula:    "roe",
					RawInputs:  map[string]float64{"roe": 0.25},
					IsFallback: false,
				},
				Agent: shared.FactorScoreItem{
					Score:      70,
					Weight:     0.15,
					Formula:    "agent_score",
					RawInputs:  map[string]float64{"score": 0.70},
					IsFallback: false,
				},
				InstitutionalSentiment: shared.FactorScoreItem{
					Score:      80,
					Weight:     0.1,
					Formula:    "inst_sentiment",
					RawInputs:  map[string]float64{"sentiment": 0.80},
					IsFallback: false,
				},
				Liquidity: shared.FactorScoreItem{
					Score:      60,
					Weight:     0.15,
					Formula:    "avg_volume",
					RawInputs:  map[string]float64{"volume": 5000},
					IsFallback: false,
				},
				Total: shared.FactorScoreItem{
					Score: 72,
				},
			},
		},
	}

	result := ExplainRecommendation(rec)

	if !strings.Contains(result, "推薦 2330") {
		t.Errorf("expected symbol in result, got: %s", result)
	}
	if !strings.Contains(result, "信念度 75") {
		t.Errorf("expected conviction in result, got: %s", result)
	}
	if !strings.Contains(result, "動能因子") {
		t.Errorf("expected momentum factor in result, got: %s", result)
	}
	if !strings.Contains(result, "85/100") {
		t.Errorf("expected momentum score in result, got: %s", result)
	}
	if !strings.Contains(result, "價值因子") {
		t.Errorf("expected value factor in result, got: %s", result)
	}
	if !strings.Contains(result, "45/100") {
		t.Errorf("expected value score in result, got: %s", result)
	}
}

func TestExplainRecommendation_ConvictionBreakdown(t *testing.T) {
	rec := recommendation.Recommendation{
		Symbol:     "2317",
		Conviction: 65,
		ConvictionBreakdown: &shared.ConvictionBreakdown{
			Base:  50,
			Floor: 30,
			Final: 65,
			Steps: []shared.ConvictionStep{
				{Rule: "base_score", Delta: 0, Reason: "初始信念"},
				{Rule: "sector_upgrade", Delta: 10, Reason: "產業升級加持"},
				{Rule: "momentum_confirm", Delta: 5, Reason: "動能確認"},
				{Rule: "liquidity_check", Delta: 0, Reason: "流動性符合"},
			},
		},
	}

	result := ExplainRecommendation(rec)

	if !strings.Contains(result, "推薦 2317") {
		t.Errorf("expected symbol in result, got: %s", result)
	}
	if !strings.Contains(result, "信念度 65") {
		t.Errorf("expected conviction in result, got: %s", result)
	}
	if !strings.Contains(result, "信念結構") {
		t.Errorf("expected conviction structure in result, got: %s", result)
	}
	if !strings.Contains(result, "基礎 50") {
		t.Errorf("expected base in result, got: %s", result)
	}
	if !strings.Contains(result, "地板 30") {
		t.Errorf("expected floor in result, got: %s", result)
	}
	if !strings.Contains(result, "最終 65") {
		t.Errorf("expected final in result, got: %s", result)
	}
	if !strings.Contains(result, "sector_upgrade") {
		t.Errorf("expected step rule in result, got: %s", result)
	}
	if !strings.Contains(result, "+10") {
		t.Errorf("expected positive delta in result, got: %s", result)
	}
}

func TestExplainRecommendation_NilBreakdown(t *testing.T) {
	rec := recommendation.Recommendation{
		Symbol:     "2498",
		Conviction: 55,
		FactorScores: shared.FactorScores{
			Total: 68,
		},
	}

	result := ExplainRecommendation(rec)

	if !strings.Contains(result, "推薦 2498") {
		t.Errorf("expected symbol in result, got: %s", result)
	}
	if !strings.Contains(result, "信念度 55") {
		t.Errorf("expected conviction in result, got: %s", result)
	}
	if !strings.Contains(result, "因子總分") {
		t.Errorf("expected fallback factor total in result, got: %s", result)
	}
	if !strings.Contains(result, "68/100") {
		t.Errorf("expected total score in result, got: %s", result)
	}
}

func TestExplainRecommendation_WithFallback(t *testing.T) {
	rec := recommendation.Recommendation{
		Symbol:     "3006",
		Conviction: 50,
		FactorScores: shared.FactorScores{
			Total: 55,
			Breakdown: &shared.FactorScoreBreakdown{
				Momentum: shared.FactorScoreItem{
					Score:      60,
					Formula:    "ret20",
					RawInputs:  map[string]float64{"ret20": 0.08},
					IsFallback: true,
				},
				Value: shared.FactorScoreItem{
					Score:      0,
					IsFallback: false,
				},
				Quality: shared.FactorScoreItem{
					Score:      50,
					Formula:    "roe",
					RawInputs:  map[string]float64{"roe": 0.15},
					IsFallback: false,
				},
				Agent: shared.FactorScoreItem{
					Score: 0,
				},
				InstitutionalSentiment: shared.FactorScoreItem{
					Score: 0,
				},
				Liquidity: shared.FactorScoreItem{
					Score: 0,
				},
				Total: shared.FactorScoreItem{
					Score: 55,
				},
			},
		},
	}

	result := ExplainRecommendation(rec)

	if !strings.Contains(result, "備援") {
		t.Errorf("expected fallback indicator in result, got: %s", result)
	}
}

func TestExplainTrace_RegimeDetection(t *testing.T) {
	trace := orchestrator.ReasoningTrace{
		SessionID:  "session-20260413-daily",
		Timestamp:  time.Now(),
		Phase:      orchestrator.PhaseRegimeDetection,
		Step:       1,
		Component:  "MacroAgent",
		Action:     "score",
		Reasoning:  "Risk-on 訊號確認",
		Confidence: 0.85,
		IsFallback: false,
	}

	result := ExplainTrace(trace)

	if !strings.Contains(result, "盤勢偵測") {
		t.Errorf("expected regime detection phase label, got: %s", result)
	}
	if !strings.Contains(result, "MacroAgent") {
		t.Errorf("expected component in result, got: %s", result)
	}
	if !strings.Contains(result, "Risk-on 訊號確認") {
		t.Errorf("expected reasoning in result, got: %s", result)
	}
	if !strings.Contains(result, "信心度 85%") {
		t.Errorf("expected confidence in result, got: %s", result)
	}
	if strings.Contains(result, "備援") {
		t.Errorf("unexpected fallback indicator, got: %s", result)
	}
}

func TestExplainTrace_Fallback(t *testing.T) {
	trace := orchestrator.ReasoningTrace{
		SessionID:  "session-20260413-daily",
		Timestamp:  time.Now(),
		Phase:      orchestrator.PhaseAgentRecommendation,
		Step:       3,
		Component:  "SectorAgent",
		Action:     "recommend",
		Reasoning:  "使用備援資料估算",
		Confidence: 0.45,
		IsFallback: true,
	}

	result := ExplainTrace(trace)

	if !strings.Contains(result, "代理推薦") {
		t.Errorf("expected agent recommendation phase label, got: %s", result)
	}
	if !strings.Contains(result, "SectorAgent") {
		t.Errorf("expected component in result, got: %s", result)
	}
	if !strings.Contains(result, "備援估算") {
		t.Errorf("expected fallback indicator, got: %s", result)
	}
	if !strings.Contains(result, "信心度 45%") {
		t.Errorf("expected low confidence, got: %s", result)
	}
}

func TestExplainTrace_AllPhases(t *testing.T) {
	phases := []struct {
		phase    string
		expected string
	}{
		{orchestrator.PhaseRegimeDetection, "盤勢偵測"},
		{orchestrator.PhaseAgentRecommendation, "代理推薦"},
		{orchestrator.PhaseControlFilter, "控制過濾"},
		{orchestrator.PhasePortfolioBuild, "組合建構"},
	}

	for _, p := range phases {
		trace := orchestrator.ReasoningTrace{
			Phase:      p.phase,
			Component:  "Test",
			Reasoning:  "test",
			Confidence: 0.5,
		}
		result := ExplainTrace(trace)
		if !strings.Contains(result, p.expected) {
			t.Errorf("phase %s: expected %s in result, got: %s", p.phase, p.expected, result)
		}
	}
}

func TestExplainRecommendation_EmptyConvictionSteps(t *testing.T) {
	rec := recommendation.Recommendation{
		Symbol:     "0050",
		Conviction: 40,
		ConvictionBreakdown: &shared.ConvictionBreakdown{
			Base:  40,
			Floor: 30,
			Final: 40,
			Steps: nil,
		},
	}

	result := ExplainRecommendation(rec)

	if !strings.Contains(result, "基礎 40") {
		t.Errorf("expected base in result, got: %s", result)
	}
	if !strings.Contains(result, "最終 40") {
		t.Errorf("expected final in result, got: %s", result)
	}
}
