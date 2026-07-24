package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// GovernmentFlowAdapter exposes operator-imported 官股行庫 readings as a
// gateway channel. The underlying provider reads a flat directory; there
// is no upstream HTTP call (manifest #E04 — honest placeholder until the
// broker-branch aggregation channel — BK-13 — is built).
type GovernmentFlowAdapter struct {
	provider *marketdata.GovernmentFlowProvider
	limiter  *rate.Limiter
}

// NewGovernmentFlowAdapter creates a new adapter.
// File-backed provider — uses rate.Inf limiter (no upstream HTTP) per Constitution Art.2.
func NewGovernmentFlowAdapter(provider *marketdata.GovernmentFlowProvider) *GovernmentFlowAdapter {
	return &GovernmentFlowAdapter{
		provider: provider,
		limiter:  rate.NewLimiter(rate.Inf, 0),
	}
}

type governmentFlowData struct {
	Available bool                              `json:"available"`
	Reading   *marketdata.GovernmentFlowReading `json:"reading,omitempty"`
}

// Fetch returns the latest available 官股行庫 reading. A missing/empty
// directory is returned as a Stale result, NOT an error — the resonance
// model needs to know "no data" is a different state than "data says X".
func (a *GovernmentFlowAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	reading, ok, err := a.provider.Latest()
	if err != nil {
		return nil, fmt.Errorf("government_flow: %w", err)
	}
	payload := governmentFlowData{Available: ok}
	if ok {
		payload.Reading = &reading
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("government_flow marshal: %w", err)
	}
	res := &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "government_flow",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}
	if !ok {
		res.Stale = true
		res.Meta.Stale = true
	}
	return res, nil
}

// HealthCheck verifies the directory exists and is readable.
func (a *GovernmentFlowAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	reading, ok, err := a.provider.Latest()
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}
	status := "ok"
	if !ok {
		status = "warn"
	}
	_ = reading
	return HealthStatus{
		Status:    status,
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}, nil
}

// RateLimit returns the limiter for file-read rate control.
func (a *GovernmentFlowAdapter) RateLimit() *rate.Limiter { return a.limiter }

// Metadata returns static channel metadata for 官股行庫 readings.
func (a *GovernmentFlowAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "government_flow",
		Country:    "台灣",
		Platform:   "TWSE",
		APIFormat:  "operator-imported",
		Path:       "data/state/government_flow/",
		Storage:    "directory",
		HasLimiter: true,
	}
}
