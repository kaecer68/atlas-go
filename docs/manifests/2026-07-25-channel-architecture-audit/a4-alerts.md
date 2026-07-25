# a4 — Alerts Architecture Audit (2026-07-25)

> Phase A scout for channel-architecture-audit. Read-only. Sources cited at file:line.

## 1. Where alerts are created

### Prometheus rules (`monitoring/rules/*.yml`)
- `monitoring/rules/wave9_channel_individual_health.yml:4` — `ChannelHighErrorRatePerChannel`
- `monitoring/rules/atlas_startup_anomaly_alerts.yml:26` — `AtlasDBInitFailure`, `AtlasChannelUsYahooErrorSustained`
- `monitoring/rules/llm_annotator_alerts.yml` — 9 alerts (`LLMAnnotatorHighErrorRate{Fast,Medium,Slow}Burn`, `LLMAnnotatorCircuitBreakerOpen`, `LLMAnnotator{RateLimited,TransportErrors,ProtocolErrors,RetryExhausted}`, `LLMAnnotatorNoTraffic`)
- `monitoring/rules/llm_annotator_recording.yml` — recording rules (no alerts, but feeds the alerts above)
- `monitoring/rules/disabled/` — 5 alerts (`RegimeChangeConfirmedSpike`, `FactorWeightRegressionHigh`, `PortfolioConcentrationDrift`, `PortfolioTurnoverDrift`, `IngestionLagP99High`) all flagged `enabled: "false"`

### In-process Go emitter
- `internal/monitoring/monitor.go:84` — `Monitor.Alert(level, category, message, metadata)` is the single funnel for all in-process alerts. Convenience wrappers at `monitor.go:204-221` (`Info/Warning/Error/Critical`) and `monitor.go:331-355` (`CompositeMonitor`).
- `internal/monitoring/stage3_rules.go:84` — `Stage3AlertEvaluator` (6 rules): `data-staleness-critical`, `data-staleness-warning`, `event-calendar-sparse`, `model-confidence-degraded`, `prediction-drift`, `prediction-drift-insufficient-history`.
- `internal/monitoring/health.go:70` — `HealthChecker.checkGateway` only increments `atlas_channel_health_errors_total` (does NOT create an in-process Alert).
- `internal/bootstrap/bootstrapper.go:47` — `RecordDBInitFailure(collector)` increments `atlas_db_init_failures_total`.
- `internal/monitoring/startup_metrics.go:80` — `RecordStage3AlertFired` increments `atlas_stage3_alerts_fired_total`.
- Domain callers in `cmd/atlas/`:
  - `cmd/atlas/background_tasks.go:28` — `background_task`
  - `cmd/atlas/operations_tasks.go:177` — `data_staleness` (fundamentals age > 90 days)
  - `cmd/atlas/main.go:960, 1011, 1200, 1229, 1479, 1496` — categories `simulation`, `crossmarket`, `etf_nav`, `universe_coverage`, `universe_watchlist`

### Outbound / inbound to Alertmanager
- `internal/alerting/webhook_publisher.go:49` — `WebhookPublisher.PublishAnomaly` serializes an `AnomalyEvent` to the Alertmanager webhook schema with `alertname=mcp_anomaly_detected` and POSTs to configured URL.
- `internal/alerting/webhook_handler.go:56` — `AlertWebhookHandler` (mounted at `/api/v1/alerts` per `cmd/atlas/main.go:891-893`) ingests Alertmanager webhook payloads into an in-memory ring buffer (cap 1000).

### MCP tools
- `cmd/atlas-mcp/server/tools_risk_alert.go:59-72` — 3 read-only tools: `alert_list`, `alert_get_stats`, `alert_get_rules`. Phase 1 deliberately excludes ack/resolve (per `auto-desc.gen.go` doc strings: "alert_acknowledge / alert_resolve … remain out of Phase 1 scope").

## 2. Where alerts are stored

