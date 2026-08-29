package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestJSONLQuoteStoreRecordAndLoad(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	bars := []domain.DailyBar{
		{Date: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), Symbol: "2330", Name: "台積電", Open: 1000, High: 1010, Low: 990, Close: 1005, Volume: 1000000, Source: "TWSE"},
		{Date: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), Symbol: "2330", Name: "台積電", Open: 1005, High: 1020, Low: 1000, Close: 1015, Volume: 1100000, Source: "TWSE"},
	}

	if err := store.RecordQuotes(bars); err != nil {
		t.Fatalf("RecordQuotes failed: %v", err)
	}

	loaded, err := store.LoadQuotes("2330", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadQuotes failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(loaded))
	}
	if loaded[0].Close != 1005 || loaded[1].Close != 1015 {
		t.Errorf("unexpected close values: %v", loaded)
	}
}

func TestJSONLQuoteStoreLoadQuotesDateRange(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	bars := []domain.DailyBar{
		{Date: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), Symbol: "2454", Name: "聯發科", Open: 800, High: 810, Low: 790, Close: 805, Volume: 500000, Source: "TWSE"},
		{Date: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), Symbol: "2454", Name: "聯發科", Open: 805, High: 820, Low: 800, Close: 815, Volume: 600000, Source: "TWSE"},
		{Date: time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC), Symbol: "2454", Name: "聯發科", Open: 815, High: 830, Low: 810, Close: 825, Volume: 700000, Source: "TWSE"},
	}

	if err := store.RecordQuotes(bars); err != nil {
		t.Fatalf("RecordQuotes failed: %v", err)
	}

	start := time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 7, 0, 0, 0, 0, time.UTC)
	loaded, err := store.LoadQuotes("2454", start, end)
	if err != nil {
		t.Fatalf("LoadQuotes failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 quotes in range, got %d", len(loaded))
	}
}

func TestJSONLQuoteStoreLoadQuotesSymbolFilter(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	bars := []domain.DailyBar{
		{Date: time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC), Symbol: "2317", Name: "鴻海", Open: 200, High: 205, Low: 198, Close: 202, Volume: 800000, Source: "TWSE"},
		{Date: time.Date(2026, 2, 3, 0, 0, 0, 0, time.UTC), Symbol: "2303", Name: "聯電", Open: 50, High: 51, Low: 49, Close: 50.5, Volume: 900000, Source: "TWSE"},
	}

	if err := store.RecordQuotes(bars); err != nil {
		t.Fatalf("RecordQuotes failed: %v", err)
	}

	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC)

	loaded2317, err := store.LoadQuotes("2317", start, end)
	if err != nil {
		t.Fatalf("LoadQuotes for 2317 failed: %v", err)
	}
	if len(loaded2317) != 1 {
		t.Fatalf("expected 1 quote for 2317, got %d", len(loaded2317))
	}
	if loaded2317[0].Symbol != "2317" {
		t.Errorf("expected symbol 2317, got %s", loaded2317[0].Symbol)
	}

	loaded2303, err := store.LoadQuotes("2303", start, end)
	if err != nil {
		t.Fatalf("LoadQuotes for 2303 failed: %v", err)
	}
	if len(loaded2303) != 1 {
		t.Fatalf("expected 1 quote for 2303, got %d", len(loaded2303))
	}
}

