package server

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// Defaults for stockpicker_walkforward. forwardDays mirrors the backtest
// holding period (DefaultForwardDays = 5); minTrain mirrors the house
// "樣本足" floor used by the HRP overlap gate (minHRPOverlap = 5).
const (
	defaultWalkforwardFolds       = 3
	defaultWalkforwardMinTrain    = 5
	defaultWalkforwardForwardDays = 5
	defaultWalkforwardCostRate    = 0.00585
)

// registerStockpickerWalkforwardTools registers the read-only walk-forward
// validation tool. It reads persisted signal outcomes (the same table the
// backtest aggregation reads) and never recomputes backtests.
func registerStockpickerWalkforwardTools(mcpSrv *mcp.Server, s *server) {
	countedAddTool(mcpSrv, &mcp.Tool{
		Name:        "stockpicker_walkforward",
		Description: autoDescOr("stockpicker_walkforward", "Run a read-only walk-forward validation over persisted stockpicker signal outcomes for one condition (never recomputes backtests). Data source: stock_signal_outcomes, read-only from PostgreSQL when DATABASE_URL is set and ATLAS_STORE_BACKEND declares postgres, otherwise read-only from the SQLite ledger configured via ATLAS_MCP_STOCKPICKER_DB. Input: condition_id (required — a cross-condition pooled walk-forward is meaningless), rolling_window (default 120d), folds (default 3), min_train (default 5), forward_days (default 5, trading days), asof (default now, PIT cutoff). Output: per-fold validation windows (val_start/val_end, train/val observations, pooled net win_rate) plus the win-rate distribution (n/mean/min/max) across folds — read the distribution, not a single Sharpe. Label windows are computed on the trading calendar (long holidays included), so purge is exact. Demoted families are allowed here: this is measurement, not ranking (unlike stock_picker_scan which withholds them). DIRECTION SEMANTICS: the pooled win_rate is reported as-is; for avoid-semantics conditions such as price-volume-top-divergence a LOW win rate confirms the bearish signal. No stored outcomes returns found=false with a clear message. Alternative: stock_get_win_rate for a single symbol."),
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, s.handleStockpickerWalkforward)
}

type stockpickerWalkforwardInput struct {
	ConditionID   string `json:"condition_id,omitempty" jsonschema:"condition id, e.g. price-volume-bottom-divergence; required (a cross-condition walk-forward is meaningless)"`
	RollingWindow string `json:"rolling_window,omitempty" jsonschema:"rolling window label, e.g. 120d; default 120d"`
	Folds         int    `json:"folds,omitempty" jsonschema:"number of walk-forward folds; default 3"`
	MinTrain      int    `json:"min_train,omitempty" jsonschema:"minimum training observations per fold; default 5"`
	ForwardDays   int    `json:"forward_days,omitempty" jsonschema:"holding period in trading days; default 5"`
	AsOf          string `json:"asof,omitempty" jsonschema:"as-of date YYYY-MM-DD (PIT cutoff; stored outcomes are returned as-is, never recomputed)"`
}

// walkforwardFoldResult is one validation fold of the walk-forward report.
type walkforwardFoldResult struct {
	ValStart string  `json:"val_start"`
	ValEnd   string  `json:"val_end"`
	TrainObs int     `json:"train_obs"`
	ValObs   int     `json:"val_obs"`
	WinRate  float64 `json:"win_rate"`
}

type stockpickerWalkforwardOutput struct {
	Found         bool                            `json:"found"`
	Message       string                          `json:"message,omitempty"`
	ConditionID   string                          `json:"condition_id,omitempty"`
	RollingWindow string                          `json:"rolling_window,omitempty"`
	AsOf          string                          `json:"asof,omitempty"`
	Folds         []walkforwardFoldResult         `json:"folds"`
	Distribution  stockpicker.HitRateDistribution `json:"distribution"`
}

func (s *server) handleStockpickerWalkforward(ctx context.Context, _ *mcp.CallToolRequest, in stockpickerWalkforwardInput) (*mcp.CallToolResult, stockpickerWalkforwardOutput, error) {
	if in.ConditionID == "" {
		return nil, stockpickerWalkforwardOutput{}, fmt.Errorf("stockpicker_walkforward: condition_id is required")
	}
	window := in.RollingWindow
	if window == "" {
		window = defaultStockWinRateWindow
	}
	foldsN := in.Folds
	if foldsN <= 0 {
		foldsN = defaultWalkforwardFolds
	}
	minTrain := in.MinTrain
	if minTrain <= 0 {
		minTrain = defaultWalkforwardMinTrain
	}
	forwardDays := in.ForwardDays
	if forwardDays <= 0 {
		forwardDays = defaultWalkforwardForwardDays
	}
	asOf := time.Now().UTC()
	asOfStr := in.AsOf
	if asOfStr != "" {
		parsed, err := time.Parse("2006-01-02", asOfStr)
		if err != nil {
			return nil, stockpickerWalkforwardOutput{}, fmt.Errorf("stockpicker_walkforward: invalid asof %q (want YYYY-MM-DD)", asOfStr)
		}
		asOf = parsed
		asOfStr = parsed.Format("2006-01-02")
	} else {
		asOfStr = asOf.Format("2006-01-02")
	}

	out := stockpickerWalkforwardOutput{
		ConditionID:   in.ConditionID,
		RollingWindow: window,
		AsOf:          asOfStr,
		Folds:         []walkforwardFoldResult{},
	}

	winRateBackend := "none"
	extraFn := func() map[string]any {
		return map[string]any{"win_rate_backend": winRateBackend}
	}

	err := s.withAuditExtra(ctx, "stockpicker_walkforward", []string{"condition_id", "rolling_window", "folds", "min_train", "forward_days", "asof"}, extraFn, func() error {
		h, err := s.winRateHandle(ctx)
		if err != nil {
			out.Message = fmt.Sprintf("no stockpicker outcome data available: %v", err)
			return nil
		}
		winRateBackend = h.backend

		outcomes, err := stockpicker.LoadOutcomesAsOf(ctx, h.db, "", stockWinRateSourcePrefix+in.ConditionID, window, asOf)
		if err != nil {
			return fmt.Errorf("stockpicker_walkforward: load outcomes: %w", err)
		}
		if len(outcomes) == 0 {
			out.Message = fmt.Sprintf("no stockpicker outcomes for condition %s window %s asof %s", in.ConditionID, window, asOfStr)
			return nil
		}

		folds := stockpicker.WalkForwardFolds(outcomes, foldsN, minTrain, forwardDays)
		if len(folds) == 0 {
			out.Message = fmt.Sprintf("insufficient outcome history for %d folds (min_train %d) for condition %s", foldsN, minTrain, in.ConditionID)
			return nil
		}

		costRate := defaultWalkforwardCostRate
		if outcomes[0].CostRate > 0 {
			costRate = outcomes[0].CostRate
		}
		rates := make([]float64, 0, len(folds))
		for _, fold := range folds {
			rate := stockpicker.FoldHitRate(fold, costRate)
			rates = append(rates, rate)
			out.Folds = append(out.Folds, walkforwardFoldResult{
				ValStart: fold.Val[0].TriggerDate,
				ValEnd:   fold.Val[len(fold.Val)-1].TriggerDate,
				TrainObs: len(fold.Train),
				ValObs:   len(fold.Val),
				WinRate:  rate,
			})
		}
		out.Distribution = stockpicker.SummarizeHitRates(rates)
		out.Found = true
		return nil
	})
	if err != nil {
		return nil, stockpickerWalkforwardOutput{}, err
	}
	return nil, out, nil
}
