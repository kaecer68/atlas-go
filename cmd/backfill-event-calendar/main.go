// Command backfill-event-calendar backfills event_calendar_history with
// historical Taiwan market calendar events (ex-dividend dates, shareholder
// meetings, MSCI quarterly rebalance dates) for the event-arbitrage arm
// (charter C4/C16).
//
// Background (R7 remediation, performance-root-cause-audit 2026-08-21):
//   - event_calendar_history (internal/ledger/sqlite_core.go) has write/read
//     plumbing (historical_store.go UpsertEventCalendar / LoadEventCalendar*)
//     but no live writer and no history — event-arbitrage backtests had no
//     historical events to consume.
//   - The original TWSE calendar endpoints (rwd/zh/exRight, rwd/zh/meeting)
//     were deprecated wholesale in 2026-06 (302 → /page-not-found.html for
//     every year incl. the current one) — verified live 2026-08-24.
//   - Working replacements: TWSE OpenAPI v1 (current-snapshot only) and the
//     static MSCI quarterly rebalance table (2023-2026, last business day of
//     Feb/May/Aug/Nov).
//
// Providers (flag -provider, default "auto"):
//   - twse          existing TWSECalendarProvider (12-month × 2 fetches per
//     year); endpoint deprecated → yields 0 events with a warn.
//   - twse-openapi  TWSE OpenAPI v1 (TWT48U_ALL + t187ap41_L); current year
//     only (OpenAPI v1 serves current snapshots).
//   - msci          static MSCI quarterly rebalance table (2023-2026).
//   - nsf           static 國安基金護盤期間表（2000-2026，人工維護）。
//   - auto          twse-openapi + msci.
//
// Writes one event_calendar_history row per event via
// ledger.HistoricalStore.UpsertEventCalendar (SQLite or PostgreSQL), with
// is_synthetic=1 (backfill marker) and source = provider name. Upserts are
// idempotent (ON CONFLICT(date, event_id) DO UPDATE) — safe to re-run.
//
// Usage:
//
//	backfill-event-calendar -workdir . -dry-run
//	backfill-event-calendar -workdir .                     # 後端跟隨 ATLAS_STORE_BACKEND
//	backfill-event-calendar -workdir . -db data/state/atlas.db
//	backfill-event-calendar -workdir . -start-year 2023 -end-year 2026 -db data/state/atlas.db
//	backfill-event-calendar -workdir . -pg -pg-dsn postgres://...
//	backfill-event-calendar -workdir . -provider msci
//	backfill-event-calendar -workdir . -provider nsf
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
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	atlasdb "github.com/kaecer68/atlas-go/internal/db"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

const (
	defaultStartYear = 2023
	// sourceSuffix is appended to event IDs so backfill rows can never
	// collide with live-pipeline rows (which use the bare data source name).
	sourceSuffix = "_backfill"
)

func main() {
	if err := runFromOSArgs(); err != nil {
		log.Fatalf("backfill-event-calendar: %v", err)
	}
}

type runConfig struct {
	workDir   string
	startYear int
	endYear   int
	dryRun    bool
	dbPath    string
	// dbExplicit 表示 -db 由使用者顯式給定（不是預設值）；顯式 flag 覆寫
	// ATLAS_STORE_BACKEND 的宣告（#2107）。
	dbExplicit bool
	usePG      bool
	pgDSN      string
	provider   string // twse | twse-openapi | msci | auto
	now        func() time.Time
	store      ledger.HistoricalStore // injectable store for tests (nil = open DB)
	factory    providerFactory        // injectable provider builder for tests
}

// namedProvider couples a provider with its display name for progress output.
type namedProvider struct {
	name     string
	provider marketdata.CalendarEventProvider
}

// providerFactory builds the providers for a given year. Kept as a field so
// tests can inject stub providers without touching the network.
type providerFactory func(cfg runConfig, year int) []namedProvider

func defaultFactory(cfg runConfig, _ int) []namedProvider {
	switch cfg.provider {
	case "twse":
		return []namedProvider{{name: "twse", provider: marketdata.NewTWSECalendarProvider()}}
	case "twse-openapi":
		return []namedProvider{{name: "twse_openapi", provider: marketdata.NewTWSEOpenAPICalendarProvider()}}
	case "msci":
		return []namedProvider{{name: "msci_static", provider: marketdata.NewMSCIRebalanceCalendarProvider()}}
	case "nsf":
		return []namedProvider{{name: "nsf_static", provider: marketdata.NewNationalStabilizationProvider()}}
	default: // auto
		return []namedProvider{
			{name: "twse_openapi", provider: marketdata.NewTWSEOpenAPICalendarProvider()},
			{name: "msci_static", provider: marketdata.NewMSCIRebalanceCalendarProvider()},
			{name: "nsf_static", provider: marketdata.NewNationalStabilizationProvider()},
		}
	}
}

