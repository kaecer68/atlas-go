package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// TWSEChannelAdapter adapts the local TWSE replay dataset to the
// DataProvider interface. It is file-based: Fetch and HealthCheck read
// data/replay/tw_extended_90days.csv (no live TWSE calls, no rate limit).
//
// 2026-08-18（N1 S3a，見 docs/operations/investigation-twse-timeout-2026-08-18.md
// §3.2）：原實作註解宣稱 file-based replay，卻打 live TWSE STOCK_DAY_ALL；在
// TWSE 官方慢時段（07:17–07:58 台北 >15s）拖垮 channel_health_twse_replay 與
// etf_nav_refresh 健康檢查。現改為真正讀取本地 replay CSV —— channel 驗證的是
// 「本地資料新鮮度」而非「TWSE 通不通」，與註解意圖對齊（k3 的來源端/inactive
// 思路：把 channel 導回它聲稱的本地來源）。
type TWSEChannelAdapter struct {
	replayPath string
	limiter    *rate.Limiter
}

// NewTWSEChannelAdapter creates a new adapter that reads quotes from the local
// replay CSV at replayPath (see config.GetReplayDataPath).
func NewTWSEChannelAdapter(replayPath string) *TWSEChannelAdapter {
	return &TWSEChannelAdapter{
		replayPath: replayPath,
		limiter:    rate.NewLimiter(rate.Inf, 0), // file-based replay, no rate limit
	}
}

// Fetch loads the latest trading day's quotes from the local replay CSV.
func (a *TWSEChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	quotes, err := a.loadLatestQuotes()
	if err != nil {
		return nil, fmt.Errorf("twse replay: %w", err)
	}
	data, err := json.Marshal(quotes)
	if err != nil {
		return nil, fmt.Errorf("twse marshal: %w", err)
	}
	return &FetchResult{
		Data: data,
		Meta: FetchMetadata{
			ChannelID:          "twse_replay",
			RateLimitRemaining: int(a.limiter.Tokens()),
			Timestamp:          time.Now(),
		},
	}, nil
}

// HealthCheck verifies the local replay CSV exists, loads, and is fresh.
// A missing or stale dataset returns a clear error instead of hitting live TWSE.
func (a *TWSEChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	quotes, latestDate, err := a.loadLatestQuotesWithDate()
	if err != nil {
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}
	if len(quotes) == 0 {
		err := errors.New("replay dataset has no quotes")
		return HealthStatus{
			Status:    "error",
			LastError: err.Error(),
			UpdatedAt: time.Now().Format(time.RFC3339),
			CheckType: "readiness",
		}, err
	}
	// Freshness thresholds mirror monitoring/service.checkReplayHealth:
	// <3d ok, <14d warn, older → error (stale).
	age := time.Since(latestDate)
	status := "ok"
	if age >= 14*24*time.Hour {
		status = "error"
		err = fmt.Errorf("replay data stale: latest date %s is %s old", latestDate.Format("2006-01-02"), age.Round(time.Hour))
	} else if age >= 3*24*time.Hour {
		status = "warn"
	}
	hs := HealthStatus{
		Status:    status,
		UpdatedAt: time.Now().Format(time.RFC3339),
		CheckType: "readiness",
	}
	if err != nil {
		hs.LastError = err.Error()
	}
	return hs, err
}

// loadLatestQuotes returns the quotes for the latest date in the replay CSV.
func (a *TWSEChannelAdapter) loadLatestQuotes() ([]domain.Quote, error) {
	quotes, _, err := a.loadLatestQuotesWithDate()
	return quotes, err
}

// loadLatestQuotesWithDate returns the quotes and the latest date in the
// replay CSV, or a clear error when the file is missing or unreadable.
func (a *TWSEChannelAdapter) loadLatestQuotesWithDate() ([]domain.Quote, time.Time, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(a.replayPath)
	if err != nil {
		return nil, time.Time{}, err
	}
	if len(ds.Dates) == 0 {
		return nil, time.Time{}, fmt.Errorf("replay dataset %s has no dates", a.replayPath)
	}
	latest := ds.Dates[len(ds.Dates)-1]
	day := ds.ByDate[latest.Format("2006-01-02")]
	quotes := make([]domain.Quote, 0, len(day))
	for _, bar := range day {
		quotes = append(quotes, domain.Quote{
			Symbol:     bar.Symbol,
			Last:       bar.Close,
			Open:       bar.Open,
			High:       bar.High,
			Low:        bar.Low,
			Volume:     bar.Volume,
			Market:     "TW",
			AsOf:       bar.Date,
			IsTradable: bar.Close > 0 && bar.Volume > 0,
			Source:     bar.Source,
		})
	}
	return quotes, latest, nil
}

// RateLimit returns the TWSE rate limiter from limits.go.
func (a *TWSEChannelAdapter) RateLimit() *rate.Limiter {
	return a.limiter
}

// Metadata returns static channel metadata for TWSE.
func (a *TWSEChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{
		ChannelID:  "twse_replay",
		Country:    "台灣",
		Platform:   "TWSE 證交所",
		APIFormat:  "json",
		Path:       "www.twse.com.tw",
		HasLimiter: true,
	}
}
