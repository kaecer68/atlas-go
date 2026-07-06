package constants

const (
	// AdminHTTPPort is the default listen address for the atlas-go HTTP API server.
	// It serves as the default for cmd/atlas (-addr flag) and cmd/backtest-window
	// (-addr flag). docker-compose.yml and Makefile must be kept in syn&#99;.
	AdminHTTPPort = ":18080"

	// AdminHTTPAddr is the default 127.0.0.1 address string for health probes
	// (e.g. cmd/atlas/api_routes.go and internal/monitoring/ health handlers).
	AdminHTTPAddr = "127.0.0.1:18080"

	// AtlasBaseURL is the default base URL for the atlas-mcp server to connect
	// to the atlas-go HTTP API. It is used as the ATLAS_BASE_URL fallback when
	// the environment variable is not set. cmd/atlas-mcp documentation must be
	// kept in syn&#99;.
	AtlasBaseURL = "http://127.0.0.1:18080"

	// FubonProxyPort is the default listen port for the fubon-proxy Python
	// microservice. It is used by ProcessManager when spawning the proxy
	// subprocess (internal/fubonproxy/manager.go) and as the default for the
	// -fubon-port flag in cmd/atlas. docker-compose.yml (FUBON_PROXY_PORT)
	// and services/fubon-proxy/main.py must be kept in syn&#99;.
	FubonProxyPort = 18081

	// FubonProxyAddr is the default 127.0.0.1 address string for fubon-proxy
	// health probes (e.g. cmd/atlas/api_routes.go and internal/monitoring/
	// health handlers).
	FubonProxyAddr = "127.0.0.1:18081"
)
