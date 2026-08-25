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
//	backfill-event-calendar -workdir . -db data/state/atlas.db
//	backfill-event-calendar -workdir . -start-year 2023 -end-year 2026 -db data/state/atlas.db
//	backfill-event-calendar -workdir . -pg -pg-dsn postgres://...
//	backfill-event-calendar -workdir . -provider msci
//	backfill-event-calendar -workdir . -provider nsf
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
	usePG     bool
	pgDSN     string
	provider  string // twse | twse-openapi | msci | auto
	now       func() time.Time
	store     ledger.HistoricalStore // injectable store for tests (nil = open DB)
	factory   providerFactory        // injectable provider builder for tests
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
		dbPath    = flag.String("db", "data/state/atlas.db", "SQLite DB path (default relative to -workdir)")
		usePG     = flag.Bool("pg", false, "write to PostgreSQL instead of SQLite")
		pgDSN     = flag.String("pg-dsn", "", "PostgreSQL DSN (default: $DATABASE_URL); requires -pg")
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
	if *usePG && *pgDSN == "" && os.Getenv("DATABASE_URL") == "" {
		return fmt.Errorf("-pg requires -pg-dsn or DATABASE_URL")
	}

	_, err := run(context.Background(), runConfig{
		workDir:   *workDir,
		startYear: *startYear,
		endYear:   *endYear,
		dryRun:    *dryRun,
		dbPath:    *dbPath,
		usePG:     *usePG,
		pgDSN:     *pgDSN,
		provider:  *provider,
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
		s, closeFn, err := openSink(ctx, cfg)
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
	printSummary(cfg, stats)
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

func printSummary(cfg runConfig, stats *runStats) {
	var sb strings.Builder
	sb.WriteString("\n== backfill-event-calendar summary ==\n")
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
