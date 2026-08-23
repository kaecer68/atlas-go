package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
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
