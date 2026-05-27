package swarm

import "math/rand"

// JumpProcess models a jump-diffusion component for extreme market events.
type JumpProcess struct {
	Lambda float64
	MuJ    float64
	SigmaJ float64
}

func (j JumpProcess) ShouldJump() bool {
	return rand.Float64() < j.Lambda
}

func (j JumpProcess) Magnitude() float64 {
	return rand.NormFloat64()*j.SigmaJ + j.MuJ
}

func JumpParamsForRegime(regime string) JumpProcess {
	switch regime {
	case "risk_on":
		return JumpProcess{Lambda: 0.01, MuJ: 0.005, SigmaJ: 0.01}
	case "risk_off":
		return JumpProcess{Lambda: 0.03, MuJ: -0.01, SigmaJ: 0.02}
	case "crisis":
		return JumpProcess{Lambda: 0.06, MuJ: -0.025, SigmaJ: 0.04}
	case "complacent":
		return JumpProcess{Lambda: 0.005, MuJ: 0.0, SigmaJ: 0.005}
	default:
		return JumpProcess{Lambda: 0.02, MuJ: -0.005, SigmaJ: 0.015}
	}
}
