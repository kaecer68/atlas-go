// Charset-aware JSON decoder for TWSE and other Taiwanese data providers.
//
// Background: TWSE historically serves some endpoints (monthly statistics,
// shareholder calendars, certain monthly reports) in Big5 or GB2312 instead
// of UTF-8, despite the JSON spec (RFC 8259 §8.1) mandating UTF-8. Parsing
// such payloads directly via json.Decoder / json.Unmarshal produces mojibake
// (`'æ'`-style garbled CJK output) or outright decode failures.
//
// This file is the single source of truth for parsing external JSON responses
// that may use a non-UTF-8 charset. Callers pass the HTTP response body and
// the Content-Type header; the helper transparently transcodes Big5 / GB2312
// / Shift_JIS / other charsets (via golang.org/x/text/encoding/htmlindex)
// before invoking the standard json.Decoder.

package marketdata

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"strings"

	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/transform"
)

// DecodeJSON reads a charset-aware JSON body into dst.
//
// contentType is parsed for a `charset=` parameter (e.g., "Big5", "GB2312",
// "Shift_JIS", "utf-8"). When the charset is missing or a UTF-8 variant
// ("utf-8", "utf8", or "ascii" — ASCII is a UTF-8 subset), DecodeJSON is
// equivalent to json.NewDecoder(body).Decode(dst).
//
// For non-UTF-8 charsets, the body is transcoded via golang.org/x/text/
// encoding/htmlindex before json.Decoder parses it. Unknown charsets return
// an error so callers can distinguish transcode failure from JSON parse failure.
//
// Example usage with an *http.Response:
//
//	var apiResp twseResponse
//	if err := DecodeJSON(resp.Body, resp.Header.Get("Content-Type"), &apiResp); err != nil {
//	    return fmt.Errorf("decode response: %w", err)
//	}
func DecodeJSON(body io.Reader, contentType string, dst any) error {
	if body == nil {
		return fmt.Errorf("charset decoder: nil body")
	}
	charset := charsetFromContentType(contentType)
	if isUTF8(charset) {
		return json.NewDecoder(body).Decode(dst)
	}
	enc, err := htmlindex.Get(charset)
	if err != nil {
		return fmt.Errorf("charset decoder: unknown charset %q in Content-Type %q: %w", charset, contentType, err)
	}
	return json.NewDecoder(transform.NewReader(body, enc.NewDecoder())).Decode(dst)
}

// charsetFromContentType extracts the `charset=` parameter from a Content-Type
// header value. Returns "" when the header is empty, malformed, or has no
// charset parameter.
//
// Examples:
//
//	"application/json; charset=Big5"          → "Big5"
//	"application/json;charset=UTF-8"           → "UTF-8"
//	"text/html; charset=BIG5; boundary=..."    → "BIG5"
//	"application/json"                         → ""
//	""                                         → ""
func charsetFromContentType(contentType string) string {
	if contentType == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	return params["charset"]
}

// isUTF8 reports whether charset designates a UTF-8 encoding (case-insensitive).
// An empty charset is treated as UTF-8 because the JSON spec mandates UTF-8
// as the default encoding for application/json payloads.
func isUTF8(charset string) bool {
	if charset == "" {
		return true
	}
	switch strings.ToLower(charset) {
	case "utf-8", "utf8", "ascii":
		return true
	default:
		return false
	}
}
