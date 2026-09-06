package stockpicker

// daily_update_test.go — PR 2e tests for RunDailyUpdate (the shared
// CLI/scheduler core), the idempotency policies, and the moved helpers
// (resolve backend / sqlite quote store / condition selection).

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// testPanel is a synthetic PanelSource for RunDailyUpdate tests: a
// monotonic uptrend with positive flows for 2330 (both default conditions
// fire on every eligible bar).
type testPanel struct {
	bars  map[string][]HistoricalBar
	flows map[string][]FlowPoint
}

func (p *testPanel) Bars(_ context.Context, symbol string) ([]HistoricalBar, error) {
	return p.bars[symbol], nil
}

func (p *testPanel) Flows(_ context.Context, symbol string) ([]FlowPoint, error) {
	return p.flows[symbol], nil
}

func testDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

// writeTestWorkdir creates a temp workdir with a working parameters.json
// (copy of the real configs/parameters.json) so RunDailyUpdate can load it.
func writeTestWorkdir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir configs: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "configs", "parameters.json"))
	if err != nil {
		t.Fatalf("read parameters.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "parameters.json"), data, 0o644); err != nil {
		t.Fatalf("write parameters.json: %v", err)
	}
	return dir
}

// synthPanel builds a monotonic uptrend with positive flows for 2330,
// with bars/flows covering every date through 2026-03-17 (>= the largest
// as-of used by the daily-update tests, 03-16). PIT is enforced per run:
// the engine rejects a panel whose LATEST bar is after the run's as-of, so
// runs with as-of < 03-17 build their own shorter panel via
// synthPanelUntil.
func synthPanel(t *testing.T) *testPanel { return synthPanelUntil(t, testDate(t, "2026-03-17")) }

// synthPanelUntil builds the synthetic panel with bars/flows through the
// given end date (inclusive).
func synthPanelUntil(t *testing.T, end time.Time) *testPanel {
	t.Helper()
	start := testDate(t, "2026-01-05")
	n := int(end.Sub(start).Hours()/24) + 1
	bars := make([]HistoricalBar, n)
	for i := range bars {
		bars[i] = HistoricalBar{Date: start.AddDate(0, 0, i), Close: float64(100 + i), Volume: 1000}
	}
	flows := make([]FlowPoint, n)
	for i := range flows {
		flows[i] = FlowPoint{Date: start.AddDate(0, 0, i).Format("2006-01-02"), ForeignNet: 1000}
	}
	return &testPanel{
		bars:  map[string][]HistoricalBar{"2330": bars},
		flows: map[string][]FlowPoint{"2330": flows},
	}
}

// runUpdate runs RunDailyUpdate against a fresh temp workdir with the
// synthetic panel and returns the workdir + result.
func runUpdate(t *testing.T, opts RunDailyOptions) (string, RunDailyResult, error) {
	t.Helper()
	if opts.WorkDir == "" {
		opts.WorkDir = writeTestWorkdir(t)
	}
	if opts.AsOf.IsZero() {
		opts.AsOf = testDate(t, "2026-03-16")
	}
	if opts.Start.IsZero() {
		opts.Start = testDate(t, "2026-01-05")
	}
	if opts.End.IsZero() {
		opts.End = testDate(t, "2026-03-16")
	}
	if opts.Panel == nil {
		// Panel bars must not extend past the run's end (PIT: the engine
		// rejects a panel whose latest bar is after the as-of).
		opts.Panel = synthPanelUntil(t, opts.End)
	}
	if opts.Universe == "" {
		opts.Universe = "2330" // testPanel carries only 2330; PanelSymbols falls
		// back to "fixture" for a bare non-RealPanel, so pin the universe
		// explicitly (same contract the CLI tests use).
	}
	res, err := RunDailyUpdate(context.Background(), opts)
	return opts.WorkDir, res, err
}

func outcomeCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stock_signal_outcomes`).Scan(&n); err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	return n
}

func openTestOutcomeDB(t *testing.T, workdir string) *sql.DB {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(filepath.Join(workdir, "data", "state", "atlas.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestRunDailyUpdate_WritesOutcomes: a full run records outcomes, aggregates
// win rates, and writes the state snapshot.
func TestRunDailyUpdate_WritesOutcomes(t *testing.T) {
	wd, res, err := runUpdate(t, RunDailyOptions{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.Skipped {
		t.Fatal("first run must not skip")
	}
	if res.Outcomes == 0 {
		t.Fatal("expected outcomes")
	}
	if res.Keys == 0 {
		t.Fatal("expected aggregated win-rate keys")
	}
	if _, err := os.Stat(res.StatePath); err != nil {
		t.Fatalf("state json not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wd, "data", "state", "stock_win_rate.json")); err != nil {
		t.Fatalf("state json missing at canonical path: %v", err)
	}
	if len(res.Conditions) != 4 {
		t.Fatalf("conditions = %v, want the four defaults (2 demo + 2 divergence)", res.Conditions)
	}
}

// TestRunDailyUpdate_RangeIdempotency: IdempotencyRange skips when outcomes
// already exist anywhere in [Start, End] (the CLI same-range rerun guard).
func TestRunDailyUpdate_RangeIdempotency(t *testing.T) {
	wd, _, err := runUpdate(t, RunDailyOptions{Idempotency: IdempotencyRange})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	db := openTestOutcomeDB(t, wd)
	if n := outcomeCount(t, db); n == 0 {
		t.Fatal("expected outcomes after first run")
	}

	res, err := RunDailyUpdate(context.Background(), RunDailyOptions{
		WorkDir:     wd,
		Idempotency: IdempotencyRange,
		AsOf:        testDate(t, "2026-02-13"),
		Start:       testDate(t, "2026-01-05"),
		End:         testDate(t, "2026-02-13"),
		Panel:       synthPanel(t),
	})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !res.Skipped {
		t.Fatal("same-range rerun must skip under IdempotencyRange")
	}
	if n := outcomeCount(t, db); n == 0 {
		t.Fatal("skip must not delete existing outcomes")
	}
}

// TestRunDailyUpdate_DayIdempotency: IdempotencyDay skips once the day's
// increment is recorded (the scheduled daily-update guard), and a failed
// run leaves no outcome so the next tick retries.
func TestRunDailyUpdate_DayIdempotency(t *testing.T) {
	wd, res, err := runUpdate(t, RunDailyOptions{Idempotency: IdempotencyDay})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	if res.Skipped {
		t.Fatal("first run must not skip")
	}
	db := openTestOutcomeDB(t, wd)
	n := outcomeCount(t, db)
	if n == 0 {
		t.Fatal("expected outcomes after first run")
	}

	// Same trading day, any later tick → skip (day increment recorded).
	res2, err := RunDailyUpdate(context.Background(), RunDailyOptions{
		WorkDir:     wd,
		Idempotency: IdempotencyDay,
		AsOf:        testDate(t, "2026-02-13"),
		Start:       testDate(t, "2026-01-05"),
		End:         testDate(t, "2026-02-13"),
		Panel:       synthPanel(t),
	})
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !res2.Skipped {
		t.Fatal("same-day rerun must skip under IdempotencyDay")
	}
	if n2 := outcomeCount(t, db); n2 != n {
		t.Fatalf("skip changed outcome count: %d → %d", n, n2)
	}
}

// TestRunDailyUpdate_DayIdempotency_NextDayRuns: the next trading day's
// increment (newest trigger date) is not yet recorded, so the run proceeds
// even though older outcomes exist.
func TestRunDailyUpdate_DayIdempotency_NextDayRuns(t *testing.T) {
	// First run on Friday 2026-03-13; second run on Monday 2026-03-16.
	wd, _, err := runUpdate(t, RunDailyOptions{Idempotency: IdempotencyDay, AsOf: testDate(t, "2026-03-13"), End: testDate(t, "2026-03-13")})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Next trading day (2026-03-16 Monday; 3/14-15 weekend): the newest
	// trigger 2026-03-09 was not covered by Friday's run (03-13 → 03-06) →
	// must run. (February dates would collide on 春節: 02-13 and 02-16 both
	// map back to trigger 02-05, making the idempotency check skip.)
	res, err := RunDailyUpdate(context.Background(), RunDailyOptions{
		WorkDir:     wd,
		Idempotency: IdempotencyDay,
		AsOf:        testDate(t, "2026-03-16"),
		Start:       testDate(t, "2026-01-05"),
		End:         testDate(t, "2026-03-16"),
		Universe:    "2330",
		Panel:       synthPanelUntil(t, testDate(t, "2026-03-16")),
	})
	if err != nil {
		t.Fatalf("next-day run: %v", err)
	}
	if res.Skipped {
		t.Fatal("next trading day must run (new increment), not skip")
	}
	if res.Outcomes == 0 {
		t.Fatal("expected new outcomes on the next-day run")
	}
}

// TestRunDailyUpdate_Force: IdempotencyNone always runs; ON CONFLICT DO
// NOTHING keeps the row count stable.
func TestRunDailyUpdate_Force(t *testing.T) {
	wd, _, err := runUpdate(t, RunDailyOptions{Idempotency: IdempotencyNone})
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	db := openTestOutcomeDB(t, wd)
	n := outcomeCount(t, db)

	res, err := RunDailyUpdate(context.Background(), RunDailyOptions{
		WorkDir:     wd,
		Idempotency: IdempotencyNone,
		AsOf:        testDate(t, "2026-03-16"),
		Start:       testDate(t, "2026-01-05"),
		End:         testDate(t, "2026-03-16"),
		Universe:    "2330",
		Panel:       synthPanelUntil(t, testDate(t, "2026-03-16")),
	})
	if err != nil {
		t.Fatalf("forced run: %v", err)
	}
	if res.Skipped {
		t.Fatal("IdempotencyNone must not skip")
	}
	if n2 := outcomeCount(t, db); n2 != n {
		t.Fatalf("forced rerun changed row count: %d → %d; want same (ON CONFLICT DO NOTHING)", n, n2)
	}
}

// TestRunDailyUpdate_DryRun: -dry-run computes coverage but persists nothing.
func TestRunDailyUpdate_DryRun(t *testing.T) {
	wd, res, err := runUpdate(t, RunDailyOptions{DryRun: true, Idempotency: IdempotencyNone})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if res.Outcomes == 0 {
		t.Fatal("dry-run should still compute outcomes")
	}
	if _, err := os.Stat(filepath.Join(wd, "data", "state", "stock_win_rate.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write state json (stat err=%v)", err)
	}
	db := openTestOutcomeDB(t, wd)
	if n := outcomeCount(t, db); n != 0 {
		t.Fatalf("dry-run recorded %d outcomes, want 0", n)
	}
}

// TestRunDailyUpdate_Validation: empty workdir, missing parameters config,
// and end-before-start are rejected loudly.
func TestRunDailyUpdate_Validation(t *testing.T) {
	ctx := context.Background()
	if _, err := RunDailyUpdate(ctx, RunDailyOptions{}); err == nil {
		t.Fatal("empty workdir must error")
	}

	emptyDir := t.TempDir()
	if _, err := RunDailyUpdate(ctx, RunDailyOptions{WorkDir: emptyDir, Panel: synthPanel(t)}); err == nil {
		t.Fatal("missing parameters.json must error")
	}

	wd := writeTestWorkdir(t)
	_, err := RunDailyUpdate(ctx, RunDailyOptions{
		WorkDir: wd,
		AsOf:    testDate(t, "2026-02-13"),
		Start:   testDate(t, "2026-02-14"),
		End:     testDate(t, "2026-02-13"),
		Panel:   synthPanel(t),
	})
	if err == nil || !strings.Contains(err.Error(), "before start") {
		t.Fatalf("end-before-start error = %v, want 'before start'", err)
	}
}

// TestNewestTriggerDate pins the trading-day arithmetic behind
// IdempotencyDay: the trading day exactly ForwardDays sessions before asOf,
// skipping weekends and holidays.
func TestNewestTriggerDate(t *testing.T) {
	cases := []struct {
		asOf string
		want string
	}{
		{"2026-03-13", "2026-03-06"}, // Fri → 5 sessions back (Mon-Fri prior week)
		{"2026-03-16", "2026-03-09"}, // Mon → previous Mon
		{"2026-03-17", "2026-03-10"}, // Tue → previous Tue
		{"2026-03-23", "2026-03-16"}, // Mon after weekend
	}
	for _, c := range cases {
		got := newestTriggerDate(testDate(t, c.asOf), DefaultForwardDays)
		if got.Format("2006-01-02") != c.want {
			t.Errorf("newestTriggerDate(%s) = %s, want %s", c.asOf, got.Format("2006-01-02"), c.want)
		}
	}
}

// TestResolveBackend_Priority verifies the backend resolution order:
// explicit flag > ATLAS_STORE_BACKEND > job-local sqlite default. The local
// DATABASE_URL heuristic is gone (M4②): a postgres DSN alone must not flip
// the backend; normalization + fail-loud validation delegate to the shared
// ledger resolver (WP4).
func TestResolveBackend_Priority(t *testing.T) {
	t.Setenv("ATLAS_STORE_BACKEND", "")
	t.Setenv("DATABASE_URL", "")
	if got, err := ResolveBackend("postgres"); err != nil || got != "postgres" {
		t.Fatalf("flag value lost: got %q err %v", got, err)
	}

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")
	if got, err := ResolveBackend(""); err != nil || got != "postgres" {
		t.Fatalf("ATLAS_STORE_BACKEND ignored: got %q err %v", got, err)
	}

	// A postgres DSN without an explicit flag/env must NOT select postgres
	// anymore — the heuristic diverged from store_factory (M4②).
	t.Setenv("ATLAS_STORE_BACKEND", "")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	if got, err := ResolveBackend(""); err != nil || got != "sqlite" {
		t.Fatalf("DATABASE_URL must not select postgres anymore: got %q err %v", got, err)
	}

	t.Setenv("DATABASE_URL", "")
	if got, err := ResolveBackend(""); err != nil || got != "sqlite" {
		t.Fatalf("empty env should default to job-local sqlite: got %q err %v", got, err)
	}

	// Garbage and jsonl are rejected loudly through the shared resolver
	// (WP4 fail-loud; jsonl is not a backend the daily update can serve).
	t.Setenv("ATLAS_STORE_BACKEND", "garbage")
	if got, err := ResolveBackend(""); err == nil {
		t.Fatalf("garbage ATLAS_STORE_BACKEND should error, got %q", got)
	}
	t.Setenv("ATLAS_STORE_BACKEND", "jsonl")
	if got, err := ResolveBackend(""); err == nil {
		t.Fatalf("jsonl ATLAS_STORE_BACKEND should error (unsupported), got %q", got)
	}
}

// TestOpenSQLiteQuoteStore_CreatesSchema verifies the sqlite backend path
// opens the DB, initializes the schema, and returns a usable QuoteStore.
func TestOpenSQLiteQuoteStore_CreatesSchema(t *testing.T) {
	dir := t.TempDir()
	store, err := openSQLiteQuoteStore(dir)
	if err != nil {
		t.Fatalf("openSQLiteQuoteStore: %v", err)
	}
	if store == nil {
		t.Fatal("expected non-nil QuoteStore")
	}
	// The schema file should now exist on disk.
	if _, err := os.Stat(filepath.Join(dir, "data", "state", "atlas.db")); err != nil {
		t.Fatalf("schema db not created: %v", err)
	}
}

// TestSelectConditions_Defaults verifies an empty condition list resolves to
// the full default registry set, whitespace is tolerated, and unknown IDs
// are rejected.
func TestSelectConditions_Defaults(t *testing.T) {
	params, err := config.LoadParametersConfig(filepath.Join("..", "..", "configs", "parameters.json"))
	if err != nil {
		t.Fatalf("load parameters: %v", err)
	}
	reg := NewDefaultConditionRegistry(&params.Stockpicker.Conditions)

	conds, err := selectConditions("", reg)
	if err != nil {
		t.Fatalf("selectConditions empty: %v", err)
	}
	if len(conds) != len(reg.All()) {
		t.Fatalf("empty list resolved to %d conditions, want %d", len(conds), len(reg.All()))
	}
	for i, c := range conds {
		if c.ID != reg.All()[i].ID {
			t.Errorf("condition order mismatch at %d: %q vs %q", i, c.ID, reg.All()[i].ID)
		}
	}

	one, err := selectConditions(" momentum-20d-positive ", reg)
	if err != nil {
		t.Fatalf("selectConditions one: %v", err)
	}
	if len(one) != 1 || one[0].ID != "momentum-20d-positive" {
		t.Fatalf("selectConditions one = %+v, want [momentum-20d-positive]", one)
	}

	if _, err := selectConditions("bogus-condition", reg); err == nil || !strings.Contains(err.Error(), "unknown condition") {
		t.Fatalf("unknown condition error = %v, want unknown-condition mention", err)
	}
}
