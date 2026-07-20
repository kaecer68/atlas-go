package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// TestStressRowsToIndex_NormalizesRegime covers D03-A: the handler-layer
// projection must apply narrative.NormalizeRegime() to each row's Regime
// field so that the HTTP response for /api/narrative/stress-index/history
// uses the canonical Regime vocabulary regardless of which writer path
// produced the row. The Source field must be preserved verbatim so that
// consumers can recover the original vocabulary if needed.
func TestStressRowsToIndex_NormalizesRegime(t *testing.T) {
	now := time.Now().UTC()
	rows := []ledger.StressRow{
		{
			Date:        "2026-04-01",
			Score:       29.77,
			Regime:      "RISK_ON",
			Source:      "synthetic",
			IsSynthetic: 0,
			CapturedAt:  now,
		},
		{
			Date:        "2026-04-02",
			Score:       -3.91,
			Regime:      "NEUTRAL",
			Source:      "synthetic",
			IsSynthetic: 0,
			CapturedAt:  now,
		},
		{
			Date:        "2026-07-20",
			Score:       16.06,
			Regime:      "low",
			Source:      "macro_ingest",
			IsSynthetic: 0,
			CapturedAt:  now,
		},
		{
			Date:        "2026-07-21",
			Score:       22.5,
			Regime:      "alert",
			Source:      "macro_ingest",
			IsSynthetic: 0,
			CapturedAt:  now,
		},
	}

	out := stressRowsToIndex(rows)

	if len(out) != 4 {
		t.Fatalf("len(out) = %d, want 4", len(out))
	}

	// Order: stressRowsToIndex walks rows in REVERSE so oldest first.
	// rows[3] (alert, 2026-07-21) → out[0]
	// rows[2] (low, 2026-07-20)   → out[1]
	// rows[1] (NEUTRAL, 2026-04-02) → out[2]
	// rows[0] (RISK_ON, 2026-04-01) → out[3]
	wantCases := []struct {
		date       string
		wantRegime string
		wantSource string
	}{
		{"2026-07-21", "NEUTRAL", "macro_ingest"}, // alert → NEUTRAL via NormalizeRegime
		{"2026-07-20", "RISK_ON", "macro_ingest"}, // low → RISK_ON
		{"2026-04-02", "NEUTRAL", "synthetic"},    // already Regime vocab
		{"2026-04-01", "RISK_ON", "synthetic"},    // already Regime vocab
	}
	for i, w := range wantCases {
		got := out[i]
		if got.Date != w.date {
			t.Errorf("out[%d].Date = %q, want %q", i, got.Date, w.date)
		}
		if got.Regime != w.wantRegime {
			t.Errorf("out[%d].Regime = %q, want %q (NormalizeRegime should map %q → %q)",
				i, got.Regime, w.wantRegime, got.Regime, w.wantRegime)
		}
		if got.Source != w.wantSource {
			t.Errorf("out[%d].Source = %q, want %q (Source must be preserved verbatim)",
				i, got.Source, w.wantSource)
		}
	}
}

// TestStressRowsToIndex_PreservesUnknownRegime covers D03-A defensive case:
// if a future vocabulary token isn't in the mapping table, NormalizeRegime
// passes it through unchanged. stressRowsToIndex must not crash and must
// surface the unknown string verbatim so consumers can detect new vocabulary.
func TestStressRowsToIndex_PreservesUnknownRegime(t *testing.T) {
	now := time.Now().UTC()
	rows := []ledger.StressRow{
		{
			Date:        "2026-12-31",
			Score:       99.9,
			Regime:      "extreme_panic",
			Source:      "macro_ingest",
			IsSynthetic: 0,
			CapturedAt:  now,
		},
	}
	out := stressRowsToIndex(rows)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].Regime != "extreme_panic" {
		t.Errorf("Regime = %q, want %q (unknown vocab must pass through)",
			out[0].Regime, "extreme_panic")
	}
}

// TestNarrativeService_GetStressIndexHistory_NormalizesRegime covers D03-A
// end-to-end: a real SQLite-backed HistoricalStore with mixed-vocab rows
// must serve the /api/narrative/stress-index/history response with all
// regimes normalized to the canonical Regime vocabulary.
func TestNarrativeService_GetStressIndexHistory_NormalizesRegime(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := ledger.InitSchema(db); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	store := ledger.NewSQLiteHistoricalStore(db)

	now := time.Now().UTC()
	mixedRows := []ledger.StressRow{
		{Date: "2026-06-01", Score: 10, Regime: "RISK_ON", Source: "synthetic", IsSynthetic: 0, CapturedAt: now},
		{Date: "2026-06-02", Score: 10, Regime: "low", Source: "macro_ingest", IsSynthetic: 0, CapturedAt: now},
		{Date: "2026-06-03", Score: 10, Regime: "alert", Source: "macro_ingest", IsSynthetic: 0, CapturedAt: now},
		{Date: "2026-06-04", Score: 10, Regime: "high", Source: "macro_ingest", IsSynthetic: 0, CapturedAt: now},
		{Date: "2026-06-05", Score: 10, Regime: "RISK_OFF", Source: "synthetic", IsSynthetic: 0, CapturedAt: now},
	}
	for _, r := range mixedRows {
		if err := store.UpsertStress(context.Background(), r); err != nil {
			t.Fatalf("upsert %s: %v", r.Date, err)
		}
	}

	svc := NewNarrativeService(dir, narrative.NewNarrativeEngine(), nil).
		WithHistoricalStore(store)

	hist := svc.GetStressIndexHistory(10)
	if len(hist) != 5 {
		t.Fatalf("len = %d, want 5", len(hist))
	}

	// ASC date order (oldest first). The pipeline that wires historicalStore
	// into the service reverses this in stressRowsToIndex, but here we call
	// UpsertStress → LoadStressHistory directly, which returns ASC.
	want := []struct {
		date   string
		regime string
		source string
	}{
		{"2026-06-01", "RISK_ON", "synthetic"},
		{"2026-06-02", "RISK_ON", "macro_ingest"},  // low → RISK_ON
		{"2026-06-03", "NEUTRAL", "macro_ingest"},  // alert → NEUTRAL
		{"2026-06-04", "RISK_OFF", "macro_ingest"}, // high → RISK_OFF
		{"2026-06-05", "RISK_OFF", "synthetic"},
	}
	for i, w := range want {
		got := hist[i]
		if got.Date != w.date {
			t.Errorf("hist[%d].Date = %q, want %q", i, got.Date, w.date)
		}
		if got.Regime != w.regime {
			t.Errorf("hist[%d].Regime = %q, want %q (normalize)", i, got.Regime, w.regime)
		}
		if got.Source != w.source {
			t.Errorf("hist[%d].Source = %q, want %q (preserve)", i, got.Source, w.source)
		}
	}
}
