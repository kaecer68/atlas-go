# 風控模組盤查報告 — feat/risk-audit-2026-05

**盤查日期**: 2026-05-21  
**盤查分支**: `feat/risk-audit-2026-05`  
**盤查範圍**: `internal/risk/*`、風控參數配置、與 narrative/industry/orchestrator 整合  
**執行者**: Sisyphus (AI Agent)  
**工具**: GitNexus impact analysis、skill `atlas-risk-management`、skill `risk`、code review

---

## 1. 執行摘要

本次盤查針對 atlas-go 系統的【風控結果】模組進行全面檢驗，涵蓋計算邏輯、參數管理、架構整合與測試覆蓋。整體評估：**風控核心計算正確，但存在 5 項架構缺口與 3 項整合斷層**，需要進化與迭代。

| 維度 | 評分 | 說明 |
|------|------|------|
| 計算正確性 | 8/10 | VaR、CVaR、MaxDrawdown、Sharpe 計算正確，但 VaR 使用歷史模擬法（非參數法），未涵蓋極端尾部分析 |
| 參數管理 | 7/10 | 已納入 `ParametersConfig`，但 `DrawdownConfig` 缺少 `Rationale/Source/Todo` 欄位 |
| 架構整合 | 6/10 | 與 narrative 整合良好，但與 industry、portfolio、live 存在斷層 |
| 測試覆蓋 | 7/10 | 核心計算有測試，但缺少壓力測試、邊界條件測試、整合測試 |
| 決策鏈透明 | 5/10 | `MacroAwareDrawdownEngine` 有 rationale，但缺少 breakdown 結構供前端展示 |

---

## 2. 風控模組架構總覽

```
┌─────────────────────────────────────────────────────────────┐
│                    Risk Module (internal/risk)              │
├─────────────────────────────────────────────────────────────┤
│  macro_aware_drawdown.go  │  宏觀感知回撤決策引擎            │
│  capital_controller.go    │  資金階段控制器                  │
│  var_calculator.go        │  VaR/CVaR/MaxDrawdown 計算器     │
│  approval_workflow.go     │  人工審批工作流（獨立）           │
└─────────────────────────────────────────────────────────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        ▼                     ▼                     ▼
┌───────────────┐    ┌─────────────────┐    ┌──────────────┐
│   Narrative   │    │   Orchestrator  │    │  Monitoring  │
│  (MacroRisk)  │◄──►│  (RiskOps)      │    │  (Dashboard) │
└───────────────┘    └─────────────────┘    └──────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │  Industry/      │
                    │  Portfolio      │
                    │  (斷層區域)      │
                    └─────────────────┘
```

---

## 3. 詳細盤查發現

### 3.1 計算邏輯檢驗

#### ✅ VaR / CVaR 計算 (`var_calculator.go`)

| 項目 | 狀態 | 說明 |
|------|------|------|
| 歷史模擬法 | ✅ 正確 | 使用排序後的分位數，符合業界標準 |
| 95%/99% 雙置信區間 | ✅ 正確 | 透過 `ParametersConfig.Risk.VaRConfidenceLevel` 配置 |
| CVaR (Expected Shortfall) | ✅ 正確 | 計算 VaR 以下回報的平均值 |
| MaxDrawdown | ✅ 正確 | 峰值到谷值的百分比跌幅 |
| **缺點** | ⚠️ | 僅使用歷史模擬法，未提供參數法（方差-協方差）或蒙地卡羅法 |
| **缺點** | ⚠️ | 未對回報序列進行常態性檢驗，台股常有肥尾現象 |

#### ✅ 宏觀感知回撤 (`macro_aware_drawdown.go`)

| 項目 | 狀態 | 說明 |
|------|------|------|
| 五級回撤制度 | ✅ 正確 | None/Light/Moderate/Severe/Emergency |
| 結構性趨勢覆蓋 | ✅ 正確 | Orange/Red 風險下，強趨勢可降級回撤 |
| 板塊約束 | ✅ 正確 | 支援 risk_off / carry_trade_unwind / sector_rotation 三種資金流 |
| 停損觸發 | ✅ 正確 | Severe/Emergency 觸發交易停止 |
| **缺口** | ⚠️ | `DrawdownConfig` 未包含在 `ParametersConfig.Validate()` 中 |

