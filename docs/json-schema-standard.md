# JSON SCHEMA STANDARD — atlas-go

**版本**: 1.0  
**日期**: 2026-06-02  
**狀態**: 權威標準（authoritative）  
**適用範圍**: `data/` 目錄下所有 JSON/JSONL 檔案  
**強制性**: CI 強制檢查（`scripts/ci/validate_json_schemas.sh`，P0.4b 實現）  
**相關文檔**: `docs/data-naming-convention.md` · `docs/data-architecture.md` · `internal/domain/`（Go domain types）

---

## 1. 背景與動機

### 1.1 問題發現

2026-06-02 的 P0.1 審計發現：atlas-go 擁有 **319 個 JSONL 檔案 + 921 個 JSON 檔案**，但 **零個 JSON Schema 檔案**。

```
$ find data/ -name "*.schema.json" | wc -l
0
```

這導致：
1. **AI 無法推理資料結構**：必須讀取實際檔案內容才能理解欄位定義
2. **無自動化驗證**：資料寫入錯誤（欄位缺失、型別錯誤）在寫入時無法偵測，直到讀取時才發現
3. **無資料契約**：生產者（Go module）與消費者（Dashboard API、前端）之間沒有結構合約
4. **重複推斷成本**：每次 AI agent 或新開發者接觸 JSONL 檔案時，都必須重新推斷結構

### 1.2 P0.2 根因

FG-1（無資料治理規範）：自專案建立以來 1,144 次 commit 中，從未有過 JSON Schema 相關的討論或文件。`cmd/gentags` 已實作 Go struct → 前端型別定義的自動生成，但從未考慮 Go struct → JSON Schema 的生成。

---

## 2. JSON Schema 版本與位置

| 項目 | 規定 |
|------|------|
| **Schema 版本** | JSON Schema **Draft-07**（相容性最佳、工具支援最廣） |
| **Schema 目錄** | `schemas/`（專案根目錄下） |
| **檔案命名** | `{data_type_name}.schema.json`（snake_case，遵循 data-naming-convention.md） |
| **每行驗證** | JSONL 檔案逐行驗證（每行為獨立 JSON 物件），不是整檔驗證 |

### 2.1 為什麼選 Draft-07

| 原因 | 說明 |
|------|------|
| **工具生態** | `ajv`（Node.js）、`check-jsonschema`（Python）、`gojsonschema`（Go）均原生支援 |
| **穩定性** | 2019-09 及之後的版本在部分工具中仍為實驗性支援 |
| **足夠表達力** | Draft-07 可完整描述 atlas-go 的所有 domain types |
| **已知限制** | `$defs`（Draft 2019-09+ 取代 `definitions`）在未來升級時才需要 |

---

## 3. Schema 與 Go Domain Types 的關係

### 3.1 對應規則

atlas-go 的 JSONL 欄位來源是 `internal/domain/` 中的 Go struct。Schema 必須與這些 struct 的 JSON tag 保持一致。

**規則**：

| JSON Schema 欄位 | 來源 |
|-----------------|------|
| 頂層欄位名稱 | Go struct 的 `json:"field_name"` tag |
| 欄位型別 | Go 型別 → JSON Schema type 映射（見下表） |
| 欄位描述 | Go struct 的 comment（`// 描述文字`） |
| 必填/可選 | 取決於 omitempty tag 及業務邏輯 |

### 3.2 型別映射

| Go 型別 | JSON Schema type | 備註 |
|---------|-----------------|------|
| `string` | `"string"` | |
| `int` / `int64` / `float64` | `"number"` | Go 無整數/浮點 JSON Schema 區分 |
| `bool` | `"boolean"` | |
| `[]T` | `{"type": "array", "items": {...}}` | |
| `map[string]T` | `{"type": "object", "additionalProperties": {...}}` | |
| `time.Time` | `{"type": "string", "format": "date-time"}` | RFC 3339 |
| `*T`（指標） | 可選欄位，不加 `required` | nil → JSON null 或欄位缺失 |
| `domain.Regime`（string enum） | `{"type": "string", "enum": [...]}` | |

### 3.3 自動生成策略

參考 `cmd/gentags` 的前端型別生成模式：

```
Go struct (with json tags) → JSON Schema
```

