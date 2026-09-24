package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// ─── mock daily quote client ────────────────────────────────────────────────

// mockDailyQuoteClient implements dailyQuoteFetcher. failDates keys use the
// TWSE YYYYMMDD api date format; calls records every api date requested so
// tests can assert exactly which dates were (not) refetched.
type mockDailyQuoteClient struct {
	mu        sync.Mutex
	called    map[string]int // apiDateStr -> call count
	failDates map[string]bool
}

func newMockDailyQuoteClient() *mockDailyQuoteClient {
	return &mockDailyQuoteClient{
		called:    map[string]int{},
		failDates: map[string]bool{},
	}
}

func (m *mockDailyQuoteClient) GetDailyQuote(ctx context.Context, date, symbol string) (domain.Quote, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.called[date]++
	if m.failDates[date] {
		return domain.Quote{}, fmt.Errorf("mock: no data for %s %s", date, symbol)
	}
	// Price varies by day-of-month so validateRecord sees a non-zero change
	// from the previous close and does not spam WARN lines.
	day, _ := strconv.Atoi(date[6:])
	last := 100.0 + float64(day%10)*0.5
	return domain.Quote{
		Symbol: symbol,
		Last:   last,
		Open:   last * 0.995,
		High:   last * 1.01,
		Low:    last * 0.99,
		Volume: 15000000,
		Market: "TW",
	}, nil
}

func (m *mockDailyQuoteClient) calledDates() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	dates := make([]string, 0, len(m.called))
	for d := range m.called {
		dates = append(dates, d)
	}
	return dates
}

func (m *mockDailyQuoteClient) callCount(date string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called[date]
}

// ─── fixture helpers ────────────────────────────────────────────────────────

// writeFixtureCSV writes a replay CSV in the production format
// (Date,Code,Name,TradeVolume,Open,High,Low,Close) containing the given
// dates for the given symbols, and returns its path.
func writeFixtureCSV(t *testing.T, dates []string, symbols []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "replay.csv")
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	_ = w.Write([]string{"Date", "Code", "Name", "TradeVolume", "Open", "High", "Low", "Close"})
	for _, date := range dates {
		for _, sym := range symbols {
			_ = w.Write([]string{date, sym, "測試股", "15000000", "100.00", "101.00", "99.00", "100.50"})
		}
	}
	w.Flush()
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write fixture csv: %v", err)
	}
	return path
}

// loadDateKeys returns the set of "Date,Code" keys present in a CSV.
func loadDateKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	records, err := loadCSV(path)
	if err != nil {
		t.Fatalf("loadCSV: %v", err)
	}
	keys := make(map[string]bool, len(records))
	for _, r := range records {
		keys[r.Date+","+r.Code] = true
	}
	return keys
}

// countRowsForDate counts CSV rows matching a date.
func countRowsForDate(t *testing.T, path, date string) int {
	t.Helper()
	records, err := loadCSV(path)
	if err != nil {
		t.Fatalf("loadCSV: %v", err)
	}
	n := 0
	for _, r := range records {
		if r.Date == date {
			n++
		}
	}
	return n
}

// setLogOutput redirects the stdlib log used by daily-replay-sync to a
// buffer for assertions, restoring the previous output on cleanup.
func setLogOutput(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// ─── tests ──────────────────────────────────────────────────────────────────

// TestGapBackfillFillsOnlyMissingDates pins the core contract: after the
// daily sync, only dates missing from the CSV are refetched; dates already
// present are never refetched (no duplicates); non-trading days are skipped.
func TestGapBackfillFillsOnlyMissingDates(t *testing.T) {
	// Fixture covers 08-18/19/21; 08-20 (Thu, trading day) is the missing
	// middle date, 08-17 (Mon, trading day) is missing at the window edge.
	path := writeFixtureCSV(t,
		[]string{"2026-08-18", "2026-08-19", "2026-08-21"},
		[]string{"2330", "0050"},
	)
	mock := newMockDailyQuoteClient()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC) // Friday
	// Window = 08-17..08-21 (5 calendar days). 08-22/23 are outside; the
	// weekend is exercised by TestGapBackfillSkipsWeekendWithoutApiCalls.

	if err := runGapBackfill(path, 5, now, mock); err != nil {
		t.Fatalf("runGapBackfill: %v", err)
	}

	// Only the two missing trading days were requested.
	got := mock.calledDates()
	if len(got) != 2 {
		t.Fatalf("expected 2 api dates requested, got %v", got)
	}
	want := map[string]bool{"20260817": true, "20260820": true}
	for _, d := range got {
		if !want[d] {
			t.Errorf("unexpected api date requested: %s", d)
		}
	}
	if mock.callCount("20260818") != 0 || mock.callCount("20260819") != 0 || mock.callCount("20260821") != 0 {
		t.Errorf("existing dates were refetched: %v", mock.called)
	}

	symbols := orchestrator.DefaultSymbols()
	if len(symbols) == 0 {
		t.Fatal("DefaultSymbols returned empty")
	}
	// Missing dates now have a row per default symbol.
	for _, date := range []string{"2026-08-17", "2026-08-20"} {
		if n := countRowsForDate(t, path, date); n != len(symbols) {
			t.Errorf("date %s has %d rows, want %d", date, n, len(symbols))
		}
	}
	// Pre-existing dates keep exactly their original rows (no duplicates).
	for _, date := range []string{"2026-08-18", "2026-08-19", "2026-08-21"} {
		if n := countRowsForDate(t, path, date); n != 2 {
			t.Errorf("existing date %s grew to %d rows, want 2 (no duplicates)", date, n)
		}
	}
	// No duplicate (Date,Code) keys anywhere.
	keys := loadDateKeys(t, path)
	total := 0
	for range keys {
		total++
	}
	if total != 2*3+len(symbols)*2 {
		t.Errorf("unexpected total unique keys %d", total)
	}
}

