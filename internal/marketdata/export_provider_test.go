package marketdata

import (
	"context"
	"testing"
	"time"
)

func TestExportStatisticsProvider_Name(t *testing.T) {
	p := NewExportStatisticsProvider()
	if p.Name() != "export_statistics" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestExportStatisticsProvider_FetchSnapshot(t *testing.T) {
	p := NewExportStatisticsProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Fatalf("FetchSnapshot failed: %v", err)
	}

	// Should return mock data when API is unavailable
	if snap.ExportElectronics.Symbol == "" {
		t.Fatalf("ExportElectronics should be populated")
	}

	if snap.ExportElectronics.Symbol != "TW_EXPORT_ELECTRONICS" {
		t.Fatalf("unexpected symbol: %s", snap.ExportElectronics.Symbol)
	}

	if snap.RecordedAt == 0 {
		t.Fatalf("RecordedAt should be set")
	}
}

func TestExportStatisticsProvider_MockSnapshot(t *testing.T) {
	p := NewExportStatisticsProvider()
	snap := p.mockSnapshot()

	if snap.ExportElectronics.Symbol != "TW_EXPORT_ELECTRONICS" {
		t.Fatalf("unexpected symbol: %s", snap.ExportElectronics.Symbol)
	}

	if snap.ExportElectronics.Value != 120.5 {
		t.Fatalf("unexpected value: %v", snap.ExportElectronics.Value)
	}

	if snap.ExportElectronics.ChangePct != 2.3 {
		t.Fatalf("unexpected change: %v", snap.ExportElectronics.ChangePct)
	}
}
