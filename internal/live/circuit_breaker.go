package live

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// CircuitState represents the current trading halt level.
type CircuitState string

const (
	CircuitNormal CircuitState = "normal" // All trading allowed
	CircuitPaused CircuitState = "paused" // No new long positions; close-only
	CircuitHalted CircuitState = "halted" // All trading stopped
)

// CircuitBreakerRule defines a single trigger condition.
type CircuitBreakerRule struct {
	Name            string
	Enabled         bool
	DailyLossPct    float64 // e.g. 2.0
	DrawdownPct     float64 // e.g. 3.0
	ConsecutiveSL   int     // e.g. 3
	CooldownMinutes int     // e.g. 15
}

// DefaultCircuitBreakerRules returns conservative production defaults.
func DefaultCircuitBreakerRules() []CircuitBreakerRule {
	return []CircuitBreakerRule{
		{
			Name:            "daily_loss_limit",
			Enabled:         true,
			DailyLossPct:    2.0,
			DrawdownPct:     3.0,
			ConsecutiveSL:   3,
			CooldownMinutes: 15,
		},
	}
}

// CircuitBreakerEvent records a state transition.
type CircuitBreakerEvent struct {
	Timestamp   time.Time    `json:"timestamp"`
	FromState   CircuitState `json:"from_state"`
	ToState     CircuitState `json:"to_state"`
	Reason      string       `json:"reason"`
	DayPnLPct   float64      `json:"day_pnl_pct"`
	DrawdownPct float64      `json:"drawdown_pct"`
}

// CircuitBreaker implements portfolio-level circuit breaker logic.
type CircuitBreaker struct {
	rules          []CircuitBreakerRule
	state          CircuitState
	stateChangedAt time.Time
	consecutiveSL  int
	lastSLTime     time.Time
	cooldownUntil  time.Time
	intradayPeak   float64
	dayStartValue  float64

	mu        sync.RWMutex
	logPath   string
	statePath string
}

// NewCircuitBreaker creates a circuit breaker with default rules.
func NewCircuitBreaker(logPath, statePath string) *CircuitBreaker {
	if logPath == "" {
		logPath = "data/state/circuit_breaker_log.jsonl"
	}
	if statePath == "" {
		statePath = livestore.DefaultCircuitBreakerStatePath
	}
	cb := &CircuitBreaker{
		rules:          DefaultCircuitBreakerRules(),
		state:          CircuitNormal,
		stateChangedAt: time.Now(),
		logPath:        logPath,
		statePath:      statePath,
	}
	if err := cb.loadState(); err != nil {
		if !os.IsNotExist(err) {
			logging.Warn("circuit_breaker", "failed_to_load_state", logging.Err(err))
		}
	}
	return cb
}

// SetRules replaces the active rules.
func (cb *CircuitBreaker) SetRules(rules []CircuitBreakerRule) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.rules = rules
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// ResetDayState resets daily counters at market open.
func (cb *CircuitBreaker) ResetDayState(startingPortfolioValue float64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitNormal
	cb.stateChangedAt = time.Now()
	cb.consecutiveSL = 0
	cb.lastSLTime = time.Time{}
	cb.cooldownUntil = time.Time{}
	cb.intradayPeak = startingPortfolioValue
	cb.dayStartValue = startingPortfolioValue
	if err := cb.persistStateLocked(); err != nil {
		logging.Warn("circuit_breaker", "failed_to_persist_state_on_reset", logging.Err(err))
	}
}

// RecordStopLoss increments the consecutive stop-loss counter.
func (cb *CircuitBreaker) RecordStopLoss() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveSL++
	cb.lastSLTime = time.Now()
	for _, r := range cb.rules {
		if !r.Enabled || r.ConsecutiveSL <= 0 {
			continue
		}
		if cb.consecutiveSL >= r.ConsecutiveSL {
			cb.cooldownUntil = time.Now().Add(time.Duration(r.CooldownMinutes) * time.Minute)
			cb.transitionLocked(CircuitPaused, fmt.Sprintf("consecutive stop losses: %d", cb.consecutiveSL), 0, 0)
			return
		}
	}
}

// Evaluate checks all rules against current portfolio and may trigger state changes.
func (cb *CircuitBreaker) Evaluate(portfolio livestore.PortfolioState, positions map[string]domain.Position, events []livestore.Event) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Auto-recover from cooldown if time has passed
	if cb.state == CircuitPaused && time.Now().After(cb.cooldownUntil) {
		cb.transitionLocked(CircuitNormal, "cooldown expired", 0, 0)
	}

	currentValue := portfolio.Cash + portfolio.UnrealizedPnL
	if currentValue > cb.intradayPeak {
		cb.intradayPeak = currentValue
	}

	dayPnLPct := 0.0
	if cb.dayStartValue > 0 {
		dayPnLPct = (portfolio.DayPnL / cb.dayStartValue) * 100
	}
	drawdownPct := 0.0
	if cb.intradayPeak > 0 {
		drawdownPct = (cb.intradayPeak - currentValue) / cb.intradayPeak * 100
	}

	for _, r := range cb.rules {
		if !r.Enabled {
			continue
		}
		// Daily loss -> Halted
		if r.DailyLossPct > 0 && dayPnLPct < -r.DailyLossPct {
			if cb.state != CircuitHalted {
				cb.transitionLocked(CircuitHalted, fmt.Sprintf("daily loss %.2f%% exceeds %.2f%%", dayPnLPct, r.DailyLossPct), dayPnLPct, drawdownPct)
			}
			return
		}
		// Drawdown -> Paused
		if r.DrawdownPct > 0 && drawdownPct > r.DrawdownPct {
			if cb.state == CircuitNormal {
				cb.transitionLocked(CircuitPaused, fmt.Sprintf("drawdown %.2f%% exceeds %.2f%%", drawdownPct, r.DrawdownPct), dayPnLPct, drawdownPct)
			}
		}
	}
}

