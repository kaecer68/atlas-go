package stockpicker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// stock_win_rate.calibration_status 三態。
//
// 刻意用 string 常數而非 winrate.go 的 CalibrationStatus 型別：儲存層新增
// 第三態 "degraded"（由未來的聚合 job 在 IS/OOS 背離或 OverfitWarning 時
// 寫入），而純數學的 CalibrationStatusFor 簽名不改（只產出
// calibrating/eligible）。兩者分離避免為了第三態去動 PR 1a 的純函數。
const (
	// WinRateCalibrating 樣本不足，僅供觀察。
	WinRateCalibrating = "calibrating"
	// WinRateEligible 樣本充足，可參考。
	WinRateEligible = "eligible"
	// WinRateDegraded 校準退化（IS/OOS 背離），由聚合 job 寫入。
	WinRateDegraded = "degraded"
)

// StockWinRateSummary 是 stock_win_rate 的一列（per symbol/source/window 聚合）。
type StockWinRateSummary struct {
	Symbol            string
	Source            string
	Window            string
	Observations      int
	Hits              int
	WinRate           float64
	WilsonLower       float64
	WilsonUpper       float64
	Confidence        float64
	CalibrationStatus string // calibrating | eligible | degraded
	NetCostRate       float64
	AvgForwardReturn  float64
	UpdatedAt         string
}

// winRateSchema 是 stock_win_rate 的 SQLite DDL。
// 與 sql/migrations/000019_stock_win_rate.up.sql（PostgreSQL mirror）一致。
const winRateSchema = `
CREATE TABLE IF NOT EXISTS stock_win_rate (
	symbol TEXT NOT NULL,
	source TEXT NOT NULL,
	window TEXT NOT NULL,
	observations INTEGER NOT NULL,
	hits INTEGER NOT NULL,
	win_rate REAL NOT NULL,
	wilson_lower REAL,
	wilson_upper REAL,
	confidence REAL,
	calibration_status TEXT NOT NULL,
	net_cost_rate REAL,
	avg_forward_return REAL,
	updated_at TEXT NOT NULL,
	UNIQUE(symbol, source, window)
);
`

// ensureWinRateSchema 在 SQLite 上建立 stock_win_rate 表（冪等）。
func ensureWinRateSchema(db *sql.DB) error {
	if _, err := db.Exec(winRateSchema); err != nil {
		return fmt.Errorf("stockpicker: ensure stock_win_rate schema: %w", err)
	}
	return nil
}

// WinRateStore 是 stock_win_rate 的 SQLite 儲存層薄包裝。
type WinRateStore struct {
	db *sql.DB
}

// NewWinRateStore 建立 WinRateStore 並確保底層表存在。
func NewWinRateStore(db *sql.DB) (*WinRateStore, error) {
	if err := ensureWinRateSchema(db); err != nil {
		return nil, err
	}
	return &WinRateStore{db: db}, nil
}

// SaveWinRate upsert 一列聚合勝率（見 package-level 版本）。
func (s *WinRateStore) SaveWinRate(ctx context.Context, summary StockWinRateSummary) error {
	return SaveWinRate(ctx, s.db, summary)
}

// LoadWinRate 依唯一鍵 (symbol, source, window) 讀取聚合勝率（見 package-level 版本）。
func (s *WinRateStore) LoadWinRate(ctx context.Context, symbol, source, window string) (StockWinRateSummary, bool, error) {
	return LoadWinRate(ctx, s.db, symbol, source, window)
}

// SaveWinRate 以 (symbol, source, window) 為唯一鍵 upsert 一列聚合勝率。
//
// calibration_status 三態（calibrating/eligible/degraded）皆可寫入；空值或
// 三態以外的值會回傳錯誤，防止非法校準態污染快查層。updated_at 為空時以
// UTC 現在時間填補。
func SaveWinRate(ctx context.Context, db *sql.DB, summary StockWinRateSummary) error {
	if err := ensureWinRateSchema(db); err != nil {
		return err
	}
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
			(symbol, source, window, observations, hits, win_rate, wilson_lower, wilson_upper,
			 confidence, calibration_status, net_cost_rate, avg_forward_return, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(symbol, source, window) DO UPDATE SET
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
		summary.Symbol, summary.Source, summary.Window,
		summary.Observations, summary.Hits, summary.WinRate,
		summary.WilsonLower, summary.WilsonUpper, summary.Confidence,
		summary.CalibrationStatus, summary.NetCostRate, summary.AvgForwardReturn,
		updatedAt)
	if err != nil {
		return fmt.Errorf("stockpicker: upsert win rate symbol=%s source=%s window=%s: %w",
			summary.Symbol, summary.Source, summary.Window, err)
	}
	return nil
}

