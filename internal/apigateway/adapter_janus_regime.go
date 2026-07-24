package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/janus"
)

// JANUSRegimeChannelAdapter adapts the JANUS engine to the DataProvider interface.
type JANUSRegimeChannelAdapter struct {
	engine  *janus.Engine
	limiter *rate.Limiter
}

// NewJANUSRegimeChannelAdapter creates a new adapter for the JANUS regime channel.
func NewJANUSRegimeChannelAdapter(engine *janus.Engine) *JANUSRegimeChannelAdapter {
	return &JANUSRegimeChannelAdapter{
		engine:  engine,
		limiter: rate.NewLimiter(rate.Inf, 0), // computed data, no rate limit
	}
}

func (a *JANUSRegimeChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if a.engine == nil {
		return nil, fmt.Errorf("janus_regime: engine not initialized")
	}
	status := a.engine.GetStatus()
	data, err := json.Marshal(status)
	if err != nil {
		return nil, fmt.Errorf("janus_regime marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "janus_regime",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
			LatencyMs:          time.Since(start).Milliseconds(),
		},
	}, nil
}

// HealthCheck verifies the JANUS engine is running.
func (a *JANUSRegimeChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if a.engine == nil {
		return HealthStatus{
			Status:    "inactive",
			LastError: "JANUS engine not initialized",
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "computed",
		}, nil
	}
	status := a.engine.GetStatus()
	if status.LastUpdated.IsZero() {
		return HealthStatus{
			Status:    "warn",
			LastError: "JANUS loaded but not yet updated",
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "computed",
		}, nil
	}
	age := time.Since(status.LastUpdated)
	if age > 7*24*time.Hour {
		return HealthStatus{
			Status:    "warn",
			LastError: fmt.Sprintf("JANUS last updated %d days ago", int(age.Hours()/24)),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "computed",
		}, nil
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "computed",
	}, nil
}

// RateLimit returns the JANUS regime rate limiter.
func (a *JANUSRegimeChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for JANUS regime.
func (a *JANUSRegimeChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "janus_regime",
		Country:    "全域",
		Platform:   "JANUS Engine",
		APIFormat:  "Internal (computed)",
		Path:       "internal/janus",
		HasLimiter: false,
	}
}
