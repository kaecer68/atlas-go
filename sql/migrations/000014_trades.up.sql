-- ============================================
-- 000014: trades table (PostgreSQL mirror of SQLite trades).
-- PostgreSQL mirror of SQLite trades DDL (sqlite_core.go). timestamp is
-- TEXT (RFC3339) matching the SQLite string format written by
-- RecordSessionTrades; trade_id is NOT unique in the SQLite source (repeated
-- daily runs can re-insert the same trade_id), so no UNIQUE constraint here
-- mirrors that behavior.
-- ============================================

CREATE TABLE IF NOT EXISTS trades (
    id BIGSERIAL PRIMARY KEY,
    trade_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    side TEXT NOT NULL,
    quantity INTEGER NOT NULL,
    price DOUBLE PRECISION NOT NULL,
    amount DOUBLE PRECISION NOT NULL,
    reason TEXT,
    timestamp TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trades_session_id ON trades (session_id);
CREATE INDEX IF NOT EXISTS idx_trades_symbol ON trades (symbol);
