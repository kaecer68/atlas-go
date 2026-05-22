# Allowed Environment Variables

本文件為 Constitution 1.2 要求之環境變數白名單。  
任何 `os.Getenv()` 調用必須在本文件中有對應條目，否則為憲法違規。

**最後更新**：2026-05-22  
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
| `ATLAS_STATE_DIR` | 狀態目錄路徑 | `data/state` |
| `ATLAS_LEDGER_DIR` | Ledger 持久化目錄 | `data/state` |
| `ATLAS_MIGRATIONS_PATH` | 資料庫遷移腳本路徑 | `sql/migrations` |
| `ATLAS_SQLITE_PATH` | SQLite 資料庫路徑 | `data/state/atlas.db` |
| `ATLAS_STORE_BACKEND` | 儲存後端 (`jsonl`/`sqlite`) | `jsonl` |
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
| `ATLAS_API_KEY` | API 認證金鑰（非資料源） | 空（未設定時停用認證） |
| `ATLAS_ADMIN_KEY` | 管理員認證金鑰（admin-only 操作） | 空（未設定時停用 admin 驗證） |
| `ATLAS_ENV` | 環境模式（`production`/`development`） | 空 |
| `DATABASE_URL` | PostgreSQL 連線字串 | 空（需手動設定） |
| `HOME` | 使用者家目錄（用於 CLI 工具路徑建構） | 系統預設 |

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

以下變數**禁止在任何業務邏輯中直接 `os.Getenv`**，必須透過 `config.GetSecret()` 讀取：

| 變數名稱 | 用途 | 對應 Provider |
|---------|------|-------------|
| `FUGLE_API_KEY` / `ATLAS_FUGLE_API_KEY` | Fugle API 金鑰 | Fugle |
| `FUBON_API_KEY` / `ATLAS_FUBON_API_KEY` | Fubon API 金鑰 | Fubon |
| `FUBON_PERSONAL_ID` | Fubon 個人 ID（DMA 登入） | Fubon |
| `FUBON_PROXY_URL` | Fubon Python Proxy URL | Fubon |
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
