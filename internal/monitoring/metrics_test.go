package monitoring

import (
	"testing"
)

func TestMetricsCollector_Screening(t *testing.T) {
	m := NewMetricsCollector()

	// 記錄篩選結果：10 通過，5 拒絕
	m.RecordScreening(10, 5)

	if m.screeningTotal != 15 {
		t.Errorf("expected total 15, got %d", m.screeningTotal)
	}

	rate := m.GetScreeningRate()
	if rate != 10.0/15.0 {
		t.Errorf("expected rate %f, got %f", 10.0/15.0, rate)
	}
}

func TestMetricsCollector_Alerts(t *testing.T) {
	m := NewMetricsCollector()

	// 觸發 3 個警報
	m.RecordAlert("circuit_breaker")
	m.RecordAlert("daily_loss")
	m.RecordAlert("circuit_breaker")

	if m.alertsTriggered != 3 {
		t.Errorf("expected 3 alerts, got %d", m.alertsTriggered)
	}

	// 確認 1 個
	m.RecordAlertAcknowledged()
	if m.alertsAcknowledged != 1 {
		t.Errorf("expected 1 acknowledged, got %d", m.alertsAcknowledged)
	}

	// 檢查按類型統計
	if m.alertsByType["circuit_breaker"] != 2 {
		t.Errorf("expected 2 circuit_breaker, got %d", m.alertsByType["circuit_breaker"])
	}
}

func TestMetricsCollector_Snapshot(t *testing.T) {
	m := NewMetricsCollector()
	m.RecordScreening(8, 2)
	m.RecordAlert("high_concentration")

	snapshot := m.GetMetricsSnapshot()

	if snapshot.ScreeningTotal != 10 {
		t.Errorf("expected total 10, got %d", snapshot.ScreeningTotal)
	}

	if snapshot.ScreeningRate != 0.8 {
		t.Errorf("expected rate 0.8, got %f", snapshot.ScreeningRate)
	}

	if snapshot.AlertsTriggered != 1 {
		t.Errorf("expected 1 alert, got %d", snapshot.AlertsTriggered)
	}
}

func TestCheckThresholds(t *testing.T) {
	// 測試低篩選率
	m := NewMetricsCollector()
	threshold := DefaultAlertThreshold()

	m.RecordScreening(5, 95) // 5% 篩選率
	violations := m.CheckThresholds(threshold)
	if len(violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(violations))
	}
	if violations[0].Metric != "screening_rate" {
		t.Errorf("expected screening_rate violation, got %s", violations[0].Metric)
	}

	// 測試正常情況
	m2 := NewMetricsCollector()
	m2.RecordScreening(50, 50) // 50% 篩選率
	violations = m2.CheckThresholds(threshold)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(violations))
	}
}
