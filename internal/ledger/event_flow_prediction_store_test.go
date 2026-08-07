package ledger

import (
	"sync"
	"testing"
	"time"
)

func TestJSONLEventFlowPredictionStore_AppendAndLoad(t *testing.T) {
	baseDir := t.TempDir()
	store := NewJSONLEventFlowPredictionStore(baseDir)

	now := time.Now().UTC()
	recs := []EventFlowPredictionRecord{
		{PredictedAt: now.AddDate(0, 0, -3), DirectionSign: 0.3, Confidence: 0.3, Direction: "inflow"},
		{PredictedAt: now.AddDate(0, 0, -2), DirectionSign: -0.5, Confidence: 0.5, Direction: "outflow"},
		{PredictedAt: now.AddDate(0, 0, -1), DirectionSign: 0, Confidence: 0.7, Direction: "neutral"},
	}

	for _, r := range recs {
		if err := store.AppendPrediction(r); err != nil {
			t.Fatalf("AppendPrediction failed: %v", err)
		}
	}

	got, err := store.LoadRecentPredictions(10)
	if err != nil {
		t.Fatalf("LoadRecentPredictions failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 records, got %d", len(got))
	}
	for i, r := range recs {
		if r.PredictedAt.Unix() != got[i].PredictedAt.Unix() {
			t.Errorf("record %d: expected ts %v, got %v", i, r.PredictedAt, got[i].PredictedAt)
		}
		if r.DirectionSign != got[i].DirectionSign {
			t.Errorf("record %d: expected sign %v, got %v", i, r.DirectionSign, got[i].DirectionSign)
		}
	}
}

func TestJSONLEventFlowPredictionStore_LoadRecentRespectsLimit(t *testing.T) {
	baseDir := t.TempDir()
	store := NewJSONLEventFlowPredictionStore(baseDir)

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if err := store.AppendPrediction(EventFlowPredictionRecord{
			PredictedAt:   now.AddDate(0, 0, i-5),
			DirectionSign: float64(i),
			Confidence:    0.5,
			Direction:     "inflow",
		}); err != nil {
			t.Fatalf("AppendPrediction failed: %v", err)
		}
	}

	got, err := store.LoadRecentPredictions(2)
	if err != nil {
		t.Fatalf("LoadRecentPredictions failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected last 2 records, got %d", len(got))
	}
	if got[0].DirectionSign != 3 || got[1].DirectionSign != 4 {
		t.Errorf("expected last two records (3, 4), got (%v, %v)", got[0].DirectionSign, got[1].DirectionSign)
	}
}

func TestJSONLEventFlowPredictionStore_RespectsMaxRecordsCap(t *testing.T) {
	baseDir := t.TempDir()
	store := &JSONLEventFlowPredictionStore{
		baseDir:    baseDir,
		maxRecords: 5,
	}

	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		if err := store.AppendPrediction(EventFlowPredictionRecord{
			PredictedAt:   now.Add(time.Duration(i) * time.Second),
			DirectionSign: float64(i),
			Confidence:    0.5,
			Direction:     "inflow",
		}); err != nil {
			t.Fatalf("AppendPrediction failed: %v", err)
		}
	}

	got, err := store.LoadRecentPredictions(0)
	if err != nil {
		t.Fatalf("LoadRecentPredictions failed: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("expected cap=5 records, got %d", len(got))
	}
	for i, r := range got {
		want := float64(i + 5)
		if r.DirectionSign != want {
			t.Errorf("record %d: expected direction_sign %v, got %v", i, want, r.DirectionSign)
		}
	}
}

func TestJSONLEventFlowPredictionStore_EncodesDirectionSignWhenZero(t *testing.T) {
	baseDir := t.TempDir()
	store := NewJSONLEventFlowPredictionStore(baseDir)

	cases := []struct {
		name      string
		direction string
		conf      float64
		want      float64
	}{
		{"inflow positive", "inflow", 0.4, 0.4},
		{"outflow negative", "outflow", 0.4, -0.4},
		{"neutral zero", "neutral", 0.6, 0},
		{"unknown zero", "", 0.7, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := EventFlowPredictionRecord{
				PredictedAt:   time.Now().UTC(),
				DirectionSign: 0,
				Confidence:    tc.conf,
				Direction:     tc.direction,
			}
			if err := store.AppendPrediction(rec); err != nil {
				t.Fatalf("AppendPrediction failed: %v", err)
			}
		})
	}

	got, err := store.LoadRecentPredictions(10)
	if err != nil {
		t.Fatalf("LoadRecentPredictions failed: %v", err)
	}
	if len(got) != len(cases) {
		t.Fatalf("expected %d records, got %d", len(cases), len(got))
	}
	for i, tc := range cases {
		if got[i].DirectionSign != tc.want {
			t.Errorf("%s: expected DirectionSign %v, got %v", tc.name, tc.want, got[i].DirectionSign)
		}
	}
}

