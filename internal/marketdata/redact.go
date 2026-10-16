package marketdata

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ─── Upstream response text must never reach a sink raw ─────────────────────
//
// Every provider in this package copies an upstream error body into a log
// line and/or an error string — that text is the only way an operator can
// tell "token rejected" from "quota exhausted" from "IP banned". But the same
// upstreams echo credential material back in that body: FinMind returns
// `{"msg":"Token is illegal.","status":400,"token_tail":"...abc"}`, and in
// production (2026-09-26 02:10Z) it returned `token_tail` on the 402 quota
// response as well. This repository is public and log excerpts are pasted into
// issues and PRs, so a fragment of a live key must not travel with them.
//
// The two rules below are deliberately redundant:
//
//  1. structural — JSON object members whose key is listed in
//     upstreamSecretJSONKeys are DROPPED (not masked): the member goes away,
//     which covers nested envelopes and also removes the field NAME, so a
//     reader cannot tell which field to go looking for;
//  2. literal — the caller-supplied credential(s) are replaced wherever they
//     appear, which covers bodies that do not parse as JSON (512-byte
//     truncation, HTML error pages, plain text).
//
// Rule 2 only applies to values of at least upstreamSecretMinLiteralLen; a
// shorter value would mangle unrelated text, and real credentials are far
// longer. Both rules keep the operator-actionable fields (status, msg, code)
// intact — a redaction that costs diagnosis gets reverted by the next
// on-call engineer, which is worse than the leak.
const (
	// upstreamRedacted is what replaces a literal credential occurrence.
	upstreamRedacted = "[REDACTED]"

	// upstreamSecretMinLiteralLen is the shortest value the literal rule
	// will replace.
	upstreamSecretMinLiteralLen = 8
)

// upstreamSecretJSONKeys lists the upstream JSON member names that carry
// credential material. Compared case-insensitively after trimming spaces.
var upstreamSecretJSONKeys = []string{
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
}

// upstreamSecretKeyRe matches the `"key": value` (or `key=value`) shape of the
// fields above in bodies that do not parse as JSON. The whole pair — key
// included — is replaced, so a log line can never be mistaken for a place to
// look for the value. Longer alternations come first so `token_tail` is not
// half-matched as `token`.
//
// Two value shapes are easy to get wrong, and both are exercised by
// TestRedactUpstreamText_SinkCorpus:
//
//   - an UNTERMINATED quoted value — the 512-byte read cap can cut a body in
//     the middle of the credential, leaving `"token_tail":"FAKE-abc` with no
//     closing quote (the `"[^"]*"` alternative cannot match that, and a naive
//     `[^",}\s]+` refuses to start on the quote, so the credential survived);
//   - an `Authorization: Bearer <credential>` header dump — matching only
//     `Bearer` as the value replaced the scheme and left the credential
//     behind (the optional scheme prefix below keeps the two together).
var upstreamSecretKeyRe = regexp.MustCompile(`(?i)"?(?:token_tail|token|access_token|refresh_token|id_token|session_token|api_key|apikey|api-key|x-api-key|client_secret|private_key|secret|password|passwd|authorization|cookie|set-cookie)"?[\s]*[:=][\s]*("[^"]*"?|(?i:(?:bearer|basic)\s+)?[^",}\s]+)`)

// redactUpstreamText removes credential material from a raw upstream body
// before it reaches a log line, an error string, or the on-disk quota state.
func redactUpstreamText(body string, secrets ...string) string {
	out := scrubUpstreamJSONSecrets(body)
	for _, s := range secrets {
		if len(s) >= upstreamSecretMinLiteralLen {
			out = strings.ReplaceAll(out, s, upstreamRedacted)
		}
	}
	return out
}

// scrubUpstreamJSONSecrets redacts the known secret fields in body, preferring
// a structural (JSON) rewrite and falling back to a textual scrub.
func scrubUpstreamJSONSecrets(body string) string {
	var decoded any
	if err := json.Unmarshal([]byte(body), &decoded); err == nil {
		if !redactUpstreamSecretsInValue(decoded) {
			return body
		}
		if reencoded, err := json.Marshal(decoded); err == nil {
			return string(reencoded)
		}
		return body
	}
	return upstreamSecretKeyRe.ReplaceAllString(body, upstreamRedacted)
}

// redactUpstreamSecretsInValue walks a decoded JSON value and DROPS every
// object member whose key is secret-looking. Reports whether anything changed.
func redactUpstreamSecretsInValue(v any) bool {
	changed := false
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			if isUpstreamSecretKey(k) {
				delete(node, k)
				changed = true
				continue
			}
			if redactUpstreamSecretsInValue(child) {
				changed = true
			}
		}
	case []any:
		for _, child := range node {
			if redactUpstreamSecretsInValue(child) {
				changed = true
			}
		}
	}
	return changed
}

// isUpstreamSecretKey reports whether a JSON member key carries credential
// material.
func isUpstreamSecretKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, secret := range upstreamSecretJSONKeys {
		if normalized == secret {
			return true
		}
	}
	return false
}
