package liveness

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

func TestIsStale_NeverRanIsStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	if !IsStale(time.Time{}, 5*time.Minute, now) {
		t.Error("zero lastRun must be stale")
	}
}

func TestIsStale_ZeroIntervalNeverStale(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	if IsStale(now.Add(-100*time.Hour), 0, now) {
		t.Error("zero interval must never be stale")
	}
}

func TestIsStale_Boundary(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	interval := 5 * time.Minute

	// Exactly interval*3 ago: NOT stale (strictly greater required).
	exact := now.Add(-interval * StaleFactor)
	if IsStale(exact, interval, now) {
		t.Errorf("lastRun exactly interval*3 ago must not be stale (got stale)")
	}
	// Just past interval*3: stale.
	past := now.Add(-interval*StaleFactor - time.Second)
	if !IsStale(past, interval, now) {
		t.Errorf("lastRun just past interval*3 must be stale")
	}
	// Fresh run: not stale.
	if IsStale(now.Add(-interval), interval, now) {
		t.Errorf("lastRun within interval must not be stale")
	}
}

// fakeIntervalProvider resolves only the given tasks.
type fakeIntervalProvider struct {
	intervals map[string]time.Duration
}

func (f *fakeIntervalProvider) Get(name string) (*apigateway.ScheduledTask, bool) {
	d, ok := f.intervals[name]
	if !ok {
		return nil, false
	}
	return &apigateway.ScheduledTask{Name: name, Interval: d}, true
}

// fakeLister implements the lister interface for monitor tests.
type fakeLister struct {
	rows []Row
	err  error
}

func (f fakeLister) List(context.Context) ([]Row, error) { return f.rows, f.err }

type alertCall struct {
	name     string
	staleFor time.Duration
	interval time.Duration
}

func TestStalenessMonitor_AlertsOncePerStaleEpisode(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	interval := 5 * time.Minute
	rows := []Row{
		{TaskName: "stale_task", LastRunAt: now.Add(-30 * time.Minute)}, // > 15m stale
		{TaskName: "fresh_task", LastRunAt: now.Add(-2 * time.Minute)},  // fresh
	}
	intervals := map[string]time.Duration{"stale_task": interval, "fresh_task": interval}

	var calls []alertCall
	m := NewStalenessMonitor(nil, &fakeIntervalProvider{intervals: intervals}, func(name string, staleFor, iv time.Duration) {
		calls = append(calls, alertCall{name: name, staleFor: staleFor, interval: iv})
	})
	m.store = fakeLister{rows: rows}

	if err := m.CheckOnce(context.Background(), now); err != nil {
		t.Fatalf("CheckOnce: %v", err)
	}
	if len(calls) != 1 || calls[0].name != "stale_task" {
		t.Fatalf("expected 1 alert for stale_task, got %+v", calls)
	}

	// Second check, same state: no duplicate alert.
	if err := m.CheckOnce(context.Background(), now.Add(time.Minute)); err != nil {
		t.Fatalf("CheckOnce: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected still 1 alert (dedup), got %d", len(calls))
	}

	// Task runs again: alert state cleared, next staleness is a new episode.
	rows[0].LastRunAt = now.Add(2 * time.Minute)
	rows[1].LastRunAt = now.Add(2 * time.Minute)
	if err := m.CheckOnce(context.Background(), now.Add(2*time.Minute)); err != nil {
		t.Fatalf("CheckOnce: %v", err)
	}
	// Only stale_task falls behind again; fresh_task stays fresh (ran 2m ago).
	rows[0].LastRunAt = now.Add(-30 * time.Minute)
	rows[1].LastRunAt = now.Add(38 * time.Minute)
	if err := m.CheckOnce(context.Background(), now.Add(40*time.Minute)); err != nil {
		t.Fatalf("CheckOnce: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 alerts after recovery + new episode, got %d (%+v)", len(calls), calls)
	}
}

func TestStalenessMonitor_SkipsCronOnlyRows(t *testing.T) {
	now := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	rows := []Row{{TaskName: "cron_geo_ingest", LastRunAt: now.Add(-72 * time.Hour)}}
	intervals := map[string]time.Duration{} // no BTM interval for cron tasks

	var calls []alertCall
	m := NewStalenessMonitor(nil, &fakeIntervalProvider{intervals: intervals}, func(name string, staleFor, iv time.Duration) {
		calls = append(calls, alertCall{name: name})
	})
	m.store = fakeLister{rows: rows}
	if err := m.CheckOnce(context.Background(), now); err != nil {
		t.Fatalf("CheckOnce: %v", err)
	}
	if len(calls) != 0 {
		t.Fatalf("cron-only row must not trigger staleness alert, got %+v", calls)
	}
}

func TestStalenessMonitor_ListErrorIsReturned(t *testing.T) {
	m := NewStalenessMonitor(nil, &fakeIntervalProvider{}, nil)
	m.store = fakeLister{err: context.DeadlineExceeded}
	if err := m.CheckOnce(context.Background(), time.Now()); err == nil {
		t.Fatal("expected error to propagate")
	}
}
