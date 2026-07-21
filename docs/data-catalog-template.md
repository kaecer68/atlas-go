# DATA CATALOG TEMPLATE — atlas-go

**版本**: 1.0  
**日期**: 2026-06-02  
**狀態**: 權威標準（authoritative）  
**適用範圍**: 所有 `data/` 目錄下的資料資產  
**強制性**: CI 強制檢查新鮮度（`scripts/ci/check_data_catalog.sh`，P0.4c 實現）  
**相關文檔**: `docs/data-architecture.md` · `docs/data-naming-convention.md` · `docs/data-maturity-standard.md`

---

## 1. 背景與動機

### 1.1 問題發現

2026-06-02 的 P0.1 審計發現：atlas-go 擁有 1,255 個資料檔案，但沒有任何統一的資料目錄。開發者和 AI agent 在尋找特定資料時，必須：
1. 猜測資料可能存在的位置
2. `ls` 或 `find` 遍歷目錄結構
3. 閱讀實際檔案內容推斷欄位定義

這導致 AI discoverability 幾乎為零（Cat C 問題群）。

### 1.2 設計目標

> **一份文件，完整描述所有資料資產的「誰、什麼、何處、何時、如何」。**

---

## 2. 目錄格式

### 2.1 雙格式策略

| 格式 | 檔案 | 用途 |
|------|------|------|
| **Markdown** | `docs/data-catalog.md` | 人類可讀、導航、快速查找 |
| **JSON** | `docs/DATA_CATALOG.json` | 機器可讀、AI agent 程式化讀取、CI 驗證 |

兩者內容一致，JSON 版本由 Markdown 版本自動產生或兩者都由產生器腳本維護。

### 2.2 Markdown 格式

每個資料資產使用以下 Markdown 模板：

```markdown
### {asset_name} `{data_path}`

| 欄位 | 值 |
|------|-----|
| **路徑** | `data/state/outcomes/recommendation_outcomes.jsonl` |
| **類型** | JSONL (append-only) |
| **格式** | 每行一個 JSON 物件 |
| **大小** | ~144MB, ~38,630 lines |
| **成熟度** | S (stable) |
| **Schema** | [`schemas/recommendation_outcomes.schema.json`](../schemas/recommendation_outcomes.schema.json) |
| **Metadata** | `data/state/outcomes/_metadata.json` |

**描述**: 所有 session 的推薦結果聚合檔，以 O_APPEND 模式累積寫入。每筆記錄包含推薦方向、forward return、因子分數、信念度 breakdown、guard 狀態。

**生產者（Producer）**:
- `internal/ledger/outcomes.go` — `RecordOutcomes()` → O_APPEND 寫入
- `internal/orchestrator/executors.go` — `collectRecommendations()` → 觸發寫入

**消費者（Consumer）**:
- `internal/monitoring/api/dashboard/` — Dashboard API 讀取
- `web/static/index.html` — 前端「投資管線」頁面
- `internal/risk/calibrator.go` — RiskGate 校準（最近 30 session）
- `cmd/backtest-window/` — 回測視窗

**建立時間**: 2026-03-30（commit #2 f71c0a2）
**最近修改**: 持續寫入
**相關文檔**: `docs/data-architecture.md` §層級 2
```

### 2.3 JSON 格式

對應的機器可讀格式：

```json
{
  "name": "recommendation_outcomes",
  "path": "data/state/outcomes/recommendation_outcomes.jsonl",
  "type": "jsonl",
  "mode": "append",
  "size": {
    "bytes": 151257088,
    "lines": 38630
  },
  "maturity": "stable",
  "schema": "schemas/recommendation_outcomes.schema.json",
  "metadata": "data/state/outcomes/_metadata.json",
  "description": "所有 session 的推薦結果聚合檔，以 O_APPEND 模式累積寫入。",
  "producers": [
    {
      "module": "internal/ledger",
      "file": "internal/ledger/outcomes.go",
      "function": "RecordOutcomes",
      "mode": "O_APPEND"
    },
    {
      "module": "internal/orchestrator",
      "file": "internal/orchestrator/executors.go",
      "function": "collectRecommendations",
      "mode": "trigger"
    }
  ],
  "consumers": [
    {
      "module": "internal/monitoring",
      "path": "internal/monitoring/api/dashboard/",
      "purpose": "Dashboard API serving"
    },
    {
      "module": "web",
      "path": "web/static/index.html",
      "purpose": "投資管線前端頁面"
    },
    {
      "module": "internal/risk",
      "path": "internal/risk/calibrator.go",
      "purpose": "RiskGate 校準（最近 30 session）"
    },
    {
      "module": "cmd/backtest-window",
      "path": "cmd/backtest-window/main.go",
      "purpose": "回測視窗計算"
    }
  ],
  "created": "2026-03-30",
  "last_modified": "continuous",
  "ci_check": "scripts/ci/validate_json_schemas.sh",
  "related_docs": [
    "docs/data-architecture.md",
    "docs/data-naming-convention.md"
  ]
}
```

