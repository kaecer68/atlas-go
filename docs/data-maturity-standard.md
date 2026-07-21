# DATA MATURITY MARKER STANDARD — atlas-go

**版本**: 1.0  
**日期**: 2026-06-02  
**狀態**: 權威標準（authoritative）  
**適用範圍**: `data/` 目錄下所有子目錄  
**強制性**: CI 強制檢查（`scripts/ci/check_data_catalog.sh`，P0.4c 實現）  
**相關文檔**: `internal/MATURITY.md`（Go code maturity）· `docs/data-directory-standard.md` · `docs/data-catalog.md`

---

## 1. 背景與動機

### 1.1 問題發現

atlas-go 的 Go 程式碼有完善的成熟度標記系統（`doc.go` 中的 `// Maturity: <tier>` + `internal/MATURITY.md` 參考表），但 **資料資產完全沒有對應機制**。

P0.1 審計發現：
- 沒有任何資料目錄告訴開發者這個資料是"穩定生產"還是"實驗中可能被刪除"
- `data/state/` 下的平面檔案與子目錄混雜，無從判斷哪些是核心依賴、哪些是實驗殘留
- AI agent 無法區分 `phase3_metrics.json`（實驗殘留？還是核心指標？）與 `recommendation_outcomes.jsonl`（穩定生產核心）

### 1.2 設計目標

> **為每個資料資產建立自我描述的中繼資料檔案，與 Go 程式碼的 maturity 層級完全對齊。**  
> **AI agent 讀取 `_metadata.json` 即可知道此資料的成熟度、維護者、用途，無需猜測。**

---

## 2. `_metadata.json` 格式

### 2.1 完整範本

每個 `data/state/` 下的子目錄（或頂層資料目錄）必須包含一個 `_metadata.json` 檔案：

```json
{
  "$schema": "https://github.com/kaecer68/atlas-go/schemas/data_metadata.schema.json",
  "name": "recommendation_outcomes",
  "description": "所有 session 的推薦結果聚合檔。以 O_APPEND 累積寫入，為系統最重要的輸出資料。",
  "maturity": "stable",
  "data_type": "jsonl",
  "format": "append_only",
  "producer_module": "internal/ledger",
  "producer_function": "RecordOutcomes",
  "consumer_modules": [
    "internal/monitoring",
    "internal/risk",
    "cmd/backtest-window"
  ],
  "schema_ref": "schemas/recommendation_outcomes.schema.json",
  "catalog_ref": "docs/data-catalog.md#recommendation_outcomes",
  "created_date": "2026-03-30",
  "last_modified": "2026-06-02",
  "estimated_size": {
    "lines": 38630,
    "bytes": 151257088,
    "growth_rate": "~100 lines/day"
  },
  "retention_policy": "indefinite",
  "backup_policy": "gitignored, PostgreSQL dual-write as secondary",
  "tags": ["outcomes", "recommendations", "forward-return", "core"],
  "notes": "全局檔案約為 per-session 總和的 14.7 倍大，可能有重複資料（P0.1 finding C.3）。"
}
```

### 2.2 欄位定義

| 欄位 | 型別 | 必填 | 說明 |
|------|------|------|------|
| `$schema` | string | 建議 | 指向 metadata schema 定義 |
| `name` | string | ✅ | 資產唯一識別名稱（snake_case） |
| `description` | string | ✅ | 一句話描述此資料的用途與內容 |
| `maturity` | string enum | ✅ | `stable` / `evolving` / `experimental` / `utility` |
| `data_type` | string enum | ✅ | `json` / `jsonl` / `csv` / `sqlite` / `yaml` / `directory` |
| `format` | string | 建議 | 額外格式資訊：`append_only`、`daily_snapshot`、`key_value` 等 |
| `producer_module` | string | ✅ | 寫入此資料的主要 Go module（如 `internal/ledger`） |
| `producer_function` | string | 建議 | 寫入函數名稱（如 `RecordOutcomes`） |
| `consumer_modules` | string[] | ✅ | 讀取此資料的 Go module 列表 |
| `schema_ref` | string | 條件 | 若有 JSON Schema，指向 `schemas/{name}.schema.json` |
| `catalog_ref` | string | 建議 | 指向 `docs/data-catalog.md` 中的對應條目 |
| `created_date` | string | 建議 | 建立日期（ISO 8601） |
| `last_modified` | string | 建議 | 最後修改日期（ISO 8601） |
| `estimated_size` | object | 建議 | `{lines, bytes, growth_rate}` — 檔案大小估計 |
| `retention_policy` | string | 建議 | 保留策略：`indefinite`、`30_days`、`manual` |
| `backup_policy` | string | 建議 | 備份策略 |
| `tags` | string[] | 建議 | 用於分類與搜尋的標籤 |
| `notes` | string | 建議 | 任何需要注意的事項（如已知問題） |

