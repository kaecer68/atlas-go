package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

// ─── deterministic clock ────────────────────────────────────────────────────
//
// 2026-09-24 is a Thursday trading day; 2026-09-25 is 中秋節 (Friday holiday)
// and 2026-09-26/27 are a weekend. Every write-path test must inject a trading
// day, otherwise runDailySync's closed-market guard skips it and the test
// asserts nothing (before the guard existed these tests silently depended on
// the wall clock, and 2 of every 7 days they cannot pass).

const (
	tradingDayDate = "2026-09-24"
	holidayDate    = "2026-09-25" // 中秋節
	weekendDate    = "2026-09-26" // Saturday
)

// at returns a time on `date` at 23:30 Taipei — the production cron slot
// (docker-compose.yml CRON 15:30 UTC = 23:30 Asia/Taipei).
func at(t *testing.T, date string) time.Time {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02 15:04", date+" 23:30", marketdata.TaiwanLocation())
	if err != nil {
		t.Fatalf("parse %s: %v", date, err)
	}
	return d
}

// syncStateDir mirrors the state directory runDailySync writes channel health
// to (two levels up from the CSV, then /state).
func syncStateDir(csvPath string) string {
	return filepath.Join(filepath.Dir(filepath.Dir(csvPath)), "state")
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

// ─── MI_INDEX fixture builders (the date-addressed source) ──────────────────
//
// Since 2026-09-26 runDailySync fetches the whole market from MI_INDEX with a
// date parameter and writes the DATA DATE the payload itself declares. These
// helpers build that shape; the production-shaped copy lives in
// testdata/mi_index_20260924_replay44.json (a real 2026-09-24 response trimmed
// to the 44 replay codes).

const miIndexFields = `["證券代號","證券名稱","成交股數","成交筆數","成交金額","開盤價","最高價","最低價","收盤價","漲跌(+/-)","漲跌價差","最後揭示買價","最後揭示買量","最後揭示賣價","最後揭示賣量","本益比"]`

// miIndexSyncResponse renders a MI_INDEX payload whose 每日收盤行情 title carries
// dataDate — the provenance stamp runDailySync guards on. `rows` are
// miIndexSyncRow values.
func miIndexSyncResponse(t *testing.T, dataDate string, rows ...string) string {
	t.Helper()
	d, err := time.ParseInLocation("2006-01-02", dataDate, marketdata.TaiwanLocation())
	if err != nil {
		t.Fatalf("miIndexSyncResponse date %q: %v", dataDate, err)
	}
	title := fmt.Sprintf("%d年%02d月%02d日", d.Year()-1911, int(d.Month()), d.Day())
	return fmt.Sprintf(`{"stat":"OK","date":%q,"tables":[
		{"title":%q,"fields":["指數","收盤指數"],"data":[["發行量加權股價指數","48,024.60"]]},
		{"title":%q,"fields":%s,"data":[%s]}]}`,
		d.Format("20060102"),
		title+" 價格指數(臺灣證券交易所)",
		title+" 每日收盤行情(全部(不含權證、牛熊證、可展延牛熊證))",
		miIndexFields, strings.Join(rows, ","))
}

// miIndexSyncRow renders one 每日收盤行情 row. The price varies with the code
// length so validateRecord does not spam WARN lines about a zero price change.
func miIndexSyncRow(code string) string {
	last := 100 + float64(len(code))
	return fmt.Sprintf(`[%q,"測試股","15,000,000","1,000","1,500,000","%.2f","%.2f","%.2f","%.2f","+","0.50","0","0","0","0","0"]`,
		code, last, last+1, last-1, last)
}

// syncClient returns the shared TWSE client the tests stubbed through
// stubSharedTWSEClient. runDailySync takes its fetcher as an argument so a test
// can also inject a fake.
func syncClient() dailySyncFetcher {
	return marketdata.GetSharedTWSEClient()
}

// syncStatusOf reads the derived channel-health record the sync recorded.
func syncStatusOf(t *testing.T, csvPath string) *monitoring.ChannelHealthRecord {
	t.Helper()
	rec := monitoring.NewChannelHealthStore(syncStateDir(csvPath)).Get("twse_replay_sync")
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

	err = runDailySync(csvPath, nil, at(t, tradingDayDate), syncClient())
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
	if err := runDailySync(csvPath, nil, at(t, tradingDayDate), syncClient()); err == nil {
		t.Fatal("first runDailySync = nil error, want the 503 failure")
	}
	if rec := syncStatusOf(t, csvPath); rec.Status != "warn" {
		t.Fatalf("status after failure = %q, want warn", rec.Status)
	}

	// 2) A healthy upstream must write the day and clear the warn — the
	//    "何時會恢復" question from the incident report.
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(miIndexSyncResponse(t, tradingDayDate, miIndexSyncRow("2330"))))
	}))
	defer good.Close()
	stubSharedTWSEClient(t, good.URL, 1)

	if err := runDailySync(csvPath, nil, at(t, tradingDayDate), syncClient()); err != nil {
		t.Fatalf("second runDailySync = error %v, want success", err)
	}

	if got := countRowsForDate(t, csvPath, tradingDayDate); got != 1 {
		t.Errorf("rows for %s = %d, want 1 (the injected exchange date, not the wall clock)", tradingDayDate, got)
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
		_, _ = w.Write([]byte(miIndexSyncResponse(t, tradingDayDate, miIndexSyncRow("9999"))))
	}))
	defer srv.Close()
	stubSharedTWSEClient(t, srv.URL, 1)
	setLogOutput(t)

	csvPath := writeFixtureCSV(t, []string{"2026-09-22"}, []string{"2330"})
	err := runDailySync(csvPath, nil, at(t, tradingDayDate), syncClient())
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
	if got := countRowsForDate(t, csvPath, tradingDayDate); got != 0 {
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

// TestRunDailySync_SkipsClosedMarket is the negative proof for the phantom-row
// defect (2026-09-26). The replay CSV's date column IS the replay trading
// calendar, so a row written on a day the exchange never traded is not "stale
// data" — it is a corrupted calendar. Production evidence: the CSV carried rows
// dated 2026-08-29, 08-30, 09-05, 09-06, 09-12, 09-13, 09-19, 09-20 and the
// 09-25 中秋節 — 44 symbols each — every one of them holding the 2026-09-24
// close, because STOCK_DAY_ALL ignores any notion of "which day" and
// runDailySync stamped the payload with time.Now().
func TestRunDailySync_SkipsClosedMarket(t *testing.T) {
	var calls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(twseJSONResponse(twseRow2330)))
	}))
	defer srv.Close()
	stubSharedTWSEClient(t, srv.URL, 1)
	logs := setLogOutput(t)

	for _, closed := range []string{weekendDate, holidayDate} {
		t.Run(closed, func(t *testing.T) {
			csvPath := writeFixtureCSV(t, []string{tradingDayDate}, []string{"2330"})
			before, err := os.ReadFile(csvPath)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			if err := runDailySync(csvPath, nil, at(t, closed), syncClient()); err != nil {
				t.Fatalf("runDailySync on closed market %s = %v, want nil (a closed market is not a failure)", closed, err)
			}

			after, err := os.ReadFile(csvPath)
			if err != nil {
				t.Fatalf("re-read csv: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Errorf("replay CSV changed on closed market %s:\nbefore=%s\nafter=%s", closed, before, after)
			}
			if got := atomic.LoadInt64(&calls); got != 0 {
				t.Errorf("HTTP requests on %s = %d, want 0 (a fetch cannot be scoped to the day, so it would return another day's rows)", closed, got)
			}
			if rec := monitoring.NewChannelHealthStore(syncStateDir(csvPath)).Get("twse_replay_sync"); rec != nil {
				t.Errorf("channel record written on %s (status=%q): a closed market must not advance LastSuccessAt", closed, rec.Status)
			}
		})
	}
	if !strings.Contains(logs.String(), "not a Taiwan trading day") {
		t.Errorf("logs must state that the day was skipped, got: %s", logs.String())
	}
}

