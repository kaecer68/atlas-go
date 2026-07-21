# Sector Allocation Simulation Closure — Handoff 與併行工作邊界

> **目的**：當前 session（worktree A）完成 SA04-06-08-11A-12A 後停下；新 opencode CLI（worktree B）接手 SA07-09-10-11B-12D。本文件定義兩邊的 contract boundary、互鎖條件與交接清單。

---

## 0. 現況摘要（截至 2026-07-18）

| ID | 狀態 | 位置 |
|----|------|------|
| SA00 | done | manifest SA00 evidence + spec 與 plan 落地 |
| SA01 | done | `internal/sectorallocation/{namespaces,closure}.go` + `scripts/verify-sector-allocation-closure.sh`（19 tests 綠）|
| SA02 | done | `internal/sectorallocation/strategic_prior.go`（10 tests 綠）|
| SA03 | done | `internal/sectorallocation/{legacy_compat,filter}.go`（5 tests 綠）|
| SA04 | partial | `Projector` 已完成（6 tests 綠）；WeightEngine interface 整合與 `defaultEngine` 重寫未做 |
| SA05–SA12 | pending | 由 A 與 B 依下方 contract 拆分 |

Worktree A 已在 `fix/round5-capital-flow-foundation`（HEAD = `d12b5526`）落地治理三件套：

- `docs/specs/sector-allocation-simulation-closure-spec.md`
- `docs/manifests/sector-allocation-simulation-closure-manifest.md`
- `docs/superpowers/plans/2026-07-18-sector-allocation-simulation-closure.md`
- `scripts/verify-sector-allocation-closure.sh`

`bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md` 與 `bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md` 為永久 gate。

---

## 1. 併行拆分（worktree A 與 B）

### 1.1 邊界 contract：worktree A 必須先生產下列 typed API，worktree B 才能安全啟動

| Interface | 路徑 | 由誰產出 | 由誰消費 |
|----------|------|---------|---------|
| `sectorallocation.ClosureStore` | `internal/sectorallocation/policy.go`（SA08） | A | B（SA07 的 snapshot 讀取 + SA11 metrics）|
| `sectorallocation.SnapshotReader` | 同上 | A | B（SA09 REST handler 改讀）|
| `sectorallocation.SectorAllocationSnapshot` | `internal/sectorallocation/closure.go` 擴充 | A | B（SA09 parity test + SA11 log）|
| `composition.CompositionPath` 與 `BuildSystem(path)` | `internal/orchestrator/composition/`（SA06）| A | B（SA07/SA09/SA10 必須知道 path 走 simulation gate）|
| `sectorallocation.LegacyCompatReader` | `internal/sectorallocation/legacy_compat.go`（SA03）| A | B（SA09 顯示 source-of-truth）|
| `sectorallocation.MacroAction` / `CapitalFlowAction` | `internal/sectorallocation/projector.go`（SA04）+ `internal/capitalflow/types.go`（SA05）| A | B（SA09 透出 + SA10 attribution）|
| `sectorallocation.SymbolL1Mapper` | `internal/industry/symbol_l1_mapper.go`（SA07）| **B** | A（SA08 內仍消費；A 在 SA08 內呼叫 B 寫好的 mapper）|
| `portfolio.SectorExposureCalculator` | `internal/portfolio/sector_exposure.go`（SA07）| **B** | A（SA08 內呼叫；SA09 也呼叫）|
| `strategy.FileComparisonStore` + `ShadowStrategyEvaluator` | `internal/strategy/`（SA10）| **B** | A（SA11 觀察用）+ B（SA09 recommendation）|
| `marketdata.FileTAIEXBenchmarkProvider` | `internal/marketdata/taiex_benchmark.go`（SA10）| **B** | A + B（SA11 metrics + SA09）|

### 1.2 Worktree A（當前 session）責任