func TestJSONLEventFlowPredictionStore_PersistsAcrossInstances(t *testing.T) {
	baseDir := t.TempDir()
	first := NewJSONLEventFlowPredictionStore(baseDir)

	want := EventFlowPredictionRecord{
		PredictedAt:   time.Now().UTC(),
		DirectionSign: 0.42,
		Confidence:    0.42,
		Direction:     "inflow",
	}
	if err := first.AppendPrediction(want); err != nil {
		t.Fatalf("AppendPrediction failed: %v", err)
	}

	second := NewJSONLEventFlowPredictionStore(baseDir)
	got, err := second.LoadRecentPredictions(0)
	if err != nil {
		t.Fatalf("LoadRecentPredictions failed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 record after re-open, got %d", len(got))
	}
	if got[0].DirectionSign != want.DirectionSign {
		t.Errorf("expected DirectionSign %v, got %v", want.DirectionSign, got[0].DirectionSign)
	}
}

func TestJSONLEventFlowPredictionStore_LenAndSizeOnEmptyStore(t *testing.T) {
	store := NewJSONLEventFlowPredictionStore(t.TempDir())
	if got := store.Len(); got != 0 {
		t.Fatalf("Len on empty store = %d, want 0", got)
	}
	if got := store.Size(); got != 0 {
		t.Fatalf("Size on empty store = %d, want 0", got)
	}
}

func TestJSONLEventFlowPredictionStore_LenAndSizeAfterAppends(t *testing.T) {
	store := NewJSONLEventFlowPredictionStore(t.TempDir())
	for i := 0; i < 3; i++ {
		if err := store.AppendPrediction(EventFlowPredictionRecord{
			PredictedAt:   time.Now().Add(time.Duration(i) * time.Hour),
			DirectionSign: float64(i + 1),
			Confidence:    0.7,
			Direction:     "inflow",
		}); err != nil {
			t.Fatalf("AppendPrediction %d: %v", i, err)
		}
	}
	if got := store.Len(); got != 3 {
		t.Fatalf("Len after 3 appends = %d, want 3", got)
	}
	if got := store.Size(); got <= 0 {
		t.Fatalf("Size after 3 appends = %d, want > 0", got)
	}
}

