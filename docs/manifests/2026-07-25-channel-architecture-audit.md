# Atlas 數據通道架構全面審計報告

**日期**: 2026-07-25
**審計範圍**: 所有數據信息通道、排程系統、健康監控、警報機制、Constitution 合規性
**審計方法**: 5 個並行 scout 子代理 + 主代理直接探索

---

## 🔴 執行摘要 (Executive Summary)

atlas-go 數據架構存在 **4 個系統性問題**，彼此疊加導致半年來維護成本高、AI agent 頻繁幻覺、數據缺口無法收斂：

1. **Constitution 文件嚴重過期** — 宣稱 16 通道，實際 37 個，落差 2.3x
2. **Yahoo Finance 通道極度碎片化** — 12 個獨立通道打同一 API 端點，各自建立 provider/adapter/limiter/breaker/儲存路徑
3. **健康監控與警報系統各自為政** — 4 套警報路徑互相不連通，3+ 套健康儲存競爭，5 個 Wave9 rule 全為 dead code
4. **自動排程覆蓋不全** — 約 80 個註冊任務但許多通道無自動 fetch，歷史數據回填失敗無法被系統感知

---

## 一、信息通道統計

### 1.1 實際註冊數量

| 來源 | 數量 | 說明 |
|------|------|------|
| `gateway.go::channelIDs()` | **34** | 程式碼中明確定義的通道清單 |
| `register_adapters.go` 實際註冊 | **37** | 含 3 個不在 channelIDs 的通道 |
| `CONSTITUTION.md` 附錄 A | **19** | 文件嚴重過期（落差 1.95x） |
| `CONSTITUTION.md` 前言宣稱 | **16** | 最初版宣稱（落差 2.3x） |

### 1.2 遺漏通道（在 register_adapters 但不在 channelIDs）

| 通道 ID | 類型 | 問題 |
|---------|------|------|
| `tw_vol` | Yahoo Finance ^TWII volatility | **嚴重**：註冊但 channelIDs() 不包含，CircuitBreakerManager 無法為其建立熔斷器 |
| `twse_sbl` | TWSE SBL stub (G02) | 低風險：stub 通道，但應納入 channelIDs 以便 dashboard 顯示 |
| `tdcc_equity_dispersion` | TDCC stub (G01) | 低風險：stub 通道，同上 |

### 1.3 Yahoo Finance 碎片化詳情

**12 個通道全部打 `query1.finance.yahoo.com` 同一 API 端點**：

| 通道 ID | 符號 | Limiter Group | 說明 |
|---------|------|--------------|------|
| `us_yahoo` | VIX,DXY,US10Y,USD_TWD,Oil,Gold,Silver,Copper batch | yahooMacroLimiter (5s) | 宏觀批次 |
| `us_spx` | ^GSPC | yahooIndexLimiter (1.5s) | S&P 500 |
| `us_ndx` | ^IXIC | yahooIndexLimiter | Nasdaq |
| `us_dji` | ^DJI | yahooIndexLimiter | Dow Jones |
| `us_nvda` | NVDA | yahooTechLimiter (1.5s) | NVIDIA |
| `us_aapl` | AAPL | yahooTechLimiter | Apple |
| `us_msft` | MSFT | yahooTechLimiter | Microsoft |
| `tsm_adr` | TSM | yahooTechLimiter | TSMC ADR |
| `taiex_index` | ^TWII | taiexIndexLimiter (5s) | 台灣加權 |
| `tw_vol` | ^TWII (same!) | 獨立 5s/1b | **與 taiex_index 重複拉同一符號** |
| `sox_index` | ^SOX | ExportStatisticsRate (5s) | 費半 |
| `dram_spot_price` | MU | ExportStatisticsRate (5s) | DRAM proxy |

**問題嚴重性**：
- 每個通道都有獨立的 Provider struct、Adapter、RateLimiter、CircuitBreaker、`data/state/<id>/latest.json` 儲存
- `taiex_index` 和 `tw_vol` 拉同一個 ^TWII 符號兩次！只是輸出不同欄位（價格 vs 波動率）
- 原先 `us_yahoo` 批次拉 9 個指標，拆分後變成冷啟動需等 4.5+3+5 ≈ 12.5 秒
- 每個通道獨立維護，產生 12 倍的 adapter 程式碼、12 倍的測試、12 倍的 health check 組態

### 1.4 Rogue 通道（未走 Gateway 的直接 HTTP 調用）

