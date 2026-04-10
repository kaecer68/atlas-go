---
name: T-004 Trace Persistence
about: Persist decision trace IDs for full reconstruction
labels: [enhancement, audit, ledger]
---

## Objective

Persist decision trace chain identifiers for proposal, commit, approval, and execution.

## Scope

- Add trace IDs to domain records
- Persist trace IDs in ledger outputs
- Surface trace IDs in session summary
- Add tests for reconstruction path

## Candidate Paths

- `internal/ledger/`
- `internal/experiment/`
- `internal/domain/`

## Acceptance Criteria

- Session summary includes proposal, commit, approval IDs
- Trace chain can be reconstructed from persisted artifacts only
- Tests validate field presence and continuity

## Validation Commands

```bash
go test ./internal/ledger/...
go test ./internal/experiment/...
```

## Notes

Prefer additive fields to avoid breaking existing artifact readers.
