package apigateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// FetchResult struct tests
// ---------------------------------------------------------------------------

func TestFetchResult_StructFields(t *testing.T) {
	now := time.Now()
	fr := FetchResult{
		Data:   []byte(`{"key":"value"}`),
		Meta:   FetchMetadata{ChannelID: "ch1", LatencyMs: 42, Timestamp: now},
		Cached: true,
		Stale:  false,
	}

	if string(fr.Data) != `{"key":"value"}` {
		t.Errorf("Data = %q, want %q", fr.Data, `{"key":"value"}`)
	}
	if fr.Meta.ChannelID != "ch1" {
		t.Errorf("Meta.ChannelID = %q, want %q", fr.Meta.ChannelID, "ch1")
	}
	if fr.Meta.LatencyMs != 42 {
		t.Errorf("Meta.LatencyMs = %d, want 42", fr.Meta.LatencyMs)
	}
	if !fr.Cached {
		t.Error("Cached should be true")
	}
	if fr.Stale {
		t.Error("Stale should be false")
	}
}

// ---------------------------------------------------------------------------
// FetchMetadata struct + JSON marshaling tests
// ---------------------------------------------------------------------------

func TestFetchMetadata_StructFields(t *testing.T) {
	ts := time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC)
	fm := FetchMetadata{
		ChannelID:          "channel-1",
		LatencyMs:          150,
		RateLimitRemaining: 99,
		Timestamp:          ts,
		Cached:             true,
		Stale:              true,
	}

	if fm.ChannelID != "channel-1" {
		t.Errorf("ChannelID = %q, want %q", fm.ChannelID, "channel-1")
	}
	if fm.LatencyMs != 150 {
		t.Errorf("LatencyMs = %d, want 150", fm.LatencyMs)
	}
	if fm.RateLimitRemaining != 99 {
		t.Errorf("RateLimitRemaining = %d, want 99", fm.RateLimitRemaining)
	}
	if !fm.Timestamp.Equal(ts) {
		t.Errorf("Timestamp = %v, want %v", fm.Timestamp, ts)
	}
	if !fm.Cached {
		t.Error("Cached should be true")
	}
	if !fm.Stale {
		t.Error("Stale should be true")
	}
}

func TestFetchMetadata_JSONMarshal(t *testing.T) {
	fm := FetchMetadata{
		ChannelID:          "ch-json",
		LatencyMs:          200,
		RateLimitRemaining: 50,
		Timestamp:          time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC),
		Cached:             false,
		Stale:              false,
	}

	data, err := json.Marshal(fm)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if result["channel_id"] != "ch-json" {
		t.Errorf("channel_id = %v, want ch-json", result["channel_id"])
	}
	if result["latency_ms"] != float64(200) {
		t.Errorf("latency_ms = %v, want 200", result["latency_ms"])
	}
	if result["rate_limit_remaining"] != float64(50) {
		t.Errorf("rate_limit_remaining = %v, want 50", result["rate_limit_remaining"])
	}
	if result["cached"] != false {
		t.Errorf("cached = %v, want false", result["cached"])
	}
	if result["stale"] != false {
		t.Errorf("stale = %v, want false", result["stale"])
	}
	if _, ok := result["timestamp"]; !ok {
		t.Error("timestamp field missing from JSON output")
	}
}

// ---------------------------------------------------------------------------
// HealthStatus struct + JSON marshaling tests
// ---------------------------------------------------------------------------

func TestHealthStatus_StructFields(t *testing.T) {
	hs := HealthStatus{
		Status:    "ok",
		LastError: "none",
		UpdatedAt: "2026-05-27T12:00:00Z",
		CheckType: "liveness",
	}

	if hs.Status != "ok" {
		t.Errorf("Status = %q, want %q", hs.Status, "ok")
	}
	if hs.LastError != "none" {
		t.Errorf("LastError = %q, want %q", hs.LastError, "none")
	}
	if hs.UpdatedAt != "2026-05-27T12:00:00Z" {
		t.Errorf("UpdatedAt = %q, want 2026-05-27T12:00:00Z", hs.UpdatedAt)
	}
	if hs.CheckType != "liveness" {
		t.Errorf("CheckType = %q, want liveness", hs.CheckType)
	}
}

