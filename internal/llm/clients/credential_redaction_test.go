package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// llmRedactionFixtureKey is a fake credential shaped like the real thing. A
// provider's 401 body echoes the key it was given, so before FU-20261016-13 the
// key travelled into atlas_llm_* records and the dashboard with the error.
const llmRedactionFixtureKey = "sk-FAKE9f3c1f0b7a2e4d5c6b7a" // secret-scan-allow: 假 fixture（sk- 形狀測試用），非真憑證

// TestDeepSeek_Errors_HideAPIKey is the negative proof for FU-20261016-13 on
// the BaseClient path: the provider body must reach the caller without the
// credential, and the status plus the provider's message skeleton must survive.
func TestDeepSeek_Errors_HideAPIKey(t *testing.T) {
	t.Run("401 body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if got := r.Header.Get("Authorization"); !strings.Contains(got, llmRedactionFixtureKey) {
				t.Errorf("fixture: expected the client to send its key, got %q", got)
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided: ` + llmRedactionFixtureKey + `","type":"invalid_request_error"}}`))
		}))
		defer srv.Close()

		client := NewDeepSeekClient(llmRedactionFixtureKey, newTestBaseClient())
		client.BaseURL = srv.URL

		_, err := client.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil)
		if err == nil {
			t.Fatal("expected an error for HTTP 401")
		}
		assertNoLLMCredential(t, "deepseek 401 body", err.Error(),
			"http status 401", "Incorrect API key provided")
	})

	t.Run("401 body quoting a key with no recognisable prefix", func(t *testing.T) {
		// The value has no provider prefix, so the shape rule cannot find it:
		// only the literal supplied at the call site can remove it. Without
		// this case a dropped literal would hide behind the shape rule.
		const plainKey = "FAKEplainkey4b7c1d2e3f4a5b6c7d8e"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided: ` + plainKey + `"}}`))
		}))
		defer srv.Close()

		client := NewDeepSeekClient(plainKey, newTestBaseClient())
		client.BaseURL = srv.URL

		_, err := client.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil)
		if err == nil {
			t.Fatal("expected an error for HTTP 401")
		}
		got := err.Error()
		t.Logf("deepseek 401 body (plain key) -> %s", got)
		if strings.Contains(got, plainKey) {
			t.Errorf("deepseek leaks the literal credential: %s", got)
		}
		if !strings.Contains(got, "Incorrect API key provided") || !strings.Contains(got, "http status 401") {
			t.Errorf("deepseek dropped the diagnostics: %s", got)
		}
	})

	t.Run("body carrying a credential the caller never held", func(t *testing.T) {
		// The base client is the last line of defence for bodies that leak
		// material the caller cannot pass in: here it is a secret-shape value
		// in the provider's own envelope. Without redaction at this layer only
		// the caller-supplied literal protects anything.
		const upstreamOnlySecret = "FAKE-upstream-sidekey-4c1d2e3f4a5b"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":"denied","token":"` + upstreamOnlySecret + `"}}`))
		}))
		defer srv.Close()

		// The client holds a DIFFERENT key, so the literal rule cannot help:
		// only the body rules at this layer can remove the upstream value.
		client := NewDeepSeekClient("FAKE-client-sidekey-9f3c1f0b7a2e", newTestBaseClient())
		client.BaseURL = srv.URL

		_, err := client.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil)
		if err == nil {
			t.Fatal("expected an error for HTTP 403")
		}
		got := err.Error()
		t.Logf("deepseek 403 body (upstream-only secret) -> %s", got)
		if strings.Contains(got, upstreamOnlySecret) || strings.Contains(got, `"token"`) {
			t.Errorf("deepseek leaks an upstream-side credential: %s", got)
		}
		if !strings.Contains(got, "http status 403") || !strings.Contains(got, "denied") {
			t.Errorf("deepseek dropped the diagnostics: %s", got)
		}
	})

	t.Run("transport error with the key in the URL", func(t *testing.T) {
		closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		closedURL := closed.URL
		closed.Close()

		client := NewDeepSeekClient(llmRedactionFixtureKey, newTestBaseClient())
		client.BaseURL = closedURL + "/v1?api_key=" + llmRedactionFixtureKey

		_, err := client.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil)
		if err == nil {
			t.Fatal("expected a transport error against a closed port")
		}
		// *url.Error formats the whole URL, credential included.
		assertNoLLMCredential(t, "deepseek transport error", err.Error(), "deepseek:", "connection refused")
	})
}

// TestMiniMax_Errors_HideAPIKey covers the second BaseClient consumer so the
// call-site wrapping cannot silently be dropped from one of them.
func TestMiniMax_Errors_HideAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"base_resp":{"status_msg":"invalid api key: ` + llmRedactionFixtureKey + `"}}`))
	}))
	defer srv.Close()

	client := NewMiniMaxClient(llmRedactionFixtureKey, newTestBaseClient())
	client.BaseURL = srv.URL

	_, err := client.Chat(context.Background(), "", []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected an error for HTTP 401")
	}
	assertNoLLMCredential(t, "minimax 401 body", err.Error(), "http status 401", "invalid api key")
}

// TestKimi_Errors_HideAPIKey covers the third BaseClient consumer.
func TestKimi_Errors_HideAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided: ` + llmRedactionFixtureKey + `"}}`))
	}))
	defer srv.Close()

	client := NewKimiClient(llmRedactionFixtureKey, newTestBaseClient())
	client.BaseURL = srv.URL

	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected an error for HTTP 401")
	}
	assertNoLLMCredential(t, "kimi 401 body", err.Error(), "http status 401", "Incorrect API key provided")
}

// assertNoLLMCredential is the shared probe: the fixture key is absent and the
// named diagnostics survive.
func assertNoLLMCredential(t *testing.T, what, got string, wantKept ...string) {
	t.Helper()
	t.Logf("%s -> %s", what, got)
	if strings.Contains(got, llmRedactionFixtureKey) {
		t.Errorf("%s leaks the provider credential: %s", what, got)
	}
	for _, want := range wantKept {
		if !strings.Contains(got, want) {
			t.Errorf("%s dropped the diagnostic text %q: %s", what, want, got)
		}
	}
}

// TestBaseClient_DoRequest_RedactsBodySecrets pins the BaseClient layer on its
// own (no provider wrapper in the way). The provider clients also hand their
// credential to redact.Error, so a probe that only drove DeepSeek.Chat could
// not tell whether this layer still works: this one can, because the body
// carries material no caller holds.
func TestBaseClient_DoRequest_RedactsBodySecrets(t *testing.T) {
	const upstreamOnlySecret = "FAKE-upstream-sidekey-4b1d2e3f4a5b"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"denied","token":"` + upstreamOnlySecret + `"}}`))
	}))
	defer srv.Close()

	base := newTestBaseClient()
	resp, _, err := base.DoRequest(context.Background(), "POST", srv.URL, nil, []byte("{}"))
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err == nil {
		t.Fatal("expected an error for HTTP 403")
	}
	got := err.Error()
	t.Logf("base client 403 -> %s", got)
	if strings.Contains(got, upstreamOnlySecret) || strings.Contains(got, `"token"`) {
		t.Errorf("base client leaks a body credential: %s", got)
	}
	if !strings.Contains(got, "http status 403") || !strings.Contains(got, "denied") {
		t.Errorf("base client dropped the diagnostics: %s", got)
	}
}
