# PostgreSQL + TimescaleDB Migration Plan

> **Status**: In Progress  
> **Started**: 2026-05-01  
> **Goal**: Migrate atlas-go from file-based storage (JSONL) to PostgreSQL + TimescaleDB for scalable, queryable data persistence.

---

## Current State

- PostgreSQL 15 deployed but only `channel_health` table is used
- All business data stored as JSONL/JSON files (~7,266 lines, ~494 files)
- No Repository abstraction layer
- docker-compose.yml uses `postgres:15-alpine`

## Target State

- TimescaleDB for time-series data (metrics, outcomes, sessions)
- PostgreSQL standard tables for relational data (alerts, users, workspaces)
- Repository pattern with interface + PostgreSQL implementation
- Existing JSONL as fallback/backup during transition

## Implementation Order

### Phase 1: Infrastructure (Done First)
1. ✅ Assess current state
2. 🔄 Modify docker-compose.yml → TimescaleDB
3. 🔄 Add DATABASE_URL to Config
4. 🔄 Create migration files (Schema + hypertables)

### Phase 2: Data Layer
5. 🔄 Create Repository interfaces
6. 🔄 Implement PostgreSQL Repository
7. 🔄 Implement TimescaleDB hypertable operations

### Phase 3: Migration
8. 🔄 Data migration scripts (JSONL → PostgreSQL)
9. 🔄 Dual-write mode (JSONL + PostgreSQL)
10. 🔄 Verify data consistency

### Phase 4: Cutover
11. 🔄 Update business code to use Repository
12. 🔄 Query optimization with TimescaleDB features
13. 🔄 Remove JSONL writes (optional, keep as backup)

---

## Schema Design

### Hypertables (Time-series)

```sql
-- Metrics (replaces metrics.jsonl)
CREATE TABLE metrics (
    time TIMESTAMPTZ NOT NULL,
    metric_name TEXT NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    agent_id TEXT,
    session_id TEXT,
    symbol TEXT,
    regime TEXT,
    metadata JSONB
);
SELECT create_hypertable('metrics', 'time', chunk_time_interval => INTERVAL '7 days');

-- Recommendation Outcomes (replaces recommendation_outcomes.jsonl)
CREATE TABLE recommendation_outcomes (
    time TIMESTAMPTZ NOT NULL,
    session_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    agent_layer TEXT,
    conviction REAL,
    passed_guards BOOLEAN,
    guard_reason TEXT,
    price REAL,
    metadata JSONB
);
SELECT create_hypertable('recommendation_outcomes', 'time', chunk_time_interval => INTERVAL '1 day');

-- Capital Flow (replaces capital_flow JSON files)
CREATE TABLE capital_flow (
    time TIMESTAMPTZ NOT NULL,
    channel TEXT NOT NULL,
    net_buy REAL,
    total_buy REAL,
    total_sell REAL
);
SELECT create_hypertable('capital_flow', 'time', chunk_time_interval => INTERVAL '7 days');
```

### Standard Tables

```sql
-- Alerts (replaces alerts.jsonl)
CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    timestamp TIMESTAMPTZ NOT NULL,
    rule TEXT NOT NULL,
    severity TEXT NOT NULL,
    message TEXT,
    value DOUBLE PRECISION,
    threshold DOUBLE PRECISION,
    acknowledged BOOLEAN DEFAULT FALSE,
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by TEXT
);
CREATE INDEX idx_alerts_timestamp ON alerts(timestamp DESC);
CREATE INDEX idx_alerts_acknowledged ON alerts(acknowledged);

-- Channel Health (already exists, keep compatible)
-- existing: channel_health table

-- Users (for future multi-tenancy)
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT UNIQUE NOT NULL,
    subscription_tier TEXT DEFAULT 'free',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Workspaces (for multi-tenancy)
CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID REFERENCES users(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

## File Structure

```
internal/
├── repository/
│   ├── interfaces.go          # Repository interfaces
│   ├── postgres_metrics.go    # MetricsRepository PG impl
│   ├── postgres_alerts.go     # AlertRepository PG impl
│   ├── postgres_outcomes.go   # OutcomeRepository PG impl
│   └── jsonl_fallback.go      # JSONL fallback impl
├── db/
│   └── db.go                  # Already exists (connection + migrations)
sql/migrations/
├── 000001_create_channel_health.up.sql    # Already exists
├── 000001_create_channel_health.down.sql  # Already exists
├── 000002_create_timescale_tables.up.sql
└── 000002_create_timescale_tables.down.sql
```

## Notes

- Keep JSONL as backup during transition (dual-write)
- Use `pgx` for PostgreSQL driver (already imported)
- TimescaleDB extension installed automatically in docker image
- Compression policy after 90 days
- Retention policy after 1 year (configurable)
