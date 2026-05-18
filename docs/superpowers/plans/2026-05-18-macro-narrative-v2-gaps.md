# Macro Narrative v2.0 — Gap Implementation Plan

**Date**: 2026-05-18  
**Author**: Metis pre-planning analysis  
**Scope**: 10 identified gaps — 3 P0 bugs, 4 P1 architecture, 3 P2 quality  
**Branch**: `fix/macro-narrative-v2-gaps`

---

## File Map

| Action | File | Reason |
|--------|------|--------|
| MODIFY | `internal/narrative/ingestor.go` | P0: confidence overwrite bug |
| MODIFY | `internal/narrative/weight_calibration.go` | P0: double normalization + P1: OutflowTarget |
| MODIFY | `configs/stress_index_weights.json` | P0: add oil/gold factors |
| MODIFY | `internal/narrative/taiwan_stress_index.go` | P1: remove hardcoded constants |
| MODIFY | `internal/config/parameters_defaults.go` | P1: add stress index defaults |
| MODIFY | `configs/parameters.json` | P1: add stress_index group |
| MODIFY | `internal/monitoring/api/narrative/handlers.go` | P1: unify stress index API endpoints |
| MODIFY | `internal/apigateway/background.go` | P1: register margin history backfill |
| MODIFY | `internal/narrative/validation_framework.go` | P2: fix future dates + unreasonable values |
| CREATE | `internal/narrative/auc_roc.go` | P2: AUC-ROC calculation |
| CREATE | `internal/narrative/auc_roc_test.go` | P2: AUC-ROC tests |
| MODIFY | `internal/narrative/taiwan_stress_index.go` | P2: remove engine.json dual-source |

---

## P0 — Bugs (Fix First, No Behavior Changes Elsewhere)

### P0-1: Confidence Overwrite Bug (`ingestor.go:104`)

**Problem**: When a theme is already active, `UpdateConfidence` unconditionally overwrites the existing confidence with the new event's value. If the new event has lower confidence, this silently degrades the active event.

**Fix**: Use `math.Max` to only update if the new confidence is higher.

**File**: `internal/narrative/ingestor.go`

```go
// BEFORE (line ~104):
m.lifecycle.UpdateConfidence(existing.ID, e.Confidence)

// AFTER:
if e.Confidence > existing.Confidence {
    m.lifecycle.UpdateConfidence(existing.ID, e.Confidence)
}
```

> Note: `existing` is a `*NarrativeEvent` returned by `GetActiveByTheme`. Verify the struct has a `Confidence float64` field before applying.

**Test command**:
```bash
go test ./internal/narrative/... -run TestPublishEvents_ConfidenceNotDowngraded -v
```

**New test** (add to `ingestor_test.go`):
```go
func TestPublishEvents_ConfidenceNotDowngraded(t *testing.T) {
    // Setup: inject lifecycle with an active event at confidence 0.9
    // Publish a new event for the same theme at confidence 0.5
    // Assert: lifecycle confidence remains 0.9, not overwritten to 0.5
}
```

**Expected output**: `PASS`

---

### P0-2: Double Normalization in `CalibrateWeights` + `ExportConfig`

**Problem**: `CalibrateWeights()` already calls `normalizeWeights()` internally. `ExportConfig()` then calls `normalizeWeights()` again on the result, causing weights to drift from their calibrated values (e.g., a weight of 0.22 becomes 0.22/sum_of_already_normalized ≈ 0.22 again only if sum=1.0 exactly, but floating-point drift accumulates over multiple export cycles).

**Fix**: `ExportConfig` should trust the weights it receives and NOT re-normalize.

**File**: `internal/narrative/weight_calibration.go`

```go
// BEFORE (line ~205):
cfg := StressIndexWeightsConfig{Scaling: scaling, Weights: normalizeWeights(weights), Thresholds: thresholds}

// AFTER:
cfg := StressIndexWeightsConfig{Scaling: scaling, Weights: weights, Thresholds: thresholds}
```

**Test command**:
```bash
go test ./internal/narrative/... -run TestExportConfig_NoDoubleNormalization -v
```

