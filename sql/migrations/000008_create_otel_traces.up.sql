-- OpenTelemetry traces storage for OTel collector postgresql exporter.
-- Schema matches opentelemetry-collector-contrib postgresql exporter v0.107.0+.
-- Converted to TimescaleDB hypertable for efficient time-range queries.

CREATE TABLE IF NOT EXISTS otel_traces (
    timestamp TIMESTAMPTZ NOT NULL,
    trace_id BYTEA NOT NULL,
    span_id BYTEA NOT NULL,
    parent_span_id BYTEA,
    trace_state VARCHAR(256),
    span_name TEXT NOT NULL,
    span_kind SMALLINT,
    service_name TEXT NOT NULL,
    resource_attributes JSONB,
    span_attributes JSONB,
    duration_ns BIGINT NOT NULL,
    status_code SMALLINT,
    status_message TEXT,
    events JSONB,
    links JSONB
);

SELECT create_hypertable('otel_traces', 'timestamp', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS otel_traces_trace_id_idx ON otel_traces (trace_id);
CREATE INDEX IF NOT EXISTS otel_traces_service_name_idx ON otel_traces (service_name);