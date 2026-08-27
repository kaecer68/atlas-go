# Allowed Environment Variables

本文件為 Constitution 1.2 要求之環境變數白名單。  
任何 `os.Getenv()` 調用必須在本文件中有對應條目，否則為憲法違規。

**最後更新**：2026-07-25
**維護者**：Atlas 數據源治理委員會

---

## 白名單規則

| 類別 | 允許範圍 | 說明 |
|------|---------|------|
| **基礎設施** | 全域 (`cmd/`, `internal/config/`) | 路徑、開關、模式設定，非機密 |
| **資料源 API Key** | 僅限 `internal/config/config.go` | 需透過 `config.GetSecret()` 讀取，禁止在業務邏輯中直接 `os.Getenv` |
| **第三方服務** | 僅限 `internal/config/config.go` | Broker、TWSE API 認證資訊 |

---

## 基礎設施變數（全域允許）

這些變數用於系統路徑、模式設定與功能開關，不涉及機密資訊：

| 變數名稱 | 用途 | 預設值 |
|---------|------|--------|
| `ATLAS_WORK_DIR` | 工作目錄路徑 | `.` |
| `ATLAS_MATURITY_FIRST_START` | maturity tracker 首次啟動日期種子（RFC3339 或 `YYYY-MM-DD`；僅在 tracker 檔不存在時使用，讓 burn-in 時鐘跨資料目錄重建存活） | 空（未設定時用首次啟動當下） |
| `ATLAS_STATE_DIR` | 狀態目錄路徑 | `data/state` |
| `ATLAS_DATA_DIR` | 資料根目錄路徑（provider cache files 等寫入此目錄下的 `state/`） | `/app/data` |
| `ATLAS_LEDGER_DIR` | Ledger 持久化目錄 | `data/state` |
| `ATLAS_EXCHANGE_RATE_CACHE` | ExchangeRate provider 跨日 daily cache 檔案路徑（覆寫預設 `ATLAS_DATA_DIR/state/exchange_rate_daily.json`） | 空（未設定時用 `ATLAS_DATA_DIR/state/exchange_rate_daily.json`） |
| `ATLAS_MIGRATIONS_PATH` | 資料庫遷移腳本路徑 | `sql/migrations` |
| `ATLAS_SQLITE_PATH` | SQLite 資料庫路徑 | `data/state/atlas.db` |
| `ATLAS_STORE_BACKEND` | 儲存後端 (`jsonl`/`sqlite`/`postgres`) | `jsonl` |
| `ATLAS_ENV_FILE` | 自訂 `.env` 檔案路徑 | `.env`（目前目錄） |
| `ATLAS_MARKET_DATA_PROVIDER` | 市場資料提供者選擇 (`twse`/`fugle`/`fubon`/`hybrid`) | `twse` |
| `ATLAS_PRIMARY_MARKET` | 主要市場代碼 | `TW` |
| `ATLAS_REPLAY_MODE` | Replay 模式 (`daily`/`tick`/`weekly`) | `daily` |
| `ATLAS_REPLAY_DATA_PATH` | Replay CSV 檔案路徑 | `samples/replay/twse_stock_day_all_sample.csv` |
| `ATLAS_REPLAY_SESSION_DATE` | 指定 replay session 日期 | 空（自動判斷） |
| `ATLAS_AGENT_REGISTRY_PATH` | Agent 註冊表路徑 | `configs/agents.json` |
| `ATLAS_BASELINE_POLICY_PATH` | Baseline policy 路徑 | `data/state/baseline_policy.json` |
| `ATLAS_PARAMETERS_CONFIG_PATH` | 參數配置路徑 | `configs/parameters.json` |
| `ATLAS_PARAMETERS_CONFIG` | 參數配置路徑（別名） | `configs/parameters.json` |
| `ATLAS_ENGINE_CONFIG` | 引擎配置路徑 | `engine.json` |
| `ATLAS_YAHOO_ENABLED` | Yahoo Finance 功能開關 | `false` |
| `ATLAS_SECTOR_ALLOCATION_CLOSURE_ENABLED` | 產業配置 closure 功能開關（SA08—啟用時 CLI simulation 不寫入 live store） | 空（未設定時使用 legacy live-store sync） |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OpenTelemetry OTLP HTTP exporter endpoint（功能開關：未設定時 fallback stdout） | 空（未設定時使用 stdout） |
| `ATLAS_API_KEY` | API 認證金鑰（非資料源） | 空（未設定時停用認證） |
| `ATLAS_ADMIN_KEY` | 管理員認證金鑰（admin-only 操作） | 空（未設定時停用 admin 驗證） |
| `ATLAS_BASE_URL` | MCP server 用：atlas-go HTTP API 基準 URL（`cmd/atlas-mcp` 端點，非資料源） | `http://127.0.0.1:18080` |
| `ATLAS_MCP_AUDIT_LOG` | MCP server 用：JSONL audit log 路徑 | `$TMPDIR/atlas-mcp-audit.log` |
| `ATLAS_ENV` | 環境模式（`production`/`development`） | 空 |
| `ATLAS_SKIP_DOCKER` | 跳過 Docker-based PostgreSQL 啟動 | 空 |
| `CI` | CI 環境旗標（integration 測試：CI 下 PG 必須可用 → fail loudly；本地缺 PG → skip） | 空 |
| `ATLAS_SKIP_PORT_PREFLIGHT` | 測試環境用：跳過 `internal/startup.Preflight` TCP port 檢查（`cmd/atlas` TestMain 設定；CI 環境常因殘留 `atlas -api` 或平行 atlas.test binary 佔住 port 18080，導致 4 個 live-broker 測試 wedge） | 空 |
| `DATABASE_URL` | PostgreSQL 連線字串 | 空（需手動設定） |
| `HOME` | 使用者家目錄（用於 CLI 工具路徑建構） | 系統預設 |
| `TMPDIR` | 系統臨時目錄（用於 MCP audit log 等預設路徑） | 系統預設 |
| `ATLAS_MCP_TOKEN` | MCP 客戶端認證 token（非資料源） | 空（未設定時 dev mode） |
| `ATLAS_MCP_ADMIN_TOKEN` | MCP admin 管理 API 認證 token（非資料源；空時拒絕所有 admin 請求） | 空 |
| `ATLAS_MCP_ADMIN_ADDR` | MCP admin HTTP API bind 位址（強制 127.0.0.1，server.go 檢查 prefix） | `127.0.0.1:9090` |
| `ATLAS_MCP_METRICS_ADDR` | MCP Prometheus metrics HTTP API bind 位址（強制 127.0.0.1，獨立於 transport port） | 空（未設定時停用） |
| `ATLAS_MCP_ROOTS_ALLOWED` | MCP 客戶端宣告的 roots 路徑白名單（CSV 格式，Phase 4 B 引入） | 空（不限制） |
| `ATLAS_MCP_PARAMS` | MCP 設定檔路徑（讀取 `mcp.roots` section，Phase 4 B 引入） | `configs/parameters.json` |
| `ATLAS_MCP_ROOTS_ALLOW_UNSAFE` | MCP roots 危險路徑驗證的 escape hatch（設為 `1` 時跳過 `/`、`/etc`、`/proc` 等系統根目錄拒絕邏輯；給理解風險的進階使用者使用，issue #903 引入） | 空（預設啟用驗證） |
| `ATLAS_MCP_TRANSPORT` | MCP transport 選擇（`stdio` / `sse` / `streamable-http`；CLI flag `-transport` 優先，env 為 fallback；PR #912 引入） | `stdio` |
| `ATLAS_MCP_ADDR` | MCP sse/streamable-http listen 位址（CLI flag `-addr` 優先，env 為 fallback；PR #912 引入；server.go 強制 127.0.0.1 prefix） | `127.0.0.1:9090` |
| `ATLAS_MCP_AUDIT_RETENTION_DAYS` | MCP audit log 保留天數 | `90` |
| `ATLAS_MCP_RATE_LIMIT_PER_MINUTE` | MCP per-tenant 每分鐘請求上限 | `30` |
| `ATLAS_MCP_RATE_LIMIT_BURST` | MCP per-tenant burst 上限 | `5` |
| `ATLAS_MCP_SAMPLING_ENABLED` | MCP sampling capability 開關 | `false` |
| `ATLAS_MCP_ELICITATION_ENABLED` | MCP elicitation capability 開關 | `false` |
| `ATLAS_MCP_ROOTS_READ_SIZE_CAP` | MCP roots 單次讀取上限（bytes） | `1048576` |
| `ATLAS_MCP_ROOTS_ALERT_ON_CHANGE` | MCP roots 變更時觸發 alert | `false` |
| `FUBON_PROXY_PYTHON` | Fubon proxy Python binary 路徑 | `python3` |
| `ATLAS_L2_4_AUTO_CRON_ENABLED` | L2.4 auto-cron feature flag | `false` |
| `ATLAS_CHARTER_MODE` | 憲章驅動時期化策略/現金控制開關（Phase C2：7 時期偵測 → 策略過濾 + 時期現金保留） | `false` |
| `LLM_ANNOTATOR_API_KEY` | LLM annotator API key（local/CI 測試用；production 必須走 gateway） | 空 |
| `ATLAS_WARMUP` | 開機熱機開關（設為 `0` 停用） | 空（預設啟用） |

