# Dashboard Data Quality Bugs — Implementation Plan

**Date**: 2026-05-19  
**Scope**: 5 root-cause fixes for macro narrative dashboard showing zero/stale values  
**Branch**: `fix/dashboard-data-quality`

---

## Bug Summary

| # | Bug | Symptom | Root Cause |
|---|-----|---------|------------|
| 1 | `mergeWithPrev()` missing fields | `retail_short_balance.value=0`, `bdi.value=0` in `latest.json` | `ingestor.go:121–170` handles 15/17 fields; `RetailShortBalance` and `BDI` never merged from previous snapshot |
| 2 | `NarrativeEngine` uninitialized macro state | `/api/narrative/stress-index/current` always returns 0 | `lastMacro`/`prevMacro`/`lastGeo` zero-valued after `NewNarrativeEngine()`; never populated at runtime |
| 3 | `TWSEMarginBalanceProvider` empty `storageDir` | Margin balance history not persisted; stale/zero reads | `dashboard_api.go:136` passes `""` instead of real storage path |
| 4 | `FrankfurterFXProvider` silent zero for USD/TWD | `usd_twd.value=0` in `latest.json` | No error handling / fallback when TWD parse fails or API returns zero |
| 5 | `HandleSeasonalAnalysis` uses static defaults | Frontend shows `-0.0%` avg market return | `handlers.go` returns `DefaultSeasonalPatterns()` without calling `DetectCurrentPatterns()` |

---

## Fix 1 — `mergeWithPrev()` Missing Fields

### File
`internal/narrative/ingestor.go`

### Location
After line 169 (after `CapexGrowth` block), before `return curr`

### Change
Add two missing merge cases:

```go
if curr.RetailShortBalance.Symbol == "" {
    curr.RetailShortBalance = prev.RetailShortBalance
}
if curr.BDI.Symbol == "" {
    curr.BDI = prev.BDI
}
```

### Dependencies
None — standalone change.

### Test Plan
```bash
# Unit test: verify mergeWithPrev propagates RetailShortBalance and BDI
go test -v -run TestMergeWithPrev ./internal/narrative/...

# Integration: ingest two snapshots where second has empty BDI; confirm merged value
go test -v -run TestMacroIngestor ./internal/narrative/...
```

### QA Verification
```bash
# After fix, trigger ingestor and check latest.json
curl -s http://localhost:8080/api/macro/snapshot | jq '.retail_short_balance.value, .bdi.value'
# Expected: non-zero values (or previous snapshot values if current fetch fails)
```

---

## Fix 2 — `NarrativeEngine` Uninitialized Macro State

### Files
- `internal/narrative/knowledge_base.go` — add `UpdateMacro()` method
- `internal/narrative/ingestor.go` — call `UpdateMacro()` after snapshot ingestion

### Change A — `knowledge_base.go`

Add method to `NarrativeEngine` (or `KnowledgeBase`, whichever holds `lastMacro`/`prevMacro`/`lastGeo`):

```go
// UpdateMacro populates the macro and geo state used by GetCurrentStressIndex.
// Must be called after each successful snapshot ingestion.
func (kb *KnowledgeBase) UpdateMacro(macro marketdata.MacroDataSnapshot, geo GeoRiskData) {
    kb.mu.Lock()
    defer kb.mu.Unlock()
    kb.prevMacro = kb.lastMacro
    kb.lastMacro = macro
    kb.lastGeo = geo
}
```

> Adjust receiver type to match actual struct holding these fields (confirm in `knowledge_base.go:415–462`).

### Change B — `ingestor.go`

After successful snapshot ingestion (after `m.engine.IngestSnapshot(snap)`), call:

```go
m.engine.UpdateMacro(snap, m.buildGeoRisk(snap))
```

Where `buildGeoRisk()` constructs `GeoRiskData` from the snapshot (or returns a zero value if geo data is not available in the snapshot).

