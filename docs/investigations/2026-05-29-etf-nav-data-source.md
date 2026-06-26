# ETF NAV 數據源調查（2026-05-29）

> **文件角色**：數據源可用性調查報告。  
> **來源**：原記載於 `internal/marketdata/AGENTS.md`，因屬調查紀錄性質，搬遷至此。  
> **權威狀態**：調查當時（2026-05-29）的通道狀態；後續若有新通道接入，應更新本文件或另開新調查。

## 背景

台股 ETF 即時淨值（NAV）是追蹤誤差與實盤決策的關鍵輸入。`internal/marketdata` 在 2026-05-29 針對五個優先級通道進行可用性調查，確認當時無任何免費 REST API 可提供即時 ETF NAV。

## 五個優先級通道的 ETF NAV 可用性

| 優先級 | 通道 | ETF NAV？ | 原因 |
|--------|------|----------|------|
| 1 | 富邦證券 | ❌ | fubon-neo SDK 僅提供 `intraday.quote()` OHLCV。proxy 4 個端點皆為即時報價，無 fund/NAV API。 |
| 2 | TWSE OpenAPI | ❌ | ETFReport/ETFNAV → 302 HTML。BFIBMS → redirect。getETFNetValue.jsp → HTML。無免費 REST API。 |
| 3 | Fugle | ❌ | fugle_client.go 僅提供即時報價 + meta。無 NAV。 |
| 4 | TEJ | ❌ | tej_provider.go 僅實作 TRAIL/TAPRCD（股價）和 TWN/AFINA（財報）。無 ETF NAV dataset。 |
| 5 | FinMind | ⚠️ 待付費 | TaiwanStockETF dataset 存在於 FinMind catalog 中，但未實作。需付費 token（每 7 天換一次）。 |

## 結論

調查當時，即時 ETF NAV 的唯一可行路徑為 **FinMind 付費 dataset**，其餘通道皆不支援。`internal/marketdata` 因此繼續使用收盤價代理（close-price proxy）作為過渡方案，直到 FinMind 付費註冊完成並接入（未實作計畫見 `.omo/plans/2026-05-29-etf-nav-finmind.md`）。

## 相關規則

- `internal/marketdata/AGENTS.md`：ETF NAV 資料來源與數據流說明。
- `internal/apigateway/CONSTITUTION.md`：數據源優先級與 fallback 規範。
