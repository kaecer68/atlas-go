# Fubon MarketData Proxy 快速指南

## 概述

由於富邦新一代 API 的 Go SDK 不支援行情查詢，我們使用 Python FastAPI 微服務作為代理，讓 Go 程式可以透過 HTTP 呼叫取得富邦即時行情。

## 架構

```
┌─────────────────┐     HTTP      ┌─────────────────────┐
│   Atlas-Go      │ ◄────────────► │  Fubon Proxy        │
│   (Go App)      │                │  (Python FastAPI)   │
│                 │                │                     │
│  FubonClient    │                │  - fubon_neo SDK    │
│  (HTTP Client)  │                │  - DMA Login        │
└─────────────────┘                │  - REST API         │
                                   └─────────────────────┘
```

## 配置

### 1. 環境變數 (.env)

```bash
# 富邦 API Key
FUBON_API_KEY=your_api_key_here

# 身分證號（DMA 登入模式，無需憑證）
FUBON_PERSONAL_ID=your_id_number

# 微服務 URL：docker-compose 預設 127.0.0.1:18081（環境變數 `FUBON_PROXY_PORT` 覆寫）；
# 　　　 standalone `python main.py` 走 main.py 內預設值 18081。
# 　　　 註：2026-06 移除的是 URL env override（`FUBON_PROXY_URL`），port env override 仍在。
```

### 2. Docker Compose

```bash
# 啟動所有服務（包含 fubon-proxy）
docker-compose up -d

# 僅啟動 fubon-proxy
docker-compose up -d fubon-proxy
```

### 3. 本地開發

**`fubon-neo` 不是公開 PyPI 套件**。它是富邦新一代 API 的官方 Python SDK,需於 [`https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk`](https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk) 簽署 API 服務申請書後手動下載(支援 Windows / macOS / Linux,Python 3.8-3.13)。

```bash
cd services/fubon-proxy

# 從官方下載對應平台的 wheel(以 Linux 為例)
curl -O https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/fubon_neo-2.2.8-cp37-abi3-manylinux_2_17_x86_64.manylinux2014_x86_64.zip
unzip -j fubon_neo-2.2.8-cp37-abi3-manylinux_*.zip '*.whl' -d /tmp/fubon_wheel
pip install /tmp/fubon_wheel/*.whl

# 然後裝其他依賴
pip install -r requirements.txt

# 設定環境變數
export FUBON_PERSONAL_ID=your_id
export FUBON_API_KEY=your_key
export FUBON_CERT_PATH=/path/to/your/cert.p12
export FUBON_CERT_PASSWORD=your_cert_password

# 啟動服務
python main.py
```

如果本機已經有現成的 venv(例如 `~/.config/atlas-go/.fubon-env/`,預裝 SDK),可直接用:

```bash
# 啟用既有 venv(SDK 已在裡面)
source ~/.config/atlas-go/.fubon-env/bin/activate
pip install -r requirements.txt
export FUBON_PERSONAL_ID=your_id
export FUBON_API_KEY=your_key
export FUBON_CERT_PATH=~/.config/atlas-go/.fubon-env/your_cert.p12
export FUBON_CERT_PASSWORD=your_cert_password
python main.py
```

## API 端點

| 端點 | 方法 | 說明 |
|------|------|------|
| `/health` | GET | 健康檢查 |
| `/quote/{symbol}` | GET | 取得個股行情 |
| `/quotes?symbols=2330,2317` | GET | 批量取得行情 |
| `/market-status` | GET | 市場狀態 |

## 測試

```bash
# 健康檢查
curl http://127.0.0.1:18081/health

# 取得 2330 行情
curl http://127.0.0.1:18081/quote/2330

# 批量取得
curl "http://127.0.0.1:18081/quotes?symbols=2330,2317,0050"

# 市場狀態
curl http://127.0.0.1:18081/market-status
```

## 故障排查

### 問題：登入失敗
- 確認 `FUBON_PERSONAL_ID` 和 `FUBON_API_KEY` 正確
- 確認 API Key 權限包含行情查詢

### 問題：連線超時
- 確認 fubon-proxy 服務已啟動
- 確認 proxy 監聽：docker-compose 路徑 `127.0.0.1:18081`（env `FUBON_PROXY_PORT`）；standalone `python main.py` 走 main.py 預設 18081

### 問題：無資料返回
- 確認為交易日（非週末/假日）
- 確認股票代碼正確

## 注意事項

1. **Session 管理**：服務會自動管理登入狀態，每小時重新登入
2. **Rate Limit**：遵守富邦 300 req/min 限制
3. **安全性**：API Key 請妥善保管，勿上傳至版本控制

## Docker 部署的關鍵設計

**`Dockerfile` 從富邦官方下載對應平台的 wheel**(`https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/...`),build 階段 `pip install` 進 image。**不需要** mount 整個 `.fubon-env` 進容器(SDK 已在 image 裡,只有 `.p12` 憑證需要 mount)。

```dockerfile
# Dockerfile 關鍵步驟(節錄)
ARG FUBON_NEO_WHEEL_URL=https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/fubon_neo-2.2.8-cp37-abi3-manylinux_2_17_x86_64.manylinux2014_x86_64.zip
RUN curl -fsSL "${FUBON_NEO_WHEEL_URL}" -o /tmp/fubon_neo.zip \
    && unzip -j /tmp/fubon_neo.zip '*.whl' -d /tmp/fubon_wheel \
    && pip install /tmp/fubon_wheel/*.whl
```

升級 SDK 版本時:
1. 到 [官方下載頁](https://www.fbs.com.tw/TradeAPI/docs/download/download-sdk) 確認新版本 + wheel URL
2. 改 `docker-compose.yml` 的 `args:` (FUBON_NEO_VERSION + FUBON_NEO_WHEEL_URL)
3. `docker compose build fubon-proxy` — 重新從官方拉 wheel install

`.p12` 憑證 mount:`docker-compose.yml` 將 `~/.config/atlas-go/.fubon-env` 掛到容器內 `/home/appuser/.config/atlas-go/.fubon-env`,讓 `main.py:_find_cert()` 的 glob 搜尋(預設 `~/.config/atlas-go/.fubon-env`)可以找到憑證。

## 相關檔案

- `services/fubon-proxy/main.py` - Python 微服務主程式(`_find_cert` 在第 64 行,`get_sdk` 在第 102 行)
- `services/fubon-proxy/Dockerfile` - Docker 建置檔(必須用 `python:3.13-slim` 對齊 venv)
- `services/fubon-proxy/requirements.txt` - Python 依賴(故意不含 fubon-neo,見上方)
- `internal/marketdata/fubon_client.go` - Go HTTP 客戶端
- `.env` - 環境變數配置
- `~/.config/atlas-go/.fubon-env/` - 預編譯的 fubon-neo venv
- `docs/ENVIRONMENT.md` § Fubon SDK - 套件下架、wheel 平台限制的完整說明
