package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	_ "modernc.org/sqlite" // driver for the read-only stockpicker SQLite handle

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// defaultStockWinRateWindow is the rolling-window label used by the
// stockpicker backfill job (cmd/run-stockpicker-backtest) when it aggregates
// raw outcomes into stock_win_rate rows.
const defaultStockWinRateWindow = "120d"

// stockWinRateSourcePrefix is the outcome source prefix written by
// RunBacktest ("stockpicker-<condition-id>"); a condition_id maps to the
// stored source via this prefix.
const stockWinRateSourcePrefix = "stockpicker-"

func registerStockWinRateTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_get_win_rate",
		Description: autoDescOr("stock_get_win_rate", "Return the persisted Phase-4 stock win-rate aggregate for a Taiwan stock symbol (read-only; never recomputes). Data source: stockpicker backfill job output (stock_win_rate + stock_signal_outcomes in the SQLite ledger configured via ATLAS_MCP_STOCKPICKER_DB). Input: symbol (+ optional condition_id, rolling_window, asof). No stored data for the symbol returns found=false with a clear message. Alternative: stock_get_quote, stock_get_technical."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleStockGetWinRate)
}

type stockWinRateInput struct {
	Symbol        string `json:"symbol" jsonschema:"the Taiwan stock symbol, e.g. 2330"`
	ConditionID   string `json:"condition_id,omitempty" jsonschema:"condition id, e.g. foreign-3d-net-buy or momentum-20d-positive; default: return every condition available for the symbol"`
	RollingWindow string `json:"rolling_window,omitempty" jsonschema:"rolling window label, e.g. 120d; default 120d"`
	AsOf          string `json:"asof,omitempty" jsonschema:"as-of date YYYY-MM-DD (informational; stored aggregates are returned as-is, never recomputed)"`
}

// stockWinRateCondition is one persisted (symbol, source, window) summary.
// DataStart/DataEnd are the earliest/latest trigger_date of the stored
// outcomes for that source (資料期間), not a recomputation.
type stockWinRateCondition struct {
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
	UpdatedAt         string  `json:"updated_at"`
	DataStart         string  `json:"data_start,omitempty"`
	DataEnd           string  `json:"data_end,omitempty"`
}

type stockWinRateOutput struct {
	Symbol        string                  `json:"symbol"`
	RollingWindow string                  `json:"rolling_window"`
	Found         bool                    `json:"found"`
	Message       string                  `json:"message,omitempty"`
	AsOf          string                  `json:"asof,omitempty"`
	Conditions    []stockWinRateCondition `json:"conditions"`
}

func (s *server) handleStockGetWinRate(ctx context.Context, _ *mcp.CallToolRequest, in stockWinRateInput) (*mcp.CallToolResult, stockWinRateOutput, error) {
	if in.Symbol == "" {
		return nil, stockWinRateOutput{}, fmt.Errorf("stock_get_win_rate: symbol is required")
	}
	window := in.RollingWindow
	if window == "" {
		window = defaultStockWinRateWindow
	}

	out := stockWinRateOutput{
		Symbol:        in.Symbol,
		RollingWindow: window,
		AsOf:          in.AsOf,
		Conditions:    []stockWinRateCondition{},
	}

	err := s.withAudit(ctx, "stock_get_win_rate", []string{"symbol", "condition_id", "rolling_window", "asof"}, func() error {
		db, err := s.stockpickerDB(ctx)
		if err != nil {
			out.Message = fmt.Sprintf("no stockpicker win-rate data available for %s: %v", in.Symbol, err)
			return nil
		}
		winStore := stockpicker.NewWinRateStore(db)
		outcomeStore := stockpicker.NewSignalOutcomeStore(db)

		sources, err := winRateSources(ctx, db, in.Symbol, window, in.ConditionID)
		if err != nil {
			return fmt.Errorf("stock_get_win_rate: list sources: %w", err)
		}
		for _, source := range sources {
			summary, found, err := winStore.LoadWinRate(ctx, in.Symbol, source, window)
			if err != nil {
				return fmt.Errorf("stock_get_win_rate: load win rate %s/%s/%s: %w", in.Symbol, source, window, err)
			}
			if !found {
				continue
			}
			out.Found = true
			cond := stockWinRateCondition{
				ConditionID:       strings.TrimPrefix(source, stockWinRateSourcePrefix),
				Source:            source,
				Observations:      summary.Observations,
				Hits:              summary.Hits,
				WinRate:           summary.WinRate,
				WilsonLower:       summary.WilsonLower,
				WilsonUpper:       summary.WilsonUpper,
				Confidence:        summary.Confidence,
				CalibrationStatus: string(summary.CalibrationStatus),
				NetCostRate:       summary.NetCostRate,
				AvgForwardReturn:  summary.AvgForwardReturn,
				UpdatedAt:         summary.UpdatedAt,
			}
			if start, end, ok := winRateOutcomeDateRange(ctx, outcomeStore, in.Symbol, source); ok {
				cond.DataStart = start
				cond.DataEnd = end
			}
			out.Conditions = append(out.Conditions, cond)
		}

		if !out.Found {
			scope := ""
			if in.ConditionID != "" {
				scope = ", condition " + in.ConditionID
			}
			out.Message = fmt.Sprintf("no stockpicker win-rate data for symbol %s (window %s%s)", in.Symbol, window, scope)
		}
		return nil
	})
	if err != nil {
		return nil, stockWinRateOutput{}, err
	}
	return nil, out, nil
}

// winRateSources resolves the outcome sources to read for a request. An
// explicit condition_id maps to the single source "stockpicker-<id>"; an
// empty condition_id lists every stockpicker source persisted for the
// (symbol, window) key, ordered by source.
func winRateSources(ctx context.Context, db *sql.DB, symbol, window, conditionID string) ([]string, error) {
	if conditionID != "" {
		return []string{stockWinRateSourcePrefix + conditionID}, nil
	}
	const q = `SELECT DISTINCT source FROM stock_win_rate
		WHERE symbol = ? AND rolling_window = ?
		  AND source LIKE 'stockpicker-%'
		ORDER BY source`
	rows, err := db.QueryContext(ctx, q, symbol, window)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// winRateOutcomeDateRange returns the earliest/latest trigger_date of the
// stored outcomes for (symbol, source) — the 資料期間 of the win-rate data.
// It reads raw rows (window=""), never aggregates.
func winRateOutcomeDateRange(ctx context.Context, store *stockpicker.SignalOutcomeStore, symbol, source string) (start, end string, ok bool) {
	outcomes, err := store.LoadOutcomes(ctx, symbol, source, "")
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

// stockpickerDB returns the cached read-only SQLite handle backing the
// stockpicker win-rate stores, opening it lazily from
// Config.StockpickerDBPath on first use. Read-only mode (mode=ro) makes the
// no-write contract structural: any accidental write fails at the driver
// level. A nil/empty path or a missing file is reported as a descriptive
// error so the handler can answer "no data" instead of panicking.
func (s *server) stockpickerDB(ctx context.Context) (*sql.DB, error) {
	s.winRateMu.Lock()
	defer s.winRateMu.Unlock()
	if s.winRateDB != nil {
		return s.winRateDB, nil
	}
	path := s.cfg.StockpickerDBPath
	if path == "" {
		return nil, fmt.Errorf("stockpicker DB not configured (set ATLAS_MCP_STOCKPICKER_DB)")
	}
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
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("open stockpicker DB %s: %w", abs, err)
	}
	s.winRateDB = db
	return db, nil
}