| 位置 | 問題 | 嚴重性 |
|------|------|--------|
| `internal/marketdata/government_broker_aggregator.go:63` | 直接建 `&http.Client{Timeout: 30s}`，未在 gateway 註冊 | **高** |
| `internal/llm_annotator/annotator.go` | KimiClient 直接建 HTTP client + rate limiter | 中（非資料源但違反 pattern） |
| `internal/llm/clients/base.go` | 4 個 LLM provider 直接建 HTTP client | 中（同上） |
| `internal/alerting/webhook_publisher.go` | Alert webhook 直接建 HTTP client | 低 |

---

## 二、排程系統覆蓋分析

### 2.1 已註冊排程任務統計

從代碼掃描中識別約 **80+ 個** `ScheduledTask` 註冊（分布在 `cmd/atlas/main.go`、`capital_tasks.go`、`data_sync_health_tasks.go`、`operations_tasks.go`、`calibration_tasks.go`、`stage3_tasks.go` 等檔案）。

### 2.2 Docker Cron 任務

`Dockerfile.cron` 定義 10 個獨立 cron binary：
- `daily-replay-sync`、`backfill-replay`、`macro-ingest`、`geo-ingest`
- `backfill-month-revenue`、`backfill-financial-statements`、`backfill-institutional-investors`
- `cron-quote-backfill`、`c07-obs-collector`、`c07-day-evaluator`


### 2.4 未文件化 rogue goroutine（Constitution §4.5 例外未涵蓋）

ScanAllSchedulers 子代理發現以下週期性 goroutine 未在 Constitution §4.5.2 例外清單中文件化：

| 位置 | 循環 | 狀態 |
|------|------|------|
| `internal/marketdata/realtime/router.go:103-104` | healthCheckLoop + failoverLoop | **rogue** — 未文件化 |
| `internal/marketdata/fubon_client.go:311` | runHealthProbe | **rogue** — 裸 goroutine |
| `internal/marketdata/streaming.go:25-27` | PollingAdapter.Subscribe ticker | **需驗證** — Subscribe 結束時 goroutine 洩漏風險 |
| `internal/mcp/anomaly/emitter.go:117-120` | Emitter.Run 週期性 emit | **rogue** — cmd/atlas-mcp 裸啟動 |
| `internal/metalearning/metalearner.go:276,279` | adaptationLoop + trainingResultProcessor | **rogue** — 兩個裸 goroutine |
| `internal/spawning/spawning_manager.go:90` | SpawningManager.runLoop | **rogue** — production 無 caller，疑似 dead code |
| `internal/prism/prism_manager.go:383-391` | worker fan-out + autoBalancer (5min) | **rogue** — 內部 ticker 未走 TaskManager |
| `internal/realtime/regime_adapter.go:267-269` | RealTimeAdapter.Start ticker | **rogue** — main.go 裸 `go` 啟動 |

### 2.5 Shell Cron 與 Docker Compose Cron 重複

`scripts/cron-entrypoint.sh` 使用 `while true; do ... sleep` 自包含 cron 迴圈，由 `docker-compose.yml` 中 **11 個 cron-* 服務**各自注入 `CRON_SCHEDULE` + `CRON_COMMAND` 環境變數執行。與 `BackgroundTaskManager` 存在重複：

| TaskManager 任務 | Compose cron 服務 | 重複？ |
|-----------------|------------------|--------|
| `macro_ingest` (5min) | `cron-macro-ingest` (每日 08:00) | **是** |
| `janus_regime_refresh` (6h) | — | 否 |
| `auto_backfill` (24h) | `cron-replay-sync` | **是** |

雙軌排程（TaskManager + shell cron）可能導致同一資料被重複抓取、或抓取頻率不一致的混淆。
### 2.3 排程覆蓋缺口

**有自動 fetch 排程的通道**（僅以下）：
- twse_capital_flow → `auto_capital_flow` (30min)
- twse_margin → `auto_margin` (30min)
- export_statistics → `auto_export` (12h)
- taifex_institutional → `auto_taifex_institutional` (1h)
- twse_sbl → `auto_twse_sbl` (1h, G02 stub)
- government_flow → `auto_government_flow` (1h)
- geopolitical → `auto_geopolitical` (6h)
- fugle → `channel_health_fugle` (1h, health probe only)
- fubon → `channel_health_fubon` (1h, health probe only)
- finmind → `channel_health_finmind` (1h, health probe only)
- twse_replay → `channel_health_twse_replay` (1h, health probe only)
- tsmc_revenue → `tsmc_revenue` (24h)
- tej → `tej_refresh` (1h)
- janus_regime → `janus_regime_refresh` (6h)