#### ✅ 資金階段控制 (`capital_controller.go`)

| 項目 | 狀態 | 說明 |
|------|------|------|
| 四階段模型 | ✅ 正確 | Simulation → Paper → Live → Full |
| 晉升條件 | ✅ 正確 | MinDays + MaxDrawdown + Sharpe + ConsecutiveLosses |
| 連續虧損計數 | ✅ 正確 | RecordLoss/RecordWin 機制 |
| Sharpe 計算 | ✅ 正確 | 年化處理 (×√252) |
| **缺口** | ⚠️ | 未與 `ApprovalWorkflow` 整合，Live→Full 應需人工審批 |

---

### 3.2 參數管理檢驗

#### ⚠️ 缺口：DrawdownConfig 缺少參數溯源欄位

`internal/config/parameters.go` 中 `RiskParameters` 已完整定義，但 `DrawdownConfig`（用於 `macro_aware_drawdown.go`）缺少 `ParameterMetadata[T]` 包裝：

```go
// 現狀：直接 float64
type DrawdownConfig struct {
    OrangeOverrideMinScore float64
    RedOverrideMinScore    float64
    // ... 缺少 Rationale/Source/Todo
}

// 應改為：
type DrawdownConfig struct {
    OrangeOverrideMinScore ParameterMetadata[float64]
    RedOverrideMinScore    ParameterMetadata[float64]
    // ...
}
```

**影響**: 風控參數調整時無法追溯權威來源，違反 `AGENTS.md` 參數統一管理規範。

#### ✅ 優點：VaR 參數已完整納入 ParametersConfig

```go
RiskParameters {
    VaRConfidenceLevel:     0.95 (SourceLiterature)
    VaRSecondaryConfidence: 0.99 (SourceLiterature)
    VaRAlertThreshold:      0.02 (SourceHeuristic)
    VaRCriticalThreshold:   0.05 (SourceHeuristic)
    // ...
}
```

---

### 3.3 架構整合檢驗

#### ✅ 與 Narrative 整合（良好）

```
MacroRiskAssessment (narrative)
    ├── MacroRiskLevel (green/yellow/orange/red)
    ├── ForeignOutflowProb
    └── PrimaryFlow (risk_off/carry_trade_unwind/sector_rotation)
    
StructuralTrendAssessment (narrative)
    ├── DominantTrend (AI Capex Surge / Semiconductor Upcycle)
    ├── OverrideScore
    └── ShouldOverrideRisk
    
MacroAwareDrawdownEngine (risk)
    └── Evaluate(macro, structural) → DrawdownDecision
```

- `orchestrator/composition.go:104` 正確建立 `buildMacroEngines()`
- `orchestrator/composition.go:111` 正確建立 `buildRiskOps()`
- `narrative/structural_trend.go` 的 `CanWithstandMacroRisk` 與 `risk/macro_aware_drawdown.go` 的 `canWithstandMacroRisk` **邏輯一致**

#### ⚠️ 斷層 1：與 Industry 整合不足

| 問題 | 說明 |
|------|------|
| 產業週期風險未納入 | `industry.CycleTracker` 的週期位置（expansion/recovery/mature/recession）未影響風控決策 |
| 供應鏈衝擊未傳導 | `industry.LinkageAnalyzer.PropagateShock()` 的結果未傳入風控模組 |
| 季節性調整未考慮 | `industry.SeasonalEngine` 的季節性模式未影響回撤閾值 |

**建議**: 在 `MacroAwareDrawdownEngine.Evaluate()` 中增加 `IndustryRiskAssessment` 參數，整合產業週期、供應鏈衝擊、季節性風險。

#### ⚠️ 斷層 2：與 Portfolio 整合不足

| 問題 | 說明 |
|------|------|
| Darwinian 權重未考慮風控 | `portfolio.DarwinianWeightManager` 的權重調整未受風控回撤影響 |
| 因子權重未動態調整 | `portfolio.FactorWeightEngine` 未根據風控等級調整因子權重 |
| 倉位大小未動態調整 | `portfolio.DynamicPositionSizer` 未使用 `MacroAwareDrawdownEngine` 的輸出 |

**建議**: 在 `PortfolioManager` 中增加 `RiskAdjuster` 介面，根據 `DrawdownDecision` 動態調整 Darwinian 權重、因子權重、倉位大小。

