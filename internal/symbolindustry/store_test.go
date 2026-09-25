package symbolindustry

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestPool opens a live Postgres pool for the env-guarded integration
// test. It is never reached when ATLAS_TEST_POSTGRES_DSN is unset.
func newTestPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

func entry(symbol, l1, status string) Entry {
	return Entry{
		Symbol:         symbol,
		CompanyName:    symbol + " 公司",
		Market:         "TWSE",
		IndustryCode:   "24",
		IndustryNameZH: "半導體業",
		CanonicalL1:    l1,
		MappingStatus:  status,
		MappingReason:  "twse-code-24",
		Source:         "TWSE:t187ap03_L",
		AsOf:           "2026-09-24",
	}
}

func TestNewStoreSQLiteRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	store, err := NewStore(ctx, "sqlite", nil, dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	// Empty slice is a no-op.
	n, err := store.UpsertAll(ctx, nil)
	if err != nil {
		t.Fatalf("UpsertAll(nil): %v", err)
	}
	if n != 0 {
		t.Fatalf("UpsertAll(nil) = %d, want 0", n)
	}
	if n, err := store.Count(ctx); err != nil || n != 0 {
		t.Fatalf("Count after no-op = %d, %v; want 0, nil", n, err)
	}

	entries := []Entry{
		entry("2330", "semiconductors", StatusMapped),
		entry("1303", "", StatusUnmapped),
		entry("6488", "semiconductors", StatusMapped),
	}
	n, err = store.UpsertAll(ctx, entries)
	if err != nil {
		t.Fatalf("UpsertAll: %v", err)
	}
	if n != 3 {
		t.Fatalf("UpsertAll = %d, want 3", n)
	}
	if n, err := store.Count(ctx); err != nil || n != 3 {
		t.Fatalf("Count = %d, %v; want 3, nil", n, err)
	}

	got, ok, err := store.Lookup(ctx, "2330")
	if err != nil {
		t.Fatalf("Lookup(2330): %v", err)
	}
	if !ok {
		t.Fatal("Lookup(2330) not found")
	}
	if got.Symbol != "2330" || got.CanonicalL1 != "semiconductors" ||
		got.MappingStatus != StatusMapped || got.IndustryNameZH != "半導體業" ||
		got.Market != "TWSE" || got.IndustryCode != "24" ||
		got.Source != "TWSE:t187ap03_L" || got.AsOf != "2026-09-24" {
		t.Fatalf("Lookup(2330) = %+v", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("Lookup(2330) UpdatedAt is zero")
	}
	if got.UpdatedAt.Location() != time.UTC {
		t.Fatalf("Lookup(2330) UpdatedAt location = %v, want UTC", got.UpdatedAt.Location())
	}

	if _, ok, err := store.Lookup(ctx, "9999"); err != nil {
		t.Fatalf("Lookup(9999): %v", err)
	} else if ok {
		t.Fatal("Lookup(9999) reported found")
	}

	all, err := store.LoadAll(ctx)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("LoadAll len = %d, want 3", len(all))
	}
	wantOrder := []string{"1303", "2330", "6488"}
	for i, want := range wantOrder {
		if all[i].Symbol != want {
			t.Fatalf("LoadAll[%d].Symbol = %q, want %q (order: %+v)", i, all[i].Symbol, want, all)
		}
	}

	// Idempotent re-upsert: same symbol is updated, Count stays put.
	updated := entry("1303", "plastics", StatusMapped)
	updated.MappingReason = "manual-override"
	if _, err := store.UpsertAll(ctx, []Entry{updated}); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
	if n, err := store.Count(ctx); err != nil || n != 3 {
		t.Fatalf("Count after re-upsert = %d, %v; want 3, nil", n, err)
	}
	got, ok, err = store.Lookup(ctx, "1303")
	if err != nil || !ok {
		t.Fatalf("Lookup(1303) after re-upsert: %v ok=%v", err, ok)
	}
	if got.CanonicalL1 != "plastics" || got.MappingStatus != StatusMapped ||
		got.MappingReason != "manual-override" {
		t.Fatalf("Lookup(1303) after re-upsert = %+v", got)
	}
}

func TestNewStoreRequiresWorkDirForSQLite(t *testing.T) {
	if _, err := NewStore(context.Background(), "sqlite", nil, ""); err == nil {
		t.Fatal("NewStore(sqlite, empty workDir) = nil error, want error")
	}
}

func TestNewStorePostgresWithoutPoolFails(t *testing.T) {
	if _, err := NewStore(context.Background(), "postgres", nil, t.TempDir()); err == nil {
		t.Fatal("NewStore(postgres, nil pool) = nil error, want error")
	}
}

func TestStorePostgresRoundTrip(t *testing.T) {
	dsn := os.Getenv("ATLAS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ATLAS_TEST_POSTGRES_DSN not set; skipping live postgres store test")
	}
	ctx := context.Background()
	pool, err := newTestPool(ctx, dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS symbol_industry (
			symbol TEXT PRIMARY KEY,
			company_name TEXT NOT NULL DEFAULT '',
			market TEXT NOT NULL DEFAULT '',
			industry_code TEXT NOT NULL DEFAULT '',
			industry_name_zh TEXT NOT NULL DEFAULT '',
			canonical_l1 TEXT NOT NULL DEFAULT '',
			mapping_status TEXT NOT NULL DEFAULT '',
			mapping_reason TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			as_of TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	store, err := NewStore(ctx, "postgres", pool, "")
	if err != nil {
		t.Fatalf("NewStore(postgres): %v", err)
	}
	sym := "TEST2330"
	if _, err := pool.Exec(ctx, `DELETE FROM symbol_industry WHERE symbol = $1`, sym); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	defer func() { _, _ = pool.Exec(ctx, `DELETE FROM symbol_industry WHERE symbol = $1`, sym) }()

	if _, err := store.UpsertAll(ctx, []Entry{entry(sym, "semiconductors", StatusMapped)}); err != nil {
		t.Fatalf("UpsertAll: %v", err)
	}
	got, ok, err := store.Lookup(ctx, sym)
	if err != nil || !ok {
		t.Fatalf("Lookup(%s): %v ok=%v", sym, err, ok)
	}
	if got.CanonicalL1 != "semiconductors" || got.UpdatedAt.IsZero() {
		t.Fatalf("Lookup(%s) = %+v", sym, got)
	}
}
