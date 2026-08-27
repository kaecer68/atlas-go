package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// cliPanel is a synthetic PanelSource injected into runWithPanel for tests.
type cliPanel struct {
	bars  map[string][]stockpicker.HistoricalBar
	flows map[string][]stockpicker.FlowPoint
}

func (p *cliPanel) Bars(_ context.Context, symbol string) ([]stockpicker.HistoricalBar, error) {
	return p.bars[symbol], nil
}

func (p *cliPanel) Flows(_ context.Context, symbol string) ([]stockpicker.FlowPoint, error) {
	return p.flows[symbol], nil
}

func cliDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return d
}

// writeTestWorkdir creates a temp workdir with a working parameters.json
// (copy of the real configs/parameters.json) so the CLI can load it.
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

// synthPanel builds a monotonic uptrend with positive flows for 2330.
func synthPanel(t *testing.T) *cliPanel {
	t.Helper()
	bars := make([]stockpicker.HistoricalBar, 40)
	for i := range bars {
		bars[i] = stockpicker.HistoricalBar{Date: cliDate(t, "2026-01-05").AddDate(0, 0, i), Close: float64(100 + i), Volume: 1000}
	}
	flows := make([]stockpicker.FlowPoint, 40)
	for i := range flows {
		flows[i] = stockpicker.FlowPoint{Date: cliDate(t, "2026-01-05").AddDate(0, 0, i).Format("2006-01-02"), ForeignNet: 1000}
	}
	return &cliPanel{
		bars:  map[string][]stockpicker.HistoricalBar{"2330": bars},
		flows: map[string][]stockpicker.FlowPoint{"2330": flows},
	}
}

// runCLI runs the CLI against a fresh temp workdir with the synthetic panel.
func runCLI(t *testing.T, panel stockpicker.PanelSource, extra ...string) (string, error) {
	t.Helper()
	dir := writeTestWorkdir(t)
	args := append([]string{"-workdir", dir, "-start", "2026-01-05", "-end", "2026-01-30", "-asof", "2026-02-13", "-universe", "2330"}, extra...)
	return dir, runWithPanel(args, panel)
}

// openCLITestDB opens the CLI's ledger DB at the workdir path.
func openCLITestDB(t *testing.T, workdir string) *sql.DB {
	t.Helper()
	db, err := ledger.OpenSQLiteDB(filepath.Join(workdir, "data", "state", "atlas.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func outcomeCount(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM stock_signal_outcomes`).Scan(&n); err != nil {
		t.Fatalf("count outcomes: %v", err)
	}
	return n
}

// TestCLI_Idempotent: same-day rerun skips (outcome rows not duplicated);
// -force reruns and ON CONFLICT DO NOTHING keeps the row count stable.
func TestCLI_Idempotent(t *testing.T) {
	wd, err := runCLI(t, synthPanel(t))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	db := openCLITestDB(t, wd)
	n := outcomeCount(t, db)
	if n == 0 {
		t.Fatal("expected outcomes after first run")
	}

	// Same args again → skip, count unchanged.
	if err := runWithPanel([]string{"-workdir", wd, "-start", "2026-01-05", "-end", "2026-01-30", "-asof", "2026-02-13", "-universe", "2330"}, synthPanel(t)); err != nil {
		t.Fatalf("second run should skip, got error: %v", err)
	}
	if n2 := outcomeCount(t, db); n2 != n {
		t.Fatalf("second run changed outcome count: %d → %d; want idempotent skip", n, n2)
	}

	// -force reruns; rows are upserted (ON CONFLICT DO NOTHING) → count same.
	if err := runWithPanel([]string{"-workdir", wd, "-start", "2026-01-05", "-end", "2026-01-30", "-asof", "2026-02-13", "-universe", "2330", "-force"}, synthPanel(t)); err != nil {
		t.Fatalf("forced run: %v", err)
	}
	if n3 := outcomeCount(t, db); n3 != n {
		t.Fatalf("forced rerun changed row count: %d → %d; want same (ON CONFLICT DO NOTHING)", n, n3)
	}
}

// TestCLI_AsOfInjected: the -asof date flows into the aggregation window and
// the state snapshot; the cutoff is not time.Now().
func TestCLI_AsOfInjected(t *testing.T) {
	wd, err := runCLI(t, synthPanel(t))
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(wd, "data", "state", "stock_win_rate.json"))
	if err != nil {
		t.Fatalf("read state json: %v", err)
	}
	var snap struct {
		AsOf string `json:"as_of"`
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.AsOf != "2026-02-13" {
		t.Fatalf("state as_of = %q, want 2026-02-13 (injected, not time.Now())", snap.AsOf)
	}

	// The aggregation window is relative to the injected as-of.
	db := openCLITestDB(t, wd)
	outcomes, err := stockpicker.LoadOutcomesAsOf(context.Background(), db, "", "", "120d", cliDate(t, "2026-02-13"))
	if err != nil {
		t.Fatalf("LoadOutcomesAsOf: %v", err)
	}
	if len(outcomes) == 0 {
		t.Fatal("expected outcomes within the injected 120d window")
	}
	for _, o := range outcomes {
		if d := cliDate(t, o.TriggerDate); d.After(cliDate(t, "2026-02-13")) {
			t.Fatalf("outcome trigger %s after as-of 2026-02-13", o.TriggerDate)
		}
	}
}

// TestCLI_FundamentalsExcluded: the CLI's backtest path produces only the
// PIT demo condition sources — no value / all_weather fundamentals.
func TestCLI_FundamentalsExcluded(t *testing.T) {
	wd, err := runCLI(t, synthPanel(t))
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	db := openCLITestDB(t, wd)
	rows, err := db.Query(`SELECT DISTINCT source FROM stock_signal_outcomes`)
	if err != nil {
		t.Fatalf("query sources: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var sources []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		sources = append(sources, s)
	}
	if len(sources) == 0 {
		t.Fatal("no outcomes recorded; expected the two demo conditions")
	}
	for _, s := range sources {
		for _, bad := range []string{"pe", "pb", "div", "yield", "value", "all-weather", "fundamental"} {
			if strings.Contains(s, bad) {
				t.Fatalf("outcome source %q contains fundamentals keyword %q; fundamentals must stay live_observe_only", s, bad)
			}
		}
	}
}

// TestCLI_DryRun: -dry-run computes coverage but persists nothing.
func TestCLI_DryRun(t *testing.T) {
	wd, err := runCLI(t, synthPanel(t), "-dry-run")
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wd, "data", "state", "stock_win_rate.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not write state json (stat err=%v)", err)
	}
	db := openCLITestDB(t, wd)
	if n := outcomeCount(t, db); n != 0 {
		t.Fatalf("dry-run recorded %d outcomes, want 0", n)
	}
}

