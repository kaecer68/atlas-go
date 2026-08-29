package marketdata

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// ─── TSMADRPremium ──────────────────────────────────────────────────────────

func TestTSMADRPremium_NormalCase(t *testing.T) {
	// ADR: $200 USD, TSMC.TW: NT$1000, USD/TWD: 31, ratio: 5
	// Equivalent: 200 * 31 / 5 = 1240
	// Premium: (1240 - 1000) / 1000 * 100 = 24%
	premium := TSMADRPremium(200, 1000, 31, 5)
	want := 24.0
	if math.Abs(premium-want) > 0.001 {
		t.Fatalf("premium = %f, want %f", premium, want)
	}
}

func TestTSMADRPremium_ZeroSharesPer(t *testing.T) {
	// sharesPer=0 should default to 5
	premium := TSMADRPremium(200, 1000, 31, 0)
	want := 24.0
	if math.Abs(premium-want) > 0.001 {
		t.Fatalf("premium = %f, want %f (default ratio 5)", premium, want)
	}
}

func TestTSMADRPremium_NegativeSharesPer(t *testing.T) {
	premium := TSMADRPremium(200, 1000, 31, -1)
	want := 24.0
	if math.Abs(premium-want) > 0.001 {
		t.Fatalf("premium = %f, want %f (default ratio 5)", premium, want)
	}
}

func TestTSMADRPremium_ZeroTsmcTWD(t *testing.T) {
	premium := TSMADRPremium(200, 0, 31, 5)
	if premium != 0 {
		t.Fatalf("premium = %f, want 0 (zero TSMC price)", premium)
	}
}

func TestTSMADRPremium_NegativeTsmcTWD(t *testing.T) {
	premium := TSMADRPremium(200, -100, 31, 5)
	if premium != 0 {
		t.Fatalf("premium = %f, want 0 (negative TSMC price)", premium)
	}
}

func TestTSMADRPremium_Discount(t *testing.T) {
	// ADR: $100, TSMC.TW: NT$700, USD/TWD: 30, ratio: 5
	// Equivalent: 100 * 30 / 5 = 600
	// Premium: (600 - 700) / 700 * 100 = -14.29%
	premium := TSMADRPremium(100, 700, 30, 5)
	if premium >= 0 {
		t.Fatalf("premium = %f, want negative (discount)", premium)
	}
}

// ─── DailyQuotaTracker ─────────────────────────────────────────────────────

func TestDailyQuotaTracker_AllowCall_DecrementsRemaining(t *testing.T) {
	dir := t.TempDir()
	tracker := NewDailyQuotaTracker("test", dir, 5)

	for i := range 5 {
		if !tracker.AllowCall() {
			t.Fatalf("AllowCall() #%d should be allowed", i+1)
		}
	}
	if tracker.AllowCall() {
		t.Fatal("AllowCall() should be denied after quota exhausted")
	}
	if got := tracker.Remaining(); got != 0 {
		t.Fatalf("Remaining() = %d, want 0", got)
	}
}

func TestDailyQuotaTracker_LoadPersistedState(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "test_daily_quota.json")

	// Pre-create state file with calls already made today.
	today := time.Now().Truncate(24 * time.Hour)
	stateData := `{"calls_today":3,"last_reset":"` + today.Format(time.RFC3339) + `"}`
	if err := os.WriteFile(stateFile, []byte(stateData), 0o644); err != nil {
		t.Fatal(err)
	}

	tracker := NewDailyQuotaTracker("test", dir, 10)
	if got := tracker.CallsToday(); got != 3 {
		t.Fatalf("CallsToday() = %d, want 3 (loaded from state)", got)
	}
	if got := tracker.Remaining(); got != 7 {
		t.Fatalf("Remaining() = %d, want 7", got)
	}
}

