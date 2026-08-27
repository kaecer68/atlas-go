package stockpicker

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StockWinRateSummary is the persisted win-rate aggregate for a single
// (symbol, source, window) key. It reuses the CalibrationStatus type from
// winrate.go and adds the window dimension required by the stock_win_rate
// schema. The third calibration state, CalibrationDegraded, is written by
// aggregation jobs when IS/OOS drift or overfitting is detected.
type StockWinRateSummary struct {
	Symbol            string            `json:"symbol"`
	Source            string            `json:"source"`
	Window            string            `json:"window"`
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

// WinRateStore is a thin SQLite-backed wrapper around stock_win_rate.
type WinRateStore struct {
	db *sql.DB
}

// NewWinRateStore creates a WinRateStore backed by db.
func NewWinRateStore(db *sql.DB) *WinRateStore {
	return &WinRateStore{db: db}
}

// SaveWinRate delegates to the package-level SaveWinRate function.
func (s *WinRateStore) SaveWinRate(ctx context.Context, summary StockWinRateSummary) error {
	return SaveWinRate(ctx, s.db, summary)
}

// LoadWinRate delegates to the package-level LoadWinRate function.
func (s *WinRateStore) LoadWinRate(ctx context.Context, symbol, source, window string) (StockWinRateSummary, bool, error) {
	return LoadWinRate(ctx, s.db, symbol, source, window)
}

// SaveWinRate upserts a StockWinRateSummary keyed by (symbol, source, window).
// The three calibration states (calibrating/eligible/degraded) are all
// accepted; any other value returns an error.
func SaveWinRate(ctx context.Context, db *sql.DB, summary StockWinRateSummary) error {
	if summary.Symbol == "" {
		return fmt.Errorf("stockpicker: win rate has empty symbol")
	}
	if summary.Source == "" {
		return fmt.Errorf("stockpicker: win rate has empty source")
	}
	if summary.Window == "" {
		return fmt.Errorf("stockpicker: win rate has empty window")
	}
	if !validCalibrationStatus(summary.CalibrationStatus) {
		return fmt.Errorf("stockpicker: invalid calibration_status %q (want calibrating/eligible/degraded)", summary.CalibrationStatus)
	}

	updatedAt := summary.UpdatedAt
	if updatedAt == "" {
		updatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	_, err := db.ExecContext(ctx, `
		INSERT INTO stock_win_rate
			(symbol, source, rolling_window, observations, hits, win_rate, wilson_lower, wilson_upper,
			 confidence, calibration_status, net_cost_rate, avg_forward_return, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(symbol, source, rolling_window) DO UPDATE SET
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
		return fmt.Errorf("stockpicker: upsert win rate symbol=%s source=%s window=%s: %w",
			summary.Symbol, summary.Source, summary.Window, err)
	}
	return nil
}

// LoadWinRate reads the StockWinRateSummary for a given (symbol, source, window).
// The second return value is false when no row exists; the error is nil in that case.
func LoadWinRate(ctx context.Context, db *sql.DB, symbol, source, window string) (StockWinRateSummary, bool, error) {
	var summary StockWinRateSummary
	var status string

	err := db.QueryRowContext(ctx, `
		SELECT symbol, source, rolling_window, observations, hits, win_rate, wilson_lower, wilson_upper,
		       confidence, calibration_status, net_cost_rate, avg_forward_return, updated_at
		FROM stock_win_rate
		WHERE symbol = ? AND source = ? AND rolling_window = ?`,
		symbol, source, window,
	).Scan(
		&summary.Symbol,
		&summary.Source,
		&summary.Window,
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
		return StockWinRateSummary{}, false, nil
	}
	if err != nil {
		return StockWinRateSummary{}, false, fmt.Errorf("stockpicker: load win rate: %w", err)
	}
	summary.CalibrationStatus = CalibrationStatus(status)
	return summary, true, nil
}

// validCalibrationStatus reports whether s is one of the three valid states.
func validCalibrationStatus(s CalibrationStatus) bool {
	return s == CalibrationCalibrating || s == CalibrationEligible || s == CalibrationDegraded
}
