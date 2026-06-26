# 開發工具指南 — 程式碼知識圖譜

> 本文件介紹 atlas-go 專案的兩套程式碼智慧工具：**GitNexus** + **codebase-memory**。
>
> 適用對象：需要理解專案架構、進行重構、或做複雜度分析的**人類開發者**與 **AI Agent**。

---

## 雙工具總覽

atlas-go 專案同時被兩個 MCP 索引，並提供互補能力：

| 工具 | MCP 名稱 | 索引名稱 | 節點 / 邊 | 獨特能力 |
|------|---------|---------|----------|---------|
| **GitNexus** | `gitnexus` | `atlas-go` | 53,385 symbols / 169,008 edges / 300 execution flows | 執行流（Process）、功能社群（Community）、API 路由映射、影響範圍分級、安全重命名 |
| **codebase-memory** | `codebase-memory` | `Users-kaecer-workspace-atlas` | 29,757 nodes / 127,367 edges | 開放 Cypher 查詢、向量語意搜尋、Leiden 叢集偵測、ADR 管理、跨服務資料流追蹤 |

> **上述數字為 2026-06-25 歷史快照**。由於 `scripts/verify-gitnexus-stats.sh` 已於 2026-06 移除（no-op），這些數字不再被 CI 自動驗證。**需要 live 數字時，請手動執行** `npx gitnexus status`（GitNexus）或 `codebase-memory_list_projects()`（codebase-memory）。

**為什麼要兩個？**
- **GitNexus** 強在「**process + community**」抽象與 PR 安全閘（impact / detect_changes / rename）。
- **codebase-memory** 強在「**Cypher + 語意搜尋 + Leiden 叢集**」的圖分析能力。
- 兩者互補：複雜度熱點掃描、跨模組統計分析 → codebase-memory；改動前 blast radius、跨檔案重命名 → GitNexus。

---

## 為什麼 atlas 高度依賴 Process 抽象

### atlas 的架構特質：深層跨模組執行流

atlas-go 是**管線化多層路由**架構。一個「下單決策」的生命週期跨越 5-8 個模組：

```
MarketData 拉取 → Orchestrator 路由 → RegimeExecutor 判定市場狀態
  → SectorAgent 選股 → CRO 風控過濾 → CIO 部位過濾
    → Simulator 模擬執行 → Ledger 寫入結果
```

任何一個模組出問題（例如 CRO 的 PreTradeGate 拒絕了某 symbol），根因可能在「上游資料源異常」或「RegimeExecutor 判斷牛熊有誤」，而不是 CRO 本身。**只讀單一模組的程式碼無法還原全貌**。

codebase-memory 的 `trace_path()` 可以從單一符號出發，沿 CALLS 邊逐層追蹤。但問題在於：

| 追蹤方式 | codebase-memory `trace_path` | GitNexus `query` |
|---------|------------------------------|------------------|
| 模式 | 手動選起點 → 逐跳遍歷 | 預先計算 300 條 named Process |
| 跨模組 | 支援，但需指定 `mode:"cross_service"` | 自動標記 community、step_index、process_type |
| 抽象層 | 無（純符號呼叫圖） | **有** — 每個 Process 有 label、entry/terminal point、community 歸屬 |
| 發現成本 | 高（需先知道起點符號名稱） | 低（自然語言一句話） |

**關鍵差異**：codebase-memory 給你**「誰呼叫誰」的拼圖碎片**。GitNexus 給你**「這條路徑在系統中扮演什麼角色」的完整地圖**。

### atlas 中的典型 Process 場景

GitNexus 已將 atlas 的常見跨模組流程預先計算為 300 個 Process。以下是與日常開發最相關的幾類：

| Process 類型 | 例子 | 涉及模組 | 為何需要抽象層 |
|-------------|------|---------|-------------|
| 風控閘門 | PreTradeGate 觸發 → HALT/REDUCE → EventBus 廣播 → Dashboard 顯示 | `risk`, `eventbus`, `sim`, `monitoring` | 跨 4 模組，單一模組 crash 會造成靜默失敗 |
| 實驗評判 | mutation → execute → judge → accept/reject → promote baseline | `experiment`, `orchestrator`, `sim`, `baseline` | 涉及 state machine 轉換，需追蹤閘門條件 |
| 資料管道 | TWSE/FinMind/Fugle → adapter → apigateway → marketdata provider → orchestrator | `marketdata`, `apigateway`, `orchestrator` | 多 provider fallback 鏈，需知道 data_status 傳遞路徑 |
| 演進循環 | 最弱 agent 識別 → mutation brief → LLM 生成新 prompt → 實驗對照 → 升級/降級 | `orchestrator`, `experiment`, `baseline`, `llm` | LLM hot-path 繞過規則強制在此生效 |

