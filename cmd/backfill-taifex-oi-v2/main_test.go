package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/text/encoding/traditionalchinese"
)

// csvBody builds a Big5-encoded TAIFEX per-contract CSV for one date.
func csvBody(date string, oiNet int64) []byte {
	utf8 := fmt.Sprintf(
		"日期,商品名稱,身份別,多方交易口數,多方交易契約金額(千元),空方交易口數,空方交易契約金額(千元),多空交易口數淨額,多空交易契約金額淨額(千元),多方未平倉口數,多方未平倉契約金額(千元),空方未平倉口數,空方未平倉契約金額(千元),多空未平倉口數淨額,多空未平倉契約金額淨額(千元)\n"+
			"%s,臺股期貨,自營商,3530,31723637,3851,34638601,-321,-2914964,5096,46031209,3397,30705944,1699,15325265\n"+
			"%s,臺股期貨,投信,540,4873561,49,442229,491,4431333,79713,719617079,3397,30666757,76316,688950322\n"+
			"%s,臺股期貨,外資及陸資,50987,457493063,51191,459352744,-204,-1859681,6735,60805243,89329,806446757,%d,-745641514\n",
		date, date, date, oiNet)
	b, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(utf8))
	if err != nil {
		panic(err)
	}
	return b
}

func big5CSVResponse(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "text/html;charset=MS950")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func noDataResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html;charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<html><body><script> alert("查無資料"); window.location.replace("futContractsDateView"); </script></body></html>`))
}

func TestParseCSV_ExtractsForeignOINet(t *testing.T) {
	body := csvBody("2024/07/01", -25944)
	text, err := decodeBody(body, "text/html;charset=MS950")
	if err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
	value, found, err := parseForeignOINetCSV(text)
	if err != nil {
		t.Fatalf("parseForeignOINetCSV: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true")
	}
	if value != -25944 {
		t.Errorf("value = %.0f, want -25944", value)
	}
}

func TestParseCSV_HeaderOnlyNoData(t *testing.T) {
	header := "日期,商品名稱,身份別,多方交易口數,多方交易契約金額(千元),空方交易口數,空方交易契約金額(千元),多空交易口數淨額,多空交易契約金額淨額(千元),多方未平倉口數,多方未平倉契約金額(千元),空方未平倉口數,空方未平倉契約金額(千元),多空未平倉口數淨額,多空未平倉契約金額淨額(千元)\n"
	_, found, err := parseForeignOINetCSV(header)
	if err != nil {
		t.Fatalf("parseForeignOINetCSV: %v", err)
	}
	if found {
		t.Fatal("found = true for header-only CSV, want false")
	}
}

func TestParseCSV_UnexpectedBody(t *testing.T) {
	_, _, err := parseForeignOINetCSV(`<html>404</html>`)
	if err == nil {
		t.Fatal("expected error for non-CSV body, got nil")
	}
	if !strings.Contains(err.Error(), "not CSV") {
		t.Errorf("error = %q, want 'not CSV' hint", err)
	}
}

func TestParseCSV_MissingForeignRow(t *testing.T) {
	utf8 := "日期,商品名稱,身份別,多方交易口數,多方交易契約金額(千元),空方交易口數,空方交易契約金額(千元),多空交易口數淨額,多空交易契約金額淨額(千元),多方未平倉口數,多方未平倉契約金額(千元),空方未平倉口數,空方未平倉契約金額(千元),多空未平倉口數淨額,多空未平倉契約金額淨額(千元)\n" +
		"2024/07/01,臺股期貨,自營商,1,2,3,4,5,6,7,8,9,10,11,12\n"
	_, found, err := parseForeignOINetCSV(utf8)
	if err != nil {
		t.Fatalf("parseForeignOINetCSV: %v", err)
	}
	if found {
		t.Fatal("found = true when 外資及陸資 row missing, want false")
	}
}

func TestDecodeBody_UTF8Passthrough(t *testing.T) {
	got, err := decodeBody([]byte("查無資料"), "text/html;charset=UTF-8")
	if err != nil {
		t.Fatalf("decodeBody: %v", err)
	}
	if got != "查無資料" {
		t.Errorf("got %q, want 查無資料", got)
	}
}

// realisticSnap returns a minimal-but-valid snapshot body with one key, as
// production snapshot files always have (merge appends new keys at the end).
func realisticSnap() string {
	return "{\n  \"taiex\": {\"symbol\":\"^TWII\",\"value\":44825.78,\"change_pct\":1.34,\"timestamp\":1700000000}\n}\n"
}

func writeSnap(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMergeSnapshot_WritesKeyPreservingBytes(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-07-01.json", "{\n  \"taiex\": {\"symbol\":\"^TWII\",\"value\":44825.78,\"change_pct\":1.34,\"timestamp\":1700000000}\n}\n")

	merged, overwrote, err := mergeSnapshot(dir, "2026-07-01", -25944, false)
	if err != nil {
		t.Fatalf("mergeSnapshot: %v", err)
	}
	if !merged {
		t.Fatal("merged = false, want true")
	}
	if overwrote {
		t.Fatal("overwrote = true for empty target, want false")
	}

	raw, err := os.ReadFile(filepath.Join(dir, "2026-07-01.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Original taiex bytes preserved untouched.
	if !strings.Contains(string(raw), "\"taiex\": {\"symbol\":\"^TWII\",\"value\":44825.78,\"change_pct\":1.34,\"timestamp\":1700000000}") {
		t.Errorf("original taiex bytes not preserved: %s", string(raw))
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	var pt macroDataPoint
	if err := json.Unmarshal(snap[snapshotField], &pt); err != nil {
		t.Fatal(err)
	}
	if pt.Symbol != snapshotSymbol || pt.Value != -25944 || pt.Timestamp == 0 {
		t.Errorf("point = %+v, want symbol=%s value=-25944 timestamp!=0", pt, snapshotSymbol)
	}
}

func TestMergeSnapshot_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-08-21.json",
		"{\n  \"foreign_futures_oi_net\": {\"symbol\":\"TX_FOREIGN_OI_NET\",\"value\":-82594,\"change_pct\":0,\"timestamp\":1786060800}\n}\n")

	merged, overwrote, err := mergeSnapshot(dir, "2026-08-21", -99999, false)
	if err != nil {
		t.Fatalf("mergeSnapshot: %v", err)
	}
	if merged {
		t.Fatal("merged = true for existing non-zero value, want refuse")
	}
	if overwrote {
		t.Fatal("overwrote = true without force")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "2026-08-21.json"))
	if !strings.Contains(string(raw), "-82594") {
		t.Errorf("existing value was clobbered: %s", string(raw))
	}
}

func TestMergeSnapshot_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-08-21.json",
		"{\n  \"foreign_futures_oi_net\": {\"symbol\":\"TX_FOREIGN_OI_NET\",\"value\":-82594,\"change_pct\":0,\"timestamp\":1786060800}\n}\n")

	merged, overwrote, err := mergeSnapshot(dir, "2026-08-21", -99999, true)
	if err != nil {
		t.Fatalf("mergeSnapshot: %v", err)
	}
	if !merged {
		t.Fatal("merged = false with force")
	}
	if !overwrote {
		t.Fatal("overwrote = false with force")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "2026-08-21.json"))
	if !strings.Contains(string(raw), "-99999") {
		t.Errorf("forced overwrite did not apply: %s", string(raw))
	}
}

// taifexStub serves Big5 CSV for trading days and 查無資料 for other weekdays.
type taifexStub struct {
	trading map[string]int64 // "2006-01-02" → foreign OI net
}

func (s *taifexStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		noDataResponse(w)
		return
	}
	// queryStartDate arrives as 2026/08/13
	raw := r.Form.Get("queryStartDate")
	normalized := strings.ReplaceAll(raw, "/", "-")
	if v, ok := s.trading[normalized]; ok {
		big5CSVResponse(w, csvBody(raw, v))
		return
	}
	noDataResponse(w)
}

func setupStub(t *testing.T, trading map[string]int64) string {
	t.Helper()
	srv := httptest.NewServer(&taifexStub{trading: trading})
	t.Cleanup(srv.Close)
	old := taifexFutDownURL
	t.Cleanup(func() { taifexFutDownURL = old })
	taifexFutDownURL = srv.URL
	return srv.URL
}

func readValue(t *testing.T, dir, date string) (float64, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, date+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var snap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatal(err)
	}
	rawPt, ok := snap[snapshotField]
	if !ok {
		return 0, false
	}
	var pt macroDataPoint
	if err := json.Unmarshal(rawPt, &pt); err != nil {
		t.Fatal(err)
	}
	return pt.Value, true
}

func TestRun_MergesTradingAndCarriesWeekend(t *testing.T) {
	root, dir := newTestRepo(t)
	// 2026-08-13 (Thu) and 2026-08-14 (Fri) are trading days; weekend follows.
	writeSnap(t, dir, "2026-08-13.json", realisticSnap())
	writeSnap(t, dir, "2026-08-14.json", realisticSnap())
	writeSnap(t, dir, "2026-08-15.json", realisticSnap())
	writeSnap(t, dir, "2026-08-16.json", realisticSnap())

	setupStub(t, map[string]int64{"2026-08-13": -86633, "2026-08-14": -86249})

	start, _ := time.Parse("2006-01-02", "2026-08-13")
	end, _ := time.Parse("2006-01-02", "2026-08-16")
	if err := run(config{workDir: root, start: start, end: end, pacing: 0, maxRetries: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}

	cases := map[string]float64{
		"2026-08-13": -86633, // fetched
		"2026-08-14": -86249, // fetched
		"2026-08-15": -86249, // Saturday carry-forward
		"2026-08-16": -86249, // Sunday carry-forward
	}
	for date, want := range cases {
		got, ok := readValue(t, dir, date)
		if !ok {
			t.Errorf("%s: field missing", date)
			continue
		}
		if got != want {
			t.Errorf("%s: value = %.0f, want %.0f", date, got, want)
		}
	}
}

func TestRun_WeekdayNoDataSkipsUnmarkedCarryForward(t *testing.T) {
	root, dir := newTestRepo(t)
	// 2026-06-19 is a weekday holiday in the stub (not in trading map).
	// H-CF-01 data-hygiene fix (2026-09-04): a weekday with no TAIFEX data
	// must NOT receive the previous session's OI unmarked — a holiday and a
	// not-yet-published trading day are indistinguishable at fetch time, and
	// the unmarked carry-forward is what produced macro(d)==FinMind(d−1) on
	// 19/33 overlap days. Weekday no-data now skips the merge entirely.
	writeSnap(t, dir, "2026-06-18.json", realisticSnap())
	writeSnap(t, dir, "2026-06-19.json", realisticSnap())
	setupStub(t, map[string]int64{"2026-06-18": -70000})

	start, _ := time.Parse("2006-01-02", "2026-06-18")
	end, _ := time.Parse("2006-01-02", "2026-06-19")
	if err := run(config{workDir: root, start: start, end: end, pacing: 0, maxRetries: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, ok := readValue(t, dir, "2026-06-19"); ok {
		t.Fatal("2026-06-19: field present, want NO unmarked weekday carry-forward (skip)")
	}
}

func TestRun_RefusesOverwriteExisting(t *testing.T) {
	root, dir := newTestRepo(t)
	writeSnap(t, dir, "2026-08-13.json",
		"{\n  \"foreign_futures_oi_net\": {\"symbol\":\"TX_FOREIGN_OI_NET\",\"value\":-100,\"change_pct\":0,\"timestamp\":1}\n}\n")
	setupStub(t, map[string]int64{"2026-08-13": -86633})

	start, _ := time.Parse("2006-01-02", "2026-08-13")
	end, _ := time.Parse("2006-01-02", "2026-08-13")
	if err := run(config{workDir: root, start: start, end: end, pacing: 0, maxRetries: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, ok := readValue(t, dir, "2026-08-13")
	if !ok || got != -100 {
		t.Errorf("existing value clobbered: got=%.0f ok=%v, want -100", got, ok)
	}
}

func TestRun_SingleDayFailureDoesNotAbort(t *testing.T) {
	root, dir := newTestRepo(t)
	writeSnap(t, dir, "2026-08-12.json", realisticSnap())
	writeSnap(t, dir, "2026-08-13.json", realisticSnap())

	stub := &taifexStub{trading: map[string]int64{"2026-08-13": -86633}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.ReplaceAll(r.FormValue("queryStartDate"), "/", "-")
		if raw == "2026-08-12" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		stub.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	old := taifexFutDownURL
	t.Cleanup(func() { taifexFutDownURL = old })
	taifexFutDownURL = srv.URL

	start, _ := time.Parse("2006-01-02", "2026-08-12")
	end, _ := time.Parse("2006-01-02", "2026-08-13")
	if err := run(config{workDir: root, start: start, end: end, pacing: 0, maxRetries: 1}); err != nil {
		t.Fatalf("run: %v", err)
	}
	// 08-12 failed (500) → must remain without the field; 08-13 must succeed.
	if _, ok := readValue(t, dir, "2026-08-12"); ok {
		t.Error("2026-08-12 got a value despite fetch failure")
	}
	got, ok := readValue(t, dir, "2026-08-13")
	if !ok || got != -86633 {
		t.Errorf("2026-08-13: got=%.0f ok=%v, want -86633", got, ok)
	}
}

func TestRun_DryRunDoesNotWrite(t *testing.T) {
	root, dir := newTestRepo(t)
	writeSnap(t, dir, "2026-08-13.json", realisticSnap())
	setupStub(t, map[string]int64{"2026-08-13": -86633})

	start, _ := time.Parse("2006-01-02", "2026-08-13")
	end, _ := time.Parse("2006-01-02", "2026-08-13")
	if err := run(config{workDir: root, start: start, end: end, pacing: 0, maxRetries: 1, dryRun: true}); err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, ok := readValue(t, dir, "2026-08-13"); ok {
		t.Error("dry-run wrote a value")
	}
}

func TestRun_BatchModeFetchesRangeOnce(t *testing.T) {
	root, dir := newTestRepo(t)
	// 2026-08-10 (Mon) .. 2026-08-14 (Fri) trading week; end is clamped so the
	// 7-day window cannot extend beyond the last available data.
	for _, d := range []string{"2026-08-10", "2026-08-11", "2026-08-12", "2026-08-13", "2026-08-14"} {
		writeSnap(t, dir, d+".json", realisticSnap())
	}

	requests := 0
	var mu sync.Mutex
	seenEnd := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		seenEnd = r.FormValue("queryEndDate")
		mu.Unlock()
		// Serve a multi-day CSV for the requested window.
		start := strings.ReplaceAll(r.FormValue("queryStartDate"), "/", "-")
		end := strings.ReplaceAll(r.FormValue("queryEndDate"), "/", "-")
		var sb strings.Builder
		sb.WriteString("日期,商品名稱,身份別,多方交易口數,多方交易契約金額(千元),空方交易口數,空方交易契約金額(千元),多空交易口數淨額,多空交易契約金額淨額(千元),多方未平倉口數,多方未平倉契約金額(千元),空方未平倉口數,空方未平倉契約金額(千元),多空未平倉口數淨額,多空未平倉契約金額淨額(千元)\n")
		values := map[string]int64{"2026-08-10": -89201, "2026-08-11": -88924, "2026-08-12": -86633, "2026-08-13": -86249, "2026-08-14": -85179}
		for d := start; d <= end; d = nextDate(d) {
			if v, ok := values[d]; ok {
				sb.WriteString(fmt.Sprintf("%s,臺股期貨,外資及陸資,1,2,3,4,5,6,7,8,9,10,%d,12\n", d, v))
			}
		}
		b, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(sb.String()))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		big5CSVResponse(w, b)
	}))
	t.Cleanup(srv.Close)
	old := taifexFutDownURL
	t.Cleanup(func() { taifexFutDownURL = old })
	taifexFutDownURL = srv.URL

	start, _ := time.Parse("2006-01-02", "2026-08-10")
	end, _ := time.Parse("2006-01-02", "2026-08-14")
	if err := run(config{workDir: root, start: start, end: end, pacing: 0, maxRetries: 1, batchDays: 7}); err != nil {
		t.Fatalf("run: %v", err)
	}
	mu.Lock()
	got := requests
	seenEndCopy := seenEnd
	mu.Unlock()
	if got != 1 {
		t.Errorf("requests = %d, want 1 (range fetch)", got)
	}
	// The batch window (7 days) must be clamped to the run end date
	// (2026-08-14): the server rejects ranges beyond the last available data.
	if seenEndCopy != "2026/08/14" {
		t.Errorf("queryEndDate = %q, want 2026/08/14 (clamped to run end)", seenEndCopy)
	}
	want := map[string]float64{
		"2026-08-10": -89201, "2026-08-11": -88924, "2026-08-12": -86633,
		"2026-08-13": -86249, "2026-08-14": -85179,
	}
	for date, w := range want {
		v, ok := readValue(t, dir, date)
		if !ok {
			t.Errorf("%s: field missing", date)
			continue
		}
		if v != w {
			t.Errorf("%s: value = %.0f, want %.0f", date, v, w)
		}
	}
}

// nextDate advances a YYYY-MM-DD string by one calendar day.
func nextDate(s string) string {
	d, _ := time.Parse("2006-01-02", s)
	return d.AddDate(0, 0, 1).Format("2006-01-02")
}

func TestCountNonZeroSnapshots(t *testing.T) {
	dir := t.TempDir()
	writeSnap(t, dir, "2026-08-13.json", "{\"foreign_futures_oi_net\":{\"symbol\":\"TX_FOREIGN_OI_NET\",\"value\":-86633,\"change_pct\":0,\"timestamp\":1}}\n")
	writeSnap(t, dir, "2026-08-14.json", "{\"foreign_futures_oi_net\":{\"symbol\":\"TX_FOREIGN_OI_NET\",\"value\":0,\"change_pct\":0,\"timestamp\":1}}\n")
	writeSnap(t, dir, "2026-08-15.json", realisticSnap())
	writeSnap(t, dir, "latest.json", "{\"foreign_futures_oi_net\":{\"symbol\":\"TX_FOREIGN_OI_NET\",\"value\":-1,\"change_pct\":0,\"timestamp\":1}}\n")

	nonzero, total, err := countNonZeroSnapshots(dir)
	if err != nil {
		t.Fatalf("countNonZeroSnapshots: %v", err)
	}
	if nonzero != 1 {
		t.Errorf("nonzero = %d, want 1", nonzero)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (latest.json excluded)", total)
	}
}

// newTestRepo creates a temp repo root with a data/state/macro dir and
// returns (repoRoot, macroDir).
func newTestRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	macroDir := filepath.Join(root, "data", "state", "macro")
	if err := os.MkdirAll(macroDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return root, macroDir
}
