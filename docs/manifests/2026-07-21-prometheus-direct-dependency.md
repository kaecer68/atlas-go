# Audit Manifest: Prometheus Direct Dependency Classification

> **Audit source**: User asked whether the uncommitted `go.mod` diff is an abandoned or already-merged change, and whether it improves or conflicts with the current system.
> **Goal**: Classify and commit the minimal correct direct dependency declarations for the MCP Prometheus observability code.
> **Scope**: `go.mod` only. Out of scope: runtime behavior, image rebuilds, unrelated working-tree artifacts, and previous binary-freshness work.
> **Created**: 2026-07-21
> **Status**: completed
---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| DEP01 | `github.com/prometheus/client_golang` is uncommitted as direct while `github.com/prometheus/common` remains indirect | Prometheus imports were added by merged MCP observability code, but dependency classification was not fully normalized | `go.mod` | Both directly imported modules are direct requirements; `go mod tidy -diff` reports no `go.mod`/`go.sum` drift; MCP server tests and build pass | accepted | none | Evidence: `a858b586`, direct imports in `cmd/atlas-mcp/server/metrics.go` and tests, `go mod tidy -diff` |

---

## Phase Tracker

### Phase A — Audit (read-only)

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Reproduce the symptom | DEP01 | accepted | Main worktree `git diff -- go.mod` shows direct move of `client_golang`; `git blame` marks it Not Committed Yet |
| Identify suspect code | DEP01 | accepted | `cmd/atlas-mcp/server/metrics.go` directly imports `client_golang`; `metrics_test.go` directly imports `prometheus/common/expfmt` |
| Form root cause hypothesis | DEP01 | accepted | Merged commit `a858b586` introduced Prometheus observability code and left dependencies indirect; later manual change corrected only one module |
| Validate hypothesis with evidence | DEP01 | accepted | `go test ./cmd/atlas-mcp/server` passes; `go mod tidy -diff` requests `prometheus/common` direct classification |

### Phase B — Plan

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Map each ID to files + changes | DEP01 | accepted | One-file `go.mod` classification change |
| Define acceptance criteria | DEP01 | accepted | tidy diff empty; package test and build pass |
| Review blast radius | DEP01 | accepted | Dependency metadata only; runtime API unchanged |

### Phase C — Implement

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Classify both direct Prometheus imports | DEP01 | done | `go.mod` direct blocks updated |

### Phase D — Close Out

| Task | ID | Status | Evidence |
|------|----|--------|----------|
| Update manifest status | DEP01 | done | version 1.1 |
| Push branch / open PR | DEP01 | pending | branch ready for merge |
| Run CI / verify | DEP01 | done | `go mod tidy -diff`, `go test ./cmd/atlas-mcp/server`, `go build ./cmd/atlas-mcp/...` |


---

## Commit Discipline

- Format: `fix(manifest): #DEP01 classify Prometheus direct dependencies`
- One commit for DEP01.
- No commit without tidy, package test, and build acceptance passing.

---

## Session-End State

- **Done this session**: confirmed the original diff was uncommitted but functionally valid; classified both direct Prometheus imports and added missing checksums
- **Remaining**: merge branch through the normal PR workflow
- **Next action**: open/merge PR `fix/prometheus-direct-dependency`
- **Uncommitted code**: no
- **Branch / PR**: `fix/prometheus-direct-dependency` / pending
- **Paused because**: no runtime conflict found

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-21 | 1.1 | Classified `client_golang` and `common` as direct dependencies; `go mod tidy`, MCP server tests, and MCP build pass. | OpenCode |
