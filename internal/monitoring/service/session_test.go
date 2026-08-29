package service

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// buildDataset 構造 5 個交易日的 replay.Dataset，prices 長度 = 5，
// 依序對應 ds.Dates[0..4]。index 4 為「今日」。
func buildDataset(t *testing.T, symbol string, prices []float64) (*replay.Dataset, time.Time) {
	t.Helper()
	if len(prices) != 5 {
		t.Fatalf("buildDataset requires exactly 5 prices, got %d", len(prices))
	}
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	dates := make([]time.Time, 5)
	byDate := make(map[string]map[string]domain.DailyBar, 5)
	for idx, p := range prices {
		d := base.AddDate(0, 0, idx)
		dates[idx] = d
		key := d.Format("2006-01-02")
		byDate[key] = map[string]domain.DailyBar{
			symbol: {
				Date:   d,
				Symbol: symbol,
				Open:   p,
				High:   p,
				Low:    p,
				Close:  p,
				Volume: 1000,
			},
		}
	}
	return &replay.Dataset{ByDate: byDate, Dates: dates}, base.AddDate(0, 0, 4)
}

// TestComputePipelineTags_High5ExcludesToday 驗證「創5日高」不該在「今日收盤
// 等於過往 4 日最高但非歷史新高」時觸發。這是回歸測試：之前因迴圈包含 i 導致
// bar.Close 與自身比較，high5 永遠 == bar.Close，標籤誤觸發。
func TestComputePipelineTags_High5ExcludesToday(t *testing.T) {
	// 過往 4 日收盤 100/100/100/100；今日收盤 100（等於過往 4 日最高，非新高）。
	ds, today := buildDataset(t, "2330", []float64{100, 100, 100, 100, 100})
	tags := computePipelineTags(ds, "2330", today)
	if contains(tags, "創5日高") {
		t.Errorf("今日收盤等於過往 4 日最高 (非新高)，不應出現「創5日高」；got tags=%v", tags)
	}
}

// TestComputePipelineTags_High5TriggeredWhenTrueHigh 驗證正常情境：
// 今日收盤 > 過往 4 日，應觸發「創5日高」。
func TestComputePipelineTags_High5TriggeredWhenTrueHigh(t *testing.T) {
	ds, today := buildDataset(t, "2330", []float64{90, 92, 95, 98, 105})
	tags := computePipelineTags(ds, "2330", today)
	if !contains(tags, "創5日高") {
		t.Errorf("今日 (105) 高於過往 4 日 (90/92/95/98)，應觸發「創5日高」；got tags=%v", tags)
	}
}

// TestComputePipelineTags_Low5ExcludesToday 對稱測試：今日收盤等於過往
// 4 日最低（但非歷史新低）時不應觸發「創5日低」。
func TestComputePipelineTags_Low5ExcludesToday(t *testing.T) {
	ds, today := buildDataset(t, "2330", []float64{100, 100, 100, 100, 100})
	tags := computePipelineTags(ds, "2330", today)
	if contains(tags, "創5日低") {
		t.Errorf("今日收盤等於過往 4 日最低 (非新低)，不應出現「創5日低」；got tags=%v", tags)
	}
}

// TestComputePipelineTags_Low5TriggeredWhenTrueLow 驗證正常情境：
// 今日收盤 < 過往 4 日，應觸發「創5日低」。
func TestComputePipelineTags_Low5TriggeredWhenTrueLow(t *testing.T) {
	ds, today := buildDataset(t, "2330", []float64{110, 108, 105, 102, 95})
	tags := computePipelineTags(ds, "2330", today)
	if !contains(tags, "創5日低") {
		t.Errorf("今日 (95) 低於過往 4 日 (110/108/105/102)，應觸發「創5日低」；got tags=%v", tags)
	}
}

func TestIsStockPickingLayer(t *testing.T) {
	tests := []struct {
		layer  string
		expect bool
	}{
		{"sector", true},
		{"style", true},
		{"superinvestor", true},
		{"regime", false},
		{"macro", false},
		{"", false},
		{"sECTOR", false},
	}
	for _, tt := range tests {
		got := isStockPickingLayer(tt.layer)
		if got != tt.expect {
			t.Errorf("isStockPickingLayer(%q) = %v, want %v", tt.layer, got, tt.expect)
		}
	}
}

func TestIsStockPickingLayerByID(t *testing.T) {
	views := []AgentUniverseViewData{
		{AgentID: "agent-001", Name: "Agent 1", Layer: "sector"},
		{AgentID: "agent-002", Name: "Agent 2", Layer: "regime"},
		{AgentID: "agent-003", Name: "Agent 3", Layer: "style"},
	}

	if !isStockPickingLayerByID("agent-001", views) {
		t.Error("agent-001 is sector layer, expected true")
	}
	if isStockPickingLayerByID("agent-002", views) {
		t.Error("agent-002 is regime layer, expected false")
	}
	if !isStockPickingLayerByID("agent-003", views) {
		t.Error("agent-003 is style layer, expected true")
	}
	if isStockPickingLayerByID("nonexistent", views) {
		t.Error("nonexistent agent should return false")
	}
	if isStockPickingLayerByID("", views) {
		t.Error("empty agentID should return false")
	}
}

