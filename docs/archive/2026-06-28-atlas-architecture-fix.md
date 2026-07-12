# Atlas Architecture Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the live-layer inversion, reduce `SystemCore` responsibility concentration without breaking plugin/test surfaces, inject registry loading into monitoring, and document the two `Position` types so their semantics are explicit.

**Architecture:** Fix the highest-risk boundary first: make `internal/live` consume pre-processed research decisions from a narrow input contract instead of holding `*orchestrator.System` and re-running the research pipeline. After that seam exists, decompose `SystemCore` into grouped dependency holders while preserving the existing `CoreServices` behavior and plugin lifecycle wiring; then inject registry access into monitoring and finish with documentation-only clarification for `Position`.

**Tech Stack:** Go 1.25, stdlib `testing`, existing package-local tests, `gofmt`, `go test`, `go build`, `go vet`, `staticcheck`.

---

## File Map

**Likely create:**
- `internal/live/agent_runner_test.go`
- `internal/orchestrator/live_execution_input.go`
- `internal/orchestrator/live_execution_input_test.go`
- `internal/monitoring/service/pipeline_test.go`

**Likely modify:**
- `internal/live/agent_runner.go`
- `internal/live/orchestrator.go`
- `internal/live/orchestrator_mode_test.go`
- `internal/live/orchestrator_test.go`
- `internal/live/store.go`
- `internal/orchestrator/system.go`
- `internal/orchestrator/plugin_host.go`
- `internal/orchestrator/system_plugins.go`
- `internal/orchestrator/factory.go`
- `internal/orchestrator/phase3_integration_test.go`
- `internal/orchestrator/human_override_test.go`
- `internal/orchestrator/capital_integration_test.go`
- `internal/monitoring/service/pipeline.go`
- `internal/monitoring/dashboard_api.go`
- `cmd/atlas/main.go`
- `cmd/experimental/staging-drill/main.go`
- `internal/portfolio/risk_manager.go`
- `internal/domain/types.go`

---

### Task 1: Define the live execution-input seam

**Files:**
- Create: `internal/live/agent_runner_test.go`
- Modify: `internal/live/agent_runner.go`
- Modify: `internal/live/store.go`

- [ ] **Step 1: Write a failing test for applying pre-processed research output into live state**

Run: `go test ./internal/live/... -run 'TestAgentRunner.*ExecutionInput'`
Expected: FAIL because `AgentRunner` has no single entrypoint for pre-processed regime/raw/final recommendations.

- [ ] **Step 2: Add a small live-side contract for pre-processed decisions**

Recommended shape:
- `ExecutionInput` data holder with `Regime`, `RawRecommendations`, `FinalRecommendations`, and optional `GuardOutcomes`
- `AgentRunner.ApplyExecutionInput(...)` or equivalent single method

Expected behavior:
- `StateStore` current regime is updated once
- pending recommendations become the raw list
- filtered recommendations become the final list

- [ ] **Step 3: Keep the old behavior compiling, but route state writes through the new method**

Update `RunContextAgent`, `RunStyleAndSectorAgents`, and `ApplyRiskFilters` to reuse the new state-writing path where practical, or mark them as transitional helpers until Task 3 removes them.

- [ ] **Step 4: Add or reuse store helpers only if needed**

If current store methods are enough, do not add more APIs.
If needed, add one focused helper in `internal/live/store.go` for atomic state updates.

- [ ] **Step 5: Run focused tests**

Run:
- `go test ./internal/live/... -run 'TestAgentRunner.*ExecutionInput'`
- `go test ./internal/live/...`

Expected: PASS, with new tests proving live can consume pre-processed inputs without orchestrator internals.

- [ ] **Step 6: Commit**

Suggested commit:
- `git commit -m "refactor: add live execution input seam"`

**Test verification method:**
- New unit test asserts regime, pending recommendations, and filtered recommendations are set from a single execution input.
- Existing live tests remain green.

**Expected output/behavior:**
- Live state can be fed from one narrow decision object.
- The state transition is explicit and testable.

---

### Task 2: Build the orchestrator-backed execution-input adapter

**Files:**
- Create: `internal/orchestrator/live_execution_input.go`
- Create: `internal/orchestrator/live_execution_input_test.go`
- Modify: `internal/orchestrator/system.go`

- [ ] **Step 1: Write a failing test proving live execution input comes from the canonical orchestrator pipeline**

