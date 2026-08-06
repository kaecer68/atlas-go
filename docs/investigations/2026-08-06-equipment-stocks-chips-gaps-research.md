# 2026-08-06 三家設備股 scope 盤查與外部調研

> 接續 `2026-08-06-equipment-stocks-chips-gaps.md` 的根因報告。
> 本文件盤查 docs/系統層級是否明確聲明「不涵蓋上櫃」、前端/MCP/handler 是否有合理告知，
> 並對「是否值得納入上櫃 3131 弘塑 / 3587 閎康 / 6187 萬潤」做外部調研。

## 1. 文件體系中的「範圍」聲明 — 全部缺席

| 應有聲明的檔 | 現況 | 缺口 |
|---|---|---|
| `docs/specs/stock-api-contract-spec.md` | §3 寫「chips 直接傳給 TWSE T86（純數字）」**沒有顯式聲明系統 scope 等於上市** | ❌ 沒寫「TWSE = 上市，不含上櫃」 |
| `docs/specs/atlas-mcp-ranker-design-spec.md` | 表格 chips/fundamentals 來源 = TWSE T86、Yahoo | ❌ 沒寫覆蓋範圍 |
| `internal/apigateway/CONSTITUTION.md` | 治理「外部數據源抓取」、但全文無 scope/symbol coverage 字眼 | ❌ 未約束資料源 = TWSE 的範圍 |
| `docs/data-sources.md` | 提到 TPEX「Use for: ...」，但 TPEX chips 不在 atlas 4 個 stocktools endpoint 範圍 | ⚠️ 提到但實際未落地 |
| `README.md` / 入口頁 | 沒看到「atlas 目前涵蓋上市普通股 1070 隻」字眼 | ❌ 沒寫 |
| `cmd/atlas-mcp/auto-desc.gen.json` MCP tool description | `stock_get_chips`、`stock_get_fundamentals`、`stock_get_quote`、`stock_get_technical` 共 4 個 tool description 沒有「TWSE 上市」範圍說明 | ❌ |

**結論**：整個正式文件體系（含 PR-level spec 與 v1.4 治理憲章）都**沒有「chips/fundamentals 不涵蓋上櫃」**的明確聲明。Leak of intent → 任何接手者、CLI 操作者、MCP caller 都不知道查詢為何失敗。

### 建議修補位置（不含在此 PR 內）

1. `docs/specs/stock-api-contract-spec.md`：新增 §1.5「Coverage Scope」段
2. `docs/reference/glossary.md` 或 `README.md`：單獨寫明「涵蓋 TWSE 上市普通股 1070 隻，含 ETF 部分；不涵蓋上櫃/興櫃；技術指標需 quote store 含 K 線」
3. `cmd/atlas-mcp/auto-desc.gen.json` 4 個 tool description：補一句「Coverage: TWSE-listed common stocks only」
4. `internal/apigateway/CONSTITUTION.md` §3：補資料源權威性時區分「哪些 source 包含上櫃 / 哪些不包含」

## 2. 前端 UI 範圍告知 — 完全沒有

| 檔 | 現況 |
|---|---|
| `shared_web/static/js/page-shells/stock-quote.js` | template 中「個股快查」標題 + 3 個 sq-section（基本面/籌碼/技術指標）+ 搜尋框；**0 字上櫃/exchange/範圍告知** |
| `shared_web/static/js/components/stock-quote-chips.js` | `renderChips` error 狀態顯「籌碼資料當日無更新，請稍後再試」——將 503 context.Canceled 摺疊為「當日無更新」誤導訊息 |
| `shared_web/static/js/components/stock-quote-fundamentals.js` | `renderFundamentals` error 顯「基本面資料暫時無法取得」、empty 顯「無基本面資料」——沒有「資料源未涵蓋」徽章 |

### 建議修補（亦不在本 PR 內）

- `stock-quote.js` template 在搜尋框下方加一行小灰字：「本系統涵蓋台灣上市股票；上櫃股票的部分資料可能無法顯示」
- `renderChips` state==='error' 改為：「此 symbol 不在本系統資料來源範圍內（chips 涵蓋 TWSE 上市，請改輸入上市股票代號）」
- `renderFundamentals` 區塊在 PE/PB 全為 0 時，加 visual badge：「snapshot 不涵蓋此 symbol（TWSE 上市 1070 隻）」