// TestGapBackfillSkipsWeekendWithoutApiCalls verifies a Saturday in the
// window is treated as a non-trading day: no API calls, no CSV rows.
func TestGapBackfillSkipsWeekendWithoutApiCalls(t *testing.T) {
	path := writeFixtureCSV(t,
		[]string{"2026-08-17", "2026-08-18", "2026-08-19", "2026-08-21"},
		[]string{"2330", "0050"},
	)
	mock := newMockDailyQuoteClient()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) // Saturday
	// Window = 08-18..08-22. Present: 18/19/21. Missing: 08-20 (Thu).
	// 08-22 (Sat) must be skipped without hitting the API.

	if err := runGapBackfill(path, 5, now, mock); err != nil {
		t.Fatalf("runGapBackfill: %v", err)
	}

	dates := mock.calledDates()
	if len(dates) != 1 || dates[0] != "20260820" {
		t.Fatalf("expected only 20260820 requested, got %v", mock.called)
	}
	if mock.callCount("20260820") != len(orchestrator.DefaultSymbols()) {
		t.Errorf("20260820 got %d calls, want one per default symbol (%d)", mock.callCount("20260820"), len(orchestrator.DefaultSymbols()))
	}
	if mock.callCount("20260822") != 0 {
		t.Errorf("weekend date 2026-08-22 was fetched: %d calls", mock.callCount("20260822"))
	}
	if n := countRowsForDate(t, path, "2026-08-22"); n != 0 {
		t.Errorf("weekend date got %d rows, want 0", n)
	}
}

// TestGapBackfillFailedDayLoggedAndRetried verifies a day whose fetch fails
// entirely logs "[GapBackfill] failed X", does not abort the run, stays
// missing, and is retried (and filled) on the next run.
func TestGapBackfillFailedDayLoggedAndRetried(t *testing.T) {
	path := writeFixtureCSV(t,
		[]string{"2026-08-18", "2026-08-19", "2026-08-21"},
		[]string{"2330", "0050"},
	)
	mock := newMockDailyQuoteClient()
	mock.failDates["20260817"] = true // Monday 08-17 fails for all symbols
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	buf := setLogOutput(t)

	if err := runGapBackfill(path, 5, now, mock); err != nil {
		t.Fatalf("runGapBackfill (run 1): %v", err)
	}
	if !strings.Contains(buf.String(), "[GapBackfill] failed 2026-08-17") {
		t.Errorf("expected '[GapBackfill] failed 2026-08-17' in log, got:\n%s", buf.String())
	}
	// Failed day stays missing; the other missing day was still filled.
	if n := countRowsForDate(t, path, "2026-08-17"); n != 0 {
		t.Errorf("failed date 2026-08-17 has %d rows, want 0", n)
	}
	if n := countRowsForDate(t, path, "2026-08-20"); n != len(orchestrator.DefaultSymbols()) {
		t.Errorf("2026-08-20 has %d rows, want %d", n, len(orchestrator.DefaultSymbols()))
	}

	// Next run: API recovers, the still-missing date is backfilled.
	delete(mock.failDates, "20260817")
	if err := runGapBackfill(path, 5, now, mock); err != nil {
		t.Fatalf("runGapBackfill (run 2): %v", err)
	}
	if n := countRowsForDate(t, path, "2026-08-17"); n != len(orchestrator.DefaultSymbols()) {
		t.Errorf("2026-08-17 after retry has %d rows, want %d", n, len(orchestrator.DefaultSymbols()))
	}
}

