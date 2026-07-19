package apigateway

import (
	"testing"

	"golang.org/x/time/rate"
)

func TestNewRateLimitManager_AllChannels(t *testing.T) {
	m := NewRateLimitManager()

	expectedChannels := map[string]bool{
		"us_yahoo":               false,
		"frankfurter_fx":         false,
		"twse_replay":            false,
		"twse_capital_flow":      false,
		"fugle":                  false,
		"fubon":                  false,
		"finmind":                false,
		"geopolitical":           false,
		"geopolitical_taiwan":    false,
		"twse_margin":            false,
		"export_statistics":      false,
		"tsmc_revenue":           false,
		"janus_regime":           false,
		"tej":                    false,
		"exchange_rate":          false,
		"sox_index":              false,
		"dram_spot_price":        false,
		"twse_sector_index":      false,
		"sector_data":            false,
		"day_trading":            false,
		"twse_etf":               false,
		"taifex_daily":           false,
		"taifex_institutional":   false,
		"twse_oddlot":            false,
		"bdi":                    false,
		"us_spx":                 false,
		"us_ndx":                 false,
		"us_dji":                 false,
		"taiex_index":            false,
		"us_nvda":                false,
		"us_aapl":                false,
		"us_msft":                false,
		"tsm_adr":                false,
		"twse_sbl":               false, // G02
		"tdcc_equity_dispersion": false, // G01
	}

	for id := range m.limiters {
		if _, exists := expectedChannels[id]; exists {
			expectedChannels[id] = true
		} else {
			t.Errorf("unexpected channel: %s", id)
		}
	}

	if len(expectedChannels) != len(m.limiters) {
		t.Errorf("channel count mismatch: expected %d, got %d", len(expectedChannels), len(m.limiters))
	}

	for channel, found := range expectedChannels {
		if !found {
			t.Errorf("missing expected channel: %s", channel)
		}
	}
}

func TestNewRateLimitManager_InfiniteRateChannels(t *testing.T) {
	m := NewRateLimitManager()

	infiniteChannels := []string{"twse_replay", "janus_regime", "sector_data"}
	for _, channel := range infiniteChannels {
		t.Run(channel, func(t *testing.T) {
			l, err := m.Get(channel)
			if err != nil {
				t.Fatalf("unexpected error getting %s: %v", channel, err)
			}
			if l.Limit() != rate.Inf {
				t.Errorf("channel %s should have infinite rate, got %v", channel, l.Limit())
			}
		})
	}
}

func TestNewRateLimitManager_NonInfiniteRateChannels(t *testing.T) {
	m := NewRateLimitManager()

	nonInfiniteChannels := []string{
		"us_yahoo", "frankfurter_fx", "twse_capital_flow", "fugle", "fubon",
		"finmind", "geopolitical", "geopolitical_taiwan", "twse_margin",
		"export_statistics", "tsmc_revenue", "tej", "exchange_rate",
		"sox_index", "dram_spot_price", "twse_sector_index", "day_trading", "twse_etf", "taifex_daily", "twse_oddlot", "bdi",
		"us_spx", "us_ndx", "us_dji", "us_nvda", "us_aapl", "us_msft", "tsm_adr",
	}
	for _, channel := range nonInfiniteChannels {
		t.Run(channel, func(t *testing.T) {
			l, err := m.Get(channel)
			if err != nil {
				t.Fatalf("unexpected error getting %s: %v", channel, err)
			}
			if l.Limit() == rate.Inf {
				t.Errorf("channel %s should NOT have infinite rate", channel)
			}
		})
	}
}