## 3. atlas-mcp 範圍告知 — 完全沒有

`cmd/atlas-mcp/server/tools_stock.go::handleStockGet{Chips,Fundamentals,Quote,Technical}`（共 4 個 handler）：

- 全部都是 `if symbol=="" return error ; cli.Get(...) ; if err return err` ——**完全不做 scope 預檢**
- `auto-desc.gen.json` 4 個 tool description 沒寫 TWSE 範圍
- 使用 MCP 查 3131 chips 拿到 `"error":"context canceled"` 文字，與前端看到的一樣

### 建議修補（亦不在本 PR 內）

- 在 4 個 handler 前加 `if isLikelyOTC(symbol)` → 把上櫃代號前綴（4xxxx, 6xxxx, 8xxxx 高機率）早拒，回 `ErrNotCovered` + 明確文字
- 或更穩：在 MCP layer 加 symbol cover precheck endpoint `/api/stock/coverage?symbol=X` 統一判斷
- `auto-desc.gen.json` 補 range hint

## 4. handler 行為模式矩陣

| Symbol 性質 | quote | chips | fundamentals | technical |
|---|---|---|---|---|
| .TW snapshot 內 (1070 隻) | 200 OK Fugle/TWSE | 200 OK (5–10s) | 200 OK (PE/PB 等) | 200 OK (QuoteStore 或 Fugle 補) |
| 上櫃 (任何 .TWO) | 200 OK Fugle | **503 context.Canceled** | **200 OK 全 0** | **200 OK** (若 QuoteStore 有) |
| snapshot 外上市 | 200 OK Fugle | 200 OK (若 T86 有) | 200 OK 全 0 | 200 OK |
| 不存在 / 下市 | 200 OK Fugle (若 list 內)<br/>404 TWSE (若完全無) | 503 context.Canceled | 200 OK 全 0 | 200 OK (可能空) |

**沒有任何一條回「out-of-scope」**——對使用者體驗而言：上櫃與不存在的差別只在 chips 是否 503，看起來都像「資料有問題」。

## 5. 精選範圍與 1070 隻關係

- 沒有「精選 100 隻」白名單設定檔。你印象可能記錯。
- `configs/agents.json` 每個 agent 自帶 `universe[]` list（個別設定 4–30 隻），是 simulation 範圍，**不是 stocktools 4 個 endpoint 的存取白名單**。
- `configs/parameters/smart_universe.json::top_n=150` 是 simulation universe 預設上限，與 stocktools 完全脫鉤。
- `data/fundamentals.json` 共 **1070 隻 = T86 上市普通股全集（純 .TW，**不含 ETF、不含 9 隻下市/特別股）**：
  - T86 raw 含 1231 隻上市普通股 + 116 隻 ETF/權證 = 1331 列
  - snapshot 不含 ETF（170 隻）
  - snapshot 不含 9 隻邊緣股（1213 1538 2321 2380 3593 4943 6225 6806 8101）

所以實際 stocktools 「合理可用 symbol 集合」可以視為這 1070 隻，但 stocktools 沒有宣告這個範圍。

## 6. 3131 弘塑 / 3587 閎康 vs 精選 100 名單

**已在 `agents.json` 全檔掃描**：3131 弘塑、3587 閎康**完全不在任何 agent universe** 內。

**結論**：
- 3131、3587 不屬於 snapshot、不屬於 agent universe、不屬於 T86 1331 列（這是因為 snapshot 與 T86 都只包上市，上櫃不在）
- 不標記為 bug——屬於「系統不涵蓋上櫃」scope 邊界，與使用者確認前提一致
- 唯一 **bug 級**問題：handler 的錯誤訊息沒有說明範圍 → 建議獨立 PR 修

## 7. 是否值得納入上櫃：弘塑 / 閎康 / 萬潤

### 你原始輸入為「3131 弘塑、3587 萬潤」

網路查證結果：
- **3131 弘塑**：弘塑科技，半導體濕製程設備，上櫃（2011/01/17 上櫃）
- **3587** 正確對應是 **閎康**，不是萬潤。**「萬潤」股票代號為 6187**。
- **「萬潤」**實為 6187，上櫃，半導體自動化設備

→ 你口頭輸入的「3587 萬潤」是口誤，**真正對應到 3587 的是閎康**。這份調研以你的原文輸入為準，調研的是 3131（弘塑）與 3587（閎康），同時為了完整性一併附上萬潤 6187。

