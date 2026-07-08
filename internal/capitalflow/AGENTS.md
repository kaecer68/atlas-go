# AGENTS.md — internal/capitalflow

**成熟度**: experimental (X-tier, Wave 11)
**模組職責**: 台股七大資金勢力日內流量 + 共振強度計算 + API 輸出

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `Handler` | `handler.go` | HTTP handler：`HandleDaily`（handler.go:37）/ `HandleSummary`（handler.go:51）。**PR #1005 後為 `Service` 的 thin layer** — pipeline 邏輯全部在 `Service`（`Service.LatestDaily`@service.go:34、`Service.Summary`@service.go:58），Handler 只負責 HTTP 包裝（context 傳遞、error→503 映射） |
| `TWSECapitalFlowChannelAdapter` | (apigateway) | TWSE 三大法人資料抓取 |
| `DailyReport` | `types.go:55` | 七大資金勢力彙整：`Forces` 內含 Foreign / InvestmentTrust / Dealer / Proprietary / PublicBank / Retail / Other |
| `ResonanceResult` | `types.go:38` | 共振結果（含 `Coefficient` 強度 [0.5, 1.5]：1.5=三勢力全對齊、0.5=foreign vs government 對立、1.0=其他；`Direction` 字串標籤） |

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

介面契約（已全部 ship）：
```go
type CapitalFlow interface {
    LatestDaily(ctx context.Context) (DailyReport, error)        // SHIP（service.go:34）
    Summary(ctx context.Context) (SummaryReport, error)          // SHIP（service.go:58）
}
```

**`LatestDaily`** 已 ship：`Service.LatestDaily` 在 `service.go:34`，可直接被 `internal/recommender` adapter 呼叫，繞過 `Handler.HandleDaily`（handler.go:37）需 `*http.Request` 的限制。

**`Summary`** 已 ship：`Service.Summary` 在 `service.go:58`，內部呼叫 `LatestDaily` 重用 pipeline (FetchSnapshot → Extract → ComputeResonance)，再用 `GenerateSummaryReport(date, forces, resonance)`（`report.go:43`）派生 `SummaryReport`。Caller cost 等同一次 `LatestDaily` 呼叫。繞過 `Handler.HandleSummary`（handler.go:51）需 `*http.Request` 的限制。

## 已知陷阱

| 陷阱 | 說明 |
|------|------|
| **共振計算公式變更** | `ResonanceResult` 算法（`ComputeResonance` in `resonance.go:13`）若改，需同步 `parameters.json` 並呼叫 SelfCalibrate 重新校準。 |
| **TWSE 假日不發布** | 週末/假日無資料；前端應 fallback 至上週五資料。 |
| **PublicBank 欄位歷史較短** | 公股行庫資料 TWSE 約 2018+ 才完整；早期資料空值。 |

## 與其他模組整合

- `internal/apigateway/adapter_twse_capital_flow.go` — adapter source
- `internal/marketdata/macro_provider.go` — provider
- `cmd/atlas-mcp/server/tools_capitalflow.go` — MCP 包裝
- `cmd/atlas/main.go:594` — `capitalflow.RegisterRoutes(mux, macroProvider)`

## 測試

- `handler_test.go` 測試 HandleDaily / HandleSummary 回應格式
- 七大勢力 completeness test（驗證 7 個欄位都有值）
- 共振分數範圍測試 [0.5, 1.5]
