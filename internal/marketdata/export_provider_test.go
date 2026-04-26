package marketdata

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestExportStatisticsProvider_Name(t *testing.T) {
	p := NewExportStatisticsProvider("data/state/export")
	if p.Name() != "export_statistics" {
		t.Fatalf("unexpected name: %s", p.Name())
	}
}

func TestExportStatisticsProvider_FetchSnapshot_RealAPI(t *testing.T) {
	p := NewExportStatisticsProvider("data/state/export")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	snap, err := p.FetchSnapshot(ctx)
	if err != nil {
		t.Logf("ExportStatisticsProvider.FetchSnapshot returned error (may be expected in CI): %v", err)
		return
	}

	if snap.ExportElectronics.Symbol != "TW_EXPORT_ELECTRONICS" {
		t.Fatalf("unexpected symbol: %s", snap.ExportElectronics.Symbol)
	}
	if snap.RecordedAt == 0 {
		t.Fatalf("RecordedAt should be set")
	}
}

func TestExportStatisticsProvider_FetchSnapshot_Decommissioned(t *testing.T) {
	p := ExportStatisticsProviderWithClient(http.DefaultClient, t.TempDir())
	p.baseURL = "https://fake-test-server"
	p.client.Timeout = 5 * time.Second

	ctx := context.Background()
	_, err := p.FetchSnapshot(ctx)
	if err == nil {
		t.Fatal("expected error since FAS210 is decommissioned")
	}
}