### 3131 弘塑（半導體濕製程設備）

- **基本面**：2025 年全年營收 65.14 億（YoY +59.93%）；2026 Q1 EPS 16.11；法人估 2026 EPS 63.35 元、YoY +39%
- **目標價**：1800 → 2200（2026/03 調升）
- **產業地位**：「先進封裝濕製程設備龍頭」、「SoIC 濕製程設備 ASP 150–200 萬美元遠高於 CoWoS 的 100–150 萬美元」
- **客戶集中**：台積電 CoWoS / SoIC 供應鏈核心
- **2027 能見度**：✅（moneydj 報導法人稱 2027 LEAP 營收突破 35 億美元）
- **媒體能見度**：TVBS 財經、半導體六強、台積電 CoPoS / CPO 概念股熱點

### 3587 閎康（半導體材料分析）

- **基本面**：2026/03 單月營收 5.38 億創歷史新高（YoY +21.27%）；2026 Q1 累計 14.24 億（YoY +15%）
- **2025 全年 EPS**：6.1 元
- **產業地位**：「半導體檢測分析實驗室」、「CoWoS 封裝良率生殺大權」、「2nm GAA、矽光子檢測實驗室龍頭」
- **AI 趨勢受惠**：2nm、先進封裝、矽光子全部吃
- **媒體能見度**：Yahoo 財經、cnyes、investing 投資檢討個案

### 6187 萬潤（半導體自動化設備）

- **基本面**：2025 年股價區間 600–1100，目前約 1070 元附近（媒體報導數字為股價非營收）
- **產業地位**：CoWoS / SoIC 自動化設備、與辛耘（3583）、弘塑（3131）並列半導體設備六強
- **媒體能見度**：高盛重新定價、CoPoS 概念股、TVBS 財經

### 是否值得納入？

站在 atlas 半導體設備策略深度而言：

| 維度 | 弘塑 | 閎康 | 萬潤 |
|---|---|---|---|
| AI 受惠度 | 🟢 高（SoIC/CoWoS 濕製程） | 🟢 高（2nm/矽光子分析） | 🟢 中（自動化） |
| 與 1070 隻現有清單同類標的 | 與 3131 同類 — 弘塑即同檔 | 與 6669 廣穎、3017 奇鋐 不同類，但與 6641 基士德（環保）更不同 — 屬半導體 capex cycle | 與 3583 辛耘 同類 — 萬潤即同類 |
| 對 strategy_ranker 排名影響 | 改變半導體設備 L2 排名 | 加強 analysis lab 新發現 | 與現有清單重疊 |
| 2026 媒體能見度 | 🟢 極高 | 🟢 高 | 🟢 高 |
| 業務體質 | 客戶 TSMC 集中度高 | 客戶分散（含聯電、GlobalFoundries） | 客戶 TSMC 集中 |

**但**：
- 三家全為**上櫃**——意味著要納入，需要 **TPEX 對應資料源**：TPEX chips provider（`/web/stock/3insti/dailyTrade` 之類）、TPEX 三大法人 data、TPEX 個股 quote 等等
- 量級約等於擴大 1 個新 provider + snapshot 改 `key suffix 接受 .TWO` + agent universe 重組 + 4 個 stocktools handler 加 TPEX 分支
- 屬於 v2 範疇——當下 PO scope 是「不涵蓋上櫃」

**建議**（待使用者裁示）：
1. **最短路徑**：現有 scope 內，改善告知 UX（§1–3）+ 不擴大
2. **擴大路徑**：分階段納入 TPEX 上櫃（先 chips、後 fundamentals）——獨立 spec、獨立 PR cycle

**若決策為「擴大」**，建議**優先納 3587 閎康**，理由：
- 屬 material analysis 新維度（atlas 現有半導體設備股都是 WLP/CAPEX 設備類），補一個新維度
- 月營收 YoY +21%、Q1 +15% 顯示 growth 屬性明確
- 媒體覆蓋高、研究報告多 → 易做 LLM 標註
- 客戶分散度比弘塑/萬潤佳（不只押 TSMC）

若使用者堅持納弘塑或萬潤，建議先認清楚：兩者客戶集中度 = 與現有清單高度重疊（只會改變 L2 排名權重而非結構性新增維度）。