---

## Broker 配置變數（僅限 `internal/config/config.go`）

這些變數用於下單系統配置，包含機密資訊，**禁止在 `internal/config/config.go` 以外直接 `os.Getenv`**：

| 變數名稱 | 用途 | 預設值 |
|---------|------|--------|
| `ATLAS_BROKER_MODE` | 下單模式 (`dry-run`/`paper`/`live`) | `dry-run` |
| `ATLAS_BROKER_ADAPTER` | 下單適配器 (`guarded`/`mock`/`http`) | `guarded` |
| `ATLAS_BROKER_MAX_RETRIES` | 最大重試次數 | `1` |
| `ATLAS_BROKER_API_BASE_URL` | Broker API 基礎 URL | 空 |
| `ATLAS_BROKER_API_KEY` | Broker API 金鑰 | 空 |
| `ATLAS_BROKER_API_SECRET` | Broker API 密鑰 | 空 |
| `ATLAS_BROKER_HTTP_TIMEOUT_SEC` | HTTP 請求超時秒數 | `5` |
| `ATLAS_BROKER_HTTP_ATTEMPTS` | HTTP 請求嘗試次數 | `2` |
| `ATLAS_BROKER_HTTP_RETRY_STATUS_CODES` | 可重試的 HTTP 狀態碼 (CSV) | `408,425,429,500,502,503,504` |
| `ATLAS_BROKER_MAX_CLOCK_SKEW_SEC` | 最大時鐘偏差容忍秒數 | `300` |
| `ATLAS_BROKER_NONCE_TTL_SEC` | Nonce 重放保護 TTL 秒數 | `300` |
| `ATLAS_BROKER_NONCE_STORE` | Nonce 儲存後端 (`memory`/`file`/`redis`) | `memory` |
| `ATLAS_BROKER_NONCE_STORE_PATH` | Nonce 檔案儲存路徑 | 空 |
| `ATLAS_BROKER_NONCE_REDIS_URL` | Nonce Redis 連線 URL | 空 |
| `ATLAS_BROKER_NONCE_REDIS_KEY_PREFIX` | Nonce Redis Key 前綴 | `atlas:nonce:` |
| `ATLAS_BROKER_SIGNER` | 簽章演算法 (`placeholder`/`hmac-sha256`) | `placeholder` |
| `ATLAS_BROKER_KEY_ID` | 簽章金鑰 ID | 空 |