// TestCLI_LookaheadFails: a panel whose latest bar is after -asof fails.
func TestCLI_LookaheadFails(t *testing.T) {
	wd := writeTestWorkdir(t)
	args := []string{"-workdir", wd, "-start", "2026-01-05", "-end", "2026-01-30", "-asof", "2026-01-20", "-universe", "2330"}
	err := runWithPanel(args, synthPanel(t)) // panel has bars through 2026-02-13 > asof
	if err == nil {
		t.Fatal("expected lookahead error when panel extends past -asof")
	}
	if !strings.Contains(err.Error(), "lookahead") {
		t.Fatalf("error = %v, want lookahead mention", err)
	}
}

// TestCLI_RejectsInvalidDates: bad -asof and end-before-start are rejected.
func TestCLI_RejectsInvalidDates(t *testing.T) {
	wd := writeTestWorkdir(t)
	if err := runWithPanel([]string{"-workdir", wd, "-asof", "not-a-date"}, synthPanel(t)); err == nil {
		t.Fatal("expected error for invalid asof")
	}
	if err := runWithPanel([]string{"-workdir", wd, "-start", "2026-02-01", "-end", "2026-01-01"}, synthPanel(t)); err == nil {
		t.Fatal("expected error for end before start")
	}
}

// TestRealPanel_FlowsFromFile pins the per-symbol flow JSON parsing used by
// the production panel (foreign_net must unmarshal into FlowPoint.ForeignNet).
func TestRealPanel_FlowsFromFile(t *testing.T) {
	dir := t.TempDir()
	flowsDir := filepath.Join(dir, "stock_flows")
	if err := os.MkdirAll(flowsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	file := `{"symbol":"2330","flows":[{"date":"2026-01-05","foreign_net":1500},{"date":"2026-01-06","foreign_net":1200}]}`
	if err := os.WriteFile(filepath.Join(flowsDir, "2330.json"), []byte(file), 0o644); err != nil {
		t.Fatalf("write flows: %v", err)
	}
	panel := &realPanel{flowsDir: flowsDir}
	flows, err := panel.Flows(context.Background(), "2330")
	if err != nil {
		t.Fatalf("Flows: %v", err)
	}
	if len(flows) != 2 {
		t.Fatalf("len(flows) = %d, want 2", len(flows))
	}
	if flows[0].ForeignNet != 1500 || flows[0].Date != "2026-01-05" {
		t.Fatalf("flow[0] = %+v, want foreign_net 1500 on 2026-01-05", flows[0])
	}
	// Missing symbol file → empty (no error), the flow condition stays silent.
	missing, err := panel.Flows(context.Background(), "9999")
	if err != nil || len(missing) != 0 {
		t.Fatalf("missing symbol: err=%v flows=%v, want empty nil", err, missing)
	}
}

// TestResolveBackend_Priority verifies the backend resolution order:
// explicit flag > ATLAS_STORE_BACKEND > DATABASE_URL heuristic > sqlite default.
func TestResolveBackend_Priority(t *testing.T) {
	t.Setenv("ATLAS_STORE_BACKEND", "")
	t.Setenv("DATABASE_URL", "")
	if got := resolveBackend("postgres"); got != "postgres" {
		t.Fatalf("flag value lost: got %q", got)
	}

	t.Setenv("ATLAS_STORE_BACKEND", "postgres")
	t.Setenv("DATABASE_URL", "")
	if got := resolveBackend(""); got != "postgres" {
		t.Fatalf("ATLAS_STORE_BACKEND ignored: got %q", got)
	}

	t.Setenv("ATLAS_STORE_BACKEND", "")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/db")
	if got := resolveBackend(""); got != "postgres" {
		t.Fatalf("DATABASE_URL heuristic failed: got %q", got)
	}

	t.Setenv("DATABASE_URL", "sqlite://file.db")
	if got := resolveBackend(""); got != "sqlite" {
		t.Fatalf("non-postgres DSN should default to sqlite: got %q", got)
	}

	t.Setenv("DATABASE_URL", "")
	if got := resolveBackend(""); got != "sqlite" {
		t.Fatalf("empty env should default to sqlite: got %q", got)
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

// TestRunWithPanel_BackendFlag exercises the full CLI with the -backend flag
// set to sqlite. The synthetic panel bypasses the real quote store, so this
// only checks that the flag is accepted and the outcome DB path is wired.
func TestRunWithPanel_BackendFlag(t *testing.T) {
	wd, err := runCLI(t, synthPanel(t), "-backend", "sqlite")
	if err != nil {
		t.Fatalf("run with -backend sqlite: %v", err)
	}
	db := openCLITestDB(t, wd)
	if n := outcomeCount(t, db); n == 0 {
		t.Fatal("expected outcomes after -backend sqlite run")
	}
}
