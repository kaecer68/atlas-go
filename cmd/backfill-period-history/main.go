// Command backfill-period-history backfills period_history and regime_history
// from historical macro snapshots (data/state/macro/YYYY-MM-DD.json).
//
// Background: persistPeriodHistory / persistRegimeHistory
// (internal/monitoring/dashboard_api.go) only accumulate forward from first
// run, so the pre-live window (macro snapshots since 2026-04-25, ~100 days)
// never reached the ledger tables. This command replays the same pure-function
// pipeline day by day and upserts one period_history + one regime_history row
// per snapshot date.
//
// Per-date pipeline (mirrors persistPeriodHistory, but uses the snapshot's own
// date instead of time.Now()):
//
//	SnapshotToPeriodIndicators → EnrichFromDir(macro dir) → EnrichMargin(margin)
//	→ EnrichBatch3(sector_index, government_flow) → DetectPeriod
//	stress := StressIndex(VIX/DXY/US10Y).ComputeStressLevel(snap)
//	regime := NormalizeRegime(stressScoreToRegime(stress))
//
// Usage:
//
//	backfill-period-history -workdir . -dry-run
//	backfill-period-history -workdir . -db data/state/atlas.db
//	backfill-period-history -workdir . -pg -pg-dsn postgres://...
//	backfill-period-history -workdir . -start 2026-05-01 -end 2026-07-31
//
// Semantics: upsert by date (ON CONFLICT(date) DO UPDATE), idempotent — safe
// to re-run. Dates that already hold a live (is_synthetic=0) row are skipped
// so a backfill run can never clobber newer ingest data.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	atlasdb "github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/calibration"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

const (
	sourceName = "period_history_backfill"
	// maxMarginWindow caps the margin-history window passed to EnrichMargin.
	// EnrichMargin only needs the most recent MinDaysMarginBalancePeak (30)
	// entries ending at the target date; the live pipeline passes the whole
	// file (which ends today), which would leak future margin data into a
	// historical date. We pass at most 60 entries ≤ target date instead.
	maxMarginWindow = 60
)

func main() {
	if err := runFromOSArgs(); err != nil {
		log.Fatalf("backfill-period-history: %v", err)
	}
}

type runConfig struct {
	workDir string
	start   string // "YYYY-MM-DD"; empty = earliest snapshot in dir
	end     string // "YYYY-MM-DD"; empty = latest snapshot in dir
	dryRun  bool
	dbPath  string
	usePG   bool
	pgDSN   string
	now     func() time.Time       // injectable for deterministic tests
	store   ledger.HistoricalStore // injectable store for tests (nil = open DB)
}

func runFromOSArgs() error {
	var (
		workDir = flag.String("workdir", ".", "atlas repo root (must contain data/state/macro)")
		start   = flag.String("start", "", "backfill start date YYYY-MM-DD (inclusive; default: earliest snapshot)")
		end     = flag.String("end", "", "backfill end date YYYY-MM-DD (inclusive; default: latest snapshot)")
		dryRun  = flag.Bool("dry-run", false, "print what would be written without touching the DB")
		dbPath  = flag.String("db", "data/state/atlas.db", "SQLite DB path (default relative to -workdir)")
		usePG   = flag.Bool("pg", false, "write to PostgreSQL instead of SQLite")
		pgDSN   = flag.String("pg-dsn", "", "PostgreSQL DSN (default: $DATABASE_URL); requires -pg")
	)
	flag.Parse()

	for _, s := range []string{*start, *end} {
		if s != "" {
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return fmt.Errorf("invalid date %q (want YYYY-MM-DD): %w", s, err)
			}
		}
	}
	if *usePG && *pgDSN == "" && os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("-pg requires -pg-dsn or DATABASE_URL")
	}

	_, err := run(context.Background(), runConfig{
		workDir: *workDir,
		start:   *start,
		end:     *end,
		dryRun:  *dryRun,
		dbPath:  *dbPath,
		usePG:   *usePG,
		pgDSN:   *pgDSN,
	})
	return err
}

type runStats struct {
	totalInDir   int
	inRange      int
	processed    int
	periodCount  map[string]int
	regimeCount  map[string]int
	fallback     int
	upsertPeriod int
	upsertRegime int
	skippedLive  int
	errors       int
	errorDates   []string
}