// TestGapBackfillWindowDisabled verifies window <= 0 disables the backfill.
func TestGapBackfillWindowDisabled(t *testing.T) {
	path := writeFixtureCSV(t, []string{"2026-08-18"}, []string{"2330"})
	mock := newMockDailyQuoteClient()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	if err := runGapBackfill(path, 0, now, mock); err != nil {
		t.Fatalf("runGapBackfill(window=0): %v", err)
	}
	if len(mock.calledDates()) != 0 {
		t.Errorf("window=0 still fetched: %v", mock.calledDates())
	}
}

// ─── 2026-09-24: degraded sync must not be recorded as ok ───────────────────
//
// Production evidence: twse_replay_sync recorded
// `context deadline exceeded (Client.Timeout exceeded while awaiting headers)`
// on 2026-09-23T15:30Z (consecutive_failures=1 → derived status warn) because
// runDailySync used a 60s context while the TWSE client's retry policy needs
// 73s, and GetQuotes had no retry at all. The CSV was left untouched (correct
// degrade) but the channel legitimately stayed "warn" for a whole day.
// These tests pin the three behaviors that must hold:
//  1. a failed fetch keeps the previous CSV byte-for-byte and records a
//     NON-ok attempt with a message that says so;
//  2. a later successful run writes the day and clears the warn;
//  3. an OK response with zero usable rows is recorded degraded, never ok.

// twseHostRewriteTransport sends the hardcoded www.twse.com.tw request to a
// local httptest server.
type twseHostRewriteTransport struct{ target string }

func (t *twseHostRewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme = "http"
	r.URL.Host = strings.TrimPrefix(t.target, "http://")
	return http.DefaultTransport.RoundTrip(r)
}

// stubSharedTWSEClient points the shared TWSE client at target with an
// explicit, fast retry policy. It resets the singleton (fresh rate-limit
// bucket + closed breaker) before and after, so mutations cannot leak.
func stubSharedTWSEClient(t *testing.T, target string, attempts int) {
	t.Helper()
	marketdata.ResetSharedTWSEClient()
	prevLimiter := marketdata.SetTWSESharedLimiterForTest(rate.NewLimiter(rate.Inf, 0))
	c := marketdata.GetSharedTWSEClient()
	c.SetHTTPClient(&http.Client{Timeout: 2 * time.Second, Transport: &twseHostRewriteTransport{target: target}})
	c.SetRetryPolicyForTest(attempts, time.Millisecond, 0)
	c.SetPerAttemptTimeoutForTest(2 * time.Second)
	t.Cleanup(func() {
		marketdata.SetTWSESharedLimiterForTest(prevLimiter)
		marketdata.ResetSharedTWSEClient()
	})
}

// twseJSONResponse builds a STOCK_DAY_ALL (JSON variant) body for the given
// quote rows.
func twseJSONResponse(rows string) string {
	return `{"stat":"OK","date":"20260924","title":"上市個股日成交資訊",
		"fields":["Code","Name","TradeVolume","TradeValue","OpeningPrice","HighestPrice","LowestPrice","ClosingPrice","Change","Transaction"],
		"data":[` + rows + `]}`
}

const twseRow2330 = `["2330","台積電","81160741","15450000000","190","191.23","189.07","190.64","+0.50","35000"]`

// syncStatusOf reads the derived channel-health record the sync recorded.
func syncStatusOf(t *testing.T, csvPath string) *monitoring.ChannelHealthRecord {
	t.Helper()
	stateDir := filepath.Join(filepath.Dir(filepath.Dir(csvPath)), "state")
	rec := monitoring.NewChannelHealthStore(stateDir).Get("twse_replay_sync")
	if rec == nil {
		t.Fatal("twse_replay_sync was not recorded")
	}
	return rec
}

