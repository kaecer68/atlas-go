# 實驗系統安全強化實施計劃

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 針對模擬交易實驗系統的三個安全問題（極端回撤、靜默權重夾制、樣本偏差）實施最小侵入性改善，所有變更必須可稽核、可測試。

**Architecture:** (1) 在 `internal/risk/` 新增組合層級三層熔斷機制；(2) 擴展 `internal/portfolio/darwinian_weights.go` 的截斷透明度；(3) 強化 `internal/experiment/judge.go` 的統計嚴謹性。三個子系統獨立實施，無依賴關係。

**Tech Stack:** Go 1.25, native Go testing, JSONL append-only persistence

---

## 優先順序與批次

| 批次 | 任務 | 預估時間 | 獨立性 |
|------|------|---------|--------|
| Wave 1 | Task 1-5: 三層熔斷機制 (問題 A) | ~2 天 | 獨立 |
| Wave 2 | Task 6-8: 觀測值門檻提高 (問題 C-P1) | ~0.5 天 | 獨立 |
| Wave 3 | Task 9-12: 可觀測夾制 (問題 B) | ~1 天 | 獨立 |
| Wave 4 | Task 13-15: Out-of-sample + Sharpe 穩定性 (問題 C-P3/P4) | ~2.5 天 | 獨立 |

---

## Wave 1: 三層熔斷機制 (問題 A)

### Task 1: 建立 `internal/risk/drawdown_guard.go` 核心結構

**Files:**
- Create: `internal/risk/drawdown_guard.go`
- Test: `internal/risk/drawdown_guard_test.go`

- [ ] **Step 1: 寫失敗測試 — DrawdownLevel 判定**

```go
package risk

import (
    "testing"
    "time"
)

func TestDrawdownGuard_Check_YellowTrigger(t *testing.T) {
    guard := NewDrawdownGuard(DrawdownGuardConfig{
        YellowThreshold:  0.15,
        OrangeThreshold:  0.25,
        RedThreshold:     0.35,
        PositionScaleYellow: 0.5,
        CashReserveYellow:   0.15,
        PauseDaysRed:        3,
    })
    
    // Portfolio value dropped 16% from max equity
    maxEquity := 1000000.0
    currentValue := 840000.0  // 16% drawdown
    
    action := guard.Check(currentValue, maxEquity)
    
    if action.Level != DrawdownYellow {
        t.Errorf("expected Yellow level, got %v", action.Level)
    }
    if !action.ShouldReducePosition {
        t.Errorf("expected ShouldReducePosition=true")
    }
    if action.PositionScale != 0.5 {
        t.Errorf("expected PositionScale=0.5, got %.2f", action.PositionScale)
    }
}

func TestDrawdownGuard_Check_RedTrigger(t *testing.T) {
    guard := NewDrawdownGuard(DrawdownGuardConfig{
        YellowThreshold: 0.15,
        OrangeThreshold: 0.25,
        RedThreshold:    0.35,
        PauseDaysRed:    3,
    })
    
    maxEquity := 1000000.0
    currentValue := 640000.0  // 36% drawdown
    
    action := guard.Check(currentValue, maxEquity)
    
    if action.Level != DrawdownRed {
        t.Errorf("expected Red level, got %v", action.Level)
    }
    if !action.ShouldLiquidate {
        t.Errorf("expected ShouldLiquidate=true")
    }
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/risk/ -run TestDrawdownGuard_Check_YellowTrigger -v
```

Expected: FAIL — `NewDrawdownGuard` undefined, `DrawdownGuardConfig` undefined

- [ ] **Step 3: 實作最小核心結構**

