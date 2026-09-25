package main

// symbol_industry_substrate.go wires the first-party per-stock industry field
// (issue #1943) into the production resolution paths.
//
// Three pieces live here:
//
//  1. storeSymbolIndustrySubstrate — the read-side adapter that turns the
//     `symbol_industry` DB rows into an industry.SymbolIndustrySubstrate.
//  2. newSymbolIndustrySubstrate — the gate-aware constructor. It returns nil
//     unless configs/parameters.json ->
//     industry.substrate_from_symbol_industry_enabled is true, so "gate off"
//     installs nothing and every resolver keeps its pre-#1943 behavior.
//  3. symbolIndustryRefreshTask — the auto_symbol_industry background task:
//     fetch the channel (writes data/state/symbol_industry.json) and mirror it
//     into the queryable per-stock field through the backend-aware store.

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/symbolindustry"
)

// symbolIndustryStateFileName is the channel's persisted snapshot, relative to
// <workDir>/data/state.
const symbolIndustryStateFileName = "symbol_industry.json"

// substrateReloadTTL bounds how stale the in-memory view of the per-stock
// industry field may get. The underlying data changes at most once a day
// (company industry codes are essentially static), so a multi-hour TTL keeps
// the hot path free of database round-trips while still picking up a fresh
// refresh without a restart.
const substrateReloadTTL = 6 * time.Hour

// storeSymbolIndustrySubstrate is the DB-backed per-stock industry substrate.
//
// It loads the full field into memory (1988 rows in the 2026-09-24 upstream
// snapshot) and answers ResolveL1 from that map: the resolvers run inside
// per-symbol loops (universe screening over ~2000 symbols, sector exposure over
// every position), so one query per symbol would be the wrong shape.
//
// Failure semantics: a load failure or an empty table leaves the cached view
// empty, which makes ResolveL1 report ok=false and every consumer fall back to
// the representative-stock tables. That fails CLOSED to the pre-#1943 behavior
// instead of guessing a sector for the whole market.
type storeSymbolIndustrySubstrate struct {
	store symbolindustry.Store
	// ttl is substrateReloadTTL in production; tests shorten it.
	ttl time.Duration
	// now is the time source (tests).
	now func() time.Time

	mu          sync.Mutex
	l1BySymbol  map[string]string
	symbols     []string
	coverage    industry.SymbolIndustryCoverage
	loadedAt    time.Time
	loadErr     error
	lookupCount int64
}

var (
	_ industry.SymbolIndustrySubstrate        = (*storeSymbolIndustrySubstrate)(nil)
	_ industry.SymbolIndustryCoverageReporter = (*storeSymbolIndustrySubstrate)(nil)
)

// ResolveL1 implements industry.SymbolIndustrySubstrate.
func (s *storeSymbolIndustrySubstrate) ResolveL1(symbol string) (industry.SectorID, bool) {
	s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookupCount++
	l1, ok := s.l1BySymbol[symbol]
	if !ok || l1 == "" {
		return "", false
	}
	return industry.SectorID(l1), true
}

// Symbols implements industry.SymbolIndustrySubstrate.
func (s *storeSymbolIndustrySubstrate) Symbols() []string {
	s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.symbols) == 0 {
		return nil
	}
	return append([]string(nil), s.symbols...)
}

// Coverage implements industry.SymbolIndustryCoverageReporter.
//
// It answers from the accounting computed during the last successful Reload,
// i.e. from the same rows that produced Symbols(): upstream population the
// channel saw, the resolved part, and the unresolved buckets with their
// reasons. No file is re-read and no upstream code is re-mapped here, so the
// audit cannot disagree with the population the pipeline actually uses.
//
// A failed or not-yet-performed load leaves Upstream at 0, which the audit
// reads as "coverage is not measurable" rather than as 100 % coverage -- an
// empty view must never look like a green one.
func (s *storeSymbolIndustrySubstrate) Coverage() industry.SymbolIndustryCoverage {
	s.ensureLoaded()
	s.mu.Lock()
	defer s.mu.Unlock()
	cov := s.coverage
	cov.Reasons = append([]string(nil), s.coverage.Reasons...)
	return cov
}

// Len returns the cached population size (no reload decision, for tests and
// diagnostics).
func (s *storeSymbolIndustrySubstrate) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.symbols)
}

// Lookups returns how many ResolveL1 calls were served. Wiring uses it as
// consumption evidence: a substrate that is installed but never asked anything
// is the #1944 "write with no reader" failure shape.
func (s *storeSymbolIndustrySubstrate) Lookups() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lookupCount
}

// Invalidate drops the cached view (called after a refresh writes new rows).
func (s *storeSymbolIndustrySubstrate) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadedAt = time.Time{}
}