func TestJSONLQuoteStoreLoadLatestQuotes(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	bars := []domain.DailyBar{
		{Date: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), Symbol: "2330", Name: "台積電", Open: 1000, High: 1010, Low: 990, Close: 1005, Volume: 1000000, Source: "TWSE"},
		{Date: time.Date(2026, 3, 11, 0, 0, 0, 0, time.UTC), Symbol: "2330", Name: "台積電", Open: 1005, High: 1020, Low: 1000, Close: 1015, Volume: 1100000, Source: "TWSE"},
		{Date: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), Symbol: "2454", Name: "聯發科", Open: 800, High: 810, Low: 790, Close: 805, Volume: 500000, Source: "TWSE"},
		{Date: time.Date(2026, 3, 12, 0, 0, 0, 0, time.UTC), Symbol: "2454", Name: "聯發科", Open: 810, High: 825, Low: 805, Close: 820, Volume: 600000, Source: "TWSE"},
	}

	if err := store.RecordQuotes(bars); err != nil {
		t.Fatalf("RecordQuotes failed: %v", err)
	}

	latest, err := store.LoadLatestQuotes([]string{"2330", "2454"})
	if err != nil {
		t.Fatalf("LoadLatestQuotes failed: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("expected 2 symbols in result, got %d", len(latest))
	}
	if latest["2330"].Close != 1015 {
		t.Errorf("expected 2330 latest close 1015, got %f", latest["2330"].Close)
	}
	if latest["2454"].Close != 820 {
		t.Errorf("expected 2454 latest close 820, got %f", latest["2454"].Close)
	}
}

func TestJSONLQuoteStoreLoadLatestQuotesEmpty(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	latest, err := store.LoadLatestQuotes([]string{"2330"})
	if err != nil {
		t.Fatalf("LoadLatestQuotes failed: %v", err)
	}
	if latest != nil {
		t.Fatalf("expected nil for unknown symbol, got %v", latest)
	}
}

func TestJSONLQuoteStoreLoadQuotesEmptyFile(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	// Write empty file
	path := filepath.Join(tmp, "quotes.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("failed to create empty file: %v", err)
	}

	loaded, err := store.LoadQuotes("2330", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadQuotes failed on empty file: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for empty file, got %v", loaded)
	}
}

func TestJSONLQuoteStoreLoadQuotesMissingFile(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	loaded, err := store.LoadQuotes("2330", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadQuotes failed on missing file: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil for missing file, got %v", loaded)
	}
}

func TestJSONLQuoteStoreAtomicWrite(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	bars := []domain.DailyBar{
		{Date: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), Symbol: "2330", Name: "台積電", Open: 1000, High: 1010, Low: 990, Close: 1005, Volume: 1000000, Source: "TWSE"},
	}

	if err := store.RecordQuotes(bars); err != nil {
		t.Fatalf("RecordQuotes failed: %v", err)
	}

	// Verify .tmp file does not exist after successful write
	tmpFile := filepath.Join(tmp, "quotes.jsonl.tmp")
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Errorf("expected .tmp file to be removed, but it exists")
	}

	// Verify final file exists
	finalFile := filepath.Join(tmp, "quotes.jsonl")
	if _, err := os.Stat(finalFile); os.IsNotExist(err) {
		t.Errorf("expected final file to exist")
	}
}

func TestJSONLQuoteStoreNoDuplicates(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	bars1 := []domain.DailyBar{
		{Date: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), Symbol: "2330", Name: "台積電", Open: 1000, High: 1010, Low: 990, Close: 1005, Volume: 1000000, Source: "TWSE"},
	}
	if err := store.RecordQuotes(bars1); err != nil {
		t.Fatalf("RecordQuotes failed: %v", err)
	}

	bars2 := []domain.DailyBar{
		{Date: time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC), Symbol: "2330", Name: "台積電", Open: 1005, High: 1020, Low: 1000, Close: 1015, Volume: 1100000, Source: "TWSE"},
	}
	if err := store.RecordQuotes(bars2); err != nil {
		t.Fatalf("RecordQuotes failed: %v", err)
	}

	loaded, err := store.LoadQuotes("2330", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 5, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadQuotes failed: %v", err)
	}
	// RecordQuotes uses O_APPEND so all batches are preserved
	if len(loaded) != 2 {
		t.Fatalf("expected 2 quotes after O_APPEND, got %d", len(loaded))
	}
	if loaded[0].Date.Day() != 1 {
		t.Errorf("expected first date day 1, got %d", loaded[0].Date.Day())
	}
	if loaded[1].Date.Day() != 2 {
		t.Errorf("expected second date day 2, got %d", loaded[1].Date.Day())
	}
}

