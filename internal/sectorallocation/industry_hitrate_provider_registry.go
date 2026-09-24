package sectorallocation

import "sync"

// Registered provider management for the industry hit-rate consumption chain.
// Composition roots in cmd/atlas wire the stocktools-backed provider here so
// the sectorallocation-side wiring (PR-β) has a path to actual data. When no
// provider is registered, the assessment decorator and ApplyIndustryHitRateToDrivers
// return gracefully (no evidence, no driver tilts) - the gate is still ON but
// the wiring is incomplete.

var (
	hitRateProviderMu sync.RWMutex
	hitRateProvider   IndustryHitRateProvider
)

// RegisterIndustryHitRateProvider installs the production provider (typically
// backed by stocktools.SQLiteWinRateProvider). Safe to call from init().
// Idempotency note: a second call replaces the first (composition roots may
// re-wire during hot-reload / test setup); the gate check happens inside
// every consumer function, so a stale pointer only matters if the new
// provider is also nil.
func RegisterIndustryHitRateProvider(p IndustryHitRateProvider) {
	hitRateProviderMu.Lock()
	defer hitRateProviderMu.Unlock()
	hitRateProvider = p
}

// ResetIndustryHitRateProvider clears the registered provider. Tests use
// this between cases; production should never call it.
func ResetIndustryHitRateProvider() {
	hitRateProviderMu.Lock()
	defer hitRateProviderMu.Unlock()
	hitRateProvider = nil
}

// GetRegisteredIndustryHitRateProvider returns the currently registered
// provider (nil if none). Pure accessor.
func GetRegisteredIndustryHitRateProvider() IndustryHitRateProvider {
	hitRateProviderMu.RLock()
	defer hitRateProviderMu.RUnlock()
	return hitRateProvider
}
