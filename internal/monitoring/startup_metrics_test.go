package monitoring

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
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

func TestRecordStage3LedgerRecords_EmitsGaugeValueMatchingStoreLen(t *testing.T) {
	c := NewMetricsCollector()
	store := ledger.NewJSONLEventFlowPredictionStore(t.TempDir())
	for i := range 7 {
		if err := store.AppendPrediction(ledger.EventFlowPredictionRecord{
			PredictedAt:   time.Now().Add(time.Duration(i) * time.Hour),
			DirectionSign: float64(i + 1),
			Confidence:    0.7,
			Direction:     "inflow",
		}); err != nil {
			t.Fatalf("AppendPrediction %d: %v", i, err)
		}
	}

	RecordStage3LedgerRecords(c, store)

	m, ok := c.GetMetric(MetricStage3LedgerRecords, map[string]string{"ledger": "event_flow_prediction"})
	if !ok {
		t.Fatalf("expected gauge %q to be registered", MetricStage3LedgerRecords)
	}
	if m.Value != 7 {
		t.Fatalf("expected gauge=7 (matches store.Len()), got %v", m.Value)
	}
	if m.Type != MetricTypeGauge {
		t.Fatalf("expected MetricTypeGauge, got %v", m.Type)
	}
	if m.Name != MetricStage3LedgerRecords {
		t.Fatalf("expected Name=%q, got %q", MetricStage3LedgerRecords, m.Name)
	}
}

func TestRecordStage3LedgerRecords_GaugeOverwriteSemantics(t *testing.T) {
	// Gauge overwrites, not accumulates. Two emits on the same store must
	// produce the latest Len() value, not the sum.
	c := NewMetricsCollector()
	store := ledger.NewJSONLEventFlowPredictionStore(t.TempDir())

	RecordStage3LedgerRecords(c, store)
	if v, _ := c.GetMetric(MetricStage3LedgerRecords, map[string]string{"ledger": "event_flow_prediction"}); v.Value != 0 {
		t.Fatalf("empty store: expected 0, got %v", v.Value)
	}

	for i := range 3 {
		if err := store.AppendPrediction(ledger.EventFlowPredictionRecord{
			PredictedAt:   time.Now().Add(time.Duration(i) * time.Hour),
			DirectionSign: float64(i + 1),
			Confidence:    0.7,
			Direction:     "inflow",
		}); err != nil {
			t.Fatalf("AppendPrediction %d: %v", i, err)
		}
	}
	RecordStage3LedgerRecords(c, store)
	m, _ := c.GetMetric(MetricStage3LedgerRecords, map[string]string{"ledger": "event_flow_prediction"})
	if m.Value != 3 {
		t.Fatalf("after 3 appends: expected gauge=3 (overwrite, not 0+3 or 0+3=accumulate), got %v", m.Value)
	}
}

func TestRecordStage3LedgerRecords_NilCollectorAndNilStoreAreSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RecordStage3LedgerRecords(nil/nil) panicked: %v", r)
		}
	}()
	RecordStage3LedgerRecords(nil, nil)
	RecordStage3LedgerRecords(NewMetricsCollector(), nil)
	RecordStage3LedgerRecords(nil, ledger.NewJSONLEventFlowPredictionStore(t.TempDir()))
}

// TestRecordDataAggregatorFailure_IncrementsCounter 驗證 RecordDataAggregatorFailure
// 會建立 atlas_data_aggregator_failures_total counter,帶 industry × kind label。
// 用 kind="no_data" 模擬「FinMind 月營收回空 array」這個 auto_cycle_update 最常見的失敗模式。
func TestRecordDataAggregatorFailure_IncrementsCounter(t *testing.T) {
	c := NewMetricsCollector()
	RecordDataAggregatorFailure(c, "electronics", "no_data")

	m, ok := c.GetMetric(MetricDataAggregatorFailures, map[string]string{
		"industry": "electronics",
		"kind":     "no_data",
	})
	if !ok {
		t.Fatalf("expected metric %q to be recorded with industry=electronics, kind=no_data", MetricDataAggregatorFailures)
	}
	if m.Value != 1 {
		t.Errorf("expected value 1, got %v", m.Value)
	}
	if m.Type != MetricTypeCounter {
		t.Errorf("expected counter type, got %q", m.Type)
	}
}

// TestRecordDataAggregatorFailure_DifferentKindsAreSeparateLabels 驗證不同 kind 不會互相覆蓋,
// 而是各自獨立 counter entry（否則就丟失根因分佈的可觀察性）。
func TestRecordDataAggregatorFailure_DifferentKindsAreSeparateLabels(t *testing.T) {
	c := NewMetricsCollector()
	RecordDataAggregatorFailure(c, "leo_satellite", "no_data")
	RecordDataAggregatorFailure(c, "leo_satellite", "no_data")
	RecordDataAggregatorFailure(c, "leo_satellite", "quota")

	for _, tc := range []struct {
		kind string
		want float64
	}{
		{"no_data", 2},
		{"quota", 1},
	} {
		m, ok := c.GetMetric(MetricDataAggregatorFailures, map[string]string{
			"industry": "leo_satellite",
			"kind":     tc.kind,
		})
		if !ok {
			t.Fatalf("expected metric for kind=%q", tc.kind)
		}
		if m.Value != tc.want {
			t.Errorf("kind=%q: expected %v, got %v", tc.kind, tc.want, m.Value)
		}
	}
}

// TestRecordDataAggregatorFailure_NilCollectorAndEmptyInputs 驗證 nil collector 跟空字串輸入都安全。
// nil collector 是 bootstrap 早期或 test 的合理輸入；空字串則會建立 label="" 的孤立 Prometheus entry,必須拒絕。
func TestRecordDataAggregatorFailure_NilCollectorAndEmptyInputs(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RecordDataAggregatorFailure panicked: %v", r)
		}
	}()
	RecordDataAggregatorFailure(nil, "electronics", "no_data")            // nil collector
	RecordDataAggregatorFailure(NewMetricsCollector(), "", "no_data")     // empty industry
	RecordDataAggregatorFailure(NewMetricsCollector(), "electronics", "") // empty kind
}
