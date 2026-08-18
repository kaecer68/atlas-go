# 調查報告 — TWSE 外部抓取 timeout (N1, 2026-08-18)

> **調查者**: prime-agent 子代理 (N1 調查先行, 不直接修 code)
> **範圍**: taiex_index / twse_etf / twse_replay 三 channel error + circuit breaker (deployer 觀察 2h 內 33 次)
> **方法**: iMac production 實機觀測 (docker logs / channel_health.json / channel_fetch_log.json / live endpoint probe) + worktree 源碼追查
> **結論一句話**: 33 次 error 的組成是 **3 個真實失敗 channel** + **監控 fetch-log 錯誤誤掛 (attribution artifact) 灌水**; 真實失敗中 1 個是外部暫態不穩 (TWSE STOCK_DAY_ALL 慢), 2 個是**系統設計缺口把「正常/可預期」情境誤判為 error** (taiex_index 盤前窗口未處理、twse_etf legacy caller 仍探測已停用 channel), twse_replay 是「註解說讀本地 CSV、實作卻打 live API」的線路錯誤。

---

## 1. 調查執行摘要

| # | 調查步驟 (任務契約) | 結果 |
|---|---|---|
| 0 | iMac `TWSE_ETF_API_KEY` env 檢查 (k3 N1-1 強制先行) | **未設置**。`~/.config/atlas-go/.env` 與容器 env 皆無 TWSE_ETF_API_KEY → twse_etf adapter **未註冊** (register_adapters.go:315 gating 生效, 二進位內含 gating 字串)。**但** health 仍顯示 `twse_etf:error` — 「33 次多數是 twse_etf」的**原始假設被推翻, 但 twse_etf error 仍然存在** (原因見 §3.3, k3 N1-1 只對了一半) |
| 1 | 分類 33 次 error | 見 §2。3 個真實失敗 channel + fetch-log 誤掛 (~33/50 條 error 欄位掛在 status=ok 的無辜 channel 上) |
| 2 | timeout/retry/CB 設定查證 | `twse_api_timeout_sec` 預設 **15s** (defaults_market.go:107); `taiex_twse_fallback.go` 硬編碼 **15s**; `adapter_twse.go` (twse_replay) 用 shared client 15s; stocktools TWSE fallback 5s context。**無統一 retry 層**。CB: 預設 threshold 3, twse_replay/twse_etf 等特調 5 (gateway.go:32-40), recovery 5min, half-open 2 |
| 3 | twse_etf gating 查證 | register_adapters.go:315 `if etfKey := config.GetSecret("TWSE_ETF_API_KEY"); etfKey != ""` → 未設置時不註冊 + Record("twse_etf", "inactive", ...)。**gating 本身正確** |
| 4 | 判定 | 外部暫態不穩 (STOCK_DAY_ALL) + 系統設計缺口 (盤前窗口 / legacy caller / 線路錯誤) 混合, 見 §4 |

---

## 2. 錯誤組成分析 (2h 窗口)

> 容器於 2026-08-17 23:16:58Z 重建 (RestartCount=0), deployer 觀察的「33 次」發生於前一 instance, 舊 log 已不可得; 本報告以重建後的同型 error pattern 為代表樣本 (23:17–00:05Z), 並以 iMac 現存 `channel_fetch_log.json` 佐證計數結構。

### 2.1 真實失敗 channel (3 個, 每 30s health summary 持續顯示 error)

| channel | 最新 error (channel_health.json) | error 型態 |
|---|---|---|
| **taiex_index** | `taiex_index: yahoo failed (invalid latest price: 0) and twse fallback failed: taiex_twse: TAIEX row not found for 20260818` | **資料未發布 (盤前)**, 非 timeout |
| **twse_etf** | `circuit breaker open for channel twse_etf` (底層: `channel not registered: twse_etf`) | **legacy caller 探測未註冊 channel → CB 計敗**, 非上游 404 |
| **twse_replay** | `twse fetch: Get "https://www.twse.com.tw/exchangeReport/STOCK_DAY_ALL": context deadline exceeded (Client.Timeout exceeded while awaiting headers)` | **外部 timeout (TWSE 慢窗口)** |

### 2.2 6h 窗口 ERROR-level 事件 (重建後, 排除 read_session_summary 雜訊)

