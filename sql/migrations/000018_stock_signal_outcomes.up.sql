-- ============================================
-- 000018: stock_signal_outcomes table.
--
-- Raw per-symbol signal trigger outcomes (fact layer) for the stock-picking
-- win-rate calibration pipeline. This is the PostgreSQL mirror of the SQLite
-- schema self-provisioned by internal/stockpicker/signal_outcome_store.go.
--
-- UNIQUE (symbol, trigger_date, source) is the P0-2 correction from the
-- stock-picking redesign review: one row per symbol/trigger/source so the
-- same symbol triggered by two sources on the same trading day is never
-- double-scored, and downstream CapitalAllocator dedup has a stable
-- provenance key. If data lands before this key exists, cleaning duplicate
-- rows later costs far more than writing it correctly now.
--
-- hit is derived from net_forward_return (NetHit: net return > 0). Gross
-- forward_return is kept only for display/provenance — costs enter the hit
-- definition and gross returns must not drive weights (P0-3).
-- ============================================

CREATE TABLE IF NOT EXISTS stock_signal_outcomes (
    id BIGSERIAL PRIMARY KEY,
    symbol TEXT NOT NULL,
    trigger_date TEXT NOT NULL,          -- YYYY-MM-DD (SessionID trading day)
    source TEXT NOT NULL,                -- "stockpicker-momentum" or agent id
    forward_return DOUBLE PRECISION,     -- gross holding-period return
    net_forward_return DOUBLE PRECISION, -- net-of-cost holding-period return
    hit BOOLEAN,                         -- net_forward_return > 0 (NetHit)
    cost_rate DOUBLE PRECISION,          -- cost rate used at trigger time
    regime TEXT,                         -- optional market regime
    created_at TEXT NOT NULL,            -- ISO-8601
    UNIQUE (symbol, trigger_date, source)
);