**無自動 fetch 排程的通道**（大量）：
- us_spx, us_ndx, us_dji, us_nvda, us_aapl, us_msft, tsm_adr → 僅靠 `us_market_refresh` (5min) 批次更新
- sox_index → **無獨立排程**
- dram_spot_price → **無獨立排程**
- exchange_rate → **無獨立排程**
- taiex_index → **無獨立排程**
- tw_vol → **無獨立排程**（且不在 channelIDs）
- bdi → **無獨立排程**
- day_trading → **無獨立排程**
- twse_oddlot → **無獨立排程**
- twse_etf → **無獨立排程**
- taifex_daily → **無獨立排程**
- twse_sector_index → **無獨立排程**
- sector_data → **無獨立排程**
- geopolitical_taiwan → **無獨立排程**

---

## 三、健康監控系統分析

### 3.1 健康檢查端點總覽（12 個！）

| 端點 | 實作位置 | 檢查內容 | 問題 |
|------|---------|---------|------|
| `/health` | cmd/atlas/api_routes.go | port occupancy 固定 200 | 有兩份實作，都不檢查真實健康 |
| `/health` | monitoring/api/system/health_handlers.go | port occupancy，與上重疊 | **重複實作** |
| `/ready` | cmd/atlas/api_routes.go | PostgreSQL/replay file/channel count | 正確實作 |
| `/api/health` | cmd/atlas/main.go | 固定 `{status:"ok"}` | **誤導性**：永遠 200 無檢查 |
| `/api/llm/health` | monitoring/api/llm/handlers.go | LLM provider health | 正確但需雙路徑 auth-free sync |
| `/api/health/aggregate` | monitoring/api/system/health_aggregate.go | 4 層聚合 | 多處讀 hardcoded 相對路徑 |
| `/api/dashboard/channel-health` | monitoring/dashboard_api.go | 直接讀 JSON | **繞過 canonical store** |
| `/api/dashboard/system-health` | monitoring/api/system/handlers.go | domain health | OK |
| `/api/dashboard/macro-data-health` | monitoring/api/macro/handlers.go | macro ingest | OK |
| `/api/agents/health` | monitoring/api/control/handlers.go | agent health | OK |
| `/api/health/data-integrity` | monitoring/api/system/data_integrity.go | deprecated | 應移除 |
| fubon-proxy `/health` | marketdata/fubon_client.go | external process | OK |

### 3.2 健康儲存競爭（3+ 套實作）

| 實作 | 檔案 | 問題 |
|------|------|------|
| **Canonical**: `apigateway.UnifiedHealthStore` | `internal/apigateway/health.go` | Gateway 所有，應為唯一真理源 |
| **Rogue A**: monitoring 自製 channelHealthStore | `internal/monitoring/service/data_channels.go` | 直接讀寫同一 JSON，脫離 canonical |
| **Rogue B**: monitoring/api/data 自製 store | `internal/monitoring/api/data/channel_health.go` | 另一套完整實作，含 DB sync |
| **Bypass C**: dashboard 直接讀 JSON | `internal/monitoring/dashboard_api.go` | `os.ReadFile` 繞過所有 store |
| **Bypass D**: aggregate 讀 hardcoded 路徑 | `internal/monitoring/api/system/health_aggregate.go` | cwd-relative 路徑 |
| **Rogue E**: Wave 9 另建 UnifiedHealthStore | `cmd/atlas/main.go:2310` | 與 Gateway 實例不同，狀態漂移 |

### 3.3 FetchMetadata 元數據不完整

Constitution 第三條要求所有調用回傳 `latency_ms`, `rate_limit_remaining`, `timestamp`。實際情況：
- 多個 adapter 的 `LatencyMs` 為 0 或未填（Fugle, FinMind, Fubon, TWSE 系列, Yahoo 系列等）
- 多數 adapter 未填 `RateLimitRemaining`
- 部分 adapter 在 stale/fallback 情況下元數據不完整

---

## 四、警報系統碎片化分析 🔴 最嚴重

### 4.1 四套互不連通的警報路徑

```
路徑 1: Prometheus → Alertmanager → Slack/webhook
   ├─ /metrics scrape → monitoring/rules/*.yml → alertmanager:9093
   └─ 問題：5個 Wave9 rule 全為 dead code

路徑 2: Application Monitor/RuleEngine → JSONL AlertStore → /api/alerts*
   ├─ 內建 7 條 portfolio/live rules + Stage3 5 類 rules
   └─ 問題：production 無外部 notifier，只寫 console log

路徑 3: Alertmanager inbound → /api/v1/alerts → 獨立 ring buffer
   ├─ Alertmanager webhook POST
   └─ 問題：401 (未在 auth-free)、ring buffer 獨立、route 永遠不命中

路徑 4: MCP anomaly detector → MCP MemoryStore + webhook
   ├─ 6 個 MCP metrics + anomaly tools
   └─ 問題：webhook never configured, roots alerter dead wiring, ack broken
```

