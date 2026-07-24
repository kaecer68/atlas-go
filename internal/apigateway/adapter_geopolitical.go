package apigateway

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// GeopoliticalChannelAdapter adapts narrative geopolitical providers to the DataProvider interface.
type GeopoliticalChannelAdapter struct {
	workDir        string
	limiter        *rate.Limiter
	globalProvider *geopolitical.CompositeGeopoliticalProvider
	taiwanProvider *geopolitical.CompositeTaiwanGeopoliticalProvider
}

// NewGeopoliticalChannelAdapter creates a new adapter for the geopolitical channel.
func NewGeopoliticalChannelAdapter(workDir string) *GeopoliticalChannelAdapter {
	return &GeopoliticalChannelAdapter{
		workDir:        workDir,
		limiter:        rate.NewLimiter(rate.Every(time.Minute), 1),
		globalProvider: geopolitical.NewCompositeGeopoliticalProvider(geopolitical.NewRSSGeopoliticalProvider(), geopolitical.NewGDELTGeopoliticalProvider()),
		taiwanProvider: geopolitical.NewCompositeTaiwanGeopoliticalProvider(geopolitical.NewTaiwanRSSGeopoliticalProvider()),
	}
}

// Fetch retrieves global and Taiwan geopolitical risk scores.
func (a *GeopoliticalChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	globalStore := geopolitical.NewGeopoliticalStore(filepath.Join(a.workDir, constants.StateGeopolitical))
	taiwanStore := geopolitical.NewGeopoliticalStore(filepath.Join(a.workDir, constants.StateGeopolitical+"/taiwan"))

	type geopoliticalResult struct {
		Global *geopolitical.GeopoliticalRiskScore `json:"global,omitempty"`
		Taiwan *geopolitical.GeopoliticalRiskScore `json:"taiwan,omitempty"`
	}

	result := &geopoliticalResult{}

	bgCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	score, err := a.globalProvider.FetchScore(bgCtx)
	cancel()
	if err != nil {
		logging.Error("apigateway", "geopolitical_fetch_failed", "err", err)
	} else {
		score.Timestamp = time.Now()
		result.Global = &score
		if err := globalStore.Save(score); err != nil {
			logging.Error("apigateway", "geopolitical_save_failed", "err", err)
		}
	}

	bgCtx2, cancel2 := context.WithTimeout(ctx, 60*time.Second)
	twScore, err := a.taiwanProvider.FetchScore(bgCtx2)
	cancel2()
	if err != nil {
		logging.Error("apigateway", "taiwan_geopolitical_fetch_failed", "err", err)
	} else {
		twScore.Timestamp = time.Now()
		result.Taiwan = &twScore
		if err := taiwanStore.Save(twScore); err != nil {
			logging.Error("apigateway", "taiwan_geopolitical_save_failed", "err", err)
		}
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("geopolitical marshal: %w", err)
	}

	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "geopolitical",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies connectivity by fetching scores.
func (a *GeopoliticalChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
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

// RateLimit returns the geopolitical rate limiter.
func (a *GeopoliticalChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for geopolitical.
func (a *GeopoliticalChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "geopolitical",
		Country:    "全球",
		Platform:   "RSS + GDELT",
		APIFormat:  "Composite",
		Path:       "geopolitical",
		HasLimiter: true,
	}
}