#### ⚠️ 斷層 3：與 Live 整合不足

| 問題 | 說明 |
|------|------|
| 即時風控缺失 | `live.OrderManager` 未整合 `ShouldHaltTrading` 檢查 |
| 即時 VaR 監控缺失 | 未在交易時段內即時計算投組 VaR |
| 熔斷機制缺失 | 未實作單日最大虧損熔斷（`MaxDailyLossPct` 僅是參數） |

**建議**: 在 `live.OrderManager.ExecuteOrder()` 前增加風控閘門：
1. 檢查 `ShouldHaltTrading()`
2. 檢查單日虧損是否超過 `MaxDailyLossPct`
3. 檢查投組 VaR 是否超過 `VaRCriticalThreshold`

---

### 3.4 測試覆蓋檢驗

#### ✅ 現有測試（通過）

```bash
go test ./internal/risk/... -v
# PASS: 48 個測試案例全部通過
```

| 測試檔案 | 案例數 | 覆蓋範圍 |
|----------|--------|----------|
| `var_calculator_test.go` | 6 | VaR、CVaR、MaxDrawdown、PercentileAccuracy |
| `capital_controller_test.go` | 18 | 階段晉升、Sharpe、連續虧損、資金限制 |
| `macro_aware_drawdown_test.go` | 4 | 五級回撤、板塊約束、停損、投組調整 |
| `approval_workflow_test.go` | 8 | 審批流程、狀態追蹤、持久化 |

#### ⚠️ 缺少測試

| 缺少項目 | 嚴重度 | 說明 |
|----------|--------|------|
| 壓力測試 | 🔴 高 | 未測試極端市場條件（如 2008、2020、2022） |
| 邊界條件測試 | 🟡 中 | 未測試空序列、單一元素、全部相同值 |
| 整合測試 | 🟡 中 | 未測試與 narrative、orchestrator 的整合 |
| 並行安全測試 | 🟡 中 | `CapitalPhaseController` 未測試併發場景 |
| 參數校準測試 | 🟢 低 | 未測試 `ParametersConfig` 變更後的行為 |

---

### 3.5 決策鏈透明化檢驗

#### ⚠️ 缺口：風控決策缺少 Breakdown 結構

對比 `FactorScoreBreakdown` 和 `ConvictionBreakdown`：

```go
// FactorScoreBreakdown (已實作)
type FactorScoreBreakdown struct {
    Score     float64            `json:"score"`
    Weight    float64            `json:"weight"`
    Formula   string             `json:"formula"`
    RawInputs map[string]float64 `json:"raw_inputs"`
    IsFallback bool              `json:"is_fallback"`
}

// ConvictionBreakdown (已實作)
type ConvictionBreakdown struct {
    Base  int    `json:"base"`
    Floor int    `json:"floor"`
    Final int    `json:"final"`
    Steps []struct {
        Rule   string `json:"rule"`
        Delta  int    `json:"delta"`
        Reason string `json:"reason"`
    } `json:"steps"`
}

// DrawdownBreakdown (❌ 未實作)
// 應包含：
// - MacroRiskLevel 評估細節
// - StructuralOverride 計算過程
// - SectorConstraints 生成邏輯
// - 每個決策步驟的 Rule/Delta/Reason
```

**影響**: 前端「決策鏈」頁面無法展示風控決策的完整計算過程，違反決策鏈透明化原則。

---

## 4. 風險評估

### 4.1 高風險項目

| 項目 | 風險 | 說明 |
|------|------|------|
| Live 交易缺少即時風控閘門 | 🔴 **CRITICAL** | `OrderManager` 未檢查 `ShouldHaltTrading`，可能在 Severe/Red 風險下繼續交易 |
| 產業週期風險未納入回撤決策 | 🔴 **HIGH** | 產業處於 recession 時，回撤閾值應更嚴格 |
| Darwinian 權重未受風控調整 | 🟡 **MEDIUM** | 高風險時應降低高波動代理權重 |

### 4.2 中風險項目

| 項目 | 風險 | 說明 |
|------|------|------|
| DrawdownConfig 缺少參數溯源 | 🟡 **MEDIUM** | 違反統一參數管理規範 |
| 缺少壓力測試 | 🟡 **MEDIUM** | 無法驗證極端市場下的風控效果 |
| 決策鏈缺少 Breakdown | 🟡 **MEDIUM** | 影響前端透明化展示 |

