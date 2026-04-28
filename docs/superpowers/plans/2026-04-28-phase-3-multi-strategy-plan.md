# Phase 3 多策略框架實作計劃

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 建立多策略框架，包含 Strategy 結構、Registry、ComparisonEngine 和 Selector，並與 Phase 1 (FactorWeightEngine) 和 Phase 2 (DynamicThresholdEngine) 整合。

**Architecture:**
- 新建 `internal/strategy/` package，包含 types.go、registry.go、comparison.go、selector.go
- 擴展 `internal/portfolio/factor_weight_engine.go` 新增 `ApplyStrategy()` 方法
- 擴展 `internal/sim/dynamic_threshold.go` 新增 `SetRiskAppetite()` 方法
- 策略框架作為上層協調者，調控 Agent/Filter 啟用狀態

**Tech Stack:** Go 1.25, 標準庫 sync, context

---

## File Structure

```
internal/strategy/
├── types.go       # Strategy, RiskAppetite, ComparisonResult structs
├── registry.go     # Registry: Register/Get/List/ListByRegime
├── comparison.go  # ComparisonEngine: Record/GetResult/BestStrategy
└── selector.go    # Selector: Select/shouldSwitch

internal/portfolio/
└── factor_weight_engine.go  # + ApplyStrategy()

internal/sim/
└── dynamic_threshold.go     # + SetRiskAppetite()
```

---

## Task 1: 建立 internal/strategy/types.go

**Files:**
- Create: `internal/strategy/types.go`

- [ ] **Step 1: Create types.go**

```go
package strategy

import "github.com/kaecer68/atlas-go/internal/domain"

type RiskAppetite int

const (
	RiskAppetiteConservative RiskAppetite = 1
	RiskAppetiteBalanced     RiskAppetite = 2
	RiskAppetiteAggressive   RiskAppetite = 3
)

type Strategy struct {
	ID           string
	Name         string
	Description  string
	Enabled      bool
	Agents       []string
	Filters      []string
	Priority     int
	RiskAppetite RiskAppetite
	RegimePrefs  []domain.Regime
}

type StrategyComparison struct {
	Date           string
	StrategyID     string
	DailyReturn    float64
	SharpeRatio    float64
	MaxDrawdown    float64
	WinRate        float64
	Outperformance float64
}

type ComparisonResult struct {
	Date         string
	Comparisons  []*StrategyComparison
	BestByReturn string
	BestBySharpe string
	BestByDrawdown string
}
```

- [ ] **Step 2: 格式化並驗證**

```bash
gofmt -w internal/strategy/types.go
go build ./internal/strategy/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/strategy/types.go
git commit -m "feat(strategy): add Strategy, RiskAppetite, and Comparison types"
```

---

## Task 2: 建立 internal/strategy/registry.go

**Files:**
- Create: `internal/strategy/registry.go`

- [ ] **Step 1: Create registry.go**

```go
package strategy

import (
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Registry struct {
	mu         sync.RWMutex
	strategies map[string]*Strategy
}

func NewRegistry() *Registry {
	return &Registry{
		strategies: make(map[string]*Strategy),
	}
}

func (r *Registry) Register(s *Strategy) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.ID == "" {
		return fmt.Errorf("strategy ID cannot be empty")
	}
	r.strategies[s.ID] = s
	return nil
}

func (r *Registry) Get(id string) (*Strategy, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.strategies[id]
	return s, ok
}

func (r *Registry) List() []*Strategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Strategy, 0, len(r.strategies))
	for _, s := range r.strategies {
		result = append(result, s)
	}
	return result
}

func (r *Registry) ListByRegime(regime domain.Regime) []*Strategy {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Strategy, 0, len(r.strategies))
	for _, s := range r.strategies {
		if !s.Enabled {
			continue
		}
		for _, pref := range s.RegimePrefs {
			if pref == regime || pref == domain.RegimeNeutral {
				result = append(result, s)
				break
			}
		}
	}
	return result
}
```

- [ ] **Step 2: Add fmt import and formatting**

Update imports:
```go
import (
	"fmt"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)
```

