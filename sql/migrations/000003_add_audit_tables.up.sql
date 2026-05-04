CREATE TABLE IF NOT EXISTS screening_rejects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id TEXT NOT NULL,
    symbol TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    skill TEXT,
    criterion TEXT NOT NULL,
    criterion_label TEXT,
    threshold TEXT,
    actual_value TEXT,
    factor_scores JSONB DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_screening_rejects_session ON screening_rejects (session_id, time DESC);
CREATE INDEX IF NOT EXISTS idx_screening_rejects_symbol ON screening_rejects (symbol, time DESC);
CREATE INDEX IF NOT EXISTS idx_screening_rejects_agent ON screening_rejects (agent_id, time DESC);

CREATE TABLE IF NOT EXISTS session_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id TEXT NOT NULL UNIQUE,
    regime TEXT,
    order_count INT DEFAULT 0,
    position_count INT DEFAULT 0,
    ending_cash DOUBLE PRECISION,
    portfolio_value DOUBLE PRECISION,
    outcome_count INT DEFAULT 0,
    broker_runtime JSONB DEFAULT '{}',
    next_experiment_agent_id TEXT,
    proposal_id TEXT,
    commit_id TEXT,
    approval_id TEXT,
    guard_outcomes JSONB DEFAULT '[]'
);

CREATE INDEX IF NOT EXISTS idx_session_summaries_time ON session_summaries (time DESC);
CREATE INDEX IF NOT EXISTS idx_session_summaries_proposal ON session_summaries (proposal_id) WHERE proposal_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS human_interventions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    intervention_id TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,
    target_agent_id TEXT,
    target_model_id TEXT,
    target_sector TEXT,
    target_symbol TEXT,
    value DOUBLE PRECISION,
    reason TEXT,
    operator TEXT,
    session_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_human_interventions_time ON human_interventions (time DESC);
CREATE INDEX IF NOT EXISTS idx_human_interventions_type ON human_interventions (type, time DESC);
CREATE INDEX IF NOT EXISTS idx_human_interventions_session ON human_interventions (session_id, time DESC) WHERE session_id IS NOT NULL;
