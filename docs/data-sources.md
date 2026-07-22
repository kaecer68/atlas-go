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

### TWSE SBL — 借券賣出餘額（`twse_sbl`，STUB G02）

| Attribute | Detail |
|-----------|--------|
| Source | TWSE（endpoint 待確認） |
| Status | **STUB (G02)** — `HealthCheck` 回傳 `inactive`；`register_adapters.go:277` 已 stub 註冊，限流已就位但 fetch 邏輯尚未實作 |
| Rate Limit | 1 req / 2s, burst 1（`limits.go:132`） |
| Channel ID | `twse_sbl` |
| Gateway Adapter | `internal/apigateway/adapter_twse_sbl.go` |
| Scheduled Task | `auto_twse_sbl`（待啟用） |

### TDCC Equity Dispersion — 集保股權分散（`tdcc_equity_dispersion`，STUB G01）

| Attribute | Detail |
|-----------|--------|
| Source | TDCC（OpenData） |
| Status | **STUB (G01)** — `HealthCheck` 回傳 `inactive`；`register_adapters.go:287` 已 stub 註冊 |
| Rate Limit | 1 req / 5s, burst 1（`limits.go:133`） |
| Channel ID | `tdcc_equity_dispersion` |
| Gateway Adapter | `internal/apigateway/adapter_tdcc_equity.go` |
| Scheduled Task | `auto_tdcc_equity_dispersion`（待啟用） |

### JANUS Regime — 內部 regime 偵測引擎（`janus_regime`）

| Attribute | Detail |
|-----------|--------|
| Source | In-process（PRISM / JANUS engine，非上游 HTTP） |
| Method | `internal/janus.Engine` 提供 RISK_ON / RISK_OFF / NEUTRAL / TRANSITIONAL |
| Frequency | 6h refresh via `janus_regime_refresh` scheduled task |
| Rate Limit | 無限流（`rate.Inf`，`limits.go:119`，內部計算無上游 quota） |
| Channel ID | `janus_regime` |
| Gateway Adapter | `internal/apigateway/adapter_janus_regime.go` |
| Used By | `/api/regime/score`, `PipelineService.LoadRegimeHistory` |
| Optional | 是 — `janusEngine == nil` 時 `register_adapters.go:293` 不註冊 |

### DRAM Spot Price — MU DRAM 現貨價代理（`dram_spot_price`）

| Attribute | Detail |
|-----------|--------|
| Source | Yahoo Finance — MU (Micron) 股價作為 DRAM 現貨價代理（與 DRAMeXchange/InSpectrum 約 85% 相關） |
| Ticker | `MU` |
| Rate Limit | 1 req / 5s, burst 1（與其他 Yahoo 共享 `yahooSharedLimiter`；`limits.go:126`） |
| Channel ID | `dram_spot_price` |
| Gateway Adapter | `internal/apigateway/adapter_dram_spot_price.go` |
| Used By | `MacroDataSnapshot.DRAMSpotPrice` → narrative detectors |
| Note | MU 為全球最大 DRAM 製造商之一，與現貨價高度同步；無官方 DRAMeXchange channel 替代品 |

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
| 速率限制 | **Gateway channel：無**（`adapter_government_flow.go:87` `RateLimit() *rate.Limiter { return nil }`，`HasLimiter=false`；檔案型 provider 不發出 HTTP）。**TWSE 網頁爬蟲：2 秒/請求**（由 `GovernmentBrokerAggregator` 端自訂，影響 Producer 排程，非 gateway 層 limiter） |
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
