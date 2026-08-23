package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// GovernmentBrokerChannelAdapter adapts the TWSE broker-level government-bank
// aggregator to the apigateway DataProvider contract.
//
// This is the C06 follow-up: the aggregator previously constructed a raw
// &http.Client{} outside the Gateway. By wrapping it as a channel it now
// receives circuit-breaker protection, unified health tracking, and a shared
// rate limiter that the Gateway can observe.
type GovernmentBrokerChannelAdapter struct {
	aggregator *marketdata.GovernmentBrokerAggregator
	limiter    *rate.Limiter
}

// NewGovernmentBrokerChannelAdapter creates an adapter that writes readings to
// the same directory consumed by GovernmentFlowProvider.
func NewGovernmentBrokerChannelAdapter(aggregator *marketdata.GovernmentBrokerAggregator) *GovernmentBrokerChannelAdapter {
	return &GovernmentBrokerChannelAdapter{
		aggregator: aggregator,
		limiter:    rate.NewLimiter(rate.Every(2*time.Second), 1),
	}
}

// Fetch triggers a full aggregate run for the previous trading day, writes
// the reading to the configured output directory, and returns the reading as
// the FetchResult payload.
func (a *GovernmentBrokerChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	date := marketdata.PreviousTradingDay(time.Now(), 1)
	reading, err := a.aggregator.AggregateDate(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("government_broker aggregate %s: %w", date.Format("20060102"), err)
	}
	// reading == nil with err == nil means "no stocks processed for the
	// requested date" (a normal non-trading-day outcome). Surface a stub
	// empty payload so the dashboard sees a successful fetch and the
	// channel-health page does NOT flag it as error.
	if reading == nil {
		data, marshalErr := json.Marshal(struct {
			Date   string `json:"date"`
			Status string `json:"status"`
		}{Date: date.Format("20060102"), Status: "no_data"})
		if marshalErr != nil {
			return nil, fmt.Errorf("government_broker marshal no_data: %w", marshalErr)
		}
		return &FetchResult{
			Data: data,
			Meta: FetchMetadata{
				ChannelID:          a.Metadata().ChannelID,
				Timestamp:          time.Now(),
				LatencyMs:          time.Since(start).Milliseconds(),
				RateLimitRemaining: int(a.limiter.Tokens()),
			},
		}, nil
	}
	data, err := json.Marshal(reading)
	if err != nil {
		return nil, fmt.Errorf("government_broker marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          a.Metadata().ChannelID,
			Timestamp:          time.Now(),
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
		},
	}, nil
}

// HealthCheck verifies that a recent government-broker reading exists and is
// fresh enough (within 48h). This avoids the expensive 50-symbol fetch on every
// health probe.
func (a *GovernmentBrokerChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	date := marketdata.PreviousTradingDay(time.Now(), 1)
	file := filepath.Join(a.aggregator.DataDir(), date.Format("20060102")+".json")
	info, err := os.Stat(file)
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}
	age := time.Since(info.ModTime())
	status := "ok"
	if age > 48*time.Hour {
		status = "stale"
	}
	return HealthStatus{
		Status:    status,
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}, nil
}

// DataState implements DataStateProvider: it reports the persisted
// government-broker reading state so contract evaluation can verify data
// validity (non-zero total_net) instead of trusting a successful fetch.
//
// This is the concrete cure for the 2026-08-22 "ok 假象": AggregateDate
// returns (nil, nil) when every upstream symbol fetch fails (e.g. all
// captcha'd), the adapter surfaces a no_data stub as a successful fetch,
// and no reading file is written for the date. DataState then reports
// Present=false, which fails the contract's value_nonzero SuccessCriteria
// and downgrades the recorded health from "ok" to "degraded".
func (a *GovernmentBrokerChannelAdapter) DataState(ctx context.Context) (DataState, error) {
	date := marketdata.PreviousTradingDay(time.Now(), 1)
	file := filepath.Join(a.aggregator.DataDir(), date.Format("20060102")+".json")

	info, err := os.Stat(file)
	if err != nil {
		if os.IsNotExist(err) {
			return DataState{
				Present: false,
				Detail:  "missing reading file for " + date.Format("20060102"),
			}, nil
		}
		return DataState{}, fmt.Errorf("government_broker stat %s: %w", file, err)
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		return DataState{}, fmt.Errorf("government_broker read %s: %w", file, err)
	}

	var reading marketdata.GovernmentFlowReading
	if err := json.Unmarshal(raw, &reading); err != nil {
		return DataState{
			Present:    true,
			NonZero:    false,
			RecordedAt: info.ModTime(),
			Detail:     fmt.Sprintf("%s unparseable reading", file),
		}, nil
	}

	return DataState{
		Present:    true,
		NonZero:    reading.TotalNet != 0,
		RecordedAt: info.ModTime(),
		Detail:     fmt.Sprintf("%s total_net=%d", file, reading.TotalNet),
	}, nil
}

// RateLimit returns the per-symbol rate limiter used by the aggregator.
func (a *GovernmentBrokerChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns the static channel metadata for TWSE government broker data.
func (a *GovernmentBrokerChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "government_broker",
		Country:    "台灣",
		Platform:   "TWSE 券商分支",
		APIFormat:  "html/csv",
		Path:       "https://bsr.twse.com.tw/bshtm/bsContent.aspx",
		HasLimiter: true,
	}
}
