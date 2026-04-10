---
name: T-003 Guard Pipeline v2
about: Implement soft/hard guard pipeline with auditable outcomes
labels: [enhancement, risk, control]
---

## Objective

Refactor control pipeline to capture both soft and hard guard outcomes with explicit reject reasons.

## Scope

- Separate soft and hard guard stages
- Persist guard outcomes to ledger
- Ensure hard reject prevents execution
- Add focused tests

## Candidate Paths

- `internal/orchestrator/`
- `internal/sim/`
- `internal/ledger/`

## Acceptance Criteria

- Guard outcomes appear in session artifacts
- Hard guard reject always blocks order execution
- Tests cover pass, soft fail, and hard fail cases

## Validation Commands

```bash
go test ./internal/orchestrator/...
go test ./internal/sim/...
```

## Notes

Avoid changing existing layer order unless required by architecture decision.
