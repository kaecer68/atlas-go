package apigateway

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// yahooChartResponse returns a minimal valid Yahoo Finance v8 chart JSON.
func yahooChartResponse(_ string, closes []float64) string {
	if len(closes) == 0 {
		closes = []float64{100.0, 101.0}
	}
	type quote struct {
		Close []float64 `json:"close"`
	}
	type result struct {
		Meta struct {
			RegularMarketTime  int64   `json:"regularMarketTime"`
			RegularMarketPrice float64 `json:"regularMarketPrice"`
		} `json:"meta"`
		Indicators struct {
			Quote []quote `json:"quote"`
		} `json:"indicators"`
	}
	type chart struct {
		Result []result `json:"result"`
	}
	type resp struct {
		Chart chart `json:"chart"`
	}
	r := resp{Chart: chart{Result: []result{{
		Meta: struct {
			RegularMarketTime  int64   `json:"regularMarketTime"`
			RegularMarketPrice float64 `json:"regularMarketPrice"`
		}{RegularMarketTime: 1700000000, RegularMarketPrice: closes[len(closes)-1]},
		Indicators: struct {
			Quote []quote `json:"quote"`
		}{Quote: []quote{{Close: closes}}},
	}}}}
	b, _ := json.Marshal(r)
	return string(b)
}

// setupYahooMockServer installs a test client on the global Yahoo session that
// redirects all Yahoo Finance hosts to the returned test server. The test server
// responds with a valid chart payload for any /v8/finance/chart/* path.
func setupYahooMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(yahooChartResponse("", nil)))
	}))
	t.Cleanup(server.Close)

	client := server.Client()
	client.Transport = &mockRedirectingTransport{
		hostMap: map[string]string{
			"query1.finance.yahoo.com": server.URL,
			"query2.finance.yahoo.com": server.URL,
			"fc.yahoo.com":             server.URL,
		},
		base: client.Transport,
	}
	marketdata.SetYahooSessionClient(client)
	return server
}

// mockRedirectingTransport rewrites requests for configured hosts to a test server.
// This lets apigateway adapter tests mock external APIs without modifying production
// providers that do not expose a SetHTTPClient hook.
type mockRedirectingTransport struct {
	hostMap map[string]string // original host -> test server base URL
	base    http.RoundTripper
}

func (t *mockRedirectingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if mapped, ok := t.hostMap[req.URL.Host]; ok {
		if mappedURL, err := url.Parse(mapped); err == nil {
			req = req.Clone(req.Context())
			req.URL.Scheme = mappedURL.Scheme
			req.URL.Host = mappedURL.Host
		}
	}
	return t.base.RoundTrip(req)
}

// withClientMockTransport returns an *http.Client whose transport redirects
// configured hosts to server. Safe for providers that expose SetHTTPClient.
func withClientMockTransport(server *httptest.Server, hosts ...string) *http.Client {
	client := server.Client()
	m := make(map[string]string, len(hosts))
	for _, h := range hosts {
		m[h] = server.URL
	}
	client.Transport = &mockRedirectingTransport{hostMap: m, base: client.Transport}
	return client
}

// writeParametersJSON writes a minimal parameters.json to a temp dir and sets
// ATLAS_PARAMETERS_CONFIG so config.GetParametersConfig() returns it.
func writeParametersJSON(t *testing.T, overrides map[string]any) string {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")

	cfg := map[string]any{
		"marketdata": map[string]any{
			"bdi_endpoint":          map[string]any{"value": ""},
			"bdi_api_timeout_sec":   map[string]any{"value": 10},
			"twse_api_rate_limit":   map[string]any{"value": 1000.0},
			"twse_api_rate_burst":   map[string]any{"value": 10},
			"twse_api_timeout_sec":  map[string]any{"value": 10},
			"fugle_api_timeout_sec": map[string]any{"value": 10},
			"fugle_rate_limit":      map[string]any{"value": 60},
			"fubon_api_timeout_sec": map[string]any{"value": 10},
			"fubon_intraday_limit":  map[string]any{"value": 100},
			"tej_api_timeout_sec":   map[string]any{"value": 10},
			"tej_calls_per_second":  map[string]any{"value": 10},
		},
		"risk_gate": map[string]any{
			"pre_trade": map[string]any{
				"max_sector_exposure_pct": map[string]any{"value": 1.0},
			},
		},
		"engine": map[string]any{
			"sector_rotation": map[string]any{
				"min_allocation": map[string]any{"value": 0.0},
				"max_allocation": map[string]any{"value": 1.0},
				"base_allocations": map[string]any{
					"value": map[string]float64{"tech": 0.25},
				},
				"macro_adjustments":  map[string]any{"value": map[string]any{}},
				"carry_adjustments":  map[string]any{"value": map[string]any{}},
				"rotate_adjustments": map[string]any{"value": map[string]any{}},
			},
		},
		"orchestrator": map[string]any{
			"sector_rotation_base_allocations":   map[string]any{"value": map[string]float64{"tech": 0.25}},
			"sector_rotation_macro_adjustments":  map[string]any{"value": map[string]any{}},
			"sector_rotation_flow_adjustments":   map[string]any{"value": map[string]any{}},
			"sector_constraints_risk_off":        map[string]any{"value": map[string]float64{}},
			"sector_constraints_carry_trade":     map[string]any{"value": map[string]float64{}},
			"sector_constraints_sector_rotation": map[string]any{"value": map[string]float64{}},
		},
		"industry": map[string]any{
			"sector_weights": map[string]any{"value": map[string]float64{"tech": 0.25}},
		},
		"narrative": map[string]any{
			"gold_change_pct_threshold":           map[string]any{"value": 0.5},
			"usdtwd_change_pct_threshold":         map[string]any{"value": 0.5},
			"semiconductor_export_drop_threshold": map[string]any{"value": 0.1},
			"retail_margin_zscore_threshold":      map[string]any{"value": 2.0},
			"tsmc_revenue_yoy_threshold":          map[string]any{"value": 0.1},
			"tsmc_revenue_positive_threshold":     map[string]any{"value": 0.0},
			"confidence_base_tsmc_revenue":        map[string]any{"value": 0.5},
			"sox_index_drop_threshold":            map[string]any{"value": 0.1},
		},
		"sector_executor": map[string]any{},
	}

	// Apply overrides by walking the nested map. Only one level of nesting is supported.
	for section, vals := range overrides {
		if secMap, ok := vals.(map[string]any); ok {
			if cfgSection, ok := cfg[section].(map[string]any); ok {
				maps.Copy(cfgSection, secMap)
			} else {
				cfg[section] = secMap
			}
		}
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal parameters: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write parameters: %v", err)
	}

	t.Setenv("ATLAS_PARAMETERS_CONFIG", path)
	config.ResetParametersConfig()
	return path
}
