package orchestrator

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
)

// Issue #1944 Batch 4, item 4. ATLAS_MARKET_DATA_PROVIDER=fubon was documented as
// a valid value in configs/allowed_env_vars.md while selectProvider() had no fubon
// branch, so the setting silently produced a hybrid provider (TWSE/FinMind/Fugle)
// with no signal at all. The fubon channel only exists as the Python
// services/fubon-proxy FastAPI service; no Go marketdata.Provider wraps it, so the
// honest fix is to drop it from the documented value set and make the fallback
// explicit rather than to invent a provider in this lane.

func TestSupportedMarketDataProvidersMatchesSelectProvider(t *testing.T) {
	got := SupportedMarketDataProviders()
	want := []string{"twse", "fugle", "hybrid"}
	if !slices.Equal(got, want) {
		t.Fatalf("SupportedMarketDataProviders() = %v, want %v", got, want)
	}
	// A clone, not the backing array: a caller must not be able to widen the set.
	got[0] = "mutated"
	if SupportedMarketDataProviders()[0] != "twse" {
		t.Fatalf("SupportedMarketDataProviders() leaks its backing array")
	}
	for _, value := range want {
		if !IsSupportedMarketDataProvider(value) {
			t.Fatalf("%q must be reported as supported", value)
		}
	}
	// The empty string selects the hybrid default branch.
	if !IsSupportedMarketDataProvider("") {
		t.Fatalf("empty value must be supported (hybrid default)")
	}
	for _, value := range []string{"fubon", "yahoo", "mock", "FUGLE"} {
		if IsSupportedMarketDataProvider(value) {
			t.Fatalf("%q must NOT be reported as supported", value)
		}
	}
}

// TestSelectProvider_FubonFallsBackToHybridNotMock pins the behaviour an operator
// gets today: the value is unsupported, so the result is a hybrid provider (never
// the mock provider, which would silently return fabricated quotes).
func TestSelectProvider_FubonFallsBackToHybridNotMock(t *testing.T) {
	cfg := config.Config{MarketDataProvider: "fubon"}
	p := selectProvider(cfg)
	if p == nil {
		t.Fatalf("selectProvider returned nil")
	}
	if m, ok := p.(interface{ IsMock() bool }); ok && m.IsMock() {
		t.Fatalf("fubon must not resolve to the mock provider")
	}
	if name := p.Name(); !strings.Contains(name, "hybrid") {
		t.Fatalf("fubon resolved to %q, want a hybrid provider", name)
	}
}

// TestAllowedEnvVarsDocDoesNotClaimFubon is the anti-drift tripwire for the exact
// defect: the documented value list must match SupportedMarketDataProviders().
func TestAllowedEnvVarsDocDoesNotClaimFubon(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "configs", "allowed_env_vars.md"))
	if err != nil {
		t.Fatalf("read allowed_env_vars.md: %v", err)
	}
	var row string
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.Contains(line, "`ATLAS_MARKET_DATA_PROVIDER`") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("configs/allowed_env_vars.md no longer documents ATLAS_MARKET_DATA_PROVIDER")
	}
	// fubon may be *mentioned* (it is exactly the value an operator must not use),
	// but it must never appear as a backticked value in the list.
	if strings.Contains(row, "`fubon`") {
		t.Fatalf("documented value list still claims fubon, which selectProvider() has no branch for:\n%s", row)
	}
	if !strings.Contains(row, "`twse`/`fugle`/`hybrid`") {
		t.Fatalf("documented value list must spell out the supported set in order:\n%s", row)
	}
	for _, value := range SupportedMarketDataProviders() {
		if !strings.Contains(row, "`"+value+"`") {
			t.Fatalf("documented row is missing supported value %q:\n%s", value, row)
		}
	}
}
