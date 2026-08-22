package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/charter"
	"github.com/kaecer68/atlas-go/internal/config"
)

// TestWithCharterMode_PhaseACompat verifies that a System built with
// CharterMode=false and no WithCharterMode keeps a nil charter (Phase A),
// and that WithCharterMode with zero options does not wire anything.
func TestWithCharterMode_PhaseACompat(t *testing.T) {
	sys, err := NewSystem(config.Config{ReplayDataPath: config.GetReplayDataPath("..")})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	if sys.charter != nil {
		t.Fatal("Phase A system must keep charter nil")
	}
	sys.WithCharterMode(charter.Options{}) // zero options → no-op
	if sys.charter != nil {
		t.Fatal("WithCharterMode(zero options) must not wire charter")
	}
}

// TestWithCharterMode_GlobalFlagAllOn verifies C2 compatibility: with
// CharterMode=true and no WithCharterMode, all five switches are on.
func TestWithCharterMode_GlobalFlagAllOn(t *testing.T) {
	sys, err := NewSystem(config.Config{ReplayDataPath: config.GetReplayDataPath(".."), CharterMode: true})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	if sys.charter == nil {
		t.Fatal("CharterMode=true must wire charter")
	}
	if !sys.charter.options.Enabled() {
		t.Fatal("CharterMode=true without options must enable all switches")
	}
	if len(sys.charter.options.Names()) != 5 {
		t.Errorf("CharterMode=true options = %v, want 5 switches (AllOn)", sys.charter.options.Names())
	}
}

// TestWithCharterMode_PerArmSwitches verifies the C3 per-arm path: a System
// built with CharterMode=false wires charter lazily via WithCharterMode, and
// only the requested switches are active.
func TestWithCharterMode_PerArmSwitches(t *testing.T) {
	sys, err := NewSystem(config.Config{ReplayDataPath: config.GetReplayDataPath("..")})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	arm2 := charter.Options{PeriodOnly: true, StrategyFilter: true}
	sys.WithCharterMode(arm2)
	if sys.charter == nil {
		t.Fatal("WithCharterMode(arm2) must wire charter")
	}
	if !sys.charter.options.PeriodOnly || !sys.charter.options.StrategyFilter {
		t.Error("arm2 must enable PeriodOnly + StrategyFilter")
	}
	if sys.charter.options.MacroFlow || sys.charter.options.CashReserve || sys.charter.options.ConvictionFloor {
		t.Error("arm2 must NOT enable MacroFlow/CashReserve/ConvictionFloor")
	}
}

// TestWithCharterMode_MergeKeepsGlobalBehavior verifies merging semantics:
// WithCharterMode on an already-wired (cfg.CharterMode=true) system keeps
// existing switches and unions the new ones.
func TestWithCharterMode_MergeKeepsGlobalBehavior(t *testing.T) {
	sys, err := NewSystem(config.Config{ReplayDataPath: config.GetReplayDataPath(".."), CharterMode: true})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	sys.WithCharterMode(charter.Options{PeriodOnly: true})
	if !sys.charter.options.Enabled() {
		t.Error("merged options must stay enabled")
	}
	if len(sys.charter.options.Names()) != 5 {
		t.Errorf("merged options = %v, want 5 switches preserved", sys.charter.options.Names())
	}
}

// TestWithCharterMode_ExecutionContextWiring verifies selective context
// wiring: a PeriodOnly system injects the period detector but not the
// strategy filter or macroflow strategy.
func TestWithCharterMode_ExecutionContextWiring(t *testing.T) {
	sys, err := NewSystem(config.Config{
		ReplayDataPath: config.GetReplayDataPath(".."),
		LedgerDir:      t.TempDir(),
		ReplayMode:     "daily",
	})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	sys.WithCharterMode(charter.Options{PeriodOnly: true})
	ctx := sys.buildExecutionContext(nil, nil)
	if ctx.PeriodDetector == nil {
		t.Error("PeriodOnly must wire PeriodDetector")
	}
	if ctx.MacroFlow != nil {
		t.Error("PeriodOnly must NOT wire MacroFlow")
	}
	if ctx.PeriodStrategyFilter != nil {
		t.Error("PeriodOnly must NOT wire PeriodStrategyFilter")
	}

	// Full arm: everything wired.
	full, err := NewSystem(config.Config{ReplayDataPath: config.GetReplayDataPath(".."), LedgerDir: t.TempDir(), ReplayMode: "daily"})
	if err != nil {
		t.Fatalf("NewSystem: %v", err)
	}
	full.WithCharterMode(charter.AllOn())
	fctx := full.buildExecutionContext(nil, nil)
	if fctx.PeriodDetector == nil || fctx.MacroFlow == nil || fctx.PeriodStrategyFilter == nil {
		t.Error("AllOn must wire PeriodDetector + MacroFlow + PeriodStrategyFilter")
	}
}