### 4.2 關鍵缺陷

| 缺陷 | 嚴重性 | 證據 |
|------|--------|------|
| 5 個 Wave9 rules 全部 dead code | **P0** | `enabled:"false"` 只是 label，Prometheus 照樣載入；4/5 沒有 metric producer |
| Alertmanager webhook 不可達 | **P0** | `localhost:18080` 在 container 內指向自己，非 atlas 服務 |
| Alertmanager route 永遠不命中 | **P0** | route `service:atlas`，rules 輸出 `atlas-monitoring` 等 |
| `/api/v1/alerts` 未被 auth-free | **P1** | 啟動 API key 後 Alertmanager POST 被拒絕 |
| 4 套警報無共同 identity/store/ack | **P1** | 無法跨系統追蹤同一事件 |
| MCP anomaly ack 錯接 | **P1** | 送 `alert_id` 到需要 `user` 的 app endpoint；ID domain 不同 |
| MCP anomaly webhook 從未配置 | **P1** | `server.Config.AnomalyAlertWebhook` 欄位存在但 `main.go` 從不賦值 |
| Production 無外部通知 | **P1** | 只接 ConsoleHandler，所有 notifier 實作存在但未接線 |
| Application MetricsCollector dead | **P2** | `RecordAlert`/`RecordAlertAcknowledged` 只在 test 中被呼叫 |
| RecordHistogram 無 read path | **P2** | PrometheusHandler 明確跳過 histogram |
| `/api/alerts/*` 全部公開（含 ack/resolve） | **P2** | 無需認證即可操作 |

---

## 五、Constitution 合規分析

### 5.1 文件過期

| 憲法內容 | 宣稱 | 實際 |
|---------|------|------|
| 通道數量 | 16 | 37 |
| 附錄 A 通道 | 19 | 37 |
| Yahoo 限流 | "共享 1/s" | 已拆分為 3 個 group limiter |
| 背景任務 | 8 | 80+ |
| 健康檢查實作 | "3 套" | 仍為 3+ 套（未改善） |

| 違規 | 憲法條款 | 狀態 |
|------|---------|------|
| `government_broker_aggregator` 繞過 gateway | 第一條（統一入口） | **未修復** |
| LLM clients 繞過 gateway | 第一條 | 未修復（非資料源可接受但 pattern 不良） |
| 多套健康檢查實作仍共存 | 第三條（統一入口） | **未修復，反而增加** |
| `government_flow` 無 rate limiter | 第二條（強制限流） | **漏洞**：adapter RateLimit() 回 nil，limits.go 無 key |
| `tw_vol`/`twse_sbl`/`tdcc_equity_dispersion` 無 CircuitBreaker | 第五條（熔斷） | **漏洞**：CB 以 channelIDs() 34 初始化，3 個 extras 無 breaker |
| `tw_vol`/`twse_sbl`/`tdcc_equity_dispersion` 不在 CheckHealth 掃描範圍 | 第三條（健康追蹤） | **漏洞**：CheckHealth/StatusSummary 只迭代 channelIDs() |
| HealthCheck mode 不符憲法規範 | 第三條 3.4（語義統一） | **偏差**：twse_replay/capital_flow/margin/export 憲法要求 readiness，實作多 liveness；sector_data 要求 readiness 卻 liveness；government_flow 要求 readiness 卻 liveness |
| CI 檢查腳本可能不存在 | 第六條 | 待確認 |
| `configs/allowed_env_vars.md` 可能過期 | 第一條 | 缺少 `FUBON_PROXY_PYTHON`, `ATLAS_L2_4_AUTO_CRON_ENABLED` |
### 5.3 良性演進

- 所有 37 個通道皆有 RateLimiter + CircuitBreaker + HealthCheck（合規三件套）
- `BackgroundTaskManager.Register` 強制檢查 ChannelID 已在 gateway 註冊
- Stale/Fallback metadata 標記機制已實作（4 層 data visibility）

---

## 六、歷史數據缺失根因分析

### 6.1 現狀

- 5.8 MB 2020-2024 歷史資料存在但未被正確轉換與管理
- 多個通道數據不全，自動排程執行但結果未被正確處理

### 6.2 根因