| Layer | Path / Table | Owner |
|---|---|---|
| JSONL (default) | `${WorkDir}/data/state/alerts/alerts.jsonl` | `internal/monitoring/alert_store.go` (`AlertStore`) |
| Postgres | `alerts` table (18 columns incl. lifecycle fields) | `internal/repository/postgres_alerts.go` (`PostgresRepository`) |
| In-memory ring buffer | `AlertWebhookHandler.store` (cap 1000) | `internal/alerting/webhook_handler.go` |
| External | Alertmanager (outbound webhook target + inbound via `/api/v1/alerts`) | `internal/alerting/webhook_publisher.go` |
| Optional outbound sinks | `WebhookNotifier`, `TelegramNotifier`, `EmailNotifier`, `MultiNotifier` | `internal/monitoring/notifier.go`, `email_notifier.go` |

`AlertStore.Save` → `domain.AlertRecord` (JSONL append). `DualWriteRepository.LoadAllAlerts` / `LoadUnacknowledgedAlerts` (`internal/repository/dual_write.go:198, 208`) prefer Postgres, fall back to JSONL.

## 3. Lifecycle

Status enum (`internal/domain/types.go:132-139`): `triggered | acknowledged | resolved | silenced`.

Path in production (`internal/monitoring/monitor.go:92-202`):
1. `Monitor.Alert` creates `domain.AlertRecord{Status: AlertStatusTriggered}`.
2. Dedup check (`AlertDeduplicator` — `internal/monitoring/dedup.go`) — within 5-min window, increments `Count` + updates `LastSeen` instead of creating a new row.
3. `AlertStore.Save` (or via DualWrite → Postgres).
4. `AutoHandler.Handle`:
   - INFO → auto-acknowledge (`AlertStore.Acknowledge(alertID, "auto-handler")`).
   - non-INFO + suppressed → drop.
   - non-INFO + not suppressed → noop (stays `triggered` until user acks).
5. Notifiers fan-out asynchronously (only if configured).

Transitions reachable via HTTP (`internal/monitoring/alert_api.go`):
- `POST /api/alerts/acknowledge` → `Acknowledged` (sets `AcknowledgedAt`, `AcknowledgedBy`, `AcknowledgedWithinSec`).
- `POST /api/alerts/acknowledge-bulk` → bulk ack.
- `POST /api/alerts/resolve` → `Resolved` (sets `ResolvedAt`, `ResolvedBy`).
- `POST /api/alerts/silence` → returns `silenced_until` but does NOT mutate the store (acknowledged gap — see "dead" section).

`AutoHandler.Recover(category)` (`internal/monitoring/autohandler.go:54`) bulk-resolves triggered alerts in a category when called.

Effective lifecycle: **ack-resolved for non-INFO** (with auto-ack for INFO). Fire-and-forget for INFO at the API layer.

## 4. Dead / dangling alerts

### Disabled Prometheus rules referencing never-emitted metrics
All five rules under `monitoring/rules/disabled/` reference metrics that are not emitted anywhere in the repo (grep across `internal/` and `cmd/` returned 0 matches):

| Alert name | Rule file | Metric missing |
|---|---|---|
| `RegimeChangeConfirmedSpike` | `disabled/wave9_regime_change_confirmed.yml` | `regime_change_confirmed_total` |
| `FactorWeightRegressionHigh` | `disabled/wave9_factor_weight_regression.yml` | `factor_weight_regression_score` |
| `PortfolioConcentrationDrift` | `disabled/wave9_drift_detected.yml` | `portfolio_max_concentration` |
| `PortfolioTurnoverDrift` | `disabled/wave9_drift_detected.yml` | `portfolio_turnover_5m` |
| `IngestionLagP99High` | `disabled/wave9_ingestion_lag_spike.yml` | `ingestion_latency_seconds_bucket` |

