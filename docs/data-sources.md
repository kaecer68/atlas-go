# Data Sources

## Recommendation

For `2026-03-29`, the recommended source hierarchy is:

1. TWSE and TPEX official datasets for baseline market structure and historical replay
2. Fugle for near-real-time quotes and websocket streaming
3. Yahoo data only as a fallback or cross-check source

## Why Not Yahoo as Primary

- no stable public developer-first official trading API
- ecosystem often relies on unofficial scraping
- field stability and uptime are harder to guarantee
- weak fit for repeatable simulation and audit requirements

## Macro & Industry Data Sources

### Yahoo Finance Macro (`internal/marketdata/yahoo_macro_provider.go`)

Fetches global macro indicators every 5 minutes via Yahoo Finance API.

| Indicator | Ticker | Description |
|-----------|--------|-------------|
| DXY | DX-Y.NYB | US Dollar Index |
| US10Y | ^TNX | 10-Year Treasury Yield |
| VIX | ^VIX | CBOE Volatility Index |
| Oil | CL=F | WTI Crude Oil Futures |
| Gold | GC=F | Gold Futures |
| Silver | SI=F | Silver Futures |
| Copper | HG=F | Copper Futures |
| JPY | JPY=X | USD/JPY Exchange Rate |
| USD/TWD | USDTWD=X | USD/TWD Exchange Rate |

### BDI — Baltic Dry Index (`internal/marketdata/bdi_provider.go`)

| Attribute | Detail |
|-----------|--------|
| Source | CNBC JSON API (`.BADI` ticker) |
| Rate Limit | 1 req / 5s |
| Channel ID | `bdi` |
| Gateway Adapter | `internal/apigateway/adapter_bdi.go` |
| Used By | `DynamicEnvModulator.BDIDeviation()` → shipping seasonal modulation |

Note: `^BDIY` is not available on Yahoo Finance. BDI is a separate channel from Yahoo macro data.

### US Market Indexes & Tech Stocks (`internal/marketdata/us_index_provider.go`, `us_tech_provider.go`, `tsm_adr_provider.go`)

Fetches US market indexes and large-cap tech stocks on-demand via Yahoo Finance v8 chart API. All channels share `yahooSharedLimiter` (1 burst, 1s) per `internal/apigateway/CONSTITUTION.md` Art. 2.3.

| Symbol | Ticker | Provider | Channel ID | Gateway Adapter |
|--------|--------|----------|------------|-----------------|
| S&P 500 | ^GSPC | `SPXIndexProvider` | `us_spx` | `adapter_us_index.go` |
| Nasdaq Composite | ^IXIC | `NDXIndexProvider` | `us_ndx` | `adapter_us_index.go` |
| Dow Jones | ^DJI | `DJIIndexProvider` | `us_dji` | `adapter_us_index.go` |
| NVIDIA | NVDA | `NVDAProvider` | `us_nvda` | `adapter_us_tech.go` |
| Apple | AAPL | `AAPLProvider` | `us_aapl` | `adapter_us_tech.go` |
| Microsoft | MSFT | `MSFTProvider` | `us_msft` | `adapter_us_tech.go` |
| TSMC ADR | TSM | `TSMADRProvider` | `tsm_adr` | `adapter_tsm_adr.go` |

Gated by `ATLAS_YAHOO_ENABLED` env var (default **`true`** as of 2026-06; explicitly set to `false` to opt out). Consumed by `/api/cross-market/status` via `MacroDataGatewayAdapter` → `gateway.Fetch(channelID)`.

**Default-flip rationale (2026-06)**: PR #484 introduced a 4-layer data-visibility safeguard (see `.claude/skills/atlas-data-visibility/SKILL.md`) that exposed 10 US channels returning silent zero values — root cause was `ATLAS_YAHOO_ENABLED` defaulting to `false`, so the channels were never registered in production. The safeguard (upper defense: detects any channel failure) and this default flip (lower prevention: registers all 10 channels) are complementary, not redundant. The safeguard remains valuable for other failure modes (rate limit, network error, Yahoo API outage).

### ETF NAV (`internal/marketdata/etf_nav_provider.go`)

| Attribute | Detail |
|-----------|--------|
| Method | Market closing prices as NAV proxy |
| Cache TTL | 4 hours |
| Rate Limit | 1 req / 2s, burst 5 |
| Coverage | 11 Taiwan ETFs (0050, 0056, 00878, 006208, 00692, 00713, 00881, 00891, 00919, 00929, 00940) |

### TWSE Sector Indices (`internal/marketdata/twse_sector_index_provider.go`)

Fetches TWSE industry index data for 8 sectors (semiconductor, ai_supply_chain, electronics, shipping, financials, energy, robotics). Used by `RecalculateFromReturns()` for empirical correlation matrix computation.

### Sector Data (`internal/marketdata/sector_data_provider.go`)