---

## TWSE API 配置變數（僅限 `internal/config/config.go`）

| 變數名稱 | 用途 | 預設值 |
|---------|------|--------|
| `ATLAS_TWSE_API_URL` | TWSE API 基礎 URL | 空 |
| `ATLAS_TWSE_API_KEY` | TWSE API 金鑰 | 空 |
| `ATLAS_TWSE_API_SECRET` | TWSE API 密鑰 | 空 |
| `ATLAS_TWSE_ACCOUNT_ID` | TWSE 帳戶 ID | 空 |

---

## 資料源 API Key（僅限 `internal/config/config.go` 透過 `config.GetSecret()` 讀取）
| `FUBON_DMA_PERSONAL_ID` | Fubon DMA 個人 ID | 空 |
| `FUBON_DMA_API_KEY` | Fubon DMA API 金鑰 | 空 |
| `FUBON_DMA_SCRIPT_PATH` | Fubon DMA Python script 路徑 | 空 |
| `FUBON_DMA_PYTHON_PATH` | Fubon DMA Python interpreter 路徑 | `python3` |

以下變數**禁止在任何業務邏輯中直接 `os.Getenv`**，必須透過 `config.GetSecret()` 讀取：

| 變數名稱 | 用途 | 對應 Provider |
|---------|------|-------------|
| `FUGLE_API_KEY` / `ATLAS_FUGLE_API_KEY` | Fugle API 金鑰 | Fugle |
| `FUBON_API_KEY` / `ATLAS_FUBON_API_KEY` | Fubon API 金鑰 | Fubon |
| `FUBON_PERSONAL_ID` | Fubon 個人 ID（DMA 登入） | Fubon |
| `FINMIND_API_KEY` | FinMind API 金鑰 | FinMind |
| `TEJ_API_KEY` | TEJ API 金鑰 | TEJ |

