package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// TaiwanGeopoliticalChannelAdapter adapts Taiwan RSS geopolitical provider.
type TaiwanGeopoliticalChannelAdapter struct {
	provider *geopolitical.TaiwanRSSGeopoliticalProvider
	workDir  string
	limiter  *rate.Limiter
}

// NewTaiwanGeopoliticalChannelAdapter creates a new adapter.
func NewTaiwanGeopoliticalChannelAdapter(workDir string) *TaiwanGeopoliticalChannelAdapter {
	return &TaiwanGeopoliticalChannelAdapter{
		provider: geopolitical.NewTaiwanRSSGeopoliticalProvider(),
		workDir:  workDir,
		limiter:  rate.NewLimiter(rate.Every(time.Minute), 1),
	}
}

// Fetch retrieves the Taiwan-specific geopolitical score.
func (a *TaiwanGeopoliticalChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	bgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	score, err := a.provider.FetchScore(bgCtx)
	if err != nil {
		return nil, fmt.Errorf("taiwan_geopolitical fetch: %w", err)
	}
	score.Timestamp = time.Now()
	store := geopolitical.NewGeopoliticalStore(filepath.Join(a.workDir, "data/state/geopolitical/taiwan"))
	if err := store.Save(score); err != nil {
		logging.Error("apigateway", "taiwan_geopolitical_save_failed", "err", err)
	}
	data, err := json.Marshal(score)
	if err != nil {
		return nil, fmt.Errorf("taiwan_geopolitical marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "geopolitical_taiwan",
			LatencyMs:          time.Since(start).Milliseconds(),
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching scores.
func (a *TaiwanGeopoliticalChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.Fetch(ctx)
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

// RateLimit returns the Taiwan geopolitical rate limiter.
func (a *TaiwanGeopoliticalChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for Taiwan geopolitical.
func (a *TaiwanGeopoliticalChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "geopolitical_taiwan",
		Country:    "台灣",
		Platform:   "CNA / 自由時報 / TVBS RSS",
		APIFormat:  "RSS XML",
		Path:       "www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw",
		HasLimiter: true,
	}
}

// SetHTTPClient sets a custom HTTP client for the underlying provider.
func (a *TaiwanGeopoliticalChannelAdapter) SetHTTPClient(client *http.Client) {
	if a.provider != nil {
		a.provider.SetHTTPClient(client)
	}
}