### 實際開發中的反模式

```text
❌ 反模式：改了 risk/gate.go 的 PreTradeCheck，只跑 risk/ 的測試就合併
   → simulator 端 filterByPreTradeGate 依賴 PreTradeCheck 的回傳值
   → 改動造成 simulator 靜默跳過所有 symbol，無報錯

✅ 正確做法：gitnexus_impact({target:"PreTradeCheck", direction:"upstream"})
   → 顯示 PreTradeCheck → filterByPreTradeGate → Engine.WithPreTradeGate
   → 知道 Simulation 執行流會被影響 → 跑 risk + sim 兩個模組的測試
```

這就是 `gitnexus_impact` 在 AGENTS.md 被列為**強制步驟**的原因——atlas 的模組耦合是透過隱式資料流（eventbus、session date、DataClass 級別），而非顯式 import，只有 Process 抽象能捕捉。

### Process vs Community 的互補關係

GitNexus 的雙抽象層彼此獨立但互補：

- **Community**（Leiden 叢集）回答「哪些程式碼屬於同一個功能區塊」— 靜態結構分組。
- **Process**（執行流）回答「這些功能區塊如何協作完成一件事」— 動態行為路徑。

對 atlas 來說，Community 幫你理解「`PreTradeGate` 和 `EventBus` 同屬 Risk 模組」，Process 幫你理解「HALT 訊號如何從 `PreTradeGate.ruleVaRLimit` → `EventBus.Publish` → `Dashboard` SSE 推送」。**後者才是日常 debug 和改 code 時最需要的資訊。**

### 何時用 Process、何時用手動追蹤

| 場景 | 用 GitNexus Process | 用 codebase-memory trace_path |
|------|-------------------|-------------------------------|
| 「下單決策的完整路徑是什麼？」 | ✅ `query({query:"order execution"})` | ❌ 需要逐層手動追 |
| 「這個 function 的 callers 有哪些？」 | `context({name})` | `trace_path({direction:"inbound"})` — 兩者都可 |
| 「`publish` 相關的函式有哪些？」 | `query({query:"event publish"})` | `search_graph({semantic_query:["publish"]})` — 兩者都可 |
| 「從 `ServeHTTP` 到 `PreTradeCheck` 的中間層？」 | ✅ `query` → 直接拿到完整 step list | 需要 `trace_path` 逐跳拼接 |
| 「哪些 Process 會被我的改動破壞？」 | ✅ `detect_changes()` — 獨有 | ❌ 無對等指令 |
| 「這個資料欄位從哪裡來、經過哪些 transform？」 | ❌ 無 data_flow 模式 | ✅ `trace_path({mode:"data_flow"})` |

---

## 重複偵測與孤兒 Code 分類

### 為什麼 atlas 需要這個機制

atlas-go 有 34 個 `internal/` 模組、1,200+ 社群、300 條執行流。多模組並行開發時，最容易出現的 AI 失誤不是「改壞既有 code」，而是**「新增了已經存在的功能」** — 兩個模組各自實作了相似的邏輯，但沒有人知道彼此存在。

典型場景：

| 場景 | 例子 | 後果 |
|------|------|------|
| 跨模組重複實作 | `risk/` 有 PreTradeGate、`live/` 也有 RiskGate，兩者都做 VaR 檢查 | 修改一個忘記改另一個 → 行為不一致 |
| 語意相近但無關 | `eventbus/` publish、`monitoring/` emit — 名稱不同但做類似的事 | 新人無法判斷「哪個才是正確的」 |
| AI 盲目新增 | 「幫我在 orchestrator 加一個 validation」→ 但 apigateway 早就有了 | 程式碼膨脹、維護成本上升 |
| 孤兒 code 誤刪 | `experiment/` 某 function 無 caller → 刪掉 → 原來是 config-driven（`agents.json` 引用） | regression |

