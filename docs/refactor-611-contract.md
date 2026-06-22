# Issue #611 Refactor Contract — Wave 9 Forward-Compatibility Interfaces

> **Created**: 2026-06-22
> **Purpose**: Freeze 6 contracts that MUST be preserved during the #611 9-file refactor.
> **Rule**: Any change to a contract below requires a dedicated PR with `gitnexus_impact` blast radius analysis.

---

## Premise Corrections

Two interfaces listed in the original Phase 0 context do NOT exist in the codebase:

| Claimed | Reality | Impact |
|---------|---------|--------|
| `WeightProvider` interface in `factor_weight_regression.go` | File does not exist. `GetWeights` is a method on concrete struct `*FactorWeightEngine`. | Refactor must preserve the method signature, not an interface. |
| `ChannelHealthProvider` interface in `gateway_adapter.go` | Interface does not exist. `ChannelErrors()` is an extension method on `*macroDataGatewayAdapter`. `MacroDataProvider` interface does NOT declare it. | Do NOT add `ChannelErrors` to `MacroDataProvider` — would break `CompositeMacroProvider`, `YahooMacroProvider`, `BDIProvider`, `HybridProvider`. |

---

## Contract 1: GetWeights (concrete method)

- **Location**: `internal/portfolio/factor_weight_engine.go:79`
- **Signature**: `func (e *FactorWeightEngine) GetWeights(regime string) map[FactorType]float64`
- **Return type**: `map[FactorType]float64` (NOT `map[string]float64`)
- **FactorType**: `type FactorType string` (`internal/portfolio/optimizer.go:17`)
- **Callers**:
  - `internal/portfolio/optimizer.go:260` — `factorWeights = fwe.GetWeights("")`
- **Existing tests**: `internal/portfolio/factor_weight_engine_test.go` (30+ tests)
- **Blast radius**: 🔴 HIGH — changing return type breaks optimizer + CI integrity check
- **Refactor rule**: Do NOT change return type. Do NOT change method name. Do NOT remove `regime` parameter.

---

## Contract 2: OnRegimeChange (concrete method)

- **Location**: `internal/portfolio/factor_weight_engine.go:179`
- **Signature**: `func (e *FactorWeightEngine) OnRegimeChange(oldRegime, newRegime string, confidence float64)`
- **Callers**:
  - `internal/orchestrator/system.go:269` — `port.factorWeightEngine.OnRegimeChange(string(p.OldRegime), string(p.NewRegime), p.Confidence)`
  - `internal/orchestrator/system.go:435` — `s.Port().factorWeightEngine.OnRegimeChange(string(oldRegime), string(regime), 0.0)`
  - `internal/orchestrator/system.go:718` — `s.Port().factorWeightEngine.OnRegimeChange(string(oldRegime), string(regime), 0.0)`
