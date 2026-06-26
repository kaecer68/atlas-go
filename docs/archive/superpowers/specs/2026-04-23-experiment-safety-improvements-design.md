# 實驗系統安全強化設計文件

**日期**: 2026-04-23  
**版本**: v1.0  
**狀態**: 設計階段 (等待批准)  
**作者**: Sisyphus / Atlas-Go AI Agent  

---

## 1. 背景與動機

在執行完整模擬交易實驗審查後，發現三個影響系統安全性的關鍵問題：

1. **極端回撤風險 (A)**: 2026-03-26~27 兩日組合回撤 -61.61%，現有止損僅針對個股，缺乏組合層級保護
2. **靜默權重調整 (B)**: Darwinian 權重限制 [0.3, 2.5] 超出時靜默截斷，無任何記錄或警告
3. **樣本偏差風險 (C)**: 實驗接受門檻最低僅需 3 個觀測值 (level_1)，統計信度不足

本設計旨在建立一套可稽核、可測試、最小侵入性的改善機制。

---

## 2. 問題 A: 極端回撤防護 — 三層熔斷機制

### 2.1 現況分析

現有系統的風控機制：
- 個股止損: `StopLossPct = 0.08` (8%)
- 單一持倉上限: `MaxPositionWeight = 0.22` (22%)
- 現金保留: `ReserveCashFraction = 0.08` (8%)
- **缺乏**: 組合層級回撤監控與自動減碼/清倉機制

### 2.2 設計目標

- 在回撤擴大初期即介入，避免單一極端事件導致組合重創
- 所有熔斷觸發與動作必須記錄於稽核軌跡
- 不影響正常交易流程 (僅在極端情境觸發)

### 2.3 三層防護架構

```
┌─────────────────────────────────────────────────────────────┐
│  層級    觸發條件 (組合回撤)    動作                          │
├─────────────────────────────────────────────────────────────┤
│  黃色    drawdown > 15%         - 新倉位規模減半              │
│  預警                              - 現金保留提升至 15%        │
│                                   - 記錄 Warning 事件          │
├─────────────────────────────────────────────────────────────┤
│  橙色    drawdown > 25%         - 禁止開新倉                  │
│  限制                              - 僅允許平倉與止損          │
│                                   - 記錄 Alert 事件            │
├─────────────────────────────────────────────────────────────┤
│  紅色    drawdown > 35%         - 強制清倉至 100% 現金        │
│  熔斷                              - 暫停交易 3 個交易日       │
│                                   - 記錄 CircuitBreaker 事件   │
└─────────────────────────────────────────────────────────────┘
```

### 2.4 實作設計

**新增檔案**: `internal/risk/drawdown_guard.go`

```go
package risk

import (
    "fmt"
    "time"
)

// DrawdownLevel 定義回撤防護層級
type DrawdownLevel int

const (
    DrawdownNormal   DrawdownLevel = iota // 正常 (< 15%)
    DrawdownYellow                        // 黃色預警 (15-25%)
    DrawdownOrange                        // 橙色限制 (25-35%)
    DrawdownRed                           // 紅色熔斷 (> 35%)
)

// DrawdownGuardConfig 三層防護設定
type DrawdownGuardConfig struct {
    YellowThreshold  float64       // 0.15
    OrangeThreshold  float64       // 0.25
    RedThreshold     float64       // 0.35
    PositionScaleYellow float64    // 0.5 (減半)
    CashReserveYellow   float64    // 0.15
    PauseDaysRed        int        // 3
}

// DrawdownGuard 組合回撤防護器
type DrawdownGuard struct {
    config      DrawdownGuardConfig
    history     []DrawdownEvent
    currentLevel DrawdownLevel
    lastTrigger time.Time
}

// DrawdownEvent 記錄每次觸發事件 (供稽核)
type DrawdownEvent struct {
    Timestamp   time.Time     `json:"timestamp"`
    Level       string        `json:"level"`
    DrawdownPct float64       `json:"drawdown_pct"`
    Action      string        `json:"action"`
    PortfolioValue float64    `json:"portfolio_value"`
    MaxEquity   float64       `json:"max_equity"`
}
```

