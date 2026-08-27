-- ============================================
-- 000019: stock_win_rate table.
--
-- Per-(symbol, source, window) win-rate aggregate (fast-lookup layer) fed by
-- the future aggregation job (PR 1c). PostgreSQL mirror of the SQLite schema
-- self-provisioned by internal/stockpicker/win_rate_store.go.
--
-- calibration_status has three states: "calibrating" (samples < min_samples),
-- "eligible" (enough samples) and "degraded" (reserved; written later by the
-- aggregation job on IS/OOS divergence or OverfitWarning). The pure-math
-- CalibrationStatusFor signature is NOT extended for the third state — the
-- storage layer owns the "degraded" semantics.
-- ============================================

CREATE TABLE IF NOT EXISTS stock_win_rate (
    symbol TEXT NOT NULL,
    source TEXT NOT NULL,
    window TEXT NOT NULL,               -- rolling window label, e.g. "120d"
    observations INTEGER NOT NULL,
    hits INTEGER NOT NULL,
    win_rate DOUBLE PRECISION NOT NULL,
    wilson_lower DOUBLE PRECISION,
    wilson_upper DOUBLE PRECISION,
    confidence DOUBLE PRECISION,
    calibration_status TEXT NOT NULL,   -- calibrating | eligible | degraded
    net_cost_rate DOUBLE PRECISION,
    avg_forward_return DOUBLE PRECISION,
    updated_at TEXT NOT NULL,           -- ISO-8601
    UNIQUE (symbol, source, window)
);
