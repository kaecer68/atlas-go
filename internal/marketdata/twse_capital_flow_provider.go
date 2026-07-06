package marketdata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// TWSECapitalFlow holds daily net buy/sell for the three major investor types.
type TWSECapitalFlow struct {
	Date               string  `json:"date"`
	ForeignInvestorNet float64 `json:"foreign_investor_net"` // 外資及陸資買賣超
	DomesticFundNet    float64 `json:"domestic_fund_net"`    // 投信買賣超
	DealerNet          float64 `json:"dealer_net"`           // 自營商買賣超
	TotalNet           float64 `json:"total_net"`
}

// TWSECapitalFlowProvider fetches Taiwan institutional investor flows from TWSE.
type TWSECapitalFlowProvider struct {
	client     *http.Client
	storageDir string
	limiter    *rate.Limiter
}

// NewTWSECapitalFlowProvider creates a new TWSE capital flow provider.
func NewTWSECapitalFlowProvider(storageDir string) *TWSECapitalFlowProvider {
	return &TWSECapitalFlowProvider{
		client:     httpclient.NewFactory().NewClient(20 * time.Second),
		storageDir: storageDir,
		limiter:    rate.NewLimiter(rate.Every(5*time.Second), 1),
	}
}

// SetHTTPClient sets a custom HTTP client for tests.
func (t *TWSECapitalFlowProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		t.client = client
	}
}

// Name returns the provider name.
func (t *TWSECapitalFlowProvider) Name() string {
	return "twse_capital_flow"
}

// FetchSnapshot retrieves the latest capital flow data and merges into MacroDataSnapshot.
func (t *TWSECapitalFlowProvider) FetchSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	flow, err := t.fetchLatestTradingDay(ctx)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("fetch capital flow: %w", err)
	}

	// Persist for audit.
	if err := t.saveFlow(flow); err != nil {
		logging.Warn("twse_capital_flow_provider", "save_flow_warning", logging.Err(err))
	}

	flowTime, _ := time.ParseInLocation("20060102", flow.Date, time.FixedZone("CST", 8*60*60))
	flowTs := flowTime.Unix()
	snap := MacroDataSnapshot{
		RecordedAt: flowTs,
	}
	snap.ForeignInvestorNet = MacroDataPoint{Symbol: "TAIWAN_FOREIGN", Value: flow.ForeignInvestorNet, Timestamp: flowTs}
	snap.DomesticFundNet = MacroDataPoint{Symbol: "TAIWAN_DOMESTIC", Value: flow.DomesticFundNet, Timestamp: flowTs}
	snap.DealerNet = MacroDataPoint{Symbol: "TAIWAN_DEALER", Value: flow.DealerNet, Timestamp: flowTs}
	return snap, nil
}

func (t *TWSECapitalFlowProvider) fetchLatestTradingDay(ctx context.Context) (TWSECapitalFlow, error) {
	now := time.Now().UTC()
	// Try up to 7 days back to find the most recent trading day with data.
	for i := range 7 {
		dateStr := now.AddDate(0, 0, -i).Format("20060102")
		flow, err := t.fetchDate(ctx, dateStr)
		if err == nil {
			return flow, nil
		}
	}
	return TWSECapitalFlow{}, fmt.Errorf("no TWSE capital flow data available in the last 7 days")
}

func (t *TWSECapitalFlowProvider) fetchDate(ctx context.Context, dateStr string) (TWSECapitalFlow, error) {
	if err := t.limiter.Wait(ctx); err != nil {
		return TWSECapitalFlow{}, fmt.Errorf("rate limit: %w", err)
	}
	url := fmt.Sprintf(constants.TWSEBaseURL+"/rwd/zh/fund/T86?response=json&date=%s&selectType=ALLBUT0999", dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return TWSECapitalFlow{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return TWSECapitalFlow{}, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TWSECapitalFlow{}, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseT86Response
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return TWSECapitalFlow{}, fmt.Errorf("decode response: %w", err)
	}
	if apiResp.Stat != "OK" || len(apiResp.Data) == 0 {
		return TWSECapitalFlow{}, fmt.Errorf("TWSE API returned no data: %s", apiResp.Stat)
	}

	var totalForeign, totalDomestic, totalDealer float64
	for _, row := range apiResp.Data {
		if len(row) < 12 {
			continue
		}
		// Column mapping based on TWSE T86 schema (19 columns as of 2026):
		// 0: 證券代號, 1: 證券名稱,
		// 2: 外陸資買進股數(不含外資自營商), 3: 外陸資賣出股數(不含外資自營商), 4: 外陸資買賣超股數(不含外資自營商),
		// 5: 外資自營商買進股數, 6: 外資自營商賣出股數, 7: 外資自營商買賣超股數,
		// 8: 投信買進股數, 9: 投信賣出股數, 10: 投信買賣超股數,
		// 11: 自營商買賣超股數(含自行+避險), 12: 自營商買進股數(自行買賣), 13: 自營商賣出股數(自行買賣), 14: 自營商買賣超股數(自行買賣),
		// 15: 自營商買進股數(避險), 16: 自營商賣出股數(避險), 17: 自營商買賣超股數(避險),
		// 18: 三大法人買賣超股數
		foreign := parseTWDVolume(row[4])
		domestic := parseTWDVolume(row[10])
		dealer := parseTWDVolume(row[11])
		totalForeign += foreign
		totalDomestic += domestic
		totalDealer += dealer
	}

	flow := TWSECapitalFlow{
		Date:               dateStr,
		ForeignInvestorNet: totalForeign / 1e8, // convert shares to rough proxy (simplified)
		DomesticFundNet:    totalDomestic / 1e8,
		DealerNet:          totalDealer / 1e8,
		TotalNet:           (totalForeign + totalDomestic + totalDealer) / 1e8,
	}
	return flow, nil
}

func (t *TWSECapitalFlowProvider) saveFlow(flow TWSECapitalFlow) error {
	if err := os.MkdirAll(t.storageDir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	path := filepath.Join(t.storageDir, flow.Date+"_capital_flow.json")
	data, err := json.MarshalIndent(flow, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal flow: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

type twseT86Response struct {
	Stat string     `json:"stat"`
	Data [][]string `json:"data"`
}

func parseTWDVolume(s string) float64 {
	// Remove commas and parse.
	s = strings.ReplaceAll(s, ",", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}
