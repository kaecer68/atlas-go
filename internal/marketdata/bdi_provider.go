package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"golang.org/x/time/rate"
)

var bdiSharedLimiter = rate.NewLimiter(rate.Every(5*time.Second), 1)

// BDIProvider fetches the Baltic Dry Index from CNBC.
type BDIProvider struct {
	mu       sync.Mutex
	client   *http.Client
	endpoint string
}

// NewBDIProvider creates a new BDI provider.
func NewBDIProvider() *BDIProvider {
	return &BDIProvider{
		client:   httpclient.NewFactory().NewClient(10 * time.Second),
		endpoint: "https://quote.cnbc.com/quote-html-webservice/quote.htm?symbols=.BADI&output=json",
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
