# Dashboard Frontend Sync for Stock Screening Layer — Design Document

**Date:** 2026-04-16  
**Status:** Approved  
**Scope:** Backend persistence + Dashboard frontend integration for `internal/screener/`

---

## 1. Problem Statement

The backend already has a declarative screening layer (`internal/screener/`, `configs/agents.json`) that filters symbols BEFORE executors generate recommendations. The Dashboard frontend currently has zero visibility into this layer. Portfolio managers cannot see:

- Which agents have active screening criteria
- Which symbols were screened out and why
- Where the screening step sits in the pipeline workflow

---

## 2. Decision: Persistence Approach

**Chosen approach:** Persist screening rejects to `screened_symbols.jsonl` per session (`data/ledger/sessions/<session_id>/screened_symbols.jsonl`).

**Rejected approach:** Compute on-the-fly in the API.

### Rationale

| Factor | Persist (Chosen) | Compute On-the-Fly |
|--------|------------------|-------------------|
| **Auditability** | Immutable JSONL trail; deterministic | Non-deterministic if fundamentals are backfilled |
| **Complexity** | One extra write at session time | API must instantiate FactorEngine + FundamentalProvider + Screener |
| **Performance** | O(1) read (JSONL) | O(n) re-evaluation per API call |
| **Code reuse** | Reuses existing ledger patterns | Duplicates screening logic in dashboard API |

The orchestrator already has the screener, quotes, and fundamentals hot in memory at session time. Capturing the reject decision once and persisting it is both cheaper and more auditable than reconstructing it later.

---

## 3. Domain Model Additions

### 3.1 `ScreeningReject` (new in `internal/domain/screening.go`)

Represents a single symbol-agent rejection for audit and display.

```go
type ScreeningReject struct {
    SessionID      string    `json:"session_id"`
    Symbol         string    `json:"symbol"`
    AgentID        string    `json:"agent_id"`
    Skill          string    `json:"skill"`
    Criterion      string    `json:"criterion"`       // machine key, e.g. "pe_max"
    CriterionLabel string    `json:"criterion_label"` // human label, e.g. "P/E ≤ 20"
    Threshold      string    `json:"threshold"`       // bound value
    ActualValue    string    `json:"actual_value"`    // value at screening time
    RecordedAt     time.Time `json:"recorded_at"`
}
```

### 3.2 `ScreenResult` (new in `internal/screener/screener.go`)

Replaces the boolean-only return so callers know *why* a symbol failed.

```go
type ScreenResult struct {
    Passed    bool
    Reason    string
    Criterion string
    Label     string
    Threshold string
    Actual    string
}
```

---

## 4. What the Portfolio Manager MUST See

For every screened-out symbol, the frontend must display:

1. **Symbol + Agent ID** — who evaluated what
2. **Failed criterion** — e.g., `P/E ≤ 20`, `Volume ≥ 1,000,000`
3. **Actual value at screening time** — e.g., `P/E was 25.3`, `Volume was 850,000`
4. **Session/timestamp** — for audit trail alignment

Without #3, the PM cannot judge whether the criterion is too tight or whether market conditions justify an override.

---

## 5. Frontend HCI Design

### 5.1 Agent Observatory — Criteria Badges

- Show criteria as **compact badge chips** next to each agent name.
- Examples: `P/E≤20`, `Vol≥1M`, `動能≥0`, `股息≥2%`
- Hover reveals full criterion name and bound.
- Source: `configs/agents.json` (static config), surfaced through the existing `/api/dashboard/universe-overlap` response (with a new `screening_criteria` field).

### 5.2 Pipeline — Screened-Out Toggle

- **Do NOT add criteria columns to the main pipeline table.** Keep focus on direction, conviction, and returns.
- Add a second checkbox: `顯示被篩選層排除的標的`
- When checked, reveal a **muted sub-table** below the main pipeline:
  - Columns: `標的 | 策略來源 | 未通過條件 | 門檻 | 實際值`
  - Visual treatment: grey text, subtle left border (`border-left: 3px solid #666`)

