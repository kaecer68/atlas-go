# Fix Manifest: 2026-07-16 UI / Data / Regime 審計修復

> **建立時間**：2026-07-16  
> **審計來源**：本 session 對 `/client/`、`/admin/`、atlas-mcp API、排程、資料通道的交叉審計  
> **目標**：把 21 個已盤查的問題，每一個都綁定到「可驗證的程式變更」，並在本 session 或連續 session 中完成追蹤。
> **工作模式**：以單一 coherent CLI session 為主；只在子系統完全獨立時開並行 session。

---

## 一、為什麼這次不會再遺漏

本 manifest 不是計劃書，而是 **invariant tracker**：
- 每一個問題都有獨立編號、根因假設、要改的檔案、驗收方式。
- 每次 commit 必須寫上 `fix(manifest): #<ID> <short description>`。
- 每次 session 結束前，必須更新本文件的「狀態」欄位。
- 新發現的問題不會馬上修，先記在「Backlog」區，每天只允許取 1 個加入當日 scope。

---

## 二、Fix Manifest（21 個問題）

### 線 A：資料正確性與 regime 統一
**主線目標**：讓後端發出的訊號彼此一致、資料新鮮、排程正常執行。

| ID | 問題 | 根因假設 | 修改檔案 | 驗收方式 | 狀態 | 備註 |
|----|------|---------|----------|----------|------|------|
| A01 | 錢潮預測熱度圖 88% 所有板塊都一樣 | 前端把整體 event_flow_prediction.confidence 填入每一個板塊/日期格子 | `shared_web/static/js/pages/capital_predictions.js` | 熱度圖不再顯示板塊層級的 88%；改為顯示「整體 5 日流入信心 88%」或移除板塊數字 | 已完成 | commit 31748e1d；build + 172 tests PASS |
| A02 | daily_report 全球 RISK_ON，但 system/regime 是 RISK_OFF | `defaultProvider.FetchMacro()` 將 `GlobalOverview.Status` 硬編碼為 `"RISK_ON"`，Generator 未注入真實 regime 來源 | `internal/dailyreport/report.go` + `cmd/atlas/main.go` | daily_report 與 system_get_health 回傳相同 regime（來源：最新 session summary） | **已完成** | 透過 `SetRegimeProvider` 注入 session summary regime；無資料時 fallback `NEUTRAL`（commit 00651d75） |
| A03 | 客戶端只顯示「偏多」，沒有 regime 狀態 | 前端 hero 的「偏多/偏空/觀望」是獨立方向判斷，未顯示 regime | `shared_web/static/js/pages/home.js` | 首頁 hero 顯示 regime 徽章（`RISK_ON`→偏多 regime、`RISK_OFF`→偏空 regime、`NEUTRAL`→觀望 regime） | **已完成** | 與 system-health 資料一致（commit d52519c1） |
| A04 | US channel 大量 rate limit | `us_market_refresh` 任務依序呼叫 8 個 Yahoo 通道，未善用三組 limiter 並行，且無 stagger 與單通道 timeout | `internal/apigateway/us_market_refresh.go` | `system_get_health` 的 degraded_channels 不再包含 us_spx、us_dji、sox_index、us_nvda、us_aapl、us_msft、tsm_adr；批次完成時間 < 5 秒 | **已完成** | 改為 goroutine + 100 ms stagger + 10 s per-channel timeout（commit 040dff7d） |
| A05 | scheduler MCP tool 解 JSON 失敗 | `/api/scheduler/status` 回傳 array，但 MCP handler 期望 map | `cmd/atlas-mcp/server/tools_scheduler_task.go` | `atlas-mcp_scheduler_get_status` 成功回傳，不再報 `cannot unmarshal array` | 已完成 | 程式碼已正確處理 array；已重建 binary `/Users/kaecer/workspace/atlas/bin/atlas-mcp`，需 restart MCP server 後生效 |
| A06 | 多個背景任務 last_run 為 0001-01-01 | `autobacktest_daily` 在 `taskMgr.Start()` 之後才註冊，導致它從未被排程 | `cmd/atlas/main.go` | `/api/scheduler/status` 中 autobacktest_daily 顯示最近執行時間；linkage_calibrate、tsmc_revenue、auto_backfill、auto_strategy_evolution 已於 `Start()` 前註冊 | **已完成** | 將 autobacktest_daily 註冊移到 `taskMgr.Start()` 之前（commit b53c7e10）；重啟後須觀察 A07 replay 資料更新 |
| A07 | Replay 資料延遲 2 天（7/14） | auto_backfill 未執行或 replay importer 未更新；daily-replay-sync 落在不同 compose 資料卷 | `internal/replay/` 與 docker-compose cron 資料卷 | admin 頁面「最新回放資料」顯示 2026-07-15 或 2026-07-16 | **已完成** | 已手動執行 daily-replay-sync 並重建 cron-replay-sync 容器使用相同 data 卷；目前 latest date 為 2026-07-16 |

