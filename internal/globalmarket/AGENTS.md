# AGENTS.md — internal/globalmarket

**成熟度**: evolving
**模組職責**: 全球總經資料管理，支援多市場配置、跨市場相關性與區域曝險監控。

---

## 核心型別

| 型別 | 檔案 | 功能 |
|------|------|------|
| `GlobalMarketManager` | `global_market.go` | 多市場管理器：啟用/停用市場、計算全球曝險、產生報告 |
| `MarketConfig` | `global_market.go` | 單一市場配置（時區、交易時間、標的、資料來源、相關性） |
| `CorrelationMatrix` | `global_market.go` | 跨市場相關性矩陣（對稱、預設 0.5） |
| `GlobalAgent` | `global_market.go` | 市場專屬 Agent（區域前綴 + 基礎 Agent ID） |
| `ExposureReport` | `global_market.go` | 投組區域與幣別分布，含限額違規檢查 |
| `GlobalExpansionConfig` | `global_market.go` | 擴展配置（主市場、啟用市場、區域限額） |

---

## 本模組特有陷阱

| 陷阱 | 說明 |
|------|------|
| **僅台灣市場預設啟用** | `initializeDefaultMarkets()` 中僅 `RegionTaiwan` 的 `Enabled: true`，其他市場需呼叫 `EnableMarket()` 啟用。 |
| **EnableMarket 會自動生成 Agent** | 啟用市場時會根據 `MarketConfig.Specialization` 自動 spawn 區域專屬 Agent。 |
| **相關性預設 0.5** | 若兩市場間未設定相關性，`GetCrossMarketCorrelation()` 與 `GetCorrelation()` 皆回傳 0.5。 |
| **標的符號推斷規則** | `.TW` = 台灣、無後綴大寫 = 美國、`.KS` = 亞洲、`.T` = 日本、`.HK` = 亞洲，無法識別時 fallback 到 `PrimaryMarket`。 |
| **區域限額檢查** | `CalculateGlobalExposure()` 會檢查 `RegionalLimits`，超額時記錄 `LimitBreach`（目前僅記錄不阻擋交易）。 |
| ** diversification score = 1 - 平均相關性** | `GetDiversificationScore()` 單一市場回傳 1.0，多市場時以 `1 - avgCorr` 計算。 |

---

## 測試

- `go test ./internal/globalmarket/...`
- 涵蓋市場啟用/停用、Agent 生成、跨市場相關性、曝險計算、符號區域推斷、限額違規檢查

(End of file - total 34 lines)