```go
package risk

import (
    "fmt"
    "time"
)

// DrawdownLevel 定義回撤防護層級
type DrawdownLevel int

const (
    DrawdownNormal DrawdownLevel = iota
    DrawdownYellow
    DrawdownOrange
    DrawdownRed
)

func (l DrawdownLevel) String() string {
    switch l {
    case DrawdownYellow:
        return "yellow"
    case DrawdownOrange:
        return "orange"
    case DrawdownRed:
        return "red"
    default:
        return "normal"
    }
}

// DrawdownGuardConfig 三層防護設定
type DrawdownGuardConfig struct {
    YellowThreshold     float64 `json:"yellow_threshold"`
    OrangeThreshold     float64 `json:"orange_threshold"`
    RedThreshold        float64 `json:"red_threshold"`
    PositionScaleYellow float64 `json:"position_scale_yellow"`
    CashReserveYellow   float64 `json:"cash_reserve_yellow"`
    PauseDaysRed        int     `json:"pause_days_red"`
}

// DrawdownAction 熔斷觸發後的動作指令
type DrawdownAction struct {
    Level                DrawdownLevel `json:"level"`
    DrawdownPct          float64       `json:"drawdown_pct"`
    ShouldReducePosition bool          `json:"should_reduce_position"`
    PositionScale        float64       `json:"position_scale"`
    ShouldHaltNewPositions bool        `json:"should_halt_new_positions"`
    ShouldLiquidate      bool          `json:"should_liquidate"`
    PauseDays            int           `json:"pause_days"`
}

// DrawdownEvent 稽核用事件記錄
type DrawdownEvent struct {
    Timestamp      time.Time `json:"timestamp"`
    Level          string    `json:"level"`
    DrawdownPct    float64   `json:"drawdown_pct"`
    Action         string    `json:"action"`
    PortfolioValue float64   `json:"portfolio_value"`
    MaxEquity      float64   `json:"max_equity"`
}

// DrawdownGuard 組合回撤防護器
type DrawdownGuard struct {
    config      DrawdownGuardConfig
    history     []DrawdownEvent
    currentLevel DrawdownLevel
    lastTrigger time.Time
}

// NewDrawdownGuard 建立新的回撤防護器
func NewDrawdownGuard(config DrawdownGuardConfig) *DrawdownGuard {
    return &DrawdownGuard{
        config:      config,
        history:     make([]DrawdownEvent, 0),
        currentLevel: DrawdownNormal,
    }
}

// Check 檢查當前回撤並回傳動作指令
func (g *DrawdownGuard) Check(portfolioValue, maxEquity float64) DrawdownAction {
    if maxEquity <= 0 {
        return DrawdownAction{Level: DrawdownNormal}
    }
    
    drawdown := (maxEquity - portfolioValue) / maxEquity
    
    action := DrawdownAction{
        DrawdownPct: drawdown,
    }
    
    switch {
    case drawdown >= g.config.RedThreshold:
        action.Level = DrawdownRed
        action.ShouldLiquidate = true
        action.PauseDays = g.config.PauseDaysRed
        g.recordEvent("red", drawdown, portfolioValue, maxEquity)
        
    case drawdown >= g.config.OrangeThreshold:
        action.Level = DrawdownOrange
        action.ShouldHaltNewPositions = true
        g.recordEvent("orange", drawdown, portfolioValue, maxEquity)
        
    case drawdown >= g.config.YellowThreshold:
        action.Level = DrawdownYellow
        action.ShouldReducePosition = true
        action.PositionScale = g.config.PositionScaleYellow
        g.recordEvent("yellow", drawdown, portfolioValue, maxEquity)
        
    default:
        action.Level = DrawdownNormal
        // Reset level when recovery
        if g.currentLevel > DrawdownNormal {
            g.recordEvent("recovery", drawdown, portfolioValue, maxEquity)
        }
    }
    
    g.currentLevel = action.Level
    return action
}

// recordEvent 記錄熔斷事件至歷史
func (g *DrawdownGuard) recordEvent(level string, drawdown, portfolioValue, maxEquity float64) {
    event := DrawdownEvent{
        Timestamp:      time.Now(),
        Level:          level,
        DrawdownPct:    drawdown,
        Action:         fmt.Sprintf("drawdown_%s_triggered", level),
        PortfolioValue: portfolioValue,
        MaxEquity:      maxEquity,
    }
    g.history = append(g.history, event)
    g.lastTrigger = time.Now()
}

// GetHistory 回傳所有熔斷事件 (供稽核)
func (g *DrawdownGuard) GetHistory() []DrawdownEvent {
    result := make([]DrawdownEvent, len(g.history))
    copy(result, g.history)
    return result
}

// GetCurrentLevel 回傳當前防護層級
func (g *DrawdownGuard) GetCurrentLevel() DrawdownLevel {
    return g.currentLevel
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/risk/ -run TestDrawdownGuard_Check_YellowTrigger -v
go test ./internal/risk/ -run TestDrawdownGuard_Check_RedTrigger -v
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/risk/drawdown_guard.go internal/risk/drawdown_guard_test.go
git commit -m "feat(risk): add drawdown guard core structure with three-level circuit breaker

- Yellow (15%): reduce position size by 50%
- Orange (25%): halt new positions
- Red (35%): force liquidation + pause
- All events recorded for audit trail"
```

---

### Task 2: 整合 DrawdownGuard 至 Sim Engine

