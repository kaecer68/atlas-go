package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// US index adapters (S&P 500, Nasdaq Composite, Dow Jones) hit Yahoo Finance
// v8 chart API. The 3 index channels share yahooIndexLimiter (1 req/1.5s) so
// they parallelize with the macro and tech groups during us_market_refresh.

// USSPXIndexChannelAdapter adapts SPXIndexProvider to the DataProvider interface.
type USSPXIndexChannelAdapter struct {
	provider *marketdata.SPXIndexProvider
	limiter  *rate.Limiter
}

func NewUSSPXIndexChannelAdapter(p *marketdata.SPXIndexProvider) *USSPXIndexChannelAdapter {
	return &USSPXIndexChannelAdapter{
		provider: p,
		limiter:  yahooIndexLimiter,
	}
}

func (a *USSPXIndexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("us_spx marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "us_spx",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *USSPXIndexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *USSPXIndexChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *USSPXIndexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID: "us_spx", Country: "美國", Platform: "Yahoo Finance",
		APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true,
	}
}

// USNDXIndexChannelAdapter adapts NDXIndexProvider to the DataProvider interface.
type USNDXIndexChannelAdapter struct {
	provider *marketdata.NDXIndexProvider
	limiter  *rate.Limiter
}

func NewUSNDXIndexChannelAdapter(p *marketdata.NDXIndexProvider) *USNDXIndexChannelAdapter {
	return &USNDXIndexChannelAdapter{
		provider: p,
		limiter:  yahooIndexLimiter,
	}
}

func (a *USNDXIndexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("us_ndx marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "us_ndx", LatencyMs: time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *USNDXIndexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *USNDXIndexChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *USNDXIndexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID: "us_ndx", Country: "美國", Platform: "Yahoo Finance",
		APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true,
	}
}

// USDJIIndexChannelAdapter adapts DJIIndexProvider to the DataProvider interface.
type USDJIIndexChannelAdapter struct {
	provider *marketdata.DJIIndexProvider
	limiter  *rate.Limiter
}

func NewUSDJIIndexChannelAdapter(p *marketdata.DJIIndexProvider) *USDJIIndexChannelAdapter {
	return &USDJIIndexChannelAdapter{
		provider: p,
		limiter:  yahooIndexLimiter,
	}
}

func (a *USDJIIndexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("us_dji marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "us_dji",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *USDJIIndexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *USDJIIndexChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *USDJIIndexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID: "us_dji", Country: "美國", Platform: "Yahoo Finance",
		APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true,
	}
}