### Step-by-step 工作流程（AI Agent 專用）

#### 新增前：重疊檢查（強制）

```
1. 用自然語言描述你打算做什麼
   → gitnexus_query({query: "<你的意圖>", task_context: "我要新增一個 <功能>"})
   → 如果返回任何執行流，代表已有類似實作 — 先讀、再決定是要擴充還是確實不同

2. 用語意向量搜尋確認沒有「名稱不同但功能相同」的實作
   → codebase-memory_search_graph({semantic_query: ["<關鍵詞1>", "<關鍵詞2>", "<關鍵詞3>"]})
   → 例如要加「check position size limit」：
     semantic_query: ["position", "size", "limit", "check", "cap"]
   → 即使現有 code 命名完全不同（如 `ruleVaRLimit`），語意搜尋也能找到

3. 如果兩個工具都返回 EMPTY：新領域 → 放心實作，commit message 標註 "new ground"
4. 如果任一工具返回 HITS：
   → 先讀取重疊的 code
   → 判斷：你的功能是它的子集？超集？平行替代？
   → 在 PR/commit 中說明為什麼不復用既有 code
```

#### 刪除前：孤兒 Code 分類（強制）

找到「無 caller」的 code 時，**不可直接刪除**。必須先分類：

| 分類 | 定義 | 判斷方法 | 處理 |
|------|------|---------|------|
| **未完成** | 功能實作一半、尚未有 caller 接入 | `git log --oneline -5 -- <file>` 看最近的 commit 是否還在開發中 | **保留**，新增 TODO 標記 |
| **已取代** | 舊版功能有新版替代方案 | `gitnexus_query({query})` → 找到替代方案的完整執行流 | **可刪**，但必須在 commit 中記錄替代品路徑 |
| **意外斷連** | 本該有 caller，但改動造成斷線 | `gitnexus_impact({target, direction:"upstream"})` → 檢查是否有曾經的 caller 被改掉 | **修復**連線，而非刪除 |
| **組態驅動** | 透過 `agents.json`、`parameters.json`、plugin registry 動態載入 | `grep -r "symbol_name" configs/ prompts/` | **不是死碼** — 不可刪 |

```
完整刪除檢查清單（來自 atlas-pre-change-protocol Step 7）：
□ gitnexus_impact({target: "<symbol>", direction: "upstream"})
□ git log --oneline -5 -- <file>
□ grep -r "symbol_name" --include="*.go" internal/
□ grep -r "symbol_name" configs/ prompts/
□ 檢查 interface satisfaction（同 package 內）
□ 如確定刪除：在 module AGENTS.md 記錄原因與替代品
```

### 兩個工具在重複偵測上的分工

| 任務 | GitNexus | codebase-memory |
|------|---------|----------------|
| 概念層重疊（「有沒有人做過風控？」） | ✅ `query` — 執行流排名 | `search_graph` — BM25 排名 |
| 語意層重疊（「limit check」vs「VaR cap」） | ❌ keyword only | ✅ `semantic_query` 跨詞彙橋接 |
| 簽名層重疊（相同參數、相同回傳） | 間接（context 比對） | `search_code({pattern})` + Cypher |
| 模組層重疊（「這應該放哪個 package？」） | `query` → 看 community 歸屬 | `get_architecture()` → Leiden 叢集 |
| 孤兒分類：是否組態驅動 | ❌ 無 config 感知 | ❌ 無 config 感知 → 需手動 `grep` |

> **核心原則**：GitNexus 回答「有沒有類似的執行流」；codebase-memory 回答「有沒有名稱不同但語意相同的實作」。兩者都要跑才算完整檢查。

---

## GitNexus

### 核心能力

GitNexus 具備「執行流（Process）」與「功能社群（Community）」雙重抽象層。它能回答「這段程式碼在系統中扮演什麼角色」，而不只是「被誰呼叫」。

**索引範圍：** 完整程式碼庫的 symbols、relationships、communities、execution flows（執行 `npx gitnexus status` 查看即時統計）。

### 常用指令

