package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	withUnlimitedTWSELimiter(t)
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

func TestFetchDateFlows(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// 3 valid rows (19 columns) + 1 short row (< 12) + 1 grand-total
		// row (empty symbol) that must be skipped.
		fmt.Fprint(w, `{"stat":"OK","data":[
			["2330","台積電","1000","500","500","0","0","0","800","300","500","400","0","0","0","0","0","0","1400"],
			["2317","鴻海","2000","1000","1000","0","0","0","600","200","400","300","0","0","0","0","0","0","1700"],
			["0050","元大台灣50","500","250","250","0","0","0","400","100","300","200","0","0","0","0","0","0","750"],
			["0056"],
			["","總計","3000","1500","1500","0","0","0","1400","400","1000","700","0","0","0","0","0","0","3200"]
		]}`)
	}))
	defer server.Close()

	p := NewTWSECapitalFlowProvider("")
	p.SetHTTPClient(&http.Client{Transport: &testT86RoundTripper{serverURL: server.URL}})

	flows, err := p.FetchDateFlows(context.Background(), "20260701")
	if err != nil {
		t.Fatalf("FetchDateFlows: %v", err)
	}
	if len(flows) != 3 {
		t.Fatalf("len(flows) = %d, want 3 (malformed + grand-total rows skipped)", len(flows))
	}

	want := []SymbolFlow{
		{Symbol: "2330", Name: "台積電", ForeignInvestorNet: 0.5, DomesticFundNet: 0.5, DealerNet: 0.4, Date: "20260701"},
		{Symbol: "2317", Name: "鴻海", ForeignInvestorNet: 1.0, DomesticFundNet: 0.4, DealerNet: 0.3, Date: "20260701"},
		{Symbol: "0050", Name: "元大台灣50", ForeignInvestorNet: 0.25, DomesticFundNet: 0.3, DealerNet: 0.2, Date: "20260701"},
	}
	for i, w := range want {
		got := flows[i]
		if got.Symbol != w.Symbol || got.Name != w.Name || got.Date != w.Date {
			t.Errorf("flows[%d] = %+v, want symbol/name/date %q/%q/%q", i, got, w.Symbol, w.Name, w.Date)
		}
		if got.ForeignInvestorNet != w.ForeignInvestorNet {
			t.Errorf("flows[%d].ForeignInvestorNet = %v, want %v", i, got.ForeignInvestorNet, w.ForeignInvestorNet)
		}
		if got.DomesticFundNet != w.DomesticFundNet {
			t.Errorf("flows[%d].DomesticFundNet = %v, want %v", i, got.DomesticFundNet, w.DomesticFundNet)
		}
		if got.DealerNet != w.DealerNet {
			t.Errorf("flows[%d].DealerNet = %v, want %v", i, got.DealerNet, w.DealerNet)
		}
	}
}

func TestFetchDateFlows_NoDataWrapsErrNoData(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"stat":"OK","data":[]}`)
	}))
	defer server.Close()

	p := NewTWSECapitalFlowProvider("")
	p.SetHTTPClient(&http.Client{Transport: &testT86RoundTripper{serverURL: server.URL}})

	_, err := p.FetchDateFlows(context.Background(), "20260704")
	if err == nil {
		t.Fatal("expected error for empty T86 response")
	}
	if !errors.Is(err, ErrNoData) {
		t.Fatalf("error = %v, want it to wrap ErrNoData (holiday classification)", err)
	}
}

func TestFetchSnapshot_ChangePctFromPreviousDay(t *testing.T) {
	withUnlimitedTWSELimiter(t)
	storageDir := t.TempDir()
	p := NewTWSECapitalFlowProvider(storageDir)

	prevDate := time.Now().UTC().AddDate(0, 0, -1).Format("20060102")
	prev := TWSECapitalFlow{
		Date:               prevDate,
		ForeignInvestorNet: 100,
		DomesticFundNet:    50,
		DealerNet:          25,
		TotalNet:           175,
	}
	data, _ := json.MarshalIndent(prev, "", "  ")
	if err := os.WriteFile(filepath.Join(storageDir, prevDate+"_capital_flow.json"), data, 0o644); err != nil {
		t.Fatalf("seed prev file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"stat":"OK","data":[["2330","台積電","1000","500","500","0","0","0","800","300","500","400","0","0","0","0","0","0","1400"]]}`)
	}))
	defer server.Close()
	p.SetHTTPClient(&http.Client{Transport: &testT86RoundTripper{serverURL: server.URL}})

	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot: %v", err)
	}

	// Regression for G-12: before the fix ChangePct was always 0 because
	// FetchSnapshot never read the previous trading day's file. After the
	// fix it should be populated. The exact value depends on the test HTTP
	// response and the seeded prev value, so we only assert non-zero here.
	if snap.ForeignInvestorNet.ChangePct == 0 {
		t.Error("ForeignInvestorNet.ChangePct = 0, expected non-zero when previous-day file exists")
	}
	if snap.DomesticFundNet.ChangePct == 0 {
		t.Error("DomesticFundNet.ChangePct = 0, expected non-zero when previous-day file exists")
	}
	if snap.DealerNet.ChangePct == 0 {
		t.Error("DealerNet.ChangePct = 0, expected non-zero when previous-day file exists")
	}
}

func TestLoadPreviousFlow_FoundAndMissing(t *testing.T) {
	storageDir := t.TempDir()
	p := NewTWSECapitalFlowProvider(storageDir)

	if _, err := p.loadPreviousFlow("20260714"); err == nil {
		t.Error("expected error when no previous file exists")
	}

	prevDate := "20260713"
	prev := TWSECapitalFlow{Date: prevDate, ForeignInvestorNet: 42}
	data, _ := json.MarshalIndent(prev, "", "  ")
	if err := os.WriteFile(filepath.Join(storageDir, prevDate+"_capital_flow.json"), data, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	got, err := p.loadPreviousFlow("20260714")
	if err != nil {
		t.Fatalf("loadPreviousFlow: %v", err)
	}
	if got.ForeignInvestorNet != 42 {
		t.Errorf("ForeignInvestorNet = %v, want 42", got.ForeignInvestorNet)
	}
}
