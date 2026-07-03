package monitoring

import (
	"testing"
)

// wireUpMockGateway 模擬 GatewayHealthChecker，回傳固定 channel map。
// 與 service/health_check_test.go 的 mock 不衝突（不同 package 各自定義）。
type wireUpMockGateway struct {
	summary map[string]string
}

func (m *wireUpMockGateway) Summary() map[string]string {
	return m.summary
}

// TestCheckGateway_RecordsChannelHealthErrorsForNonOK 驗證 checkGateway 對 status != "ok"
// 的 channel 會 increment atlas_channel_health_errors_total{channel="..."}；status == "ok"
// 的 channel 不會寫入（避免 cardinality 噪音）。
func TestCheckGateway_RecordsChannelHealthErrorsForNonOK(t *testing.T) {
	collector := NewMetricsCollector()
	mock := &wireUpMockGateway{
		summary: map[string]string{
			"us_yahoo": "error",
			"fugle":    "ok",
			"us_nvda":  "ok",
			"twse":     "error",
		},
	}

	hc := NewHealthChecker(NewMonitor(), nil)
	hc.SetGateway(mock)
	hc.SetCollector(collector)

	hc.checkGateway()

	// us_yahoo 應有 counter = 1
	m, ok := collector.GetMetric(MetricChannelHealthErrors, map[string]string{"channel": "us_yahoo"})
	if !ok {
		t.Fatalf("expected us_yahoo metric to exist")
	}
	if m.Value != 1 {
		t.Errorf("expected us_yahoo value 1, got %v", m.Value)
	}

	// twse 應有 counter = 1
	m, ok = collector.GetMetric(MetricChannelHealthErrors, map[string]string{"channel": "twse"})
	if !ok {
		t.Fatalf("expected twse metric to exist")
	}
	if m.Value != 1 {
		t.Errorf("expected twse value 1, got %v", m.Value)
	}

	// fugle（status=ok）不應有 metric
	_, ok = collector.GetMetric(MetricChannelHealthErrors, map[string]string{"channel": "fugle"})
	if ok {
		t.Errorf("expected NO metric for ok channel fugle, but found one")
	}

	// us_nvda（status=ok）不應有 metric
	_, ok = collector.GetMetric(MetricChannelHealthErrors, map[string]string{"channel": "us_nvda"})
	if ok {
		t.Errorf("expected NO metric for ok channel us_nvda, but found one")
	}
}

// TestCheckGateway_AccumulatesAcrossCycles 驗證多次 checkGateway 會累加（同一 channel
// 每次都是 error，counter 應持續增加），這是「sustained error」告警邏輯的基礎。
func TestCheckGateway_AccumulatesAcrossCycles(t *testing.T) {
	collector := NewMetricsCollector()
	mock := &wireUpMockGateway{
		summary: map[string]string{"us_yahoo": "error"},
	}

	hc := NewHealthChecker(NewMonitor(), nil)
	hc.SetGateway(mock)
	hc.SetCollector(collector)

	hc.checkGateway()
	hc.checkGateway()
	hc.checkGateway()

	m, ok := collector.GetMetric(MetricChannelHealthErrors, map[string]string{"channel": "us_yahoo"})
	if !ok {
		t.Fatalf("expected us_yahoo metric to exist")
	}
	if m.Value != 3 {
		t.Errorf("expected us_yahoo value 3 after 3 cycles, got %v", m.Value)
	}
}

// TestCheckGateway_NilCollectorDoesNotPanic 驗證 collector 未注入時不 panic
// （health.go 早於 main.go collector 初始化時可能發生）。
func TestCheckGateway_NilCollectorDoesNotPanic(t *testing.T) {
	mock := &wireUpMockGateway{
		summary: map[string]string{"us_yahoo": "error"},
	}
	hc := NewHealthChecker(NewMonitor(), nil)
	hc.SetGateway(mock)
	// 不呼叫 SetCollector — collector 維持 nil

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("checkGateway with nil collector panicked: %v", r)
		}
	}()
	hc.checkGateway()
}

// TestCheckGateway_NoGatewayDoesNotTouchMetrics 驗證 nil gateway 時不碰 metrics
// （確保既有 nil-safe 行為不被破壞）。
func TestCheckGateway_NoGatewayDoesNotTouchMetrics(t *testing.T) {
	collector := NewMetricsCollector()
	hc := NewHealthChecker(NewMonitor(), nil)
	hc.SetCollector(collector)
	// 不呼叫 SetGateway

	hc.checkGateway()

	all := collector.GetAllMetrics()
	for _, m := range all {
		if m.Name == MetricChannelHealthErrors {
			t.Errorf("expected no metric when gateway nil, got %+v", m)
		}
	}
}