func runFromOSArgs() error {
	var (
		workDir   = flag.String("workdir", ".", "atlas repo root (default: current dir)")
		startYear = flag.Int("start-year", defaultStartYear, "first backfill year (inclusive)")
		endYear   = flag.Int("end-year", 0, "last backfill year (inclusive; default: current year)")
		dryRun    = flag.Bool("dry-run", false, "print what would be written without touching the DB")
		dbPath    = flag.String("db", "data/state/atlas.db", "explicit SQLite override (relative to -workdir); without it the backend follows ATLAS_STORE_BACKEND")
		usePG     = flag.Bool("pg", false, "explicit PostgreSQL override (DSN: -pg-dsn or $DATABASE_URL)")
		pgDSN     = flag.String("pg-dsn", "", "PostgreSQL DSN (default: $DATABASE_URL); used by the postgres backend")
		provider  = flag.String("provider", "auto", "provider: twse | twse-openapi | msci | auto")
	)
	flag.Parse()

	curYear := time.Now().Year()
	if *endYear == 0 {
		*endYear = curYear
	}
	switch *provider {
	case "twse", "twse-openapi", "msci", "nsf", "auto":
	default:
		return fmt.Errorf("invalid -provider %q (want twse|twse-openapi|msci|auto)", *provider)
	}
	if *startYear < 2000 || *endYear > curYear+1 {
		return fmt.Errorf("year range out of bounds: start=%d end=%d (2000..%d)", *startYear, *endYear, curYear+1)
	}
	if *startYear > *endYear {
		return fmt.Errorf("-start-year %d > -end-year %d", *startYear, *endYear)
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
		startYear:  *startYear,
		endYear:    *endYear,
		dryRun:     *dryRun,
		dbPath:     *dbPath,
		dbExplicit: dbSet,
		usePG:      *usePG,
		pgDSN:      *pgDSN,
		provider:   *provider,
	})
	return err
}

type runStats struct {
	years          []int
	eventsFetched  map[string]int // provider → fetched
	eventsWritten  map[string]int // provider → written
	eventsSkipped  map[string]int // provider → skipped (dup / invalid date)
	errors         int
	errorProviders []string
}

func run(ctx context.Context, cfg runConfig) (*runStats, error) {
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.factory == nil {
		cfg.factory = defaultFactory
	}

	// 後端決策跟隨宣告（ATLAS_STORE_BACKEND）；顯式 flag 已在 backendFor 內覆寫。
	appCfg := config.Load()
	backendLabel := "(injected store)"
	if cfg.store == nil {
		if b, bErr := backendFor(cfg, appCfg); bErr == nil {
			backendLabel = b
		}
	}

	years := collectYears(cfg.startYear, cfg.endYear)
	stats := &runStats{
		years:         years,
		eventsFetched: map[string]int{},
		eventsWritten: map[string]int{},
		eventsSkipped: map[string]int{},
	}

	var store ledger.HistoricalStore
	var closeStore func() error
	if cfg.store != nil {
		store = cfg.store
	} else if !cfg.dryRun {
		s, closeFn, err := openSink(ctx, cfg, appCfg)
		if err != nil {
			return nil, err
		}
		store = s
		closeStore = closeFn
		defer func() { _ = closeStore() }()
	}

	// seen dedups within a single run by (date, event_id). The event_id embeds
	// the provider name (<provider>_backfill_<type>_<date>), so the same event
	// type from different providers intentionally does NOT collide — msci
	// rebalance vs ex_dividend on the same date are distinct events. (k3 audit:
	// the earlier "across providers" comment was misleading.)
	seen := make(map[string]bool) // (date|event_id) within-run dedup
	for _, year := range years {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		providers := cfg.factory(cfg, year)
		for _, np := range providers {
			events, err := np.provider.FetchEvents(ctx, year)
			if err != nil {
				stats.errors++
				stats.errorProviders = append(stats.errorProviders, fmt.Sprintf("%s/%d", np.name, year))
				fmt.Fprintf(os.Stderr, "[%s %d] fetch failed: %v\n", np.name, year, err)
				continue
			}
			stats.eventsFetched[np.name] += len(events)
			for _, ev := range events {
				row, ok := toEventRow(np.name, ev, cfg.now())
				if !ok {
					stats.eventsSkipped[np.name]++
					continue
				}
				key := row.Date + "|" + row.EventID
				if seen[key] {
					stats.eventsSkipped[np.name]++
					continue
				}
				seen[key] = true
				if cfg.dryRun {
					fmt.Printf("[dry-run %s %d] %s %s %s\n", np.name, year, row.Date, row.EventID, firstNonEmpty(ev.Name, ev.Description))
					continue
				}
				if err := store.UpsertEventCalendar(ctx, row); err != nil {
					stats.errors++
					stats.errorProviders = append(stats.errorProviders, fmt.Sprintf("%s/%d:%s", np.name, year, row.EventID))
					fmt.Fprintf(os.Stderr, "[%s %d] upsert %s: %v\n", np.name, year, row.EventID, err)
					continue
				}
				stats.eventsWritten[np.name]++
			}
		}
	}
	printSummary(cfg, stats, backendLabel)
	return stats, nil
}