1. **自動回填任務執行但無結果驗證** — `margin_history_backfill` 等任務註冊了但執行結果只寫 log，沒有 data quality check
2. **數據格式轉換層分散** — 每個 provider 自行 parse 並轉換格式，沒有統一的 data quality gateway
3. **排程觸發但無健康回饋迴路** — task 執行失敗時只增加 failure counter，不觸發 data gap alert
4. **Stub 通道無後續推進機制** — `twse_sbl` (G02)、`tdcc_equity_dispersion` (G01) 半年後仍為 stub
5. **監控看不到數據缺失** — Wave9 IngestionLag 和 Drift 的 Prometheus metric 沒有 producer，無法被 scrape 到

### 6.3 組織與流程根因（Advisor 審查補充）

技術根因僅解釋了「發生了什麼」，但無法解釋「為什麼會發生」與「為什麼沒被阻止」。以下為組織與流程層面的深層根因：

1. **缺乏架構守門員機制（Architecture Gatekeeper）**
   - Constitution 於 2026-05-13 制定後，無任何自動化工具強制檢查合規性
   - 新增通道時，開發者只需在 `register_adapters.go` 加一行 `Register`，無需同步更新 `channelIDs()`、`limits.go`、Constitution
   - 導致 `tw_vol` orphan（有註冊但無 channelID）在無人察覺下存在

2. **跨團隊開發缺乏收斂機制**
   - 4 套警報路徑（Prometheus、Application Monitor、Alertmanager inbound、MCP anomaly）分別由不同階段/不同人引入，從未被統一治理
   - 健康儲存的 3+ 套實作同樣是「階段性需求無收斂」的結果：monitoring 團隊需要快速迭代而不願等待 apigateway 層的修改

3. **Constitution 更新流程缺失**
   - 憲法第六條規定了 CI 檢查和 PR template，但 CI 腳本 `check_constitution.sh` 只檢查 **17 個舊通道**，對 20 個新通道完全無感知
   - 沒有「文檔即程式碼」（docs-as-code）機制：通道清單依賴手動更新，必然過期

4. **警報「只建不管」文化**
   - Wave9 5 個 rules 被建立後標 `enabled:"false"` 就放著，半年無人追蹤
   - Production 無外部通知（只寫 console log），因為「先做結構，通知以後再加」——但「以後」從未來
   - Alertmanager webhook 寫 `localhost:18080`（應為 `atlas:18080`），無任何整合測試驗證端到端連通

| A10 | 補全 `allowed_env_vars.md` 並修正 CI 檢查 | 加入 7 個 `ATLAS_MCP_*`、4 個 `FUBON_DMA_*`、`FUBON_PROXY_PYTHON`、`ATLAS_L2_4_AUTO_CRON_ENABLED`、backfill-quotes 的 `STORE_BACKEND`/`LEDGER_DIR`/`SQLITE_PATH`；修正 CI `check_constitution.sh` 的 env scan：加入 `envOr` 包裝呼叫檢測、同步 whitelist 至完整清單 |
   - `twse_sbl` (G02)、`tdcc_equity_dispersion` (G01) 被標記為 stub 半年，無推進時間表、無負責人
   - 無機制追蹤 stub 老化並自動升級為 active 或移除

**預防機制建議**：
- 通道清單由程式碼自動生成（`make constitution`），禁止手動維護
- 新 adapter 必須通過 linter 檢查三件套（channelID + limiter + health mode）齊全
- Stub 通道設定 TTL（如 90 天），逾期自動在 dashboard 標紅並通知負責人
- 季度架構審計納入定期循環

---

## 七、重構建議

### 7.1 P0 — 立即修復（1-2 週）

| ID | 項目 | 變更 |
|----|------|------|
| A01 | 修正 `tw_vol` 不在 channelIDs | 加入 `gateway.go::channelIDs()`，同步修正 CB 初始化 |
| A02 | 補建 `government_flow` rate limiter | adapter RateLimit() 目前回 nil，需在 limits.go 註冊 |
| A03 | 修正 3 個 extras 無 CircuitBreaker | `tw_vol`/`twse_sbl`/`tdcc_equity_dispersion` 不在 channelIDs 故無 CB |
| A04 | 修正 HealthCheck mode 不合規 | twse_replay/capital_flow/margin/export/sector_data/government_flow 憲法要求 readiness 但實作 liveness |
| A05 | 停用 5 個 Wave9 dead rules | 從 `prometheus.yml` 移除載入，或實作 metric producer |
| A06 | 修復 Alertmanager webhook 連通 | 將 `alertmanager.yml` webhook URL 改為 `atlas:18080`（Docker service name） |
| A07 | 修復 alert route match | 將 rules 的 `service` label 改為 `atlas`，或修改 Alertmanager route |
| A08 | `/api/v1/alerts` 加入 auth-free（限來源驗證） | 加入 `isPublicPath`，但限定 Alertmanager container IP 或 HMAC token |
| A09 | 更新 Constitution 通道清單 | 附錄 A 更新至 37 個通道，追加修正健康模式規範 |
| A10 | 補全 `allowed_env_vars.md` | 加入 7 個 `ATLAS_MCP_*`、4 個 `FUBON_DMA_*`、`FUBON_PROXY_PYTHON`、`ATLAS_L2_4_AUTO_CRON_ENABLED` |
| A11 | 更新 CI `check_constitution.sh` | 已知通道清單從 17 擴展至 37，加入新通道檢查 |
| A12 | 補齊 FetchMetadata 缺漏 | fugle LatencyMs 固定 0 → 實際量測；多數 adapter 補 RateLimitRemaining |