### 2.4 必填欄位

| JSON 欄位 | 說明 | 必填 | 範例 |
|-----------|------|------|------|
| `name` | 資產唯一識別名稱 | ✅ | `"recommendation_outcomes"` |
| `path` | 相對於專案根目錄的路徑 | ✅ | `"data/state/outcomes/recommendation_outcomes.jsonl"` |
| `type` | 資料格式 | ✅ | `"jsonl"`, `"json"`, `"csv"`, `"sqlite"` |
| `maturity` | 成熟度層級 | ✅ | `"stable"`, `"evolving"`, `"experimental"`, `"utility"` |
| `description` | 一句話描述 | ✅ | `"所有 session 的推薦結果聚合檔"` |
| `producers` | 寫入此資料的 Go module 列表 | ✅ | `[{...}]` |
| `consumers` | 讀取此資料的 Go module 列表 | ✅ | `[{...}]` |
| `size.bytes` | 檔案大小（位元組） | 建議 | `151257088` |
| `size.lines` | JSONL 行數 | JSONL 才建議 | `38630` |
| `schema` | JSON Schema 檔案路徑 | 有 Schema 時必填 | `"schemas/recommendation_outcomes.schema.json"` |
| `metadata` | `_metadata.json` 路徑 | state/ 下必填 | `"data/state/outcomes/_metadata.json"` |
| `created` | 建立日期（ISO 8601） | 建議 | `"2026-03-30"` |
| `ci_check` | 對應的 CI 檢查腳本 | 建議 | `"scripts/ci/validate_json_schemas.sh"` |

---

## 3. 資產分類

### 3.1 按資料類型

| 分類 | 說明 | 目錄 |
|------|------|------|
| `jsonl` | JSON Lines（append-only） | `data/state/` |
| `json` | 單一 JSON 檔案 | `data/state/` 子目錄 |
| `daily_json` | 每日 JSON 快照 | `data/state/macro/`, `margin/`, `capital_flow/` |
| `csv` | CSV 回放數據 | `data/replay/` |
| `sqlite` | SQLite 資料庫 | `data/state/atlas.db` |
| `yaml` | YAML 配置 | `data/state/constraint-mutations/` |

### 3.2 按成熟度

對齊 `docs/data-maturity-standard.md`：

| Tier | 標記 | 說明 |
|------|------|------|
| **S** | `stable` | 在生產執行路徑中，API 結構穩定 |
| **E** | `evolving` | 功能完整但仍在迭代 |
| **X** | `experimental` | 研究性質，可能被廢棄 |
| **U** | `utility` | 輔助工具資料，非 runtime |

---

## 4. AI Discoverability 設計

### 4.1 AI Agent 如何發現資料

```
1. AI 讀取 docs/DATA_CATALOG.json（機器可讀，一次載入）
   → 獲得所有 39 個資料資產的清單、路徑、格式、描述
   
2. AI 對特定資產有疑問：
   → 讀取 docs/data-catalog.md（人類可讀，含詳細說明）
   → 讀取 data/state/{name}/_metadata.json（每個子目錄的 metadata）
   → 讀取 schemas/{name}.schema.json（結構定義）

3. AI 需要理解資料流：
   → 讀取 docs/data-architecture.md（架構文件）
   → 使用 catalog 的 producers/consumers 欄位追蹤讀寫關係
```

### 4.2 設計原則

| 原則 | 說明 |
|------|------|
| **一次載入** | AI 可透過讀取一個 JSON 檔案（`DATA_CATALOG.json`）獲得全部資料資產概覽 |
| **階層式深入** | catalog（總覽）→ _metadata.json（目錄級）→ schema（結構級）→ 實際資料（內容級） |
| **雙向關聯** | producers/consumers 欄位讓 AI 可以從資料追溯到程式碼，或從程式碼找到資料 |
| **自我描述** | `_metadata.json` 放在資料旁邊，無需額外查詢 |

---

## 5. 目錄維護

### 5.1 何時更新

| 觸發條件 | 必須更新 | 建議更新 |
|---------|---------|---------|
| 新增資料檔案或目錄 | `data-catalog.md` + `.json` | — |
| 刪除資料檔案或目錄 | `data-catalog.md` + `.json` | — |
| 變更資料結構（新增/刪除欄位） | Schema 檔案 | catalog 中的 `description` 欄位 |
| 變更 producers/consumers | catalog 中的對應欄位 | — |
| 檔案大小顯著變化（>20%） | catalog 中的 `size` 欄位 | — |

### 5.2 CI 新鮮度檢查

`scripts/ci/check_data_catalog.sh`（P0.4c）會執行：

```
1. 列舉 data/ 目錄下所有檔案
2. 對照 catalog 中的 path 欄位
3. 報告:
   - catalog 中有但檔案已不存在 → ❌ 錯誤（stale entry）
   - 檔案存在但 catalog 中無記錄 → ⚠️ 警告（undocumented）
   - 子目錄存在但無 _metadata.json → ⚠️ 警告
```

