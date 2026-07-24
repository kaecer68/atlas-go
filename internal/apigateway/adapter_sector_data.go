package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type SectorDataChannelAdapter struct {
	provider *marketdata.SectorDataProvider
	limiter  *rate.Limiter
}

func NewSectorDataChannelAdapter(p *marketdata.SectorDataProvider) *SectorDataChannelAdapter {
	return &SectorDataChannelAdapter{
		provider: p,
		limiter:  rate.NewLimiter(rate.Inf, 0),
	}
}

func (a *SectorDataChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("sector data marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID: "sector_data", LatencyMs: time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp: time.Now(),
	}}, nil
}

func (a *SectorDataChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

func (a *SectorDataChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *SectorDataChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "sector_data", Country: "台灣", Platform: "TWSE", APIFormat: "CSV/JSON", Path: "data/state/sector_data", HasLimiter: false}
}
