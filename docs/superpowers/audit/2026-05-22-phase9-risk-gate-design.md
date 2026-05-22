# Phase 9: 自我風控機制 — 詳細技術設計方案

> **狀態**: 設計草案  
> **日期**: 2026-05-22  
> **相依**: Phase 8（控制與稽核）修復

---

## 1. 設計目標

將分散在 7 個模組的風控訊號收斂為 **Unified Risk Gate**，實現：

1. **Pre-trade Gate**: 下單前強制檢查（硬攔截）
2. **In-trade Gate**: 持倉中即時監控（自動執行）
3. **Post-trade Gate**: 盤後評估與策略調整（自動降級/暫停）

核心理念：**從「人看報告做決定」升級為「系統自動攔截 + 人可覆寫」**。

---

## 2. 新增檔案結構

```
internal/risk/
├── gate.go           # Unified Risk Gate 入口 + RiskGate interface
├── pre_trade.go      # Pre-trade Gate 實作
├── in_trade.go       # In-trade Gate 實作（即時監控循環）
├── post_trade.go     # Post-trade Gate 實作（盤後評估）
├── decision.go       # RiskDecision, RiskAction, Verdict 等型別定義
├── audit.go          # RiskAuditLog 風控事件持久化
├── gate_test.go       # 整合測試
├── pre_trade_test.go
├── in_trade_test.go
└── post_trade_test.go
```

---

## 3. 核心型別定義 (`decision.go`)

```go
package risk

import "time"

// RiskPhase 風控階段
type RiskPhase string

const (
    PhasePreTrade  RiskPhase = "pre_trade"
    PhaseInTrade   RiskPhase = "in_trade"
    PhasePostTrade RiskPhase = "post_trade"
)

// Verdict 風控判決
type Verdict string

const (
    VerdictAllow     Verdict = "ALLOW"       // 放行
    VerdictReduce    Verdict = "REDUCE"      // 減碼（依 target_pct）
    VerdictBlock     Verdict = "BLOCK"       // 攔截（本次不下單）
    VerdictHalt      Verdict = "HALT"        // 緊急停止（賣出/凍結）
    VerdictAlertOnly Verdict = "ALERT_ONLY"  // 僅告警
)

// ActionType 風控動作類型
type ActionType string

const (
    ActionSell      ActionType = "SELL"
    ActionReduce    ActionType = "REDUCE"
    ActionFreeze    ActionType = "FREEZE"
    ActionLiquidate ActionType = "LIQUIDATE"
    ActionNotify    ActionType = "NOTIFY"
)

// RiskDecision 風控決策
type RiskDecision struct {
    Phase       RiskPhase    `json:"phase"`
    Verdict     Verdict      `json:"verdict"`
    Action      RiskAction   `json:"action"`
    Reason      string       `json:"reason"`
    Details     []RuleResult `json:"details"`
    Timestamp   time.Time    `json:"timestamp"`
    Overridable bool         `json:"overridable"`
}

// RiskAction 具體動作
type RiskAction struct {
    Type        ActionType `json:"type"`
    TargetPct   float64    `json:"target_pct"`    // 目標倉位百分比（0=全部賣出，0.5=減半）
    Symbols     []string   `json:"symbols,omitempty"`
    Sectors     []string   `json:"sectors,omitempty"`
    Description string     `json:"description"`
}

// RuleResult 單一規則結果
type RuleResult struct {
    RuleName     string  `json:"rule_name"`
    Passed       bool    `json:"passed"`
    CurrentValue float64 `json:"current_value"`
    Threshold    float64 `json:"threshold"`
    Severity     string  `json:"severity"` // CRITICAL, WARNING, INFO
    Message      string  `json:"message,omitempty"`
}

// RiskGateStatus 風控閘道狀態
type RiskGateStatus struct {
    Active       bool            `json:"active"`
    Mode         string          `json:"mode"` // NORMAL, CAUTIOUS, DEFENSIVE, SUSPENDED
    LastDecision *RiskDecision   `json:"last_decision,omitempty"`
    RuleStats    map[string]int  `json:"rule_stats"` // rule_name → trigger count
    UpdatedAt    time.Time       `json:"updated_at"`
}
```