---

## 稽核規則

```bash
# 檢查所有 os.Getenv 是否在白名單中
# 此檢查由 scripts/ci/check_constitution.sh 執行
grep -r "os.Getenv" --include="*.go" . \
  | grep -v "_test.go" \
  | grep -v "internal/config/config.go" \
  > /tmp/env_violations.txt

# 若輸出非空，表示有未授權的 os.Getenv 調用
```

---

## 修訂歷史

| 版本 | 日期 | 修訂內容 |
|------|------|---------|
| v1.0 | 2026-05-22 | 初版，依據 Constitution 1.2 建立完整白名單 |
| v1.1 | 2026-07-01 | 新增 `ATLAS_MCP_ADMIN_TOKEN` 與 `ATLAS_MCP_ADMIN_ADDR`（Phase 3 殘餘 Item 3 — multi-tenant MCP token 管理） |
| v1.2 | 2026-07-01 | 新增 `ATLAS_MCP_METRICS_ADDR`（Phase 4 Direction A — MCP observability）|
| v1.3 | 2026-07-01 | 新增 `ATLAS_MCP_ROOTS_ALLOWED`（Phase 4 B — protocol extensions roots 白名單） |
| v1.4 | 2026-07-01 | 新增 `ATLAS_SKIP_PORT_PREFLIGHT`（測試環境 escape hatch，配套 Phase C T-104） |
| v1.5 | 2026-07-02 | 新增 `ATLAS_MCP_ROOTS_ALLOW_UNSAFE`（issue #903 — MCP roots 危險路徑驗證 escape hatch） |
| v1.6 | 2026-07-28 | 新增 `ATLAS_WARMUP`（啟動 eager warmup 開關，PR #1411） |
| v1.7 | 2026-08-17 | 新增 `ATLAS_MATURITY_FIRST_START`（maturity tracker 啟動日期種子，PR #P0-F'） |