// dayResult is the per-date classification outcome (dry-run + write path).
type dayResult struct {
	date     string
	period   string
	regime   string
	stress   float64
	fallback bool
	ind      portfolio.PeriodIndicators
}

// run executes the backfill. Exposed for tests; cfg.now defaults to time.Now.
func run(ctx context.Context, cfg runConfig) (*runStats, error) {
	if cfg.now == nil {
		cfg.now = time.Now
	}

	macroDir := filepath.Join(cfg.workDir, "data", "state", "macro")
	if _, err := os.Stat(macroDir); err != nil {
		return nil, fmt.Errorf("macro dir %s: %w", macroDir, err)
	}
	marginDir := filepath.Join(cfg.workDir, "data", "state", "margin")
	sectorDir := filepath.Join(cfg.workDir, "data", "state", "sector_index")
	govDir := filepath.Join(cfg.workDir, "data", "state", "government_flow")

	dates, err := collectSnapshotDates(macroDir, cfg.start, cfg.end)
	if err != nil {
		return nil, err
	}
	if len(dates) == 0 {
		return nil, fmt.Errorf("no dated macro snapshots in %s (start=%q end=%q)", macroDir, cfg.start, cfg.end)
	}

	stats := &runStats{
		totalInDir:  countDatedSnapshots(macroDir),
		inRange:     len(dates),
		periodCount: map[string]int{},
		regimeCount: map[string]int{},
	}

	var store ledger.HistoricalStore
	var closeStore func() error
	if cfg.store != nil {
		store = cfg.store
	} else if !cfg.dryRun {
		store, closeStore, err = openSink(ctx, cfg)
		if err != nil {
			return nil, err
		}
		defer func() { _ = closeStore() }()
	}

	stressIndex := portfolio.NewStressIndexFromConfig(portfolio.DefaultStressIndexConfig())
	detector := portfolio.NewPeriodDetectorWithDefaults()
	calc := portfolio.NewCalculator()

	for _, date := range dates {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		snapPath := filepath.Join(macroDir, date+".json")
		data, err := os.ReadFile(snapPath)
		if err != nil {
			stats.errors++
			stats.errorDates = append(stats.errorDates, date)
			fmt.Fprintf(os.Stderr, "[%s] read snapshot: %v\n", date, err)
			continue
		}
		var snap marketdata.MacroDataSnapshot
		if err := json.Unmarshal(data, &snap); err != nil {
			stats.errors++
			stats.errorDates = append(stats.errorDates, date)
			fmt.Fprintf(os.Stderr, "[%s] unmarshal snapshot: %v\n", date, err)
			continue
		}
		if recDate := recordedDate(snap); recDate != "" && recDate != date {
			log.Printf("warn: snapshot %s has recorded_at date %s; using filename date %s", date, recDate, date)
		}

		res := computeDay(date, snap, macroDir, marginDir, sectorDir, govDir, calc, detector, stressIndex)
		stats.processed++
		stats.periodCount[res.period]++
		stats.regimeCount[res.regime]++
		if res.fallback {
			stats.fallback++
		}

		if cfg.dryRun {
			printDryRunLine(res)
			continue
		}

		writtenP, writtenR, skipped, err := upsertDay(ctx, store, res, snap, cfg.now())
		if err != nil {
			stats.errors++
			stats.errorDates = append(stats.errorDates, date)
			fmt.Fprintf(os.Stderr, "[%s] upsert failed: %v\n", date, err)
			continue
		}
		stats.upsertPeriod += writtenP
		stats.upsertRegime += writtenR
		stats.skippedLive += skipped
		fmt.Printf("[%s] period=%-14s regime=%-10s upserted(p=%d r=%d)\n", date, res.period, res.regime, writtenP, writtenR)
	}

	printSummary(cfg, stats)
	return stats, nil
}

