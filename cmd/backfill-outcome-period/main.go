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
//	backfill-outcome-period -workdir .                     # 後端跟隨 ATLAS_STORE_BACKEND
//	backfill-outcome-period -workdir . -db data/state/atlas.db
//	backfill-outcome-period -workdir . -pg -pg-dsn postgres://...
//	backfill-outcome-period -workdir . -jsonl data/state
//	backfill-outcome-period -workdir . -start 2026-04-01 -end 2026-06-30   # 只動這個交易日窗口
//
// Trading-day window and blast-radius guard (#2124): -start/-end bound the
// trading day (Asia/Taipei, inclusive) in **every** mode — candidates, the SQL
// UPDATE, and the JSONL rewrite — so a run cannot silently touch rows outside
// the window the operator asked for. When the SQL modes would update more than
// maxUnattendedRows rows in one run, the command reports the count and refuses
// to write unless -force is passed. The report line always prints the window
// (window=(whole table) when no bound is set, which is the legacy behavior).
//
// Backend decision (#2107): the mode follows the **declared**
// ATLAS_STORE_BACKEND unless an explicit -pg / -jsonl / -db overrides it. It
// is deliberately NOT "sqlite by default" — that shape let a host with
// ATLAS_STORE_BACKEND=postgres silently open the job-local sqlite artifact
// (data/state/atlas.db). A declared backend with no implementation here
// (jsonl: period_history is a relational table) is a hard error, never a
// silent downgrade; `-jsonl <dir>` remains the explicit way to rewrite the
// JSONL outcome files.
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

	"github.com/kaecer68/atlas-go/internal/config"
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
	// dbExplicit 表示 -db 由使用者顯式給定（不是預設值）；顯式 flag 覆寫
	// ATLAS_STORE_BACKEND 的宣告（#2107）。
	dbExplicit bool
	usePG      bool
	pgDSN      string
	jsonl      string
	// start / end 是**交易日**窗口（Asia/Taipei 的 YYYY-MM-DD，含端點）。
	// 空字串 = 不設界（沿用舊行為），但此時影響列數上限（maxUnattendedRows）就是
	// 唯一的護欄 —— 那正是 #2124 要修的形狀：一次 UPDATE 改寫整張表。
	start string
	end   string
	// force 明示允許超過 maxUnattendedRows 的自動更新（#2124）。
	force  bool
	dryRun bool
}

// maxUnattendedRows 是「沒有明示 -force 時可自動更新的列數上限」。
//
// 這條 CLI 的意外形狀不是「多改一列」，而是「整張表被改寫」：market_period IS NULL
// 的歷史列只要當天有 period_history 就會被填。上限不是精準的權限控制，而是讓
// 「影響範圍明顯大於預期」時停下來要人明示（-force 或縮小 -start/-end）。
//
// 是 var（非 const）以便測試用低門檻驗證「超門檻 ⇒ 中止且不寫入」，
// 與本檔的 initPostgresPool seam 同一手法。
var maxUnattendedRows = 1000

// initPostgresPool 是 atlasdb.Init（ping ＋ 套用 <workdir>/sql/migrations）的 seam：
// 單元測試替換它就能在沒有 PostgreSQL 的情況下驗證 postgres 路徑的決策與 wiring。
var initPostgresPool = atlasdb.Init

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
	fs.StringVar(&cfg.start, "start", "", "trading-day window start YYYY-MM-DD (Asia/Taipei, inclusive; default: no bound)")
	fs.StringVar(&cfg.end, "end", "", "trading-day window end YYYY-MM-DD (Asia/Taipei, inclusive; default: no bound)")
	fs.BoolVar(&cfg.force, "force", false,
		fmt.Sprintf("allow updating more than %d rows in one run (the SQL modes report the affected count before writing)", maxUnattendedRows))
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
	if err := validateWindow(cfg.start, cfg.end); err != nil {
		return err
	}
	dbSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			dbSet = true
		}
	})
	cfg.dbExplicit = dbSet
	if err := checkModeFlags(dbSet, cfg.usePG, cfg.jsonl); err != nil {
		return err
	}

	appCfg := config.Load()
	mode, err := resolveMode(cfg, appCfg)
	if err != nil {
		return err
	}

	ctx := context.Background()
	switch mode {
	case modePostgres:
		return runPostgres(ctx, cfg, appCfg, stdout)
	case modeSQLite:
		return runSQLite(ctx, cfg, stdout)
	case modeJSONL:
		return runJSONL(ctx, cfg, appCfg, stdout)
	default:
		return fmt.Errorf("unexpected store mode %q", mode)
	}
}