// toEventRow converts provider data into a ledger row. event_id is
// "<source>_<event_type>_<date>[_<symbol>]" so every (date, event_id) is
// unique even when many symbols share an ex-date (the table PK is
// (date, event_id)). ok=false when the date is invalid.
func toEventRow(providerName string, ev marketdata.CalendarProviderData, capturedAt time.Time) (ledger.EventCalendarRow, bool) {
	if _, err := time.Parse("2006-01-02", ev.Date); err != nil {
		return ledger.EventCalendarRow{}, false
	}
	id := providerName + sourceSuffix + "_" + ev.EventType + "_" + ev.Date
	if ev.Symbol != "" {
		id += "_" + ev.Symbol
	}
	return ledger.EventCalendarRow{
		Date:        ev.Date,
		EventID:     id,
		ActiveTheme: ev.EventType,
		Source:      providerName,
		CapturedAt:  capturedAt,
		IsSynthetic: 1,
	}, true
}

// collectYears returns [start..end] inclusive, ascending.
func collectYears(start, end int) []int {
	var years []int
	for y := start; y <= end; y++ {
		years = append(years, y)
	}
	return years
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

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
// 宣告的後端若在本指令沒有實作（jsonl：event_calendar_history 是關聯表）也是明確錯誤。
// 回傳的 closer 負責關閉本函式自己開的資源。
func openSink(ctx context.Context, cfg runConfig, appCfg config.Config) (ledger.HistoricalStore, func() error, error) {
	backend, err := backendFor(cfg, appCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("event calendar store: %w", err)
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
				"event calendar store: backend %q requires a PostgreSQL DSN: pass -pg-dsn or set DATABASE_URL (refusing to fall back to sqlite)",
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
			"event calendar store: backend %q has no history implementation (use -db <path>, -pg, or ATLAS_STORE_BACKEND=sqlite|postgres)",
			backend)
	}
}

func printSummary(cfg runConfig, stats *runStats, backend string) {
	var sb strings.Builder
	sb.WriteString("\n== backfill-event-calendar summary ==\n")
	fmt.Fprintf(&sb, "storage backend       %s\n", backend)
	fmt.Fprintf(&sb, "years                 %s\n", joinYears(stats.years))
	fmt.Fprintf(&sb, "provider mode         %s\n", cfg.provider)
	providers := sortedKeys(stats.eventsFetched)
	if len(providers) == 0 {
		providers = sortedKeys(stats.eventsWritten)
	}
	for _, p := range providers {
		fmt.Fprintf(&sb, "  %-14s fetched=%-6d written=%-6d skipped=%-6d\n",
			p, stats.eventsFetched[p], stats.eventsWritten[p], stats.eventsSkipped[p])
	}
	if cfg.dryRun {
		sb.WriteString("dry-run: no rows written\n")
	}
	fmt.Fprintf(&sb, "errors                %d\n", stats.errors)
	if len(stats.errorProviders) > 0 {
		fmt.Fprintf(&sb, "error detail: %s\n", strings.Join(stats.errorProviders, ", "))
	}
	fmt.Print(sb.String())
}

func joinYears(years []int) string {
	parts := make([]string, len(years))
	for i, y := range years {
		parts[i] = fmt.Sprintf("%d", y)
	}
	return strings.Join(parts, ",")
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