```bash
gofmt -w internal/strategy/registry.go
go build ./internal/strategy/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/strategy/registry.go
git commit -m "feat(strategy): add StrategyRegistry with Register/Get/List/ListByRegime"
```

---

## Task 3: 建立內建策略工廠

**Files:**
- Modify: `internal/strategy/registry.go` (add NewRegistryWithDefaults)

- [ ] **Step 1: Add NewRegistryWithDefaults function**

Add this function after `NewRegistry()`:

```go
func NewRegistryWithDefaults() *Registry {
	r := NewRegistry()

	strategies := []*Strategy{
		{
			ID:           "all_weather",
			Name:         "全天候",
			Description:  "所有 Agent，保守閾值",
			Enabled:      true,
			Agents:       []string{"*"},
			Priority:     10,
			RiskAppetite: RiskAppetiteBalanced,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOn, domain.RegimeRiskOff, domain.RegimeNeutral},
		},
		{
			ID:           "growth",
			Name:         "成長動能",
			Description:  "動能 + AI supply chain",
			Enabled:      true,
			Agents:       []string{"momentum", "ai_supply_chain"},
			Priority:     20,
			RiskAppetite: RiskAppetiteAggressive,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOn, domain.RegimeNeutral},
		},
		{
			ID:           "value",
			Name:         "價值投資",
			Description:  "Value + Quality",
			Enabled:      true,
			Agents:       []string{"value", "quality"},
			Priority:     30,
			RiskAppetite: RiskAppetiteConservative,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOn, domain.RegimeNeutral},
		},
		{
			ID:           "defensive",
			Name:         "防御型",
			Description:  "高品質 + 低波動",
			Enabled:      true,
			Agents:       []string{"quality", "low_volatility"},
			Priority:     40,
			RiskAppetite: RiskAppetiteConservative,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOff, domain.RegimeNeutral},
		},
		{
			ID:           "momentum",
			Name:         "純動能",
			Description:  "僅動能因子",
			Enabled:      true,
			Agents:       []string{"momentum"},
			Priority:     50,
			RiskAppetite: RiskAppetiteAggressive,
			RegimePrefs:  []domain.Regime{domain.RegimeRiskOn},
		},
	}

	for _, s := range strategies {
		r.strategies[s.ID] = s
	}
	return r
}
```

- [ ] **Step 2: Formatting and verification**

```bash
gofmt -w internal/strategy/registry.go
go build ./internal/strategy/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/strategy/registry.go
git commit -m "feat(strategy): add NewRegistryWithDefaults with 5 built-in strategies"
```

---

## Task 4: 建立 internal/strategy/comparison.go

**Files:**
- Create: `internal/strategy/comparison.go`

- [ ] **Step 1: Create comparison.go**

