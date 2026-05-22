package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
)

// AlertRule 告警规则
type AlertRule struct {
	Name        string
	Description string
	Condition   func(state *livestore.State) (bool, string)
	Level       AlertLevel
	Cooldown    time.Duration
}

// RuleEngine 规则引擎
type RuleEngine struct {
	rules         []AlertRule
	monitor       *Monitor
	lastFired     map[string]time.Time
	mu            sync.RWMutex
	checkInterval time.Duration
}

// NewRuleEngine 创建规则引擎
func NewRuleEngine(monitor *Monitor) *RuleEngine {
	return &RuleEngine{
		rules:         make([]AlertRule, 0),
		monitor:       monitor,
		lastFired:     make(map[string]time.Time),
		checkInterval: 30 * time.Second,
	}
}

// RegisterRule 注册告警规则
func (e *RuleEngine) RegisterRule(rule AlertRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
}

// SetCheckInterval 设置检查间隔
func (e *RuleEngine) SetCheckInterval(interval time.Duration) {
	e.checkInterval = interval
}

// Start 启动规则引擎
func (e *RuleEngine) Start(ctx context.Context, stateStore *livestore.StateStore) {
	ticker := time.NewTicker(e.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if stateStore != nil {
				state := stateStore.GetState()
				e.EvaluateRules(state)
			}
		}
	}
}

func (e *RuleEngine) EvaluateRules(state *livestore.State) {
	e.mu.RLock()
	rules := make([]AlertRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	for _, rule := range rules {
		if lastFired, ok := e.lastFired[rule.Name]; ok {
			if time.Since(lastFired) < rule.Cooldown {
				continue
			}
		}

		triggered, message := rule.Condition(state)
		if triggered {
			e.monitor.Alert(rule.Level, "rule_engine", fmt.Sprintf("[%s] %s", rule.Name, message), map[string]any{
				"rule":        rule.Name,
				"description": rule.Description,
			})
			e.lastFired[rule.Name] = time.Now()
		}
	}
}

func DefaultRules() []AlertRule {
	params := config.GetParametersConfig().Alert
	return []AlertRule{
		{
			Name:        "portfolio_value_drop",
			Description: "Portfolio value dropped significantly",
			Condition: func(state *livestore.State) (bool, string) {
				if state == nil {
					return false, ""
				}
				if state.Portfolio.Cash < params.MinCashThreshold.Value {
					return true, fmt.Sprintf("Cash level low: %.2f (threshold: %.2f)", state.Portfolio.Cash, params.MinCashThreshold.Value)
				}
				return false, ""
			},
			Level:    AlertLevelWarning,
			Cooldown: time.Duration(params.RuleEngineCooldownSec.Value) * time.Second,
		},
		{
			Name:        "position_concentration",
			Description: "High position concentration detected",
			Condition: func(state *livestore.State) (bool, string) {
				if state == nil || len(state.Positions) == 0 {
					return false, ""
				}
				if len(state.Positions) > params.MaxPositionsCount.Value {
					return true, fmt.Sprintf("Too many positions: %d (threshold: %d)", len(state.Positions), params.MaxPositionsCount.Value)
				}
				return false, ""
			},
			Level:    AlertLevelWarning,
			Cooldown: time.Duration(params.RuleEngineCooldownSec.Value*2) * time.Second,
		},
		{
			Name:        "system_ready",
			Description: "System is ready for trading",
			Condition: func(state *livestore.State) (bool, string) {
				if state == nil {
					return false, ""
				}
				// 只在系统初始化时触发一次
				return true, "Trading system initialized and ready"
			},
			Level:    AlertLevelInfo,
			Cooldown: 24 * time.Hour,
		},
	}
}

func LiveTradingRules() []AlertRule {
	params := config.GetParametersConfig().Alert
	return []AlertRule{
		{
			Name:        "circuit_breaker_triggered",
			Description: "Circuit breaker entered paused or halted state",
			Condition: func(state *livestore.State) (bool, string) {
				if state == nil {
					return false, ""
				}
				dayPnLPct := 0.0
				if state.Portfolio.Cash > 0 {
					dayPnLPct = (state.Portfolio.DayPnL / state.Portfolio.Cash) * 100
				}
				if dayPnLPct < params.DailyLossCriticalPct.Value*100 {
					return true, fmt.Sprintf("Day PnL %.2f%% breached daily loss threshold %.2f%%", dayPnLPct, params.DailyLossCriticalPct.Value*100)
				}
				return false, ""
			},
			Level:    AlertLevelCritical,
			Cooldown: time.Duration(params.RuleEngineCooldownSec.Value/5) * time.Second,
		},
		{
			Name:        "daily_loss_warning",
			Description: "Portfolio day PnL approaching daily loss limit",
			Condition: func(state *livestore.State) (bool, string) {
				if state == nil {
					return false, ""
				}
				dayPnLPct := 0.0
				if state.Portfolio.Cash > 0 {
					dayPnLPct = (state.Portfolio.DayPnL / state.Portfolio.Cash) * 100
				}
				if dayPnLPct < params.DailyLossWarningPct.Value*100 && dayPnLPct >= params.DailyLossCriticalPct.Value*100 {
					return true, fmt.Sprintf("Day PnL warning: %.2f%% (threshold: %.2f%%)", dayPnLPct, params.DailyLossWarningPct.Value*100)
				}
				return false, ""
			},
			Level:    AlertLevelWarning,
			Cooldown: time.Duration(params.RuleEngineCooldownSec.Value) * time.Second,
		},
		{
			Name:        "high_position_concentration",
			Description: "Single position exceeds max weight of portfolio",
			Condition: func(state *livestore.State) (bool, string) {
				if state == nil || len(state.Positions) == 0 {
					return false, ""
				}
				totalValue := state.Portfolio.Cash + state.Portfolio.UnrealizedPnL
				if totalValue <= 0 {
					return false, ""
				}
				for _, p := range state.Positions {
					weight := p.MarketValue / totalValue
					if weight > params.MaxPositionWeightPct.Value {
						return true, fmt.Sprintf("Position %s weight %.1f%% exceeds %.1f%%", p.Symbol, weight*100, params.MaxPositionWeightPct.Value*100)
					}
				}
				return false, ""
			},
			Level:    AlertLevelError,
			Cooldown: time.Duration(params.RuleEngineCooldownSec.Value*2) * time.Second,
		},
		{
			Name:        "unrealized_loss_position",
			Description: "Position unrealized loss exceeds threshold",
			Condition: func(state *livestore.State) (bool, string) {
				if state == nil || len(state.Positions) == 0 {
					return false, ""
				}
				for _, p := range state.Positions {
					if p.AverageCost > 0 {
						lossPct := (p.MarketValue/p.AverageCost - 1) * 100
						if lossPct < params.MaxUnrealizedLossPct.Value*100 {
							return true, fmt.Sprintf("Position %s unrealized loss %.2f%% (threshold: %.2f%%)", p.Symbol, lossPct, params.MaxUnrealizedLossPct.Value*100)
						}
					}
				}
				return false, ""
			},
			Level:    AlertLevelWarning,
			Cooldown: time.Duration(params.RuleEngineCooldownSec.Value) * time.Second,
		},
	}
}
