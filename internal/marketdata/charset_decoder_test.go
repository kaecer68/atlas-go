package marketdata

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/text/encoding/traditionalchinese"
)

// TestDecodeJSON_UTF8Explicit verifies that an explicit `charset=utf-8`
// Content-Type header bypasses the transcode path and decodes normally.
func TestDecodeJSON_UTF8Explicit(t *testing.T) {
	payload := `{"name":"台積電","code":"2330"}`
	var got map[string]string
	if err := DecodeJSON(strings.NewReader(payload), "application/json; charset=utf-8", &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["name"] != "台積電" {
		t.Errorf("name = %q, want %q", got["name"], "台積電")
	}
	if got["code"] != "2330" {
		t.Errorf("code = %q, want %q", got["code"], "2330")
	}
}

// TestDecodeJSON_UTF8MixedCase verifies case-insensitive UTF-8 charset handling.
func TestDecodeJSON_UTF8MixedCase(t *testing.T) {
	payload := `{"x":1}`
	for _, ct := range []string{
		"application/json; charset=UTF-8",
		"application/json; charset=Utf-8",
		"application/json; charset=utf8",
		"application/json; charset=ascii",
	} {
		var got map[string]int
		if err := DecodeJSON(strings.NewReader(payload), ct, &got); err != nil {
			t.Errorf("ct=%q: unexpected error: %v", ct, err)
		}
		if got["x"] != 1 {
			t.Errorf("ct=%q: x = %d, want 1", ct, got["x"])
		}
	}
}

// TestDecodeJSON_Big5Explicit verifies the production bug fix:
// Big5-encoded JSON body + `charset=Big5` Content-Type → correctly transcoded
// to UTF-8 and parsed (instead of mojibake / decode failure).
func TestDecodeJSON_Big5Explicit(t *testing.T) {
	utf8 := `{"name":"台積電","code":"2330"}`
	big5Bytes, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(utf8))
	if err != nil {
		t.Fatalf("Big5 encode fixture: %v", err)
	}

	var got map[string]string
	if err := DecodeJSON(bytes.NewReader(big5Bytes), "text/html; charset=Big5", &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["name"] != "台積電" {
		t.Errorf("name = %q, want %q (Big5 transcode failed)", got["name"], "台積電")
	}
	if got["code"] != "2330" {
		t.Errorf("code = %q, want %q", got["code"], "2330")
	}
}

// TestDecodeJSON_Big5MixedCase verifies htmlindex.Lookup is case-insensitive
// for charset names (TWSE serves "Big5" / "BIG5" / "big5" interchangeably).
func TestDecodeJSON_Big5MixedCase(t *testing.T) {
	utf8 := `{"x":"測試"}`
	big5Bytes, _ := traditionalchinese.Big5.NewEncoder().Bytes([]byte(utf8))

	for _, ct := range []string{
		"application/json; charset=Big5",
		"application/json; charset=BIG5",
		"application/json; charset=big5",
	} {
		var got map[string]string
		if err := DecodeJSON(bytes.NewReader(big5Bytes), ct, &got); err != nil {
			t.Errorf("ct=%q: unexpected error: %v", ct, err)
		}
		if got["x"] != "測試" {
			t.Errorf("ct=%q: x = %q, want %q", ct, got["x"], "測試")
		}
	}
}

// TestDecodeJSON_NoCharsetDefaultsUTF8 verifies that the absence of a charset
// parameter falls back to UTF-8 (JSON spec default per RFC 8259).
func TestDecodeJSON_NoCharsetDefaultsUTF8(t *testing.T) {
	payload := `{"name":"台積電"}`
	var got map[string]string
	if err := DecodeJSON(strings.NewReader(payload), "application/json", &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["name"] != "台積電" {
		t.Errorf("name = %q, want %q", got["name"], "台積電")
	}
}

// TestDecodeJSON_EmptyContentTypeDefaultsUTF8 verifies that an empty
// Content-Type header (which some clients/servers omit) defaults to UTF-8.
func TestDecodeJSON_EmptyContentTypeDefaultsUTF8(t *testing.T) {
	payload := `{"x":42}`
	var got map[string]int
	if err := DecodeJSON(strings.NewReader(payload), "", &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["x"] != 42 {
		t.Errorf("x = %d, want 42", got["x"])
	}
}

// TestDecodeJSON_UnknownCharsetReturnsError verifies that an unrecognized
// charset name fails fast with a descriptive error rather than silently
// producing mojibake or returning a generic JSON parse error.
func TestDecodeJSON_UnknownCharsetReturnsError(t *testing.T) {
	body := strings.NewReader(`{"x":1}`)
	var got map[string]int
	err := DecodeJSON(body, "application/json; charset=zz-not-a-real-charset", &got)
	if err == nil {
		t.Fatal("expected error for unknown charset, got nil")
	}
	if !strings.Contains(err.Error(), "unknown charset") {
		t.Errorf("error message %q should mention 'unknown charset'", err.Error())
	}
	if !strings.Contains(err.Error(), "zz-not-a-real-charset") {
		t.Errorf("error message %q should include the offending charset name", err.Error())
	}
}

// TestDecodeJSON_MalformedContentType verifies graceful handling of malformed
// Content-Type headers — they default to UTF-8 rather than crashing.
func TestDecodeJSON_MalformedContentType(t *testing.T) {
	payload := `{"x":1}`
	var got map[string]int
	if err := DecodeJSON(strings.NewReader(payload), "this is not a mime type", &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["x"] != 1 {
		t.Errorf("x = %d, want 1", got["x"])
	}
}

// TestDecodeJSON_NilBodyReturnsError verifies the API rejects nil bodies
// with a clear error rather than panicking inside json.NewDecoder.
func TestDecodeJSON_NilBodyReturnsError(t *testing.T) {
	var got map[string]int
	err := DecodeJSON(nil, "application/json", &got)
	if err == nil {
		t.Fatal("expected error for nil body, got nil")
	}
	if !strings.Contains(err.Error(), "nil body") {
		t.Errorf("error = %v, want 'nil body' substring", err)
	}
}

// TestDecodeJSON_MojibakeRoundTrip reproduces the exact production bug:
// feeding Big5-encoded JSON bytes to a UTF-8-only decoder would either fail
// to parse or silently produce mojibake. Our helper must transcode correctly.
func TestDecodeJSON_MojibakeRoundTrip(t *testing.T) {
	utf8 := `{"name":"台積電"}`
	big5Bytes, err := traditionalchinese.Big5.NewEncoder().Bytes([]byte(utf8))
	if err != nil {
		t.Fatalf("Big5 encode: %v", err)
	}

	// Sanity check: the Big5 bytes MUST differ from UTF-8 bytes (otherwise
	// the test is vacuous and won't catch a regression where the helper
	// stops trans-coding).
	if bytes.Equal(big5Bytes, []byte(utf8)) {
		t.Fatal("Big5 fixture must differ from UTF-8 payload")
	}

	// Demonstrate the original bug: parsing Big5 bytes as UTF-8 fails.
	var control map[string]string
	controlErr := json.NewDecoder(bytes.NewReader(big5Bytes)).Decode(&control)
	if controlErr == nil && control["name"] == "台積電" {
		t.Fatal("control case should not produce correct UTF-8 decode of Big5 bytes")
	}

	// Now verify our helper recovers the correct UTF-8 content.
	var got map[string]string
	if err := DecodeJSON(bytes.NewReader(big5Bytes), "application/json; charset=Big5", &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["name"] != "台積電" {
		t.Errorf("transcoded name = %q, want %q (Big5 transcode failed)", got["name"], "台積電")
	}
}

// TestDecodeJSON_StreamingLargerPayload verifies the streaming reader path
// handles realistic payload sizes that exceed any internal buffer boundary,
// AND that the helper actually transcodes Big5 — not silently skipping it.
// Guards against future regressions where the helper stops trans-coding.
func TestDecodeJSON_StreamingLargerPayload(t *testing.T) {
	const rows = 200
	var utf8 bytes.Buffer
	utf8.WriteString(`{"rows":[`)
	for i := range rows {
		if i > 0 {
			utf8.WriteString(",")
		}
		utf8.WriteString(`"台積電"`)
	}
	utf8.WriteString(`]}`)

	// Encode the UTF-8 payload into Big5 bytes — this is the realistic
	// production scenario (TWSE monthly stats served as Big5).
	big5Bytes, err := traditionalchinese.Big5.NewEncoder().Bytes(utf8.Bytes())
	if err != nil {
		t.Fatalf("Big5 encode fixture: %v", err)
	}
	// Sanity check: Big5 bytes must differ from UTF-8 (otherwise the
	// transcode path is never exercised and the test is vacuous).
	if bytes.Equal(big5Bytes, utf8.Bytes()) {
		t.Fatal("Big5 fixture must differ from UTF-8 payload")
	}

	var got struct {
		Rows []string `json:"rows"`
	}
	if err := DecodeJSON(bytes.NewReader(big5Bytes), "text/html; charset=Big5", &got); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Rows) != rows {
		t.Fatalf("rows count = %d, want %d", len(got.Rows), rows)
	}
	for i, r := range got.Rows {
		if r != "台積電" {
			t.Errorf("rows[%d] = %q, want %q (Big5 transcode failed at row %d)", i, r, "台積電", i)
			break
		}
	}
}

// TestCharsetFromContentType is a table-driven test covering edge cases of
// the Content-Type parser used internally by DecodeJSON.
func TestCharsetFromContentType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no charset param", "application/json", ""},
		{"utf-8 lowercase", "application/json; charset=utf-8", "utf-8"},
		{"UTF-8 mixed case", "application/json; charset=UTF-8", "UTF-8"},
		{"Big5 mixed case", "application/json; charset=Big5", "Big5"},
		{"BIG5 uppercase", "text/html; charset=BIG5", "BIG5"},
		{"utf8 no dash", "application/json; charset=utf8", "utf8"},
		{"ascii", "application/json; charset=ascii", "ascii"},
		{"charset first param", "application/json; charset=Big5; boundary=xyz", "Big5"},
		{"charset middle param", "application/json; foo=bar; charset=Big5; baz=qux", "Big5"},
		{"no quotes around value", "application/json;charset=Big5", "Big5"},
		{"unrelated param only", "application/json; foo=bar", ""},
		{"malformed", "this is not a mime type", ""},
		{"empty charset value", "application/json; charset=", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := charsetFromContentType(tt.in)
			if got != tt.want {
				t.Errorf("charsetFromContentType(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestIsUTF8 is a table-driven test covering the UTF-8 detection short-circuit
// used to skip the transcode path entirely for already-UTF-8 payloads.
func TestIsUTF8(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", true},
		{"utf-8", true},
		{"UTF-8", true},
		{"Utf-8", true},
		{"utf8", true},
		{"UTF8", true},
		{"ascii", true},
		{"ASCII", true},
		{"Big5", false},
		{"big5", false},
		{"GB2312", false},
		{"GB18030", false},
		{"Shift_JIS", false},
		{"latin1", false},
	}
	for _, tt := range tests {
		got := isUTF8(tt.in)
		if got != tt.want {
			t.Errorf("isUTF8(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
