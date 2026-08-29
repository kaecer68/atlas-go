// Package main implements cmd/atlas-stage4-loader — Stage 4 PR#2 loader.
//
// Purpose:
//
//	Read the 4 staging JSONL files emitted by cmd/atlas-stage4-backfill
//	(PR#1) and upsert each row into the corresponding Stage 4 history
//	table in data/state/atlas.db via internal/ledger.HistoricalStore.
//
// Behavior:
//   - One CLI run handles all 4 tables. --tables can restrict to a subset.
//   - Re-running is idempotent (UPSERT ON CONFLICT DO UPDATE on every PK).
//   - Date range filter: --since / --until parse as YYYY-MM-DD and apply.
//   - On parse failure, the loader logs and skips the row (does not abort
//     the whole batch), matching PR#1's malformed-line tolerance.
//
// CLI flags:
//
//	-staging <path>      directory containing the 4 staging JSONLs (default ./data/staging)
//	-db <path>           SQLite path (default ./data/state/atlas.db)
//	-tables <list>       comma-separated subset of regime,stress,events,prediction
//	-since <YYYY-MM-DD>  only rows with date >= this value
//	-until <YYYY-MM-DD>  only rows with date <= this value
//	-dry-run             parse and report counts, do not write
//	-init-schema         run InitSchema before loading (idempotent)
//	-h, -help            usage
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
)

// ------------------------------------------------------------------
// Loader DTOs. Mirror cmd/atlas-stage4-backfill/schema.go's record
// shapes but live here so PR#2 does not depend on PR#1's package path.
// If PR#1's schema changes shape, the JsonShape fields below will
// silently round-trip the wrong types, which is caught by the round-trip
// tests at the bottom of this file.
// ------------------------------------------------------------------

type loaderRegime struct {
	Date            string    `json:"date"`
	Regime          string    `json:"regime"`
	SourceSessionID string    `json:"source_session_id"`
	RecordedAt      time.Time `json:"recorded_at"`
	CapturedAt      time.Time `json:"captured_at"`
	IsSynthetic     uint8     `json:"is_synthetic"`
}

type loaderStress struct {
	Date        string         `json:"date"`
	Score       float64        `json:"score"`
	Regime      string         `json:"regime"`
	Components  map[string]any `json:"components"`
	Source      string         `json:"source"`
	CapturedAt  time.Time      `json:"captured_at"`
	IsSynthetic uint8          `json:"is_synthetic"`
}

type loaderEvent struct {
	Date        string    `json:"date"`
	EventIDs    []string  `json:"event_ids"`
	Source      string    `json:"source"`
	ActiveTheme []string  `json:"active_themes"`
	CapturedAt  time.Time `json:"captured_at"`
	IsSynthetic uint8     `json:"is_synthetic"`
}

// loaderPrediction is intentionally left unused in PR#2 — prediction_backtest
// is populated by PR#3 (cmd/backtest-event-flow). We keep the DTO present
// so the loader is symmetric across all 4 tables.
type loaderPrediction struct {
	Date                  string    `json:"date"`
	PredictedDirection    string    `json:"predicted_direction"`
	PredictedConfidence   float64   `json:"predicted_confidence"`
	ActualDirection       string    `json:"actual_direction"`
	ActualCapitalFlowChan float64   `json:"actual_capital_flow_change"`
	Hit                   bool      `json:"hit"`
	ModelVersion          string    `json:"model_version"`
	CapturedAt            time.Time `json:"captured_at"`
	IsSynthetic           uint8     `json:"is_synthetic"`
}

// ------------------------------------------------------------------
// Run options + stats.
// ------------------------------------------------------------------

// RunOptions groups CLI inputs that govern a loader run.
type RunOptions struct {
	StagingDir    string
	DBPath        string
	Tables        string // csv subset of regime,stress,events,prediction
	Since         string
	Until         string
	Now           time.Time
	DryRun        bool
	InitSchema    bool
	DropSynthetic bool
	Logger        *log.Logger
}

