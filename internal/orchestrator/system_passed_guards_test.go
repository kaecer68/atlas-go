package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// TestBuildSyntheticOutcomesMarksAllAgentsPassedForSharedSymbol 驗證：當多個 agent
// 推薦同一標的且該標的通過 CIO 聚合（出現在 finalRecs）時，所有原始 agent 的
// outcome 都應標記為 PassedGuards=true。對應 internal/orchestrator/AGENTS.md
// 「ID 混淆」陷阱。
func TestBuildSyntheticOutcomesMarksAllAgentsPassedForSharedSymbol(t *testing.T) {
	asOf := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	rawRecs := []domain.Recommendation{
		{
			Agent:      "financials-desk-01",
			Skill:      "financials_desk",
			Layer:      domain.LayerStyle,
			Symbol:     "2881.TW",
			Side:       domain.SideBuy,
			Conviction: 60,
			Reason:     "financials thesis A",
		},
		{
			Agent:      "value-yield-01",
			Skill:      "value_yield",
			Layer:      domain.LayerStyle,
			Symbol:     "2881.TW",
			Side:       domain.SideBuy,
			Conviction: 70,
			Reason:     "value yield thesis B",
		},
		{
			Agent:      "shipping-desk-01",
			Skill:      "shipping_desk",
			Layer:      domain.LayerStyle,
			Symbol:     "2609.TW",
			Side:       domain.SideBuy,
			Conviction: 65,
			Reason:     "shipping thesis",
		},
	}

	// CIO aggregator 將同標的聚合成單筆 rec，並以「最佳 agent」覆寫 Agent 欄位。
	// 此處 2881.TW 由 value-yield-01 勝出，2609.TW 由 shipping-desk-01 勝出。
	finalRecs := []domain.Recommendation{
		{
			Agent:      "value-yield-01",
			Skill:      "cio_portfolio",
			Layer:      domain.LayerControl,
			Symbol:     "2881.TW",
			Side:       domain.SideBuy,
			Conviction: 65,
			Reason:     "[crowded:2 agents] aggregated",
		},
		{
			Agent:      "shipping-desk-01",
			Skill:      "cio_portfolio",
			Layer:      domain.LayerControl,
			Symbol:     "2609.TW",
			Side:       domain.SideBuy,
			Conviction: 65,
			Reason:     "aggregated",
		},
	}

	quotes := []domain.Quote{
		{Symbol: "2881.TW", Open: 68, Last: 68.5, IsTradable: true},
		{Symbol: "2609.TW", Open: 245, Last: 246, IsTradable: true},
	}

	outcomes := buildSyntheticOutcomes(rawRecs, finalRecs, quotes, asOf, string(domain.RegimeRiskOn), nil)
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 outcomes (one per raw rec), got %d", len(outcomes))
	}

	// 對應 2881.TW 的兩筆原始推薦都應標記為 PassedGuards=true。
	for _, out := range outcomes {
		if out.Symbol != "2881.TW" {
			continue
		}
		if !out.PassedGuards {
			t.Errorf("expected PassedGuards=true for agent %q on 2881.TW (bestAgent=%q in finalRecs), got false (GuardReason=%q)",
				out.AgentID, "value-yield-01", out.GuardReason)
		}
		if out.GuardReason != "" {
			t.Errorf("expected empty GuardReason when passed, got %q for agent %q", out.GuardReason, out.AgentID)
		}
	}

	for _, out := range outcomes {
		if out.Regime != string(domain.RegimeRiskOn) {
			t.Errorf("expected Regime=%q, got %q", string(domain.RegimeRiskOn), out.Regime)
		}
	}
}

// TestBuildSyntheticOutcomesMarksUnpassedSymbolFailed 驗證對照組：未進入 finalRecs
// 的標的（被控制層過濾掉）其所有原始推薦應標記為 PassedGuards=false。
func TestBuildSyntheticOutcomesMarksUnpassedSymbolFailed(t *testing.T) {
	asOf := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	rawRecs := []domain.Recommendation{
		{
			Agent:      "financials-desk-01",
			Skill:      "financials_desk",
			Layer:      domain.LayerStyle,
			Symbol:     "2881.TW",
			Side:       domain.SideBuy,
			Conviction: 60,
			Reason:     "filtered by CRO",
		},
	}

	// finalRecs 為空集合（假設 2881.TW 被 CRO 阻擋或 CIO 阻擋）
	finalRecs := []domain.Recommendation{}

	outcomes := buildSyntheticOutcomes(rawRecs, finalRecs, nil, asOf, string(domain.RegimeRiskOff), nil)
	if len(outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(outcomes))
	}
	if outcomes[0].PassedGuards {
		t.Errorf("expected PassedGuards=false for filtered symbol, got true")
	}
	if outcomes[0].GuardReason != "未通過控制層過濾" {
		t.Errorf("expected GuardReason=%q, got %q", "未通過控制層過濾", outcomes[0].GuardReason)
	}
	if outcomes[0].Regime != string(domain.RegimeRiskOff) {
		t.Errorf("expected Regime=%q, got %q", string(domain.RegimeRiskOff), outcomes[0].Regime)
	}
}

// TestBuildReplayOutcomesMarksAllAgentsPassedForSharedSymbol 對應
// buildReplayOutcomes 的同類測試。
func TestBuildReplayOutcomesMarksAllAgentsPassedForSharedSymbol(t *testing.T) {
	ds := &replay.Dataset{
		ByDate: map[string]map[string]domain.DailyBar{
			"2026-04-02": {
				"2881.TW": {Date: time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC), Symbol: "2881.TW", Close: 68.5},
			},
			"2026-04-03": {
				"2881.TW": {Date: time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC), Symbol: "2881.TW", Close: 69.0},
			},
		},
		Dates: []time.Time{
			time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 4, 3, 0, 0, 0, 0, time.UTC),
		},
	}

	rawRecs := []domain.Recommendation{
		{
			Agent:      "financials-desk-01",
			Skill:      "financials_desk",
			Layer:      domain.LayerStyle,
			Symbol:     "2881.TW",
			Side:       domain.SideBuy,
			Conviction: 60,
			Reason:     "financials thesis A",
		},
		{
			Agent:      "value-yield-01",
			Skill:      "value_yield",
			Layer:      domain.LayerStyle,
			Symbol:     "2881.TW",
			Side:       domain.SideBuy,
			Conviction: 70,
			Reason:     "value yield thesis B",
		},
	}

	finalRecs := []domain.Recommendation{
		{
			Agent:      "value-yield-01",
			Skill:      "cio_portfolio",
			Layer:      domain.LayerControl,
			Symbol:     "2881.TW",
			Side:       domain.SideBuy,
			Conviction: 65,
			Reason:     "aggregated",
		},
	}

	asOf := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	outcomes := buildReplayOutcomes(rawRecs, finalRecs, nil, asOf, string(domain.RegimeRiskOn), ds, nil)

	if len(outcomes) != 2 {
		t.Fatalf("expected 2 outcomes, got %d", len(outcomes))
	}

	for _, out := range outcomes {
		if !out.PassedGuards {
			t.Errorf("expected PassedGuards=true for agent %q on 2881.TW, got false (GuardReason=%q)",
				out.AgentID, out.GuardReason)
		}
		if out.AgentID != "financials-desk-01" && out.AgentID != "value-yield-01" {
			t.Errorf("unexpected AgentID %q in outcome", out.AgentID)
		}
		if out.Regime != string(domain.RegimeRiskOn) {
			t.Errorf("expected Regime=%q for agent %q, got %q", string(domain.RegimeRiskOn), out.AgentID, out.Regime)
		}
	}
}
