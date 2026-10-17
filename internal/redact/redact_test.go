package redact

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The fixture values are fake. They are long enough to clear minLiteralLen and
// shaped like the real thing so the assertions stay readable.
const (
	fixtureKey    = "FAKE-key-9f3c1f0b7a2e4d5c"
	fixtureTail   = "FAKE-tail-4c1d2e3f4a5b6c7d"
	fixtureURLkey = "FAKE-url-7d1e2a3b4c5d6e7f"
)

// TestText_SinkCorpus pins the rules against the text shapes the callers
// actually meet: well-formed envelopes, envelopes cut in half by a read cap,
// HTML error pages, header dumps, plain text and URLs. Every case asserts BOTH
// directions — the credential is gone AND the operator-actionable fields
// survive — because a redactor that eats the message gets reverted by the next
// on-call engineer.
func TestText_SinkCorpus(t *testing.T) {
	cases := []struct {
		name string
		text string
		// wantKept must remain in the output (diagnosability).
		wantKept []string
		// wantNotIn must be absent from the output.
		wantNotIn []string
	}{
		{
			name:      "finmind 400 token is illegal",
			text:      `{"msg":"Token is illegal.","status":400,"token_tail":"` + fixtureTail + `"}`,
			wantKept:  []string{"Token is illegal.", "400"},
			wantNotIn: []string{fixtureTail, "token_tail"},
		},
		{
			name:      "nested envelope",
			text:      `{"error":{"code":"AUTH","token_tail":"` + fixtureTail + `"},"msg":"bad key","status":401}`,
			wantKept:  []string{"AUTH", "bad key", "401"},
			wantNotIn: []string{fixtureTail, "token_tail"},
		},
		{
			name:      "truncated json (read cap cut the envelope)",
			text:      `{"msg":"Token is illegal.","status":400,"token_tail":"` + fixtureTail,
			wantKept:  []string{"Token is illegal."},
			wantNotIn: []string{fixtureTail, "token_tail"},
		},
		{
			name:      "html error page",
			text:      `<html><body>429 Too Many Requests<br>token_tail=` + fixtureTail + `</body></html>`,
			wantKept:  []string{"429 Too Many Requests"},
			wantNotIn: []string{fixtureTail, "token_tail"},
		},
		{
			name:      "header dump",
			text:      "HTTP/1.1 401 Unauthorized\r\nAuthorization: Bearer " + fixtureTail + "\r\nContent-Type: application/json",
			wantKept:  []string{"401 Unauthorized", "Content-Type"},
			wantNotIn: []string{fixtureTail},
		},
		{
			// The caller may not hold the value it must redact: an upstream
			// can quote a key the caller never saw. The shape rule covers it.
			name:      "provider-shaped key the caller cannot pass in",
			text:      `{"error":{"message":"Incorrect API key provided: sk-live9f3c1f0b7a2e4d5c6b7a"}}`,
			wantKept:  []string{"Incorrect API key provided"},
			wantNotIn: []string{"sk-live9f3c1f0b7a2e4d5c6b7a"},
		},
		{
			name:     "short prose is not a credential shape",
			text:     `{"msg":"unsupported scheme sk- and hf_ prefixes"}`,
			wantKept: []string{"sk-", "hf_"},
		},
		{
			name:      "plain text with the literal credential only",
			text:      "upstream rejected the call; offending key " + fixtureKey + " is stale",
			wantKept:  []string{"upstream rejected the call", "stale"},
			wantNotIn: []string{fixtureKey},
		},
		{
			// Over-redaction guard: a JSON body whose MESSAGE mentions the
			// word "token" is not a credential member and must be untouched.
			name:     "message text is not a credential member",
			text:     `{"msg":"token: expired, please rotate","status":400}`,
			wantKept: []string{"token: expired, please rotate", "400"},
		},
		{
			// The transport-error shape: the credential rides in the URL and
			// the URL rides in the error text. The parameter NAME stays so an
			// operator can see which credential the upstream refused.
			name:      "transport error carries the query-string credential",
			text:      `Get "http://127.0.0.1:1/api?api_key=` + fixtureURLkey + `&coid=2330": dial tcp 127.0.0.1:1: connect: connection refused`,
			wantKept:  []string{"api_key=", "coid=2330", "connection refused"},
			wantNotIn: []string{fixtureURLkey},
		},
		{
			name:      "query string credential in a plain body",
			text:      `upstream 401 for /v1/quote?symbol=2330&api_key=` + fixtureURLkey + `&format=json`,
			wantKept:  []string{"symbol=2330", "format=json", "401"},
			wantNotIn: []string{fixtureURLkey},
		},
		{
			name:      "userinfo password",
			text:      `Get "https://broker:` + fixtureURLkey + `@api.example.test/orders": EOF`,
			wantKept:  []string{"broker:", "api.example.test/orders"},
			wantNotIn: []string{fixtureURLkey},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Text(tc.text, fixtureKey)

			for _, leak := range append([]string{fixtureKey}, tc.wantNotIn...) {
				if strings.Contains(got, leak) {
					t.Errorf("redacted text still contains %q: %s", leak, got)
				}
			}
			for _, want := range tc.wantKept {
				if !strings.Contains(got, want) {
					t.Errorf("redaction dropped the diagnostic text %q: %s", want, got)
				}
			}
			if twice := Text(got, fixtureKey); twice != got {
				t.Errorf("redaction is not idempotent:\n once: %s\ntwice: %s", got, twice)
			}
		})
	}
}

