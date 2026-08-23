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

// TWSECapitalFlow holds daily net buy/sell for the three major investor types,
// with sub-type splits for foreign and dealer per TWSE T86 column schema.
type TWSECapitalFlow struct {
	Date               string  `json:"date"`
	ForeignInvestorNet float64 `json:"foreign_investor_net"` // 外陸資買賣超(不含外資自營商) — column 4
	ForeignDealerNet   float64 `json:"foreign_dealer_net"`   // 外資自營商買賣超 — column 7
	DomesticFundNet    float64 `json:"domestic_fund_net"`    // 投信買賣超 — column 10
	DealerNet          float64 `json:"dealer_net"`           // 自營商合計(自行+避險) — column 11
	DealerSelfNet      float64 `json:"dealer_self_net"`      // 自營商自行買賣 — column 14
	DealerHedgingNet   float64 `json:"dealer_hedging_net"`   // 自營商避險 — column 17
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
		// P1-13: shared TWSE token bucket (was an independent 1/5s limiter).
		limiter: getTWSESharedLimiter(),
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

// SetRateLimiter overrides the rate limiter (tests only; P1-13 shared-bucket
// tests use SetTWSESharedLimiterForTest instead).
func (t *TWSECapitalFlowProvider) SetRateLimiter(l *rate.Limiter) {
	if l != nil {
		t.limiter = l
	}
}

// SymbolFlow holds per-symbol institutional investor flow from TWSE T86.
type SymbolFlow struct {
	Symbol             string  `json:"symbol"`
	Name               string  `json:"name"`
	ForeignInvestorNet float64 `json:"foreign_investor_net"`
	DomesticFundNet    float64 `json:"domestic_fund_net"`
	DealerNet          float64 `json:"dealer_net"`
	Date               string  `json:"date"`
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

	return t.buildSnapshotFromFlow(flow), nil
}

// buildSnapshotFromFlow converts a TWSECapitalFlow into a MacroDataSnapshot.
func (t *TWSECapitalFlowProvider) buildSnapshotFromFlow(flow TWSECapitalFlow) MacroDataSnapshot {
	flowTime, _ := time.ParseInLocation("20060102", flow.Date, time.FixedZone("CST", 8*60*60))
	flowTs := flowTime.Unix()

	snap := MacroDataSnapshot{RecordedAt: flowTs}
	prev, _ := t.loadPreviousFlow(flow.Date)
	snap.ForeignInvestorNet = MacroDataPoint{
		Symbol: "TAIWAN_FOREIGN", Value: flow.ForeignInvestorNet,
		ChangePct: percentChange(flow.ForeignInvestorNet, prev.ForeignInvestorNet),
		Timestamp: flowTs,
	}
	snap.ForeignDealerNet = MacroDataPoint{
		Symbol: "TAIWAN_FOREIGN_DEALER", Value: flow.ForeignDealerNet,
		ChangePct: percentChange(flow.ForeignDealerNet, prev.ForeignDealerNet),
		Timestamp: flowTs,
	}
	snap.DomesticFundNet = MacroDataPoint{
		Symbol: "TAIWAN_DOMESTIC", Value: flow.DomesticFundNet,
		ChangePct: percentChange(flow.DomesticFundNet, prev.DomesticFundNet),
		Timestamp: flowTs,
	}
	snap.DealerNet = MacroDataPoint{
		Symbol: "TAIWAN_DEALER", Value: flow.DealerNet,
		ChangePct: percentChange(flow.DealerNet, prev.DealerNet),
		Timestamp: flowTs,
	}
	snap.DealerSelfNet = MacroDataPoint{
		Symbol: "TAIWAN_DEALER_SELF", Value: flow.DealerSelfNet,
		ChangePct: percentChange(flow.DealerSelfNet, prev.DealerSelfNet),
		Timestamp: flowTs,
	}
	snap.DealerHedgingNet = MacroDataPoint{
		Symbol: "TAIWAN_DEALER_HEDGING", Value: flow.DealerHedgingNet,
		ChangePct: percentChange(flow.DealerHedgingNet, prev.DealerHedgingNet),
		Timestamp: flowTs,
	}
	return snap
}

// LatestSavedSnapshot returns a MacroDataSnapshot built from the most recent
// persisted daily flow file. It is intended as a fallback for the gateway
// channel adapter when the live TWSE API has no data (holidays/weekends).
func (t *TWSECapitalFlowProvider) LatestSavedSnapshot(ctx context.Context) (MacroDataSnapshot, error) {
	entries, err := os.ReadDir(t.storageDir)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("read storage dir: %w", err)
	}
	const suffix = "_capital_flow.json"
	var latestDate string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, suffix) {
			continue
		}
		datePart := strings.TrimSuffix(name, suffix)
		if len(datePart) != 8 {
			continue
		}
		if datePart > latestDate {
			latestDate = datePart
		}
	}
	if latestDate == "" {
		return MacroDataSnapshot{}, fmt.Errorf("no saved capital flow snapshot")
	}
	path := filepath.Join(t.storageDir, latestDate+suffix)
	data, err := os.ReadFile(path)
	if err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("read saved flow: %w", err)
	}
	var flow TWSECapitalFlow
	if err := json.Unmarshal(data, &flow); err != nil {
		return MacroDataSnapshot{}, fmt.Errorf("decode saved flow: %w", err)
	}
	if flow.Date == "" {
		flow.Date = latestDate
	}
	return t.buildSnapshotFromFlow(flow), nil
}

