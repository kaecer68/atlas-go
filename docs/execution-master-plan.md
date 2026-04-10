# Execution Master Plan v1.0

## Purpose

Single source of truth for immediate implementation execution.
This plan is governance-first and audit-first, aligned with replay safety and baseline discipline.

## Scope

- Decision governance implementation
- Macro intelligence integration lane
- Guard pipeline v2
- Parallel simulation and comparison
- Visualization API baseline
- Human-in-the-loop controls

## Master Index Map (M0-M8)

| ID | Theme | Target Output | Gate |
|----|-------|---------------|------|
| M0 | Governance Foundation | ADR, ownership map, change protocol | Protocol adopted |
| M1 | Contract Freeze | Proposal/commit/event schemas | Contract tests pass |
| M2 | Macro Intelligence | Event ingest + factor snapshots | Replay factor timeline stable |
| M3 | Decision Traceability | proposal -> commit -> approve -> execute chain | Trace IDs complete |
| M4 | Guard Pipeline v2 | Soft/hard guard evaluation | Reject reasons recoverable |
| M5 | Parallel Simulation | Base/stress/shock scenario runner | Deterministic results |
| M6 | Visualization Baseline | Macro radar, agent observatory, forecast-vs-reality APIs | Dashboard reads real data |
| M7 | Human-In-The-Loop | Approval and revert workflow | One-command rollback |
| M8 | Production Readiness | SLOs, alerts, runbooks, drills | Staging sign-off |

## Task Backlog (T-001 to T-005)

### T-001 Decision State Machine

Goal:
- Enforce valid transitions in experiment lifecycle.

Primary touchpoints:
- `internal/experiment/`
- `internal/evolution/`

Acceptance:
- Invalid transitions are blocked.
- Error messages include explicit state and attempted transition.

### T-002 Proposal/Commit Contract

Goal:
- Define and enforce proposal/commit payload schema.

Primary touchpoints:
- `internal/domain/`
- `cmd/execute-experiment/`
- `cmd/judge-experiment/`

Acceptance:
- Schema validation runs before execution.
- Malformed payloads fail fast and are logged.

### T-003 Guard Pipeline v2

Goal:
- Split controls into soft and hard guards with explicit outcomes.

Primary touchpoints:
- `internal/orchestrator/`
- `internal/sim/`

Acceptance:
- Guard outcomes persist into ledger.
- Hard reject always blocks execution.

### T-004 Trace Persistence

Goal:
- Persist trace chain identifiers for full reconstruction.

Primary touchpoints:
- `internal/ledger/`
- `internal/experiment/`

Acceptance:
- Session summary exposes `proposal_id`, `commit_id`, and `approval_id`.
- Trace chain can be replayed from ledger artifacts only.

### T-005 Visualization API Baseline

Goal:
- Expose API responses for key governance dashboards.

Primary touchpoints:
- `cmd/atlas/`
- `internal/monitoring/`

Acceptance:
- Endpoints return contract-compliant JSON.
- Data source is real trace/ledger content, not mock placeholders.

## Sprint Rhythm

## Sprint 1 (Foundation)

- Freeze schema and state machine behavior.
- Implement trace persistence base fields.
- Add focused tests for transition and contract validation.

Expected result:
- Stable contract layer with auditable lifecycle transitions.

## Sprint 2 (Control + Simulation)

- Implement guard pipeline v2.
- Add scenario simulation modes and compare outputs.
- Integrate guard and trace outputs into session artifacts.

Expected result:
- Deterministic replay under multiple scenarios with clear control outcomes.

## Sprint 3 (Visualization + Operations)

- Ship first dashboard APIs.
- Wire human approval and rollback flow.
- Run operational drills and verify runbook steps.

Expected result:
- Governance loop observable and operationally enforceable.

## Daily Operating Cadence

1. Review prior session evidence in `data/state/sessions/`.
2. Execute one atomic implementation slice.
3. Run focused tests for impacted packages.
4. Run replay validation for determinism and trace completeness.
5. Record gate decision and next mutation priority.

## Mandatory Validation Commands

