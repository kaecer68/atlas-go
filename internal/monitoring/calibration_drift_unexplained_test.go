package monitoring

import (
	"strings"
	"testing"
	"time"
)

// 逐 key 的「無法解釋的偏離」counter（atlas_calibration_drift_unexplained_total）。
//
// 這組測試把三件事釘成可執行規格:
//  1. 條件與 `drift_unexplained` 那則 WARN 逐條對應（out_of_window / not_applicable）;
//  2. 它是 **counter**（每一輪 +1,與「每一輪一則 WARN」一一對應）而不是 gauge;
//  3. `ssot_moved`（設計行為）與「無法比較」（RunOK=false）都**不得**計數。
func driftObs(now time.Time, outOfWindow, unknown, invalidated []string) CalibrationDriftObservation {
	return CalibrationDriftObservation{
		Artifact:        CalibrationArtifactParametersOverlay,
		BaselinePath:    "configs/parameters.json",
		OverlayPath:     "data/state/parameters.calibrated.json",
		CheckedAt:       now,
		RunOK:           true,
		DriftedKeys:     append(append([]string{}, outOfWindow...), invalidated...),
		OutOfWindowKeys: outOfWindow,
		UnknownKeys:     unknown,
		InvalidatedKeys: invalidated,
	}
}

func counterValue(t *testing.T, c *MetricsCollector, key, reason string) (float64, bool) {
	t.Helper()
	m, ok := c.GetMetric(MetricCalibrationDriftUnexplainedTotal, map[string]string{
		"artifact": CalibrationArtifactParametersOverlay,
		"key":      key,
		"reason":   reason,
	})
	if !ok {
		return 0, false
	}
	return m.Value, true
}

func TestEmitCalibrationDriftGauges_UnexplainedCounterCarriesKeyAndReason(t *testing.T) {
	now := time.Date(2026, 10, 17, 6, 27, 0, 0, time.UTC)
	collector := NewMetricsCollector()

	emitCalibrationDriftGauges(collector, driftObs(now,
		[]string{"darwinian.risk_volatility_threshold.value"},
		[]string{"risk.max_position_size"},
		[]string{"industry.seasonal_patterns"}))
	// 第二輪:同一個 key 仍然無法解釋 ⇒ counter 必須再加一（counter 語意,
	// 不是 gauge 的覆蓋語意）。
	emitCalibrationDriftGauges(collector, driftObs(now.Add(5*time.Minute),
		[]string{"darwinian.risk_volatility_threshold.value"},
		[]string{"risk.max_position_size"},
		[]string{"industry.seasonal_patterns"}))

	cases := []struct {
		key, reason string
		want        float64
	}{
		{"darwinian.risk_volatility_threshold.value", DriftUnexplainedReasonOutOfWindow, 2},
		{"risk.max_position_size", DriftUnexplainedReasonNotApplicable, 2},
	}
	for _, tc := range cases {
		got, ok := counterValue(t, collector, tc.key, tc.reason)
		if !ok {
			t.Fatalf("缺少 %s{key=%q,reason=%q}", MetricCalibrationDriftUnexplainedTotal, tc.key, tc.reason)
		}
		if got != tc.want {
			t.Errorf("%s{key=%q,reason=%q} = %v, want %v（兩輪 ⇒ counter 累加）",
				MetricCalibrationDriftUnexplainedTotal, tc.key, tc.reason, got, tc.want)
		}
		m, _ := collector.GetMetric(MetricCalibrationDriftUnexplainedTotal, map[string]string{
			"artifact": CalibrationArtifactParametersOverlay,
			"key":      tc.key,
			"reason":   tc.reason,
		})
		if m.Type != MetricTypeCounter {
			t.Errorf("%s 的型別是 %v, want counter（Prometheus 端要用 increase() 讀）",
				MetricCalibrationDriftUnexplainedTotal, m.Type)
		}
	}

	// ssot_moved 是設計行為（受審查的憲章編輯優先）⇒ 不得出現在這個 counter 裡。
	if _, ok := counterValue(t, collector, "industry.seasonal_patterns", DriftUnexplainedReasonNotApplicable); ok {
		t.Error("ssot_moved 的 key 不該計入 unexplained counter（它不是「說不通」）")
	}
	if _, ok := counterValue(t, collector, "industry.seasonal_patterns", DriftUnexplainedReasonOutOfWindow); ok {
		t.Error("ssot_moved 的 key 不該計入 unexplained counter（它不是「說不通」）")
	}
}

