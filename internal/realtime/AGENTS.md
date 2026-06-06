# AGENTS.md — internal/realtime

**成熟度**: evolving
**模組職責**: 即時資料橋接，以 sub-second 頻率監控盤勢並動態調整 Agent 權重。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `RealTimeAdapter` | `regime_adapter.go` | 即時監控主控：資料攝取、盤勢偵測、權重調整 |
| `RegimeDetector` | `regime_adapter.go` | 從市場資料視窗偵測盤勢（平穩/波動/上漲/下跌/反轉/突破/崩跌） |
| `MarketDataPoint` | `regime_adapter.go` | 單筆市場觀測（價格、成交量、買賣價、時間戳） |
| `RegimeType` | `regime_adapter.go` | 即時盤勢分類（與 `domain.Regime` 不同，粒度更細） |
| `RealTimeStats` | `regime_adapter.go` | 監控統計（標的數、Agent 數、盤勢分布、平均信心度） |

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **RegimeType 與 domain.Regime 不同** | `RegimeType` 有 7 種細分盤勢（calm、volatile、trending_up 等），與 orchestrator 層的 `domain.Regime`（risk_on/risk_off/neutral）是不同概念。 |
| **資料視窗預設 60 筆** | `DataWindowSize` 預設 60，`DetectRegime()` 需要至少 30 筆才會進行偵測，不足時回傳 `RegimeCalm`。 |
| **權重調整有上限** | 單次權重變化不超過 `MaxWeightChange`（預設 0.5），且權重不會低於 `MinWeight`（預設 0.1）。 |
| **信心度與波動率反向** | `GetRegimeConfidence()` 以 `1 - volatility/threshold` 計算，波動越高信心越低。 |
| **ApplyToRecommendation 調整 conviction** | 將原始 conviction 乘以即時權重，結果 clamp 在 [1, 100]。 |
| **未註冊 Agent 回傳預設權重 1.0** | `GetAgentWeight()` 對未註冊的 agent-symbol 組合回傳 1.0，不會報錯。 |

---

## 測試

- `go test ./internal/realtime/...`
- 涵蓋 Adapter 初始化、資料攝取、Agent 註冊、盤勢偵測、信心度計算、Recommendation 調整

(End of file - total 34 lines)
