# internal/portfolio AGENTS.md

`internal/portfolio` 負責台股投資組合的權重管理與因子計算，是系統「模擬優先」與「稽核導向」的核心。

**Same-package 拆分原則**：`FactorEngine` 被 11 個 consumer 使用，`DarwinianWeightManager` 被 9 個 consumer 使用。**跨 package 拆分**會增加耦合而不減少實際耦合，因此禁止。但 **同 package 內的多檔案拆分**（依職責分離）是被允許且鼓勵的，條件：公開 API 完全不變、所有 consumer 零修改、Factor Change Protocol (§12) 不被破壞。PR #684 是範本。

---

## KEY CONCEPTS

### 1. Darwinian Weights
- 權重強制夾制於 `[0.3, 2.5]`；信念值再夾制於 `[ConvictionClampMin, ConvictionClampMax]`（預設 `[1, 250]`）。
- `PerformDailyAdjustment` 依 20 天 Rolling Sharpe 分層調整：Top 1/3 × 1.05，Bottom 1/3 × 0.95。
- 主要檔案：`darwinian_weights.go`、`agent_weights.go`

### 2. FactorEngine
- 計算 Momentum、Value、Quality、Agent、InstitutionalSentiment、Liquidity 六類因子。
- 回傳 `FactorScoreBreakdown` 含 `Formula`、`RawInputs`、`IsFallback`，確保審計透明。
- 已拆分為 12 個同 package 檔案（1 entry stub + 11 職責分離），詳見程式碼註解。
- 主要檔案：`factor_engine.go`（entry stub）

### 3. FactorWeightEngine
- 根據市場事件與 Regime 動態調整 12 因子權重，總和正規化為 1.0。
- 基礎權重與事件 delta 皆來自 `ParametersConfig.FactorWeight`，唯一權威來源。
- 主要檔案：`factor_weight_engine.go`

### 4. Optimizer
- 組合優化流程：`aggregateRecommendations` → `calculateMultiFactorScores` → `allocateInitialWeights` → `applyConstraints` → `buildPositions`。
- **有歷史資料** → Ledoit-Wolf 共變異數 + Active-set QP；**無歷史資料** → fallback 線性歸一化。
- 主要檔案：`optimizer.go`

### 5. Other Components
| 元件 | 職責 | 主要檔案 |
|------|------|---------|
| Capital Allocator | 依 Conviction 分配可部署資本，含稅務整合 | `capital_allocator.go` |
| Sector Rotator | 依宏觀風險等級調整行業配置 | `sector_rotator.go` |
| Risk Manager | 回撤、日虧損、集中度、停損/止盈警報 | `risk_manager.go` |
| Sizer | Kelly + 波動率 + ATR + 流動性 + 相關性懲罰 | `sizing.go` |
| Post-Trade Analyzer | WinRate、Sharpe、PnL 歸因、執行質量 | `analysis.go` |
| Historical Prices & Fundamental Provider | 提供歷史價格與基本面資料 | `historical_prices.go`、`fundamental_loader.go` |

#### Corporate Action Adjustment
- `AdjustForCorporateActions()` 對歷史價格進行除權息/減資向後調整，運算冪等。
- `ensureAdjusted()` 使用 24h TTL 快取；fetch 失敗時不標記 adjusted，下次重試。
- 詳細公式與整合點見 `historical_prices.go`、`factor_engine_helpers.go` 與對應測試。

---

## ANTI-PATTERNS

- **Silent Clamping**：權重調整在 `constrainWeight` 中靜默完成，外部調用者需檢查 `adjustments` 回傳值。
- **Ignoring IsFallback**：Judge 或決策鏈審查時必須檢查 `IsFallback`。
- **Mutable Slice Reuse**：`ApplyDarwinianWeights` 會生成新 slice，切勿直接修改傳入原切片。
- **不檢查 AgentHealth 就放行**：`muted` agent 不應進入 pipeline；`IsAgentHealthy()` 對 unknown agent 預設 true。
- **Optimizer 未 Attach FactorEngine**：直接 `NewOptimizer()` 而不 `WithFactorEngine()` 會導致因子計算 fallback。
- **FactorWeightEngine 未正規化**：`GetWeights()` 回傳前必須經過 `normalizeWeights()`。
- **忽略過期事件**：`Update()` 必須定期呼叫以移除 faded/expired 事件。

---

## DATA FLOW

```
Market Data → FactorEngine → Optimizer.Optimize()
                                    ↓
CapitalAllocator.Allocate() → Sizer.CalculateSize()
                                    ↓
RiskManager.AddPosition() → PostTradeAnalyzer.Record()
```

**Darwinian Weights 平行於整條 pipeline**：Agent Recommendations → `ApplyDarwinianWeights()` → Modified Conviction → Optimizer

---

## KEY TYPES

| 結構體 | 檔案 | 用途 |
|--------|------|------|
| `DarwinianWeightManager` | `darwinian_weights.go` | 達爾文權重動態調整 |
| `FactorEngine` | `factor_engine.go` | 多因子評分計算 |
| `AgentHealthManager` | `agent_health.go` | 代理健康狀態追蹤 |
| `Optimizer` | `optimizer.go` | 組合優化核心 |
| `CapitalAllocator` | `capital_allocator.go` | 資本配置 |
| `SectorRotator` | `sector_rotator.go` | 行業輪動 |
| `RiskManager` | `risk_manager.go` | 風險控制 |
| `Sizer` | `sizing.go` | 倉位規模計算 |
| `PostTradeAnalyzer` | `analysis.go` | 盤後分析 |

---

## 12. Factor Change Protocol

新增、刪除或改名任何 `FactorType` 時，**必須順序更新以下 8 個位置**：

| Step | 位置 |
|------|------|
| 1 | `optimizer.go` FactorType 常數宣告 |
| 2 | `factor_weight_engine.go` `defaultBaseWeights` map |
| 3 | `internal/domain/shared/shared.go` `FactorScoreBreakdown` struct |
| 4 | `internal/domain/shared/shared.go` `FactorScores` struct |
| 5 | `optimizer_pipeline.go` `symbolScore` struct |
| 6 | `optimizer_pipeline.go` `calculateMultiFactorScores` totalScore 計算 |
| 7 | `factor_engine_aggregate.go` `CalculateAllScoresWithBreakdown` breakdown 建構 |
| 8 | `factor_weight_engine.go` `applyEventAdjustment` / `strategyDeltas` / `GetWeights` |

完成後執行：

```bash
go generate .
bash scripts/ci/verify_factor_integrity.sh
go build ./... && go test ./internal/portfolio/...
```

**CI 強制**：`quality.yml` 的 `factor-integrity` job 會自動執行 `verify_factor_integrity.sh`。

---

## VERIFICATION

```bash
go build ./internal/portfolio/...
go test ./internal/portfolio/...
go vet ./internal/portfolio/...
test -z "$(gofmt -l internal/portfolio/)"
```
