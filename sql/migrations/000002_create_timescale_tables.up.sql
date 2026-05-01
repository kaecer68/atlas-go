-- Enable TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ============================================
-- Hypertables: Time-series data
-- ============================================

-- Metrics (replaces metrics.jsonl)
CREATE TABLE IF NOT EXISTS metrics (
    time TIMESTAMPTZ NOT NULL,
    metric_name TEXT NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    agent_id TEXT,
    session_id TEXT,
    symbol TEXT,
    regime TEXT,
    metadata JSONB DEFAULT '{}'
);

-- Create hypertable for metrics (7-day chunks)
SELECT create_hypertable('metrics', 'time', chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

-- Index for common queries
CREATE INDEX IF NOT EXISTS idx_metrics_name_time ON metrics (metric_name, time DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_agent_time ON metrics (agent_id, time DESC) WHERE agent_id IS NOT NULL;

-- Recommendation Outcomes (replaces recommendation_outcomes.jsonl)
CREATE TABLE IF NOT EXISTS recommendation_outcomes (
    time TIMESTAMPTZ NOT NULL,
    session_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_layer TEXT,
    conviction REAL,
    passed_guards BOOLEAN,
    guard_reason TEXT,
    price REAL,
    metadata JSONB DEFAULT '{}'
);

-- Create hypertable for outcomes (1-day chunks for finer granularity)
SELECT create_hypertable('recommendation_outcomes', 'time', chunk_time_interval => INTERVAL '1 day', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_outcomes_session ON recommendation_outcomes (session_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_outcomes_symbol ON recommendation_outcomes (symbol, time DESC);
CREATE INDEX IF NOT EXISTS idx_outcomes_agent ON recommendation_outcomes (agent_id, time DESC);

-- Capital Flow (replaces capital_flow JSON files)
CREATE TABLE IF NOT EXISTS capital_flow (
    time TIMESTAMPTZ NOT NULL,
    channel TEXT NOT NULL, -- 'foreign', 'domestic', 'dealer'
    net_buy REAL,
    total_buy REAL,
    total_sell REAL
);

SELECT create_hypertable('capital_flow', 'time', chunk_time_interval => INTERVAL '7 days', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_capital_flow_channel ON capital_flow (channel, time DESC);

-- ============================================
-- Standard Tables: Relational data
-- ============================================

-- Alerts (replaces alerts.jsonl)
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    rule TEXT NOT NULL,
    severity TEXT NOT NULL,
    message TEXT,
    value DOUBLE PRECISION,
    threshold DOUBLE PRECISION,
    acknowledged BOOLEAN DEFAULT FALSE,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by TEXT
);

CREATE INDEX IF NOT EXISTS idx_alerts_timestamp ON alerts (timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_acknowledged ON alerts (acknowledged) WHERE NOT acknowledged;
CREATE INDEX IF NOT EXISTS idx_alerts_severity ON alerts (severity, timestamp DESC);

-- Export Statistics (replaces export/*.json files)
CREATE TABLE IF NOT EXISTS export_statistics (
    time TIMESTAMPTZ NOT NULL,
    year INT NOT NULL,
    month INT NOT NULL,
    export_total DOUBLE PRECISION,
    import_total DOUBLE PRECISION,
    trade_balance DOUBLE PRECISION
);

SELECT create_hypertable('export_statistics', 'time', chunk_time_interval => INTERVAL '1 year', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_export_year_month ON export_statistics (year, month);

-- ============================================
-- Multi-tenancy foundation (for future commercial use)
-- ============================================

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    subscription_tier TEXT DEFAULT 'free', -- 'free', 'pro', 'enterprise'
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    baseline_policy JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_workspaces_user ON workspaces (user_id);

-- ============================================
-- Compression and Retention Policies
-- ============================================

-- Enable compression on metrics (after 90 days)
ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'metric_name'
);

-- Enable compression on recommendation_outcomes (after 90 days)
ALTER TABLE recommendation_outcomes SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'session_id'
);

-- Add compression policies
SELECT add_compression_policy('metrics', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_compression_policy('recommendation_outcomes', INTERVAL '90 days', if_not_exists => TRUE);

-- Add retention policies (keep 1 year of raw data)
SELECT add_retention_policy('metrics', INTERVAL '1 year', if_not_exists => TRUE);
SELECT add_retention_policy('recommendation_outcomes', INTERVAL '1 year', if_not_exists => TRUE);
SELECT add_retention_policy('capital_flow', INTERVAL '2 years', if_not_exists => TRUE);