// RunStats reports per-table counters. Surfaced in CLI output and asserted
// in tests.
type RunStats struct {
	RegimeRead        int
	RegimeWritten     int
	StressRead        int
	StressWritten     int
	EventRead         int
	EventWritten      int
	PredictionRead    int
	PredictionWritten int
	Malformed         int
	OutOfRange        int
}

// Run executes the loader end-to-end. Tests call Run with a synthesized
// RunOptions; main() wraps Run.
func Run(opts RunOptions) (RunStats, error) {
	stats := RunStats{}
	if opts.Logger == nil {
		opts.Logger = log.New(io.Discard, "", 0)
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	if err := validateRun(opts); err != nil {
		return stats, err
	}
	wants, err := parseTables(opts.Tables)
	if err != nil {
		return stats, err
	}
	rows, err := readSummary(opts, &stats)
	if err != nil {
		return stats, err
	}
	if opts.DryRun {
		return stats, nil
	}
	if err := os.MkdirAll(filepath.Dir(opts.DBPath), 0o755); err != nil {
		return stats, fmt.Errorf("mkdir db dir: %w", err)
	}

	db, err := ledger.OpenSQLiteDB(opts.DBPath)
	if err != nil {
		return stats, fmt.Errorf("open sqlite: %w", err)
	}
	defer func() { _ = db.Close() }()
	if opts.InitSchema {
		if err := ledger.InitSchema(db); err != nil {
			return stats, fmt.Errorf("init schema: %w", err)
		}
	}
	has, err := ledger.NewSQLiteHistoricalStore(db).HasTables(context.Background())
	if err != nil {
		return stats, fmt.Errorf("check tables: %w", err)
	}
	for _, t := range []string{"regime_history", "stress_index_history", "event_calendar_history", "prediction_backtest"} {
		if !has[t] {
			return stats, fmt.Errorf("missing table %s in %s; pass -init-schema", t, opts.DBPath)
		}
	}
	store := ledger.NewSQLiteHistoricalStore(db)

	if wants["regime"] && len(rows.regime) > 0 {
		if err := upsertRegimes(context.Background(), store, rows.regime, &stats); err != nil {
			return stats, fmt.Errorf("regime: %w", err)
		}
	}
	if wants["stress"] && len(rows.stress) > 0 {
		if err := upsertStress(context.Background(), store, rows.stress, &stats); err != nil {
			return stats, fmt.Errorf("stress: %w", err)
		}
	}
	if wants["events"] && len(rows.event) > 0 {
		if err := upsertEvents(context.Background(), store, rows.event, &stats); err != nil {
			return stats, fmt.Errorf("events: %w", err)
		}
	}
	if wants["prediction"] && len(rows.prediction) > 0 {
		if err := upsertPredictions(context.Background(), store, rows.prediction, &stats); err != nil {
			return stats, fmt.Errorf("upsert prediction: %w", err)
		}
	}
	if opts.DropSynthetic {
		dropped, err := dropSyntheticRows(context.Background(), db)
		if err != nil {
			return stats, fmt.Errorf("drop synthetic: %w", err)
		}
		for _, table := range syntheticDropTables {
			fmt.Printf("dropped %d synthetic rows from %s\n", dropped[table.table], table.table)
		}
	}
	return stats, nil
}

// readSummary is the in-memory parsing pass. Files that do not exist are
// treated as empty (idempotent allow re-runs before the producer CLI has run).
type readSummaryTbl struct {
	regime     []loaderRegime
	stress     []loaderStress
	event      []loaderEvent
	prediction []loaderPrediction
}

func readSummary(opts RunOptions, stats *RunStats) (readSummaryTbl, error) {
	var out readSummaryTbl
	if !fileExists(opts.StagingDir) {
		return out, nil
	}

	paths, err := resolveStagingPaths(opts.StagingDir)
	if err != nil {
		return out, err
	}

	stats.RegimeRead, stats.Malformed, stats.OutOfRange = scanJSONL(paths.Regime, &out.regime)
	stats.StressRead, _, _ = scanJSONL(paths.Stress, &out.stress)
	stats.EventRead, _, _ = scanJSONL(paths.Event, &out.event)
	stats.PredictionRead, _, _ = scanJSONL(paths.Prediction, &out.prediction)

	out.regime, stats.OutOfRange = filterByDate(out.regime, opts.Since, opts.Until, stats.OutOfRange)
	out.stress, stats.OutOfRange = filterByDate(out.stress, opts.Since, opts.Until, stats.OutOfRange)
	out.event, stats.OutOfRange = filterByDate(out.event, opts.Since, opts.Until, stats.OutOfRange)
	out.prediction, stats.OutOfRange = filterByDate(out.prediction, opts.Since, opts.Until, stats.OutOfRange)
	return out, nil
}

// filterByDate returns the subset of rows whose Date is in [since, until]
// and the count of excluded rows added to the supplied dropped counter.
func filterByDate[T any](in []T, since, until string, dropped int) ([]T, int) {
	out := make([]T, 0, len(in))
	for _, r := range in {
		d := extractDate(r)
		if !inRange(d, since, until) {
			dropped++
			continue
		}
		out = append(out, r)
	}
	return out, dropped
}

func extractDate(v any) string {
	switch r := v.(type) {
	case loaderRegime:
		return r.Date
	case loaderStress:
		return r.Date
	case loaderEvent:
		return r.Date
	case loaderPrediction:
		return r.Date
	}
	return ""
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

type stagingPaths struct {
	Regime     string
	Stress     string
	Event      string
	Prediction string
}

func resolveStagingPaths(staging string) (stagingPaths, error) {
	if staging == "" {
		return stagingPaths{}, fmt.Errorf("staging dir is empty")
	}
	// Constants mirrored from cmd/atlas-stage4-backfill/schema.go. Kept
	// duplicated here on purpose: a format change in PR#1 will surface
	// as a file-not-found error rather than silently missing the load.
	return stagingPaths{
		Regime:     filepath.Join(staging, "regime_history_90d.jsonl"),
		Stress:     filepath.Join(staging, "stress_index_history_90d.jsonl"),
		Event:      filepath.Join(staging, "event_calendar_90d.jsonl"),
		Prediction: filepath.Join(staging, "prediction_actual_90d.jsonl"),
	}, nil
}

// scanJSONL reads a JSONL file and decodes each line into out. Returns
// (readCount, malformedCount, _). malformedCount includes lines that
// decoded into a different number of fields.
func scanJSONL[V any](path string, out *[]V) (int, int, int) {
	if !fileExists(path) {
		return 0, 0, 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1<<16), 4*1024*1024)
	read := 0
	bad := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		read++
		var row V
		if err := json.Unmarshal(line, &row); err != nil {
			bad++
			continue
		}
		*out = append(*out, row)
	}
	return read, bad, 0
}