---

## 4. 參數配置（新增到 `configs/parameters.json`）

```json
{
  "risk_gate": {
    "pre_trade": {
      "max_position_pct": {
        "value": 0.15,
        "rationale": "單一持股最大曝險 15%",
        "source": "literature"
      },
      "max_sector_exposure_pct": {
        "value": 0.40,
        "rationale": "單一產業最大曝險 40%",
        "source": "heuristic"
      },
      "var_confidence_level": {
        "value": 0.95,
        "rationale": "95% VaR 作為曝險上限",
        "source": "literature"
      },
      "var_limit_pct": {
        "value": 0.02,
        "rationale": "VaR 不得超過組合價值 2%",
        "source": "heuristic"
      },
      "min_cash_buffer_pct": {
        "value": 0.05,
        "rationale": "至少保留 5% 現金緩衝",
        "source": "heuristic"
      },
      "max_correlation": {
        "value": 0.70,
        "rationale": "與現有持倉相關性 > 0.7 則降低權重",
        "source": "heuristic"
      },
      "min_adv_ratio": {
        "value": 0.01,
        "rationale": "下單量不得超過日均量 1%",
        "source": "literature"
      }
    },
    "in_trade": {
      "monitor_interval_sec": {
        "value": 30,
        "rationale": "每 30 秒檢查一次",
        "source": "heuristic"
      },
      "stop_loss_pct": {
        "value": 0.10,
        "rationale": "個股虧損達 10% 即止損",
        "source": "heuristic"
      },
      "take_profit_pct": {
        "value": 0.30,
        "rationale": "個股獲利達 30% 考慮部分獲利了結",
        "source": "heuristic"
      },
      "trailing_stop_atr_mult": {
        "value": 2.0,
        "rationale": "2x ATR trailing stop",
        "source": "literature"
      },
      "volatility_spike_mult": {
        "value": 3.0,
        "rationale": "波動率超過 3 倍歷史均值 → 減碼",
        "source": "empirical"
      },
      "circuit_breaker_daily_loss_pct": {
        "value": 0.05,
        "rationale": "單日組合虧損 5% → 暫停交易",
        "source": "heuristic"
      }
    },
    "post_trade": {
      "max_drawdown_halt_pct": {
        "value": 0.20,
        "rationale": "最大回撤 20% → SUSPENDED",
        "source": "heuristic"
      },
      "max_drawdown_defensive_pct": {
        "value": 0.10,
        "rationale": "最大回撤 10% → DEFENSIVE（減半倉）",
        "source": "heuristic"
      },
      "min_rolling_sharpe": {
        "value": 0.0,
        "rationale": "滾動 Sharpe < 0 → CAUTIOUS",
        "source": "literature"
      },
      "consecutive_loss_days": {
        "value": 5,
        "rationale": "連續虧損 5 天 → mute agent",
        "source": "heuristic"
      },
      "evaluation_interval_hours": {
        "value": 24,
        "rationale": "每日盤後評估",
        "source": "heuristic"
      }
    }
  }
}
```

---

## 5. RiskGate 介面 (`gate.go`)

