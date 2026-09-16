package marketdata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func blsStubServer(t *testing.T, status string, entries string, httpStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		// The BLS API emits "message" as an ARRAY of strings (empty on success).
		// Mirroring the real wire format here is what would have caught the
		// production unmarshal failure ("cannot unmarshal array ... .message").
		msg := []string{}
		if status != "REQUEST_SUCCEEDED" {
			msg = []string{"stub rejection reason"}
		}
		body, _ := json.Marshal(map[string]any{
			"status":  status,
			"message": msg,
			"Results": map[string]any{
				"series": []map[string]any{
					{"seriesID": blsCPISeriesID, "data": json.RawMessage(entries)},
				},
			},
		})
		w.WriteHeader(httpStatus)
		_, _ = w.Write(body)
	}))
}

func newStubCPIProvider(t *testing.T, srv *httptest.Server) *BLSCPIProvider {
	t.Helper()
	p := NewBLSCPIProvider()
	p.endpoint = srv.URL
	p.nowFn = func() time.Time { return time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC) }
	return p
}

// TestBLSCPIProvider_DerivesYoYAndMoM is the core contract test: the provider
// must turn monthly CPI index levels into the YoY percentage the narrative
// detectors consume (MarketNarrativeData.CPIYoY).
func TestBLSCPIProvider_DerivesYoYAndMoM(t *testing.T) {
	entries := `[
		{"year":"2026","period":"M08","periodName":"August","value":"329.6"},
		{"year":"2026","period":"M07","periodName":"July","value":"328.0"},
		{"year":"2025","period":"M08","periodName":"August","value":"317.0"},
		{"year":"2024","period":"M08","periodName":"August","value":"312.0"},
		{"year":"2026","period":"M13","periodName":"Annual","value":"320.0"}
	]`
	srv := blsStubServer(t, "REQUEST_SUCCEEDED", entries, http.StatusOK)
	defer srv.Close()

	snap, err := newStubCPIProvider(t, srv).FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("fetch snapshot: %v", err)
	}

	wantYoY := (329.6 - 317.0) / 317.0 * 100
	if diff := snap.CPIYoY.Value - wantYoY; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("CPIYoY = %v, want %v", snap.CPIYoY.Value, wantYoY)
	}
	wantMoM := (329.6 - 328.0) / 328.0 * 100
	if diff := snap.CPIYoY.ChangePct - wantMoM; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("MoM = %v, want %v", snap.CPIYoY.ChangePct, wantMoM)
	}
	if snap.CPIYoY.Symbol != blsCPISeriesID {
		t.Errorf("Symbol = %q, want %q", snap.CPIYoY.Symbol, blsCPISeriesID)
	}
	// The M13 annual-average row must not displace the monthly latest value.
	if snap.CPIYoY.Timestamp == 0 {
		t.Error("Timestamp not set")
	}
}

// TestBLSCPIProvider_SkipsAnnualAverageOnly verifies M13 rows alone are rejected
// rather than silently treated as a monthly observation.
func TestBLSCPIProvider_SkipsAnnualAverageOnly(t *testing.T) {
	entries := `[{"year":"2026","period":"M13","periodName":"Annual","value":"320.0"}]`
	srv := blsStubServer(t, "REQUEST_SUCCEEDED", entries, http.StatusOK)
	defer srv.Close()

	if _, err := newStubCPIProvider(t, srv).FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when only annual-average rows are present")
	}
}

// TestBLSCPIProvider_RequiresSameMonthLastYear guards the YoY computation: a
// single month without a year-ago counterpart is an error, not a silent 0.
func TestBLSCPIProvider_RequiresSameMonthLastYear(t *testing.T) {
	entries := `[{"year":"2026","period":"M08","periodName":"August","value":"329.6"}]`
	srv := blsStubServer(t, "REQUEST_SUCCEEDED", entries, http.StatusOK)
	defer srv.Close()

	if _, err := newStubCPIProvider(t, srv).FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected error when no same-month prior-year observation exists")
	}
}

// TestBLSCPIProvider_ErrorsOnBadResponses covers the degradation paths that let
// the composite provider mark the channel failed instead of writing zeros.
func TestBLSCPIProvider_ErrorsOnBadResponses(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		httpStatus int
	}{
		{"http error", "REQUEST_SUCCEEDED", http.StatusInternalServerError},
		{"bls rejected", "REQUEST_NOT_PROCESSED", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := blsStubServer(t, tc.status, `[]`, tc.httpStatus)
			defer srv.Close()
			if _, err := newStubCPIProvider(t, srv).FetchSnapshot(context.Background()); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

// TestBLSCPIProvider_Name pins the channel id used by the channel contract.
func TestBLSCPIProvider_Name(t *testing.T) {
	if got := NewBLSCPIProvider().Name(); got != "us_cpi" {
		t.Fatalf("Name() = %q, want us_cpi", got)
	}
}

// TestBLSCPIProvider_AcceptsArrayShapedMessage is the production regression:
// the live BLS API returns {"status":"REQUEST_SUCCEEDED","message":[],...}
// and a string-typed field made every fetch fail with
// "cannot unmarshal array into Go struct field .message of type string".
func TestBLSCPIProvider_AcceptsArrayShapedMessage(t *testing.T) {
	entries := `[
		{"year":"2026","period":"M08","periodName":"August","value":"329.6"},
		{"year":"2025","period":"M08","periodName":"August","value":"317.0"}
	]`
	// Raw handler (not the shared stub) so the literal wire shape is pinned.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"REQUEST_SUCCEEDED","message":[],"Results":{"series":[{"seriesID":"CUUR0000SA0","data":` + entries + `}]}}`))
	}))
	defer srv.Close()

	snap, err := newStubCPIProvider(t, srv).FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("array-shaped message must parse, got: %v", err)
	}
	want := (329.6 - 317.0) / 317.0 * 100
	if diff := snap.CPIYoY.Value - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("CPIYoY = %v, want %v", snap.CPIYoY.Value, want)
	}
}

// TestBLSCPIProvider_ReportsArrayMessageOnRejection verifies the rejection path
// joins the array message instead of printing a Go type error.
func TestBLSCPIProvider_ReportsArrayMessageOnRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"REQUEST_NOT_PROCESSED","message":["Series does not exist","bad id"],"Results":{}}`))
	}))
	defer srv.Close()

	_, err := newStubCPIProvider(t, srv).FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected error on rejected status")
	}
	if !strings.Contains(err.Error(), "Series does not exist") || !strings.Contains(err.Error(), "bad id") {
		t.Errorf("error should carry the BLS message array, got: %v", err)
	}
}
