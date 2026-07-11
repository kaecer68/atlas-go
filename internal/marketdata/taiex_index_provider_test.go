package marketdata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTAIEXIndexProvider_Name(t *testing.T) {
	if got := NewTAIEXIndexProvider().Name(); got != "taiex_index" {
		t.Errorf("Name() = %q, want taiex_index", got)
	}
}

func TestTAIEXIndexProvider_FetchSnapshot_Success(t *testing.T) {
	mockResponse := `{
		"chart": {
			"result": [
				{
					"meta": {"regularMarketTime": 1714500000, "regularMarketPrice": 23000.0},
					"indicators": {"quote": [{"close": [22800.0, 23000.0]}]}
				}
			]
		}
	}`

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "^TWII") {
			t.Errorf("unexpected path: %s, expected ^TWII", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	snap, err := NewTAIEXIndexProvider().FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}
	if snap.TAIEX.Symbol != "^TWII" {
		t.Errorf("Symbol = %q, want ^TWII", snap.TAIEX.Symbol)
	}
	if snap.TAIEX.Value != 23000.0 {
		t.Errorf("Value = %v, want 23000.0", snap.TAIEX.Value)
	}
	expectedPct := 0.88 // (23000-22800)/22800*100 = 0.877..., rounds to 0.88
	if snap.TAIEX.ChangePct != expectedPct {
		t.Errorf("ChangePct = %v, want %v", snap.TAIEX.ChangePct, expectedPct)
	}
	if snap.TAIEX.Timestamp != 1714500000 {
		t.Errorf("Timestamp = %v, want 1714500000", snap.TAIEX.Timestamp)
	}
}

func TestTAIEXIndexProvider_FetchSnapshot_NoChartResult(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"chart":{"result":[]}}`))
	}))
	defer server.Close()

	origHosts := yahooHosts
	yahooHosts = []string{strings.TrimPrefix(server.URL, "https://")}
	defer func() { yahooHosts = origHosts }()
	SetYahooSessionClient(server.Client())

	_, err := NewTAIEXIndexProvider().FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("FetchSnapshot() expected error for empty chart result")
	}
}