- **A 必須先做**（blocker for B）：
  - SA04：把 `Projector` 接到 `WeightEngine.ComputeProjectedTarget`；重寫 `defaultEngine` 使 `ComputeWeights` 透過 `Projector` 投影；既有 `engine_impl_test.go` 與 `integration_test.go` 同步修；既有 `sector_allocation.base_weights` 12 keys 場景刪除。
  - SA05：新增 `internal/capitalflow/action_mapper.go`（typed enum + NoOp + Default 預設禁用）。
  - SA06：`internal/orchestrator/composition/` + `cmd/atlas/main.go` 六 callsite 改走 `root.BuildSystem(path)`；`internal/monitoring/service/industry.go` 移除構造時 partial engine。
  - SA08：新增 `internal/sectorallocation/policy.go`（`ClosureStore`/`FileClosureStore`/`SnapshotReader`/`MutationReceipt`/`ConsumptionReceipt`/`SectorAllocationPolicy`）；新增 `internal/portfolio/sector_budget_allocator.go`；新增 `internal/orchestrator/session_resolver.go`（`TradingSessionResolver` + `ReplayNextSessionResolver` + `NoOp`）；改 `internal/sim/engine.go` 加 `RunWithStateForSession`；改 `internal/orchestrator/strategy_evolver.go` 移除假 true；**移除 `cmd/atlas/main.go:1889-1917` CLI simulation 內 `livestore.NewStateStore` 同步**（這是 B 不可動的關鍵邊界）。
  - SA11.A：feature flag、preflight、state manager（這些必須在 SA06 之後做才能 inject）。
  - SA12.A+B：cleanup、negative evidence script、verifier extension、原 F05 + BK-16 狀態同步。

- **A 不可動的檔案**：
  - `internal/industry/symbol_l1_mapper.go`（B 寫）
  - `internal/portfolio/sector_exposure.go`（B 寫）
  - `internal/strategy/comparison_store.go`、`internal/strategy/shadow_evaluator.go`（B 寫）
  - `internal/marketdata/taiex_benchmark.go`（B 寫）
  - 任何 frontend（`shared_web`、`admin_web`、`client_web`）的 UI 變更（B 寫 SA09 cross-interface）

### 1.3 Worktree B（新 opencode CLI）責任

- **B 必須等 A 至少完成 SA06**（B 才知道 `CompositionPath` 與 `BuildSystem`）。
- **B 自己的範圍**：
  - SA07：新增 `internal/industry/symbol_l1_mapper.go` 與 `internal/portfolio/sector_exposure.go`；改 `internal/orchestrator/system.go:968` 取代 `currentSectorAllocations()=nil`。
  - SA09：改 `internal/monitoring/api/industry/handlers.go` 改讀 persisted snapshot；改 `internal/monitoring/service/industry.go` 移除 `baseWeight*1.2/1.1/0.7`；改 frontend 顯示 view-model；MCP 改 description。
  - SA10：新增 `internal/strategy/{comparison_store,shadow_evaluator,f06_engine}.go` + `internal/marketdata/taiex_benchmark.go`；改 `internal/recommender/handler.go` 移除 hardcoded `["growth","momentum","all_weather","value","defensive"]`。
  - SA11.B：`internal/orchestrator/sector_allocation_closure_metrics.go`（11 個 `sac.*` events）+ `cmd/experimental/sector-allocation-closure-preflight/main.go` + `docs/operations/sector-allocation-closure-{runbook,observation-log,rollback-drills}.md`。
  - SA12.D：`docs/operations/sector-allocation-closure-verification-report.md`（含 A 完成後的 runtime 證據）+ `docs/operations/sector-allocation-closure-runbook.md` 補章節 + `docs/documentation-map.md` + `docs/reference/guidelines-index.md` 同步。

- **B 不可動的檔案**：
  - `cmd/atlas/main.go`（SA06 是 A 的責任；B 不可改 callsite）
  - `internal/orchestrator/composition/`（A 的責任）
  - `internal/orchestrator/strategy_evolver.go`（SA08 是 A 的責任）
  - `internal/sim/engine.go`（SA08 是 A 的責任）
  - `internal/capitalflow/action_mapper.go`（SA05 是 A 的責任）
  - 任何 `internal/live/`、`livestore`、`broker` 相關檔案
  - `docs/specs/`、`docs/manifests/`、`docs/superpowers/plans/` 治理文件

