package marketdata

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func buildTWIIChartJSON(timestamp int64, closes []float64) []byte {
	var sb strings.Builder
	sb.WriteString(`{"chart":{"result":[{"meta":{"regularMarketTime":`)
	sb.WriteString(fmt.Sprintf("%d", timestamp))
	sb.WriteString(`},"indicators":{"quote":[{"close":[`)
	for i, c := range closes {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%g", c))
	}
	sb.WriteString(`]}]}}]}}`)
	return []byte(sb.String())
}

func TestTaiwanVolatilityProvider_FetchSnapshot_StaleCacheRejects(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	// Fix "now" to a weekday (Wednesday 2026-07-29) so the freshness expectation is stable.
	fixedNow := time.Date(2026, 7, 29, 9, 0, 0, 0, twseLocation)
	origNowFunc := twiiNowFunc
	twiiNowFunc = func() time.Time { return fixedNow }
	defer func() { twiiNowFunc = origNowFunc }()

	// Cache data timestamped two days earlier (Monday 2026-07-27).
	staleTs := time.Date(2026, 7, 27, 13, 30, 0, 0, twseLocation).Unix()
	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = 23000.0 + float64(i)*10
	}
	twiiCache.set(buildTWIIChartJSON(staleTs, closes), "1d", "3mo")

	p := NewTaiwanVolatilityProvider()
	_, err := p.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error for stale cache, got nil")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Fatalf("expected stale error, got %v", err)
	}
}

func TestTaiwanVolatilityProvider_FetchSnapshot_FreshCacheAccepts(t *testing.T) {
	twiiCache.reset()
	defer twiiCache.reset()

	fixedNow := time.Date(2026, 7, 29, 9, 0, 0, 0, twseLocation)
	origNowFunc := twiiNowFunc
	twiiNowFunc = func() time.Time { return fixedNow }
	defer func() { twiiNowFunc = origNowFunc }()

	freshTs := fixedNow.Unix()
	closes := make([]float64, 25)
	for i := range closes {
		closes[i] = 23000.0 + float64(i)*10
	}
	twiiCache.set(buildTWIIChartJSON(freshTs, closes), "1d", "3mo")

	p := NewTaiwanVolatilityProvider()
	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.HistoricalVolatility.Symbol != "^TWII" {
		t.Errorf("Symbol = %q, want ^TWII", snap.HistoricalVolatility.Symbol)
	}
	if snap.HistoricalVolatility.Value != closes[len(closes)-1] {
		t.Errorf("Value = %v, want %v", snap.HistoricalVolatility.Value, closes[len(closes)-1])
	}
	if snap.HistoricalVolatility.ChangePct <= 0 || math.IsNaN(snap.HistoricalVolatility.ChangePct) {
		t.Errorf("expected positive volatility, got %v", snap.HistoricalVolatility.ChangePct)
	}
}