func (t *TWSECapitalFlowProvider) loadPreviousFlow(currentDate string) (TWSECapitalFlow, error) {
	current, err := time.ParseInLocation("20060102", currentDate, time.FixedZone("CST", 8*60*60))
	if err != nil {
		return TWSECapitalFlow{}, err
	}
	for i := 1; i <= 7; i++ {
		prevDate := current.AddDate(0, 0, -i).Format("20060102")
		path := filepath.Join(t.storageDir, prevDate+"_capital_flow.json")
		data, err := os.ReadFile(path)
		if err == nil {
			var flow TWSECapitalFlow
			if err := json.Unmarshal(data, &flow); err == nil {
				return flow, nil
			}
		}
	}
	return TWSECapitalFlow{}, fmt.Errorf("no previous flow available in 7 days before %s", currentDate)
}

// FetchSymbolFlow retrieves institutional investor flow for a single symbol on a given date.
func (t *TWSECapitalFlowProvider) FetchSymbolFlow(ctx context.Context, symbol, dateStr string) (SymbolFlow, error) {
	rows, err := t.fetchDateRows(ctx, dateStr)
	if err != nil {
		return SymbolFlow{}, err
	}
	for _, row := range rows {
		if len(row) < 12 {
			continue
		}
		if row[0] != symbol {
			continue
		}
		return SymbolFlow{
			Symbol:             symbol,
			Name:               row[1],
			ForeignInvestorNet: parseTWDVolume(row[4]) / 1e3,
			DomesticFundNet:    parseTWDVolume(row[10]) / 1e3,
			DealerNet:          parseTWDVolume(row[11]) / 1e3,
			Date:               dateStr,
		}, nil
	}
	return SymbolFlow{}, fmt.Errorf("symbol %s not found for %s", symbol, dateStr)
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
	return TWSECapitalFlow{}, fmt.Errorf("%w: no TWSE capital flow data available in the last 7 days", ErrNoData)
}

func (t *TWSECapitalFlowProvider) fetchDate(ctx context.Context, dateStr string) (TWSECapitalFlow, error) {
	rows, err := t.fetchDateRows(ctx, dateStr)
	if err != nil {
		return TWSECapitalFlow{}, err
	}
	var totalForeign, totalForeignDealer, totalDomestic, totalDealer, totalDealerSelf, totalDealerHedging float64
	for _, row := range rows {
		if len(row) < 18 {
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
		totalForeign += parseTWDVolume(row[4])
		totalForeignDealer += parseTWDVolume(row[7])
		totalDomestic += parseTWDVolume(row[10])
		totalDealer += parseTWDVolume(row[11])
		totalDealerSelf += parseTWDVolume(row[14])
		totalDealerHedging += parseTWDVolume(row[17])
	}

	flow := TWSECapitalFlow{
		Date:               dateStr,
		ForeignInvestorNet: totalForeign / 1e8,
		ForeignDealerNet:   totalForeignDealer / 1e8,
		DomesticFundNet:    totalDomestic / 1e8,
		DealerNet:          totalDealer / 1e8,
		DealerSelfNet:      totalDealerSelf / 1e8,
		DealerHedgingNet:   totalDealerHedging / 1e8,
		TotalNet:           (totalForeign + totalForeignDealer + totalDomestic + totalDealer) / 1e8,
	}
	return flow, nil
}

func (t *TWSECapitalFlowProvider) fetchDateRows(ctx context.Context, dateStr string) ([][]string, error) {
	if err := t.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit: %w", err)
	}
	url := fmt.Sprintf(constants.TWSEBaseURL+"/rwd/zh/fund/T86?response=json&date=%s&selectType=ALLBUT0999", dateStr)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var apiResp twseT86Response
	if err := DecodeJSON(bytes.NewReader(body), resp.Header.Get("Content-Type"), &apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if apiResp.Stat != "OK" || len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("TWSE API returned no data: %s", apiResp.Stat)
	}
	return apiResp.Data, nil
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
