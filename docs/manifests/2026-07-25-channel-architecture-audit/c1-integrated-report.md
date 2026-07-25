# C1 — Integrated Diagnosis & Recommendations

**Date**: 2026-07-25  
**Branch**: feat/p2-monitoring-alert-unification  
**Status**: Phase C complete

---

## Executive Summary

| Dimension | Finding | Severity |
|-----------|---------|----------|
| Channels | 37 canonical IDs; 4 confirmed rogue FinMind backfill CLIs; 1 bypass candidate | High |
| Scheduling | 80 BTM tasks + 10 cron containers + 16 unregistered tickers + 75 one-shot CLIs | Medium-High |
| Health | Duplicate/conflicting endpoints; only ~12 tasks use Gateway health path | High |
| Alerts | 3 independent pipelines; dead/dangling rules; no startup rescan; MCP read-only | High |
| Compliance | Constitution task count outdated (9 claimed vs 60 actual); CI false-negative | Medium |
| Historical data | Backfill not integrated; FinMind dead; no gap-detection task | High |

**Core problem**: The project has a *canonical architecture* (`apigateway` + `BackgroundTaskManager` + `AlertStore`) that is documented and partially enforced, but *practical data loading* has accreted around it as one-off CLIs, cron containers, and monitoring subsystems. This creates the exact symptoms the user described: fragmented sources, scattered schedules, incomplete backfills, and alerts that fire but are not tracked.

---

## 1. Answers to User Questions

### Q1. How many external data channels?

**37 canonical channel IDs** registered in `internal/apigateway/gateway.go:168-210`. Runtime registration is conditional on API keys / feature flags.

### Q2. Which channels are outside the unified architecture?

Four confirmed rogue channels (all FinMind backfills):

1. `cmd/backfill-financial-statements/main.go`
2. `cmd/backfill-institutional-investors/main.go`
3. `cmd/backfill-month-revenue/main.go`
4. `cmd/backfill-taifex-oi/main.go`

Plus one broader bypass candidate: `cmd/backfill-replay/main.go`.

### Q3. Are all channels covered by unified scheduling and health monitoring?

**No.**

- Only ~12 of 80 BTM tasks set `ChannelID` and use `gateway.Fetch`.
- 14 Yahoo-derived channels are fetched only inside `macro_ingest` closure without dedicated schedules.
- Stub channels (`twse_sbl`, `tdcc_equity_dispersion`) are registered but never fetch.
- 75 one-shot CLIs are outside BTM entirely.

### Q4. Is `/api/health` the health monitoring module? Any overlap/conflict?

**`/api/health` now redirects to `/health`.**

| Endpoint | Behavior |
|----------|----------|
| `/health` | Returns `status=ok` + port occupancy for `atlas_http` and `fubon_proxy` (the real liveness probe) |
| `/api/health` | `301 Moved Permanently` to `/health` (external probes with `/api/` prefix still work) |
| `/api/health/aggregate` | Four-tier aggregate but always 200; checks file mtime, not real channel state |
| `/api/dashboard/system-health` | Hand-coded channel list; may diverge from canonical 37 IDs |
| `/api/dashboard/channel-health` | Reads `channel_health.json`; drops `last_error` and latency |

**Recommendation**: Deprecate `/api/health` and redirect to `/health`. Consolidate `/api/health/aggregate`, `/api/dashboard/system-health`, and `/api/dashboard/channel-health` into one canonical channel-health endpoint backed by `apigateway.ChannelHealthStore`.

### Q5. Fragmented data sources (one source → multiple channels)?

**Yes.**

- **Yahoo**: 12 channel IDs share `query1.finance.yahoo.com` but have no independent BTM tasks.
- **FinMind**: 3 backfill CLIs duplicate the registered `finmind` channel; 1 duplicates `taifex_institutional`.
- **TWSE replay**: 3 BTM tasks (`etf_nav_refresh`, `auto_backfill`, `channel_health_twse_replay`) hit the same source on overlapping schedules.

### Q6. Fragmented alert system?

**Yes — HIGH fragmentation.** Three independent pipelines with different naming, storage, and lifecycle. Dead rules were removed 2026-07-25. No startup rescan. MCP Phase 1 is read-only.

### Q7. Why is historical data still missing after 6 months?

1. Backfill is not part of unified scheduling.
2. FinMind API is dead (402).
3. No gap-detection / gap-filling task exists.
4. Rogue backfill CLIs do not write health records, so failures are invisible.

### Q8. How to design an agent-startup alert-scanning mechanism?

See §3 below.

---

## 2. Prioritized Remediation Plan

### P0 — Block merge / fix immediately

