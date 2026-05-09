package ledger

import (
	"database/sql"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestSQLiteQuoteStore_RecordAndLoad(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	store := NewSQLiteQuoteStore(db)

	quotes := []domain.DailyBar{
		{Symbol: "2330", Name: "台積電", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Open: 1000, High: 1010, Low: 990, Close: 1005, Volume: 1000000, Source: "TWSE"},
		{Symbol: "2330", Name: "台積電", Date: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), Open: 1005, High: 1020, Low: 1000, Close: 1015, Volume: 1100000, Source: "TWSE"},
		{Symbol: "2330", Name: "台積電", Date: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), Open: 1015, High: 1030, Low: 1010, Close: 1025, Volume: 1200000, Source: "TWSE"},
	}

	if err := store.RecordQuotes(quotes); err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	got, err := store.LoadQuotes("2330", start, end)
	if err != nil {
		t.Fatalf("LoadQuotes: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("expected 2 quotes, got %d", len(got))
	}

	if got[0].Close != 1005 || got[1].Close != 1015 {
		t.Errorf("unexpected close prices: %v", got)
	}
}

func TestSQLiteQuoteStore_LoadLatestQuotes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	store := NewSQLiteQuoteStore(db)

	quotes := []domain.DailyBar{
		{Symbol: "2330", Name: "台積電", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Open: 1000, High: 1010, Low: 990, Close: 1005, Volume: 1000000, Source: "TWSE"},
		{Symbol: "2330", Name: "台積電", Date: time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC), Open: 1005, High: 1020, Low: 1000, Close: 1015, Volume: 1100000, Source: "TWSE"},
		{Symbol: "2317", Name: "鴻海", Date: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC), Open: 200, High: 210, Low: 195, Close: 205, Volume: 500000, Source: "TWSE"},
	}

	if err := store.RecordQuotes(quotes); err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}

	got, err := store.LoadLatestQuotes([]string{"2330", "2317", "9999"})
	if err != nil {
		t.Fatalf("LoadLatestQuotes: %v", err)
	}

	if got["2330"].Close != 1015 {
		t.Errorf("expected 2330 close 1015, got %v", got["2330"].Close)
	}
	if got["2317"].Close != 205 {
		t.Errorf("expected 2317 close 205, got %v", got["2317"].Close)
	}
	if _, ok := got["9999"]; ok {
		t.Errorf("expected no entry for 9999")
	}
}

func TestSQLiteQuoteStore_EmptyInputs(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	store := NewSQLiteQuoteStore(db)

	if err := store.RecordQuotes(nil); err != nil {
		t.Fatalf("RecordQuotes with nil: %v", err)
	}
	if err := store.RecordQuotes([]domain.DailyBar{}); err != nil {
		t.Fatalf("RecordQuotes with empty: %v", err)
	}

	got, err := store.LoadLatestQuotes(nil)
	if err != nil {
		t.Fatalf("LoadLatestQuotes with nil: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d", len(got))
	}

	got, err = store.LoadLatestQuotes([]string{})
	if err != nil {
		t.Fatalf("LoadLatestQuotes with empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d", len(got))
	}
}