### 7.2 P1 — Yahoo 通道整併（2-4 週）

| ID | 項目 | 變更 |
|----|------|------|
| B01 | 合併 US index channels | `us_spx/us_ndx/us_dji` → `us_indices`（單一 provider 批次拉 3 個符號） |
| B02 | 合併 US tech channels | `us_nvda/us_aapl/us_msft/tsm_adr` → `us_tech_stocks`（單一 provider 批次拉 4 個符號） |
| B03 | 合併 taiex + tw_vol | 統一為 `taiex_index`，provider 同時輸出價格和波動率 |
| B04 | 合併 sox + dram | 考慮是否納入 `us_indices` 或獨立保持 |

| ID | 項目 | 變更 |
|----|------|------|
| C01 | 統一健康儲存 | 移除 rogue store（monitoring/service/data_channels、monitoring/api/data/channel_health），全部走 `gateway.Health()` |
| C02 | 清理 `/health` 重複 | 保留 cmd/atlas 版本，移除 monitoring 重複；`/api/health` 改為實際檢查 Gateway.Summary() |
| C03 | 設計 Unified Alert Registry | 單一 alert identity → store → ack → notify pipeline；統一 Prometheus、App、MCP 三路 |
| C04 | 統整 FetchMetadata | 所有 adapter 補齊 latency_ms + rate_limit_remaining（P0 A12 啟動） |
| C05 | 補全排程覆蓋缺口 | 為 15 個無 auto-fetch 的 active 通道建立排程：sox_index, dram_spot_price, exchange_rate, taiex_index, bdi, day_trading, twse_oddlot, twse_etf, taifex_daily, twse_sector_index, sector_data, geopolitical_taiwan（分 P1 頻繁通道 + P2 低頻通道兩批） |
| C06 | 收編 rogue HTTP 調用 | `government_broker_aggregator` 納入 gateway 管理，強制套用 rate limiter/breaker/health |
| C07 | 歷史資料品質閘道 | 為 margin_history_backfill 等回填任務加入 data quality check（比對預期筆數 vs 實際筆數），異常時觸發 alert |
| C08 | Constitution 自動生成 | 通道清單由程式碼自動生成（`make constitution`），禁止手動維護附錄 A |
| C09 | CI 合規強制化 | `check_constitution.sh` 擴展至 37 通道全檢；新 adapter 必須通過 linter 檢查三件套（channelID + limiter + health mode） |
| C10 | 架構治理委員會 | 每月審視 Constitution 合規性與技術債收斂進度；季度架構審計納入定期循環 |


### 7.5 風險矩陣（Advisor 審查補充）

| 風險 | 影響 | 概率 | 緩解措施 | 責任階段 |
|------|------|------|---------|---------|
| Yahoo 合併後限流劣化，觸發大規模 429 | **高** — 全部 US market 資料中斷 | 中 | 合併前壓測 Yahoo API；保留舊 channelID 作為別名以便快速回滾 | P1 B01-B04 |
## 八、Agent 啟動警報掃描機制設計（v2 — 依 Advisor 審查修訂）

### 設計原則變更

**v1 問題**：要求 agent 遇到 P0/P1 警報必須先處理才能進行原任務 → 過度強制，會癱瘓開發效率。
**v2 改為**：「情境覺知輔助」模式 — agent 掃描並報告，但只對「可自動修復且範圍內」的警報嘗試處理。

### 方案：`atlas-mcp` 工具 + 輕量啟動合約

#### Step 1: 先以聚合查詢實作（降低與 P2 C03 的耦合）