func TestNewRateLimitManager_IndependentLimiters(t *testing.T) {
	m := NewRateLimitManager()

	usLimiter, err := m.Get("us_yahoo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fxLimiter, err := m.Get("frankfurter_fx")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// frankfurter_fx hits api.frankfurter.app, NOT Yahoo Finance.
	// It must have its own independent limiter — no longer sharing with us_yahoo.
	if usLimiter == fxLimiter {
		t.Error("us_yahoo and frankfurter_fx should have independent limiters (different API endpoints)")
	}

	if usLimiter != yahooMacroLimiter {
		t.Error("us_yahoo should use the yahooMacroLimiter instance")
	}

	if fxLimiter == yahooMacroLimiter || fxLimiter == yahooIndexLimiter || fxLimiter == yahooTechLimiter {
		t.Error("frankfurter_fx should NOT use any Yahoo group limiter (hits Frankfurter API)")
	}
}

// TestNewRateLimitManager_YahooGroupSplit asserts the 3-way grouping:
//
//	macro: us_yahoo                          → yahooMacroLimiter
//	index: us_spx / us_ndx / us_dji          → yahooIndexLimiter
//	tech:  us_nvda / us_aapl / us_msft / tsm_adr → yahooTechLimiter
//
// Within a group the channels share a limiter; across groups they must not.
func TestNewRateLimitManager_YahooGroupSplit(t *testing.T) {
	m := NewRateLimitManager()

	macroChans := []string{"us_yahoo"}
	indexChans := []string{"us_spx", "us_ndx", "us_dji"}
	techChans := []string{"us_nvda", "us_aapl", "us_msft", "tsm_adr"}

	groupExpectations := []struct {
		name     string
		channels []string
		want     *rate.Limiter
	}{
		{"macro", macroChans, yahooMacroLimiter},
		{"index", indexChans, yahooIndexLimiter},
		{"tech", techChans, yahooTechLimiter},
	}
	for _, g := range groupExpectations {
		for _, ch := range g.channels {
			got, err := m.Get(ch)
			if err != nil {
				t.Fatalf("group %s: unexpected error for %s: %v", g.name, ch, err)
			}
			if got != g.want {
				t.Errorf("group %s: channel %s should use %s instance, got different pointer", g.name, ch, g.name)
			}
		}
	}

	groups := []*rate.Limiter{yahooMacroLimiter, yahooIndexLimiter, yahooTechLimiter}
	for i := 0; i < len(groups); i++ {
		for j := i + 1; j < len(groups); j++ {
			if groups[i] == groups[j] {
				t.Errorf("group limiters %d and %d should be distinct instances", i, j)
			}
		}
	}
}

// TestNewRateLimitManager_YahooGroupsDrainIndependently verifies that
// draining one Yahoo group does not block traffic on a different group.
func TestNewRateLimitManager_YahooGroupsDrainIndependently(t *testing.T) {
	m := NewRateLimitManager()

	indexLim, _ := m.Get("us_spx")
	techLim, _ := m.Get("us_nvda")
	macroLim, _ := m.Get("us_yahoo")

	_ = indexLim.Allow()

	if !techLim.Allow() {
		t.Error("tech group should be unaffected by index group drain (independent limiters)")
	}
	if !macroLim.Allow() {
		t.Error("macro group should be unaffected by index group drain (independent limiters)")
	}
}

func TestNewRateLimitManager_IndependentLimitersDrainIndependently(t *testing.T) {
	m := NewRateLimitManager()

	usLimiter, _ := m.Get("us_yahoo")
	fxLimiter, _ := m.Get("frankfurter_fx")

	// Draining us_yahoo should NOT affect frankfurter_fx (independent limiters).
	_ = usLimiter.Allow() // consume the single burst token
	if !fxLimiter.Allow() {
		t.Error("frankfurter_fx should NOT be affected by us_yahoo drain (independent limiters)")
	}
}

func TestGet_ValidChannel(t *testing.T) {
	m := NewRateLimitManager()

	l, err := m.Get("twse_capital_flow")
	if err != nil {
		t.Fatalf("expected no error for valid channel, got: %v", err)
	}
	if l == nil {
		t.Fatal("expected non-nil limiter for valid channel")
	}
}

func TestGet_InvalidChannel(t *testing.T) {
	m := NewRateLimitManager()

	l, err := m.Get("nonexistent_channel")
	if err == nil {
		t.Error("expected error for unknown channel, got nil")
	}
	if l != nil {
		t.Error("expected nil limiter for unknown channel")
	}
}

func TestGet_EmptyChannel(t *testing.T) {
	m := NewRateLimitManager()

	l, err := m.Get("")
	if err == nil {
		t.Error("expected error for empty channel ID")
	}
	if l != nil {
		t.Error("expected nil limiter for empty channel ID")
	}
}

func TestRegister_NewChannel(t *testing.T) {
	m := NewRateLimitManager()

	newLimiter := rate.NewLimiter(1, 3)
	m.Register("test_channel", newLimiter)

	l, err := m.Get("test_channel")
	if err != nil {
		t.Fatalf("expected registered channel to be retrievable, got: %v", err)
	}
	if l != newLimiter {
		t.Error("Get should return the same limiter instance that was registered")
	}
}

func TestRegister_ReplaceExistingChannel(t *testing.T) {
	m := NewRateLimitManager()

	oldLimiter, _ := m.Get("fugle")

	newLimiter := rate.NewLimiter(100, 50)
	m.Register("fugle", newLimiter)

	l, err := m.Get("fugle")
	if err != nil {
		t.Fatalf("expected replaced channel to be retrievable, got: %v", err)
	}
	if l == oldLimiter {
		t.Error("registered limiter should replace the old one")
	}
	if l != newLimiter {
		t.Error("Get should return the new limiter instance after replacement")
	}
}

func TestRegister_PreservesOtherChannels(t *testing.T) {
	m := NewRateLimitManager()
	origStatus := m.Status()
	origCount := len(origStatus)

	newLimiter := rate.NewLimiter(1, 3)
	m.Register("test_channel", newLimiter)

	newStatus := m.Status()
	if len(newStatus) != origCount+1 {
		t.Errorf("Register should add to limiters, expected %d channels, got %d", origCount+1, len(newStatus))
	}

	// Verify all original channels still exist
	for id := range origStatus {
		if _, ok := newStatus[id]; !ok {
			t.Errorf("original channel %s should still exist after registering new channel", id)
		}
	}
}

func TestRemaining_ValidChannel(t *testing.T) {
	m := NewRateLimitManager()

	tokens, err := m.Remaining("fugle")
	if err != nil {
		t.Fatalf("expected no error for valid channel, got: %v", err)
	}
	if tokens < 0 {
		t.Errorf("expected non-negative tokens, got %f", tokens)
	}
	// Fugle burst is 1, so tokens should be <= burst
	if tokens > float64(FugleBasicBurst) {
		t.Errorf("tokens should not exceed burst, got %f > %d", tokens, FugleBasicBurst)
	}
}

func TestRemaining_InvalidChannel(t *testing.T) {
	m := NewRateLimitManager()

	tokens, err := m.Remaining("nonexistent_channel")
	if err == nil {
		t.Error("expected error for unknown channel")
	}
	if tokens != 0 {
		t.Errorf("expected 0 tokens for unknown channel, got %f", tokens)
	}
}

func TestStatus_AllChannelsPresent(t *testing.T) {
	m := NewRateLimitManager()

	status := m.Status()
	if len(status) != len(m.limiters) {
		t.Errorf("Status should return %d entries, got %d", len(m.limiters), len(status))
	}

	for id := range m.limiters {
		if s, ok := status[id]; !ok {
			t.Errorf("Status missing channel: %s", id)
		} else if s.ChannelID != id {
			t.Errorf("Status for %s has wrong ChannelID: %s", id, s.ChannelID)
		}
	}
}

func TestStatus_FieldsAreSet(t *testing.T) {
	m := NewRateLimitManager()
	status := m.Status()

	for id, s := range status {
		t.Run(id, func(t *testing.T) {
			if s.ChannelID == "" {
				t.Error("ChannelID should not be empty")
			}
			if s.ChannelID != id {
				t.Errorf("ChannelID mismatch: status key=%s, field=%s", id, s.ChannelID)
			}
			// Remaining should be a non-negative number for all channels
			if s.Remaining < 0 {
				t.Errorf("Remaining should not be negative for channel %s, got %f", id, s.Remaining)
			}
			if s.Burst < 0 {
				t.Errorf("Burst should not be negative for channel %s", id)
			}
		})
	}
}

func TestStatus_InfiniteChannelsHaveZeroBurst(t *testing.T) {
	m := NewRateLimitManager()
	status := m.Status()

	infiniteChannels := []string{"twse_replay", "janus_regime", "sector_data"}
	for _, channel := range infiniteChannels {
		s, ok := status[channel]
		if !ok {
			t.Errorf("Status missing channel: %s", channel)
			continue
		}
		if s.Burst != 0 {
			t.Errorf("channel %s should have burst=0 (infinite rate), got %d", channel, s.Burst)
		}
		// With infinite rate and burst=0, remaining is 0 (no tokens to accumulate).
		// The channel can always consume instantly via rate.Inf.
	}
}

func TestStatus_BurstMatchesConfig(t *testing.T) {
	m := NewRateLimitManager()
	status := m.Status()

	tests := []struct {
		channel string
		burst   int
	}{
		{"us_yahoo", YahooFinanceBurst},
		{"frankfurter_fx", FrankfurterFXBurst},
		{"twse_replay", 0},
		{"twse_capital_flow", TWSECapitalFlowBurst},
		{"fugle", FugleBasicBurst},
		{"fubon", FugleBasicBurst},
		{"finmind", FinMindFreeBurst},
		{"geopolitical", GeopoliticalBurst},
		{"geopolitical_taiwan", GeopoliticalBurst},
		{"twse_margin", TWSEMarginBurst},
		{"export_statistics", ExportStatisticsBurst},
		{"tsmc_revenue", FinMindFreeBurst},
		{"janus_regime", 0},
		{"tej", TEJBurst},
		{"exchange_rate", ExportStatisticsBurst},
		{"sox_index", ExportStatisticsBurst},
		{"sector_data", 0},
		{"day_trading", TWSEMarginBurst},
		{"dram_spot_price", ExportStatisticsBurst},
		{"twse_sector_index", ExportStatisticsBurst},
		{"bdi", ExportStatisticsBurst},
		{"us_spx", YahooIndexBurst},
		{"us_ndx", YahooIndexBurst},
		{"us_dji", YahooIndexBurst},
		{"us_nvda", YahooTechBurst},
		{"us_aapl", YahooTechBurst},
		{"us_msft", YahooTechBurst},
		{"tsm_adr", YahooTechBurst},
	}

	for _, tt := range tests {
		t.Run(tt.channel, func(t *testing.T) {
			s, ok := status[tt.channel]
			if !ok {
				t.Fatalf("Status missing channel: %s", tt.channel)
			}
			if s.Burst != tt.burst {
				t.Errorf("channel %s: expected burst %d, got %d", tt.channel, tt.burst, s.Burst)
			}
		})
	}
}

func TestStatus_ReturnsFreshMap(t *testing.T) {
	m := NewRateLimitManager()

	status1 := m.Status()
	status2 := m.Status()

	if len(status1) == 0 || len(status2) == 0 {
		t.Fatal("Status should not return empty maps")
	}

	// Mutate first map, verify second is unaffected
	status1["__fake__"] = RateLimitStatus{}
	if _, ok := status2["__fake__"]; ok {
		t.Error("Status should return a fresh copy, mutations should not affect later calls")
	}
}

func TestRateLimitStatus_Struct(t *testing.T) {
	s := RateLimitStatus{
		ChannelID: "test_channel",
		Remaining: 3.5,
		Burst:     5,
	}

	if s.ChannelID != "test_channel" {
		t.Errorf("ChannelID = %s, want test_channel", s.ChannelID)
	}
	if s.Remaining != 3.5 {
		t.Errorf("Remaining = %f, want 3.5", s.Remaining)
	}
	if s.Burst != 5 {
		t.Errorf("Burst = %d, want 5", s.Burst)
	}
}

func TestRateLimitStatus_ZeroValue(t *testing.T) {
	var s RateLimitStatus

	if s.ChannelID != "" {
		t.Errorf("zero ChannelID should be empty, got %s", s.ChannelID)
	}
	if s.Remaining != 0 {
		t.Errorf("zero Remaining should be 0, got %f", s.Remaining)
	}
	if s.Burst != 0 {
		t.Errorf("zero Burst should be 0, got %d", s.Burst)
	}
}

func TestRemaining_ReturnsApproximateTokens(t *testing.T) {
	m := NewRateLimitManager()

	// For a fresh limiter with burst=5, tokens should be a float value
	tokens, err := m.Remaining("twse_capital_flow")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be approximately equal to burst (1.0), may be slightly different due to time
	if tokens < 0 || tokens > float64(TWSECapitalFlowBurst)+0.01 {
		t.Errorf("tokens out of range for fresh limiter: %f (burst=%d)", tokens, TWSECapitalFlowBurst)
	}
}
