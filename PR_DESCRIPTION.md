## PR 標題
feat: 在 executor 之前引入宣告式個股篩選層，並公開 FactorEngine

## 摘要
本 PR 將 atlas-go 從「僅有模擬倉位管理」推進為「具備真實選股篩選能力」的投資研究系統。核心改動包含：

1. **提取並公開 `portfolio.FactorEngine`**：將原本 `Optimizer` 內的私有因子計算（動能、價值、品質）提取為獨立、可重用的公開引擎。
2. **新增 `internal/screener/` 套件**：實作宣告式篩選邏輯，支援 P/E、P/B、股息率、20 日動能、日內成交量、最小總因子分數等條件。
3. **將篩選層掛入 orchestrator pipeline**：在 `collectRecommendations` 階段，於 `Recommendation()` **之前**執行 `plugins.Screen()`，達成「先篩選、後生成推薦」。
4. **移除 sector executors 的硬編碼 conviction**：改以 `dynamicSignalStrength(quote, params)` 根據盤中價量動態計算，且 params 可從 agent 的 `screening_criteria` 自動繼承。
5. **更新 `configs/agents.json`**：為 13 個 sector / style / superinvestor agent 配置 `screening_criteria`。
6. **補上 E2E 整合測試**：驗證經過篩選的推薦仍能正確流經 evolution → PRISM → experiment → judge 完整晉升/評估路徑。

## 主要變更檔案

### 新增檔案
- `internal/domain/screening.go` — `ScreeningCriteria`、`RangeFilter`、`MinFilter`、`HasFilters()`
- `internal/portfolio/factor_engine.go` — 公開的 `FactorEngine`（動能/價值/品質/綜合分數計算）
- `internal/portfolio/factor_engine_test.go` — FactorEngine 單元測試
- `internal/screener/screener.go` — `Screener` 介面與 `Engine` 實作
- `internal/screener/engine_test.go` — Screener 單元測試（85.5% 覆蓋率）
- `internal/experiment/orchestrator_integration_test.go` — PRISM/experiment/judge E2E 測試

### 修改檔案
- `configs/agents.json` — 為 13 個 agent 加入 `screening_criteria`
- `internal/domain/registry.go` — `AgentSpec` 新增 `ScreeningCriteria` 欄位
- `internal/orchestrator/executors.go` — 在 `collectRecommendations` 前置 `Screen()`；新增 exported `ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins`
- `internal/orchestrator/plugin_registry.go` — 新增 `screener` 欄位、`WithScreener()`、`Screen()`
- `internal/orchestrator/system.go` — `NewSystem` 統一建立 `FactorEngine` 與 `Screener`，掛載到 `PluginRegistry`
- `internal/orchestrator/plugin_sector.go` — 移除硬編碼 conviction（84/78/64），改為參數化 `dynamicSignalStrength`
- `internal/orchestrator/plugin_style.go` — `GrowthMomentumExecutor` 改用動態 conviction
- `internal/orchestrator/plugin_additional.go` — Financials/Shipping/ValueYield/EarningsQuality/TechnicalBreakout 改用動態 conviction
- `internal/orchestrator/alpha_discovery.go` — 改為直接持有 `FactorEngine`，移除 dummy `Optimize()` 繞道
- `internal/portfolio/optimizer.go` — 加入 `factorEngine` 欄位，移除私有方法改為內部委託（刪除 ~132 行重複程式碼）
- 相關 `*_test.go` — 更新 constructor 與新增篩選整合測試

## 向後相容性
- **既有公開 API 簽名完全保留**。舊的 `ExecuteRegistryResearchDetailedWithPolicyAndGuards` 等函式行為不變（內部仍以 `NewPluginRegistry()` 預設執行）。
- 新增的 `ExecuteRegistryResearchDetailedWithPolicyAndGuardsAndPlugins` 為擴充用 exported 函式，供測試與進階使用情境注入自訂 `PluginRegistry`。

## 驗證結果
```bash
go test ./...        # ✅ 全綠
go vet ./...         # ✅ 通過
staticcheck ./...    # ✅ 通過
gofmt -l .           # ✅ 無問題
```

## 覆蓋率
- **總覆蓋率**：**44.4%**（> 40% 門檻）
- `internal/screener`：85.5%
- `internal/orchestrator`：59.2%
- `internal/experiment`：71.6%
- `internal/domain`：70.4%

## 設計決策與注意事項
1. **為何 `dynamicSignalStrength` 要參數化？**
   不同產業/風格 agent 對成交量的敏感度不同（例如 value_yield 可能不在乎 5000K 門檻）。透過 `signalParamsFromAgent(agent)` 自動讀取 `screening_criteria.volume_intraday.min`，讓 executor 的訊號強度激勵與篩選閘門保持一致。

2. **為何在 `PluginRegistry` 層做篩選而非各 executor 內部？**
   將篩選邏輯統一前置，避免 9 個 executor 各自重複實作相同的 P/E、P/B、volume 檢查，也確保未來新增篩選條件時只需改一處。

3. **`FactorEngine` 的執行緒安全**
   使用 `sync.RWMutex` 保護 `history` 與 `fundamentals` 欄位，與 `Optimizer` 既有模式一致。

## Review 狀態
- Goal & Constraint Verification：PASS
- QA Execution：PASS
- Code Quality：PASS
- Security Review：PASS（LOW severity，無需處理）
- 待使用者確認後即可準備 merge。