// LoadWinRate 依唯一鍵 (symbol, source, window) 讀取聚合勝率。
// 找不到時回傳零值 summary、found=false、nil error。
func LoadWinRate(ctx context.Context, db *sql.DB, symbol, source, window string) (StockWinRateSummary, bool, error) {
	if err := ensureWinRateSchema(db); err != nil {
		return StockWinRateSummary{}, false, err
	}

	row := db.QueryRowContext(ctx, `
		SELECT symbol, source, window, observations, hits, win_rate, wilson_lower, wilson_upper,
		       confidence, calibration_status, net_cost_rate, avg_forward_return, updated_at
		FROM stock_win_rate
		WHERE symbol = ? AND source = ? AND window = ?`,
		symbol, source, window)

	var (
		summary          StockWinRateSummary
		wilsonLower      sql.NullFloat64
		wilsonUpper      sql.NullFloat64
		confidence       sql.NullFloat64
		netCostRate      sql.NullFloat64
		avgForwardReturn sql.NullFloat64
	)
	err := row.Scan(
		&summary.Symbol, &summary.Source, &summary.Window,
		&summary.Observations, &summary.Hits, &summary.WinRate,
		&wilsonLower, &wilsonUpper, &confidence,
		&summary.CalibrationStatus, &netCostRate, &avgForwardReturn,
		&summary.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return StockWinRateSummary{}, false, nil
	}
	if err != nil {
		return StockWinRateSummary{}, false, fmt.Errorf(
			"stockpicker: load win rate symbol=%s source=%s window=%s: %w",
			symbol, source, window, err)
	}

	summary.WilsonLower = wilsonLower.Float64
	summary.WilsonUpper = wilsonUpper.Float64
	summary.Confidence = confidence.Float64
	summary.NetCostRate = netCostRate.Float64
	summary.AvgForwardReturn = avgForwardReturn.Float64
	return summary, true, nil
}

// validCalibrationStatus 檢查 calibration_status 是否為三態之一。
func validCalibrationStatus(s string) bool {
	return s == WinRateCalibrating || s == WinRateEligible || s == WinRateDegraded
}

// aggregateWinRateWithoutConsistency 計算 SignalWinRateSummary，但不做
// SignalWinRate 的單一 symbol/source 一致性檢查。
//
// 這是 PR 1a 驗收報告 §5-1 預告問題的解法：StockWinRate / StrategyWinRate
// 目前直接委派 SignalWinRate，會繼承「所有 outcomes 必須同 symbol、同
// source」的檢查；而跨 symbol（策略層）或跨 source（股票層）的聚合必然混用
// symbol/source，直接呼叫會得到 mixed symbols/sources 錯誤。
//
// 本函式與 SignalWinRate 共用同一套純數學（NetHit / WinRate /
// WilsonScoreInterval / CalibrationStatusFor），只省略一致性檢查，因此回傳的
// summary 不填 Symbol/Source（跨維度聚合沒有單一值可填）。刻意不導出：公開
// 呼叫端仍應走 SignalWinRate / StockWinRate / StrategyWinRate。
func aggregateWinRateWithoutConsistency(outcomes []SignalOutcome, costRate float64, minSamples int, confidence float64) SignalWinRateSummary {
	var summary SignalWinRateSummary
	if len(outcomes) == 0 {
		summary.Confidence = confidence
		summary.CalibrationStatus = CalibrationStatusFor(0, minSamples)
		return summary
	}

	var totalReturn float64
	for _, o := range outcomes {
		if NetHit(o.ForwardReturn, costRate) {
			summary.Hits++
		}
		totalReturn += o.ForwardReturn
	}

	n := len(outcomes)
	summary.Observations = n
	summary.WinRate = WinRate(summary.Hits, n)
	summary.WilsonLower, summary.WilsonUpper = WilsonScoreInterval(summary.Hits, n, confidence)
	summary.Confidence = confidence
	summary.CalibrationStatus = CalibrationStatusFor(n, minSamples)
	summary.NetCostRate = costRate
	summary.AvgForwardReturn = totalReturn / float64(n)
	return summary
}
