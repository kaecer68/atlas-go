package risk

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestRiskGate_PreTradeCheckAllow(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	order := OrderIntent{Symbol: "2330", Side: "BUY", Notional: 50_000, Sector: "semiconductor"}
	pf := PortfolioState{
		TotalValue:     1_000_000,
		Cash:           200_000,
		SectorExposure: map[string]float64{"semiconductor": 200_000},
		Positions:      map[string]float64{},
		Var95:          -10_000,
	}

	dec, err := g.PreTradeCheck(context.Background(), order, pf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictAllow {
		t.Errorf("expected ALLOW, got %s (reason: %s)", dec.Verdict, dec.Reason)
	}
}

func TestRiskGate_PreTradeCheckBlocked(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	order := OrderIntent{Symbol: "2330", Side: "BUY", Notional: 300_000, Sector: "semiconductor"}
	pf := PortfolioState{
		TotalValue:     1_000_000,
		Cash:           500_000,
		SectorExposure: map[string]float64{"semiconductor": 300_000},
		Positions:      map[string]float64{"2330": 100_000},
		Var95:          -10_000,
	}
	// 100k existing + 300k new = 400k / 1M = 40% > 15% max position

	dec, err := g.PreTradeCheck(context.Background(), order, pf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK, got %s", dec.Verdict)
	}
}

func TestRiskGate_SuspendedModeBlocksAll(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	g.SetMode(ModeSuspended)

	order := OrderIntent{Symbol: "2330", Side: "BUY", Notional: 1000}
	pf := PortfolioState{TotalValue: 1_000_000, Cash: 500_000}

	dec, err := g.PreTradeCheck(context.Background(), order, pf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK in SUSPENDED mode, got %s", dec.Verdict)
	}
	if dec.Mode != string(ModeSuspended) {
		t.Errorf("expected SUSPENDED mode in decision, got %s", dec.Mode)
	}
}

func TestRiskGate_ModeTransition(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())

	if g.Mode() != ModeNormal {
		t.Errorf("initial mode should be NORMAL, got %s", g.Mode())
	}

	g.SetMode(ModeCautious)
	if g.Mode() != ModeCautious {
		t.Errorf("after SetMode(CAUTIOUS), mode = %s, want CAUTIOUS", g.Mode())
	}

	g.SetMode(ModeDefensive)
	if g.Mode() != ModeDefensive {
		t.Errorf("after SetMode(DEFENSIVE), mode = %s, want DEFENSIVE", g.Mode())
	}

	g.SetMode(ModeSuspended)
	if g.Mode() != ModeSuspended {
		t.Errorf("after SetMode(SUSPENDED), mode = %s, want SUSPENDED", g.Mode())
	}
}

func TestRiskGate_Subscribe(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())

	received := make(chan RiskDecision, 1)
	g.Subscribe(func(d RiskDecision) {
		received <- d
	})

	g.SetMode(ModeCautious)

	select {
	case d := <-received:
		if d.Phase != PhasePostTrade {
			t.Errorf("expected PostTrade phase for mode change, got %s", d.Phase)
		}
		if d.Mode != string(ModeCautious) {
			t.Errorf("expected CAUTIOUS mode, got %s", d.Mode)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for mode change event")
	}
}

func TestRiskGate_InTradeCheckNotInitialized(t *testing.T) {
	g := &RiskGate{} // nil inTrade

	_, err := g.InTradeCheck(context.Background(), nil, 0, 0, 0)
	if err == nil {
		t.Fatal("expected error for uninitialized inTrade gate")
	}
}

func TestRiskGate_PreTradeCheckNotInitialized(t *testing.T) {
	g := &RiskGate{} // nil preTrade

	_, err := g.PreTradeCheck(context.Background(), OrderIntent{}, PortfolioState{})
	if err == nil {
		t.Fatal("expected error for uninitialized preTrade gate")
	}
}

func TestRiskGate_PostTradeCheckNotInitialized(t *testing.T) {
	g := &RiskGate{} // nil postTrade

	_, err := g.PostTradeCheck(context.Background(), PostTradeInput{})
	if err == nil {
		t.Fatal("expected error for uninitialized postTrade gate")
	}
}