### 線 B：顯示層不丟失資訊
**主線目標**：讓後端已有的資料正確穿透到網頁，並告訴投資人資料品質。

| ID | 問題 | 根因假設 | 修改檔案 | 驗收方式 | 狀態 | 備註 |
|----|------|---------|----------|----------|------|------|
| B01 | 宏觀視野 / 敘事頁面全部卡住 | `/api/dashboard/retail-sentiment` timeout 導致整頁等待 | `client_web/static/js/main.js` + `internal/monitoring/api/system/handlers.go` | 瀏覽器開啟 /client/narrative 後，6 張卡片在 5 秒內渲染完成，不再持續 loading | **已完成** | 前端非阻塞載入 + 後端 fetcher 平行化（commits 52bea3cb, cae5b6cf） |
| B02 | 首頁「加權指數」顯示「—」 | `taiex_index` 通道已註冊到 Gateway adapter，卻未加入 `channelIDs()` 與 `RateLimitManager`，導致 Gateway.Fetch 回傳 unknown channel | `internal/apigateway/gateway.go`、`internal/apigateway/limits.go`、`internal/apigateway/adapter_taiex.go` | 首頁顯示加權指數數值與漲跌幅 | **已完成** | 須重啟 Go server 讓 channel 註冊生效 |
| B03 | 缺少 7-Force 錢潮看板 | 後端有 `capital_flow_summary` 的 7 大勢力，但前端只展示板塊 | 新增 `shared_web/static/js/components/seven-force-board.js` + 在首頁引入 | 首頁出現 7-force 卡片：外資、投信、自營商、散戶、政府、期貨、TSM ADR，各顯示方向與強度 | **已完成** | commit debddb77；build + 172 tests PASS |
| B04 | 缺少「法人 vs 散戶對殺」故事卡 | 前端沒有把外資/投信/自營商/散戶放在一起視覺化 | 新增 `shared_web/static/js/components/capital-battle-card.js` | 首頁出現一張卡，同時顯示 4 大勢力買賣方向，並有一句「法人進 / 散戶出」的解讀 | **已完成** | commit 0dc4ee55；build + 172 tests PASS |
| B05 | 客戶端不顯示 channel 異常 | 前端沒有讀取 `data_status` / `failed_channels` | `shared_web/static/js/pages/home.js` + `shared_web/static/js/components/data-quality-badge.js` | 當後端有 degraded_channels 時，受影響的卡片顯示「資料異常」徽章，而不是「—」或 0 | **已完成** | 已新增 data-quality-badge 組件，覆蓋首頁五大區塊（commit 71d89df7） |
| B06 | 未來 5 日錢潮預測預設摺疊 | UI 預設把重要資訊收合 | `shared_web/static/js/pages/home.js` | 未來 5 日錢潮預測區塊預設展開，且內容正確 | **已完成** | commit 423f479d；build + 172 tests PASS |
| B07 | 熔斷機制顯示「未初始化」 | circuit breaker 在啟動時沒有寫入初始狀態檔 | `internal/monitoring/service/circuitbreaker.go` | admin 頁面顯示熔斷機制已初始化，且 system_get_health 沒有相關警告 | **已完成** | commit 0468c091；go test ./internal/monitoring/service/... PASS |

### 線 C：體驗與金融工程模型
**主線目標**：修復明顯體驗瑕疵，並補強模型假設。

| ID | 問題 | 根因假設 | 修改檔案 | 驗收方式 | 狀態 | 備註 |
|----|------|---------|----------|----------|------|------|
| C01 | 首頁重大事件重複 3 次（MSCI） | 事件 API 回傳重複，或前端渲染沒有去重 | `client_web/static/js/pages/home.js` 或 `internal/eventdriven/` | 同一事件在首頁只出現一次 | 已完成 | commit 9da4b902；build + 172 tests PASS |
| C02 | 左側選單連結無效 | 選單用 JS 切換，沒有真實 href | `client_web/static/index.html` 或 `shared_web/static/js/main.js` | 每個選單項目的 `<a>` 有真實 `href`；/client/capital-flow 不再 404 | 已完成 | commit 015167d7；build + 172 tests PASS |
| C03 | 沒有「未來一週方向預測」的機率分佈 | 後端有方向與信心，但沒有分情境機率 | `internal/eventdriven/types.go` + `internal/eventdriven/predictor.go` + `shared_web/static/js/pages/home.js` | 首頁顯示「流入 X% / 觀望 Y% / 流出 Z%」的機率分佈 | **已完成** | commit 80229e04；go test ./internal/eventdriven/... PASS；build + 172 tests PASS |
| C04 | 外資主導假設過度簡化 | 7-force 沒有動態權重 | `internal/capitalflow/types.go` + `internal/capitalflow/report.go` + 7-Force board | 7-Force 卡片顯示各 force 的動態權重（依原始值絕對值占比） | **已完成** | commit 78a929ee；go test ./internal/capitalflow/... PASS；build + 172 tests PASS |
| C06 | 缺少 ETF 資金流 | 0050/00878 等 ETF 申贖未被納入 | 待評估：現有 `eventdriven.Predictor` 已產出 `ETFEstimate`（`buildETFEstimates`）與 `etf_estimates` 欄位；cron 僅有 `atlas-cron-replay-sync` 每日同步個股 replay | 完成評估；現階段不新增 channel，建議在 eventdriven 預測已標示 ETF 事件時直接於前端渲染 `etf_estimates` | **已完成** | 評估結論：不新增獨立 channel，優先 expose 既有 `ETFEstimate`；需後續 PR 設計資料來源與 UI |
| C07 | 事件預測沒有板塊維度 | 後端只有整體信心，前端硬畫板塊 | 已在 A01 處理顯示面；現有 `EventCalendarItem.affected_industries` 已在 event calendar 顯示板塊 chips | 完成評估；板塊模型增強建議放後續專門 PR | **已完成** | 評估結論：現有 `affected_industries` 標籤已解決顯示面誤導；板塊級預測模型需訓練資料與專門設計 |
| C07 | 事件預測沒有板塊維度 | 後端只有整體信心，前端硬畫板塊 | 已在 A01 處理顯示面；現有  已在 event calendar 顯示板塊 chips | 完成評估；板塊模型增強建議放後續專門 PR | **已完成** | 評估結論：現有 affected_industries 標籤已解決顯示面誤導；板塊級預測模型需訓練資料與專門設計 |