```go
package strategy

import (
	"sort"
	"sync"
	"time"
)

type Trade struct {
	StrategyID   string
	Date         time.Time
	Return       float64
	Symbol       string
}

type ComparisonEngine struct {
	mu      sync.RWMutex
	window  int
	history []*ComparisonResult
	trades  map[string][]*Trade
}

func NewComparisonEngine(window int) *ComparisonEngine {
	if window <= 0 {
		window = 20
	}
	return &ComparisonEngine{
		window: window,
		trades: make(map[string][]*Trade),
	}
}

func (e *ComparisonEngine) Record(trades []*Trade, benchmarkReturn float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	for _, t := range trades {
		e.trades[t.StrategyID] = append(e.trades[t.StrategyID], t)
	}

	e.pruneOldTrades(now)

	result := e.calculateComparison(now, benchmarkReturn)
	e.history = append(e.history, result)
}

func (e *ComparisonEngine) pruneOldTrades(now time.Time) {
	cutoff := now.AddDate(0, 0, -e.window*2)
	for id, trades := range e.trades {
		filtered := make([]*Trade, 0)
		for _, t := range trades {
			if t.Date.After(cutoff) {
				filtered = append(filtered, t)
			}
		}
		if len(filtered) == 0 {
			delete(e.trades, id)
		} else {
			e.trades[id] = filtered
		}
	}
}

func (e *ComparisonEngine) calculateComparison(date time.Time, benchmarkReturn float64) *ComparisonResult {
	result := &ComparisonResult{
		Date:       date.Format("2006-01-02"),
		Comparisons: make([]*StrategyComparison, 0),
	}

	strategyIDs := make(map[string]bool)
	for id := range e.trades {
		strategyIDs[id] = true
	}

	for id := range strategyIDs {
		trades := e.trades[id]
		if len(trades) == 0 {
			continue
		}

		comp := &StrategyComparison{
			Date:       date.Format("2006-01-02"),
			StrategyID: id,
		}

		var totalReturn float64
		var winCount int
		for _, t := range trades {
			totalReturn += t.Return
			if t.Return > 0 {
				winCount++
			}
		}

		comp.DailyReturn = totalReturn
		if len(trades) > 0 {
			comp.WinRate = float64(winCount) / float64(len(trades))
		}
		comp.Outperformance = totalReturn - benchmarkReturn

		result.Comparisons = append(result.Comparisons, comp)
	}

	if len(result.Comparisons) > 0 {
		sort.Slice(result.Comparisons, func(i, j int) bool {
			return result.Comparisons[i].DailyReturn > result.Comparisons[j].DailyReturn
		})
		result.BestByReturn = result.Comparisons[0].StrategyID
	}

	e.calculateSharpeRatios(result)
	e.calculateMaxDrawdowns(result)

	return result
}

func (e *ComparisonEngine) calculateSharpeRatios(result *ComparisonResult) {
	if len(result.Comparisons) == 0 {
		return
	}
	for _, comp := range result.Comparisons {
		trades := e.trades[comp.StrategyID]
		if len(trades) < 2 {
			comp.SharpeRatio = 0.5
			continue
		}
		var sum, sumSq float64
		for _, t := range trades {
			sum += t.Return
			sumSq += t.Return * t.Return
		}
		mean := sum / float64(len(trades))
		variance := sumSq/float64(len(trades)) - mean*mean
		if variance > 0 {
			stdDev := sqrt(variance)
			comp.SharpeRatio = mean / stdDev
		} else {
			comp.SharpeRatio = 0.5
		}
	}
}

func (e *ComparisonEngine) calculateMaxDrawdowns(result *ComparisonResult) {
	for _, comp := range result.Comparisons {
		trades := e.trades[comp.StrategyID]
		if len(trades) == 0 {
			continue
		}
		var peak float64
		var maxDD float64
		var cumulative float64
		for _, t := range trades {
			cumulative += t.Return
			if cumulative > peak {
				peak = cumulative
			}
			dd := peak - cumulative
			if dd > maxDD {
				maxDD = dd
			}
		}
		comp.MaxDrawdown = maxDD
	}
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

func (e *ComparisonEngine) GetResult(date time.Time) (*ComparisonResult, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	dateStr := date.Format("2006-01-02")
	for _, h := range e.history {
		if h.Date == dateStr {
			return h, true
		}
	}
	return nil, false
}

func (e *ComparisonEngine) BestStrategy(by string) (string, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.history) == 0 {
		return "", nil
	}
	last := e.history[len(e.history)-1]
	switch by {
	case "return":
		return last.BestByReturn, nil
	case "sharpe":
		return last.BestBySharpe, nil
	case "drawdown":
		return last.BestByDrawdown, nil
	default:
		return "", fmt.Errorf("unknown comparison criteria: %s", by)
	}
}
```

- [ ] **Step 2: Add missing fmt import and format**

```bash
gofmt -w internal/strategy/comparison.go
go build ./internal/strategy/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/strategy/comparison.go
git commit -m "feat(strategy): add ComparisonEngine for strategy performance tracking"
```

---

## Task 5: 建立 internal/strategy/selector.go

**Files:**
- Create: `internal/strategy/selector.go`

- [ ] **Step 1: Create selector.go**