### 1.4 互鎖與整合順序

```
T0: A 完成 SA04 + SA05 + SA06 → 產出 contract（commit hash 標記）
T1: B 從 A 的 commit hash 拉 base，可開始 SA07 / SA09 / SA10
T2: A 完成 SA08（含 CLI live-sync isolation）→ 產出 `ClosureStore` + `SectorBudgetAllocator` + `TradingSessionResolver`
T3: B 整合 SA08 的 contract 到 SA10 的 outcome attribution 與 SA11 metrics
T4: A 完成 SA11.A（flag + state + preflight）
T5: B 完成 SA11.B（metrics + observation log + drill log + runbook）
T6: A 與 B 各自做 SA12 負責的範圍
T7: 兩個 branch 透過 PR 整合；PR body 必須引用 manifest SA00 row + 此 handoff 文件
```

---

## 2. Worktree B 建立步驟（給新 opencode CLI 操作者）

```bash
# 1. 建立新 worktree（必須在 A 完成 SA06 之後執行）
cd /Users/kaecer/workspace/atlas
git fetch
git worktree add ../atlas-SA07-10 -b feat/SA07-10-closure-consumer \
  fix/round5-capital-flow-foundation
cd ../atlas-SA07-10

# 2. 啟動新的 opencode CLI 工作區
opencode .

# 3. 把接手提示詞貼給新的 opencode session
```

---

## 3. 接手提示詞（給 worktree B 的新 opencode CLI）

把以下整段複製貼上即可。

---

### 接手提示詞開始

