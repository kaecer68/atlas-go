package industry

import (
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// SiliconCyclePhase represents the phase of the semiconductor industry cycle.
// Phases are ordered integers from recovery through contraction.
type SiliconCyclePhase int

const (
	// PhaseBottomRecovery is phase 0 — 谷底復甦: early signals of semiconductor recovery.
	PhaseBottomRecovery SiliconCyclePhase = 0

	// PhaseExpansionConfirmed is phase 1 — 擴張確認: broad-based semiconductor strength.
	PhaseExpansionConfirmed SiliconCyclePhase = 1

	// PhaseOverheat is phase 2 — 過熱期: semiconductor overheating signals.
	PhaseOverheat SiliconCyclePhase = 2

	// PhaseContraction is phase 3 — 收縮期: semiconductor downturn.
	PhaseContraction SiliconCyclePhase = 3
)

// String returns the human-readable Chinese name of the phase.
func (p SiliconCyclePhase) String() string {
	switch p {
	case PhaseBottomRecovery:
		return "谷底復甦"
	case PhaseExpansionConfirmed:
		return "擴張確認"
	case PhaseOverheat:
		return "過熱期"
	case PhaseContraction:
		return "收縮期"
	default:
		return "未知"
	}
}

// SiliconIndicators holds the input indicators for silicon cycle phase detection.
// All values are year-over-year percentages except DRAMSpotPriceTrend which is
// a normalized trend signal.
type SiliconIndicators struct {
	// TSMCMonthlyRevenueYoY is 台積電月營收年增率 (fraction, e.g. 0.25 = +25%).
	TSMCMonthlyRevenueYoY float64

	// GlobalSemiconductorBillingsYoY is 全球半導體出貨年增率 (fraction).
	GlobalSemiconductorBillingsYoY float64

	// DRAMSpotPriceTrend is DRAM現貨價格趨勢 (normalized, positive = rising).
	DRAMSpotPriceTrend float64

	// TaiwanSemiconductorIndexMA is 台灣半導體指數偏離季線比例 (fraction).
	TaiwanSemiconductorIndexMA float64

	// TSMCCapexGuidance is 台積電資本支出指引變動 (fraction, negative = cut).
	TSMCCapexGuidance float64

	// PhiladelphiaSOXIndexYoY is 費城半導體指數年增率 (fraction).
	PhiladelphiaSOXIndexYoY float64
}

// SiliconCycleParams holds configurable thresholds for silicon cycle phase detection.
type SiliconCycleParams struct {
	// RevenueYoYThreshold triggers expansion when TSMC revenue growth exceeds this.
	RevenueYoYThreshold float64

	// BillingsYoYThreshold triggers expansion when global billings growth exceeds this.
	BillingsYoYThreshold float64

	// DRAMStabilizationThreshold marks DRAM trend as "stabilizing" above this value.
	DRAMStabilizationThreshold float64

	// BillingsStabilizationThreshold marks billings as "stabilizing" above this value.
	BillingsStabilizationThreshold float64

	// IndexMAPercentThreshold triggers overheat when TW semi index exceeds MA by this.
	IndexMAPercentThreshold float64

	// SOXExtremeThreshold triggers overheat when Philly SOX YoY exceeds this.
	SOXExtremeThreshold float64

	// CapexCutThreshold triggers contraction when TSMC capex guidance drops below negative of this.
	CapexCutThreshold float64

	// MinConfidence is the minimum confidence to override a prior phase detection.
	MinConfidence float64

	// HistoryWindowSize is the maximum number of phase transitions to retain.
	HistoryWindowSize int
}

// defaultSiliconCycleParams returns sensible defaults for silicon cycle detection.
// These are used when ParametersConfig does not contain silicon-specific thresholds.
func defaultSiliconCycleParams() SiliconCycleParams {
	return SiliconCycleParams{
		RevenueYoYThreshold:            0.15,
		BillingsYoYThreshold:           0.10,
		DRAMStabilizationThreshold:     0.0,
		BillingsStabilizationThreshold: -0.05,
		IndexMAPercentThreshold:        0.20,
		SOXExtremeThreshold:            0.40,
		CapexCutThreshold:              0.10,
		MinConfidence:                  0.60,
		HistoryWindowSize:              60,
	}
}

// getSiliconParams returns silicon cycle parameters from config or defaults.
// Follows the same pattern as defaultCycleThresholds() in cycle.go.
func getSiliconParams() SiliconCycleParams {
	params := defaultSiliconCycleParams()
	cfg := config.GetParametersConfig()
	if cfg == nil {
		return params
	}
	// Override with config-driven values if available.
	// In a future integration, SiliconCycle thresholds would be added to
	// ParametersConfig.Industry as a ParameterMetadata[SiliconCycleParams] field.
	// For now, use the defaults as the authoritative source.
	_ = cfg // config is available but silicon-specific fields pending integration
	return params
}

// PhaseTransition records a state machine transition event.
type PhaseTransition struct {
	FromPhase  SiliconCyclePhase `json:"from_phase"`
	ToPhase    SiliconCyclePhase `json:"to_phase"`
	Timestamp  time.Time         `json:"timestamp"`
	Indicators SiliconIndicators `json:"indicators"`
}

// SiliconCycleTracker monitors and tracks the semiconductor industry cycle
// using a 4-phase state machine driven by key silicon industry indicators.
type SiliconCycleTracker struct {
	currentPhase      SiliconCyclePhase
	mu                sync.RWMutex
	history           []PhaseTransition
	latestIndicators SiliconIndicators
	hasIndicators     bool
}

// NewSiliconCycleTracker creates a new silicon cycle engine initialized to
// PhaseBottomRecovery (phase 0).
func NewSiliconCycleTracker() *SiliconCycleTracker {
	return &SiliconCycleTracker{
		currentPhase: PhaseBottomRecovery,
		history:      make([]PhaseTransition, 0),
	}
}

// DetectPhase evaluates the current silicon indicators against the state machine
// rules and returns the resulting phase. If a phase transition occurs, it is
// recorded in the engine's history. The input time is used as the transition
// timestamp.
func (e *SiliconCycleTracker) DetectPhase(now time.Time, indicators SiliconIndicators) SiliconCyclePhase {
	e.mu.Lock()
	defer e.mu.Unlock()

	params := getSiliconParams()
	prevPhase := e.currentPhase
	newPhase := e.evaluateTransition(prevPhase, indicators, params)

	e.latestIndicators = indicators
	e.hasIndicators = true

	if newPhase != prevPhase {
		e.history = append(e.history, PhaseTransition{
			FromPhase:  prevPhase,
			ToPhase:    newPhase,
			Timestamp:  now,
			Indicators: indicators,
		})
		// Trim history to window size
		if len(e.history) > params.HistoryWindowSize {
			e.history = e.history[len(e.history)-params.HistoryWindowSize:]
		}
		e.currentPhase = newPhase
	}

	return e.currentPhase
}

// evaluateTransition applies the state machine rules to determine the next phase.
// Each phase has specific transition conditions. Only the transition from the
// current phase is evaluated — this provides hysteresis and prevents rapid
// phase flickering.
func (e *SiliconCycleTracker) evaluateTransition(current SiliconCyclePhase, ind SiliconIndicators, p SiliconCycleParams) SiliconCyclePhase {
	switch current {
	case PhaseBottomRecovery:
		// 0 → 1 trigger: revenue recovery, billings recovery, DRAM trending up.
		if ind.TSMCMonthlyRevenueYoY > p.RevenueYoYThreshold &&
			ind.GlobalSemiconductorBillingsYoY > p.BillingsYoYThreshold &&
			ind.DRAMSpotPriceTrend > p.DRAMStabilizationThreshold {
			return PhaseExpansionConfirmed
		}
		return PhaseBottomRecovery

	case PhaseExpansionConfirmed:
		// 1 → 2 trigger: index overheated (far above MA) OR Philly SOX extreme.
		if ind.TaiwanSemiconductorIndexMA > p.IndexMAPercentThreshold ||
			ind.PhiladelphiaSOXIndexYoY > p.SOXExtremeThreshold {
			return PhaseOverheat
		}
		// 1 → 3 trigger: capex cut signals downturn even from expansion.
		if ind.TSMCCapexGuidance < -p.CapexCutThreshold {
			return PhaseContraction
		}
		return PhaseExpansionConfirmed

	case PhaseOverheat:
		// 2 → 3 trigger: capex cut signals contraction.
		if ind.TSMCCapexGuidance < -p.CapexCutThreshold {
			return PhaseContraction
		}
		return PhaseOverheat

	case PhaseContraction:
		// 3 → 0 trigger: billings stabilization AND DRAM bottoming out.
		if ind.GlobalSemiconductorBillingsYoY > p.BillingsStabilizationThreshold &&
			ind.DRAMSpotPriceTrend > p.DRAMStabilizationThreshold {
			return PhaseBottomRecovery
		}
		return PhaseContraction

	default:
		return PhaseBottomRecovery
	}
}

// GetCurrentPhase returns the current silicon cycle phase.
func (e *SiliconCycleTracker) GetCurrentPhase() SiliconCyclePhase {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentPhase
}

// GetPhaseName returns the human-readable Chinese name for a phase.
func GetPhaseName(phase SiliconCyclePhase) string {
	return phase.String()
}

// GetPhaseDescription returns a detailed description of each phase.
func GetPhaseDescription(phase SiliconCyclePhase) string {
	switch phase {
	case PhaseBottomRecovery:
		return "半導體谷底復甦期：台積電營收回穩、全球半導體出貨開始回暖、DRAM價格觸底，市場信心逐步恢復。適合逐步建立半導體相關部位。"
	case PhaseExpansionConfirmed:
		return "半導體擴張確認期：營收與出貨同步成長、DRAM價格趨勢向上、下游需求強勁。半導體族群全面走強，適合積極配置。"
	case PhaseOverheat:
		return "半導體過熱期：指數大幅偏離均線、費城半導體指數極端上漲、市場過度樂觀。適合逐步減碼，注意回檔風險。"
	case PhaseContraction:
		return "半導體收縮期：台積電資本支出下修、需求放緩、庫存去化進行中。半導體族群面臨下行壓力，適合防禦性配置。"
	default:
		return "未知階段"
	}
}

// GetPhaseScore returns a normalized score from 0.0 to 1.0 for each phase.
// Higher scores indicate more favorable conditions for semiconductor investment.
func GetPhaseScore(phase SiliconCyclePhase) float64 {
	switch phase {
	case PhaseExpansionConfirmed:
		return 1.0
	case PhaseBottomRecovery:
		return 0.65
	case PhaseOverheat:
		return 0.40
	case PhaseContraction:
		return 0.15
	default:
		return 0.0
	}
}

// GetTypicalDuration returns the typical duration in days for each phase.
func GetTypicalDuration(phase SiliconCyclePhase) int {
	switch phase {
	case PhaseBottomRecovery:
		return 90
	case PhaseExpansionConfirmed:
		return 360
	case PhaseOverheat:
		return 120
	case PhaseContraction:
		return 180
	default:
		return 0
	}
}

// GetPhaseWeightMultiplier returns a portfolio weight multiplier for each phase.
// Phase 0 (recovery): 1.05 — moderate boost
// Phase 1 (expansion): 1.10 — strongest allocation
// Phase 2 (overheat):  0.90 — reduce exposure
// Phase 3 (contraction): 0.85 — defensive underweight
func GetPhaseWeightMultiplier(phase SiliconCyclePhase) float64 {
	switch phase {
	case PhaseBottomRecovery:
		return 1.05
	case PhaseExpansionConfirmed:
		return 1.10
	case PhaseOverheat:
		return 0.90
	case PhaseContraction:
		return 0.85
	default:
		return 1.0
	}
}

// GetHistory returns the history of phase transitions.
func (e *SiliconCycleTracker) GetHistory() []PhaseTransition {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]PhaseTransition, len(e.history))
	copy(result, e.history)
	return result
}

