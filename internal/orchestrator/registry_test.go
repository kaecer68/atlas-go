package orchestrator

import "testing"

func TestSeedRegistryIsValid(t *testing.T) {
	reg := SeedRegistry()
	if err := ValidateRegistry(reg); err != nil {
		t.Fatalf("registry validation failed: %v", err)
	}
	if len(reg.Agents) < 5 {
		t.Fatalf("expected multiple seeded agents")
	}
}
