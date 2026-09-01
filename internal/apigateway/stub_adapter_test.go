package apigateway

// TestG01G02Channels_LiveLifecycle guards the live lifecycle of the two
// previously-stub channels (twse_sbl=G02, tdcc_equity_dispersion=G01).
// Wiring landed 2026-09-01 (FinMind TaiwanDailyShortSaleBalances /
// TaiwanStockHoldingSharesPer): before any fetch HealthCheck must be
// "unknown" (never "inactive"/"ok"), and after a recorded success it must
// be "ok". The provider-level mapping and state transitions are covered by
// internal/marketdata/g01_g02_wiring_test.go. Regression intent inherited
// from the old stub test: the dashboard must never silently lie about data
// freshness in either direction.

import (
	"context"
	"testing"
)

func TestG01G02Channels_LiveLifecycle(t *testing.T) {
	cases := []struct {
		name string
		adp  DataProvider
	}{
		{"twse_sbl", NewTWSESBLChannelAdapter()},
		{"tdcc_equity_dispersion", NewTDCClientChannelAdapter()},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			meta := c.adp.Metadata()
			if meta.Stub {
				t.Errorf("%s: Metadata().Stub = true, want false (channel is live since G01/G02 wiring)", c.name)
			}

			hs, err := c.adp.HealthCheck(context.Background())
			if err != nil {
				t.Fatalf("HealthCheck returned error: %v", err)
			}
			if hs.Status != "unknown" {
				t.Errorf("%s: HealthCheck.Status = %q, want %q before the first fetch completes", c.name, hs.Status, "unknown")
			}
			if hs.LastError == "" {
				t.Errorf("%s: HealthCheck.LastError must explain the unknown state (no fetch yet)", c.name)
			}
		})
	}
}
