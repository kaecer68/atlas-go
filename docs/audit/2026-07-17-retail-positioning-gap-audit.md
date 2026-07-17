# 散戶定位與錢潮鏈路缺口審計 — 2026-07-17

> **審計動機**：業主重新校準產品定位——台灣**散戶**的「模擬交易暨投資策略觀測輔助平台」；機器自動優先（自動排程/糾錯/進化）、網頁為主、atlas-mcp 為輔；核心假設是台股由外資主導、與美元/美股連動、電子偏科、有春節/選舉週期慣性，系統應觀測三大法人（擴充為六~七類資金勢力）客觀數值變化以預測「錢潮」流向，幫散戶找機會。
> **方法**：6 個唯讀探索代理（文件定位/資料通道/錢潮鏈/策略鏈/UI/MCP 排程自癒）+ atlas-mcp 活體端點驗證 + 台灣市場資料權威來源事實查核。
> **範圍聲明**：本審計不重複列報 2026-07-15（13 缺口）與 2026-07-16（21 項 manifest）已結案項目，但會追蹤其「修復後仍殘留」的部分。

---

## 一、執行摘要

**一句話**：觀測層（資料→七大勢力→壓力指數→季節性）相當完整且串聯；但業主因果鏈的三個「預測」動詞——**外部因子推估外資、錢潮預測的誤差自修正、產業/個股預測交給策略層**——目前分別是**不存在、半殘、斷裂**。系統停在「展示給人看」，尚未「餵給策略用」；而 UI 端散戶最需要的「策略競爭狀況」恰好完全無出口。

| 業主問題 | 判定 |
|---|---|
| 定位是否清晰表達？ | ⚠️ 部分。散戶 persona、網頁優先、台灣市場前提假設均未完整成文 |
| 系統設計與方向相輔相成？ | ⚠️ 觀測層是；預測/傳導/策略消費層否 |
| 能構成預測錢潮的條件？ | ❌ 目前是規則式評分 + 無誤差閉環；最領先指標（外資期貨 OI）零覆蓋 |
| 產業短期漲跌預測？ | ⚠️ 有骨架（SectorPredictor，規則式），無驗證、不進策略層 |
| 後端連動無斷裂？ | ❌ 至少 10 處明確斷點（見 §四） |
| UI 合乎投資人需要？ | ⚠️ 大體良好且白話文案是強項；缺策略競爭、互動模擬、agent 解說入口、歷史趨勢圖 |

---

## 二、定位表述審計（文件面）

| 業主主張 | 判定 | 說明 |
|---|---|---|
| 目標用戶=散戶 | 部分成文 | 只有「零售投資人/投資人」（`docs/design.md:14-19`）；「散戶」在 docs 中皆指被觀測的資料對象（RSI-tw、七力之一），非用戶 persona |
| 模擬交易+觀測輔助 | 已成文 | `README.md:3`「simulation-first」；但 live 路徑亦成文（雙重 opt-in），表述需留意 |
| 機器自動優先三機制 | 部分成文 | 統一陳述只在 `docs/archive/2026-06-26-warmup-auto-evolution-design.md:18-23`（已歸檔）；現行文件散見各機制 |
| 台灣市場特殊性前提 | 部分成文 | `docs/llm-integration-strategy-framework.md:387-400` §3.1a 列六項慣性（美元依賴/美股連動/外匯操控/外貿依賴/單一產業集中/淺碟效應），但自我限定為「LLM 評估框架而非設計輸入」；春節/選舉在事件模型 |
| 網頁優先、MCP 為輔 | **未成文** | 現行文為雙軌並列；`docs/investor/README.md:3` 投資人入門完全以 MCP 安裝為路徑，`README.md:24-35` 以 MCP 為對外賣點 |
| 端到端藍圖 | 已成文（工程視角） | `docs/architecture.md:84-133` Wave 11 框架；缺面向投資人的一頁式「錢潮故事線」 |
| 七大資金勢力定義 | 部分成文 | 列舉只在已歸檔 manifest 與 `internal/capitalflow/forces.go:17-19`；「為何加入期貨/TSM ADR/政府」的分類學未成文 |

**建議**：補一份一頁式產品定位文件（建議 `docs/reference/product-positioning.md`），內容：散戶 persona（懂 AI 的散戶 vs 一般散戶的區隔）、模擬+觀測定位、機器優先三機制、網頁優先/MCP 為輔、台灣市場六項前提假設、七大資金勢力分類學。這是後續所有設計取捨的仲裁依據。

---

## 三、目標因果鏈 vs 系統現況（核心對照）

