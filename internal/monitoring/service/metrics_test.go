package service

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestMetricsService_GetMetrics_Screening(t *testing.T) {
	collector := &MetricsCollectorAdapter{
		GetScreeningRateFunc: func() float64 { return 0.42 },
		GetMetricsSnapshotFunc: func() MetricsSnapshot {
			return MetricsSnapshot{
				ScreeningTotal:  100,
				ScreeningPassed: 42,
			}
		},
	}
	svc := NewMetricsService(collector, nil)

	result := svc.GetMetrics("screening")
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["screening_rate"] != 0.42 {
		t.Errorf("screening_rate = %v, want 0.42", m["screening_rate"])
	}
	if m["screening_total"] != int64(100) {
		t.Errorf("screening_total = %v, want 100", m["screening_total"])
	}
	if m["screening_passed"] != int64(42) {
		t.Errorf("screening_passed = %v, want 42", m["screening_passed"])
	}
}

func TestMetricsService_GetMetrics_Alerts(t *testing.T) {
	collector := &MetricsCollectorAdapter{
		GetMetricsSnapshotFunc: func() MetricsSnapshot {
			return MetricsSnapshot{
				AlertsTriggered:    10,
				AlertsAcknowledged: 7,
				AlertsByType:       map[string]int64{"price": 3, "volume": 7},
			}
		},
	}
	svc := NewMetricsService(collector, nil)

	result := svc.GetMetrics("alerts")
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", result)
	}
	if m["alerts_triggered"] != int64(10) {
		t.Errorf("alerts_triggered = %v, want 10", m["alerts_triggered"])
	}
	if m["alerts_acknowledged"] != int64(7) {
		t.Errorf("alerts_acknowledged = %v, want 7", m["alerts_acknowledged"])
	}
}

func TestMetricsService_GetMetrics_All(t *testing.T) {
	snapshot := MetricsSnapshot{ScreeningTotal: 5, ScreeningPassed: 3, ScreeningRate: 0.6}
	collector := &MetricsCollectorAdapter{
		GetMetricsSnapshotFunc: func() MetricsSnapshot { return snapshot },
	}
	svc := NewMetricsService(collector, nil)

	for _, key := range []string{"all", ""} {
		got := svc.GetMetrics(key)
		m, ok := got.(MetricsSnapshot)
		if !ok {
			t.Fatalf("GetMetrics(%q) expected MetricsSnapshot, got %T", key, got)
		}
		if m.ScreeningTotal != snapshot.ScreeningTotal ||
			m.ScreeningPassed != snapshot.ScreeningPassed ||
			m.ScreeningRate != snapshot.ScreeningRate {
			t.Errorf("GetMetrics(%q) = %+v, want %+v", key, m, snapshot)
		}
	}
}

func TestMetricsService_GetMetricsTrend(t *testing.T) {
	now := time.Now()
	collector := &MetricsCollectorAdapter{
		GetMetricsSnapshotFunc: func() MetricsSnapshot {
			return MetricsSnapshot{ScreeningRate: 0.5, ScreeningTotal: 10}
		},
	}
	history := &MetricsHistoryAdapter{
		GetTrendFunc: func(metric string) []TrendPoint {
			if metric == "screening_rate" {
				return []TrendPoint{
					{Timestamp: now.Add(-1 * time.Hour), Value: 0.4},
					{Timestamp: now.Add(-25 * time.Hour), Value: 0.3},
					{Timestamp: now.Add(-2 * time.Hour), Value: 0.45},
				}
			}
			return nil
		},
	}
	svc := NewMetricsService(collector, history)

	m := svc.GetMetricsTrend("screening_rate", "24h")
	if m["metric"] != "screening_rate" {
		t.Errorf("metric = %v, want screening_rate", m["metric"])
	}
	if m["period"] != "24h" {
		t.Errorf("period = %v, want 24h", m["period"])
	}

	trend := m["trend"].([]TrendPoint)
	if len(trend) != 2 {
		t.Errorf("expected 2 trend points within 24h, got %d", len(trend))
	}
	if m["data_points"] != 2 {
		t.Errorf("data_points = %v, want 2", m["data_points"])
	}
}

