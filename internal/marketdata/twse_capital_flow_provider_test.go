package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/kaecer68/atlas-go/internal/constants"
)

func TestParseTWDVolume(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1,234,567", 1234567},
		{"  -890  ", -890},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := parseTWDVolume(tt.input)
		if got != tt.want {
			t.Fatalf("parseTWDVolume(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestNewTWSECapitalFlowProvider(t *testing.T) {
	p := NewTWSECapitalFlowProvider(constants.StateCapitalFlow)
	if p.Name() != "twse_capital_flow" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

type testT86RoundTripper struct {
	serverURL string
}

func (rt *testT86RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	u := req.URL
	parsed, err := url.Parse(rt.serverURL)
	if err != nil {
		return nil, err
	}
	u.Scheme = parsed.Scheme
	u.Host = parsed.Host
	return http.DefaultTransport.RoundTrip(req)
}

func TestFetchSymbolFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"stat":"OK","data":[["2330","台積電","1000","500","500","0","0","0","800","300","500","400","0","0","0","0","0","0","1400"]]}`)
	}))
	defer server.Close()

	p := NewTWSECapitalFlowProvider("")
	p.SetHTTPClient(&http.Client{Transport: &testT86RoundTripper{serverURL: server.URL}})

	flow, err := p.FetchSymbolFlow(context.Background(), "2330", "20260701")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flow.Symbol != "2330" {
		t.Fatalf("symbol=%s", flow.Symbol)
	}
	if flow.Name != "台積電" {
		t.Fatalf("name=%s", flow.Name)
	}
	if flow.ForeignInvestorNet != 0.5 {
		t.Fatalf("foreign=%v", flow.ForeignInvestorNet)
	}
	if flow.DomesticFundNet != 0.5 {
		t.Fatalf("domestic=%v", flow.DomesticFundNet)
	}
	if flow.DealerNet != 0.4 {
		t.Fatalf("dealer=%v", flow.DealerNet)
	}
}
