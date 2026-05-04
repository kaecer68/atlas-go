# Atlas Data Providers & Channel Management Skill

**版本**: 1.0  
**日期**: 2026-05-03  
**職責**: 理解與維護 Atlas-Go 的市場資料抓取、資訊通道健康檢查與 HybridProvider 回退邏輯  

---

## 1. 資料提供商對照表

| Provider | 費用 | Rate Limit | 資料類型 | Go SDK | 認證方式 | 使用場景 |
|---------|------|-----------|---------|--------|---------|---------|
| **TWSE OpenAPI** | 免費 | 0.6 req/s (3/5s) | 即時全市場 | ✅ 原生 | 無需 | **首選**，盤中全市場行情 |
| **FinMind** | 免費 | 600 req/min | 歷史/指定日期 | ✅ 原生 | API Key | 歷史資料回填、健康檢查 |
| **Fubon** | 免費（需帳戶）| 300 req/min | 即時 | ❌ 僅 Python/JS/C#/C++ | 帳密+憑證 | 即時行情（需 Python 微服務）|
| **Fugle** | **付費** | 50 req/min | 即時 | ✅ 原生 | API Key | **最後手段**，付費即時行情 |
| **TEJ** | 免費試用 | 500 calls/day | 財務/營收 | ✅ 原生 | API Key | 財務報表、月營收 |

**關鍵原則**：
- TWSE 是**完全免費**且無需帳戶的首選
- FinMind 也是**免費**，適合歷史資料
- Fubon **免費但需富邦證券帳戶**，目前 Go SDK **不支援行情 API**
- Fugle **最貴**，僅在免費源都失敗時使用，有 Circuit Breaker 保護

---

## 2. HybridProvider 回退鏈

### 2.1 優先順序

```
GetQuotes 呼叫時的執行順序：

1. TWSE OpenAPI（免費，無認證）
   ↓ 失敗/無資料
2. FinMind（免費，需 FINMIND_API_KEY）
   ↓ 失敗/無資料
3. Fubon（免費，需 FUBON_API_KEY + 帳戶）
   ↓ 失敗/無資料
4. Fugle（付費，需 FUGLE_API_KEY）
   ← Circuit Breaker 保護，避免頻繁呼叫付費 API
```

### 2.2 Circuit Breaker 機制

針對**付費的 Fugle** 啟用 Circuit Breaker：

| 狀態 | 行為 |
|------|------|
| **Closed** | 正常運作，允許 Fugle 請求 |
| **Open** | Fugle 連續失敗 3 次後開啟，**阻擋所有 Fugle 請求 5 分鐘** |
| **Half-Open** | 5 分鐘後嘗試恢復，最多允許 2 次測試請求 |

**何時觸發**：
- HTTP 錯誤（timeout、connection refused、5xx）
- 返回無效資料（價格全為 0）
- 返回空資料

### 2.3 初始化方式

```go
// 檔案: internal/orchestrator/system.go
case "hybrid", "":
    return marketdata.NewHybridProvider(
        marketdata.NewTWSEClient(),  // TWSE 客戶端
        cfg.FinMindAPIKey,            // FinMind API Key
        cfg.FubonAPIKey,              // Fubon API Key
        cfg.FugleAPIKey,              // Fugle API Key
    )
```

---

## 3. 資訊通道健康檢查

### 3.1 健康檢查端點

**API**: `GET /api/dashboard/data-channels`

**實作檔案**: `internal/monitoring/api/data/handlers.go`

### 3.2 各通道檢查方式

| 通道 | 檢查方式 | 特殊處理 |
|------|---------|---------|
| **TWSE** | 讀取 `data/replay/tw_extended_90days.csv` 時間戳 | 檢查最新資料日期 |
| **FinMind** | `GetStockPrice(ctx, "2330", nearestTradingDay(date))` | 自動跳過週末與假日，最多嘗試 10 個交易日 |
| **Fubon** | `CheckMarketStatus(ctx)` → `GetQuote(ctx, "0050")` | 檢查 API 連線狀態 |
| **Fugle** | `CheckMarketStatus(ctx)` → `GetQuote(ctx, "0050")` | 檢查 API 連線狀態 |
| **Yahoo** | 讀取 `data/state/macro/latest.json` | 總經指標 |

### 3.3 FinMind 健康檢查的特殊邏輯

```go
// 自動處理週末與國定假日
checkDate := time.Now()
var err error
for attempts := 0; attempts < 10; attempts++ {
    checkDate = nearestTradingDay(checkDate)  // 跳過週六/週日
    _, err = finmindClient.GetStockPrice(ctx, "2330", checkDate.Format("2006-01-02"))
    if err == nil {
        break
    }
    checkDate = checkDate.AddDate(0, 0, -1)  // 往前一天繼續試
}
```

**nearestTradingDay 函數**：
- 輸入任意日期
- 如果是週六/週日，往前推到最近交易日
- **不處理國定假日**（由 API 返回空資料後自動回退）

---

## 4. 環境變數配置

### 4.1 必要配置（`.env` 檔案）