func TestHealthStatus_JSONMarshal(t *testing.T) {
	hs := HealthStatus{
		Status:    "warn",
		LastError: "timeout",
		UpdatedAt: "2026-05-27T12:00:00Z",
		CheckType: "readiness",
	}

	data, err := json.Marshal(hs)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if result["status"] != "warn" {
		t.Errorf("status = %v, want warn", result["status"])
	}
	if result["last_error"] != "timeout" {
		t.Errorf("last_error = %v, want timeout", result["last_error"])
	}
	if result["updated_at"] != "2026-05-27T12:00:00Z" {
		t.Errorf("updated_at = %v", result["updated_at"])
	}
	if result["check_type"] != "readiness" {
		t.Errorf("check_type = %v, want readiness", result["check_type"])
	}
}

func TestHealthStatus_JSONMarshal_OmitEmptyLastError(t *testing.T) {
	hs := HealthStatus{
		Status:    "ok",
		UpdatedAt: "2026-05-27T12:00:00Z",
		CheckType: "computed",
	}

	data, err := json.Marshal(hs)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	if strings.Contains(string(data), `"last_error"`) {
		t.Errorf("last_error should be omitted when empty, got: %s", data)
	}
}

// ---------------------------------------------------------------------------
// ChannelMetadata struct tests
// ---------------------------------------------------------------------------

