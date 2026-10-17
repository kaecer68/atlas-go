// Package redact removes credential material from text before it reaches a
// sink an operator can read: a log line, an error string, an alert payload, a
// channel-health row, a CLI's stderr or a file under logs/.
//
// Why this lives in its own package (FU-20261016-15): the rules below started
// life as unexported helpers inside internal/marketdata (FU-20260926-02), so
// every other package that needed them had to keep its own copy of the field
// list. Copies drift, and a drifted list is a silent leak. There is now one
// list and one implementation; internal/marketdata keeps a thin wrapper so its
// call sites did not have to churn.
//
// The guidance every caller is expected to follow: redact the upstream text,
// never drop it. An operator has to be able to tell "token rejected" from
// "quota exhausted" from "IP banned", and a redaction that eats the message
// gets reverted by the next on-call engineer — which is worse than the leak.
package redact

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// Marker replaces credential material. It is deliberately visible: a reader
// must be able to see that something was removed at this position instead of
// wondering whether the value was empty.
const Marker = "[REDACTED]"

// minLiteralLen is the shortest caller-supplied secret the literal rule will
// replace. A shorter value (a 1-3 byte string) would gut unrelated text; real
// credentials are far longer.
const minLiteralLen = 8

// keyShapeMinLen is how many characters must follow a credential prefix before
// scrubKeyShapes treats the run as a secret. Providers issue far more; this
// bound keeps prose ("sk-", "hf_") from being redacted.
const keyShapeMinLen = 12

// Text removes credential material from a raw upstream body, error message or
// URL-bearing diagnostic string.
//
// Three rules run in order, and they are deliberately redundant:
//
//  1. URL — values of sensitive query parameters (?api_key=…, &token=…) and
//     the password half of URL userinfo are replaced with Marker. The
//     parameter NAME is kept: "the upstream rejected api_key" is diagnosis,
//     the value is the secret.
//  2. structural — a text that parses as JSON has every object member whose
//     key is secret-looking DROPPED (member and name both), which covers
//     nested envelopes.
//  3. textual — for text that does not parse as JSON (an envelope cut in half
//     by a read cap, an HTML error page, a header dump), the `key=value` /
//     `"key": value` shapes of the same member names are replaced wholesale;
//     and finally every caller-supplied secret of at least minLiteralLen is
//     replaced wherever it appears.
//
// TEXTS THAT PARSE AS JSON NEVER REACH RULE 3, so a message that merely
// mentions the word "token" ({"msg":"token: expired"}) is left alone.
func Text(text string, secrets ...string) string {
	out, parsed := scrubJSON(maskURLSecrets(text))
	if !parsed {
		out = scrubKeyValuePairs(out)
	}
	for _, secret := range secrets {
		if len(secret) >= minLiteralLen {
			out = strings.ReplaceAll(out, secret, Marker)
		}
	}
	return scrubKeyShapes(out)
}

// keyShapeRe matches credential material by SHAPE, for the case where the
// caller does not hold the value it must redact: a CLI that reads a URL from
// its arguments, or an error whose body quotes a key the caller never saw
// (LLM providers answer a 401 with `Incorrect API key provided: sk-…`).
//
// The prefixes are the ones this repo's providers actually issue; matching is
// case-sensitive except for the upper-case-only vendor forms. Over-redaction is
// bounded by requiring at least minKeyShapeLen characters after the prefix, so
// prose like "sk-" alone survives.
var keyShapeRe = regexp.MustCompile(`\b(?:sk-ant-|sk-proj-|sk-|ALTAKSP-|ALTAK-|ghp_|gho_|ghs_|glpat-|xoxb-|xoxp-|hf_|AIza|AKIA)[A-Za-z0-9_.-]{` + strconv.Itoa(keyShapeMinLen) + `,}`)

// scrubKeyShapes replaces provider-shaped credentials with Marker.
func scrubKeyShapes(text string) string {
	return keyShapeRe.ReplaceAllString(text, Marker)
}

// Error returns an error whose message carries no credential material while
// its Unwrap chain still reaches the original err, so errors.Is / errors.As
// keep working at the call sites that classify failures.
//
// When nothing needs redacting the original error is returned unchanged, so a
// caller that wraps with %w keeps the concrete type it had before.
func Error(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	message := Text(err.Error(), secrets...)
	if message == err.Error() {
		return err
	}
	return redactedError{original: err, message: message}
}

// redactedError carries a redacted message and the error it came from.
type redactedError struct {
	original error
	message  string
}

func (e redactedError) Error() string { return e.message }

// Unwrap exposes the original error so errors.Is / errors.As still match.
func (e redactedError) Unwrap() error { return e.original }

// secretJSONKeys lists the upstream JSON member names that carry credential
// material. Compared case-insensitively after trimming spaces.
var secretJSONKeys = []string{
	"token",
	"token_tail",
	"access_token",
	"refresh_token",
	"id_token",
	"session_token",
	"api_key",
	"apikey",
	"api-key",
	"x-api-key",
	"secret",
	"client_secret",
	"private_key",
	"password",
	"passwd",
	"authorization",
	"cookie",
	"set-cookie",
	// PII members. A broker's rejection body echoes the account it refused,
	// and order records / alerts are read by more people than the trading
	// desk (FU-20261016-12). Dropping the member is enough: the caller that
	// holds the value also passes it as a literal when it appears in prose.
	"account",
	"account_id",
	"account_no",
	"accountno",
	"acct",
	"cust_no",
	"personal_id",
	"id_number",
}

// secretQueryParams lists the query-parameter names whose VALUE is credential
// material. Matching is case-insensitive and ignores '-' / '_' separators, so
// `api_key`, `apiKey`, `API-KEY` and `apikey` are one entry. `key` and
// `signature` are here because brokers and data vendors put HMAC material in
// the query string.
var secretQueryParams = []string{
	"apikey",
	"token",
	"accesstoken",
	"refreshtoken",
	"idtoken",
	"sessiontoken",
	"key",
	"secret",
	"clientsecret",
	"privatekey",
	"password",
	"passwd",
	"authorization",
	"auth",
	"cookie",
	"setcookie",
	"signature",
	"sig",
	"hash",
	"hmac",
	"credential",
	"credentials",
	"sessionkey",
	"subscriptionkey",
}

// scrubJSON redacts the known secret fields in text, preferring a structural
// (JSON) rewrite. The second return value reports whether text parsed as JSON
// at all — when it did, the result is final and the textual rule must not run
// on it (see Text).
func scrubJSON(text string) (string, bool) {
	var decoded any
	if err := json.Unmarshal([]byte(text), &decoded); err != nil {
		return text, false
	}
	if !removeSecretsInValue(decoded) {
		return text, true
	}
	if reencoded, err := json.Marshal(decoded); err == nil {
		return string(reencoded), true
	}
	return text, true
}

// removeSecretsInValue walks a decoded JSON value and DROPS every object
// member whose key is secret-looking. Reports whether anything changed.
func removeSecretsInValue(v any) bool {
	changed := false
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			if isSecretKey(k) {
				delete(node, k)
				changed = true
				continue
			}
			if removeSecretsInValue(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range node {
			if removeSecretsInValue(child) {
				changed = true
			}
		}
	}
	return changed
}

// isSecretKey reports whether a JSON member key carries credential material.
func isSecretKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, secret := range secretJSONKeys {
		if normalized == secret {
			return true
		}
	}
	return false
}
