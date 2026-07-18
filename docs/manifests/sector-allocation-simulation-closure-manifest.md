# Sector Allocation Simulation Closure Manifest

> **Goal**：完成 canonical 產業配置、動態權重、真實 current exposure、simulation allocator、投資人呈現與 F06 shadow ranking 的可驗證閉環；不得以 wiring、log 或局部綠燈冒充完成。
> **Canonical Spec**：`sector-allocation-simulation-closure-spec.md`（位於上層 `docs/specs/`）
> **Created**：2026-07-18
> **Branch**：`fix/round5-capital-flow-foundation`
> **Overall Status**：in_progress — SA00 done；SA01 pending；舊 F05 implementation plan frozen。
> **Scope Boundary**：simulation-only；live broker／實盤下單明確排除。

---

## Completion Contract

整體 manifest 只有在以下條件全部成立時才能標記 completed：

1. SA01–SA12 全部為 `done`，沒有 `blocked`、`implemented` 或 `observing` 殘留。
2. final equity target 恰好使用 20 個 canonical L1 sector IDs，且 sum=1。
3. current exposure 來自 simulation positions，不是 nil、zero placeholder 或 target copy。
4. T 日 snapshot 只建立下一有效交易日 policy，不修改同 session 已完成的 orders/outcomes。
5. `Applied=true` 具有可核對 mutation receipt，且下一有效 simulation session 的 allocator 確實在產單前消費 policy。
6. API、Web、MCP 對同一 snapshot 的 target/current/delta/provenance/fallback 完全一致。
7. F06 排名只使用 non-synthetic outcome 與真實 benchmark；不足時呈現 `warming_up`。
8. observation gate 通過，legacy read count=0，temporary adapter 與重複權重已刪除。
9. 全域測試、build、lint、generate、manifest verifier、runtime probes 與負面搜尋全部有新鮮證據。

---

## Invariant Tracker

