# OOS Validation Integration Design

**Date:** 2026-04-25
**Status:** Approved
**Approach:** Judge-Integrated Hard Gate (Approach A)

## Summary

Integrate Out-of-Sample (OOS) validation into the Atlas-Go experiment judge pipeline. After a candidate passes in-sample acceptance, it must ALSO pass validation on an unseen forward window before being promoted to `ExperimentAccepted`. This prevents overfitting and ensures commercial rigor.

## Background

Currently, `Judge.Evaluate()` calls `comparePromptPerformanceDetailed()` using in-sample replay data. `OOSValidator` exists as a stub with all methods returning placeholder values. The goal is to make OOS validation a real gate in the experiment lifecycle.

## Design Decisions

### 1. Data Structure

**Decision:** Store OOS results directly in `PromptExperimentResult.OOSResult` (pointer, nullable).

```go
// internal/domain/registry.go
type PromptExperimentResult struct {
    // ... existing fields ...
    OOSResult *OOSResult  // nil when OOS not run (pre-OOS era)
}
```

**Rationale:**
- Single JSON file per experiment keeps data cohesive
- Pointer type allows backwards compatibility
- No external storage or new top-level types needed

**Extended `OOSResult`:**

```go
// internal/experiment/oos_validator.go
type OOSResult struct {
    Passed         bool
    BaselineScore  float64
    CandidateScore float64
    Improvement    float64
    WindowDays     int
    Observations   int
    OOSWindowStart time.Time
    OOSWindowEnd   time.Time
    UsedFallback   bool
    ValidationAt   time.Time  // when OOS was run
    Reason         string     // human-readable result
}
```

### 2. Pipeline Integration

**Decision:** Integrate OOS in `Judge.Evaluate()` AFTER in-sample acceptance but BEFORE status transition.

**Flow:**
```
Judge.Evaluate()
  → comparePromptPerformanceDetailed() [in-sample]
  → passesAcceptance() [in-sample gate]
  → IF accepted: oosValidator.ValidateWithBrief() [OOS gate]
  → IF OOS passes: status = ExperimentAccepted
  → IF OOS fails: status = ExperimentRejected, reason = "OOS validation failed"
```

**Code change in judge.go:**

```go
func (j *Judge) Evaluate(resultPath string) error {
    // ... existing in-sample evaluation ...
    
    accepted, reason := j.passesAcceptance(result, baselineScore, candidateScore)
    
    if accepted {
        oosResult, err := j.oosValidator.ValidateWithBrief(
            result.CandidatePrompt,
            j.baselinePath,
            result.Brief,
            result.Experiment.WindowEnd,
        )
        if err != nil {
            accepted = false
            reason = fmt.Sprintf("OOS validation error: %v", err)
        } else if !oosResult.Passed {
            accepted = false
            reason = fmt.Sprintf("OOS validation failed: %s", oosResult.Reason)
        }
        
        result.OOSResult = oosResult
    }
    
    // ... existing status transition logic ...
}
```

**Rationale:**
- Judge owns all validation (single responsibility)
- No changes to baseline promotion flow
- Clean separation: Judge validates, baseline promotes

### 3. OOS Failure Handling

**Decision:** Hard rejection. OOS failure transitions to `ExperimentRejected`.

**Behavior:**
- `accepted` becomes `false`
- `reason` includes OOS failure details
- Status transitions to `ExperimentRejected`
- `OOSResult` is still persisted for audit

**No new status needed.** The existing state machine remains:
```
ExperimentRunning → ExperimentAccepted (in-sample + OOS pass)
                  → ExperimentRejected (in-sample fail OR OOS fail)
```

**Rationale:**
- Commercial rigor: overfit candidates must not proceed
- Simplicity: no new statuses or special cases
- Audit trail: `OOSResult` stored in result file shows why

### 4. OOS Window Selection

**Decision:** Use available replay data AFTER the experiment's primary window, targeting 30 days but dynamically clamped.

```go
func (v *OOSValidator) determineOOSWindow(primaryWindowEnd time.Time) (start, end time.Time, err error) {
    oosStart := primaryWindowEnd.AddDate(0, 0, 1)
    oosEnd := oosStart.AddDate(0, 0, DefaultOOSWindowDays)
    
    // Clamp to available data
    availableEnd := v.availableReplayDataEnd()
    if oosEnd.After(availableEnd) {
        oosEnd = availableEnd
    }
    
    if oosStart.After(oosEnd) {
        return zero, zero, fmt.Errorf("no OOS data available after %v", primaryWindowEnd)
    }
    
    return oosStart, oosEnd, nil
}
```

**Fallback:** If observations < 3, `UsedFallback = true` but still compute and evaluate.

**Rationale:**
- More robust than fixed calendar dates
- Adapts to actual available data
- Still provides meaningful validation with limited data

