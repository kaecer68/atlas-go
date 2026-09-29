// Command backfill-period-history-range backfills period_history from
// OHLCV-only data for a configurable date range.
//
// Background: #1740 Phase 1 R9 / Phase 0 report shows that period_history
// only carries 101 rows (the live ingest window 2026-04-25..2026-08-22).
// This command lets the operator populate the historical window
// 2020-2024 with detector output derived from OHLCV alone, using
// finmind_2020_2024.jsonl as the data source.
//
// Per-date pipeline (mirrors Phase 0 measurement):
//
//	JSONL rows → TAIEX proxy (default 0050.TW close) + MarketVolume (sum)
//	            → []portfolio.SnapshotEntry[0..i]
//	            → calc.Enrich(ind, entries)        // sets TAIEX MA5/MA20/Slope + MarketVolumeMA20
//	            → detector.DetectAssessment(ind)   // classifies the day
//	            → store.UpsertPeriod(ctx, PeriodRow{...})
//
// Field coverage (per Phase 0 report §1, 39 fields total):
//   - TAIEXPrice, TAIEXMA5, TAIEXMA20, TAIEXMA20Slope, MarketVolume, MarketVolumeMA20
//     are filled from OHLCV (6/39).
//   - The remaining 33 fields stay zero-valued (VIX, DXY, SOX*, Foreign*, TWD*,
//     Margin*, DayTradeRatio, PublicBank*, SectorRotationFlag, NationalFundActive,
//     Geo*) — the detector treats zero as "data unavailable" and skips those
//     conditions (per-period PeriodDetector logic).
//
// Detector consequence (per Phase 0 report §3): only black_swan (TAIEX deviates
// from MA20 < -5%) and consolidation (default fall-through) trigger; the other
// five periods are unreachable from OHLCV alone. is_fallback stays false for
// every day because TAIEXPrice is non-zero on every in-window day (see Phase 0
// report §2 indicatorsHaveData contract).
//
// This is acceptable for Phase 1 R9: the goal is to populate period_history
// with a deterministic, idempotent row per trading day so downstream tooling
// can be exercised. R1-R4 data ingest (macro/foreign/margin) will refine the
// classifier in later phases.
//
// Usage:
//
//	backfill-period-history-range -workdir . -dry-run
//	backfill-period-history-range -workdir .                    # 後端跟隨 ATLAS_STORE_BACKEND
//	backfill-period-history-range -workdir . -db data/state/atlas.db
//	backfill-period-history-range -workdir . -start 2020-01-02 -end 2024-12-31
//	backfill-period-history-range -workdir . -source /path/to/other.jsonl
//	backfill-period-history-range -workdir . -pg -pg-dsn postgres://...
//
// Storage backend (#2107): the backend follows the **declared**
// ATLAS_STORE_BACKEND; an explicit -pg / -db overrides it. It is deliberately
// NOT "sqlite by default, Postgres on request" — that shape let a host with
// ATLAS_STORE_BACKEND=postgres silently write the job-local sqlite artifact
// (data/state/atlas.db). With a postgres backend this command opens its own
// pool (-pg-dsn, default $DATABASE_URL; migrations from
// <workdir>/sql/migrations), injects it into the ledger factory, and fails
// loudly when no DSN is available. A declared backend with no implementation
// here (jsonl) is a hard error too.
//
// Semantics: upsert by date (ON CONFLICT(date) DO UPDATE), idempotent — safe
// to re-run. Dates that already hold a live (is_synthetic=0) row are skipped
// so a backfill run can never clobber newer ingest data.
//
// Constraint: no network calls. Operates entirely on the local JSONL file.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	atlasdb "github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