// TestRunDailySync_WritesTheDataDateThePayloadDeclares pins the corrected
// writer contract: the Date column comes from the response, never from the
// clock. Before the fix the same run stamped the payload with time.Now() and
// produced a row dated 2026-09-26 holding the 2026-09-24 close.
func TestRunDailySync_WritesTheDataDateThePayloadDeclares(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(miIndexSyncResponse(t, tradingDayDate, miIndexSyncRow("2330"))))
	}))
	defer srv.Close()
	stubSharedTWSEClient(t, srv.URL, 1)
	setLogOutput(t)

	csvPath := writeFixtureCSV(t, []string{"2026-09-23"}, []string{"2330"})
	if err := runDailySync(csvPath, nil, at(t, tradingDayDate), syncClient()); err != nil {
		t.Fatalf("runDailySync: %v", err)
	}
	if got := countRowsForDate(t, csvPath, tradingDayDate); got != 1 {
		t.Errorf("rows for %s = %d, want 1 (the payload's own data date)", tradingDayDate, got)
	}
	rec := syncStatusOf(t, csvPath)
	if rec.Status != "ok" || rec.LastSuccessAt == "" {
		t.Errorf("channel record = status %q last_success_at %q, want ok with a timestamp", rec.Status, rec.LastSuccessAt)
	}
}

