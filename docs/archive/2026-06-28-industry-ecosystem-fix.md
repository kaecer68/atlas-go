# Industry Ecosystem Audit Fix — Implementation Plan

> **Created**: 2026-05-18
> **Scope**: Fix 10 defects (D1–D10) in `internal/industry/` across 4 sub-modules
> **Priority**: P0 (critical) → P1 (important) → P2 (improvement)
> **Status**: Plan ready, awaiting approval to execute

---

## Overview

After the macro-narrative module upgrade (new data sources, `ConfidenceSource`, `HitRate`, `SeasonalBridge`), the industry ecosystem's 4 sub-modules have accumulated defects in:
- **Validation basis** (hardcoded thresholds, dead calibration code, data duplication)
- **Computation logic** (correlation clamp error, negative-growth confidence inversion, static correlation matrix)

### Sub-module Map

| Sub-module | Files | P0 | P1 | P2 |
|---|---|---|---|---|
| Supply Chain Linkage | `linkage.go`, `correlation_loader.go` | D7 | D8 | — |
| Cycle Compass | `cycle.go`, `threshold_calibrator.go`, `sector_data_bridge.go` | D5, D2 | D3, D4 | — |
| Seasonal Patterns | `seasonality.go` | — | — | D9 |
| Industry Map | `types.go`, `classification.go` | — | — | D1, D6, D10 |

---

## Phase 1: P0 — Critical Fixes (Must Fix First)

### Fix D7: `getNarrativeAdjustedCorrelation` Clamp Error

**File**: `internal/industry/linkage.go:389`
**Severity**: CRITICAL — Portfolio optimization computes wrong hedge ratios
**Root Cause**: `math.Max(0, math.Min(1.0, adjusted))` clamps correlation to [0, 1], zeroing out negative correlations. Defensive assets (bonds, gold, yen) that are negatively correlated with equities lose their hedging benefit in portfolio optimization.

**Current Code**:
```go
return math.Max(0, math.Min(1.0, adjusted))
```

**Fix**:
```go
return math.Max(-1.0, math.Min(1.0, adjusted))
```

**Verification**:
1. Update `linkage_test.go` to include negative correlation test cases
2. Run `go test ./internal/industry/... -run TestShockPropagation`
3. Verify negative correlations survive narrative adjustment

**Risk**: Low — simple constant change. `CorrelationMatrix.GetCorrelation` already stores raw values in [-1,1]. The clamp was the only place that destroyed negative values.

---

### Fix D5: `calculateConfidence` Negative-Growth Logic Inversion

**File**: `internal/industry/cycle.go:399-401`
**Severity**: CRITICAL — Recession signals suppressed; cycle phase judgments unreliable
**Root Cause**: When revenue or profit growth is negative, the `boundary` contribution (how far metrics are from phase thresholds) is halved via `boundary *= 0.5`. The logic assumes: negative growth = bad data quality → lower confidence. In reality, strongly negative growth is unambiguous proof of contraction — it should NOT reduce confidence.

**Current Code**:
```go
if metrics.RevenueGrowthYoY < 0 || metrics.ProfitGrowthYoY < 0 {
    boundary *= 0.5
}
```

**Fix**: Replace the penalization with differentiated treatment:
```go
if metrics.RevenueGrowthYoY < 0 || metrics.ProfitGrowthYoY < 0 {
    // Negative growth = contraction; high boundary confidence means
    // the decline is unambiguous — it should NOT reduce confidence.
    if boundary > 0.7 {
        // Strong boundary confidence in contraction territory → maintain it
        boundary = math.Max(boundary, 0.8)
    }
    // If boundary is already low (near phase transition), keep it low
}
```

Or simpler, more defensible approach:
```go
// Remove the penalty entirely and let the signal portion carry the weight.
// Negative growth is unambiguous evidence, not low-quality data.
// boundary already correctly measures distance from thresholds.
```

**Alternative approach**: Instead of modifying boundary, add a `contractionStrength` multiplier based on magnitude of negative growth:
```go
if metrics.RevenueGrowthYoY < 0 || metrics.ProfitGrowthYoY < 0 {
    // Negative growth = strong contraction signal
    // Strengthen confidence proportional to magnitude of decline
    contractionMagnitude := math.Min(math.Abs(metrics.RevenueGrowthYoY), 1.0)
    signal += contractionMagnitude * cfgSignal.ContractionWeight // new param
}
```