### 5.3 自動化產生工具

建議實作 `scripts/gen_data_catalog.sh`：

```bash
#!/bin/bash
# 掃描 data/ 目錄，自動產生 catalog 的骨架
# 人工補充 descriptions、producers、consumers 等語意欄位
```

---

## 6. 現有資料資產總覽

以下是 2026-06-02 審計中發現的所有資料資產，按其目錄分類：

### 6.1 頂層檔案

| 資產 | 路徑 | 類型 | 大小 |
|------|------|------|------|
| fundamentals | `data/fundamentals.json` | JSON | ~84KB |
| test_returns | `data/test_returns.json` | JSON | — |

### 6.2 data/state/ 子目錄（23 個）

| # | 目錄 | 說明 | 典型檔案數 |
|---|------|------|-----------|
| 1 | `alerts/` | 系統告警 | 1 |
| 2 | `approvals/` | 人工核准記錄 | 74 |
| 3 | `autobacktest/` | 自動回測記錄 | 1 |
| 4 | `branch_protection/` | 分支保護快照 | 4 |
| 5 | `capital_flow/` | 資金流向（每日） | 82 |
| 6 | `constraint_mutations/` | 限制條件突變 | 1 YAML |
| 7 | `strategy_techniques/` | 投資心法狀態 | 12 |
| 8 | `experiments/` | 實驗記錄 + archive | 183 |
| 9 | `export/` | 匯出資料 | 3 |
| 10 | `finmind/` | FinMind API 快取 | 1 |
| 11 | `fubon/` | Fubon API 快取 | 1 |
| 12 | `fugle/` | Fugle API 快取 | 1 |
| 13 | `geopolitical/` | 地緣政治數據 | 2 |
| 14 | `live/state/` | 即時交易狀態 | — |
| 15 | `macro/` | 總經指標（每日） | 38 |
| 16 | `margin/` | 融資融券（每日） | 34 |
| 17 | `ml_models/` | ML 模型檔案 | 4 |
| 18 | `mutation_briefs/` | 突變提案 | 132 |
| 19 | `parameter_snapshots/` | 參數快照 | 17 |
| 20 | `sessions/` | Session 記錄 | 100 |
| 21 | `swarm_training/` | Swarm 訓練 | 5 JSONL |
| 22 | `traces/` | 執行追蹤 | 27 |
| 23 | `tsmc_revenue/` | 台積電營收 | 2 |
| — | `windows/` | 回測視窗 | 95 |

### 6.3 data/state/ 平面檔案（17 個，待遷移）

| 資產 | 平面檔案路徑 | 目標子目錄（P3.0） |
|------|------------|------------------|
| atlas_db | `data/state/atlas.db` | TBD（P2.1） |
| baseline_policy | `data/state/baseline_policy.json` | `data/state/baseline/` |
| channel_health | `data/state/channel_health.json` | `data/state/channel_health/` |
| clamping_events | `data/state/clamping_events.jsonl` | `data/state/clamping/` |
| darwinian_history | `data/state/darwinian_history.jsonl` | `data/state/darwinian/` |
| darwinian_weights | `data/state/darwinian_weights.json` | `data/state/darwinian/` |
| experiments | `data/state/experiments.jsonl` | `data/state/experiments/` |
| human_interventions | `data/state/human_interventions.jsonl` | `data/state/human_interventions/` |
| maturity_tracker | `data/state/maturity_tracker.json` | `data/state/maturity/` |
| metalearner_state | `data/state/metalearner_state.json` | `data/state/metalearner/` |
| metrics | `data/state/metrics.jsonl` | `data/state/metrics/` |
| phase3_metrics | `data/state/phase3_metrics.json` | `data/state/metrics/` |
| recommendation_outcomes | `data/state/recommendation_outcomes.jsonl` | `data/state/outcomes/` |
| simulation_state | `data/state/simulation_state.json` | `data/state/simulation/` |
| swarm_latest | `data/state/swarm_latest.json` | `data/state/swarm/` |
| backup | `data/state/recommendation_outcomes.jsonl.backup.*` | `data/archive/` |

---

## 7. 相關文檔

| 文檔 | 關係 |
|------|------|
| `docs/data-architecture.md` | 各資料類型的詳細讀寫路徑 |
| `docs/data-directory-standard.md` | 目錄結構規範 |
| `docs/data-naming-convention.md` | 檔案命名規則 |
| `docs/data-maturity-standard.md` | `_metadata.json` 格式 |
| `docs/json-schema-standard.md` | JSON Schema 定義標準 |
| `.omo/audit/2026-06-02-p0-2-root-cause-analysis.md`（內部）| 根因分析|`.omo/audit/2026-06-02-p0-2-root-cause-analysis.md`（內部）| 根因分析 |
