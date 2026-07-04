package marketdata

import (
	"os"
	"testing"
)

// TestFubonClient_DefaultURL_UsesFubonProxy (P1-S1) verifies that
// NewFubonClient() uses "http://fubon-proxy:18081" as the default proxy URL
// (Docker compose service name, resolved via Docker DNS).
// 本機開發時 register_adapters.go 會自動切換至 127.0.0.1。
func TestFubonClient_DefaultURL_UsesFubonProxy(t *testing.T) {
	client := NewFubonClient()

	want := "http://fubon-proxy:18081"
	if client.proxyURL != want {
		t.Errorf("NewFubonClient().proxyURL = %q, want %q\n\n"+
			"原因：fubon-proxy 已容器化，透過 Docker DNS service name fubon-proxy 連線，"+
			"不須再依賴 host.docker.internal。",
			client.proxyURL, want)
	}
}

// TestFubonClient_RejectsFUBONProxyURL (P1-S3, PR #572 regression guard)
// verifies that setting the FUBON_PROXY_URL environment variable does NOT
// override the default proxy URL. PR #572 explicitly removed
// os.Getenv("FUBON_PROXY_URL") from fubon_client.go; the client must always
// use fubonproxy.ProxyBaseURL() regardless of environment.
func TestFubonClient_RejectsFUBONProxyURL(t *testing.T) {
	t.Setenv("FUBON_PROXY_URL", "http://evil-override.example.com:9999")
	ResetSharedFubonClient()

	client := NewFubonClient()

	want := "http://fubon-proxy:18081"
	if client.proxyURL != want {
		t.Errorf("NewFubonClient().proxyURL = %q after setting FUBON_PROXY_URL, want %q\n\n"+
			"PR #572 已移除 FUBON_PROXY_URL 環境變數覆寫。FubonClient 必須永遠使用"+
			"fubonproxy.ProxyBaseURL() 回傳值。",
			client.proxyURL, want)
	}

	if v := os.Getenv("FUBON_PROXY_URL"); v != "" && v != "http://evil-override.example.com:9999" {
		t.Logf("FUBON_PROXY_URL env value: %q", v)
	}
}
