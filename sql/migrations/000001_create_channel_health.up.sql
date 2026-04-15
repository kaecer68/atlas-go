CREATE TABLE IF NOT EXISTS channel_health (
    channel_id VARCHAR(64) PRIMARY KEY,
    status VARCHAR(32) NOT NULL,
    last_fetch_at TIMESTAMPTZ NOT NULL,
    last_error TEXT,
    last_success_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_health_status ON channel_health(status);
CREATE INDEX IF NOT EXISTS idx_channel_health_fetch_at ON channel_health(last_fetch_at);
