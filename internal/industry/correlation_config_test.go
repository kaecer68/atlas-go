package industry

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

var coreIndustries = []string{
	"semiconductor",
	"ai_supply_chain",
	"electronics",
	"robotics",
	"financials",
	"shipping",
	"energy",
	"foundry",
	"server_assembly",
	"cooling",
}

func TestCorrelationMatrixComplete(t *testing.T) {
	cfg, err := config.LoadParametersConfig("../../configs/parameters.json")
	if err != nil {
		t.Fatalf("failed to load parameters config: %v", err)
	}

	matrix := cfg.Industry.LinkageParams.Value.CorrelationMatrix
	if len(matrix) == 0 {
		t.Fatal("correlation matrix is empty")
	}

	cm := LoadCorrelationMatrixFromConfig(&cfg.Industry.LinkageParams.Value)

	coreSet := make(map[string]bool, len(coreIndustries))
	for _, id := range coreIndustries {
		coreSet[id] = true
	}

	for _, indA := range coreIndustries {
		for _, indB := range coreIndustries {
			if indA >= indB {
				continue
			}
			_, ok := cm.GetCorrelation(indA, indB)
			if !ok {
				t.Errorf("missing correlation entry for %s ↔ %s", indA, indB)
			}
		}
	}
}

func TestCorrelationRange(t *testing.T) {
	cfg, err := config.LoadParametersConfig("../../configs/parameters.json")
	if err != nil {
		t.Fatalf("failed to load parameters config: %v", err)
	}

	matrix := cfg.Industry.LinkageParams.Value.CorrelationMatrix
	if len(matrix) == 0 {
		t.Fatal("correlation matrix is empty")
	}

	for key, value := range matrix {
		if value < -1.0 || value > 1.0 {
			t.Errorf("correlation %s = %f is outside range [-1.0, 1.0]", key, value)
		}
	}
}

func TestCorrelationSymmetry(t *testing.T) {
	cfg, err := config.LoadParametersConfig("../../configs/parameters.json")
	if err != nil {
		t.Fatalf("failed to load parameters config: %v", err)
	}

	matrix := cfg.Industry.LinkageParams.Value.CorrelationMatrix
	if len(matrix) == 0 {
		t.Fatal("correlation matrix is empty")
	}

	cm := LoadCorrelationMatrixFromConfig(&cfg.Industry.LinkageParams.Value)
	all := cm.GetAllCorrelations()

	for indA, peers := range all {
		for indB, corrAB := range peers {
			corrBA, ok := cm.GetCorrelation(indB, indA)
			if !ok {
				t.Errorf("asymmetric: %s → %s exists but %s → %s does not", indA, indB, indB, indA)
				continue
			}
			if corrAB != corrBA {
				t.Errorf("asymmetric correlation: %s↔%s = %f but %s↔%s = %f",
					indA, indB, corrAB, indB, indA, corrBA)
			}
		}
	}
}

func TestCorrelationKeyFormat(t *testing.T) {
	cfg, err := config.LoadParametersConfig("../../configs/parameters.json")
	if err != nil {
		t.Fatalf("failed to load parameters config: %v", err)
	}

	matrix := cfg.Industry.LinkageParams.Value.CorrelationMatrix
	if len(matrix) == 0 {
		t.Fatal("correlation matrix is empty")
	}

	for key := range matrix {
		parts := strings.Split(key, "↔")
		if len(parts) != 2 {
			t.Errorf("invalid correlation key format (missing ↔ separator): %s", key)
			continue
		}
		a, b := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if a == "" || b == "" {
			t.Errorf("empty industry name in correlation key: %s", key)
		}
		if a == b {
			t.Errorf("self-correlation entry found: %s", key)
		}
	}
}

func TestAllSpecifiedPairsPresent(t *testing.T) {
	cfg, err := config.LoadParametersConfig("../../configs/parameters.json")
	if err != nil {
		t.Fatalf("failed to load parameters config: %v", err)
	}

	cm := LoadCorrelationMatrixFromConfig(&cfg.Industry.LinkageParams.Value)

	specifiedPairs := []struct {
		a, b   string
		expect float64
	}{
		{"foundry", "semiconductor", 0.88},
		{"foundry", "ai_supply_chain", 0.82},
		{"foundry", "electronics", 0.60},
		{"foundry", "financials", 0.12},
		{"foundry", "shipping", -0.08},
		{"foundry", "energy", 0.05},
		{"server_assembly", "semiconductor", 0.75},
		{"server_assembly", "ai_supply_chain", 0.80},
		{"server_assembly", "cooling", 0.70},
		{"server_assembly", "electronics", 0.55},
		{"server_assembly", "financials", 0.15},
		{"cooling", "semiconductor", 0.50},
		{"cooling", "ai_supply_chain", 0.60},
		{"cooling", "electronics", 0.45},
		{"cooling", "energy", 0.25},
	}

	for _, p := range specifiedPairs {
		corr, ok := cm.GetCorrelation(p.a, p.b)
		if !ok {
			t.Errorf("missing specified pair %s ↔ %s", p.a, p.b)
			continue
		}
		if fmt.Sprintf("%.2f", corr) != fmt.Sprintf("%.2f", p.expect) {
			t.Errorf("%s ↔ %s: expected %.2f, got %.2f", p.a, p.b, p.expect, corr)
		}
	}
}