Reads from `data/sector_data/sector_data.json`. Provides TSMC revenue, CoWoS utilization, SOX index, capex growth. Gracefully degrades to zeros if file missing.

### TAIEX 20-Day Volatility — 台股指數波動率（`tw_vol`）

| Attribute | Detail |
|-----------|--------|
| Source | Yahoo Finance — `^TWII` (TAIEX) 3-month daily bars |
| Method | 計算 20 個交易日對數報酬率標準差 × √252（年化） |
| Frequency | On-demand via `tw_vol` channel；production 由 `cfg.YahooEnabled` gate 控制（見 `internal/apigateway/register_adapters.go`） |
| Rate Limit | 1 req / 5s, burst 1（`limits.go:123`，`ExportStatisticsRate` 共享） |
| Channel ID | `tw_vol` |
| Gateway Adapter | `internal/apigateway/adapter_tw_vol.go` |
| Used By | `internal/strategy_techniques/evaluator.go:resolveField("HistoricalVolatility")` 讀 `MacroDataSnapshot.HistoricalVolatility.Value`（= ^TWII 最新收盤價）；`internal/feature/feature.go:434` 為公式定義來源 |
| Provider | `internal/marketdata/taiwan_volatility_provider.go` |

**⚠️ `ChangePct` 語意警示**：此 provider 把**年化波動率**（例 0.18 = 18%）寫入 `MacroDataPoint.ChangePct`，把**最新收盤價**寫入 `MacroDataPoint.Value`。其他 channel 的 `ChangePct` 為「日漲跌幅 %」，**tw_vol 為唯一例外**。下游若以「日漲跌 %」解讀此 channel 的 `ChangePct` 會被誤導為「年化波動率 × 252 倍誤差」。已知消費者（strategy_techniques、feature）只讀 `.Value`，不受影響。新增 consumer 須特別注意此語意差異。

### 排程與健康監控

- **Cron 排程**：`docker-compose.yml:cron-macro-ingest` 每天 08:00 跑 `/app/macro-ingest`（US market close 後）
- **Provider wire**：`cmd/macro-ingest/main.go:58` 已 wire 進 `CompositeMacroProvider`
- **Health 記錄**：`RecordChannelFetchWithPool` 對 15 個 channel（含 tw_vol）寫 `data/state/channel_health/`；**修正於 feat/tw-vol-channel-2026-07-22，之前只記 us_yahoo + frankfurter_fx 兩個 channel**
- **Alert**：`monitoring/rules/wave9_channel_individual_health.yml:ChannelHighErrorRatePerChannel` 自動涵蓋

### 歷史資料

- **`tw_vol` 是 sliding 20d 計算**：每次 Fetch 從 Yahoo 拉 3-month bars 重新算年化波動率，**不需 backfill**
- **`MacroDataSnapshot` daily**：macro-ingest 自動保存 `data/state/macro_snapshot/YYYY-MM-DD.json` + `latest.json` + `previous.json`（`internal/narrative/ingestor.go:saveSnapshot`）。tw_vol 從啟用日（2026-07-22）起自然累積 daily 歷史

### 前端 / MCP 穿透

- **MCP `data_get_channels`** + **`data_get_channel_detail`**：自動可見（透過 `gateway.ChannelIDs()` 動態列舉）
- **HTTP `/api/dashboard/data-channels`**：同上
- **前端 admin `datachannels.js`**：同上
- **前端 `home.js` marketPulse group**：`shared_web/static/js/pages/home.js` 已加 `tw_vol`（緊鄰 `taiex_index`），投資人首頁可見 channel 健康 badge
- **MCP canary**：`data_get_channels` 在 canary test 名單 → 自動觸及 tw_vol

## Provider Roles

### TWSE

Use for:

- daily market reports
- listed stock daily data
- market breadth
- institutional and ownership related datasets where available

### TPEX

Use for:

- OTC stock daily data
- supplementary market structure for Taiwan equities

### Fugle

Use for:

- near-real-time quote snapshots
- websocket streaming
- intraday simulation expansion

### Yahoo

Use for:

- sanity checks
- enrichment of public-facing quote views
- backup display data only

### FinMind

⚠️ **Status: Limited Availability** (as of 2026-05-13)

FinMind API returns **HTTP 402** with message `"Requests reach the upper limit"` when the account quota is exhausted. This affects:
- TaiwanStockMonthRevenue (TSMC revenue data)
- TaiwanStockFinancialStatements
- TaiwanStockInstitutionalInvestorsBuySell

**Rate Limits by Membership Tier:**

| Tier | Hourly Limit | Dataset Access |
|------|-------------|----------------|
| Unregistered | 300 req/hr | Basic |
| Free Member | 600 req/hr | Basic |
| Backer (Paid) | Higher | + "backer" marked datasets |
| Sponsor (Paid) | Highest | All including real-time & minute-level |

