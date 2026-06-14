package risk

import (
	"testing"
)

func TestVerdictLevel_Allow(t *testing.T) {
	if VerdictAllow.Level() != LevelAllow {
		t.Errorf("VerdictAllow.Level() = %v, want LevelAllow (%v)", VerdictAllow.Level(), LevelAllow)
	}
}

func TestVerdictLevel_AlertOnly(t *testing.T) {
	if VerdictAlertOnly.Level() != LevelAlertOnly {
		t.Errorf("VerdictAlertOnly.Level() = %v, want LevelAlertOnly", VerdictAlertOnly.Level())
	}
}

func TestVerdictLevel_Reduce(t *testing.T) {
	if VerdictReduce.Level() != LevelReduce {
		t.Errorf("VerdictReduce.Level() = %v, want LevelReduce", VerdictReduce.Level())
	}
}

func TestVerdictLevel_Block(t *testing.T) {
	if VerdictBlock.Level() != LevelBlock {
		t.Errorf("VerdictBlock.Level() = %v, want LevelBlock", VerdictBlock.Level())
	}
}

func TestVerdictLevel_Halt(t *testing.T) {
	if VerdictHalt.Level() != LevelHalt {
		t.Errorf("VerdictHalt.Level() = %v, want LevelHalt", VerdictHalt.Level())
	}
}

func TestVerdictLevel_Unknown(t *testing.T) {
	v := Verdict("UNKNOWN_TYPE")
	if v.Level() != LevelAllow {
		t.Errorf("unknown verdict.Level() = %v, want LevelAllow (default)", v.Level())
	}
}

func TestVerdictLevel_Ordering(t *testing.T) {
	// VerdictLevel values should be in ascending severity order
	if LevelAllow >= LevelAlertOnly {
		t.Error("LevelAllow should be less than LevelAlertOnly")
	}
	if LevelAlertOnly >= LevelReduce {
		t.Error("LevelAlertOnly should be less than LevelReduce")
	}
	if LevelReduce >= LevelBlock {
		t.Error("LevelReduce should be less than LevelBlock")
	}
	if LevelBlock >= LevelHalt {
		t.Error("LevelBlock should be less than LevelHalt")
	}
}

func TestRiskPhaseConstants(t *testing.T) {
	if PhasePreTrade != "pre_trade" {
		t.Errorf("PhasePreTrade = %q, want pre_trade", PhasePreTrade)
	}
	if PhaseInTrade != "in_trade" {
		t.Errorf("PhaseInTrade = %q, want in_trade", PhaseInTrade)
	}
	if PhasePostTrade != "post_trade" {
		t.Errorf("PhasePostTrade = %q, want post_trade", PhasePostTrade)
	}
}

func TestActionTypeConstants(t *testing.T) {
	if ActionSell != "SELL" {
		t.Errorf("ActionSell = %q, want SELL", ActionSell)
	}
	if ActionReduce != "REDUCE" {
		t.Errorf("ActionReduce = %q, want REDUCE", ActionReduce)
	}
	if ActionFreeze != "FREEZE" {
		t.Errorf("ActionFreeze = %q, want FREEZE", ActionFreeze)
	}
	if ActionLiquidate != "LIQUIDATE" {
		t.Errorf("ActionLiquidate = %q, want LIQUIDATE", ActionLiquidate)
	}
	if ActionNotify != "NOTIFY" {
		t.Errorf("ActionNotify = %q, want NOTIFY", ActionNotify)
	}
}
