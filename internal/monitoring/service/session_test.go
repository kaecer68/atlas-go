package service

import (
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

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