**Files:**
- Modify: `internal/sim/engine.go:15-25` (新增欄位)
- Modify: `internal/sim/engine.go:91-162` (RunDay 方法)
- Modify: `internal/domain/types.go` (新增 constraints 欄位)
- Test: `internal/sim/engine_test.go`

- [ ] **Step 1: 寫失敗測試 — Engine 觸發熔斷後減半倉位**

在 `internal/sim/engine_test.go` 新增：

```go
func TestEngine_DrawdownGuard_YellowReducesPosition(t *testing.T) {
    constraints := domain.SimulationConstraints{
        StartingCash:         1000000,
        MaxPositionWeight:    0.22,
        MaxOpenPositions:     5,
        StopLossPct:          0.08,
        DrawdownGuardEnabled: true,
    }
    
    engine := NewEngine(constraints)
    guard := risk.NewDrawdownGuard(risk.DrawdownGuardConfig{
        YellowThreshold:     0.15,
        PositionScaleYellow: 0.5,
    })
    engine.WithDrawdownGuard(guard)
    
    // First day: establish a high equity
    quotes := []domain.Quote{
        {Symbol: "2330.TW", Last: 500, Open: 500, IsTradable: true},
    }
    recs := []domain.Recommendation{
        {Agent: "test", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80},
    }
    
    result1 := engine.Run(domain.RegimeNeutral, quotes, recs)
    
    // Second day: massive drop to trigger yellow
    quotes2 := []domain.Quote{
        {Symbol: "2330.TW", Last: 400, Open: 400, IsTradable: true},
    }
    
    state := domain.NewSimulationState(constraints.StartingCash)
    // ... setup state to simulate the position from day 1 ...
    
    result2 := engine.RunWithState(&state, domain.RegimeNeutral, quotes2, recs)
    
    if guard.GetCurrentLevel() != risk.DrawdownYellow {
        t.Errorf("expected yellow level after 20%% drop")
    }
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/sim/ -run TestEngine_DrawdownGuard_YellowReducesPosition -v
```

Expected: FAIL — `WithDrawdownGuard` method undefined

- [ ] **Step 3: 修改 Engine 結構與 RunDay 方法**

在 `internal/sim/engine.go` Engine struct 新增欄位：

```go
type Engine struct {
    // ... existing fields ...
    drawdownGuard *risk.DrawdownGuard  // NEW
}
```

新增 With 方法：

```go
// WithDrawdownGuard attaches a drawdown guard for portfolio-level circuit breaker.
func (e *Engine) WithDrawdownGuard(g *risk.DrawdownGuard) *Engine {
    e.drawdownGuard = g
    return e
}
```

修改 `RunDay` 方法，在步驟 4 之後加入步驟 5：

```go
// 4. Record daily metrics
portfolioValue := state.PortfolioValue()
state.EquityCurve = append(state.EquityCurve, portfolioValue)
// ... existing equity curve logic ...

// 5. Drawdown guard check
var effectiveMaxPositionWeight = e.constraints.MaxPositionWeight
var effectiveCashReserve = e.constraints.ReserveCashFraction
var skipBuyLogic = false

if e.drawdownGuard != nil && e.constraints.DrawdownGuardEnabled {
    action := e.drawdownGuard.Check(portfolioValue, state.MaxEquity)
    
    switch action.Level {
    case risk.DrawdownYellow:
        effectiveMaxPositionWeight *= action.PositionScale
        effectiveCashReserve = e.constraints.ReserveCashFraction + 0.07 // boost reserve
        if effectiveCashReserve > 0.5 {
            effectiveCashReserve = 0.5
        }
    case risk.DrawdownOrange:
        skipBuyLogic = true
    case risk.DrawdownRed:
        skipBuyLogic = true
        // Force liquidation will be handled by executeSells
        e.liquidateAllPositions(state, quoteBySymbol)
    }
}

// Pass effective constraints to buy logic
if !skipBuyLogic {
    // ... existing buy logic but use effectiveMaxPositionWeight ...
}
```

新增 `liquidateAllPositions` 方法：

```go
func (e *Engine) liquidateAllPositions(
    state *domain.SimulationState,
    quoteBySymbol map[string]domain.Quote,
) []domain.Order {
    var orders []domain.Order
    remaining := make([]domain.Position, 0)
    
    for _, pos := range state.Positions {
        quote, ok := quoteBySymbol[pos.Symbol]
        if !ok || !quote.IsTradable {
            remaining = append(remaining, pos)
            continue
        }
        
        slippageBPS := e.getSlippageBPS(pos.Symbol, quoteBySymbol)
        price := applyBPS(quote.Last, -(slippageBPS + e.constraints.TransactionCostBPS))
        proceeds := float64(pos.Quantity) * price
        state.Cash += proceeds
        
        orders = append(orders, domain.Order{
            Symbol:   pos.Symbol,
            Side:     domain.SideSell,
            Quantity: pos.Quantity,
            Price:    price,
            Reason:   "drawdown_guard_red_liquidation",
        })
    }
    
    state.Positions = remaining
    return orders
}
```