```
你是 atlas-go repo 中 worktree B 的接手 AI。
工作目錄: /Users/kaecer/workspace/atlas-SA07-10
branch: feat/SA07-10-closure-consumer
你的責任是完成 sector allocation closure manifest 的 SA07、SA09、SA10、SA11.B 與 SA12.D；
SA04、SA05、SA06、SA08、SA11.A、SA12.A/B 與 SA12.C 由另一個 worktree A 負責。

工作環境必讀:
1. docs/handoff/2026-07-18-sector-allocation-closure-progress.md
2. docs/specs/sector-allocation-simulation-closure-spec.md（唯一正本）
3. docs/manifests/sector-allocation-simulation-closure-manifest.md（你只更新 SA07/SA09/SA10/SA11/SA12 的 Notes）
4. docs/superpowers/plans/2026-07-18-sector-allocation-simulation-closure.md（你的 Task 7/9/10/11.B/12.D）
5. /Users/kaecer/.agents/skills/using-superpowers/SKILL.md
6. /Users/kaecer/.agents/skills/test-driven-development/SKILL.md

鐵則:
- 任何 commit 前必跑: go test ./... -count=1 && go build ./... && gofmt -l . && \
  bash scripts/verify-manifest.sh docs/manifests/sector-allocation-simulation-closure-manifest.md && \
  bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
- 寫生產碼前必寫失敗測試；不許直接寫 production code 而無 failing test
- 每個 ID 兩個 commit: implementation 與 evidence
- 不可動: cmd/atlas/main.go、internal/orchestrator/composition/、internal/orchestrator/strategy_evolver.go、
  internal/sim/engine.go、internal/capitalflow/action_mapper.go、任何 internal/live/ 與 broker 相關檔案、
  docs/specs/、docs/manifests/（除 SA07/SA09/SA10/SA11/SA12 Notes 與本來 commit ID 之外）、docs/superpowers/plans/
- 完成 SA11 observation 需要真實 simulation session；若 staging 環境不可用，標 observing 並在 Notes 記錄 blocker owner
- 觀察期嚴禁宣稱金融準確；SA-INV-13/14/15/16 鎖住
- 任何模糊從 manifest SA-INV-01–20 與 spec §3-10 找答案；不靠 subagent summary 當證據

你的 Task 7 細節:
- 新增 internal/industry/symbol_l1_mapper.go: SymbolL1Mapper 結構，含 bySymbol map、ResolveL1(symbol) (industry.SectorID, bool)、
  normalize 規則 2330 與 2330.TW 同視、拒絕 fuzzy map、拒絕 L2 直返
- 新增 internal/portfolio/sector_exposure.go: SectorExposureCalculator 結構 + Calculate(positions, quotes, asOf) SectorExposure
  必須恰好 20 L1、sum=1±1e-9（完整時）或 sum=0（空 portfolio）、unknown symbol 進 unmapped list 且 Complete=false
- 改 internal/orchestrator/system.go:968: currentSectorAllocations() 改為呼叫 exposure 計算；刪除 nil stub
- 改 internal/orchestrator/system_risk_session.go: 用真實 current；不再被 return history 或 capital controller 包住
- 改 internal/orchestrator/composition/root.go: 注入 mapper 與 calculator（從 BuildSystem path 進入）

你的 Task 9 細節:
- 改 internal/monitoring/api/industry/handlers.go: HandleSectorAllocationPlan 改為呼叫 h.Svc.GetLatestSectorAllocation(ctx)，
  不再 ComputeWeights；response 對應 SectorAllocationSnapshot 的 typed payload
- 改 internal/monitoring/service/industry.go: IndustryService.GetLatestSectorAllocation(ctx) 透過 SnapshotReader；NewIndustryService
  不建 partial engine（SA06 已處理；但 B 必須驗證）
- 改 internal/monitoring/api/industry/handlers.go: snapshot 不存在回 503 + fallback_reason=snapshot_unavailable
- 改 shared_web/static/js/pages/industry.js: buildSectorAllocationViewModel / buildSectorAllocationHTML；顯示 target/current/delta/
  as_of/effective_from/model_version/calibration_status/source/applied/fallback/unmapped/derivation/receipt
- 改 cmd/atlas-mcp/server/tools_industry_ext.go: sector_allocation_plan tool 仍 passthrough；description 明確
  "Latest persisted simulation sector-allocation snapshot, including target/current/delta, provenance, fallback status, mutation receipt, and next-session consumption evidence."

你的 Task 10 細節:
- 新增 internal/strategy/comparison_store.go: FileComparisonStore + ComparisonStore interface
- 新增 internal/strategy/shadow_evaluator.go: ShadowStrategyEvaluator + Evaluate(outcomes, tradingDate, benchmark) []StrategyDailyObservation
  規則: zero tradingDate 或 !benchmark.Available → nil；IsSynthetic || !PassedGuards → skip；dedupe AgentID+\x00+Symbol
  (higher Conviction wins); per enabled strategy: conviction-weighted mean ForwardReturn; 每筆 EvaluationMode="shadow"
- 新增 internal/strategy/f06_engine.go: RankingSnapshot + ComputeWarmingUpState
- 改 internal/strategy/comparison.go: NewComparisonEngine(window, store) (*ComparisonEngine, error)；Ranking() 依 ComputeWarmingUpState
  決定是否回 warming_up
- 新增 internal/marketdata/taiex_benchmark.go: FileTAIEXBenchmarkProvider + TAIEXBenchmarkProvider interface
- 改 internal/orchestrator/composition/root.go: BuildSystem 內建 FileComparisonStore(ledgerDir/strategy_comparison.json, 365) 與
  ShadowStrategyEvaluator 與 FileTAIEXBenchmarkProvider 注入 StrategyLayer
- 改 internal/orchestrator/system.go: recordShadowStrategyDay 內用 domain.SessionDateFromID(session.ID) 當 tradingDate；
  tradingDate zero 或 benchmark.Available=false 不寫 store、回 warming_up + benchmark_unavailable
- 改 internal/recommender/handler.go: 移除 []string{"growth","momentum","all_weather","value","defensive"} hardcoded；
  buildShadowStrategy(ShadowRankingProvider, *StrategyRecommendation, *[]string) 取代 buildPremiumStrategy
- 改 cmd/atlas-mcp/server/tools_recommendation.go: passthrough + evaluation_mode 透出

你的 Task 11.B 細節（dark launch infrastructure）:
- 新增 internal/orchestrator/sector_allocation_closure_metrics.go: SACMetrics interface + 11 個 Emit 方法
  事件: sac.snapshot.start / target / current / fallback / projection / snapshot.end /
        sac.policy.applied / consumed / sac.legacy.read / sac.fallback.count / sac.rollback.drill
  每個 event 必含 feature=sector_allocation_closure、version=sa.0.1、session_id
- 新增 cmd/experimental/sector-allocation-closure-preflight/main.go: 5 auto + 3 manual checks + validateLocalhostURL
  5 automatable: flag_default_off / flag_env_not_set / canonical_20_l1_seeded / duplicate_prior_search=0 / legacy_base_allocations_read_counter==0
  exit codes: 0/1/2
- 新增 docs/operations/sector-allocation-closure-{runbook,observation-log,rollback-drills}.md
- 改 scripts/ci/check_no_duplicate_preflight.sh: ALLOW_LIST 加入 cmd/experimental/sector-allocation-closure-preflight
- 改 docs/specs/experimental-feature-launch-gate-spec.md: §Reference Implementations 加 SA11 row
- 改 docs/documentation-map.md: 加 SA11/SA12 entry

你的 Task 12.D 細節（runbook + verification report）:
- 新增 docs/operations/sector-allocation-closure-verification-report.md: 引用 9 條 negative evidence check 結果、
  20 條 binding invariants 滿足證據、closure verifier 17 條 pass 證據、零售 manifest F05 + BK-16 done 同步紀錄
- 更新 docs/operations/sector-allocation-closure-runbook.md: 依觀察期實際狀況補章節
- 更新 docs/documentation-map.md: 加 SA11/SA12 entry
- 更新 docs/reference/guidelines-index.md: 加 sector allocation closure 索引

執行順序:
1. 啟動後跑: cat docs/handoff/2026-07-18-sector-allocation-closure-progress.md
2. 跑: git log --oneline -10 確認當前 commit 包含 d12b5526（SA03 完成）
3. 跑: bash scripts/verify-sector-allocation-closure.sh docs/manifests/sector-allocation-simulation-closure-manifest.md
4. 若上述未含 SA06 標記為 done，停止並提示 worktree A 尚未完成 contract；不要開始
5. 若 A 已完成 SA06，從 Task 7 開始依 TDD 執行
6. 每次 commit 前跑 gate
7. 全部完成後跑最後一次完整 gate + 寫 verification report

完畢訊號: 給我:
- SA07/SA09/SA10/SA11/SA12 全部 commit hash 與日期
- 最後一次完整 gate 的 exit code
- 觀察期需要 manual 確認的 7 項
```

### 接手提示詞結束

---

## 4. Worktree A 必須完成後才能 unlock B 的檢查清單

```bash
# 從根目錄檢查
cd /Users/kaecer/workspace/atlas
git log --oneline | grep -E "feat\(manifest\): #SA0[456]|feat\(manifest\): #SA08"
# 預期看到 3+ 個 commit:
#   feat(manifest): #SA04 canonical 20-L1 WeightEngine
#   feat(manifest): #SA05 capital-flow anti-corruption
#   feat(manifest): #SA06 composition-root shared engine + six-path matrix
#   feat(manifest): #SA08 next-session policy + allocator consumption + CLI live-sync isolation
```

並在 manifest 中查詢：

```bash
grep -E '\| SA0[4568] \|' docs/manifests/sector-allocation-simulation-closure-manifest.md
# 確認 status 欄為 implemented
```

若上述條件未滿足，B 不可啟動。