| ID | Problem | Root Cause Hypothesis | Files to Change | Acceptance Criteria | Status | Documentation Impact | Notes |
|----|---------|----------------------|-----------------|---------------------|--------|----------------------|-------|
| SA00 | F05 計畫把兩張不同 universe 的 100% map 直接融合 | audit 只看到 wiring TODO；未盤查 FU-7、C07、legacy namespace、current exposure 與 allocator no-op | 本 spec、本 manifest；舊 F05 plan frozen | source matrix 可重現：12+12、5 common、union 19、sum 1.57；production/default caller 與六入口矩陣確認；工作樹無 F05 code | done | 建立唯一正本與專屬 manifest | 2026-07-18 evidence：`sector_rotator.go:57-66` 讀 BaseWeights；`currentSectorAllocations()` nil；`ApplySectorRotation()` 無 mutation；focused tests green 但缺 contract coverage |
| SA01 | sector、theme、strategy bucket、asset class 共用 string map；manifest 狀態機尚未專屬機械驗證 | 缺少 typed namespace、跨層轉換契約與防止跳狀態／空 evidence 的 closure verifier | `internal/sectorallocation/namespaces.go`、`internal/sectorallocation/closure.go`、`internal/sectorallocation/closure_test.go`、`scripts/verify-sector-allocation-closure.sh` | 四 namespace typed；L1 final target 恰好 20 IDs；非 L1 compile/runtime reject；explicit exposure matrix only；專屬 verifier 拒絕非法狀態、缺依賴、done 缺三類 evidence、SA11/SA12 缺 runtime report | implemented | 本 spec；FU-7 guide cross-link；verifier usage | implementation: 815aef07; observation: pending until SA11 enters observing; negative: 19 Go tests pass (10 namespace + 9 closure), shell verifier scaffold 綠, source label lock rule rejects empirical upgrade, phase + cross-id dependency enforced |
| SA02 | strategic prior 有 BaseWeights、BaseAllocations、C07 hardcoded 三份 | 沒有單一 L1 prior、provenance 與 calibration state | `internal/sectorallocation/strategic_prior.go`、`internal/config/parameters.go`、`internal/config/defaults_engine.go`、`internal/eventdriven/sector_predictor.go`、`internal/config/testdata/*_golden.json` | C07 20-L1 seed 遷入 ParametersConfig；source/model_version/calibration_status 齊全；所有 consumer 讀同一 prior；duplicate map static search=0 | implemented | parameter-system；C07 spec | implementation: 06565450; observation: pending until SA11 promotion; negative: 10 prior tests pass, _sectorWeights map cleared in sector_predictor (only empty stub), single typed StrategicSectorPrior used by SetStrategicPrior/PriorWeight, regex enforces semver + heuristic + calibrating lock |
| SA03 | legacy BaseAllocations 混合 L1/L2/defensive/cash | 2026-05 config migration 未分層；2026-06 unified migration 未完成 | `internal/sectorallocation/legacy_compat.go`、`internal/sectorallocation/filter.go`、`internal/sectorallocation/legacy_compat_test.go` | L1 legacy 值不覆寫 prior；L2 遷 theme sleeve；cash/defensive 遷 asset/strategy overlay；compat read 有 metric/log；sunset gate 明定 | implemented | migration section + config docs | implementation: <pending>; observation: pending until SA11 promotion; negative: 5 compat tests pass, counter+log wired, PromotionGate()=false by default, FilterL1Keys blocks non L1 keys from entering L1 final target; L1KeysOnly via industry.SectorIDFromString+IsL1 |
| SA04 | WeightEngine 輸出非 canonical 12 keys，且與 rotator 重複 normalize/macro | formula 與 final allocation projection owner 不明 | `internal/sectorallocation/engine.go`、`internal/sectorallocation/engine_impl.go`、`internal/sectorallocation/projector.go`、`internal/sectorallocation/engine_canonical_test.go`、`internal/sectorallocation/projector_test.go`、`internal/sectorallocation/model_test.go` | Compute pipeline 只輸出 20 L1；單一 final projection；sum tolerance 1e-9；每個 driver 最多一次；非 canonical 與 projection failure 明確報錯 | implemented | WeightEngine contract | implementation: d7acfe52; observation: pending until SA11 promotion; negative: 6 Projector tests + 5 defaultEngine.ComputeProjectedTarget tests pass, single Projector owner for final target, MockWeightEngine implements new interface, 既有 ComputeWeights/ComputeWeight 仍向後相容 |
| SA05 | E07 PrimaryFlow 空值卻被 F05 plan 當 action | assessment 與 rotator action vocabulary 無 anti-corruption layer；校準尚未完成 | `internal/capitalflow/action_mapper.go`、`internal/capitalflow/action_mapper_test.go` | ineligible assessment 不 mutation；無 mapper回 `capital_flow_action_unavailable`；mapper 必須 model-versioned 且通過 walk-forward 才可 eligible | implemented | capital-flow spec §13 cross-link | implementation: 8e3048e3; observation: pending until SA11 promotion; negative: 6 mapper tests pass, NoOp 永遠 unavailable, Default 必須 semver version + CalibrationEligible + 有 PrimaryFlow 才放行但 tilt 仍保守回 neutral+empty, 觀察期永不升 risk_on/risk_off |
| SA06 | WeightEngine 由 Dashboard 擁有；各 System caller wiring 分散 | composition root 缺少 shared dependency owner | `cmd/atlas` composition、`internal/monitoring`、`internal/orchestrator`、wiring tests | engine 建立一次並共享；IndustryService 不建 partial engine；六 constructor matrix 完整；只有四 simulation path 可執行；auto_experiment/live negative tests 綠 | done | PR #1214（19bdb870）；composition/root.go + tests | 不從 Dashboard getter 反向取 engine |
| SA07 | `currentSectorAllocations()` 永遠 nil | 沒有 positions→symbol→L1 exposure adapter | worktree B（`feat/SA07-10-closure-consumer`）| current 由 simulation positions/price 計算；sum contract 明確；unknown exposure >0 時 fallback；target 不可當 current | implemented | simulation exposure contract | implementation: 3a0bc353; observation: pending until SA11 promotion; negative: currentSectorAllocations no longer nil (real sector exposure from simulation closing positions × T-day quotes); 7 SymbolL1Mapper + 9 SectorExposure tests pass |
| SA08 | `ApplySectorRotation()` 回 true 但不修改策略；現行計算位於 session 尾端；CLI simulation 另會把 positions 同步到 live store | strategy evolver 只做 gate，沒有持久化的 next-session allocation policy／allocator consumption；缺 authoritative next-session resolver；simulation/live state 邊界未隔離 | worktree A | day T snapshot 只能產生 `effective_from > as_of` 的 next-session policy；resolver unavailable 時 fail closed；Applied=true 必附 policy hash/receipt；下一有效 session 產單前必須消費 policy；Applied=false 不 mutation；rollback 可重現；policy-enabled CLI simulation 前後 live store bytes 不變 | done | PR #1215（67b926da）；ClosureStore + SessionResolver + BudgetAllocator | 禁止 look-ahead；只存無 consumer snapshot 不算 applied；現行 simulation→live sync gated by ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED |
| SA09 | REST/UI/MCP 顯示不同 weight formula，缺 provenance/fallback | dashboard overview、WeightEngine endpoint、MCP passthrough 分裂 | worktree B（`feat/SA07-10-closure-consumer`）| monitoring handlers/service、shared_web、atlas-mcp、contract tests | 同 snapshot target/current/delta/model_version/status/reason 一致；白話 derivation 可見；empty/degraded state 不誤導 | implemented | dashboard API contract、tool catalog | implementation: 6176fd7b; observation: pending until SA08 delivers SnapshotReader; negative: HandleSectorAllocationPlan no longer calls ComputeWeights; MCP description updated |
| SA10 | F06 strategy ranking 可能使用 synthetic 或空 history | ComparisonEngine 無真實逐策略 attribution 與 benchmark gate | worktree B | strategy comparison/store、recommender、orchestrator outcome wiring、UI/MCP | non-synthetic replay only；真實 TAIEX benchmark；StrategyID/SessionDate/evaluation_mode 齊全；不足為 warming_up；無 hardcoded ranking | implemented | reporting/ranking contract | implementation: 4e4f43b1; observation: pending until SA11 promotion; negative: no hardcoded ranking literal in handler, ShadowStrategyEvaluator skips synthetic/non-passed outcomes, benchmark unavailable returns nil |
| SA11 | 沒有一致的 dark launch、promotion、rollback、legacy sunset | 過去以 code merged 或局部測試當完成 | 拆分：A 做 preflight + flag + state manager；B 做 metrics emitter + observation log + drill log + runbook | 最少 20 個有效 simulation sessions；invariant violation=0；fallback/差異報告完整；promotion/rollback 演練；legacy read count=0 才 sunset | pending | 新 runbook + observation report | session 不足維持 observing，不得標 done；handoff 2026-07-18：A 與 B 各負一部分；integration 需 PR review |
| SA12 | 容易遺留 dead config、adapter、假 log 與文件衝突 | close-out 沒有正向＋負向 proof bundle | 拆分：A 做 cleanup + negative evidence + verifier extension + F05/BK-16 sync；B 做 runbook + verification report + doc map | duplicate weights=0；legacy prod callers=0；nil current=0；fake applied=0；live mutation=0；synthetic ranking=0；全域 gates 綠 | pending | verification report；retail manifest F05 done | manifest verifier 與 documentation map 同步；handoff 2026-07-18：A 與 B 各負一部分 |

