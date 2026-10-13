// Package stocktools — win-rate REST endpoint (PR 3c, Part 1).
//
// GET /api/stock/win_rate exposes the Phase-4 stockpicker win-rate
// aggregates (stock_win_rate + stock_signal_outcomes) as a read-only HTTP
// endpoint, mirroring the MCP tool stock_get_win_rate
// (cmd/atlas-mcp/server/tools_stock_winrate.go) so the client_web stock-quote
// page and the MCP channel answer the same question with the same semantics
// (200 + found=false when no data exists, never a recomputation).
//
// C6b (2026-10-13): the four win-rate endpoints read PostgreSQL when the
// deployment declares the postgres store backend (same rule as the MCP tools
// after C6), and fall back to the read-only SQLite ledger otherwise; see
// win_rate_backend.go. Every query below is written with $N placeholders so one
// SQL text runs on both backends.
package stocktools

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // driver for the read-only stockpicker SQLite handle

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// defaultWinRateWindow is the rolling-window label used by the stockpicker
// backfill job (cmd/run-stockpicker-backtest) when it aggregates raw
// outcomes into stock_win_rate rows. Kept in lockstep with the MCP tool's
// default (cmd/atlas-mcp/server/tools_stock_winrate.go).
const defaultWinRateWindow = "120d"

// winRateSourcePrefix is the outcome source prefix written by the
// stockpicker backfill ("stockpicker-<condition-id>"); a condition_id maps
// to the stored source via this prefix. Mirrors the MCP tool.
const winRateSourcePrefix = "stockpicker-"

// WinRateCondition is one persisted (symbol, source, window) summary.
// DataStart/DataEnd are the earliest/latest trigger_date of the stored
// outcomes for that source (資料期間), not a recomputation.
type WinRateCondition struct {
	ConditionID       string  `json:"condition_id"`
	Source            string  `json:"source"`
	Observations      int     `json:"observations"`
	Hits              int     `json:"hits"`
	WinRate           float64 `json:"win_rate"`
	WilsonLower       float64 `json:"wilson_lower"`
	WilsonUpper       float64 `json:"wilson_upper"`
	Confidence        float64 `json:"confidence"`
	CalibrationStatus string  `json:"calibration_status"`
	NetCostRate       float64 `json:"net_cost_rate"`
	AvgForwardReturn  float64 `json:"avg_forward_return"`
	// BreakEvenCostRate 是 forward 回報中位數：勝率跌破 5 成的來回成本上界。
	// 0 = 該列在 migration 000028 之前聚合（尚無此量測），**不得**解讀為
	// 「無 edge」——對照 NetCostRate 判讀。
	BreakEvenCostRate float64 `json:"break_even_cost_rate"`
	UpdatedAt         string  `json:"updated_at"` // ISO-8601
	DataStart         string  `json:"data_start,omitempty"`
	DataEnd           string  `json:"data_end,omitempty"`
}

// WinRateResponse is the JSON body of GET /api/stock/win_rate. found=false
// (+ message) means the symbol has no stored win-rate data — an
// informational answer, not an error, matching the MCP tool contract.
type WinRateResponse struct {
	Symbol        string             `json:"symbol"`
	RollingWindow string             `json:"rolling_window"`
	Found         bool               `json:"found"`
	Message       string             `json:"message,omitempty"`
	Conditions    []WinRateCondition `json:"conditions"`
}

// WinRateProvider is the minimal read-only interface the
// /api/stock/win_rate handler depends on. It exposes exactly the three
// reads the handler needs (source listing, summary load, outcome date
// range) so tests can inject a fake without opening SQLite.
type WinRateProvider interface {
	// Sources lists the outcome sources persisted for (symbol, window),
	// restricted to stockpicker sources ("stockpicker-<condition-id>"),
	// ordered by source.
	Sources(ctx context.Context, symbol, window string) ([]string, error)
	// LoadWinRate returns the persisted win-rate summary for
	// (symbol, source, window). found=false when no row exists.
	LoadWinRate(ctx context.Context, symbol, source, window string) (stockpicker.StockWinRateSummary, bool, error)
	// OutcomeDateRange returns the earliest/latest trigger_date of the
	// stored outcomes for (symbol, source); ok=false when there are none.
	OutcomeDateRange(ctx context.Context, symbol, source string) (start, end string, ok bool)
}

