package sectorallocation

import "sync"

// ---- Registered industry hit-rate provider -------------------------------
//
// The consumption chain reads through a port (IndustryHitRateProvider) so the
// stockpicker-backed aggregate can be injected at the composition root without
// sectorallocation importing stockpicker (see industry_hitrate_consume.go).
//
// Registering is unconditional and harmless while the config gate is off: every
// consumer checks the gate before touching the provider. Unregistering is the
// rollback path (tests use it between cases).

var (
	hitRateProviderMu sync.RWMutex
	hitRateProvider   IndustryHitRateProvider
)

// RegisterIndustryHitRateProvider installs the production provider. A second
// call replaces the previous one so a re-wired composition root (or a test
// fixture) cannot leave a stale pointer behind.
func RegisterIndustryHitRateProvider(p IndustryHitRateProvider) {
	hitRateProviderMu.Lock()
	defer hitRateProviderMu.Unlock()
	hitRateProvider = p
}

// ResetIndustryHitRateProvider clears the registered provider. Tests use it
// between cases; it is also the operator-facing rollback if a wiring change
// needs to be backed out without a config reload.
func ResetIndustryHitRateProvider() {
	hitRateProviderMu.Lock()
	defer hitRateProviderMu.Unlock()
	hitRateProvider = nil
}

// GetRegisteredIndustryHitRateProvider returns the registered provider, or nil
// when none is wired.
func GetRegisteredIndustryHitRateProvider() IndustryHitRateProvider {
	hitRateProviderMu.RLock()
	defer hitRateProviderMu.RUnlock()
	return hitRateProvider
}