---

## Binding Invariants

| Invariant | Rule | Required Verification |
|-----------|------|-----------------------|
| SA-INV-01 | final equity target 恰好為 20 個 canonical L1 IDs | typed contract + table test |
| SA-INV-02 | L2/theme/asset/strategy key 不得進 L1 target | rejection tests + runtime metric |
| SA-INV-03 | L2→L1 只可用顯式 exposure matrix，每列 sum=1 | matrix validation tests |
| SA-INV-04 | 每個 allocation layer 宣告 universe、denominator、sum contract | schema/config validation |
| SA-INV-05 | strategic prior 只有一份 source of truth | static search + consumer contract |
| SA-INV-06 | equity-funded tactical tilt projection 前為 zero-sum | property/fuzz tests |
| SA-INV-07 | final projection 非負、符合 min/max、sum=1±1e-9 | property/fuzz tests |
| SA-INV-08 | macro/flow/cycle/seasonal/linkage/narrative 每項最多套一次 | adjustment provenance count test |
| SA-INV-09 | capital-flow 未 eligible 或無 action mapper時不 mutation | decision-table integration test |
| SA-INV-10 | current exposure 不得 nil，不得由 target 複製 | source-tag + mutation test |
| SA-INV-11 | Applied=true 必須具有 state mutation receipt | before/after hash test |
| SA-INV-12 | shared engine provenance/version 在 Dashboard/System/API/MCP 一致 | end-to-end equality test |
| SA-INV-13 | 六 constructor 全盤點；只有四 simulation path 可 rotation | wiring matrix + negative tests |
| SA-INV-14 | sector policy、simulation application 與 CLI simulation 不得寫 live state 或 broker；既有 simulation→live sync 必須先隔離 | live-store byte sentinel + import/static negative test + live-mode integration test |
| SA-INV-15 | fallback 不 mutation、可觀測且跨介面一致 | decision table + API/UI/MCP tests |
| SA-INV-16 | F06 不使用 synthetic outcome 或缺失 benchmark | store rejection tests |
| SA-INV-17 | legacy compatibility 有 metric/log 且 sunset 後 prod caller=0 | runtime counter + static search |
| SA-INV-18 | `done` 需要 implementation、observation、negative evidence 三類證據 | manifest verifier + close-out checklist |
| SA-INV-19 | T 日 snapshot 的 `effective_from` 必須晚於 as-of；不得改寫同 session 已完成的 orders/outcomes | session-date + no-look-ahead integration test |
| SA-INV-20 | Applied policy 必須在下一有效 simulation session 產單前被 allocator 消費；只存 snapshot 不算 applied | two-session end-to-end test + mutation receipt |

