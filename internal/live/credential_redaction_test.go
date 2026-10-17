package live

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// Fake broker credentials. A broker's 4xx/5xx body echoes the account and the
// credential prefix it received, so before FU-20261016-12 these values reached
// the order records and the risk alerts verbatim.
const (
	liveRedactionFixtureKey    = "FAKE-live-api-key-1a2b3c4d5e6f"
	liveRedactionFixtureSecret = "FAKE-live-api-secret-7a8b9c0d1e2f"
	liveRedactionFixtureAcct   = "ACC-SECRET-99887766"
)

// liveRedactionBody is the shape of a rejected order as brokers actually
// answer it: an error code, the account as its own member, and the credential
// quoted twice (as a header dump and as a query parameter).
//
// The account is a MEMBER here because the vendor-neutral HTTP adapter does
// not hold the account identifier and therefore cannot pass it as a literal.
// A body that quotes the account inside free prose is only covered by the
// adapters that hold it (TWSE, Fubon DMA) — see the "known limits" note in
// FU-20261016-12.
func liveRedactionBody() string {
	return `{"code":"ORDER_REJECTED","account_id":"` + liveRedactionFixtureAcct +
		`","message":"invalid X-Api-Key ` + liveRedactionFixtureKey + `","detail":"api_key=` + liveRedactionFixtureSecret + `"}`
}

// newRedactionTestTWSEAdapter reuses the package helper with fixture secrets.
func newRedactionTestTWSEAdapter(t *testing.T, handler http.HandlerFunc) *TWSEBrokerAdapter {
	t.Helper()
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	cb := NewCircuitBreaker("", "")
	cb.state = CircuitNormal
	return NewTWSEBrokerAdapter(TWSEBrokerAdapterConfig{
		BaseURL:              ts.URL,
		APIKey:               liveRedactionFixtureKey,
		APISecret:            liveRedactionFixtureSecret,
		AccountID:            liveRedactionFixtureAcct,
		Timeout:              2 * time.Second,
		MaxRequestsPerSecond: 1000,
	}, cb)
}

// TestTWSEBrokerAdapter_Errors_HideCredentials is the negative proof for the
// three TWSE error paths (submit / status / cancel).
func TestTWSEBrokerAdapter_Errors_HideCredentials(t *testing.T) {
	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 10, Price: 99.5}

	t.Run("submit order rejected", func(t *testing.T) {
		adapter := newRedactionTestTWSEAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(liveRedactionBody()))
		})
		_, err := adapter.SubmitOrder(context.Background(), order)
		if err == nil {
			t.Fatal("expected an error for HTTP 400")
		}
		assertNoBrokerCredential(t, "twse submit order", err.Error(), "status=400", "ORDER_REJECTED", "invalid X-Api-Key")
	})

	t.Run("query order status fails", func(t *testing.T) {
		adapter := newRedactionTestTWSEAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(liveRedactionBody()))
		})
		_, err := adapter.QueryOrderStatus(context.Background(), "TWSE-001")
		if err == nil {
			t.Fatal("expected an error for HTTP 500")
		}
		assertNoBrokerCredential(t, "twse query order status", err.Error(), "status=500", "ORDER_REJECTED")
	})

	t.Run("cancel order fails", func(t *testing.T) {
		adapter := newRedactionTestTWSEAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(liveRedactionBody()))
		})
		err := adapter.CancelOrder(context.Background(), "TWSE-404")
		if err == nil {
			t.Fatal("expected an error for HTTP 404")
		}
		assertNoBrokerCredential(t, "twse cancel order", err.Error(), "status=404", "ORDER_REJECTED")
	})
}

// TestHTTPBrokerAdapter_Errors_HideCredentials covers the shared HTTP adapter:
// the 4xx body path and the transport path (which formats the whole URL).
func TestHTTPBrokerAdapter_Errors_HideCredentials(t *testing.T) {
	order := domain.Order{Symbol: "2330", Side: domain.SideBuy, Quantity: 10, Price: 99.5}

	t.Run("broker rejects the request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(liveRedactionBody()))
		}))
		defer server.Close()

		adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
			BaseURL:     server.URL,
			APIKey:      liveRedactionFixtureKey,
			APISecret:   liveRedactionFixtureSecret,
			KeyID:       "kid-1",
			Timeout:     2 * time.Second,
			MaxAttempts: 1,
			Client:      server.Client(),
		})
		_, err := adapter.SubmitOrder(context.Background(), order)
		if err == nil {
			t.Fatal("expected an error for HTTP 403")
		}
		assertNoBrokerCredential(t, "http broker rejected", err.Error(), "status=403", "ORDER_REJECTED")
	})

	t.Run("transport error keeps the URL shape but not the credential", func(t *testing.T) {
		closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		closedURL := closed.URL
		closed.Close()

		adapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
			BaseURL:     closedURL + "/orders?api_key=" + liveRedactionFixtureKey,
			APIKey:      liveRedactionFixtureKey,
			APISecret:   liveRedactionFixtureSecret,
			Timeout:     2 * time.Second,
			MaxAttempts: 1,
		})
		_, err := adapter.SubmitOrder(context.Background(), order)
		if err == nil {
			t.Fatal("expected a transport error against a closed port")
		}
		assertNoBrokerCredential(t, "http broker transport", err.Error(), "http request failed", "connection refused")
	})
}

// TestFubonDMA_UnmarshalError_HidesCredentials covers the subprocess path: an
// unparseable line used to be copied into the error whole.
func TestFubonDMA_UnmarshalError_HidesCredentials(t *testing.T) {
	adapter := NewFubonDMAAdapter(FubonDMAAdapterConfig{
		PersonalID: "A123456789",
		APIKey:     liveRedactionFixtureKey,
		ScriptPath: "/nonexistent/fubon.py",
	})
	adapter.stdin = nopWriteCloser{}
	adapter.stdout = bufio.NewReader(strings.NewReader(
		`not-json {"api_key":"` + liveRedactionFixtureKey + `","account":"` + liveRedactionFixtureAcct + `"}` + "\n"))

	_, err := adapter.sendRequest(fubonDMARequest{Cmd: "ping"})
	if err == nil {
		t.Fatal("expected an error for an unparseable subprocess response")
	}
	assertNoBrokerCredential(t, "fubon dma unmarshal", err.Error(), "unmarshal response", "not-json")
}

// nopWriteCloser satisfies the subprocess stdin contract for the probe.
type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }

var _ io.WriteCloser = nopWriteCloser{}

// assertNoBrokerCredential is the shared probe: no fixture value survives, and
// the named diagnostics do.
func assertNoBrokerCredential(t *testing.T, what, got string, wantKept ...string) {
	t.Helper()
	t.Logf("%s -> %s", what, got)
	for _, leak := range []string{liveRedactionFixtureKey, liveRedactionFixtureSecret, liveRedactionFixtureAcct} {
		if strings.Contains(got, leak) {
			t.Errorf("%s leaks %q: %s", what, leak, got)
		}
	}
	for _, want := range wantKept {
		if !strings.Contains(got, want) {
			t.Errorf("%s dropped the diagnostic text %q: %s", what, want, got)
		}
	}
}