```go
package risk

import (
    "context"
    "sync"

    "github.com/kaecer68/atlas-go/internal/config"
)

// RiskGate 統一風控入口
type RiskGate struct {
    mu           sync.RWMutex
    mode         string // NORMAL, CAUTIOUS, DEFENSIVE, SUSPENDED
    preTrade     *PreTradeGate
    inTrade      *InTradeGate
    postTrade    *PostTradeGate
    auditLog     *RiskAuditLog
    decisionCh   chan RiskDecision // 決策事件通道
    subscribers  []func(RiskDecision)
}

// NewRiskGate 建立風控閘道
func NewRiskGate(
    rm *RiskManager,
    vc *VaRCalculator,
    opt *PortfolioOptimizer,
    sizing *PositionSizer,
    auditLog *RiskAuditLog,
) *RiskGate {
    g := &RiskGate{
        mode:       "NORMAL",
        auditLog:   auditLog,
        decisionCh: make(chan RiskDecision, 100),
    }
    g.preTrade = NewPreTradeGate(rm, vc, opt, sizing)
    g.inTrade = NewInTradeGate()
    g.postTrade = NewPostTradeGate()
    return g
}

// PreTradeCheck 下單前檢查
func (g *RiskGate) PreTradeCheck(ctx context.Context, order OrderIntent, pf PortfolioState) (*RiskDecision, error) {
    g.mu.RLock()
    defer g.mu.RUnlock()
    
    // SUSPENDED 模式直接攔截
    if g.mode == "SUSPENDED" {
        return &RiskDecision{
            Phase:   PhasePreTrade,
            Verdict: VerdictBlock,
            Reason:  "risk gate suspended - all trading halted",
            Action: RiskAction{
                Type: ActionFreeze,
                Description: "系統已暫停所有交易，請聯繫風控官",
            },
            Overridable: false,
        }, nil
    }
    
    decision, err := g.preTrade.Check(ctx, order, pf, g.mode)
    if err != nil {
        return nil, err
    }
    
    // DEFENSIVE 模式下自動降低目標倉位
    if g.mode == "DEFENSIVE" && decision.Action.TargetPct > 0.5 {
        decision.Action.TargetPct = 0.5
    }
    
    // 記錄審計
    g.auditLog.Record(ctx, decision)
    
    // 發布事件
    select {
    case g.decisionCh <- *decision:
    default:
    }
    
    return decision, nil
}

// InTradeCheck 持倉中檢查
func (g *RiskGate) InTradeCheck(ctx context.Context, pf PortfolioState, mkt MarketSnapshot) (*RiskDecision, error) {
    g.mu.RLock()
    defer g.mu.RUnlock()
    
    decision, err := g.inTrade.Check(ctx, pf, mkt)
    if err != nil {
        return nil, err
    }
    
    if decision.Verdict == VerdictHalt {
        g.auditLog.Record(ctx, decision)
        select {
        case g.decisionCh <- *decision:
        default:
        }
    }
    
    return decision, nil
}

// PostTradeEval 盤後評估 → 自動調整 mode
func (g *RiskGate) PostTradeEval(ctx context.Context, session SessionSummary) (*RiskDecision, error) {
    g.mu.Lock()
    defer g.mu.Unlock()
    
    decision, err := g.postTrade.Evaluate(ctx, session)
    if err != nil {
        return nil, err
    }
    
    // 根據決策自動調整 mode
    switch decision.Verdict {
    case VerdictHalt:
        g.mode = "SUSPENDED"
    case VerdictReduce:
        g.mode = "DEFENSIVE"
    case VerdictAlertOnly:
        g.mode = "CAUTIOUS"
    default:
        g.mode = "NORMAL"
    }
    
    g.auditLog.Record(ctx, decision)
    return decision, nil
}

// SetMode 人工設定風控模式（需審批）
func (g *RiskGate) SetMode(mode string, operator string) error {
    validModes := map[string]bool{"NORMAL": true, "CAUTIOUS": true, "DEFENSIVE": true, "SUSPENDED": true}
    if !validModes[mode] {
        return fmt.Errorf("invalid mode: %s", mode)
    }
    
    g.mu.Lock()
    defer g.mu.Unlock()
    g.mode = mode
    
    g.auditLog.Record(context.Background(), &RiskDecision{
        Phase:   PhasePostTrade,
        Verdict: VerdictAlertOnly,
        Reason:  fmt.Sprintf("manual mode change to %s by %s", mode, operator),
        Action: RiskAction{
            Type: ActionNotify,
            Description: fmt.Sprintf("Risk gate mode manually set to %s", mode),
        },
    })
    
    return nil
}

// GetStatus 取得目前狀態
func (g *RiskGate) GetStatus() RiskGateStatus {
    g.mu.RLock()
    defer g.mu.RUnlock()
    return RiskGateStatus{
        Active: true,
        Mode:   g.mode,
    }
}

// Subscribe 訂閱風控事件
func (g *RiskGate) Subscribe(fn func(RiskDecision)) {
    g.mu.Lock()
    defer g.mu.Unlock()
    g.subscribers = append(g.subscribers, fn)
}

// Start 啟動 In-trade 監控循環
func (g *RiskGate) Start(ctx context.Context, taskMgr *BackgroundTaskManager) {
    g.inTrade.Start(ctx, func(pf PortfolioState, mkt MarketSnapshot) {
        decision, _ := g.InTradeCheck(ctx, pf, mkt)
        if decision.Verdict == VerdictHalt {
            // 通知所有訂閱者
            for _, sub := range g.subscribers {
                sub(*decision)
            }
        }
    }, taskMgr)
}
```

