package marketdata

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/constants"
)

func TestParseTWDVolume(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1,234,567", 1234567},
		{"  -890  ", -890},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := parseTWDVolume(tt.input)
		if got != tt.want {
			t.Fatalf("parseTWDVolume(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestNewTWSECapitalFlowProvider(t *testing.T) {
	p := NewTWSECapitalFlowProvider(constants.StateCapitalFlow)
	if p.Name() != "twse_capital_flow" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}