- [ ] **Step 4: 修改 SimulationConstraints**

在 `internal/domain/types.go` 新增：

```go
type SimulationConstraints struct {
    // ... existing fields ...
    
    DrawdownGuardEnabled bool   `json:"drawdown_guard_enabled"`
    DrawdownGuardConfig  string `json:"drawdown_guard_config,omitempty"`
}
```

- [ ] **Step 5: 執行測試確認通過**

```bash
go test ./internal/sim/ -run TestEngine_DrawdownGuard_YellowReducesPosition -v
```

- [ ] **Step 6: Commit**

```bash
git add internal/sim/engine.go internal/sim/engine_test.go internal/domain/types.go
git commit -m "feat(sim): integrate drawdown guard into engine

- Engine.WithDrawdownGuard() for optional attachment
- Yellow: scale position size, boost cash reserve
- Orange: halt new positions
- Red: force liquidation with 'drawdown_guard_red_liquidation' reason
- Add DrawdownGuardEnabled/DrawdownGuardConfig to SimulationConstraints"
```

---

### Task 3: 建立熔斷事件持久化機制

**Files:**
- Create: `internal/ledger/drawdown_events.go`
- Test: `internal/ledger/drawdown_events_test.go`

- [ ] **Step 1: 寫失敗測試 — 事件寫入 JSONL**

```go
package ledger

import (
    "os"
    "testing"
    "time"
    
    "github.com/kaecer68/atlas-go/internal/risk"
)

func TestDrawdownEventStore_Write(t *testing.T) {
    tmpDir := t.TempDir()
    store := NewDrawdownEventStore(tmpDir + "/drawdown_events.jsonl")
    
    event := risk.DrawdownEvent{
        Timestamp:      time.Now(),
        Level:          "yellow",
        DrawdownPct:    0.16,
        Action:         "drawdown_yellow_triggered",
        PortfolioValue: 840000,
        MaxEquity:      1000000,
    }
    
    err := store.Write(event)
    if err != nil {
        t.Fatalf("write event: %v", err)
    }
    
    // Verify file exists
    _, err = os.Stat(tmpDir + "/drawdown_events.jsonl")
    if os.IsNotExist(err) {
        t.Errorf("expected file to exist")
    }
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/ledger/ -run TestDrawdownEventStore_Write -v
```

Expected: FAIL — `NewDrawdownEventStore` undefined

- [ ] **Step 3: 實作 DrawdownEventStore**

```go
package ledger

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"
    
    "github.com/kaecer68/atlas-go/internal/risk"
)

// DrawdownEventStore persists drawdown events to JSONL for audit trail.
type DrawdownEventStore struct {
    path string
    mu   sync.Mutex
}

// NewDrawdownEventStore creates a new store at the given path.
func NewDrawdownEventStore(path string) *DrawdownEventStore {
    return &DrawdownEventStore{path: path}
}

// Write appends a drawdown event to the JSONL file.
func (s *DrawdownEventStore) Write(event risk.DrawdownEvent) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    dir := filepath.Dir(s.path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return fmt.Errorf("create directory: %w", err)
    }
    
    record := struct {
        RecordedAt time.Time `json:"recorded_at"`
        risk.DrawdownEvent
    }{
        RecordedAt:      time.Now(),
        DrawdownEvent:   event,
    }
    
    line, err := json.Marshal(record)
    if err != nil {
        return fmt.Errorf("marshal event: %w", err)
    }
    
    f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
    if err != nil {
        return fmt.Errorf("open file: %w", err)
    }
    defer f.Close()
    
    if _, err := f.Write(append(line, '\n')); err != nil {
        return fmt.Errorf("write event: %w", err)
    }
    
    return nil
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/ledger/ -run TestDrawdownEventStore_Write -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/ledger/drawdown_events.go internal/ledger/drawdown_events_test.go
git commit -m "feat(ledger): add DrawdownEventStore for audit trail persistence

- Append-only JSONL format
- Thread-safe with mutex"
```

---

### Task 4: 配置檔與預設值

**Files:**
- Create: `configs/drawdown-guard.json`
- Modify: `internal/config/config.go` (若存在載入邏輯)

- [ ] **Step 1: 建立預設配置檔**

