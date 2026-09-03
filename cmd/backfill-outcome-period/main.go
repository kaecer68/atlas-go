// Command backfill-outcome-period backfills recommendation_outcomes with the
// seven-period market classification (capital-flow Phase 2 PR-2a).
//
// Background: PR-2a joins period_history at outcome-write time, so outcomes
// recorded before the feature shipped carry NULL market_period /
// market_period_source. This command fills that gap for every supported
// backend, matching each outcome's trading day against period_history.date:
//
//   - sqlite:   UPDATE outcomes against data/state/atlas.db (the trading day
//     is the first 10 chars of the stored timestamp).
//   - postgres: UPDATE recommendation_outcomes joined on
//     to_char(time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD') —
//     the SSoT backend on production.
//   - jsonl:    rewrites recommendation_outcomes.jsonl and
//     sessions/*/recommendation_outcomes.jsonl in place (temp +
//     rename), filling market_period on rows whose trading day has
//     a period_history row.
//
// The period provenance mirrors PR-2a write semantics: period_history rows
// with is_synthetic=1 (OHLCV backfill) set market_period_source='synthetic';
// live rows set 'live'. A trading day with no period_history row is left
// untouched (empty = "unknown" in period matrices) — never guessed.
//
// Usage:
//
//	backfill-outcome-period -workdir . -dry-run
//	backfill-outcome-period -workdir . -db data/state/atlas.db
//	backfill-outcome-period -workdir . -pg -pg-dsn postgres://...
//	backfill-outcome-period -workdir . -jsonl data/state
//
// All modes are idempotent (rows that already carry market_period are
// skipped) and safe to re-run.
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
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	atlasdb "github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// backfillResult summarizes one backend pass.
type backfillResult struct {
	Total     int // candidate rows (market_period IS NULL / empty) examined
	Matched   int // rows whose trading day resolved to a period_history row
	Unmatched int // rows left empty (no period_history row for the day)
	Errors    int // rows that failed to update
}

func (r backfillResult) String() string {
	return fmt.Sprintf("total=%d matched=%d unmatched=%d errors=%d", r.Total, r.Matched, r.Unmatched, r.Errors)
}

type runConfig struct {
	workDir string
	dbPath  string
	usePG   bool
	pgDSN   string
	jsonl   string
	dryRun  bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("backfill-outcome-period", flag.ContinueOnError)
	fs.SetOutput(stdout)
	cfg := runConfig{}
	fs.StringVar(&cfg.workDir, "workdir", ".", "working directory (config root)")
	fs.StringVar(&cfg.dbPath, "db", "", "sqlite db path (default: <workdir>/data/state/atlas.db)")
	fs.BoolVar(&cfg.usePG, "pg", false, "backfill PostgreSQL (SSoT production backend)")
	fs.StringVar(&cfg.pgDSN, "pg-dsn", "", "PostgreSQL DSN (default: $DATABASE_URL)")
	fs.StringVar(&cfg.jsonl, "jsonl", "", "rewrite JSONL outcome files under this dir (default: <workdir>/data/state)")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "report what would change without writing")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if cfg.workDir == "" {
		return fmt.Errorf("-workdir is required")
	}

	ctx := context.Background()
	switch {
	case cfg.usePG:
		return runPostgres(ctx, cfg, stdout)
	case cfg.jsonl != "":
		return runJSONL(ctx, cfg, stdout)
	default:
		return runSQLite(ctx, cfg, stdout)
	}
}

// openSQLite opens (and migrates) the SQLite ledger under cfg.
func openSQLite(cfg runConfig) (*sql.DB, error) {
	dbPath := cfg.dbPath
	if dbPath == "" {
		dbPath = filepath.Join(cfg.workDir, "data", "state", "atlas.db")
	}
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(cfg.workDir, dbPath)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	db, err := ledger.OpenSQLiteDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	if err := ledger.InitSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return db, nil
}

func runSQLite(ctx context.Context, cfg runConfig, stdout io.Writer) error {
	db, err := openSQLite(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	res, err := backfillSQLiteDB(ctx, db, cfg.dryRun)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "sqlite backfill %s (%s)\n", res.String(), dryRunLabel(cfg.dryRun))
	return nil
}

