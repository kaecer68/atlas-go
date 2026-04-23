# Roadmap

## Phase 1: Foundation

- initialize standalone repository
- add GitHub workflow and templates
- define canonical domain types
- implement simple simulation engine
- support mock and replay market data providers

## Phase 2: Taiwan Replay MVP

- TWSE and TPEX daily adapters
- daily replay runner
- scorecard and performance ledger
- first sector and macro agent prompts
- sessionized replay artifacts

## Phase 3: OpenClaw Training Loop ✅

- agent registry
- prompt versioning
- rolling evaluation windows
- keep-or-revert experiment flow
- ledger-backed weakest-agent selection
- multi-session backtest windows
- intelligent mutation type selection (prompt_tightening, risk_rule_change, portfolio_constraint_revision)
- interactive mutation proposal scripts

## Phase 4: Near-Real-Time Paper Trading ✅

- Fugle snapshots ✅
- TWSE OpenAPI integration ✅
- Hybrid Provider (Fugle + TWSE fallback) ✅
- Live State Store ✅
- Event-driven orchestration ✅
- Monitoring and alerting ✅

## Phase 5: Portfolio Intelligence ✅

- Multi-factor portfolio optimization ✅
- Risk-adjusted position sizing ✅
- Dynamic regime-based allocation ✅
- Agent weighting system ✅
- Style rotation detection ✅
- Post-trade analysis ✅

## Phase 6: Production Trading (Next)

- Decision chain transparency (FactorScores breakdown, ConvictionBreakdown, MacroEvent confidence) ✅
- Real broker integration
- Live order management
- Risk circuit breakers
- Performance reporting

## Execution Roadmap (2026 Q2-Q4)

### Short Term (2-6 weeks)

- Freeze proposal/commit/event contracts and add validation checks
- Implement decision state machine with explicit transition guards
- Persist trace fields (`proposal_id`, `commit_id`, `approval_id`) in ledger artifacts
- Build guard pipeline v2 with auditable reject reasons
- Deliver first dashboard APIs for macro radar, agent observatory, and forecast-vs-reality

Exit criteria:
- Contract tests pass and are versioned
- Replay runs remain deterministic
- Every experiment record can be reconstructed end-to-end

### Mid Term (6-12 weeks)

- Add macro event ingestion pipeline with factor timeline snapshots
- Add parallel simulation scenarios (base, stress, shock)
- Wire human approval/revert flow into command path
- Introduce weekly governance review with acceptance scorecards

Exit criteria:
- Scenario comparisons are stable across repeated runs
- Human-in-the-loop checkpoints are enforced before promotion
- Guard outcomes are visible in monitoring panels

### Long Term (3-6 months)

- Integrate production broker adapter with strict circuit breakers
- Add SLO-driven operations dashboards and incident runbooks
- Run staged drills for rollback and degraded-data operation
- Promote to production with controlled capital ramp

Exit criteria:
- Staging drills pass with documented evidence
- Rollback completes within defined operational window
- Risk and audit controls satisfy production checklist