**修改檔案**: `internal/sim/engine.go`

在 `RunDay` 方法中，步驟 4 (記錄 daily metrics) 之後加入：

```go
// 5. Drawdown guard check
if e.drawdownGuard != nil {
    guardAction := e.drawdownGuard.Check(portfolioValue, state.MaxEquity)
    if guardAction.ShouldReducePosition {
        // 調整買入邏輯參數
        effectiveMaxPositionWeight *= guardAction.PositionScale
    }
    if guardAction.ShouldHaltNewPositions {
        // 跳過買入邏輯
        skipBuyLogic = true
    }
    if guardAction.ShouldLiquidate {
        // 強制平倉
        e.liquidateAllPositions(state, quoteBySymbol)
    }
}
```

**修改檔案**: `internal/domain/types.go`

在 `SimulationConstraints` 中新增：

```go
type SimulationConstraints struct {
    // ... existing fields ...
    
    // DrawdownGuardEnabled 是否啟用組合回撤防護
    DrawdownGuardEnabled bool `json:"drawdown_guard_enabled"`
    
    // DrawdownGuardConfig 三層防護設定 (JSON 格式)
    DrawdownGuardConfig string `json:"drawdown_guard_config,omitempty"`
}
```

### 2.5 配置範例

`configs/drawdown-guard.json`:

```json
{
  "yellow_threshold": 0.15,
  "orange_threshold": 0.25,
  "red_threshold": 0.35,
  "position_scale_yellow": 0.5,
  "cash_reserve_yellow": 0.15,
  "pause_days_red": 3
}
```

### 2.6 測試策略

- `TestDrawdownGuard_YellowTrigger`: 驗證 16% 回撤觸發黃色預警，倉位減半
- `TestDrawdownGuard_OrangeTrigger`: 驗證 26% 回撤觸發橙色限制，禁止新倉
- `TestDrawdownGuard_RedTrigger`: 驗證 36% 回撤觸發紅色熔斷，強制清倉
- `TestDrawdownGuard_Recovery`: 驗證回撤收斂後自動解除限制

---

## 3. 問題 B: Darwinian 權重靜默夾制 — 可觀測夾制機制

### 3.1 現況分析

現有 `constrainWeight()` 函式：

```go
func (m *DarwinianWeightManager) constrainWeight(weight float64) float64 {
    if weight < DarwinianWeightMin {  // 0.3
        return DarwinianWeightMin
    }
    if weight > DarwinianWeightMax {  // 2.5
        return DarwinianWeightMax
    }
    return weight
}
```

**問題**: 超出邊界時靜默截斷，無記錄、無警告。長期累積可能導致：
- 表現極差的 Agent 被卡在 0.3，但系統無法識別「持續觸底」
- 表現極佳的 Agent 被卡在 2.5，但系統無法識別「持續封頂」

### 3.2 設計目標

- 每次截斷都記錄於 `DarwinianAgentWeight` 結構中
- 連續截斷達到閾值時觸發警告
- 報告中標記截斷狀態，供審查

### 3.3 實作設計

**修改檔案**: `internal/portfolio/darwinian_weights.go`

新增欄位至 `DarwinianAgentWeight`:

```go
type DarwinianAgentWeight struct {
    // ... existing fields ...
    
    // ClippingHistory 記錄最近 30 次截斷事件
    ClippingHistory []ClippingEvent `json:"clipping_history,omitempty"`
    
    // ConsecutiveClips 連續截斷次數
    ConsecutiveClips int `json:"consecutive_clips"`
    
    // IsCurrentlyClipped 當前是否處於截斷狀態
    IsCurrentlyClipped bool `json:"is_currently_clipped"`
    
    // ClipDirection 截斷方向: "min" 或 "max"
    ClipDirection string `json:"clip_direction,omitempty"`
}

type ClippingEvent struct {
    Timestamp    time.Time `json:"timestamp"`
    OriginalVal  float64   `json:"original_value"`
    ClippedVal   float64   `json:"clipped_value"`
    Direction    string    `json:"direction"` // "min" or "max"
    Reason       string    `json:"reason"`    // e.g., "daily_adjustment", "manual_override"
}
```