// ensureLoaded refreshes the cache when it is empty or older than the TTL. A
// failed load is retried on the next call rather than cached as "permanently
// empty", so a substrate that is installed before the channel has ever run
// starts working as soon as the rows exist.
func (s *storeSymbolIndustrySubstrate) ensureLoaded() {
	if s == nil {
		return
	}
	s.mu.Lock()
	fresh := len(s.symbols) > 0 && s.now().Sub(s.loadedAt) < s.ttl
	s.mu.Unlock()
	if fresh {
		return
	}
	if err := s.Reload(context.Background()); err != nil {
		logging.Warn("symbol_industry", "substrate_load_failed",
			logging.FStr("backend", s.storeBackendName()),
			logging.Err(err))
	}
}

func (s *storeSymbolIndustrySubstrate) storeBackendName() string {
	if s.store == nil {
		return "none"
	}
	return "store"
}

// Reload reads the full per-stock industry field from the store.
func (s *storeSymbolIndustrySubstrate) Reload(ctx context.Context) error {
	if s.store == nil {
		return fmt.Errorf("symbol_industry: substrate has no store wired")
	}
	entries, err := s.store.LoadAll(ctx)
	if err != nil {
		s.mu.Lock()
		s.loadErr = err
		s.mu.Unlock()
		return err
	}
	byL1 := make(map[string]string, len(entries))
	symbols := make([]string, 0, len(entries))
	for _, e := range entries {
		// Only rows with a canonical L1 answer are loadable: unmapped /
		// unknown upstream codes must stay unresolved (reported by the channel
		// snapshot), never imputed to a sector (issue #1943 #1958 rule).
		if e.Symbol == "" || e.CanonicalL1 == "" {
			continue
		}
		byL1[e.Symbol] = e.CanonicalL1
		symbols = append(symbols, e.Symbol)
	}
	// The coverage accounting is derived here, from the same `entries` the
	// population loop above walks: this loader is the only component that sees
	// BOTH the upstream rows (1988 in production) and the resolved ones (1599),
	// so it is the only place that can produce an honest denominator without
	// re-reading a file or re-mapping a code.
	coverage := symbolIndustryCoverageFromEntries(entries)
	s.mu.Lock()
	s.l1BySymbol = byL1
	s.symbols = symbols
	s.coverage = coverage
	s.loadedAt = s.now()
	s.loadErr = nil
	s.mu.Unlock()
	return nil
}

// symbolIndustryCoverageFromEntries buckets the store's rows into the
// industry.SymbolIndustryCoverage accounting.
//
// Pure on purpose: the bucketing is the part worth unit-testing (status
// constants and the "resolved == installed" invariant), and it must be testable
// without a database or a live channel.
//
//   - Upstream  = every row the store holds = the upstream rows the first-party
//     channel saw. Rows the channel could not classify are COUNTED here, which
//     is the whole point: they are the missing population.
//   - Resolved  = the rows with a canonical L1 answer, i.e. exactly the rows
//     Reload installs into the substrate (same predicate), so the audit's
//     numerator is the pipeline's population by construction.
//   - Unmapped / Unknown = bucketed by MappingStatus with the
//     internal/symbolindustry status constants (no re-typed literals).
//   - Reasons   = the distinct non-empty MappingReason values of the rows that
//     are NOT resolved. This restriction is DELIBERATE: a resolved row's reason
//     documents why its mapping is sound ("canonical L1 cement 為唯一對應"), and
//     carrying ~20 of those strings into every audit line would drown the
//     reasons that describe a gap. The complete per-row reasons stay available
//     in the channel snapshot (data/state/symbol_industry.json: entries[].mapping_reason
//     plus the unmapped_codes / unknown_codes dispositions).
func symbolIndustryCoverageFromEntries(entries []symbolindustry.Entry) industry.SymbolIndustryCoverage {
	cov := industry.SymbolIndustryCoverage{Upstream: len(entries)}
	reasons := make(map[string]struct{})
	for _, e := range entries {
		resolved := e.Symbol != "" && e.CanonicalL1 != ""
		if resolved {
			cov.Resolved++
		}
		switch e.MappingStatus {
		case symbolindustry.StatusUnmapped:
			cov.Unmapped++
		case symbolindustry.StatusUnknown:
			cov.Unknown++
		}
		if resolved {
			continue
		}
		if reason := strings.TrimSpace(e.MappingReason); reason != "" {
			reasons[reason] = struct{}{}
		}
	}
	if len(reasons) > 0 {
		cov.Reasons = make([]string, 0, len(reasons))
		for reason := range reasons {
			cov.Reasons = append(cov.Reasons, reason)
		}
		slices.Sort(cov.Reasons)
	}
	return cov
}