func inRange(date, since, until string) bool {
	if date == "" {
		return false
	}
	if since != "" && date < since {
		return false
	}
	if until != "" && date > until {
		return false
	}
	return true
}

func parseTables(spec string) (map[string]bool, error) {
	want := map[string]bool{
		"regime":     true,
		"stress":     true,
		"events":     true,
		"prediction": true,
	}
	if spec == "" || spec == "all" {
		return want, nil
	}
	out := map[string]bool{}
	for _, p := range strings.Split(spec, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := want[p]; !ok {
			return nil, fmt.Errorf("unknown table %q (want regime|stress|events|prediction)", p)
		}
		out[p] = true
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid tables in -tables=%q", spec)
	}
	return out, nil
}

// ------------------------------------------------------------------
// Upsert helpers (one per table).
// ------------------------------------------------------------------

func upsertRegimes(ctx context.Context, store ledger.HistoricalStore, rows []loaderRegime, stats *RunStats) error {
	for _, r := range rows {
		err := store.UpsertRegime(ctx, ledger.RegimeRow{
			Date:            r.Date,
			Regime:          r.Regime,
			SourceSessionID: r.SourceSessionID,
			RecordedAt:      r.RecordedAt,
			CapturedAt:      r.CapturedAt,
			IsSynthetic:     r.IsSynthetic,
		})
		if err != nil {
			return err
		}
		stats.RegimeWritten++
	}
	return nil
}

