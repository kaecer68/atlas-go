package strategy_techniques

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// validFrame 回傳一個通過 Validate() 的完整 StrategyFrame。
// 測試案例以此為基礎，修改特定欄位後預期失敗。
func validFrame() StrategyFrame {
	return StrategyFrame{
		ID:              "test-l1-dxy-weakness",
		Name:            "DXY 轉弱 → 資金回流亞洲",
		Layer:           LayerL1GlobalLiquidity,
		Summary:         "DXY 連兩日下行，帶動資金回流新興市場，台股受惠。",
		Rationale:       "歷史 2017、2020 都呈現此規律。",
		Conditions:      []Condition{{Field: "DXY", Operator: "lt", Value: 105}},
		Themes:          []string{"US_rates_up"},
		Regimes:         []string{"NOVEL"},
		Sectors:         []string{"semiconductor", "export"},
		Direction:       DirectionUp,
		Risk:            RiskMedium,
		Source:          SourceBacktest,
		Status:          StatusActive,
		AttributionMode: AttributionModeRuleBased,
		HitRate:         0.68,
		TotalTests:      100,
		TotalHits:       68,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

// ─── Validate 正常路徑 ──────────────────────────────────────────────

func TestStrategyFrame_Valid(t *testing.T) {
	f := validFrame()
	if err := f.Validate(); err != nil {
		t.Errorf("expected valid frame, got error: %v", err)
	}
}

// ─── Validate 邊界：必填欄位缺失 ──────────────────────────────────

func TestStrategyFrame_EmptyID(t *testing.T) {
	f := validFrame()
	f.ID = ""
	err := f.Validate()
	if err == nil {
		t.Fatal("expected error for empty ID")
	}
	if !strings.Contains(err.Error(), "ID") {
		t.Errorf("error should mention ID, got: %v", err)
	}
}

func TestStrategyFrame_EmptyName(t *testing.T) {
	f := validFrame()
	f.Name = ""
	err := f.Validate()
	if err == nil {
		t.Fatal("expected error for empty Name")
	}
}

func TestStrategyFrame_EmptySummary(t *testing.T) {
	f := validFrame()
	f.Summary = ""
	err := f.Validate()
	if err == nil {
		t.Fatal("expected error for empty Summary")
	}
}

func TestStrategyFrame_EmptyConditions(t *testing.T) {
	f := validFrame()
	f.Conditions = nil
	err := f.Validate()
	if err == nil {
		t.Fatal("expected error for empty Conditions")
	}
}

// ─── Validate 邊界：enum 欄位非法 ─────────────────────────────────

func TestStrategyFrame_InvalidLayer(t *testing.T) {
	f := validFrame()
	f.Layer = Layer("L99")
	if err := f.Validate(); err == nil {
		t.Error("expected error for invalid Layer")
	}
}

func TestStrategyFrame_InvalidDirection(t *testing.T) {
	f := validFrame()
	f.Direction = Direction("sideways")
	if err := f.Validate(); err == nil {
		t.Error("expected error for invalid Direction")
	}
}

func TestStrategyFrame_InvalidRisk(t *testing.T) {
	f := validFrame()
	f.Risk = Risk("critical")
	if err := f.Validate(); err == nil {
		t.Error("expected error for invalid Risk")
	}
}

func TestStrategyFrame_InvalidSource(t *testing.T) {
	f := validFrame()
	f.Source = Source("magical")
	if err := f.Validate(); err == nil {
		t.Error("expected error for invalid Source")
	}
}

func TestStrategyFrame_InvalidStatus(t *testing.T) {
	f := validFrame()
	f.Status = Status("unknown")
	if err := f.Validate(); err == nil {
		t.Error("expected error for invalid Status")
	}
}

func TestStrategyFrame_InvalidAttributionMode(t *testing.T) {
	f := validFrame()
	f.AttributionMode = AttributionMode("quantum")
	if err := f.Validate(); err == nil {
		t.Error("expected error for invalid AttributionMode")
	}
}

// ─── Validate 邊界：數值越界 ─────────────────────────────────────

func TestStrategyFrame_HitRateOver1(t *testing.T) {
	f := validFrame()
	f.HitRate = 1.5
	if err := f.Validate(); err == nil {
		t.Error("expected error for HitRate > 1")
	}
}

func TestStrategyFrame_HitRateNegative(t *testing.T) {
	f := validFrame()
	f.HitRate = -0.1
	if err := f.Validate(); err == nil {
		t.Error("expected error for HitRate < 0")
	}
}

func TestStrategyFrame_NegativeTotalTests(t *testing.T) {
	f := validFrame()
	f.TotalTests = -1
	if err := f.Validate(); err == nil {
		t.Error("expected error for negative TotalTests")
	}
}

func TestStrategyFrame_TotalHitsOverTests(t *testing.T) {
	f := validFrame()
	f.TotalHits = 200
	f.TotalTests = 100
	if err := f.Validate(); err == nil {
		t.Error("expected error for TotalHits > TotalTests")
	}
}

// ─── JSON roundtrip ─────────────────────────────────────────────

func TestStrategyFrame_JSON_Roundtrip(t *testing.T) {
	original := validFrame()
	original.CreatedAt = time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	original.UpdatedAt = time.Date(2024, 6, 11, 9, 0, 0, 0, time.UTC)

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// 確認 JSON 為 snake_case
	if !strings.Contains(string(data), `"id":"test-l1-dxy-weakness"`) {
		t.Errorf("expected snake_case id field, got: %s", string(data))
	}
	if !strings.Contains(string(data), `"hit_rate":0.68`) {
		t.Errorf("expected snake_case hit_rate field, got: %s", string(data))
	}

	var decoded StrategyFrame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: %s vs %s", decoded.ID, original.ID)
	}
	if decoded.Layer != original.Layer {
		t.Errorf("Layer mismatch: %s vs %s", decoded.Layer, original.Layer)
	}
	if decoded.Direction != original.Direction {
		t.Errorf("Direction mismatch: %s vs %s", decoded.Direction, original.Direction)
	}
	if decoded.HitRate != original.HitRate {
		t.Errorf("HitRate mismatch: %f vs %f", decoded.HitRate, original.HitRate)
	}
	if decoded.TotalHits != original.TotalHits {
		t.Errorf("TotalHits mismatch: %d vs %d", decoded.TotalHits, original.TotalHits)
	}
	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("CreatedAt mismatch: %v vs %v", decoded.CreatedAt, original.CreatedAt)
	}
	if len(decoded.Conditions) != len(original.Conditions) {
		t.Errorf("Conditions length mismatch: %d vs %d", len(decoded.Conditions), len(original.Conditions))
	}
}

// ─── JSON 容錯：enum 收到未知值時不 panic，後續 Validate() 會擋 ──

func TestStrategyFrame_JSON_UnknownEnumDoesNotPanic(t *testing.T) {
	raw := `{
		"id": "x",
		"name": "x",
		"summary": "x",
		"conditions": [{"field":"DXY","operator":"lt","value":105}],
		"layer": "L99",
		"direction": "sideways",
		"risk": "critical",
		"source": "magical",
		"status": "unknown",
		"attribution_mode": "quantum",
		"hit_rate": 0.5,
		"total_tests": 10,
		"total_hits": 5
	}`
	var f StrategyFrame
	if err := json.Unmarshal([]byte(raw), &f); err != nil {
		t.Fatalf("Unmarshal should not error on unknown enum (Validate handles it): %v", err)
	}
	if err := f.Validate(); err == nil {
		t.Error("expected Validate() to fail for unknown enum values")
	}
}
