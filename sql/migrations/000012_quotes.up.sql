-- ============================================
-- 000012: quotes table (PostgreSQL mirror of SQLite quotes, Stage 1 contract).
-- PostgreSQL mirror of SQLiteQuoteStore tables (sqlite_core.go quotes DDL).
-- Rationale: SQLite atlas.db holds 66,959 quote rows; StoreBackend=postgres
-- must read them from PostgreSQL (JSONL fallback only covers ~6.5k rows from
-- the ~30-day cron backfill, losing 60k+ rows — M4).
-- date is TEXT (YYYY-MM-DD) matching the SQLite string format; string
-- comparison is equivalent for date ordering.
-- ============================================

CREATE TABLE IF NOT EXISTS quotes (
    id BIGSERIAL PRIMARY KEY,
    symbol TEXT NOT NULL,
    name TEXT,
    date TEXT NOT NULL,
    open DOUBLE PRECISION,
    high DOUBLE PRECISION,
    low DOUBLE PRECISION,
    close DOUBLE PRECISION,
    volume BIGINT,
    source TEXT,
    CONSTRAINT uq_quotes_symbol_date UNIQUE (symbol, date)
);

CREATE INDEX IF NOT EXISTS idx_quotes_symbol_date ON quotes (symbol, date);
