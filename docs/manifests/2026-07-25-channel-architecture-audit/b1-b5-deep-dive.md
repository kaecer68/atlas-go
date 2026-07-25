# B1–B5 Deep-Dive Findings

**Date**: 2026-07-25  
**Branch**: feat/p2-monitoring-alert-unification  
**Depends on**: `a1-channels.json`, `a2-tasks.json`, `a3-health.json`, `a4-alerts.json`, `a5-violations.json`

---

## B1. Rogue Channels — Root Cause Analysis

### Identified Rogues

| Tool | Direct Env | Duplicate Registered Channel | Why It Exists |
|------|------------|----------------------------|---------------|
| `cmd/backfill-financial-statements` | `FINMIND_API_KEY` | `finmind` | Historical CLI for bulk backfill of monthly/statement data |
| `cmd/backfill-institutional-investors` | `FINMIND_API_KEY` | `finmind` | Historical CLI for daily institutional buy/sell backfill |
| `cmd/backfill-month-revenue` | `FINMIND_API_KEY` | `finmind` | Historical CLI for monthly revenue backfill |
| `cmd/backfill-taifex-oi` | `FINMIND_API_KEY` | `taifex_institutional` / `finmind` | Historical CLI for TAIFEX OI backfill |

### Root Causes

1. **Backfill is treated as a separate concern from runtime fetch.** Backfill tools were created as standalone CLIs to load historical data once, not as recurring tasks. They predate or ignored the `apigateway` registry.
2. **FinMind API is discontinued (402).** The Constitution already notes this. The rogues still depend on FinMind, so they are not only rogue but also **likely broken**.
3. **No CI gate blocks new rogue channels.** `scripts/ci/check_constitution.sh` only warns and exits 0.
4. **One-shot CLIs bypass `BackgroundTaskManager`**, so they never write channel health records, never trigger alerts, and never appear in monitoring dashboards.

### Broader Bypass Candidate

- `cmd/backfill-replay/main.go:126,242-250` uses a raw `http.Client`. It builds `backfill-replay` in `Dockerfile.cron` but has no cron container, so it is effectively dead code or a manual-only tool.

---

## B2. Fragmented Data Sources — Same Source, Multiple Channels

### Yahoo Family Fragmentation

| Source Endpoint | Registered Channel IDs | Count | Problem |
|-----------------|------------------------|-------|---------|
| `query1.finance.yahoo.com` | `us_yahoo`, `sox_index`, `dram_spot_price`, `us_spx`, `us_ndx`, `us_dji`, `taiex_index`, `tw_vol`, `us_nvda`, `us_aapl`, `us_msft`, `tsm_adr` | 12 | 12 IDs share one HTTP endpoint but have no independent BTM fetch tasks; all pulled inside `macro_ingest` closure |

**Impact**: Health status is reported per channel, but the actual HTTP fetch is a single batch call. A failure at the shared endpoint produces 12 "channel errors" in health store, but the Wave9 synthesizer / Prometheus rule only sees aggregate `atlas_channel_health_errors_total` without clear indication that they share one source.

### FinMind Fragmentation

| Data Domain | Registered Channel | Rogue Duplicate |
|-------------|-------------------|-----------------|
| Taiwan stock financial statements | `finmind` | `backfill-financial-statements` |
| Institutional buy/sell | `finmind` | `backfill-institutional-investors` |
| Monthly revenue | `finmind` | `backfill-month-revenue` |
| TAIFEX OI | `taifex_institutional` | `backfill-taifex-oi` |

**Impact**: Four backfill binaries maintain their own rate limiting, retry logic, and file formats, duplicating the adapter logic already in `internal/apigateway/adapter_finmind.go` / `adapter_taifex_institutional.go`.

### TWSE Replay Fragmentation

| Data | Channel | Tasks |
|------|---------|-------|
| TWSE daily quotes / replay | `twse_replay` | `etf_nav_refresh`, `auto_backfill`, `channel_health_twse_replay` |

`twse_replay` has 3 BTM tasks. They likely overlap on the same source data but run on different schedules (24h / 24h / 1h). Without a unified fetch-and-share cache, the same CSV/JSON file is fetched multiple times per day.

---

## B3. Scheduling & Monitoring Coverage Gaps

### Scheduling Gaps

