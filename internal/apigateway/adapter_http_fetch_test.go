package apigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestMarketDataBackedAdaptersFetchAndHealthCheckUseInjectedHTTPClient(t *testing.T) {
	tests := []struct {
		name      string
		adapter   func(*http.Client, string) DataProvider
		responder func(http.ResponseWriter, *http.Request)
		channelID string
	}{
		{
			name: "day trading",
			adapter: func(client *http.Client, _ string) DataProvider {
				p := marketdata.NewDayTradingProvider()
				p.SetHTTPClient(client)
				return &DayTradingChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: twseTableResponder(map[string]any{"stat": "OK", "tables": []map[string]any{{"data": [][]string{{"1,000", "10.5", "2,000", "20.5", "3,000", "30.5"}}}}}),
			channelID: "day_trading",
		},
		{
			name: "exchange rate",
			adapter: func(client *http.Client, _ string) DataProvider {
				p := marketdata.NewExchangeRateProvider()
				p.SetHTTPClient(client)
				return &ExchangeRateChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: jsonResponder(map[string]any{"result": "success", "base_code": "USD", "rates": map[string]float64{"TWD": 31.2, "JPY": 157.1}}),
			channelID: "exchange_rate",
		},
		{
			name: "frankfurter fx",
			adapter: func(client *http.Client, _ string) DataProvider {
				p := marketdata.NewFrankfurterFXProvider()
				p.SetHTTPClient(client)
				return &FrankfurterFXChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: jsonResponder(map[string]any{"date": "2026-06-12", "base": "USD", "rates": map[string]float64{"JPY": 157.1}}),
			channelID: "frankfurter_fx",
		},
		{
			name: "tej",
			adapter: func(client *http.Client, _ string) DataProvider {
				marketdata.ResetSharedTEJClient()
				p := marketdata.GetSharedTEJClient("test-key")
				p.SetHTTPClient(client)
				return &TEJChannelAdapter{client: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: jsonResponder(map[string]any{"datatable": map[string]any{"data": [][]any{{"2330", "2026-06-12", 900.0, 910.0, 890.0, 905.0, 1000.0, 905000.0}}}}),
			channelID: "tej",
		},
		{
			name: "twse capital flow",
			adapter: func(client *http.Client, dir string) DataProvider {
				p := marketdata.NewTWSECapitalFlowProvider(dir)
				p.SetHTTPClient(client)
				return &TWSECapitalFlowChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: jsonResponder(map[string]any{"stat": "OK", "data": [][]string{{"2330", "台積電", "0", "0", "100,000,000", "0", "0", "0", "0", "0", "20,000,000", "5,000,000"}}}),
			channelID: "twse_capital_flow",
		},
		{
			name: "twse margin",
			adapter: func(client *http.Client, dir string) DataProvider {
				p := marketdata.NewTWSEMarginBalanceProvider(dir)
				p.SetHTTPClient(client)
				return &TWSEMarginChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: twseTableResponder(map[string]any{"stat": "OK", "tables": []map[string]any{{"data": [][]string{{"融資金額", "0", "0", "0", "900,000", "1,000,000"}, {"融券", "0", "0", "0", "100", "120"}}}}}),
			channelID: "twse_margin",
		},
		{
			name: "twse etf",
			adapter: func(client *http.Client, _ string) DataProvider {
				p := marketdata.NewTWSEETFProvider()
				p.SetHTTPClient(client)
				return &TWSEETFChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: twseTableResponder(map[string]any{"stat": "OK", "tables": []map[string]any{{"data": [][]string{{"0050", "1,000", "2,000", "300"}}}}}),
			channelID: "twse_etf",
		},
		{
			name: "twse odd lot",
			adapter: func(client *http.Client, _ string) DataProvider {
				p := marketdata.NewTWSEOddLotProvider()
				p.SetHTTPClient(client)
				return &TWSEOddLotChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: twseTableResponder(map[string]any{"stat": "OK", "tables": []map[string]any{{"data": [][]string{{"2330", "台積電", "1,000", "0", "0", "900", "0", "0", "910"}}}}}),
			channelID: "twse_oddlot",
		},
		{
			name: "twse sector index",
			adapter: func(client *http.Client, dir string) DataProvider {
				p := marketdata.NewTWSESectorIndexProvider(dir)
				p.SetHTTPClient(client)
				return &TWSESectorIndexChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}
			},
			responder: jsonResponder([]map[string]string{{"指數": "半導體類指數", "收盤指數": "1,234.56", "漲跌百分比": "1.23"}}),
			channelID: "twse_sector_index",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.responder))
			defer server.Close()

			fetchAdapter := tt.adapter(rewriteHTTPClient(server.URL), t.TempDir())
			result, err := fetchAdapter.Fetch(context.Background())
			if err != nil {
				t.Fatalf("Fetch() error = %v", err)
			}
			if result == nil || result.Meta.ChannelID != tt.channelID || len(result.Data) == 0 {
				t.Fatalf("Fetch() result = %#v, want data for channel %s", result, tt.channelID)
			}

			healthAdapter := tt.adapter(rewriteHTTPClient(server.URL), t.TempDir())
			status, err := healthAdapter.HealthCheck(context.Background())
			if err != nil {
				t.Fatalf("HealthCheck() error = %v", err)
			}
			if status.Status != "ok" || (status.CheckType != "liveness" && status.CheckType != "readiness") {
				t.Fatalf("HealthCheck() = %#v, want ok (liveness or readiness)", status)
			}
		})
	}
}

func TestMarketDataBackedAdaptersHealthCheckReportsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom"}`))
	}))
	defer server.Close()

	p := marketdata.NewExchangeRateProvider()
	p.SetHTTPClient(rewriteHTTPClient(server.URL))
	adapter := &ExchangeRateChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}

	status, err := adapter.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("HealthCheck() error = nil, want HTTP failure")
	}
	if status.Status != "error" || status.LastError == "" || status.CheckType != "liveness" {
		t.Fatalf("HealthCheck() = %#v, want error status with last_error", status)
	}
}

