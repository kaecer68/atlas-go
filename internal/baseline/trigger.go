package baseline

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// Violation represents a single baseline constraint violation.
type Violation struct {
	Symbol   string  `json:"symbol"`
	Field    string  `json:"field"` // e.g. "stop_loss_pct", "max_holding_days"
	Actual   float64 `json:"actual"`
	Limit    float64 `json:"limit"`
	Severity string  `json:"severity"` // "warn" | "error"
	Message  string  `json:"message"`
}

// Trigger subscribes to EventPositionUpdate and evaluates baseline constraints.
type Trigger struct {
	manager   *Manager
	bus       eventbus.EventBus
	sub       eventbus.Subscription
	started   bool
	logger    *slog.Logger
	startTime time.Time
}

// NewTrigger creates a Trigger bound to a Manager (for policy loading) and an event bus.
func NewTrigger(manager *Manager, bus eventbus.EventBus) *Trigger {
	return &Trigger{
		manager: manager,
		bus:     bus,
		logger:  logging.With("baseline_trigger"),
	}
}

// Start begins listening for position update events. Idempotent: returns an error if already started.
func (t *Trigger) Start(ctx context.Context) error {
	if t.started {
		return fmt.Errorf("trigger already started")
	}
	if t.manager == nil {
		return fmt.Errorf("manager is required")
	}
	if t.bus == nil {
		return fmt.Errorf("event bus is required")
	}
	t.sub = t.bus.Subscribe(eventbus.EventPositionUpdate, t.onPositionUpdate)
	t.started = true
	t.startTime = time.Now()
	t.logger.InfoContext(ctx, "trigger_started", "policy_path", t.manager.path)
	return nil
}

// Stop unsubscribes from the event bus. Double-stop is a no-op.
func (t *Trigger) Stop() error {
	if !t.started {
		return nil
	}
	if t.sub.Cancel != nil {
		t.sub.Cancel()
	}
	t.sub = eventbus.Subscription{}
	t.started = false
	t.logger.Info("trigger_stopped", "uptime", time.Since(t.startTime).String())
	return nil
}

func (t *Trigger) onPositionUpdate(ctx context.Context, event eventbus.BusEvent) error {
	payload, ok := event.Payload.(eventbus.PositionEventPayload)
	if !ok {
		return nil // wrong payload type, ignore
	}

	policy, err := Load(t.manager.path)
	if err != nil {
		t.logger.WarnContext(ctx, "policy_load_failed", logging.Err(err))
		return nil
	}

	violations := t.evaluate(payload, policy)
	for _, v := range violations {
		t.logger.WarnContext(
			ctx, "baseline_violation",
			"symbol", v.Symbol,
			"field", v.Field,
			"actual", v.Actual,
			"limit", v.Limit,
			"severity", v.Severity,
			"message", v.Message,
		)
	}
	return nil
}

// evaluate checks the position against the policy's simulation constraints.
// It does not aggregate portfolio-level risk; per-position checks only.
// StopLossPct/TakeProfitPct follow the same convention as internal/sim/engine.go:
// positive values represent the magnitude of the move that triggers the rule.
func (t *Trigger) evaluate(p eventbus.PositionEventPayload, policy Policy) []Violation {
	var violations []Violation
	pos := p.Position

	if pos.AverageCost <= 0 {
		return violations
	}

	pctChange := (pos.CurrentPrice - pos.AverageCost) / pos.AverageCost

	// 1. Stop loss check: trigger when the loss magnitude exceeds StopLossPct.
	if policy.Constraints.StopLossPct > 0 && pctChange <= -policy.Constraints.StopLossPct {
		violations = append(violations, Violation{
			Symbol:   pos.Symbol,
			Field:    "stop_loss_pct",
			Actual:   pctChange,
			Limit:    -policy.Constraints.StopLossPct,
			Severity: "error",
			Message:  fmt.Sprintf("stop loss triggered at %.2f%% (limit %.2f%%)", pctChange*100, -policy.Constraints.StopLossPct*100),
		})
	}

	// 2. Take profit check.
	if policy.Constraints.TakeProfitPct > 0 && pctChange >= policy.Constraints.TakeProfitPct {
		violations = append(violations, Violation{
			Symbol:   pos.Symbol,
			Field:    "take_profit_pct",
			Actual:   pctChange,
			Limit:    policy.Constraints.TakeProfitPct,
			Severity: "warn",
			Message:  fmt.Sprintf("take profit threshold reached at %.2f%%", pctChange*100),
		})
	}

	// 3. Max holding days check.
	if policy.Constraints.MaxHoldingDays > 0 && !pos.EntryDate.IsZero() {
		holdingDays := int(time.Since(pos.EntryDate).Hours() / 24)
		if holdingDays >= policy.Constraints.MaxHoldingDays {
			violations = append(violations, Violation{
				Symbol:   pos.Symbol,
				Field:    "max_holding_days",
				Actual:   float64(holdingDays),
				Limit:    float64(policy.Constraints.MaxHoldingDays),
				Severity: "warn",
				Message:  fmt.Sprintf("held for %d days, exceeding max %d", holdingDays, policy.Constraints.MaxHoldingDays),
			})
		}
	}

	return violations
}