```bash
# TWSE - 無需設定，完全免費

# FinMind（免費）
FINMIND_API_KEY=your_finmind_api_key

# Fubon（免費，需富邦帳戶）
FUBON_API_KEY=your_fubon_api_key

# Fugle（付費）
FUGLE_API_KEY=your_fugle_api_key
```

### 4.2 配置讀取優先順序

```go
// 檔案: internal/config/config.go
FubonAPIKey: envOrPriority("FUBON_API_KEY", "ATLAS_FUBON_API_KEY")
FugleAPIKey: envOrPriority("FUGLE_API_KEY", "ATLAS_FUGLE_API_KEY")
FinMindAPIKey: envOr("FINMIND_API_KEY", "")
```

- `envOrPriority`：嘗試多個環境變數名稱，第一個有值的生效
- `envOr`：單一環境變數，無值時使用預設值

---

## 5. 故障排查指南

### 5.1 常見問題速查

| 問題 | 可能原因 | 解決方案 |
|------|---------|---------|
| **FinMind: no price data** | 查詢日期為週末/假日 | 健康檢查已自動處理 |
| **Fubon: SSL connection timeout** | 富邦伺服器 TLS 問題 | 非程式問題，等待富邦修復 |
| **Fugle: 429 Rate limit** | 超過 50 req/min | 啟用 Circuit Breaker，等待冷卻 |
| **TWSE: 空資料** | 非交易日或盤後 | 正常行為，等待下一交易日 |
| **所有 Provider 失敗** | 網路問題 | 檢查網路連線，查看日誌 |

### 5.2 診斷命令

```bash
# 1. 檢查各 API 連線
curl -s http://localhost:8080/api/dashboard/data-channels | jq '.channels[] | {id: .channel_id, status: .status, error: .last_error}'

# 2. 直接測試 TWSE
curl "https://openapi.twse.com.tw/v1/exchangeReport/STOCK_DAY_ALL"

# 3. 直接測試 FinMind
curl -H "Authorization: Bearer $FINMIND_API_KEY" "https://api.finmindtrade.com/api/v4/data?dataset=TaiwanStockPrice&data_id=2330&start_date=2026-04-30&end_date=2026-04-30"

# 4. 檢查環境變數
echo "FinMind: $FINMIND_API_KEY"
echo "Fubon: $FUBON_API_KEY"
echo "Fugle: $FUGLE_API_KEY"

# 5. 查看日誌
tail -f logs/atlas.log | grep -E "HybridProvider|marketdata|error"
```

### 5.3 Fubon 特殊說明

**關鍵限制**：富邦新一代 API 的 **Go SDK 僅支援「證券交易帳務及條件單」，不支援「行情查詢」**。

**行情 API 支援語言**：
- ✅ Python
- ✅ Node.js
- ✅ C#
- ✅ C++
- ❌ Go

**如果要使用 Fubon 行情**：
1. 下載 [Python SDK](https://www.fbs.com.tw/TradeAPI_SDK/fubon_binary/fubon_neo-2.2.8-cp37-abi3-macosx_11_0_arm64.zip)
2. 建立 Python 微服務（FastAPI/Flask）
3. Go 程式透過 HTTP 呼叫 Python 微服務

---

## 6. 檔案位置

| 職責 | 檔案 |
|------|------|
| HybridProvider 核心邏輯 | `internal/marketdata/hybrid_provider.go` |
| Provider 介面定義 | `internal/marketdata/provider.go` |
| TWSE 實作 | `internal/marketdata/twse_openapi.go` |
| FinMind 實作 | `internal/marketdata/finmind_client.go` |
| Fubon 實作 | `internal/marketdata/fubon_client.go` |
| Fugle 實作 | `internal/marketdata/fugle_client.go` |
| 健康檢查 API | `internal/monitoring/api/data/handlers.go` |
| 環境變數配置 | `internal/config/config.go` |
| Provider 初始化 | `internal/orchestrator/system.go` |

---

## 7. 修改注意事項

### 7.1 修改 HybridProvider 時

1. **保持優先順序**：TWSE → FinMind → Fubon → Fugle
2. **Circuit Breaker 僅針對 Fugle**：避免浪費付費額度
3. **測試所有場景**：無 API Key、部分 API Key、全部 API Key

### 7.2 新增 Provider 時

1. 實作 `Provider` 介面：`Name() string`、`GetQuotes(ctx, asOf, symbols)`
2. 在 `HybridProvider` 中加入回退鏈
3. 在 `config.go` 新增環境變數
4. 在 `handlers.go` 新增健康檢查
5. 更新本 Skill 文件

### 7.3 CI 檢查

修改後必須執行：
```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./internal/marketdata/...
go vet ./...
```

---

## 8. 外部參考

- [富邦新一代 API 文件](https://www.fbs.com.tw/TradeAPI/)
- [FinMind API 文件](https://finmindtrade.com/)
- [Fugle API 文件](https://developer.fugle.tw/)
- [TWSE OpenAPI](https://openapi.twse.com.tw/)