const (
	// sourceName is written into period_history.source for every backfilled
	// row. Mirrors the existing convention in cmd/backfill-period-history
	// (which uses "period_history_backfill") but flags this run as
	// OHLCV-derived so downstream consumers can distinguish it from the
	// macro-snapshot path.
	sourceName = "period_history_range_backfill_ohlcv"
	// defaultSourcePath is the canonical Phase 0 JSONL path (relative to
	// -workdir). Phase 0 verified 42,366 rows / 35 symbols / 1,216 dates
	// covering 2020-01-02..2024-12-31.
	defaultSourcePath = "data/replay/finmind_2020_2024.jsonl"
	// defaultTAIEXProxy is the symbol whose close is used as the TAIEX
	// proxy for the OHLCV-only pipeline. 0050.TW is the largest TWSE
	// ETF and has full 1,216-day coverage in the Phase 0 dataset; the
	// Phase 0 report uses the same proxy for its 1,216-day measurement.
	defaultTAIEXProxy = "0050.TW"
	// defaultStart / defaultEnd pin the standard 2020-2024 window.
	defaultStart = "2020-01-02"
	defaultEnd   = "2024-12-31"
)

// ---------------------------------------------------------------------------
// JSONL record
// ---------------------------------------------------------------------------

// ohlcvRow is one line of the FinMind JSONL feed (8 columns).
// Matches the schema described in Phase 0 report §5.2.
type ohlcvRow struct {
	Date   string  `json:"date"`
	Symbol string  `json:"symbol"`
	Name   string  `json:"name"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// ---------------------------------------------------------------------------
// Configuration & stats
// ---------------------------------------------------------------------------

type runConfig struct {
	workDir    string
	sourcePath string // OHLCV JSONL path (relative to workDir unless absolute)
	start      string // YYYY-MM-DD inclusive; "" = first date in dataset
	end        string // YYYY-MM-DD inclusive; "" = last date in dataset
	dryRun     bool
	dbPath     string
	// dbExplicit 表示 -db 由使用者顯式給定（不是預設值）；顯式 flag 覆寫
	// ATLAS_STORE_BACKEND 的宣告（#2107）。
	dbExplicit bool
	usePG      bool
	pgDSN      string
	taiexProxy string // symbol used for TAIEXPrice
	now        func() time.Time
	store      ledger.HistoricalStore // injectable; nil = open sink
}

type runStats struct {
	rowsRead         int
	datesInDataset   int
	datesInRange     int
	processed        int
	periodCount      map[string]int
	fallbackCount    int
	upsertPeriod     int
	skippedLive      int
	errors           int
	errorDates       []string
	firstDate        string
	lastDate         string
	taiexMissingDays int // days where the TAIEX proxy symbol had no close (→ TAIEX=0)
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

func main() {
	if err := runFromOSArgs(); err != nil {
		log.Fatalf("backfill-period-history-range: %v", err)
	}
}

func runFromOSArgs() error {
	var (
		workDir    = flag.String("workdir", ".", "atlas repo root (must contain data/state)")
		sourcePath = flag.String("source", defaultSourcePath, "OHLCV JSONL path (absolute or relative to -workdir)")
		start      = flag.String("start", defaultStart, "backfill start date YYYY-MM-DD inclusive (default: 2020-01-02)")
		end        = flag.String("end", defaultEnd, "backfill end date YYYY-MM-DD inclusive (default: 2024-12-31)")
		dryRun     = flag.Bool("dry-run", false, "print what would be written without touching the DB")
		dbPath     = flag.String("db", "data/state/atlas.db", "explicit SQLite override (relative to -workdir); without it the backend follows ATLAS_STORE_BACKEND")
		usePG      = flag.Bool("pg", false, "explicit PostgreSQL override (DSN: -pg-dsn or $DATABASE_URL)")
		pgDSN      = flag.String("pg-dsn", "", "PostgreSQL DSN (default: $DATABASE_URL); used by the postgres backend")
		taiexProxy = flag.String("taiex-proxy", defaultTAIEXProxy, "symbol whose close is used as TAIEX proxy")
	)
	flag.Parse()

	for _, s := range []string{*start, *end} {
		if s != "" {
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return fmt.Errorf("invalid date %q (want YYYY-MM-DD): %w", s, err)
			}
		}
	}
	dbSet := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			dbSet = true
		}
	})
	if err := checkBackendFlags(dbSet, *usePG); err != nil {
		return err
	}
	if *usePG && *pgDSN == "" && os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("-pg requires -pg-dsn or DATABASE_URL (refusing to fall back to sqlite)")
	}

	_, err := run(context.Background(), runConfig{
		workDir:    *workDir,
		sourcePath: *sourcePath,
		start:      *start,
		end:        *end,
		dryRun:     *dryRun,
		dbPath:     *dbPath,
		dbExplicit: dbSet,
		usePG:      *usePG,
		pgDSN:      *pgDSN,
		taiexProxy: *taiexProxy,
	})
	return err
}

// ---------------------------------------------------------------------------
// Core pipeline
// ---------------------------------------------------------------------------

// run executes the backfill end-to-end and returns stats. Tests call run
// directly with a synthesized runConfig (cfg.store / cfg.now injectable);
// main() wraps run and prints the summary.
//
// Pipeline:
//  1. Read JSONL (single pass) → daySeries[date] = {TAIEX, MarketVolume}.
//  2. Build time-sorted series of SnapshotEntry.
//  3. For each date in [start, end], feed [0..i+1] entries to calc.Enrich.
//  4. Detector classifies; UpsertPeriod writes the row.
func run(ctx context.Context, cfg runConfig) (*runStats, error) {
	if cfg.now == nil {
		cfg.now = time.Now
	}

	stats := &runStats{
		periodCount: map[string]int{},
	}
	// 後端決策跟隨宣告（ATLAS_STORE_BACKEND）；顯式 flag 已在 backendFor 內覆寫。
	appCfg := config.Load()
	backendLabel := "(injected store)"
	if cfg.store == nil {
		if b, bErr := backendFor(cfg, appCfg); bErr == nil {
			backendLabel = b
		}
	}

	srcPath := cfg.sourcePath
	if !filepath.IsAbs(srcPath) {
		srcPath = filepath.Join(cfg.workDir, srcPath)
	}
	dayMap, err := loadJSONL(srcPath, cfg.taiexProxy)
	if err != nil {
		return stats, fmt.Errorf("load JSONL: %w", err)
	}
	stats.rowsRead = sumRowCount(dayMap)

	// Build sorted date series of TAIEX/MarketVolume.
	dates := make([]string, 0, len(dayMap))
	for d := range dayMap {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	stats.datesInDataset = len(dates)
	if len(dates) > 0 {
		stats.firstDate = dates[0]
		stats.lastDate = dates[len(dates)-1]
	}

	// Filter to [start, end].
	var filtered []string
	for _, d := range dates {
		if cfg.start != "" && d < cfg.start {
			continue
		}
		if cfg.end != "" && d > cfg.end {
			continue
		}
		filtered = append(filtered, d)
	}
	stats.datesInRange = len(filtered)
	if len(filtered) == 0 {
		return stats, fmt.Errorf("no trading dates in [%s, %s] (dataset spans %s..%s)",
			cfg.start, cfg.end, stats.firstDate, stats.lastDate)
	}

	var store ledger.HistoricalStore
	var closeStore func() error
	if cfg.store != nil {
		store = cfg.store
	} else if !cfg.dryRun {
		store, closeStore, err = openSink(ctx, cfg, appCfg)
		if err != nil {
			return stats, err
		}
		defer func() { _ = closeStore() }()
	}

	calc := portfolio.NewCalculator()
	detector := portfolio.NewPeriodDetectorWithDefaults()

	// entries is a rolling window of SnapshotEntry; we append one per day
	// inside the loop and pass the prefix [0..i] (entries[:i+1]) to Enrich.
	entries := make([]portfolio.SnapshotEntry, 0, len(filtered))

	for _, date := range filtered {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		day := dayMap[date]

		// Append today's entry. SOX, TSMADR, USDTWD, ForeignInvestorNet,
		// ForeignFuturesOINet all stay zero (no macro data) — the detector
		// guards handle zero by skipping those conditions.
		entries = append(entries, portfolio.SnapshotEntry{
			TradingDate:  date,
			TAIEX:        day.taiex,
			MarketVolume: day.volume,
		})
		if day.taiex == 0 {
			stats.taiexMissingDays++
		}

		// Zero-init the indicator struct; Enrich fills the OHLCV-derived
		// subset (TAIEX MA5/MA20/Slope + MarketVolumeMA20). All other
		// fields remain 0 / false, signaling "data unavailable" to the
		// detector (per period_detector.go semantics).
		ind := portfolio.PeriodIndicators{}
		calc.Enrich(&ind, entries)
		ind.TAIEXPrice = day.taiex // overwrite: Enrich doesn't set today's price
		ind.MarketVolume = day.volume

		assessment, derr := detector.DetectAssessment(ind)
		if derr != nil {
			stats.errors++
			stats.errorDates = append(stats.errorDates, date)
			fmt.Fprintf(os.Stderr, "[%s] detect: %v\n", date, derr)
			continue
		}
		stats.processed++
		stats.periodCount[string(assessment.MarketPeriod)]++
		if assessment.IsFallback {
			stats.fallbackCount++
		}

		if cfg.dryRun {
			printDryRunLine(date, day.taiex, assessment)
			continue
		}

		written, skipped, err := upsertDay(ctx, store, date, assessment, cfg.now())
		if err != nil {
			stats.errors++
			stats.errorDates = append(stats.errorDates, date)
			fmt.Fprintf(os.Stderr, "[%s] upsert: %v\n", date, err)
			continue
		}
		stats.upsertPeriod += written
		stats.skippedLive += skipped
	}

	printSummary(cfg, stats, backendLabel)
	return stats, nil
}

// ---------------------------------------------------------------------------
// JSONL loading
// ---------------------------------------------------------------------------

// perDay is one calendar date's aggregated OHLCV view: the TAIEX proxy's
// close price and the total volume across all symbols.
type perDay struct {
	taiex  float64 // TAIEX proxy close (0050.TW by default)
	volume float64 // sum of every symbol's volume on this date
}

// loadJSONL streams the OHLCV file and returns one perDay per date, sorted
// by date implicitly (caller sorts). The TAIEX proxy's close is taken from
// cfg.taiexProxy; if that symbol is missing for a date, the day is left
// with taiex=0 (counted by stats.taiexMissingDays).
//
// We do a single linear pass — the file is small enough (~5.7 MB) for the
// in-memory map; the test fixture uses a synthetic 100-day file.
func loadJSONL(path, taiexProxy string) (map[string]perDay, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	out := map[string]perDay{}
	scanner := bufio.NewScanner(f)
	// 1 MB buffer is plenty for any single line in our JSONL.
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var row ohlcvRow
		if err := json.Unmarshal(line, &row); err != nil {
			return nil, fmt.Errorf("parse JSONL line: %w (line head: %.40q)", err, line)
		}
		if row.Date == "" || row.Symbol == "" {
			continue
		}
		day := out[row.Date]
		day.volume += row.Volume
		if row.Symbol == taiexProxy {
			day.taiex = row.Close
		}
		out[row.Date] = day
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan JSONL: %w", err)
	}
	return out, nil
}

// sumRowCount is only used to surface stats.rowsRead; it returns the
// total volume-row count by summing per-day symbols. Not exact for mixed
// symbols per day, but adequate as a sanity check.
func sumRowCount(m map[string]perDay) int {
	return len(m) // one entry per date — volume row count is in loadJSONL side
}

// ---------------------------------------------------------------------------
// Sink & upsert
// ---------------------------------------------------------------------------

// initPostgresPool 是 atlasdb.Init（ping ＋ 套用 <workdir>/sql/migrations）的 seam：
// 單元測試替換它就能在沒有 PostgreSQL 的情況下驗證「開池 → 注入 → 建 store」的順序。
var initPostgresPool = atlasdb.Init

// checkBackendFlags 拒絕互相矛盾的顯式 flag：-pg（postgres）與 -db（sqlite）
// 不能同時給。兩個都不給時後端跟隨 ATLAS_STORE_BACKEND（#2107）。
func checkBackendFlags(dbSet, usePG bool) error {
	if dbSet && usePG {
		return fmt.Errorf("-pg and -db are mutually exclusive (pick one backend explicitly, or drop both and let ATLAS_STORE_BACKEND decide)")
	}
	return nil
}

// backendFor 回傳本次執行實際使用的儲存後端（單一決策路徑，#2107）。
//
// 顯式 flag 優先（-pg／-db），否則**跟隨宣告**的 ATLAS_STORE_BACKEND。
// 刻意不再「預設 sqlite、postgres 需 -pg」—— 那個形狀讓生產（宣告 postgres）
// 靜默寫到本機 sqlite artifact（data/state/atlas.db），正是 #2107 的事故。
func backendFor(cfg runConfig, appCfg config.Config) (string, error) {
	switch {
	case cfg.usePG:
		return ledger.ResolveStoreBackend("postgres")
	case cfg.dbExplicit:
		return ledger.ResolveStoreBackend("sqlite")
	default:
		return ledger.ResolveStoreBackend(appCfg.StoreBackend)
	}
}

// openSink 依 flag／環境開啟 store；**不降級**。
//
// postgres 後端的三個必要步驟（缺一就會在生產以「requires pool」失敗）：
//  1. 取 DSN（-pg-dsn 優先，其次 $DATABASE_URL）—— 沒有就**明確**報錯，絕不退回 sqlite
//  2. initPostgresPool 建立連線池（ping ＋ 套用 migrations）
//  3. 注入 store factory（ledger.SetPostgresPool）**再**建 store
//
// 宣告的後端若在本指令沒有實作（jsonl：period_history 是關聯表）也是明確錯誤。
// 回傳的 closer 負責關閉本函式自己開的資源。
func openSink(ctx context.Context, cfg runConfig, appCfg config.Config) (ledger.HistoricalStore, func() error, error) {
	backend, err := backendFor(cfg, appCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("period history store: %w", err)
	}

	switch backend {
	case "sqlite":
		dbPath := cfg.dbPath
		if dbPath == "" {
			dbPath = filepath.Join("data", "state", "atlas.db")
		}
		if !filepath.IsAbs(dbPath) {
			dbPath = filepath.Join(cfg.workDir, dbPath)
		}
		if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
			return nil, nil, fmt.Errorf("mkdir db dir: %w", err)
		}
		sqlDB, err := ledger.OpenSQLiteDB(dbPath)
		if err != nil {
			return nil, nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
		}
		if err := ledger.InitSchema(sqlDB); err != nil {
			_ = sqlDB.Close()
			return nil, nil, fmt.Errorf("init schema: %w", err)
		}
		return ledger.NewSQLiteHistoricalStore(sqlDB), sqlDB.Close, nil

	case "postgres":
		dsn := cfg.pgDSN
		if dsn == "" {
			dsn = appCfg.DatabaseURL
		}
		if dsn == "" {
			// 明確可診斷：說清楚是哪個後端、要補哪個 flag／環境變數。
			return nil, nil, fmt.Errorf(
				"period history store: backend %q requires a PostgreSQL DSN: pass -pg-dsn or set DATABASE_URL (refusing to fall back to sqlite)",
				backend)
		}
		pool, err := initPostgresPool(ctx, dsn, filepath.Join(cfg.workDir, "sql", "migrations"))
		if err != nil {
			return nil, nil, fmt.Errorf("connect postgres: %w", err)
		}
		ledger.SetPostgresPool(pool)
		pgCfg := appCfg
		pgCfg.StoreBackend = backend
		store, err := ledger.NewHistoricalStore(pgCfg)
		if err != nil {
			pool.Close()
			return nil, nil, err
		}
		return store, func() error { pool.Close(); return nil }, nil

	default:
		return nil, nil, fmt.Errorf(
			"period history store: backend %q has no history implementation (use -db <path>, -pg, or ATLAS_STORE_BACKEND=sqlite|postgres)",
			backend)
	}
}

// upsertDay writes one period_history row for the day. A date that already
// holds a live (is_synthetic=0) row is skipped (preserve newer ingest data;
// the backfill only fills the gap). Returns (wrote, skippedLive, error).
func upsertDay(ctx context.Context, store ledger.HistoricalStore, date string, assessment portfolio.PeriodAssessment, now time.Time) (int, int, error) {
	if existing, ok, err := store.LoadPeriodByDateAll(ctx, date); err == nil && ok && existing.IsSynthetic == 0 {
		return 0, 1, nil
	}
	recordedAt := now
	row := ledger.PeriodRow{
		Date:            date,
		Period:          string(assessment.MarketPeriod),
		RecordedAt:      recordedAt,
		CapturedAt:      now,
		IsSynthetic:     1,
		Source:          sourceName,
		DetectorVersion: portfolio.PeriodDetectorVersionV3,
	}
	if err := store.UpsertPeriod(ctx, row); err != nil {
		return 0, 0, fmt.Errorf("upsert period %s: %w", date, err)
	}
	return 1, 0, nil
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func printDryRunLine(date string, taiex float64, assessment portfolio.PeriodAssessment) {
	fmt.Printf("[%s] DRY-RUN period=%-15s isFallback=%-5v confidence=%.2f conditionsHit=%d/%d taiex=%.2f\n",
		date, assessment.MarketPeriod, assessment.IsFallback,
		assessment.Confidence, assessment.ConditionsHit, assessment.ConditionsTotal,
		taiex)
}

func printSummary(cfg runConfig, stats *runStats, backend string) {
	var sb strings.Builder
	sb.WriteString("\n== backfill-period-history-range summary ==\n")
	fmt.Fprintf(&sb, "storage backend       %s\n", backend)
	fmt.Fprintf(&sb, "source                %s\n", cfg.sourcePath)
	fmt.Fprintf(&sb, "workdir               %s\n", cfg.workDir)
	fmt.Fprintf(&sb, "taiex proxy           %s\n", cfg.taiexProxy)
	fmt.Fprintf(&sb, "date range            %s .. %s\n", stats.firstDate, stats.lastDate)
	fmt.Fprintf(&sb, "dates in dataset      %d\n", stats.datesInDataset)
	fmt.Fprintf(&sb, "dates in window       %d\n", stats.datesInRange)
	fmt.Fprintf(&sb, "processed             %d\n", stats.processed)
	fmt.Fprintf(&sb, "is_fallback (true)    %d\n", stats.fallbackCount)
	periods := sortedKeys(stats.periodCount)
	fmt.Fprintf(&sb, "period:               %s\n", kvList(stats.periodCount, periods))
	if cfg.dryRun {
		sb.WriteString("dry-run: no rows written\n")
	} else {
		fmt.Fprintf(&sb, "upserted period rows  %d\n", stats.upsertPeriod)
		fmt.Fprintf(&sb, "skipped (live rows)   %d\n", stats.skippedLive)
	}
	fmt.Fprintf(&sb, "TAIEX proxy missing   %d days\n", stats.taiexMissingDays)
	fmt.Fprintf(&sb, "errors                %d\n", stats.errors)
	if len(stats.errorDates) > 0 {
		fmt.Fprintf(&sb, "error dates: %s\n", strings.Join(stats.errorDates, ","))
	}
	fmt.Print(sb.String())
	_ = io.Discard // keep io import live in case we add a -quiet flag later
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func kvList(m map[string]int, keys []string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
	}
	return strings.Join(parts, " ")
}