**Official Documentation:**
- IP Ban Policy: https://finmind.github.io/BanIPPolicy/
- API Usage Count: https://finmind.github.io/api_usage_count/

**Mitigation in Atlas:**
- `TSMCRevenueProvider` implements cache fallback (`loadLatestSnapshot`) — returns last known data when FinMind is unavailable
- `cmd/atlas` checks `cfg.FinMindAPIKey` before starting auto-fetch tasks
- All FinMind-dependent features degrade gracefully without crashing

**Recommendation:**
- For TSMC revenue: rely on cached data + manual backfill when FinMind is available
- For historical data: prefer TWSE OpenAPI (no rate limits, no API key required)
- Consider upgrading to FinMind Backer/Sponsor if real-time data is critical

## MVP Provider Strategy

### Phase 1

- build replay engine on daily data
- implement TWSE and TPEX adapters first

### Phase 2

- add Fugle snapshot support
- support delayed or near-real-time paper-trading views

### Phase 3

- add Fugle websocket event loop
- run event-driven intraday simulations


---

## Government Flow — 官股行庫資金代理（BK-13）

> **⚠️ 此通道與其他所有通道本質不同**：它不是外部 REST API，而是從 TWSE 公開網頁（Open Data）程式化讀取券商分點資料，再依方法論篩選聚合。

### 資料來源

| 屬性 | 內容 |
|------|------|
| 原始來源 | TWSE `bsr.twse.com.tw` — 券商分點買賣超日報表（公開資料，非 API） |
| 取得方式 | HTTP GET → HTML parse / CSV fallback |
| 頻率 | 每日 T+1（盤後公布，次日排程抓取） |
| 速率限制 | 2 秒/請求（自訂 limiter，TWSE 無官方 rate limit 公告） |
| 範圍 | TW50 成份股（50 檔權值股） |
| Channel ID | `government_flow` |
| Provider | `GovernmentFlowProvider`（唯讀消費端） |
| Producer | `GovernmentBrokerAggregator`（BK-13 寫入端） |
| 儲存格式 | `data/state/government_flow/YYYYMMDD.json` — flat JSON 檔案 |
| Source 標籤 | `broker-aggregate` |

### 與其他通道的關鍵差異

| 維度 | 其他通道（TWSE T86 / Fugle / Yahoo） | government_flow |
|------|--------------------------------------|-----------------|
| 資料介面 | REST API / JSON / CSV | **公開網頁 → 程式化讀取** |
| 資料粒度 | 個股 × 投資人類型（外資/投信/自營） | **券商分點 × 個股** |
| 更新方式 | Gateway adapter 直接 fetch | **Producer 寫檔 → Provider 讀檔** |
| 上游依賴 | 外部 API 可用性 | TWSE 網站可用性 |
| 角色分類 | official_actor（參與法人共識） | **behavioral_proxy（不參與共識，獨立訊號）** |

### 方法論（BK-13）

**為何不用全體券商，而要篩選 5 家核心行庫？**

TWSE 不公布「官股行庫」整體買賣超（它不是三大法人中的獨立類別）。但實務上可透過券商分點資料間接觀測：

1. **篩選 5 家核心行庫**（非 8 家）：合庫(8060)、土銀(8030)、臺灣銀(8040)、台企銀(8010)、彰化(8064)
   — 這 5 家的一般散戶開戶數極低，分點進出更能反映政府基金動向
2. **僅取總公司分點**（非所有分行）：減少雜訊
3. **僅觀察權值股**（TW50 成份股）：政府護盤只買大盤影響力高的股票
4. **T+1 聚合**：TWSE 盤後公布，次日排程自動抓取

參考文獻：YOTTA 友讀〈看懂「八大行庫買賣超」，了解政府如何操作股市？〉

### 應用方式

```
GovernmentBrokerAggregator（Producer，28h 排程）
  → TW50 成份股 × 5 家行庫總公司分點
  → 加總淨買賣超 → 寫入 data/state/government_flow/YYYYMMDD.json
  → GovernmentFlowProvider.Latest() 自動讀取
  → GovernmentFlowAdapter → gateway channel
  → MacroDataSnapshot.GovernmentNet
  → ForceExtractor.scoreGovernment() → ForceScore{DataAvailable: true}
  → 累積 ≥30 筆後參與 capital_flow 共振模型
```

### 生效條件與限制

- **資料累積需求**：需 ≥30 個交易日資料後，官股 force 才參與共振（capital-flow spec §4）
- **資料缺檔處理**：無資料或取不到時 `DataAvailable=false`，trend=neutral，**不寫入零值樣本**（CF-INV-06）
- **gateway 錯誤處理**：Stale result，不報錯 — 共振模型需區分「無資料」與「資料說中性」
- **回溯限制**：此通道僅從部署日開始累積，無法回溯歷史（TWSE 分點網頁不保留歷史查詢）
