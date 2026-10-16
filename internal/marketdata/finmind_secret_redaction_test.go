package marketdata

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/logging"
)

// ─── FU-20260926-02 — upstream response text must never reach a sink raw ────
//
// The original F-item recorded that FinMind echoes a fragment of the caller's
// API key back in `token_tail` on auth/quota errors and that the client copied
// the whole body into the log line and into the error string. The 402 path was
// fixed by #2006 (sanitizeFinMindBody); these tests pin the sinks that were
// still open, and they pin the requirement with a NEGATIVE assertion (the fake
// secret must be absent) plus a POSITIVE one (the operator-actionable upstream
// fields must survive) so "redact everything" is not an acceptable fix.
//
// No test here talks to FinMind: every case drives an httptest server. The
// secret is a fixture value, never a real credential.
const redactionFixtureSecret = "FAKE-token-tail-7c41d9" // #nosec G101 -- fake fixture, never a credential

// redactionFixtureKey is the API key the fake client sends; it embeds the
// fixture secret so a leak can be detected both as `token_tail` and as the
// literal credential.
const redactionFixtureKey = "FAKE-api-key-" + redactionFixtureSecret

// captureSlog redirects the package logging sink for one test and returns the
// buffer that collects the formatted records.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	previous := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { logging.SetLogger(previous) })
	return buf
}

// assertNoSecretLeak fails when any forbidden fragment appears in what.
func assertNoSecretLeak(t *testing.T, what, s string) {
	t.Helper()
	for _, forbidden := range []string{redactionFixtureSecret, redactionFixtureKey, "token_tail"} {
		if strings.Contains(s, forbidden) {
			t.Errorf("%s leaks %q: %s", what, forbidden, s)
		}
	}
}

// TestFinMindClient_Non2xx_LogLineHidesTokenFragment is the acceptance
// condition of FU-20260926-02 for the LOG sink: the WARN line written on a
// non-2xx response must not carry `token_tail` or the key, while still naming
// the status and the upstream message an operator needs.
func TestFinMindClient_Non2xx_LogLineHidesTokenFragment(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantInLogs []string
	}{
		{
			name:   "400 token is illegal (fixture from FU-20260926-02)",
			status: http.StatusBadRequest,
			body:   `{"msg":"Token is illegal.","status":400,"token_tail":"` + redactionFixtureSecret + `"}`,
			wantInLogs: []string{
				"fetch_non_2xx", "400", "Token is illegal.",
			},
		},
		{
			name:   "402 upper limit with token_tail (production 2026-09-26 shape)",
			status: http.StatusPaymentRequired,
			body:   `{"msg":"Requests reach the upper limit.","status":402,"data":[],"token_tail":"` + redactionFixtureSecret + `"}`,
			wantInLogs: []string{
				"fetch_non_2xx", "402", "upper limit", "TaiwanStockPrice",
			},
		},
		{
			name:   "403 ip banned with a literal key in the body",
			status: http.StatusForbidden,
			body:   `{"msg":"ip banned (key ` + redactionFixtureKey + `)","retry_after":3,"token_tail":"` + redactionFixtureSecret + `"}`,
			wantInLogs: []string{
				"fetch_non_2xx", "403",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureSlog(t)
			c, _ := newUpstreamQuotaClient(t, t.TempDir(), redactionFixtureKey, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26")
			if err == nil {
				t.Fatal("expected a non-2xx error")
			}

			logged := buf.String()
			if logged == "" {
				t.Fatal("no log line captured; the non-2xx path must keep logging the upstream reason")
			}
			assertNoSecretLeak(t, "log line", logged)
			assertNoSecretLeak(t, "error", err.Error())
			for _, want := range tc.wantInLogs {
				if !strings.Contains(logged, want) {
					t.Errorf("log line lost the diagnostic field %q: %s", want, logged)
				}
			}
		})
	}
}

// TestFinMindClient_RetryExhausted5xx_HidesTokenFragment covers the sink that
// #2006 MISSED: fetchDataset delegates 429/5xx to the shared fetchWithRetry,
// which embeds the raw upstream body in its error ("http status N after M
// attempts: <body>") and throws the response away. No caller can sanitize that
// body — by the time FinMind sees the error, the text is already built.
func TestFinMindClient_RetryExhausted5xx_HidesTokenFragment(t *testing.T) {
	buf := captureSlog(t)
	c, hits := newUpstreamQuotaClient(t, t.TempDir(), redactionFixtureKey, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"msg":"upstream exploded","status":503,"token_tail":"` + redactionFixtureSecret + `"}`))
	})

	_, err := c.FetchDatasetRaw(context.Background(), "TaiwanStockPrice", "2330", "2026-09-26", "2026-09-26")
	if err == nil {
		t.Fatal("expected the exhausted retry to fail")
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (retryCfg.maxAttempts=1)", got)
	}

	assertNoSecretLeak(t, "retry error", err.Error())
	assertNoSecretLeak(t, "log line", buf.String())
	// Positive: the retry sink must stay diagnosable — status and message.
	for _, want := range []string{"503", "upstream exploded"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("retry error lost the diagnostic field %q: %s", want, err)
		}
	}
}

// TestFinMindClient_FetchTaiwan5SecIndex_APIError_HidesTokenFragment covers the
// second FinMind sink #2006 missed: FetchTaiwan5SecIndex has its own
// non-200-under-HTTP-200 envelope and embedded raw.Msg verbatim, while its twin
// in fetchDataset (finmind_client.go, "API error") sanitizes the same field.
func TestFinMindClient_FetchTaiwan5SecIndex_APIError_HidesTokenFragment(t *testing.T) {
	c, _ := newUpstreamQuotaClient(t, t.TempDir(), redactionFixtureKey, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"msg":"Token is illegal. (` + redactionFixtureKey + `)","status":400,"data":[]}`))
	})
	c.SetRateLimiter(rate.NewLimiter(rate.Inf, 1))

	_, err := c.FetchTaiwan5SecIndex(context.Background(), "2026-09-26")
	if err == nil {
		t.Fatal("expected an API error from the 200/status!=200 envelope")
	}
	assertNoSecretLeak(t, "5sec index error", err.Error())
	if !strings.Contains(err.Error(), "Token is illegal.") {
		t.Errorf("5sec index error must keep the upstream message, got %s", err)
	}
}

// TestFetchWithRetry_ExhaustedBody_HidesCredentialFragments pins the SHARED
// retry sink directly. FinMind is not the only caller: fugle, twse_openapi,
// twse_mi_index, export, tpex and taifex inherit this function, and after the
// final attempt it is the only surviving copy of the upstream body (the body
// itself is drained and closed on the way out). A credential echoed there
// therefore reaches every one of those channels, which is why the redaction
// lives here and not in each caller.
func TestFetchWithRetry_ExhaustedBody_HidesCredentialFragments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"msg":"upstream exploded","status":503,"token_tail":"` + redactionFixtureSecret + `"}`))
	}))
	defer srv.Close()

	cfg := retryConfig{maxAttempts: 2, baseBackoff: time.Millisecond}
	_, err := fetchWithRetry(context.Background(), srv.Client(), retryTestRequest(t, srv.URL), cfg)
	if err == nil {
		t.Fatal("fetchWithRetry = nil error, want an error after exhausting attempts")
	}
	assertNoSecretLeak(t, "shared retry error", err.Error())
	for _, want := range []string{"503", "upstream exploded", "2 attempts"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("shared retry error lost the diagnostic field %q: %s", want, err)
		}
	}
}