| Gap | Evidence | Risk |
|-----|----------|------|
| 14 Yahoo-derived channels have no dedicated BTM task | `a2-tasks.md` §8 | Data only fetched when `macro_ingest` happens to include them; no explicit schedule per channel |
| Stub channels (`twse_sbl`, `tdcc_equity_dispersion`) registered but never fetch | `a1-channels.json` | Health shows `inactive` permanently; consumes registry/health scan overhead |
| 75 one-shot CLIs are not scheduled | `a2-tasks.json` | Historical backfills require manual or external cron orchestration |
| `backfill-replay` built but not scheduled | `a2-tasks.md` §3 | Dead artifact in `Dockerfile.cron` |

### Monitoring Gaps

| Gap | Evidence | Risk |
|-----|----------|------|
| Only ~12 of 80 BTM tasks set `ChannelID` and use `gateway.Fetch` | `a2-tasks.md` §8 | 64 tasks bypass breaker, rate limit, and FetchResult health metadata |
| `/api/health` hardcoded `status=ok` | `a3-health.json` | Operator believes system healthy even when subsystems are down |
| `/api/health/aggregate` only checks `channel_health.json` mtime, not actual channel state | `a3-health.json` | Stale file with fresh mtime masks failed fetches |
| Wave9 synthesizer skips `warn`/`error` without `LastError` | `a3-health.json` | Silent data degradation |
| Grafana System Health has no per-channel panels | `a3-health.json` | Visual blind spot |
| Prometheus only one per-channel alert rule | `a3-health.json` | No latency / staleness / fallback alerts |

---

## B4. Alert Fragmentation & Management Debt

### Three Independent Alert Pipelines

| Pipeline | Source | Storage | Lifecycle | Naming Style |
|----------|--------|---------|-----------|--------------|
| Prometheus rules | `monitoring/rules/*.yml` | Alertmanager (external) | External | CamelCase alertname, lowercase severity |
| In-process Monitor→AlertStore | `internal/monitoring/monitor.go` | JSONL / Postgres / in-memory ring | triggered → ack → resolved | snake_case category, UPPER severity |
| Alertmanager webhooks | `internal/alerting/*.go` | In-memory ring buffer (cap 1000) | None | `mcp_anomaly_detected` |

### Why Alerts Are "Never Found or Managed"

1. **Auto-ack for INFO.** `AutoHandler` immediately acknowledges INFO-level alerts, so they never appear in `/api/alerts/unacknowledged`.
2. **No startup rescan.** After a process restart, existing triggered alerts are not reloaded. New alerts only appear when emitted again.
3. **MCP is read-only Phase 1.** `alert_acknowledge` / `alert_resolve` / `alert_silence` exist as HTTP APIs but not as MCP tools, so AI agents cannot manage them through the canonical interface.
4. **Silence endpoint is a stub.** `POST /api/alerts/silence` returns `silenced_until` but does not persist it.
5. **Dead rules are never pruned.** Disabled rules reference metrics that are not emitted; they accumulated under `monitoring/rules/disabled/` without removal plan until removed 2026-07-25.

---

## B5. Missing Historical Data — Root Cause

### Why 2020–2024 Data Is Still Incomplete

1. **Backfill is not integrated into unified scheduling.** Backfill binaries are one-shot CLIs triggered manually or by external cron. There is no BTM task that says "backfill any gap older than N days."
2. **FinMind API discontinued.** The Constitution notes FinMind returned 402. Rogues still point at FinMind, so historical backfills through them fail.
3. **No gap-detection task.** No task scans existing data, compares desired range (e.g., 2020–2024), and schedules missing dates.
4. **Data conversion is ad-hoc.** Each backfill tool writes its own format; there is no shared importer/validator that normalizes into the canonical domain types.
5. **No ownership tracking.** It is unclear which channel "owns" a given data type, so backfill responsibilities are scattered.

### Evidence

- `cmd/backfill-*` tools use `FINMIND_API_KEY` (rogue, API likely dead).
- `Dockerfile.cron` builds `backfill-replay` but no container runs it.
- `a5-violations.json` shows 26 `os.Getenv` callsites, many in backfill/cron tools.
- `a2-tasks.json` shows no BTM task named `auto_backfill_gap` or similar.

---

## Key Inferences (flagged)

- `[INFERENCE]` Some of the 16 unregistered tickers are justified (WebSocket keep-alive, live broker mode), but they are not centrally registered, so a future audit cannot distinguish legitimate from accidental goroutines.
- `[INFERENCE]` The 14 Yahoo channels may be intentionally split for health granularity, but the lack of per-channel BTM tasks means the granularity is misleading.
- `[INFERENCE]` Dead Prometheus rules were likely disabled because the underlying metrics were never wired; they should be removed or the metrics should be implemented.