### 5.3 Workflow Banner

Insert a new step in the pipeline workflow:

```
AI 推薦 → 篩選層 → 控制層 → 模擬投組
```

The `篩選層` step may optionally show a count badge like `13 檔被篩除` after data loads.

---

## 6. Implementation Steps

### Phase A — Backend: Capture and Persist

1. **`internal/domain/screening.go`**
   - Add `ScreeningReject` type.

2. **`internal/screener/screener.go`**
   - Add `ScreenDetailed` method returning `ScreenResult`.
   - Keep `Screen` as a thin wrapper for backward compatibility.

3. **`internal/orchestrator/plugin_registry.go`**
   - Add `ScreenDetailed` method on `PluginRegistry`.

4. **`internal/orchestrator/executors.go`**
   - Thread `sessionID` and a `[]ScreeningReject` accumulator into `collectRecommendations`.
   - On rejection, append a `ScreeningReject` and continue.

5. **`internal/ledger/ledger.go`**
   - Add `RecordSessionScreeningRejects(sessionID, rejects)` — JSONL write.
   - Add `LoadSessionScreeningRejects(sessionID)` — JSONL read.

6. **`internal/orchestrator/system.go` (or backtest runner)**
   - After `RecordSessionOutcomes`, call `RecordSessionScreeningRejects`.

### Phase B — Backend: API Exposure

7. **`internal/monitoring/dashboard_api.go`**
   - In `handleRecommendationPipeline`, load `screened_symbols.jsonl` and return it in `RecommendationPipelineResponse.ScreenedItems`.
   - In `handleUniverseOverlap`, populate `AgentUniverseView.ScreeningCriteria` from registry.

### Phase C — Frontend: Display

8. **`web/static/index.html`**
   - **Workflow banner:** insert `篩選層` step.
   - **Pipeline toggle:** add checkbox and `togglePipelineShowScreened` handler.
   - **Screened-out table:** render `screened_items` from API in a muted sub-section.
   - **Agent Observatory:** render criteria badges for each agent.

---

## 7. Files to Touch

| File | Change |
|------|--------|
| `internal/domain/screening.go` | Add `ScreeningReject` |
| `internal/screener/screener.go` | Add `ScreenDetailed`; keep `Screen` wrapper |
| `internal/screener/engine_test.go` | Tests for `ScreenDetailed` |
| `internal/orchestrator/plugin_registry.go` | Add `ScreenDetailed` to `PluginRegistry` |
| `internal/orchestrator/executors.go` | Thread `sessionID` + reject accumulator |
| `internal/orchestrator/executors_test.go` | Update test signatures |
| `internal/ledger/ledger.go` | JSONL read/write for screening rejects |
| `internal/ledger/ledger_test.go` | Tests for new read/write |
| `internal/monitoring/dashboard_api.go` | Load rejects; enrich overlap response |
| `internal/monitoring/dashboard_api_test.go` | Tests for pipeline screened items |
| `web/static/index.html` | Banner, toggle, badges, screened-out table |

---

## 8. Testing Checklist

- [ ] `ScreenDetailed` returns correct `Criterion`, `Label`, `Threshold`, and `Actual` for each filter type (PE, PB, Volume, Momentum, MinTotalFactorScore).
- [ ] `RecordSessionScreeningRejects` / `LoadSessionScreeningRejects` round-trip correctly.
- [ ] Backtest produces `screened_symbols.jsonl` inside the session directory.
- [ ] `/api/dashboard/recommendation-pipeline` returns `screened_items` for the requested session.
- [ ] `/api/dashboard/universe-overlap` includes `screening_criteria` per agent.
- [ ] Frontend renders badges in Agent Observatory.
- [ ] Frontend toggle reveals screened-out symbols with correct columns.
- [ ] CI checks pass: `gofmt`, `go vet`, `staticcheck`, `go test ./...`.