```
[1] 外部因子（美元利率/美股/美債/日圓/原油）
      │  資料：✅ 齊（Yahoo macro 5 分鐘：DXY/^TNX/VIX/WTI/JPY/USDTWD/SPX/NDX/SOX/NVDA/TSM）
      ▼  傳導模型：❌ 不存在 —— 只進 stress index（外資是輸入非預測目標）、MacroRiskAssessment（風控燈號）、macroflow（只讀 VIX≥35）
[2] 推估外資動向
      │  現貨：✅ T86 每日；期貨 OI：❌ 零覆蓋（Futures force 是回傳 0 的 placeholder，forces.go:44-50）
      ▼
[3] 六大~七大資金勢力共振
      │  ✅ 存在但 2/7 死（futures、government 恆 0）→ 共振係數 1.5/0.5 是邏輯不可達的死碼，production 恆 1.0
      ▼
[4] 錢潮觀測/預測/修正
      │  觀測 ✅（Z-score + 品質分 + UI）；預測 ⚠️ 規則式（事件權重+tilt→sigmoid，活體 5 天預測複製貼上）；
      │  修正 ❌（drift alert 拿 [0,1] confidence 比 [-3,3] QualityScore，單位不同且只告警；backtest 只跑過 synthetic）
      ▼
[5] 產業/週期/供應鏈 → 標的
      │  產業配置 ✅ 但孤立（WeightEngine 策略層零呼叫）；板塊 5 日方向 ⚠️ 規則式；
      │  春節 ✅ 有校準機制（但校準後 acc=0.4）；選舉 ❌ 零統計（config 自標 todo）；
      │  供應鏈擴散 ✅；美股事件→台股個股傳導 ❌；個股 forecast ❌ stub（永遠 hold/50）
      ▼
[6] 漲跌趨勢預測 + 策略競爭
      │  L1-L5 心法庫 ⚠️ 靜態種子、hit_rate 無回饋；策略排名 ⚠️ 只吃靜態 hit_rate；
      │  策略競爭引擎 ❌ 空心（ComparisonEngine.Record 無生產呼叫端）；
      │  真閉環在 agent 層達爾文權重 ✅（20 agents 每日自動調權）
      ▼
[7] 散戶機會 → 網頁呈現
      │  推薦 ⚠️ registered/premium 內容寫死，與排名零串聯；free tier 活體回 stress_index_unavailable
      ▼
[8] UI：覆蓋良好；缺策略競爭出口、互動模擬、agent 解說入口、歷史趨勢圖
```

### 方法論評語（金融工程觀點）

1. **七大勢力分類混雜「主體」與「代理」**。外資/投信/自營/官股/散戶是主體；期貨 OI 與 TSM ADR 是外資行為的**載體/情緒代理**，與 foreign 高度共線。目前 TSM ADR 以獨立勢力計入動態權重（依 |raw| 占比），活體觀測其權重 30%、外資 0%——單一股票 ADR 主導「資金共振」在方法論上站不住。建議：主體維持 5 類；期貨 OI 降格為外資的**領先維度**、TSM ADR 降格為外部情緒特徵，共振模型才有意義。
2. **「外部因子→外資」是可建模的**（外資台股流向與 DXY/美中利差/VIX/TWD 的關係有充分實務依據），且系統已有 `WeightCalibrationEngine`（用歷史 macro+flow 算各因子對實際外資流向的命中率）可當起點——把它從「校 stress 權重」擴展為「產出外資方向/機率預估」是最短路徑。
3. **預測要可信的最低三要件**（非數學炫技）：領先指標資料（外資期貨 OI）、預測 vs 實際的同單位誤差追蹤、誤差回饋校準。目前三者皆缺/半殘。

---

## 四、斷點與缺口總表

### P0：已損壞 / 資料正確性（1-2 週）

