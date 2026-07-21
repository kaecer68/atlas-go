package apigateway

import (
	"context"
	"strings"
	"testing"
)

// TestStubChannels_SelfIdentify verifies that the two known stub
// channels (twse_sbl, tdcc_equity_dispersion) are self-marked via
// Metadata().Stub=true and HealthCheck=inactive, so the alerting
// path in monitoring/service/data_channels.go can skip them and the
// dashboard can render them as "not yet live" without false-positive
// alerts.
//
// Regression guard: if a future change reverts HealthCheck to "ok"
// on a stub channel, the dashboard will silently lie about data
// freshness. This test exists to make that mistake loud.
func TestStubChannels_SelfIdentify(t *testing.T) {
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
			if !meta.Stub {
				t.Errorf("%s: Metadata().Stub = false, want true (otherwise dashboard misreports it as live)", c.name)
			}

			hs, err := c.adp.HealthCheck(context.Background())
			if err != nil {
				t.Fatalf("HealthCheck returned error: %v", err)
			}
			if hs.Status != "inactive" {
				t.Errorf("%s: HealthCheck.Status = %q, want \"inactive\" (otherwise alerting fires on a stub)", c.name, hs.Status)
			}
			if hs.LastError == "" {
				t.Errorf("%s: HealthCheck.LastError must explain why the channel is inactive (pointers to G01/G02)", c.name)
			}
			// Last error must reference the G01/G02 gate so an
			// operator reading the dashboard can find the
			// implementation tracking issue.
			want := map[string]string{
				"twse_sbl":               "G02",
				"tdcc_equity_dispersion": "G01",
			}[c.name]
			if !strings.Contains(hs.LastError, want) {
				t.Errorf("%s: HealthCheck.LastError %q must mention gate %q so operators can find the tracking issue", c.name, hs.LastError, want)
			}
		})
	}
}
