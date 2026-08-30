package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ─── test helpers ────────────────────────────────────────────────────────────

// finmindIndexTestClient builds a FinMindClient whose HTTP traffic is
// redirected to the given handler, with an unlimited rate limiter and an
// isolated quota tracker (t.TempDir).
func finmindIndexTestClient(t *testing.T, handler http.Handler) *FinMindClient {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	c := NewFinMindClientWithStateDir("test-key", t.TempDir())
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	c.httpClient = &http.Client{
		Transport: &rewriteTransport{target: ts.URL, inner: http.DefaultTransport},
	}
	return c
}

// serveFinMind5sIndex returns a handler that serves canned
// TaiwanStockEvery5SecondsIndex rows per requested start_date. Unknown dates
// return an empty data array (like a non-trading day). Rows are keyed by
// date string; each entry is []finmind5sRow{stock_id, kind, time, price}.
type finmind5sRow struct {
	stockID string
	kind    string
	time    string
	price   float64
}

func serveFinMind5sIndex(byDate map[string][]finmind5sRow) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("start_date")
		rows := byDate[date]
		type row struct {
			Date    string  `json:"date"`
			Time    string  `json:"time"`
			StockID string  `json:"stock_id"`
			Price   float64 `json:"price"`
			Kind    string  `json:"kind"`
		}
		out := make([]row, 0, len(rows))
		for _, rr := range rows {
			out = append(out, row{Date: date, Time: rr.time, StockID: rr.stockID, Price: rr.price, Kind: rr.kind})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = writeFinmindJSON(w, map[string]any{
			"msg":    "success",
			"status": 200,
			"data":   out,
		})
	})
}

// finmindIndexDay builds one row per mapped series at 13:30:00, using the
// close values from prices (canonical ID → close).
func finmindIndexDay(prices map[string]float64) []finmind5sRow {
	// canonical → FinMind English series (inverse of finmindSectorSeries)
	seriesByCanonical := make(map[string]string, len(finmindSectorSeries))
	for series, canonical := range finmindSectorSeries {
		seriesByCanonical[canonical] = series
	}
	rows := make([]finmind5sRow, 0, len(prices))
	for canonical, price := range prices {
		series, ok := seriesByCanonical[canonical]
		if !ok {
			continue
		}
		rows = append(rows, finmind5sRow{stockID: series, kind: "twse", time: "13:30:00", price: price})
	}
	return rows
}

// ─── mapping table ───────────────────────────────────────────────────────────

func TestFinMindSectorSeriesMapping(t *testing.T) {
	if len(finmindSectorSeries) != 18 {
		t.Fatalf("finmindSectorSeries has %d entries, want 18", len(finmindSectorSeries))
	}
	seen := make(map[string]string)
	for series, canonical := range finmindSectorSeries {
		if !canonicalSectorIDs[canonical] {
			t.Errorf("series %q maps to %q which is not in canonicalSectorIDs", series, canonical)
		}
		if prev, dup := seen[canonical]; dup {
			t.Errorf("canonical %q duplicated by series %q and %q", canonical, prev, series)
		}
		seen[canonical] = series
	}
	// Every canonical 18 ID must be covered exactly once.
	for id := range canonicalSectorIDs {
		if _, ok := seen[id]; !ok {
			t.Errorf("canonical sector %q has no FinMind series mapping", id)
		}
	}
}

// ─── fetch behavior ──────────────────────────────────────────────────────────

func TestFinMindSectorIndexProvider_FetchTwoDays(t *testing.T) {
	day1 := map[string]float64{
		"semiconductor": 400.0, "electronics": 800.0, "shipping": 250.0, "financials": 1400.0,
		"auto": 320.0, "biotech": 70.0, "cement": 200.0, "construction": 340.0,
		"energy": 135.0, "food": 1900.0, "machinery": 240.0, "optoelectronics": 45.0,
		"other_electronics": 100.0, "plastics": 280.0, "retail": 300.0, "steel": 170.0,
		"telecom": 130.0, "textiles": 680.0,
	}
	day2 := make(map[string]float64, len(day1))
	for k, v := range day1 {
		day2[k] = v * 1.02 // +2% across the board
	}
	handler := serveFinMind5sIndex(map[string][]finmind5sRow{
		"2021-06-11": finmindIndexDay(day1), // previous Friday (seeds the cache)
		"2021-06-14": finmindIndexDay(day1),
		"2021-06-15": finmindIndexDay(day2),
	})
	p := NewFinMindSectorIndexProviderWithClient(finmindIndexTestClient(t, handler))

	ctx := context.Background()
	result, err := p.FetchSectorIndices(ctx, mustParseDay(t, "2021-06-14"), mustParseDay(t, "2021-06-15"))
	if err != nil {
		t.Fatalf("FetchSectorIndices: %v", err)
	}

	if len(result) != 18 {
		t.Fatalf("got %d industries, want 18: %v", len(result), industryKeys(result))
	}

	// 06-14 return vs 06-11 (seeded prev): 0%.
	// 06-15 return vs 06-14: +2%.
	for industry, series := range result {
		if len(series) != 2 {
			t.Errorf("%s: expected 2 data points, got %d", industry, len(series))
			continue
		}
		if series[0].Date != "2021-06-14" || series[1].Date != "2021-06-15" {
			t.Errorf("%s: unexpected dates %s, %s", industry, series[0].Date, series[1].Date)
		}
		if series[0].ReturnPct != 0 {
			t.Errorf("%s: 06-14 return_pct = %v, want 0 (first fetched day)", industry, series[0].ReturnPct)
		}
		if got := round2(series[1].ReturnPct); got != 2.0 {
			t.Errorf("%s: 06-15 return_pct = %v, want 2.0", industry, series[1].ReturnPct)
		}
		if series[1].Index != round2(day2[industry]) {
			t.Errorf("%s: 06-15 index = %v, want %v", industry, series[1].Index, day2[industry])
		}
	}
}