func TestChannelMetadata_StructFields(t *testing.T) {
	cm := ChannelMetadata{
		ChannelID:  "test-channel",
		Country:    "US",
		Platform:   "REST",
		APIFormat:  "JSON",
		Path:       "/api/data",
		Storage:    "cache",
		HasLimiter: true,
	}

	if cm.ChannelID != "test-channel" {
		t.Errorf("ChannelID = %q, want test-channel", cm.ChannelID)
	}
	if cm.Country != "US" {
		t.Errorf("Country = %q, want US", cm.Country)
	}
	if cm.Platform != "REST" {
		t.Errorf("Platform = %q, want REST", cm.Platform)
	}
	if cm.APIFormat != "JSON" {
		t.Errorf("APIFormat = %q, want JSON", cm.APIFormat)
	}
	if cm.Path != "/api/data" {
		t.Errorf("Path = %q, want /api/data", cm.Path)
	}
	if cm.Storage != "cache" {
		t.Errorf("Storage = %q, want cache", cm.Storage)
	}
	if !cm.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

// ---------------------------------------------------------------------------
// HTTPProvider tests
// ---------------------------------------------------------------------------

func TestHTTPProvider_Fetch_Success(t *testing.T) {
	expectedData := []byte(`{"key":"value"}`)
	p := &HTTPProvider{
		name:    "test-http-channel",
		limiter: rate.NewLimiter(rate.Every(time.Second), 10),
		meta:    ChannelMetadata{ChannelID: "test-http-channel", Country: "US"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return expectedData, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok", CheckType: "liveness"}, nil
		},
	}

	ctx := context.Background()
	result, err := p.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if string(result.Data) != string(expectedData) {
		t.Errorf("Data = %q, want %q", result.Data, expectedData)
	}
	if result.Meta.ChannelID != "test-http-channel" {
		t.Errorf("Meta.ChannelID = %q, want test-http-channel", result.Meta.ChannelID)
	}
	if result.Meta.LatencyMs < 0 {
		t.Errorf("Meta.LatencyMs = %d, want >= 0", result.Meta.LatencyMs)
	}
	if result.Meta.RateLimitRemaining < 0 {
		t.Errorf("Meta.RateLimitRemaining = %d, want >= 0", result.Meta.RateLimitRemaining)
	}
	if result.Meta.Timestamp.IsZero() {
		t.Error("Meta.Timestamp should not be zero")
	}
}

func TestHTTPProvider_Fetch_Error(t *testing.T) {
	p := &HTTPProvider{
		name:    "test-http-channel",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "test-http-channel"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return nil, errors.New("network error")
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok"}, nil
		},
	}

	ctx := context.Background()
	result, err := p.Fetch(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Errorf("result should be nil on error, got %+v", result)
	}
	if !strings.Contains(err.Error(), "fetch test-http-channel") {
		t.Errorf("error should contain channel name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "network error") {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestHTTPProvider_Fetch_RateLimit_Wait(t *testing.T) {
	// Create a limiter with burst=0 so Wait blocks immediately
	p := &HTTPProvider{
		name:    "rate-limited-channel",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "rate-limited-channel"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return []byte(`data`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok"}, nil
		},
	}

	// Use a canceled context to trigger rate limit error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Fetch(ctx)
	if err == nil {
		t.Fatal("expected an error from canceled context (rate limit), got nil")
	}
}

func TestHTTPProvider_HealthCheck(t *testing.T) {
	expected := HealthStatus{Status: "ok", CheckType: "liveness"}
	p := &HTTPProvider{
		name:    "test-http",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "test-http"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return []byte(`data`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return expected, nil
		},
	}

	ctx := context.Background()
	status, err := p.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if status.Status != expected.Status {
		t.Errorf("Status = %q, want %q", status.Status, expected.Status)
	}
	if status.CheckType != expected.CheckType {
		t.Errorf("CheckType = %q, want %q", status.CheckType, expected.CheckType)
	}
}

func TestHTTPProvider_HealthCheck_Error(t *testing.T) {
	p := &HTTPProvider{
		name:    "test-http",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "test-http"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return []byte(`data`), nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{}, errors.New("health check timeout")
		},
	}

	ctx := context.Background()
	_, err := p.HealthCheck(ctx)
	if err == nil {
		t.Fatal("expected health check error, got nil")
	}
	if err.Error() != "health check timeout" {
		t.Errorf("error = %q, want %q", err.Error(), "health check timeout")
	}
}

func TestHTTPProvider_RateLimit(t *testing.T) {
	limiter := rate.NewLimiter(5, 10)
	p := &HTTPProvider{
		name:    "test-http",
		limiter: limiter,
		meta:    ChannelMetadata{ChannelID: "test-http"},
		fetcher: func(ctx context.Context) ([]byte, error) {
			return nil, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{}, nil
		},
	}

	got := p.RateLimit()
	if got != limiter {
		t.Errorf("RateLimit returned different limiter instance")
	}
}

func TestHTTPProvider_Metadata(t *testing.T) {
	meta := ChannelMetadata{
		ChannelID:  "http-chan",
		Country:    "TW",
		Platform:   "REST",
		APIFormat:  "JSON",
		Path:       "/api/v1/quotes",
		Storage:    "cache",
		HasLimiter: true,
	}
	p := &HTTPProvider{
		name:    "http-chan",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    meta,
		fetcher: func(ctx context.Context) ([]byte, error) {
			return nil, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{}, nil
		},
	}

	got := p.Metadata()
	if got.ChannelID != meta.ChannelID {
		t.Errorf("ChannelID = %q, want %q", got.ChannelID, meta.ChannelID)
	}
	if got.Country != meta.Country {
		t.Errorf("Country = %q, want %q", got.Country, meta.Country)
	}
	if got.Platform != meta.Platform {
		t.Errorf("Platform = %q, want %q", got.Platform, meta.Platform)
	}
}

// ---------------------------------------------------------------------------
// FileProvider tests
// ---------------------------------------------------------------------------

func TestFileProvider_Fetch(t *testing.T) {
	p := &FileProvider{
		name:    "file-channel",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "file-channel", Country: "US"},
		path:    "/tmp/data.csv",
		parser: func(data []byte) ([]byte, error) {
			return data, nil
		},
	}

	ctx := context.Background()
	result, err := p.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if result == nil {
		t.Fatal("result should not be nil")
	}
	if result.Meta.ChannelID != "file-channel" {
		t.Errorf("Meta.ChannelID = %q, want file-channel", result.Meta.ChannelID)
	}
	if result.Meta.LatencyMs < 0 {
		t.Errorf("Meta.LatencyMs = %d, want >= 0", result.Meta.LatencyMs)
	}
	if result.Meta.RateLimitRemaining < 0 {
		t.Errorf("Meta.RateLimitRemaining = %d, want >= 0", result.Meta.RateLimitRemaining)
	}
	if result.Meta.Timestamp.IsZero() {
		t.Error("Meta.Timestamp should not be zero")
	}
	// FileProvider does not populate Data by default
	if result.Data != nil {
		t.Logf("Data = %q (may be nil for FileProvider)", result.Data)
	}
}

