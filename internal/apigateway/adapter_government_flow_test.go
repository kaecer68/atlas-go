package apigateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestGovernmentFlowAdapter_Metadata(t *testing.T) {
	a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(t.TempDir()))
	m := a.Metadata()
	if m.ChannelID != "government_flow" {
		t.Errorf("channel id=%s", m.ChannelID)
	}
	if !m.HasLimiter {
		t.Error("file-backed adapter must report HasLimiter=true per Constitution Art.2")
	}
	if a.RateLimit() == nil {
		t.Error("file-backed adapter must return non-nil limiter per Constitution Art.2")
	}
}

func TestGovernmentFlowAdapter_Fetch_StaleOnEmpty(t *testing.T) {
	a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(t.TempDir()))
	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("empty dir must not error: %v", err)
	}
	if !res.Stale {
		t.Error("empty dir must mark result Stale")
	}
	var p struct {
		Available bool `json:"available"`
	}
	if err := json.Unmarshal(res.Data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Available {
		t.Error("payload.available must be false on empty dir")
	}
}

func TestGovernmentFlowAdapter_Fetch_OK(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20260716.json"), []byte(`{"date":"20260716","total_net":2500000000,"source":"broker-aggregate"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(dir))
	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Stale {
		t.Error("expected fresh result")
	}
	var p struct {
		Available bool                              `json:"available"`
		Reading   *marketdata.GovernmentFlowReading `json:"reading"`
	}
	if err := json.Unmarshal(res.Data, &p); err != nil {
		t.Fatal(err)
	}
	if !p.Available || p.Reading == nil {
		t.Fatalf("expected reading: %+v", p)
	}
	if p.Reading.TotalNet != 2500000000 {
		t.Errorf("total_net=%d", p.Reading.TotalNet)
	}
}

func TestGovernmentFlowAdapter_HealthCheck_NoData(t *testing.T) {
	a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(t.TempDir()))
	h, err := a.HealthCheck(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.Status != "warn" {
		t.Errorf("expected warn status for empty dir, got %s", h.Status)
	}
}

func TestGovernmentFlowAdapter_Fetch_ContextCancelled(t *testing.T) {
	a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(t.TempDir()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := a.Fetch(ctx)
	if err == nil && res != nil && !res.Stale && !res.Fallback {
		t.Error("Fetch with cancelled context must not return a fresh result")
	}
	// File-backed adapter never errors on cancellation — it just sees no files.
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("unexpected error: %v", err)
	}
}