### 5. Backwards Compatibility

**Decision:** Grandfather existing `ExperimentAccepted` experiments.

**Behavior:**
- Existing records without `OOSResult` are pre-OOS era
- They can still be promoted via `promote-baseline`
- No retroactive OOS validation
- `Promote()` check unchanged: `result.Experiment.Status == ExperimentAccepted`

**Rationale:**
- Non-breaking change
- Existing accepted experiments remain valid
- Clear transition point: new experiments get OOS, old ones don't

## Implementation Details

### OOSValidator.ValidateWithBrief()

```go
func (v *OOSValidator) ValidateWithBrief(
    candidatePath, baselinePath string,
    brief domain.MutationBrief,
    primaryWindowEnd time.Time,
) (*OOSResult, error) {
    
    oosStart, oosEnd, err := v.determineOOSWindow(primaryWindowEnd)
    if err != nil {
        return nil, err
    }
    
    candidate, err := os.ReadFile(candidatePath)
    if err != nil {
        return nil, fmt.Errorf("read candidate: %w", err)
    }
    
    var baselineScore, candidateScore float64
    var observations int
    
    switch brief.MutationType {
    case "risk_rule_change", "portfolio_constraint_revision":
        baselineScore, candidateScore, observations, err = 
            scoreConstraintWindowWithObservations(v.store, baselinePath, string(candidate), oosStart, oosEnd)
    default:
        baselineScore, candidateScore, observations, err = 
            scorePromptWindowWithObservations(v.store, baselinePath, string(candidate), oosStart, oosEnd)
    }
    
    if err != nil {
        return nil, fmt.Errorf("OOS scoring: %w", err)
    }
    
    improvement := candidateScore - baselineScore
    passed := improvement > oosAcceptanceThreshold() && observations >= oosMinimumObservations()
    
    return &OOSResult{
        Passed:         passed,
        BaselineScore:  baselineScore,
        CandidateScore: candidateScore,
        Improvement:    improvement,
        WindowDays:     int(oosEnd.Sub(oosStart).Hours() / 24),
        Observations:   observations,
        OOSWindowStart: oosStart,
        OOSWindowEnd:   oosEnd,
        UsedFallback:   observations < oosMinimumObservations(),
        ValidationAt:   time.Now(),
        Reason:         fmt.Sprintf("improvement=%.4f, observations=%d", improvement, observations),
    }, nil
}
```

### Judge Constructor Update

```go
func NewJudge(store *ledger.Store, replayDataPath, baselinePath string) *Judge {
    return &Judge{
        store:          store,
        replayDataPath: replayDataPath,
        baselinePath:   baselinePath,
        oosValidator:   NewOOSValidator(replayDataPath, store),
    }
}
```

## Testing Strategy

1. **Unit tests for `determineOOSWindow`:**
   - Window starts day after primary window end
   - Window clamps to available data
   - Error when no data available

2. **Integration tests for `ValidateWithBrief`:**
   - Prompt tightening mutation
   - Risk rule change mutation
   - Portfolio constraint revision mutation
   - Fallback when insufficient observations

3. **Judge integration tests:**
   - Experiment accepted when both in-sample and OOS pass
   - Experiment rejected when in-sample passes but OOS fails
   - OOSResult persisted in result file

4. **Backwards compatibility:**
   - Existing ExperimentAccepted records without OOSResult can still be promoted

## Files to Modify

| File | Change |
|------|--------|
| `internal/domain/registry.go` | Add `OOSResult *OOSResult` to `PromptExperimentResult` |
| `internal/experiment/oos_validator.go` | Implement `Validate()`, `ValidateWithBrief()`, `ValidateWithConstraints()` |
| `internal/experiment/judge.go` | Add `oosValidator` field, call OOS after `passesAcceptance()` |
| `internal/experiment/judge_test.go` | Add tests for OOS integration |
| `internal/experiment/oos_validator_test.go` | Add tests for actual OOS scoring |

## Acceptance Criteria

- [ ] OOS validation runs automatically for all new experiments
- [ ] Candidates must pass BOTH in-sample and OOS to be accepted
- [ ] OOS failure results in `ExperimentRejected` status
- [ ] OOS results are persisted in experiment result JSON
- [ ] Existing `ExperimentAccepted` records remain promotable without OOS
- [ ] All tests pass with >40% coverage
- [ ] `go vet`, `staticcheck`, `gofmt` clean

## Open Questions

None. All 5 architectural questions have been resolved:

1. **Data structure:** `OOSResult` embedded in `PromptExperimentResult`
2. **Pipeline location:** In `Judge.Evaluate()` after in-sample acceptance
3. **Failure handling:** Hard rejection to `ExperimentRejected`
4. **OOS window:** Available replay data after primary window, dynamically clamped
5. **Backwards compatibility:** Grandfather existing accepted experiments