| # | 問題 | 證據 |
|---|---|---|
| P0-1 | MCP `strategy_ranker` array→map 解碼失敗（與已修的 scheduler_get_status 同模式漏修；測試 mock 錯誤助長漏網）；建議全面掃 `*map[string]any` decode 的 handler | `cmd/atlas-mcp/server/tools_strategy_ranker.go:20`、`tools_strategy.go:35-37` vs `internal/strategy_ranker/handler.go:46,50` |
| P0-2 | MCP `trace_get_reasoning` 永遠 400（tool 無 session_id 參數，後端必填） | `tools_llm_trace.go:101-109` vs `reasoning_handler.go:43` |
| P0-3 | autobacktest 脆弱鏈：每小時空轉洗掉 consecutive_failures 且無 last_error；overlap 守門毫秒誤殺（每天唯一 13:30 tick 可能被吃）；replay 停滯時 snapshot_exists_skip 靜默；last_auto_date 無 staleness 告警 | `internal/scheduler/background.go:277-325`、`internal/autobacktest/runner.go:50-53` |
| P0-4 | Darwinian history 寫 CWD 相對路徑 → trend API 讀停更 7 週的 repo-root 舊檔 | `internal/orchestrator/composition.go:237-238` vs `internal/monitoring/service/pipeline.go:914` |
| P0-5 | 實驗迴路堵塞：628 筆 planned 積壓（每週僅消化 1）、最後 promote 2026-04-25、AutoProposer 從未接線、晉升時 pre-promotion Sharpe 恆記 0.0 使 AutoRollback 退化偵測失效 | `data/state/experiments.jsonl`、`internal/experiment/auto_proposer.go:31`、`auto_judge_promoter.go:195-198` |
| P0-6 | replay 缺資料時 synthetic outcomes 餵達爾文權重（假數據污染進化依據，RecordOutcome 不區分 IsSynthetic） | `internal/orchestrator/system.go:532-534,783-826` |
| P0-7 | `daily_report` 與 `capital_flow_summary` 同日口径矛盾（偏多/中度流入 vs 強勁流出）；`daily_report.global.summary`「偏寬鬆」與 status RISK_OFF 語意矛盾 | 活體 2026-07-16 實測 |
| P0-8 | `get_recommendations` free tier 活體回 `stress_index_unavailable; capital_flow_unavailable` 且零推薦——推薦管線走了另一條失效資料路徑 | 活體實測 vs `internal/recommender/handler.go:151-172` |
| P0-9 | 前端壞出口：首頁「查看決策鏈」跳 404；portfolio 死按鈕；散戶版 HTML 殘留 admin「提示詞比對/晉升 Baseline」modal；pipeline 場次下拉永遠載入中；premium 升級鈕未實作 | `shared_web/static/js/pages/home.js:1036`、`client_web/static/index.html:166-181` 等 |
| P0-10 | `mcp_quickstart` 5 個子請求全吞錯（部分失敗靜默回 null）；`system_get_health` status 欄位漂移（後端無此欄位）；`llms.txt` 過時（79-81 vs 實際 110 tools）；tool-catalog 稱 experiment_judge 為 LLM 評審（實為統計式） | `tools_briefing.go:29-33`、`tools.go:274` vs `system.go:60-71`、`tool-catalog.md:99` |

### P1：因果鏈斷點（2-6 週）

| # | 缺口 | 說明 |
|---|---|---|
| P1-1 | **外資台指期未平倉通道**（TAIFEX 三大法人，盤後） | 外資動向最領先的公開指標，目前零覆蓋；接上後 Futures force 才有生命、共振 1.5/0.5 才不是死碼。TAIFEX 每日盤後公布，有公開查詢頁 |
| P1-2 | **「外部因子→外資推估」傳導模型** | 業主因果鏈第一哩。以 WeightCalibrationEngine 為基礎，產出外資方向/機率預估；先簡單（logistic/計分卡）再迭代 |
| P1-3 | **預測誤差閉環** | 統一 drift 比較單位（都轉成方向命中/信心）；prediction_backtest 接真實 90 天資料（目前 blocked 於 loader）；誤差回饋校準 predictor 的 0.3 tilt/門檻/事件 BaseWeight |
| P1-4 | **預測進策略層** | orchestrator 消費 eventdriven 預測與 sector allocation；`portfolio/sector_rotator.go:40` 的 TODO 接上 WeightEngine |
| P1-5 | **推薦接真排名** | recommender 消費 strategy_ranker/ComparisonEngine；`ComparisonEngine.Record()` 接線（策略競爭才有實數）；registered/premium 內容去寫死 |
| P1-6 | 集保股權分散（週頻）+ 借券賣出餘額（日頻）通道 | 大戶/散戶持股分級與對沖/放空訊號；皆官方免費資料 |
| P1-7 | 選舉行情校準（config todo 兌現）+ `year_end_rally` 校準值 46% 異常複查 | `configs/parameters.json:3813`；`cmd/calibrate-seasonal` 產出 |
| P1-8 | 策略競爭 UI（darwinian status/trend + agent observatory）+ 三大法人歷史趨勢圖 | 業主明確要「策略競爭狀況」；後端 API 都在，純前端工作 |
| P1-9 | Government force 代理設計 | 公股行庫無直接公開日資料，需以八大行庫券商分點或新聞事件推估——先定義方法論再接線，否則維持 0 並在 UI 明示 |
| P1-10 | 七大勢力分類重構（見 §三方法論評語 1） | 期貨/TSM ADR 降格為維度/特徵，主體 5 類 |

