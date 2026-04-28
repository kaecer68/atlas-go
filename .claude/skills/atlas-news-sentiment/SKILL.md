# Skill: atlas-news-sentiment

## 描述

**新聞情緒分析系統** - 為台股市場提供新聞情緒作為投資決策的輸入因子。

## 任務觸發

當 AI 代理需要：
- 實作新聞情緒分析功能
- 評估新聞情緒數據來源
- 將新聞情緒整合至因子系統
- 處理台股新聞覆蓋限制

## 重要發現：Finnhub 限制

**Finnhub News Sentiment API 僅支援美股公司**（Premium 功能），**無法直接用於台股**。

這是選擇數據源時的關鍵限制。

## 替代方案評估

| 方案 | 台股覆蓋 | 情緒分析 | 成本 | 備註 |
|------|----------|----------|------|------|
| TWSE 開放資料 | 完整 | 無 | 免費 | 可取得新聞量/討論度作為代理指標 |
| TEJ（台灣經濟新報） | 完整 | 有 | 昂貴 | 台灣專業金融數據庫 |
| 自行建置 NLP | 可擴展 | 可訓練 | 高 | 需要 NLP 模型與訓練資料 |
| Finage | 部分 | API 提供 | 中 | 需確認台股覆蓋範圍 |

### 建議實作路徑

**短期（立即）**：
採用 TWSE 開放資料作為代理指標

**中期（3-6 個月）**：
評估 Finage 或其他替代方案

**長期（6+ 個月）**：
若預算允許，採用 TEJ 或自行建置 NLP 模型

## 代理指標方案（TWSE 開放資料）

### 資料來源

**公開資訊觀測站（MOPS）**：
- 異常公告數量（重大訊息、記者會）
- 公告頻率的異常變化

**使用方式**：
```go
type NewsSentimentProxy struct {
    MOPSAnnouncementCount  int       // 當日/週公告數量
    MOPSAnnouncementDelta  float64   // 相比前一週變化率
    ForumBuzzScore         float64   // 假設從 forum aggregator 取得
}

func (p *NewsSentimentProxy) Score() float64 {
    // 標準化至 [-1, 1]
    // 正值 = 正面情緒，負值 = 負面情緒
}
```

### 情緒分數計算

```
NewsSentiment = MOPS_Score × 0.6 + ForumBuzz × 0.4
```

其中：
- `MOPS_Score`: 根據公告數量異常與方向計算
- `ForumBuzz`: 根據論壇討論熱度計算

## 實作位置

| 元件 | 檔案 | 狀態 |
|------|------|------|
| NewsSentimentProvider | `internal/marketdata/news_sentiment_provider.go` | 待建立 |
| MOPSScraper | `internal/marketdata/mops_scraper.go` | 待建立 |
| SentimentScoreCalculator | `internal/marketdata/sentiment_calculator.go` | 待建立 |

## 數據來源

### TWSE 開放資料 API
- **端點**：`https://openapi.twse.com.tw/v1/`
- **Swagger**：`https://openapi.twse.com.tw/v1/swagger.json`
- **涵蓋**：上市個股日收盤價、月均價、異常公告

### 政府資料開放平臺
- **URL**：`https://data.gov.tw/en/datasets/11548`
- **說明**：上市股票日收盤價、月均價

## 與因子系統整合

新聞情緒作為額外因子輸入：

```go
type NewsSentimentFactor struct {
    DailySentiment  float64  // 當日新聞情緒 [-1, 1]
    WeeklyTrend     float64  // 週趨勢 [-1, 1]
    AnnouncementCount int   // 重大公告數量
}
```

## 設計原則

1. **代理指標驗證**：在正式採用前，必須與實際股價走勢驗證相關性
2. **多元數據源**：不依賴單一數據源，採用多源交叉驗證
3. **可擴展性**：預留介面以便未來升級至更完善的數據源

## 驗證要求

```bash
go test ./internal/marketdata/...      # 新聞情緒測試
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total  # ≥ 40%
```

## 驗證標準

- **相關性驗證**：新聞情緒分數與股價走勢相關性 > 0.3
- **領先/落後驗證**：確認新聞情緒是否具有預測能力
- **回測驗證**：將新聞情緒因子加入現有因子模型，驗證是否改善 Sharpe Ratio

## 數據更新頻率

- **每日更新**：MOPS 公告於盤後發布
- **延遲處理**：注意公開資訊可能有 1-2 日延遲

## 限制與風險

| 限制 | 緩解措施 |
|------|----------|
| Finnhub 不支援台股 | 採用替代方案 |
| TWSE 無直接情緒分析 | 使用代理指標並驗證相關性 |
| 自行建置 NLP 成本高 | 分階段實作，先用代理指標 |
| 新聞延遲 | 注意資料時間戳，僅用於中期趨勢判斷 |