func TestMetricsService_GetMetricsTrend_Defaults(t *testing.T) {
	collector := &MetricsCollectorAdapter{
		GetMetricsSnapshotFunc: func() MetricsSnapshot { return MetricsSnapshot{} },
	}
	history := &MetricsHistoryAdapter{GetTrendFunc: func(string) []TrendPoint { return nil }}
	svc := NewMetricsService(collector, history)

	m := svc.GetMetricsTrend("", "")
	if m["metric"] != "screening_rate" {
		t.Errorf("default metric = %v, want screening_rate", m["metric"])
	}
	if m["period"] != "24h" {
		t.Errorf("default period = %v, want 24h", m["period"])
	}
	if m["duration"] != "24h0m0s" {
		t.Errorf("default duration = %v, want 24h0m0s", m["duration"])
	}
}

func TestMetricsService_CheckDataQuality_NilChecker(t *testing.T) {
	svc := NewMetricsService(nil, nil)
	report := svc.CheckDataQuality(nil)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Checks) != 0 {
		t.Errorf("expected 0 checks, got %d", len(report.Checks))
	}
	if report.Overall != StatusOK {
		t.Errorf("expected overall ok, got %q", report.Overall)
	}
	if report.Score != 100.0 {
		t.Errorf("expected score 100, got %f", report.Score)
	}
}

func TestMetricsService_CheckDataQuality_WithChecker(t *testing.T) {
	checker := &mockDataQualityChecker{
		report: &DataQualityReport{
			Checks: []DataQualityCheck{
				{Name: "db", Status: StatusOK, Message: "db ok"},
				{Name: "redis", Status: StatusOK, Message: "redis ok"},
				{Name: "http", Status: StatusWarning, Message: "http slow"},
			},
			Overall: StatusWarning,
			Score:   85.0,
		},
	}
	svc := NewMetricsService(nil, nil)
	report := svc.CheckDataQuality(checker)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Checks) != 3 {
		t.Errorf("expected 3 checks, got %d", len(report.Checks))
	}
	if report.Overall != StatusWarning {
		t.Errorf("expected overall warning, got %q", report.Overall)
	}
	if report.Score != 85.0 {
		t.Errorf("expected score 85, got %f", report.Score)
	}
	if !checker.runCalled {
		t.Error("expected checker.RunAll to be called")
	}
}

func TestMetricsService_CheckDataQuality_Timeout(t *testing.T) {
	checker := &mockDataQualityChecker{
		delay: 50 * time.Millisecond,
		report: &DataQualityReport{
			Checks:  []DataQualityCheck{{Name: "fast", Status: StatusOK}},
			Overall: StatusOK,
			Score:   100.0,
		},
	}
	svc := NewMetricsService(nil, nil)
	report := svc.CheckDataQuality(checker)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if !checker.runCalled {
		t.Error("expected checker.RunAll to be called")
	}
}

func TestMetricsService_GetMetrics_JSONRoundTrip(t *testing.T) {
	collector := &MetricsCollectorAdapter{
		GetMetricsSnapshotFunc: func() MetricsSnapshot {
			return MetricsSnapshot{
				ScreeningTotal:     100,
				ScreeningPassed:    42,
				ScreeningRate:      0.42,
				AlertsTriggered:    5,
				AlertsAcknowledged: 2,
				AlertsByType:       map[string]int64{"price": 5},
				Timestamp:          time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC),
			}
		},
	}
	svc := NewMetricsService(collector, nil)

	result := svc.GetMetrics("all")
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["screening_total"] != float64(100) {
		t.Errorf("screening_total = %v", decoded["screening_total"])
	}
	if decoded["screening_rate"] != 0.42 {
		t.Errorf("screening_rate = %v", decoded["screening_rate"])
	}
}

func TestMetricsService_ConcurrentRecord(t *testing.T) {
	var mu sync.Mutex
	counter := int64(0)
	collector := &MetricsCollectorAdapter{
		GetScreeningRateFunc: func() float64 {
			mu.Lock()
			defer mu.Unlock()
			return float64(counter) / 100.0
		},
		GetMetricsSnapshotFunc: func() MetricsSnapshot {
			mu.Lock()
			defer mu.Unlock()
			return MetricsSnapshot{ScreeningTotal: counter}
		},
	}
	svc := NewMetricsService(collector, nil)

	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			for range 100 {
				mu.Lock()
				counter++
				mu.Unlock()
				svc.GetMetrics("screening")
			}
		})
	}
	wg.Wait()

	if counter != 1000 {
		t.Errorf("expected counter 1000, got %d", counter)
	}
}

