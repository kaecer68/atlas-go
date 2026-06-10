# Alert System Redesign — Phase 1 Design Document

**Status**: ✅ Approved by user (2026-06-10)  
**PR**: #459 (design + alert config)  
**Target**: 3–5 PRs across 8–10 work days  
**Epic**: Full 4-phase implementation (backend → API → Dashboard → automation)

---

## 1. Problem Statement

Current alert system produces **16,880 alerts** with **99.5% noise** (16,806 heartbeats), zero acknowledged, no lifecycle, no pagination, dead `value/threshold` fields, and a 60-line frontend that dumps all rows into a single `innerHTML`.

**User’s 6 UX questions that triggered this redesign:**
1. Why require manual confirmation for machine-generated alerts?
2. Why does the 「規則」 column show "simulation" with no context?
3. What does "RISK_OFF 0 orders" mean — is it an error?
4. Why is every 「數值」 0.00?
5. How do I know if an old alert is still active or already fixed?
6. No pagination — 16k rows kills the browser.

---

## 2. Root Cause Analysis (Phase 1 Audit Results)

### 2.1 Data Model — 10 fields, missing lifecycle

```go
type AlertRecord struct {
    ID            string    // uuid
    Timestamp     time.Time // when fired
    Rule          string    // e.g. "channel_health_summary"
    Severity      AlertLevel// info/warning/error/critical
    Message       string    // human text
    Value         float64   // ❌ ALWAYS 0 (bug in monitor.go:115–122)
    Threshold     float64   // ❌ ALWAYS 0 (same bug)
    Acknowledged  bool      // manual only
    AcknowledgedAt*time.Time
    AcknowledgedBy string
}
```

**Missing:** `status`, `resolved_at`, `dedup_key`, `first_seen`, `last_seen`, `count`.

**Indexes:** 3 (timestamp, rule, acknowledged). No dedup index.

### 2.2 AlertAPI wraps JSONL AlertStore — bypasses PostgreSQL

- Reads go through `AlertStore` (JSONL append-only), **not** `DualWriteRepository`.
- `Acknowledge()` rewrites the **entire JSONL file** → O(n²) for n acks.
- No pagination, no time-range endpoint, no bulk ack.

### 2.3 Frontend — 60-line `alerts.js`, single `innerHTML`

- Renders all 16,880 rows at once.
- `acknowledgeAlert()` not exposed to `window` → inline `onclick` throws `ReferenceError`.
- Missing imports (`postJSON`, `notify`, `exportTableToCSV`).
- `showUnacknowledgedOnly()` is a `console.log` stub.
- No CSS reuse (patterns exist in `kpi-grid`, `filter-panel`, `paginateTable`).

### 2.4 Severity Classification Bug

- `AutoExperimentMonitor` uses raw string severities (`"warning"`) bypassing `AlertLevel` enum.
- `RISK_OFF` (expected no-trade state) tagged as `WARNING` — false positive.

### 2.5 Alert Producers — 16 sources mapped

| Producer | Count | Type | Action |
|----------|-------|------|--------|
| `channel_health_summary` | 16,806 | 30s heartbeat | ❌ Noise — demote to dashboard widget |
| `simulation` (RISK_OFF 0-orders) | 54 | Expected behavior | ❌ Suppress — not an alert |
| `experiment` (success/reject/defer) | 14 | Lifecycle event | ❌ Demote to log |
| `background_task` (failures) | 4 | Real error | ✅ Keep — but only at exactly 3 consecutive failures |
| `etf_nav` | 2 | Daily check | ✅ Keep |

**After fixes: ~16,880 → ~10 active alerts (99.94% reduction).**

---

## 3. Critical Decisions (User Confirmed ✅)

### Decision 1: One-time Auto-cleanup of Heartbeats
- **Action**: Delete all 16,806 `channel_health_summary` alerts older than 24h.
- **Future**: Heartbeats move to a `/health` dashboard widget, **not** the alert stream.
- **Rationale**: Heartbeat ≠ alert. Separation is critical per industry standards (Prometheus AlertManager, Stripe).

### Decision 2: Suppress RISK_OFF 0-orders Alerts
- **Action**: `simulation` rule with `regime=RISK_OFF` and `orders=0` → silently logged, never alerted.
- **Rationale**: RISK_OFF means "don't trade". Zero orders is correct behavior, not a warning.

### Decision 3: Full 4-Phase Implementation
- **Scope**: Backend migration + API expansion + Dashboard frontend + automation rules.
- **Timeline**: 8–10 work days, 3–5 PRs.
- **Rationale**: Partial fixes (just frontend pagination) don’t solve the root cause (noise generation).

