# codebase-memory — 深度圖分析與語意搜尋

## 觸發條件
- 新增功能前，想確認是否已有語意相似的實作
- 需要執行 openCypher 查詢做結構化分析
- 需要追蹤跨服務資料流（`trace_path`）
- 需要管理 ADR（Architecture Decision Records）
- 需要複雜度熱點掃描
- 使用者提到：semantic search, Cypher, graph analysis, ADR, trace_path, Louvain cluster

## 核心能力

codebase-memory 提供 **SQLite-backed 知識圖譜**，開放 **openCypher 查詢語言**與**向量語意搜尋**。

**常用工具：**
| 場景 | 工具 | 說明 |
|------|------|------|
| 語意查詢 | `codebase-memory_search_graph({semantic_query:[...]})` | 模糊搜尋，找語意相關的符號 |
| 結構化查詢 | `codebase-memory_cypher({query:"MATCH ..."})` | openCypher，精確查詢節點與邊 |
| 跨服務資料流 | `codebase-memory_trace_path({from, to})` | 追蹤從 A 到 B 的完整呼叫鏈 |
| ADR 管理 | `codebase-memory_manage_adr()` | 架構決策記錄 |
| 複雜度掃描 | `codebase-memory_scan_complexity()` | 找出高複雜度熱點 |
| 索引狀態 | `codebase-memory_list_projects()` | 列出已索引的專案及節點統計 |

## 何時用這個 vs 其他工具

| 場景 | 優先工具 | 理由 |
|------|---------|------|
| 快速看一眼程式碼 | **CodeGraph** | 單次 call 拿源碼＋呼叫路徑，最快 |
| 改 code 前評估風險 | **GitNexus** | `impact()` 回傳 CRITICAL/HIGH 警告 |
| 語意模糊搜尋 | **codebase-memory** ⬅️ 這個 | 唯一支援 `semantic_query` |
| openCypher 結構查詢 | **codebase-memory** ⬅️ 這個 | 唯一支援 Cypher |
| ADR / 複雜度分析 | **codebase-memory** ⬅️ 這個 | 獨佔功能 |
| 動態分派追蹤 | **CodeGraph** | 唯一支援 callbacks/re-render hop |

> 完整工具路由決策樹請見 [`docs/TOOLS.md`](../../../docs/TOOLS.md)。
