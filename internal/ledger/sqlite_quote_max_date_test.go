package ledger

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestSQLiteQuoteStore_MaxQuoteDate(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := NewSQLiteQuoteStore(db)

	// 空表 ⇒ zero time, nil error（「空表」是可觀測狀態，不是錯誤）。
	got, err := store.MaxQuoteDate(context.Background())
	if err != nil {
		t.Fatalf("MaxQuoteDate on empty table: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("MaxQuoteDate on empty table = %v, want zero", got)
	}

	quotes := []domain.DailyBar{
		{Symbol: "2330", Date: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC), Close: 1025, Source: "TWSE"},
		{Symbol: "2330", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), Close: 1005, Source: "TWSE"},
	}
	if err := store.RecordQuotes(quotes); err != nil {
		t.Fatalf("RecordQuotes: %v", err)
	}

	got, err = store.MaxQuoteDate(context.Background())
	if err != nil {
		t.Fatalf("MaxQuoteDate: %v", err)
	}
	if got.Format("2006-01-02") != "2026-01-06" {
		t.Fatalf("MaxQuoteDate = %v, want 2026-01-06", got.Format("2006-01-02"))
	}

	// 型別斷言：SQLiteQuoteStore 實作 QuoteMaxDater。
	var _ QuoteMaxDater = store
}