// backfillSQLiteDB backfills market_period on the SQLite outcomes table.
// Rows already carrying a value are never touched (idempotent). Rows whose
// day has no period_history row stay NULL (unknown, not guessed).
func backfillSQLiteDB(ctx context.Context, db *sql.DB, dryRun bool) (backfillResult, error) {
	var res backfillResult
	countQuery := `
		SELECT
			(SELECT COUNT(*) FROM outcomes WHERE market_period IS NULL),
			(SELECT COUNT(*) FROM outcomes WHERE market_period IS NULL
			   AND EXISTS (SELECT 1 FROM period_history WHERE date = substr(outcomes.timestamp, 1, 10)))`
	if err := db.QueryRowContext(ctx, countQuery).Scan(&res.Total, &res.Matched); err != nil {
		return res, fmt.Errorf("count candidates: %w", err)
	}
	res.Unmatched = res.Total - res.Matched
	if dryRun || res.Matched == 0 {
		return res, nil
	}
	updateQuery := `
		UPDATE outcomes SET
			market_period = (SELECT period FROM period_history WHERE date = substr(outcomes.timestamp, 1, 10)),
			market_period_source = (SELECT CASE WHEN is_synthetic = 1 THEN 'synthetic' ELSE 'live' END
			                          FROM period_history WHERE date = substr(outcomes.timestamp, 1, 10))
		WHERE market_period IS NULL
		  AND EXISTS (SELECT 1 FROM period_history WHERE date = substr(outcomes.timestamp, 1, 10))`
	if _, err := db.ExecContext(ctx, updateQuery); err != nil {
		return res, fmt.Errorf("update outcomes: %w", err)
	}
	return res, nil
}

func runJSONL(ctx context.Context, cfg runConfig, stdout io.Writer) error {
	db, err := openSQLite(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	hist := ledger.NewSQLiteHistoricalStore(db)

	dir := cfg.jsonl
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cfg.workDir, dir)
	}
	periodFor := func(date string) (string, string, bool) {
		row, ok, err := hist.LoadPeriodByDateAll(ctx, date)
		if err != nil || !ok {
			return "", "", false
		}
		src := "live"
		if row.IsSynthetic == 1 {
			src = "synthetic"
		}
		return row.Period, src, true
	}

	files := discoverJSONL(dir)
	var total, matched int
	for _, path := range files {
		n, m, err := backfillJSONLFile(path, periodFor, cfg.dryRun)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		total += n
		matched += m
		_, _ = fmt.Fprintf(stdout, "  %s: examined=%d filled=%d\n", path, n, m)
	}
	res := backfillResult{Total: total, Matched: matched, Unmatched: total - matched}
	_, _ = fmt.Fprintf(stdout, "jsonl backfill %s (%s)\n", res.String(), dryRunLabel(cfg.dryRun))
	return nil
}

func discoverJSONL(baseDir string) []string {
	var out []string
	global := filepath.Join(baseDir, "recommendation_outcomes.jsonl")
	if _, err := os.Stat(global); err == nil {
		out = append(out, global)
	}
	matches, _ := filepath.Glob(filepath.Join(baseDir, "sessions", "*", "recommendation_outcomes.jsonl"))
	out = append(out, matches...)
	return out
}

