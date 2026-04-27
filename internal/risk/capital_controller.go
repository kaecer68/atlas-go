package risk

import (
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// CapitalPhaseController manages capital phase transitions and limits.
type CapitalPhaseController struct {
	config   domain.CapitalPhaseConfig
	snapshot domain.CapitalSnapshot
}

// NewCapitalPhaseController creates a controller with the given config.
func NewCapitalPhaseController(cfg domain.CapitalPhaseConfig) *CapitalPhaseController {
	return &CapitalPhaseController{
		config: cfg,
		snapshot: domain.CapitalSnapshot{
			Phase:          cfg.CurrentPhase,
			PhaseStartDate: cfg.PhaseStartDate,
			CanAdvance:     false,
		},
	}
}

// GetSnapshot returns the current capital snapshot.
func (c *CapitalPhaseController) GetSnapshot() domain.CapitalSnapshot {
	return c.snapshot
}

// UpdateMetrics updates the rolling Sharpe ratio and max drawdown in the snapshot.
func (c *CapitalPhaseController) UpdateMetrics(rollingSharpe, maxDrawdown float64) {
	daysInPhase := int(time.Since(c.config.PhaseStartDate).Hours() / 24)

	c.snapshot.RollingSharpe = rollingSharpe
	c.snapshot.MaxDrawdown = maxDrawdown
	c.snapshot.DaysInPhase = daysInPhase

	canAdvance, reason := c.evaluateAdvanceCriteria()
	c.snapshot.CanAdvance = canAdvance
	c.snapshot.AdvanceReason = reason
}

// CanAdvance checks if the current phase meets criteria to transition to the next phase.
func (c *CapitalPhaseController) CanAdvance() (bool, string) {
	return c.evaluateAdvanceCriteria()
}

// AdvancePhase transitions to the next capital phase.
func (c *CapitalPhaseController) AdvancePhase() error {
	canAdvance, reason := c.evaluateAdvanceCriteria()
	if !canAdvance {
		return fmt.Errorf("cannot advance: %s", reason)
	}

	nextPhase := c.nextPhase()
	if nextPhase == "" {
		return fmt.Errorf("already at final phase %q", string(c.config.CurrentPhase))
	}

	c.config.CurrentPhase = nextPhase
	c.config.PhaseStartDate = time.Now()
	c.snapshot.Phase = nextPhase
	c.snapshot.PhaseStartDate = c.config.PhaseStartDate
	c.snapshot.DaysInPhase = 0
	c.snapshot.CanAdvance = false
	c.snapshot.AdvanceReason = ""

	return nil
}

// GetCapitalLimit returns the capital limit multiplier for the current phase.
func (c *CapitalPhaseController) GetCapitalLimit() float64 {
	limit, ok := c.config.CapitalLimits[string(c.config.CurrentPhase)]
	if !ok {
		return 1.0
	}
	return limit
}

// CalculateMaxPositionSize calculates the maximum position size based on
// total capital and the current phase's capital limit.
func (c *CapitalPhaseController) CalculateMaxPositionSize(totalCapital float64) float64 {
	limit := c.GetCapitalLimit()
	return totalCapital * limit
}

func (c *CapitalPhaseController) evaluateAdvanceCriteria() (bool, string) {
	daysInPhase := int(time.Since(c.config.PhaseStartDate).Hours() / 24)

	if daysInPhase < c.config.MinDaysPerPhase {
		return false, fmt.Sprintf("minimum days not met: %d < %d", daysInPhase, c.config.MinDaysPerPhase)
	}

	if c.snapshot.MaxDrawdown > c.config.MaxDrawdownLimit {
		return false, fmt.Sprintf("drawdown limit exceeded: %.4f > %.4f", c.snapshot.MaxDrawdown, c.config.MaxDrawdownLimit)
	}

	if c.snapshot.RollingSharpe < c.config.SharpeThreshold {
		return false, fmt.Sprintf("sharpe threshold not met: %.4f < %.4f", c.snapshot.RollingSharpe, c.config.SharpeThreshold)
	}

	if c.snapshot.ConsecutiveLosses >= 5 {
		return false, fmt.Sprintf("consecutive loss limit exceeded: %d >= 5", c.snapshot.ConsecutiveLosses)
	}

	return true, "all criteria met"
}

// nextPhase returns the next phase in the progression, or empty string if at the end.
func (c *CapitalPhaseController) nextPhase() domain.CapitalPhase {
	phaseOrder := []domain.CapitalPhase{
		domain.PhaseSimulation,
		domain.PhasePaper,
		domain.PhaseLive,
		domain.PhaseFull,
	}

	for i, p := range phaseOrder {
		if p == c.config.CurrentPhase {
			if i+1 < len(phaseOrder) {
				return phaseOrder[i+1]
			}
			return ""
		}
	}
	return ""
}

func (c *CapitalPhaseController) RecordLoss() {
	c.snapshot.ConsecutiveLosses++
}

func (c *CapitalPhaseController) RecordWin() {
	c.snapshot.ConsecutiveLosses = 0
}

func CalculateSharpeRatio(dailyReturns []float64) float64 {
	if len(dailyReturns) < 2 {
		return 0.0
	}

	var sum float64
	for _, r := range dailyReturns {
		sum += r
	}
	mean := sum / float64(len(dailyReturns))

	var variance float64
	for _, r := range dailyReturns {
		diff := r - mean
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(len(dailyReturns)-1))

	if stdDev == 0 {
		return 0.0
	}

	return (mean / stdDev) * math.Sqrt(252)
}