修改 `constrainWeight` 方法：

```go
func (m *DarwinianWeightManager) constrainWeight(weight float64, agentID string, reason string) float64 {
    var clipped float64
    var direction string
    
    if weight < DarwinianWeightMin {
        clipped = DarwinianWeightMin
        direction = "min"
    } else if weight > DarwinianWeightMax {
        clipped = DarwinianWeightMax
        direction = "max"
    } else {
        // 未觸發截斷，重置連續計數
        if w, ok := m.weights[agentID]; ok {
            w.ConsecutiveClips = 0
            w.IsCurrentlyClipped = false
            w.ClipDirection = ""
        }
        return weight
    }
    
    // 觸發截斷，記錄事件
    if w, ok := m.weights[agentID]; ok {
        event := ClippingEvent{
            Timestamp:   time.Now(),
            OriginalVal: weight,
            ClippedVal:  clipped,
            Direction:   direction,
            Reason:      reason,
        }
        
        w.ClippingHistory = append(w.ClippingHistory, event)
        if len(w.ClippingHistory) > 30 {
            w.ClippingHistory = w.ClippingHistory[1:]
        }
        
        // 更新連續截斷計數
        if w.ClipDirection == direction {
            w.ConsecutiveClips++
        } else {
            w.ConsecutiveClips = 1
        }
        w.ClipDirection = direction
        w.IsCurrentlyClipped = true
        
        // 連續 3 次以上觸發警告
        if w.ConsecutiveClips >= 3 {
            log.Printf("[DARWINIAN-WARNING] Agent %s clipped %s for %d consecutive adjustments. "+
                "Current weight: %.2f. Consider prompt review or disabling.", 
                agentID, direction, w.ConsecutiveClips, clipped)
        }
    }
    
    return clipped
}
```

**修改報告生成**: `GenerateReport()`

在報告中新增截斷統計：

```go
type DarwinianWeightReport struct {
    // ... existing fields ...
    
    // ClippedAgents 當前處於截斷狀態的 Agent 列表
    ClippedAgents []ClippedAgentInfo `json:"clipped_agents"`
    
    // TotalClippingEvents 今日截斷事件總數
    TotalClippingEvents int `json:"total_clipping_events"`
}

type ClippedAgentInfo struct {
    AgentID          string  `json:"agent_id"`
    Weight           float64 `json:"weight"`
    Direction        string  `json:"direction"`
    ConsecutiveClips int     `json:"consecutive_clips"`
}
```

### 3.4 測試策略

- `TestConstrainWeight_LogsClip`: 驗證截斷時記錄事件
- `TestConstrainWeight_ConsecutiveWarning`: 驗證連續 3 次觸發警告
- `TestConstrainWeight_ResetOnUnclip`: 驗證未截斷時重置連續計數

---

## 4. 問題 C: 實驗樣本偏差 — 統計嚴謹性強化

### 4.1 現況分析

現有觀測值門檻 (`internal/experiment/judge.go`):

```go
func requiredObservationCountForMaturity(maturity string) int {
    switch maturity {
    case "level_3_regime_aware":
        return 12
    case "level_2_window_validated", "level_2_validated":
        return 8
    case "level_1_exploratory":
        return 3
    default:
        return 3
    }
}
```

**問題**: 
- level_1 僅需 3 筆觀測，統計信度極低
- 無 Sharpe ratio 標準誤差檢查
- 無 Out-of-sample 驗證要求

### 4.2 設計目標