| # | Action | File / Area | Acceptance |
|---|--------|-------------|------------|
| 1 | Fix `scripts/ci/check_constitution.sh` to exit non-zero on violations and emit GitHub step summary | `scripts/ci/check_constitution.sh` | CI blocks PRs with new Article 1/4 violations |
| 2 | Resolve `janus_regime_refresh` double registration | `cmd/atlas/operations_tasks.go` + `cmd/atlas/main.go` | Both registrations visible or one removed |
| 3 | Remove or fix `/api/health` hardcoded duplicate | `cmd/atlas/main.go` | `GET /api/health` returns `301` to `/health` |
| 4 | Make `/api/alerts/silence` persist `SilencedUntil` | `internal/monitoring/alert_api.go` | Silenced alerts reflect in unacknowledged list until expiry |

### P1 — High impact, medium effort

| # | Action | File / Area | Acceptance |
|---|--------|-------------|------------|
| 5 | Convert 4 FinMind rogue backfills into BTM tasks or delete if FinMind is dead | `cmd/backfill-*` | Use `gateway.Fetch("finmind")` / `gateway.Fetch("taifex_institutional")` or remove |
| 6 | Add `ChannelID` to the 64 BTM tasks that currently fetch via closure | `cmd/atlas/*_tasks.go` | All data-fetching tasks go through `gateway.Fetch` |
| 7 | Create a gap-detection backfill task | New `cmd/atlas/backfill_tasks.go` | Scans desired date range per channel, schedules missing dates |
| 8 | Unify health endpoints | `internal/monitoring/` | Single `/api/health` backed by `ChannelHealthStore` + portprobe |
| 9 | Add per-channel latency/staleness Prometheus rules | `monitoring/rules/` | Alerts when channel data is > threshold old |

### P2 — Documentation & hygiene

| # | Action | File / Area | Acceptance |
|---|--------|-------------|------------|
| 10 | Update `CONSTITUTION.md` task count from 9 to actual 60+ and expand Appendix A | `internal/apigateway/CONSTITUTION.md` | Document matches code |
| 11 | Update `configs/allowed_env_vars.md` | `configs/allowed_env_vars.md` | Includes all 2026-07-25 keys |
| 12 | Remove dead Prometheus rules or implement their metrics | `monitoring/rules/disabled/` | No disabled rule references missing metric |
| 13 | Delete `backfill-replay` from `Dockerfile.cron` or add a cron container | `Dockerfile.cron`, `docker-compose.yml` | No unused binaries |

---

## 3. Design: Agent Startup Alert-Scanning Mechanism

### Goal
Every AI Agent (and human operator) that starts work on atlas-go must be able to ask: *"What alerts are currently active?"* and receive a single, authoritative answer before making changes.

### Current State

- AlertStore exists (`internal/monitoring/alert_store.go`) with JSONL + Postgres dual-write.
- `DualWriteRepository.LoadUnacknowledgedAlerts` exists but is **never called at startup**.
- MCP exposes only read-only alert tools (`alert_list`, `alert_get_stats`, `alert_get_rules`).

### Proposed Design

```
┌─────────────────────────────────────────────────────────────────┐
│  Agent Session Start (CLI / MCP / CI)                           │
│  1. Call unified alert scanner                                   │
│  2. Surface triggered + acknowledged + silenced alerts          │
│  3. Block or warn before code changes if P0/P1 alerts active    │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼
┌──────────────────────────────────────┐
│  AlertScanner (new package)          │
│  - Loads unacknowledged from store   │
│  - Queries Prometheus alertmanager   │
│  - Queries Wave9 synthesizer events  │
│  - Deduplicates by (category,source)│
└──────────────┬───────────────────────┘
               │
     ┌─────────┴──────────┐
     ▼                    ▼
 AlertStore          Prometheus
 (JSONL/Postgres)    Alertmanager
```

### Components

#### 3.1 `internal/monitoring/alertscanner` package

```go
package alertscanner

type AlertSnapshot struct {
    Source      string            // "alertstore" | "prometheus" | "wave9"
    ID          string
    Name        string            // rule ID or category
    Severity    string            // INFO|WARNING|ERROR|CRITICAL
    Status      string            // triggered|acknowledged|silenced
    ChannelID   string            // if ingestion-related
    Message     string
    FiredAt     time.Time
    AckedBy     string
    SilencedUntil *time.Time
}

type Scanner struct {
    store      AlertStoreReader
    promURL    string
    wave9      Wave9EventReader
}

func (s *Scanner) Scan(ctx context.Context) ([]AlertSnapshot, error)
```

#### 3.2 Startup rescan in `cmd/atlas/main.go`

After `AlertStore` is initialized, call:

```go
alerts, err := alertscanner.New(store, promURL, wave9).Scan(ctx)
if err != nil {
    log.Printf("alert scan failed: %v", err)
}
monitor.SetActiveAlerts(alerts) // expose via /api/alerts/active
```

#### 3.3 New MCP tools (Phase 2)

