package orchestrator

import "testing"

func TestHashString_Deterministic(t *testing.T) {
	h1 := hashString("test")
	h2 := hashString("test")

	if h1 != h2 {
		t.Errorf("hashString should be deterministic, got %d and %d", h1, h2)
	}
}

func TestHashString_DifferentSymbols(t *testing.T) {
	h1 := hashString("2330")
	h2 := hashString("2317")

	if h1 == h2 {
		t.Errorf("different symbols should produce different hashes, got both %d", h1)
	}
}
