package monitoring

import (
	"testing"
)

// TestRecordDBInitFailure_IncrementsCounter 驗證 RecordDBInitFailure 會建立
// atlas_db_init_failures_total counter，並帶 phase="startup" label。
func TestRecordDBInitFailure_IncrementsCounter(t *testing.T) {
	c := NewMetricsCollector()
	RecordDBInitFailure(c)

	m, ok := c.GetMetric(MetricDBInitFailures, map[string]string{"phase": "startup"})
	if !ok {
		t.Fatalf("expected metric %q to be recorded with phase=startup label", MetricDBInitFailures)
	}
	if m.Value != 1 {
		t.Errorf("expected value 1 after one call, got %v", m.Value)
	}
	if m.Type != MetricTypeCounter {
		t.Errorf("expected type counter, got %q", m.Type)
	}
}

// TestRecordDBInitFailure_Accumulates 驗證多次呼叫會累加（counter 語意）。
func TestRecordDBInitFailure_Accumulates(t *testing.T) {
	c := NewMetricsCollector()
	RecordDBInitFailure(c)
	RecordDBInitFailure(c)
	RecordDBInitFailure(c)

	m, ok := c.GetMetric(MetricDBInitFailures, map[string]string{"phase": "startup"})
	if !ok {
		t.Fatalf("expected metric to be recorded")
	}
	if m.Value != 3 {
		t.Errorf("expected value 3 after 3 calls, got %v", m.Value)
	}
}

// TestRecordDBInitFailure_NilCollector 驗證 nil collector 不會 panic（呼叫端可能未設定 collector）。
func TestRecordDBInitFailure_NilCollector(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RecordDBInitFailure(nil) panicked: %v", r)
		}
	}()
	RecordDBInitFailure(nil)
}

// TestRecordDBInitFailure_AppearsInGetAllMetrics 驗證 metric 會被 GetAllMetrics 列出
// （這是 PrometheusHandler 的讀取來源——若沒列出就等於 dead code）。
func TestRecordDBInitFailure_AppearsInGetAllMetrics(t *testing.T) {
	c := NewMetricsCollector()
	RecordDBInitFailure(c)

	all := c.GetAllMetrics()
	found := false
	for _, m := range all {
		if m.Name == MetricDBInitFailures {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %q to appear in GetAllMetrics(), got: %v", MetricDBInitFailures, all)
	}
}

// TestRecordChannelHealthError_IncrementsCounter 驗證單一 channel 會建立獨立 counter。
func TestRecordChannelHealthError_IncrementsCounter(t *testing.T) {
	c := NewMetricsCollector()
	RecordChannelHealthError(c, "us_yahoo")

	m, ok := c.GetMetric(MetricChannelHealthErrors, map[string]string{"channel": "us_yahoo"})
	if !ok {
		t.Fatalf("expected metric %q to be recorded for channel=us_yahoo", MetricChannelHealthErrors)
	}
	if m.Value != 1 {
		t.Errorf("expected value 1, got %v", m.Value)
	}
	if m.Labels["channel"] != "us_yahoo" {
		t.Errorf("expected label channel=us_yahoo, got %q", m.Labels["channel"])
	}
}

// TestRecordChannelHealthError_DifferentChannelsSeparate 驗證不同 channel 是獨立 entry（label 隔離）。
func TestRecordChannelHealthError_DifferentChannelsSeparate(t *testing.T) {
	c := NewMetricsCollector()
	RecordChannelHealthError(c, "us_yahoo")
	RecordChannelHealthError(c, "fugle")
	RecordChannelHealthError(c, "us_yahoo") // 同 channel 第二次

	usYahoo, ok := c.GetMetric(MetricChannelHealthErrors, map[string]string{"channel": "us_yahoo"})
	if !ok {
		t.Fatalf("expected us_yahoo entry to exist")
	}
	if usYahoo.Value != 2 {
		t.Errorf("expected us_yahoo value 2, got %v", usYahoo.Value)
	}

	fugle, ok := c.GetMetric(MetricChannelHealthErrors, map[string]string{"channel": "fugle"})
	if !ok {
		t.Fatalf("expected fugle entry to exist")
	}
	if fugle.Value != 1 {
		t.Errorf("expected fugle value 1, got %v", fugle.Value)
	}
}

// TestRecordChannelHealthError_NilCollector 驗證 nil collector 不會 panic。
func TestRecordChannelHealthError_NilCollector(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RecordChannelHealthError(nil, ...) panicked: %v", r)
		}
	}()
	RecordChannelHealthError(nil, "us_yahoo")
}