```
1  task_failed name=auto_taiex_index        err="circuit breaker open for channel taiex_index"
1  task_failed name=channel_health_twse_replay err="...STOCK_DAY_ALL context deadline exceeded"
1  task_failed name=etf_nav_refresh         err="etf_nav_refresh fetch: twse fetch: ...STOCK_DAY_ALL context deadline exceeded"
1  stocktools: TWSE quote fallback failed  symbol=TSM  (STOCK_DAY_ALL timeout)
1  stocktools: TWSE quote fallback failed  symbol=NVDA (STOCK_DAY_ALL timeout)
+ 10  feed_failed  TWSE RSS 新聞 feed (geopolitical, rate limit timeout — 同上游慢窗口, 非本 N1 三 channel)
```

### 2.3 ⚠️ 監控 fetch-log 誤掛 (灌水來源, 高度疑似「33 次」的主要計數來源)

`channel_fetch_log.json` (50 條 ring buffer) 中 **35 條帶 error 欄位, 其中 33 條是 status=ok 的無辜 channel** 掛著別人的 error 文字:

```
export_statistics  status=ok  error="circuit breaker open for channel taiex_index"
us_ndx             status=ok  error="circuit breaker open for channel taiex_index"
exchange_rate      status=ok  error="channel not registered: twse_etf"
... (共 33 條)
```

→ admin data-channels / fetch-log 面板若以「error 欄位非空」計數, 2h 內極易累積到 ~33 條, 與 deployer 觀察數吻合。**真正的失敗 channel 只有 3 個**; 誤掛的產生路徑 (某 probe 迴圈共用 error 變數或 health sweep 錯誤文字串流) 本次靜態追查未鎖定到單一 call site (所有 `Record()` 生產 call site 皆傳自身 errMsg), 列為 follow-up。

---

## 3. 根因分析 (逐 channel)

### 3.1 taiex_index — 系統設計缺口: 交易日盤前窗口未處理 ⚠️ (非 timeout)

**證據鏈**:
1. 2026-08-18 是**星期二 (交易日)**。error 發生於 07:xx–08:0x 台北時間 (盤前)。
2. Live probe 實證 (08:0x 台北, iMac host):
   - Yahoo ^TWII `interval=1d&range=3mo`: 最新一筆 close = **null/0** (盤前 in-progress bar) → `parseYahooTAIEX` 判 `latest == 0` → "invalid latest price: 0"
   - TWSE MI_INDEX `date=20260818&type=IND`: `stat=OK` 但 **`data: []` 空** → `fetchTWSETAIEXFallback` → "TAIEX row not found for 20260818" (TWSE 約 14:00 後才發布當日指數)
3. 源碼: `taiex_index_provider.go` 只有 **週末/假日 gate** (`isTaiwanTradingDay`), 無**交易日盤前 (00:00–09:00) rollback**; 而同一份 code 的 `twiiCacheTimestampIsCurrentTradingDay` (taiwan_index_cache.go:42-56) **已有盤前處理** (`now.Hour() < twseMarketOpenHour → expected = latestTaiwanTradingDay(now-1d)`) — FetchSnapshot 沒沿用。
4. 後果: auto_taiex_index (1h) + 啟動期 probe 在盤前連續失敗 → CB (threshold 3) 開路 → health 顯示 error。**09:00 後 Yahoo 有真實 bar 即自癒** (現況: last_success 2026-08-17T15:57Z)。

**判定**: 不是 timeout 問題 (plan 假設 (b) 部分不成立), 是**交易日盤前資料窗口被當成 error** — 與週末/假日同型的既有設計缺口 (盤前應回退前一交易日 close)。

### 3.2 twse_replay — 線路錯誤: 註解說本地 CSV, 實作打 live TWSE ⚠️ (plan 假設 #5 錯誤)

**證據鏈**:
1. `adapter_twse.go`: `NewTWSEChannelAdapter` 註解 **"file-based replay, no rate limit"**, 但 `Fetch`/`HealthCheck` 實作是 `client.GetQuotes(ctx)` → **live GET `https://www.twse.com.tw/exchangeReport/STOCK_DAY_ALL`** (twse_openapi.go:110), shared client timeout 15s。
2. 觸發任務: `channel_health_twse_replay` (1h, data_sync_health_tasks.go:244) 與 `etf_nav_refresh` (24h, main.go:1479) 皆 `gateway.Fetch("twse_replay")` → live fetch。
3. **本地 replay CSV 存在且新鮮**: `/app/data/replay/tw_extended_90days.csv` (234KB, 2026-08-17 15:30 更新) — channel 代表的資料其實是好的。
4. 外部時序: 23:17–23:58Z (07:17–07:58 台北) TWSE STOCK_DAY_ALL 對 iMac 慢到 >15s (多次 "context deadline exceeded"); **08:0x 後 live probe 0.1s 200 OK** → 暫態慢窗口, 非持續故障。
5. 無 retry: 15s 一次失敗即計 CB (threshold 5)。