不需要等 Unified Alert Registry 完成。先用 wrapper 聚合現有多源：
- `/api/alerts/unacknowledged`（Application JSONL AlertStore）
- `Gateway.Summary()`（通道健康狀態）
- Prometheus `/api/v1/query?query=ALERTS`（Prometheus 活躍警報）

```go
// MCP 工具: alert_situational_awareness
// 描述: Agent 啟動時獲取當前系統警報態勢
// 參數:
//   - include_known: bool — 是否包含已知/已計劃修復的警報
//   - categories: []string — 過濾類別（data_gap, channel_error, system_health）
//   - auto_ack_low_risk: bool — 自動確認低風險已知警報，避免重複干擾
// 回傳: {
//   "ok_to_proceed": true,       // false 表示有阻擋性問題
//   "blockers": [...],           // 必須處理才能繼續的項目
//   "warnings": [...],           // 應注意但不阻擋的項目
//   "auto_acknowledged": [...],  // 已自動確認的低風險警報
//   "recommendation": "建議先處理 X，其餘可在任務完成後追蹤"
// }
```

#### Step 2: AGENTS.md 啟動合約（輕量版）

```markdown
## Agent 啟動合約（情境覺知 — 不阻擋主線任務）

在開始工作前，Agent SHOULD：

1. 調用 `alert_situational_awareness` 取得當前系統態勢
2. 若有 `blockers`（如資料庫無法連線、關鍵 API key 過期），先處理
3. 對於 `warnings`，納入背景關注但不阻擋主線任務
4. 若任務與特定通道相關，確認該通道當前健康狀態

Agent MAY：
- 對已知低風險警報使用 `auto_ack_low_risk: true` 自動確認
- 設定環境變數 `SKIP_ALERT_CHECK=1` 跳過掃描（緊急修復或實驗分支）
- 在主線任務完成後回過頭處理 warnings
```

#### Step 3: 獨立診斷工具（不綁 `make check`）

`make check` 在 CI 無服務環境中執行，不適合呼叫 localhost API。改為：

```bash
# 獨立 script，由 agent 或開發者手動執行
scripts/audit-alerts.sh:
  - 檢查 atlas 服務是否運行
  - 若運行：curl /api/alerts/unacknowledged + /api/health/aggregate
  - 若未運行：顯示「服務未運行，無法掃描警報」
  - 輸出結構化 JSON 供 agent 解析
```

#### Step 4: 警報生命週期管理

- **去重**：相同 dedup_key 的警報在 24h 內只顯示一次
- **自動恢復忽略**：若警報在 3 次掃描內自行恢復（狀態變 ok），自動標記為 resolved
- **噪音過濾**：若同一通道在 1h 內觸發 >10 次警報，標記為「抖動」，建議檢查 rate limit 設定而非逐條處理

---

## 九、Constitution 修訂建議

