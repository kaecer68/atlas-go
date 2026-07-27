package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/fubonproxy"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// fubonProxyBaseURL 是 fubon-proxy listen URL 的**文件/測試參考字串**。
// PR #837 user prompt 列為 A1 root cause:此字串以前散落在 3 個 source files
// (fubon_client.go / hybrid_provider.go / register_adapters.go) 各自硬編碼,
// 任一處忘記同步就會造成 port drift → channel recurring failure。
//
// 唯一 runtime 構造點已搬到 `fubonproxy.ProxyBaseURL()`,本檔案禁止再以
// `fmt.Sprintf("http://...:%d", ...)` 構造 proxyURL — 違反將被
// `fubon_url_guard_test.go` 的 AST 字串禁制擋下。
//
// `fubon-proxy` (Docker compose service name,PR #941) 取代 `host.docker.internal`。
// 在容器內 Docker DNS 解析 service name 到 bridge network 的 container IP,
// 不再依賴 Docker Desktop host→container port forwarding (有可靠度問題)。
// 本機開發時 register_adapters.go 會自動呼叫 fubonproxy.SetProxyHost("127.0.0.1")。
//
// 歷史:原本用 host.docker.internal 的理由(RCA: PR #495)是因為 fubon-proxy Python
// 跑在 macOS host 而非容器內,而從 container 用 127.0.0.1 會 hit 自己 loopback(錯的)。
// 2026-07-04 PR #940 把 fubon-proxy 也容器化後,情境已改變。
//
//lint:ignore U1000 kept for documentation and test reference
const fubonProxyBaseURL = "http://fubon-proxy:18081"

type FubonClient struct {
	proxyURL        string
	httpClient      *http.Client
	healthClient    *http.Client
	intradayLimiter *rate.Limiter

	healthy       atomic.Bool
	healthStop    chan struct{}
	healthRunning atomic.Bool
}

type FubonQuoteResponse struct {
	Symbol         string  `json:"symbol"`
	Name           string  `json:"name"`
	Last           float64 `json:"last"`
	Open           float64 `json:"open"`
	High           float64 `json:"high"`
	Low            float64 `json:"low"`
	Volume         int     `json:"volume"`
	ReferencePrice float64 `json:"reference_price"`
	PreviousClose  float64 `json:"previous_close"`
	Change         float64 `json:"change"`
	ChangePercent  float64 `json:"change_percent"`
	Bids           []struct {
		Price float64 `json:"price"`
		Size  int     `json:"size"`
	} `json:"bids"`
	Asks []struct {
		Price float64 `json:"price"`
		Size  int     `json:"size"`
	} `json:"asks"`
	IsOpen    bool   `json:"is_open"`
	IsClose   bool   `json:"is_close"`
	Timestamp int64  `json:"timestamp"`
	Source    string `json:"source"`
}

type FubonMarketStatus struct {
	Status    string `json:"status"`
	IsOpen    bool   `json:"is_open"`
	Timestamp int    `json:"timestamp"`
}

var (
	sharedFubonClient     *FubonClient
	sharedFubonClientOnce sync.Once
	sharedFubonClientMu   sync.RWMutex
)

// GetSharedFubonClient returns a singleton FubonClient that all components
// share. Using a single client ensures consistent proxy URL, shared HTTP
// client, and one intraday rate-limiter token bucket across all call sites
// (gateway channels, hybrid provider).
func GetSharedFubonClient() *FubonClient {
	sharedFubonClientOnce.Do(func() {
		sharedFubonClient = newFubonClient()
		sharedFubonClient.StartHealthProbe(15 * time.Second)
	})
	return sharedFubonClient
}

// ResetSharedFubonClient clears the singleton (for tests).
func ResetSharedFubonClient() {
	sharedFubonClientMu.Lock()
	defer sharedFubonClientMu.Unlock()
	if sharedFubonClient != nil {
		sharedFubonClient.StopHealthProbe()
	}
	sharedFubonClient = nil
	sharedFubonClientOnce = sync.Once{}
}

// NewFubonClient creates a standalone FubonClient with its own rate limiter.
// Prefer GetSharedFubonClient in production to avoid multiple independent
// token buckets that can collectively exceed the rate limit.
func NewFubonClient() *FubonClient {
	return newFubonClient()
}

func newFubonClient() *FubonClient {
	params := config.GetParametersConfig()
	c := &FubonClient{
		proxyURL:        fubonproxy.ProxyBaseURL(),
		httpClient:      httpclient.NewFactory().NewClient(time.Duration(params.Marketdata.FubonAPITimeoutSec.Value) * time.Second),
		healthClient:    httpclient.NewFactory().NewClient(2 * time.Second),
		intradayLimiter: rate.NewLimiter(rate.Every(time.Minute/time.Duration(params.Marketdata.FubonIntradayLimit.Value)), params.Marketdata.FubonIntradayLimit.Value),
	}
	c.healthy.Store(true)
	return c
}

func (c *FubonClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
}

func (c *FubonClient) SetHealthClient(client *http.Client) {
	c.healthClient = client
}

