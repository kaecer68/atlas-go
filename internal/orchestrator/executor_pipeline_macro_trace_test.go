package orchestrator

import (
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ─── fixtures ───────────────────────────────────────────────────────────

// macroTraceRegistry is a minimal registry with one style agent (so the control
// layer has recommendations to scale) plus the CRO and CIO control agents (so
// the control layer records its own traces before the macro_flow trace).
func macroTraceRegistry() domain.AgentRegistry {
	return domain.AgentRegistry{
		Version: 1,
		Agents: []domain.AgentSpec{
			{ID: "gm-01", Name: "Growth Momentum", Layer: domain.LayerStyle, Skill: "growth_momentum", Enabled: true, Universe: []string{"2317.TW"}},
			{ID: "cro-01", Name: "CRO", Layer: domain.LayerControl, Skill: "cro_risk", Enabled: true},
			{ID: "cio-01", Name: "CIO", Layer: domain.LayerControl, Skill: "cio_portfolio", Enabled: true},
		},
	}
}

func macroTraceSnapshot() *marketdata.MacroDataSnapshot {
	return &marketdata.MacroDataSnapshot{
		// VIX >= 35 * 1.5 -> black_swan, i.e. a non-trivial risk-off adjustment.
		VIX:        marketdata.MacroDataPoint{Value: 54},
		RecordedAt: time.Now().Unix(),
	}
}

func traceIndex(traces []ReasoningTrace, action string) int {
	for i, tr := range traces {
		if tr.Action == action {
			return i
		}
	}
	return -1
}

func traceActions(traces []ReasoningTrace) []string {
	out := make([]string, 0, len(traces))
	for _, tr := range traces {
		out = append(out, tr.Action)
	}
	return out
}

func traceData(t *testing.T, tr ReasoningTrace) map[string]any {
	t.Helper()
	data, ok := tr.Data.(map[string]any)
	if !ok {
		t.Fatalf("trace %q data = %T, want map[string]any", tr.Action, tr.Data)
	}
	return data
}

// ─── N-A2: macro_flow trace position ────────────────────────────────────

// TestExecuteWithContext_MacroFlowTraceIsRecordedAfterApplyControl is the
// regression test for issue #1944 N-A2: the macro_flow reasoning trace used to be
// recorded BEFORE ApplyControl ran, so it announced an application before the
// adjustment had touched a single conviction.
func TestExecuteWithContext_MacroFlowTraceIsRecordedAfterApplyControl(t *testing.T) {
	scratch := NewScratchpad("macro-trace-order", t.TempDir())
	execCtx := ExecutionContext{
		SessionID:         "macro-trace-order",
		Registry:          macroTraceRegistry(),
		Quotes:            charterTestQuotes(),
		Plugins:           NewPluginRegistry(),
		MacroFlow:         DefaultMacroFlowStrategy{engine: macroflow.NewEngine(0)},
		MacroDataSnapshot: macroTraceSnapshot(),
		Scratchpad:        scratch,
	}

	result := ExecuteWithContext(execCtx)
	if result.MacroFlowAdjustment == nil {
		t.Fatal("expected a macro-flow adjustment to be computed")
	}

	traces := scratch.Traces()
	croIdx := traceIndex(traces, "apply_cro_filter")
	macroIdx := traceIndex(traces, "macro_flow.applied")
	if croIdx < 0 {
		t.Fatalf("expected an apply_cro_filter trace from the control layer, got %v", traceActions(traces))
	}
	if macroIdx < 0 {
		t.Fatalf("expected a macro_flow.applied trace, got %v", traceActions(traces))
	}
	if macroIdx < croIdx {
		t.Fatalf("macro_flow.applied recorded at index %d, before apply_cro_filter at index %d — "+
			"the trace must be recorded after the control layer applied the adjustment (traces: %v)",
			macroIdx, croIdx, traceActions(traces))
	}
	if got := traceData(t, traces[macroIdx])["applied"]; got != true {
		t.Errorf("applied = %v, want true (control layer ran with RequireCROPass=true)", got)
	}
	if !strings.Contains(traces[macroIdx].Reasoning, "macro_flow applied") {
		t.Errorf("reasoning = %q, want it to state the adjustment was applied", traces[macroIdx].Reasoning)
	}
}

// TestExecuteWithContext_MacroFlowTraceHonestWhenControlLayerBypassed: with
// RequireCROPass=false the control layer returns before
// applyMacroConvictionScaling, so the adjustment is never applied and the trace
// must say so instead of claiming "applied".
func TestExecuteWithContext_MacroFlowTraceHonestWhenControlLayerBypassed(t *testing.T) {
	scratch := NewScratchpad("macro-trace-skipped", t.TempDir())
	execCtx := ExecutionContext{
		SessionID: "macro-trace-skipped",
		Registry:  macroTraceRegistry(),
		Quotes:    charterTestQuotes(),
		Plugins:   NewPluginRegistry(),
		// Non-zero policy so the pipeline does not substitute the default policy.
		Policy:            domain.ExecutionPolicy{ConvictionFloor: 50, RequireCROPass: false},
		MacroFlow:         DefaultMacroFlowStrategy{engine: macroflow.NewEngine(0)},
		MacroDataSnapshot: macroTraceSnapshot(),
		Scratchpad:        scratch,
	}

	result := ExecuteWithContext(execCtx)
	if result.MacroFlowAdjustment == nil {
		t.Fatal("expected a macro-flow adjustment to be computed")
	}

	traces := scratch.Traces()
	if idx := traceIndex(traces, "macro_flow.applied"); idx >= 0 {
		t.Fatalf("control layer bypassed, so nothing may claim macro_flow.applied; got %v", traceActions(traces))
	}
	idx := traceIndex(traces, "macro_flow.skipped")
	if idx < 0 {
		t.Fatalf("expected an honest macro_flow.skipped trace, got %v", traceActions(traces))
	}
	data := traceData(t, traces[idx])
	if got := data["applied"]; got != false {
		t.Errorf("applied = %v, want false", got)
	}
	reason, _ := data["not_applied_reason"].(string)
	if !strings.Contains(reason, "RequireCROPass") {
		t.Errorf("not_applied_reason = %q, want it to name the bypassed control layer", reason)
	}
	if !strings.Contains(traces[idx].Reasoning, "NOT applied") {
		t.Errorf("reasoning = %q, want it to state the adjustment was not applied", traces[idx].Reasoning)
	}
}

// TestMacroAdjustmentAppliesTo covers the shared predicate that both the control
// layer and the trace rely on.
func TestMacroAdjustmentAppliesTo(t *testing.T) {
	adj := &macroflow.AdjustmentResult{RiskLevel: macroflow.RiskRed}
	recs := []domain.Recommendation{{Symbol: "2317.TW"}}
	croOn := domain.ExecutionPolicy{RequireCROPass: true}
	croOff := domain.ExecutionPolicy{RequireCROPass: false}

	cases := []struct {
		name     string
		adj      *macroflow.AdjustmentResult
		recs     []domain.Recommendation
		policy   domain.ExecutionPolicy
		want     bool
		wantSkip string
	}{
		{"applied", adj, recs, croOn, true, ""},
		{"no adjustment", nil, recs, croOn, false, "no macro-flow adjustment was computed"},
		{"control layer bypassed", adj, recs, croOff, false, "control layer bypassed (policy.RequireCROPass=false)"},
		{"nothing to scale", adj, nil, croOn, false, "no recommendations to scale"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := macroAdjustmentAppliesTo(tc.adj, tc.recs, tc.policy); got != tc.want {
				t.Errorf("macroAdjustmentAppliesTo = %v, want %v", got, tc.want)
			}
			if got := macroAdjustmentSkipReason(tc.adj, tc.recs, tc.policy); got != tc.wantSkip {
				t.Errorf("macroAdjustmentSkipReason = %q, want %q", got, tc.wantSkip)
			}
		})
	}
}
