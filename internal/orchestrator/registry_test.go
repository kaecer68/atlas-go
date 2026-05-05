package orchestrator

import (
	"context"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/screener"
)

func TestSeedRegistryIsValid(t *testing.T) {
	reg := SeedRegistry()
	if err := ValidateRegistry(reg, ""); err != nil {
		t.Fatalf("registry validation failed: %v", err)
	}
	if len(reg.Agents) < 5 {
		t.Fatalf("expected multiple seeded agents")
	}
}

func TestPluginRegistryScreenDetailed(t *testing.T) {
	r := NewPluginRegistry().WithScreener(screener.NewEngine(nil, nil))
	minVol := int64(1000000)
	agent := domain.AgentSpec{
		ID: "test-agent",
		ScreeningCriteria: domain.ScreeningCriteria{
			VolumeIntraday: &domain.MinFilter{Min: &minVol},
		},
	}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Volume: 500000, IsTradable: true}}
	res, err := r.ScreenDetailed(context.Background(), agent, "2330.TW", quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Passed {
		t.Fatal("expected fail")
	}
	if res.Criterion == "" {
		t.Fatal("expected criterion")
	}
}

func TestPluginRegistryScreenDetailedNoScreener(t *testing.T) {
	r := NewPluginRegistry()
	agent := domain.AgentSpec{
		ID: "test-agent",
		ScreeningCriteria: domain.ScreeningCriteria{
			VolumeIntraday: &domain.MinFilter{Min: int64Ptr(1000000)},
		},
	}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Volume: 500000, IsTradable: true}}
	res, err := r.ScreenDetailed(context.Background(), agent, "2330.TW", quotes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Passed {
		t.Fatal("expected pass when no screener attached")
	}
}

func int64Ptr(i int64) *int64 {
	return &i
}