**判定**: 混合 — **外部暫態慢 (真實)** + **系統線路錯誤 (把 live fetch 掛在「本地 replay」channel 上, 使本地資料完好卻報 error)**。

### 3.3 twse_etf — gating 生效但 legacy caller 仍探測 ⚠️ (k3 N1-1 修正後重新歸因)

**證據鏈**:
1. Gating 正確: 無 TWSE_ETF_API_KEY → adapter **未註冊** (二進位含 gating 字串, 容器 env 無 key) + 註冊時 Record "inactive"。
2. **但** `NewETFFetcher` (monitoring/gateway_adapter.go:758-773) 仍把 `fetcher(ctx, "twse_etf")` 當「第一優先」探測 (註解明言 legacy 路徑; 失敗才落回 Fubon PCF)。
3. `gateway.Fetch("twse_etf")` → `breaker.Call` → `registry.Get` 失敗 → **"channel not registered: twse_etf" 被當 CB failure 計數** (gateway.go:60-63) → 5 連敗 → CB open → `Record("twse_etf", "error", "circuit breaker open for channel twse_etf")` **覆蓋掉註冊時的 "inactive" 紀錄**。
4. 靜態 40-channel 清單 (gateway.go:220 `channelIDs()` 含 twse_etf) 確保 CB/health slot 恆存在 → error 狀態持久可見。

**判定**: 系統設計缺口 — **停用機制 (inactive) 被 legacy caller 繞過**, 產生「未註冊 channel 的 CB-open」噪音。這正是 k3 N1-2 說的: 應在**來源端** (caller 不再探測) 排除, 而非在 CB 層新造排除邏輯。

### 3.4 stocktools TWSE fallback (NVDA/TSM) — 次要噪音

fugle 對 US symbol 回 404 (Resource Not Found) → fallback TWSE STOCK_DAY_ALL (5s context) → 同慢窗口 timeout → 503。屬同一上游慢窗口的連帶噪音; 且「fugle 404 後 fallback TWSE 查 US symbol」本身就是低價值路徑 (TWSE 無 US symbol)。

---

## 4. 判定: 外部不穩 vs 系統設定

| 維度 | 判定 |
|---|---|
| 外部 API 真實不穩? | **是 (暫態)**: TWSE STOCK_DAY_ALL 在 07:17–07:58 台北對 iMac 慢於 15s; 08:0x 後恢復 0.1s。MI_INDEX 盤前 `data:[]` 是**正常發布時序**, 非故障 |
| 系統設定問題? | **是 (3 處設計缺口)**: ① taiex_index 盤前無 rollback; ② twse_replay 註解/實作不符 (live fetch 冒充本地 replay); ③ NewETFFetcher 仍探測停用 channel → CB 噪音 |
| CB 太敏感? | 次要: taiex_index threshold 3 在「盤前窗口」情境偏易開 (每小時 1 次 probe + 啟動 probe 就會開); twse_replay/twse_etf 已特調 5。**不建議先調 CB** — 應先修來源 (k3 N1-2) |
| 服務健康受影響? | **否** (L3 驗收同結論): CB 開路後 gateway.Fetch 回 stale cache (Layer 1 fallback), 消費端 (replay/ETF 走 Fubon PCF) 有替代路徑 |

---

## 5. 建議方案 (修復優先序, 依 k3 修正: 優先用 inactive/來源端機制, 不在 CB 層新造排除)

### S1 (最高優先, 小改, k3 N1-2 落地) — twse_etf: 移除 legacy 探測
`NewETFFetcher` (monitoring/gateway_adapter.go:768) 刪除 `fetcher(ctx, "twse_etf")` 第一優先嘗試, 直接走 Fubon PCF (B03 superseded 2026-08-17, subC3 已改消費 Fubon)。效果: twse_etf 不再有任何 fetch → 不計 CB → health 維持 "inactive" (既有 inactive 機制), error 噪音歸零。**不在 CB 層新增任何邏輯** (k3 N1-2 明示)。附測試: ETFFetcher 不呼叫 twse_etf channel。