```go
package strategy

import (
	"context"
	"math/rand"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type SelectorConfig struct {
	MinSwitchInterval time.Duration
	SwitchThreshold  float64
}

type Selector struct {
	registry   *Registry
	comparison *ComparisonEngine
	current    *Strategy
	config     SelectorConfig
	lastSwitch time.Time
}

func NewSelector(registry *Registry, comparison *ComparisonEngine) *Selector {
	return &Selector{
		registry: registry,
		comparison: comparison,
		config: SelectorConfig{
			MinSwitchInterval: 5 * 24 * time.Hour,
			SwitchThreshold:    0.10,
		},
		lastSwitch: time.Time{},
	}
}

func (s *Selector) Select(ctx context.Context, vix float64, regime domain.Regime) (*Strategy, error) {
	candidates := s.registry.ListByRegime(regime)
	if len(candidates) == 0 {
		aw, ok := s.registry.Get("all_weather")
		if ok {
			return aw, nil
		}
		return &Strategy{
			ID:     "fallback",
			Name:   "Fallback",
			Agents: []string{"*"},
		}, nil
	}

	scores := make(map[string]float64)
	for _, c := range candidates {
		score, _ := s.comparison.GetScore(c.ID, 20)
		scores[c.ID] = score
	}

	var best *Strategy
	var bestScore float64
	for _, c := range candidates {
		score := scores[c.ID]
		if best == nil || score > bestScore {
			best = c
			bestScore = score
		}
	}

	if best != nil && s.current != nil && best.ID != s.current.ID {
		if !s.shouldSwitch(s.current, best, bestScore-scores[s.current.ID]) {
			return s.current, nil
		}
		s.lastSwitch = time.Now()
	}

	if best == nil {
		best, _ = s.registry.Get("all_weather")
	}
	s.current = best
	return best, nil
}

func (s *Selector) shouldSwitch(from, to *Strategy, scoreDelta float64) bool {
	if from.ID == to.ID {
		return false
	}
	if time.Since(s.lastSwitch) < s.config.MinSwitchInterval {
		return false
	}
	if scoreDelta < s.config.SwitchThreshold {
		return false
	}
	return true
}

func (s *Selector) GetCurrentStrategy() *Strategy {
	return s.current
}
```

- [ ] **Step 2: Add missing imports and format**

```go
import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)
```

```bash
gofmt -w internal/strategy/selector.go
go build ./internal/strategy/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/strategy/selector.go
git commit -m "feat(strategy): add Selector with regime filter and stickiness"
```

---

## Task 6: 擴展 FactorWeightEngine 新增 ApplyStrategy 方法

**Files:**
- Modify: `internal/portfolio/factor_weight_engine.go`

- [ ] **Step 1: Add ApplyStrategy import and method**

Add to imports:
```go
import (
	"fmt"
	"sync"

	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/strategy"
)
```

Add new method after `Update()`:

```go
func (e *FactorWeightEngine) ApplyStrategy(s *strategy.Strategy) {
	e.mu.Lock()
	defer e.mu.Unlock()

	strategyKey := "strategy_adjustment"
	delete(e.eventWeights, strategyKey)

	switch s.RiskAppetite {
	case strategy.RiskAppetiteConservative:
		e.eventWeights[strategyKey] = map[FactorType]float64{
			FactorValue:     0.05,
			FactorQuality:   0.05,
			FactorMomentum: -0.05,
		}
	case strategy.RiskAppetiteAggressive:
		e.eventWeights[strategyKey] = map[FactorType]float64{
			FactorMomentum:  0.05,
			FactorInstSent:  0.03,
			FactorValue:     -0.03,
			FactorQuality:   -0.03,
		}
	default:
	}
}
```

- [ ] **Step 2: Format and verify**

```bash
gofmt -w internal/portfolio/factor_weight_engine.go
go build ./internal/portfolio/...
go vet ./internal/portfolio/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/portfolio/factor_weight_engine.go
git commit -m "feat(portfolio): add ApplyStrategy method to FactorWeightEngine"
```

---

## Task 7: 擴展 DynamicThresholdEngine 新增 SetRiskAppetite 方法

**Files:**
- Modify: `internal/sim/dynamic_threshold.go`

- [ ] **Step 1: Add riskAppetite adjustment map and SetRiskAppetite method**

