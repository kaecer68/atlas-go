package monitoring

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/live"
)

// AlertRule 告警规则
type AlertRule struct {
	Name        string
	Description string
	Condition   func(state *live.State) (bool, string)
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
func (e *RuleEngine) Start(ctx context.Context, stateStore *live.StateStore) {
	ticker := time.NewTicker(e.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if stateStore != nil {
				state := stateStore.GetState()
				e.evaluateRules(state)
			}
		}
	}
}

// evaluateRules 评估所有规则
func (e *RuleEngine) evaluateRules(state *live.State) {
	e.mu.RLock()
	rules := make([]AlertRule, len(e.rules))
	copy(rules, e.rules)
	e.mu.RUnlock()

	for _, rule := range rules {
		// 检查冷却时间
		if lastFired, ok := e.lastFired[rule.Name]; ok {
			if time.Since(lastFired) < rule.Cooldown {
				continue
			}
		}

		// 评估条件
		triggered, message := rule.Condition(state)
		if triggered {
			e.monitor.Alert(rule.Level, "rule_engine", fmt.Sprintf("[%s] %s", rule.Name, message), map[string]interface{}{
				"rule":        rule.Name,
				"description": rule.Description,
			})
			e.lastFired[rule.Name] = time.Now()
		}
	}
}

// DefaultRules 返回默认告警规则
func DefaultRules() []AlertRule {
	return []AlertRule{
		{
			Name:        "portfolio_value_drop",
			Description: "Portfolio value dropped significantly",
			Condition: func(state *live.State) (bool, string) {
				if state == nil {
					return false, ""
				}
				// 简化的示例：检查现金是否异常低
				if state.Portfolio.Cash < 100000 {
					return true, fmt.Sprintf("Cash level low: %.2f", state.Portfolio.Cash)
				}
				return false, ""
			},
			Level:    AlertLevelWarning,
			Cooldown: 5 * time.Minute,
		},
		{
			Name:        "position_concentration",
			Description: "High position concentration detected",
			Condition: func(state *live.State) (bool, string) {
				if state == nil || len(state.Positions) == 0 {
					return false, ""
				}
				// 检查持仓数量异常
				if len(state.Positions) > 20 {
					return true, fmt.Sprintf("Too many positions: %d", len(state.Positions))
				}
				return false, ""
			},
			Level:    AlertLevelWarning,
			Cooldown: 10 * time.Minute,
		},
		{
			Name:        "system_ready",
			Description: "System is ready for trading",
			Condition: func(state *live.State) (bool, string) {
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
