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

// httpClient calls atlas-go's HTTP API. It forwards ATLAS_API_KEY when set and
// supports GET / POST with JSON bodies. It is safe for concurrent use.
type httpClient struct {
	base   string
	apiKey string
	httpc  *http.Client
}

// newHTTPClient builds a client targeting atlasBaseURL with the per-call timeout
// from cfg.HTTPTimeout. apiKey is forwarded in the X-API-Key header.
func newHTTPClient(cfg Config) *httpClient {
	return &httpClient{
		base:   strings.TrimRight(cfg.AtlasBaseURL, "/"),
		apiKey: cfg.APIToken,
		httpc:  &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

// Get issues a GET to atlasBaseURL+path?query and decodes the JSON response into
// result. Any non-2xx response is treated as an error with the body quoted.
func (c *httpClient) Get(ctx context.Context, path string, query url.Values, result any) error {
	u := c.base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return c.do(ctx, http.MethodGet, u, nil, result)
}

// PostJSON issues a POST with a JSON body. body may be nil.
func (c *httpClient) PostJSON(ctx context.Context, path string, body, result any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("httpClient: marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	return c.do(ctx, http.MethodPost, c.base+path, reader, result)
}

// do is shared by Get/PostJSON. result may be nil.
func (c *httpClient) do(ctx context.Context, method, fullURL string, body io.Reader, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return fmt.Errorf("httpClient: new request: %w", err)
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
		return fmt.Errorf("httpClient: do: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("httpClient: read body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("httpClient: %s %s -> %d: %s", method, fullURL, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(raw, result); err != nil {
		return fmt.Errorf("httpClient: decode body: %w (raw=%d bytes)", err, len(raw))
	}
	return nil
}

// Base reports the configured base URL. Used by tests / dashboards.
func (c *httpClient) Base() string { return c.base }

// compile-time assertion httpClient satisfies a minimal interface for testing.
var _ = time.Second
