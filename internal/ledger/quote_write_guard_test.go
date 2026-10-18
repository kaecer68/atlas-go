package ledger

import (
	"bytes"
	"database/sql"
	"errors"
	"log/slog"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// quoteGuardCapture installs a WARN-capturing logger for the duration of one
// test. The guard's warn counter is reset here because it is a process-wide
// global shared by every store implementation.
func quoteGuardCapture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prevLogger := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	quoteRejectionWarn.mu.Lock()
	prevSeen := quoteRejectionWarn.seen
	quoteRejectionWarn.seen = 0
	quoteRejectionWarn.mu.Unlock()

	t.Cleanup(func() {
		logging.SetLogger(prevLogger)
		quoteRejectionWarn.mu.Lock()
		quoteRejectionWarn.seen = prevSeen
		quoteRejectionWarn.mu.Unlock()
	})
	return &buf
}

func newTestSQLiteQuoteStore(t *testing.T) *SQLiteQuoteStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return NewSQLiteQuoteStore(db)
}

// TestQuoteWriteRejectReason pins the guard predicate: a non-positive or
// non-finite close is refused (FU-20260930-03), non-finite open/high/low are
// refused, and a zero open/high/low with a usable close is KEPT (the production
// cron quote backfill builds DailyBar{Close, Volume} only, so refusing those
// rows would discard a usable close).
func TestQuoteWriteRejectReason(t *testing.T) {
	cases := []struct {
		name       string
		bar        domain.DailyBar
		wantOK     bool
		wantReason string
	}{
		{"normal bar", domain.DailyBar{Open: 100, High: 101, Low: 99, Close: 100.5}, true, ""},
		{"close zero is the documented bad value", domain.DailyBar{Close: 0}, false, QuoteInvalidReasonNonPositive},
		{"negative close", domain.DailyBar{Close: -5}, false, QuoteInvalidReasonNonPositive},
		{"nan close", domain.DailyBar{Close: math.NaN()}, false, QuoteInvalidReasonNonFinite},
		{"+inf close", domain.DailyBar{Close: math.Inf(1)}, false, QuoteInvalidReasonNonFinite},
		{"-inf close", domain.DailyBar{Close: math.Inf(-1)}, false, QuoteInvalidReasonNonFinite},
		{"nan open", domain.DailyBar{Open: math.NaN(), Close: 100}, false, QuoteInvalidReasonNonFinite},
		{"inf high", domain.DailyBar{High: math.Inf(1), Close: 100}, false, QuoteInvalidReasonNonFinite},
		{"nan low", domain.DailyBar{Low: math.NaN(), Close: 100}, false, QuoteInvalidReasonNonFinite},
		{"zero open with usable close is kept", domain.DailyBar{Open: 0, High: 0, Low: 0, Close: 100}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason, ok := quoteWriteRejectReason(tc.bar)
			if ok != tc.wantOK {
				t.Fatalf("quoteWriteRejectReason ok = %v, want %v", ok, tc.wantOK)
			}
			if reason != tc.wantReason {
				t.Fatalf("quoteWriteRejectReason reason = %q, want %q", reason, tc.wantReason)
			}
		})
	}
}

// TestSQLiteQuoteStoreWriteGuard covers the three cases the write-path guard
// must distinguish: a normal row still writes, close=0 is refused (and counted
// + logged), NaN is refused, and one bad row never aborts the rest of the batch.
func TestSQLiteQuoteStoreWriteGuard(t *testing.T) {
	logs := quoteGuardCapture(t)
	store := newTestSQLiteQuoteStore(t)

	day := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	batch := []domain.DailyBar{
		{Symbol: "2330", Date: day, Open: 900, High: 910, Low: 890, Close: 905, Volume: 1000, Source: "fugle"},
		{Symbol: "2330", Date: day.AddDate(0, 0, 1), Open: 0, High: 0, Low: 0, Close: 0, Volume: 0, Source: "finmind"},
		{Symbol: "2330", Date: day.AddDate(0, 0, 2), Open: 910, High: 920, Low: 900, Close: math.NaN(), Volume: 1200, Source: "fugle"},
		// Intraday columns unknown but the close is usable: must be written.
		{Symbol: "2330", Date: day.AddDate(0, 0, 3), Close: 915, Volume: 1300, Source: "cron_backfill"},
	}

	if err := store.RecordQuotes(batch); err != nil {
		t.Fatalf("RecordQuotes with a partially invalid batch must not fail: %v", err)
	}

	got, err := store.LoadQuotes("2330", day, day.AddDate(0, 0, 10))
	if err != nil {
		t.Fatalf("LoadQuotes: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 stored rows (valid close only), got %d: %+v", len(got), got)
	}
	if got[0].Close != 905 || got[1].Close != 915 {
		t.Fatalf("stored closes = %v, %v; want 905, 915", got[0].Close, got[1].Close)
	}

	logged := logs.String()
	if got := strings.Count(logged, "quote_write_rejected"); got != 2 {
		t.Fatalf("expected one WARN per refused row (2), got %d: %q", got, logged)
	}
	for _, want := range []string{"symbol=2330", "date=2026-08-11", "source=finmind", "reason=non_positive",
		"backend=sqlite", "reason=non_finite"} {
		if !strings.Contains(logged, want) {
			t.Fatalf("WARN log missing %q: %q", want, logged)
		}
	}
}

