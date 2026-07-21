# Markdown Reporting Render Contract

> **文件角色**：atlas-go 結構化資料 Markdown 渲染契約。
> **取代對象**：原 internal/reporting/AGENTS.md（已遷移至此）。

本目錄負責將結構化資料渲染為人類可讀的 Markdown 報告。

---

## 職責邊界

- **輸入**：`domain.BacktestWindowSummary`、`[]domain.Scorecard`、ledger 資料
- **輸出**：Markdown 字串（純函數，無 I/O，無副作用）
- **不負責**：檔案寫入、API 回應、資料持久化

## 渲染規範

1. **單一入口**：`RenderMarkdown(data BacktestReportData) string` 是全報告的唯一渲染函式
2. **組合子函式**：
   - `RenderASCIIChart` — 權益曲線 ASCII 圖
   - `RenderAgentPerformanceTable` — Agent 績效表格
   - `RenderMutationSummary` — 變異統計段落
   - `BuildAgentRows` — `[]domain.Scorecard` → `[]AgentPerformanceRow`
3. **純函數原則**：所有渲染函式不接受 `io.Writer`，只回傳 `string`
4. **無外部依賴**：僅使用 `internal/domain`，不使用模板引擎

## 欄位完整性契約

`BacktestWindowSummary` 的每個非零欄位都必須出現在渲染結果中。

測試 `TestRenderMarkdown_CoversAllSummaryFields` 強制驗證此契約。
新增 `BacktestWindowSummary` 欄位時，必須同步更新 `RenderMarkdown` 與該測試。

## 反模式

- **禁止**將 `RenderMarkdown` 改為寫入檔案（副作用）
- **禁止**引入 `text/template` 或 `html/template`
- **禁止**新增中間轉換結構（如第二個 `ReportData`），保持 `BacktestReportData` 為單一輸入型別
