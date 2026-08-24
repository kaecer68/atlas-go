package marketdata

import (
	"testing"

	"golang.org/x/time/rate"
)

// withUnlimitedTWSELimiter replaces the shared TWSE token bucket (P1-13)
// with an unlimited one for the duration of a test. Mock-server tests that
// make several requests would otherwise be paced by the 0.6 req/s policy
// bucket and run for many seconds.
func withUnlimitedTWSELimiter(t *testing.T) {
	t.Helper()
	old := SetTWSESharedLimiterForTest(rate.NewLimiter(rate.Inf, 1))
	t.Cleanup(func() { SetTWSESharedLimiterForTest(old) })
}
