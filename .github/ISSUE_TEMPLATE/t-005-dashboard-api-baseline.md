---
name: T-005 Dashboard API Baseline
about: Build first API slice for governance dashboards
labels: [enhancement, api, monitoring]
---

## Objective

Expose baseline APIs for macro radar, agent observatory, and forecast-vs-reality views.

## Scope

- Define response contracts
- Implement endpoints with real ledger/trace data
- Add API-level tests
- Verify mobile/desktop compatible payload shape

## Candidate Paths

- `cmd/atlas/`
- `internal/monitoring/`
- `internal/ledger/`

## Acceptance Criteria

- Endpoints return contract-compliant JSON
- Data source is real artifacts, not placeholder data
- Tests cover empty data and normal data scenarios

## Validation Commands

```bash
go test ./internal/monitoring/...
go test ./cmd/atlas/...
```

## Notes

Keep API contracts stable and versioned for frontend evolution.