```json
{
  "yellow_threshold": 0.15,
  "orange_threshold": 0.25,
  "red_threshold": 0.35,
  "position_scale_yellow": 0.5,
  "cash_reserve_yellow": 0.15,
  "pause_days_red": 3,
  "_comment": "Three-level portfolio drawdown protection. Adjust thresholds based on risk appetite."
}
```

- [ ] **Step 2: Commit**

```bash
git add configs/drawdown-guard.json
git commit -m "config: add drawdown guard default configuration"
```

---

### Task 5: Wave 1 驗收測試

- [ ] **執行完整套件測試**

```bash
go test ./internal/risk/... -v
go test ./internal/sim/... -v
go test ./internal/ledger/... -v
```

Expected: ALL PASS

- [ ] **執行覆蓋率檢查**

```bash
go test -coverprofile=coverage.out ./internal/risk/...
go tool cover -func=coverage.out | grep total
```

Expected: coverage ≥ 40%

---

## Wave 2: 觀測值門檻提高 (問題 C - P1)

### Task 6: 修改 requiredObservationCountForMaturity

**Files:**
- Modify: `internal/experiment/judge.go:290-301`
- Test: `internal/experiment/judge_test.go`

- [ ] **Step 1: 寫失敗測試 — 驗證新門檻**

```go
func TestRequiredObservationCountForMaturity_UpdatedThresholds(t *testing.T) {
    tests := []struct {
        maturity string
        want     int
    }{
        {"level_3_regime_aware", 25},
        {"level_2_window_validated", 15},
        {"level_2_validated", 15},
        {"level_1_exploratory", 8},
        {"unknown", 8},
    }
    
    for _, tt := range tests {
        got := requiredObservationCountForMaturity(tt.maturity)
        if got != tt.want {
            t.Errorf("maturity=%q: got %d, want %d", tt.maturity, got, tt.want)
        }
    }
}
```

- [ ] **Step 2: 執行測試確認失敗**

```bash
go test ./internal/experiment/ -run TestRequiredObservationCountForMaturity_UpdatedThresholds -v
```

Expected: FAIL — 舊門檻值不匹配

- [ ] **Step 3: 修改門檻值**

```go
func requiredObservationCountForMaturity(maturity string) int {
    switch maturity {
    case "level_3_regime_aware":
        return 25  // increased from 12
    case "level_2_window_validated", "level_2_validated":
        return 15  // increased from 8
    case "level_1_exploratory":
        return 8   // increased from 3
    default:
        return 8   // increased from 3
    }
}
```

- [ ] **Step 4: 執行測試確認通過**

```bash
go test ./internal/experiment/ -run TestRequiredObservationCountForMaturity_UpdatedThresholds -v
```

- [ ] **Step 5: Commit**

```bash
git add internal/experiment/judge.go internal/experiment/judge_test.go
git commit -m "feat(experiment): raise observation thresholds for statistical rigor

- level_3: 12 → 25
- level_2: 8 → 15
- level_1: 3 → 8
- Prevents overfitting on small samples"
```

---

### Task 7: 更新現有測試與向後相容

- [ ] **檢查並更新受影響的測試**

```bash
grep -rn "level_1_exploratory\|requiredObservationCount" internal/experiment/*_test.go
```

更新任何 hardcode 舊門檻值的測試。

- [ ] **執行完整 experiment 套件**

```bash
go test ./internal/experiment/... -v
```

Expected: ALL PASS

- [ ] **Commit**

```bash
git add internal/experiment/*_test.go
git commit -m "test(experiment): update tests for new observation thresholds"
```

---

### Task 8: Wave 2 驗收

- [ ] **驗證既有實驗不受影響**

```bash
go run ./cmd/judge-experiment -result data/state/experiments/exec-growth-momentum-01-1776674825.json
```

Expected: 正常執行 (已儲存的實驗結果不受影響)

---

## Wave 3: 可觀測夾制 (問題 B)

### Task 9: 擴展 DarwinianAgentWeight 結構

**Files:**
- Modify: `internal/portfolio/darwinian_weights.go:31-46`
- Test: `internal/portfolio/darwinian_weights_test.go`

- [ ] **Step 1: 寫失敗測試 — 截斷事件記錄**

