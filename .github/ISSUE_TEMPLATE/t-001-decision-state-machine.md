---
name: T-001 Decision State Machine
about: Implement lifecycle transition enforcement for experiment governance
labels: [enhancement, governance, orchestrator]
---

## Objective

Implement an explicit decision state machine that enforces valid transitions in the experiment lifecycle.

## Scope

- Define allowed transitions
- Block invalid transitions
- Return context-rich errors
- Add tests for all major paths

## Candidate Paths

- `internal/experiment/`
- `internal/evolution/`

## Acceptance Criteria

- Invalid transitions are rejected
- Error includes current state and attempted next state
- Focused tests pass for transition matrix

## Validation Commands

```bash
go test ./internal/experiment/...
go test ./internal/evolution/...
```

## Notes

Keep transition logic deterministic and side-effect free.