func TestFileProvider_Fetch_RateLimit(t *testing.T) {
	// Use a canceled context with a blocking limiter
	limiter := rate.NewLimiter(0, 0)
	// Force limiter to be exhausted with a canceled context
	p := &FileProvider{
		name:    "file-chan",
		limiter: limiter,
		meta:    ChannelMetadata{ChannelID: "file-chan"},
		path:    "/tmp/data.csv",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Fetch(ctx)
	if err == nil {
		t.Fatal("expected rate limit / context error, got nil")
	}
}

func TestFileProvider_HealthCheck(t *testing.T) {
	p := &FileProvider{
		name:    "file-chan",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    ChannelMetadata{ChannelID: "file-chan"},
	}

	ctx := context.Background()
	status, err := p.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
	if status.CheckType != "readiness" {
		t.Errorf("CheckType = %q, want readiness", status.CheckType)
	}
	if status.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
}

func TestFileProvider_RateLimit(t *testing.T) {
	limiter := rate.NewLimiter(rate.Inf, 0)
	p := &FileProvider{
		name:    "file-chan",
		limiter: limiter,
		meta:    ChannelMetadata{ChannelID: "file-chan"},
	}

	got := p.RateLimit()
	if got != limiter {
		t.Errorf("RateLimit returned different limiter instance")
	}
}

func TestFileProvider_Metadata(t *testing.T) {
	meta := ChannelMetadata{
		ChannelID:  "file-chan",
		Country:    "JP",
		Platform:   "FILE",
		APIFormat:  "CSV",
		Path:       "/data/quotes.csv",
		Storage:    "disk",
		HasLimiter: false,
	}
	p := &FileProvider{
		name:    "file-chan",
		limiter: rate.NewLimiter(rate.Inf, 0),
		meta:    meta,
	}

	got := p.Metadata()
	if got.ChannelID != meta.ChannelID {
		t.Errorf("ChannelID = %q, want %q", got.ChannelID, meta.ChannelID)
	}
	if got.APIFormat != meta.APIFormat {
		t.Errorf("APIFormat = %q, want %q", got.APIFormat, meta.APIFormat)
	}
	if got.Storage != meta.Storage {
		t.Errorf("Storage = %q, want %q", got.Storage, meta.Storage)
	}
}

// ---------------------------------------------------------------------------
// ComputeProvider tests
// ---------------------------------------------------------------------------

func TestComputeProvider_Fetch_Success(t *testing.T) {
	expectedData := []byte(`{"result":42}`)
	p := &ComputeProvider{
		name: "compute-channel",
		meta: ChannelMetadata{ChannelID: "compute-channel", Country: "US"},
		compute: func(ctx context.Context) ([]byte, error) {
			return expectedData, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{Status: "ok", CheckType: "computed"}, nil
		},
	}

	ctx := context.Background()
	result, err := p.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch failed: %v", err)
	}

	if string(result.Data) != string(expectedData) {
		t.Errorf("Data = %q, want %q", result.Data, expectedData)
	}
	if result.Meta.ChannelID != "compute-channel" {
		t.Errorf("Meta.ChannelID = %q, want compute-channel", result.Meta.ChannelID)
	}
	if result.Meta.LatencyMs < 0 {
		t.Errorf("Meta.LatencyMs = %d, want >= 0", result.Meta.LatencyMs)
	}
	if result.Meta.Timestamp.IsZero() {
		t.Error("Meta.Timestamp should not be zero")
	}
	// ComputeProvider does not set RateLimitRemaining (no rate limiter)
	if result.Meta.RateLimitRemaining != 0 {
		t.Errorf("Meta.RateLimitRemaining = %d, want 0 (not set for ComputeProvider)", result.Meta.RateLimitRemaining)
	}
}