- **Existing tests**: `internal/portfolio/factor_weight_engine_test.go`
- **Blast radius**: 🔴 HIGH — 3 call sites in `system.go` (a #611 target file)
- **Refactor rule**: Do NOT change signature. If `system.go` is refactored, all 3 call sites must be preserved.

---

## Contract 3: EventRegimeChange + PublishRegimeChange + RegimeEventPayload

- **Constant**: `internal/eventbus/eventbus.go:26` — `EventRegimeChange EventType = "market.regime.change"`
- **Publisher**: `internal/eventbus/eventbus.go:604` — `func (b *ChannelEventBus) PublishRegimeChange(oldRegime, newRegime domain.Regime, confidence float64, determinedBy string)`
- **Payload**: `internal/eventbus/eventbus.go:100-105` — `RegimeEventPayload{OldRegime, NewRegime, Confidence, DeterminedBy}`
- **JSON tags**: `old_regime`, `new_regime`, `confidence`, `determined_by`
- **Live alias**: `internal/live/eventbus.go:33` (const) + `:16` (type alias)
- **Subscribers**:
  - `cmd/atlas/main.go:1972`
  - `internal/orchestrator/system.go:266`
- **Existing tests**: `internal/eventbus/eventbus_test.go` (TestPublishRegimeChange)
- **Contract tests**: `internal/eventbus/contract_test.go` (Tests 1,5,8,11) + `internal/live/contract_test.go` (Tests 1,5)
- **Blast radius**: 🔴 HIGH — 2 subscribers + live alias
- **Refactor rule**: Do NOT change event string value. Do NOT change publisher signature. Do NOT change payload field names or JSON tags. Do NOT break live alias re-export.

---

## Contract 4: ChannelErrors (extension method, NOT interface)

- **Location**: `internal/monitoring/gateway_adapter.go:39`
- **Signature**: `func (a *macroDataGatewayAdapter) ChannelErrors() map[string]string`
- **Interface assertion**: `internal/monitoring/gateway_adapter.go:456` — `var _ marketdata.MacroDataProvider = (*macroDataGatewayAdapter)(nil)`
- **IMPORTANT**: `MacroDataProvider` interface (`internal/marketdata/macro_provider.go:57-60`) declares only `Name()` and `FetchSnapshot()`. `ChannelErrors()` is NOT part of this interface.
- **Data visibility layer**: L2 of 4-layer safeguard (see `internal/monitoring/AGENTS.md`)
- **Existing tests**: 3 tests in `internal/monitoring/gateway_adapter_test.go`
- **Contract tests**: `internal/monitoring/contract_test.go` (Test 1)
- **Blast radius**: 🟠 MEDIUM — adapter-only, but adding to interface would break 4+ providers
- **Refactor rule**: Do NOT add `ChannelErrors` to `MacroDataProvider` interface. Do NOT change method signature. Do NOT remove the compile-time interface assertion.

---

## Contract 5: EventPositionUpdate + PublishPositionUpdate + PositionEventPayload

- **Constant**: `internal/eventbus/eventbus.go:29` — `EventPositionUpdate EventType = "portfolio.position.update"`
- **Publisher**: `internal/eventbus/eventbus.go:620` — `func (b *ChannelEventBus) PublishPositionUpdate(symbol string, position domain.Position, changeType string)`
- **Payload**: `internal/eventbus/eventbus.go:107-112` — `PositionEventPayload{Symbol, Position, ChangeType}`
- **JSON tags**: `symbol`, `position`, `change_type`
- **Live alias**: `internal/live/eventbus.go:34` (const) + `:17` (type alias)
- **Subscribers**: `internal/live/orchestrator.go:341`
- **Existing tests**: `internal/eventbus/eventbus_test.go` (TestPublishPositionUpdate)
- **Contract tests**: `internal/eventbus/contract_test.go` (Tests 2,6,9,12) + `internal/live/contract_test.go` (Tests 2,6)
- **Blast radius**: 🟠 MEDIUM — 1 subscriber + live alias
- **Refactor rule**: Do NOT change event string value. Do NOT change publisher signature. Do NOT break live alias.

---

## Contract 6: EventPortfolioPnL + EventMarketSnapshot + PublishMarketSnapshot

### EventPortfolioPnL
- **Constant**: `internal/eventbus/eventbus.go:30` — `EventPortfolioPnL EventType = "portfolio.pnl.update"`
- **Publisher**: **NONE** (no `PublishPortfolioPnL` helper — consumers use `bus.Publish(BusEvent{...})` directly)
- **Live alias**: `internal/live/eventbus.go:35`
- **Blast radius**: 🟢 LOW — constant only, no publisher to break

### EventMarketSnapshot
- **Constant**: `internal/eventbus/eventbus.go:20` — `EventMarketSnapshot EventType = "market.snapshot"`
- **Publisher**: `internal/eventbus/eventbus.go:559` — `func (b *ChannelEventBus) PublishMarketSnapshot(quote domain.Quote)`
- **Payload**: `internal/eventbus/eventbus.go:93-97` — `MarketEventPayload{Symbol, Quote, Timestamp}`
- **Live alias**: `internal/live/eventbus.go:29` (const) + `:15` (type alias)
- **Subscribers**: `internal/live/orchestrator.go:320,336,596`
- **Existing tests**: `internal/eventbus/eventbus_test.go` (TestPublishMarketSnapshot)
- **Contract tests**: `internal/eventbus/contract_test.go` (Tests 3,4,7,10) + `internal/live/contract_test.go` (Tests 3,4)
- **Blast radius**: 🟠 MEDIUM — 3 subscribers + live alias
- **Refactor rule**: Do NOT change event string value. Do NOT change publisher signature.

---

## Live Alias Re-exports (Cross-Cutting Contract)

- **File**: `internal/live/eventbus.go:10-52`
- **Type aliases** (lines 10-26): `EventType`, `BusEvent`, `EventHandler`, `Subscription`, `MarketEventPayload`, `RegimeEventPayload`, `PositionEventPayload`, `EventBus`, `ChannelEventBus`, + 5 others
- **Const aliases** (lines 28-50): `EventMarketSnapshot`, `EventRegimeChange`, `EventPositionUpdate`, `EventPortfolioPnL`, + 16 others
- **Constructor** (line 52): `func NewChannelEventBus(bufferSize int) *ChannelEventBus`
- **Refactor rule**: All aliases MUST continue to compile. If any eventbus type/const is renamed, the alias must be updated in the SAME PR.

---

## Refactor Safety Rules

1. Do NOT change any method signature listed in Contracts 1-6
2. Do NOT change any event constant string value
3. Do NOT change any payload struct field names or JSON tags
4. Do NOT break `internal/live/eventbus.go` alias re-exports
5. Do NOT change `GetWeights` return type from `map[FactorType]float64`
6. Do NOT add `ChannelErrors` to `MacroDataProvider` interface
7. Any contract change requires a dedicated PR with `gitnexus_impact` blast radius analysis
8. Contract tests in `internal/eventbus/contract_test.go`, `internal/live/contract_test.go`, `internal/portfolio/contract_test.go`, and `internal/monitoring/contract_test.go` MUST pass before and after every #611 PR
