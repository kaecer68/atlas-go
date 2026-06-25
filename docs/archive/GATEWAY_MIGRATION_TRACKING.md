# Gateway Migration Tracking

## 目標

將所有分散在各處的直接 Provider/Client 實例化遷移至統一的 `Gateway` 模式。

## ⚠️ 重要說明

- **所有行號已過期**：本文件行號對應當時的程式碼版本（2026-05-16），目前（2026-05-23）多個檔案已大幅改寫，行號可能完全偏離。
- **NewDashboardAPI()（legacy）屬於 test-only fallback**：16 個 `dashboard_api.go` 項目中有 14 個位於 legacy 建構子，僅供測試使用。生產路徑已使用 `NewDashboardAPIWithGateway()`。
- **NewDashboardAPIWithGateway()** 生產路徑原有 2 個直接實例化，已全部修正（見 Wave 1a）。
- **Wave 2（Orchestrator ↔ Gateway 橋接）已透過 `GatewayBackedProvider` 完成**：`selectProvider()` 仍建立 provider 實例，但 `GatewayBackedProvider` 包裝其上提供獨立 rate limiter；`SectorDataProvider` 為 local file reader（讀取 `sector_data.json`），資料流為 `Gateway channel → 寫入磁碟 → SectorDataProvider 讀取`，架構正確。詳見 Wave 2 說明。

## 狀態（2026-05-23）

### ✅ 已完成

| 項目 | 修正檔 | 備註 |
|------|--------|------|
| `handlers.go` day trading provider | Wave 1b | 建立 `DayTradingChannelAdapter` + 透過 Gateway 呼叫 |
| `dashboard_api.go` TaiwanGeoProvider (Gateway 建構子) | Wave 1a | 建立 `taiwanGeopoliticalGatewayAdapter`，改為 `GeopoliticalRiskProvider` 介面 |
| `data_channels.go` TaiwanGeoProvider 型別 | Wave 1a | 欄位與參數從具體型別改為 `GeopoliticalRiskProvider` 介面 |

### 🔲 待處理（24 項，分波次）

#### ✅ Wave 2 — Orchestrator ↔ Gateway 橋接（已完成）

Wave 2 的 bridge 已透過 `GatewayBackedProvider`（`internal/orchestrator/gateway_provider.go`）完成：

- **`selectProvider()`（system.go）**：`GatewayBackedProvider` 包裝 `selectProvider()` 並加上獨立 rate limiter（50 req/s, burst 10）。`selectProvider()` 仍負責建立底層 provider（Fugle/TWSE/Hybrid），但模擬路徑不再與 DashboardAPI 的 per-channel rate limit 競爭。Gateway 的 channel 系統不涵蓋 `GetQuotes()` 即時行情，不需進一步遷移。
- **`buildMacroEngines()` SectorDataProvider（composition.go）**：`SectorDataProvider` 為 **local file reader**（讀取 `sector_data.json`），非網路 client。資料流為 `Gateway sector_data channel → 寫入磁碟 → SectorDataProvider 讀取`，一個寫一個讀，架構正確。
- **已無待辦項目**。

#### Wave 1 Legacy — 剩餘 15 項（非生產路徑，低優先）

- `internal/monitoring/dashboard_api.go` (legacy `NewDashboardAPI()`) — 14 個直接 provider 實例化
  - Yahoo Finance macro provider
  - Frankfurter FX provider
  - SOX index provider
  - TWSE capital flow provider
  - TWSE margin balance provider
  - Export statistics provider
  - Sector data provider
  - TSMC revenue provider
  - Composite macro provider
  - Geopolitical providers (global composite, RSS, GDELT)
  - Taiwan RSS provider
  - FinMind client
  - FinMind dividend provider
- `internal/monitoring/dashboard_api.go` — FinMind client for dividends（行號過期）

#### 💤 合理例外（不需修改）

- `cmd/experimental/validate-twse-capital-flow/main.go` — 實驗性 CLI，接受直接實例化

### 📝 追蹤補充

#### 未追蹤項目（不在原始 27 項中）

- `internal/monitoring/dashboard_api.go` — `NewExchangeRateProvider()` 在 legacy 建構子中直接實例化（`exchange_rate` channel 已在 Gateway 註冊但未使用）

---

## 優先級建議（2026-05-23 更新）

| 優先級 | 範圍 | 數量 | 說明 |
|--------|------|------|------|
| **N/A** | `internal/orchestrator/`（核心協調層） | 0 | ✅ Wave 2 已完成（GatewayBackedProvider + 架構確認） |
| **Low** | `internal/monitoring/dashboard_api.go`（legacy 建構子） | 15 | 非生產路徑，可暫時擱置 |
| **N/A** | `internal/monitoring/service/` | 0 | ❌ data_channels.go 3 項已確認 STALE（無 direct constructors），已移除 |

> 最後更新: 2026-05-23
> 來源: AI 輔助掃描 + Wave 1 實作 + Wave 2 架構審查