func (c *FubonClient) GetQuote(ctx context.Context, symbol string) (domain.Quote, error) {
	if err := c.intradayLimiter.Wait(ctx); err != nil {
		return domain.Quote{}, fmt.Errorf("fubon proxy: rate limit wait: %w", ErrRateLimited)
	}

	endpoint := fmt.Sprintf("%s/quote/%s", c.proxyURL, symbol)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("fubon proxy: create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("fubon proxy: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return domain.Quote{}, fmt.Errorf("fubon proxy: status %d", resp.StatusCode)
	}

	var fubonResp FubonQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&fubonResp); err != nil {
		return domain.Quote{}, fmt.Errorf("fubon proxy: decode response: %w", err)
	}

	quote := domain.Quote{
		Symbol:     symbol,
		Last:       fubonResp.Last,
		Open:       fubonResp.Open,
		High:       fubonResp.High,
		Low:        fubonResp.Low,
		Volume:     int64(fubonResp.Volume),
		Market:     "TW",
		AsOf:       time.Now(),
		IsTradable: fubonResp.IsOpen && !fubonResp.IsClose,
		Source:     "fubon",
	}

	return quote, nil
}

func (c *FubonClient) GetQuotes(ctx context.Context, symbols []string) ([]domain.Quote, error) {
	if len(symbols) == 0 {
		return []domain.Quote{}, nil
	}

	if len(symbols) == 1 {
		quote, err := c.GetQuote(ctx, symbols[0])
		if err != nil {
			return nil, err
		}
		return []domain.Quote{quote}, nil
	}

	if err := c.intradayLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("fubon proxy: rate limit wait: %w", ErrRateLimited)
	}

	endpoint := fmt.Sprintf("%s/quotes", c.proxyURL)
	params := url.Values{}
	params.Set("symbols", symbols[0])
	for i := 1; i < len(symbols); i++ {
		params.Set("symbols", params.Get("symbols")+","+symbols[i])
	}

	reqURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("fubon proxy: create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fubon proxy: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fubon proxy: status %d", resp.StatusCode)
	}

	var fubonResps []FubonQuoteResponse
	if err := json.NewDecoder(resp.Body).Decode(&fubonResps); err != nil {
		return nil, fmt.Errorf("fubon proxy: decode response: %w", err)
	}

	quotes := make([]domain.Quote, 0, len(fubonResps))
	for _, r := range fubonResps {
		quotes = append(quotes, domain.Quote{
			Symbol:     r.Symbol,
			Last:       r.Last,
			Open:       r.Open,
			High:       r.High,
			Low:        r.Low,
			Volume:     int64(r.Volume),
			Market:     "TW",
			AsOf:       time.Now(),
			IsTradable: r.IsOpen && !r.IsClose,
			Source:     "fubon",
		})
	}

	return quotes, nil
}

func (c *FubonClient) CheckMarketStatus(ctx context.Context) (bool, error) {
	endpoint := fmt.Sprintf("%s/market-status", c.proxyURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return false, fmt.Errorf("fubon proxy: create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("fubon proxy: http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("fubon proxy: status %d", resp.StatusCode)
	}

	var status FubonMarketStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return false, fmt.Errorf("fubon proxy: decode response: %w", err)
	}

	return status.IsOpen, nil
}

func (c *FubonClient) HealthCheck(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/health", c.proxyURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("fubon proxy: create health request: %w", err)
	}

	resp, err := c.healthClient.Do(req)
	if err != nil {
		return fmt.Errorf("fubon proxy: health check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fubon proxy: health check status %d", resp.StatusCode)
	}

	return nil
}

const healthProbeConsecutiveFailures = 3

func (c *FubonClient) StartHealthProbe(interval time.Duration) {
	if c.healthRunning.Swap(true) {
		return
	}
	c.healthStop = make(chan struct{})
	c.healthy.Store(true)
	go c.runHealthProbe(interval)
}

func (c *FubonClient) StopHealthProbe() {
	if !c.healthRunning.Swap(false) {
		return
	}
	close(c.healthStop)
}

func (c *FubonClient) IsHealthy() bool {
	return c.healthy.Load()
}

func (c *FubonClient) runHealthProbe(interval time.Duration) {
	backoff := interval
	maxBackoff := 5 * time.Minute
	var failures atomic.Int32
	probe := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := c.HealthCheck(ctx); err != nil {
			n := failures.Add(1)
			if n >= healthProbeConsecutiveFailures {
				c.healthy.Store(false)
				// Exponential backoff: double wait each failure, capped at 5 min.
				backoff = time.Duration(1<<min(int(n), 8)) * interval / 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			logging.Warn("fubon_client", "health_probe_failed",
				logging.Err(err),
				logging.FInt("consecutive_failures", int(n)),
				logging.FFloat64("backoff_sec", backoff.Seconds()))
		} else {
			failures.Store(0)
			c.healthy.Store(true)
			backoff = interval
		}
	}
	probe()
	for {
		select {
		case <-c.healthStop:
			return
		case <-time.After(backoff):
			probe()
		}
	}
}
