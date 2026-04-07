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

- Real broker integration
- Live order management
- Risk circuit breakers
- Performance reporting
