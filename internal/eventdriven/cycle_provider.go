package eventdriven

// CyclePhaseSource is the subset of industry.CycleTracker the sector predictor
// consumes: the continuous phase score it projects into `cycle_position`.
type CyclePhaseSource interface {
	GetContinuousPhaseScore(industryID string) float64
	// EvidenceTier reports how the tracker's cycle position was obtained:
	// "empirical" (written by the measured data path more than once),
	// "estimated" (startup seed only) or "insufficient" (no data).
	EvidenceTier(industryID string) string
}

// cycleEvidenceEmpirical mirrors industry.CycleTracker.EvidenceTier()'s
// "empirical" tier (history longer than the single startup seed entry).
const cycleEvidenceEmpirical = "empirical"

// MeasuredCycleProvider adapts a CyclePhaseSource for the sector predictor and
// refuses to answer for industries whose position is still just the startup
// seed.
//
// Why the filter (#1944 Batch 4, item I4 cycle half): every CycleTracker starts
// with config-seeded positions (industry.initializeDefaultPositions), and the
// measured path is the 6-hourly auto_cycle_update task, which writes
// FinMind-derived revenue/profit growth through CycleTracker.UpdatePosition.
// Injecting the tracker raw would present a config seed as a measured cycle
// position for every industry the data task has not covered yet; returning the
// neutral 0.0 for those keeps the predictor's cycle_position contribution
// identical to the unwired behavior until real data exists.
type MeasuredCycleProvider struct {
	src CyclePhaseSource
}

// NewMeasuredCycleProvider wraps src. A nil src yields a provider that always
// reports the neutral 0.0 (i.e. no cycle contribution), never a panic.
func NewMeasuredCycleProvider(src CyclePhaseSource) *MeasuredCycleProvider {
	return &MeasuredCycleProvider{src: src}
}

// GetContinuousPhaseScore returns the tracker's phase score for industries with
// measured evidence, and the neutral 0.0 otherwise.
func (p *MeasuredCycleProvider) GetContinuousPhaseScore(industryID string) float64 {
	if p == nil || p.src == nil {
		return 0.0
	}
	if p.src.EvidenceTier(industryID) != cycleEvidenceEmpirical {
		return 0.0
	}
	return p.src.GetContinuousPhaseScore(industryID)
}
