package stockpicker

import (
	"context"
	"database/sql"
	"testing"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// openWinRateTestDB opens an in-memory SQLite database with the ledger schema.
func openWinRateTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	return db
}

func sampleWinRateSummary() StockWinRateSummary {
	return StockWinRateSummary{
		Symbol:            "2330",
		Source:            "stockpicker-momentum",
		Window:            "120d",
		Observations:      40,
		Hits:              24,
		WinRate:           0.6,
		WilsonLower:       0.44,
		WilsonUpper:       0.74,
		Confidence:        0.95,
		CalibrationStatus: CalibrationEligible,
		NetCostRate:       0.00585,
		AvgForwardReturn:  0.012,
		UpdatedAt:         "2026-08-27T12:00:00Z",
	}
}

func TestWinRateStore_SaveAndLoad(t *testing.T) {
	db := openWinRateTestDB(t)
	store := NewWinRateStore(db)
	ctx := context.Background()

	in := sampleWinRateSummary()
	if err := store.SaveWinRate(ctx, in); err != nil {
		t.Fatalf("SaveWinRate: %v", err)
	}

	got, found, err := store.LoadWinRate(ctx, "2330", "stockpicker-momentum", "120d")
	if err != nil {
		t.Fatalf("LoadWinRate: %v", err)
	}
	if !found {
		t.Fatal("LoadWinRate: found = false, want true")
	}
	if got != in {
		t.Fatalf("round trip mismatch: got = %+v, want = %+v", got, in)
	}

	_, found, err = store.LoadWinRate(ctx, "2330", "stockpicker-momentum", "60d")
	if err != nil {
		t.Fatalf("LoadWinRate(missing): %v", err)
	}
	if found {
		t.Fatal("LoadWinRate(missing): found = true, want false")
	}
}

func TestWinRateStore_UpsertUpdates(t *testing.T) {
	db := openWinRateTestDB(t)
	store := NewWinRateStore(db)
	ctx := context.Background()

	first := sampleWinRateSummary()
	if err := store.SaveWinRate(ctx, first); err != nil {
		t.Fatalf("first SaveWinRate: %v", err)
	}

	second := first
	second.Observations = 45
	second.Hits = 30
	second.WinRate = 30.0 / 45.0
	second.CalibrationStatus = CalibrationDegraded
	second.UpdatedAt = "2026-08-27T13:00:00Z"
	if err := store.SaveWinRate(ctx, second); err != nil {
		t.Fatalf("second SaveWinRate (upsert): %v", err)
	}

	got, found, err := store.LoadWinRate(ctx, first.Symbol, first.Source, first.Window)
	if err != nil {
		t.Fatalf("LoadWinRate: %v", err)
	}
	if !found {
		t.Fatal("LoadWinRate: found = false, want true")
	}
	if got.Observations != 45 || got.Hits != 30 || got.WinRate != 30.0/45.0 {
		t.Fatalf("upsert did not update numeric fields: got %+v", got)
	}
	if got.CalibrationStatus != CalibrationDegraded || got.UpdatedAt != "2026-08-27T13:00:00Z" {
		t.Fatalf("upsert did not update status/updated_at: got %+v", got)
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM stock_win_rate WHERE symbol = ? AND source = ? AND window = ?`,
		first.Symbol, first.Source, first.Window,
	).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("upsert produced %d rows, want 1", n)
	}
}

func TestCalibrationStatusRoundTrip(t *testing.T) {
	db := openWinRateTestDB(t)
	store := NewWinRateStore(db)
	ctx := context.Background()

	for _, status := range []CalibrationStatus{CalibrationCalibrating, CalibrationEligible, CalibrationDegraded} {
		in := sampleWinRateSummary()
		in.Source = "source-" + string(status)
		in.CalibrationStatus = status

		if err := store.SaveWinRate(ctx, in); err != nil {
			t.Fatalf("SaveWinRate(%s): %v", status, err)
		}

		got, found, err := store.LoadWinRate(ctx, in.Symbol, in.Source, in.Window)
		if err != nil {
			t.Fatalf("LoadWinRate(%s): %v", status, err)
		}
		if !found {
			t.Fatalf("LoadWinRate(%s): found = false, want true", status)
		}
		if got.CalibrationStatus != status {
			t.Fatalf("calibration status round trip = %q, want %q", got.CalibrationStatus, status)
		}
	}
}

func TestWinRateStore_RejectsInvalidCalibrationStatus(t *testing.T) {
	db := openWinRateTestDB(t)
	store := NewWinRateStore(db)

	in := sampleWinRateSummary()
	in.CalibrationStatus = CalibrationStatus("not-a-status")
	if err := store.SaveWinRate(context.Background(), in); err == nil {
		t.Fatal("SaveWinRate with invalid calibration_status: want error, got nil")
	}
}