// backfillJSONLFile rewrites one JSONL outcome file, filling MarketPeriod /
// MarketPeriodSource on rows whose trading day resolves via periodFor.
// Rows that already carry a period pass through unchanged; the file is only
// rewritten when at least one row changed.
func backfillJSONLFile(path string, periodFor func(date string) (period, source string, ok bool), dryRun bool) (examined, filled int, err error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("stat %s: %w", path, err)
	}
	if info.Size() == 0 {
		return 0, 0, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	tmp, err := os.CreateTemp(filepath.Dir(path), ".backfill-outcome-period-*.tmp")
	if err != nil {
		return 0, 0, fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	changed := false
	line := 0
	writeRaw := func(raw string) error {
		if _, err := tmp.WriteString(raw); err != nil {
			return err
		}
		if _, err := tmp.WriteString("\n"); err != nil {
			return err
		}
		return nil
	}
	for scanner.Scan() {
		line++
		raw := scanner.Text()
		if strings.TrimSpace(raw) == "" {
			if err := writeRaw(""); err != nil {
				return 0, 0, err
			}
			continue
		}
		var o domain.RecommendationOutcome
		if err := json.Unmarshal([]byte(raw), &o); err != nil {
			return 0, 0, fmt.Errorf("line %d: decode: %w", line, err)
		}
		examined++
		if o.MarketPeriod != "" || o.MarketPeriodSource != "" {
			if err := writeRaw(raw); err != nil {
				return 0, 0, err
			}
			continue
		}
		period, source, ok := periodFor(tradingDateOf(o))
		if !ok {
			if err := writeRaw(raw); err != nil {
				return 0, 0, err
			}
			continue
		}
		filled++
		o.MarketPeriod = period
		o.MarketPeriodSource = source
		out, err := json.Marshal(o)
		if err != nil {
			return 0, 0, fmt.Errorf("line %d: encode: %w", line, err)
		}
		if err := writeRaw(string(out)); err != nil {
			return 0, 0, err
		}
		changed = true
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("scan %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return 0, 0, err
	}
	if changed && !dryRun {
		if err := os.Rename(tmpPath, path); err != nil {
			return 0, 0, fmt.Errorf("rename over %s: %w", path, err)
		}
	}
	return examined, filled, nil
}

// tradingDateOf extracts the outcome's trading day (YYYY-MM-DD). The write
// path stores Window = asOf.Format("2006-01-02") for daily outcomes, so a
// date-shaped Window wins; RecordedAt is the fallback.
func tradingDateOf(o domain.RecommendationOutcome) string {
	if len(o.Window) == 10 && o.Window[4] == '-' && o.Window[7] == '-' {
		return o.Window
	}
	if !o.RecordedAt.IsZero() {
		return o.RecordedAt.Format("2006-01-02")
	}
	return ""
}

func runPostgres(ctx context.Context, cfg runConfig, stdout io.Writer) error {
	dsn := cfg.pgDSN
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		return fmt.Errorf("-pg requires -pg-dsn or $DATABASE_URL")
	}
	migrationsPath := filepath.Join(cfg.workDir, "sql", "migrations")
	pool, err := atlasdb.Init(ctx, dsn, migrationsPath)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	res, err := backfillPostgres(ctx, pool, cfg.dryRun)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "postgres backfill %s (%s)\n", res.String(), dryRunLabel(cfg.dryRun))
	return nil
}

// backfillPostgres backfills market_period on recommendation_outcomes via a
// period_history join. Trading day = the outcome instant in Asia/Taipei
// (outcomes are session-dated in Taipei time; pgx stores the instant UTC).
func backfillPostgres(ctx context.Context, pool *pgxpool.Pool, dryRun bool) (backfillResult, error) {
	var res backfillResult
	countQuery := `
		SELECT
			(SELECT COUNT(*) FROM recommendation_outcomes WHERE market_period IS NULL),
			(SELECT COUNT(*) FROM recommendation_outcomes o
			   WHERE o.market_period IS NULL
			     AND EXISTS (SELECT 1 FROM period_history ph
			                 WHERE ph.date = to_char(o.time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD')))`
	if err := pool.QueryRow(ctx, countQuery).Scan(&res.Total, &res.Matched); err != nil {
		return res, fmt.Errorf("count candidates: %w", err)
	}
	res.Unmatched = res.Total - res.Matched
	if dryRun || res.Matched == 0 {
		return res, nil
	}
	updateQuery := `
		UPDATE recommendation_outcomes o
		SET market_period = ph.period,
		    market_period_source = CASE WHEN ph.is_synthetic = 1 THEN 'synthetic' ELSE 'live' END
		FROM period_history ph
		WHERE o.market_period IS NULL
		  AND ph.date = to_char(o.time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD')`
	if _, err := pool.Exec(ctx, updateQuery); err != nil {
		return res, fmt.Errorf("update recommendation_outcomes: %w", err)
	}
	return res, nil
}

func dryRunLabel(dryRun bool) string {
	if dryRun {
		return "dry-run (no writes)"
	}
	return "wrote"
}