```go
func TestConstrainWeight_LogsClippingEvent(t *testing.T) {
    mgr := NewDarwinianWeightManager("/tmp/test.json")
    mgr.InitializeFromRegistry(domain.AgentRegistry{
        Agents: []domain.AgentConfig{
            {ID: "test-agent", Enabled: true, Layer: domain.LayerSector},
        },
    })
    
    // Attempt to set weight below minimum (0.3)
    w := mgr.GetAgentWeightData("test-agent")
    if w == nil {
        t.Fatal("agent not found")
    }
    
    // Simulate a daily adjustment that would clip
    // We'll test via PerformDailyAdjustment or directly
    // For direct test, we need to expose constrainWeight behavior
}
```

- [ ] **Step 2: 修改 DarwinianAgentWeight 結構**

```go
type DarwinianAgentWeight struct {
    // ... existing fields ...
    
    // NEW: clipping transparency fields
    ClippingHistory    []ClippingEvent `json:"clipping_history,omitempty"`
    ConsecutiveClips   int             `json:"consecutive_clips"`
    IsCurrentlyClipped bool            `json:"is_currently_clipped"`
    ClipDirection      string          `json:"clip_direction,omitempty"`
}

type ClippingEvent struct {
    Timestamp   time.Time `json:"timestamp"`
    OriginalVal float64   `json:"original_value"`
    ClippedVal  float64   `json:"clipped_value"`
    Direction   string    `json:"direction"` // "min" or "max"
    Reason      string    `json:"reason"`
}
```

- [ ] **Step 3: 修改 constrainWeight 為可觀測版本**

```go
// constrainWeight ensures weight stays within [0.3, 2.5] bounds
// and records clipping events for audit trail.
func (m *DarwinianWeightManager) constrainWeight(weight float64, agentID string, reason string) float64 {
    w, exists := m.weights[agentID]
    if !exists {
        return weight // Can't record without agent
    }
    
    var clipped float64
    var direction string
    clipped = weight
    
    if weight < DarwinianWeightMin {
        clipped = DarwinianWeightMin
        direction = "min"
    } else if weight > DarwinianWeightMax {
        clipped = DarwinianWeightMax
        direction = "max"
    } else {
        // Not clipped — reset consecutive count
        w.ConsecutiveClips = 0
        w.IsCurrentlyClipped = false
        w.ClipDirection = ""
        return weight
    }
    
    // Record clipping event
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
    
    // Update consecutive clips
    if w.ClipDirection == direction {
        w.ConsecutiveClips++
    } else {
        w.ConsecutiveClips = 1
    }
    w.ClipDirection = direction
    w.IsCurrentlyClipped = true
    
    // Warning on consecutive clipping
    if w.ConsecutiveClips >= 3 {
        // Use standard log or a configurable logger
        // For now, we'll add a method to retrieve warnings
    }
    
    return clipped
}
```

- [ ] **Step 4: 更新所有調用點傳入 agentID 和 reason**

修改 `PerformDailyAdjustment` 中的調用：

```go
// Before:
// w.Weight = m.constrainWeight(oldWeight * multiplier)

// After:
w.Weight = m.constrainWeight(oldWeight*multiplier, w.AgentID, "daily_adjustment")
```

- [ ] **Step 5: Commit**

```bash
git add internal/portfolio/darwinian_weights.go
git commit -m "feat(portfolio): add clipping transparency to Darwinian weights

- ClippingEvent records original/clipped values, direction, reason
- ConsecutiveClips tracks streaks
- IsCurrentlyClipped flag for quick checks
- History capped at 30 events"
```

---

### Task 10: 新增連續截斷警告機制

**Files:**
- Modify: `internal/portfolio/darwinian_weights.go`
- Modify: `internal/portfolio/darwinian_weights.go` (GenerateReport)
- Test: `internal/portfolio/darwinian_weights_test.go`

- [ ] **Step 1: 新增警告回傳方法**

```go
// ClippingWarning 結構化警告訊息
type ClippingWarning struct {
    AgentID          string    `json:"agent_id"`
    Direction        string    `json:"direction"`
    ConsecutiveClips int       `json:"consecutive_clips"`
    CurrentWeight    float64   `json:"current_weight"`
    Timestamp        time.Time `json:"timestamp"`
    Message          string    `json:"message"`
}

// GetClippingWarnings 回傳所有連續截斷 ≥3 的警告
func (m *DarwinianWeightManager) GetClippingWarnings() []ClippingWarning {
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    var warnings []ClippingWarning
    for _, w := range m.weights {
        if w.ConsecutiveClips >= 3 {
            warnings = append(warnings, ClippingWarning{
                AgentID:          w.AgentID,
                Direction:        w.ClipDirection,
                ConsecutiveClips: w.ConsecutiveClips,
                CurrentWeight:    w.Weight,
                Timestamp:        time.Now(),
                Message: fmt.Sprintf(
                    "Agent %s clipped %s for %d consecutive adjustments. "+
                        "Current weight: %.2f. Consider prompt review or disabling.",
                    w.AgentID, w.ClipDirection, w.ConsecutiveClips, w.Weight,
                ),
            })
        }
    }
    return warnings
}
```

