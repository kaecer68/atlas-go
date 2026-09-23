package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TWSEMarginChannelAdapter adapts a TWSEMarginBalanceProvider to the DataProvider interface.
type TWSEMarginChannelAdapter struct {
	provider *marketdata.TWSEMarginBalanceProvider
	limiter  *rate.Limiter
}

// NewTWSEMarginChannelAdapter creates a new adapter for the TWSE margin channel.
func NewTWSEMarginChannelAdapter(provider *marketdata.TWSEMarginBalanceProvider) *TWSEMarginChannelAdapter {
	return &TWSEMarginChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(TWSEMarginRate, TWSEMarginBurst),
	}
}

// SetFinMindClient wires the (shared) FinMind client that fills
// margin_maintenance_ratio. Pass nil to disable the fill.
//
// The fill itself lives in TWSEMarginBalanceProvider (it used to sit here as
// an adapter-only post-step, which left the macro-ingest cron snapshot without
// the ratio); this method forwards so gateway wiring keeps working.
func (a *TWSEMarginChannelAdapter) SetFinMindClient(c *marketdata.FinMindClient) {
	a.provider.SetFinMindClient(c)
}

// Fetch retrieves the latest margin balance snapshot.
func (a *TWSEMarginChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		// Non-trading days or holidays: TWSE returns no data for the past 7 days,
		// which is expected behavior. Return a stale result instead of an error
		// to avoid triggering the circuit breaker.
		// P1-9: typed no-data classification (previously string matching).
		if errors.Is(err, marketdata.ErrNoData) {
			return &FetchResult{Stale: true, Meta: FetchMetadata{ChannelID: "twse_margin", Timestamp: time.Now()}}, nil
		}
		return nil, fmt.Errorf("margin fetch: %w", err)
	}
	// margin_maintenance_ratio is already filled inside the provider when a
	// FinMind client is wired (see SetFinMindClient above); no adapter-side
	// post-step is needed.

	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("margin marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "twse_margin",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *TWSEMarginChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}, nil
}

// RateLimit returns the TWSE margin rate limiter.
func (a *TWSEMarginChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TWSE margin balance.
func (a *TWSEMarginChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_margin",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "json",
		Path:       "www.twse.com.tw/rwd/zh/marginTrading",
		HasLimiter: true,
	}
}