Run: `go test ./internal/orchestrator/... -run 'Test.*LiveExecutionInput'`
Expected: FAIL because there is no adapter that converts canonical research output into the live contract.

- [ ] **Step 2: Implement a narrow adapter that reuses `ExecuteWithContext`**

Recommended responsibility:
- fetch quotes
- call canonical `ExecuteWithContext(...)`
- return `ExecutionInput` with regime, raw recommendations, final recommendations, and guard outcomes

Important:
- do not duplicate `inferRegime`, screening, recommendation generation, or control filtering
- include the same Darwinian-weighted/control-filtered behavior used by simulation

- [ ] **Step 3: Reuse existing orchestrator helpers instead of copying logic**

Prefer existing code paths around:
- `ExecuteWithContext(...)`
- narrative adjustment only if live explicitly still needs it
- `PluginRegistry`, execution policy, registry, and weight manager from orchestrator state

- [ ] **Step 4: Keep the adapter minimal and side-effect free**

Do not write ledger/session state here.
This adapter should only produce inputs for live execution.

- [ ] **Step 5: Run focused tests**

Run:
- `go test ./internal/orchestrator/... -run 'Test.*LiveExecutionInput'`
- `go test ./internal/orchestrator/...`

Expected: PASS, with proof that live input is now derived from the canonical orchestrator research path.

- [ ] **Step 6: Commit**

Suggested commit:
- `git commit -m "refactor: derive live decisions from orchestrator pipeline"`

**Test verification method:**
- Unit test validates the adapter returns regime/raw/final decisions from the canonical pipeline.
- Add a regression assertion for the hidden risk found during analysis: live inputs include the same weighted/control-filtered result shape as simulation.

**Expected output/behavior:**
- There is one research pipeline again.
- Live no longer needs to know how to infer regime, screen, recommend, and filter.

---

### Task 3: Remove live package layer inversion

**Files:**
- Modify: `internal/live/orchestrator.go`
- Modify: `internal/live/agent_runner.go`
- Modify: `internal/live/orchestrator_mode_test.go`
- Modify: `cmd/atlas/main.go`
- Modify: `cmd/experimental/staging-drill/main.go`

- [ ] **Step 1: Write a failing constructor/wiring test for `live.NewOrchestrator`**

Run: `go test ./internal/live/... -run 'TestNewOrchestrator.*ExecutionInput|TestNewOrchestrator.*NoSystem'`
Expected: FAIL because `NewOrchestrator` still requires `*orchestrator.System`.

- [ ] **Step 2: Replace the `*orchestrator.System` field with a narrow provider dependency**

Target state:
- `internal/live` depends on a small input provider interface
- `internal/live` does not import `internal/orchestrator` in its core orchestration path

- [ ] **Step 3: Update `NewOrchestrator` to accept the narrow dependency**

Remove the direct `system *orchestrator.System` requirement.
Keep broker-mode behavior unchanged.

- [ ] **Step 4: Update market-open/intraday callbacks to request pre-processed inputs**

Replace:
- `RunContextAgent`
- `RunStyleAndSectorAgents`
- `ApplyRiskFilters`

with one or two explicit calls that obtain and apply execution input.

- [ ] **Step 5: Update command entrypoints to pass the adapter**

Modify:
- `cmd/atlas/main.go`
- `cmd/experimental/staging-drill/main.go`

Expected: both entrypoints pass the new adapter instead of a full orchestrator system.

- [ ] **Step 6: Run focused tests**

Run:
- `go test ./internal/live/...`
- `go test ./cmd/...`

Expected: PASS, with no direct live dependency on `*orchestrator.System`.

- [ ] **Step 7: Commit**

Suggested commit:
- `git commit -m "refactor: decouple live orchestrator from system"`

**Test verification method:**
- Constructor tests prove `live.NewOrchestrator` only needs the narrow provider.
- Existing live broker mode tests still pass.

**Expected output/behavior:**
- `internal/live` becomes an execution consumer.
- Research pipeline ownership returns to `internal/orchestrator`.

---

### Task 4: Decompose `SystemCore` into logical dependency groups

**Files:**
- Modify: `internal/orchestrator/system.go`
- Modify: `internal/orchestrator/plugin_host.go`
- Modify: `internal/orchestrator/system_plugins.go`
- Modify: `internal/orchestrator/factory.go`
- Modify: `internal/orchestrator/phase3_integration_test.go`
- Modify: `internal/orchestrator/human_override_test.go`
- Modify: `internal/orchestrator/capital_integration_test.go`

