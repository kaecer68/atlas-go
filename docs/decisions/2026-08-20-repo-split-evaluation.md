# 評估報告：投資策略／投資管線／組合持倉／績效報告／模擬交易 是否拆出獨立 repo

> 評估日期：2026-08-20 ｜ 評估者：prime-agent（L2） ｜ **審計：kimi-k3（k3-256k）— 裁決 PASS WITH CHANGES，本版已依審計意見修正**
> 範圍：五個功能模組的「系統架構 / 業務架構 / 遷移風險」三面向（審計另要求補「數據證據可信度」揭露）
> 方法：ACI 工具（codegraph / gitnexus / codebase-memory / go list 依賴分析）+ 原始碼與資料檔直接計算
> 審計紀錄：k3 逐項以 ACI 重現本報告 27 項事實主張（16 採納 / 9 修正 / 1 駁回），結構性結論全部重現成功；修正內容見 §6 修訂紀錄

---

## 0. TL;DR（結論摘要）

**不建議現在拆成獨立 repo。** 五個功能在程式碼層面是「強連通核心」（互相依賴、共享同一套地基），
拆 repo 等於把「五功能 + 共享層 + API 層 + 前端 + 資料庫 + 排程 + CI + 部署」整包拆出去——
實測五功能的**遞移依賴閉包佔全系統 72%**（52/76 個 internal 模組），不是切一刀，而是搬家。
且業務價值（績效）尚未被驗證，拆出去只是「換一個地方亂」。

但拆分動機本身成立，值得用四個階段逐步兌現：
1. **Phase 0（現在，1–2 週）**：monorepo 內做 bounded context 收斂 + 績效數據審計（先分離「賠錢」是策略問題還是數據問題）；
2. **Phase 1（1–2 個月後）**：邏輯解耦（介面化 + API/資料契約收斂），把五功能收成一個內聚模組樹；
3. **Phase 1.5（選項評估）**：Go multi-module monorepo（同 repo 拆獨立 go.mod，編譯器強制邊界）——比拆 repo 更輕、是 Phase 2 的踏腳石；
4. **Phase 2（條件成熟才做）**：以「獨立服務」試點（先拆 process 不拆 repo），業務驗證過關後才拆 repo。

拆 repo 的正確前提是「商業價值已驗證」（有外部客戶／訂單，或引擎績效證明有 alpha），
而不是「維護成本高」——成本高是現象，拆 repo 不會自動解決耦合，只會把耦合搬到 repo 之間。

---

## 1. 評估範圍與現況盤點

### 1.1 五個功能對應的模組與呈現位置

