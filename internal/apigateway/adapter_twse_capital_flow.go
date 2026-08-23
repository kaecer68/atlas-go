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

// TWSECapitalFlowChannelAdapter adapts a TWSECapitalFlowProvider to the DataProvider interface.
type TWSECapitalFlowChannelAdapter struct {
	provider *marketdata.TWSECapitalFlowProvider
	limiter  *rate.Limiter
}

// NewTWSECapitalFlowChannelAdapter creates a new adapter for the TWSE capital flow channel.
func NewTWSECapitalFlowChannelAdapter(provider *marketdata.TWSECapitalFlowProvider) *TWSECapitalFlowChannelAdapter {
	return &TWSECapitalFlowChannelAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(TWSECapitalFlowRate, TWSECapitalFlowBurst),
	}
}

// Fetch retrieves the latest capital flow snapshot.
func (a *TWSECapitalFlowChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	snap, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		// Non-trading days or holidays: TWSE returns no data for the past 7 days,
		// which is expected behavior. Return a stale result instead of an error
		// to avoid triggering the circuit breaker. If a persisted daily flow file
		// exists, serve it so downstream consumers still see usable numbers.
		// P1-9: typed no-data classification (previously string matching
		// "no TWSE"/"no data", which broke when the upstream error message
		// changed). TWSECapitalFlowProvider now wraps ErrNoData.
		if errors.Is(err, marketdata.ErrNoData) {
			meta := FetchMetadata{ChannelID: "twse_capital_flow", Timestamp: time.Now()}
			fallbackSnap, fallbackErr := a.provider.LatestSavedSnapshot(ctx)
			if fallbackErr == nil {
				data, marshalErr := json.Marshal(fallbackSnap)
				if marshalErr == nil {
					return &FetchResult{Data: data, Stale: true, Meta: meta}, nil
				}
			}
			return &FetchResult{Stale: true, Meta: meta}, nil
		}
		return nil, fmt.Errorf("capital_flow fetch: %w", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		return nil, fmt.Errorf("capital_flow marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "twse_capital_flow",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching a snapshot.
func (a *TWSECapitalFlowChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

// RateLimit returns the TWSE capital flow rate limiter.
func (a *TWSECapitalFlowChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TWSE capital flow.
func (a *TWSECapitalFlowChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_capital_flow",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "json",
		Path:       "www.twse.com.tw/rwd/zh/fund/T86",
		HasLimiter: true,
	}
}
