package redact

import (
	"net/url"
	"regexp"
	"strings"
)

// This file holds the two rules that operate on SHAPES rather than on a
// known member list: `key=value` pairs in free text, and credential material
// carried in a URL (query-string parameters and userinfo passwords).

// keyValueRe matches the `"key": value` (or `key=value`) shape of the secret
// member names in text that does not parse as JSON. The whole pair — key
// included — is replaced, so a log line can never be mistaken for a place to
// look for the value. Longer alternations come first so `token_tail` is not
// half-matched as `token`.
//
// Three value shapes are easy to get wrong, and all three are exercised by the
// corpus tests:
//
//   - an UNTERMINATED quoted value — a read cap can cut a body in the middle
//     of the credential, leaving `"token_tail":"FAKE-abc` with no closing
//     quote (the `"[^"]*"` alternative cannot match that, and a naive
//     `[^",}\s]+` refuses to start on the quote, so the credential survived);
//   - an `Authorization: Bearer <credential>` header dump — matching only
//     `Bearer` as the value replaced the scheme and left the credential
//     behind (the optional scheme prefix below keeps the two together);
//   - a query string, `…?api_key=abc&coid=2330` — `&` is excluded from the
//     value so the pair stops at the next parameter instead of swallowing the
//     rest of the URL (the value itself is masked by scrubQueryValues, which
//     runs first and keeps the parameter name).
var keyValueRe = regexp.MustCompile(`(?i)"?(?:token_tail|token|access_token|refresh_token|id_token|session_token|api_key|apikey|api-key|x-api-key|client_secret|private_key|secret|password|passwd|authorization|cookie|set-cookie|account_id|account_no|accountno|account|acct|cust_no|personal_id|id_number)"?[\s]*[:=][\s]*("[^"]*"?|(?i:(?:bearer|basic)\s+)?[^",}\s&]+)`)

// queryPairRe matches one `?name=value` / `&name=value` pair. The value class
// excludes the characters that end a parameter or a quoted URL, so a match
// never spans two parameters.
var queryPairRe = regexp.MustCompile(`([?&;])([^=&?#\s"'<>]+)=([^&#\s"'<>]*)`)

// userinfoRe matches the `//user:password@host` shape of a URL.
var userinfoRe = regexp.MustCompile(`//([^/@\s]+)@`)

// URL masks the credential material carried inside a URL: sensitive
// query-parameter values and the password half of the userinfo. Everything
// else — scheme, host, path, parameter names, non-secret parameters — is left
// byte-for-byte intact, because that is what makes the URL diagnosable.
func URL(raw string) string {
	return scrubUserinfo(scrubQueryValues(raw))
}

// maskURLSecrets applies the URL rule to a text that may embed zero, one or
// several URLs (an `*url.Error`, for example, reads
// `Get "https://host/path?api_key=…": dial tcp …`).
func maskURLSecrets(text string) string {
	return scrubUserinfo(scrubQueryValues(text))
}

// scrubQueryValues replaces the value of every sensitive query parameter with
// Marker, keeping the parameter name.
func scrubQueryValues(text string) string {
	return queryPairRe.ReplaceAllStringFunc(text, func(match string) string {
		parts := queryPairRe.FindStringSubmatch(match)
		if len(parts) < 4 || !isSecretQueryParam(parts[2]) {
			return match
		}
		// An empty value carries no secret, and seeing that the credential
		// was ABSENT is itself the diagnosis — keep it visible.
		if parts[3] == "" || parts[3] == Marker {
			return match
		}
		return parts[1] + parts[2] + "=" + Marker
	})
}

// scrubUserinfo replaces the password half of URL userinfo with Marker. A
// userinfo without a password (the common case) is left alone.
func scrubUserinfo(text string) string {
	return userinfoRe.ReplaceAllStringFunc(text, func(match string) string {
		inner := strings.TrimSuffix(strings.TrimPrefix(match, "//"), "@")
		user, password, hasPassword := strings.Cut(inner, ":")
		if !hasPassword || password == Marker {
			return match
		}
		return "//" + user + ":" + Marker + "@"
	})
}

// scrubKeyValuePairs replaces every secret-named key/value pair in text with
// Marker. A pair that is already masked is kept as-is, so the parameter NAME
// survives when the URL rule ran first and the call is idempotent.
func scrubKeyValuePairs(text string) string {
	return keyValueRe.ReplaceAllStringFunc(text, func(match string) string {
		if strings.Contains(match, Marker) {
			return match
		}
		return Marker
	})
}

// isSecretQueryParam reports whether a query-parameter name carries
// credential material. Matching ignores case, '-', '_' and '.' so `api_key`,
// `apiKey`, `API-KEY` and `apikey` are the same parameter.
func isSecretQueryParam(name string) bool {
	normalized := normalizeParamName(name)
	for _, secret := range secretQueryParams {
		if normalized == secret {
			return true
		}
	}
	return false
}

// normalizeParamName lower-cases a parameter name and drops the separators
// vendors disagree about.
func normalizeParamName(name string) string {
	decoded := name
	if unescaped, err := url.QueryUnescape(name); err == nil {
		decoded = unescaped
	}
	normalized := strings.ToLower(strings.TrimSpace(decoded))
	normalized = strings.NewReplacer("-", "", "_", "", ".", "", " ", "").Replace(normalized)
	return normalized
}
