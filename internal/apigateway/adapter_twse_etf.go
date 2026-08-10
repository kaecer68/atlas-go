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

// TWSEETFChannelAdapter adapts the TWSE ETF provider.
//
// ⚠️ 資料源已移除（2026-08-10 實測 TWT44U → 307 → 404，非 IP rate-limit）。
// A05 typed error 讓 ErrETFUpstream / ErrETFSchema 正確觸發 circuit breaker，
// ErrETFNoTradingData 才轉 stale；實際運行會因 upstream 307 HTML 進入
// ErrETFSchema error 路徑。見 known_issues `twse_etf_upstream_60d`。
type TWSEETFChannelAdapter struct {
	provider *marketdata.TWSEETFProvider
	limiter  *rate.Limiter
}

// NewTWSEETFChannelAdapter creates a new adapter.
func NewTWSEETFChannelAdapter() *TWSEETFChannelAdapter {
	return &TWSEETFChannelAdapter{
		provider: marketdata.NewTWSEETFProvider(),
		limiter:  rate.NewLimiter(rate.Every(1*time.Second), 1),
	}
}

func (a *TWSEETFChannelAdapter) Fetch(ctx context.Context) (*FetchResult, error) {
	start := time.Now()
	if err := waitForLimiter(ctx, a.limiter); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	stats, err := a.provider.FetchLatest(ctx)
	if err != nil {
		// A05（2026-08-10 audit）：只有 ErrETFNoTradingData（上游正常回覆
		// 但最近 7 天無交易資料 = 假日/休市）才轉成 stale，避免觸發 circuit
		// breaker。403/timeout（ErrETFUpstream）與 schema 改變
		// （ErrETFSchema）是真實故障，必須以 error 回傳反映到 breaker —
		// 舊邏輯用錯誤訊息 substring 判斷，把所有情況都偽裝成假日。
		if errors.Is(err, marketdata.ErrETFNoTradingData) {
			return &FetchResult{Stale: true, Meta: FetchMetadata{ChannelID: "twse_etf", Timestamp: time.Now()}}, nil
		}
		return nil, err
	}
	data, err := json.Marshal(stats)
	if err != nil {
		return nil, fmt.Errorf("twse etf marshal: %w", err)
	}
	return &FetchResult{Data: data, Meta: FetchMetadata{
		ChannelID:          "twse_etf",
		LatencyMs:          time.Since(start).Milliseconds(),
		RateLimitRemaining: int(a.limiter.Tokens()),
		Timestamp:          time.Now(),
	}}, nil
}

func (a *TWSEETFChannelAdapter) HealthCheck(ctx context.Context) (HealthStatus, error) {
	_, err := a.provider.FetchLatest(ctx)
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

func (a *TWSEETFChannelAdapter) RateLimit() *rate.Limiter { return a.limiter }

func (a *TWSEETFChannelAdapter) Metadata() ChannelMetadata {
	return ChannelMetadata{ChannelID: "twse_etf", Country: "台灣", Platform: "TWSE", APIFormat: "REST JSON", Path: "www.twse.com.tw/exchangeReport/TWT44U", HasLimiter: true}
}
