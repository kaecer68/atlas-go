package apigateway

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/apigateway/httpclient"
)

// HTTPClientConfig is an alias for backward compatibility.
// Use httpclient.Config directly in new code.
type HTTPClientConfig = httpclient.Config

// DefaultHTTPClientConfig is an alias for backward compatibility.
func DefaultHTTPClientConfig() HTTPClientConfig {
	return httpclient.DefaultConfig()
}

// NewHTTPClient is an alias for backward compatibility.
// All data source HTTP clients should use this factory instead of direct &http.Client{}.
func NewHTTPClient(cfg HTTPClientConfig) *http.Client {
	return httpclient.New(cfg)
}

// HTTPClientFactory is an alias for backward compatibility.
// Deprecated: direct instantiation of &http.Client{} violates the Constitution.
// Use HTTPClientFactory.New() instead.
type HTTPClientFactory = httpclient.Factory

// NewHTTPClientFactory is an alias for backward compatibility.
func NewHTTPClientFactory() *HTTPClientFactory {
	return httpclient.NewFactory()
}
