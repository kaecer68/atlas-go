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

// TAIEXIndexChannelAdapter adapts TAIEXIndexProvider (Taiwan Stock Exchange
// Capitalization Weighted Stock Index, ^TWII) to the DataProvider interface.
type TAIEXIndexChannelAdapter struct {
	provider *marketdata.TAIEXIndexProvider
	limiter  *rate.Limiter
}

func NewTAIEXIndexChannelAdapter(p *marketdata.TAIEXIndexProvider) *TAIEXIndexChannelAdapter {
	return &TAIEXIndexChannelAdapter{
		provider: p,
		limiter:  taiexIndexLimiter,
	}
}

func (a *TAIEXIndexChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
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
		return nil, fmt.Errorf("taiex index marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "taiex_index",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *TAIEXIndexChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchSnapshot(ctx)
	if err != nil {
		// 2026-09-06 修復: 週末/假日/pre-market 為 ErrNoData 等待態 —
		// status 維持 warn 級以下（與 tdcc/finmind adapter 同語意）,
		// 不再觸發 ChannelHealthStatusError（實證: 週六全日 error 誤報）。
		status := "error"
		if errors.Is(err, marketdata.ErrNoData) {
			status = "warn"
		}
		return HealthStatus{
			Status:    status,
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

func (a *TAIEXIndexChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TAIEXIndexChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID: "taiex_index", Country: "台灣", Platform: "Yahoo Finance",
		APIFormat: "REST JSON", Path: "query1.finance.yahoo.com", HasLimiter: true,
	}
}