// WinRateDBProvider is the production WinRateProvider (and, since it reads
// through one handle, ConditionWinRateProvider, IndustryWinRateProvider and
// FamilyExpectancyProvider as well).
//
// It reads through exactly one of two handles:
//
//   - a backend-resolving WinRateSource (NewWinRateDBProvider): production
//     wiring — PostgreSQL when the deployment declares it and it is reachable,
//     the read-only SQLite ledger otherwise (C6b);
//   - a fixed *sql.DB (NewWinRateDBProviderForHandle): one already-open
//     read-only ledger handle, used by tests and by single-backend callers.
//
// Read-only is a property of the handles, not of this type: the SQLite handle
// is opened with mode=ro and the PostgreSQL session with
// default_transaction_read_only=on, so any accidental write fails at the driver
// level on either backend.
type WinRateDBProvider struct {
	db  *sql.DB
	src *WinRateSource
}

// NewWinRateDBProvider builds the backend-resolving provider used by
// cmd/atlas: every read asks src which store to use (PostgreSQL first, SQLite
// fallback), so a PostgreSQL outage degrades to the SQLite ledger and a
// PostgreSQL that comes back is picked up without a restart.
func NewWinRateDBProvider(src *WinRateSource) *WinRateDBProvider {
	return &WinRateDBProvider{src: src}
}

// NewWinRateDBProviderForHandle wraps an already-open read-only ledger handle,
// without backend resolution.
func NewWinRateDBProviderForHandle(db *sql.DB) *WinRateDBProvider {
	return &WinRateDBProvider{db: db}
}

// handle returns the database this call must read. Resolution failures are
// reported as errors (and answered as 503 by the handlers) instead of silently
// reading a different store.
func (p *WinRateDBProvider) handle(ctx context.Context) (*sql.DB, error) {
	switch {
	case p.src != nil:
		h, err := p.src.Handle(ctx)
		if err != nil {
			return nil, err
		}
		return h.db, nil
	case p.db != nil:
		return p.db, nil
	default:
		return nil, errors.New("stocktools: win-rate provider has no database handle")
	}
}