- 提高最小觀測值門檻
- 添加統計穩定性檢查 (標準誤差)
- 強制 Candidate 在獨立窗口驗證

### 4.3 實作設計

**修改檔案**: `internal/experiment/judge.go`

#### 4.3.1 提高觀測值門檻

```go
func requiredObservationCountForMaturity(maturity string) int {
    switch maturity {
    case "level_3_regime_aware":
        return 25  // 從 12 提升至 25
    case "level_2_window_validated", "level_2_validated":
        return 15  // 從 8 提升至 15
    case "level_1_exploratory":
        return 8   // 從 3 提升至 8
    default:
        return 8   // 從 3 提升至 8
    }
}
```

#### 4.3.2 新增統計穩定性檢查

```go
// SharpeStabilityCheck 檢查 Sharpe ratio 估計的穩定性
type SharpeStabilityCheck struct {
    RequiredMaxSE float64  // 0.5 (最大允許標準誤差)
}

func (c *SharpeStabilityCheck) Check(observations []float64) (bool, float64) {
    if len(observations) < 10 {
        return false, 0
    }
    
    mean, stdDev := calculateMeanStdDev(observations)
    if stdDev == 0 {
        return false, 0
    }
    
    sharpe := mean / stdDev
    // Sharpe SE ≈ sqrt((1 + 0.5*sharpe^2) / n)
    se := math.Sqrt((1 + 0.5*sharpe*sharpe) / float64(len(observations)))
    
    return se <= c.RequiredMaxSE, se
}
```

#### 4.3.3 新增 Out-of-sample 驗證

```go
// OutOfSampleValidator 獨立窗口驗證器
type OutOfSampleValidator struct {
    store          *ledger.Store
    replayDataPath string
}

// Validate 要求 Candidate 在第二個獨立窗口也優於 Baseline
func (v *OutOfSampleValidator) Validate(
    brief domain.MutationBrief,
    candidatePromptPath string,
    primaryWindowID string,
) (bool, string, error) {
    // 1. 尋找與 primary window 不重疊的 secondary window
    secondaryWindow, err := v.findNonOverlappingWindow(primaryWindowID)
    if err != nil {
        return false, "", err
    }
    
    // 2. 在 secondary window 上評估 Baseline
    baselineScore, err := v.evaluatePrompt(brief, "", secondaryWindow)
    if err != nil {
        return false, "", err
    }
    
    // 3. 在 secondary window 上評估 Candidate
    candidateScore, err := v.evaluatePrompt(brief, candidatePromptPath, secondaryWindow)
    if err != nil {
        return false, "", err
    }
    
    // 4. 檢查 Candidate 是否仍優於 Baseline
    if candidateScore <= baselineScore {
        return false, fmt.Sprintf(
            "Out-of-sample validation failed: candidate %.4f <= baseline %.4f in window %s",
            candidateScore, baselineScore, secondaryWindow.ID,
        ), nil
    }
    
    return true, fmt.Sprintf(
        "Out-of-sample validated: candidate %.4f > baseline %.4f in window %s",
        candidateScore, baselineScore, secondaryWindow.ID,
    ), nil
}
```

#### 4.3.4 整合至 Judge.Evaluate

```go
func (j *Judge) Evaluate(resultPath string) (domain.PromptExperimentResult, error) {
    // ... existing code ...
    
    // 新增: 統計穩定性檢查
    stabilityCheck := SharpeStabilityCheck{RequiredMaxSE: 0.5}
    isStable, se := stabilityCheck.Check(/* candidate observations */)
    if !isStable {
        checks = append(checks, fmt.Sprintf("sharpe stability check failed: SE=%.4f > 0.5", se))
        // 不直接拒絕，但記錄警告
    } else {
        checks = append(checks, fmt.Sprintf("sharpe stability check passed: SE=%.4f", se))
    }
    
    // 新增: Out-of-sample 驗證 (僅對 level_2 以上)
    if isLevel2OrAbove(result.Brief.MaturityLevel) {
        oosValidator := OutOfSampleValidator{store: j.store, replayDataPath: j.replayDataPath}
        oosPassed, oosMsg, err := oosValidator.Validate(result.Brief, result.CandidatePrompt, result.Brief.WindowID)
        if err != nil {
            return domain.PromptExperimentResult{}, fmt.Errorf("out-of-sample validation: %w", err)
        }
        checks = append(checks, oosMsg)
        if !oosPassed {
            accepted = false
            acceptanceNote = "rejected: out-of-sample validation failed"
        }
    }
    
    // ... existing code ...
}
```

