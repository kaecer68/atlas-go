package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Defaults for stock_picker_scan. The min_observations default mirrors the
// platform's "樣本足" gate (calibration min_samples = 30 in config defaults;
// we keep a slightly looser 20 so the scan stays usable for early windows).
const (
	defaultScanMinObservations = 20
	defaultScanMinWinRate      = 0.5
	defaultScanTopN            = 10
	maxScanTopN                = 200
)

// registerStockPickerScanTools registers the read-only multi-symbol win-rate
// scan tool. It reads persisted stock_win_rate rows (the same table
// stock_get_win_rate reads) and never recomputes aggregates.
func registerStockPickerScanTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stock_picker_scan",
		Description: autoDescOr("stock_picker_scan", "Scan persisted Phase-4 stock win-rate aggregates across symbols and return the best candidates (read-only; never recomputes). Data source: stockpicker backfill job output (stock_win_rate in the SQLite ledger configured via ATLAS_MCP_STOCKPICKER_DB). Input: optional condition_id, rolling_window, min_observations, min_win_rate, top_n (default 10), sort_by (wilson_lower default | win_rate), asof, direction. Candidates are filtered to observations >= min_observations, win_rate >= min_win_rate, calibration_status=eligible, then sorted and truncated to top_n. DIRECTION SEMANTICS: conditions are buy-side by default; condition price-volume-top-divergence (頂背離) is AVOID-semantics — a LOW forward win rate after trigger confirms the bearish signal, so for it pass direction=avoid to invert the filter (win_rate <= max_win_rate, default 0.5) and ordering (weakest forward performance first). Without direction=avoid the default buy filter hides exactly the rows where the avoid signal is strongest (k3 review F1). No stored data returns found=false with a clear message. Alternative: stock_get_win_rate for a single symbol."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleStockPickerScan)
}

type stockPickerScanInput struct {
	ConditionID     string  `json:"condition_id,omitempty" jsonschema:"condition id, e.g. foreign-3d-net-buy or momentum-20d-positive; default: scan across every stockpicker condition"`
	RollingWindow   string  `json:"rolling_window,omitempty" jsonschema:"rolling window label, e.g. 120d; default 120d"`
	MinObservations int     `json:"min_observations,omitempty" jsonschema:"minimum stored observations (sample size) for a candidate; default 20"`
	MinWinRate      float64 `json:"min_win_rate,omitempty" jsonschema:"minimum win_rate (0..1) for a candidate; default 0.5"`
	TopN            int     `json:"top_n,omitempty" jsonschema:"maximum number of candidates to return; default 10, capped at 200"`
	SortBy          string  `json:"sort_by,omitempty" jsonschema:"sort key: wilson_lower (default, sample-size weighted) or win_rate"`
	AsOf            string  `json:"asof,omitempty" jsonschema:"as-of date YYYY-MM-DD (informational; stored aggregates are returned as-is, never recomputed)"`
	Direction       string  `json:"direction,omitempty" jsonschema:"buy (default) or avoid; use avoid for avoid-semantics conditions such as price-volume-top-divergence — inverts the win-rate filter to win_rate <= min_win_rate and orders weakest forward performance first"`
}

// stockPickerScanCandidate is one persisted (symbol, source, window) summary
// that passed the scan filters.
type stockPickerScanCandidate struct {
	Symbol            string  `json:"symbol"`
	ConditionID       string  `json:"condition_id"`
	Source            string  `json:"source"`
	RollingWindow     string  `json:"rolling_window"`
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
}

type stockPickerScanOutput struct {
	Found      bool                       `json:"found"`
	Message    string                     `json:"message,omitempty"`
	AsOf       string                     `json:"asof,omitempty"`
	Direction  string                     `json:"direction,omitempty"` // "buy" (default) or "avoid" — echo so consumers can't misread the ranking
	Total      int                        `json:"total"`
	Candidates []stockPickerScanCandidate `json:"candidates"`
}