- [ ] **Step 2: 在報告中標記截斷狀態**

修改 `GenerateReport()`：

```go
type DarwinianWeightReport struct {
    // ... existing fields ...
    ClippedAgents       []ClippedAgentInfo `json:"clipped_agents"`
    TotalClippingEvents int                `json:"total_clipping_events"`
    Warnings            []ClippingWarning  `json:"warnings,omitempty"`
}

type ClippedAgentInfo struct {
    AgentID          string  `json:"agent_id"`
    Weight           float64 `json:"weight"`
    Direction        string  `json:"direction"`
    ConsecutiveClips int     `json:"consecutive_clips"`
}
```

在 `GenerateReport()` 中加入：

```go
// Collect clipped agents
for _, agent := range allAgents {
    if agent.IsCurrentlyClipped {
        report.ClippedAgents = append(report.ClippedAgents, ClippedAgentInfo{
            AgentID:          agent.AgentID,
            Weight:           agent.Weight,
            Direction:        agent.ClipDirection,
            ConsecutiveClips: agent.ConsecutiveClips,
        })
        report.TotalClippingEvents += len(agent.ClippingHistory)
    }
}

// Add warnings
report.Warnings = m.GetClippingWarnings()
```

- [ ] **Step 3: Commit**

```bash
git add internal/portfolio/darwinian_weights.go
git commit -m "feat(portfolio): add clipping warnings to Darwinian weight report

- GetClippingWarnings() returns agents with ≥3 consecutive clips
- Report includes clipped agent list and total event count
- Structured warnings for programmatic handling"
```

---

### Task 11: 測試 Wave 3

```bash
go test ./internal/portfolio/... -v
```

Expected: ALL PASS

---

### Task 12: Wave 3 驗收

- [ ] **驗證報告包含截斷資訊**

撰寫一個快速驗證腳本：

```bash
cat > /tmp/test_clipping.go <<'EOF'
package main

import (
    "encoding/json"
    "fmt"
    "github.com/kaecer68/atlas-go/internal/portfolio"
)

func main() {
    mgr := portfolio.NewDarwinianWeightManager("/tmp/test_darwinian.json")
    // ... setup and trigger clipping ...
    report := mgr.GenerateReport()
    out, _ := json.MarshalIndent(report, "", "  ")
    fmt.Println(string(out))
}
EOF
```

---

## Wave 4: Out-of-Sample + Sharpe 穩定性 (問題 C - P3/P4)

### Task 13: 新增 Sharpe 穩定性檢查

**Files:**
- Create: `internal/experiment/sharpe_stability.go`
- Test: `internal/experiment/sharpe_stability_test.go`

- [ ] **Step 1: 寫失敗測試**

```go
package experiment

import (
    "math"
    "testing"
)

func TestSharpeStabilityCheck_Pass(t *testing.T) {
    check := SharpeStabilityCheck{RequiredMaxSE: 0.5}
    
    // 20 observations with moderate Sharpe
    observations := make([]float64, 20)
    for i := range observations {
        observations[i] = 0.02 + float64(i)*0.001
    }
    
    passed, se := check.Check(observations)
    if !passed {
        t.Errorf("expected to pass with SE=%.4f", se)
    }
}

func TestSharpeStabilityCheck_FailSmallSample(t *testing.T) {
    check := SharpeStabilityCheck{RequiredMaxSE: 0.5}
    
    observations := []float64{0.01, 0.02, 0.015} // only 3
    
    passed, _ := check.Check(observations)
    if passed {
        t.Errorf("expected to fail with <10 observations")
    }
}
```

- [ ] **Step 2: 實作 SharpeStabilityCheck**

```go
package experiment

import "math"

// SharpeStabilityCheck validates that Sharpe ratio estimates are stable.
type SharpeStabilityCheck struct {
    RequiredMaxSE float64 // Maximum allowed standard error
}

// Check returns true if the observations produce a stable Sharpe estimate.
func (c *SharpeStabilityCheck) Check(observations []float64) (bool, float64) {
    if len(observations) < 10 {
        return false, 0
    }
    
    mean := 0.0
    for _, o := range observations {
        mean += o
    }
    mean /= float64(len(observations))
    
    variance := 0.0
    for _, o := range observations {
        diff := o - mean
        variance += diff * diff
    }
    variance /= float64(len(observations))
    stdDev := math.Sqrt(variance)
    
    if stdDev == 0 {
        return false, 0
    }
    
    sharpe := mean / stdDev
    // Approximate standard error of Sharpe ratio
    se := math.Sqrt((1 + 0.5*sharpe*sharpe) / float64(len(observations)))
    
    return se <= c.RequiredMaxSE, se
}
```

