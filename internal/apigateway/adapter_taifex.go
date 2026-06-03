package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// taifexDailyData is the combined payload marshaled by the adapter.
type taifexDailyData struct {
	PCR             *marketdata.PCRStats        `json:"pcr"`
	RetailFuturesOI *marketdata.RetailFuturesOI `json:"retail_futures_oi"`
}

// TaifexChannelAdapter adapts the TAIFEX provider to the DataProvider interface.
type TaifexChannelAdapter struct {
	provider *marketdata.TAIFEXProvider
	limiter  *rate.Limiter
}

// NewTaifexChannelAdapter creates a new adapter for the TAIFEX daily channel.
func NewTaifexChannelAdapter() *TaifexChannelAdapter {
	return &TaifexChannelAdapter{
		provider: marketdata.NewTAIFEXProvider(),
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Fetch retrieves both PCR and retail futures OI from TAIFEX.
func (a *TaifexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()

	if err := a.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}

	pcr, err := a.provider.FetchPCR(ctx)
	if err != nil {
		return nil, fmt.Errorf("taifex pcr: %w", err)
	}

	oi, err := a.provider.FetchRetailFuturesOI(ctx)
	if err != nil {
		return nil, fmt.Errorf("taifex retail oi: %w", err)
	}

	data, err := json.Marshal(taifexDailyData{
		PCR:             pcr,
		RetailFuturesOI: oi,
	})
	if err != nil {
		return nil, fmt.Errorf("taifex marshal: %w", err)
	}

	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "taifex_daily",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching PCR data.
func (a *TaifexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchPCR(ctx)
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

// RateLimit returns the TAIFEX daily rate limiter.
func (a *TaifexChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TAIFEX daily data.
func (a *TaifexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "taifex_daily",
		Country:    "台灣",
		Platform:   "TAIFEX 期交所",
		APIFormat:  "REST JSON",
		Path:       "openapi.taifex.com.tw/v1/PutCallRatio",
		HasLimiter: true,
	}
}
