CREATE TABLE IF NOT EXISTS task_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_type TEXT NOT NULL,
    command_name TEXT NOT NULL,
    command_args JSONB NOT NULL DEFAULT '[]',
    request_payload JSONB NOT NULL DEFAULT '{}',
    status TEXT NOT NULL,
    actor TEXT NOT NULL,
    actor_source TEXT NOT NULL DEFAULT 'web_ui',
    idempotency_key TEXT NULL,
    retry_of UUID NULL REFERENCES task_executions(id),
    parent_execution_id UUID NULL REFERENCES task_executions(id),
    experiment_id TEXT NULL,
    result_path TEXT NULL,
    baseline_version_before INT NULL,
    baseline_version_after INT NULL,
    requires_confirmation BOOLEAN NOT NULL DEFAULT FALSE,
    confirmed_at TIMESTAMPTZ NULL,
    cancel_requested_at TIMESTAMPTZ NULL,
    submitted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ NULL,
    finished_at TIMESTAMPTZ NULL,
    exit_code INT NULL,
    error_message TEXT NULL,
    summary JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_exec_status_submitted ON task_executions (status, submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_exec_type_submitted ON task_executions (task_type, submitted_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_exec_experiment ON task_executions (experiment_id);
CREATE INDEX IF NOT EXISTS idx_task_exec_retry ON task_executions (retry_of);
CREATE UNIQUE INDEX IF NOT EXISTS idx_task_exec_idempotency ON task_executions (idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS task_execution_events (
    execution_id UUID NOT NULL REFERENCES task_executions(id) ON DELETE CASCADE,
    seq BIGINT NOT NULL,
    event_type TEXT NOT NULL,
    stream TEXT NOT NULL,
    level TEXT NULL,
    message TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (execution_id, seq)
);

CREATE TABLE IF NOT EXISTS experiment_lineage (
    experiment_id TEXT PRIMARY KEY,
    execution_id UUID NOT NULL REFERENCES task_executions(id),
    parent_experiment_id TEXT NULL REFERENCES experiment_lineage(experiment_id),
    root_experiment_id TEXT NOT NULL,
    lineage_depth INT NOT NULL DEFAULT 0,
    target_agent_id TEXT NOT NULL,
    target_skill TEXT NOT NULL,
    mutation_type TEXT NOT NULL,
    brief_path TEXT NULL,
    candidate_path TEXT NULL,
    result_path TEXT NULL,
    status TEXT NOT NULL,
    git_commit_id TEXT NULL,
    params_snapshot JSONB NOT NULL DEFAULT '{}',
    result_snapshot JSONB NOT NULL DEFAULT '{}',
    baseline_value DOUBLE PRECISION NULL,
    candidate_value DOUBLE PRECISION NULL,
    improvement_value DOUBLE PRECISION NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    judged_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_lineage_parent ON experiment_lineage (parent_experiment_id);
CREATE INDEX IF NOT EXISTS idx_lineage_root ON experiment_lineage (root_experiment_id);
CREATE INDEX IF NOT EXISTS idx_lineage_target ON experiment_lineage (target_agent_id, target_skill);

CREATE TABLE IF NOT EXISTS baseline_history (
    id BIGSERIAL PRIMARY KEY,
    execution_id UUID NULL REFERENCES task_executions(id),
    experiment_id TEXT NULL,
    version_before INT NOT NULL,
    version_after INT NOT NULL,
    promoted_by TEXT NOT NULL,
    promoted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    baseline_path TEXT NOT NULL,
    diff_summary JSONB NOT NULL DEFAULT '{}',
    diff_patch TEXT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_baseline_history_exp ON baseline_history (experiment_id);
CREATE INDEX IF NOT EXISTS idx_baseline_history_time ON baseline_history (promoted_at DESC);

CREATE TABLE IF NOT EXISTS metric_trends (
    id BIGSERIAL PRIMARY KEY,
    execution_id UUID NOT NULL REFERENCES task_executions(id),
    experiment_id TEXT NULL REFERENCES experiment_lineage(experiment_id),
    series_key TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    metric_scope TEXT NOT NULL,
    metric_value DOUBLE PRECISION NOT NULL,
    baseline_value DOUBLE PRECISION NULL,
    delta_value DOUBLE PRECISION NULL,
    sampled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    tags JSONB NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_metric_trends_exp ON metric_trends (experiment_id, metric_name, sampled_at DESC);
CREATE INDEX IF NOT EXISTS idx_metric_trends_series ON metric_trends (series_key, metric_name, sampled_at DESC);
