package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
)

var bdiSharedLimiter = rate.NewLimiter(rate.Every(5*time.Second), 1)

// SetBDILimiterForTest replaces the shared BDI rate limiter for tests.
// Returns the previous limiter for caller to restore with defer/Cleanup.
func SetBDILimiterForTest(l *rate.Limiter) *rate.Limiter {
	old := bdiSharedLimiter
	bdiSharedLimiter = l
	return old
}

// BDIProvider fetches the Baltic Dry Index from CNBC.
type BDIProvider struct {
	client   *http.Client
	endpoint string
}

// NewBDIProvider creates a new BDI provider.
// Timeout and endpoint are sourced from ParametersConfig (bdi_api_timeout_sec, bdi_endpoint).
func NewBDIProvider() *BDIProvider {
	timeout := 10 // default fallback
	endpoint := "https://quote.cnbc.com/quote-html-webservice/quote.htm?symbols=.BADI&output=json"
	if cfg := config.GetParametersConfig(); cfg != nil {
		timeout = cfg.Marketdata.BDIAPITimeoutSec.Value
		if timeout < 1 {
			timeout = 10
		}
		if cfg.Marketdata.BDIEndpoint.Value != "" {
			endpoint = cfg.Marketdata.BDIEndpoint.Value
		}
	}
	return &BDIProvider{
		client:   httpclient.NewFactory().NewClient(time.Duration(timeout) * time.Second),
		endpoint: endpoint,
	}
}

// Name returns the provider name.
func (p *BDIProvider) Name() string {
	return "bdi"
}

// FetchSnapshot retrieves the latest BDI value from CNBC.
func (p *BDIProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	if err := bdiSharedLimiter.Wait(ctx); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("bdi rate limit: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint, nil)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("bdi request: %w", err)
	}
	req.Header.Set("User-Agent", "atlas-go/1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("bdi fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return MacroDataSnapshot{}, fmt.Errorf("bdi http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("bdi read body: %w", err)
	}

	var cnbcResp cnbcQuickQuoteResponse
	if err := json.Unmarshal(body, &cnbcResp); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("bdi unmarshal: %w", err)
	}

	quotes := cnbcResp.QuickQuoteResult.QuickQuote
	if len(quotes) == 0 {
		return MacroDataSnapshot{}, fmt.Errorf("bdi: empty QuickQuote array")
	}

	qq := quotes[0]
	if qq.Last == "" {
		return MacroDataSnapshot{}, fmt.Errorf("bdi: missing last price field")
	}

	value, err := strconv.ParseFloat(qq.Last, 64)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("bdi parse last: %w", err)
	}

	changePct := 0.0
	if qq.ChangePct != "" {
		changePct, err = strconv.ParseFloat(qq.ChangePct, 64)
		if err != nil {
			return MacroDataSnapshot{}, fmt.Errorf("bdi parse change_pct: %w", err)
		}
	}

	var ts int64
	if qq.LastTimeMsec != "" {
		ms, err := strconv.ParseInt(qq.LastTimeMsec, 10, 64)
		if err != nil {
			return MacroDataSnapshot{}, fmt.Errorf("bdi parse last_time_msec: %w", err)
		}
		ts = ms / 1000
	}

	return MacroDataSnapshot{
		Bdi: MacroDataPoint{
			Symbol:    ".BADI",
			Value:     value,
			ChangePct: changePct,
			Timestamp: ts,
		},
		RecordedAt: time.Now().Unix(),
	}, nil
}

type cnbcQuickQuoteResponse struct {
	QuickQuoteResult struct {
		QuickQuote []cnbcQuickQuote `json:"QuickQuote"`
	} `json:"QuickQuoteResult"`
}

type cnbcQuickQuote struct {
	Symbol       string `json:"symbol"`
	Last         string `json:"last"`
	ChangePct    string `json:"change_pct"`
	LastTimeMsec string `json:"last_time_msec"`
}