**Recommendation**: Remove the `boundary *= 0.5` penalty. The `boundary` metric already correctly reflects unambiguous positioning. Strongly negative values are far from positive thresholds → high boundary — that's correct behavior.

**Verification**:
1. Unit test: verify confidence does NOT decrease when revenue goes from +1% to -1%
2. Unit test: verify confidence INCREASES when growth goes from -5% to -20% (stronger recession = higher certainty it IS recession)
3. Run `go test ./internal/industry/... -run TestConfidence`

**Risk**: Low — removes a logically invalid penalty. The signal portion already encodes data quality through normalization. Boundary confidence is correctly computed as distance from thresholds.

---

### Fix D2: `sector_data_bridge.go` Data Duplication

**File**: `internal/industry/sector_data_bridge.go:41-44`
**Severity**: CRITICAL — Semiconductor data polluted by AI supply chain revenue
**Root Cause**: Lines 41-42 reuse the `metrics` struct (with `IndustryID` set to `"semiconductor"`) but the `RevenueGrowthYoY` and `CapacityUtilization` values still contain AI supply chain data (TSMC revenue = AI-specific, not overall semiconductor).

**Current Code**:
```go
metrics.IndustryID = "semiconductor"
tracker.UpdatePosition("semiconductor", metrics)
```

**Fix**: Semiconductor needs its own data source. Options:
1. **Immediate fix**: Remove the semiconductor line entirely (return to default initialized values) — better to have neutral defaults than wrong data
2. **Proper fix**: Add a separate data field in `MacroDataSnapshot` for semiconductor industry metrics (e.g., `SemiconductorRevenue`) or compute semiconductor-specific metrics from TWSE semiconductor index data

For P0, use option 1 (remove duplicate + add TODO):
```go
// TODO(P1): Add proper semiconductor data field to MacroDataSnapshot.
// Current sector_data.json only provides TSMC-specific data (ai_supply_chain),
// which is incorrectly propagated to the broader semiconductor industry.
// See: https://github.com/kaecer68/atlas-go/issues/XXX
```

And keep only the `ai_supply_chain` update.

**Verification**:
1. Check that `semiconductor` position uses `initializeDefaultPositions()` values, not AI chain values
2. Run `go build ./...` to ensure no compilation errors
3. Add comment explaining why semiconductor is excluded

**Risk**: Low for P0 — returns to pre-bridge behavior for semiconductor. A proper fix (P1+) requires new data source.

---

## Phase 2: P1 — Important Fixes

### Fix D4: Enable `threshold_calibrator` Results in CycleTracker

**File**: `internal/industry/threshold_calibrator.go` + `internal/industry/cycle.go`
**Severity**: IMPORTANT — Revenue thresholds are static hardcoded values despite having empirical calibration pipeline
**Root Cause**: `CalibrateThresholdsFromFile()` produces P25/P50/P75 from real data but `detectBusinessCycle()` in `cycle.go` still uses hardcoded `CycleThresholdConfig` defaults (+20%/-5%).

**Fix**:
1. Add a `SetCalibratedThresholds([]CalibrationResult)` method to `CycleTracker`
2. In `detectBusinessCycle()`, use calibrated thresholds when available, fall back to config defaults
3. Add a periodic recalibration trigger (e.g., via `BackgroundTaskManager`)

```go
// New method on CycleTracker
func (ct *CycleTracker) SetCalibratedThresholds(results []CalibrationResult) {
    ct.mu.Lock()
    defer ct.mu.Unlock()
    for _, r := range results {
        if ct.calibratedThresholds == nil {
            ct.calibratedThresholds = make(map[string]CalibrationResult)
        }
        ct.calibratedThresholds[r.IndustryID] = r
    }
}
```

In `detectBusinessCycle()`:
```go
// Check for calibrated thresholds first
if ct.calibratedThresholds != nil {
    if cal, ok := ct.calibratedThresholds[metrics.IndustryID]; ok && cal.SampleSize >= 30 {
        // Use P25/P50/P75 as expansion/recovery/mature thresholds
        thresholds = config.CycleThresholdConfig{
            ExpansionRevenuePct: cal.P75,
            ExpansionProfitPct:  cal.P75,
            RecoveryRevenuePct:  cal.P50,
            RecoveryProfitPct:   cal.P50,
            MatureRevenuePct:    cal.P25,
            MatureProfitPct:     cal.P25,
        }
    }
}
```

**Verification**:
1. Write test with mock CalibrationResult
2. Verify calibrated thresholds override defaults
3. Run `go test ./internal/industry/... -run TestThresholdCalibration`

