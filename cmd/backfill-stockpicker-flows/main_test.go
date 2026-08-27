package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// t86RoundTripper rewrites requests to the httptest server (same pattern as
// the marketdata package tests) so the provider hits the stub instead of
// the real TWSE endpoint.
type t86RoundTripper struct{ serverURL string }

func (rt *t86RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	u := req.URL
	parsed, err := url.Parse(rt.serverURL)
	if err != nil {
		return nil, err
	}
	u.Scheme = parsed.Scheme
	u.Host = parsed.Host
	return http.DefaultTransport.RoundTrip(req)
}

// stubProvider builds a provider pointed at a stub server with an unlimited
// rate limiter (tests must not be paced by the production shared bucket).
func stubProvider(t *testing.T, serverURL string) *marketdata.TWSECapitalFlowProvider {
	t.Helper()
	p := marketdata.NewTWSECapitalFlowProvider("")
	p.SetHTTPClient(&http.Client{Transport: &t86RoundTripper{serverURL: serverURL}})
	p.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))
	return p
}

// t86Server serves a fake T86 response; rowsByDate keys are YYYYMMDD and a
// missing key returns an empty-data response (holiday-like). Each row is a
// 19-column T86 record with row[4] = foreign-investor net share count.
func t86Server(rowsByDate map[string][][]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rows, ok := rowsByDate[r.URL.Query().Get("date")]
		if !ok {
			fmt.Fprint(w, `{"stat":"OK","data":[]}`)
			return
		}
		data, _ := json.Marshal(rows)
		fmt.Fprintf(w, `{"stat":"OK","data":%s}`, data)
	}))
}

// t86Row builds a 19-column T86 row for symbol/name with the given
// foreign-investor net raw count in column 4 (values divided by 1e3).
func t86Row(symbol, name string, foreignNet int) []string {
	return []string{
		symbol, name,
		"1000", "500", fmt.Sprintf("%d", foreignNet), // 2..4: foreign
		"0", "0", "0", // 5..7: foreign dealer
		"800", "300", "500", // 8..10: domestic fund
		"400", "0", "0", "0", "0", "0", "0", // 11..17: dealer
		"1400", // 18: total
	}
}

// testConfig builds a run config with the given workdir and window.
func testConfig(t *testing.T, workdir, start, end string, minRows int, p *marketdata.TWSECapitalFlowProvider) config {
	t.Helper()
	st, err := time.ParseInLocation("2006-01-02", start, time.Local)
	if err != nil {
		t.Fatalf("parse start: %v", err)
	}
	en, err := time.ParseInLocation("2006-01-02", end, time.Local)
	if err != nil {
		t.Fatalf("parse end: %v", err)
	}
	return config{workDir: workdir, start: st, end: en, minRows: minRows, provider: p}
}

func readFlowFileAt(t *testing.T, dir, symbol string) (flowFile, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, symbol+".json"))
	if err != nil {
		return flowFile{}, err
	}
	var f flowFile
	if err := json.Unmarshal(data, &f); err != nil {
		return flowFile{}, err
	}
	return f, nil
}

func flowsDirOf(t *testing.T, workdir string) string {
	t.Helper()
	return filepath.Join(workdir, "data", "state", "stock_flows")
}

// readFlowFile decodes a written symbol file for assertions.
func readFlowFile(t *testing.T, path string) flowFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var f flowFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	return f
}

// TestRun_MergeAndIdempotent: two weekday fetches merge into per-symbol
// files, sorted by date; a rerun adds no duplicates (idempotent).
func TestRun_MergeAndIdempotent(t *testing.T) {
	server := t86Server(map[string][][]string{
		"20260105": {t86Row("2330", "台積電", 500), t86Row("2317", "鴻海", 700), t86Row("0050", "元大台灣50", 300)},
		"20260106": {t86Row("2330", "台積電", 600), t86Row("2317", "鴻海", 800), t86Row("0050", "元大台灣50", 400)},
	})
	defer server.Close()
	p := stubProvider(t, server.URL)

	workdir := t.TempDir()
	cfg := testConfig(t, workdir, "2026-01-05", "2026-01-06", 2, p)
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("first run: %v", err)
	}

	dir := flowsDirOf(t, workdir)
	for _, sym := range []string{"2330", "2317", "0050"} {
		if _, err := os.Stat(filepath.Join(dir, sym+".json")); err != nil {
			t.Fatalf("%s.json missing after run: %v", sym, err)
		}
	}

	f := readFlowFile(t, filepath.Join(dir, "2330.json"))
	if f.Symbol != "2330" || len(f.Flows) != 2 {
		t.Fatalf("2330 file = symbol %q flows %d, want 2330 with 2 flows", f.Symbol, len(f.Flows))
	}
	if f.Flows[0].Date != "2026-01-05" || f.Flows[0].ForeignNet != 0.5 {
		t.Fatalf("flow[0] = %+v, want 2026-01-05 / 0.5", f.Flows[0])
	}
	if f.Flows[1].Date != "2026-01-06" || f.Flows[1].ForeignNet != 0.6 {
		t.Fatalf("flow[1] = %+v, want 2026-01-06 / 0.6", f.Flows[1])
	}

	// Idempotent rerun: same dates, no duplicates.
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("rerun: %v", err)
	}
	f2 := readFlowFile(t, filepath.Join(dir, "2330.json"))
	if len(f2.Flows) != 2 {
		t.Fatalf("rerun duplicated flows: got %d, want 2", len(f2.Flows))
	}
	if f2.Flows[0].Date != "2026-01-05" || f2.Flows[1].Date != "2026-01-06" {
		t.Fatalf("rerun flow dates = %s,%s; want sorted 01-05,01-06", f2.Flows[0].Date, f2.Flows[1].Date)
	}
}

