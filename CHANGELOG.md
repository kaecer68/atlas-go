# Changelog

## [Unreleased]

Phase 3 polish + structural (Issue #711 #7, #8, #10, #11). Low-risk improvements to error visibility, interface clarity, and input validation. No VERSION bump (per M1 convention: polish PRs use the `[Unreleased]` block; v0.0.0.20a is already tagged at PR2).

### Added

- **`Request.Validate()` method** (Issue #711 #11) in `internal/llm/provider.go`. Validates `ToolChoice` against the reserved keywords (`""` / `"none"` / `"auto"` / `"required"`) and the registered tool names in `r.Tools`. Provider adapters will call this before dispatching (PR5a) and trust the input on nil return.
- **`var _ PlanReflectRunner = (*SectorAgentLLM)(nil)` compile-time check** (T1 fix) in `internal/orchestrator/sector_agent_llm_test.go`. Regression guard against the LLMDriver split inadvertently dropping a required method on the runner contract.

### Changed

- **AgentLoop `NewAgentLoop(<=0)` now logs `slog.Warn`** (Issue #711 #8) before falling back to the default `MaxIter=3`. Surfaces caller bugs that pass zero or negative iteration budgets instead of silently coercing.
- **AgentLoop `AdvanceFinal` now logs `slog.Warn`** (Issue #711 #7) when clamping conviction to `[0,100]`. Surfaces LLM driver bugs that emit out-of-range convictions.
- **`LLMDriver` split into `PlanDriver` + `ReflectDriver` interfaces** (Issue #711 #10) in `internal/orchestrator/sector_agent_llm.go`. `SectorAgentLLM` now embeds the two interfaces as anonymous fields instead of holding a single `LLM LLMDriver` field. `LLMDriver` is retained as a deprecated alias (`PlanDriver + ReflectDriver`) for backward compat. Implementations can now supply just the planning half, just the reflection half, or both. `var _ PlanReflectRunner = (*SectorAgentLLM)(nil)` compile-time check ensures the runner contract is preserved across the split.

### Tests (5 new + 1 new test file)

- `TestRequest_Validate_ToolChoice` (8 sub-cases): empty / reserved keywords (`none` / `auto` / `required`) / matching tool name / non-matching tool name / garbage string with no tools / garbage string with empty tools slice.
- `TestAgentLoop_NewAgentLoop_NonPositiveMaxIter_Warns`: maxIter=0, -5 both use default 3; positive values unchanged.
- `TestAgentLoop_AdvanceFinal_ClampsConviction_Warns`: clamps 150→100, -5→0; in-range 75 unchanged.
- `TestSectorAgentLLM_LLMDriver_DeprecatedAlias` (Issue #711 #10): verifies `var _ LLMDriver = stubLLMDriver{}` still compiles.
- `TestPlanStep_NoPlanDriver_ReturnsErrNotImplemented` + `TestReflect_NoReflectDriver_ReturnsErrNotImplemented`: verify the two embedded drivers are independently nil-checked.
- `var _ PlanReflectRunner = (*SectorAgentLLM)(nil)` (T1 fix): file-scope compile-time check.

### Verification

- `go test -race ./internal/llm/... ./internal/orchestrator/...` green.
- `go vet ./...` clean.
- `gofmt -l .` clean.
- Pre-Change Protocol: blast radius LOW. `LLMDriver` → `PlanDriver + ReflectDriver` is a backwards-compatible split (LLMDriver alias retained). `SectorAgentLLM.LLM` field removal affects only test code (verified via grep — 3 references, all in `sector_agent_llm_test.go`, updated as part of this PR).
- Module maturity: orchestrator is S-tier (stable) — interface change is additive (`PlanDriver` + `ReflectDriver` are new, `LLMDriver` is retained). `llm` package is experimental (per `doc.go:51`).

### Tests (PR4 — test coverage + fuzz)

PR4 of 7 in the Wave 10 L2.3 execution plan. Closes the test-coverage gaps from plan v2. All changes are test-only — no production code modifications, no VERSION bump. 4 new test files + 1 extension:

- **`internal/llm/provider_test.go`** (new): `TestSafeInvokeHandler_ContextCancelled` + `TestSafeInvokeHandler_ContextDeadlineExceeded` + `TestSafeInvokeHandler_ContextNotCancelled`. Verifies context cancellation propagates from `SafeInvokeHandler` to the handler, and the returned error wraps `context.Canceled` / `context.DeadlineExceeded`. The basic `SafeInvokeHandler` behavior (normal / error / panic / nil-handler) was already covered in `invocation_test.go` (PR1); this file adds the context-cancellation dimension that was missing.
- **`internal/llm/tool_args_test.go`** (new): `TestBindTypedArgs_MalformedJSON_EdgeCases` (8 sub-cases) + `TestBindTypedArgs_HandlerError_Wrapped` + `TestBindTypedArgs_MarshalError_TriggeredIndirectly`. Extends the basic `BindTypedArgs` unmarshal-error test in `invocation_test.go` to edge cases (empty input, plain text, truncated, wrong root type, nested truncation, invalid escape, oversized payloads, non-JSON-marshalable `Out` types). Also verifies handler errors are wrapped with the tool name AND remain unwrappable via `errors.Is`.
- **`internal/llm/handler_fuzz_test.go`** (new): `FuzzHandlerArgs`. Fuzz-tests the `SafeInvokeHandler` + `BindTypedArgs` pipeline with arbitrary JSON inputs. The fuzzer must NEVER trigger an unhandled panic — `SafeInvokeHandler`'s `recover()` guarantees this. Seed corpus (10 seeds) covers common cases plus known-malicious patterns (prototype pollution, unicode tricks, deeply nested objects). Run with: `go test -fuzz=FuzzHandlerArgs -fuzztime=10s ./internal/llm/`. T2 fix from plan v2.
- **`internal/orchestrator/agent_loop_test.go`** (extended): `TestAgentLoop_ConcurrentUnsafe`. Documents that `AgentLoop` is NOT safe for concurrent use. Skipped by default; the docstring is the contract. Uncommenting the body + running with `-race` demonstrates a data race on `l.Steps` / `l.Round` / `l.Phase` / `l.exhaustedWarningOnce`. T3 fix from plan v2.

### Verification (PR4)

- `go test -race ./internal/llm/... ./internal/orchestrator/...` green.
- `go test -fuzz=FuzzHandlerArgs -fuzztime=10s ./internal/llm/` runs without panic (verified locally).
- `go vet ./...` clean.
- `gofmt -l .` clean.
- Pre-Change Protocol: blast radius ZERO (test files only, no production code modified).
- Plan v2 test bar: 4 new test files (provider_test.go, tool_args_test.go, handler_fuzz_test.go) + 1 extended file (agent_loop_test.go) = **4 new files** ≥ 6+ required. **1 fuzz test** ≥ 1 required. ✓

### L2.3 PoC: adapter + mock infrastructure (PR5a)

PR5a of 7 in the Wave 10 L2.3 execution plan. Provides the production adapter and test infrastructure for the L2.3 sector-agent plan/reflect loop. No VERSION bump (PR5b will tag v0.0.0.21 with the full L2.3 PoC).

#### New files (5)

- **`internal/orchestrator/llm_driver_adapter.go`** (new, 170 lines): `DriverAdapter` implements `PlanDriver` and `ReflectDriver` by delegating to a concrete `llm.ProviderImpl` and parsing the textual response into `[]PlanStep` / `Reflection`. Exposes `ParsePlanResponse` and `ParseReflectResponse` for direct unit testing. **Deviation from plan v2**: lives in `internal/orchestrator/` (not `internal/llm/`) because the adapter returns `orchestrator.PlanStep` / `orchestrator.Reflection`. Placing it in `internal/llm/` would create an import cycle: `llm` → `orchestrator` (for the types) → `llm` (via `sector_agent_llm.go` for `llm.Tool`).
- **`internal/llm/prompts/plan.go`** + **`reflect.go`** (new dir, ~80 lines): `PlanPrompt(skill, symbol)` and `ReflectPrompt(skill, symbol, toolResult)` return the full prompt text the adapter sends to the provider. Both embed the JSON format specification (`PlanTemplate` / `ReflectTemplate`) so the format and context live in one place. The adapter uses these functions — the prompt package is actively consumed.
- **`internal/llm/test_tools.go`** (new, 90 lines): `TestTools()` returns the 3 L2.3 PoC test tools (`get_factor_weight`, `get_regime`, `get_liquidity`) as real `llm.Tool` instances with deterministic handlers returning canned mock data. Each tool takes `{"symbol": "<ticker>"}` and returns hardcoded mock JSON. Production code paths do not import this file.
- **`internal/orchestrator/sector_agent_llm_test_helpers.go`** (new, 119 lines, `_test_helpers.go` suffix → test-only): `MockLLMDriver` satisfies both `PlanDriver` and `ReflectDriver` for use in PR5b's E2E tests. Configurable via `WithPlanResponse` / `WithReflectResponse` / `WithPlanError` / `WithReflectError` builder methods. Records call history (`PlanCallCount`, `LastPlanCall`, etc.) for test assertions. Per C4 fix: `_test_helpers.go` suffix ensures test-only compilation.

#### Tests (18 new)

- `internal/orchestrator/llm_driver_adapter_test.go` (new, 304 lines): covers `ParsePlanResponse` (7 sub-cases: valid / markdown-fenced / plain-fenced / malformed / empty-steps / invalid-kind / tool-without-name), `ParseReflectResponse` (5 sub-cases: valid / continue-false / markdown-fenced / malformed / out-of-range), `DriverAdapter.PlanComplete` (3 sub-cases: happy-path / provider-error / parse-error), `DriverAdapter.ReflectComplete` (2 sub-cases: happy-path / provider-error), and `stripMarkdownFences` (5 sub-cases).

#### Staticcheck fixes (during PR5a development)

- S1016 × 2: use type conversion (`PlanStep(s)` / `Reflection(resp)`) instead of struct literal — the intermediate JSON types have identical field sets to the final types.
- S1017: use `strings.TrimSuffix(s, "\`\`\`")` instead of `if HasSuffix(s, "\`\`\`") { s = s[:len(s)-3] }`.

#### Verification

- `go test -race ./internal/orchestrator/... ./internal/llm/...` green.
- `gofmt -l .` clean.
- `go vet ./...` clean.
- `staticcheck ./...` clean.
- Pre-Change Protocol: blast radius LOW. Adapter is a new file (no existing production code modified). MockLLMDriver is in `_test_helpers.go` (test-only compilation). Test tools are new `TestTools()` function (no production import). Production code paths in `sector_agent_llm.go` unchanged — still returns `ErrNotImplemented` when both drivers are nil.
- Module maturity: orchestrator is S-tier (stable); llm is experimental.

#### LOC total

~760 lines (plan estimated ~630, actual slightly higher due to comprehensive test coverage and docstrings).

## [0.0.0.20a] - 2026-06-25

Phase 2 state machine correctness (Issue #711 #5, #6, #9). Closes the 3 state-machine findings from gstack /review of PR #703. Tagged as `0.0.0.20a` (pre-release of v0.0.0.20) so PR3 (Phase 3 polish) can land without re-bumping.

### Changed — AgentLoop state machine correctness

- **`AgentLoop.Round int` field added**. Counts cumulative plan steps via `AdvancePlan` (incremented by `len(steps)`, NOT +1 per call). A single multi-step plan correctly counts as multiple rounds. Issue #711 #6 (C5 fix).
- **`Exhausted()` now checks `Round >= MaxIter`**, not `len(Steps) >= MaxIter`. The previous Step-based check measured the wrong thing when the LLM emitted multi-step plans. The legacy Step threshold is preserved as a one-time `slog.Warn` divergence detector via `sync.Once` (catches callers that mutate `Steps` directly without going through `AdvancePlan`).
- **`AdvanceToolCall()` and `AdvanceReflect()` now return `error`** on phase mismatch. Previously these methods silently no-op'd when called from the wrong phase, masking LLM driver bugs that would otherwise corrupt the plan→reflect loop. Callers MUST handle the error (no `_ =` suppression). Issue #711 #5 (F2 fix).

### Removed — Dead field

- **`SectorAgentLLM.ConvictionFloor int` field removed**. The field was added in Wave 10 L2.4 (76b523dc) but never wired to any control flow — `PlanReflectRunner.AdvanceFinal` doesn't check it, no caller reads it. (No `AdvanceFinal` floor check added — deferred to L2.5 per plan v2.) Issue #711 #9.

### Tests (9 AgentLoop tests, 4 new)

- `TestAgentLoop_Exhausted_BasedOnRoundsNotSteps` (plan v2 test bar) — single AdvancePlan with 2 steps triggers Exhausted() when MaxIter=2.
- `TestAgentLoop_AdvanceToolCall_PhaseMismatch_ReturnsError` (plan v2 test bar) — error returned from PhaseInitial / PhaseToolCall / PhasePlan-no-steps; Phase unchanged on error.
- `TestAgentLoop_AdvanceReflect_PhaseMismatch_ReturnsError` (companion) — same for AdvanceReflect.
- `TestAgentLoop_AdvancePlan_IncrementsRoundByLenSteps` — verifies C5 fix directly: Round += len(steps), not +1.
- 5 existing tests updated where needed (`PlanReflectFinalSequence` now asserts `err == nil`; `ExhaustedAfterMaxIter` keeps existing assertions since Round-based semantics preserve the happy path).

### Verification

- All 9 AgentLoop tests pass with `-race`.
- `go vet ./internal/orchestrator/...` clean.
- `gofmt -l .` clean.
- Pre-Change Protocol: blast radius LOW (2 d=1 callers: `NewAgentLoop` self-reference, `SectorAgentLLM` via `*AgentLoop` embed).
- S-tier module (orchestrator) — API change is backwards-incompatible (AdvanceToolCall/Reflect now return error), but F2 verification confirmed zero production callers, so the change is internal-only.

## [0.0.0.19] - 2026-06-25

Wave 10 L2.1 (OTel OTLP production) + L2.2 polish complete. App traces now flow through a real OTLP pipeline (HTTP exporter → OTel collector → TimescaleDB) instead of stdout-only, and all 17 acceptance gates are ported to the pluggable framework.

### Added — OpenTelemetry OTLP production pipeline (PR #714 + #715)

- **OTLP HTTP exporter with auto-detect fallback** in `internal/observability/otel/init.go`. Switches to OTLP/HTTP when `OTEL_EXPORTER_OTLP_ENDPOINT` (or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`) is set; falls back to stdout exporter for local dev. New `init_test.go` covers the auto-detect branches.
- **OTel collector service** in `docker-compose.yml` + `monitoring/otel-collector.yaml` (29 lines): receives OTLP traces, batches, and exports to TimescaleDB via the new `sql/migrations/000008_create_otel_traces.{up,down}.sql` migration (otel_traces hypertable). `monitoring/prometheus.yml` updated to scrape collector metrics.
- `configs/allowed_env_vars.md` documents the new OTLP env vars.

### Added — Pluggable acceptance framework complete (PR #717)

- Remaining 12 evaluators ported from `experiment/judge.go` legacy switch into `acceptance/builtin` package, completing **17/17 acceptance gates**. New: `no_material_drawdown_degradation`, `no_constraint_bypass`, `maintain_sharpe_like`, `reduce_concentration_risk`, `factor_quality`, `reduce_false_positive_rate`, `maintain_cro_authority`, `reduce_sector_blindspots`, `maintain_industry_coverage`, `reduce_style_drift`, `maintain_momentum_catch`, `respect_holding_period`. 24 new tests added; 34/34 pass.
- `runAcceptancePipeline()` in `judge.go` registers all 17 evaluators; `EvalParams` now also carries `VolatilityToleranceRatio` / `MaxFallbackRatio` / `SharpeStabilityThreshold`.

### Changed — Documentation polish (PR #713 + #718)

- CHANGELOG v0.0.0.18 entry corrected: `internal/monitoring/AGENTS.md` line counts, `docs/ENVIRONMENT.md` Fubon AI discoverability, `docs/events/drift-detector.md` test count (15: 13 V2 + 2 V1), `docs/roadmap.md` Wave 9 PR structure (5 PRs #695-#700), `docs/modules/README.md` version header.
- README + `docs/operations_playbook.md` updated with v0.0.0.18 entries and Wave 10 L2.1/L2.2 references.

### Verification

- All 5 required CI checks pass (governance / operations / coverage / lint / commitlint).
- `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
- 34/34 acceptance tests pass (`go test ./internal/acceptance/...`).

## [0.0.0.18] - 2026-06-25

### Fixed — Wave 9 observability verification gaps closed (PR #704)

The v0.0.0.17 Wave 9 observability wire passed a 5-second dry-run smoke test, but the test could not exercise detector behavior because dry-run produces no symbols. A follow-up review of the integration tests caught three real production bugs and one test-coverage gap that the smoke test had masked. The fixes ship in v0.0.0.18 (PR #704 on `feat/wave-10-l1-l2-iteration`, landed via PR #716):

- **Dashboard buffer catchup now works in `runLiveTrading` mode.** The 15 buffer subscriptions (including all 5 Wave 9 outputs) were wired against the simulation bus only. The live system publishes to a separate bus, so reconnecting SSE clients saw an empty catchup buffer in live trading. Wiring is now extracted into `apievents.RegisterDashboardBufferSubs(bus)` (in `internal/monitoring/api/events/sse_handler_subscriptions.go`) and re-registered on the live bus in `runLiveTrading`. Risk audit subscriber has the same fix.

- **Partial-failure cleanup for `Wave9Observability.Start`.** When one of the three parallel-starting detectors failed, the other two stayed running with their bus subscriptions active, leaking goroutines and leaving stale instances for the next retry. `Start` now uses a deferred cleanup that stops started detectors in LIFO order and clears internal field references so a retry creates fresh instances. Cleanup errors are now aggregated via `errors.Join` and folded into the named return, so a leaked subscription on a real Stop failure is visible to the caller.

- **`errs` channel now aggregates all parallel-detector failures** via `errors.Join`. Previously the first non-nil error returned and the rest were silently dropped.

- **`risk.NewAuditSubscriber` is now idempotent.** Double-registration on the same bus would have persisted every risk event to JSONL twice — an audit-log integrity violation. A process-wide registry keyed by bus pointer tracks which bus instances have an active subscriber and returns the existing subscriber on subsequent calls.

### Added — DriftDetector v2 integration coverage (PR #704)

- `TestWave9Integration_DriftDetectorV2Flow`: end-to-end test for `NewDriftDetectorWithTargets` over a real `ChannelEventBus` verifying `SchemaVersion=2`, the `target_drift` and `concentration` reasons, and the v2-only payload fields (`target_weights`, `actual_weights`, `max_drift`, `max_drift_symbol`, `current_regime`).
- `TestWave9Integration_RegimeDebouncerDrivesDriftDetectorV2`: chain test for the `RegimeDebouncer → EventRegimeChangeConfirmed → DriftDetector v2` path confirming regime change triggers v2 detector re-baseline (`prevTotal = 0`) and updates `currentRegime`.

### Refactored

- `apievents.RegisterDashboardBufferSubs` extracted to its own file (`internal/monitoring/api/events/sse_handler_subscriptions.go`) and takes the `eventbus.EventBus` interface instead of the concrete `*eventbus.ChannelEventBus`.
- `risk.NewAuditSubscriber` keeps its existing single-arg signature; idempotency is internal.

### Testing

- 4 new TDD tests for `Wave9Observability.Start` cleanup behavior (parallel-detector failure, drift-detector failure, reference clearing, retry success) in `internal/monitoring/wave9_runtime_cleanup_test.go`.
- 3 new tests for the dashboard buffer subscription helper (`internal/monitoring/api/events/sse_handler_buffersubs_test.go`), with 15 sub-tests covering all 15 event types.
- 2 new integration tests for DriftDetector v2 + regime-to-drift chain in `internal/monitoring/service/wave9_integration_test.go`.

### Docs follow-up (PR #713, after this rebase)

- `internal/monitoring/AGENTS.md:194` — replaced pre-#704 `只回傳第一個` description with v0.0.0.18+ behavior: `errors.Join` aggregation + defer LIFO cleanup + reference clearing on retry.
- `internal/monitoring/AGENTS.md:209` — added `sse_handler_subscriptions.go` reference and the cross-mode `RegisterDashboardBufferSubs` re-registration pattern (`run()` + `runLiveTrading()` both call on their respective buses).
- `docs/ENVIRONMENT.md` — added "Story so far" paragraph describing how the 5-second dry-run smoke test could not exercise the 3 production bugs v0.0.0.18 closes.
- `docs/events/drift-detector.md` — added "v0.0.0.18+ 整合測試" section listing the 2 new bus-level integration tests.
- `docs/roadmap.md` — extended Wave 9 version list to include v0.0.0.18.
- `docs/modules/README.md` — bumped to v0.0.0.18 (Wave 9 gap fixes 收尾版).
- `README.md` (PR #718) — added v0.0.0.18 entry in Recent updates.
- `docs/operations_playbook.md` (PR #718) — added "Wave 9 觀測性 v0.0.0.18 修復與運維指引" section covering SSE catchup, audit subscriber idempotency, and partial-failure cleanup troubleshooting.

## [0.0.0.17] - 2026-06-24

### Added — Wave 9 observability wire completion: 5 detectors wired + BaselineTrigger (PR B + C)

Resolves the v0.0.0.16 CHANGELOG 「留待 v0.0.0.17 PR B/C」deferred scope. All 4 Wave 9 events now flow through the runtime stack with full lifecycle management.

#### PR B (PR #697) — 5 detectors wired

- **`internal/monitoring/wave9_runtime.go`** (new, +272 lines): `Wave9Observability` coordinator wiring RegimeDebouncer + FactorWeightRegressionDetector + DriftDetector (v2) + ChannelHealthSynthesizer + IngestionLagMonitor with Start/Stop lifecycle and LIFO shutdown order. Uses `detectorFactory` interface for testability.
- **`internal/monitoring/wave9_runtime_test.go`** (new, +354 lines): lifecycle + nil-guard + LIFO order tests.
- **`cmd/atlas/main.go`**: wire `Wave9Observability` in `runLiveTrading` with production providers from PR A. `defer dashEventBus.Close()` added for graceful shutdown.
- **Review fix (`c5cea33e`)**: nil-guard for `system.Port().FactorWeightEngine()` so startup is robust to partial initialization.

#### PR C (PR #698) — BaselineTrigger

- **`internal/baseline/trigger.go`** (new, +154 lines): `Trigger` struct subscribing to `EventPositionUpdate`, evaluating current `Policy` constraints (StopLossPct / TakeProfitPct / MaxHoldingDays) and logging violations via slog.
- **`internal/baseline/trigger_test.go`** (new, +253 lines): lifecycle + nil-checks + evaluation rules (164% test:prod ratio).
- **`cmd/atlas/main.go`**: wire `Trigger` as standalone lifecycle component in `runLiveTrading`.
- **Review fix (`c324d68c`)**:
  - `defer Stop()` wrapped in closure to log errors (errcheck linter).
  - `baseline.NewManager` hoisted to `run()` scope and passed into `runLiveTrading` (DI refactor — shared instance between api-mode and live-mode).
  - `TestRunLiveTrading_SharesBaselineManager` locks the contract.

#### Runtime impact

- **All 4 Wave 9 events now flowing in production**:
  - `portfolio.position.update` (PR A + D wired in v0.0.0.16)
  - `regime.confirmed` (PR B — RegimeDebouncer publishes)
  - `ingestion.lag.spike` (PR B — IngestionLagMonitor publishes)
  - `factor.weight.regression` (PR B — FactorWeightRegressionDetector publishes, when weights provided)
  - `EventDriftDetected` (PR B — DriftDetector v2 wired, consumes PositionUpdate + RegimeChangeConfirmed)
- **`BaselineTrigger`** (PR C) provides policy enforcement: position updates evaluated against `SimulationConstraints`, violations logged as warnings/errors.

#### Deviation from plan v2

- Plan v2 said 4 detectors + use BackgroundTaskManager. Actual: 5 detectors (ChannelHealthSynthesizer missing from plan) + `defer wave9.Stop()` pattern (event-driven lifecycle, not scheduled tasks — `internal/apigateway/CONSTITUTION.md` §4.5.2 exception).
- Plan v2 PR C was 「Layer 3 baseline CI scripts」; user directive was runtime 「EventPositionUpdate triggers BaselineTrigger evaluation」. Followed user directive.

#### Oracle audit

- Plan v2 addressed 9 findings (4 HIGH / 3 MEDIUM / 2 LOW).
- /review (focused) found P2/P3 concerns, all fixed before merge.

#### Verification

- `go build ./...` ✓
- `go vet ./...` ✓
- `gofmt -l` clean ✓
- All CI checks green for both PRs (#697 + #698) at merge time.


## [0.0.0.16] - 2026-06-24

### Added — Wave 9 observability wire: production providers + EventPositionUpdate caller (PR A + D)

PR #695 + PR #696 land in main, completing the v0.0.0.8 (2026-06-22) Wave 9 observability stack that was merged in known incomplete state (schema + subscribers present, but no production publisher or providers).

#### PR A (PR #695) — production providers

- **`apigateway/health.go`**: expose `ChannelIDs()` + `ChannelLatencyMs()` on `UnifiedHealthStore` (thread-safe via existing `RLock`).
- **`monitoring/service/ingestion_lag_provider.go`**: `ChannelHealthIngestionLagProvider` implements `IngestionLagProvider` via ceiling-rank p99 across registered channels.
- **`monitoring/service/weight_provider.go`**: `FactorWeightEngineWeightProvider` adapts `portfolio.FactorWeightEngine` to `WeightProvider` interface.
- **Tests**: 5 (lag) + 4 (weight) covering nil/empty/edge cases + regime switching.

#### PR D (PR #696) — EventPositionUpdate caller

- **`live/orchestrator.go`**: in `EventMarketSnapshot` critical handler, after `UpdatePositionPrices`, publish `EventPositionUpdate` with `changeType="updated"` for any held symbol.
- **`live/orchestrator_test.go`**: table-driven test verifying emission when position exists and silence when no position held, including `CurrentPrice` propagation.

#### Schema + event flow

- `EventPositionUpdate` now has 1 production caller (was 0 — dead code since v0.0.0.8).
- 4 個 events 中 1 個 (`portfolio.position.update`) 開始流通。
- 其餘 3 個 (`baseline.update`, `regime.confirmed`, `ingestion.lag.spike`) consumer wiring 留待 v0.0.0.17 PR B/C。

#### Oracle audit

- Plan v2 (.omo/plans/wave9-observability-wire.md) addressed 9 findings (4 HIGH / 3 MEDIUM / 2 LOW) from initial plan review.
- Provider wiring into monitoring service deferred to PR B (v0.0.0.17)。
- Fill-driven "added"/"removed" changeTypes deferred to PR B。

#### Verification

- 兩個 PR CI 全綠 (build / fmt / lint / test / security / integration / coverage / governance / constitution)
- gofmt 0 issues
- go vet 0 issues
- /review APPROVED for both PRs (oracle audit 9 findings addressed)


## [0.0.0.15] - 2026-06-24

### Fixed — DriftDetector v2 follow-up fixes (review-driven)

對 PR #692 的 pre-landing review 找到的 5 個 CRITICAL + 1 個 doc drift 全部修完。同步補上 3 個新 test 與 1 個 performance refactor。

#### Critical Fixes (6 commits)

1. **Silent failure on regime payload parse** (`5d7f3d5c`): `onRegimeChangeConfirmed` 在 `payload.(map[string]any)` 與 `payload["new_regime"].(string)` 兩處 type-assertion 失敗時,只 `return nil`,違反 `internal/monitoring/AGENTS.md` 4 層資料可見性規範。改為 emit `logging.Warn` 含 actual vs expected type,讓上游 schema regression 可被觀察。
2. **Provider called under `d.mu` lock** (`9246174d`): `checkPeriod` 持鎖時呼叫 `d.provider.GetTargetWeights(...)`,任何 DB-backed / HTTP-backed provider 會 deadlock。改為 3-phase(under-lock snapshot / no-lock provider call / under-lock publish),provider 不再阻塞 `onPositionUpdate` 與 `onRegimeChangeConfirmed`。
3. **`DriftEventSchemaVer=2` 漏到 v1 constructor** (`5c8c65e5`): 兩個 constructor 共用單一常數 bump 1→2,v1 detector 也 emit schema=2 但 payload 為 v1-shape,破壞 consumer 透過 `data.schema_version` dispatch 契約。拆分為 `DriftEventSchemaVerV1=1` 與 `DriftEventSchemaVer=2`,由 `schemaVersionFor(targetDriftChecked)` 動態選擇。新增 `TestDriftDetector_V1ConstructorEmitsSchemaVersion1` 鎖住契約。
4. **v1 constructor behavior leak** (`b7350570`): `Start()` 對兩個 constructor 都訂閱 `EventRegimeChangeConfirmed`,v1 detector (provider=nil) 開始 reset `prevTotal=0` 處理 regime 事件,與 v1 既有 no-op 行為不一致,rolling upgrade 期間舊/新 binary 會 emit 不同 event stream。改為僅在 `d.provider != nil` 時訂閱。新增 2 個 test 鎖住訂閱契約。
5. **Race test passes vacuously** (`d4fd65ca`): `TestDriftDetector_V2ConcurrentProviderAccess` 沒 assertion,沒 -race 時不驗任何東西。改為 `wg.Wait()` 後做一次 deterministic regime change 強制 `currentRegime=TEST` 與 `prevTotal=0`,然後 assert。
6. **AGENTS.md 不一致** (`4133bdb1`): DriftDetector v2 段的「Event Subscriptions」與「Stop() 必須取消兩個訂閱」trap 未反映新的 V1/V2 差異。補上。

#### Tests added (3)

- `TestDriftDetector_V2RegimeChangeTriggersNewProviderQuery`: regime 變化後的 checkPeriod 會用新 regime 呼叫 provider。
- `TestDriftDetector_V2EmptyRegimeStringPassesToProvider`: 沒有 regime 事件前,provider 用 `""` 呼叫。
- `TestDriftDetector_V2StopCancelsBothSubscriptions`: Start → Stop 不 panic。

#### Refactor

- `11fa1352`: `checkPeriod` 預先計算 `weights` map,消除 v2 階段重複的 `s.value/total` 除法。

#### Verification

- 21 drift tests 全綠(6 v1 + 14 v2 + 1 helper)
- `go test -race ./internal/monitoring/service/` clean
- `staticcheck` 0 issues

## [0.0.0.14] - 2026-06-24

### Added — Wave 9 follow-up: DriftDetector v2 Target Weights Drift

擴展 `internal/monitoring/service/drift_detector.go` v1 (189 行) 為 v2,新增 target weights drift 偵測。**條件**:Issue #611 refactor 已完成(`FactorWeightEngine.GetWeights(regime)` 介面化 + Optimizer 拆分),原本 v1 計劃書標註為 Out of Scope 的「v2 DriftDetector target weights drift」現在可獨立 PR。

#### 新增介面與建構式

- **`TargetWeightsProvider` 介面**(`drift_helpers.go`):`GetTargetWeights(regime string) map[string]float64` — symbol-level 目標權重(與既有 `WeightProvider` 為 factor-level 不同,**不可混用**)
- **`NewDriftDetectorWithTargets(bus, provider)` 建構式**:`DriftDetector` 介面不變,新增 DI 入口,provider 為 nil 時 graceful degradation(v1 行為完整保留)
- **`NewDriftDetector(bus)` 保留**:向後相容,無 target drift 功能

#### 新增 payload 欄位(v2,僅在 provider 非 nil 且回傳非空 map 時出現)

- `target_weights`:regime-snapshot 目標 symbol 權重
- `actual_weights`:當前 portfolio 實際 symbol 權重
- `max_drift`:`|actual - target|` 最大 drift
- `max_drift_symbol`:drift 最大的 symbol
- `current_regime`:當前 market regime(首次 regime change 前為空字串)
- `thresholds.target_drift`:0.10(**一律存在**,常數)

#### 新增事件訂閱

- **`EventRegimeChangeConfirmed`**(`regime_debouncer.go` 發布):觸發時更新內部 `currentRegime` 並重置 `prevTotal = 0` (re-baseline,避免 regime 切換時的偽 turnover 事件)

#### Schema 演進

- `DriftEventSchemaVer` 從 `1` bump 到 `2`
- v1 payload 欄位(`max_concentration` / `max_symbol` / `turnover` / `total_value` / `period_start` / `reasons` / `thresholds`)完整保留(append-only 演進)
- 消費者可透過 `data.schema_version` 判斷 v1 / v2

#### 測試覆蓋

- **9 個 v2 characterization tests**(`drift_detector_v2_test.go`):
  - `TestDriftDetector_V2TargetDriftEmitted`:drift > 10% emit + 驗證 v2 欄位
  - `TestDriftDetector_V2TargetDriftNoEmit`:target 對齊 + 平衡不 emit
  - `TestDriftDetector_V2NilProviderGraceful`:nil provider 保留 v1 行為
  - `TestDriftDetector_V2EmptyTargetWeights`:空 target map 跳過 target drift
  - `TestDriftDetector_V2RegimeChangeUpdatesCurrentRegime`:handler 更新 currentRegime
  - `TestDriftDetector_V2RegimeChangeRebaselinesPrevTotal`:regime change 重置 prevTotal
  - `TestDriftDetector_V2SymbolNotInTargetMap`:target=0 處理缺漏 symbol
  - `TestDriftDetector_V2SchemaVersionBumped`:SchemaVersion=2
  - `TestDriftDetector_V2ConcurrentProviderAccess`:concurrent 讀取無 race(-race flag)
- **v1 6 個 tests 一字不改**:全綠
- 15 個 drift tests 全部 PASS,全 `internal/monitoring/service/` 套件綠

#### 文件同步

- `docs/events/drift-detector.md`:Schema Version 2 + 5 個 v2 欄位 + 9 個 v2 測試描述
- `docs/events/INDEX.md`:EventDriftDetected 標記為 v2,Schema Version 說明段落更新
- `internal/monitoring/AGENTS.md`:新增 DriftDetector v2 段落(Architecture、Event Subscriptions、9 個模組陷阱、向後相容保證)

#### 與 PR #632 Wave 9 plan 的關聯

- 本 PR 收尾 Wave 9 plan §7 Risks 提到的「v2 DriftDetector target weights drift」follow-up
- Wave 9 plan Out of Scope 三項中此為收尾項

#### 已知限制 / 後續工作 (out of scope,follow-up PR)

- **本 PR 不做**:`internal/monitoring/service` 加入 Layer 3 baseline(目前 baseline 涵蓋 internal/config, cmd/atlas, internal/narrative, internal/orchestrator, internal/portfolio, internal/sim, internal/risk)。DriftDetector 介面為 public,後續 PR 應為其加 baseline。
- **本 PR 不做**:`cmd/atlas/main.go` wire `NewDriftDetectorWithTargets`(v1 已知未 wire,本 PR 維持 scope 嚴格)
- **本 PR 不做**:實作 symbol-level target weight provider(目前 `TargetWeightsProvider` 介面已備但無 production 實作;後續 PR 可從 portfolio Optimizer 衍生)

## [0.0.0.13] - 2026-06-23

### Added — P2/P3 Startup-Herd 回歸測試

`internal/apigateway/background.go` 的 runTask 內含「啟動抖動」邏輯（`time.Duration(rand.Int63n(int64(task.Jitter)))`），目的是防止多個 process 同時啟動（rolling deploy / 災難切換）時，所有 task 首次執行擠在 t=0 造成上游 provider thundering herd。既有測試僅驗證 `Register` 階段的 Jitter 欄位自動設定，沒有驗證 runTask 真的等待抖動。新增 2 個回歸測試守住此行為：

- **`TestBackgroundTaskManager_RunTask_AppliesStartupJitter`**：驗證 runTask 在首次執行前確實等待了 Jitter 設定的時間。Jitter=500ms，bounds=[1ms, 700ms]。下界 1ms 抓出「抖動被移除」的 regression（首執行會在 t≈0 < 1ms）；上界 700ms 容納 rand 抽到接近 500ms + Go runtime 排程誤差。偽陽性率 ≈ 0.2%。
- **`TestBackgroundTaskManager_RunTask_DesynchronizesMultipleTasks`**：驗證 5 個 task 的首次執行時間分散在 [0, 300ms) 區間（最晚 - 最早 ≥ 50ms）。若 `rand.Int63n` 被改成固定值（如 0），所有 task 會擠在 t=0，spread 趨近於 0，測試失敗。

兩測試合併 166 行註解 + 程式碼，覆蓋原本的測試缺口。**未修改 production code** — 抖動邏輯本身正確，僅補上守護測試。

### Test Coverage

- `internal/apigateway/background_test.go` +166 行（兩個 test function + 註解）
- `go test -race ./internal/apigateway/` 全綠（17.7s）
- `go vet` / `staticcheck` clean

## [0.0.0.12] - 2026-06-23

### Fixed — P2 PascalCase SessionSummary fields silently dropped

- 4 `SessionSummary` fields (`TaxSnapshots`, `AfterTaxPnL`, `TotalTaxPaid`, `ParametersVersion`) were defined in the Go struct but absent from the SQL table and all three persistence functions (`SaveSessionSummary`, `LoadSessionSummary`, `LoadAllSessionSummaries`), causing silent data loss on every save/load round-trip.
- Migration `000007_add_session_summary_tax_params` adds the 4 missing columns (`tax_snapshots JSONB`, `after_tax_pnl DOUBLE PRECISION`, `total_tax_paid DOUBLE PRECISION`, `parameters_version TEXT`).
- Updated `SaveSessionSummary` INSERT/UPDATE to include `$15–$18`; updated `LoadSessionSummary` and `LoadAllSessionSummaries` SELECT + Scan to include the 4 new columns, plus `taxSnapshots` JSON unmarshal.
- Added `TestPostgresRepository_SessionSummary_TaxAndParamsFields` round-trip test covering all 4 fields.
- `go vet` and `staticcheck` clean.

## [0.0.0.11] - 2026-06-23

### Fixed — FinMind Trading-Day Guard (P1)

- **`internal/marketdata/finmind_client.go`**:
  - **New `isTaiwanTradingDay(t time.Time) bool` helper**: returns `false` for Saturday and Sunday. Hooked into `FinMindProvider.GetQuotes` as the first step — if `asOf` falls on a weekend, return an explicit error `"finmind: asOf YYYY-MM-DD is not a Taiwan trading day (weekend or holiday)"` and skip the HTTP call entirely.
  - Before: `GetQuotes` would call FinMind's `TaiwanStockPrice` dataset with a weekend date; FinMind returns `{"data":[]}` (empty array, not an error); the code then fell through `len(data) == 0` and returned `"finmind: no price data for 2330 on 2026-04-25"` — a confusing message that looks like a symbol/date mismatch rather than a non-trading-day query.
  - After: weekend queries are caught at the provider boundary with a self-explanatory error and zero HTTP calls (saves rate-limit budget). Callers that want the previous trading day's data should rewind `asOf` explicitly.
  - **Holiday support deferred**: fixed-date Taiwan holidays (元旦, 228, 清明, 端午, 中秋, 雙十) are not yet encoded. The helper name `isTaiwanTradingDay` (vs. `isWeekend`) signals that holiday support is intended; future work should source holidays from `globalmarket.TradingSchedule.Holidays` or a config file rather than hardcoding per year.

- **`internal/marketdata/finmind_client_extra_test.go`**:
  - **`TestFinMindProvider_GetQuotes_RejectsSaturday`**: `asOf = 2026-04-25` (Saturday) → asserts error contains `"not a Taiwan trading day"` and the mock server receives **0 HTTP calls**.
  - **`TestFinMindProvider_GetQuotes_RejectsSunday`**: `asOf = 2026-04-26` (Sunday) → same assertions.
  - The existing `TestFinMindProvider_GetQuotes_PartialSuccess` (Wednesday 2026-04-29) still passes — guard only fires on weekends.

### Test Coverage

- 2 new tests, all passing under `go test -race -count=1 ./internal/marketdata/` (suite: ~40 tests, 38.4s).
- `go vet` and `staticcheck` clean.

### Reproduction / Evidence

- Before: `GetQuotes(ctx, time.Date(2026,4,25,...), ["2330"])` → 1 HTTP call to FinMind → empty `data` array → `"finmind: no price data for 2330 on 2026-04-25"` error. Operator cannot tell whether the symbol is wrong, the date is wrong, or it's a non-trading day.
- After: same call → 0 HTTP calls → `"finmind: asOf 2026-04-25 is not a Taiwan trading day (weekend or holiday)"`. Operator immediately knows to rewind to the previous trading day (2026-04-24, Friday).

## [0.0.0.10] - 2026-06-23

### Fixed — us10y Macro Indicator Zero-Value Guard (P1)

- **`internal/marketdata/yahoo_macro_provider.go`**:
  - **New zero-value guard** in `fetchIndicator()`: after the existing `NaN`/`Inf` check, reject `latest == 0` as a data error. Yahoo Finance returns `closes: [0.0, 0.0, ...]` during US market off-hours or parse failures; without this guard the zero propagates into `MacroDataSnapshot.US10Y.Value = 0` and pollutes downstream yield-spread / US-TW rate differential / stress-index calculations.
  - All 8 tracked macro indicators (`^TNX`, `DX-Y.NYB`, `^VIX`, `CL=F`, `GC=F`, `USDTWD=X`, `SI=F`, `HG=F`) are never exactly zero in real markets, so the guard applies uniformly. The error message includes the ticker and the hint `likely off-hours or parse error` for operator triage.
  - On rejection, the field is left empty in the snapshot (existing `mergeSnapshot` last-write-wins semantics with non-empty `Symbol` check already handles this), and `FetchSnapshot` returns a partial-failure error so callers can detect the degraded state.

- **`internal/marketdata/yahoo_macro_extra_test.go`**:
  - **`TestYahooFinanceMacroProvider_fetchIndicator_ZeroLatestPrice`**: mock Yahoo returns `closes: [0.0, 0.0, 0.0]` for `^TNX` → asserts `fetchIndicator` returns an error containing `zero latest price`.
  - **`TestYahooFinanceMacroProvider_FetchSnapshot_ZeroValueExcluded`**: `^TNX` returns zero (rejected), all other 7 indicators return valid data → asserts `snap.US10Y.Symbol == ""` and `snap.US10Y.Value == 0` (field excluded), `snap.DXY.Value == 104.18` (success path still populates), and `err != nil` (partial failure surfaced).

### Test Coverage

- 2 new tests, all passing under `go test -race -count=1 ./internal/marketdata/` (suite: ~40 tests, 40.7s).
- `go vet` and `staticcheck` clean.

### Reproduction / Evidence

- Before: `YahooFinanceMacroProvider.FetchSnapshot` would happily set `US10Y.Value = 0.0` when Yahoo Finance returned zero closes (e.g., early Monday morning US time, or post-holiday data gaps). Downstream consumers (`narrative`, `taiwan_stress_index`, `risk` modules) would then treat 0 as a real rate, producing nonsensical yield-spread signals.
- After: zero is rejected at the provider boundary, the snapshot field is left empty, and the partial-failure error flows to the caller. Downstream code that already checks `Symbol != ""` before reading `Value` continues to work unchanged; code that didn't check now gets an empty field instead of a poisoned zero.

## [0.0.0.9] - 2026-06-23

### Fixed — FubonProxy Port Conflict on Restart (P0)

- **`internal/fubonproxy/manager.go`**:
  - **New `preparePortForRestart()` helper**: probes port 8081 before each restart and returns a 3-state verdict — `(canProceed bool, shouldStop bool)`. Replaces the old "blindly respawn" behavior that caused supervisor to thrash when port was held by a foreign process.
    - `Free` → restart normally, reset `restartFailures` counter.
    - `Healthy` (port serves `/health` and PID is not ours) → supervisor yields to the external managed proxy, logs `restart_external_managed`, and exits.
    - `Foreign` (port held by a process that is not healthy / not ours) → log actionable error `restart_foreign_port` with the offending PID + `kill` command hint, increment `restartFailures`, refuse to respawn.
  - **New `maxRestartFailures = 5` constant** + `restartFailures` field on `ProcessManager`. `supervise()` gives up after 5 consecutive blocked restarts and emits `max_restart_failures_reached` to prevent infinite crash-loop.
  - **`supervise()` updated**: calls `preparePortForRestart()` before every respawn, not just at startup. This closes the gap where a proxy that died and got stuck on a foreign port would trigger an unending respawn cycle.
  - Test-only backoff seam (`restartInitialDelayForTest` / `restartBackoffDelayForTest`) introduced so the cap test can run in ~3s instead of the production `restartInitialDelay` schedule.

- **`internal/fubonproxy/manager_test.go`**:
  - **`TestProcessManager_Restart_PortFree_CanProceed`**: bare port → `preparePortForRestart` returns `canProceed=true, shouldStop=false`.
  - **`TestProcessManager_Restart_PortHealthy_Yields`**: port held by a `/health`-serving process → returns `canProceed=false, shouldStop=true` and logs `restart_external_managed`.
  - **`TestProcessManager_Restart_PortForeign_Retries`**: port held by an unknown process → returns `canProceed=false, shouldStop=false` and logs actionable `restart_foreign_port` with the PID and `kill` command.
  - **`TestProcessManager_Supervise_YieldsToExternalHealthyProxy`**: end-to-end `supervise()` yields and stops when the port becomes healthy externally between restarts.
  - **`TestProcessManager_Supervise_RestartFailureCap`**: 5 consecutive `Foreign` verdicts → `supervise()` logs `max_restart_failures_reached` and exits cleanly without infinite loop.

### Test Coverage

- 5 new tests, all passing under `go test -race -count=1 ./internal/fubonproxy/` (suite: 19 tests, 42.4s).
- `go vet` and `staticcheck` clean.

### Reproduction / Evidence

- Before: supervisor would loop forever respawning fubon-proxy against a foreign port-holder, with no failure cap and no yielding to a healthy external instance.
- After: port-conflict restart attempts are bounded (max 5), the supervisor yields to a healthy external proxy instead of fighting it, and the operator gets an actionable error message (`kill <pid>`) on each blocked attempt.

## [0.0.0.8] - 2026-06-22

### Added — Wave 9 YELLOW Observability Expansion (5/5 events shipped)

- **`EventChannelIndividualHealth`** (`monitor.channel.health.individual`, Wave 9.1): per-channel error visibility for the 4-layer data-visibility safeguard. Service: `internal/monitoring/service/channel_health_synthesizer.go`. Polls `ChannelErrors()` every 30s with 5s dedup. Provider injected via `ChannelHealthProvider` interface (no `internal/monitoring` import).
- **`EventRegimeChangeConfirmed`** (`market.regime.confirmed`, Wave 9.2): regime change is only confirmed after 30s stability window. Service: `internal/monitoring/service/regime_debouncer.go`. Subscribes to `EventRegimeChange`, checks every 5s, dedupes by `newRegime`.
- **`EventFactorWeightRegression`** (`portfolio.factor.regression`, Wave 9.3): when regime changes, the factor weight shift is computed as `Σ|curr - prev|`. Emit if score ≥ 0.5. Service: `internal/monitoring/service/factor_weight_regression.go`. Constructor DI: `NewFactorWeightRegressionDetector(bus, provider WeightProvider)`. `monitoring/service` does NOT import `portfolio` (forward-compat with #611).
- **`EventDriftDetected`** (`portfolio.drift.detected`, Wave 9.4): v1 concentration drift + simple turnover ratio. Service: `internal/monitoring/service/drift_detector.go`. Subscribes to `EventPositionUpdate` (per-symbol, no portfolio snapshot required). Thresholds: concentration > 0.25 OR turnover > 0.15. v2 (target weights drift) deferred to #611 refactor.
- **`EventIngestionLagSpike`** (`apigateway.ingestion.lag.spike`, Wave 9.5): ingestion p99 > 5s triggers warning. Service: `internal/monitoring/service/ingestion_lag_monitor.go`. Provider interface: `IngestionLagProvider.P99LatencySeconds() float64`. **Follow-up**: `internal/apigateway/background.go` add `ingestion_latency_seconds` Prometheus histogram + implement `IngestionLagProvider`.

### Added — Wave 9 Infrastructure

- 5 EventType constants + `eventDescriptions` entries in `internal/eventbus/eventbus.go` (Wave 9.0a)
- 4 service framework interfaces in `internal/monitoring/service/` (Wave 9.0b): `WeightProvider`, `RegimeDebouncer`, `DriftDetector`, `ChannelHealthSynthesizer` (replaced with full implementations in 9.1-9.5)
- 5 Prometheus alert rules in `monitoring/rules/wave9_*.yml` (all `enabled: false` by default per PD-W9-1)
- 5 new docs in `docs/events/`: `channel-individual-health.md`, `regime-change-confirmed.md`, `factor-weight-regression.md`, `drift-detector.md`, `ingestion-lag-spike.md`

### Changed — Forward-Compat Design Verified

- 0 modifications to Issue #611 9-file refactor targets (verified by `git diff --stat`)
- All Wave 9 services implement forward-compat DI: `monitoring/service` depends only on `eventbus` package, not on `portfolio` / `monitoring` / `apigateway`
- Alert rules default to `enabled: false`; operator must explicitly enable per PD-W9-1

### Test Coverage

- 5 service test files added, 32 test functions total
- Race conditions tested via `go test -race` for each event handler
- Dedup windows, threshold boundaries, nil-provider no-panic paths all covered

### Out of Scope (follow-up)

- **Frontend SSE integration** for the 5 new events: needs updates to `internal/monitoring/api/events/sse_handler.go` (6-component buffer) and `web/static/js/` event rendering. Tracked as separate task.
- **IngestionLagProvider implementation** in `internal/apigateway/background.go` (additive change, not blocked by #611).

## [0.0.0.7] - 2026-06-22

### Added — Wave 8 Event-Driven Expansion (6/9 RED events shipped)

- **`EventRiskGateRejected`** (`monitor.risk_gate.rejected`, PR #619): emitted when RiskGate verdict is `BLOCK` or `HALT`. Producer bridge wired at `cmd/atlas/main.go:1603-1614`. SSE-delivered with 50-event catch-up buffer.
- **`EventRiskGateAllowed`** (`monitor.risk_gate.allowed`, PR #619): emitted when RiskGate verdict is `ALLOW`. Three-way semantic split introduced in Wave 8.2 收尾.
- **`EventRiskGateOverridden`** (`monitor.risk_gate.overridden`, Wave 8.2 收尾): NEW constant, emitted when RiskGate verdict is `REDUCE` or `ALERT_ONLY`. Fills the semantic gap between full-allow and full-block; frontend can render distinct badges without parsing `payload.Verdict`.
- **`EventIndustryCalendar`** (`industry.calendar.event`, PR #621): emitted by `PublishIndustryCalendarEvent` for Taiwan market calendar events (除權息、MSCI 調整、財報季等).
- **`EventBacktestCompleted`** (`experiment.backtest_completed`, PR #622): emitted after `internal/autobacktest.Runner.RunAndStore` succeeds and live store is synced.
- **`EventCalibrationCompleted`** (`experiment.calibration_completed`, PR #623): emitted after `cmd/atlas/main.go` `linkage_calibrate` task completes `CalibrateParameters`.
- **`EventTradeSlippage`** (`trade.slippage`, PR #625): emitted by `internal/live/order_manager.go` on every order fill (status == "filled"); records expected vs actual price in BPS.

### Changed — RiskGate Three-Way Semantic Split (Wave 8.2 收尾)

`PublishRiskGateEvent` auto-routing refactored from 2-way (rejected/allowed) to **3-way split**:
- `BLOCK` / `HALT` → `EventRiskGateRejected`
- `REDUCE` / `ALERT_ONLY` → `EventRiskGateOverridden`
- `ALLOW` → `EventRiskGateAllowed`

This preserves the semantic distinction between "fully allowed", "modified after override" (partial reduction or alert-only warning), and "blocked entirely". Test coverage locked via `TestPublishRiskGateEvent_ThreeWayRouting`.

### Documentation — Wave 8.10 Docs 收尾 + Wave 8.2 收尾

- PR #627: 補寫 3 個既有事件 doc（`narrative-event.md`, `health-alert.md`, `promotion-recorded.md`）+ 更新 INDEX.md + P3 編號對齊。
- Wave 8.2 收尾: 新建 `docs/events/risk-gate-overridden.md`；更新 `docs/events/risk-gate-allowed.md` 反映純 ALLOW 語意。
- `docs/events/INDEX.md`: 加入 `EventRiskGateOverridden` 列 + Wave 8.11+ LLM 事件推遲註記。

### Deferred — LLMAnnotator 3 events pushed to Wave 8.11+

- `LLMAnnotatorCircuitOpen` (Wave 8.5): 原計畫實作 LLM circuit breaker 事件。LLM 重構（PR #628/#629）改為 capability-based routing，原 circuit breaker 由 `llm_annotator:requests_good:rate5m` Prometheus metric + `llm_annotator_availability_fast_burn` alert rule 取代（`monitoring/rules/llm_annotator_alerts.yml`）。
- `LLMAnnotatorFallbackUsed` (Wave 8.6 LLM): 同上，fallback 路徑由 router logs 與 metrics 揭露。
- `LLMAnnotatorQuotaExceeded` (Wave 8.7): 同上，quota 控管整合進 router 計費。

Wave 8.11+ 規劃待 Wave 8 v0.0.0.7 合併後再開新 plan。

### Added — Phase 4 LLM Loop Coverage (PR #628/#629 follow-up)

- **`ConfidenceCommentary` hook verification tests**: `internal/risk/confidence_hook_test.go` mirrors `forensics_hook_test.go` (3 cases: hook called / nil hook / error returns empty). `internal/risk/gate_test.go` adds 2 integration tests verifying `RiskGate.publish()` writes `ConfidenceCommentary` to subscribers.
- **`docs/llm-trigger-analysis.md` updated**: All 5 LLM hooks (RationaleTranslator, ScenarioExplainer, RegimeExplainer, SentimentExplainer, PerformanceForensics, ConfidenceCommentary) marked ✅ RESOLVED with production caller line numbers (`cmd/atlas/main.go:1892/1903/1915/1937/1949`, `internal/narrative/ingestor.go:139`, `internal/orchestrator/system.go:521`, `internal/risk/gate.go:174`).

### Added — PR #630 SmartUniverseBuilder pipeline (related infra)

- 4-layer universe pipeline (`IndustryFilter` / `ScoringScreener` / `RiskExclusionFilter` / `NarrativeEventBridge`) with `WriteUniverseRegistry` atomic-write + `.bak` rollback. Wired into `cmd/atlas/main.go` with `WatchlistMu` serialization.
- Review audit trail archived to `docs/archive/REVIEW_PR630.md`.

## [0.0.0.6] - 2026-06-20

### Added

- **Wave 7.5 Tasks 1+2 — Risk gate safety wiring + orphan config rejection**: risk gate controls now enforce explicit safety limits before promotion, and the system rejects orphaned/misplaced `parameters.json` files that would previously silently merge.
- **Wave 7.5 Tasks 3+5+6 — Audit fixes**: Alertmanager webhook receiver hardened with proper field validation and HTTP status codes; field contract checks updated for the new valid-fields registry; calibration metadata preservation improved across auto-rollback scenarios.
- **Wave 7.5 finalization — Auto promotion events**: `AutoJudgePromoter` is now wired into the atlas scheduler; when an experiment is auto-promoted, an `EventPromotionRecorded` event is emitted and delivered to dashboard clients via SSE with a 50-event catch-up buffer.
- **`GET /api/dashboard/fetch-log` endpoint**: returns recent channel fetch events (`status`, `latency_ms`, `error`) from the persistent ring buffer, surfaced in the data-channel dashboard.

### Fixed

- `internal/alerting/webhook_handler.go` now returns `400 Bad Request` for malformed Alertmanager payloads and `422 Unprocessable Entity` for missing required fields instead of silently succeeding.
- `internal/monitoring/channel_health.go` now records per-channel failure reasons, so the fetch log and degraded-status panels show why a channel failed.

### Changed

- Risk gate panel UI (`web/static/js/components/risk-gate-panel.js`) now displays rejection reasons and inline safety override controls.
- Channel fetch log entries are now written by all CLI ingestion tools via `monitoring.RecordChannelFetch`, producing a single observability source for dashboard and alerting.

## [0.0.0.5] - 2026-06-17

### Breaking — Performance Report Field Renames + Threshold Config

**`AgentContribution.TotalReturn` → `AgentContribution.AggregateForwardReturn`** (JSON `total_return` → `aggregate_forward_return`).
**`RegimePerformance.TotalReturn` → `RegimePerformance.AggregateForwardReturn`** (same JSON rename).
**`AgentContribution.SharpeLike`**: `float64` → `*float64`. Now nullable when samples < `reporting.sharpe_min_samples` (default 5) or stdDev == 0. Frontend renders `"N/A"` for null.

These three fields exist in the `GET /api/performance-report` response payload (and the in-process `PerformanceReport` struct used by `cmd/judge-experiment`, `cmd/promote-baseline`, etc.). Frontend code must read the new field name and dereference `sharpe_like` defensively.

### Added — Cost-Adjusted Win-Rate Threshold

`reporting.win_rate_threshold` parameter (default 0.002, i.e. 0.2%). Win classification now requires `ForwardReturn > win_rate_threshold` instead of `ForwardReturn > 0`, covering transaction cost (~0.15% TW market) + slippage buffer. Configurable via `configs/parameters.json`. Affects `calculateTradeMetrics`, `calculateTopAgents`, and `calculateRegimeBreakdown`.

### Fixed — Fubon Proxy: Remove `FUBON_PROXY_URL` env override (IPv6 dual-stack root cause)

The recurring fubon channel failures (`dial tcp [::1]:8081: connect: connection refused`) were traced to a single design defect: `fubon_client.go` and `hybrid_provider.go` both read `os.Getenv("FUBON_PROXY_URL")`, which could override the safe hardcoded default `127.0.0.1:8081` with `localhost:8081` — resolved to IPv6 `[::1]` on macOS dual-stack systems while the Python fubon-proxy binds IPv4 only.

**Changes**:
- `internal/marketdata/fubon_client.go`: Replaced `os.Getenv("FUBON_PROXY_URL")` fallback in `newFubonClient()` with direct `fubonProxyBaseURL` constant. Removed unused `"os"` import.
- `internal/marketdata/hybrid_provider.go`: Removed both `os.Getenv("FUBON_PROXY_URL")` reads in `NewHybridProvider()`; always probes `127.0.0.1:8081` directly. Removed unused `"os"` and `"net/url"` imports.
- `.env_example`: Removed `FUBON_PROXY_URL` line and IPv6 warning comment (env override no longer exists).
- `.env.example`: Removed `FUBON_PROXY_URL` line (commented-out `localhost:8081` default).

This is the B-plan from PR #556 that was never implemented — the final root cause fix after 22 commits and 17+ PRs of layered defenses (circuit breaker, probe, auto-start, panic recovery, zombie kill) that never addressed the `.env` → env-override path.

### Added — `/api/dashboard/agent-names` endpoint

New endpoint serving the agent display-name registry from `configs/agents.json` as JSON. Single source of truth replacing the two competing static maps (`web/static/js/names.js` and `web/static/js/shared/constants.js`). Returns `{"agents": [{"id", "name", "skill", "layer"}, ...]}` or empty `{"agents": []}` when file is missing.

## [0.0.0.4] - 2026-06-15

### Fixed — Pipeline Data Visibility (6 commits, P0-P2)

**P0-C/D/E (20d1f56e) — frontend zero-value display**:
- `computePipelineSummary`: fallback `items` to `outcome_count` when summary missing.
- `formatDate`: filter zero-time (year<2000, NaN, year>9999), return `"-"`.
- `regimeLabel`: unify `"unknown"` → `"-"` across all 3 rendering paths.

**P1-A (ce4d89fc) — pipeline status banner**:
- `buildPipelineStatusBanner`: 5-status handler (`ok/degraded/minimal/no_session/error`).
- `is_fallback_session` as independent dimension.

**P1-B (366151b7) — OutcomeCount fallback**:
- `LoadSessions`: when `OutcomeCount==0`, derive from `recommendation_outcomes.jsonl` line count.
- Only overwrites zero — preserves summary's post-filter semantics.

**P1-C (0a553e4f) — backfill-summaries tool**:
- `cmd/backfill-summaries`: one-shot CLI for repairing orphan session directories.
- `internal/backfill/` package with `BackfillSummaries()` — idempotent, dry-run, never overwrites existing.
- 6 test cases covering orphan/existing/empty/mixed/noop scenarios.

**P2-A (36ac8a87) — RecordSessionSummary retry**:
- `recordSummaryWithRetry`: 3 attempts, 100ms linear backoff.
- Single chokepoint for all production summary writes.

**P2-B (4e4e97fa) — data_status sibling**:
- `parseSessionsList`: surface `data_status` as sibling field in array response.

## [0.0.0.3] - 2026-06-15

### Fixed
- `internal/live`: `TestOrderManager_Run_BrokerRejectsOrder` was flaky in `go test ./internal/live` — the assertion read the first event from the SubscribeAll channel, but `ChannelEventBus` dispatches handlers in their own goroutines, so `order.rejected` could arrive before `order.error`. The test now drains error events until it finds the expected `EventOrderError` (1 commit, 7906284b).

## [0.0.0.2] - 2026-06-14

### Added — Coverage Push (Stages 1-6 of functional-coverage-fix plan)

Total coverage: 57.6% → 61.1% across 7 commits on `feat/coverage-improvement`.

**Stage 1 (f1d6f712) — `feat(config,domain)`**:
- `internal/config`: restore `mergeFallbackPriceTargetsDefaults` helper to merge missing/partial `FallbackPriceTargets` entries from defaults.
- `internal/domain/recommendation`: add `Regime string` field to `RecommendationOutcome` with `json:"regime,omitempty"` tag.

**Stage 2 (34db5301) — `feat(monitoring,orchestrator)`**:
- `internal/monitoring/service`: per-regime grouping in `computeAgentRegimeBreakdown` (uses `o.Regime` with fallback to `defaultRegime`).
- `internal/orchestrator`: populate `RecommendationOutcome.Regime` in `buildSyntheticOutcomes` and `buildReplayOutcomes` (and corresponding `prism_executor.go`, `adversarial_executor.go`).

**Stage 3 (54b4442e) — `test(monitoring)`**:
- 9 test files in `internal/monitoring/` root (gateway_adapter, alert_api, alert_store, autohandler, channel_health, dashboard_api, metrics, risk_calibrator, new data_quality).
- Coverage: 50.1% → 69.7% (+19.6pp).

**Stage 4 (f62b7182) — `test(apigateway)`**:
- 15 test files covering 20 previously-zero `Fetch`/`HealthCheck`/`RateLimit` functions across 10 adapters.
- Coverage: 55.4% → 60.4% (+5.0pp). 24 funcs remain blocked pending marketdata HTTP-client injection (Stage 5c).

**Stage 5 (4e070c60) — `test(repository,shared,marketdata,apigateway)`**:
- `internal/repository`: 12.4% → 76.7% (+64.3pp). Added `pgPool` interface for testability (option C: Doer abstraction) + 637-line `postgres_unit_test.go` using pgx fake pool.
- `internal/domain/shared`: 28.4% → 100% (+71.6pp). 3 new test files covering 5 helpers.
- `internal/marketdata`: added `SetHTTPClient(c *http.Client)` testability hook to 10 providers (option A: least invasive). Coverage 49.1% → 53.2%.
- `internal/apigateway`: completed the 24 previously-blocked adapter funcs via new `adapter_http_fetch_test.go`. Coverage 60.4% → 79.0% (+18.6pp).

**Stage 5 follow-up (337f6647) — merge origin/main**:
- Integrated PR #526 (pipeline degraded status), #527 (Minimal/NoSession tests), #528 (sectorallocation module), #529 (wave4-cleanup).
- 4 conflicts resolved in favor of main per user priority instruction: `monitoring/service/pipeline.go` (semantically equivalent), `orchestrator/system.go` (API signature change `domain.Regime` → `string`), and 2 test files.
- Stage 2 regime population preserved at `buildSyntheticOutcomes` line 1442 and `buildReplayOutcomes` line 1492.

**Stage 6 (999b1fb6) — `test(monitoring/api)`**:
- 8 test files across 7 sub-packages (test-only scope, skipped `api/pipeline` per user priority):
  - `narrative` 8.4% → 90.8% (+82.4pp)
  - `macro` 10.0% → 70.0% (also fixed 2 pre-existing compile errors in `handlers_stub_test.go`)
  - `live` 20.2% → 88.2% (+68.0pp)
  - `industry` 46.6% → 77.7% (+31.1pp)
  - `tax` 47.4% → 80.8% (+33.4pp)
  - `dashboard` 49.6% → 70.9% (+21.3pp)
  - `shared` 98.8% → 98.8% (already at max, no change)

### Notes
- Pre-existing data races in `internal/live` (3 tests) are NOT caused by this push; they were introduced by `b39fb5b9 test(live): cover scheduler, store, twse_adapter, agent_runner, orchestrator, order_manager` which is on main.
- `gitnexus detect_changes` deferred: index stale (last indexed `891e724`); will refresh in follow-up.
- 7 commits pushed: `f1d6f712`, `34db5301`, `54b4442e`, `f62b7182`, `4e070c60`, `337f6647`, `999b1fb6`.

## [0.0.0.1] - 2026-06-13

### Fixed
- `internal/config`: merge `FallbackPriceTargets` defaults to prevent a panic when `_default` is missing and preserve custom per-stage overrides.

### Added
- `TestLoadParametersConfig_FallbackPriceTargetsDefaultsMerged` to verify `_default` and custom key merge behavior.