// newSymbolIndustrySubstrate returns the per-stock industry substrate when the
// config gate is on and the store can be opened, and nil otherwise.
//
// nil is the pre-#1943 wiring, so a caller never needs a second gate check: the
// decision is made here, once, from the declared parameter
// industry.substrate_from_symbol_industry_enabled (default false).
func newSymbolIndustrySubstrate(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) industry.SymbolIndustrySubstrate {
	if !config.GetIndustrySubstrateFromSymbolIndustryEnabled() {
		logging.Info("symbol_industry", "substrate_disabled",
			"gate", "industry.substrate_from_symbol_industry_enabled")
		return nil
	}
	store, err := symbolindustry.NewStore(ctx, cfg.StoreBackend, pool, cfg.WorkDir)
	if err != nil {
		logging.Warn("symbol_industry", "substrate_store_unavailable",
			logging.FStr("backend", cfg.StoreBackend),
			logging.Err(err))
		return nil
	}
	sub := &storeSymbolIndustrySubstrate{
		store: store,
		ttl:   substrateReloadTTL,
		now:   time.Now,
	}
	if err := sub.Reload(ctx); err != nil {
		// Fail closed: keep the substrate installed (it retries on the next
		// lookup) but log loudly — an empty population means the gate has no
		// effect yet.
		logging.Warn("symbol_industry", "substrate_initial_load_failed", logging.Err(err))
	}
	logging.Info("symbol_industry", "substrate_installed",
		"population", len(sub.Symbols()),
		"gate", "industry.substrate_from_symbol_industry_enabled")
	return sub
}

// symbolIndustryStatePath returns the channel snapshot path for a work dir.
func symbolIndustryStatePath(workDir string) string {
	return filepath.Join(workDir, "data", "state", symbolIndustryStateFileName)
}

// symbolIndustrySnapshotIsFresh reports whether the snapshot was written today
// (Taipei). The upstream company-industry table changes at most daily, so one
// fetch per day is enough and the 5-minute task tick stays cheap.
func symbolIndustrySnapshotIsFresh(snap symbolindustry.Snapshot, now time.Time) bool {
	if snap.UpdatedAt == "" {
		return false
	}
	updated, err := time.Parse(time.RFC3339, snap.UpdatedAt)
	if err != nil {
		return false
	}
	tpe := now.In(substrateTaipeiLoc)
	return updated.In(substrateTaipeiLoc).Format("2006-01-02") == tpe.Format("2006-01-02")
}

var substrateTaipeiLoc = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return loc
}()

// symbolIndustryRefreshTask builds the auto_symbol_industry task closure.
//
// Steps:
//  1. read the state file; fetch the channel through the gateway only when the
//     snapshot is missing, unreadable or older than today;
//  2. mirror the snapshot into the queryable per-stock industry field through
//     symbolindustry.NewStore (backend-aware: Postgres in production, the
//     job-local SQLite artifact for dev/CLI — never a hard-coded SQLite path);
//  3. invalidate the substrate cache so the new rows are visible immediately.
//
// The DB mirror is skipped when the row count already matches the snapshot, so
// a healthy steady state costs one COUNT per day. If the fetch fails the ingest
// is skipped: the state file is the source of the mirror, and mirroring a stale
// file silently would hide the failure that matters.
func symbolIndustryRefreshTask(
	gateway *apigateway.Gateway,
	cfg config.Config,
	pool *pgxpool.Pool,
	substrate industry.SymbolIndustrySubstrate,
) func(ctx context.Context) error {
	return func(ctx context.Context) error {
		statePath := symbolIndustryStatePath(cfg.WorkDir)

		snap, snapErr := symbolindustry.ReadSnapshot(statePath)
		stale := snapErr != nil || !symbolIndustrySnapshotIsFresh(snap, time.Now())
		if stale && gateway != nil {
			if _, err := gateway.Fetch(ctx, "symbol_industry"); err != nil {
				return fmt.Errorf("symbol_industry: fetch: %w", err)
			}
			snap, snapErr = symbolindustry.ReadSnapshot(statePath)
		}
		if snapErr != nil {
			if gateway == nil {
				// No gateway and nothing readable: nothing to mirror. Not an
				// error (the -simulate / CLI paths run without a gateway), but
				// the operator must be able to see why the field is empty.
				logging.Warn("symbol_industry", "refresh_skipped_no_gateway",
					logging.FStr("state_file", statePath),
					logging.FStr("error", snapErr.Error()))
				return nil
			}
			return fmt.Errorf("symbol_industry: read snapshot: %w", snapErr)
		}
		if len(snap.Entries) == 0 {
			return fmt.Errorf("symbol_industry: snapshot %s carries no entries", statePath)
		}

		store, err := symbolindustry.NewStore(ctx, cfg.StoreBackend, pool, cfg.WorkDir)
		if err != nil {
			return fmt.Errorf("symbol_industry: store: %w", err)
		}
		existing, err := store.Count(ctx)
		if err != nil {
			return fmt.Errorf("symbol_industry: count: %w", err)
		}
		if existing != len(snap.Entries) {
			written, err := store.UpsertAll(ctx, snap.Entries)
			if err != nil {
				return fmt.Errorf("symbol_industry: ingest: %w", err)
			}
			logging.Info("symbol_industry", "ingested",
				"rows", written,
				"previous_rows", existing,
				"mapped", snap.Counts.Mapped,
				"canonical_l1", snap.Counts.CanonicalL1)
		}
		// Invalidate the resolver cache so the freshly mirrored rows are
		// visible without a restart. The assertion is explicit (and nil-safe):
		// the task owns the lifecycle of the cached view, the interface port
		// deliberately does not expose it.
		if inv, ok := substrate.(interface{ Invalidate() }); ok {
			inv.Invalidate()
		}
		return nil
	}
}