**New test** (add to `weight_calibration_test.go`):
```go
func TestExportConfig_NoDoubleNormalization(t *testing.T) {
    // Calibrate weights → get result W (already normalized, sum=1.0)
    // ExportConfig(tmpDir, W, ...)
    // Read back the JSON
    // Assert: each weight in JSON matches W exactly (within 1e-9 tolerance)
}
```

**Expected output**: `PASS`

---

### P0-3: `configs/stress_index_weights.json` Missing Oil/Gold

**Problem**: The JSON config has 6 factors (dxy, us10y, foreign_flow, vix, jpy, geopolitical). The Go calculator (`taiwan_stress_index.go`) expects 8 factors including oil and gold. When the config is loaded, oil/gold weights default to 0, making the calculator silently ignore those factors.

**Fix**: Add oil and gold to the JSON config. Rebalance weights to sum to 1.00.

**File**: `configs/stress_index_weights.json`

```json
{
  "description": "外資出逃壓力指數八因子權重與縮放係數。所有權重總和必須為 1.00。",
  "version": "1.1.0",
  "last_updated": "2026-05-18",

  "scaling": {
    "dxy":          5.0,
    "us10y":        2.0,
    "foreign_flow": 10.0,
    "vix":          2.5,
    "jpy":          10.0,
    "geopolitical": 1.0,
    "oil":          2.0,
    "gold":         2.0
  },

  "weights": {
    "dxy":          0.13,
    "us10y":        0.18,
    "foreign_flow": 0.22,
    "vix":          0.13,
    "jpy":          0.08,
    "geopolitical": 0.13,
    "oil":          0.07,
    "gold":         0.06
  },

  "thresholds": {
    "crisis": 70.0,
    "high":   50.0,
    "alert":  30.0
  }
}
```

> Weight rationale: oil (0.07) and gold (0.06) sourced from existing Go constants `stressWeightOil`/`stressWeightGold`. Existing 6-factor weights scaled down proportionally to maintain sum=1.00.

**Verification**:
```bash
# Confirm weights sum to 1.00
python3 -c "
import json, sys
d = json.load(open('configs/stress_index_weights.json'))
s = sum(d['weights'].values())
print(f'sum={s:.6f}')
assert abs(s - 1.0) < 1e-9, 'weights do not sum to 1.0'
print('OK')
"
```

**Test command**:
```bash
go test ./internal/narrative/... -run TestLoadStressIndexWeights_HasOilGold -v
```

**New test** (add to `taiwan_stress_index_test.go`):
```go
func TestLoadStressIndexWeights_HasOilGold(t *testing.T) {
    cfg, err := LoadStressIndexWeightsFromFile("../../configs/stress_index_weights.json")
    if err != nil {
        t.Fatalf("load: %v", err)
    }
    if cfg.Weights.Oil == 0 {
        t.Error("oil weight is 0 — missing from config")
    }
    if cfg.Weights.Gold == 0 {
        t.Error("gold weight is 0 — missing from config")
    }
}
```

---

## P1 — Architecture

### P1-1: Migrate Hardcoded Constants to `parameters.json`

**Problem**: `taiwan_stress_index.go` lines 44-62 define 16 constants (8 scale factors + 8 weights + 3 thresholds) as Go `const`. These cannot be tuned without recompiling.

**Step 1**: Add defaults to `internal/config/parameters_defaults.go`:

```go
// In the stress_index group section:
"stress_index.scale.dxy":           {Value: 5.0,   Min: 0.1, Max: 50.0, Description: "DXY 縮放係數"},
"stress_index.scale.us10y":         {Value: 2.0,   Min: 0.1, Max: 20.0, Description: "US10Y 縮放係數"},
"stress_index.scale.foreign_flow":  {Value: 10.0,  Min: 0.1, Max: 100.0, Description: "外資流向縮放係數"},
"stress_index.scale.vix":           {Value: 2.5,   Min: 0.1, Max: 10.0, Description: "VIX 縮放係數"},
"stress_index.scale.jpy":           {Value: 10.0,  Min: 0.1, Max: 100.0, Description: "JPY 縮放係數"},
"stress_index.scale.geopolitical":  {Value: 1.0,   Min: 0.1, Max: 10.0, Description: "地緣風險縮放係數"},
"stress_index.scale.oil":           {Value: 2.0,   Min: 0.1, Max: 20.0, Description: "原油縮放係數"},
"stress_index.scale.gold":          {Value: 2.0,   Min: 0.1, Max: 20.0, Description: "黃金縮放係數"},
"stress_index.threshold.crisis":    {Value: 70.0,  Min: 50.0, Max: 100.0, Description: "危機閾值"},
"stress_index.threshold.high":      {Value: 50.0,  Min: 30.0, Max: 80.0,  Description: "高壓閾值"},
"stress_index.threshold.alert":     {Value: 30.0,  Min: 10.0, Max: 60.0,  Description: "警示閾值"},
```