// TestURL_MasksCredentialParametersAndKeepsEverythingElse covers the exported
// URL entry point on its own: the shape it must produce, and the parameters it
// must NOT touch.
func TestURL_MasksCredentialParametersAndKeepsEverythingElse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "api_key in the middle of a query",
			in:   "https://api.tej.test/api/datatables/TRAIL/TAPRCD.json?api_key=" + fixtureURLkey + "&coid=2330&mdate.gte=2025-01-03",
			want: "https://api.tej.test/api/datatables/TRAIL/TAPRCD.json?api_key=" + Marker + "&coid=2330&mdate.gte=2025-01-03",
		},
		{
			name: "casing and separator variants",
			in:   "https://x.test/p?API-KEY=" + fixtureURLkey + "&APIKey2=keepme&apikey=" + fixtureURLkey,
			want: "https://x.test/p?API-KEY=" + Marker + "&APIKey2=keepme&apikey=" + Marker,
		},
		{
			name: "token and signature",
			in:   "https://x.test/p?token=" + fixtureURLkey + "&signature=" + fixtureKey + "&page=2",
			want: "https://x.test/p?token=" + Marker + "&signature=" + Marker + "&page=2",
		},
		{
			name: "non-secret parameters untouched",
			in:   "https://x.test/p?symbol=2330&date=2025-01-03&type=IND",
			want: "https://x.test/p?symbol=2330&date=2025-01-03&type=IND",
		},
		{
			name: "userinfo password",
			in:   "https://broker:" + fixtureURLkey + "@api.example.test/orders",
			want: "https://broker:" + Marker + "@api.example.test/orders",
		},
		{
			name: "userinfo without a password is untouched",
			in:   "https://token-only@api.example.test/orders",
			want: "https://token-only@api.example.test/orders",
		},
		{
			name: "empty value",
			in:   "https://x.test/p?api_key=&page=1",
			want: "https://x.test/p?api_key=&page=1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := URL(tc.in); got != tc.want {
				t.Errorf("URL()\n got: %s\nwant: %s", got, tc.want)
			}
			if twice := URL(URL(tc.in)); twice != URL(tc.in) {
				t.Errorf("URL() is not idempotent: %s", twice)
			}
		})
	}
}