### 2.3 `maturity` 欄位列舉值

完全對齊 `internal/MATURITY.md` 的層級定義：

| 值 | Tier | 說明 | 判斷標準 |
|----|------|------|----------|
| `stable` | S | 穩定生產 | 在生產執行路徑中、被多個模組依賴、格式穩定 |
| `evolving` | E | 演進中 | 功能完整但格式可能調整、少數模組使用 |
| `experimental` | X | 實驗中 | 研究性質、可能被廢棄、不應被核心模組依賴 |
| `utility` | U | 輔助工具 | CLI 工具輸出、一次性驗證資料、非 runtime |

### 2.4 判斷規則

| 條件 | maturity |
|------|----------|
| 被 `cmd/atlas` 直接讀取 | `stable` |
| 被 2+ 個 `internal/` 模組依賴 | `stable` |
| 格式在過去 3 個月內未變更 | `stable` |
| 被 1 個 `internal/` 模組使用，格式可能變 | `evolving` |
| 僅被 `cmd/experimental/` 使用 | `experimental` |
| 僅被 CLI 工具產生 | `utility` |

---

## 3. 現有資料資產的 maturity 建議

### 3.1 Stable（S）

| 資產 | 目錄 | 理由 |
|------|------|------|
| recommendation_outcomes | `data/state/outcomes/` | 核心輸出，Dashboard API、RiskGate、回測均依賴 |
| darwinian_weights | `data/state/darwinian/` | 組合權重，被 orchestrator 和 portfolio 讀取 |
| darwinian_history | `data/state/darwinian/` | 權重演進歷史 |
| baseline_policy | `data/state/baseline/` | Baseline 版本控制核心 |
| experiments | `data/state/experiments/` | 實驗生命週期核心 |
| human_interventions | `data/state/human_interventions/` | 人工干預稽核軌跡 |
| sessions/ 目錄 | `data/state/sessions/` | Session 持久層（data-architecture.md 層級 1） |
| approvals/ 目錄 | `data/state/approvals/` | 人工核准記錄 |

### 3.2 Evolving（E）

| 資產 | 目錄 | 理由 |
|------|------|------|
| macro/ | `data/state/macro/` | 仍在迭代新增指標 |
| margin/ | `data/state/margin/` | 格式穩定但使用範圍有限 |
| capital_flow/ | `data/state/capital_flow/` | 格式穩定但使用範圍有限 |
| metrics | `data/state/metrics/` | 監控指標，格式可能擴展 |
| clamping_events | `data/state/clamping/` | 事件記錄，格式穩定 |
| mutation_briefs/ | `data/state/mutation_briefs/` | 突變提案，格式可能調整 |
| swarm_latest + swarm_training | `data/state/swarm/` | Swarm 訓練仍在迭代 |
| metalearner_state | `data/state/metalearner/` | 元學習狀態，仍在開發 |

### 3.3 Experimental（X）

| 資產 | 目錄 | 理由 |
|------|------|------|
| ml_models/ | `data/state/ml_models/` | ML 模型，實驗階段 |
| geopolitical/ | `data/state/geopolitical/` | 地緣政治數據，使用範圍有限 |
| strategy_techniques/ | `data/state/strategy_techniques/` | 投資心法狀態，已穩定生產 |
| tsmc_revenue/ | `data/state/tsmc_revenue/` | 台積電營收分析，使用範圍有限 |
| windows/ | `data/state/windows/` | 回測視窗，實驗性質 |

### 3.4 Utility（U）

| 資產 | 目錄 | 理由 |
|------|------|------|
| channel_health | `data/state/channel_health/` | API 健康檢查快照 |
| branch_protection/ | `data/state/branch_protection/` | 分支保護快照，非 runtime |
| export/ | `data/state/export/` | 匯出工具輸出 |
| parameter_snapshots/ | `data/state/parameter_snapshots/` | 參數快照，輔助用途 |
| finmind/ / fubon/ / fugle/ | `data/state/{finmind,fubon,fugle}/` | 第三方 API 快取 |
| traces/ | `data/state/traces/` | 執行追蹤，診斷用途 |
| maturity_tracker | `data/state/maturity/` | 成熟度追蹤工具輸出 |
| autobacktest/ | `data/state/autobacktest/` | 自動回測記錄 |
| constraint_mutations/ | `data/state/constraint_mutations/` | 限制條件突變輔助 |
| simulation_state | `data/state/simulation/` | 模擬狀態（可能遷入 E） |
| alerts/ | `data/state/alerts/` | 系統告警記錄 |

