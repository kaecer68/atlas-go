package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// US tech stock adapters (NVDA, AAPL, MSFT) hit Yahoo Finance v8 chart API.
// The 3 tech channels (plus tsm_adr) share yahooTechLimiter (1 req/1.5s) so
// they parallelize with the macro and index groups during us_market_refresh.

// USNVDAChannelAdapter adapts NVDAProvider to the DataProvider interface.
type USNVDAChannelAdapter struct {
	provider *marketdata.NVDAProvider
	limiter  *rate.Limiter
}

func NewUSNVDAChannelAdapter(p *marketdata.NVDAProvider) *USNVDAChannelAdapter {
	return &USNVDAChannelAdapter{
		provider: p,
		limiter:  yahooTechLimiter,
	}
}

func (a *USNVDAChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("us_nvda marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "us_nvda",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *USNVDAChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *USNVDAChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *USNVDAChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID: "us_nvda", Country: "美國", Platform: "Yahoo Finance",
		APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true,
	}
}

// USAAPLChannelAdapter adapts AAPLProvider to the DataProvider interface.
type USAAPLChannelAdapter struct {
	provider *marketdata.AAPLProvider
	limiter  *rate.Limiter
}

func NewUSAAPLChannelAdapter(p *marketdata.AAPLProvider) *USAAPLChannelAdapter {
	return &USAAPLChannelAdapter{
		provider: p,
		limiter:  yahooTechLimiter,
	}
}

func (a *USAAPLChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("us_aapl marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "us_aapl", LatencyMs: time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *USAAPLChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *USAAPLChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *USAAPLChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID: "us_aapl", Country: "美國", Platform: "Yahoo Finance",
		APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true,
	}
}

// USMSFTChannelAdapter adapts MSFTProvider to the DataProvider interface.
type USMSFTChannelAdapter struct {
	provider *marketdata.MSFTProvider
	limiter  *rate.Limiter
}

func NewUSMSFTChannelAdapter(p *marketdata.MSFTProvider) *USMSFTChannelAdapter {
	return &USMSFTChannelAdapter{
		provider: p,
		limiter:  yahooTechLimiter,
	}
}

func (a *USMSFTChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("us_msft marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "us_msft",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *USMSFTChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *USMSFTChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *USMSFTChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID: "us_msft", Country: "美國", Platform: "Yahoo Finance",
		APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true,
	}
}