// GetLatestIndicators returns the most recently observed silicon indicators
// and whether any have been recorded (false for a fresh tracker).
func (e *SiliconCycleTracker) GetLatestIndicators() (SiliconIndicators, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.latestIndicators, e.hasIndicators
}

// GetTransitionCount returns the number of recorded phase transitions.
func (e *SiliconCycleTracker) GetTransitionCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.history)
}

// Reset resets the engine to PhaseBottomRecovery with an empty history.
func (e *SiliconCycleTracker) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.currentPhase = PhaseBottomRecovery
	e.history = make([]PhaseTransition, 0)
}

// DaysInCurrentPhase estimates the number of days spent in the current phase
// based on the most recent transition timestamp. Returns 0 if no transition
// has been recorded.
func (e *SiliconCycleTracker) DaysInCurrentPhase(now time.Time) int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.history) == 0 {
		return 0
	}
	lastTransition := e.history[len(e.history)-1].Timestamp
	return int(math.Max(0, now.Sub(lastTransition).Hours()/24.0))
}

// IsFavorable returns true when the current phase is favorable for
// semiconductor investment (recovery or expansion).
func (e *SiliconCycleTracker) IsFavorable() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentPhase == PhaseBottomRecovery || e.currentPhase == PhaseExpansionConfirmed
}