```bash
# 重建索引（修改大量程式碼後執行；使用 --skip-agents-md 避免注入 markdown 區塊）
npx gitnexus analyze --skip-agents-md

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
| 改函式前評估風險 | `gitnexus_impact()` | 查看直接呼叫者、受影響的執行流、風險等級（LOW/MEDIUM/HIGH/CRITICAL） |
| 理解系統運作 | `gitnexus_query()` | 自然語言查詢，BM25 + 向量 + RRF 混合排名，返回執行流與相關符號 |
| 追 bug 根因 | `gitnexus_trace()` | 追蹤從 A 到 B 的完整呼叫鏈 |
| 跨模組重構 | `gitnexus_rename()` | 安全改名，理解呼叫圖（圖 + regex 雙路徑） |
| PR 前檢查 | `gitnexus_detect_changes()` | 確認變更只影響預期的符號和執行流 |
| API 路由映射 | `gitnexus_route_map()` | 列出哪些 handler 服務哪些 route |
| Response shape 比對 | `gitnexus_shape_check()` | 偵測 consumer 存取 route 沒回應的欄位 |
| 多倉分析 | `gitnexus_query({repo:"@<group>"})` | 跨 monorepo group 查詢 |

### 資源入口

- `gitnexus://repo/atlas-go/context` — 專案總覽
- `gitnexus://repo/atlas-go/clusters` — 所有功能社群
- `gitnexus://repo/atlas-go/processes` — 所有執行流
- `gitnexus://repo/atlas-go/process/{name}` — 單一執行流逐步追蹤

### 唯一擁有（GitNexus 獨佔）

- **`rename`** — 跨檔案安全改名（圖 + regex 雙路徑，自動標記 confidence）
- **`detect_changes`** — pre-commit blast radius（diff → 符號 → 受影響 process）
- **`route_map`** / **`shape_check`** — API 路由 consumer 映射 + response shape mismatch 偵測
- **`tool_map`** — MCP/RPC tool 定義與 handler 對應
- **group mode** — `@<group>` 跨 monorepo 查詢
- **UID-based 零歧義查詢**（`target_uid`）— 多個同名符號時不需猜測

---

## codebase-memory

### 核心能力

codebase-memory 提供 SQLite-backed 知識圖譜，開放 **openCypher 查詢語言**與**向量語意搜尋**。它的強項是「任意切片分析」與「語意模糊查詢」。

**節點統計（atlas-go，2026-06-25）：** 29,757 nodes / 127,367 edges / 92.7 MB

### 常用指令

```bash
# 查看已索引專案清單
codebase-memory_list_projects()

# 索引（或重建索引）當前專案
codebase-memory_index_repository({repo_path: ".", mode: "full"})

# 自然語言查詢（BM25 + 向量混合）
codebase-memory_search_graph({query: "risk gate pre-trade check"})

# 語意模糊查詢（多關鍵字 AND）
codebase-memory_search_graph({
  semantic_query: ["publish", "send", "event"]
})

# 開放 Cypher 查詢（任意複雜度）
codebase-memory_query_graph({
  query: "MATCH (f:Function) WHERE f.cyclomatic >= 10
          RETURN f.qualified_name, f.cyclomatic ORDER BY f.cyclomatic DESC"
})

# 取得 Leiden 叢集架構
codebase-memory_get_architecture()

# 單一符號 360° 視角
codebase-memory_explore({query: "RiskManager VaR95"})

# ADR 管理
codebase-memory_manage_adr({mode: "get"})
codebase-memory_manage_adr({mode: "update", content: "..."})
```

### 使用場景

| 場景 | 指令 | 說明 |
|------|------|------|
| 複雜度熱點掃描 | `query_graph({query:"MATCH (f:Function) WHERE f.cyclomatic >= 10 ..."})` | 一次性掃所有 hot path（含 nested loop depth、linear scan in loop） |
| 架構叢集偵測 | `get_architecture()` | Leiden 演算法自動標註 de-facto 模組邊界 |
| 語意模糊搜尋 | `search_graph({semantic_query:[...]})` | 「send」找「publish」、跨詞彙橋接 |
| 路徑追蹤 | `trace_path({function_name, direction, mode:"data_flow"})` | 呼叫鏈 + 參數傳遞追蹤 |
| ADR 管理 | `manage_adr({mode})` | 架構決策紀錄 CRUD |
| 跨服務資料流 | `trace_path({mode:"cross_service"})` | 透過 HTTP / async 跨 service 追蹤 |
| 程式碼全文搜尋 | `search_code({pattern, regex:true})` | grep + graph 雙重增強 |

