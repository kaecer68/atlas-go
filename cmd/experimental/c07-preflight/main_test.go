package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// TestValidateLocalhostURL: pure-function SSRF guard. We test it instead of
// HTTP-based checks because HTTP checks require a live atlas instance.
//
// L2.4 has no test file; we set the more rigorous pattern by covering
// the SSRF guard comprehensively.
func TestValidateLocalhostURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr bool
		reason  string
	}{
		// Allowed
		{"http://localhost", "http://localhost", false, "default docker compose URL"},
		{"http://localhost:18080", "http://localhost:18080", false, "with port"},
		{"http://127.0.0.1:18080", "http://127.0.0.1:18080", false, "loopback IP"},
		{"http://[::1]:18080", "http://[::1]:18080", false, "IPv6 loopback"},
		{"http://0.0.0.0:18080", "http://0.0.0.0:18080", false, "all-interfaces bind"},
		{"https://localhost:18080", "https://localhost:18080", false, "https for reverse proxy"},

		// Blocked — non-loopback (SSRF guard)
		{"https://atlas.example.com", "https://atlas.example.com", true, "external host"},
		{"https://prod.atlas.io", "https://prod.atlas.io", true, "production-ish domain"},
		{"http://10.0.0.5:18080", "http://10.0.0.5:18080", true, "private LAN IP"},

		// Blocked — bad scheme
		{"file://localhost/etc/passwd", "file://localhost/etc/passwd", true, "file scheme"},
		{"ftp://localhost", "ftp://localhost", true, "ftp scheme"},
		{"gopher://localhost:9001", "gopher://localhost:9001", true, "gopher scheme (SSRF favorite)"},

		// Blocked — malformed
		{"empty string", "", true, "no scheme or host"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLocalhostURL(tt.rawURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLocalhostURL(%q) error = %v, wantErr = %v (reason: %s)",
					tt.rawURL, err, tt.wantErr, tt.reason)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{"shorter than n returns unchanged", "hello", 10, "hello"},
		{"exact length returns unchanged", "12345", 5, "12345"},
		{"longer than n truncates with ellipsis", "abcdefghij", 5, "abcde..."},
		{"empty string", "", 10, ""},
		{"n=0 returns just ellipsis on non-empty", "hello", 0, "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.n)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
			}
		})
	}
}

// TestPrintResult: redirect stdout, verify marker + name + message layout.
// Helps prevent accidental Emoji/marker regressions that would degrade
// operator readability of preflight output.
func TestPrintResult(t *testing.T) {
	tests := []struct {
		name         string
		result       checkResult
		wantContains []string
	}{
		{
			name:         "OK result",
			result:       checkResult{Name: "test-ok", OK: true, Message: "ok info"},
			wantContains: []string{"✅", "test-ok", "ok info"},
		},
		{
			name:         "FAIL result",
			result:       checkResult{Name: "test-fail", OK: false, Message: "broken"},
			wantContains: []string{"❌", "test-fail", "broken"},
		},
		{
			name:         "Manual result",
			result:       checkResult{Name: "test-manual", OK: false, Manual: true, Message: "operator must do X"},
			wantContains: []string{"👤", "test-manual", "operator must do X"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// redirect stdout to a pipe
			origStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printResult(tt.result)
			_ = w.Close()
			os.Stdout = origStdout

			out, _ := io.ReadAll(r)
			got := string(out)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("printResult output missing %q\n got: %q", want, got)
				}
			}
		})
	}
}
