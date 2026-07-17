package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// TaifexInstitutionalAdapter adapts the TAIFEX provider to the 三大法人 期貨 OI channel.
type TaifexInstitutionalAdapter struct {
	provider *marketdata.TAIFEXProvider
	limiter  *rate.Limiter
}

// NewTaifexInstitutionalAdapter creates a new adapter for the TAIFEX
// institutional futures channel.
func NewTaifexInstitutionalAdapter() *TaifexInstitutionalAdapter {
	return &TaifexInstitutionalAdapter{
		provider: marketdata.NewTAIFEXProvider(),
		limiter:  rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// Fetch retrieves 三大法人 期貨 OI from TAIFEX for the latest session.
func (a *TaifexInstitutionalAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	data, err := a.provider.FetchInstitutionalFuturesDaily(ctx)
	if err != nil {
		return nil, fmt.Errorf("taifex institutional: %w", err)
	}
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("taifex institutional marshal: %w", err)
	}
	return &FetchResult{
		Data: payload,
		Meta: FetchMetadata{
			ChannelID:          "taifex_institutional",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching 三大法人 期貨 OI data.
func (a *TaifexInstitutionalAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchInstitutionalFuturesDaily(ctx)
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

// RateLimit returns the TAIFEX institutional rate limiter.
func (a *TaifexInstitutionalAdapter) RateLimit() *rate.Limiter { return a.limiter }

// Metadata returns static channel metadata for TAIFEX 三大法人 期貨 OI.
func (a *TaifexInstitutionalAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "taifex_institutional",
		Country:    "台灣",
		Platform:   "TAIFEX 期交所",
		APIFormat:  "REST JSON",
		Path:       "openapi.taifex.com.tw/v1/MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate",
		HasLimiter: true,
	}
}