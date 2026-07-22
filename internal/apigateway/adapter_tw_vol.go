package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TaiwanVolatilityChannelAdapter adapts marketdata.TaiwanVolatilityProvider to the
// gateway DataProvider interface for the `tw_vol` channel.
//
// Source: Yahoo Finance ^TWII (TAIEX) daily bars, 3-month range.
// Output: MacroDataSnapshot with HistoricalVolatility set:
//   - Value     = latest ^TWII close price
//   - ChangePct = annualized 20-day log-return volatility (NOT daily change;
//     see internal/marketdata/taiwan_volatility_provider.go for the
//     mis-shared ChangePct semantic — consumers reading ChangePct
//     from a MacroDataSnapshot must be aware that this provider is
//     the only source where ChangePct carries volatility, not
//     daily % change). Strategy technique evaluator reads .Value
//     (latest close) and is safe.
type TaiwanVolatilityChannelAdapter struct {
	provider *marketdata.TaiwanVolatilityProvider
	limiter  *rate.Limiter
}

// NewTaiwanVolatilityChannelAdapter creates a new adapter for the tw_vol channel.
func NewTaiwanVolatilityChannelAdapter(p *marketdata.TaiwanVolatilityProvider) *TaiwanVolatilityChannelAdapter {
	return &TaiwanVolatilityChannelAdapter{
		provider: p,
		// 1 req / 5s, burst 1 — shares the Yahoo Finance etiquette used by
		// other Yahoo-backed channels (ExportStatisticsRate in limits.go).
		// The provider internally also calls yahooSharedLimiter.Wait, so
		// requests are throttled both here and inside the provider.
		limiter: rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Fetch retrieves the latest ^TWII snapshot (price + 20d volatility).
func (a *TaiwanVolatilityChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("tw_vol rate limit: %w", err)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("tw_vol marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "tw_vol",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

// HealthCheck probes the upstream Yahoo fetch; an error is returned when
// the provider cannot pull enough bars (>=21) to compute volatility_20d.
func (a *TaiwanVolatilityChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "liveness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "liveness",
	}, nil
}

// RateLimit returns the channel-level rate limiter (5s/request, burst 1).
// The provider also waits on the shared Yahoo session limiter; both apply.
func (a *TaiwanVolatilityChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

// Metadata returns static channel metadata for tw_vol.
func (a *TaiwanVolatilityChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "tw_vol",
		Country:    "台灣",
		Platform:   "Yahoo Finance",
		APIFormat:  "REST JSON",
		Path:       "query1.finance.yahoo.com",
		HasLimiter: true,
	}
}
