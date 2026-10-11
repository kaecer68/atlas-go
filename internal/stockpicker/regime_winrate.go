package stockpicker

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RegimeWinRateSummary 是單一 (symbol, source, window, regime) stratum 的
// 勝率聚合：與 StockWinRateSummary 同口徑，多 regime 維度。空 regime 的
// outcome 歸入 UnknownRegime（family_expectancy.go 同口徑，永不回填推測）。
type RegimeWinRateSummary struct {
	Symbol            string            `json:"symbol"`
	Source            string            `json:"source"`
	Window            string            `json:"window"`
	Regime            string            `json:"regime"`
	Observations      int               `json:"observations"`
	Hits              int               `json:"hits"`
	WinRate           float64           `json:"win_rate"`
	WilsonLower       float64           `json:"wilson_lower"`
	WilsonUpper       float64           `json:"wilson_upper"`
	Confidence        float64           `json:"confidence"`
	CalibrationStatus CalibrationStatus `json:"calibration_status"`
	NetCostRate       float64           `json:"net_cost_rate"`
	AvgForwardReturn  float64           `json:"avg_forward_return"`
	UpdatedAt         string            `json:"updated_at"` // ISO-8601
}

// normalizeOutcomeRegime 將 outcome 的 regime 標籤正規化：空白/空 → UnknownRegime。
func normalizeOutcomeRegime(regime string) string {
	if strings.TrimSpace(regime) == "" {
		return UnknownRegime
	}
	return strings.TrimSpace(regime)
}

// GroupAndSummarizeByRegime 按 (symbol, source, regime) 分組並逐組走
// aggregateWinRate 同一口徑（PR 1b helper，與 GroupAndSummarize 一致）。
// window 標籤僅帶入 key，不做輸入過濾（呼叫端以 LoadOutcomesAsOf 過濾）。
// 空輸入回傳空切片。
func GroupAndSummarizeByRegime(outcomes []SignalOutcome, window string, costRate float64, minSamples int, confidence float64) []RegimeWinRateSummary {
	byKey := make(map[string][]SignalOutcome)
	keys := make([]string, 0)
	for _, o := range outcomes {
		key := o.Symbol + "\x00" + o.Source + "\x00" + normalizeOutcomeRegime(o.Regime)
		if _, ok := byKey[key]; !ok {
			keys = append(keys, key)
		}
		byKey[key] = append(byKey[key], o)
	}
	sort.Strings(keys)

	summaries := make([]RegimeWinRateSummary, 0, len(keys))
	for _, key := range keys {
		parts := strings.Split(key, "\x00")
		group := byKey[key]
		agg := aggregateWinRate(group, costRate, minSamples, confidence)
		summaries = append(summaries, RegimeWinRateSummary{
			Symbol:            parts[0],
			Source:            parts[1],
			Window:            window,
			Regime:            parts[2],
			Observations:      agg.Observations,
			Hits:              agg.Hits,
			WinRate:           agg.WinRate,
			WilsonLower:       agg.WilsonLower,
			WilsonUpper:       agg.WilsonUpper,
			Confidence:        agg.Confidence,
			CalibrationStatus: agg.CalibrationStatus,
			NetCostRate:       agg.NetCostRate,
			AvgForwardReturn:  agg.AvgForwardReturn,
		})
	}
	return summaries
}

// AggregateRegimeFromStore 載入 outcomes（可選 rolling window 截斷），按
// regime 分層聚合後 upsert 進 stock_win_rate_by_regime。
//
// v1 不做 degraded pass（該邏輯鎖定 pooled 序列；regime stratum 的 IS/OOS
// 背離偵測留待後續）。
func AggregateRegimeFromStore(ctx context.Context, outcomesStore *SignalOutcomeStore, winStore *WinRateStore, window string, costRate float64, minSamples int, confidence float64, asOf time.Time) ([]RegimeWinRateSummary, error) {
	outcomes, err := LoadOutcomesAsOf(ctx, outcomesStore.db, "", "", window, asOf)
	if err != nil {
		return nil, fmt.Errorf("stockpicker: regime aggregate load outcomes: %w", err)
	}
	summaries := GroupAndSummarizeByRegime(outcomes, window, costRate, minSamples, confidence)
	for _, s := range summaries {
		if err := winStore.SaveRegimeWinRate(ctx, s); err != nil {
			return nil, fmt.Errorf("stockpicker: regime aggregate save %s/%s/%s/%s: %w", s.Symbol, s.Source, s.Window, s.Regime, err)
		}
	}
	return summaries, nil
}