// Sources implements WinRateProvider.
//
// $N placeholders: the same text runs on both backends (see
// winRateReadHandle) — positional binding on the SQLite driver maps arguments
// to the parameters in order of appearance.
func (p *WinRateDBProvider) Sources(ctx context.Context, symbol, window string) ([]string, error) {
	db, err := p.handle(ctx)
	if err != nil {
		return nil, fmt.Errorf("stocktools: list win-rate sources %s/%s: %w", symbol, window, err)
	}
	const q = `SELECT DISTINCT source FROM stock_win_rate
		WHERE symbol = $1 AND rolling_window = $2
		  AND source LIKE 'stockpicker-%'
		ORDER BY source`
	rows, err := db.QueryContext(ctx, q, symbol, window)
	if err != nil {
		return nil, fmt.Errorf("stocktools: list win-rate sources %s/%s: %w", symbol, window, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return nil, fmt.Errorf("stocktools: scan win-rate source: %w", err)
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// LoadWinRate implements WinRateProvider.
func (p *WinRateDBProvider) LoadWinRate(ctx context.Context, symbol, source, window string) (stockpicker.StockWinRateSummary, bool, error) {
	db, err := p.handle(ctx)
	if err != nil {
		return stockpicker.StockWinRateSummary{}, false, err
	}
	return stockpicker.LoadWinRate(ctx, db, symbol, source, window)
}

// OutcomeDateRange implements WinRateProvider. It reads raw rows
// (window=""), never aggregates.
//
// A resolution failure is reported as ok=false, exactly like a read failure:
// the interface has no error to carry (the handler omits data_start/data_end),
// and the failure is already surfaced by Sources or LoadWinRate earlier in the
// same request.
func (p *WinRateDBProvider) OutcomeDateRange(ctx context.Context, symbol, source string) (start, end string, ok bool) {
	db, err := p.handle(ctx)
	if err != nil {
		return "", "", false
	}
	outcomes, err := stockpicker.LoadOutcomes(ctx, db, symbol, source, "")
	if err != nil || len(outcomes) == 0 {
		return "", "", false
	}
	// LoadOutcomes orders rows by trigger_date ASC (signal_outcome_store.go),
	// so the first row is the earliest trigger_date and the last is the
	// latest — a min/max scan over all rows would be redundant (k3 review
	// M2). First/last row reads only.
	return outcomes[0].TriggerDate, outcomes[len(outcomes)-1].TriggerDate, true
}

// OpenWinRateDB opens the stockpicker ledger at path in read-only mode,
// returning a descriptive error when the path is missing so the caller can
// answer "no data configured" instead of panicking. Mirrors the MCP
// server's stockpickerDB() open (cmd/atlas-mcp/server/tools_stock_winrate.go).
func OpenWinRateDB(path string) (*sql.DB, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("stockpicker DB path %s: %w", path, err)
	}
	if _, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("stockpicker DB %s: %w", abs, err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(abs)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open stockpicker DB %s: %w", abs, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open stockpicker DB %s: %w", abs, err)
	}
	return db, nil
}

// stripSymbolSuffix removes the exchange-suffix variants callers may pass
// (.TW / .TWO) — the stockpicker ledger stores bare 4–6 digit codes.
func stripSymbolSuffix(symbol string) string {
	return strings.TrimSuffix(strings.TrimSuffix(symbol, ".TW"), ".TWO")
}

// ConditionWinRateProvider is the minimal read-only interface the
// /api/stock/condition_winrate handler depends on: one condition-level
// (cross-symbol) aggregate read. Kept separate from WinRateProvider so the
// existing per-symbol fakes stay valid (issue #1865).
type ConditionWinRateProvider interface {
	// LoadConditionWinRate returns the cross-symbol aggregate for one
	// source ("stockpicker-<condition-id>") over the rolling window.
	// regime ("" = all regimes, or e.g. "RISK_ON") restricts the aggregate
	// to outcomes whose trigger-date regime matches (issue #1863).
	// found=false when the source has zero stored outcomes in the window.
	LoadConditionWinRate(ctx context.Context, source, window, regime string) (stockpicker.ConditionWinRateSummary, bool, error)
}

// LoadConditionWinRate implements ConditionWinRateProvider: it reads raw
// outcomes across all symbols (symbol="") for the source and aggregates
// them with the same math as the per-symbol path (stockpicker.ConditionWinRate
// shares WinRate/WilsonScoreInterval/CalibrationStatusFor with
// SignalWinRate). Cost rate and min-samples come from the live parameters
// config, same as the daily aggregation job.
func (p *WinRateDBProvider) LoadConditionWinRate(ctx context.Context, source, window, regime string) (stockpicker.ConditionWinRateSummary, bool, error) {
	db, err := p.handle(ctx)
	if err != nil {
		return stockpicker.ConditionWinRateSummary{}, false, err
	}
	outcomes, err := stockpicker.LoadOutcomes(ctx, db, "", source, window)
	if err != nil {
		return stockpicker.ConditionWinRateSummary{}, false, fmt.Errorf("stocktools: load condition outcomes %s: %w", source, err)
	}
	if regime != "" {
		// Regime-stratified view (issue #1863): in-memory filter on the
		// outcome's trigger-date regime. Rows written before regime tagging
		// (Regime == "") are excluded from filtered views — they cannot be
		// attributed, and silently pooling them would skew the strata.
		filtered := outcomes[:0]
		for _, o := range outcomes {
			if o.Regime == regime {
				filtered = append(filtered, o)
			}
		}
		outcomes = filtered
	}
	if len(outcomes) == 0 {
		return stockpicker.ConditionWinRateSummary{}, false, nil
	}
	params := config.GetParametersConfig()
	summary := stockpicker.ConditionWinRate(source, outcomes,
		params.Stockpicker.Costs.RoundTripPct.Value,
		params.Stockpicker.Calibration.MinSamples.Value,
		0.95)
	summary.Window = window
	summary.Regime = regime
	return summary, true, nil
}
