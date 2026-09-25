package marketdata

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSectorData(t *testing.T, dir, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := filepath.Join(dir, "sector_data.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

const sectorDataSample = `{"ai_revenue_growth":45.2,"cowos_utilization":92.5,"capex_growth":15.3,"semiconductor_index":4520,"updated_at":"2026-05-12T00:00:00+08:00"}`

// TestResolveSectorDataDirMatchesShippedFile is the repo-layout evidence for
// issue #1944 Batch 2 (Q6 I14): the single path authority must resolve to the
// file the repository actually ships, and the legacy reader paths must not
// exist (they were what made the channel silently dead).
func TestResolveSectorDataDirMatchesShippedFile(t *testing.T) {
	root := filepath.Join("..", "..")
	shipped := filepath.Join(ResolveSectorDataDir(root), "sector_data.json")
	if _, err := os.Stat(shipped); err != nil {
		t.Fatalf("ResolveSectorDataDir does not point at the shipped file %s: %v", shipped, err)
	}
	for _, legacy := range []string{
		filepath.Join(root, "data", "state", "sector_data", "sector_data.json"),
		filepath.Join(root, "data", "state", "sector_data.json"),
		filepath.Join(root, "sector_data.json"),
	} {
		if _, err := os.Stat(legacy); err == nil {
			t.Errorf("legacy reader path %s exists — re-check the I14 path-mismatch finding", legacy)
		}
	}
}

// TestSectorDataProvider_StateReportsFreshness proves a successful read exposes
// the file's OWN updated_at, so a stale file can be told apart from a fresh one
// (the provider still degrades to zeros with a nil error).
func TestSectorDataProvider_StateReportsFreshness(t *testing.T) {
	dir := t.TempDir()
	writeSectorData(t, dir, sectorDataSample)

	p := NewSectorDataProvider(dir)
	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if snap.TSMCRevenue.Value != 45.2 {
		t.Fatalf("TSMCRevenue.Value = %v, want 45.2", snap.TSMCRevenue.Value)
	}
	state := p.State()
	if !state.Found {
		t.Fatalf("State().Found = false, want true (%+v)", state)
	}
	want, _ := time.Parse(time.RFC3339, "2026-05-12T00:00:00+08:00")
	if !state.DataUpdatedAt.Equal(want) {
		t.Fatalf("DataUpdatedAt = %s, want %s", state.DataUpdatedAt, want)
	}
	if state.Reason != "ok" {
		t.Fatalf("Reason = %q, want ok", state.Reason)
	}
	if state.Path != filepath.Join(dir, "sector_data.json") {
		t.Fatalf("Path = %q", state.Path)
	}
}

// TestSectorDataProvider_StateReportsMissing keeps the graceful-degradation
// contract (zeros + nil error) while making the absence observable.
func TestSectorDataProvider_StateReportsMissing(t *testing.T) {
	p := NewSectorDataProvider(filepath.Join(t.TempDir(), "nope"))
	snap, err := p.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if snap.TSMCRevenue.Value != 0 || snap.CapexGrowth.Value != 0 {
		t.Fatalf("missing file must yield a zero snapshot, got %+v", snap)
	}
	state := p.State()
	if state.Found || state.Reason != "missing" || !state.DataUpdatedAt.IsZero() {
		t.Fatalf("State() = %+v, want Found=false Reason=missing", state)
	}
}

// TestSectorDataProvider_StateReportsBadInput covers the two silent cases that
// used to look identical to "all zeros": malformed JSON and an unparseable
// updated_at.
func TestSectorDataProvider_StateReportsBadInput(t *testing.T) {
	t.Run("invalid_json", func(t *testing.T) {
		dir := t.TempDir()
		writeSectorData(t, dir, "{not json")
		p := NewSectorDataProvider(dir)
		if _, err := p.FetchSnapshot(context.Background()); err != nil {
			t.Fatalf("invalid json must degrade, got %v", err)
		}
		if state := p.State(); state.Found || state.Reason != "invalid_json" {
			t.Fatalf("State() = %+v, want Found=false Reason=invalid_json", state)
		}
	})
	t.Run("unparseable_timestamp", func(t *testing.T) {
		dir := t.TempDir()
		writeSectorData(t, dir, `{"ai_revenue_growth":10,"updated_at":"12 May 2026"}`)
		p := NewSectorDataProvider(dir)
		if _, err := p.FetchSnapshot(context.Background()); err != nil {
			t.Fatalf("unparseable timestamp must degrade, got %v", err)
		}
		state := p.State()
		if !state.Found || state.Reason != "unparseable_timestamp" || !state.DataUpdatedAt.IsZero() {
			t.Fatalf("State() = %+v, want Found=true Reason=unparseable_timestamp DataUpdatedAt=zero", state)
		}
	})
}
