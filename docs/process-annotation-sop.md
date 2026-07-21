# GitNexus Process 標註 SOP

> **目的**：把 GitNexus 自動偵測到的「機械化」process 摘要，升級為「人類可讀、Agent 可推理」的完整描述。
> **現狀**：GitNexus 僅給機械化 summary（如 `Main → LoadMaturityState`）。無 trigger / dependency / criticality metadata。
> **目標**：每個 WA-XXX workflow 對應的 entry-point 都有結構化、可被 agent 解析的描述。

---

## 1. 標註載體

### 1.1 方案 A — Go 函式結構化註解（推薦 Phase 1）

每個 workflow 入口函式加上固定 tag，後續可寫小工具解析為 `docs/PROCESSES.json`。

```go
// atlas:process WA-301-llm-loop
// atlas:description 透過 LLM router 對單一標的執行 Plan → ToolCall → Reflect loop，
//                  直到 LLM 發出最終 conviction（見 docs/specs/agent-loop-state-machine-spec.md）
// atlas:triggers
//   - event: SectorAgentLLM.PlanReflectRunner.PlanStep
//   - cron: -
//   - manual: -
// atlas:depends_on
//   - WA-201 (narrative events consumed via tool registry)
//   - WA-500 (strategy context from current strategy set)
// atlas:produces
//   - domain.Recommendation {symbol, conviction, reasoning}
// atlas:criticality P0
// atlas:test_coverage 87%
func PlanStepRunner(ctx context.Context, ...) (...)
```

**優點**：維護成本最低（就在程式碼旁邊）、review 時自然看到、無 new format to learn
**限制**：tag 數量多時易亂；超過 8 個 tag 應改用 YAML

### 1.2 方案 B — 維護 docs/reference/processes.yaml（推薦 Phase 2）

```yaml
# docs/reference/processes.yaml
- id: WA-301-llm-loop
  symbol: internal/orchestrator/sector_agent_llm.go::SectorAgentLLM.PlanStepRunner
  description: |
    透過 LLM router 對單一標的執行 Plan → ToolCall → Reflect loop。
    詳見 agent-loop-state-machine.md。
  triggers:
    - event: SectorAgentLLM.PlanReflectRunner
    - cron: null
  depends_on: [WA-201, WA-500]
  produces: domain.Recommendation
  criticality: P0
  test_coverage: 87
  last_verified: 2026-06-30
  verified_by: agent-rollout-team
```

**優點**：不污染程式碼；可集中管理；適合長期演進
**限制**：可能 stale（程式碼改了但 YAML 沒改）→ 需 CI check

---

## 2. SOP — 標註流程

### 2.1 第一次標註（一次性 sweep）

1. **盤查所有 entry-points**：對照 [`workflow-map.md`](./reference/workflow-map.md) §3 的 42 條 workflow
2. **為每個 entry 加 A 方案 tag**（每個 5-15 分鐘，視依賴複雜度）
3. **同步建立 `docs/reference/processes.yaml`** 作為單一真相來源
4. **驗證**：GitNexus 跑 `npx gitnexus analyze --processes-metadata docs/reference/processes.yaml`（需 GitNexus 支援，未來 PR）

### 2.2 後續維護

任何修改 workflow 的人必須：
1. 同步更新 A 方案 tag（如改了函式簽名）
2. 在 PR 描述中 link 到 processes.yaml 的對應行
3. CI 檢查：git diff 若改 entry-point 但未改 processes.yaml → 警告（不 block）

---

## 3. 待標註清單（基於 [`workflow-map.md`](./reference/workflow-map.md) v1）

| ID | 入口 | Phase |
|----|------|-------|
| WA-001 ~ 005 | `cmd/atlas/main.go::run*` | 1 |
| WA-101~103 | `internal/marketdata/*::Ingest*` | 1 |
| WA-200~202 | `internal/janus/*::Detect*`, `internal/narrative/*::*`, `internal/crossmarket/*::*` | 2 |
| WA-300~303 | `internal/orchestrator/sector_agent_llm.go::*` | 1（最關鍵） |
| WA-400~403 | `internal/risk/gate.go::*` | 1 |
| WA-500~505 | `internal/orchestrator/strategy_evolver.go::*`, `internal/experiment/*::*`, `internal/calibration/*::*` | 2 |
| WA-600~606 | `internal/monitoring/*::*`, `internal/alert/*::*`, `internal/portfolio/agent_health.go::*` | 2 |
| WA-700~701 | `internal/prism/*::*`, `internal/swarm/*::*` | 3 |

**總計**：21 條 + 預估 35-40 個 entry-points（部分 workflow 含多入口）

---

## 4. 驗收標準

- [ ] 42 條 workflow 全部有 `WA-XXX` 標籤
- [ ] 至少 80% 的 entry-point 有 Phase A tag
- [ ] `docs/reference/processes.yaml` 與 Phase A tag 同步
- [ ] CI 檢查「改 entry 必改 yaml」運作
- [ ] GitNexus 在 query WA-301 時回傳完整 description（驗證標註生效）
