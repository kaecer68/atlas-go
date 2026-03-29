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
	if len(ds.Dates) != 2 {
		t.Fatalf("expected 2 dates, got %d", len(ds.Dates))
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
