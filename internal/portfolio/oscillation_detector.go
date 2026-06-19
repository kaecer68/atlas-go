package portfolio

import (
	"fmt"
	"sync"
)

// OscillationVerdict captures whether the current calibration would oscillate
// relative to the previous one, and the suggested damping factor.
type OscillationVerdict struct {
	Detected        bool
	DampingFactor   float64
	AffectedFactors []FactorType
	Reason          string
}

var (
	oscMu             sync.Mutex
	lastFactorDeltas  map[FactorType]float64
)

// detectOscillation compares the proposed delta (optimal - current) against
// the previously applied delta. A factor is "flipping" when the two deltas
// have opposite signs. Two or more flips across the same calibration cycle is
// treated as oscillation, and the caller should dampen the adjustment by the
// returned factor (1.0 = no dampen).
func detectOscillation(current, optimal map[FactorType]float64) OscillationVerdict {
	oscMu.Lock()
	prev := lastFactorDeltas
	oscMu.Unlock()

	var flipped []FactorType
	newDelta := make(map[FactorType]float64, len(optimal))
	for ft, opt := range optimal {
		cur := current[ft]
		delta := opt - cur
		newDelta[ft] = delta
		if p, ok := prev[ft]; ok && p != 0 && delta != 0 {
			if (p > 0) != (delta > 0) {
				flipped = append(flipped, ft)
			}
		}
	}

	if len(flipped) >= 2 {
		oscMu.Lock()
		lastFactorDeltas = newDelta
		oscMu.Unlock()
		return OscillationVerdict{
			Detected:        true,
			DampingFactor:   0.5,
			AffectedFactors: flipped,
			Reason:          fmt.Sprintf("%d factors flipped sign vs prior calibration", len(flipped)),
		}
	}

	oscMu.Lock()
	lastFactorDeltas = newDelta
	oscMu.Unlock()
	return OscillationVerdict{Detected: false, DampingFactor: 1.0}
}

func dampenWeights(weights, current map[FactorType]float64, factor float64) map[FactorType]float64 {
	out := make(map[FactorType]float64, len(weights))
	for ft, w := range weights {
		cur := current[ft]
		out[ft] = cur + (w-cur)*factor
	}
	return out
}

// resetOscillationState clears the oscillation history. Test-only.
func resetOscillationState() {
	oscMu.Lock()
	lastFactorDeltas = nil
	oscMu.Unlock()
}