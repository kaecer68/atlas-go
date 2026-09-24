package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/logging"
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
	Available        bool                              `json:"available"`
	Reading          *marketdata.GovernmentFlowReading `json:"reading,omitempty"`
	InsuranceReading *marketdata.GovernmentFlowReading `json:"insurance_reading,omitempty"`
}

// Fetch returns the latest available 官股行庫 reading. A missing/empty
// directory is returned as a Stale result, NOT an error — the resonance
// model needs to know "no data" is a different state than "data says X".
//
// Issue #1940 R1: a reading whose total_net is exactly 0 is the same
// "no data" state (see GovernmentFlowReading.HasData). It is reported as
// unavailable and is NOT copied into the payload, so the gateway cannot
// seed MacroDataSnapshot.GovernmentNet with a placeholder — the file's
// existence is still visible through DataState, which the channel contract
// uses to degrade the channel instead of reporting a false "ok".
func (a *GovernmentFlowAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	reading, ok, err := a.provider.Latest()
	if err != nil {
		return nil, fmt.Errorf("government_flow: %w", err)
	}
	usable := ok && reading.HasData()
	if ok && !usable {
		logging.Warn("apigateway", "government_flow_zero_reading_ignored",
			logging.FStr("date", reading.Date),
			logging.FInt("total_net", int(reading.TotalNet)),
			logging.FStr("source", reading.Source))
	}
	payload := governmentFlowData{Available: usable}
	if usable {
		payload.Reading = &reading
	}
	// Best-effort: also fetch insurance company flow.
	if insReading, insOK, insErr := a.provider.LatestInsurance(); insErr == nil && insOK {
		payload.InsuranceReading = &insReading
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
	if !usable {
		res.Stale = true
		res.Meta.Stale = true
	}
	return res, nil
}

// DataState implements DataStateProvider for the government_flow channel.
//
// Issue #1940 R1: the channel reported "ok" for 18 trading days while
// every reading was the zero placeholder, because the contract only asked
// whether a file exists (SuccessCriteriaFileExists) and the adapter's own
// HealthCheck agreed. DataState exposes the reading's value so the
// contract (now SuccessCriteriaValueNonzero) can tell "published reading"
// apart from "placeholder file" and degrade the channel instead of
// publishing a false ok.
func (a *GovernmentFlowAdapter) DataState(ctx context.Context) (DataState, error) {
	if err := ctx.Err(); err != nil {
		return DataState{}, err
	}
	reading, ok, err := a.provider.Latest()
	if err != nil {
		return DataState{}, fmt.Errorf("government_flow data state: %w", err)
	}
	if !ok {
		return DataState{
			Present: false,
			Detail:  "no YYYYMMDD.json reading under " + a.provider.DataDir(),
		}, nil
	}
	recordedAt, _ := time.Parse("20060102", reading.Date)
	return DataState{
		Present:    true,
		NonZero:    reading.HasData(),
		RecordedAt: recordedAt,
		Detail: fmt.Sprintf("latest reading %s total_net=%d source=%s",
			reading.Date, reading.TotalNet, reading.Source),
	}, nil
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