// TestRunDailySync_RefusesPayloadForAnotherDate is the negative proof for the
// response-date guard. A payload that describes 2026-09-17 while 2026-09-24 was
// requested is exactly what an always-latest upstream answers with (the
// pre-fix STOCK_DAY_ALL behavior) — storing it would date the 09-17 close as
// 09-24 and push the replay calendar onto a day the exchange never traded.
//
// Removing the guard makes this test fail: the CSV then gains a 2026-09-24 row
// carrying 09-17 prices and the channel records "ok".
func TestRunDailySync_RefusesPayloadForAnotherDate(t *testing.T) {
	const staleDate = "2026-09-17"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(miIndexSyncResponse(t, staleDate, miIndexSyncRow("2330"))))
	}))
	defer srv.Close()
	stubSharedTWSEClient(t, srv.URL, 1)
	setLogOutput(t)

	csvPath := writeFixtureCSV(t, []string{"2026-09-16"}, []string{"2330"})
	before, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	err = runDailySync(csvPath, nil, at(t, tradingDayDate), syncClient())
	if err == nil {
		t.Fatal("a payload for another date = nil error, want a refusal")
	}
	for _, want := range []string{staleDate, tradingDayDate, "refusing to write a mis-dated row"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must mention %q", err.Error(), want)
		}
	}

	after, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatalf("re-read csv: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("replay CSV changed on a mis-dated payload:\nbefore=%s\nafter=%s", before, after)
	}
	if got := countRowsForDate(t, csvPath, tradingDayDate); got != 0 {
		t.Errorf("rows written for %s = %d, want 0", tradingDayDate, got)
	}
	rec := syncStatusOf(t, csvPath)
	if rec.Status == "ok" {
		t.Error("status = ok although no data landed — a mis-dated payload must not look healthy")
	}
	if rec.LastSuccessAt != "" {
		t.Errorf("last_success_at = %q, want empty (nothing landed)", rec.LastSuccessAt)
	}
}