// CanPlaceOrder returns true if the given order side is allowed in current state.
func (cb *CircuitBreaker) CanPlaceOrder(side domain.Side) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	switch cb.state {
	case CircuitNormal:
		return true
	case CircuitPaused:
		return side == domain.SideSell
	case CircuitHalted:
		return false
	}
	return false
}

func (cb *CircuitBreaker) transitionLocked(to CircuitState, reason string, dayPnLPct, drawdownPct float64) {
	if cb.state == to {
		return
	}
	event := CircuitBreakerEvent{
		Timestamp:   time.Now(),
		FromState:   cb.state,
		ToState:     to,
		Reason:      reason,
		DayPnLPct:   dayPnLPct,
		DrawdownPct: drawdownPct,
	}
	cb.state = to
	cb.stateChangedAt = time.Now()
	if err := cb.appendLog(event); err != nil {
		logging.Warn("circuit_breaker", "failed_to_append_log", logging.Err(err))
	}
	if err := cb.persistStateLocked(); err != nil {
		logging.Warn("circuit_breaker", "failed_to_persist_state", logging.Err(err))
	}
	logging.Info(
		"circuit_breaker", "state_transition",
		"from_state", event.FromState,
		"to_state", event.ToState,
		"reason", reason,
	)
}

func (cb *CircuitBreaker) appendLog(event CircuitBreakerEvent) error {
	if err := os.MkdirAll(filepath.Dir(cb.logPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	f, err := os.OpenFile(cb.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	return enc.Encode(event)
}

func (cb *CircuitBreaker) persistStateLocked() error {
	if err := os.MkdirAll(filepath.Dir(cb.statePath), 0o755); err != nil {
		return fmt.Errorf("mkdir for circuit breaker state: %w", err)
	}
	state := struct {
		State          CircuitState `json:"state"`
		StateChangedAt time.Time    `json:"state_changed_at"`
		ConsecutiveSL  int          `json:"consecutive_sl"`
		CooldownUntil  time.Time    `json:"cooldown_until"`
		IntradayPeak   float64      `json:"intraday_peak"`
		DayStartValue  float64      `json:"day_start_value"`
	}{
		State:          cb.state,
		StateChangedAt: cb.stateChangedAt,
		ConsecutiveSL:  cb.consecutiveSL,
		CooldownUntil:  cb.cooldownUntil,
		IntradayPeak:   cb.intradayPeak,
		DayStartValue:  cb.dayStartValue,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal circuit breaker state: %w", err)
	}
	if err := livestore.WriteFileAtomic(cb.statePath, string(data)); err != nil {
		return fmt.Errorf("persist circuit breaker state: %w", err)
	}
	return nil
}

func (cb *CircuitBreaker) loadState() error {
	data, err := os.ReadFile(cb.statePath)
	if err != nil {
		return fmt.Errorf("read circuit breaker state: %w", err)
	}
	var state struct {
		State          CircuitState `json:"state"`
		StateChangedAt time.Time    `json:"state_changed_at"`
		ConsecutiveSL  int          `json:"consecutive_sl"`
		CooldownUntil  time.Time    `json:"cooldown_until"`
		IntradayPeak   float64      `json:"intraday_peak"`
		DayStartValue  float64      `json:"day_start_value"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("unmarshal circuit breaker state: %w", err)
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = state.State
	cb.stateChangedAt = state.StateChangedAt
	cb.consecutiveSL = state.ConsecutiveSL
	cb.cooldownUntil = state.CooldownUntil
	cb.intradayPeak = state.IntradayPeak
	cb.dayStartValue = state.DayStartValue
	return nil
}

func (cb *CircuitBreaker) LoadEvents() ([]CircuitBreakerEvent, error) {
	cb.mu.RLock()
	logPath := cb.logPath
	cb.mu.RUnlock()

	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read circuit breaker log: %w", err)
	}
	if len(data) == 0 {
		return nil, nil
	}
	var events []CircuitBreakerEvent
	lines := strings.SplitSeq(strings.TrimSpace(string(data)), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev CircuitBreakerEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		events = append(events, ev)
	}
	return events, nil
}

func (cb *CircuitBreaker) Reset(reason string) error {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if reason == "" {
		reason = "manual_reset"
	}
	cb.transitionLocked(CircuitNormal, reason, 0, 0)
	return nil
}
