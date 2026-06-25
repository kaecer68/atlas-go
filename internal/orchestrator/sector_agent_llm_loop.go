package orchestrator

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// BaseConvictionDriver is the deterministic LLMDriver used by the
// SectorAgentLLM loop when no real LLM backend is wired. It returns a
// single "final" plan step whose Args carry the input conviction, allowing
// the loop to converge in one iteration without invoking any external
// service.
//
// Wave 11 L2.1 (Issue #719, Phase 2): this driver is the placeholder that
// makes the LLM-driven sector agent loop observable in production without
// pulling a real LLM call into the S/E hot path. It preserves replay
// reproducibility (deterministic output) and gives the Darwinian weight
// loop a real Conviction floor to operate on.
//
// When a production LLMDriver is wired via system.WithLLMSectorAgents(drv),
// BaseConvictionDriver is replaced by the production driver (typically
// backed by llm.Router). The loop path is identical.
type BaseConvictionDriver struct {
	// PlanDelay simulates an LLM round-trip latency for observability.
	PlanDelay time.Duration
}

// NewBaseConvictionDriver returns a BaseConvictionDriver with the default
// plan delay (no delay; the loop is meant to be fast for backtests).
func NewBaseConvictionDriver() *BaseConvictionDriver {
	return &BaseConvictionDriver{}
}

// PlanComplete returns a single-step plan whose Args carry the input skill
// and symbol so the loop driver can detect sector context. The driver
// intentionally returns just one step so the loop converges to reflection
// on the next tick without an additional round-trip.
func (d *BaseConvictionDriver) PlanComplete(
	ctx context.Context,
	skill, symbol string,
) ([]PlanStep, error) {
	if d.PlanDelay > 0 {
		select {
		case <-time.After(d.PlanDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return []PlanStep{
		{
			Kind:     "thought",
			ToolName: "",
			Args: map[string]any{
				"skill":  skill,
				"symbol": symbol,
			},
			Note: "base conviction plan: defer to deterministic sector path",
		},
	}, nil
}

// ReflectComplete always signals convergence (Continue=false) with the
// unchanged conviction. This keeps the loop deterministic and bounded.
func (d *BaseConvictionDriver) ReflectComplete(
	ctx context.Context,
	skill, symbol, toolResult string,
) (Reflection, error) {
	return Reflection{
		Continue:        false,
		FinalConviction: 0,
		Reasoning:       "base conviction reflection: loop converged",
	}, nil
}

// RunSectorAgentLoop drives a SectorAgentLLM through its plan → tool_call →
// reflect cycle, returning the final conviction (0..100). The loop is
// bounded by SectorAgentLLM.AgentLoop.MaxIter; an iteration that exceeds
// the budget forces FinalConviction to the agent's ConvictionFloor so the
// caller gets a deterministic fallback.
//
// This function is the Phase 2 wiring for Issue #719: when the flag is on
// and a non-nil driver is present, the sector-layer recommendation path
// invokes this loop instead of returning the deterministic rec directly.
// When the driver is nil, callers should fall back to the deterministic path
// (RunSectorAgentLoop is not called).
func RunSectorAgentLoop(
	ctx context.Context,
	agent *SectorAgentLLM,
	symbol string,
	base domain.Recommendation,
) (int, error) {
	if agent == nil {
		return 0, ErrNotImplemented
	}
	if agent.LLM == nil {
		return 0, ErrNotImplemented
	}
	if agent.ConvictionFloor > 0 {
		base.Conviction = agent.ConvictionFloor
	}
	steps, err := agent.PlanStep(ctx, symbol, base)
	if err != nil {
		return base.Conviction, err
	}
	agent.AdvancePlan(steps)
	for _, step := range steps {
		agent.AdvanceToolCall()
		result, err := agent.RunToolCall(ctx, step)
		if err != nil {
			return base.Conviction, err
		}
		agent.AdvanceReflect()
		refl, err := agent.Reflect(ctx, symbol, result, base.Conviction)
		if err != nil {
			return base.Conviction, err
		}
		if refl.FinalConviction > 0 {
			base.Conviction = clampConviction(refl.FinalConviction)
		}
		if !refl.Continue || agent.Exhausted() {
			break
		}
	}
	agent.AdvanceFinal(base.Conviction)
	return base.Conviction, nil
}

func clampConviction(c int) int {
	if c < 0 {
		return 0
	}
	if c > 100 {
		return 100
	}
	return c
}
