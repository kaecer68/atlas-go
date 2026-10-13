package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

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
		Description: autoDescOr("stock_get_win_rate", "Return the persisted Phase-4 stock win-rate aggregate for a Taiwan stock symbol (read-only; never recomputes). Data source: stockpicker backfill job output (stock_win_rate + stock_signal_outcomes), read-only from PostgreSQL when DATABASE_URL is set and ATLAS_STORE_BACKEND declares postgres, otherwise read-only from the SQLite ledger configured via ATLAS_MCP_STOCKPICKER_DB (both paths run the same SQL; the PostgreSQL handle is opened default_transaction_read_only=on and the SQLite handle mode=ro, so neither can write). Input: symbol (+ optional condition_id, rolling_window, asof). Known condition_ids: foreign-3d-net-buy, momentum-20d-positive, price-volume-top-divergence, price-volume-bottom-divergence. DIRECTION SEMANTICS: price-volume-top-divergence (頂背離) is AVOID-semantics — a LOW win_rate / negative avg_forward_return after trigger CONFIRMS the bearish avoid signal, do not read it as a bad condition. No stored data for the symbol returns found=false with a clear message. Alternative: stock_get_quote, stock_get_technical."),
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

	// winRateBackend records which ledger actually answered this call, so the
	// audit log says where a number came from (PostgreSQL, the SQLite
	// fallback, or nobody). "none" only happens when neither backend could be
	// opened, which is also when the handler answers no-data.
	winRateBackend := "none"
	extraFn := func() map[string]any {
		return map[string]any{"win_rate_backend": winRateBackend}
	}

	err := s.withAuditExtra(ctx, "stock_get_win_rate", []string{"symbol", "condition_id", "rolling_window", "asof"}, extraFn, func() error {
		h, err := s.winRateHandle(ctx)
		if err != nil {
			out.Message = fmt.Sprintf("no stockpicker win-rate data available for %s: %v", in.Symbol, err)
			return nil
		}
		winRateBackend = h.backend
		winStore := stockpicker.NewWinRateStore(h.db)
		outcomeStore := stockpicker.NewSignalOutcomeStore(h.db)

		sources, err := winRateSources(ctx, h.db, in.Symbol, window, in.ConditionID)
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
	// $N placeholders (not ?): the same statement runs on both backends, see
	// winRateReadHandle for why SQLite accepts PostgreSQL-style placeholders.
	const q = `SELECT DISTINCT source FROM stock_win_rate
		WHERE symbol = $1 AND rolling_window = $2
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