### Dependencies
Fix 1 should be applied first so `RetailShortBalance`/`BDI` are populated before `UpdateMacro` is called.

### Test Plan
```bash
go test -v -run TestGetCurrentStressIndex ./internal/narrative/...
go test -v -run TestKnowledgeBase ./internal/narrative/...
```

### QA Verification
```bash
# Narrative stress index endpoint (was always 0)
curl -s http://localhost:8080/api/narrative/stress-index/current | jq '.stress_index'
# Expected: non-zero value matching /api/taiwan/stress-index (~37)

# Cross-check with macro endpoint
curl -s http://localhost:8080/api/taiwan/stress-index | jq '.value'
```

---

## Fix 3 — `TWSEMarginBalanceProvider` Empty `storageDir`

### File
`internal/monitoring/dashboard_api.go`

### Location
Line 136 (call to `NewTWSEMarginBalanceProvider`)

### Change
Replace empty string with real storage directory:

```go
// Before
provider := marketdata.NewTWSEMarginBalanceProvider("")

// After
storageDir := filepath.Join(m.config.DataDir, "marketdata", "margin")
if err := os.MkdirAll(storageDir, 0755); err != nil {
    return fmt.Errorf("create margin storage dir: %w", err)
}
provider := marketdata.NewTWSEMarginBalanceProvider(storageDir)
```

> `m.config.DataDir` should resolve to `data/` (confirm via `internal/config/config.go`). Add `"os"` and `"path/filepath"` imports if not already present.

### Dependencies
None — standalone change.

### Test Plan
```bash
go test -v -run TestTWSEMarginBalance ./internal/marketdata/...
go test -v -run TestDashboardAPI ./internal/monitoring/...
```

### QA Verification
```bash
curl -s http://localhost:8080/api/macro/snapshot | jq '.retail_margin_balance.value'
# Expected: non-zero value; also verify data/marketdata/margin/ directory is created
ls data/marketdata/margin/
```

---

## Fix 4 — `FrankfurterFXProvider` Silent Zero for USD/TWD

### File
`internal/marketdata/frankfurter_provider.go`

### Change
Add explicit zero-value guard and fallback after parsing the TWD rate:

```go
twdRate, ok := rates["TWD"]
if !ok || twdRate == 0 {
    logging.Err("frankfurter: TWD rate missing or zero, using previous value")
    // Return previous snapshot value or skip update
    return prev, fmt.Errorf("frankfurter: TWD rate zero or missing")
}
```

Also add HTTP response status check before JSON decode:

```go
if resp.StatusCode != http.StatusOK {
    return prev, fmt.Errorf("frankfurter: unexpected status %d", resp.StatusCode)
}
```

### Dependencies
None — standalone change.

### Test Plan
```bash
go test -v -run TestFrankfurterFXProvider ./internal/marketdata/...
# Add table-driven test: mock response with TWD=0 → expect error returned, prev value preserved
```

### QA Verification
```bash
curl -s http://localhost:8080/api/macro/snapshot | jq '.usd_twd.value'
# Expected: non-zero (e.g., ~32.x TWD per USD)

# Verify Frankfurter API directly
curl -s "https://api.frankfurter.app/latest?from=USD&to=JPY,TWD" | jq '.rates.TWD'
```

---

## Fix 5 — `HandleSeasonalAnalysis` Uses Static Defaults

### File
`internal/monitoring/api/narrative/handlers.go`

### Change
Replace static `DefaultSeasonalPatterns()` call with live detection:

```go
// Before (approximate)
patterns := industry.DefaultSeasonalPatterns()

// After
patterns, err := h.seasonalEngine.DetectCurrentPatterns(time.Now())
if err != nil {
    logging.Err("seasonal: DetectCurrentPatterns failed, using defaults: " + err.Error())
    patterns = industry.DefaultSeasonalPatterns()
}
```

> Confirm exact handler method name and `seasonalEngine` field availability. If `SeasonalEngine` is not injected into the handler, add it via constructor injection following existing handler patterns.