- [ ] **Step 1: Write a failing test that locks the current public/plugin behavior**

Run:
- `go test ./internal/orchestrator/... -run 'TestSystemWithPRISM|TestSystemWithSwarm|TestSystemWithSpawning|TestApplyHumanOverrides|TestWithCapitalManagement'`

Expected: PASS before refactor; record this as the safety baseline.

- [ ] **Step 2: Introduce grouped internal structs inside `SystemCore`**

Recommended groups:
- `simulationDeps`
- `strategyDeps`
- `controlDeps`
- `persistenceDeps`
- `runtimeState`

Keep `SystemCore` itself as the outward shell so existing embedding and plugin wiring survive.

- [ ] **Step 3: Move fields into grouped structs without changing public method signatures**

Preserve:
- `GetReplay()`
- `GetRegistry()`
- `GetPolicy()`
- `GetLastOutcomes()`
- `Registry()`
- `GetPlugins()`
- `GetExecutionPolicy()`

Use forwarding methods where needed.

- [ ] **Step 4: Update internal call sites incrementally**

Touch only the places that must follow the new grouping:
- simulation methods in `system.go`
- plugin registration in `system_plugins.go`
- factory wiring in `factory.go`

Do not widen visibility just to make tests easier.

- [ ] **Step 5: Keep plugin host compatibility stable**

`Plugin` already depends on `CoreServices`.
Do not break `Attach(core CoreServices)` behavior.
Only adjust `PluginHost` signatures if necessary, and prefer the smallest change.

- [ ] **Step 6: Update tests that construct `&System{SystemCore: &SystemCore{...}}`**

Several orchestrator tests instantiate `SystemCore` directly with selected fields.
Adjust only the minimum required construction pattern.

- [ ] **Step 7: Run orchestrator-focused verification**

Run:
- `go test ./internal/orchestrator/...`
- `go build ./...`

Expected: PASS, with plugin tests and direct-construction tests green.

- [ ] **Step 8: Commit**

Suggested commit:
- `git commit -m "refactor: group system core dependencies"`

**Test verification method:**
- Existing orchestrator tests remain green.
- No public/plugin behavior changes.
- Direct field users in tests and factory are updated cleanly.

**Expected output/behavior:**
- `SystemCore` remains externally compatible.
- Internal responsibilities are grouped and easier to reason about.
- Plugin attachments still work through `CoreServices`.

---

### Task 5: Inject registry access into monitoring

**Files:**
- Create: `internal/monitoring/service/pipeline_test.go`
- Modify: `internal/monitoring/service/pipeline.go`
- Modify: `internal/monitoring/dashboard_api.go`

- [ ] **Step 1: Write a failing test for `LoadUniverseOverlap` with an injected registry provider**

Run: `go test ./internal/monitoring/... -run 'TestPipelineService.*UniverseOverlap'`
Expected: FAIL because `PipelineService` still hard-codes `orchestrator.LoadRegistry()`.

- [ ] **Step 2: Add a narrow registry provider interface**

Recommended interface:
- `LoadRegistry() (domain.AgentRegistry, error)`

Keep it in `internal/monitoring/service/pipeline.go` unless reuse clearly appears.

- [ ] **Step 3: Inject the provider through `NewPipelineService`**

Constructor target:
- `NewPipelineService(workDir, ledgerDir string, registryProvider RegistryProvider)`

Keep the default wiring in `DashboardAPI`.
Do not let `PipelineService` construct orchestrator state by itself.

- [ ] **Step 4: Preserve fallback behavior only where explicitly intended**

If fallback to `SeedRegistry()` remains desirable, move that decision into the injected provider or constructor wiring.
Do not silently hide parse/config errors inside `LoadUniverseOverlap()`.

- [ ] **Step 5: Update dashboard wiring**

Modify `internal/monitoring/dashboard_api.go` so route registration builds the provider once and injects it.

- [ ] **Step 6: Run focused tests**

Run:
- `go test ./internal/monitoring/...`
- `go test ./internal/monitoring/... -run 'TestPipelineService.*UniverseOverlap'`

Expected: PASS, with tests proving `PipelineService` can run from a stub provider.

- [ ] **Step 7: Commit**

Suggested commit:
- `git commit -m "refactor: inject monitoring registry provider"`

**Test verification method:**
- A stub provider drives `LoadUniverseOverlap`.
- No direct `orchestrator.LoadRegistry()` call remains in the service logic.