// computeDay runs the full enrich → detect pipeline for one snapshot date.
// All enrich steps are best-effort: missing dirs / insufficient history leave
// the affected indicator at zero (honest degradation, detector guards handle it).
func computeDay(date string, snap marketdata.MacroDataSnapshot, macroDir, marginDir, sectorDir, govDir string, calc *portfolio.PeriodIndicatorsCalculator, detector *portfolio.PeriodDetector, stressIndex *portfolio.StressIndex) dayResult {
	ind := monitoring.SnapshotToPeriodIndicators(snap)

	if dirExists(macroDir) {
		_ = calc.EnrichFromDir(&ind, date, macroDir)
	}
	if dirExists(marginDir) {
		if entries, err := narrative.LoadMarginHistory(marginDir); err == nil {
			// Filter to entries ≤ target date so a historical day never sees
			// future margin data (the live pipeline passes the full file).
			var window []narrative.MarginHistoryEntry
			for _, e := range entries {
				if e.Date > date {
					continue
				}
				window = append(window, e)
			}
			if len(window) > maxMarginWindow {
				window = window[len(window)-maxMarginWindow:]
			}
			if len(window) > 0 {
				pm := make([]portfolio.MarginEntry, len(window))
				for i, me := range window {
					pm[i] = portfolio.MarginEntry{Date: me.Date, MarginBalance: me.MarginBalance}
				}
				calc.EnrichMargin(&ind, pm)
			}
		}
	}
	if dirExists(sectorDir) || dirExists(govDir) {
		calc.EnrichBatch3(&ind, date, sectorDir, govDir)
	}

	assessment, _ := detector.DetectAssessment(ind)
	stress := stressIndex.ComputeStressLevel(snap)
	regime := narrative.NormalizeRegime(stressScoreToRegime(stress))
	return dayResult{
		date:     date,
		period:   string(assessment.MarketPeriod),
		regime:   regime,
		stress:   stress,
		fallback: assessment.IsFallback,
		ind:      ind,
	}
}

// stressScoreToRegime buckets a 0-100 stress score into the stress vocabulary
// (low / alert / high / crisis) using the same thresholds as the live
// TaiwanStressCalculator (calibration.StressThresholdAlert/High/Crisis).
func stressScoreToRegime(score float64) string {
	switch {
	case score >= calibration.StressThresholdCrisis:
		return "crisis"
	case score >= calibration.StressThresholdHigh:
		return "high"
	case score >= calibration.StressThresholdAlert:
		return "alert"
	default:
		return "low"
	}
}

// recordedDate returns the snapshot RecordedAt as YYYY-MM-DD ("" if unset).
func recordedDate(snap marketdata.MacroDataSnapshot) string {
	if snap.RecordedAt <= 0 {
		return ""
	}
	return time.Unix(snap.RecordedAt, 0).UTC().Format("2006-01-02")
}

// upsertDay writes one period + one regime row for the day. A date that
// already holds a live (is_synthetic=0) row is skipped (keep newer ingest
// data; the backfill only fills the gap). Returns (periodWrites, regimeWrites,
// skippedLive, error).
func upsertDay(ctx context.Context, store ledger.HistoricalStore, res dayResult, snap marketdata.MacroDataSnapshot, now time.Time) (int, int, int, error) {
	if existing, ok, err := store.LoadPeriodByDateAll(ctx, res.date); err == nil && ok && existing.IsSynthetic == 0 {
		return 0, 0, 1, nil
	}
	recordedAt := now
	if snap.RecordedAt > 0 {
		recordedAt = time.Unix(snap.RecordedAt, 0).UTC()
	}
	pRow := ledger.PeriodRow{
		Date:            res.date,
		Period:          res.period,
		RecordedAt:      recordedAt,
		CapturedAt:      now,
		IsSynthetic:     1,
		Source:          sourceName,
		DetectorVersion: portfolio.PeriodDetectorVersionV3,
	}
	if err := store.UpsertPeriod(ctx, pRow); err != nil {
		return 0, 0, 0, fmt.Errorf("upsert period %s: %w", res.date, err)
	}
	rRow := ledger.RegimeRow{
		Date:            res.date,
		Regime:          res.regime,
		SourceSessionID: sourceName + ":" + res.date,
		RecordedAt:      recordedAt,
		CapturedAt:      now,
		IsSynthetic:     1,
		Source:          sourceName,
	}
	if err := store.UpsertRegime(ctx, rRow); err != nil {
		return 1, 0, 0, fmt.Errorf("upsert regime %s: %w", res.date, err)
	}
	return 1, 1, 0, nil
}