| 功能 | 後端核心模組 | 前端頁面（shared_web） | API 端點（舉例） |
|---|---|---|---|
| 投資策略 | strategy / strategy_ranker / strategy_techniques / baseline / metalearning | strategies.js（投資心法/策略排名） | /api/strategies*, /api/strategy-ranker/rank |
| 投資管線 | orchestrator / backtest / autobacktest / macroflow / janus / prism / replay | pipeline.js（推薦管線/AI 觀測台） | /api/dashboard/recommendation-pipeline, /api/synergy/darwinian/*, /api/regime/* |
| 組合持倉 | portfolio（Darwinian）/ sectorallocation / retail | portfolio.js（持倉健康/稅務） | /api/dashboard/portfolio-state, /api/dashboard/tax-snapshot |
| 績效報告 | reporting / ledger / dailyreport | performance-report.js（KPI/最佳AI/月度） | /api/dashboard/performance-report(+export), /api/report/* |
| 模擬交易 | sim / live / replay / experiment（評判晉升） | experiments.js（模擬交易=評判/比對/晉升） | /api/experiment/*, /api/traces/sim-latest |

### 1.2 系統總體數字（拆 repo 的真實規模；k3 審計已逐項核實）

| 指標 | 數值（審計核實） | 備註 |
|---|---|---|
| Go packages | 204 個 | ✅ |
| Go 非測試 LOC | ~233K | ✅ |
| internal 模組 | 76 個 | ⚠️ 修正自 77（AGENTS_INDEX.md 已 stale） |
| 資料庫 migration | 17 支（34 個 up/down 檔）／33 張表 | ⚠️ 修正自「34 支/32 表」 |
| HTTP API 端點 | ~194 個（其中五功能相關 37–45，視歸屬口徑） | ✅ 同量級 |
| MCP tools | 117–119 個（其中五功能相關 23 個） | ✅ |
| 排程任務（scheduler 註冊） | 87–91 個 | ⚠️ 修正自 ~97 |
| 前端 | shared_web SPA 同時供 client_web + admin_web | ✅ |
| 運行時 | 單一 binary `atlas -api -live` + 獨立 cron 容器 | ✅ |
| 狀態資料 | data/state 242MB JSONL（recommendation_outcomes 45,661 筆） | ✅ |

---

## 2. 系統架構面向

### 2.1 現況：高度耦合的單體，五功能是「強連通核心」

**核心事實（go list 依賴圖實測）**：

1. **五功能之間互相依賴，形成強連通分量（SCC）**。跨組依賴約 29–31 條有向邊（口徑視 replay
   歸屬 pipeline 或 sim 而定），雙向環全部存在：
   - 投資管線是 hub：管線組 → 策略組 6 條、持倉組 5 條、績效組 4 條、模擬組 2 條（組級口徑）；
   - 反向亦存在：experiment（模擬組）→ orchestrator/replay/portfolio/ledger/sim/baseline；
     reporting（績效組）→ orchestrator/portfolio；live（模擬組）→ orchestrator；sim → portfolio。
   - **任一組都無法單獨抽離**——抽任何一組，其餘四組立即缺依賴。

2. **五功能共享同一套地基**：domain（五組 17 個 package 引用）、config（14）、logging（11）、
   eventbus（8）、marketdata（7）、constants、narrative、risk、industry。核心領域型別
   （Scorecard、SessionSummary、Trade、Position、RecommendationOutcome 等）全部住在
   `internal/domain`——這是全 repo 76 個模組的共用層。

3. **最大單一服務組件就是五功能的對外窗口**：`internal/monitoring` 的 DashboardAPI（31,428 LOC）
   自述「聚合 ledger、narrative、orchestrator 與 sim 的資料並提供 HTTP 介面」。~194 個 API 端點中
   37–45 個服務五功能，且績效報告 handler（`api/performance`）與投資管線 handler（`api/pipeline`）
   在同一個 monitoring 服務樹下註冊。

4. **消費端橫跨全系統**：主程式 `cmd/atlas` 一次 import 全部五組；`internal/monitoring/service`
   全 import；`scheduler` import 四組；`taskexec` import 四組；atlas-mcp server 的 tools_backtest /
   tools_experiment / tools_control / tools_rank 等 23 個 tool 直接呼叫五功能。

### 2.2 拆 repo 的系統性後果

若現在直接拆，需要同時搬走：

- **共享地基的拷貝**：domain / config / eventbus / repository / marketdata 等共享包不是「五功能的」，
  拆出去要嘛複製一份（雙邊 drift 風險），要嘛抽成獨立 library repo（額外一個 repo 要維護）。
- **資料層**：單一 Postgres 33 張表混合五功能與其他功能的表；JSONL state（242MB）同目錄混放。
  拆 repo 前必須先定義跨 repo 的資料契約（誰寫誰讀、schema 誰管、遷移誰跑）。
- **API 與前端**：前端 shared_web 同時服務 client_web（投資者）與 admin_web（營運），五功能頁面
  與其他頁面共用 auth/tier gating/SSE/esbuild bundle；拆 repo 後前端要嘛雙 repo 各自維護 SPA，
  要嘛維持單一前端打兩個後端（跨域/CORS/認證成本）。
- **排程與運行時**：87–91 個排程任務中大量是五功能的（auto_daily_simulation、auto_experiment、
  auto_judge_promoter、auto_strategy_evolution、prism_training、window_backtest、
  factor_weight_calibrate…），目前全在單一 binary 內用 in-process eventbus 溝通；拆服務後要改成
  跨進程通訊。

**量化（k3 審計獨立重現）**：五功能核心模組 LOC 約 53.4K（22.9%），但**遞移依賴閉包
（含所有被五功能直接/間接依賴的模組）實測 168,446 LOC = 全系統 72.2%、52/76 個模組**。
拆 repo 不是抽屜裡拿一個模組，是搬家。

### 2.3 系統架構面向評分

| 項目 | 現況 | 拆 repo 後 |
|---|---|---|
| 模組內聚性 | 五功能本身內聚（同一個投資決策閉環） | 不變（內聚的是功能群，不是 repo 邊界） |
| 耦合度 | 極高（SCC + 共享地基 + 單一 API 層） | 耦合轉移到 repo 之間（API/資料契約） |
| 可獨立部署 | 否 | 理論上可，但需先建立跨服務邊界 |
| 維護成本 | 高（monorepo 大，改動牽扯廣） | **短期更高**（雙 repo + 契約維護 + 雙 CI） |

> 系統架構結論：**拆 repo 在架構上不可行於現狀**（沒有可切的一刀），但「五功能是內聚的 bounded
> context」這個判斷是對的——正確動作是先在 monorepo 內把它收斂成明確邊界，而不是急著拆 repo。

---

## 3. 業務架構面向

### 3.1 現況：五功能在「觀測站」裡的定位矛盾

產品定位（product-positioning.md v1.0）：**給台灣散戶的「模擬交易暨投資策略觀測輔助智慧平台」**。
但盤點後發現關鍵矛盾：

1. **觀測站的門面內容不是五功能**。首頁/主力是七維錢潮雷達、市場總覽、產業地圖、個股快查、
   總經敘事——這些是 capitalflow / marketdata / industry / stocktools / narrative 等**其他**模組。
   五功能（策略→管線→持倉→績效→模擬交易）是背後的「AI 模擬投資引擎」，對公開訪客的
   直接價值感低、可理解性差。

2. **會員制剛上線**（2026-08-20 GUEST_MODE=false 翻轉，commit 977840d3）。tier 分
   public/free/premium/admin；recommender（推薦）是 premium 內容；投資管線/績效報告目前未見
   明確 premium 板塊化 gating（k3 實測 api/pipeline 與 api/performance 無 RequireTier 檢查）。

3. **引擎績效未證明有價值，且證據本身有品質問題（k3 審計重點修正）**：
   - 實測 recommendation_outcomes（45,661 筆）全期平均 forward return +6.08%、勝率 80.1%，
     逐月走勢：2026-05 +17.9%（勝率 85.2%）→ 06 +2.4%（87.7%）→ 07 **−0.56%（35.2%）**；
   - ⚠️ **但 56.0% 的樣本是 is_synthetic=true 合成樣本**（2026-06 更高達 76.7%）：
     - 剔除合成樣本後真實樣本全期 **+12.2%／勝率 80.0%**（比混樣數字好看）；
     - 2026-07 真實樣本仍為 **−0.46%／勝率 47.0%**；真實且過 guards（n=605）**−1.42%／勝率 56.7%**；
     - 2026-05 真實且過 guards 出現 **+35.1%／勝率 100%**——100% 勝率本身是危險訊號
       （疑似 lookahead／survivorship／樣本偏差），數據可信度需質疑；
   - ⚠️ **資料止於 2026-07-16**（檔案 mtime 2026-07-20），距報告日（08-20）已過一個月，
     「目前仍賠錢」的「目前」需以 8 月資料重新確認；
   - 方向穩健性：無論哪種切法（全樣本／真實／真實+guards），2026-07 皆為負，「逐月惡化」結論
     站得住；但**「賠錢」與「數據問題」目前無法分離**——合成樣本混入績效敘事、guard 過濾後的
     樣本太小且異常，Phase 0 的數據審計必須先解決這個問題，否則引擎「自我進化閉環」的判斷基礎
     （Darwinian 權重、baseline 升降級都吃同一份數據）同樣受污染。

### 3.2 拆 repo 對業務的意義（銷售願景評估）

**拆出去當銷售工具商品的邏輯**：
- ✅ 五功能確實是一個完整可打包的「策略模擬 + 組合管理 + 績效報告」引擎，業務邊界比觀測站清晰；
- ✅ 獨立 repo = 獨立版本、獨立計費顆粒度、可對外授權（license）而不洩漏觀測站其他內容；
- ✅ 未來若要賣 B2B（給投顧/教學單位）或 B2C 工具，獨立的 repo/服務邊界是必要條件。

**但銷售前有三個硬前提未滿足**：
1. **績效價值未驗證**：銷售工具的前提是「工具有效」。目前引擎賠錢（至少 7 月為負）、數據缺漏且
   混入大量合成樣本，賣出去等於賣負面口碑。**拆 repo 不會改善績效，只會把賠錢引擎獨立出去。**
2. **產品型態不匹配**：目前的組合持倉/績效報告是「系統級模擬」（整個系統的持倉與績效），
   不是「用戶級」（每個用戶自己的持倉）。要賣給散戶，需要 per-user portfolio 化——
   userstate 模組才剛起步（types.go 僅 42 行），這是比拆 repo 更根本的產品缺口。
3. **⚠️ 法規合規（k3 審計新增，必讀）**：在台灣把「投資策略 + 組合建議 + 績效報告」引擎賣給
   散戶／B2B，直接觸及**證券投資顧問事業相關法規**（投顧執照、自動化投資建議的監管定性、
   績效廣告規範）。這是比「per-user 化」更硬的銷售 gate——Phase 2 拆 repo 之前必須先完成
   法規定性評估，否則拆出來也不能賣。

**拆 repo 對觀測站的傷害**：五功能是「策略觀測」差異化內容（投資心法/策略排名/管線可視化）。
拆出去後觀測站剩純數據看板，差異化下降——除非拆的同時保留觀測站的消費介面（API/前端），
那又回到「拆 repo 但耦合仍在」的局面。

### 3.3 業務架構面向評分

| 項目 | 評估 |
|---|---|
| 業務邊界清晰度 | ✅ 五功能是內聚業務域（投資決策閉環），適合獨立產品化 |
| 價值驗證狀態 | ❌ 未驗證（績效賠錢、數據缺漏、樣本含 56% 合成） |
| 銷售可行性 | ⚠️ 需先通過法規定性 + 產品型態從「系統級模擬」轉「用戶級」 |
| 拆 repo 的業務收益 | 短期無（不改善績效）；中期有（獨立授權/計費） |

> 業務架構結論：**「拆出來賣」的方向合理，但順序錯了**。應該是先驗證績效（含數據品質）→
> 再法規定性 → 再產品化（per-user）→ 最後才談獨立 repo。拆 repo 是銷售的「最後一步」，不是第一步。

---

## 4. 遷移風險面向

### 4.1 風險矩陣（k3 審計調整「團隊認知」為 🟠 中）

| 風險項 | 等級 | 說明 |
|---|---|---|
| 資料一致性 | 🔴 高 | 單一 Postgres（33 表）+ 242MB JSONL 混放；拆 repo 需先定義跨 repo 資料契約與同步機制；recommendation_outcomes 45K 筆歷史不可分割 |
| 運行時邊界 | 🔴 高 | 現在是單進程 in-process eventbus；拆服務需改跨進程通訊（事件/HTTP/queue），orchestrator 的 PluginHost（PRISM/JANUS）與監控層全部要重接 |
| 領域型別共享 | 🔴 高 | Scorecard/SessionSummary/Trade 等核心型別在 internal/domain；拆出去要嘛複製（drift）要嘛抽 library |
| API 契約 | 🟠 中高 | 37–45/194 端點屬五功能，但與 monitoring 共用 auth/tier/audit middleware；拆 API 服務會動到整個認證與監控體系 |
| 前端 | 🟠 中高 | shared_web 被 client_web + admin_web 共用；五功能頁面與其他頁面同 bundle、同 gating |
| MCP | 🟠 中 | atlas-mcp 117–119 tools 含五功能 23 個；拆 repo 後 MCP server 要拆或聚合（影響 Hermes/OpenClaw 整合） |
| CI/品質閘門 | 🟠 中 | 6 道 gate（gofmt/vet/staticcheck/golangci-lint/gosec）+ 60% 覆蓋門檻；拆兩個 repo 要雙份維護 |
| 排程/部署 | 🟠 中 | iMac docker-compose（atlas + cron 容器）；拆服務 = 新容器、DB/網路連線、備份/監控全部調整 |
| 團隊/工具鏈認知 | 🟠 中 | 開發實質由 1 人 + AI agents 驅動；ACI 工具（gitnexus/codegraph/codebase-memory）都是 **per-repo 索引**——拆 repo 後跨 repo 影響面分析直接失效，AGENTS.md 的 ACI-first 規範只對單 repo 成立，跨 repo 語境成本上升 |

### 4.2 風險量化估計

- **直接拆 repo**：保守估計 **3–6 週工程**（含資料契約、共享層、API、前端、部署、CI 全鏈路），
  期間主站持續上線運作（iMac production），回歸風險集中於 DB/API 契約，且**沒有可回滾的中間態**。
- **邏輯解耦先行**：1–2 週即可完成 bounded context 收斂 + 績效數據審計，風險低、可增量驗證。
- **關鍵不對稱**：拆 repo 是「大成本、不可逆、價值未驗證」；不拆是「低成本、可逆、可逐步逼近」。

---

## 5. 綜合評估與建議

### 5.1 三面向總結

| 面向 | 現況 | 拆 repo 合適度 |
|---|---|---|
| 系統架構 | 強連通單體，無乾淨切點（依賴閉包 72%） | ❌ 不適合（拆不動） |
| 業務架構 | 業務邊界清晰但價值未驗證、法規未定性、產品型態未就緒 | ⚠️ 方向對、時機不對 |
| 遷移風險 | 資料/運行時/領域層三重高風險，無中間態 | ❌ 風險報酬不對稱 |

**整體結論：現在不拆。** 五功能適合「先收斂、後驗證、再獨立」，不適合「現在就拆」。

### 5.2 建議路線圖

**Phase 0 — 立即（1–2 週）：收斂與驗證（不拆 repo）**
1. 在 monorepo 內把五功能收斂為明確 bounded context（如 `internal/investment/` 聚合層或明確的
   package boundary 文件），先讓「五功能的邊界」在程式碼與文件上可見；
2. 做一次**績效數據審計（升級版）**：先回答「合成樣本（56%）是否該進績效敘事」，再驗證
   recommendation_outcomes / darwinian / baseline 的數據品質（缺漏率、計算口徑、guard 過濾後
   樣本規模、2026-05 勝率 100% 的異常成因、7 月中斷至今的缺口），並補 8 月資料；
3. 用審計結果決定：引擎的自我進化閉環是否要調整（例如 stop-loss、regime 過濾、樣本門檻、
   合成樣本標記），而不是急著換 repo。

**Phase 1 — 1–2 個月後：邏輯解耦（仍不拆 repo）**
4. 五功能的對外介面（API handler / repository interface / MCP tool）收斂到單一契約層，
   內部實作可自由重構而不影響其他模組；
5. 前端維持同一 SPA，只收斂後端路由邊界；DB schema 標記五功能 table 的歸屬與 owner；
6. 建立「五功能」的獨立發布版本號與 changelog（monorepo 內即可做）。

**Phase 1.5 — 選項評估：Go multi-module monorepo（k3 審計新增建議）**
7. 把五功能切成獨立 `go.mod` module、但留在同一 repo。好處：不拆 repo 就強制邊界
   （跨 module import 顯性化、雙向依賴直接編譯失敗）、獨立版本號、天然是 Phase 2 拆 repo 的踏腳石。
   成本：go.work 設定、CI 調整、初期有重構噪音。**建議在 Phase 1 完成後正式評估此選項**——
   若 multi-module 能滿足需求，就不必走到 Phase 2。

**Phase 2 — 條件成熟才拆（repo / 服務）**
拆的觸發條件（**全部**滿足才啟動）：
- ✅ 有第一個外部客戶／銷售訂單（價值被市場驗證）；或引擎績效證明 alpha；**且**
- ✅ 績效定義量化：連續 **N≥3 個月** alpha > 0，基準 = 加權報酬指數，**扣除交易成本與滑價後**，
  且通過 Phase 0 數據品質門檻（合成樣本剔除、樣本規模、guard 口徑明文化）；**且**
- ✅ **法規定性完成**：投顧法規評估確認可銷售（或已取得相應資格/已排除適用）；**且**
- ✅ 觀測站與五功能的發布節奏/團隊確實分離（例如五功能要獨立 SLA）。

拆的順序建議：**資料契約 → 領域層 → API 服務 → 前端 → 排程 → CI/部署**，
並以「獨立服務（新 repo 內跑、共用資料庫）」作為過渡，最後才切資料庫。

### 5.3 給業主的三個關鍵問題（決策前必答）

1. **拆 repo 想要解決的根本問題是什麼？** 是維護成本、績效可信度、還是銷售可行性？
   三者解法不同：維護成本→解耦（不一定要拆 repo）；績效→數據審計+策略調整（拆 repo 無關）；
   銷售→法規定性+產品化+驗證（拆 repo 是最後一步）。
2. **如果拆出去後績效還是賠錢，這個 repo 要賣給誰？** 銷售價值必須建立在績效或數據價值上；
   且先確認賣「投資建議工具」是否需要投顧資格。
3. **觀測站失去五功能後，差異化內容剩下什麼？** 需先規劃觀測站與五功能之間的「消費契約」
   （API/前端保留），否則拆 repo 會同時傷害觀測站。

---

## 6. k3 審計紀錄與修訂對照

> 審計員：kimi-k3（kimi-coding/k3-256k）｜ 裁決：**PASS WITH CHANGES**（核心結論全部重現成功，
> 修正項如下）。審計意見書完整版存於 /tmp/repo_split_audit_opinion.md。

| # | 審計意見 | 處理 | 本版狀態 |
|---|---|---|---|
| 1 | §3.1 未揭露樣本組成（56% is_synthetic、資料止於 07-16、真實樣本口徑數字、05 月 100% 勝率異常） | 已補 §3.1.3 完整揭露與質疑 | ✅ 已修 |
| 2 | 法規/合規面向完全缺席（投顧法規 gate） | 已補 §3.2.3 + Phase 2 觸發條件 | ✅ 已修 |
| 3 | 事實錯誤：模組 77→76、migration 34→17/33 表、taskexec 3→4 組、experiment 組別標籤、跨組邊 25→29–31、作者數、端點 50→37–45、排程 97→87–91 | 全部修正（見 §1.2 / §2.1 / §4.1） | ✅ 已修 |
| 4 | 建議 Phase 1→2 之間插入 multi-module monorepo 評估 | 已補 Phase 1.5 | ✅ 已修 |
| 5 | Phase 2 alpha 觸發條件量化（N≥3、基準、成本口徑） | 已補 §5.2 Phase 2 | ✅ 已修 |
| 6 | 風險矩陣「團隊認知」升為 🟠 中（ACI per-repo 索引失效） | 已補 §4.1 末列 | ✅ 已修 |

---

## 7. 附錄：關鍵證據索引

| 證據 | 位置 |
|---|---|
| 產品定位（散戶/模擬交易/觀測） | docs/reference/product-positioning.md |
| DashboardAPI 為最大單一服務組件 | internal/monitoring/AGENTS.md |
| 五功能跨組依賴 ~29–31 條（SCC） | go list 依賴圖（本報告 §2.1；k3 重現） |
| 依賴閉包 72.2%（168,446 LOC / 52 模組） | go list 遞移閉包（k3 獨立驗證，§2.2） |
| 共享地基引用數 | go list 依賴圖（domain 17 / config 14 / logging 11 / eventbus 8 / marketdata 7） |
| 消費端（cmd/atlas、monitoring、scheduler、taskexec） | go list 依賴圖（§2.1.4） |
| 會員制翻轉（GUEST_MODE=false） | git log 977840d3；docs/specs/guest-mode-spec.md |
| 會員 gating 三原則 | docs/guides/membership-gating.md |
| tier 分類（public/free/premium/admin） | docs/operations/tier-boundary.md |
| 績效數據（45,661 筆；56% 合成；07 月轉負；止於 07-16） | data/state/recommendation_outcomes.jsonl（本報告 §3.1，k3 重算核實） |
| 部署（單 binary + cron 容器） | docker-compose.yml；docs/operations/docker-compose.crons.yml |
| 排程任務（87–91） | internal/scheduler + cmd/atlas |
| MCP tools（117–119，含五功能 23） | docs/reference/tool-catalog.md；cmd/atlas-mcp/server/tools_* |
