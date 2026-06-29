package marketdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// fubonProxyBaseURL 是 fubon-proxy listen URL prefix 的預設常數。
// PR 2 後改用 currentFubonPort 動態構造;此常數保留供測試與唯讀驗證使用。
//
// host.docker.internal 而非 127.0.0.1,因為 fubon-proxy 在 macOS host 上執行
// (原生 Python),atlas 容器必須透過 Docker Desktop 的 host-gateway 別名連線到 host。
// Docker Desktop 4.13+ (macOS/Windows) 支援 host.docker.internal 自動解析為 host gateway IP。
// 注意:不使用 localhost 避免 macOS / Linux 雙棧環境下 Go net.Dial 優先走 IPv6 [::1]。
const fubonProxyBaseURL = "http://host.docker.internal:8081"

// defaultFubonProxyPort 是 fubon-proxy listen port 的預設值(8081)。
// 對應 internal/fubonproxy.manager.go 的 defaultFubonProxyPort 常數(互相鏡像,
// 避免雙邊漂移)。cmd/atlas -fubon-port flag 透過 SetFubonProxyPort 動態覆寫。
const defaultFubonProxyPort = 8081

// currentFubonPort 是 FubonClient 構造時實際使用的 proxy listen port。
// 由 cmd/atlas/main.go 的 -fubon-port flag 透過 SetFubonProxyPort 設定。
// PR 2 Oracle 4th-round verdict F12:FubonClient.proxyURL 與
// ProcessManager.healthURL 必須來自同一個 port 值,確保 Atlas 與 fubon-proxy
// 在 alt-port 模式下仍是同一個服務。
var currentFubonPort = defaultFubonProxyPort

// SetFubonProxyPort 設定 fubon-proxy listen port,影響所有後續 newFubonClient()
// 構造的 proxyURL。port <= 0 → 不覆寫(保留測試用 helper 的預設值)。
// PR 2 F11:port != 8081 必須 log INFO,告知 user 實際的 listen port。
func SetFubonProxyPort(port int) {
	if port > 0 {
		if port != defaultFubonProxyPort {
			logging.Info("marketdata", "fubon_client_custom_port",
				"port", port,
				"message", "FubonClient 將透過 host.docker.internal 連線到非預設 port(預設 8081)",
			)
		}
		currentFubonPort = port
	}
}

type FubonClient struct {
	proxyURL        string
	httpClient      *http.Client
	intradayLimiter *rate.Limiter
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
	})
	return sharedFubonClient
}

// ResetSharedFubonClient clears the singleton (for tests).
func ResetSharedFubonClient() {
	sharedFubonClientMu.Lock()
	defer sharedFubonClientMu.Unlock()
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
	// fubonProxyBaseURL 保留供文件/測試唯讀使用;實際 proxyURL 改用 currentFubonPort
	// 動態構造,以支援 -fubon-port flag(PR 2 Oracle 4th-round verdict F12)。
	proxyURL := fmt.Sprintf("http://host.docker.internal:%d", currentFubonPort)
	return &FubonClient{
		proxyURL:        proxyURL,
		httpClient:      httpclient.NewFactory().NewClient(time.Duration(params.Marketdata.FubonAPITimeoutSec.Value) * time.Second),
		intradayLimiter: rate.NewLimiter(rate.Every(time.Minute/time.Duration(params.Marketdata.FubonIntradayLimit.Value)), params.Marketdata.FubonIntradayLimit.Value),
	}
}

func (c *FubonClient) SetHTTPClient(client *http.Client) {
	c.httpClient = client
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fubon proxy: health check failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fubon proxy: health check status %d", resp.StatusCode)
	}

	return nil
}