// String returns a human-readable summary of the engine state.
func (e *SiliconCycleTracker) String() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return fmt.Sprintf("SiliconCycle: Phase=%s(%d), Transitions=%d",
		e.currentPhase.String(), e.currentPhase, len(e.history))
}

// ExtractSiliconIndicators maps a MacroDataSnapshot into the six
// SiliconIndicators used by SiliconCycleTracker.DetectPhase.
//
// Conversion rules (consistent with silicon_cycle_test.go fixtures):
//   - TSMCMonthlyRevenueYoY:  MacroDataSnapshot.TSMCRevenue.ChangePct / 100
//   - GlobalSemiconductorBillingsYoY: SOX index YoY (scaled by 0.85, the
//     historical correlation between SOX YoY and WSTS billings YoY)
//   - DRAMSpotPriceTrend:     MacroDataSnapshot.DRAMSpotPrice.ChangePct / 100
//     (MU stock daily change serves as a high-frequency DRAM proxy)
//   - TaiwanSemiconductorIndexMA: MacroDataSnapshot.TaiwanSemiIndex.ChangePct / 100
//   - TSMCCapexGuidance:      heuristic from TSMC revenue YoY direction
//     (revenue growth >15% → +0.05; revenue decline → -0.05; else 0)
//   - PhiladelphiaSOXIndexYoY: SOX index YoY (annualized daily proxy)
//
// All four heuristic-derived values default to 0.0 when the underlying
// snapshot fields are zero-valued, so providers that have not yet been
// integrated do not poison phase detection.
func ExtractSiliconIndicators(snap marketdata.MacroDataSnapshot) SiliconIndicators {
	// TSMC capex heuristic: revenue growth >15% implies capex expansion;
	// revenue decline implies capex contraction. Scaled to ±0.05 for subtle
	// influence on the state machine (capex alone should not dominate).
	tsmcRevYoY := snap.TSMCRevenue.ChangePct / 100.0
	capexSignal := 0.0
	if tsmcRevYoY > 0.15 {
		capexSignal = 0.05
	} else if tsmcRevYoY < 0.0 {
		capexSignal = -0.05
	}

	// DRAM trend from MU (Micron) daily change: positive → DRAM trending up.
	dramTrend := snap.DRAMSpotPrice.ChangePct / 100.0

	// SOX YoY from provider (now computes proper year-over-year via 1y range).
	soxYoY := snap.SOXIndex.ChangePct / 100.0

	return SiliconIndicators{
		TSMCMonthlyRevenueYoY:          tsmcRevYoY,
		GlobalSemiconductorBillingsYoY: soxYoY * 0.85, // SOX → billings scaling (~85% correlation)
		DRAMSpotPriceTrend:             dramTrend,
		TaiwanSemiconductorIndexMA:     snap.TaiwanSemiIndex.ChangePct / 100.0,
		TSMCCapexGuidance:              capexSignal,
		PhiladelphiaSOXIndexYoY:        soxYoY,
	}
}
