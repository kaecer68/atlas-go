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
		Timestamp:          time.Now(),
	}}, nil
}

// HealthCheck reports the verdict for the sector_data channel.
//
// Issue #1944 Batch 2 (Q6 I14): the provider degrades gracefully (missing file →
// zero snapshot, nil error), so this check used to return "ok" for a channel
// whose backing file does not exist — a green light on a dead channel. It now
// reads the provider's load state and reports StatusDegraded with the resolved
// path and the file's own updated_at whenever the data is missing, malformed or
// older than the contract's freshness window.
func (a *SectorDataChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		return HealthStatus{
			Status:    StatusError,
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}

	state := a.provider.State()
	window := ChannelContracts().Contract("sector_data").EffectiveFreshnessWindow()
	switch {
	case !state.Found:
		return HealthStatus{
			Status:    StatusDegraded,
			LastError: fmt.Sprintf("sector_data.json not loaded (%s): %s", state.Reason, state.Path),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, nil
	case state.DataUpdatedAt.IsZero():
		return HealthStatus{
			Status:    StatusDegraded,
			LastError: fmt.Sprintf("sector_data.json has no usable updated_at (%s): %s", state.Reason, state.Path),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, nil
	case window > 0 && time.Since(state.DataUpdatedAt) > window:
		return HealthStatus{
			Status: StatusDegraded,
			LastError: fmt.Sprintf("sector_data.json stale: updated_at=%s age=%s > %s (%s)",
				state.DataUpdatedAt.Format(time.RFC3339), time.Since(state.DataUpdatedAt).Round(time.Hour), window, state.Path),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, nil
	}
	return HealthStatus{
		Status:    StatusOK,
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}, nil
}

func (a *SectorDataChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *SectorDataChannelAdapter) Metadata() ChannelMetadata {
	// Path is the single-authority location (marketdata.SectorDataDirRel);
	// it used to advertise data/state/sector_data, which nothing writes
	// (issue #1944 Batch 2, Q6 I14).
	return ChannelMetadata{ChannelID: "sector_data", Country: "台灣", Platform: "TWSE", APIFormat: "CSV/JSON", Path: marketdata.SectorDataDirRel, HasLimiter: false}
}
