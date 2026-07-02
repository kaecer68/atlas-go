package orchestrator

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/macroflow"
)

func TestRegimeToRiskLevel(t *testing.T) {
	cases := []struct {
		name string
		in   domain.Regime
		want macroflow.RiskLevel
	}{
		{"risk_off_to_red", domain.RegimeRiskOff, macroflow.RiskRed},
		{"risk_on_to_yellow", domain.RegimeRiskOn, macroflow.RiskYellow},
		{"neutral_to_yellow", domain.RegimeNeutral, macroflow.RiskYellow},
		{"empty_defaults_to_yellow", "", macroflow.RiskYellow},
		{"unknown_defaults_to_yellow", "nonsense", macroflow.RiskYellow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := regimeToRiskLevel(tc.in)
			if got != tc.want {
				t.Errorf("regimeToRiskLevel(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