// 乾淨的一輪（沒有 out_of_window、沒有 not_applicable）⇒ 這個 counter 必須
// **不存在**。若它被寫成「每輪無條件輸出 0」,`increase()` 之外的誤讀（例如有人
// 直接查 `> 0`）就會在健康的系統上得到假訊號;缺席才是正確的形狀。
func TestEmitCalibrationDriftGauges_NoUnexplainedCounterWhenClean(t *testing.T) {
	now := time.Date(2026, 10, 17, 6, 27, 0, 0, time.UTC)
	collector := NewMetricsCollector()

	emitCalibrationDriftGauges(collector, driftObs(now, nil, nil, []string{"industry.seasonal_patterns"}))

	if _, ok := collector.GetMetric(MetricCalibrationDriftUnexplainedTotal, map[string]string{
		"artifact": CalibrationArtifactParametersOverlay,
	}); ok {
		t.Fatalf("乾淨的一輪不得輸出 %s（缺席 = 沒有無法解釋的偏離）", MetricCalibrationDriftUnexplainedTotal)
	}
	// gauge 仍然要有（可見性與 counter 是兩件事）。
	if m, ok := collector.GetMetric(MetricCalibrationDriftOutOfWindowKeys, map[string]string{
		"artifact": CalibrationArtifactParametersOverlay,
	}); !ok || m.Value != 0 {
		t.Errorf("%s 必須是 0 且存在（got ok=%v value=%v）", MetricCalibrationDriftOutOfWindowKeys, ok, m.Value)
	}
}

// RunOK=false（基準讀不到/overlay 解不開）⇒ 整個數值族都不輸出,包含這個 counter:
// 「無法比較」時「無法解釋」是**未知**,不是事實。凍結的舊值會被讀成現況。
func TestEmitCalibrationDriftGauges_UnmeasurableEmitsNoUnexplainedCounter(t *testing.T) {
	collector := NewMetricsCollector()
	obs := CalibrationDriftObservation{
		Artifact:         CalibrationArtifactParametersOverlay,
		RunOK:            false,
		UnverifiableCode: CalibrationDriftBaselineUnreadable,
	}
	emitCalibrationDriftGauges(collector, obs)

	if _, ok := collector.GetMetric(MetricCalibrationDriftUnexplainedTotal, map[string]string{
		"artifact": CalibrationArtifactParametersOverlay,
	}); ok {
		t.Fatalf("run_ok=0 時不得輸出 %s", MetricCalibrationDriftUnexplainedTotal)
	}
	if _, ok := collector.GetMetric(MetricCalibrationDriftKeys, map[string]string{
		"artifact": CalibrationArtifactParametersOverlay,
	}); ok {
		t.Fatal("run_ok=0 時不得輸出 drift_keys（0 會被讀成「沒有偏離」）")
	}
}

// 名稱與標籤表面:序列名必須以既有 `atlas_calibration_drift_` 前綴開頭,
// 且 `key`/`reason` 是穩定的標籤名（告警/儀表板/文件都靠它們）。
func TestCalibrationDriftUnexplainedTotalSurface(t *testing.T) {
	if !strings.HasPrefix(MetricCalibrationDriftUnexplainedTotal, "atlas_calibration_drift_") {
		t.Errorf("%s 必須沿用 atlas_calibration_drift_ 前綴（既有命名風格）", MetricCalibrationDriftUnexplainedTotal)
	}
	if !strings.HasSuffix(MetricCalibrationDriftUnexplainedTotal, "_total") {
		t.Errorf("%s 是 counter,依 Prometheus 慣例必須以 _total 結尾", MetricCalibrationDriftUnexplainedTotal)
	}
	if DriftUnexplainedReasonNotApplicable != DriftDropReasonNotApplicable {
		t.Errorf("not_applicable 必須與 gauge 族的字串相同（got %q）", DriftUnexplainedReasonNotApplicable)
	}
}