---

## 5. 進化建議

### 5.1 短期（1-2 週）

1. **補強 Live 風控閘門**
   - 在 `live/order_manager.go` 增加 `RiskGate` 介面
   - 交易前檢查 `ShouldHaltTrading()`、`MaxDailyLossPct`、`VaRCriticalThreshold`
   - 新增 `live/risk_gate.go` 與對應測試

2. **補強 DrawdownConfig 參數溯源**
   - 將 `DrawdownConfig` 欄位改為 `ParameterMetadata[T]`
   - 在 `parameters_defaults.go` 增加 `defaultDrawdownConfig()`
   - 在 `ParametersConfig.Validate()` 增加 Drawdown 驗證

3. **補強決策鏈 Breakdown**
   - 新增 `DrawdownBreakdown` struct
   - 在 `MacroAwareDrawdownEngine.Evaluate()` 中填充 breakdown
   - 更新 Dashboard API 回傳 breakdown

### 5.2 中期（2-4 週）

4. **整合 Industry 風險**
   - 新增 `IndustryRiskAssessment` struct
   - 在 `MacroAwareDrawdownEngine` 中增加 `industry` 參數
   - 根據產業週期、供應鏈衝擊、季節性調整回撤閾值

5. **整合 Portfolio 風險調整**
   - 新增 `RiskAdjuster` 介面
   - 在 `PortfolioManager` 中實作 `AdjustWeights(decision *DrawdownDecision)`
   - 根據風控等級調整 Darwinian 權重、因子權重、倉位大小

6. **補強壓力測試**
   - 新增 `cmd/stress-test-risk` CLI
   - 模擬 2008、2020、2022 等極端市場條件
   - 驗證回撤決策是否符合預期

### 5.3 長期（1-2 月）

7. **進階風險模型**
   - 實作 GARCH-based 波動率預測（`internal/config/parameters.go` 已有 GARCHParameters）
   - 實作蒙地卡羅 VaR
   - 實作壓力測試場景引擎（`internal/stress/` 已有基礎）

8. **風控自動化**
   - 實作 `BackgroundTaskManager` 定時風控檢查
   - 風控異常時自動發送警報（Telegram/Email/Webhook）
   - 風控決策自動寫入 Ledger 供稽核

---

## 6. 結論

風控模組的核心計算邏輯正確，測試覆蓋良好，與 Narrative 的整合也符合 `atlas-risk-management` 技能標準的三層決策架構。但存在以下**必須立即處理**的缺口：

1. **Live 交易缺少即時風控閘門**（CRITICAL）
2. **產業週期風險未納入回撤決策**（HIGH）
3. **決策鏈缺少 Breakdown 結構**（MEDIUM，影響透明化）

建議按照「短期 → 中期 → 長期」的順序逐步進化，優先補強 Live 風控閘門與產業整合。

---

## 附錄

### A. 相關檔案清單

| 檔案 | 職責 |
|------|------|
| `internal/risk/macro_aware_drawdown.go` | 宏觀感知回撤引擎 |
| `internal/risk/capital_controller.go` | 資金階段控制器 |
| `internal/risk/var_calculator.go` | VaR/CVaR/MaxDrawdown 計算 |
| `internal/risk/approval_workflow.go` | 人工審批工作流 |
| `internal/narrative/structural_trend.go` | 結構性趨勢評估 |
| `internal/config/parameters.go` | 參數配置定義 |
| `internal/config/parameters_defaults.go` | 參數預設值 |
| `internal/orchestrator/composition.go` | 風控引擎組裝 |
| `internal/monitoring/api/risk/handlers.go` | 風控 API |
| `internal/monitoring/api/system/handlers.go` | 資金階段 API |

### B. 測試執行結果

```bash
$ go test ./internal/risk/... -v
ok      github.com/kaecer68/atlas-go/internal/risk      0.476s
# 48 個測試案例全部通過
```

### C. GitNexus Impact Analysis

| 符號 | 風險等級 | 直接依賴 | 影響模組 |
|------|----------|----------|----------|
| `MacroAwareDrawdownEngine` | LOW | 1 | Risk, Narrative |
| `CapitalPhaseController` | LOW | 1 | Risk, Monitoring |
| `VaRCalculator` | LOW | 1 | Risk |