func TestFinMindSectorIndexProvider_WalkBackFindsPrevDay(t *testing.T) {
	day1 := map[string]float64{"semiconductor": 400.0}
	day2 := map[string]float64{"semiconductor": 408.0}
	handler := serveFinMind5sIndex(map[string][]finmind5sRow{
		"2021-06-14": finmindIndexDay(day1),
		"2021-06-15": finmindIndexDay(day2),
	})
	p := NewFinMindSectorIndexProviderWithClient(finmindIndexTestClient(t, handler))

	// Cold start: fetching only 06-15 must walk back to 06-14 for the return.
	result, err := p.FetchSectorIndices(context.Background(), mustParseDay(t, "2021-06-15"), mustParseDay(t, "2021-06-15"))
	if err != nil {
		t.Fatalf("FetchSectorIndices: %v", err)
	}
	semis := result["semiconductor"]
	if len(semis) != 1 {
		t.Fatalf("semiconductor: got %d points, want 1", len(semis))
	}
	if got := round2(semis[0].ReturnPct); got != 2.0 {
		t.Errorf("return_pct = %v, want 2.0 (computed against walked-back 06-14)", semis[0].ReturnPct)
	}
}

func TestFinMindSectorIndexProvider_LastPrintWins(t *testing.T) {
	rows := []finmind5sRow{
		{stockID: "Semiconductor", kind: "twse", time: "09:00:00", price: 400.0},
		{stockID: "Semiconductor", kind: "twse", time: "13:25:00", price: 415.0},
		{stockID: "Semiconductor", kind: "twse", time: "13:30:00", price: 418.45},
		{stockID: "Semiconductor", kind: "tpex", time: "13:30:00", price: 999.0},    // tpex must be dropped
		{stockID: "TAIEX", kind: "twse", time: "13:30:00", price: 17371.29},         // unmapped
		{stockID: "NonFinanceSubIndex", kind: "twse", time: "13:30:00", price: 5.0}, // unmapped
	}
	handler := serveFinMind5sIndex(map[string][]finmind5sRow{"2021-06-15": rows})
	p := NewFinMindSectorIndexProviderWithClient(finmindIndexTestClient(t, handler))

	result, err := p.FetchSectorIndices(context.Background(), mustParseDay(t, "2021-06-15"), mustParseDay(t, "2021-06-15"))
	if err != nil {
		t.Fatalf("FetchSectorIndices: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d industries, want 1 (only semiconductor mapped): %v", len(result), industryKeys(result))
	}
	if got := result["semiconductor"][0].Index; got != 418.45 {
		t.Errorf("semiconductor index = %v, want 418.45 (last twse print)", got)
	}
}

func TestFinMindSectorIndexProvider_SkipsHoliday(t *testing.T) {
	// 2021-06-14 was a real Taiwan holiday (端午節) — serve empty; 06-15 data.
	rows := finmindIndexDay(map[string]float64{"semiconductor": 400.0})
	handler := serveFinMind5sIndex(map[string][]finmind5sRow{"2021-06-15": rows})
	p := NewFinMindSectorIndexProviderWithClient(finmindIndexTestClient(t, handler))

	result, err := p.FetchSectorIndices(context.Background(), mustParseDay(t, "2021-06-14"), mustParseDay(t, "2021-06-15"))
	if err != nil {
		t.Fatalf("FetchSectorIndices: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d industries, want 1", len(result))
	}
	if result["semiconductor"][0].Date != "2021-06-15" {
		t.Errorf("expected only 2021-06-15 data, got %s", result["semiconductor"][0].Date)
	}
}

func TestFinMindSectorIndexProvider_Name(t *testing.T) {
	p := NewFinMindSectorIndexProviderWithClient(NewFinMindClient("k"))
	if p.Name() != "finmind_sector_index" {
		t.Errorf("Name() = %q, want finmind_sector_index", p.Name())
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func mustParseDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return d
}

func writeFinmindJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}
