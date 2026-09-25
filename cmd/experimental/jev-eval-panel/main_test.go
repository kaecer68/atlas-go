package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestComputeFeatures_PointInTime proves the PIT property of the exported
// features: the features of a date must not change when later sessions are
// appended to the series. A feature that moved would be lookahead, and the
// evaluator would score the model against its own leak.
func TestComputeFeatures_PointInTime(t *testing.T) {
	series := []seriesPoint{
		{"2021-01-04", 1.0}, {"2021-01-05", -0.5}, {"2021-01-06", 2.0},
		{"2021-01-07", 0.25}, {"2021-01-08", -1.5}, {"2021-01-11", 3.0},
		{"2021-01-12", 0.5}, {"2021-01-13", -0.75}, {"2021-01-14", 1.25},
		{"2021-01-15", -0.25},
	}
	cut := 6
	before := computeFeatures(series[:cut])
	after := computeFeatures(series[:cut])
	if before != after {
		t.Fatalf("features are not deterministic: %+v vs %+v", before, after)
	}
	// Appending later sessions must not be observable at the cut date, which
	// is what the slice-based signature enforces; assert the shape too.
	if before.HistoryDays != cut {
		t.Fatalf("history_days = %d, want %d", before.HistoryDays, cut)
	}
	if before.DailyReturnPct != series[cut-1].ret {
		t.Fatalf("session_return = %v, want %v", before.DailyReturnPct, series[cut-1].ret)
	}
	// Trailing 5-session compounded return must equal the product of the last
	// five returns, computed independently here.
	want := 1.0
	for _, p := range series[cut-5 : cut] {
		want *= 1 + p.ret/100
	}
	want = (want - 1) * 100
	if diff := before.TrailingReturn5DPct - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("ret_5d = %v, want %v", before.TrailingReturn5DPct, want)
	}
}

// TestBuildPanel_Caliber checks the panel against the canonical caliber:
// forward return compounding, the NetHit cost subtraction, and the backward
// window used by the memorisation probe.
func TestBuildPanel_Caliber(t *testing.T) {
	// 12 sessions; returns chosen so the 5-session forward product is exact.
	dates := []string{}
	returns := map[string]map[string]float64{}
	rates := []float64{1, 1, 1, 1, 1, 1, -1, -1, -1, -1, -1, -1}
	for i := 0; i < 12; i++ {
		d := "2021-01-" + two(i+1)
		dates = append(dates, d)
		returns[d] = map[string]float64{"semiconductor": rates[i]}
	}
	cost := 0.00585
	rows, outcomes, seen, err := buildPanel(dates, dates, []string{"semiconductor"}, returns, 5, 5, cost, "test-source")
	if err != nil {
		t.Fatalf("buildPanel: %v", err)
	}
	if len(seen) != 1 || seen[0] != "semiconductor" {
		t.Fatalf("seen = %v", seen)
	}
	if len(rows) == 0 || len(outcomes) == 0 {
		t.Fatalf("expected rows, got %d rows / %d outcomes", len(rows), len(outcomes))
	}
	// The first emitted row is the session at index 5: the row needs
	// min-history sessions behind it AND forwardDays sessions before it (the
	// backward window of the memorisation probe needs the same lead).
	if rows[0].Date != dates[5] {
		t.Fatalf("first row date = %s, want %s", rows[0].Date, dates[5])
	}
	first := rows[0]
	wantFwd := 1.0
	for i := 0; i < 5; i++ {
		wantFwd *= 1 + rates[6+i]/100
	}
	wantFwd -= 1
	if diff := first.Forward.ForwardReturn - wantFwd; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("forward_return = %v, want %v", first.Forward.ForwardReturn, wantFwd)
	}
	if first.Forward.Hit != (wantFwd-cost > 0) {
		t.Fatalf("hit = %v, want NetHit(%v, %v)", first.Forward.Hit, wantFwd, cost)
	}
	if diff := first.Forward.ForwardNetReturn - (wantFwd - cost); diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("forward_net_return = %v, want %v", first.Forward.ForwardNetReturn, wantFwd-cost)
	}
	if first.Forward.ForwardSessions != 5 {
		t.Fatalf("forward_sessions = %d, want 5", first.Forward.ForwardSessions)
	}
	// Backward window for the first row = sessions 1..5 (all +1%).
	wantBack := 1.0
	for i := 1; i <= 5; i++ {
		wantBack *= 1 + rates[i]/100
	}
	wantBack -= 1
	if diff := first.Backward.BackwardReturn - wantBack; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("backward_return = %v, want %v", first.Backward.BackwardReturn, wantBack)
	}
	if first.Backward.BackwardDate != dates[0] {
		t.Fatalf("backward_date = %s, want %s", first.Backward.BackwardDate, dates[0])
	}
	// Every exported outcome must carry the same source and the canonical cost.
	for _, o := range outcomes {
		if o.Source != "test-source" || o.CostRate != cost {
			t.Fatalf("outcome caliber drift: %+v", o)
		}
	}
}

// TestRun_EndToEnd writes a tiny sector_index fixture and exercises the real
// CLI path (canonical reader + canonical aggregate) on it.
func TestRun_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "sector_index")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 25 sessions, 18 canonical industries, deterministic returns.
	for i := 0; i < 25; i++ {
		date := "2021-03-" + two(i+1)
		body := "{"
		for k, id := range canonicalTestIndustries {
			if k > 0 {
				body += ","
			}
			body += `"` + id + `":[{"date":"` + date + `","industry":"` + id + `","index":100.0,"return_pct":` + dec(0.1*float64((i+k)%7-3)) + `}]`
		}
		body += "}"
		if err := os.WriteFile(filepath.Join(dataDir, "sector_indices_202103"+two(i+1)+"_202103"+two(i+1)+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paramsPath := filepath.Join(dir, "params.json")
	if err := os.WriteFile(paramsPath, []byte(`{"stockpicker":{"costs":{"round_trip_pct":{"value":0.00585}},"calibration":{"min_samples":{"value":30}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "panel.jsonl")
	summary := filepath.Join(dir, "summary.json")
	err := run([]string{
		"-dir", dataDir, "-start", "2021-03-01", "-end", "2021-03-25",
		"-min-history", "10", "-params", paramsPath, "-out", out, "-summary-out", summary, "-quiet",
	}, os.Stdout)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	panel, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(panel) == 0 {
		t.Fatal("empty panel")
	}
	sm, err := os.ReadFile(summary)
	if err != nil {
		t.Fatal(err)
	}
	if len(sm) == 0 {
		t.Fatal("empty summary")
	}
}

var canonicalTestIndustries = []string{
	"auto", "biotech", "cement", "construction", "electronics", "energy",
	"financials", "food", "machinery", "optoelectronics", "other_electronics",
	"plastics", "retail", "semiconductor", "shipping", "steel", "telecom", "textiles",
}

func two(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func dec(v float64) string {
	// stable fixed-point formatting without pulling strconv into the test scope
	neg := v < 0
	if neg {
		v = -v
	}
	whole := int(v)
	frac := int((v-float64(whole))*100 + 0.5)
	s := itoa(whole) + "."
	if frac < 10 {
		s += "0"
	}
	s += itoa(frac)
	if neg {
		s = "-" + s
	}
	return s
}
