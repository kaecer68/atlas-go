package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// SymbolIndustryChannelAdapter adapts the first-party per-stock industry
// provider (issue #1943).
//
// The channel is the only per-symbol source of truth for "which canonical L1
// sector does this listed company belong to". It reads the two official
// OpenAPI payloads (TWSE `t187ap03_L` 上市 產業別 / TPEx `mopsfin_t187ap03_O`
// 上櫃 SecuritiesIndustryCode), maps every 2-digit code through the declared
// namespace K table (internal/sectormap/twse_industry_code.go, landed by
// #1958) and persists one canonical snapshot to
// <stateDir>/symbol_industry.json.
//
// Numbers, not trust: the state file carries the per-status tallies
// (mapped/unmapped/unknown + per-L1 counts) so an operator can see the
// population size and every code that has no defensible target, instead of
// inferring it from a file's existence.
//
// The adapter keeps the payload the gateway caches SMALL (counts + code
// dispositions): the 1988-row entry list is the state file's job, and the
// macro/gateway cache must not grow by ~600 KB per fetch.
type SymbolIndustryChannelAdapter struct {
	provider *marketdata.SymbolIndustryProvider
	limiter  *rate.Limiter
}

// NewSymbolIndustryChannelAdapter creates the adapter. stateDir is the
// directory the state file is written to (production: <workDir>/data/state).
func NewSymbolIndustryChannelAdapter(stateDir string) *SymbolIndustryChannelAdapter {
	return &SymbolIndustryChannelAdapter{
		provider: marketdata.NewSymbolIndustryProvider(stateDir),
		// 1 req / 5s: two upstream calls per refresh, both official and
		// bulk (the provider additionally waits on the shared TWSE / TPEx
		// token buckets). Same tier as the other bulk TWSE channels.
		limiter: rate.NewLimiter(ExportStatisticsRate, ExportStatisticsBurst),
	}
}

// SetProvider overrides the provider (tests).
func (a *SymbolIndustryChannelAdapter) SetProvider(p *marketdata.SymbolIndustryProvider) {
	if p != nil {
		a.provider = p
	}
}

// StateFile returns the persisted snapshot path.
func (a *SymbolIndustryChannelAdapter) StateFile() string {
	if a.provider == nil {
		return ""
	}
	return a.provider.StateFile()
}

// symbolIndustryChannelPayload is what the gateway caches: the snapshot
// WITHOUT the per-symbol rows.
type symbolIndustryChannelPayload struct {
	Channel       string                                     `json:"channel"`
	UpdatedAt     string                                     `json:"updated_at"`
	Sources       []string                                   `json:"sources"`
	Counts        marketdata.SymbolIndustryCounts            `json:"counts"`
	L1Counts      map[string]int                             `json:"l1_counts"`
	UnmappedCodes []marketdata.SymbolIndustryCodeDisposition `json:"unmapped_codes"`
	UnknownCodes  []marketdata.SymbolIndustryCodeDisposition `json:"unknown_codes"`
	StateFile     string                                     `json:"state_file"`
}

func (a *SymbolIndustryChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, fmt.Errorf("symbol_industry rate limit: %w", err)
	}

	snap, err := a.provider.FetchAll(ctx)
	if err != nil {
		return nil, err
	}

	payload := symbolIndustryChannelPayload{
		Channel:       snap.Channel,
		UpdatedAt:     snap.UpdatedAt,
		Sources:       snap.Sources,
		Counts:        snap.Counts,
		L1Counts:      snap.L1Counts,
		UnmappedCodes: snap.UnmappedCodes,
		UnknownCodes:  snap.UnknownCodes,
		StateFile:     a.provider.StateFile(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("symbol_industry marshal: %w", err)
	}

	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "symbol_industry",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// DataState implements DataStateProvider. The contract for this channel is
// HealthSource=file_state + SuccessCriteria=value_nonzero + DegradedOnEmpty,
// so "the file exists" is never enough: the entry count is what turns a
// successful-but-empty refresh into a degraded channel instead of a false ok
// (#1953 rule, applied here from the start).
func (a *SymbolIndustryChannelAdapter) DataState(ctx context.Context) (DataState, error) {
	if err := ctx.Err(); err != nil {
		return DataState{}, err
	}
	if a.provider == nil {
		return DataState{Present: false, Detail: "symbol_industry provider not wired"}, nil
	}
	snap, err := a.provider.LoadSnapshot()
	if err != nil {
		return DataState{Present: false, Detail: fmt.Sprintf("state file unreadable: %v", err)}, nil
	}
	if snap == nil || len(snap.Entries) == 0 {
		return DataState{
			Present: snap != nil,
			Detail:  "no entries in " + a.provider.StateFile(),
		}, nil
	}
	recordedAt, _ := time.Parse(time.RFC3339, snap.UpdatedAt)
	return DataState{
		Present:    true,
		NonZero:    true,
		RecordedAt: recordedAt,
		Detail: fmt.Sprintf("%d symbols (%d mapped, %d unmapped, %d unknown codes) from %v",
			snap.Counts.Total, snap.Counts.Mapped, snap.Counts.Unmapped, snap.Counts.Unknown, snap.Sources),
	}, nil
}

// HealthCheck is a readiness check on the persisted snapshot: the upstream
// fetch is the scheduled task's job, the health signal is "is there usable
// data on disk".
func (a *SymbolIndustryChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	if err := ctx.Err(); err != nil {
		return HealthStatus{}, err
	}
	if a.provider == nil {
		return HealthStatus{
			Status:    "error",
			LastError: "symbol_industry provider not wired",
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, fmt.Errorf("symbol_industry: provider not wired")
	}
	_, err := a.provider.LoadSnapshot()
	if err != nil {
		return HealthStatus{
			Status:    "warn",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, nil
	}
	ds, dsErr := a.DataState(ctx)
	if dsErr != nil || !ds.NonZero {
		detail := "no usable symbol_industry data"
		if dsErr != nil {
			detail = dsErr.Error()
		} else if ds.Detail != "" {
			detail = ds.Detail
		}
		return HealthStatus{
			Status:    "warn",
			LastError: detail,
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, nil
	}
	return HealthStatus{
		Status:    "ok",
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}, nil
}

func (a *SymbolIndustryChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *SymbolIndustryChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "symbol_industry",
		Country:    "台灣",
		Platform:   "TWSE / TPEx OpenAPI",
		APIFormat:  "REST JSON",
		Path:       "openapi.twse.com.tw/v1/opendata/t187ap03_L + www.tpex.org.tw/openapi/v1/mopsfin_t187ap03_O",
		HasLimiter: true,
	}
}