func TestDailyQuotaTracker_LoadOldState_ResetsDay(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "test_daily_quota.json")

	// State from yesterday should be ignored.
	yesterday := time.Now().Add(-24 * time.Hour).Truncate(24 * time.Hour)
	stateData := `{"calls_today":8,"last_reset":"` + yesterday.Format(time.RFC3339) + `"}`
	if err := os.WriteFile(stateFile, []byte(stateData), 0o644); err != nil {
		t.Fatal(err)
	}

	tracker := NewDailyQuotaTracker("test", dir, 10)
	if got := tracker.CallsToday(); got != 0 {
		t.Fatalf("CallsToday() = %d, want 0 (yesterday state ignored)", got)
	}
}

func TestDailyQuotaTracker_Remaining_ClampsToZero(t *testing.T) {
	dir := t.TempDir()
	tracker := NewDailyQuotaTracker("test", dir, 10)
	tracker.SetLimit(5)
	// Manually exhaust quota by calling AllowCall 10 times but limit set to 5.
	// Actually SetLimit doesn't reset callsToday, so if we had calls > limit, Remaining clamps.
	// We simulate by calling AllowCall to exhaustion, then reducing limit.
	for range 5 {
		tracker.AllowCall()
	}
	tracker.SetLimit(3)
	if got := tracker.Remaining(); got != 0 {
		t.Fatalf("Remaining() = %d, want 0 (clamped, callsToday=5 > limit=3)", got)
	}
}

// ─── PollingAdapter ─────────────────────────────────────────────────────────

type stubProvider struct{}

func (s stubProvider) Name() string { return "stub" }
func (s stubProvider) GetQuotes(ctx context.Context, asOf time.Time, syms []string) ([]domain.Quote, error) {
	return []domain.Quote{{Symbol: "2330", Last: 500}}, nil
}

func TestPollingAdapter_Unsubscribe(t *testing.T) {
	adapter := &PollingAdapter{Base: stubProvider{}, Interval: 1}
	if err := adapter.Unsubscribe(context.Background(), nil); err != nil {
		t.Fatalf("Unsubscribe() unexpected error: %v", err)
	}
}

func TestPollingAdapter_Subscribe_FiresCallback(t *testing.T) {
	adapter := &PollingAdapter{Base: stubProvider{}, Interval: 1}
	ctx, cancel := context.WithCancel(context.Background())

	received := make(chan domain.Quote, 5)
	handler := func(q domain.Quote) { received <- q }

	go func() {
		_ = adapter.Subscribe(ctx, []string{"2330"}, handler)
	}()

	// Wait for at least one tick.
	select {
	case q := <-received:
		if q.Symbol != "2330" {
			t.Fatalf("expected 2330, got %s", q.Symbol)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for quote")
	}

	cancel()
}

// ─── mockQuote ──────────────────────────────────────────────────────────────

func TestMockQuote_AdditionalSymbols(t *testing.T) {
	now := time.Now()

	tests := []struct {
		symbol string
		want   float64
	}{
		{"2330", 785},
		{"2330.TW", 785},
		{"2317", 162},
		{"2382", 268},
		{"0050", 192},
		{"0050.TW", 192},
		{"2603", 215},
		{"9999.UNKNOWN", 50},
	}

	for _, tc := range tests {
		q := mockQuote(tc.symbol, now, "mock")
		if q.Last != tc.want {
			t.Errorf("mockQuote(%q).Last = %f, want %f", tc.symbol, q.Last, tc.want)
		}
		if q.Source != "mock" {
			t.Errorf("mockQuote(%q).Source = %q, want %q", tc.symbol, q.Source, "mock")
		}
		if q.Market != "TW" {
			t.Errorf("mockQuote(%q).Market = %q, want %q", tc.symbol, q.Market, "TW")
		}
	}
}

func TestMockQuote_Fields(t *testing.T) {
	q := mockQuote("2330", time.Now(), "test-src")

	if !q.IsTradable {
		t.Error("IsTradable should be true")
	}
	if q.Open <= 0 || q.High <= 0 || q.Low <= 0 {
		t.Error("OHL should be > 0")
	}
	if q.High < q.Low {
		t.Error("High should be >= Low")
	}
	if q.Volume != 15000000 {
		t.Errorf("Volume = %d, want 15000000", q.Volume)
	}
}

// ─── MicrostructureProvider ─────────────────────────────────────────────────

func TestMicrostructureProvider_ZeroAvgVolume(t *testing.T) {
	lookup := func(s string) int64 { return 0 }
	p := NewMicrostructureProvider(lookup)

	q := domain.Quote{Symbol: "2330", Last: 500, High: 510, Low: 490, Volume: 1000000}
	snap := p.Calculate("2330", q)

	if snap.Symbol != "2330" {
		t.Errorf("Symbol = %q, want 2330", snap.Symbol)
	}
}

// ─── Source enum ────────────────────────────────────────────────────────────

func TestSource_String(t *testing.T) {
	tests := []struct {
		s    Source
		want string
	}{
		{SourceTWSE, "twse"},
		{SourceProxy, "proxy"},
		{Source(99), "unknown"},
	}

	for _, tc := range tests {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Source(%d).String() = %q, want %q", tc.s, got, tc.want)
		}
	}
}