func TestRunDailySync_FailedFetchKeepsCSVAndRecordsNonOk(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"stat":"fail"}`))
	}))
	defer srv.Close()
	stubSharedTWSEClient(t, srv.URL, 1)
	setLogOutput(t)

	csvPath := writeFixtureCSV(t, []string{"2026-09-22"}, []string{"2330"})
	before, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	err = runDailySync(csvPath, nil)
	if err == nil {
		t.Fatal("runDailySync = nil error, want a failure for an upstream 502")
	}
	if !strings.Contains(err.Error(), "replay CSV left unchanged") {
		t.Errorf("error %q must state that the CSV was left unchanged (degrade, not silent success)", err.Error())
	}

	after, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("re-read csv: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("replay CSV changed on a failed fetch:\nbefore=%s\nafter=%s", before, after)
	}

	rec := syncStatusOf(t, csvPath)
	if rec.Status != "warn" {
		t.Errorf("status = %q, want warn (single failure: non-paging, but never ok)", rec.Status)
	}
	if rec.ConsecutiveFailures != 1 {
		t.Errorf("consecutive_failures = %d, want 1", rec.ConsecutiveFailures)
	}
	if rec.LastSuccessAt != "" {
		t.Errorf("last_success_at = %q, want empty (no data landed, so freshness must not advance)", rec.LastSuccessAt)
	}
	if !strings.Contains(rec.LastError, "replay CSV left unchanged") {
		t.Errorf("last_error = %q, want the degrade note", rec.LastError)
	}
}

func TestRunDailySync_SuccessAfterFailureClearsWarn(t *testing.T) {
	csvPath := writeFixtureCSV(t, []string{"2026-09-22"}, []string{"2330"})
	setLogOutput(t)

	// 1) A slow/erroring upstream records warn.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	stubSharedTWSEClient(t, bad.URL, 1)
	if err := runDailySync(csvPath, nil); err == nil {
		t.Fatal("first runDailySync = nil error, want the 503 failure")
	}
	if rec := syncStatusOf(t, csvPath); rec.Status != "warn" {
		t.Fatalf("status after failure = %q, want warn", rec.Status)
	}

	// 2) A healthy upstream must write the day and clear the warn — the
	//    "何時會恢復" question from the incident report.
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twseJSONResponse(twseRow2330)))
	}))
	defer good.Close()
	stubSharedTWSEClient(t, good.URL, 1)

	if err := runDailySync(csvPath, nil); err != nil {
		t.Fatalf("second runDailySync = error %v, want success", err)
	}

	today := time.Now().Format("2006-01-02")
	if got := countRowsForDate(t, csvPath, today); got != 1 {
		t.Errorf("rows for %s = %d, want 1", today, got)
	}
	rec := syncStatusOf(t, csvPath)
	if rec.Status != "ok" {
		t.Errorf("status after a success = %q, want ok (a success must clear the warn)", rec.Status)
	}
	if rec.ConsecutiveFailures != 0 {
		t.Errorf("consecutive_failures = %d, want 0", rec.ConsecutiveFailures)
	}
	if rec.LastError != "" {
		t.Errorf("last_error = %q, want empty after success", rec.LastError)
	}
	if rec.LastSuccessAt == "" {
		t.Error("last_success_at must advance on success")
	}
}

func TestRunDailySync_ZeroUsableRowsIsDegradedNotOk(t *testing.T) {
	// Upstream answers OK but nothing matches our symbol set (schema drift /
	// symbol rename): recording ok would show a healthy channel while the CSV
	// silently gained no row for the day.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twseJSONResponse(`["9999","不存在","100","100","10","11","9","10","+0.00","1"]`)))
	}))
	defer srv.Close()
	stubSharedTWSEClient(t, srv.URL, 1)
	setLogOutput(t)

	csvPath := writeFixtureCSV(t, []string{"2026-09-22"}, []string{"2330"})
	err := runDailySync(csvPath, nil)
	if err == nil {
		t.Fatal("runDailySync = nil error, want a failure when no target symbol was fetched")
	}
	if !strings.Contains(err.Error(), "none matched") {
		t.Errorf("error %q should explain that no replay symbol matched", err.Error())
	}
	rec := syncStatusOf(t, csvPath)
	if rec.Status == "ok" {
		t.Error("status = ok, want degraded (no data landed)")
	}
	if rec.Status != "degraded" {
		t.Errorf("status = %q, want degraded", rec.Status)
	}
	if rec.LastSuccessAt != "" {
		t.Errorf("last_success_at = %q, want empty", rec.LastSuccessAt)
	}
	if got := countRowsForDate(t, csvPath, time.Now().Format("2006-01-02")); got != 0 {
		t.Errorf("rows written = %d, want 0", got)
	}
}

func TestRunDailySyncContextBudgetExceedsLegacyLimit(t *testing.T) {
	// Guards the root cause: the deadline must be derived from the client's
	// retry policy, not hardcoded shorter than it needs.
	marketdata.ResetSharedTWSEClient()
	t.Cleanup(marketdata.ResetSharedTWSEClient)
	budget := marketdata.GetSharedTWSEClient().FetchBudget()
	if budget <= 60*time.Second {
		t.Errorf("FetchBudget() = %v, want > 60s (the removed hardcoded context)", budget)
	}
}