func (s *server) handleStockPickerScan(ctx context.Context, _ *mcp.CallToolRequest, in stockPickerScanInput) (*mcp.CallToolResult, stockPickerScanOutput, error) {
	window := in.RollingWindow
	if window == "" {
		window = defaultStockWinRateWindow
	}
	minObs := in.MinObservations
	if minObs <= 0 {
		minObs = defaultScanMinObservations
	}
	minWinRate := in.MinWinRate
	if minWinRate <= 0 {
		minWinRate = defaultScanMinWinRate
	}
	if minWinRate > 1 {
		return nil, stockPickerScanOutput{}, fmt.Errorf("stock_picker_scan: min_win_rate %v out of range (0..1)", minWinRate)
	}
	topN := in.TopN
	if topN <= 0 {
		topN = defaultScanTopN
	}
	if topN > maxScanTopN {
		topN = maxScanTopN
	}
	sortCol := "wilson_lower"
	switch in.SortBy {
	case "", "wilson_lower":
	case "win_rate":
		sortCol = "win_rate"
	default:
		return nil, stockPickerScanOutput{}, fmt.Errorf("stock_picker_scan: sort_by %q must be wilson_lower or win_rate", in.SortBy)
	}

	// direction (k3 review F1): avoid-semantics conditions (頂背離) are
	// confirmed by LOW forward win rates, so the buy filter
	// (win_rate >= min) hides exactly the strongest avoid signals. In avoid
	// mode the filter becomes win_rate <= min_win_rate (reinterpreted as a
	// ceiling) and ordering flips to weakest-forward-performance first.
	direction := in.Direction
	if direction == "" {
		direction = "buy"
	}
	if direction != "buy" && direction != "avoid" {
		return nil, stockPickerScanOutput{}, fmt.Errorf("stock_picker_scan: direction %q must be buy or avoid", in.Direction)
	}

	out := stockPickerScanOutput{
		AsOf:       in.AsOf,
		Direction:  direction,
		Candidates: []stockPickerScanCandidate{},
	}

	err := s.withAudit(ctx, "stock_picker_scan", []string{"condition_id", "rolling_window", "min_observations", "min_win_rate", "top_n", "sort_by", "asof"}, func() error {
		db, err := s.stockpickerDB(ctx)
		if err != nil {
			out.Message = fmt.Sprintf("no stockpicker win-rate data available for scan: %v", err)
			return nil
		}

		rows, err := scanWinRateRows(ctx, db, window, in.ConditionID, minObs, minWinRate, sortCol, direction)
		if err != nil {
			return fmt.Errorf("stock_picker_scan: query win rates: %w", err)
		}
		if len(rows) == 0 {
			out.Message = scanNoDataMessage(window, in.ConditionID, minObs, minWinRate)
			return nil
		}

		out.Found = true
		out.Total = len(rows)
		if len(rows) > topN {
			rows = rows[:topN]
		}
		out.Candidates = rows
		return nil
	})
	if err != nil {
		return nil, stockPickerScanOutput{}, err
	}
	return nil, out, nil
}

// scanWinRateRows lists persisted eligible win-rate rows for the window
// (optionally restricted to one condition), filtered by the caller's
// min_observations / min_win_rate / calibration_status=eligible gates and
// ordered by sortCol (one of the whitelisted columns wilson_lower|win_rate).
// It reads stock_win_rate directly — the same persisted aggregates
// stock_get_win_rate reads — and never recomputes.
func scanWinRateRows(ctx context.Context, db *sql.DB, window, conditionID string, minObs int, minWinRate float64, sortCol, direction string) ([]stockPickerScanCandidate, error) {
	query := `SELECT symbol, source, rolling_window, observations, hits, win_rate,
		wilson_lower, wilson_upper, confidence, calibration_status,
		net_cost_rate, avg_forward_return, updated_at
	FROM stock_win_rate
	WHERE rolling_window = ? AND source LIKE 'stockpicker-%'
	  AND observations >= ? AND win_rate >= ?
	  AND calibration_status = 'eligible'`
	args := []any{window, minObs, minWinRate}
	if conditionID != "" {
		query += ` AND source = ?`
		args = append(args, stockWinRateSourcePrefix+conditionID)
	}
	// sortCol is whitelisted by the handler (wilson_lower | win_rate); select
	// the full ORDER BY clause from fixed constants so no input reaches SQL
	// text (gosec G202).
	orderBy := "wilson_lower DESC, symbol ASC"
	if sortCol == "win_rate" {
		orderBy = "win_rate DESC, symbol ASC"
	}
	if direction == "avoid" {
		// Avoid semantics (頂背離): the strongest signals are the LOWEST
		// forward win rates. win_rate >= ? above is the buy-side floor;
		// replace it with a ceiling and invert the ranking so the most
		// confidently-bad rows (lowest wilson_upper / win_rate) come first.
		query = strings.Replace(query, "win_rate >= ?", "win_rate <= ?", 1)
		orderBy = "wilson_upper ASC, symbol ASC"
		if sortCol == "win_rate" {
			orderBy = "win_rate ASC, symbol ASC"
		}
	}
	query += ` ORDER BY ` + orderBy

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []stockPickerScanCandidate
	for rows.Next() {
		var c stockPickerScanCandidate
		var status string
		if err := rows.Scan(
			&c.Symbol, &c.Source, &c.RollingWindow, &c.Observations, &c.Hits, &c.WinRate,
			&c.WilsonLower, &c.WilsonUpper, &c.Confidence, &status,
			&c.NetCostRate, &c.AvgForwardReturn, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		c.CalibrationStatus = status
		c.ConditionID = strings.TrimPrefix(c.Source, stockWinRateSourcePrefix)
		out = append(out, c)
	}
	return out, rows.Err()
}

// scanNoDataMessage builds the clear no-data message for the scan tool.
func scanNoDataMessage(window, conditionID string, minObs int, minWinRate float64) string {
	msg := fmt.Sprintf("no stockpicker scan candidates for window %s (min_observations %d, min_win_rate %.2f, calibration_status eligible", window, minObs, minWinRate)
	if conditionID != "" {
		msg += ", condition " + conditionID
	}
	msg += ")"
	return msg
}
