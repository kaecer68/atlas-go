// Package stocktools — win-rate REST endpoint (PR 3c, Part 1).
//
// GET /api/stock/win_rate exposes the Phase-4 stockpicker win-rate
// aggregates (stock_win_rate + stock_signal_outcomes in the job-local
// SQLite ledger) as a read-only HTTP endpoint, mirroring the MCP tool
// stock_get_win_rate (cmd/atlas-mcp/server/tools_stock_winrate.go) so the
// client_web stock-quote page and the MCP channel answer the same question
// with the same semantics (200 + found=false when no data exists, never a
// recomputation).
package stocktools

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // driver for the read-only stockpicker SQLite handle

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

// SQLiteWinRateProvider is the production WinRateProvider backed by the
// job-local stockpicker SQLite ledger (data/state/atlas.db or
// ATLAS_MCP_STOCKPICKER_DB). Read-only by construction: the handle is
// opened with mode=ro, so any accidental write fails at the driver level.
type SQLiteWinRateProvider struct {
	db *sql.DB
}

// NewSQLiteWinRateProvider wraps an open read-only ledger handle.
func NewSQLiteWinRateProvider(db *sql.DB) *SQLiteWinRateProvider {
	return &SQLiteWinRateProvider{db: db}
}

// Sources implements WinRateProvider.
func (p *SQLiteWinRateProvider) Sources(ctx context.Context, symbol, window string) ([]string, error) {
	const q = `SELECT DISTINCT source FROM stock_win_rate
		WHERE symbol = ? AND rolling_window = ?
		  AND source LIKE 'stockpicker-%'
		ORDER BY source`
	rows, err := p.db.QueryContext(ctx, q, symbol, window)
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
func (p *SQLiteWinRateProvider) LoadWinRate(ctx context.Context, symbol, source, window string) (stockpicker.StockWinRateSummary, bool, error) {
	return stockpicker.LoadWinRate(ctx, p.db, symbol, source, window)
}

// OutcomeDateRange implements WinRateProvider. It reads raw rows
// (window=""), never aggregates.
func (p *SQLiteWinRateProvider) OutcomeDateRange(ctx context.Context, symbol, source string) (start, end string, ok bool) {
	outcomes, err := stockpicker.LoadOutcomes(ctx, p.db, symbol, source, "")
	if err != nil || len(outcomes) == 0 {
		return "", "", false
	}
	start, end = outcomes[0].TriggerDate, outcomes[0].TriggerDate
	for _, o := range outcomes[1:] {
		if o.TriggerDate < start {
			start = o.TriggerDate
		}
		if o.TriggerDate > end {
			end = o.TriggerDate
		}
	}
	return start, end, true
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