// TestError_PreservesUnwrapChain is the property the call sites depend on:
// a redacted error must still satisfy errors.Is for the sentinels they
// classify on, otherwise removing the leak would break retry classification.
func TestError_PreservesUnwrapChain(t *testing.T) {
	sentinel := errors.New("circuit open")
	inner := fmt.Errorf("http status 401: %s: %w", `{"error":{"message":"Incorrect API key provided: `+fixtureKey+`"}}`, sentinel)

	got := Error(inner, fixtureKey)
	if strings.Contains(got.Error(), fixtureKey) {
		t.Fatalf("redacted error leaks the credential: %v", got)
	}
	if !strings.Contains(got.Error(), "http status 401") {
		t.Errorf("redacted error dropped the status: %v", got)
	}
	if !errors.Is(got, sentinel) {
		t.Errorf("redacted error lost errors.Is(%v)", sentinel)
	}
	if !strings.Contains(got.Error(), "Incorrect API key provided") {
		t.Errorf("redacted error dropped the provider message skeleton: %v", got)
	}
}

// TestError_ReturnsOriginalWhenNothingToRedact keeps the type identity of the
// error when no rule fires, so callers that type-assert keep working.
func TestError_ReturnsOriginalWhenNothingToRedact(t *testing.T) {
	original := errors.New("dial tcp: connection refused")
	if got := Error(original); got != original {
		t.Errorf("Error() must return the original error when nothing to redact, got %v", got)
	}
	if got := Error(nil); got != nil {
		t.Errorf("Error(nil) must be nil, got %v", got)
	}
}

// TestText_ShortSecretsAreNotLiterals documents the one bound in the literal
// rule: a credential shorter than minLiteralLen is NOT replaced textually
// (replacing a 1-3 byte string would gut unrelated text). Such a value is
// still dropped when it arrives as a JSON member, which is how upstreams
// actually send it.
func TestText_ShortSecretsAreNotLiterals(t *testing.T) {
	const short = "abc"
	got := Text(`{"msg":"no data","status":204,"token_tail":"`+short+`"}`, short)
	if strings.Contains(got, "token_tail") {
		t.Errorf("short secret must still be dropped as a JSON member: %s", got)
	}
	if !strings.Contains(got, "no data") || !strings.Contains(got, "204") {
		t.Errorf("diagnostics lost: %s", got)
	}
}

// TestSecretLists_Coverage fails when a credential name is deleted from either
// list. The corpus above only proves that the names it exercises are redacted;
// this assertion makes a silent shrink of a list — the cheapest way to
// reintroduce FU-20260926-02 — a red test.
func TestSecretLists_Coverage(t *testing.T) {
	requiredJSON := []string{
		"token", "token_tail",
		"api_key", "apikey", "api-key", "x-api-key",
		"access_token", "refresh_token", "id_token", "session_token",
		"secret", "client_secret", "private_key",
		"password", "passwd",
		"authorization", "cookie", "set-cookie",
	}
	for _, want := range requiredJSON {
		if !isSecretKey(want) {
			t.Errorf("secretJSONKeys no longer covers %q", want)
		}
		if !isSecretKey(strings.ToUpper(want)) {
			t.Errorf("secret key matching must be case-insensitive (missing %q)", strings.ToUpper(want))
		}
	}
	if isSecretKey("msg") || isSecretKey("status") || isSecretKey("data") {
		t.Error("secretJSONKeys must not cover diagnostic members (msg/status/data)")
	}

	requiredQuery := []string{"api_key", "apikey", "API-KEY", "apiKey", "token", "access_token", "key", "secret", "password", "signature"}
	for _, want := range requiredQuery {
		if !isSecretQueryParam(want) {
			t.Errorf("secretQueryParams no longer covers %q", want)
		}
	}
	for _, keep := range []string{"symbol", "coid", "page", "format", "type", "date"} {
		if isSecretQueryParam(keep) {
			t.Errorf("secretQueryParams must not cover the diagnostic parameter %q", keep)
		}
	}
}