### S2 (中優先, 小改) — taiex_index: 盤前窗口回退前一交易日
`TAIEXIndexProvider.FetchSnapshot` / `fetchTWSETAIEXFallback` 加入盤前分支: 交易日且 `now.Hour() < twseMarketOpenHour` (09:00) 時 `target = latestTaiwanTradingDay(now.AddDate(0,0,-1))` — 完全沿用 `twiiCacheTimestampIsCurrentTradingDay` (taiwan_index_cache.go:49-54) 既有模式。效果: 盤前改serve 前一交易日 close (與週末/假日 gate 同語意), 不再誤報 error、不再觸發 CB。附測試: 盤前 (07:00) 交易日 FetchSnapshot 回傳前一交易日值且不計失敗。

### S3 (中優先, 中改) — twse_replay: 線路修正 (二選一)
- **S3a (推薦)**: `TWSEChannelAdapter` 改讀本地 replay CSV (與註解一致, `data/replay/tw_extended_90days.csv`, 有 `config.GetReplayDataPath`), 不再 live fetch; channel_health_twse_replay / etf_nav_refresh 即驗證「本地資料新鮮度」而非「TWSE 通不通」。**本質上就是 k3 說的「inactive/來源端」思路 — 把 channel 導回它聲稱的本地來源**。
- S3b: 保留 live fetch, 但加 1 次 retry (短 backoff, 只對 timeout/5xx) + timeout 15→20s。
附測試: adapter 對缺檔/舊檔回明確錯誤; live-fetch 模式對 timeout 觸發 retry。

### S4 (低優先, 校準) — timeout/retry 統一
`twse_api_timeout_sec` 15→20s (defaults_market.go:107 Todo 本就寫 [10,30] 待校準; 實測 twse_sector_index 10s / dram_spot_price 10s 邊緣, STOCK_DAY_ALL 慢窗口 >15s); 統一 `taiex_twse_fallback.go` 硬編碼 15s 改讀 config 參數 (消除三處魔法數字: defaults_market / taiex_twse_fallback / twse_etf_provider)。

### S5 (follow-up, 不併本 PR) — fetch-log 誤掛灌水
`channel_fetch_log.json` 33/50 條 status=ok 卻帶他人 error 文字 — 影響任何以 fetch-log 計數的觀測 (疑為「33 次」主要來源)。需另開調查鎖定 probe 迴圈的共享 error 狀態。

### 不做 (依調查證據)
- ❌ 調 CB 閾值放寬 (taiex_index 3→5): 治標不治本, k3 N1-2 反對在 CB 層解決; 修 S2 後盤前不再失敗, CB 自然不開。
- ❌ 修 twse_etf provider timeout/404: 上游已死 (known_issues twse_etf_upstream_60d), 非可修 bug。

---

## 6. 驗收標準 (若採納 S1-S3)

- 24h 觀察: `channel_health_summary` 不再出現 `twse_etf:error` (改 inactive) 與 `taiex_index:error` (盤前窗口改 ok/stale 前一交易日); `twse_replay` 僅在 TWSE 真實慢窗口時 error。
- fetch-log: status=ok 的 channel 不再帶 error 文字。
- 單元測試: 上述 S1/S2/S3 各附測試, 同 commit。

---

## 7. 附錄 — 證據

- iMac `.env` 無 TWSE_ETF_API_KEY (grep exit 1); 容器 env 亦無 (docker exec env)。
- 二進位含 gating 字串: `strings /app/atlas-go | grep -c TWSE_ETF_API_KEY` = 2。
- channel_health.json (2026-08-18T00:01:31Z): taiex_index/twse_etf/twse_replay 三 channel error 如上表。
- channel_fetch_log.json: 50 條中 35 帶 error 欄位, 33 條 status=ok 誤掛 (export_statistics/us_ndx/us_msft/... 掛 taiex_index/twse_etf CB 文字)。
- Live probe (2026-08-18 ~08:0x 台北, iMac host): STOCK_DAY_ALL 0.10s/0.11s/0.11s (200); MI_INDEX 20260818 `data:[]`; Yahoo ^TWII 最新 close null; (MacBook path STOCK_DAY_ALL 0.13s)。
- docker logs: task_failed auto_taiex_index / channel_health_twse_replay / etf_nav_refresh; stocktools TWSE fallback failed (TSM/NVDA)。