---

## 6. 與 Orchestrator 整合

```go
// internal/orchestrator/system.go 修改點

type System struct {
    // ... 現有欄位 ...
    riskGate *risk.RiskGate  // 新增
}

func NewSystem(..., riskGate *risk.RiskGate) *System {
    s := &System{
        // ...
        riskGate: riskGate,
    }
    // 訂閱 HALT 事件 → 自動停止所有 agent
    riskGate.Subscribe(func(d risk.RiskDecision) {
        if d.Verdict == risk.VerdictHalt {
            s.StopAllAgents(d.Reason)
        }
    })
    return s
}

// executeTrade 下單前必須通過 Pre-trade Gate
func (s *System) executeTrade(ctx context.Context, order OrderIntent) error {
    // 1. Pre-trade Gate
    decision, err := s.riskGate.PreTradeCheck(ctx, order, s.portfolio.State())
    if err != nil {
        s.logger.Error("risk gate error", "err", err)
        return fmt.Errorf("risk gate: %w", err)
    }
    
    if decision.Verdict == risk.VerdictBlock || decision.Verdict == risk.VerdictHalt {
        s.logger.Warn("order blocked", "symbol", order.Symbol, "reason", decision.Reason)
        // 記錄被攔截的訂單
        s.auditLog.RecordBlockedOrder(ctx, order, decision)
        return fmt.Errorf("risk gate blocked: %s", decision.Reason)
    }
    
    // 2. REDUCE → 調整下單量
    if decision.Verdict == risk.VerdictReduce {
        order.Quantity = int(float64(order.Quantity) * decision.Action.TargetPct)
        if order.Quantity == 0 {
            return fmt.Errorf("order reduced to zero by risk gate")
        }
    }
    
    // 3. 執行下單
    return s.broker.PlaceOrder(ctx, order)
}

// runDailyPostTrade 每日盤後評估（由 BackgroundTaskManager 排程）
func (s *System) runDailyPostTrade(ctx context.Context) error {
    session := s.GetTodaySession()
    decision, err := s.riskGate.PostTradeEval(ctx, session)
    if err != nil {
        return err
    }
    
    // 根據 mode 調整策略
    switch s.riskGate.GetStatus().Mode {
    case "SUSPENDED":
        s.StopAllAgents("risk gate suspended")
    case "DEFENSIVE":
        s.FactorWeightEngine.SetMode("defensive")
        s.PositionSizer.SetMaxPositionPct(0.07) // 降為 7%
    case "CAUTIOUS":
        s.FactorWeightEngine.SetMode("conservative")
    default:
        s.FactorWeightEngine.SetMode("normal")
    }
    
    s.logger.Info("post-trade eval completed", "mode", s.riskGate.GetStatus().Mode)
    return nil
}
```

---

## 7. 前端風控面板（新增）

