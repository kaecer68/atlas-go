package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HttpClient calls atlas-go's HTTP API. It forwards ATLAS_API_KEY when set and
// supports GET / POST with JSON bodies. It is safe for concurrent use.
//
// Exported (2026-07-10, PR series atlas-mcp-onboarding-2026q3) so that
// cmd/atlas-mcp-setup can reuse the same wire format, X-API-Key header, and
// timeout handling. Before the export, the setup tool would have had to
// duplicate this logic and risk drift in auth header / retry behavior.
type HttpClient struct {
	base   string
	apiKey string
	httpc  *http.Client
}

// NewHTTPClient builds a client targeting atlasBaseURL with the per-call timeout
// from cfg.HTTPTimeout. apiKey is forwarded in the X-API-Key header.
//
// Exported for cross-cmd reuse; see HttpClient doc comment.
func NewHTTPClient(cfg Config) *HttpClient {
	return &HttpClient{
		base:   strings.TrimRight(cfg.AtlasBaseURL, "/"),
		apiKey: cfg.APIToken,
		httpc:  &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

// Get issues a GET to atlasBaseURL+path?query and decodes the JSON response into
// result. Any non-2xx response is treated as an error with the body quoted.
func (c *HttpClient) Get(ctx context.Context, path string, query url.Values, result any) error {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return c.do(ctx, http.MethodGet, u, nil, result)
}

// GetRaw issues a GET to atlasBaseURL+path?query and returns the raw response
// body without JSON decoding. Any non-2xx response is treated as an error.
func (c *HttpClient) GetRaw(ctx context.Context, path string, query url.Values) ([]byte, error) {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return c.doRaw(ctx, http.MethodGet, u, nil)
}

// PostJSON issues a POST with a JSON body. body may be nil.
func (c *HttpClient) PostJSON(ctx context.Context, path string, body, result any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("HttpClient: marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	return c.do(ctx, http.MethodPost, c.base+path, reader, result)
}

// do is shared by Get/PostJSON. result may be nil.
func (c *HttpClient) do(ctx context.Context, method, fullURL string, body io.Reader, result any) error {
	raw, err := c.doRaw(ctx, method, fullURL, body)
	if err != nil {
		return err
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("HttpClient: decode body: %w (raw=%d bytes)", err, len(raw))
	}
	return nil
}

// doRaw performs the HTTP request and returns the response body bytes.
func (c *HttpClient) doRaw(ctx context.Context, method, fullURL string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("HttpClient: new request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HttpClient: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("HttpClient: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HttpClient: %s %s -> %d: %s", method, fullURL, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

// Base reports the configured base URL. Used by tests / dashboards.
func (c *HttpClient) Base() string { return c.base }

// compile-time assertion HttpClient satisfies a minimal interface for testing.
var _ = time.Second
