---
name: T-002 Proposal Commit Contract
about: Define and enforce proposal/commit schema contracts
labels: [enhancement, schema, governance]
---

## Objective

Create auditable schema contracts for proposal and commit payloads.

## Scope

- Add proposal/commit contract fields
- Validate payload before execution
- Fail fast on malformed input
- Add contract tests

## Candidate Paths

- `internal/domain/`
- `cmd/execute-experiment/`
- `cmd/judge-experiment/`

## Acceptance Criteria

- Schema validation runs before execute/judge steps
- Invalid payload returns explicit error
- Contract tests cover missing required fields and incompatible versions

## Validation Commands

```bash
go test ./internal/domain/...
go test ./cmd/execute-experiment/...
go test ./cmd/judge-experiment/...
```

## Notes

Version schema fields to preserve backward compatibility.