### Dependencies
None — standalone change.

### Test Plan
```bash
go test -v -run TestHandleSeasonalAnalysis ./internal/monitoring/...
go test -v -run TestDetectCurrentPatterns ./internal/industry/...
```

### QA Verification
```bash
curl -s http://localhost:8080/api/narrative/seasonal | jq '.patterns[].avg_market_return'
# Expected: non-zero values (e.g., dividend_season ~0.025, not -0.0%)
```

---

## Parallel Execution Plan

```
Wave 1 (independent, run simultaneously):
  ├── Fix 1: ingestor.go mergeWithPrev()         [~15 min]
  ├── Fix 3: dashboard_api.go storageDir          [~15 min]
  ├── Fix 4: frankfurter_provider.go zero guard   [~20 min]
  └── Fix 5: handlers.go DetectCurrentPatterns    [~20 min]

Wave 2 (depends on Fix 1):
  └── Fix 2: knowledge_base.go UpdateMacro()      [~30 min]
             + ingestor.go call site
```

---

## Execution Checklist

```bash
# 1. Create feature branch
git checkout main && git pull origin main
git checkout -b fix/dashboard-data-quality

# 2. Apply Wave 1 fixes (can be done in parallel)
# Fix 1: internal/narrative/ingestor.go
# Fix 3: internal/monitoring/dashboard_api.go
# Fix 4: internal/marketdata/frankfurter_provider.go
# Fix 5: internal/monitoring/api/narrative/handlers.go

# 3. Apply Wave 2 fix
# Fix 2: internal/narrative/knowledge_base.go + ingestor.go

# 4. CI checks
test -z "$(gofmt -l .)"
go build ./...
go test ./...
go vet ./...
staticcheck ./...

# 5. Coverage check
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
# Must be >= 40%

# 6. Push and PR
git push -u origin fix/dashboard-data-quality
gh pr create --title "fix(narrative): resolve 5 dashboard data quality bugs" \
  --body "## Summary
- Fix mergeWithPrev() missing RetailShortBalance and BDI fields
- Add UpdateMacro() to KnowledgeBase; call from ingestor after ingestion
- Pass real storageDir to TWSEMarginBalanceProvider
- Add zero-value guard to FrankfurterFXProvider USD/TWD
- Call DetectCurrentPatterns() in HandleSeasonalAnalysis instead of static defaults

## Test Results
- go build ./... ✅
- go test ./... ✅
- Coverage >= 40% ✅

## Risk
Low — all fixes are additive or guard-clause additions. No API contract changes." \
  --base main
```

---

## Acceptance Criteria

| Fix | Command | Expected |
|-----|---------|----------|
| 1 | `curl .../api/macro/snapshot \| jq '.retail_short_balance.value'` | Non-zero |
| 1 | `curl .../api/macro/snapshot \| jq '.bdi.value'` | Non-zero |
| 2 | `curl .../api/narrative/stress-index/current \| jq '.stress_index'` | ~37 (matches `/api/taiwan/stress-index`) |
| 3 | `ls data/marketdata/margin/` | Directory exists with history files |
| 4 | `curl .../api/macro/snapshot \| jq '.usd_twd.value'` | ~32.x |
| 5 | `curl .../api/narrative/seasonal \| jq '.patterns[].avg_market_return'` | Non-zero values |

---

## Files Modified (Summary)

| File | Fix # | Change Type |
|------|-------|-------------|
| `internal/narrative/ingestor.go` | 1, 2 | Add 2 merge cases; add `UpdateMacro()` call |
| `internal/narrative/knowledge_base.go` | 2 | Add `UpdateMacro()` method |
| `internal/monitoring/dashboard_api.go` | 3 | Pass real `storageDir` |
| `internal/marketdata/frankfurter_provider.go` | 4 | Add zero-value guard + HTTP status check |
| `internal/monitoring/api/narrative/handlers.go` | 5 | Call `DetectCurrentPatterns()` |

