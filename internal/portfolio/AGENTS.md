# internal/portfolio AGENTS.md

`internal/portfolio` 負責台股投資組合的權重管理與因子計算，是系統「模擬優先」與「稽核導向」的核心。

**Same-package 拆分原則**：`FactorEngine` 被 11 個 consumer 使用、`DarwinianWeightManager` 被 9 個 consumer 使用。**跨 package 拆分**會增加耦合而不減少實際耦合，禁止。但**同 package 內多檔案拆分**（依職責分離）允許且鼓勵：公開 API 完全不變、consumer 零修改、Factor Change Protocol (§12) 不被破壞。PR #684 是範本。

---

## KEY CONCEPTS

### 1. Darwinian Weights

- 權重強制夾制於 `[0.3, 2.5]`；信念值再夾制於 `[ConvictionClampMin, ConvictionClampMax]`（預設 `[1, 250]`）
- `PerformDailyAdjustment` 依 20 天 Rolling Sharpe 分層：Top 1/3 × 1.05，Bottom 1/3 × 0.95
- 主要檔案：`darwinian_weights.go`、`agent_weights.go`

### 2. FactorEngine

- 計算 Momentum、Value、Quality、Agent、InstitutionalSentiment、Liquidity 六類因子
- 回傳 `FactorScoreBreakdown` 含 `Formula` / `RawInputs` / `IsFallback`，確保審計透明
- 已拆分為 12 個同 package 檔案（1 entry stub + 11 職責分離）

### 3. FactorWeightEngine

- 根據市場事件與 Regime 動態調整 12 因子權重，總和正規化為 1.0
- 基礎權重與事件 delta 皆來自 `ParametersConfig.FactorWeight`（唯一權威來源）

### 4. Optimizer

- 流程：`aggregateRecommendations` → `calculateMultiFactorScores` → `allocateInitialWeights` → `applyConstraints` → `buildPositions`
- **有歷史資料** → Ledoit-Wolf 共變異數 + Active-set QP；**無歷史資料** → fallback 線性歸一化

### 5. Other Components

| 元件 | 主要檔案 |
|------|----------|
| Capital Allocator（依 Conviction 分配資本，含稅務整合） | `capital_allocator.go` |
| Sector Rotator（依宏觀風險調整行業配置） | `sector_rotator.go` |
| Risk Manager（回撤/日虧損/集中度/停損止盈） | `risk_manager.go` |
| Sizer（Kelly + 波動率 + ATR + 流動性 + 相關性懲罰） | `sizing.go` |
| Post-Trade Analyzer（WinRate / Sharpe / PnL 歸因） | `analysis.go` |
| Corporate Action Adjustment | `historical_prices.go`、`factor_engine_helpers.go` |

`AdjustForCorporateActions()` 對歷史價格做除權息/減資向後調整，運算冪等；`ensureAdjusted()` 用 24h TTL 快取。

---

## ANTI-PATTERNS

- **Silent Clamping**：權重調整在 `constrainWeight` 中靜默完成，需檢查 `adjustments` 回傳值
- **Ignoring IsFallback**：Judge 或決策鏈審查必檢查 `IsFallback`
- **Mutable Slice Reuse**：`ApplyDarwinianWeights` 會生成新 slice，**切勿直接修改傳入原切片**
- **不檢查 AgentHealth**：`muted` agent 不應進入 pipeline；`IsAgentHealthy()` 對 unknown agent 預設 true
- **Optimizer 未 Attach FactorEngine**：直接 `NewOptimizer()` 而不 `WithFactorEngine()` 會導致因子計算 fallback
- **FactorWeightEngine 未正規化**：`GetWeights()` 回傳前必須經 `normalizeWeights()`
- **忽略過期事件**：`Update()` 必定期呼叫以移除 faded/expired 事件

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

## Factor Change Protocol

新增、刪除或改名任何 `FactorType` 時，必須執行 **8 步同步協議**。完整步驟、實作位置與驗證指令見 **`.claude/skills/atlas-factor-change-protocol/SKILL.md`**。

**簡要原則**：

- 8 個位置必須同步：`optimizer.go` 常數、`factor_weight_engine.go` 的 `defaultBaseWeights` 與事件調整、`internal/domain/shared/shared.go` 的兩個 struct、`optimizer_pipeline.go` 的 `symbolScore` 與 `calculateMultiFactorScores`、`factor_engine_aggregate.go` 的 breakdown 建構
- 完成後必跑 `go generate .`、`bash scripts/ci/verify_factor_integrity.sh`、`go build ./... && go test ./internal/portfolio/...`
- CI `quality.yml` 的 `factor-integrity` job 自動執行驗證

---

## VERIFICATION

```bash
go test ./internal/portfolio/...
test -z "$(gofmt -l internal/portfolio/)"
```