| 方式 | 工具 | 狀態 |
|------|------|------|
| **手動編寫** | 直接寫 `.schema.json` | ✅ 當前階段（P1.2） |
| **半自動** | `go run ./cmd/gentags --schema` | 📋 未來計劃 |
| **全自動** | CI 自動檢查 schema 與 struct 一致性 | 📋 未來計劃 |

---

## 4. Schema 範本

以下是以 `recommendation_outcomes.jsonl` 為例的 Schema 範本：

### 4.1 範本結構

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "$id": "https://github.com/kaecer68/atlas-go/schemas/recommendation_outcomes.schema.json",
  "title": "Recommendation Outcome",
  "description": "單筆推薦的 forward return 評估結果。每行為獨立 JSON 物件。",
  "type": "object",
  "required": ["session_id", "symbol", "agent_id", "recommendation"],
  "properties": {
    "session_id": {
      "type": "string",
      "description": "交易日 session ID，格式 session-YYYYMMDD-daily",
      "pattern": "^session-\\d{8}-daily$"
    },
    "symbol": {
      "type": "string",
      "description": "股票代碼，4 位數字字串，如 '2330'",
      "pattern": "^\\d{4}$"
    },
    "agent_id": {
      "type": "string",
      "description": "產生推薦的 agent 識別碼"
    },
    "recommendation": {
      "type": "string",
      "description": "推薦方向",
      "enum": ["buy", "sell", "hold"]
    },
    "confidence": {
      "type": "number",
      "description": "信心度分數 0.0-1.0",
      "minimum": 0,
      "maximum": 1
    },
    "forward_return_1d": {
      "type": "number",
      "description": "1 日 forward return（可為 null）"
    },
    "forward_return_5d": {
      "type": "number",
      "description": "5 日 forward return（可為 null）"
    },
    "factor_scores": {
      "type": "object",
      "description": "因子分數 breakdown",
      "properties": {
        "momentum": { "type": "number" },
        "value": { "type": "number" },
        "quality": { "type": "number" }
      }
    },
    "conviction_breakdown": {
      "type": "object",
      "description": "信念度計算 breakdown",
      "properties": {
        "base": { "type": "number" },
        "final": { "type": "number" },
        "steps": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "rule": { "type": "string" },
              "delta": { "type": "number" },
              "reason": { "type": "string" }
            }
          }
        }
      }
    },
    "passed_guards": {
      "type": "boolean",
      "description": "是否通過控制層 guard"
    },
    "recorded_at": {
      "type": "string",
      "format": "date-time",
      "description": "記錄時間（RFC 3339）"
    }
  },
  "additionalProperties": true
}
```

### 4.2 關鍵設計決策

| 決策 | 理由 |
|------|------|
| `additionalProperties: true`（而非 `false`） | JSONL 欄位隨版本演進頻繁新增，嚴格模式導致大量誤報；CI 應只檢查必要欄位存在 |
| 最小 `required` 欄位 | 僅標記業務上不可或缺的欄位（session_id、symbol、agent_id、recommendation） |
| `enum` 用於 domain enum | `recommendation`、`regime` 等 domain enum 必須列舉所有有效值 |
| `pattern` 驗證 ID 格式 | session_id 格式強制正則驗證，防止格式漂移 |

---

## 5. 需要 Schema 的資料類型

### 5.1 優先級

| 優先級 | 資料類型 | 檔案 | 原因 |
|--------|---------|------|------|
| **P0** | recommendation_outcomes | `data/state/recommendation_outcomes.jsonl` + per-session | 最大（144MB）、最重要、API serving |
| **P1** | experiments | `data/state/experiments.jsonl` | 實驗生命週期核心 |
| **P1** | human_interventions | `data/state/human_interventions.jsonl` | 人工干預稽核軌跡 |
| **P1** | darwinian_history | `data/state/darwinian_history.jsonl` | 權重演進歷史 |
| **P2** | metrics | `data/state/metrics.jsonl` | 監控指標 |
| **P2** | clamping_events | `data/state/clamping_events.jsonl` | 權重夾制事件 |
| **P2** | screened_symbols | `data/state/sessions/*/screened_symbols.jsonl` | 篩選器記錄 |
| **P3** | swarm_training | `data/state/swarm_training/` | Swarm 訓練過程 |
| **P3** | traces | `data/state/traces/` | 執行追蹤 |

### 5.2 不需要 Schema 的資料

| 資料類型 | 原因 |
|---------|------|
| CSV 檔案 | 非 JSON 格式，欄位驗證由 importer 自行處理 |
| SQLite 資料庫 | 架構由 migration 管理 |
| 每日 JSON 檔案（macro/、margin/、capital_flow/） | 欄位隨來源 API 變化，Schema 維護成本過高；改為 CI 檢查檔案結構一致性（P0.4a） |
| 第三方快取（finmind/、fubon/、fugle/） | 外部 API 回應，結構不在我們控制範圍 |

---

## 6. CI 驗證策略

### 6.1 驗證腳本

`scripts/ci/validate_json_schemas.sh`（P0.4b 實現）的設計：

```bash
# 驗證邏輯（偽代碼）
for schema in schemas/*.schema.json; do
    data_type=$(basename "$schema" .schema.json)
    # 找到對應的 JSONL 檔案
    for jsonl in $(find data/ -name "${data_type}.jsonl"); do
        # 逐行驗證（streaming，支援大檔案）
        while IFS= read -r line; do
            echo "$line" | ajv validate -s "$schema" --errors=text
        done < "$jsonl"
    done
done
```

### 6.2 錯誤處理

| 錯誤類型 | 行為 | 範例 |
|---------|------|------|
| Schema 存在但無對應 JSONL | ⚠️ 警告 | "Schema `experiments.schema.json` has no matching data files" |
| JSONL 存在但無 Schema | ⚠️ 警告（非阻塞） | "`data/state/traces/*.jsonl` has no schema — please add one" |
| 欄位驗證失敗 | ❌ 錯誤（阻塞 CI） | "Line 42: `session_id` missing (required)" |
| 型別不匹配 | ❌ 錯誤（阻塞 CI） | "Line 128: `confidence` expected number, got string" |

### 6.3 大型檔案處理

`recommendation_outcomes.jsonl` 有 38,630 行（144MB），完整驗證耗時過長。策略：

1. **快速模式（CI）**：僅驗證前 1,000 行
2. **完整模式（手動）**：`bash scripts/ci/validate_json_schemas.sh --full`
3. **增量模式（未來）**：僅驗證自上次 commit 後新增的行

---

## 7. Schema 維護流程

```
1. 當 Go domain struct 新增/修改/刪除欄位時：
   → 檢查 schemas/ 中是否有對應 schema
   → 如有，手動更新 schema（同步變更）
   → 或執行 go run ./cmd/gentags --schema-todo（產生待辦清單）

2. 當新增 JSONL 檔案類型時：
   → 建立新 schema 檔案於 schemas/{name}.schema.json
   → 遵循本文件第 4 節的範本結構
   → 更新 docs/data-catalog.md 中的 schema_ref 欄位

3. 每次 PR：
   → CI 自動執行 validate_json_schemas.sh
   → 若有 schema 變更，CI 檢查對應 JSONL 的前 1000 行
```

---

## 8. 與現有工具的整合

| 工具 | 關係 |
|------|------|
| `cmd/gentags` | 未來可擴展為 Go struct → JSON Schema 自動生成（類似現有的 Go struct → 前端型別生成） |
| `internal/domain/` | Schema 的 truth source — 所有 JSON 欄位名稱必須與 Go struct 的 `json:` tag 一致 |
| `internal/ledger/` | JSONL 寫入的 source of truth — schema 驗證應在此層或 CI 層進行 |
| `docs/data-catalog.md` | 每個資料資產的 `schema_ref` 欄位指向對應的 schema 檔案 |

---

## 9. 相關文檔

| 文檔 | 關係 |
|------|------|
| `docs/data-naming-convention.md` | Schema 檔案命名遵循 R1（snake_case） |
| `docs/data-directory-standard.md` | Schema 存放位置定義 |
| `docs/data-architecture.md` | 各資料類型的讀寫路徑 |
| `docs/data-catalog.md` | 完整資料目錄，含 `schema_ref` |
| `internal/domain/` | Go domain types（欄位名稱的 truth source） |