### Decision 4: background_task Alert — Exactly 3 Failures + Recovery
- **Action**: Alert fires **only** when a task fails exactly 3 consecutive times.
- **Recovery**: When task succeeds after failure streak, emit a "recovered" signal (auto-resolve).
- **Rationale**: Single failure is transient; 3+ is a real problem. Recovery signal closes the loop.

### Decision 5: Experiment Success/Reject/Defer → Logging
- **Action**: `experiment_success`, `experiment_rejected`, `experiment_deferred` → structured logs only.
- **Rationale**: These are lifecycle events, not anomalies. They belong in audit logs, not the alert stream.

---

## 4. Architecture Overview

### 4.1 Target State (After All Phases)

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Alert Sources  │────▶│ AlertDeduplicator│────▶│  SeverityRouter │
│  (16 producers) │     │ (dedup_key +    │     │ (P0–P4 routing) │
│                 │     │  count + window)│     │                 │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                       ┌────────────────────────────────┼────────────────────────────────┐
                       │                                │                                │
                       ▼                                ▼                                ▼
              ┌──────────────┐                 ┌──────────────┐                 ┌──────────────┐
              │  AutoHandler  │                 │  AlertStore  │                 │  Dashboard   │
              │ (suppress +   │                 │  (JSONL +    │                 │  (KPI +      │
              │  auto-ack)    │                 │   PG dual)   │                 │   detail)    │
              └──────────────┘                 └──────────────┘                 └──────────────┘
```

### 4.2 Data Model Changes

**New fields on `AlertRecord`:**
```go
Status        AlertStatus // triggered | acknowledged | resolved | silenced
DedupKey      string      // "rule:source:metric" for grouping
FirstSeen     time.Time   // first occurrence
LastSeen      time.Time   // most recent occurrence
Count         int         // dedup count
ResolvedAt    *time.Time  // when condition cleared
ResolvedBy    string      // "auto" | "user:admin" | "system"
SilencedUntil *time.Time  // mute expiration
```

**New enum:**
```go
type AlertStatus string
const (
    AlertStatusTriggered    AlertStatus = "triggered"
    AlertStatusAcknowledged AlertStatus = "acknowledged"
    AlertStatusResolved     AlertStatus = "resolved"
    AlertStatusSilenced     AlertStatus = "silenced"
)
```

### 4.3 Severity Routing (Industry Standard)

| Severity | Route | Response Time | Example |
|----------|-------|---------------|---------|
| CRITICAL | Push + SMS + Slack page on-call | 5 min | daily_loss_critical breach |
| ERROR | Slack @mention | 15 min | background_task 3 failures |
| WARNING | Slack channel, no page | 1 hour | etf_nav fetch delayed |
| INFO | Dashboard only | N/A | Heartbeats (demoted to widget) |

---

## 5. Phase Plan

### Phase 2A — Backend Core (PR #1)
- [ ] Migration `000006`: add `status`, `dedup_key`, `first_seen`, `last_seen`, `count`, `resolved_at`, `resolved_by`, `silenced_until`
- [ ] Fix `monitor.go:115–122`: copy metadata → `Value`/`Threshold`
- [ ] Implement `AlertDeduplicator`: dedup_key + 5-min window + count
- [ ] Implement `AutoHandler`:
  - INFO → auto-ack
  - RISK_OFF 0-orders → suppress
  - 3-fail background_task → alert
  - Recovery signal → auto-resolve
- [ ] One-time cleanup: delete `channel_health_summary` older than 24h
- [ ] Severity fix: `RISK_OFF` → INFO (not WARNING)

### Phase 2B — API Expansion (PR #2)
- [ ] `GET /api/alerts` — add query params: `page`, `page_size`, `severity`, `status`, `rule`, `from`, `to`, `sort`
- [ ] `GET /api/alerts/stats` — KPI summary (total, unack, by severity, trend)
- [ ] `POST /api/alerts/acknowledge-bulk` — batch ack
- [ ] `POST /api/alerts/silence` — mute by rule/duration
- [ ] `GET /api/alerts/rules` — list active rules with counts

### Phase 3 — Dashboard Frontend (PR #3)
- [ ] Dashboard overview: KPI cards (total/unack/critical/warning) + health status + sparkline
- [ ] Filterable detail table: time range, severity, rule, status filters + pagination
- [ ] Action buttons: Acknowledge, Silence, Resolve (with confirmation modal)
- [ ] Automation rules UI: toggle suppress rules, set thresholds

### Phase 4 — Testing + Deploy (PR #4/5)
- [ ] Unit tests: deduplication, auto-handler, severity routing
- [ ] Integration tests: full flow from producer → dedup → auto-ack → dashboard
- [ ] E2E tests: frontend filters, pagination, bulk actions
- [ ] CI pass + PR review + merge

---

## 6. API Specification

### 6.1 List Alerts (Enhanced)
```
GET /api/alerts?page=1&page_size=50&severity=warning&status=triggered&from=2026-06-01T00:00:00Z&sort=timestamp_desc