// TestRecordChannelHealthError_EmptyChannel 驗證空 channel name 不會建立 entry。
func TestRecordChannelHealthError_EmptyChannel(t *testing.T) {
	c := NewMetricsCollector()
	RecordChannelHealthError(c, "")

	all := c.GetAllMetrics()
	for _, m := range all {
		if m.Name == MetricChannelHealthErrors {
			t.Errorf("expected no metric for empty channel, got: %+v", m)
		}
	}
}

// TestMetricsNames_FollowPrometheusConvention 驗證 metric 名稱符合 Prometheus 慣例
// （snake_case、以 _total 結尾 for counter）。這是 PR #925 死掉的根因之一。
func TestMetricsNames_FollowPrometheusConvention(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{MetricDBInitFailures, "atlas_db_init_failures_total"},
		{MetricChannelHealthErrors, "atlas_channel_health_errors_total"},
		{MetricStage3TaskRuns, "atlas_stage3_task_runs_total"},
		{MetricStage3AlertsFired, "atlas_stage3_alerts_fired_total"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name != tc.want {
				t.Errorf("metric name drift: got %q, want %q", tc.name, tc.want)
			}
		})
	}
}

func TestRecordStage3TaskRun_IncrementsPerTaskResultLabel(t *testing.T) {
	c := NewMetricsCollector()
	RecordStage3TaskRun(c, "sync-events-daily", "success")
	RecordStage3TaskRun(c, "sync-events-daily", "success")
	RecordStage3TaskRun(c, "sync-events-daily", "failed")

	m, ok := c.GetMetric(MetricStage3TaskRuns, map[string]string{
		"task":   "sync-events-daily",
		"result": "success",
	})
	if !ok || m.Value != 2 {
		t.Fatalf("expected success counter=2, got %+v ok=%v", m, ok)
	}
	m, ok = c.GetMetric(MetricStage3TaskRuns, map[string]string{
		"task":   "sync-events-daily",
		"result": "failed",
	})
	if !ok || m.Value != 1 {
		t.Fatalf("expected failed counter=1, got %+v ok=%v", m, ok)
	}
}

func TestRecordStage3TaskRun_NilCollectorAndEmptyArgsAreSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RecordStage3TaskRun(nil/empty) panicked: %v", r)
		}
	}()
	RecordStage3TaskRun(nil, "sync-events-daily", "success")
	RecordStage3TaskRun(NewMetricsCollector(), "", "success")
	RecordStage3TaskRun(NewMetricsCollector(), "sync-events-daily", "")
}

func TestRecordStage3AlertFired_LowercasesSeverity(t *testing.T) {
	c := NewMetricsCollector()
	RecordStage3AlertFired(c, "data-staleness-critical", AlertLevelCritical)
	RecordStage3AlertFired(c, "data-staleness-warning", AlertLevelWarning)
	RecordStage3AlertFired(c, "prediction-drift", AlertLevelInfo)

	for _, tc := range []struct {
		rule, sev string
	}{
		{"data-staleness-critical", "critical"},
		{"data-staleness-warning", "warning"},
		{"prediction-drift", "info"},
	} {
		m, ok := c.GetMetric(MetricStage3AlertsFired, map[string]string{
			"rule":     tc.rule,
			"severity": tc.sev,
		})
		if !ok || m.Value != 1 {
			t.Fatalf("expected %s/%s counter=1, got %+v ok=%v", tc.rule, tc.sev, m, ok)
		}
	}
}

func TestRecordStage3AlertFired_NilCollectorAndEmptyRuleIDAreSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RecordStage3AlertFired(nil/empty) panicked: %v", r)
		}
	}()
	RecordStage3AlertFired(nil, "rule", AlertLevelInfo)
	RecordStage3AlertFired(NewMetricsCollector(), "", AlertLevelInfo)
}
