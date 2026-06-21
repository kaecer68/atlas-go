package marketdata

import (
	"os"
	"testing"
)

// TestFubonClient_DefaultURL_UsesHostDockerInternal (P1-S1) verifies that
// NewFubonClient() uses "host.docker.internal:8081" as the default proxy URL,
// NOT 127.0.0.1:8081. The fubon-proxy runs natively on macOS, and the atlas
// Go process runs inside a Docker container. Using 127.0.0.1:8081 targets the
// container's own loopback (wrong); host.docker.internal:8081 reaches the host.
func TestFubonClient_DefaultURL_UsesHostDockerInternal(t *testing.T) {
	client := NewFubonClient()

	want := "http://host.docker.internal:8081"
	if client.proxyURL != want {
		t.Errorf("NewFubonClient().proxyURL = %q, want %q\n\n"+
			"原因：fubon-proxy 在 macOS host 上執行，atlas 容器必須透過 host.docker.internal:8081 連線，"+
			"而非 127.0.0.1:8081（容器自身的 loopback）。",
			client.proxyURL, want)
	}
}

// TestFubonClient_RejectsFUBONProxyURL (P1-S3, PR #572 regression guard)
// verifies that setting the FUBON_PROXY_URL environment variable does NOT
// override the hardcoded default proxy URL. PR #572 explicitly removed
// os.Getenv("FUBON_PROXY_URL") from fubon_client.go; the client must always
// use the hardcoded constant regardless of environment.
func TestFubonClient_RejectsFUBONProxyURL(t *testing.T) {
	// Simulate a user trying to override via env var —
	// this should have zero effect on the client URL.
	t.Setenv("FUBON_PROXY_URL", "http://evil-override.example.com:9999")
	ResetSharedFubonClient()

	client := NewFubonClient()

	want := "http://host.docker.internal:8081"
	if client.proxyURL != want {
		t.Errorf("NewFubonClient().proxyURL = %q after setting FUBON_PROXY_URL, want %q\n\n"+
			"PR #572 已移除 FUBON_PROXY_URL 環境變數覆寫。FubonClient 必須永遠使用"+
			"硬編碼常數，不允許外部輸入繞過安全預設值。",
			client.proxyURL, want)
	}

	// Also verify os.Getenv("FUBON_PROXY_URL") is never called in
	// the fubon_client.go production code. The URL guard test
	// (fubon_url_guard_test.go) checks this at the AST level.
	if v := os.Getenv("FUBON_PROXY_URL"); v != "" && v != "http://evil-override.example.com:9999" {
		t.Logf("FUBON_PROXY_URL env value: %q", v)
	}
}