```bash
gofmt -w .
go test ./internal/experiment/...
go test ./internal/evolution/...
go test ./internal/orchestrator/...
go test ./internal/sim/...
go build ./...
go test ./...
```

## Governance Gates

- G1 Contract Gate: schema compatibility and validation pass
- G2 Behavior Gate: deterministic replay preserved
- G3 Safety Gate: hard guard blocking and audit reasons verified
- G4 Audit Gate: full trace chain recoverable from ledger
- G5 Operations Gate: rollback and runbook drill completed

## Remaining Phase Closure Checklist (M2/M5/M6/M8)

Use this checklist to close remaining phases with auditable evidence.

### M2 Macro Intelligence

Done definition:
- Event ingestion produces consistent factor snapshots across replay windows.
- Factor timeline can be regenerated from source data without drift.

Validation commands:
```bash
go test ./internal/globalmarket/...
go test ./internal/replay/...
go run ./cmd/backtest-window -start 2026-03-26 -end 2026-03-27
```

Required evidence artifacts:
- data/state/sessions/<session-id>/summary.json
- data/state/sessions/<session-id>/recommendation_outcomes.jsonl
- data/state/windows/window-<start>-<end>.json

Sign-off criteria:
- Replay on same window is deterministic after normalizing generated timestamps.
- No missing factor fields in session artifacts.

### M5 Parallel Simulation

Done definition:
- Base/stress/shock scenario outputs are reproducible.
- Scenario comparison is preserved in ledger-ready artifacts.

Validation commands:
```bash
go test ./internal/sim/...
go test ./internal/portfolio/...
bash ./scripts/openclaw/verify-parallel-scenarios.sh
```

Required evidence artifacts:
- data/state/sessions/<session-id>/summary.json
- data/state/sessions/<session-id>/experiments.jsonl
- data/state/windows/window-<start>-<end>.json

Sign-off criteria:
- Determinism hash is stable across clean ledger directories.
- Guard and scenario outputs are both traceable in artifacts.

### M6 Visualization Baseline

Done definition:
- Monitoring APIs return contract-compliant JSON from real session/ledger data.
- Dashboard endpoints expose trace-linked metrics for macro and agent views.

Validation commands:
```bash
go test ./internal/monitoring/...
go test ./cmd/atlas/...
go test ./internal/ledger/... -run TestRecordSessionSummaryPersistsTraceIDs -count=1
```

Required evidence artifacts:
- data/state/sessions/<session-id>/summary.json
- data/state/sessions/<session-id>/recommendation_outcomes.jsonl

Sign-off criteria:
- Endpoint payloads resolve trace identifiers from persisted artifacts.
- No placeholder/mock-only dashboard responses in production path.

### M8 Production Readiness

Done definition:
- Runbook, rollback, and alert workflow are validated in a staging drill.
- Branch protection enforces governance gate before merge.

Validation commands:
```bash
bash ./scripts/openclaw/verify-operations-gate.sh
bash ./scripts/openclaw/verify-governance-gates.sh
go build ./...
go test ./...
```

Required evidence artifacts:
- data/state/approvals/<decision-file>.json
- data/state/sessions/<session-id>/summary.json
- docs/operations-playbook.md
- .github/workflows/ci.yml

Sign-off criteria:
- Required status checks `ci / governance` and `ci / operations` are enabled in branch protection.
- One dry-run rollback and one replayed approval event succeed end-to-end.

## Immediate Start Order

1. T-002 Proposal/Commit Contract
2. T-001 Decision State Machine
3. T-004 Trace Persistence
4. T-003 Guard Pipeline v2
5. T-005 Visualization API Baseline

Rationale:
- Contracts first to avoid downstream rework.
- State machine before guard expansion.
- Trace persistence before visualization to ensure data integrity.

## Non-Negotiable Constraints

- Baseline policy must be loaded before experiment execution or judgment.
- Replay window confidence must be checked before promotion decisions.
- Avoid mixed mutations in a single experiment cycle.
- Keep changes auditable through session artifacts and explicit IDs.
