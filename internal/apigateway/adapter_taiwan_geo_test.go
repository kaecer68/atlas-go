package apigateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

// --- existing minimal tests (kept verbatim) -------------------------------

func TestTaiwanGeopoliticalChannelAdapter_Metadata(t *testing.T) {
	a := &TaiwanGeopoliticalChannelAdapter{}
	m := a.Metadata()
	if m.ChannelID != "geopolitical_taiwan" {
		t.Errorf("ChannelID = %q, want geopolitical_taiwan", m.ChannelID)
	}
	if m.Country != "台灣" {
		t.Errorf("Country = %q, want 台灣", m.Country)
	}
	if m.Platform != "CNA / 自由時報 / TVBS RSS" {
		t.Errorf("Platform = %q, want CNA / 自由時報 / TVBS RSS", m.Platform)
	}
	if m.APIFormat != "RSS XML" {
		t.Errorf("APIFormat = %q, want RSS XML", m.APIFormat)
	}
	if m.Path != "www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw" {
		t.Errorf("Path = %q, want www.cna.com.tw / news.ltn.com.tw / news.tvbs.com.tw", m.Path)
	}
	if !m.HasLimiter {
		t.Error("HasLimiter should be true")
	}
}

func TestTaiwanGeopoliticalChannelAdapter_RateLimit(t *testing.T) {
	a := &TaiwanGeopoliticalChannelAdapter{
		limiter: rate.NewLimiter(rate.Every(time.Minute), 1),
	}
	limiter := a.RateLimit()
	if limiter == nil {
		t.Fatal("RateLimit() returned nil")
	}
}

// --- Fetch + HealthCheck tests --------------------------------------------

// taiwanRSSXML is a minimal valid RSS document with cross-strait keywords.
const taiwanRSSXML = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Taiwan Geopolitical Feed</title>
    <item>
      <title>台海緊張局勢升溫</title>
      <description>兩岸關係最新發展與軍演動態</description>
    </item>
  </channel>
</rss>`

// buildTaiwanGeoAdapter wires a taiwan_geo adapter with a mock-backed provider.
// Uses composite.SetHTTPClient to cover the propagation method.
func buildTaiwanGeoAdapter(t *testing.T, transport *geopoliticalMockTransport) *TaiwanGeopoliticalChannelAdapter {
	t.Helper()

	twRss := geopolitical.NewTaiwanRSSGeopoliticalProvider()
	twComp := geopolitical.NewCompositeTaiwanGeopoliticalProvider(twRss)
	twComp.SetHTTPClient(&http.Client{Transport: transport})

	return &TaiwanGeopoliticalChannelAdapter{
		provider: twRss,
		workDir:  t.TempDir(),
		limiter:  rate.NewLimiter(rate.Every(time.Minute), 1),
	}
}

func TestTaiwanGeopoliticalChannelAdapter_New(t *testing.T) {
	a := NewTaiwanGeopoliticalChannelAdapter(t.TempDir())
	if a == nil {
		t.Fatal("NewTaiwanGeopoliticalChannelAdapter returned nil")
	}
	if a.provider == nil {
		t.Error("provider should be set")
	}
	if a.workDir == "" {
		t.Error("workDir should be set")
	}
	if a.limiter == nil {
		t.Error("limiter should be set")
	}
}

func TestTaiwanGeopoliticalChannelAdapter_Fetch_Success(t *testing.T) {
	transport := newGeopoliticalMockTransport()
	transport.responses["https://www.cna.com.tw/cna/rss/rssfa.xml"] = taiwanRSSXML

	a := buildTaiwanGeoAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := a.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result == nil {
		t.Fatal("Fetch returned nil result")
	}
	if result.Meta.ChannelID != "geopolitical_taiwan" {
		t.Errorf("Meta.ChannelID = %q, want geopolitical_taiwan", result.Meta.ChannelID)
	}
	if result.Data == nil {
		t.Fatal("Fetch returned nil Data")
	}

	var score geopolitical.GeopoliticalRiskScore
	if err := json.Unmarshal(result.Data, &score); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if score.Region != "Taiwan" {
		t.Errorf("Region = %q, want Taiwan", score.Region)
	}
}

func TestTaiwanGeopoliticalChannelAdapter_Fetch_Degraded(t *testing.T) {
	// No mock entries — all 4 feeds 404. The TaiwanRSS provider's FetchScore
	// still returns a score (Intensity=5 floor) but with no successful feeds.
	// The adapter accepts this and never errors.
	transport := newGeopoliticalMockTransport()

	a := buildTaiwanGeoAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := a.Fetch(ctx)
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}

	var score geopolitical.GeopoliticalRiskScore
	if err := json.Unmarshal(result.Data, &score); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if score.Intensity != 5 {
		t.Errorf("Intensity = %v, want 5 (degraded floor when 0 matches)", score.Intensity)
	}
}

func TestTaiwanGeopoliticalChannelAdapter_HealthCheck_Ok(t *testing.T) {
	transport := newGeopoliticalMockTransport()
	transport.responses["https://www.cna.com.tw/cna/rss/rssfa.xml"] = taiwanRSSXML

	a := buildTaiwanGeoAdapter(t, transport)

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

func TestTaiwanGeopoliticalChannelAdapter_HealthCheck_Degraded(t *testing.T) {
	// Provider never returns an error → HealthCheck still returns ok even
	// when no feeds succeed. The error branch in HealthCheck is unreachable
	// through public APIs given the current provider implementation.
	transport := newGeopoliticalMockTransport()

	a := buildTaiwanGeoAdapter(t, transport)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	status, err := a.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck error: %v", err)
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok (no error path in provider)", status.Status)
	}
}
