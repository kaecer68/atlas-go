package apigateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// --- existing minimal tests (kept verbatim) -------------------------------

func TestGeopoliticalChannelAdapter_Metadata(t *testing.T) {
	a := &GeopoliticalChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "geopolitical" {
		t.Errorf("ChannelID = %q, want geopolitical", m.ChannelID)
	}
	if m.Country != "全球" {
		t.Errorf("Country = %q, want 全球", m.Country)
	}
	if m.Platform != "RSS + GDELT" {
		t.Errorf("Platform = %q, want RSS + GDELT", m.Platform)
	}
	if m.APIFormat != "Composite" {
		t.Errorf("APIFormat = %q, want Composite", m.APIFormat)
	}
	if m.Path != "geopolitical" {
		t.Errorf("Path = %q, want geopolitical", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestGeopoliticalChannelAdapter_RateLimit(t *testing.T) {
	a := &GeopoliticalChannelAdapter{
		limiter: rate.NewLimiter(rate.Every(time.Minute), 1),
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

// --- shared mock transport (geopolitical) ---------------------------------

// geopoliticalMockTransport returns canned responses for any URL whose full
// string has one of the registered keys as a prefix. Using prefixes lets the
// GDELT URL (with its date-based query string) match a static key, while
// exact-match keys still work for the hard-coded RSS feed URLs.
type geopoliticalMockTransport struct {
	mu        sync.Mutex
	responses map[string]string
	errors    map[string]error
	calls     []string
}

func newGeopoliticalMockTransport() *geopoliticalMockTransport {
	return &geopoliticalMockTransport{
		responses: map[string]string{},
		errors:    map[string]error{},
	}
}

func (m *geopoliticalMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	url := req.URL.String()
	m.calls = append(m.calls, url)

	for prefix, err := range m.errors {
		if strings.HasPrefix(url, prefix) {
			return nil, err
		}
	}
	for prefix, body := range m.responses {
		if strings.HasPrefix(url, prefix) {
			return cannedResponse(req, http.StatusOK, body), nil
		}
	}
	return cannedResponse(req, http.StatusNotFound, ""), nil
}

func cannedResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Request:    req,
	}
}

const (
	bbcFeedURL       = "http://feeds.bbci.co.uk/news/world/middle_east/rss.xml"
	aljazeeraFeedURL = "https://www.aljazeera.com/xml/rss/all.xml"
	gdeltFeedURL     = "https://api.gdeltproject.org/api/v2/summary/summary"
)

// conflictRSSXML has Israel/Hamas keywords that match the geopolitical feed's default keyword set.
const conflictRSSXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Conflict Feed</title>
    <item>
      <title>Israel Hamas conflict update</title>
      <description>Missile strikes reported in Gaza</description>
    </item>
  </channel>
</rss>`

// emptyRSSXML is a valid feed with no items — produces 0 matches.
const emptyRSSXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Quiet Feed</title>
  </channel>
</rss>`

// gdeltResponseJSON simulates a GDELT 2.0 article-count response.
const gdeltResponseJSON = `{"totalRecords": 15, "articles": []}`

// gdeltInvalidJSON makes GDELT's json.Unmarshal fail — surfaces as provider error.
const gdeltInvalidJSON = `{"totalRecords": "not-a-number"}`

// buildGeopoliticalAdapter wires a geopolitical adapter with mock-backed providers.
// Uses composite.SetHTTPClient (which propagates to inner providers) to also
// cover the convenience propagation method.
func buildGeopoliticalAdapter(t *testing.T, transport *geopoliticalMockTransport) *GeopoliticalChannelAdapter {
	t.Helper()

	rss := geopolitical.NewRSSGeopoliticalProvider()
	gdelt := geopolitical.NewGDELTGeopoliticalProvider()
	twRss := geopolitical.NewTaiwanRSSGeopoliticalProvider()

	globalComp := geopolitical.NewCompositeGeopoliticalProvider(rss, gdelt)
	globalComp.SetHTTPClient(&http.Client{Transport: transport})

	twComp := geopolitical.NewCompositeTaiwanGeopoliticalProvider(twRss)
	twComp.SetHTTPClient(&http.Client{Transport: transport})

	return &GeopoliticalChannelAdapter{
		workDir:        t.TempDir(),
		limiter:        rate.NewLimiter(rate.Every(time.Minute), 1),
		globalProvider: globalComp,
		taiwanProvider: twComp,
	}
}

// unmarshalGeopoliticalResult decodes the Fetch result's JSON payload.
func unmarshalGeopoliticalResult(t *testing.T, data []byte) struct {
	Global *geopolitical.GeopoliticalRiskScore `json:"global"`
	Taiwan *geopolitical.GeopoliticalRiskScore `json:"taiwan"`
} {
	t.Helper()
	var parsed struct {
		Global *geopolitical.GeopoliticalRiskScore `json:"global"`
		Taiwan *geopolitical.GeopoliticalRiskScore `json:"taiwan"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal geopolitical result: %v", err)
	}
	return parsed
}

// --- Fetch + HealthCheck tests --------------------------------------------

func TestGeopoliticalChannelAdapter_New(t *testing.T) {
	a := NewGeopoliticalChannelAdapter(t.TempDir())
	if a == nil {
		t.Fatal("NewGeopoliticalChannelAdapter returned nil")
	}
	if a.workDir == "" {
		t.Error("workDir should be set")
	}
	if a.globalProvider == nil {
		t.Error("globalProvider should be set")
	}
	if a.taiwanProvider == nil {
		t.Error("taiwanProvider should be set")
	}
	if a.limiter == nil {
		t.Error("limiter should be set")
	}
}

func TestGeopoliticalChannelAdapter_Fetch_Success(t *testing.T) {
	transport := newGeopoliticalMockTransport()
	transport.responses[bbcFeedURL] = conflictRSSXML
	transport.responses[aljazeeraFeedURL] = emptyRSSXML
	transport.responses[gdeltFeedURL] = gdeltResponseJSON
	transport.responses["https://www.cna.com.tw/cna/rss/rssfa.xml"] = emptyRSSXML

	a := buildGeopoliticalAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := a.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Fetch returned nil result")
	}
	if result.Meta.ChannelID != "geopolitical" {
		t.Errorf("Meta.ChannelID = %q, want geopolitical", result.Meta.ChannelID)
	}
	if result.Data == nil {
		t.Fatal("Fetch returned nil Data")
	}

	parsed := unmarshalGeopoliticalResult(t, result.Data)
	if parsed.Global == nil {
		t.Fatal("expected Global score, got nil")
	}
	if parsed.Taiwan == nil {
		t.Fatal("expected Taiwan score, got nil")
	}
	if parsed.Global.Region != "Middle East" {
		t.Errorf("Global.Region = %q, want Middle East", parsed.Global.Region)
	}
	if parsed.Taiwan.Region != "Taiwan" {
		t.Errorf("Taiwan.Region = %q, want Taiwan", parsed.Taiwan.Region)
	}
	// Global composite averages RSS + GDELT: Sources has both provider names.
	if !containsSource(parsed.Global.Sources, "rss_geopolitical") {
		t.Errorf("Global.Sources missing rss_geopolitical: %v", parsed.Global.Sources)
	}
	if !containsSource(parsed.Global.Sources, "gdelt_geopolitical") {
		t.Errorf("Global.Sources missing gdelt_geopolitical: %v", parsed.Global.Sources)
	}
}

func TestGeopoliticalChannelAdapter_Fetch_GlobalDegraded(t *testing.T) {
	// GDELT returns invalid JSON so GDELT's FetchScore errors out.
	// RSS succeeds, so the global composite produces a score using only RSS data.
	transport := newGeopoliticalMockTransport()
	transport.responses[bbcFeedURL] = conflictRSSXML
	transport.responses[aljazeeraFeedURL] = emptyRSSXML
	transport.responses[gdeltFeedURL] = gdeltInvalidJSON
	transport.responses["https://www.cna.com.tw/cna/rss/rssfa.xml"] = emptyRSSXML

	a := buildGeopoliticalAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := a.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	parsed := unmarshalGeopoliticalResult(t, result.Data)
	if parsed.Global == nil {
		t.Fatal("expected Global score (degraded but present), got nil")
	}
	if containsSource(parsed.Global.Sources, "gdelt_geopolitical") {
		t.Errorf("Global.Sources should not include gdelt_geopolitical after error: %v", parsed.Global.Sources)
	}
	if !containsSource(parsed.Global.Sources, "rss_geopolitical") {
		t.Errorf("Global.Sources should still include rss_geopolitical: %v", parsed.Global.Sources)
	}
	if parsed.Taiwan == nil {
		t.Error("expected Taiwan score, got nil")
	}
}

func TestGeopoliticalChannelAdapter_Fetch_TaiwanDegraded(t *testing.T) {
	// No mock entries for any Taiwan feed URL — all 4 feeds 404, RSS provider
	// returns 0-match score (Intensity=5 floor). Composite accepts it as a
	// "successful" provider, but the score is degraded.
	transport := newGeopoliticalMockTransport()
	transport.responses[bbcFeedURL] = conflictRSSXML
	transport.responses[aljazeeraFeedURL] = emptyRSSXML
	transport.responses[gdeltFeedURL] = gdeltResponseJSON

	a := buildGeopoliticalAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := a.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	parsed := unmarshalGeopoliticalResult(t, result.Data)
	if parsed.Global == nil {
		t.Fatal("expected Global score, got nil")
	}
	if parsed.Taiwan == nil {
		t.Fatal("expected Taiwan score (degraded but present), got nil")
	}
	if parsed.Taiwan.Intensity != 5 {
		t.Errorf("Taiwan.Intensity = %v, want 5 (minimum floor when 0 matches)", parsed.Taiwan.Intensity)
	}
}

func TestGeopoliticalChannelAdapter_Fetch_BothDegraded(t *testing.T) {
	// GDELT invalid + no Taiwan responses: both composites produce degraded scores.
	transport := newGeopoliticalMockTransport()
	transport.responses[bbcFeedURL] = conflictRSSXML
	transport.responses[aljazeeraFeedURL] = emptyRSSXML
	transport.responses[gdeltFeedURL] = gdeltInvalidJSON

	a := buildGeopoliticalAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := a.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	parsed := unmarshalGeopoliticalResult(t, result.Data)
	if parsed.Global == nil {
		t.Fatal("expected Global score (RSS only), got nil")
	}
	if parsed.Taiwan == nil {
		t.Fatal("expected Taiwan score (degraded), got nil")
	}
	if parsed.Taiwan.Intensity != 5 {
		t.Errorf("Taiwan.Intensity = %v, want 5 (degraded floor)", parsed.Taiwan.Intensity)
	}
}

func TestGeopoliticalChannelAdapter_HealthCheck_Ok(t *testing.T) {
	transport := newGeopoliticalMockTransport()
	transport.responses[bbcFeedURL] = conflictRSSXML
	transport.responses[aljazeeraFeedURL] = emptyRSSXML
	transport.responses[gdeltFeedURL] = gdeltResponseJSON
	transport.responses["https://www.cna.com.tw/cna/rss/rssfa.xml"] = emptyRSSXML

	a := buildGeopoliticalAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status, err := a.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
	if status.CheckType != "liveness" {
		t.Errorf("CheckType = %q, want liveness", status.CheckType)
	}
	if status.UpdatedAt == "" {
		t.Error("UpdatedAt should be set")
	}
}

func TestGeopoliticalChannelAdapter_HealthCheck_Degraded(t *testing.T) {
	// Even with degraded providers, the geopolitical adapter's Fetch never
	// returns an error (the RSS provider's FetchScore swallows per-feed errors
	// and returns a 0-match score). HealthCheck therefore still returns ok.
	transport := newGeopoliticalMockTransport()
	// No responses at all.

	a := buildGeopoliticalAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status, err := a.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok (degraded but no error path)", status.Status)
	}
}

// containsSource returns true if needle is in haystack.
func containsSource(haystack []string, needle string) bool {
	return slices.Contains(haystack, needle)
}