### 唯一擁有（codebase-memory 獨佔）

- **openCypher 查詢語言** — 任意切片分析（complexity、cognitive、loop_count、transitive_loop_depth、linear_scan_in_loop、alloc_in_loop、param_count、max_access_depth）
- **語意向量搜尋** — 多關鍵字 AND 過濾、跨詞彙橋接
- **Leiden 社群偵測** — 自動計算 de-facto 模組邊界（含 cohesion 分數、代表性 top_nodes）
- **ADR 管理** — `manage_adr({mode})` 整合
- **跨服務追蹤** — HTTP / Channel / async 多跳追蹤
- **BM25 ranking score** — 結構感知加權（Function +10 / Route +8 / Class +5）
- **穩定性** — SQLite-backed、讀取 sub-millisecond、檔案監聽 ~1s 延遲

---

## 路由決策樹

```
需要改動符號？
├─ 是 → 改前評估 blast radius ─→ GitNexus `impact()` + `detect_changes()`
│                              └─ 跨檔案改名 ─→ GitNexus `rename()`
└─ 否 → 純查詢？
         ├─ 自然語言概念 → GitNexus `query()`（有執行流排名）
         ├─ 模糊詞彙橋接 → codebase-memory `semantic_query`
         ├─ 統計/複雜度分析 → codebase-memory `query_graph()` Cypher
         ├─ 架構叢集 → codebase-memory `get_architecture()`
         ├─ API 路由 → GitNexus `route_map()` / `shape_check()`
         ├─ ADR → codebase-memory `manage_adr()`
         └─ 跨倉分析 → GitNexus `query({repo:"@<group>"})`
```

---

## 索引更新時機

| 工具 | 更新時機 | 指令 |
|------|---------|------|
| GitNexus | 大規模重構後、PR 合併前 | `npx gitnexus analyze --skip-agents-md` |
| codebase-memory | 大規模重構後、模組新增時 | `codebase-memory_index_repository({repo_path, mode:"full"})` |

> 兩者皆有檔案監聽，~1s 內自動同步小改動。**重大改動後建議手動重建**。

---

## 設定驗證

### GitNexus

```bash
# 確認 MCP 已配置
npx gitnexus status

# 列出已索引專案
gitnexus_list_repos()
# 期望看到 atlas-go (53,385 nodes / 169,008 edges / 300 processes)
```

### codebase-memory

```bash
# 確認 MCP 已配置並列出專案
codebase-memory_list_projects()
# 期望看到 Users-kaecer-workspace-atlas (29,757 nodes / 127,367 edges, 92.7 MB)

# 確認索引可用
codebase-memory_get_graph_schema({project: "Users-kaecer-workspace-atlas"})
# 期望返回 node labels 與 edge types 清單
```

---

## 已知限制

### GitNexus

- 索引延遲檔案寫入 ~1s
- 跨檔案解析以最佳化名稱匹配為主；歧義呼叫可能返回多個候選
- 無 query language（僅自然語言 + UID 零歧義）

### codebase-memory

- Cypher 查詢有 100k row ceiling；大範圍查詢需在 Cypher 加 `LIMIT` 或用 `search_graph + offset/limit` 分頁
- 跨檔案名稱匹配為 best-effort
- 預設模式（`fast`）跳過相似度/semantic 邊；向量搜尋需 `moderate`/`full` 模式

---

## 相關文件

- `AGENTS.md` — AI 工具使用規則（AI 專用速查表）
- `CLAUDE.md` — GitNexus 完整規範與工具使用準則（Always Do / Never Do）
- `internal/AGENTS_INDEX.md` — 模組索引與成熟度
- `docs/architecture.md` — 系統架構詳細說明

> 註：`scripts/verify-gitnexus-stats.sh` 已於 2026-06 移除（no-op script，從未檢查到任何 doc 中的 pattern）。GitNexus 索引大小請直接用 `npx gitnexus status` 查詢。