func rewriteHTTPClient(serverURL string) *http.Client {
	return &http.Client{Transport: rewriteTransport{target: serverURL, base: http.DefaultTransport}}
}

type rewriteTransport struct {
	target string
	base   http.RoundTripper
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	targetReq, err := http.NewRequestWithContext(req.Context(), req.Method, t.target+req.URL.RequestURI(), req.Body)
	if err != nil {
		return nil, err
	}
	clone.URL = targetReq.URL
	clone.Host = targetReq.Host
	return t.base.RoundTrip(clone)
}

func jsonResponder(payload any) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func twseTableResponder(payload map[string]any) func(http.ResponseWriter, *http.Request) {
	return jsonResponder(payload)
}

var (
	_ DataProvider = (*DayTradingChannelAdapter)(nil)
	_ DataProvider = (*ExchangeRateChannelAdapter)(nil)
	_ DataProvider = (*FrankfurterFXChannelAdapter)(nil)
	_ DataProvider = (*TEJChannelAdapter)(nil)
	_ DataProvider = (*TWSECapitalFlowChannelAdapter)(nil)
	_ DataProvider = (*TWSEMarginChannelAdapter)(nil)
	_ DataProvider = (*TWSEETFChannelAdapter)(nil)
	_ DataProvider = (*TWSEOddLotChannelAdapter)(nil)
	_ DataProvider = (*MarketVolumeChannelAdapter)(nil)
	_ DataProvider = (*TWSESectorIndexChannelAdapter)(nil)
)

func TestTaifexChannelAdapterHealthCheckUsesInjectedHTTPClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(jsonResponder([]map[string]string{{
		"Date":                time.Now().Format("2006/01/02"),
		"PutVolume":           "100",
		"CallVolume":          "200",
		"PutCallVolumeRatio%": "50",
		"PutOI":               "300",
		"CallOI":              "400",
		"PutCallOIRatio%":     "75",
	}})))
	defer server.Close()

	p := marketdata.NewTAIFEXProvider()
	p.SetHTTPClient(rewriteHTTPClient(server.URL))
	adapter := &TaifexChannelAdapter{provider: p, limiter: rate.NewLimiter(rate.Inf, 1)}

	status, err := adapter.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck() error = %v", err)
	}
	if status.Status != "ok" || status.CheckType != "liveness" {
		t.Fatalf("HealthCheck() = %#v, want ok liveness", status)
	}
}