---

### Fix D3: Add Leading Indicators to `detectBusinessCycle` + Connect to Narrative

**File**: `internal/industry/cycle.go`
**Severity**: IMPORTANT — Cycle detection relies solely on lagging indicators (revenue/profit YoY), ignoring PMI, Book-to-Bill ratio, and macro-narrative events

**Fix**:
1. Extend `IndustryMetrics` to include PMI (float64, optional) and BookToBill (float64, optional)
2. Add a `MacroIndicatorProvider` interface for injecting external macro data
3. In `detectBusinessCycle()`, blend leading indicators with current revenue-based detection:

```go
// MacroIndicatorProvider provides leading macro indicators
type MacroIndicatorProvider interface {
    PMI() float64            // 0-100 scale
    BookToBill() float64     // ratio
    LeadingIndex() float64   // composite leading index
}
```

4. Add a `leadingSignal` that can upgrade/downgrade cycle phase when leading indicators strongly disagree with lagging indicators:

```go
func (ct *CycleTracker) computeLeadingSignal(industryID string) (float64, bool) {
    if ct.macroProvider == nil {
        return 0, false
    }
    pmi := ct.macroProvider.PMI()
    btb := ct.macroProvider.BookToBill()
    // PMI > 50 = expansion, < 50 = contraction
    // BtB > 1.0 = expansion, < 1.0 = contraction
    signal := 0.0
    weight := 0.0
    if pmi > 0 {
        signal += (pmi - 50) / 50.0 * 0.6
        weight += 0.6
    }
    if btb > 0 {
        signal += (btb - 1.0) * 0.4
        weight += 0.4
    }
    if weight == 0 {
        return 0, false
    }
    return signal / weight, true
}
```

5. Use leading signal to validate or challenge the revenue-based detection:
   - If revenue-based says Expansion but PMI < 45 and BtB < 0.9 → downgrade to Recovery (leading indicators warning)
   - If revenue-based says Recession but PMI > 55 and BtB > 1.1 → upgrade to Recovery (leading indicators suggesting turnaround)

6. Connect `narrativeAdjust` influence to `detectBusinessCycle` more explicitly — the current code at line 317-324 already adjusts revenue/profit by narrative bias, but this should be logged and visibly reported in `CyclePosition`.

**Verification**:
1. Test: revenue says Expansion, PMI says contraction → phase should be Recovery (not Expansion)
2. Test: revenue says Recession, PMI says expansion → phase should be Recovery (not Recession)
3. Run `go test ./internal/industry/... -run TestCycle`

**Risk**: Medium — adds new dependency on macro data availability. Must be designed with graceful degradation when PMI/BtB not available (return current behavior).

---

### Fix D8: Implement Regime-Conditional Correlation

**File**: `internal/industry/correlation_loader.go` + `internal/industry/linkage.go`
**Severity**: IMPORTANT — Static correlation matrix underestimates systemic risk during crises when correlations spike

**Current State**: 
- `RegimeAdjustedCorrelation()` already exists and boosts correlation during Recession by `RecessionCorrelationBoost` (line 258-264)
- BUT: Only boosts UPWARD. No mechanism to reduce correlations during normal/low-stress regimes
- Minimum observation threshold is 15 (line 287, 290) — too low for statistical significance

**Fix**:
1. **Multi-regime correlation**: Extend `RegimeAdjustedCorrelation()` to handle all 4 cycle phases, not just Recession:
   ```go
   switch {
   case phaseA == CycleRecession || phaseB == CycleRecession:
       return math.Min(baseCorr*(1.0+boost), 1.0) // Crisis: correlations rise
   case phaseA == CycleExpansion && phaseB == CycleExpansion:
       return baseCorr * 0.85 // Both expanding: correlations moderate
   default:
       return baseCorr
   }
   ```

2. **Increase minimum observation threshold**: Change from 15 to 60:
   ```go
   if n < 60 { // was 15
       continue
   }
   ```

3. **Add observation count to CorrelationMatrix** for transparency:
   ```go
   type CorrelationMatrix struct {
       correlations  map[string]map[string]float64
       observations  map[string]map[string]int // NEW: per-pair observation count
       window        int
       ...
   }
   ```

**Verification**:
1. Test: same pair in Recession → correlation boosted
2. Test: same pair in Expansion → correlation NOT boosted (or slightly dampened)
3. Test: < 60 observations → pair skipped
4. Run `go test ./internal/industry/... -run TestCorrelation`

