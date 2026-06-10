package monitoring

import (
	"path/filepath"
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

func TestMetricKey_LabelCollision(t *testing.T) {
	k1 := metricKey("orders_total", map[string]string{"symbol": "2330", "side": "buy"})
	k2 := metricKey("orders_total", map[string]string{"symbol": "2317", "side": "sell"})
	k3 := metricKey("orders_total", map[string]string{"side": "buy", "symbol": "2330"}) // same as k1, different order
	k4 := metricKey("orders_total", nil)

	if k1 == k2 {
		t.Errorf("different labels must produce different keys: k1=%s, k2=%s", k1, k2)
	}
	if k1 != k3 {
		t.Errorf("identical labels (different order) must produce identical keys: k1=%s, k3=%s", k1, k3)
	}
	if k4 != "orders_total" {
		t.Errorf("nil labels must produce name only: got %q, want %q", k4, "orders_total")
	}
}

func TestMetricKey_NoCollisionAfterRecord(t *testing.T) {
	m := NewMetricsCollector()
	m.RecordCounter("orders_total", 1, map[string]string{"symbol": "2330", "side": "buy"})
	m.RecordCounter("orders_total", 1, map[string]string{"symbol": "2317", "side": "sell"})
	m.RecordCounter("orders_total", 1, map[string]string{"symbol": "2330", "side": "buy"}) // duplicate of first

	all := m.GetAllMetrics()
	var sym2330, sym2317 float64
	found2330, found2317 := false, false
	for _, metric := range all {
		if metric.Name != "orders_total" {
			continue
		}
		switch metric.Labels["symbol"] {
		case "2330":
			sym2330 = metric.Value
			found2330 = true
		case "2317":
			sym2317 = metric.Value
			found2317 = true
		}
	}
	if !found2330 || !found2317 {
		t.Fatalf("expected both labels to be tracked, found 2330=%v (val=%v), 2317=%v (val=%v)", found2330, sym2330, found2317, sym2317)
	}
	if sym2330 != 2 {
		t.Errorf("expected orders_total{symbol=2330}=2 (1+1 duplicates), got %v", sym2330)
	}
	if sym2317 != 1 {
		t.Errorf("expected orders_total{symbol=2317}=1, got %v", sym2317)
	}
}

func TestGetAlertTriggerCount(t *testing.T) {
	m := NewMetricsCollector()

	// Count is 0 initially
	if got := m.GetAlertTriggerCount(); got != 0 {
		t.Errorf("initial count = %v, want 0", got)
	}

	m.RecordAlert("circuit_breaker")
	m.RecordAlert("regime_change")
	m.RecordAlert("circuit_breaker")

	if got := m.GetAlertTriggerCount(); got != 3 {
		t.Errorf("after 3 alerts count = %v, want 3", got)
	}
}

func TestCheckThresholds_AlertTriggerCount_BelowThreshold(t *testing.T) {
	m := NewMetricsCollector()
	for i := 0; i < 50; i++ {
		m.RecordAlert("test")
	}
	threshold := AlertThreshold{
		MinScreeningRate:        0.0,
		MaxAlertTriggerRate:     100,
		MaxUnacknowledgedAlerts: 1000,
	}
	violations := m.CheckThresholds(threshold)
	for _, v := range violations {
		if v.Metric == "alert_trigger_rate" {
			t.Errorf("did not expect alert_trigger_rate violation when count=50 below threshold=100, got: %+v", v)
		}
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

func TestMetricsCollector_PersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "metrics.jsonl")

	// First collector: write some events
	m1, err := NewMetricsCollectorWithPath(path)
	if err != nil {
		t.Fatalf("create m1: %v", err)
	}
	m1.RecordScreening(7, 3) // 7 passed, 3 rejected
	m1.RecordAlert("circuit_breaker")
	m1.RecordAlert("regime_change")
	m1.RecordAlertAcknowledged()
	m1.RecordCounter("orders_total", 5, map[string]string{"symbol": "2330", "side": "buy"})
	m1.RecordGauge("portfolio_total", 1000000, nil)

	// Second collector: load from same path
	m2, err := NewMetricsCollectorWithPath(path)
	if err != nil {
		t.Fatalf("create m2: %v", err)
	}
	if got := m2.GetScreeningRate(); got < 0.69 || got > 0.71 {
		t.Errorf("replayed screening rate = %v, want ~0.7", got)
	}
	if got := m2.GetAlertTriggerCount(); got != 2 {
		t.Errorf("replayed alert count = %v, want 2", got)
	}
	metric, ok := m2.GetMetric("orders_total", map[string]string{"symbol": "2330", "side": "buy"})
	if !ok {
		t.Error("orders_total{symbol=2330,side=buy} not found after replay")
	} else if metric.Value != 5 {
		t.Errorf("replayed orders_total value = %v, want 5", metric.Value)
	}
	gauge, ok := m2.GetMetric("portfolio_total", nil)
	if !ok {
		t.Error("portfolio_total gauge not found after replay")
	} else if gauge.Value != 1000000 {
		t.Errorf("replayed portfolio_total = %v, want 1000000", gauge.Value)
	}
}

func TestMetricsCollector_NoPersistenceWhenPathEmpty(t *testing.T) {
	m, err := NewMetricsCollectorWithPath("")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	m.RecordScreening(5, 5)
	if got := m.GetScreeningRate(); got != 0.5 {
		t.Errorf("screening rate = %v, want 0.5", got)
	}
	// No file should be written when path is empty
}