func contains(ss []string, target string) bool {
	return slices.Contains(ss, target)
}

func TestComputePipelineTags_NilDataset(t *testing.T) {
	tags := computePipelineTags(nil, "2330", time.Now())
	if tags != nil {
		t.Errorf("expected nil for nil dataset, got %v", tags)
	}
}

func TestComputePipelineTags_MissingSymbol(t *testing.T) {
	ds, today := buildDataset(t, "2330", []float64{100, 100, 100, 100, 100})
	tags := computePipelineTags(ds, "missing", today)
	if tags != nil {
		t.Errorf("expected nil for missing symbol, got %v", tags)
	}
}

func TestComputePipelineTags_LongRed(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	ds := &replay.Dataset{
		Dates: []time.Time{base.AddDate(0, 0, 4)},
		ByDate: map[string]map[string]domain.DailyBar{
			"2026-06-05": {"2330": {Symbol: "2330", Open: 100, Close: 110, Volume: 1000}},
		},
	}
	tags := computePipelineTags(ds, "2330", base.AddDate(0, 0, 4))
	if !contains(tags, "長紅") {
		t.Errorf("expected 長紅 tag for +10%% move, got %v", tags)
	}
}

func TestComputePipelineTags_LongBlack(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	ds := &replay.Dataset{
		Dates: []time.Time{base.AddDate(0, 0, 4)},
		ByDate: map[string]map[string]domain.DailyBar{
			"2026-06-05": {"2330": {Symbol: "2330", Open: 100, Close: 90, Volume: 1000}},
		},
	}
	tags := computePipelineTags(ds, "2330", base.AddDate(0, 0, 4))
	if !contains(tags, "長黑") {
		t.Errorf("expected 長黑 tag for -10%% move, got %v", tags)
	}
}

func TestComputePipelineTags_VolumeSurge(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	ds := &replay.Dataset{
		Dates: []time.Time{base, base.AddDate(0, 0, 1), base.AddDate(0, 0, 2), base.AddDate(0, 0, 3), base.AddDate(0, 0, 4)},
		ByDate: map[string]map[string]domain.DailyBar{
			"2026-06-01": {"2330": {Symbol: "2330", Open: 100, Close: 100, Volume: 1000}},
			"2026-06-02": {"2330": {Symbol: "2330", Open: 100, Close: 100, Volume: 1000}},
			"2026-06-03": {"2330": {Symbol: "2330", Open: 100, Close: 100, Volume: 1000}},
			"2026-06-04": {"2330": {Symbol: "2330", Open: 100, Close: 100, Volume: 1000}},
			"2026-06-05": {"2330": {Symbol: "2330", Open: 100, Close: 105, Volume: 2000}},
		},
	}
	tags := computePipelineTags(ds, "2330", base.AddDate(0, 0, 4))
	if !contains(tags, "放量") {
		t.Errorf("expected 放量 tag for volume surge, got %v", tags)
	}
}

func TestComputePipelineTags_Wrapper(t *testing.T) {
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	ds := &replay.Dataset{
		Dates: []time.Time{base.AddDate(0, 0, 4)},
		ByDate: map[string]map[string]domain.DailyBar{
			"2026-06-05": {"2330": {Symbol: "2330", Open: 100, Close: 110, Volume: 1000}},
		},
	}
	tags, err := ComputePipelineTags(context.TODO(), ds, "2330", base.AddDate(0, 0, 4))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !contains(tags, "長紅") {
		t.Errorf("expected 長紅 tag, got %v", tags)
	}
}

func TestFallbackPriceTargets_WithDefault(t *testing.T) {
	target, stopLoss, err := FallbackPriceTargets(context.TODO(), "nonexistent-skill", 100.0, domain.SideBuy)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if target <= 100.0 || stopLoss >= 100.0 {
		t.Errorf("target (%f) should be > price (100), stopLoss (%f) should be < price (100)", target, stopLoss)
	}
}

func TestFallbackPriceTargets_SellSide(t *testing.T) {
	target, stopLoss, err := FallbackPriceTargets(context.TODO(), "nonexistent-skill", 100.0, domain.SideSell)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if stopLoss <= 100.0 || target >= 100.0 {
		t.Errorf("for sell side: stopLoss (%f) should be > price (100), target (%f) should be < price (100)", stopLoss, target)
	}
}