// TestJSONLEventFlowPredictionStore_ConcurrentAppendsNoLoss 跑 5 個 goroutine
// 同時 Append,確認 read-modify-write 內的 Mutex 沒有 race。
func TestJSONLEventFlowPredictionStore_ConcurrentAppendsNoLoss(t *testing.T) {
	store := NewJSONLEventFlowPredictionStore(t.TempDir())
	const goroutines = 5
	const perGoroutine = 50
	allStart := time.Now().Add(-24 * time.Hour)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				rec := EventFlowPredictionRecord{
					PredictedAt:   allStart.Add(time.Duration(g*perGoroutine+i) * time.Millisecond),
					DirectionSign: float64(g*perGoroutine + i),
					Confidence:    0.7,
					Direction:     "inflow",
				}
				if err := store.AppendPrediction(rec); err != nil {
					t.Errorf("goroutine %d append %d: %v", g, i, err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	want := goroutines * perGoroutine
	records, err := store.LoadRecentPredictions(0)
	if err != nil {
		t.Fatalf("LoadRecentPredictions: %v", err)
	}
	if len(records) != want {
		t.Fatalf("record count after %d concurrent appends = %d, want %d", want, len(records), want)
	}
	seen := make(map[time.Time]struct{}, want)
	for _, r := range records {
		if _, dup := seen[r.PredictedAt]; dup {
			t.Fatalf("duplicated PredictedAt %s found (race detected)", r.PredictedAt.Format(time.RFC3339Nano))
		}
		seen[r.PredictedAt] = struct{}{}
	}
}

func TestJSONLEventFlowPredictionStore_UpdateActualFillsT1Outcome(t *testing.T) {
	store := NewJSONLEventFlowPredictionStore(t.TempDir())

	// Predicted at 13:45 Taipei (=05:45 UTC) on a given date.
	predictedAt := time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC)
	if err := store.AppendPrediction(EventFlowPredictionRecord{
		PredictedAt:   predictedAt,
		DirectionSign: 0.6,
		Confidence:    0.6,
		Direction:     "inflow",
	}); err != nil {
		t.Fatalf("AppendPrediction: %v", err)
	}

	// T+1 reconcile: actual realized as outflow (negative).
	if err := store.UpdateActual(predictedAt, -0.4, "twse_t86"); err != nil {
		t.Fatalf("UpdateActual: %v", err)
	}

	got, err := store.LoadRecentPredictions(10)
	if err != nil {
		t.Fatalf("LoadRecentPredictions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}
	if got[0].ActualSign != -0.4 {
		t.Errorf("ActualSign = %v, want -0.4", got[0].ActualSign)
	}
	if got[0].ActualSource != "twse_t86" {
		t.Errorf("ActualSource = %q, want twse_t86", got[0].ActualSource)
	}
	if got[0].ActualCapturedAt == nil {
		t.Fatal("ActualCapturedAt = nil, want set after reconcile")
	}
}

func TestJSONLEventFlowPredictionStore_UpdateActualNoMatchReturnsErr(t *testing.T) {
	store := NewJSONLEventFlowPredictionStore(t.TempDir())

	// One record on 8/6; try to reconcile a different date.
	predictedAt := time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC)
	if err := store.AppendPrediction(EventFlowPredictionRecord{
		PredictedAt:   predictedAt,
		DirectionSign: 0.6,
		Confidence:    0.6,
		Direction:     "inflow",
	}); err != nil {
		t.Fatalf("AppendPrediction: %v", err)
	}

	otherDate := time.Date(2026, 8, 5, 5, 45, 0, 0, time.UTC)
	err := store.UpdateActual(otherDate, -0.4, "twse_t86")
	if err != ErrPredictionNotFound {
		t.Fatalf("UpdateActual wrong date: err = %v, want ErrPredictionNotFound", err)
	}
}

func TestJSONLEventFlowPredictionStore_LoadByDateRoundTrip(t *testing.T) {
	store := NewJSONLEventFlowPredictionStore(t.TempDir())

	predictedAt := time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC)
	if err := store.AppendPrediction(EventFlowPredictionRecord{
		PredictedAt:   predictedAt,
		DirectionSign: 0.6,
		Confidence:    0.6,
		Direction:     "inflow",
	}); err != nil {
		t.Fatalf("AppendPrediction: %v", err)
	}

	// Lookup by the same Taipei calendar date (any time-of-day works).
	got, err := store.LoadByDate(time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadByDate: %v", err)
	}
	if got.PredictedAt.Unix() != predictedAt.Unix() {
		t.Errorf("LoadByDate returned PredictedAt %v, want %v", got.PredictedAt, predictedAt)
	}

	// Miss: different date.
	if _, err := store.LoadByDate(time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)); err != ErrPredictionNotFound {
		t.Fatalf("LoadByDate miss: err = %v, want ErrPredictionNotFound", err)
	}
}

// TestJSONLEventFlowPredictionStore_TaipeiDateAcrossUTCMidnight guards the
// samePredictionDate contract: records are matched by their Taipei calendar
// date, so a record captured at 2026-08-05T20:00:00Z (== 2026-08-06 04:00
// Taipei) must be found by LoadByDate(2026-08-06) and must NOT match
// 2026-08-05 — the exact cross-midnight boundary the T+1 reconciler depends
// on (reconciler runs 14:30 Taipei, prediction captured 13:45 Taipei).
func TestJSONLEventFlowPredictionStore_TaipeiDateAcrossUTCMidnight(t *testing.T) {
	store := NewJSONLEventFlowPredictionStore(t.TempDir())

	// 2026-08-05T20:00Z == 2026-08-06 04:00 Taipei — Taipei date is 8/6.
	predictedAt := time.Date(2026, 8, 5, 20, 0, 0, 0, time.UTC)
	if err := store.AppendPrediction(EventFlowPredictionRecord{
		PredictedAt:   predictedAt,
		DirectionSign: 0.6,
		Confidence:    0.6,
		Direction:     "inflow",
	}); err != nil {
		t.Fatalf("AppendPrediction: %v", err)
	}

	// Must match Taipei 8/6 (UTC 2026-08-06T00:00Z is within Taipei 8/6).
	got, err := store.LoadByDate(time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("LoadByDate(Taipei 8/6): %v", err)
	}
	if got.PredictedAt.Unix() != predictedAt.Unix() {
		t.Errorf("expected record %v, got %v", predictedAt, got.PredictedAt)
	}

	// Must NOT match Taipei 8/5 — the UTC date differs from Taipei date.
	if _, err := store.LoadByDate(time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)); err != ErrPredictionNotFound {
		t.Fatalf("LoadByDate(Taipei 8/5) = %v, want ErrPredictionNotFound (Taipei date 8/6)", err)
	}
	// UpdateActual must also target the Taipei 8/6 record.
	if err := store.UpdateActual(time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC), -0.4, "twse_t86"); err != nil {
		t.Fatalf("UpdateActual(Taipei 8/6): %v", err)
	}
	got2, _ := store.LoadByDate(predictedAt)
	if got2.ActualSign != -0.4 {
		t.Fatalf("reconciled ActualSign = %v, want -0.4", got2.ActualSign)
	}
}

