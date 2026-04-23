# AGENTS.md — internal/domain

本目錄定義 `atlas-go` 的**規範型別 (Canonical Types)**。所有跨套件傳遞的領域物件均在此宣告，確保系統各層級對資料格式有一致的認知。

---

## OVERVIEW

`internal/domain` 是系統的型別核心，不包含業務邏輯、協調邏輯或持久化實作。
- **職責**：定義 DTO、領域實體、Enum 常數與 JSON 序列化標籤。
- **依賴規範**：禁止依賴其他 `internal/*` 套件，僅能依賴標準庫（如 `time`）。

---

## TYPE PATTERNS

### String Enums (Simulation-First)
為了確保 JSON 序列化（`ledger` 持久化）的可讀性與穩定性，所有 Enum 均使用 **string 型別**，禁止使用 `iota`。

```go
type Regime string

const (
    RegimeRiskOn  Regime = "RISK_ON"
    RegimeRiskOff Regime = "RISK_OFF"
    RegimeNeutral Regime = "NEUTRAL"
)
```

- **適用對象**：`Regime`、`Side`、`AgentLayer`、`ExperimentStatus` 等。

### JSON Serialization
本專案與前端（Dashboard）及持久化層（JSONL）通訊時，一律使用 **snake_case**。

- **規範**：Struct 欄位必須明確標註 `json:"field_name"`。
- **Omitempty**：對於選填或 breakdown 欄位（如 `FactorScores` 中的 `Breakdown`），請加上 `,omitempty` 以維持日誌簡潔。

---

## CONVENTIONS

1. **零值語義**：對於數值型別（如 `Score`），請考慮零值是否具有特定語義。若需要區分「未設定」與「零分」，請使用指標。
2. **層級對齊**：`AgentLayer` 常數必須與 `configs/agents.json` 中的層級定義嚴格對齊。
3. **擴充欄位**：
    - `Recommendation` 與 `RecommendationOutcome` 應包含決策鏈透明化所需的 `FactorScores` 與 `ConvictionBreakdown`。
    - 所有新增到 `Recommendation` 的欄位，若需在 Dashboard 顯示，必須同時更新 `RecommendationOutcome` 與 `api/dashboard` 的解析結構。
4. **禁止**：
    - 禁止在此套件實作任何帶有副作用（I/O、全域變數修改）的方法。
    - 禁止在此定義與特定資料庫（如 PostgreSQL）強耦合的標籤（如 `gorm`），除非該型別僅用於該用途。
