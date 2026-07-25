package apigateway

import (
	"testing"
)

func TestUSMarketChannels(t *testing.T) {
	channels := USMarketChannels()
	if len(channels) != 8 {
		t.Errorf("len(USMarketChannels) = %d, want 8", len(channels))
	}
	expected := map[string]bool{
		"us_spx":    true,
		"us_ndx":    true,
		"us_dji":    true,
		"sox_index": true,
		"us_nvda":   true,
		"us_aapl":   true,
		"us_msft":   true,
		"tsm_adr":   true,
	}
	for _, ch := range channels {
		if !expected[ch] {
			t.Errorf("unexpected channel %q in USMarketChannels", ch)
		}
		delete(expected, ch)
	}
	if len(expected) > 0 {
		t.Errorf("missing channels: %v", expected)
	}
}