### P2：深化（6-12 週）

| # | 缺口 | 說明 |
|---|---|---|
| P2-1 | 券商分點資料 → 主力成本線、分點集中度 | 散戶圈最看重的籌碼訊號；資料需購買或評估合法抓取，先做成本/合規評估 |
| P2-2 | 投信「被迫買入」模型 | 基金/ETF 規模變化（投信投顧公會每日公布）→ 成分股被迫買盤估計；搭便車訊號 |
| P2-3 | 美股事件→台股供應鏈個股傳導（NVDA 財報事件源） | 供應鏈圖已有，缺事件型觸發 |
| P2-4 | 散戶 explainer compose 型 MCP tool（「今天為什麼跌」）+ 站內 agent 解說入口（擴展 strategies 頁 AI 歸因模式） | 「懂 AI 的散戶」區隔的關鍵體驗 |
| P2-5 | 互動式模擬交易（散戶自行下單的 paper trading） | 目前只能唯讀看 AI 投組 |
| P2-6 | 端到端鏈路活檢 synthetic probe（資料→策略→推薦全鏈路活性探測） | 目前只有被動狀態讀取 |
| P2-7 | TPEx 上櫃三大法人/融資券 | 櫃買市場全缺 |
| P2-8 | EPFR 類外資流向（付費，評估）+ 個別 ETF 申贖/折溢價 | 目前只有全市場合計淨申購且無排程 |

---

## 五、事實查核（業主提供的 Kimi 參考資料 + 台灣市場前提）

### 5.1 Kimi 參考資料勘誤

