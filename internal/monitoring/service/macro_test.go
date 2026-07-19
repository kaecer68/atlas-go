package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestNewMacroService(t *testing.T) {
	svc := NewMacroService("/tmp/workdir", nil, nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.WorkDir != "/tmp/workdir" {
		t.Errorf("expected WorkDir /tmp/workdir, got %q", svc.WorkDir)
	}
}

type stubProvider struct{}

func (stubProvider) Name() string { return "stub" }
func (stubProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	return marketdata.MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
}

func writeDatedSnapshot(t *testing.T, dir, date string, snap marketdata.MacroDataSnapshot, corrupt bool) {
	t.Helper()
	if snap.RecordedAt == 0 {
		snap.RecordedAt = time.Now().Unix()
	}
	var data []byte
	if corrupt {
		data = []byte("{not valid json")
	} else {
		var err error
		data, err = json.Marshal(snap)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, date+".json"), data, 0o644); err != nil {
		t.Fatalf("write %s: %v", date, err)
	}
}

func newServiceWithDatedSnapshots(t *testing.T, dated map[string]marketdata.MacroDataSnapshot, corruptDates map[string]bool) *MacroService {
	t.Helper()
	dir := t.TempDir()
	for date, snap := range dated {
		writeDatedSnapshot(t, dir, date, snap, false)
	}
	for date := range corruptDates {
		writeDatedSnapshot(t, dir, date, marketdata.MacroDataSnapshot{}, true)
	}
	ingestor := narrative.NewMacroIngestor(stubProvider{}, dir)
	return NewMacroService(t.TempDir(), ingestor, narrative.NewTaiwanStressCalculator(nil, ""))
}

func TestListSnapshotsInRange_FullRange(t *testing.T) {
	dated := map[string]marketdata.MacroDataSnapshot{
		"2026-04-21": {DXY: marketdata.MacroDataPoint{Symbol: "DXY", Value: 104}},
		"2026-04-22": {DXY: marketdata.MacroDataPoint{Symbol: "DXY", Value: 105}},
		"2026-04-23": {DXY: marketdata.MacroDataPoint{Symbol: "DXY", Value: 106}},
	}
	svc := newServiceWithDatedSnapshots(t, dated, nil)

	snapshots, missing, capHit, err := svc.ListSnapshotsInRange(context.Background(), "2026-04-21", "2026-04-23", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
	}
	if len(missing) != 0 {
		t.Errorf("expected 0 missing, got %v", missing)
	}
	if capHit {
		t.Error("expected capacityLimitHit=false")
	}
	if snapshots[0].TradingDate != "2026-04-21" {
		t.Errorf("first snapshot date = %q, want 2026-04-21", snapshots[0].TradingDate)
	}
	if snapshots[2].TradingDate != "2026-04-23" {
		t.Errorf("last snapshot date = %q, want 2026-04-23", snapshots[2].TradingDate)
	}
	for _, s := range snapshots {
		if s.SourceStatus != "complete" {
			t.Errorf("expected complete, got %q for %s", s.SourceStatus, s.TradingDate)
		}
		if s.Snapshot == nil {
			t.Errorf("expected non-nil snapshot for %s", s.TradingDate)
		}
	}
}

func TestListSnapshotsInRange_MissingAndCorrupt(t *testing.T) {
	dated := map[string]marketdata.MacroDataSnapshot{
		"2026-04-21": {DXY: marketdata.MacroDataPoint{Value: 100}},
	}
	corrupt := map[string]bool{"2026-04-22": true}
	svc := newServiceWithDatedSnapshots(t, dated, corrupt)

	snapshots, missing, _, err := svc.ListSnapshotsInRange(context.Background(), "2026-04-20", "2026-04-25", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("expected 1 valid snapshot, got %d", len(snapshots))
	}
	if len(missing) != 1 {
		t.Fatalf("expected 1 missing (corrupt file), got %v", missing)
	}
	if missing[0] != "2026-04-22" {
		t.Errorf("missing date = %q, want 2026-04-22", missing[0])
	}
}

func TestListSnapshotsInRange_CapacityClamp(t *testing.T) {
	dated := make(map[string]marketdata.MacroDataSnapshot)
	for i := 1; i <= 10; i++ {
		date := time.Date(2026, 4, i, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		dated[date] = marketdata.MacroDataSnapshot{DXY: marketdata.MacroDataPoint{Value: float64(i)}}
	}
	svc := newServiceWithDatedSnapshots(t, dated, nil)

	snapshots, _, capHit, err := svc.ListSnapshotsInRange(context.Background(), "2026-04-01", "2026-04-30", 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(snapshots) != 5 {
		t.Fatalf("expected 5 snapshots (clamped), got %d", len(snapshots))
	}
	if !capHit {
		t.Error("expected capacityLimitHit=true")
	}
	if snapshots[0].TradingDate != "2026-04-06" {
		t.Errorf("expected most-recent 5 to start at 2026-04-06, got %s", snapshots[0].TradingDate)
	}
	if snapshots[4].TradingDate != "2026-04-10" {
		t.Errorf("expected most-recent 5 to end at 2026-04-10, got %s", snapshots[4].TradingDate)
	}
}

func TestListSnapshotsInRange_LimitCapAt365(t *testing.T) {
	dated := map[string]marketdata.MacroDataSnapshot{
		"2026-04-21": {DXY: marketdata.MacroDataPoint{Value: 100}},
	}
	svc := newServiceWithDatedSnapshots(t, dated, nil)

	_, _, _, err := svc.ListSnapshotsInRange(context.Background(), "2026-04-21", "2026-04-21", 999)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestListSnapshotsInRange_UnreadableDir(t *testing.T) {
	ingestor := narrative.NewMacroIngestor(stubProvider{}, "/nonexistent/path/that/does/not/exist")
	svc := NewMacroService(t.TempDir(), ingestor, narrative.NewTaiwanStressCalculator(nil, ""))
	_, _, _, err := svc.ListSnapshotsInRange(context.Background(), "2026-04-21", "2026-04-21", 0)
	if err == nil {
		t.Fatal("expected error when snapshot dir unreadable")
	}
}
