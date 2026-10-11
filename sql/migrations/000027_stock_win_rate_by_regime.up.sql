-- 000027_stock_win_rate_by_regime.up.sql
-- Phase 1 regime-conditional gate: per-symbol, per-source, per-window,
-- per-regime win-rate strata. Same metrics as stock_win_rate (000019) plus
-- the regime dimension. Empty-regime outcomes aggregate under 'unknown'
-- (never imputed backwards).
-- NOTE: column is rolling_window, not window (window is a PostgreSQL reserved word).
CREATE TABLE IF NOT EXISTS stock_win_rate_by_regime (
    symbol TEXT NOT NULL,
    source TEXT NOT NULL,
    rolling_window TEXT NOT NULL,
    regime TEXT NOT NULL,
    observations INTEGER NOT NULL,
    hits INTEGER NOT NULL,
    win_rate DOUBLE PRECISION NOT NULL,
    wilson_lower DOUBLE PRECISION,
    wilson_upper DOUBLE PRECISION,
    confidence DOUBLE PRECISION,
    calibration_status TEXT NOT NULL,
    net_cost_rate DOUBLE PRECISION,
    avg_forward_return DOUBLE PRECISION,
    updated_at TEXT NOT NULL,
    UNIQUE(symbol, source, rolling_window, regime)
);

CREATE INDEX IF NOT EXISTS idx_stock_win_rate_by_regime_key
    ON stock_win_rate_by_regime(symbol, source, rolling_window, regime);