// checkModeFlags 拒絕互相矛盾的顯式 flag：-db（sqlite）不能與 -pg／-jsonl 同時給。
// 全部都不給時模式跟隨 ATLAS_STORE_BACKEND（#2107）。
func checkModeFlags(dbSet, usePG bool, jsonlDir string) error {
	if dbSet && (usePG || jsonlDir != "") {
		return fmt.Errorf("-db is mutually exclusive with -pg / -jsonl (pick one mode explicitly, or drop all and let ATLAS_STORE_BACKEND decide)")
	}
	return nil
}

// storeMode 是本指令的輸出目標模式（單一決策路徑，#2107）。
type storeMode string

const (
	modePostgres storeMode = "postgres"
	modeSQLite   storeMode = "sqlite"
	modeJSONL    storeMode = "jsonl"
)

// resolveMode 決定本次執行的輸出目標。
//
// 顯式 flag 優先（-pg ＞ -jsonl ＞ -db），否則**跟隨宣告**的 ATLAS_STORE_BACKEND。
// 宣告 jsonl 是明確錯誤：本指令填的是關聯式欄位（period_history join
// recommendation_outcomes），HistoricalStore 沒有 jsonl 實作；要改寫 jsonl 檔
// 請顯式給 -jsonl <dir>。刻意不「沒有 flag 就寫 sqlite」—— 那正是 #2107。
func resolveMode(cfg runConfig, appCfg config.Config) (storeMode, error) {
	switch {
	case cfg.usePG:
		return modePostgres, nil
	case cfg.jsonl != "":
		return modeJSONL, nil
	case cfg.dbExplicit:
		return modeSQLite, nil
	}
	backend, err := ledger.ResolveStoreBackend(appCfg.StoreBackend)
	if err != nil {
		return "", err
	}
	switch backend {
	case "postgres":
		return modePostgres, nil
	case "sqlite":
		return modeSQLite, nil
	default:
		return "", fmt.Errorf(
			"store backend %q has no relational implementation (period_history and recommendation_outcomes are relational tables): pass -db <path>, -pg, or -jsonl <dir> explicitly",
			backend)
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

// periodHistoryStore 開啟 period_history 的讀取來源。
//
// 宣告的後端（或顯式 -pg）是 postgres ⇒ 自行開池（-pg-dsn，預設 $DATABASE_URL；
// migrations 取 <workdir>/sql/migrations）並用 PostgresHistoricalStore；其餘 ⇒
// sqlite（-db 或預設 <workdir>/data/state/atlas.db）。沒有 DSN 時明確失敗，
// 絕不退回 sqlite（#2107）。
func periodHistoryStore(ctx context.Context, cfg runConfig, appCfg config.Config) (ledger.HistoricalStore, func() error, error) {
	backend, err := ledger.ResolveStoreBackend(appCfg.StoreBackend)
	if err != nil {
		return nil, nil, err
	}
	if cfg.usePG || backend == "postgres" {
		dsn := cfg.pgDSN
		if dsn == "" {
			dsn = appCfg.DatabaseURL
		}
		if dsn == "" {
			return nil, nil, fmt.Errorf(
				"period_history source: backend %q requires a PostgreSQL DSN: pass -pg-dsn or set DATABASE_URL (refusing to fall back to sqlite)",
				backend)
		}
		pool, err := initPostgresPool(ctx, dsn, filepath.Join(cfg.workDir, "sql", "migrations"))
		if err != nil {
			return nil, nil, fmt.Errorf("connect postgres: %w", err)
		}
		return ledger.NewPostgresHistoricalStore(pool), func() error { pool.Close(); return nil }, nil
	}
	db, err := openSQLite(cfg)
	if err != nil {
		return nil, nil, err
	}
	return ledger.NewSQLiteHistoricalStore(db), db.Close, nil
}

func runSQLite(ctx context.Context, cfg runConfig, stdout io.Writer) error {
	db, err := openSQLite(cfg)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	res, err := backfillSQLiteDB(ctx, db, cfg)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "sqlite backfill %s window=%s (%s)\n",
		res.String(), windowLabel(cfg.start, cfg.end), dryRunLabel(cfg.dryRun))
	return nil
}

// backfillSQLiteDB backfills market_period on the SQLite outcomes table.
// Rows already carrying a value are never touched (idempotent). Rows whose
// day has no period_history row stay NULL (unknown, not guessed).
//
// #2124: the trading day is bounded by -start/-end in **both** the candidate
// count and the UPDATE, so a run can no longer rewrite rows outside the window
// the operator asked for. The same guard (maxUnattendedRows / -force) applies.
func backfillSQLiteDB(ctx context.Context, db *sql.DB, cfg runConfig) (backfillResult, error) {
	var res backfillResult
	countQuery := `
		SELECT
			(SELECT COUNT(*) FROM outcomes WHERE market_period IS NULL
			   AND (? = '' OR substr(outcomes.timestamp, 1, 10) >= ?)
			   AND (? = '' OR substr(outcomes.timestamp, 1, 10) <= ?)),
			(SELECT COUNT(*) FROM outcomes WHERE market_period IS NULL
			   AND (? = '' OR substr(outcomes.timestamp, 1, 10) >= ?)
			   AND (? = '' OR substr(outcomes.timestamp, 1, 10) <= ?)
			   AND EXISTS (SELECT 1 FROM period_history WHERE date = substr(outcomes.timestamp, 1, 10)))`
	windowArgs := []any{cfg.start, cfg.start, cfg.end, cfg.end}
	if err := db.QueryRowContext(ctx, countQuery, append(append([]any{}, windowArgs...), windowArgs...)...).
		Scan(&res.Total, &res.Matched); err != nil {
		return res, fmt.Errorf("count candidates: %w", err)
	}
	res.Unmatched = res.Total - res.Matched
	if cfg.dryRun || res.Matched == 0 {
		return res, nil
	}
	// 護欄必須在 UPDATE 之前（#2124）。
	if err := guardLargeUpdate(res, cfg.force); err != nil {
		return res, err
	}
	updateQuery := `
		UPDATE outcomes SET
			market_period = (SELECT period FROM period_history WHERE date = substr(outcomes.timestamp, 1, 10)),
			market_period_source = (SELECT CASE WHEN is_synthetic = 1 THEN 'synthetic' ELSE 'live' END
			                          FROM period_history WHERE date = substr(outcomes.timestamp, 1, 10))
		WHERE market_period IS NULL
		  AND EXISTS (SELECT 1 FROM period_history WHERE date = substr(outcomes.timestamp, 1, 10))
		  AND (? = '' OR substr(outcomes.timestamp, 1, 10) >= ?)
		  AND (? = '' OR substr(outcomes.timestamp, 1, 10) <= ?)`
	if _, err := db.ExecContext(ctx, updateQuery, windowArgs...); err != nil {
		return res, fmt.Errorf("update outcomes: %w", err)
	}
	return res, nil
}

func runJSONL(ctx context.Context, cfg runConfig, appCfg config.Config, stdout io.Writer) error {
	// period_history 的讀取來源跟隨宣告的後端：宣告 postgres（生產）時讀 PG，
	// 其餘讀 sqlite。刻意不「-jsonl 就開一份 job-local sqlite」—— 那正是 #2107。
	hist, closeHist, err := periodHistoryStore(ctx, cfg, appCfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeHist() }()

	dir := cfg.jsonl
	if dir == "" {
		dir = filepath.Join("data", "state")
	}
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
	// #2124：同一組 -start/-end 也套在 jsonl 模式（逐列以交易日過濾），
	// 否則「同一個 flag 在某個模式被靜默忽略」就是另一種坑。
	periodForWindowed := func(date string) (string, string, bool) {
		if !inWindow(cfg.start, cfg.end, date) {
			return "", "", false
		}
		return periodFor(date)
	}
	for _, path := range files {
		n, m, err := backfillJSONLFile(path, periodForWindowed, cfg.dryRun)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		total += n
		matched += m
		_, _ = fmt.Fprintf(stdout, "  %s: examined=%d filled=%d\n", path, n, m)
	}
	res := backfillResult{Total: total, Matched: matched, Unmatched: total - matched}
	_, _ = fmt.Fprintf(stdout, "jsonl backfill %s window=%s (%s)\n",
		res.String(), windowLabel(cfg.start, cfg.end), dryRunLabel(cfg.dryRun))
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

func runPostgres(ctx context.Context, cfg runConfig, appCfg config.Config, stdout io.Writer) error {
	dsn := cfg.pgDSN
	if dsn == "" {
		dsn = appCfg.DatabaseURL
	}
	if dsn == "" {
		// 明確可診斷：說清楚是哪個模式、要補哪個 flag／環境變數。
		return fmt.Errorf("postgres mode requires a PostgreSQL DSN: pass -pg-dsn or set DATABASE_URL (refusing to fall back to sqlite)")
	}
	pool, err := initPostgresPool(ctx, dsn, filepath.Join(cfg.workDir, "sql", "migrations"))
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()

	res, err := backfillPostgres(ctx, pool, cfg)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "postgres backfill %s window=%s (%s)\n",
		res.String(), windowLabel(cfg.start, cfg.end), dryRunLabel(cfg.dryRun))
	return nil
}

// backfillPostgres backfills market_period on recommendation_outcomes via a
// period_history join. Trading day = the outcome instant in Asia/Taipei
// (outcomes are session-dated in Taipei time; pgx stores the instant UTC).
// backfillPostgres backfills market_period on recommendation_outcomes via a
// period_history join. Trading day = the outcome instant in Asia/Taipei
// (outcomes are session-dated in Taipei time; pgx stores the instant UTC).
//
// #2124: the trading day is bounded by -start/-end in both the candidate count
// and the UPDATE. Before this change the UPDATE had no window at all, so a
// manual run could rewrite the whole table (every row whose day has a
// period_history row) while the operator believed they scoped it.
func backfillPostgres(ctx context.Context, pool *pgxpool.Pool, cfg runConfig) (backfillResult, error) {
	var res backfillResult
	countQuery := `
		SELECT
			(SELECT COUNT(*) FROM recommendation_outcomes o
			   WHERE o.market_period IS NULL
			     AND ($1 = '' OR to_char(o.time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD') >= $1)
			     AND ($2 = '' OR to_char(o.time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD') <= $2)),
			(SELECT COUNT(*) FROM recommendation_outcomes o
			   WHERE o.market_period IS NULL
			     AND ($1 = '' OR to_char(o.time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD') >= $1)
			     AND ($2 = '' OR to_char(o.time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD') <= $2)
			     AND EXISTS (SELECT 1 FROM period_history ph
			                 WHERE ph.date = to_char(o.time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD')))`
	if err := pool.QueryRow(ctx, countQuery, cfg.start, cfg.end).Scan(&res.Total, &res.Matched); err != nil {
		return res, fmt.Errorf("count candidates: %w", err)
	}
	res.Unmatched = res.Total - res.Matched
	if cfg.dryRun || res.Matched == 0 {
		return res, nil
	}
	// 護欄必須在 UPDATE 之前（#2124）。
	if err := guardLargeUpdate(res, cfg.force); err != nil {
		return res, err
	}
	updateQuery := `
		UPDATE recommendation_outcomes o
		SET market_period = ph.period,
		    market_period_source = CASE WHEN ph.is_synthetic = 1 THEN 'synthetic' ELSE 'live' END
		FROM period_history ph
		WHERE o.market_period IS NULL
		  AND ph.date = to_char(o.time AT TIME ZONE 'Asia/Taipei', 'YYYY-MM-DD')
		  AND ($1 = '' OR ph.date >= $1)
		  AND ($2 = '' OR ph.date <= $2)`
	if _, err := pool.Exec(ctx, updateQuery, cfg.start, cfg.end); err != nil {
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