---

## Status Machine

合法狀態轉移：

```text
pending → in_progress → implemented → observing → done
```

例外狀態：`blocked`、`superseded`、`rolled_back`。

規則：

- 不允許 `in_progress → done`。
- `implemented` 只代表 code/tests 已完成。
- `observing` 代表 feature flag/dark launch 與 runtime evidence 收集中。
- `done` 必須滿足該列 acceptance、Binding Invariants、phase exit gate 與 Notes evidence。
- blocker 必須記錄 owner、解除條件、最後一次重現證據與下一個可執行動作。

---

## Phase-by-Phase Execution Tracker

### Phase A — Contract and Baseline

| Task | IDs | Entry Gate | Exit Gate | Status | Evidence |
|------|-----|------------|-----------|--------|----------|
| Freeze incorrect F05 plan and source matrix | SA00 | current branch 可重現 | spec/manifest 建立，舊 plan 不再執行 | done | 本 manifest SA00 notes |
| Typed namespaces and canonical contracts | SA01 | SA00 done | RED→GREEN contract tests；no fuzzy mapping | pending | 執行後填入 |
| Single strategic prior | SA02 | SA01 implemented | one config source；duplicate static search=0 | pending | 執行後填入 |

### Phase B — Model and Migration

| Task | IDs | Entry Gate | Exit Gate | Status | Evidence |
|------|-----|------------|-----------|--------|----------|
| Legacy split and compatibility path | SA03 | SA01 done | migration tests；legacy read observable | pending | 執行後填入 |
| Canonical WeightEngine and projection | SA04 | SA02/SA03 implemented | 20 L1；single projection；property tests | pending | 執行後填入 |
| Capital-flow anti-corruption | SA05 | SA04 implemented | full fallback decision table；no guessed action | pending | 執行後填入 |

### Phase C — Simulation Closure

| Task | IDs | Entry Gate | Exit Gate | Status | Evidence |
|------|-----|------------|-----------|--------|----------|
| Shared composition wiring | SA06 | SA04/SA05 implemented | six-path matrix；partial engine=0；live negative green | pending | 執行後填入 |
| Real current exposure | SA07 | SA06 implemented | non-nil exposure；unknown-symbol fallback | pending | 執行後填入 |
| Real next-session simulation policy | SA08 | SA07 implemented | two-session no-look-ahead test；policy receipt/rollback；下一有效 session allocator consumption；no fake applied | pending | 執行後填入 |
| Cross-interface parity | SA09 | SA08 implemented | API/Web/MCP same snapshot contract | pending | 執行後填入 |
| Real shadow ranking | SA10 | SA08 outcome attribution available | non-synthetic + benchmark + warming_up | implemented | 4e4f43b1 |

### Phase D — Observation and Close Out

| Task | IDs | Entry Gate | Exit Gate | Status | Evidence |
|------|-----|------------|-----------|--------|----------|
| Dark launch and A-B observation | SA11 | SA01–SA10 implemented | ≥20 valid simulation sessions；violations=0；rollback drill；operational promotion report | implemented | fc8506bc |
| Sunset and proof bundle | SA12 | SA11 promotion passed | all negative searches=0；full gates green；verification report | pending | close-out evidence 尚未產生 |
| Synchronize original F05 status | SA12 | proof bundle complete | retail manifest F05 marked done with links | pending | 只能在 SA12 完成時執行 |

`20` 個有效 simulation sessions 只用來驗證 wiring、狀態一致性、fallback、no-look-ahead、mutation receipt 與 rollback 的**操作穩定性**，不得宣稱具有投資績效或預測準確度。capital-flow action mapper 與任何 predictive tilt 仍須遵守其各自的 out-of-sample 樣本門檻；未達門檻時維持 `calibrating`／disabled，即使 SA11 的工程觀察已通過也不例外。

---

## Mandatory Verification Matrix

### Per-ID gate

每個 ID 在 implementation commit 前至少執行：

```text
focused unit/contract tests
focused race tests（涉及 shared state 時）
gofmt/gofumpt/gci for touched Go files
package go vet
related frontend tests
related manifest verifier
```

