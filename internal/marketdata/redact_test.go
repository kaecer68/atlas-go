package marketdata

import (
	"strings"
	"testing"
)

// TestRedactUpstreamText_SinkCorpus pins the two redaction rules against the
// body shapes this package actually meets: well-formed envelopes, envelopes
// cut in half by the 512-byte read cap, HTML error pages, header dumps and
// plain text. Every case asserts BOTH directions — the credential is gone AND
// the operator-actionable fields survive — because a redactor that eats the
// message gets reverted by the next on-call engineer.
func TestRedactUpstreamText_SinkCorpus(t *testing.T) {
	const key = redactionFixtureKey
	const tail = redactionFixtureSecret

	cases := []struct {
		name string
		body string
		// wantKept must remain in the output (diagnosability).
		wantKept []string
	}{
		{
			name:     "finmind 400 token is illegal",
			body:     `{"msg":"Token is illegal.","status":400,"token_tail":"` + tail + `"}`,
			wantKept: []string{"Token is illegal.", "400"},
		},
		{
			name:     "finmind 402 quota envelope (production shape)",
			body:     `{"msg":"Requests reach the upper limit. https://finmindtrade.com/","status":402,"data":[],"token_tail":"` + tail + `"}`,
			wantKept: []string{"upper limit", "402"},
		},
		{
			name:     "nested envelope",
			body:     `{"error":{"code":"AUTH","token_tail":"` + tail + `"},"msg":"bad key","status":401}`,
			wantKept: []string{"AUTH", "bad key", "401"},
		},
		{
			name:     "secret inside a data array",
			body:     `{"msg":"ok","data":[{"symbol":"2330","token":"` + tail + `"}]}`,
			wantKept: []string{"2330"},
		},
		{
			name:     "key casing and spacing variants",
			body:     `{"Token_Tail":"` + tail + `","API-KEY":"` + key + `","x-api-key":"` + key + `","msg":"m","status":500}`,
			wantKept: []string{"m", "500"},
		},
		{
			name:     "every listed credential member",
			body:     `{"access_token":"` + tail + `A","refresh_token":"` + tail + `B","id_token":"` + tail + `C","session_token":"` + tail + `D","client_secret":"` + tail + `E","private_key":"` + tail + `F","passwd":"` + tail + `G","cookie":"` + tail + `H","set-cookie":"` + tail + `I","secret":"` + tail + `J","password":"` + tail + `K","authorization":"` + tail + `L","msg":"diag","status":403}`,
			wantKept: []string{"diag", "403"},
		},
		{
			// The 512-byte cap can cut the envelope in half, so the JSON
			// branch never runs. The textual rule has to catch it.
			name:     "truncated json",
			body:     `{"msg":"Token is illegal.","status":400,"token_tail":"` + tail,
			wantKept: []string{"Token is illegal."},
		},
		{
			name:     "html error page",
			body:     `<html><body>429 Too Many Requests<br>token_tail=` + tail + `</body></html>`,
			wantKept: []string{"429 Too Many Requests"},
		},
		{
			name:     "header dump",
			body:     "HTTP/1.1 401 Unauthorized\r\nAuthorization: Bearer " + tail + "\r\nContent-Type: application/json",
			wantKept: []string{"401 Unauthorized", "Content-Type"},
		},
		{
			name:     "plain text with the literal credential only",
			body:     "upstream rejected the call; offending key " + key + " is stale",
			wantKept: []string{"upstream rejected the call", "stale"},
		},
		{
			// Over-redaction guard: a JSON body whose MESSAGE mentions the
			// word "token" is not a credential member and must be untouched.
			name:     "message text is not a credential member",
			body:     `{"msg":"token: expired, please rotate","status":400}`,
			wantKept: []string{"token: expired, please rotate", "400"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactUpstreamText(tc.body, key)

			if strings.Contains(got, tail) {
				t.Errorf("redacted body leaks the credential fragment %q: %s", tail, got)
			}
			if strings.Contains(got, key) {
				t.Errorf("redacted body leaks the literal credential: %s", got)
			}
			if strings.Contains(strings.ToLower(got), "token_tail") {
				t.Errorf("redacted body keeps the field NAME token_tail: %s", got)
			}
			for _, want := range tc.wantKept {
				if !strings.Contains(got, want) {
					t.Errorf("redaction dropped the diagnostic text %q: %s", want, got)
				}
			}
			if twice := redactUpstreamText(got, key); twice != got {
				t.Errorf("redaction is not idempotent:\n once: %s\ntwice: %s", got, twice)
			}
		})
	}
}

// TestRedactUpstreamText_ShortSecretsAreNotLiterals documents the one bound in
// the literal rule: a credential shorter than upstreamSecretMinLiteralLen is
// NOT replaced textually (replacing a 1-3 byte string would gut unrelated
// text). Such a value is still dropped when it arrives as a JSON member, which
// is how upstreams actually send it.
func TestRedactUpstreamText_ShortSecretsAreNotLiterals(t *testing.T) {
	const short = "abc"
	body := `{"msg":"no data","status":204,"token_tail":"` + short + `"}`
	got := redactUpstreamText(body, short)
	if strings.Contains(got, "token_tail") {
		t.Errorf("short secret must still be dropped as a JSON member: %s", got)
	}
	if !strings.Contains(got, "no data") || !strings.Contains(got, "204") {
		t.Errorf("diagnostics lost: %s", got)
	}
}

// TestRedactUpstreamText_CredentialMemberCoverage fails when a credential
// member stops being redacted. The corpus above only proves that the members
// it exercises are redacted; this assertion makes a silent shrink of the list —
// the cheapest way to reintroduce FU-20260926-02 — a red test. It drives the
// package entry point (redactUpstreamText) rather than the internal list, so it
// keeps guarding this call site now that the list lives in internal/redact.
func TestRedactUpstreamText_CredentialMemberCoverage(t *testing.T) {
	required := []string{
		"token", "token_tail",
		"api_key", "apikey", "api-key", "x-api-key",
		"access_token", "refresh_token", "id_token", "session_token",
		"secret", "client_secret", "private_key",
		"password", "passwd",
		"authorization", "cookie", "set-cookie",
	}
	for _, key := range required {
		for _, variant := range []string{key, strings.ToUpper(key)} {
			body := `{"` + variant + `":"` + redactionFixtureSecret + `","msg":"diag","status":500}`
			got := redactUpstreamText(body, redactionFixtureKey)
			if strings.Contains(got, redactionFixtureSecret) {
				t.Errorf("credential member %q is no longer redacted: %s", variant, got)
			}
			if strings.Contains(strings.ToLower(got), strings.ToLower(variant)) {
				t.Errorf("credential member NAME %q survives redaction: %s", variant, got)
			}
			if !strings.Contains(got, "diag") || !strings.Contains(got, "500") {
				t.Errorf("diagnostics lost for member %q: %s", variant, got)
			}
		}
	}
	for _, keep := range []string{"msg", "status", "data"} {
		body := `{"` + keep + `":"please keep me","status":500}`
		if got := redactUpstreamText(body); !strings.Contains(got, "please keep me") {
			t.Errorf("diagnostic member %q must never be treated as a credential: %s", keep, got)
		}
	}
}