// TestRunDailySync_FiltersTheRealPayloadToTheReplayUniverse runs the sync
// against a real 2026-09-24 MI_INDEX response trimmed to the 44 replay codes
// (testdata/). It pins three things at once:
//   - the `tables` envelope is parsed (the pre-fix flat fields/data parser read
//     0 rows from this body),
//   - column naming maps onto the CSV schema (Date,Code,Name,TradeVolume,
//     Open,High,Low,Close),
//   - the replay-symbol filter still drops everything else (the fixture holds
//     44 codes; orchestrator.DefaultSymbols() is 41 here — the 3 CSV-only codes
//     1216/2357/3231 must not be written).
func TestRunDailySync_FiltersTheRealPayloadToTheReplayUniverse(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "mi_index_20260924_replay44.json"))
	if err != nil {
		t.Fatalf("read testdata payload: %v", err)
	}
	var fixture struct {
		Tables []struct {
			Fields []string   `json:"fields"`
			Data   [][]string `json:"data"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatalf("fixture is not valid JSON: %v", err)
	}
	var fixtureCodes []string
	for _, tbl := range fixture.Tables {
		for _, f := range tbl.Fields {
			if f == "證券代號" {
				for _, row := range tbl.Data {
					fixtureCodes = append(fixtureCodes, row[0])
				}
			}
		}
	}
	if len(fixtureCodes) != 44 {
		t.Fatalf("fixture carries %d codes, want the 44 replay codes", len(fixtureCodes))
	}
	if !marketdata.IsTaiwanTradingDay(at(t, tradingDayDate)) {
		t.Fatalf("%s must be a trading day for this fixture", tradingDayDate)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	stubSharedTWSEClient(t, srv.URL, 1)
	setLogOutput(t)

	csvPath := writeFixtureCSV(t, []string{"2026-09-23"}, []string{"2330"})
	if err := runDailySync(csvPath, nil, at(t, tradingDayDate), syncClient()); err != nil {
		t.Fatalf("runDailySync against the real payload: %v", err)
	}

	targets := orchestrator.DefaultSymbols()
	if got := countRowsForDate(t, csvPath, tradingDayDate); got != len(targets) {
		t.Errorf("rows for %s = %d, want %d (every replay symbol present in the 44-code payload)", tradingDayDate, got, len(targets))
	}
	written := make(map[string]bool)
	records, err := loadCSV(csvPath)
	if err != nil {
		t.Fatalf("loadCSV: %v", err)
	}
	for _, r := range records {
		if r.Date == tradingDayDate {
			written[r.Code] = true
		}
	}
	for _, sym := range targets {
		code := stripSuffix(sym)
		if !written[code] {
			t.Errorf("replay symbol %s missing from the CSV although the payload contains it", code)
		}
	}
	// Codes the payload has but the replay universe does not: dropped.
	for _, code := range []string{"1216", "2357", "3231"} {
		if written[code] {
			t.Errorf("non-replay code %s was written (44-code payload must still be filtered)", code)
		}
	}
	// Spot-check the mapping against the real payload (values are the ones the
	// live 2026-09-24 response carried).
	var got *csvRecord
	for i := range records {
		if records[i].Date == tradingDayDate && records[i].Code == "2330" {
			got = &records[i]
		}
	}
	if got == nil {
		t.Fatal("2330 row not written")
	}
	if got.TradeVolume != 14557662 || got.Open != 2480 || got.High != 2490 || got.Low != 2470 || got.Close != 2475 {
		t.Errorf("2330 row = %+v, want the live 2026-09-24 values (volume 14557662, O 2480, H 2490, L 2470, C 2475)", *got)
	}
}

// TestRunDailySync_SyntheticWeekWritesOnlyTradingDays is the phantom-row
// regression measurement, done entirely on fixtures (no production data is
// touched). A fake upstream mirrors MI_INDEX's real behaviour — a payload whose
// title date equals the requested date on trading days, the Chinese "no data"
// stat on closed ones — and is replayed across 2026-09-21..09-27, the week that
// contains the 09-25 中秋節 holiday and the 09-26/27 weekend production wrote
// phantoms for.
//
// The assertion that matters for "no phantom rows any more" is the second one:
// every written date satisfies marketdata.IsTaiwanTradingDay, which is the same
// authority cmd/clean-replay-weekends classifies with (classifyDate:
// weekend → holiday via IsTaiwanTradingDay → trading). A CSV in which no date
// fails that predicate is one the repo's own cleaning tool removes 0 rows from.
func TestRunDailySync_SyntheticWeekWritesOnlyTradingDays(t *testing.T) {
	var mu sync.Mutex
	requested := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		mu.Lock()
		requested[date]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		day, err := time.ParseInLocation("20060102", date, marketdata.TaiwanLocation())
		if err != nil || !marketdata.IsTaiwanTradingDay(day) {
			_, _ = w.Write([]byte(`{"stat":"很抱歉，沒有符合條件的資料!","type":"ALLBUT0999"}`))
			return
		}
		_, _ = w.Write([]byte(miIndexSyncResponse(t, day.Format("2006-01-02"), miIndexSyncRow("2330"))))
	}))
	defer srv.Close()
	stubSharedTWSEClient(t, srv.URL, 1)
	setLogOutput(t)

	csvPath := writeFixtureCSV(t, []string{"2026-09-18"}, []string{"2330"})
	var wrote []string
	for day := 21; day <= 27; day++ {
		date := fmt.Sprintf("2026-09-%02d", day)
		if err := runDailySync(csvPath, nil, at(t, date), syncClient()); err != nil {
			t.Fatalf("runDailySync(%s) = %v, want nil", date, err)
		}
		if countRowsForDate(t, csvPath, date) > 0 {
			wrote = append(wrote, date)
		}
	}

	want := []string{"2026-09-21", "2026-09-22", "2026-09-23", "2026-09-24"}
	if !reflect.DeepEqual(wrote, want) {
		t.Errorf("dates written = %v, want %v (09-25 中秋節 and the 09-26/27 weekend must not be written)", wrote, want)
	}
	for _, date := range wrote {
		day, err := time.ParseInLocation("2006-01-02", date, marketdata.TaiwanLocation())
		if err != nil || !marketdata.IsTaiwanTradingDay(day) {
			t.Errorf("phantom row written for non-trading day %s", date)
		}
	}

	// One token-bucket slot per trading day: closed days are refused before any
	// HTTP call, so the upstream never sees them.
	if len(requested) != len(want) {
		t.Errorf("upstream request dates = %v, want exactly %v", requested, want)
	}
	for _, date := range []string{"20260925", "20260926", "20260927"} {
		if n := requested[date]; n != 0 {
			t.Errorf("upstream called %d time(s) on closed day %s, want 0", n, date)
		}
	}
}