| Kimi 主張 | 查核結果 |
|---|---|
| 信號 1「TWSE OpenAPI 每 5 分鐘三大法人」 | ❌ **官方無盤中法人資料**。三大法人買賣超為盤後公布（約 14:30 後）；盤中僅券商系統的分點即時買賣超可當領先代理（[參考](https://cevisub.com/technical-analysis-guide/net-selling-meaning-taiwan-stocks-guide/)） |
| 信號 8「集保戶數每月公布」 | ❌ **實為每週**（每週最後營業日結束後公布股權分散表）（[iT邦](https://ithelp.ithome.com.tw/m/articles/10293719)、[wistock](https://blog.wistock.ai/wistalk/free-official-taiwan-stocks-investor-data-guide/)） |
| 信號 4 外資期貨未平倉 T+0 盤後 | ✅ 正確，期交所每日盤後公布（[TAIFEX](https://www.taifex.com.tw/cht/3/futContractsDate)） |
| TWAP/VWAP/冰山單/收盤競價/期貨對沖/借券對沖等執行方式 | ✅ 概念正確（業界標準執行演算法），可作為「痕跡辨識」教學素材 |
| 「連買 3 天雜訊/5 天信號/10 天趨勢」「50 億門檻」「20,000 口」「±3% 成本線」等具體數字 | ⚠️ 屬經驗法則，無權威背書。**正確用法：當作待校準假設**，用本系統既有的校準框架（calibrate-seasonal、WeightCalibrationEngine 模式）從 TWSE 歷史驗證後再寫入 parameters.json——這正是「自動進化」哲學的用武之地，而非硬編碼 |
| 投信「被迫買入」（申購潮→持續性非自願買盤） | ✅ 概念正確且可操作（基金規模每日公開），已列入 P2-2 |
| 「察覺後有 5-10 天安全窗口」 | ⚠️ 方向合理但屬機率性，非保證；應以回測呈現「歷史上連買 N 日後 M 日延續機率」而非寫死天數 |

### 5.2 台灣市場前提的權威支撐（業主假設大體成立）

- **台積電佔大盤約 38-43%、電子股佔市值約 70-75%**（[央行報告轉引](https://www.vcoinforo.com/gu-shi-zi-xun/5985.html)、[玉山銀](https://www.esunbank.com/zh-tw/-/media/New-ESUNBANK/Marketing/marketing-sector/wealth/vip_lecture/251218-2.pdf)、[群益權值計算](https://stock.capital.com.tw/z/zm/zmd/zmdb.djhtm)）→「電子偏科、台積電領軍」成立
- **春節行情**：近 20 年開紅盤日上漲機率 71-75%（[永豐金](https://securities.sinopac.com/seNews/20240126120441320000000000000011.html)、[口袋證券](https://www.pocket.tw/school/report/perspective/5090/)）→ 有統計支撐；但注意本系統校準後春節 pattern 對「產業層級」的預測力僅 0.4——**大盤層級的紅包行情 ≠ 產業選股有效**，兩者要分開表述
- **選舉行情**：證據混合。地方選舉選前 40-20 交易日上漲機率 86%、選後 40 日 86%（[富邦](https://www.fubon.com/financialholdings/news/news_1260703_047075.htm)）；但總統大選前後報酬不一致、國際情勢影響更大（[MacroMicro](https://www.macromicro.me/blog/quantitative-report-does-the-election-market-exist-a-look-at-past-election-trends)、[Money101](https://www.money101.com.tw/blog/%E9%81%B8%E8%88%89%E8%A1%8C%E6%83%85-%E5%8F%B0%E8%82%A1)）→ 應區分地方/總統層級建模，目前系統選舉規則完全未校準（P1-7）
- **外資主導**：外資持有台股市值約四成、台積電個股外資持股逾七成，淺碟市場下外資進出對指數影響放大——前提成立，這也讓 P1-1（外資期貨 OI）與 P1-2（外資推估模型）成為最高槓桿缺口

---

## 六、系統健康度（活體 2026-07-16）

**運作良好**：52 個排程任務在跑（資料抓取/回測/實驗/18 校準/清理）；自癒齊備（熔斷 half-open 自動恢復、降級回 stale cache、AutoRollback、auto_backfill 補缺口、VIX≥35 危機熔斷）；agent 層達爾文權重真閉環（20 agents、權重今日仍在演化）；MCP 112 tools 註冊與目錄零漂移；22 個資料通道大部分 ok。

**需注意**：`auto_cycle_update` 連續失敗 2 次、`auto_experiment` 1 次、`calibration_cycle` 被停用（last_run 0001-01-01）；macro-ingest 部分失敗被 mask 成 ok（`cmd/macro-ingest/main.go:73-78` INTENTIONAL STUB）；admin 通道頁靜態清單漏列 18 個已註冊通道；FinMind 402 配額風險使個股法人資料最多落後一個月；報告產生無排程（on-demand）；品質/陳舊告警多為偵測型不自動修。

**對「機器自動優先」的體檢結論**：自動排程 ✅、自動進化 ⚠️（閉環存在但實驗堵塞 3 個月）、自動糾錯 ⚠️（機制齊但可觀測性弱——失敗會被空轉洗掉、skip 無告警）。「機制在、眼睛瞎」是主要風險。

---

## 七、給業主的三個誠實提醒

1. **不要過度承諾「散戶找到漏洞」**。法人資料 T+0 盤後才有，散戶最早 T+1 行動；「安全窗口」是機率性的。平台的真實價值是把「觀測→解讀→追蹤→紀律」系統化，降低散戶的資訊處理成本與情緒干擾——這個定位比「找到漏洞」更站得住，也更適合寫進對外文案。
2. **「懂 AI 的散戶」區隔成立，但目前 agent 入口恰好是缺口**。站內無任何 agent 解說整合（MCP 只有工程師門檻的教學頁）。若這是核心賣點，P2-4（站內解說入口 + explainer tool）的優先級應上調。
3. **Kimi 參考資料的價值在「可校準的假設清單」而非「可直接用的規則」**。系統最獨特的資產是校準/進化框架；把外部 heuristic 一律經過「歷史驗證 → 寫回 parameters.json → 持續追蹤命中率」的管道消化，才是與其他平台（寫死規則）的真正差異化。

---

## 附：證據索引

- 文件定位：Agent A 報告（docs 全文盤點，17 篇既有審計清單）
- 資料通道：Agent B 報告（17 項資料存在性總表 + 通道註冊落差）
- 錢潮鏈：Agent C 報告（鏈路圖 + 10 斷點，`internal/capitalflow/`、`internal/eventdriven/`、`internal/sectorallocation/`）
- 策略鏈：Agent D 報告（L1-L5 心法庫 vs 組合策略、達爾文權重、實驗堵塞，B1-B12 斷點表）
- UI：Agent E 報告（13 路由清單、需求覆蓋矩陣、無出口能力表、壞鏈 bug）
- MCP/排程/自癒：Agent F 報告（112 tools 分類、52 排程、自癒盤點、MCP 層 bug 根因）
- 活體驗證：`system_get_health`、`data_get_channels`、`capital_flow_summary`、`daily_report`、`event_flow_prediction`、`sector_allocation_plan`、`synergy_get_darwinian_status`、`scheduler_get_status`、`backtest_status`、`get_recommendations`、`strategy_ranker`（失敗）