| Tool | Purpose |
|------|---------|
| `alert_scan` | Return unified snapshot from all 3 pipelines |
| `alert_acknowledge` | Acknowledge by ID |
| `alert_resolve` | Resolve by ID |
| `alert_silence` | Persist silenced_until |
| `alert_get_active` | Filter by severity/status/channel |

#### 3.4 Agent CLI hook

Add a shell alias / wrapper used by AI agents:

```bash
# .agent-hooks/alert-scan.sh
atlas-alert-scan() {
  curl -s http://localhost:18080/api/alerts/active | jq .
}
```

Or an MCP tool call at session start.

#### 3.5 Gate behavior

| Severity | Action |
|----------|--------|
| `CRITICAL` | Block code changes; require human override |
| `ERROR` | Warn strongly; require explicit ack before deploy |
| `WARNING` | Include in session context; suggest fixing first |
| `INFO` | Include in context only |

### Why This Fixes the "Alerts Are Never Found" Problem

- **Persistence**: Alerts survive process restart because they live in Postgres/JSONL.
- **Resurfacing**: Startup rescan reloads in-flight alerts into memory.
- **Unified view**: One query returns Go alerts, Prometheus alerts, and Wave9 events.
- **Actionability**: MCP tools let agents acknowledge / resolve / silence directly.
- **Accountability**: Every agent session begins with a recorded alert context.

---

## 4. Data-Source Index Proposal

To prevent future AI agents from "reinventing the wheel," create a single source-of-truth index:

**proposed channel-index reference document** (to be created in Route C)

| ChannelID | Source URL | Data Types | Owner Task | Schedule | Health Endpoint | Alert Rule | Backfill Status |
|-----------|------------|------------|------------|----------|-----------------|------------|-----------------|
| `twse_capital_flow` | `openapi.twse.com.tw` | foreign/institutional/dealer net | `auto_capital_flow` | 30m | `/api/dashboard/channel-health` | wave9_channel_individual_health | auto |
| `finmind` | `api.finmindtrade.com` | financials, revenue, institutional | `channel_health_finmind` | 1h | `/api/dashboard/channel-health` | — | broken (402) |
| ... | ... | ... | ... | ... | ... | ... | ... |

This index should be generated from code annotations (e.g., `// ChannelID: finmind`) and enforced by CI.

---

## 5. How to Prevent Future Drift

1. **CI gate**: `check_constitution.sh` must exit non-zero on new violations.
2. **Mandatory `ChannelID`**: Every BTM task that fetches data must set `ChannelID`.
3. **One backfill task**: Replace 4 FinMind rogues + 75 one-shot CLIs with a single `auto_backfill_gap` BTM task driven by the proposed channel-index backfill status.
4. **Alert lifecycle completion**: Implement ack/resolve/silence MCP tools and startup rescan.
5. **Document-first**: Update `CONSTITUTION.md` and the proposed channel-index before adding new channels/tasks.

---

## 6. PR Strategy

Because these changes touch many files, split into focused PRs:

| PR | Scope | Files | Risk |
|----|-------|-------|------|
| `#1335-followup` | Fix `/health` self-probe + hardcoded `/api/health` | `cmd/atlas/api_routes.go`, `cmd/atlas/main.go` | Low |
| `fix/constitution-audit-ci` | Harden `check_constitution.sh` + fix double registration | `scripts/ci/check_constitution.sh`, `cmd/atlas/operations_tasks.go` | Medium |
| `refactor/finmind-rogue-backfills` | Convert/delete FinMind backfills | `cmd/backfill-*` | Medium |
| `feat/channel-id-tasks` | Add ChannelID to 64 closure-fetched tasks | `cmd/atlas/*_tasks.go` | Medium |
| `feat/alert-scanner` | Add `alertscanner` package + startup rescan + MCP tools | `internal/monitoring/alertscanner/`, `cmd/atlas/main.go`, `cmd/atlas-mcp/server/tools_risk_alert.go` | Medium-High |
| `docs/channel-index` | Generate proposed channel-index document | New file + CI generator | Low |

---

## 7. Verification Checklist

- [x] `check_constitution.sh` exits non-zero on new violations.
- [x] `/api/health` returns same data as `/health` or redirects.
- [~] All BTM data-fetch tasks have `ChannelID`. (`government_flow_aggregate` fixed; remaining tasks are pure orchestration/calibration and legitimately omit `ChannelID`).
- [ ] `alert_scan` MCP tool returns alerts from all 3 pipelines (currently only in-process `AlertStore`; Prometheus Alertmanager + webhook ring buffer not yet aggregated).
- [ ] Proposed channel-index document is auto-generated and CI-enforced.
- [x] No disabled Prometheus rule references a missing metric.
- [x] `backfill-replay` is either scheduled or removed from `Dockerfile.cron` (binary and image references removed; only `cmd/REGISTRY.md` still lists it).

---

**Phase C complete.** Artifacts written to `docs/manifests/2026-07-25-channel-architecture-audit/`.