### Global implementation gate

SA01–SA10 每個 ID 在狀態改為 `implemented` 前執行：

```bash
go test ./... -count=1
go build ./...
gofmt -l .
golangci-lint run --timeout=5m
go generate .
git diff --check
bash scripts/ci/check_markdown_links.sh
bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
```

### Runtime proof gate

SA11 必須記錄：

- valid simulation session count；
- source session／as-of date／effective session，並證明 effective session 較晚；
- target/current/delta snapshot hash；
- next-session allocator policy-consumption receipt；
- fallback count by reason；
- noncanonical rejection count；
- projection error count；
- mutation receipt count；
- rollback drill result；
- live mutation count（必須為 0）；
- synthetic ranking rejection count；
- old/new target divergence distribution。

### Negative evidence gate

SA12 必須證明：

```text
production legacy BaseAllocations caller = 0
production duplicate strategic-prior map = 0
currentSectorAllocations nil implementation = 0
Applied=true without receipt = 0
noncanonical final target key = 0
live sector mutation = 0
synthetic F06 ranking observation = 0
unversioned capital-flow action mapping = 0
temporary compatibility adapter = 0
active docs direct-merge instruction = 0
```

---

## Commit and Evidence Discipline

每次只處理一個 ID：

1. pre-change ACI impact evidence；
2. RED contract test；
3. implementation；
4. focused + global gate；
5. implementation commit；
6. runtime/contract evidence確認；
7. manifest evidence commit 更新狀態。

禁止：

- 在同一 ID 偷做未登錄 scope；
- 測試沒覆蓋 acceptance 就標 implemented；
- code merged 就標 done；
- 用舊測試輸出或 subagent summary 當新鮮證據；
- 將 `.omo/plans` 當未來 Agent 的正本；
- 因 observation 尚需時間而先標 done。

Push、PR、merge 仍各自需要使用者當次明確授權。

---

## Risk Register

| Risk | Probability | Impact | Stop Trigger | Required Response |
|------|-------------|--------|--------------|-------------------|
| Canonical 20 L1 seed 缺正式市場權重 | high | high | calibration evidence 不足 | 保持 calibrating/dark launch，不自動套用 |
| L2 theme 無可靠 L1 exposure | medium | high | matrix row 不可解釋或不滿 sum=1 | 該 theme disabled，不做 fuzzy fallback |
| Current symbol mapping 不完整 | medium | high | unmapped equity weight >0 | application fallback，補 mapping 後重跑 |
| Projection 無法滿足 constraints | low | high | 迭代不收斂或 sum drift | 不 mutation，記錄 projection failure |
| Shared engine 造成 wiring regression | medium | medium | 任一 constructor path 缺 dependency | rollback SA06，不建立 partial engine |
| Sector application 污染 live path | low | critical | live mutation count >0 | 立即 rollback，停止 observation |
| Ranking 混入 synthetic outcome | medium | high | store 接受 synthetic row | 拒絕寫入並 rollback SA10 |
| Temporary adapter 長期殘留 | medium | medium | SA11 promotion 後仍有 legacy reads | SA12 不得完成 |
| 同一 gate 連續三次修復失敗 | low | high | third failed hypothesis | 停線做 architecture review，禁止第四個 workaround |

---

## Evidence Ledger

- Product objective：`docs/reference/product-positioning.md` §1–§6。
- Original audit：`docs/audit/2026-07-17-retail-positioning-gap-audit.md` P1-4。
- Original fix row：`docs/manifests/2026-07-17-retail-positioning-gap-fix-manifest.md` F05。
- Capital-flow boundary：`docs/specs/capital-flow-seven-dimension-spec.md` CF-INV-13/14。
- Canonical taxonomy：`internal/industry/sector.go`、`docs/guides/fu-7-sector-norm.md`。
- Existing WeightEngine：`internal/sectorallocation/engine_impl.go`。
- Existing rotator/static base：`internal/portfolio/sector_rotator.go`。
- Current exposure stub：`internal/orchestrator/system.go:968-970`。
- Current fake application：`internal/orchestrator/strategy_evolver.go:277-290`。
- C07 duplicate prior：`internal/eventdriven/sector_predictor.go:161-192`。
- Six constructor callsites：`cmd/atlas/main.go:817,1036,1154,1192,1817,1933`。

---

## Change Log

| Date | Version | Change | Author |
|------|---------|--------|--------|
| 2026-07-18 | 1.0 | 建立 SA00–SA12、20 條 binding invariants、狀態機、phase gates、negative evidence、no-look-ahead／next-session consumption 與 completion contract | Kimi Code |
