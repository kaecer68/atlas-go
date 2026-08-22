package config

import "testing"

func TestLoad_CharterModeFlag(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("ATLAS_CHARTER_MODE", "")
	if cfg := Load(); cfg.CharterMode {
		t.Error("ATLAS_CHARTER_MODE should default to false (Phase C2 opt-in)")
	}
	t.Setenv("ATLAS_CHARTER_MODE", "true")
	if cfg := Load(); !cfg.CharterMode {
		t.Error("ATLAS_CHARTER_MODE=true should enable CharterMode")
	}
	t.Setenv("ATLAS_CHARTER_MODE", "false")
	if cfg := Load(); cfg.CharterMode {
		t.Error("ATLAS_CHARTER_MODE=false should disable CharterMode")
	}
}