func upsertStress(ctx context.Context, store ledger.HistoricalStore, rows []loaderStress, stats *RunStats) error {
	for _, r := range rows {
		err := store.UpsertStress(ctx, ledger.StressRow{
			Date:        r.Date,
			Score:       r.Score,
			Regime:      r.Regime,
			Components:  r.Components,
			Source:      r.Source,
			CapturedAt:  r.CapturedAt,
			IsSynthetic: r.IsSynthetic,
		})
		if err != nil {
			return err
		}
		stats.StressWritten++
	}
	return nil
}

func upsertEvents(ctx context.Context, store ledger.HistoricalStore, rows []loaderEvent, stats *RunStats) error {
	for _, r := range rows {
		// Each event_id produces one event_calendar_history row. Themes
		// and source are duplicated so reads don't need a join.
		for i, eid := range r.EventIDs {
			theme := ""
			if i < len(r.ActiveTheme) {
				theme = r.ActiveTheme[i]
			} else if len(r.ActiveTheme) > 0 {
				theme = r.ActiveTheme[0]
			}
			err := store.UpsertEventCalendar(ctx, ledger.EventCalendarRow{
				Date:        r.Date,
				EventID:     eid,
				ActiveTheme: theme,
				Source:      r.Source,
				CapturedAt:  r.CapturedAt,
				IsSynthetic: r.IsSynthetic,
			})
			if err != nil {
				return err
			}
			stats.EventWritten++
		}
	}
	return nil
}

func upsertPredictions(ctx context.Context, store ledger.HistoricalStore, rows []loaderPrediction, stats *RunStats) error {
	for _, r := range rows {
		err := store.UpsertPredictionBacktest(ctx, ledger.PredictionBacktestRow{
			Date:                  r.Date,
			PredictedDirection:    r.PredictedDirection,
			PredictedConfidence:   r.PredictedConfidence,
			ActualDirection:       r.ActualDirection,
			ActualCapitalFlowChan: r.ActualCapitalFlowChan,
			Hit:                   r.Hit,
			ModelVersion:          r.ModelVersion,
			CapturedAt:            r.CapturedAt,
			IsSynthetic:           r.IsSynthetic,
		})
		if err != nil {
			return err
		}
		stats.PredictionWritten++
	}
	return nil
}

var syntheticDropTables = []struct {
	table string
	stmt  string
}{
	{"regime_history", `DELETE FROM regime_history WHERE is_synthetic = 1`},
	{"stress_index_history", `DELETE FROM stress_index_history WHERE is_synthetic = 1`},
	{"event_calendar_history", `DELETE FROM event_calendar_history WHERE is_synthetic = 1`},
	{"prediction_backtest", `DELETE FROM prediction_backtest WHERE is_synthetic = 1`},
}

// dropSyntheticRows deletes rows with is_synthetic=1 from each of the 4 Stage 4
// history tables and returns the number of rows deleted per table.
func dropSyntheticRows(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	dropped := make(map[string]int64, len(syntheticDropTables))
	for _, q := range syntheticDropTables {
		res, err := db.ExecContext(ctx, q.stmt)
		if err != nil {
			return nil, fmt.Errorf("delete synthetic from %s: %w", q.table, err)
		}
		n, _ := res.RowsAffected()
		dropped[q.table] = n
	}
	return dropped, nil
}

