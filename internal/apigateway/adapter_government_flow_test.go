package apigateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// TestGovernmentFlowAdapter_Fetch_ZeroReadingIsUnavailable is the R1
// channel-level regression: a 0-value placeholder file must not be exposed
// as available data, or the gateway seeds MacroDataSnapshot.GovernmentNet
// with it and the capital-flow pipeline persists a zero sample.
func TestGovernmentFlowAdapter_Fetch_ZeroReadingIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	// The exact shape found in data/state/government_flow/20260728.json.
	if err := os.WriteFile(filepath.Join(dir, "20260728.json"), []byte(`{"date":"20260728","total_net":0,"source":"broker-aggregate"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(dir))
	res, err := a.Fetch(context.Background())
	if err != nil {
		t.Fatalf("zero reading must not error: %v", err)
	}
	if !res.Stale {
		t.Error("zero reading must mark the result Stale")
	}
	var p struct {
		Available bool                              `json:"available"`
		Reading   *marketdata.GovernmentFlowReading `json:"reading"`
	}
	if err := json.Unmarshal(res.Data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Available {
		t.Error("payload.available must be false for a zero reading (CF-INV-06)")
	}
	if p.Reading != nil {
		t.Errorf("payload must not carry the placeholder reading, got %+v", p.Reading)
	}
}

// TestGovernmentFlowAdapter_DataState locks the contract hook that turns a
// placeholder file into a degraded channel instead of a false "ok".
func TestGovernmentFlowAdapter_DataState(t *testing.T) {
	t.Run("missing reading", func(t *testing.T) {
		a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(t.TempDir()))
		ds, err := a.DataState(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if ds.Present || ds.NonZero {
			t.Errorf("DataState = %+v, want Present=false NonZero=false", ds)
		}
	})
	t.Run("zero placeholder", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "20260728.json"), []byte(`{"date":"20260728","total_net":0,"source":"broker-aggregate"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(dir))
		ds, err := a.DataState(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ds.Present {
			t.Error("DataState.Present must be true: the file exists")
		}
		if ds.NonZero {
			t.Error("DataState.NonZero must be false for a zero reading")
		}
		if ds.RecordedAt.IsZero() {
			t.Error("DataState.RecordedAt must carry the reading date")
		}
	})
	t.Run("real reading", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "20260904.json"), []byte(`{"date":"20260904","total_net":-6441260000,"source":"media-curated"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(dir))
		ds, err := a.DataState(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ds.Present || !ds.NonZero {
			t.Errorf("DataState = %+v, want Present=true NonZero=true", ds)
		}
		if got := ds.RecordedAt.Format("2006-01-02"); got != "2026-09-04" {
			t.Errorf("RecordedAt = %s, want 2026-09-04", got)
		}
	})
}

// TestEvaluateContractHealth_GovernmentFlowZeroReadingDegrades is the
// registry-level R1 gate: with SuccessCriteria=value_nonzero, an "ok"
// HealthCheck on a placeholder file must come back degraded.
func TestEvaluateContractHealth_GovernmentFlowZeroReadingDegrades(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20260728.json"), []byte(`{"date":"20260728","total_net":0,"source":"broker-aggregate"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a := NewGovernmentFlowAdapter(marketdata.NewGovernmentFlowProvider(dir))
	contract := ChannelContracts().Contract("government_flow")
	base := HealthStatus{Status: "ok", UpdatedAt: time.Now().Format(time.RFC3339)}
	got := EvaluateContractHealth(context.Background(), contract, a, base)
	if got.Status != "degraded" {
		t.Fatalf("status = %q, want degraded (zero total_net fails value_nonzero)", got.Status)
	}
	if got.LastError == "" {
		t.Error("degraded health must carry the data-state reason")
	}

	// A real reading keeps "ok".
	if err := os.WriteFile(filepath.Join(dir, "20260827.json"), []byte(`{"date":"20260827","total_net":-10384970000,"source":"media-curated"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := EvaluateContractHealth(context.Background(), contract, a, HealthStatus{Status: "ok"}); got.Status != "ok" {
		t.Errorf("status = %q, want ok for a non-zero reading", got.Status)
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