func TestRiskGate_PreTradeCheckDefensiveMode(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	g.SetMode(ModeDefensive)

	// In DEFENSIVE mode, action.TargetPct > 0.5 should be capped to 0.5
	// But since all rules pass, the action won't be set.
	// This tests that PreTradeCheck works in DEFENSIVE mode without error.
	order := OrderIntent{Symbol: "2330", Side: "BUY", Notional: 10_000, Sector: "semiconductor"}
	pf := PortfolioState{TotalValue: 1_000_000, Cash: 500_000, Positions: map[string]float64{}, Var95: -5_000}

	dec, err := g.PreTradeCheck(context.Background(), order, pf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = dec
}

func TestRiskGate_NewRiskGateConfig(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	if g == nil {
		t.Fatal("NewRiskGate returned nil")
	}
	if g.preTrade == nil {
		t.Error("preTrade gate is nil")
	}
	if g.inTrade == nil {
		t.Error("inTrade gate is nil")
	}
	if g.postTrade == nil {
		t.Error("postTrade gate is nil")
	}
}

func TestRiskGate_PostTradeCheckModeChange(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())

	// Trigger drawdown > 20% → should switch to SUSPENDED
	input := PostTradeInput{
		CurrentDrawdownPct: 0.25,
		RollingSharpe:      0.5,
		ConsecutiveLosses:  0,
	}

	dec, err := g.PostTradeCheck(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Mode != string(ModeSuspended) {
		t.Errorf("expected mode SUSPENDED, got %s", dec.Mode)
	}

	// Gate mode should also be updated
	if g.Mode() != ModeSuspended {
		t.Errorf("gate mode should be SUSPENDED after PostTradeCheck, got %s", g.Mode())
	}
}

func TestRiskGate_SetPreTradeRSITwScore(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	g.SetPreTradeRSITwScore(0.75)

	order := OrderIntent{Symbol: "2330", Side: "BUY", Notional: 10_000, Sector: "semiconductor"}
	pf := PortfolioState{TotalValue: 1_000_000, Cash: 500_000, Positions: map[string]float64{}, Var95: -5_000}

	dec, err := g.PreTradeCheck(context.Background(), order, pf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictBlock {
		t.Errorf("expected BLOCK for extreme RSI-tw score, got %s", dec.Verdict)
	}

	found := false
	for _, d := range dec.Details {
		if d.RuleName == "retail_sentiment" && !d.Passed {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected retail_sentiment rule to fail")
	}
}

func TestRiskGate_SetPreTradeRSITwScore_Reduce(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	g.SetPreTradeRSITwScore(0.55)

	order := OrderIntent{Symbol: "2330", Side: "BUY", Notional: 10_000, Sector: "semiconductor"}
	pf := PortfolioState{TotalValue: 1_000_000, Cash: 500_000, Positions: map[string]float64{}, Var95: -5_000}

	dec, err := g.PreTradeCheck(context.Background(), order, pf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec.Verdict != VerdictReduce {
		t.Errorf("expected REDUCE for elevated RSI-tw score, got %s", dec.Verdict)
	}
}

func TestRiskGate_PublishWritesConfidenceCommentary(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	defer func() { ConfidenceCommentary = nil }()

	ConfidenceCommentary = func(ctx context.Context, decision any) (string, error) {
		if _, ok := decision.(RiskDecision); !ok {
			t.Errorf("unexpected decision type: %T", decision)
		}
		return "VaR at 1.8x limit; recommend defensive posture", nil
	}

	received := make(chan RiskDecision, 1)
	g.Subscribe(func(d RiskDecision) {
		received <- d
	})

	g.SetMode(ModeDefensive)

	select {
	case d := <-received:
		if d.ConfidenceCommentary != "VaR at 1.8x limit; recommend defensive posture" {
			t.Errorf("expected subscriber to receive populated ConfidenceCommentary, got %q", d.ConfidenceCommentary)
		}
		if d.Mode != string(ModeDefensive) {
			t.Errorf("expected mode DEFENSIVE, got %s", d.Mode)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for mode change event")
	}
}

func TestRiskGate_PublishConfidenceCommentaryNilHook(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	defer func() { ConfidenceCommentary = nil }()
	ConfidenceCommentary = nil

	received := make(chan RiskDecision, 1)
	g.Subscribe(func(d RiskDecision) {
		received <- d
	})

	g.SetMode(ModeCautious)

	select {
	case d := <-received:
		if d.ConfidenceCommentary != "" {
			t.Errorf("expected empty ConfidenceCommentary with nil hook, got %q", d.ConfidenceCommentary)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for mode change event")
	}
}

func TestRiskGate_WithMaturityTracker(t *testing.T) {
	g := NewRiskGate(NewPreTradeGate(), NewInTradeGate(), NewPostTradeGate())
	mt := domain.NewMaturityTrackerWithStart(time.Now().UTC())
	g.WithMaturityTracker(mt)

	order := OrderIntent{Symbol: "2330", Side: "BUY", Notional: 10_000, Sector: "semiconductor"}
	pf := PortfolioState{TotalValue: 1_000_000, Cash: 500_000, Positions: map[string]float64{}, Var95: -5_000}

	dec, err := g.PreTradeCheck(context.Background(), order, pf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, d := range dec.Details {
		if d.RuleName == "var_limit" && d.Passed && strings.Contains(d.Message, "warming") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected VaR warming message during burn-in maturity")
	}
}
