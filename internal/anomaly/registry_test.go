package anomaly

import (
	"context"
	"errors"
	"testing"
	"time"
)

type recordingPublisher struct {
	published []struct {
		eventType string
		payload   any
	}
}

func (r *recordingPublisher) Publish(_ context.Context, eventType string, payload any) error {
	r.published = append(r.published, struct {
		eventType string
		payload   any
	}{eventType: eventType, payload: payload})
	return nil
}

type recordingAlerter struct {
	calls [][]Anomaly
}

func (r *recordingAlerter) PublishAnomalies(_ context.Context, anomalies []Anomaly) error {
	r.calls = append(r.calls, anomalies)
	return nil
}

type constantDetector struct {
	name      string
	anomalies []Anomaly
}

func (c *constantDetector) Name() string { return c.name }

func (c *constantDetector) Detect(_ context.Context, _ []AuditEntryV2) ([]Anomaly, error) {
	return c.anomalies, nil
}

func TestRegistry_RegisterAndDetectAggregatesResults(t *testing.T) {
	pub := &recordingPublisher{}
	alert := &recordingAlerter{}
	r := NewRegistry(DefaultConfig(), pub, alert)
	r.Register(&constantDetector{
		name: "d1",
		anomalies: []Anomaly{
			{Type: "d1", Score: 3.0, DetectedAt: time.Now()},
		},
	})
	r.Register(&constantDetector{
		name: "d2",
		anomalies: []Anomaly{
			{Type: "d2", Score: 4.0, DetectedAt: time.Now()},
		},
	})

	got, err := r.Detect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Detect = %d anomalies, want 2", len(got))
	}
}

func TestRegistry_Detect_publishesAndAlerts(t *testing.T) {
	pub := &recordingPublisher{}
	alert := &recordingAlerter{}
	r := NewRegistry(DefaultConfig(), pub, alert)
	r.Register(&constantDetector{
		name: "d1",
		anomalies: []Anomaly{
			{Type: "d1", Score: 3.0, DetectedAt: time.Now()},
		},
	})

	_, err := r.Detect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(pub.published) != 1 {
		t.Errorf("publisher calls = %d, want 1", len(pub.published))
	}
	if len(alert.calls) != 1 {
		t.Errorf("alerter calls = %d, want 1", len(alert.calls))
	}
}

func TestRegistry_Detect_skipsPublishWhenEmpty(t *testing.T) {
	pub := &recordingPublisher{}
	alert := &recordingAlerter{}
	r := NewRegistry(DefaultConfig(), pub, alert)

	_, err := r.Detect(context.Background(), nil)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if len(pub.published) != 0 {
		t.Errorf("publisher calls = %d, want 0", len(pub.published))
	}
	if len(alert.calls) != 0 {
		t.Errorf("alerter calls = %d, want 0", len(alert.calls))
	}
}

func TestRegistry_Detect_propagatesDetectorError(t *testing.T) {
	pub := &recordingPublisher{}
	alert := &recordingAlerter{}
	r := NewRegistry(DefaultConfig(), pub, alert)
	r.Register(&errorDetector{})

	_, err := r.Detect(context.Background(), nil)
	if err == nil {
		t.Fatal("Detect did not return error")
	}
}

type errorDetector struct{}

func (errorDetector) Name() string { return "error" }

func (errorDetector) Detect(_ context.Context, _ []AuditEntryV2) ([]Anomaly, error) {
	return nil, errors.New("boom")
}

func TestRegistry_RunLoop_triggersFetchAndDetect(t *testing.T) {
	pub := &recordingPublisher{}
	alert := &recordingAlerter{}
	cfg := DefaultConfig()
	cfg.DetectIntervalSec = 1
	r := NewRegistry(cfg, pub, alert)
	r.now = func() time.Time { return time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) }
	r.Register(&constantDetector{
		name: "d1",
		anomalies: []Anomaly{
			{Type: "d1", Score: 3.0, DetectedAt: r.now()},
		},
	})

	fetchCalls := 0
	fetch := func(_ context.Context) ([]AuditEntryV2, error) {
		fetchCalls++
		return []AuditEntryV2{{TS: r.now().Format(time.RFC3339), Tool: "t", Status: "ok"}}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	r.RunLoop(ctx, fetch)

	if fetchCalls < 1 {
		t.Errorf("fetch calls = %d, want >= 1", fetchCalls)
	}
	if len(pub.published) < 1 {
		t.Errorf("publisher calls = %d, want >= 1", len(pub.published))
	}
}
