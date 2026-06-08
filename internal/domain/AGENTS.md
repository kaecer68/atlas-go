# AGENTS.md — internal/domain

`internal/domain` 是系統的型別核心，定義所有跨套件傳遞的 Canonical Types。

**無業務邏輯、無協調邏輯、無持久化實作。**

---

## 本模組特有規範

以下規範為本模組獨有，非重複根 `agents.md`：

1. **零值語義**：數值型別（如 `Score`）若需區分「未設定」與「零分」，請使用指標（`*float64`）。

2. **AgentLayer 對齊**：`AgentLayer` 常數必須與 `configs/agents.json` 中的層級定義嚴格對齊。

3. **擴充欄位同步（Recommendation）**：
   - 若新增欄位到 `Recommendation`，必須同時更新 `RecommendationOutcome`（確保決策鏈透明化所需的 `FactorScores` 與 `ConvictionBreakdown`）。
   - Dashboard 顯示所需的新欄位必須同步更新解析結構。

4. **Scorecard OOS 欄位同步鏈（跨模組）**：
   - `domain.Scorecard` 新增欄位時，必須同步更新：
     1. **`ledger.BuildScorecards()`** — OOS 分離計算邏輯（`window_splitter.go` 的 `Split()`、`sharpeTrendSlope()` helper）
     2. **`internal/monitoring/dashboard_api.go`** — API response 結構（agentObservatoryScorecard 等 mapping 結構）
     3. **`web-ui/js/dashboard.js`** — 前端 OOS 欄位渲染（observatory 表格列）
     4. **`internal/web/field_types.go`** / **`valid_fields.json`** — `go generate` 自動同步
   - 遺漏任一環節會導致 OOS 欄位在 API response 中遺失或前端顯示 `undefined`。

4. **禁止**：
   - 禁止在 domain 中實作任何 I/O 或全域變數修改。
   - 禁止定義資料庫特定標籤（如 `gorm`）。

## 通用規範（請參考根 `agents.md`）

- String Enum 型別、snake_case JSON 序列化等通用慣例，均在根目錄 `agents.md` 的《程式碼慣例》章節中有明確定義，請以該文件為準。
