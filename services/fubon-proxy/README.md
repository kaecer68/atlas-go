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

# 微服務 URL（可選，預設 localhost:8081）
FUBON_PROXY_URL=http://127.0.0.1:8081
```

### 2. Docker Compose

```bash
# 啟動所有服務（包含 fubon-proxy）
docker-compose up -d

# 僅啟動 fubon-proxy
docker-compose up -d fubon-proxy
```

### 3. 本地開發

```bash
cd services/fubon-proxy

# 建立虛擬環境
python3 -m venv .venv
source .venv/bin/activate

# 安裝依賴
pip install -r requirements.txt

# 設定環境變數
export FUBON_PERSONAL_ID=your_id
export FUBON_API_KEY=your_key

# 啟動服務
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
curl http://127.0.0.1:8081/health

# 取得 2330 行情
curl http://127.0.0.1:8081/quote/2330

# 批量取得
curl "http://127.0.0.1:8081/quotes?symbols=2330,2317,0050"

# 市場狀態
curl http://127.0.0.1:8081/market-status
```

## 故障排查

### 問題：登入失敗
- 確認 `FUBON_PERSONAL_ID` 和 `FUBON_API_KEY` 正確
- 確認 API Key 權限包含行情查詢

### 問題：連線超時
- 確認 fubon-proxy 服務已啟動
- 檢查 `FUBON_PROXY_URL` 設定正確

### 問題：無資料返回
- 確認為交易日（非週末/假日）
- 確認股票代碼正確

## 注意事項

1. **Session 管理**：服務會自動管理登入狀態，每小時重新登入
2. **Rate Limit**：遵守富邦 300 req/min 限制
3. **安全性**：API Key 請妥善保管，勿上傳至版本控制

## 相關檔案

- `services/fubon-proxy/main.py` - Python 微服務主程式
- `services/fubon-proxy/Dockerfile` - Docker 建置檔
- `services/fubon-proxy/requirements.txt` - Python 依賴
- `internal/marketdata/fubon_client.go` - Go HTTP 客戶端
- `.env` - 環境變數配置