**Step 2**: Add `stress_index` group to `configs/parameters.json` (follow existing group format, e.g., `industry.linkage_params`).

**Step 3**: Modify `TaiwanStressCalculator` to accept a config struct instead of using constants:

```go
// BEFORE: calculator uses package-level const
score += stressScaleDXY * snap.DXYChange * stressWeightDXY

// AFTER: calculator uses injected config
score += cfg.Scaling.DXY * snap.DXYChange * cfg.Weights.DXY
```

> The `StressIndexWeightsConfig` struct (already used by `weight_calibration.go`) already has `Scaling` and `Weights` fields — reuse it.

**Step 4**: Update `NewTaiwanStressCalculator` constructor to load from `parameters.json` via `config.GetFloat64("stress_index.scale.dxy")` etc., falling back to the existing JSON file.

**Test command**:
```bash
go test ./internal/narrative/... -run TestTaiwanStressCalculator_UsesConfigNotConst -v
go build ./...
```

---

### P1-2: `OutflowTarget` Should Use t+5 Forecast

**Problem**: `weight_calibration.go` line 100 sets `OutflowTarget: outflow` where `outflow = -foreignNet` (same-day value). This means the calibration optimizes weights to predict today's outflow using today's data — a trivial in-sample fit, not a useful predictive model.

**Fix**: When building `CalibrationRecord`, look ahead 5 records to set `OutflowTarget`.

**File**: `internal/narrative/weight_calibration.go`

```go
// AFTER building the full records slice, apply t+5 targets:
for i := range records {
    if i+5 < len(records) {
        records[i].OutflowTarget = records[i+5].Outflow
    } else {
        // For the last 5 records, use same-day as fallback
        records[i].OutflowTarget = records[i].Outflow
    }
}
```

> This must happen AFTER the full `records` slice is built, not during construction.

**Test command**:
```bash
go test ./internal/narrative/... -run TestCalibrationRecords_OutflowTargetIsT5 -v
```

**New test**:
```go
func TestCalibrationRecords_OutflowTargetIsT5(t *testing.T) {
    // Build 10 synthetic records with known outflow values [0,1,2,...,9]
    // Apply t+5 logic
    // Assert: records[0].OutflowTarget == 5, records[1].OutflowTarget == 6, ...
    // Assert: records[7].OutflowTarget == records[7].Outflow (fallback)
}
```

---

### P1-3: Unify Stress Index API Endpoints

**Problem**: Narrative routes exist in `internal/monitoring/api/narrative/handlers.go` but stress index endpoints are not consistently registered. Callers may hit 404 for `/api/narrative/stress-index/current` or `/api/narrative/stress-index/history`.

**Action**: Audit `handlers.go` and ensure these routes are registered:

| Method | Path | Handler |
|--------|------|---------|
| GET | `/api/narrative/stress-index/current` | `HandleStressIndexCurrent` |
| GET | `/api/narrative/stress-index/history` | `HandleStressIndexHistory` |
| GET | `/api/narrative/stress-index/thresholds` | `HandleStressIndexThresholds` |

**Verification**:
```bash
# Start server in test mode, then:
curl -s http://localhost:8080/api/narrative/stress-index/current | jq '.score'
# Expected: a float64 value, not a 404
```

---

### P1-4: Register Margin History Backfill in Auto-Scheduler

**Problem**: `internal/apigateway/background.go` (`BackgroundTaskManager`) does not register a margin history backfill task. When the system starts fresh, margin history is empty, causing stress index calculations that depend on it to use zero/fallback values.

**Action**: Add a backfill task registration in `background.go`:

```go
// In the task registration section, after existing tasks:
m.RegisterTask(BackgroundTask{
    Name:     "margin-history-backfill",
    Interval: 24 * time.Hour,
    Jitter:   5 * time.Minute,
    Run: func(ctx context.Context) error {
        return m.deps.MarginHistoryBackfiller.Backfill(ctx)
    },
})
```

> Requires `MarginHistoryBackfiller` interface to be defined and injected. If the interface doesn't exist yet, create it in `internal/narrative/` following the existing provider pattern.

**Test command**:
```bash
go test ./internal/apigateway/... -run TestBackgroundTaskManager_RegistersMarginBackfill -v
```

---

## P2 — Quality

### P2-1: Fix Validation Framework Test Cases

**Problem**: `validation_framework.go` has two issues:
1. `"US-Iran Tensions with Oil Spike"` uses date `"2026-03-15"` — a future date relative to when the framework was written, making it non-historical.
2. Several cases use `DXY: 20.0` and `US10Y: 50.0` simultaneously — these represent extreme outlier values that may not reflect realistic co-movement.

**Fix**:
- Replace `"2026-03-15"` with a real historical date (e.g., `"2019-09-14"` — Saudi Aramco drone attack).
- Cap `US10Y` values at realistic levels (e.g., max 5.5 for absolute yield, or use change-in-bps if that's the intended unit).
- Add a comment clarifying units for each field.

**File**: `internal/narrative/validation_framework.go`

```go
// Add unit comments to struct:
type StressEventTestCase struct {
    // ...
    DXY          float64 // DXY index change (%), e.g. 2.5 = 2.5% appreciation
    US10Y        float64 // US 10Y yield in basis points change, e.g. 50 = +50bps
    VIX          float64 // VIX absolute level, e.g. 35.0
    ForeignFlow  float64 // Net foreign flow in TWD billions, negative = outflow
    // ...
}
```

**Test command**:
```bash
go test ./internal/narrative/... -run TestDefaultStressEventTestCases_NoDatesInFuture -v
```

**New test**:
```go
func TestDefaultStressEventTestCases_NoDatesInFuture(t *testing.T) {
    today := time.Now().Format("2006-01-02")
    for _, tc := range DefaultStressEventTestCases() {
        if tc.Date > today {
            t.Errorf("case %q has future date %s", tc.Name, tc.Date)
        }
    }
}
```

---

### P2-2: Add AUC-ROC Calculation

**Problem**: The calibration engine (`ComputeFactorAccuracy`) returns per-factor accuracy as a simple hit rate. There is no AUC-ROC metric, making it impossible to evaluate the model's discrimination ability across thresholds.

**New file**: `internal/narrative/auc_roc.go`

```go
package narrative

import "sort"

// AUCROCResult holds the AUC-ROC score and supporting data.
type AUCROCResult struct {
    AUC        float64
    ThresholdN int
}

// ComputeAUCROC computes the AUC-ROC for binary classification.
// labels: true=1 (outflow event), false=0 (no event)
// scores: predicted probability or stress score (higher = more likely outflow)
func ComputeAUCROC(labels []bool, scores []float64) AUCROCResult {
    if len(labels) != len(scores) || len(labels) == 0 {
        return AUCROCResult{}
    }

    type pair struct {
        score float64
        label bool
    }
    pairs := make([]pair, len(labels))
    for i := range labels {
        pairs[i] = pair{scores[i], labels[i]}
    }
    sort.Slice(pairs, func(i, j int) bool {
        return pairs[i].score > pairs[j].score
    })

    var tp, fp, prevTP, prevFP float64
    var auc float64
    for _, p := range pairs {
        if p.label {
            tp++
        } else {
            fp++
        }
        // Trapezoidal rule
        auc += (fp - prevFP) * (tp + prevTP) / 2
        prevTP, prevFP = tp, fp
    }

    totalPos := tp
    totalNeg := fp
    if totalPos == 0 || totalNeg == 0 {
        return AUCROCResult{AUC: 0.5, ThresholdN: len(labels)}
    }
    return AUCROCResult{
        AUC:        auc / (totalPos * totalNeg),
        ThresholdN: len(labels),
    }
}
```

**New file**: `internal/narrative/auc_roc_test.go`

```go
package narrative

import (
    "math"
    "testing"
)

func TestComputeAUCROC_PerfectClassifier(t *testing.T) {
    labels := []bool{true, true, false, false}
    scores := []float64{0.9, 0.8, 0.2, 0.1}
    result := ComputeAUCROC(labels, scores)
    if math.Abs(result.AUC-1.0) > 1e-9 {
        t.Errorf("expected AUC=1.0, got %.4f", result.AUC)
    }
}

func TestComputeAUCROC_RandomClassifier(t *testing.T) {
    labels := []bool{true, false, true, false}
    scores := []float64{0.5, 0.5, 0.5, 0.5}
    result := ComputeAUCROC(labels, scores)
    if result.AUC < 0.4 || result.AUC > 0.6 {
        t.Errorf("expected AUC≈0.5 for random, got %.4f", result.AUC)
    }
}

func TestComputeAUCROC_EmptyInput(t *testing.T) {
    result := ComputeAUCROC(nil, nil)
    if result.AUC != 0 {
        t.Errorf("expected AUC=0 for empty input, got %.4f", result.AUC)
    }
}
```

**Test command**:
```bash
go test ./internal/narrative/... -run TestComputeAUCROC -v
```

**Expected output**:
```
--- PASS: TestComputeAUCROC_PerfectClassifier
--- PASS: TestComputeAUCROC_RandomClassifier
--- PASS: TestComputeAUCROC_EmptyInput
```

---

### P2-3: Resolve `engine.json` vs `parameters.json` Dual-Source Conflict

**Problem**: Some narrative configuration is read from `engine.json` (if it exists) and some from `parameters.json`. This creates ambiguity about which file is authoritative.

**Action**:
1. Audit all `LoadFrom*` / `ReadConfig*` calls in `internal/narrative/` to identify which files they read.
2. Designate `parameters.json` as the single source of truth for runtime-tunable parameters.
3. If `engine.json` exists, add a deprecation warning log on startup:
   ```go
   if _, err := os.Stat("configs/engine.json"); err == nil {
       logging.Warn("configs/engine.json is deprecated; migrate values to configs/parameters.json")
   }
   ```
4. Document the migration path in a comment at the top of the relevant loader.

**Test command**:
```bash
go test ./internal/narrative/... -v 2>&1 | grep -i "FAIL\|PASS" | tail -5
go build ./...
```

---

## Integration Verification

Run after all tasks are complete:

```bash
# 1. Format check
test -z "$(gofmt -l ./internal/narrative/... ./internal/apigateway/... ./internal/config/...)"

# 2. Build
go build ./...

# 3. Unit tests (narrative package)
go test ./internal/narrative/... -v -count=1 2>&1 | tail -20

# 4. Full test suite with race detector
go test -race ./... 2>&1 | grep -E "FAIL|ok" | tail -20

# 5. Coverage check (must be ≥ 40%)
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total

# 6. Static analysis
go vet ./...
staticcheck ./...

# 7. Governance gates
bash ./scripts/openclaw/verify_governance_gates.sh --require-scenario-diversity
bash ./scripts/openclaw/verify_operations_gate.sh
```

**Expected final state**:
- All tests pass
- Coverage ≥ 40%
- `go vet` and `staticcheck` clean
- `configs/stress_index_weights.json` has 8 factors, weights sum to 1.00
- No `engine.json` deprecation warnings in test output

---

## Execution Order

```
P0-3 (JSON config)  ←  no code deps, do first
P0-1 (confidence)   ←  isolated to ingestor.go
P0-2 (normalization)←  isolated to weight_calibration.go
P1-1 (constants)    ←  depends on P0-3 (config structure)
P1-2 (OutflowTarget)←  isolated to weight_calibration.go
P1-3 (API routes)   ←  isolated to handlers.go
P1-4 (scheduler)    ←  isolated to background.go
P2-1 (test cases)   ←  isolated to validation_framework.go
P2-2 (AUC-ROC)      ←  new files, no deps
P2-3 (dual source)  ←  audit + deprecation warning
```

Total estimated tasks: 10  
Estimated effort: ~4-6 hours for an experienced Go developer familiar with the codebase.