// TestJSONLEventFlowPredictionStore_UpdateActualIdempotent guards that a
// second UpdateActual for the same date overwrites in place and does NOT
// duplicate the record (append is daily-once via handler, but a retry of the
// reconciler must stay idempotent).
func TestJSONLEventFlowPredictionStore_UpdateActualIdempotent(t *testing.T) {
	store := NewJSONLEventFlowPredictionStore(t.TempDir())
	predictedAt := time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC)
	if err := store.AppendPrediction(EventFlowPredictionRecord{
		PredictedAt:   predictedAt,
		DirectionSign: 0.6,
		Confidence:    0.6,
		Direction:     "inflow",
	}); err != nil {
		t.Fatalf("AppendPrediction: %v", err)
	}

	if err := store.UpdateActual(predictedAt, -0.4, "twse_t86"); err != nil {
		t.Fatalf("UpdateActual #1: %v", err)
	}
	if err := store.UpdateActual(predictedAt, 0.3, "twse_t86"); err != nil {
		t.Fatalf("UpdateActual #2: %v", err)
	}

	records, err := store.LoadRecentPredictions(10)
	if err != nil {
		t.Fatalf("LoadRecentPredictions: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record after re-update, got %d", len(records))
	}
	if records[0].ActualSign != 0.3 {
		t.Fatalf("ActualSign = %v after re-update, want 0.3 (overwrite in place)", records[0].ActualSign)
	}
}

// TestJSONLEventFlowPredictionStore_UnreconciledNilAfterReload guards the
// pointer distinction: an unreconciled record reloads with ActualCapturedAt
// == nil (JSON omits it), never a zero timestamp.
func TestJSONLEventFlowPredictionStore_UnreconciledNilAfterReload(t *testing.T) {
	baseDir := t.TempDir()
	s1 := NewJSONLEventFlowPredictionStore(baseDir)
	predictedAt := time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC)
	if err := s1.AppendPrediction(EventFlowPredictionRecord{
		PredictedAt:   predictedAt,
		DirectionSign: 0.6,
		Confidence:    0.6,
		Direction:     "inflow",
	}); err != nil {
		t.Fatalf("AppendPrediction: %v", err)
	}

	s2 := NewJSONLEventFlowPredictionStore(baseDir)
	got, err := s2.LoadByDate(predictedAt)
	if err != nil {
		t.Fatalf("LoadByDate: %v", err)
	}
	if got.ActualCapturedAt != nil {
		t.Fatalf("unreconciled ActualCapturedAt = %v, want nil", got.ActualCapturedAt)
	}
	if got.ActualSign != 0 || got.ActualSource != "" {
		t.Fatalf("unreconciled record should have zero actual fields, got sign=%v source=%q", got.ActualSign, got.ActualSource)
	}
}

func TestJSONLEventFlowPredictionStore_ActualPersistsAcrossInstances(t *testing.T) {
	baseDir := t.TempDir()
	predictedAt := time.Date(2026, 8, 6, 5, 45, 0, 0, time.UTC)

	// Write + reconcile on one instance.
	s1 := NewJSONLEventFlowPredictionStore(baseDir)
	if err := s1.AppendPrediction(EventFlowPredictionRecord{
		PredictedAt:   predictedAt,
		DirectionSign: 0.6,
		Confidence:    0.6,
		Direction:     "inflow",
	}); err != nil {
		t.Fatalf("AppendPrediction: %v", err)
	}
	if err := s1.UpdateActual(predictedAt, -0.4, "twse_t86"); err != nil {
		t.Fatalf("UpdateActual: %v", err)
	}

	// Read on a fresh instance — actual must survive the JSONL round-trip.
	s2 := NewJSONLEventFlowPredictionStore(baseDir)
	got, err := s2.LoadByDate(predictedAt)
	if err != nil {
		t.Fatalf("LoadByDate on fresh instance: %v", err)
	}
	if got.ActualSign != -0.4 || got.ActualSource != "twse_t86" {
		t.Errorf("fresh instance ActualSign/Source = %v/%q, want -0.4/twse_t86", got.ActualSign, got.ActualSource)
	}
	if got.ActualCapturedAt == nil {
		t.Fatal("ActualCapturedAt = nil after reload, want set")
	}
}