func TestMetricsService_GetMetricsTrend_Periods(t *testing.T) {
	now := time.Now()
	collector := &MetricsCollectorAdapter{
		GetMetricsSnapshotFunc: func() MetricsSnapshot { return MetricsSnapshot{} },
	}
	history := &MetricsHistoryAdapter{
		GetTrendFunc: func(metric string) []TrendPoint {
			return []TrendPoint{{Timestamp: now.Add(-25 * time.Hour), Value: 0.3}}
		},
	}
	svc := NewMetricsService(collector, history)

	tests := []struct {
		period   string
		expected int // data points
	}{
		{"24h", 0},
		{"7d", 1},
		{"30d", 1},
		{"invalid", 0},
	}
	for _, tt := range tests {
		m := svc.GetMetricsTrend("screening_rate", tt.period)
		if got := m["data_points"]; got != tt.expected {
			t.Errorf("period %s: data_points = %v, want %d", tt.period, got, tt.expected)
		}
		if !strings.HasSuffix(m["duration"].(string), "0s") && !strings.HasSuffix(m["duration"].(string), "0m0s") {
			t.Errorf("period %s: unexpected duration %v", tt.period, m["duration"])
		}
	}
}

type mockDataQualityChecker struct {
	report    *DataQualityReport
	delay     time.Duration
	runCalled bool
}

func (m *mockDataQualityChecker) RunAll(ctx context.Context) *DataQualityReport {
	m.runCalled = true
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
		}
	}
	return m.report
}

func TestMetricsService_GetThresholds(t *testing.T) {
	violations := []ThresholdViolation{
		{Metric: "screening_rate", Current: 0.05, Threshold: 0.1, Severity: "warning", Message: "x"},
		{Metric: "alert_trigger_rate", Current: 150, Threshold: 100, Severity: "critical", Message: "y"},
	}
	collector := &MetricsCollectorAdapter{
		CheckThresholdsFunc: func(t AlertThreshold) []ThresholdViolation { return violations },
	}
	svc := NewMetricsService(collector, nil)
	report := svc.GetThresholds()
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Count != 2 {
		t.Errorf("count = %d, want 2", report.Count)
	}
	if len(report.Violations) != 2 {
		t.Errorf("len(violations) = %d, want 2", len(report.Violations))
	}
	if report.Threshold.MinScreeningRate != 0.1 {
		t.Errorf("threshold.min_screening_rate = %v, want 0.1", report.Threshold.MinScreeningRate)
	}
	if report.Threshold.MaxAlertTriggerRate != 100 {
		t.Errorf("threshold.max_alert_trigger_rate = %v, want 100", report.Threshold.MaxAlertTriggerRate)
	}
	if report.Threshold.MaxUnacknowledgedAlerts != 10 {
		t.Errorf("threshold.max_unacknowledged_alerts = %v, want 10", report.Threshold.MaxUnacknowledgedAlerts)
	}
	if report.CheckedAt.IsZero() {
		t.Error("checked_at should be populated")
	}
}

func TestMetricsService_GetThresholds_NilCollector(t *testing.T) {
	svc := NewMetricsService(nil, nil)
	report := svc.GetThresholds()
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Count != 0 {
		t.Errorf("count = %d, want 0", report.Count)
	}
	if report.Violations == nil {
		t.Error("violations should be non-nil empty slice, got nil")
	}
	if len(report.Violations) != 0 {
		t.Errorf("len(violations) = %d, want 0", len(report.Violations))
	}
	if report.Threshold.MinScreeningRate != 0.1 {
		t.Error("default threshold should still be populated")
	}
}

func TestMetricsService_GetThresholds_EmptyViolations(t *testing.T) {
	collector := &MetricsCollectorAdapter{
		CheckThresholdsFunc: func(t AlertThreshold) []ThresholdViolation { return nil },
	}
	svc := NewMetricsService(collector, nil)
	report := svc.GetThresholds()
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Violations == nil {
		t.Error("violations should be non-nil empty slice, got nil")
	}
	if report.Count != 0 {
		t.Errorf("count = %d, want 0", report.Count)
	}
}