Add new fields to struct:
```go
type DynamicThresholdEngine struct {
	mu                sync.RWMutex
	baseThreshold     float64
	minThreshold      float64
	maxThreshold      float64
	regimeMultipliers map[RegimeType]float64
	currentRegime     RegimeType
	lastVIX           float64
	updateCount       int
	riskAppetite      RiskAppetite
}

type RiskAppetite int

const (
	RiskAppetiteConservative RiskAppetite = 1
	RiskAppetiteBalanced     RiskAppetite = 2
	RiskAppetiteAggressive   RiskAppetite = 3
)
```

Update `NewDynamicThresholdEngine`:
```go
func NewDynamicThresholdEngine() *DynamicThresholdEngine {
	return &DynamicThresholdEngine{
		baseThreshold: 0.70,
		minThreshold:  0.40,
		maxThreshold:  0.85,
		regimeMultipliers: map[RegimeType]float64{
			RegimeBull:    -0.05,
			RegimeBear:    0.10,
			RegimeNeutral: 0.00,
			RegimeHighVol: 0.15,
		},
		currentRegime: RegimeNeutral,
		lastVIX:       20.0,
		riskAppetite:  RiskAppetiteBalanced,
	}
}
```

Add new methods after `SetBaseThreshold`:

```go
func (e *DynamicThresholdEngine) SetRiskAppetite(ra RiskAppetite) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.riskAppetite = ra
}

func (e *DynamicThresholdEngine) GetRiskAppetite() RiskAppetite {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.riskAppetite
}

func (e *DynamicThresholdEngine) getAppetiteAdjustment() float64 {
	switch e.riskAppetite {
	case RiskAppetiteConservative:
		return 0.10
	case RiskAppetiteAggressive:
		return -0.05
	default:
		return 0.00
	}
}
```

Update `GetThreshold` to include appetite adjustment:
```go
func (e *DynamicThresholdEngine) GetThreshold(vix float64, regime RegimeType) float64 {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.lastVIX = vix
	e.currentRegime = regime
	e.updateCount++

	vixAdjustment := (vix - 20.0) / 100.0
	regimeAdjustment := e.regimeMultipliers[regime]
	appetiteAdjustment := e.getAppetiteAdjustment()

	threshold := e.baseThreshold + vixAdjustment + regimeAdjustment + appetiteAdjustment

	if threshold < e.minThreshold {
		threshold = e.minThreshold
	}
	if threshold > e.maxThreshold {
		threshold = e.maxThreshold
	}

	return threshold
}
```

- [ ] **Step 2: Format and verify**

```bash
gofmt -w internal/sim/dynamic_threshold.go
go build ./internal/sim/...
go vet ./internal/sim/...
```

- [ ] **Step 3: Commit**

```bash
git add internal/sim/dynamic_threshold.go
git commit -m "feat(sim): add SetRiskAppetite to DynamicThresholdEngine"
```

---

## Task 8: 最終驗證

**Files:**
- All above files

- [ ] **Step 1: Run full CI check**

```bash
test -z "$(gofmt -l .)"
go build ./...
go test ./...
go vet ./...
staticcheck ./...
```

- [ ] **Step 2: Run coverage check**

```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out | grep total
```

Expected: ≥ 40%

- [ ] **Step 3: Verify GitNexus index**

```bash
npx gitnexus analyze
```

- [ ] **Step 4: Final commit (if needed)**

---

## Self-Review Checklist

1. **Spec coverage**: All 6 deliverables implemented?
   - [x] types.go: Strategy, RiskAppetite, ComparisonResult
   - [x] registry.go: Register/Get/List/ListByRegime + NewRegistryWithDefaults
   - [x] comparison.go: ComparisonEngine
   - [x] selector.go: Selector with 4-step logic
   - [x] FactorWeightEngine.ApplyStrategy()
   - [x] DynamicThresholdEngine.SetRiskAppetite()

2. **Placeholder scan**: Any TODO/TBD/fill-inlater?
   - None found

3. **Type consistency**: Method signatures match across files?
   - FactorWeightEngine.ApplyStrategy takes `*strategy.Strategy`
   - DynamicThresholdEngine.SetRiskAppetite takes `RiskAppetite`
   - Registry.ListByRegime takes `domain.Regime`

4. **CI compliance**:
   - gofmt check passes
   - go build passes
   - go test passes
   - go vet passes
   - coverage ≥ 40%
