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

// writeParametersJSON copies the repo's configs/parameters.json to a temp
// dir, applies test-only overrides, and rewires config.GetParametersConfig()
// to read it. We start from the real file (rather than a hand-rolled
// partial config) because LoadParametersConfig → Validate() enforces
// invariant sums (e.g. sector_rotation.base_allocations must equal
// 1.0±0.01) that are impractical to maintain by hand here. Copying the
// real file keeps every other field canonical and makes the override
// surface obvious.
func writeParametersJSON(t *testing.T, overrides map[string]any) string {
	t.Helper()

	srcPath := findRepoParametersJSON(t)
	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read repo parameters.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(srcData, &cfg); err != nil {
		t.Fatalf("parse repo parameters.json: %v", err)
	}

	for section, vals := range overrides {
		if secMap, ok := vals.(map[string]any); ok {
			if cfgSection, ok := cfg[section].(map[string]any); ok {
				maps.Copy(cfgSection, secMap)
			} else {
				cfg[section] = secMap
			}
		}
	}

	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "parameters.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal parameters: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write parameters: %v", err)
	}

	t.Setenv("ATLAS_PARAMETERS_CONFIG", path)
	config.SetParametersConfigPath(path)
	config.ResetParametersConfig()
	return path
}

// findRepoParametersJSON walks up from the working directory until it finds
// configs/parameters.json. Tests run from the package directory, so this
// locates the canonical file even when the package is moved.
func findRepoParametersJSON(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for range 6 {
		candidate := filepath.Join(dir, "configs", "parameters.json")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate configs/parameters.json from working directory")
	return ""
}