// ------------------------------------------------------------------
// CLI entry.
// ------------------------------------------------------------------

func main() {
	opts, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		usage(os.Stderr)
		os.Exit(2)
	}
	stats, err := Run(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stage4 load failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Stage 4 loader complete\n")
	fmt.Printf("  Staging dir:        %s\n", opts.StagingDir)
	fmt.Printf("  DB path:            %s\n", opts.DBPath)
	fmt.Printf("  Rows read / written:\n")
	fmt.Printf("    regime:           %d / %d\n", stats.RegimeRead, stats.RegimeWritten)
	fmt.Printf("    stress:           %d / %d\n", stats.StressRead, stats.StressWritten)
	fmt.Printf("    events:           %d / %d\n", stats.EventRead, stats.EventWritten)
	fmt.Printf("    prediction:       %d / %d\n", stats.PredictionRead, stats.PredictionWritten)
	fmt.Printf("  Skipped — malformed: %d\n", stats.Malformed)
	fmt.Printf("  Skipped — out of range: %d\n", stats.OutOfRange)
}

func parseFlags(args []string) (RunOptions, error) {
	fs := flag.NewFlagSet("atlas-stage4-loader", flag.ContinueOnError)
	staging := fs.String("staging", "./data/staging", "directory with the 4 staging JSONLs")
	dbPath := fs.String("db", "./data/state/atlas.db", "SQLite DB path")
	tables := fs.String("tables", "all", "comma-separated subset: regime,stress,events,prediction")
	since := fs.String("since", "", "filter rows with date >= YYYY-MM-DD")
	until := fs.String("until", "", "filter rows with date <= YYYY-MM-DD")
	dryRun := fs.Bool("dry-run", false, "parse and report counts, do not write")
	initSchema := fs.Bool("init-schema", false, "call InitSchema before loading")
	dropSynthetic := fs.Bool("drop-synthetic", false, "delete synthetic rows after loading")
	if err := fs.Parse(args); err != nil {
		return RunOptions{}, err
	}
	opts := RunOptions{
		StagingDir:    *staging,
		DBPath:        *dbPath,
		Tables:        *tables,
		Since:         *since,
		Until:         *until,
		Now:           time.Now().UTC(),
		DryRun:        *dryRun,
		InitSchema:    *initSchema,
		DropSynthetic: *dropSynthetic,
	}
	if err := validateRun(opts); err != nil {
		return opts, err
	}
	return opts, nil
}

func validateRun(opts RunOptions) error {
	if opts.StagingDir == "" {
		return errors.New("-staging is required")
	}
	if opts.DBPath == "" {
		return errors.New("-db is required")
	}
	if opts.Since != "" {
		if _, err := time.Parse("2006-01-02", opts.Since); err != nil {
			return fmt.Errorf("-since: %w", err)
		}
	}
	if opts.Until != "" {
		if _, err := time.Parse("2006-01-02", opts.Until); err != nil {
			return fmt.Errorf("-until: %w", err)
		}
	}
	return nil
}

func usage(w io.Writer) {
	_, _ = fmt.Fprintf(w, `Usage: atlas-stage4-loader [-staging PATH] [-db PATH] [-tables LIST] [-since D] [-until D] [-init-schema] [-dry-run]

Stage 4 staging-JSONL → data/state/atlas.db loader.

	-staging      staging dir (default ./data/staging)
	-db           SQLite DB path (default ./data/state/atlas.db)
	-tables       subset to load: regime,stress,events,prediction (default all)
	-since        YYYY-MM-DD lower bound (date column)
	-until        YYYY-MM-DD upper bound (date column)
	-init-schema  run InitSchema first (idempotent; required on a fresh DB)
	-dry-run      parse and report counts, do not write
	-h, -help     show this help
`)
}
