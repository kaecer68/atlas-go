# Domain Canonical Types 規格

> **文件角色**：atlas-go 跨模組 Canonical Types 規格（atlas-go 唯一類型核心）。
> **取代對象**：原 internal/domain/AGENTS.md（已遷移至此）。
> **設計權威**：`internal/domain/types.go` 為 source of truth。

`internal/domain` 是系統的型別核心，定義所有跨套件傳遞的 Canonical Types。**無業務邏輯、無協調邏輯、無持久化實作**。

---

## 規範

### 1. 零值語義
數值型別（如 `Score`）若需區分「未設定」與「零分」，請使用指標（`*float64`）。

### 2. AgentLayer 對齊
`AgentLayer` 常數必須與 `configs/agents.json` 中的層級定義嚴格對齊。

### 3. 擴充欄位同步（Recommendation）
- 若新增欄位到 `Recommendation`，必須同時更新 `RecommendationOutcome`（確保決策鏈透明化所需的 `FactorScores` 與 `ConvictionBreakdown`）
- Dashboard 顯示所需的新欄位必須同步更新解析結構

### 4. Scorecard OOS 欄位同步鏈（跨模組）

`domain.Scorecard` 新增欄位時，必須同步更新：

1. **`ledger.BuildScorecards()`** — OOS 分離計算邏輯（`window_splitter.go` 的 `Split()`、`sharpeTrendSlope()` helper）
2. **`internal/monitoring/dashboard_api.go`** — API response 結構（`agentObservatoryScorecard` 等 mapping 結構）
3. **前端 OOS 欄位渲染**（observatory 表格列）
4. **`internal/web/field_types.go`** / **`valid_fields.json`** — `go generate` 自動同步

遺漏任一環節會導致 OOS 欄位在 API response 中遺失或前端顯示 `undefined`。

### 5. 純計算函式的歸屬

跨模組共用的純函式（如 Sharpe、Sortino、Calmar、Calmar-like、波動率等）應放在 `internal/domain/shared/` 子套件，並由各呼叫模組以 type alias re-export 維持向後相容。

範例：`shared.ComputeSharpe(returns, shared.SharpeConfig{Frequency: shared.FrequencyTWSE, RiskFreeRate: 0.015})`。

`portfolio.ComputeSharpe` 為保留 API 的薄 wrapper（type alias），canonical 在 `shared`。若發現新的 duplicate 計算，請合併至 `shared/` 而非保留本地版本。

### 6. CorporateAction canonical 類型

`domain.CorporateAction` 是法人事件（除息、除權、減資）的 canonical 型別。所有跨模組傳遞的法人事件必須用此型別，不得繞道使用 `domain.DividendRecord` 或 marketdata 內部型別。

範例：

```go
ca := domain.CorporateAction{
    Symbol:         "2330",
    ExDate:         time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
    CashDividend:   12.0,
    ReferencePrice: 938.0,
    Source:         "twse_calendar",
}
```

下游消費介面（給 β 工作區的 `AdjustForCorporateActions` 用）：`internal/domain/shared/corporate_action.go` 的 `ActionEffect{Type, ExDate, Adjustment}`，由演算法把 `CorporateAction` 轉成 `ActionEffect`。

資料來源整合層：`internal/marketdata.CorporateActionProvider` interface，首選實作為 `internal/marketdata.AggregatedCorporateActionProvider`（TWSE 為主、FinMind 補缺、Symbol+ExDate 去重）。

JSON tag 為 snake_case；前端型別由 `cmd/gentags` 自動同步（見 `internal/domain/types.go` 的 `go:generate` directive）。

### 7. 禁止

- 禁止在 domain 中實作任何 I/O 或全域變數修改
- 禁止定義資料庫特定標籤（如 `gorm`）

---

## 通用規範

String Enum 型別、snake_case JSON 序列化等通用慣例，均在根目錄 `AGENTS.md` 的《程式碼慣例》章節中明確定義，以該文件為準。
