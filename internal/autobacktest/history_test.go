package autobacktest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestHistoryAppend(t *testing.T) {
	dir := t.TempDir()
	hist := NewHistory(dir)

	snap := AutoSnapshot{
		Date:          time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		PortfolioVal:  100000.0,
		VaR95:         -0.03,
		SharpeShort:   1.2,
		SharpeLong:    1.0,
		DrawdownPct:   -0.05,
		SignalCount:   1,
		ActiveSignals: []string{"VaR_WARNING"},
		ShortTermAvg:  102000.0,
		LongTermAvg:   101000.0,
		DeltaPct:      0.01,
	}

	if err := hist.Append(snap); err != nil {
		t.Fatalf("Append: %v", err)
	}

	filename := filepath.Join(dir, "autobacktest", "snapshots.jsonl")
	if _, err := os.Stat(filename); err != nil {
		t.Fatalf("expected snapshots.jsonl to exist: %v", err)
	}
}

func TestHistoryAppendCreatesDir(t *testing.T) {
	dir := t.TempDir()
	hist := NewHistory(dir)

	snap := AutoSnapshot{Date: time.Now()}
	if err := hist.Append(snap); err != nil {
		t.Fatalf("Append: %v", err)
	}

	expectedDir := filepath.Join(dir, "autobacktest")
	if _, err := os.Stat(expectedDir); err != nil {
		t.Fatalf("expected autobacktest dir to be created: %v", err)
	}
}

func TestHistoryLatestN(t *testing.T) {
	dir := t.TempDir()
	hist := NewHistory(dir)

	for i := 0; i < 5; i++ {
		snap := AutoSnapshot{
			Date:         time.Date(2026, 1, i+2, 0, 0, 0, 0, time.UTC),
			PortfolioVal: float64(10000 * (i + 1)),
		}
		if err := hist.Append(snap); err != nil {
			t.Fatalf("Append[%d]: %v", i, err)
		}
	}

	snaps, err := hist.LatestN(3)
	if err != nil {
		t.Fatalf("LatestN(3): %v", err)
	}
	if len(snaps) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snaps))
	}
	if snaps[0].Date.Day() != 4 {
		t.Fatalf("expected first snap day=4, got day=%d", snaps[0].Date.Day())
	}
	if snaps[2].Date.Day() != 6 {
		t.Fatalf("expected last snap day=6, got day=%d", snaps[2].Date.Day())
	}
}

func TestHistoryLatestNLessThanN(t *testing.T) {
	dir := t.TempDir()
	hist := NewHistory(dir)

	for i := 1; i <= 2; i++ {
		snap := AutoSnapshot{Date: time.Date(2026, 1, i+1, 0, 0, 0, 0, time.UTC)}
		if err := hist.Append(snap); err != nil {
			t.Fatalf("Append[%d]: %v", i, err)
		}
	}

	snaps, err := hist.LatestN(5)
	if err != nil {
		t.Fatalf("LatestN(5): %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots when N=5 and only 2 stored, got %d", len(snaps))
	}
}

func TestHistoryLatestNMissingFile(t *testing.T) {
	dir := t.TempDir()
	hist := NewHistory(dir)

	snaps, err := hist.LatestN(10)
	if err != nil {
		t.Fatalf("LatestN on missing file: expected nil err, got %v", err)
	}
	if snaps != nil {
		t.Fatalf("expected nil snapshots for missing file, got %v", snaps)
	}
}

func TestAutoSnapshotJSONRoundtrip(t *testing.T) {
	snap := AutoSnapshot{
		Date:          time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
		PortfolioVal:  123456.78,
		VaR95:         -0.04,
		SharpeShort:   1.5,
		SharpeLong:    1.2,
		DrawdownPct:   -0.08,
		SignalCount:   2,
		ActiveSignals: []string{"VaR_WARNING", "SHARPE_DEGRADATION"},
		ShortTermAvg:  125000.0,
		LongTermAvg:   120000.0,
		DeltaPct:      0.0417,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var restored AutoSnapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if restored.PortfolioVal != snap.PortfolioVal {
		t.Errorf("PortfolioVal: got %f, want %f", restored.PortfolioVal, snap.PortfolioVal)
	}
	if restored.VaR95 != snap.VaR95 {
		t.Errorf("VaR95: got %f, want %f", restored.VaR95, snap.VaR95)
	}
	if len(restored.ActiveSignals) != len(snap.ActiveSignals) {
		t.Errorf("ActiveSignals length: got %d, want %d", len(restored.ActiveSignals), len(snap.ActiveSignals))
	}
}

var _ domain.Regime = domain.RegimeRiskOn