### 4.4 向後相容性

- 現有實驗結果不受影響 (僅修改 Judge 邏輯，不修改已儲存結果)
- `level_1` 實驗仍可執行，但門檻提高 (3 → 8)
- Out-of-sample 僅對 `level_2` 以上強制要求

### 4.5 測試策略

- `TestRequiredObservation_Level1`: 驗證 level_1 需 8 筆
- `TestSharpeStabilityCheck`: 驗證標準誤差計算與閾值判斷
- `TestOutOfSampleValidator`: 驗證獨立窗口評估邏輯
- `TestJudgeIntegration`: 驗證完整 Judge 流程包含新檢查

---

## 5. 實作優先順序

| 優先級 | 項目 | 預估工作量 | 風險降低效果 |
|--------|------|-----------|-------------|
| P0 | 問題 A: 三層熔斷機制 | 2 天 | **最高** (防止 -60% 回撤) |
| P1 | 問題 C: 觀測值門檻提高 | 0.5 天 | 高 (防止過擬合) |
| P2 | 問題 B: 可觀測夾制 | 1 天 | 中 (提升透明度) |
| P3 | 問題 C: Out-of-sample 驗證 | 1.5 天 | 中 (增強穩健性) |
| P4 | 問題 C: Sharpe 穩定性檢查 | 1 天 | 中 (統計嚴謹性) |

---

## 6. 驗收標準

### 6.1 功能驗收

- [ ] 模擬 40% 回撤情境，系統觸發紅色熔斷並強制清倉
- [ ] Darwinian 權重連續 3 次觸底後，日誌輸出警告訊息
- [ ] level_1 實驗提交 5 筆觀測值時，Judge 明確拒絕並提示需要 8 筆

### 6.2 測試驗收

- [ ] `go test ./internal/risk/...` 全部通過 (新增 drawdown guard 測試)
- [ ] `go test ./internal/portfolio/...` 全部通過 (新增 clipping 測試)
- [ ] `go test ./internal/experiment/...` 全部通過 (新增 observation/OOS 測試)
- [ ] `go test ./...` 總覆蓋率維持 ≥ 40%

### 6.3 稽核驗收

- [ ] `data/state/drawdown_events.jsonl` 記錄所有熔斷觸發
- [ ] `data/state/darwinian_weights.json` 包含 `clipping_history`
- [ ] Experiment JSON 包含 `out_of_sample_result` 欄位

---

## 7. 風險評估

| 風險 | 可能性 | 影響 | 緩解措施 |
|------|--------|------|---------|
| 熔斷機制過度觸發 | 中 | 中 | 閾值可配置，允許根據市場環境調整 |
| 觀測值門檻提高導致實驗週期變長 | 高 | 低 | level_1 僅需 8 筆，對日常迭代影響有限 |
| 向後相容性問題 | 低 | 中 | 所有變更均為新增欄位，不影響既有資料 |

---

## 8. 待決定事項

1. **紅色熔斷後暫停天數**: 設計為 3 天，是否需根據 regime 調整？
2. **Out-of-sample 窗口選擇**: 優先選擇「時間不重疊」或「regime 不同」？
3. **Sharpe SE 閾值**: 設計為 0.5，是否需要根據樣本數動態調整？

---

*設計文件版本: 1.0*  
*最後更新: 2026-04-23*  
*狀態: 等待用戶審查與批准*