func TestComputeProvider_Fetch_Error(t *testing.T) {
	p := &ComputeProvider{
		name: "compute-channel",
		meta: ChannelMetadata{ChannelID: "compute-channel"},
		compute: func(ctx context.Context) ([]byte, error) {
			return nil, errors.New("computation failed")
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{}, nil
		},
	}

	ctx := context.Background()
	result, err := p.Fetch(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if result != nil {
		t.Errorf("result should be nil on error, got %+v", result)
	}
	if !strings.Contains(err.Error(), "compute compute-channel") {
		t.Errorf("error should contain channel name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "computation failed") {
		t.Errorf("error should wrap original error, got: %v", err)
	}
}

func TestComputeProvider_HealthCheck_Success(t *testing.T) {
	expected := HealthStatus{Status: "ok", CheckType: "computed"}
	p := &ComputeProvider{
		name: "compute-channel",
		meta: ChannelMetadata{ChannelID: "compute-channel"},
		compute: func(ctx context.Context) ([]byte, error) {
			return nil, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return expected, nil
		},
	}

	ctx := context.Background()
	status, err := p.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if status.Status != expected.Status {
		t.Errorf("Status = %q, want %q", status.Status, expected.Status)
	}
	if status.CheckType != expected.CheckType {
		t.Errorf("CheckType = %q, want %q", status.CheckType, expected.CheckType)
	}
}

func TestComputeProvider_HealthCheck_Error(t *testing.T) {
	p := &ComputeProvider{
		name: "compute-channel",
		meta: ChannelMetadata{ChannelID: "compute-channel"},
		compute: func(ctx context.Context) ([]byte, error) {
			return nil, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{}, errors.New("compute health failure")
		},
	}

	ctx := context.Background()
	_, err := p.HealthCheck(ctx)
	if err == nil {
		t.Fatal("expected error from healthFunc, got nil")
	}
	if err.Error() != "compute health failure" {
		t.Errorf("error = %q, want %q", err.Error(), "compute health failure")
	}
}

func TestComputeProvider_RateLimit(t *testing.T) {
	p := &ComputeProvider{
		name: "compute-channel",
		meta: ChannelMetadata{ChannelID: "compute-channel"},
		compute: func(ctx context.Context) ([]byte, error) {
			return nil, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{}, nil
		},
	}

	got := p.RateLimit()
	if got == nil {
		t.Fatal("RateLimit should not return nil")
	}
	if got.Limit() != rate.Inf {
		t.Errorf("Limit = %v, want Inf", got.Limit())
	}
	if got.Burst() != 0 {
		t.Errorf("Burst = %d, want 0", got.Burst())
	}
}

func TestComputeProvider_Metadata(t *testing.T) {
	meta := ChannelMetadata{
		ChannelID:  "compute-chan",
		Country:    "CN",
		Platform:   "IN_MEMORY",
		APIFormat:  "BINARY",
		Path:       "",
		Storage:    "memory",
		HasLimiter: false,
	}
	p := &ComputeProvider{
		name: "compute-chan",
		meta: meta,
		compute: func(ctx context.Context) ([]byte, error) {
			return nil, nil
		},
		healthFunc: func(ctx context.Context) (HealthStatus, error) {
			return HealthStatus{}, nil
		},
	}

	got := p.Metadata()
	if got.ChannelID != meta.ChannelID {
		t.Errorf("ChannelID = %q, want %q", got.ChannelID, meta.ChannelID)
	}
	if got.Country != meta.Country {
		t.Errorf("Country = %q, want %q", got.Country, meta.Country)
	}
	if got.HasLimiter != meta.HasLimiter {
		t.Errorf("HasLimiter = %v, want %v", got.HasLimiter, meta.HasLimiter)
	}
}

// ---------------------------------------------------------------------------
// DataProvider interface compliance (compile-time check)
// ---------------------------------------------------------------------------

func TestDataProvider_InterfaceCompliance(t *testing.T) {
	// Compile-time: all three types must satisfy DataProvider
	var _ DataProvider = (*HTTPProvider)(nil)
	var _ DataProvider = (*FileProvider)(nil)
	var _ DataProvider = (*ComputeProvider)(nil)
	// If this test compiles, interface compliance is verified at compile time.
	t.Log("All provider types satisfy DataProvider interface")
}