### LLM Annotator rule suite
`monitoring/rules/llm_annotator_alerts.yml` references `llm_annotator_requests_total{outcome=...}` which is **never emitted in this repo** (zero matches). It is expected to come from the external LLM annotator service. Strictly speaking integration-dependent rather than dangling — but if that service is not deployed, all 9 LLM rules are dead.

### Stage 3 alerts
The 6 Stage 3 rule IDs emit through `Monitor.Alert` (in-process), but `atlas_stage3_alerts_fired_total` (counter, `internal/monitoring/startup_metrics.go:37`) is emitted by `RecordStage3AlertFired` with no Prometheus rule consuming it. The Stage3AlertDeps wiring in `cmd/atlas/operations_tasks.go` and `cmd/atlas/main.go` looks complete; the metric is observed but no alert fires on it.

### `/api/alerts/silence` returns but does not persist
`internal/monitoring/alert_api.go` `handleSilence` computes `silenced_until` and returns it, but the implementation does not persist a suppression or update `AlertRecord.SilencedUntil`. Docstring acknowledges: "The actual suppression is wired via suppress_categories in parameters.json (separate PR scope)."

### MCP `alert_list_unacknowledged`
Documented in the harness as `mcp__atlas_mcp_alert_list_unacknowledged` and referenced from `auto-desc.gen.go` (e.g. `alert_list` description: "Companion to alert_list_unacknowledged (Phase 1) which is unack-only"). **Not registered** in `cmd/atlas-mcp/server/tools_risk_alert.go` — only `alert_list`, `alert_get_stats`, `alert_get_rules` are present. Handler `handleAlertUnacknowledged` is absent.

## 5. Naming consistency

| Domain | Rule ID style | Example |
|---|---|---|
| Prometheus rules | CamelCase alert name | `ChannelHighErrorRatePerChannel`, `AtlasDBInitFailure`, `LLMAnnotatorCircuitBreakerOpen` |
| Go Monitor category | snake_case short string | `simulation`, `etf_nav`, `background_task`, `data_staleness`, `universe_coverage` |
| Stage 3 rule IDs | kebab-case snake | `data-staleness-critical`, `prediction-drift`, `model-confidence-degraded` |
| MCP anomaly | snake_case UPPER labels | `mcp_anomaly_detected`, `tenant_id`, `anomaly_type` |

Severity is unified via `AlertLevel.String()` (`monitor.go:24-37`): `INFO/WARNING/ERROR/CRITICAL` uppercase. Prometheus rules use lowercase `severity: info|warning|critical` (no `error`). The `alert_api.go handleStats` switch accepts `"INFO|WARNING|ERROR|CRITICAL"` but the comment in the code uses uppercase — consistent within Go, divergent from Prometheus label convention.

Dedup key construction (`monitor.go:148`): `category + ":" + level.String()`. Prometheus uses `alertname` label. No shared identifier between the two pipelines.

## 6. Agent-startup alert scanning

**None.** No code path scans the AlertStore on process startup to re-surface existing `triggered` / `acknowledged` / `resolved` alerts. Searched for `ScanAlertsOnStartup`, `RecoverAlerts`, `ResurrectAlerts`, `ResumeAlert`, `RestoreAlertState`, `agent_startup_alert` — zero matches. `DualWriteRepository.LoadAllAlerts` / `LoadUnacknowledgedAlerts` exist (`internal/repository/dual_write.go:198, 208`) but no caller in `cmd/atlas/` invokes them at boot. Consequence: a restart leaves the operator blind to in-flight alerts until something emits a new one.

## Fragmentation score: HIGH

Three independent alert pipelines (Prometheus rules, in-process Monitor→AlertStore, Alertmanager webhooks) with overlapping scope, divergent naming, divergent severity labels, divergent lifecycle assumptions. Stage 3 emits counters with no rule consuming them; `/api/alerts/silence` is a stub; MCP layer is Phase 1 partial; no startup rescan. The disabled rules reference never-emitted metrics — they need either metric implementation or removal.