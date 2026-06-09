package risk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
)

// CapitalPhaseController manages capital phase transitions and limits.
type CapitalPhaseController struct {
	config      domain.CapitalPhaseConfig
	snapshot    domain.CapitalSnapshot
	persistPath string
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

func NewCapitalPhaseControllerWithPersistence(cfg domain.CapitalPhaseConfig, persistDir string) *CapitalPhaseController {
	c := NewCapitalPhaseController(cfg)
	if persistDir != "" {
		c.persistPath = filepath.Join(persistDir, "capital_phase_state.json")
		if saved, err := c.LoadState(); err == nil {
			c.config = saved.Config
			c.snapshot = saved.Snapshot
		}
	}
	return c
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
	c.snapshot.ConsecutiveLosses = 0

	return c.SaveState()
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

	if c.snapshot.ConsecutiveLosses >= c.config.MaxConsecutiveLosses {
		return false, fmt.Sprintf("consecutive loss limit exceeded: %d >= %d", c.snapshot.ConsecutiveLosses, c.config.MaxConsecutiveLosses)
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
	c.persistIfConfigured()
}

func (c *CapitalPhaseController) RecordWin() {
	c.snapshot.ConsecutiveLosses = 0
	c.persistIfConfigured()
}

func (c *CapitalPhaseController) persistIfConfigured() {
	if c.persistPath == "" {
		return
	}
	_ = c.SaveState()
}

type PersistedState struct {
	Config   domain.CapitalPhaseConfig `json:"config"`
	Snapshot domain.CapitalSnapshot    `json:"snapshot"`
}

func (c *CapitalPhaseController) SaveState() error {
	if c.persistPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.persistPath), 0o755); err != nil {
		return fmt.Errorf("create persist dir: %w", err)
	}
	data, err := json.MarshalIndent(PersistedState{Config: c.config, Snapshot: c.snapshot}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	if err := os.WriteFile(c.persistPath, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

func (c *CapitalPhaseController) LoadState() (*PersistedState, error) {
	if c.persistPath == "" {
		return nil, fmt.Errorf("no persist path configured")
	}
	data, err := os.ReadFile(c.persistPath)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var state PersistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &state, nil
}

func CalculateSharpeRatio(dailyReturns []float64) float64 {
	return shared.ComputeSharpe(dailyReturns, shared.SharpeConfig{
		Frequency:  shared.FrequencyPerDay,
		MinSamples: 2,
	})
}
