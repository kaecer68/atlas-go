package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeChart serves a Yahoo v8 chart payload built from (day, close) rows in UTC.
func fakeChart(t *testing.T, rows []struct {
	day   string
	close float64
}) *httptest.Server {
	t.Helper()
	ts := make([]int64, 0, len(rows))
	closes := make([]*float64, 0, len(rows))
	for _, r := range rows {
		day, err := time.ParseInLocation("2006-01-02", r.day, time.UTC)
		if err != nil {
			t.Fatalf("bad day %q: %v", r.day, err)
		}
		// 13:00 UTC bar time → same UTC date.
		bar := day.Add(13 * time.Hour)
		c := r.close
		ts = append(ts, bar.Unix())
		closes = append(closes, &c)
	}
	payload := map[string]interface{}{
		"chart": map[string]interface{}{
			"result": []interface{}{map[string]interface{}{
				"meta":      map[string]interface{}{"exchangeTimezoneName": "UTC"},
				"timestamp": ts,
				"indicators": map[string]interface{}{
					"quote": []interface{}{map[string]interface{}{"close": closes}},
				},
			}},
			"error": nil,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchHistoryComputesChangePct(t *testing.T) {
	orig := chartURLTemplate
	t.Cleanup(func() { chartURLTemplate = orig })

	srv := fakeChart(t, []struct {
		day   string
		close float64
	}{
		{"2026-08-31", 100.0},
		{"2026-09-01", 102.0},
		{"2026-09-02", 99.0},  // ≈ -2.94%
		{"2026-09-03", 198.0}, // implausible +100% → bar rejected
	})
	chartURLTemplate = srv.URL + "/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d"

	start, _ := time.ParseInLocation("2006-01-02", "2026-09-01", time.Local)
	end, _ := time.ParseInLocation("2006-01-02", "2026-09-03", time.Local)
	ch := channel{field: "tsm_adr", symbol: "TSM", ticker: "TEST"}

	bars, err := fetchHistory(context.Background(), srv.Client(), ch, start, end)
	if err != nil {
		t.Fatalf("fetchHistory: %v", err)
	}
	// Expect 2026-09-01 (change +2%), 2026-09-02 (change <0); 09-03 rejected.
	if len(bars) != 2 {
		t.Fatalf("want 2 bars, got %d: %+v", len(bars), bars)
	}
	if bars[0].date != "2026-09-01" || bars[0].value != 102.0 {
		t.Fatalf("bar0 = %+v", bars[0])
	}
	if diff := bars[0].changePct - 2.0; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("bar0 changePct = %v, want 2.0", bars[0].changePct)
	}
	if bars[1].date != "2026-09-02" || bars[1].changePct >= 0 {
		t.Fatalf("bar1 = %+v, want negative change", bars[1])
	}
}

func TestFetchHistoryYahooError(t *testing.T) {
	orig := chartURLTemplate
	t.Cleanup(func() { chartURLTemplate = orig })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"chart":{"result":null,"error":{"code":"Bad Request","description":"oops"}}}`))
	}))
	t.Cleanup(srv.Close)
	chartURLTemplate = srv.URL + "/v8/finance/chart/%s?period1=%d&period2=%d&interval=1d"

	start, _ := time.ParseInLocation("2006-01-02", "2026-09-01", time.Local)
	end, _ := time.ParseInLocation("2006-01-02", "2026-09-03", time.Local)
	if _, err := fetchHistory(context.Background(), srv.Client(), channel{ticker: "TEST"}, start, end); err == nil {
		t.Fatal("want error for yahoo error payload")
	}
}

func TestMergeBarFillsAndSkips(t *testing.T) {
	dir := t.TempDir()
	ch := channel{field: "tsm_adr", symbol: "TSM", ticker: "TSM"}
	b := bar{date: "2026-09-01", value: 300.5, changePct: 1.25, ts: 1788192000}

	// 1) Missing file → filled, snapshot gains the point.
	action, err := mergeBar(dir, ch, b)
	if err != nil || action != "filled" {
		t.Fatalf("action=%s err=%v", action, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "2026-09-01.json"))
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	var pt struct {
		Symbol string  `json:"symbol"`
		Value  float64 `json:"value"`
	}
	if err := json.Unmarshal(snap["tsm_adr"], &pt); err != nil || pt.Value != 300.5 || pt.Symbol != "TSM" {
		t.Fatalf("point = %+v err=%v", pt, err)
	}

	// 2) Existing non-zero value → skipped, file untouched.
	before, _ := os.ReadFile(filepath.Join(dir, "2026-09-01.json"))
	action, err = mergeBar(dir, ch, bar{date: "2026-09-01", value: 999, changePct: -2})
	if err != nil || action != "skipped_existing" {
		t.Fatalf("action=%s err=%v", action, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "2026-09-01.json"))
	if string(before) != string(after) {
		t.Fatal("existing non-zero value was overwritten")
	}

	// 3) Existing zero-value tsm_adr with non-zero change_pct → skipped
	//    (live snapshots can carry change-only points).
	existing := map[string]interface{}{
		"tsm_adr": map[string]interface{}{"symbol": "TSM", "value": 0, "change_pct": 0.8, "timestamp": 1},
	}
	data, _ := json.Marshal(existing)
	_ = os.WriteFile(filepath.Join(dir, "2026-09-02.json"), data, 0o644)
	action, err = mergeBar(dir, ch, bar{date: "2026-09-02", value: 310, changePct: 0.8, ts: 2})
	if err != nil || action != "skipped_existing" {
		t.Fatalf("action=%s err=%v", action, err)
	}

	// 4) taiex zero-value point (no change_pct yet) → filled.
	tch := channel{field: "taiex", symbol: "^TWII", ticker: "^TWII"}
	action, err = mergeBar(dir, tch, bar{date: "2026-09-02", value: 23000, changePct: 0.4, ts: 3})
	if err != nil || action != "filled" {
		t.Fatalf("action=%s err=%v", action, err)
	}
	raw, _ = os.ReadFile(filepath.Join(dir, "2026-09-02.json"))
	_ = json.Unmarshal(raw, &snap)
	var taiexPt struct {
		Value float64 `json:"value"`
	}
	_ = json.Unmarshal(snap["taiex"], &taiexPt)
	if taiexPt.Value != 23000 {
		t.Fatalf("taiex point = %v", taiexPt.Value)
	}
}