```javascript
// web/static/js/components/risk-gate-panel.js

export async function renderRiskGatePanel(containerId) {
    const container = document.getElementById(containerId);
    
    // 取得風控狀態
    const status = await fetchAPI('/api/risk/gate-status');
    const history = await fetchAPI('/api/risk/gate-history?limit=20');
    
    container.innerHTML = `
        <div class="risk-gate-panel">
            <!-- 模式指示器 -->
            <div class="gate-mode mode-${status.mode.toLowerCase()}">
                <span class="mode-label">風控模式</span>
                <span class="mode-value">${status.mode}</span>
            </div>
            
            <!-- 手動模式切換 -->
            <div class="mode-controls">
                <button onclick="setRiskMode('NORMAL')" ${status.mode === 'NORMAL' ? 'disabled' : ''}>正常</button>
                <button onclick="setRiskMode('CAUTIOUS')" ${status.mode === 'CAUTIOUS' ? 'disabled' : ''}>謹慎</button>
                <button onclick="setRiskMode('DEFENSIVE')" ${status.mode === 'DEFENSIVE' ? 'disabled' : ''}>防禦</button>
                <button onclick="setRiskMode('SUSPENDED')" ${status.mode === 'SUSPENDED' ? 'disabled' : ''}>暫停</button>
            </div>
            
            <!-- 最近風控事件 -->
            <div class="gate-history">
                <h4>最近風控事件</h4>
                ${history.map(e => renderRiskEvent(e)).join('')}
            </div>
            
            <!-- 規則觸發統計 -->
            <div class="rule-stats">
                <h4>規則觸發統計（本週）</h4>
                ${Object.entries(status.rule_stats).map(([rule, count]) => 
                    `<div class="stat-row"><span>${rule}</span><span>${count} 次</span></div>`
                ).join('')}
            </div>
        </div>
    `;
}

function renderRiskEvent(event) {
    const severityClass = event.severity === 'CRITICAL' ? 'critical' : 
                          event.severity === 'WARNING' ? 'warning' : 'info';
    return `
        <div class="risk-event ${severityClass}">
            <span class="event-time">${formatTime(event.timestamp)}</span>
            <span class="event-phase">${event.phase}</span>
            <span class="event-verdict">${event.verdict}</span>
            <span class="event-reason">${event.reason}</span>
        </div>
    `;
}
```

---

## 8. 測試策略

```go
// internal/risk/gate_test.go

func TestPreTradeGate_BlocksOverMaxPosition(t *testing.T) {
    gate := setupTestGate()
    order := OrderIntent{Symbol: "2330", Notional: 200_000} // 20% of portfolio
    pf := PortfolioState{TotalValue: 1_000_000, Positions: make(map[string]float64)}
    pf.Positions["2330"] = 0 // no existing position
    
    decision, err := gate.PreTradeCheck(context.Background(), order, pf)
    require.NoError(t, err)
    assert.Equal(t, VerdictBlock, decision.Verdict, "should block order > 15% max position")
    assert.Contains(t, decision.Reason, "max_position_pct")
}

func TestInTradeGate_StopLossSell(t *testing.T) {
    gate := setupTestGate()
    gate.inTrade.RegisterPosition("2330", 500.0)
    gate.inTrade.SetStopLoss("2330", 450.0) // 10% below entry
    
    // 價格跌到 450
    decision, err := gate.InTradeCheck(context.Background(),
        PortfolioState{Positions: map[string]float64{"2330": 500}},
        MarketSnapshot{Prices: map[string]float64{"2330": 450}},
    )
    
    require.NoError(t, err)
    assert.Equal(t, VerdictHalt, decision.Verdict)
    assert.Equal(t, ActionSell, decision.Action.Type)
    assert.Contains(t, decision.Reason, "stop-loss")
}

func TestPostTradeGate_DrawdownTriggersHalt(t *testing.T) {
    gate := setupTestGate()
    session := SessionSummary{
        MaxDrawdown: 0.25, // 25% drawdown
        RollingSharpe: 0.5,
    }
    
    decision, err := gate.PostTradeEval(context.Background(), session)
    require.NoError(t, err)
    assert.Equal(t, VerdictHalt, decision.Verdict)
    
    // 確認 mode 已更新
    assert.Equal(t, "SUSPENDED", gate.GetStatus().Mode)
}
```

---

> **設計原則總結**:
> 1. **統一入口**: 所有風控檢查經由單一 RiskGate，不允許繞過
> 2. **硬攔截 + 可覆寫**: 預設自動攔截，但保留人工覆寫通道（需審批）
> 3. **審計閉環**: 每個決策都記錄，可回溯驗證有效性
> 4. **漸進式防禦**: NORMAL → CAUTIOUS → DEFENSIVE → SUSPENDED 四級自動切換
> 5. **參數集中管理**: 所有閾值在 parameters.json，支援熱更新