// TestSQLiteQuoteStoreAllRowsRejected verifies the "nothing was written" case is
// visible to the caller: a batch whose rows were all refused returns
// *QuoteRejectionsError wrapping ErrAllQuotesRejected instead of pretending the
// symbol was persisted.
func TestSQLiteQuoteStoreAllRowsRejected(t *testing.T) {
	quoteGuardCapture(t)
	store := newTestSQLiteQuoteStore(t)

	day := time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)
	err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "2380", Date: day, Close: 0, Source: "finmind"},
		{Symbol: "2380", Date: day.AddDate(0, 0, 1), Close: math.NaN(), Source: "finmind"},
	})
	if err == nil {
		t.Fatal("expected an error when every row is rejected")
	}
	if !errors.Is(err, ErrAllQuotesRejected) {
		t.Fatalf("error %v does not wrap ErrAllQuotesRejected", err)
	}
	var rej *QuoteRejectionsError
	if !errors.As(err, &rej) {
		t.Fatalf("error %v is not *QuoteRejectionsError", err)
	}
	if rej.Backend != QuoteBackendSQLite || rej.Total != 2 || rej.Rejected != 2 {
		t.Fatalf("unexpected rejection error: %+v", rej)
	}
	if rej.Symbol != "2380" || rej.Date != "2026-06-29" || rej.Reason != QuoteInvalidReasonNonPositive {
		t.Fatalf("rejection error does not identify the first bad row: %+v", rej)
	}
	got, loadErr := store.LoadQuotes("2380", day, day.AddDate(0, 0, 5))
	if loadErr != nil {
		t.Fatalf("LoadQuotes: %v", loadErr)
	}
	if len(got) != 0 {
		t.Fatalf("expected no stored rows for an all-invalid batch, got %+v", got)
	}
}

// TestJSONLQuoteStoreWriteGuard pins the same contract on the legacy JSONL
// backend so all three stores answer a bad row the same way.
func TestJSONLQuoteStoreWriteGuard(t *testing.T) {
	quoteGuardCapture(t)
	store := NewJSONLQuoteStore(t.TempDir())
	day := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	err := store.RecordQuotes([]domain.DailyBar{
		{Symbol: "5904", Date: day, Close: 720, Volume: 100, Source: "twse"},
		{Symbol: "5904", Date: day.AddDate(0, 0, 1), Close: 0, Source: "twse"},
	})
	if err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}
	got, err := store.LoadQuotes("5904", day, day.AddDate(0, 0, 5))
	if err != nil {
		t.Fatalf("LoadQuotes: %v", err)
	}
	if len(got) != 1 || got[0].Close != 720 {
		t.Fatalf("expected only the valid row, got %+v", got)
	}
}

// TestQuoteWriteGuardWarnCap pins that a corrupted series cannot flood the log:
// per-row WARNs stop at the cap and one suppression line says so.
func TestQuoteWriteGuardWarnCap(t *testing.T) {
	logs := quoteGuardCapture(t)
	store := newTestSQLiteQuoteStore(t)

	batch := make([]domain.DailyBar, 0, quoteRejectionWarnCap+10)
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < quoteRejectionWarnCap+10; i++ {
		batch = append(batch, domain.DailyBar{Symbol: "9999", Date: day.AddDate(0, 0, i), Close: 0})
	}
	if err := store.RecordQuotes(batch); err == nil {
		t.Fatal("expected ErrAllQuotesRejected for an all-invalid batch")
	}

	if got := strings.Count(logs.String(), "quote_write_rejected"); got != quoteRejectionWarnCap {
		t.Fatalf("per-row WARN count = %d, want %d", got, quoteRejectionWarnCap)
	}
	if !strings.Contains(logs.String(), "quote_write_rejection_suppressed") {
		t.Fatal("expected the warn-cap suppression line")
	}
}
