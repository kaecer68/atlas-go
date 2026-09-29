package marketdata

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testDay(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %s: %v", s, err)
	}
	return parsed
}

func series(t *testing.T, values map[string]float64) []DatedClose {
	t.Helper()
	out := make([]DatedClose, 0, len(values))
	for date, close := range values {
		out = append(out, DatedClose{Date: testDay(t, date), Close: close})
	}
	// sort by date
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Date.Before(out[i].Date) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestActionsFromOfficialSeriesSplit(t *testing.T) {
	// Raw: a 1-for-4 split on 2025-06-18 (188.65 -> 47.57 shape, simplified).
	raw := series(t, map[string]float64{
		"2025-06-16": 188.65,
		"2025-06-17": 189.00,
		"2025-06-18": 47.57,
		"2025-06-19": 47.90,
	})
	// Official adjusted series smooths the split: pre-event bars live in
	// post-event terms (exactly raw/4 here, so the official/raw ratio is constant
	// between events, as measured on real data), the newest bar keeps its raw
	// value.
	official := series(t, map[string]float64{
		"2025-06-16": 188.65 / 4,
		"2025-06-17": 189.00 / 4,
		"2025-06-18": 47.57,
		"2025-06-19": 47.90,
	})

	actions := ActionsFromOfficialSeries("SPLIT.TW", raw, official, DefaultOfficialAdjustedRatioTolerance)
	if len(actions) != 1 {
		t.Fatalf("want exactly 1 action, got %d (%+v)", len(actions), actions)
	}
	got := actions[0]
	wantDate := testDay(t, "2025-06-18")
	if !got.ExDate.Equal(wantDate) {
		t.Errorf("ExDate = %s, want %s", got.ExDate.Format("2006-01-02"), wantDate.Format("2006-01-02"))
	}
	if got.Source != OfficialSeriesActionSource {
		t.Errorf("Source = %q, want %q", got.Source, OfficialSeriesActionSource)
	}
	// factor = ReferencePrice / postEventRawClose must equal ratio_pre/ratio_post.
	const ratioBefore = (188.65 / 4) / 188.65 // official/raw before the split
	const ratioAfter = 1.0                    // official == raw after the split
	wantFactor := ratioBefore / ratioAfter
	wantReference := 47.57 * wantFactor
	if math.Abs(got.ReferencePrice-wantReference) > 1e-9 {
		t.Errorf("ReferencePrice = %v, want %v", got.ReferencePrice, wantReference)
	}
	if math.Abs(wantFactor-0.25) > 1e-3 {
		t.Errorf("derived factor = %v, want ~0.25", wantFactor)
	}
}

func TestActionsFromOfficialSeriesCashDividend(t *testing.T) {
	raw := series(t, map[string]float64{
		"2026-07-20": 102.5,
		"2026-07-21": 102.5,
		"2026-07-22": 102.5,
	})
	official := series(t, map[string]float64{
		"2026-07-20": 101.9,
		"2026-07-21": 102.5,
		"2026-07-22": 102.5,
	})
	actions := ActionsFromOfficialSeries("CASH.TW", raw, official, DefaultOfficialAdjustedRatioTolerance)
	if len(actions) != 1 {
		t.Fatalf("want 1 action, got %d", len(actions))
	}
	// ratio_pre = 101.9/102.5 = 0.99415..., ratio_post = 1 => factor ~0.99415
	if actions[0].ReferencePrice <= 0 || actions[0].ReferencePrice >= 102.5 {
		t.Errorf("ReferencePrice = %v, want between 0 and the raw close", actions[0].ReferencePrice)
	}
}

func TestActionsFromOfficialSeriesNoChangeIsNoOp(t *testing.T) {
	raw := series(t, map[string]float64{"2026-01-02": 50, "2026-01-05": 51, "2026-01-06": 52})
	official := series(t, map[string]float64{"2026-01-02": 50, "2026-01-05": 51, "2026-01-06": 52})
	if actions := ActionsFromOfficialSeries("CALM.TW", raw, official, DefaultOfficialAdjustedRatioTolerance); actions != nil {
		t.Errorf("want no actions for an unchanged ratio, got %+v", actions)
	}
}

func TestActionsFromOfficialSeriesSkipsDatesMissingFromOfficial(t *testing.T) {
	// The raw series has 2026-03-03 but the official one does not: the gap must
	// not fabricate an event, and it must not advance the reference ratio.
	raw := series(t, map[string]float64{
		"2026-03-02": 100,
		"2026-03-03": 200,
		"2026-03-04": 100,
	})
	official := series(t, map[string]float64{
		"2026-03-02": 100,
		"2026-03-04": 100,
	})
	if actions := ActionsFromOfficialSeries("GAP.TW", raw, official, DefaultOfficialAdjustedRatioTolerance); len(actions) != 0 {
		t.Errorf("want no actions when the ratio never changes, got %+v", actions)
	}
}

func TestActionsFromOfficialSeriesIgnoresNonPositiveRaw(t *testing.T) {
	raw := series(t, map[string]float64{
		"2026-03-02": 100,
		"2026-03-03": 0, // bad row: must be skipped, never used as a post-event anchor
		"2026-03-04": 50,
	})
	official := series(t, map[string]float64{
		"2026-03-02": 100,
		"2026-03-03": 100,
		"2026-03-04": 50,
	})
	actions := ActionsFromOfficialSeries("BAD.TW", raw, official, DefaultOfficialAdjustedRatioTolerance)
	for _, a := range actions {
		if a.ExDate.Equal(testDay(t, "2026-03-03")) {
			t.Fatalf("action emitted on a non-positive raw bar: %+v", a)
		}
	}
}

func TestLoadDatedClosesJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "X.jsonl")
	body := `{"date":"2026-01-02","close":100.5,"stock_id":"X"}
{"date":"2026-01-05","close":101}
not json at all
{"date":"bad-date","close":102}
{"date":"2026-01-06","close":0}
{"date":"2026-01-07","close":-3}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadDatedClosesJSONL(path)
	if err != nil {
		t.Fatalf("LoadDatedClosesJSONL: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 usable rows, got %d (%+v)", len(got), got)
	}
	if !got[0].Date.Equal(testDay(t, "2026-01-02")) || got[0].Close != 100.5 {
		t.Errorf("first row = %+v", got[0])
	}
	if !got[1].Date.Equal(testDay(t, "2026-01-05")) || got[1].Close != 101 {
		t.Errorf("second row = %+v", got[1])
	}
}

func TestLoadOfficialAdjustedSeriesDir(t *testing.T) {
	if _, err := LoadOfficialAdjustedSeriesDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("want an error for a missing directory (callers rely on it for the no-op rollback)")
	}
	empty := t.TempDir()
	if _, err := LoadOfficialAdjustedSeriesDir(empty); err == nil {
		t.Error("want an error for a directory without usable series")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "0050.TW.jsonl"), []byte("{\"date\":\"2026-01-02\",\"close\":100}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadOfficialAdjustedSeriesDir(dir)
	if err != nil {
		t.Fatalf("LoadOfficialAdjustedSeriesDir: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 series, got %d", len(got))
	}
	if _, ok := got["0050.TW"]; !ok {
		t.Errorf("key must be the file base name, got %v", got)
	}
}

func TestActionsFromOfficialSeriesIgnoresRoundingDrift(t *testing.T) {
	// Real official series round to a handful of decimals, so the official/raw
	// ratio drifts by up to ~1.2e-7 between events (measured over 1,600 days for
	// 0050.TW). That drift must NOT be mistaken for a corporate action.
	raw := series(t, map[string]float64{
		"2026-01-02": 100,
		"2026-01-05": 100,
		"2026-01-06": 100,
	})
	official := series(t, map[string]float64{
		"2026-01-02": 100 * (1 + 5e-8),
		"2026-01-05": 100 * (1 - 4e-8),
		"2026-01-06": 100,
	})
	if actions := ActionsFromOfficialSeries("DRIFT.TW", raw, official, DefaultOfficialAdjustedRatioTolerance); len(actions) != 0 {
		t.Errorf("rounding drift must not fabricate actions, got %+v", actions)
	}
}
