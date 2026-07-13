# CodeGraph — 輕量快速源碼探索

## 觸發條件
- 快速理解一段程式碼（how does X work）
- 需要查看符號的呼叫路徑與 blast radius
- 需要追蹤動態分派（callbacks、React re-render）
- 編輯前想了解影響範圍
- 使用者提到：codegraph, explore, source exploration, quick lookup, call path, blast radius

## 核心能力

CodeGraph 提供 **SQLite 知識圖譜**，透過單一工具 `codegraph_explore` 實現「Read-equivalent」查詢。**一次呼叫同時回傳**：
1. 符號的逐行源碼（等同 `Read`）
2. 符號間的呼叫路徑（含動態分派 hop）
3. blast radius 摘要（誰依賴這個符號）

**唯一工具（也是唯一的 MCP tool）：**
```
codegraph_explore({query: "auth middleware login"})
```

查詢可以是自然語言問題，也可以是符號名稱的集合。支援 `maxFiles` 參數控制回傳檔案數（預設 12）。

## 主要使用場景

| 場景 | 查詢範例 |
|------|---------|
| 理解架構 | `codegraph_explore({query:"how does auth flow work"})` |
| 追蹤 bug | `codegraph_explore({query:"validateToken loginUser"})` |
| 編輯前評估 | `codegraph_explore({query:"RiskGate calibrate"})` — 查看 blast radius |
| 動態分派追蹤 | `codegraph_explore({query:"renderScene mutateElement"})` — 獨有強項 |

## ⚠️ Staleness Banner

若 MCP 回應開頭出現 `⚠️ Some files referenced below were edited since the last index sync…`，表示列出的檔案 pending re-index。**對這些檔案直接 `Read`，不要信任 codegraph 回傳的內容**。未列在 banner 內的檔案仍然可信。

檔案索引由 file watcher daemon 自動維護，延遲約 2 秒。

## 與 codebase-memory 的分工

| CodeGraph 優先 | codebase-memory 優先 |
|---------------|---------------------|
| 輕量快速瀏覽（單次 call） | 深度語意搜尋（`semantic_query`） |
| 動態分派 hop 追蹤 | openCypher 結構化查詢 |
| 單一工具極簡介面 | ADR 管理、複雜度掃描 |
| 編輯前 blast radius | 跨服務資料流追蹤（`trace_path`） |

> 完整工具路由決策樹請見 [`docs/tools.md`](../../../docs/tools.md)。
> **注意**：codebase-memory 與 codegraph 在「單次源碼＋呼叫路徑查詢」有功能重疊 — **此時優先使用 codebase-memory**（Hybrid LSP 型別解析更強，支援 158 語言）。
