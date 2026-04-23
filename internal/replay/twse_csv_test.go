package replay

import (
	"testing"
	"time"
)

func TestLoadTWSEOpenDataCSV(t *testing.T) {
	ds, err := LoadTWSEOpenDataCSV("../../samples/replay/twse_stock_day_all_sample.csv")
	if err != nil {
		t.Fatalf("load replay dataset: %v", err)
	}
	// 樣本數據包含 6 個交易日 (2026-03-20 至 2026-03-27)
	if len(ds.Dates) != 6 {
		t.Fatalf("expected 6 dates, got %d", len(ds.Dates))
	}

	date, _ := time.Parse("2006-01-02", "2026-03-26")
	ret, ok := ds.ForwardReturn("2330.TW", date, 1)
	if !ok {
		t.Fatalf("expected forward return")
	}
	if ret == 0 {
		t.Fatalf("expected non-zero forward return")
	}
}
