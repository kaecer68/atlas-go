//go:build integration

package repository

import (
	"context"
	"testing"
	"time"
)

func TestDualWriteCapitalFlow_RecordAndQueryLatest(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	if err := repo.RecordCapitalFlow(ctx, "foreign", 100.0, 500.0, 400.0); err != nil {
		t.Fatalf("RecordCapitalFlow failed: %v", err)
	}

	// Allow a small delay for time ordering
	time.Sleep(10 * time.Millisecond)

	if err := repo.RecordCapitalFlow(ctx, "foreign", 200.0, 600.0, 400.0); err != nil {
		t.Fatalf("RecordCapitalFlow (second) failed: %v", err)
	}

	latest, err := repo.QueryLatestCapitalFlow(ctx, "foreign")
	if err != nil {
		t.Fatalf("QueryLatestCapitalFlow failed: %v", err)
	}
	if latest == nil {
		t.Fatal("Expected non-nil CapitalFlowRecord")
	}
	if latest.NetBuy != 200.0 {
		t.Errorf("Expected NetBuy 200.0, got %f", latest.NetBuy)
	}
	if latest.Channel != "foreign" {
		t.Errorf("Expected channel 'foreign', got %q", latest.Channel)
	}
}

func TestDualWriteCapitalFlow_QueryRange(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	if err := repo.RecordCapitalFlow(ctx, "domestic", 50.0, 300.0, 250.0); err != nil {
		t.Fatalf("RecordCapitalFlow failed: %v", err)
	}

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	records, err := repo.QueryCapitalFlowRange(ctx, "domestic", start, end)
	if err != nil {
		t.Fatalf("QueryCapitalFlowRange failed: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("Expected at least 1 capital flow record")
	}
	if records[0].NetBuy != 50.0 {
		t.Errorf("Expected NetBuy 50.0, got %f", records[0].NetBuy)
	}
}

func TestDualWriteExportStats_SaveAndQueryLatest(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	if err := repo.SaveExportStats(ctx, 114, 5, 1000.0, 800.0, 200.0); err != nil {
		t.Fatalf("SaveExportStats failed: %v", err)
	}

	latest, err := repo.QueryLatestExportStats(ctx)
	if err != nil {
		t.Fatalf("QueryLatestExportStats failed: %v", err)
	}
	if latest == nil {
		t.Fatal("Expected non-nil ExportStatsRecord")
	}
	if latest.ExportTotal != 1000.0 {
		t.Errorf("Expected ExportTotal 1000.0, got %f", latest.ExportTotal)
	}
	if latest.ImportTotal != 800.0 {
		t.Errorf("Expected ImportTotal 800.0, got %f", latest.ImportTotal)
	}
	if latest.TradeBalance != 200.0 {
		t.Errorf("Expected TradeBalance 200.0, got %f", latest.TradeBalance)
	}
}

func TestDualWriteExportStats_QueryByYearMonth(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	if err := repo.SaveExportStats(ctx, 114, 5, 2000.0, 1500.0, 500.0); err != nil {
		t.Fatalf("SaveExportStats failed: %v", err)
	}

	rec, err := repo.QueryExportStatsByYearMonth(ctx, 114, 5)
	if err != nil {
		t.Fatalf("QueryExportStatsByYearMonth failed: %v", err)
	}
	if rec == nil {
		t.Fatal("Expected non-nil ExportStatsRecord")
	}
	if rec.Year != 114 || rec.Month != 5 {
		t.Errorf("Expected year=114 month=5, got year=%d month=%d", rec.Year, rec.Month)
	}

	// Upsert — same year/month should update
	if err := repo.SaveExportStats(ctx, 114, 5, 2100.0, 1600.0, 500.0); err != nil {
		t.Fatalf("SaveExportStats (upsert) failed: %v", err)
	}
	rec2, err := repo.QueryExportStatsByYearMonth(ctx, 114, 5)
	if err != nil {
		t.Fatalf("QueryExportStatsByYearMonth (after upsert) failed: %v", err)
	}
	if rec2.ExportTotal != 2100.0 {
		t.Errorf("Expected upserted ExportTotal 2100.0, got %f", rec2.ExportTotal)
	}
}