---

## 4. `_metadata.json` 的放置規則

### 4.1 放置層級

| 目錄結構 | metadata 位置 | 說明 |
|---------|-------------|------|
| 子目錄（有內容） | `data/state/{name}/_metadata.json` | 每個有實際資料的子目錄一個 |
| 每日數據子目錄 | `data/state/macro/_metadata.json` | 整個目錄一個（不為每天建立） |
| 嵌套子目錄 | 最內層子目錄各一個 | 如 `data/state/sessions/` 不需要（sessions 內部結構已標準化） |
| 頂層目錄 | `data/replay/_metadata.json`、`data/cache/_metadata.json` 等 | 頂層分類目錄也需要 |

### 4.2 例外

| 目錄 | 理由 | 處理 |
|------|------|------|
| `data/state/live/state/` | 內部結構由即時交易模組管理 | 維持現狀，由 live 模組自行管理 |
| `data/state/sessions/` 下個別 session | 自動生成，數量過多（100+） | 不需要個別 `_metadata.json`，由 sessions/ 目錄級 metadata 描述 |
| `data/replay/` 下的個別檔案 | 唯讀歷史數據 | 由 `data/replay/_metadata.json` 統籌描述 |

---

## 5. 維護與更新

### 5.1 何時更新

| 變更 | 更新欄位 |
|------|---------|
| 新增資料目錄 | 建立新的 `_metadata.json` |
| 刪除資料目錄 | 刪除對應的 `_metadata.json` |
| 新增 consumer modules | 更新 `consumer_modules` 陣列 |
| maturity 層級變更 | 更新 `maturity` 欄位（X→E 或 E→S 需 PR review） |
| 檔案大小顯著變化 | 更新 `estimated_size` |
| 新增 JSON Schema | 新增 `schema_ref` 欄位 |

### 5.2 maturity 變更流程

與 Go 程式碼 maturity 變更相同：

```
1. 修改 _metadata.json 中的 maturity 欄位
2. 更新 docs/data-catalog.md 中的對應條目
3. 執行 CI 檢查：bash scripts/ci/check_data_catalog.sh
4. X→E 或 E→S 視為晉升，需 PR review
5. S→E 或任何降級需 migration plan（說明如何處理現有依賴）
```

---

## 6. CI 強制執行

`scripts/ci/check_data_catalog.sh`（P0.4c）會檢查：

```
1. data/state/ 下每個子目錄都有 _metadata.json
   → 缺少 → ⚠️ 警告

2. _metadata.json 中的 maturity 欄位為有效值
   → 無效 → ❌ 錯誤

3. _metadata.json 中的 maturity 與 catalog 中的 maturity 一致
   → 不一致 → ❌ 錯誤

4. catalog 中有條目但 _metadata.json 不存在 → ⚠️ 警告
5. _metadata.json 存在但 catalog 中無條目 → ⚠️ 警告
```

---

## 7. 自動化產生工具

`scripts/gen_data_metadata.sh`（P2.3 實現）：

```bash
#!/bin/bash
# 基於 docs/DATA_CATALOG.json 自動產生或更新 _metadata.json 檔案
# 使用 jq 從 catalog 中提取對應欄位，產生每個子目錄的 _metadata.json
```

---

## 8. 相關文檔

| 文檔 | 關係 |
|------|------|
| `internal/MATURITY.md` | Go 程式碼成熟度參考（本文件完全對齊其層級定義） |
| `docs/data-directory-standard.md` | 定義哪些目錄需要 `_metadata.json` |
| `docs/data-catalog.md` | 資料目錄（`maturity` 欄位與 `_metadata.json` 保持一致） |
| `docs/json-schema-standard.md` | `schema_ref` 欄位指向的 schema 定義 |
| `.omo/audit/2026-06-02-p0-2-root-cause-analysis.md`（內部）| 根因分析（FG-1: 無規範）| 根因分析（FG-1: 無規範） |