// TestRun_ZeroRowsFails: a weekday whose response parses to zero rows (stat
// OK, no holiday message) is a hard failure → run returns error (main would
// exit non-zero).
func TestRun_ZeroRowsFails(t *testing.T) {
	// stat OK with data, but every row is malformed (< 12 columns) → 0 rows.
	server := t86Server(map[string][][]string{
		"20260105": {{"0056"}},
	})
	defer server.Close()
	p := stubProvider(t, server.URL)

	workdir := t.TempDir()
	cfg := testConfig(t, workdir, "2026-01-05", "2026-01-05", 2, p)
	err := run(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error for 0-row trading day, got nil")
	}
	if !strings.Contains(err.Error(), "min-rows") {
		t.Fatalf("error %q does not mention min-rows", err)
	}
	entries, err := os.ReadDir(flowsDirOf(t, workdir))
	if err != nil {
		t.Fatalf("read flows dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("no flow files should be written on failure, got %d entries", len(entries))
	}
}

// TestRun_NoDataSkips: TWSE returning no data (non-trading day message) is
// skipped, not a failure.
// TestRun_AllSkippedFails (PR review P0): when the whole window yields no
// data (systematic stat != OK / invalid range / every day ErrNoData), the
// run must fail loudly instead of reporting a fake success with 0 files.
func TestRun_AllSkippedFails(t *testing.T) {
	server := t86Server(nil) // every date → empty data → ErrNoData
	defer server.Close()
	p := stubProvider(t, server.URL)

	workdir := t.TempDir()
	cfg := testConfig(t, workdir, "2026-01-05", "2026-01-06", 2, p)
	if err := run(context.Background(), cfg); err == nil {
		t.Fatal("all-skipped window must fail (fake success gate), got nil error")
	}
	entries, err := os.ReadDir(flowsDirOf(t, workdir))
	if err != nil {
		t.Fatalf("read flows dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("skipped days must not write files, got %d entries", len(entries))
	}
}

// TestRun_MixedSkipAndOK: a window with some no-data days and some real data
// succeeds and writes only the real-data days (skip is per-day, not whole-run).
func TestRun_MixedSkipAndOK(t *testing.T) {
	server := t86Server(map[string][][]string{
		"20260105": {t86Row("2330", "台積電", 500)}, // Mon: real data
		// 20260106 Tue: missing key → empty data → skip
		"20260107": {t86Row("2330", "台積電", 600)}, // Wed: real data
	})
	defer server.Close()
	p := stubProvider(t, server.URL)

	workdir := t.TempDir()
	cfg := testConfig(t, workdir, "2026-01-05", "2026-01-07", 1, p)
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("mixed skip+ok should succeed, got error: %v", err)
	}
	f, err := readFlowFileAt(t, flowsDirOf(t, workdir), "2330")
	if err != nil {
		t.Fatalf("read 2330 flow file: %v", err)
	}
	if len(f.Flows) != 2 {
		t.Fatalf("expected 2 flow points (Mon+Wed), got %d", len(f.Flows))
	}
}

// TestRun_DryRun: -dry-run prints the date list and writes nothing.
func TestRun_DryRun(t *testing.T) {
	server := t86Server(map[string][][]string{"20260105": {t86Row("2330", "台積電", 500)}})
	defer server.Close()
	p := stubProvider(t, server.URL)

	workdir := t.TempDir()
	cfg := testConfig(t, workdir, "2026-01-05", "2026-01-09", 2, p) // Mon..Fri
	cfg.dryRun = true
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if _, err := os.Stat(flowsDirOf(t, workdir)); !os.IsNotExist(err) {
		t.Fatal("dry-run must not create the flows directory")
	}
}

// TestRun_SymbolsFilter: -symbols restricts which symbol files are written.
func TestRun_SymbolsFilter(t *testing.T) {
	server := t86Server(map[string][][]string{
		"20260105": {t86Row("2330", "台積電", 500), t86Row("2317", "鴻海", 700), t86Row("0050", "元大台灣50", 300)},
	})
	defer server.Close()
	p := stubProvider(t, server.URL)

	workdir := t.TempDir()
	cfg := testConfig(t, workdir, "2026-01-05", "2026-01-05", 2, p)
	cfg.symbols = map[string]bool{"2330": true}
	if err := run(context.Background(), cfg); err != nil {
		t.Fatalf("run: %v", err)
	}

	dir := flowsDirOf(t, workdir)
	if _, err := os.Stat(filepath.Join(dir, "2330.json")); err != nil {
		t.Fatalf("2330.json should exist: %v", err)
	}
	for _, sym := range []string{"2317", "0050"} {
		if _, err := os.Stat(filepath.Join(dir, sym+".json")); !os.IsNotExist(err) {
			t.Fatalf("%s.json should be filtered out", sym)
		}
	}
}

// TestWeekdays skips weekends and keeps the window inclusive.
func TestWeekdays(t *testing.T) {
	start := time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local) // Fri
	end := time.Date(2026, 1, 9, 0, 0, 0, 0, time.Local)   // Fri
	days := weekdays(start, end)
	if len(days) != 6 { // Fri, Mon, Tue, Wed, Thu, Fri
		t.Fatalf("weekdays = %d, want 6 (weekends skipped)", len(days))
	}
	for _, d := range days {
		if wd := d.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("weekday %s is a weekend", d.Format("2006-01-02"))
		}
	}
}
