# Skill: atlas-news-sentiment

> ⚠️ **此技能描述的功能尚未實作（純藍圖／設計提案）**  
> **實作狀態**：❌ 未實作 — 所有引用檔案均不存在  
> **最後審計**：2026-06-02  
> **計畫狀態**：保留為未來計畫，設計意圖仍然有效

## 描述

**新聞情緒分析系統** — 擷取公開資訊觀測站（MOPS）、新聞網站等來源，計算個股新聞情緒分數。

## 任務觸發

當 AI 代理需要：
- 實作新聞情緒分析功能
- 整合公開資訊觀測站（MOPS）資料
- 將新聞情緒分數整合至因子計算系統

## 核心概念

### 1. 新聞來源

| 來源 | 提供者 | 狀態 |
|------|--------|------|
| 公開資訊觀測站（MOPS） | MOPSScraper | ❌ 未實作 |
| 新聞 API（未來擴充） | NewsAPIProvider | ❌ 未實作 |
| 社群媒體（選項） | SocialSentimentProvider | ❌ 未實作 |

### 2. 情緒計算管線

```
News Source → Scraper → NLP 處理 → SentimentCalculator → 因子輸入
```

### 3. 情緒分數結構

```go
type NewsSentiment struct {
    Symbol    string
    Timestamp time.Time
    Source    string
    Title     string
    Body      string
    Score     float64  // -1.0 ~ 1.0
    Keywords  []string
}
```

### 4. 整合路徑

情緒分數整合至 FactorEngine → 影響因子權重：
- `InstitutionalSentiment` 因子 → 法人情緒
- `Narrative` 因子 → 敘事事件
- 透過 `FactorBridge` 導入

## 實作位置（計畫）

| 元件 | 建議檔案 | 狀態 |
|------|---------|------|
| NewsSentimentProvider | `internal/news/news_sentiment_provider.go` | ❌ 未實作 |
| MOPS Scraper | `internal/news/mops_scraper.go` | ❌ 未實作 |
| SentimentCalculator | `internal/news/sentiment_calculator.go` | ❌ 未實作 |

## 實作考量

### 需要決定的設計問題

1. **NLP 方案**：本地（Go）vs 外部 API（OpenAI / 專用 NLP API）
2. **更新頻率**：MOPS 公告為即時性，但新聞可能有延遲
3. **快取策略**：避免重複擷取相同內容
4. **統一 API 架構**：必須透過 `internal/apigateway/gateway.go` 統一管理 HTTP 請求（參見 `internal/apigateway/CONSTITUTION.md` 第一條）

### 台灣股市特有考量

- MOPS 公告格式：重大訊息、財務報告、股東會等
- 中文 NLP 需求（繁體中文）
- 台灣特有詞彙（除權息、法說會、庫藏股等）
- 須注意 insider trading 法規 — 不可使用未公開資訊

## 與其他技能整合

- `atlas-event-driven-weights`：新聞情緒作為因子輸入
- `atlas-dynamic-correlation`：情緒變化與市場事件的關聯
- `atlas-macro-narrative`：巨集觀敘事事件的輔助確認

## 數據來源考量

- MOPS API：`https://mops.twse.com.tw/`（公開資訊觀測站）
- 注意 MOPS API 可能有 rate limit
- 替代方案：TEJ、CMoney 等商業數據提供商

## 驗證要求（實作後）

```bash
go test ./internal/news/...
```

## 設計原則

1. **統一 API 管理**：使用 Gateway 而非直接建立 HTTP client
2. **向後相容**：不破壞現有因子系統
3. **可配置**：參數管理（如 NLP provider、API keys）應集中於 `config/parameters.go`
4. **評分可解釋**：每個情緒分數應帶有來源和信心度（如 NarrativeEvent 的 ConfidenceSource + HitRate）

---

*技能版本: 0.1（藍圖）*  
*最後更新: 2026-06-02*
*狀態: 計畫階段 — 等待實作*