// openSink opens the target store: SQLite (-db, default) or PostgreSQL (-pg).
func openSink(ctx context.Context, cfg runConfig) (ledger.HistoricalStore, func() error, error) {
	if cfg.usePG {
		dsn := cfg.pgDSN
		if dsn == "" {
			dsn = os.Getenv("DATABASE_URL")
		}
		migrationsPath := filepath.Join(cfg.workDir, "sql", "migrations")
		pool, err := atlasdb.Init(ctx, dsn, migrationsPath)
		if err != nil {
			return nil, nil, fmt.Errorf("connect postgres: %w", err)
		}
		return ledger.NewPostgresHistoricalStore(pool), func() error { pool.Close(); return nil }, nil
	}

	dbPath := cfg.dbPath
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
}

// collectSnapshotDates returns dated snapshot names (YYYY-MM-DD) in the macro
// dir, sorted ascending, filtered to [start, end]. latest/previous/_metadata
// and any non-dated files are skipped.
func collectSnapshotDates(macroDir, start, end string) ([]string, error) {
	entries, err := os.ReadDir(macroDir)
	if err != nil {
		return nil, fmt.Errorf("read macro dir: %w", err)
	}
	var dates []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if name == "latest.json" || name == "previous.json" || name == "_metadata.json" {
			continue
		}
		date := strings.TrimSuffix(name, ".json")
		if !isDatedName(date) {
			continue
		}
		if start != "" && date < start {
			continue
		}
		if end != "" && date > end {
			continue
		}
		dates = append(dates, date)
	}
	sort.Strings(dates)
	return dates, nil
}

func countDatedSnapshots(macroDir string) int {
	entries, err := os.ReadDir(macroDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		if name == "latest.json" || name == "previous.json" || name == "_metadata.json" {
			continue
		}
		if isDatedName(strings.TrimSuffix(name, ".json")) {
			n++
		}
	}
	return n
}

func isDatedName(s string) bool {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func printDryRunLine(res dayResult) {
	ind := res.ind
	fmt.Printf("[%s] DRY-RUN period=%-14s regime=%-10s stress=%6.2f fallback=%-5v vix=%7.2f taiex=%9.2f ma20=%9.2f fnet5d=%7.2f volma20=%9.2f margin_peak=%9.2f\n",
		res.date, res.period, res.regime, res.stress, res.fallback,
		ind.VIX, ind.TAIEXPrice, ind.TAIEXMA20, ind.ForeignNet5DayAvg, ind.MarketVolumeMA20, ind.MarginBalancePeak)
}

func printSummary(cfg runConfig, stats *runStats) {
	var sb strings.Builder
	sb.WriteString("\n== backfill-period-history summary ==\n")
	fmt.Fprintf(&sb, "snapshots in dir      %d\n", stats.totalInDir)
	fmt.Fprintf(&sb, "in range              %d\n", stats.inRange)
	fmt.Fprintf(&sb, "processed             %d\n", stats.processed)
	fmt.Fprintf(&sb, "fallback (no data)    %d\n", stats.fallback)
	periods := sortedKeys(stats.periodCount)
	fmt.Fprintf(&sb, "period: %s\n", kvList(stats.periodCount, periods))
	regimes := sortedKeys(stats.regimeCount)
	fmt.Fprintf(&sb, "regime: %s\n", kvList(stats.regimeCount, regimes))
	if cfg.dryRun {
		sb.WriteString("dry-run: no rows written\n")
	} else {
		fmt.Fprintf(&sb, "upserted period rows  %d\n", stats.upsertPeriod)
		fmt.Fprintf(&sb, "upserted regime rows  %d\n", stats.upsertRegime)
		fmt.Fprintf(&sb, "skipped (live rows)   %d\n", stats.skippedLive)
	}
	fmt.Fprintf(&sb, "errors                %d\n", stats.errors)
	if len(stats.errorDates) > 0 {
		fmt.Fprintf(&sb, "error dates: %s\n", strings.Join(stats.errorDates, ","))
	}
	fmt.Print(sb.String())
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
