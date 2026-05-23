//go:build integration

package repository

import (
	"context"
	"testing"
	"time"
)

func TestDualWriteMetrics_RecordAndQuery(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	// Record a metric
	err := repo.Record(ctx, "test_metric", 42.5, map[string]string{
		"agent_id":   "test_agent",
		"session_id": "test_session",
		"symbol":     "2330.TW",
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// QueryRange — should find our metric
	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	points, err := repo.QueryRange(ctx, "test_metric", start, end)
	if err != nil {
		t.Fatalf("QueryRange failed: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("Expected at least 1 metric point")
	}
	if points[0].Value != 42.5 {
		t.Errorf("Expected value 42.5, got %f", points[0].Value)
	}
	if points[0].Name != "test_metric" {
		t.Errorf("Expected metric_name 'test_metric', got %q", points[0].Name)
	}
}

func TestDualWriteMetrics_QueryLatest(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	// Record two values and get the latest
	_ = repo.Record(ctx, "latest_test", 10.0, map[string]string{"agent_id": "a1"})
	_ = repo.Record(ctx, "latest_test", 20.0, map[string]string{"agent_id": "a1"})

	// Allow a small delay for time ordering
	time.Sleep(10 * time.Millisecond)

	latest, err := repo.QueryLatest(ctx, "latest_test", map[string]string{"agent_id": "a1"})
	if err != nil {
		t.Fatalf("QueryLatest failed: %v", err)
	}
	if latest == nil {
		t.Fatal("Expected non-nil result from QueryLatest")
	}
	if latest.Value != 20.0 {
		t.Errorf("Expected latest value 20.0, got %f", latest.Value)
	}
}

func TestDualWriteMetrics_Aggregate(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	// Record multiple values
	for _, v := range []float64{10, 20, 30, 40} {
		_ = repo.Record(ctx, "agg_test", v, map[string]string{"agent_id": "a1"})
	}

	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)

	sum, err := repo.Aggregate(ctx, "agg_test", start, end, "sum")
	if err != nil {
		t.Fatalf("Aggregate sum failed: %v", err)
	}
	if sum != 100 {
		t.Errorf("Expected sum 100, got %f", sum)
	}

	avg, err := repo.Aggregate(ctx, "agg_test", start, end, "avg")
	if err != nil {
		t.Fatalf("Aggregate avg failed: %v", err)
	}
	if avg != 25 {
		t.Errorf("Expected avg 25, got %f", avg)
	}

	count, err := repo.Aggregate(ctx, "agg_test", start, end, "count")
	if err != nil {
		t.Fatalf("Aggregate count failed: %v", err)
	}
	if count != 4 {
		t.Errorf("Expected count 4, got %f", count)
	}
}

func TestDualWriteMetrics_SaveSnapshotRoundTrip(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	snap := &MetricsSnapshot{
		ScreeningTotal:     100,
		ScreeningPassed:    75,
		ScreeningRate:      0.75,
		AlertsTriggered:    5,
		AlertsAcknowledged: 3,
		Timestamp:          time.Now(),
	}

	err := repo.SaveSnapshot(ctx, snap)
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	// Load today — should find the snapshot
	loaded, err := repo.LoadToday(ctx)
	if err != nil {
		t.Fatalf("LoadToday failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("Expected non-nil snapshot from LoadToday")
	}
	if loaded.ScreeningTotal != 100 {
		t.Errorf("Expected ScreeningTotal 100, got %d", loaded.ScreeningTotal)
	}
	if loaded.ScreeningRate != 0.75 {
		t.Errorf("Expected ScreeningRate 0.75, got %f", loaded.ScreeningRate)
	}
}

func TestDualWriteMetrics_LoadRecent(t *testing.T) {
	repo := newTestDualWrite(t)
	ctx := context.Background()

	// Save a snapshot
	snap := &MetricsSnapshot{
		ScreeningTotal: 50,
		Timestamp:      time.Now(),
	}
	if err := repo.SaveSnapshot(ctx, snap); err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	recent, err := repo.LoadRecent(ctx, 10)
	if err != nil {
		t.Fatalf("LoadRecent failed: %v", err)
	}
	if len(recent) == 0 {
		t.Fatal("Expected at least 1 recent snapshot")
	}
	if recent[0].ScreeningTotal != 50 {
		t.Errorf("Expected ScreeningTotal 50, got %d", recent[0].ScreeningTotal)
	}
}
