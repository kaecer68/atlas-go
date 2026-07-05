package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

func TestBuildUSMacroChannels(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	snap := &marketdata.MacroDataSnapshot{
		RecordedAt: now.Unix(),
		SPXIndex:   marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5000, ChangePct: 0.5, Timestamp: now.Unix()},
		NDXIndex:   marketdata.MacroDataPoint{Symbol: "^NDX", Value: 18000, ChangePct: 0.3, Timestamp: now.Unix()},
		DJIIndex:   marketdata.MacroDataPoint{Symbol: "^DJI", Value: 38000, ChangePct: 0.2, Timestamp: now.Unix()},
		SOXIndex:   marketdata.MacroDataPoint{Symbol: "^SOX", Value: 4200, ChangePct: 1.2, Timestamp: now.Unix()},
		NVDA:       marketdata.MacroDataPoint{Symbol: "NVDA", Value: 120, ChangePct: 2.0, Timestamp: now.Unix()},
		AAPL:       marketdata.MacroDataPoint{Symbol: "AAPL", Value: 200, ChangePct: -0.5, Timestamp: now.Unix()},
		MSFT:       marketdata.MacroDataPoint{Symbol: "MSFT", Value: 420, ChangePct: 0.1, Timestamp: now.Unix()},
		TSMADR:     marketdata.MacroDataPoint{Symbol: "TSM", Value: 160, ChangePct: 0.8, Timestamp: now.Unix()},
	}

	svc := NewDataChannelService("/tmp", nil, nil, nil, nil, nil, "", "", "", "")
	channels := svc.buildUSMacroChannels(now, snap)

	if len(channels) != 8 {
		t.Fatalf("expected 8 channels, got %d", len(channels))
	}

	want := map[string]string{
		"us_spx":    "S\u0026P 500 指數 (^GSPC)",
		"us_ndx":    "NASDAQ 100 指數 (^NDX)",
		"us_dji":    "道瓊工業指數 (^DJI)",
		"sox_index": "費城半導體指數 (^SOX)",
		"us_nvda":   "NVIDIA (NVDA)",
		"us_aapl":   "Apple (AAPL)",
		"us_msft":   "Microsoft (MSFT)",
		"tsm_adr":   "台積電 ADR (TSM)",
	}

	seen := make(map[string]bool)
	for _, c := range channels {
		if c.ChannelID == "" {
			t.Error("channel with empty ChannelID")
		}
		if seen[c.ChannelID] {
			t.Errorf("duplicate channelID %q", c.ChannelID)
		}
		seen[c.ChannelID] = true

		wantLabel, ok := want[c.ChannelID]
		if !ok {
			t.Errorf("unexpected channelID %q", c.ChannelID)
			continue
		}
		if c.Platform != wantLabel {
			t.Errorf("%s: expected platform %q, got %q", c.ChannelID, wantLabel, c.Platform)
		}
		if c.Country != "美國" {
			t.Errorf("%s: expected country 美國, got %q", c.ChannelID, c.Country)
		}
		if c.Storage != "data/state/macro/latest.json" {
			t.Errorf("%s: expected storage data/state/macro/latest.json, got %q", c.ChannelID, c.Storage)
		}
		if c.Status != "ok" {
			t.Errorf("%s: expected status ok, got %q", c.ChannelID, c.Status)
		}
		if c.StatusText != "正常" {
			t.Errorf("%s: expected status text 正常, got %q", c.ChannelID, c.StatusText)
		}
	}

	for id := range want {
		if !seen[id] {
			t.Errorf("missing channel %q", id)
		}
	}
}

func TestBuildUSMacroChannels_NilSnapshot(t *testing.T) {
	now := time.Now()
	svc := NewDataChannelService("/tmp", nil, nil, nil, nil, nil, "", "", "", "")
	channels := svc.buildUSMacroChannels(now, nil)

	if len(channels) != 8 {
		t.Fatalf("expected 8 channels, got %d", len(channels))
	}

	for _, c := range channels {
		if c.Status != "error" {
			t.Errorf("%s: expected error status for nil snapshot, got %q", c.ChannelID, c.Status)
		}
		if c.StatusText != "異常" {
			t.Errorf("%s: expected status text 異常 for nil snapshot, got %q", c.ChannelID, c.StatusText)
		}
		if c.UpdatedAt == "" {
			t.Errorf("%s: expected non-empty UpdatedAt for nil snapshot", c.ChannelID)
		}
	}
}

func TestLatestMacroSnapshotOrZero_FallbackWithoutIngestor(t *testing.T) {
	tmpDir := t.TempDir()
	stateDir := filepath.Join(tmpDir, "data/state/macro")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	snap := marketdata.MacroDataSnapshot{
		RecordedAt: time.Now().Unix(),
		SPXIndex:   marketdata.MacroDataPoint{Symbol: "^GSPC", Value: 5000, Timestamp: time.Now().Unix()},
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "latest.json"), b, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	svc := NewDataChannelService(tmpDir, nil, nil, nil, nil, nil, "", "", "", "")
	got := svc.latestMacroSnapshotOrZero()
	if got == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if got.SPXIndex.Symbol != "^GSPC" {
		t.Errorf("expected SPX symbol ^GSPC, got %q", got.SPXIndex.Symbol)
	}
}

func TestLatestMacroSnapshotOrZero_MissingFile(t *testing.T) {
	svc := NewDataChannelService(t.TempDir(), nil, nil, nil, nil, nil, "", "", "", "")
	got := svc.latestMacroSnapshotOrZero()
	if got == nil {
		t.Fatal("expected non-nil snapshot on missing file")
	}
	if got.SPXIndex.Symbol != "" {
		t.Errorf("expected empty snapshot on missing file, got symbol %q", got.SPXIndex.Symbol)
	}
}