func TestJSONLQuoteStoreConcurrentWrites(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	const goroutines = 8
	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)
	symbols := make([]string, goroutines)
	for g := range goroutines {
		wg.Add(1)
		symbols[g] = fmt.Sprintf("233%d", g)
		go func(g int) {
			defer wg.Done()
			bars := []domain.DailyBar{
				{Date: time.Date(2026, 6, g+1, 0, 0, 0, 0, time.UTC), Symbol: symbols[g], Name: "test", Open: 100, High: 110, Low: 90, Close: 105, Volume: 1000, Source: "test"},
			}
			if err := store.RecordQuotes(bars); err != nil {
				errCh <- fmt.Errorf("goroutine %d: %w", g, err)
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent RecordQuotes failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "quotes.jsonl.tmp")); !os.IsNotExist(err) {
		t.Errorf("expected .tmp file to be removed after concurrent writes, but it exists")
	}

	seen := make(map[string]bool, goroutines)
	for _, sym := range symbols {
		loaded, err := store.LoadQuotes(sym, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC))
		if err != nil {
			t.Fatalf("LoadQuotes(%s) failed: %v", sym, err)
		}
		if len(loaded) != 1 {
			t.Errorf("symbol %s: expected exactly 1 bar, got %d", sym, len(loaded))
		}
		if len(loaded) == 1 {
			if loaded[0].Symbol != sym {
				t.Errorf("symbol mismatch: expected %s, got %s", sym, loaded[0].Symbol)
			}
			seen[sym] = true
		}
	}
	if len(seen) != goroutines {
		t.Errorf("expected all %d goroutines to succeed, got %d winners: %v", goroutines, len(seen), seen)
	}
}

func TestJSONLQuoteStoreRecordQuotesDedup(t *testing.T) {
	tmp := t.TempDir()
	store := NewJSONLQuoteStore(tmp)

	date := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	nextDate := date.AddDate(0, 0, 1)

	if err := store.RecordQuotes([]domain.DailyBar{
		{Date: date, Symbol: "2330", Name: "台積電", Open: 1000, High: 1010, Low: 990, Close: 1005, Volume: 1000000, Source: "FinMind"},
	}); err != nil {
		t.Fatalf("first RecordQuotes failed: %v", err)
	}

	if err := store.RecordQuotes([]domain.DailyBar{
		// Same (symbol, date) — must be ignored (first wins).
		{Date: date, Symbol: "2330", Name: "台積電", Open: 2000, High: 2010, Low: 1990, Close: 2005, Volume: 9999999, Source: "DifferentSource"},
		// New (symbol, date) — must be appended.
		{Date: nextDate, Symbol: "2330", Name: "台積電", Open: 1006, High: 1020, Low: 1001, Close: 1015, Volume: 1100000, Source: "FinMind"},
	}); err != nil {
		t.Fatalf("second RecordQuotes failed: %v", err)
	}

	loaded, err := store.LoadQuotes("2330", date, nextDate)
	if err != nil {
		t.Fatalf("LoadQuotes failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 unique bars after dedup, got %d", len(loaded))
	}
	if loaded[0].Close != 1005 {
		t.Errorf("expected first bar Close=1005 (first-wins preserved original), got %f", loaded[0].Close)
	}
	if loaded[0].Source != "FinMind" {
		t.Errorf("expected first bar Source=FinMind (first-wins preserved original), got %s", loaded[0].Source)
	}
	if loaded[1].Date != nextDate || loaded[1].Close != 1015 {
		t.Errorf("expected second bar to be 2026-07-02 Close=1015, got %s Close=%f",
			loaded[1].Date.Format("2006-01-02"), loaded[1].Close)
	}
}