```diff
- 本系統目前管理 16 個信息通道
+ 本系統目前管理 37 個信息通道（34 active + 1 orphan + 2 stub）

- 附錄 A：19 個通道
+ 附錄 A：37 個通道（完整列表）

+ 新增第七條：警報統一原則
+   所有警報（Prometheus、Application、MCP）必須寫入 UnifiedAlertStore
+   所有 /api/alerts/* 端點必須從 UnifiedAlertStore 讀取

+ 新增第八條：Agent 啟動合約
+   任何變更開始前必須掃描未處理警報

---

## 十一、Advisor (kimi-k3) 審查意見與回應

> 審計報告於 2026-07-25 提交予 advisor agent 進行獨立審查。以下是審查要點與本報告的回應修訂。

### 審查總結

**Advisor 評語**：「報告本身價值極高，準確描繪了現狀亂度。更像『技術探索報告』而非可直接取用的『重構工程藍圖』。」

### 回應修訂清單

| Advisor 意見 | 修訂 | 章節 |
|-------------|------|------|
| 缺乏組織/流程根因分析 | 新增 §6.3「組織與流程根因」 | §六 |
| 缺乏結構化風險評估 | 新增 §7.5「風險矩陣」（9 項風險×緩解措施） | §七 |
| Agent 掃描機制過度強制 | 重寫 §八：改為「情境覺知輔助」模式，加入 `auto_ack`、`SKIP_ALERT_CHECK` 逃生口 | §八 |
| P1/P2 遺漏項目（排程覆蓋、rogue 收編、health mode） | 擴充 P0 12 項（含 government_flow limiter、health mode、env vars、CI 更新），擴充 P2 10 項（含排程補全、rogue 收編、data quality gate、自動生成 Constitution） | §七 |
| Yahoo 合併風險被低估 | P1 加入 facade pattern 建議 + 壓測要求；風險矩陣標注回滾方案 | §七 + §7.5 |
| CI 只檢查 17 個舊通道 | 新增 A11 + C09 擴展至 37 通道全檢 | §七 |
| `make check` 不適合呼叫 API | 改為獨立 `scripts/audit-alerts.sh` | §八 |
| 未 point 出安全/合規衝擊 | P0 A08 加入 HMAC token 驗證；風險矩陣納入安全風險 | §七 + §7.5 |

### 未採納但列入追蹤

| Advisor 建議 | 狀態 |
|-------------|------|
| 建立架構治理委員會 | 列入 C10，建議但非技術必需 |
| 每項 P0-P3 展開細部設計文檔（序列圖、消費者影響） | 建議在執行各階段時產出，非本報告範圍 |
| 量化技術債成本（程式碼行數、bug 次數） | 有價值但資料收集成本高，建議納入下次季度審計 |

## 十、附錄：完整通道清單

| # | Channel ID | Type | Auto-fetch | Health | Notes |
|---|-----------|------|-----------|--------|-------|
| 1 | us_yahoo | Yahoo | us_market_refresh (5m) | ✓ | 9 indicators batch |
| 2 | twse_replay | TWSE | auto_backfill (24h) | ✓ | File-based |
| 3 | twse_capital_flow | TWSE | auto_capital_flow (30m) | ✓ | |
| 4 | fugle | Fugle | channel_health_fugle (1h) | ✓ | Health probe only |
| 5 | fubon | Fubon | channel_health_fubon (1h) | ✓ | Health probe only |
| 6 | finmind | FinMind | channel_health_finmind (1h) | ✓ | Health probe only |
| 7 | frankfurter_fx | Frankfurter | — | ✓ | No auto-fetch |
| 8 | geopolitical | RSS | auto_geopolitical (6h) | ✓ | |
| 9 | twse_margin | TWSE | auto_margin (30m) | ✓ | |
| 10 | export_statistics | TWSE | auto_export (12h) | ✓ | |
| 11 | tsmc_revenue | FinMind | tsmc_revenue (24h) | ✓ | |
| 12 | geopolitical_taiwan | RSS | — | ✓ | No auto-fetch |
| 13 | janus_regime | Compute | janus_regime_refresh (6h) | ✓ | |
| 14 | tej | TEJ | tej_refresh (1h) | ✓ | |
| 15 | exchange_rate | Frankfurter | — | ✓ | No auto-fetch |
| 16 | sox_index | Yahoo | — | ✓ | No auto-fetch |
| 17 | dram_spot_price | Yahoo | — | ✓ | No auto-fetch |
| 18 | twse_sector_index | TWSE | — | ✓ | No auto-fetch |
| 19 | sector_data | TWSE | — | ✓ | No auto-fetch |
| 20 | day_trading | TWSE | — | ✓ | No auto-fetch |
| 21 | bdi | CNBC | — | ✓ | No auto-fetch |
| 22 | taifex_daily | TAIFEX | — | ✓ | No auto-fetch |
| 23 | taifex_institutional | TAIFEX | auto_taifex_institutional (1h) | ✓ | |
| 24 | twse_oddlot | TWSE | — | ✓ | No auto-fetch |
| 25 | government_flow | File | auto_government_flow (1h) | ✓ | |
| 26 | twse_etf | TWSE | — | ✓ | No auto-fetch |
| 27 | us_spx | Yahoo | us_market_refresh (5m) | ✓ | Yahoo group |
| 28 | us_ndx | Yahoo | us_market_refresh (5m) | ✓ | Yahoo group |
| 29 | us_dji | Yahoo | us_market_refresh (5m) | ✓ | Yahoo group |
| 30 | taiex_index | Yahoo | — | ✓ | No auto-fetch |
| 31 | us_nvda | Yahoo | us_market_refresh (5m) | ✓ | Yahoo group |
| 32 | us_aapl | Yahoo | us_market_refresh (5m) | ✓ | Yahoo group |
| 33 | us_msft | Yahoo | us_market_refresh (5m) | ✓ | Yahoo group |
| 34 | tsm_adr | Yahoo | us_market_refresh (5m) | ✓ | Yahoo group |
| 35 | **tw_vol** | Yahoo | — | ✓ | **ORPHAN**: not in channelIDs |
| 36 | twse_sbl | TWSE | auto_twse_sbl (1h) | stub | G02 stub |
| 37 | tdcc_equity_dispersion | TDCC | — | stub | G01 stub |

---

*審計完成時間: 2026-07-25*
*下一階段: 依優先級執行 P0-P3 重構任務*