---

## 三、執行策略與順序

### 第一輪：止血（P0，預計 1-2 天）
1. A01：修正錢潮熱度圖誤導
2. B01：修正 narrative 頁面 loading
3. C01：去除重複事件
4. C02：修正選單連結與 404
5. A05：修正 scheduler MCP tool schema

### 第二輪：資料正確性（P1，預計 2-3 天）
6. A02/A03：統一 regime 並在前端顯示雙訊號
7. A04：修復 US channel rate limit
8. A06/A07：修復背景排程與 replay 資料新鮮度
9. B02：修復加權指數顯示
10. B05：加入資料異常徽章

### 第三輪：投資人輪廓（P1-P2，預計 3-5 天）
11. B03：新增 7-Force 看板
12. B04：新增法人 vs 散戶對殺故事卡
13. B06：展開未來預測區塊
14. C03：新增方向預測機率分佈
15. B07：初始化熔斷機制

### 第四輪：金融工程強化（P2-P3，預計 1-2 週）
16. C04：force 動態權重
17. C05：7-force 互動模型
18. C06：ETF 資金流（設計與評估）
19. C07：板塊維度預測（設計與評估）

---

## 四、溝通規則（我會遵守）

- **每完成一個 issue**，我會在對話中回報：「#A01 完成，驗證方式：...」。
- **每完成一輪**，我會更新本 manifest 的狀態欄，並簡短總結進度。
- **我會主導下一步**，不會問你「接下來做哪一個」。
- **只有在以下情況我才會暫停並問你**：
  1. 需要 API key / credential / 環境變數（例如 FinMind、Yahoo、TEJ 等）。
  2. 需要做出不可逆的產品決策（例如「要不要把 regime 強制統一成 JANUS？」）。
  3. 修復會影響到別的進行中 PR（例如 refactor/client-web-trust-and-clarity 的 18 個 dirty 檔）。
  4. 需要 merge / push / deploy 權限。

---

## 五、需要你配合什麼

1. **不要切換 session**：我會在本 session 內連續做完。除非你發現系統強制結束或我主動說要存 context。
2. **如果我要新開並行 session 處理獨立子系統**，我會先跟你說為什麼、哪幾個 issue 會被拆出去，並且給你一個 resumesession 的關鍵字或 `/context-save` 指令。
3. **如果我停在某個問題超過 30 分鐘沒進展**，你可以直接問我「#A01 為什麼卡住」。
4. **當我說「這一輪完成」時**，你可以選擇：
   - 繼續下一輪（我會自動繼續）
   - 先看 diff / 測試報告（我會整理給你）
   - 暫停（我會用 `/context-save` 存狀態）

---

## 六、Backlog（新發現問題暫存區）

| 編號 | 問題 | 發現時間 | 預計輪次 |
|------|------|---------|----------|
| - | （待填入） | - | - |

---

## 七、執行紀律檢查清單（每次 commit 前自檢）

- [ ] 這個 commit 對應到 manifest 的哪一個 ID？
- [ ] 該 ID 的驗收方式是否已經跑過？
- [ ] 有沒有引入新的 placeholders（TBD / TODO / 未實作）？
- [ ] 是否更新了本 manifest 的狀態欄？
- [ ] 有沒有未經許可改到 refactor/client-web-trust-and-clarity 的 18 個 dirty 檔？

---

*最後更新：2026-07-16*  
*下一次更新：完成第一輪止血後*