**Expected output/behavior:**
- Monitoring depends on a data source contract, not orchestrator internals.
- Registry-loading failures become explicit and testable.

---

### Task 6: Document the two `Position` types

**Files:**
- Modify: `internal/portfolio/risk_manager.go`
- Modify: `internal/domain/types.go`
- Modify: `internal/portfolio/risk_manager_test.go` only if a doc-adjacent regression test is warranted

- [ ] **Step 1: Add a failing documentation-oriented review check**

Run:
- `grep -n "type Position struct" internal/domain/types.go internal/portfolio/risk_manager.go`

Expected: two same-named structs with no strong semantic distinction in comments.

- [ ] **Step 2: Add explicit type comments**

Document:
- `domain.Position` is the canonical cross-package snapshot/execution type
- `portfolio.Position` is internal/transient risk-tracking state with timestamps and split realized/unrealized bookkeeping

- [ ] **Step 3: Clarify ambiguous fields in the portfolio type comments**

Call out:
- `Size` semantics
- `EntryPrice` semantics
- why this type is not interchangeable with `domain.Position`

- [ ] **Step 4: Add a package-local note only if the comments are still not enough**

Prefer type comments first.
Do not add a new doc file unless comments remain unclear.

- [ ] **Step 5: Run focused verification**

Run:
- `go test ./internal/portfolio/...`
- `go build ./...`

Expected: PASS, no behavior changes.

- [ ] **Step 6: Commit**

Suggested commit:
- `git commit -m "docs: clarify domain and portfolio position types"`

**Test verification method:**
- No functional regressions.
- Comments make the semantic distinction obvious during code review.

**Expected output/behavior:**
- Future refactors do not confuse canonical persisted positions with transient risk-tracking positions.

---

### Task 7: Final integration verification

**Files:**
- Modify only if verification reveals breakage
- No planned new files

- [ ] **Step 1: Run package-focused validation**

Run:
- `go test ./internal/live/...`
- `go test ./internal/orchestrator/...`
- `go test ./internal/monitoring/...`
- `go test ./internal/portfolio/...`

Expected: PASS

- [ ] **Step 2: Run repository-wide build and test**

Run:
- `test -z "$(gofmt -l .)"`
- `go build ./...`
- `go test ./...`

Expected: PASS

- [ ] **Step 3: Run quality checks**

Run:
- `go vet ./...`
- `staticcheck ./...`

Expected: PASS

- [ ] **Step 4: Refresh the graph after code changes**

Run:
- `graphify update .`

Expected: graph refresh completes without changing runtime behavior.

- [ ] **Step 5: Review the diff for scope**

Check:
- live no longer owns research logic
- orchestrator still exposes the same outward behavior
- monitoring no longer loads registry internally
- position comments are clear

- [ ] **Step 6: Commit any final fixups only if verification required code changes**

Suggested commit:
- `git commit -m "test: finalize architecture refactor verification"`

**Test verification method:**
- CI-aligned commands all pass.
- No unexpected architectural regressions show up in the final diff.

**Expected output/behavior:**
- The four issues are addressed in isolated, reviewable commits.
- The refactor is safe to review and merge incrementally.

---

## Atomic Commit Strategy

1. `refactor: add live execution input seam`
2. `refactor: derive live decisions from orchestrator pipeline`
3. `refactor: decouple live orchestrator from system`
4. `refactor: group system core dependencies`
5. `refactor: inject monitoring registry provider`
6. `docs: clarify domain and portfolio position types`
7. `test: finalize architecture refactor verification`

---

## TDD Rules For Execution

- Every new seam starts with a failing test in the package that owns the seam.
- Do not move to the next task until the focused package tests pass.
- Prefer stubs over mocks; these packages already use lightweight direct construction in tests.
- When refactoring `SystemCore`, lock existing plugin and direct-construction tests first, then change internals.
- For the live refactor, prove behavior with state-store assertions, not only interface-shape tests.

---

## Key Risks To Watch

- `internal/live/agent_runner.go` currently duplicates the research pipeline and appears to skip Darwinian weighting; preserve canonical behavior through the adapter.
- Several orchestrator tests directly construct `SystemCore`; the grouping refactor must preserve this style or update tests minimally.
- `internal/orchestrator/factory.go` currently touches internal `SystemCore` fields directly; expect this file to need small adjustments during Issue 1.
- `PipelineService.LoadUniverseOverlap()` currently masks registry-load failures by falling back internally; decide explicitly whether fallback belongs in the provider or should surface as an error.