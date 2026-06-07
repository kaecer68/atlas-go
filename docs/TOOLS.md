# 開發工具指南

> 本文件比較 atlas-go 專案的三個程式碼知識圖譜工具：GitNexus、CodeGraph、Graphify。
> 
> 適用對象：需要理解專案架構或進行重構的**人類開發者**。

---

## 工具總覽

| 工具 | 類型 | 最佳用途 | 索引方式 | 更新成本 |
|------|------|---------|---------|---------|
| **GitNexus** | 知識圖譜 + 執行流分析 | 改程式碼前的影響範圍評估 | `npx gitnexus analyze` | 中（需手動重建） |
| **CodeGraph** | 輕量符號呼叫圖 | 快速查詢函式呼叫鏈 | 自動（`.codegraph/` 目錄） | 低（檔案變更時自動更新） |
| **Graphify** | 視覺化互動圖譜 | 新進人員理解專案架構 | `graphify update .` | 低（AST-only，無 API 成本） |

---

## GitNexus

### 核心能力

GitNexus 是唯一具備「執行流（Process）」與「功能社群（Community）」雙重抽象層的工具。它能回答「這段程式碼在系統中扮演什麼角色」，而不只是「被誰呼叫」。

**節點統計（atlas-go）：** 39,417 節點、119,534 關係、300 個執行流、1,036 個社群

### 常用指令

```bash
# 重建索引（修改大量程式碼後執行）
npx gitnexus analyze

# 查詢特定概念相關的執行流
gitnexus_query({query: "auth validation logic"})

# 修改前的影響範圍分析（**強制執行**）
gitnexus_impact({target: "validateUser", direction: "upstream"})

# 檢查變更影響的執行流
gitnexus_detect_changes()
```

### 使用場景

| 場景 | 指令 | 說明 |
|------|------|------|
| 改函式前評估風險 | `gitnexus_impact()` | 查看直接呼叫者、受影響的執行流、風險等級 |
| 理解系統運作 | `gitnexus_query()` | 自然語言查詢，返回執行流與相關符號 |
| 追 bug 根因 | `gitnexus_trace()` | 追蹤從 A 到 B 的完整呼叫鏈 |
| 跨模組重構 | `gitnexus_rename()` | 安全改名，理解呼叫圖 |
| PR 前檢查 | `gitnexus_detect_changes()` | 確認變更只影響預期的符號和執行流 |

### 資源入口

- `gitnexus://repo/atlas-go/context` — 專案總覽
- `gitnexus://repo/atlas-go/clusters` — 所有功能社群
- `gitnexus://repo/atlas-go/processes` — 所有執行流

---

## CodeGraph

### 核心能力

CodeGraph 是輕量級符號呼叫圖，適合快速查詢「誰呼叫了這個函式」或「這個函式呼叫了誰」。

**節點統計（atlas-go）：** 16,305 節點、40,849 邊

### 常用指令

```bash
# 檢查索引狀態
codegraph_status

# 搜尋符號
codegraph_search("validateUser")

# 查詢函式的呼叫者
codegraph_callers("ValidateUser")

# 查詢函式呼叫了誰
codegraph_callees("ValidateUser")

# 追蹤從 A 到 B 的呼叫路徑
codegraph_trace("handleRequest", "execute_sql")

# 理解特定任務的相關程式碼
codegraph_context("JWT auth implementation")
```

### 使用場景

| 場景 | 指令 | 說明 |
|------|------|------|
| 快速找函式定義 | `codegraph_search()` | 比 grep 更精確，理解語法結構 |
| 追蹤呼叫鏈 | `codegraph_trace()` | 從 API handler 追到資料庫查詢 |
| 理解模組邊界 | `codegraph_context()` | 獲取任務相關的入口點與關鍵程式碼 |

---

## Graphify

### 核心能力

Graphify 產生**互動式 HTML 視覺化圖譜**，最適合新進人員理解專案架構，或製作報告時使用。

**節點統計（atlas-go）：** 10,812 節點、30,921 邊、200 個社群

### 常用指令

```bash
# 更新知識圖譜（AST-only，無 API 成本）
graphify update .

# 產生互動式 HTML 報告
graphify

# 更新子圖譜（將大圖切成 4 個小於 700 節點的子圖譜）
bash scripts/regenerate-subgraphs.sh
```

### 子圖譜系統

由於完整圖譜過大（>10,000 節點），我們將其切成 4 個主題子圖譜：

| 子圖譜 | 內容 |
|--------|------|
| `core` | 核心架構與協調層 |
| `analysis` | 分析與評估模組 |
| `research` | 研究與實驗模組 |
| `infra` | 基礎設施與資料層 |

**導覽入口：** `graphify-out/subgraphs/index.html`

### 使用場景

| 場景 | 說明 |
|------|------|
| 新進人員 onboard | 開啟 `graphify-out/subgraphs/index.html`，視覺化理解模組關係 |
| 架構報告 | 產生 HTML 嵌入簡報 |
| 程式碼審查 | 視覺化顯示變更影響的社群 |

---

## 三工具決策矩陣

| 情境 | 推薦工具 | 原因 |
|------|---------|------|
| 改程式碼前評估風險 | **GitNexus** | 唯一有 impact analysis 和 process 抽象 |
| 快速查詢函式呼叫鏈 | **CodeGraph** | 輕量、即時、適合單一查詢 |
| 理解系統整體架構 | **Graphify** | 視覺化最直覺 |
| 追蹤 bug 根因 | **GitNexus** 或 **CodeGraph** | GitNexus 有完整執行流，CodeGraph 有精確追蹤 |
| 跨模組重構 | **GitNexus** | 安全改名 + 影響範圍 |
| 新進人員 onboard | **Graphify** → **GitNexus** | 先看全貌，再細查 |
| 自動化 CI 檢查 | **GitNexus** | `detect_changes()` 可整合進 pipeline |

---

## 維護腳本

### 更新 Graphify 子圖譜

```bash
# 執行 graphify update + 切片腳本
bash scripts/regenerate-subgraphs.sh
```

此腳本會：
1. 執行 `graphify update .` 更新主圖譜
2. 執行 `python3 scripts/slice-graph.py` 將主圖切成 4 個子圖譜
3. 每個子圖譜 < 700 節點，可正常產生互動式 HTML

---

## 索引更新時機

| 工具 | 更新時機 | 指令 |
|------|---------|------|
| GitNexus | 大規模重構後、PR 合併前 | `npx gitnexus analyze` |
| CodeGraph | 自動更新（檔案變更時） | 無需手動操作 |
| Graphify | 新增模組後、報告前 | `graphify update .` |

---

## 相關文件

- `AGENTS.md` — AI 工具使用規則（AI 專用）
- `internal/AGENTS_INDEX.md` — 模組索引與成熟度
- `docs/architecture.md` — 系統架構詳細說明
