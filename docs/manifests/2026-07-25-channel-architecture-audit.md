# Channel / Scheduler / Monitoring / Alert Architecture Audit Manifest

**Date**: 2026-07-25  
**Branch**: feat/p2-monitoring-alert-unification  
**Status**: Phase A - Structural survey in progress  
**Owner**: Atlas AI Agent  
**Trigger**: User request to audit data-source fragmentation, rogue channels, scheduler/monitoring gaps, and alert management debt.

---

## Executive Questions

1. How many external data-fetching channels exist in the codebase (registered + rogue)?
2. Which channels are outside the unified `apigateway` architecture (`os.Getenv` HTTP clients, unregistered tasks)?
3. Are all channels covered by unified scheduling (`BackgroundTaskManager`) and health monitoring?
4. Is `/api/health` the canonical health endpoint? Are there overlapping or conflicting health endpoints?
5. Are single-source data types split across multiple channels, causing redundant fetches and fragmented schedules?
6. How fragmented is the alert system? Are alerts managed, or fire-and-forget?
7. Why is historical data still missing after 6+ months of maintenance?
8. What mechanism should be built so every AI Agent scans active alerts before starting work?

---

## Phase A: Structural Survey

| Area | Question | Assigned Agent | Output Format |
|------|----------|----------------|---------------|
| A1 Channels | Enumerate every external data source / channel ID, registry or not. | ChannelScout | `channels.json` + rogue list |
| A2 Scheduler | Enumerate every scheduled/cron/background task that fetches data. | SchedulerScout | `tasks.json` + unregistered list |
| A3 Health | Enumerate every health/monitoring endpoint and per-channel health signal. | HealthScout | `health_endpoints.json` + overlap matrix |
| A4 Alerts | Enumerate every alert generation path, storage, and lifecycle. | AlertScout | `alerts.json` + fragmentation score |
| A5 Constitution Compliance | Cross-check A1-A4 against `internal/apigateway/CONSTITUTION.md`. | ComplianceScout | `violations.json` |

## Phase B: Deep Dive

| Area | Question | Depends On |
|------|----------|------------|
| B1 Rogue channels | Why do they exist? Do they duplicate registered channels? | A1, A5 |
| B2 Data fragmentation | Same source split into multiple channels/schedules? | A1, A2 |
| B3 Monitoring gaps | Channels/tasks with no health/alerts? | A1-A4 |
| B4 Alert debt | Alerts that fire but are never acknowledged/resolved? | A4 |
| B5 Missing historical data | Which channels claim backfill but never run / never convert? | A1-A3 |

## Phase C: Integrated Diagnosis & Recommendations

- Consolidated report with concrete counts, names, and file references.
- Proposed `docs/reference/channel-index.md` schema (to be created in Route C).
- Proposed agent-startup alert-scanning mechanism.
- Prioritized remediation plan.

---

## Agent Contract

Each Phase A scout MUST:
1. Use GitNexus / codebase-memory / codegraph first, grep only for text position confirmation.
2. Produce a JSON/MD artifact under `docs/manifests/2026-07-25-channel-architecture-audit/`.
3. Include exact file paths, line numbers, and channel/task/alert names.
4. Distinguish **verified facts** from **inferences** with `[INFERENCE]` markers.
5. NOT modify code.

---

## Output Directory

```
docs/manifests/2026-07-25-channel-architecture-audit/
├── a1-channels.json
├── a2-tasks.json
├── a3-health.json
├── a4-alerts.json
├── a5-violations.json
├── b1-b5-deep-dive.md
└── c1-integrated-report.md
```