// SaveRegimeWinRate upserts a RegimeWinRateSummary keyed by
// (symbol, source, rolling_window, regime). `$n` placeholders keep the
// statement valid in both SQLite and PostgreSQL (dual-write in daily_update).
func (s *WinRateStore) SaveRegimeWinRate(ctx context.Context, summary RegimeWinRateSummary) error {
	return SaveRegimeWinRate(ctx, s.db, summary)
}

// LoadRegimeWinRate reads one stratum. The second return value is false when
// no row exists; the error is nil in that case.
func (s *WinRateStore) LoadRegimeWinRate(ctx context.Context, symbol, source, window, regime string) (RegimeWinRateSummary, bool, error) {
	return LoadRegimeWinRate(ctx, s.db, symbol, source, window, regime)
}

// SaveRegimeWinRate upserts a RegimeWinRateSummary keyed by
// (symbol, source, rolling_window, regime).
func SaveRegimeWinRate(ctx context.Context, db *sql.DB, summary RegimeWinRateSummary) error {
	if summary.Symbol == "" {
		return fmt.Errorf("stockpicker: regime win rate has empty symbol")
	}
	if summary.Source == "" {
		return fmt.Errorf("stockpicker: regime win rate has empty source")
	}
	if summary.Window == "" {
		return fmt.Errorf("stockpicker: regime win rate has empty window")
	}
	if summary.Regime == "" {
		return fmt.Errorf("stockpicker: regime win rate has empty regime")
	}
	if !validCalibrationStatus(summary.CalibrationStatus) {
		return fmt.Errorf("stockpicker: invalid calibration_status %q (want calibrating/eligible/degraded)", summary.CalibrationStatus)
	}

	updatedAt := summary.UpdatedAt
	if updatedAt == "" {
		updatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO stock_win_rate_by_regime
			(symbol, source, rolling_window, regime, observations, hits, win_rate, wilson_lower, wilson_upper,
			 confidence, calibration_status, net_cost_rate, avg_forward_return, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		ON CONFLICT(symbol, source, rolling_window, regime) DO UPDATE SET
			observations = excluded.observations,
			hits = excluded.hits,
			win_rate = excluded.win_rate,
			wilson_lower = excluded.wilson_lower,
			wilson_upper = excluded.wilson_upper,
			confidence = excluded.confidence,
			calibration_status = excluded.calibration_status,
			net_cost_rate = excluded.net_cost_rate,
			avg_forward_return = excluded.avg_forward_return,
			updated_at = excluded.updated_at`,
		summary.Symbol,
		summary.Source,
		summary.Window,
		summary.Regime,
		summary.Observations,
		summary.Hits,
		summary.WinRate,
		summary.WilsonLower,
		summary.WilsonUpper,
		summary.Confidence,
		string(summary.CalibrationStatus),
		summary.NetCostRate,
		summary.AvgForwardReturn,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("stockpicker: upsert regime win rate symbol=%s source=%s window=%s regime=%s: %w",
			summary.Symbol, summary.Source, summary.Window, summary.Regime, err)
	}
	return nil
}

// LoadRegimeWinRate reads the RegimeWinRateSummary for a given
// (symbol, source, window, regime).
func LoadRegimeWinRate(ctx context.Context, db *sql.DB, symbol, source, window, regime string) (RegimeWinRateSummary, bool, error) {
	var summary RegimeWinRateSummary
	var status string

	err := db.QueryRowContext(ctx, `
		SELECT symbol, source, rolling_window, regime, observations, hits, win_rate, wilson_lower, wilson_upper,
		       confidence, calibration_status, net_cost_rate, avg_forward_return, updated_at
		FROM stock_win_rate_by_regime
		WHERE symbol = $1 AND source = $2 AND rolling_window = $3 AND regime = $4`,
		symbol, source, window, regime,
	).Scan(
		&summary.Symbol,
		&summary.Source,
		&summary.Window,
		&summary.Regime,
		&summary.Observations,
		&summary.Hits,
		&summary.WinRate,
		&summary.WilsonLower,
		&summary.WilsonUpper,
		&summary.Confidence,
		&status,
		&summary.NetCostRate,
		&summary.AvgForwardReturn,
		&summary.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return RegimeWinRateSummary{}, false, nil
	}
	if err != nil {
		return RegimeWinRateSummary{}, false, fmt.Errorf("stockpicker: load regime win rate: %w", err)
	}
	summary.CalibrationStatus = CalibrationStatus(status)
	return summary, true, nil
}
