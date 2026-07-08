# AGENTS.md — internal/capitalflow

**成熟度**: experimental (X-tier, Wave 11)
**模組職責**: 台股七大資金勢力日內流量 + 共振強度計算 + API 輸出

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Handler` | `handler.go` | HTTP handler：`HandleDaily` / `HandleSummary` |
| `TWSECapitalFlowChannelAdapter` | (apigateway) | TWSE 三大法人資料抓取 |
| `DailySnapshot` | `types.go` | 七大資金勢力：Foreign / InvestmentTrust / Dealer / Proprietary / PublicBank / Retail / Other |
| `ResonanceScore` | `types.go` | 共振強度 −1 to +1，越大表示買方一致 |

## 七大資金勢力

```
Foreign (外資) — 國際資本動向，最強追蹤信號
Dealer (自營商) — 短期投機
InvestmentTrust (投信) — 台股本土法人
Proprietary (券商自營) — 子集
PublicBank (公股銀行) — 政府護盤指標
Retail (散戶) — 反指標
Other (其他) — 餘額
```

## 資料流

```
GET /api/capital-flow/daily
       ↓
Handler.HandleDaily
       ↓
TWSECapitalFlowChannelAdapter.Fetch
       ↓
macro_provider.ComposeSnapshot
       ↓
Resonance(七勢力) → 共振分數
       ↓
JSON response
```

## 與 P0-1 的關係

Sprint 2 T9 將使 `internal/recommender::HandleRecommendations::CapitalFlow` 欄位接入 `Handler.HandleDaily`。

預期介面：
```go
type CapitalFlow interface {
    LatestDaily() (*DailySnapshot, error)
}
```

## 已知陷阱

| 陷阱 | 說明 |
|------|------|
| **共振計算公式變更** | `ResonanceScore` 算法若改，需同步 `parameters.json` 並呼叫 SelfCalibrate 重新校準。 |
| **TWSE 假日不發布** | 週末/假日無資料；前端應 fallback 至上週五資料。 |
| **PublicBank 欄位歷史較短** | 公股行庫資料 TWSE 約 2018+ 才完整；早期資料空值。 |

## 與其他模組整合

- `internal/apigateway/adapter_twse_capital_flow.go` — adapter source
- `internal/marketdata/macro_provider.go` — provider
- `cmd/atlas-mcp/server/tools_capitalflow.go` — MCP 包裝
- `cmd/atlas/main.go:575` — `capitalflow.RegisterRoutes(mux, macroProvider)`

## 測試

- `handler_test.go` 測試 HandleDaily / HandleSummary 回應格式
- 七大勢力 completeness test（驗證 7 個欄位都有值）
- 共振分數範圍測試 [−1, +1]