- [ ] **Step 3: Commit**

```bash
git add internal/experiment/sharpe_stability.go internal/experiment/sharpe_stability_test.go
git commit -m "feat(experiment): add Sharpe stability check for experiment validation

- SE threshold defaults to 0.5
- Requires minimum 10 observations
- Uses Lo (2002) approximate Sharpe SE formula"
```

---

### Task 14: 新增 Out-of-Sample 驗證器

**Files:**
- Create: `internal/experiment/oos_validator.go`
- Test: `internal/experiment/oos_validator_test.go`

- [ ] **Step 1: 寫失敗測試**

```go
package experiment

import "testing"

func TestOutOfSampleValidator_Validate(t *testing.T) {
    // This requires mocking the ledger store and replay data
    // We'll write an integration-style test
    
    validator := &OutOfSampleValidator{
        // ... setup mock store ...
    }
    
    // Test with non-overlapping window
    // Expected: validation result based on mock data
}
```

- [ ] **Step 2: 實作 OutOfSampleValidator 骨架**

```go
package experiment

import (
    "fmt"
    "github.com/kaecer68/atlas-go/internal/domain"
    "github.com/kaecer68/atlas-go/internal/ledger"
)

// OutOfSampleValidator validates candidates on independent windows.
type OutOfSampleValidator struct {
    store          *ledger.Store
    replayDataPath string
}

// Validate checks if the candidate outperforms baseline on a secondary window.
func (v *OutOfSampleValidator) Validate(
    brief domain.MutationBrief,
    candidatePromptPath string,
    primaryWindowID string,
) (bool, string, error) {
    // Placeholder: actual implementation requires window selection logic
    // and prompt evaluation on alternative data.
    
    // 1. Find non-overlapping window
    // 2. Evaluate baseline on secondary window
    // 3. Evaluate candidate on secondary window
    // 4. Compare scores
    
    return true, "out-of-sample validation: placeholder", nil
}
```

- [ ] **Step 3: Commit (骨架版本)**

```bash
git add internal/experiment/oos_validator.go
git commit -m "feat(experiment): add OutOfSampleValidator skeleton

- Framework for independent window validation
- Full implementation depends on window management integration"
```

---

### Task 15: Wave 4 整合與驗收

- [ ] **整合 Sharpe 檢查至 Judge**

修改 `internal/experiment/judge.go` 的 `Evaluate` 方法：

```go
// After computing summary scores:
stabilityCheck := SharpeStabilityCheck{RequiredMaxSE: 0.5}
if summary.CandidateObservations >= 10 {
    // Extract candidate observations for stability check
    // This requires piping raw observations through summary
    // For now, add a placeholder integration point
    _ = stabilityCheck
}
```

- [ ] **執行完整測試**

```bash
go test ./internal/experiment/... -v
go test ./... -v 2>&1 | tail -20
```

- [ ] **執行覆蓋率檢查**

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

Expected: coverage ≥ 40%

---

## 最終驗收清單

### 功能驗收

- [ ] 模擬 40% 回撤情境，系統觸發紅色熔斷並強制清倉
- [ ] Darwinian 權重連續 3 次觸底後，報告包含警告
- [ ] level_1 實驗提交 5 筆觀測值時，Judge 明確拒絕並提示需要 8 筆

### 測試驗收

- [ ] `go test ./internal/risk/...` 全部通過
- [ ] `go test ./internal/sim/...` 全部通過
- [ ] `go test ./internal/portfolio/...` 全部通過
- [ ] `go test ./internal/experiment/...` 全部通過
- [ ] `go test ./...` 總覆蓋率 ≥ 40%

### 稽核驗收

- [ ] `data/state/drawdown_events.jsonl` 記錄所有熔斷觸發
- [ ] `data/state/darwinian_weights.json` 包含 `clipping_history`
- [ ] Experiment JSON 包含統計穩定性檢查結果

### 建置驗收

- [ ] `go build ./...` 成功
- [ ] `gofmt -l .` 無輸出
- [ ] `go vet ./...` 無錯誤

---

*實施計劃版本: 1.0*  
*設計文件: docs/superpowers/specs/2026-04-23-experiment-safety-improvements-design.md*  
*最後更新: 2026-04-23*