**Risk**: Low for the regime extension (already partially implemented). Medium for the observation threshold change — verify that existing datasets still produce sufficient pairs.

---

## Phase 3: P2 — Improvement Fixes

### Fix D9: `detectThemeDirection` Disallow Silent Heuristic Fallback

**File**: `internal/industry/seasonality.go:562-573`
**Severity**: IMPROVEMENT — When `dynamicEnv == nil`, the function silently falls back to heuristic hardcoded values without any warning

**Current Code**:
```go
func (se *SeasonalEngine) detectThemeDirection(theme string) float64 {
    if se.dynamicEnv == nil {
        // Fallback to heuristic when no macro data available
        switch theme { ... }
    }
    ...
}
```

**Fix**:
1. Add logging warning when falling back to heuristic:
   ```go
   if se.dynamicEnv == nil {
       logging.Warn("seasonality", "detect_theme_no_dynamic_env",
           "theme", theme,
           "message", "Using heuristic fallback; connect DynamicEnvModulator for macro-aware detection")
       // Fallback to heuristic...
   }
   ```

2. Add an `evidence_quality` tag to the return value or to the adjustment:
   ```go
   type ThemeDirection struct {
       Direction      float64
       EvidenceSource string // "macro_data" or "heuristic"
   }
   ```

**Verification**:
1. Verify warning is logged when dynamicEnv is nil
2. Run `go test ./internal/industry/... -run TestSeasonal`

---

### Fix D1, D6, D10: Minor Documentation & Consistency

- **D1**: `detectBusinessCycle` parameter documentation — clarify that `narrativeAdjust` function is optional and how it interacts with threshold detection
- **D6**: Add `IndustryMetrics.LeadingIndicator` field documentation to distinguish from lagging indicators
- **D10**: `RepresentativeStocks` in `IndustrySegment` should be dynamically refreshable from market data instead of static JSON config

---

## Execution Order

### Wave 1 (P0 — Independent, can be done in parallel)
1. **D7**: `linkage.go` clamp fix (1 line change + test update)
2. **D5**: `cycle.go` confidence logic fix (~10 line change + tests)
3. **D2**: `sector_data_bridge.go` remove semiconductor duplication (delete 3 lines + add TODO)

### Wave 2 (P1 — After P0 verified)
4. **D4**: Enable calibrated thresholds in CycleTracker (new method + integration)
5. **D3**: Add leading indicators + narrative connection (new interface + detection logic)
6. **D8**: Multi-regime correlation + observation threshold (extend existing function)

### Wave 3 (P2 — After P1 verified)
7. **D9**: Add logging + evidence_source to detectThemeDirection
8. **D1, D6, D10**: Documentation + static→dynamic representative stocks

---

## Test Strategy

| Phase | Test Command | Expected |
|---|---|---|
| Pre-fix baseline | `go test ./internal/industry/...` | Verify current state |
| After Wave 1 | `go test ./internal/industry/... -run "TestShockPropagation|TestConfidence"` | All pass |
| After Wave 2 | `go test ./internal/industry/...` | All pass, no regressions |
| Final | `go build ./... && go test ./... && go vet ./...` | Clean CI |

---

## Rollback Plan

Each fix is atomic and independently revertible:
- D7: Revert one line in `linkage.go`
- D5: Revert confidence calculation in `cycle.go`
- D2: Revert 3 lines in `sector_data_bridge.go`
- D4: Remove `SetCalibratedThresholds` call — falls back to config defaults
- D3: Set `macroProvider` to nil — falls back to current behavior
- D8: Uses existing `SetCycleProvider(nil)` — falls back to raw correlations

---

## Open Questions

1. **D2 full fix**: Where should semiconductor-specific metrics come from? TWSE semiconductor sub-index? Revenue aggregate of all semiconductor stocks in `sector_symbols.json`?
2. **D3 data source**: Is PMI data available through existing `marketdata.Provider`? Or does it need a new data source?
3. **D8 observation threshold**: Does the current replay dataset have 60+ observations for most industry pairs? Need to verify before changing from 15→60.

---

## Constraints

- No changes to public API signatures without explicit approval
- All fixes must be backward-compatible (old clients calling existing methods without narrative/cycle providers must still work)
- Must pass `gofmt -l .`, `go build ./...`, `go test ./...`, `go vet ./...`
- Follow Go coding conventions from `.github/instructions/go-core.instructions.md`