// ─── stripCommas ────────────────────────────────────────────────────────────

func TestStripCommas(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"1,940.00", "1940.00"},
		{"123", "123"},
		{"1,234,567", "1234567"},
		{"", ""},
		{"no commas here", "no commas here"},
	}

	for _, tc := range tests {
		if got := stripCommas(tc.input); got != tc.want {
			t.Errorf("stripCommas(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ─── rowAt ──────────────────────────────────────────────────────────────────

func TestRowAt(t *testing.T) {
	row := []string{"a", "b", "c"}

	if got := rowAt(row, 0, "fallback"); got != "a" {
		t.Errorf("rowAt(0) = %q, want a", got)
	}
	if got := rowAt(row, 1, "fallback"); got != "b" {
		t.Errorf("rowAt(1) = %q, want b", got)
	}
	if got := rowAt(row, 3, "fallback"); got != "fallback" {
		t.Errorf("rowAt(3) = %q, want fallback", got)
	}
	if got := rowAt(row, 100, "fallback"); got != "fallback" {
		t.Errorf("rowAt(100) = %q, want fallback", got)
	}
	if got := rowAt(nil, 0, "fallback"); got != "fallback" {
		t.Errorf("rowAt(nil, 0) = %q, want fallback", got)
	}
}

// ─── toString ───────────────────────────────────────────────────────────────

func TestToString(t *testing.T) {
	if got := toString("  hello  "); got != "hello" {
		t.Errorf("toString = %q, want %q", got, "hello")
	}
	if got := toString(42); got != "42" {
		t.Errorf("toString(42) = %q, want 42", got)
	}
	if got := toString(3.14); got != "3.14" {
		t.Errorf("toString(3.14) = %q, want 3.14", got)
	}
}

// ─── parseFloat ─────────────────────────────────────────────────────────────

func TestParseFloat(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"3.14", 3.14},
		{"42", 42},
		{"-10.5", -10.5},
		{"(5.0)", -5.0},
		{"(1234.56)", -1234.56},
		{"0", 0},
	}

	for _, tc := range tests {
		got, err := parseFloat(tc.input)
		if err != nil {
			t.Errorf("parseFloat(%q) unexpected error: %v", tc.input, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseFloat(%q) = %f, want %f", tc.input, got, tc.want)
		}
	}
}

func TestParseFloat_Invalid(t *testing.T) {
	_, err := parseFloat("not-a-number")
	if err == nil {
		t.Fatal("expected error for invalid float")
	}
}

func TestParseFloat_EmptyParens(t *testing.T) {
	// parseFloat strips "()" then Sscanf on "" fails (n==0).
	_, err := parseFloat("()")
	if err == nil {
		t.Fatal("expected error for empty string after stripping parens")
	}
}

// ─── MicrostructureProvider zero-last ──────────────────────────────────────

func TestMicrostructureProvider_ZeroLast(t *testing.T) {
	lookup := func(s string) int64 { return 1000000 }
	p := NewMicrostructureProvider(lookup)

	q := domain.Quote{Symbol: "2330", Last: 0, High: 10, Low: 5, Volume: 1000000}
	snap := p.Calculate("2330", q)

	if snap.RealizedVolatility != 0 {
		t.Errorf("realized vol should be 0 when Last is 0, got %f", snap.RealizedVolatility)
	}
}
