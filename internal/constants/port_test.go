package constants

import (
	"strings"
	"testing"
)

// TestPortConstantsNonEmpty ensures no constant accidentally becomes empty
// after a careless edit. An empty value here would silently break startup
// of every cmd binary that uses these as flag defaults.
func TestPortConstantsNonEmpty(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		value string
	}{
		{"AdminHTTPPort", AdminHTTPPort},
		{"AdminHTTPAddr", AdminHTTPAddr},
		{"AtlasBaseURL", AtlasBaseURL},
		{"FubonProxyAddr", FubonProxyAddr},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.value == "" {
				t.Fatalf("%s must not be empty", c.name)
			}
		})
	}
	if FubonProxyPort <= 0 || FubonProxyPort > 65535 {
		t.Fatalf("FubonProxyPort = %d, want valid TCP port (1..65535)", FubonProxyPort)
	}
}

// TestPortAddrConsistency verifies that the address strings (used by health
// probes and MCP clients) embed the same port as the bare-port constants.
// Catches drift where someone changes AdminHTTPPort but forgets AdminHTTPAddr.
func TestPortAddrConsistency(t *testing.T) {
	t.Parallel()

	// AdminHTTPAddr must end with ":" + AdminHTTPPort-stripped-leading-colon.
	portSuffix := strings.TrimPrefix(AdminHTTPPort, ":")
	if !strings.HasSuffix(AdminHTTPAddr, ":"+portSuffix) {
		t.Errorf("AdminHTTPAddr = %q does not embed port %q from AdminHTTPPort",
			AdminHTTPAddr, portSuffix)
	}

	// FubonProxyAddr must end with the integer FubonProxyPort.
	fubonSuffix := ":" + itoa(FubonProxyPort)
	if !strings.HasSuffix(FubonProxyAddr, fubonSuffix) {
		t.Errorf("FubonProxyAddr = %q does not embed port %d from FubonProxyPort",
			FubonProxyAddr, FubonProxyPort)
	}

	// AtlasBaseURL must embed the same port as AdminHTTPAddr.
	if !strings.Contains(AtlasBaseURL, AdminHTTPAddr) {
		t.Errorf("AtlasBaseURL = %q does not contain AdminHTTPAddr = %q",
			AtlasBaseURL, AdminHTTPAddr)
	}
}

// TestPortsDoNotCollide ensures the two services (atlas HTTP API and
// fubon-proxy) don't accidentally share a port. If they do, ProcessManager
// would fail to spawn the second one in standalone mode.
func TestPortsDoNotCollide(t *testing.T) {
	t.Parallel()
	if AdminHTTPPort == ":"+itoa(FubonProxyPort) {
		t.Fatalf("AdminHTTPPort (%s) collides with FubonProxyPort (%d)",
			AdminHTTPPort, FubonProxyPort)
	}
}

// itoa is a tiny strconv.Itoa to avoid importing strconv for one use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
