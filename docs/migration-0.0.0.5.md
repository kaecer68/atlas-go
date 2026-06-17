# Migration Guide: 0.0.0.4 → 0.0.0.5

> **Status**: required for any consumer of `/api/performance-report` or `internal/reporting.PerformanceReport`.
> **Effective**: 2026-06-17 (release `0.0.0.5`, PR [#564](https://github.com/kaecer68/atlas-go/pull/564))

This release ships 3 breaking changes in the performance-report API and 1 additive
configuration parameter. All changes are documented in
[CHANGELOG.md § 0.0.0.5](../../CHANGELOG.md).

## Overview

| # | Change | Type | Effort |
|---|--------|------|--------|
| 1 | `TotalReturn` → `AggregateForwardReturn` (field rename) | Breaking | 5 min |
| 2 | `SharpeLike` `float64` → `*float64` (nil-pun) | Breaking | 15 min |
| 3 | New endpoint `/api/dashboard/agent-names` | Additive | 5 min |
| 4 | New parameter `reporting.win_rate_threshold` | Additive | 0 min (default works) |

Frontend (vanilla JS) and backend (Go) consumers need coordinated updates for #1 and #2.
#3 replaces two competing static maps with a single API source of truth.
#4 has a sensible default and requires no action unless you want a different threshold.

---

## 1. `TotalReturn` → `AggregateForwardReturn`

### What changed

Two Go structs in `internal/reporting/performance.go` renamed a field, and the
corresponding JSON tags changed.

| Struct | Old field | New field | Old JSON | New JSON |
|--------|-----------|-----------|----------|----------|
| `AgentContribution` | `TotalReturn` | `AggregateForwardReturn` | `total_return` | `aggregate_forward_return` |
| `RegimePerformance` | `TotalReturn` | `AggregateForwardReturn` | `total_return` | `aggregate_forward_return` |

The field's **meaning did not change** (sum of per-recommendation `ForwardReturn`).
The rename clarifies that this is not a portfolio MTM return.

### Why

The previous name `total_return` was semantically misleading — it suggested
portfolio mark-to-market, but the value is the **sum of per-recommendation
forward returns**. The new name is unambiguous.

### Frontend migration

```javascript
// BEFORE
const totalReturn = agent.total_return;  // number

// AFTER
const aggregateForwardReturn = agent.aggregate_forward_return;  // number
```

Update the field name in:

- `web/static/js/components/performance-report.js` (per-agent table, regime breakdown table)
- Any other file grepping `total_return` in `web/static/js/`

The TypeScript types in `web/static/js/shared/field_types.ts` are auto-regenerated
via `go generate ./...`; the new field is present from `0.0.0.5` onward.

### Backend migration

```go
// BEFORE
type AgentContribution struct {
    TotalReturn float64 `json:"total_return"`
}

// AFTER
type AgentContribution struct {
    AggregateForwardReturn float64 `json:"aggregate_forward_return"`
}
```

In-process consumers (`cmd/judge-experiment`, `cmd/promote-baseline`,
`internal/monitoring`) that read the struct must update the field name.
JSON deserializers (HTTP clients, file readers) get the new field automatically.

### Verification

```bash
# No more references to old name in production code
grep -rn "total_return" internal/ cmd/ web/ --include="*.go" --include="*.js" \
    | grep -v "_test.go" | grep -v "// " | grep -v "valid_fields.json"
# Expected: empty output (test files and intentional comments are OK)
```

---

## 2. `SharpeLike` `float64` → `*float64`

### What changed

`AgentContribution.SharpeLike` is now `*float64` (pointer) instead of `float64`.
It returns `nil` when:
- Sample count `< reporting.sharpe_min_samples` (default 5), **or**
- Standard deviation is 0 (constant-return series)

The frontend renders `"N/A"` for `null`.

### Why

Previously, a value of `0.0` for `SharpeLike` was indistinguishable from
"agent had 0 trades" vs "agent had constant returns" vs "agent had < 60
samples and the legacy code returned 0 as a placeholder". The nil-pun
makes insufficient data explicit and lets the UI render `N/A` instead of
a misleading `0.00`.

### Frontend migration

```javascript
// BEFORE
const sharpe = agent.sharpe_like;  // number, possibly 0
const formatted = sharpe.toFixed(2);  // "0.00" — misleading

// AFTER
const sharpe = agent.sharpe_like;  // number | null
const formatted = sharpe === null ? 'N/A' : sharpe.toFixed(2);
```

The reference implementation in `web/static/js/components/performance-report.js`
already handles both cases:

```javascript
${a.sharpe_like === null ? '<td>N/A</td>' : `<td>${a.sharpe_like.toFixed(2)}</td>`}
```

### Backend migration

```go
// BEFORE
type AgentContribution struct {
    SharpeLike float64 `json:"sharpe_like"`
}
value := agent.SharpeLike  // always a number

// AFTER
type AgentContribution struct {
    SharpeLike *float64 `json:"sharpe_like"`
}
if agent.SharpeLike == nil {
    // insufficient data — handle explicitly
} else {
    value := *agent.SharpeLike
}
```

For sorting or aggregation, decide explicitly:
- Skip nil entries
- Treat as a configurable sentinel (e.g., `-99`)
- Filter before computing aggregates

### Verification

```bash
# All SharpeLike readers should handle nil
grep -rn "\.SharpeLike\b" --include="*.go" internal/ cmd/ \
    | grep -v "_test.go" \
    | grep -v "// "
# For each hit, verify nil-handling is present (or the reader is the canonical writer)
```

---

## 3. New endpoint: `/api/dashboard/agent-names`

### What changed

A new endpoint serves the agent display-name registry from `configs/agents.json`
as JSON. It is the **single source of truth** for the performance-report
frontend, replacing two competing static maps that drifted out of sync:

- `web/static/js/names.js` (29 entries, stale)
- `web/static/js/shared/constants.js` (38 entries, partial)

### Why

The two static maps had different coverage of the agent registry, causing
mixed Chinese/English fallback rendering when an agent was missing from one
but present in the other. The new endpoint serves the live `configs/agents.json`
registry, so adding a new agent there propagates immediately.

### Frontend migration

```javascript
// BEFORE
import { AGENT_NAMES } from '/static/js/names.js';  // 29 entries
// or
import { AGENT_DISPLAY_NAMES } from '/static/js/shared/constants.js';  // 38 entries

// AFTER
async function loadAgentNames() {
  const resp = await fetch('/api/dashboard/agent-names');
  const { agents } = await resp.json();
  return new Map(agents.map(a => [a.id, a]));
}
const NAMES = await loadAgentNames();
const displayName = NAMES.get(agent.id)?.name ?? agent.id;
```

The endpoint returns:
```json
{
  "agents": [
    { "id": "foreign-flow-01", "name": "外資流向追蹤", "skill": "flow", "layer": "macro" },
    { "id": "us-macro-spx-01", "name": "美股 S&P 500 觀察", "skill": "us-macro", "layer": "macro" }
  ]
}
```

Empty array `{"agents": []}` on missing file (graceful degradation).

### Backend migration

No migration required — this is purely additive. Existing handlers and
endpoints are unchanged.

### Verification

```bash
curl -s http://localhost:8080/api/dashboard/agent-names | jq '.agents | length'
# Expected: 38 (or whatever your configs/agents.json count is)

curl -sI http://localhost:8080/api/dashboard/agent-names
# Expected: HTTP/1.1 200 OK, Content-Type: application/json
```

---

## 4. New parameter: `reporting.win_rate_threshold`

### What changed

A new field `reporting.win_rate_threshold` (default `0.002`, i.e. 0.2%) is
added to `ParametersConfig`. Win classification in 3 calculation paths
(`calculateTradeMetrics`, `calculateTopAgents`, `calculateRegimeBreakdown`)
now requires `ForwardReturn > win_rate_threshold` instead of
`ForwardReturn > 0`.

### Why

The previous `> 0` threshold counted tiny positive returns as wins, even
when the return was below transaction costs (~0.15% TW market) plus slippage.
A `ForwardReturn` of 0.001 (0.1%) used to be a "win" but is a loss after costs.

The 0.2% default covers the transaction cost + a small slippage buffer.

### Frontend migration

No migration required — the threshold is server-side only. The win rate
display will reflect the new calculation immediately.

### Backend migration

No migration required — the parameter has a sensible default. To customize:

```json
// configs/parameters.json
{
  "reporting": {
    "win_rate_threshold": 0.003,    // 0.3% for higher-cost environments
    "sharpe_min_samples": 5          // already shipped in 0.0.0.5
  }
}
```

Validation rejects out-of-range values:
- `win_rate_threshold` must be in `[0, 1)`
- `sharpe_min_samples` must be `>= 1`

### Verification

```bash
# All Sharpe / win-rate calculations use the new parameter
grep -rn "ForwardReturn > 0\|ForwardReturn > winRateThreshold" internal/reporting/
# Expected: only the winRateThreshold() calls
```

---

## Rollback strategy

If you need to roll back 0.0.0.5 in production:

1. Revert the field renames in `internal/reporting/performance.go` (single file)
2. Revert `SharpeLike` to `float64` (single field)
3. Frontend can keep its dual-map fallback; it will read whatever the backend
   returns.

The new endpoint and parameter are additive and have safe defaults — they
can remain in place even if the field renames are reverted.

## Related links

- [CHANGELOG.md § 0.0.0.5](../../CHANGELOG.md)
- [PR #564](https://github.com/kaecer68/atlas-go/pull/564)
- [docs/swagger.json § /api/dashboard/agent-names](../swagger.json)
- [.claude/skills/atlas-data-visibility/SKILL.md](../../.claude/skills/atlas-data-visibility/SKILL.md) — see "single source of truth" pattern