Response:
{
  "alerts": [...],
  "pagination": {
    "page": 1,
    "page_size": 50,
    "total": 128,
    "total_pages": 3
  },
  "summary": {
    "total": 128,
    "unacknowledged": 12,
    "by_severity": {"critical": 2, "warning": 10, "info": 116}
  }
}
```

### 6.2 Dashboard Stats
```
GET /api/alerts/stats

Response:
{
  "kpi": {
    "total_24h": 12,
    "unacknowledged": 2,
    "critical": 0,
    "warning": 2
  },
  "health": {
    "channel_health_summary": {"status": "ok", "last_seen": "2026-06-10T09:58:00Z"},
    "etf_nav": {"status": "ok", "last_seen": "2026-06-10T00:00:00Z"},
    "background_task": {"status": "ok", "last_seen": "2026-06-10T09:55:00Z"}
  },
  "trend": [
    {"hour": "2026-06-10T08:00:00Z", "count": 3, "severity": "warning"},
    {"hour": "2026-06-10T09:00:00Z", "count": 1, "severity": "info"}
  ]
}
```

### 6.3 Bulk Acknowledge
```
POST /api/alerts/acknowledge-bulk
Body: {"ids": ["uuid1", "uuid2", ...]}

Response: {"acknowledged": 2, "failed": 0}
```

### 6.4 Silence Rules
```
POST /api/alerts/silence
Body: {"rule": "simulation", "duration_minutes": 60, "reason": "RISK_OFF expected"}

Response: {"silenced_until": "2026-06-10T11:00:00Z"}
```

---

## 7. Configuration

New `configs/parameters.json` section (already added in this PR):

```json
{
  "alert": {
    "daily_loss_critical_pct": {
      "value": -0.02,
      "rationale": "Daily PnL critical threshold (-2%)",
      "source": "heuristic"
    },
    "daily_loss_warning_pct": {
      "value": -0.015,
      "rationale": "Daily PnL warning threshold (-1.5%)",
      "source": "heuristic"
    },
    "heartbeat_ttl_minutes": {
      "value": 5,
      "rationale": "Heartbeat older than 5min → channel down",
      "source": "heuristic"
    },
    "dedup_window_minutes": {
      "value": 5,
      "rationale": "Group identical alerts within 5min",
      "source": "heuristic"
    },
    "auto_ack_severity": {
      "value": ["info"],
      "rationale": "INFO alerts auto-acknowledged",
      "source": "heuristic"
    },
    "suppress_rules": {
      "value": ["simulation:risk_off_zero_orders"],
      "rationale": "RISK_OFF with 0 orders is expected",
      "source": "heuristic"
    }
  }
}
```

---

## 8. Risk & Mitigation

| Risk | Impact | Mitigation |
|------|--------|------------|
| One-time cleanup deletes real alerts | Low | Only delete `rule=channel_health_summary` AND older than 24h |
| Dedup groups unrelated alerts | Medium | Dedup key includes `rule:source:metric`, not just message |
| Auto-ack hides real problems | Low | Only INFO auto-acked; WARNING+ require manual |
| Migration locks table | Medium | Add columns as nullable; backfill in batches |
| Frontend bundle size | Low | Reuse existing CSS/JS patterns; no new deps |

---

## 9. Acceptance Criteria

- [ ] `/api/alerts` returns ≤ 20 alerts after cleanup (was 16,880)
- [ ] `channel_health_summary` alerts = 0 in alert stream
- [ ] `RISK_OFF 0-orders` alerts = 0
- [ ] `experiment_success/reject/defer` alerts = 0
- [ ] Dashboard loads in < 1s (was > 10s for 16k rows)
- [ ] Pagination works: 50/page, sortable, filterable
- [ ] Bulk ack: select multiple → ack → status changes
- [ ] CI passes: `go test ./...`, `npm run build`, `go build ./...`

---

## 10. References

- Frontend fix PR: #458 (acknowledgeAlert binding, imports, pagination prep)
- Audit report: `docs/alert_monitoring_audit_report.md`
- Industry research: PagerDuty Events API, Prometheus AlertManager, Grafana, Stripe alerting blog

---

*Design approved by user 2026-06-10. Proceeding to Phase 2 implementation.*