package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// writeCalibrationParameters 在 workDir 的慣例路徑寫一份最小但結構有效的
// parameters.json（只有 ValidateCalibration 真的會讀的欄位）。
func writeCalibrationParameters(t *testing.T, workDir string, updatedAt time.Time) string {
	t.Helper()
	dir := filepath.Join(workDir, "configs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	updatedAtField := "null"
	if !updatedAt.IsZero() {
		updatedAtField = fmt.Sprintf("%q", updatedAt.Format(time.RFC3339Nano))
	}
	body := fmt.Sprintf(`{
	  "version": "test",
	  "updated_at": %s,
	  "industry": {"classification_tree": {"value": {"segments": [
	    {"id": "semiconductor", "name": "半導體產業", "level": 1, "representative_stocks": ["2330.TW"]}
	  ]}}}
	}`, updatedAtField)
	p := filepath.Join(dir, "parameters.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func scrapeMetrics(t *testing.T, c *monitoring.MetricsCollector) string {
	t.Helper()
	rec := httptest.NewRecorder()
	monitoring.PrometheusHandler(c).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	return rec.Body.String()
}

// 註冊面：任務必須真的被 registerBackfillTasks 掛上（否則整個監控是 inert ——
// 正是 issue #1944 的主題）。
func TestRegisterBackfillTasks_CalibrationFreshnessMetricsRegistered(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerBackfillTasks(backfillDeps{
		taskMgr: mgr,
		cfg:     config.Config{WorkDir: t.TempDir()},
	})
	task, ok := mgr.Get("calibration_freshness_metrics_export")
	if !ok {
		t.Fatal("calibration_freshness_metrics_export task was not registered")
	}
	if task.Interval != calibrationFreshnessMetricsInterval {
		t.Errorf("interval=%v，want %v", task.Interval, calibrationFreshnessMetricsInterval)
	}
	if !task.Enabled {
		t.Error("task 預設必須 enabled，否則監控永遠不會被執行")
	}
}

// 掛載點解析：優先用應用自己的權威路徑，沒有時回退 workDir 慣例路徑。
func TestCalibrationParametersPath_PrefersAuthoritativePath(t *testing.T) {
	prev := config.GetParametersConfigPath()
	t.Cleanup(func() { config.SetParametersConfigPath(prev) })

	workDir := t.TempDir()
	config.SetParametersConfigPath("")
	if got, want := calibrationParametersPath(workDir), filepath.Join(workDir, "configs", "parameters.json"); got != want {
		t.Errorf("無權威路徑時 got %q，want %q", got, want)
	}

	authoritative := filepath.Join(t.TempDir(), "authoritative-parameters.json")
	config.SetParametersConfigPath(authoritative)
	if got := calibrationParametersPath(workDir); got != authoritative {
		t.Errorf("有權威路徑時 got %q，want %q（檢查必須看應用實際寫入的那個檔案）", got, authoritative)
	}
}

// 正向案例：新鮮 ⇒ 序列存在且為 1（而且不需要任何人工動作）。
func TestExportCalibrationFreshnessMetrics_FreshArtifact(t *testing.T) {
	workDir := t.TempDir()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	path := writeCalibrationParameters(t, workDir, now.Add(-65*time.Minute))

	prev := config.GetParametersConfigPath()
	t.Cleanup(func() { config.SetParametersConfigPath(prev) })
	config.SetParametersConfigPath(path)

	collector := monitoring.NewMetricsCollector()
	obs := exportCalibrationFreshnessMetrics(workDir, collector, now)
	if !obs.RunOK || !obs.Fresh {
		t.Fatalf("run_ok=%v fresh=%v，預期皆為 true", obs.RunOK, obs.Fresh)
	}

	body := scrapeMetrics(t, collector)
	for _, want := range []string{
		`atlas_calibration_freshness_ok{artifact="parameters"} 1.000000`,
		`atlas_calibration_freshness_run_ok{artifact="parameters"} 1.000000`,
		`atlas_calibration_freshness_age_seconds{artifact="parameters"} 3900.000000`,
		fmt.Sprintf(`atlas_calibration_last_calibrated_timestamp_seconds{artifact="parameters"} %d.000000`, now.Add(-65*time.Minute).Unix()),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics 缺少 %q\n%s", want, body)
		}
	}
}

// 負向案例 1：不新鮮 ⇒ 判定值翻 0，但「檢查有跑」必須仍是 1。
func TestExportCalibrationFreshnessMetrics_StaleArtifact(t *testing.T) {
	workDir := t.TempDir()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	path := writeCalibrationParameters(t, workDir, now.Add(-72*time.Hour))

	prev := config.GetParametersConfigPath()
	t.Cleanup(func() { config.SetParametersConfigPath(prev) })
	config.SetParametersConfigPath(path)

	collector := monitoring.NewMetricsCollector()
	obs := exportCalibrationFreshnessMetrics(workDir, collector, now)
	if !obs.RunOK || obs.Fresh {
		t.Fatalf("run_ok=%v fresh=%v，預期 run_ok=true / fresh=false", obs.RunOK, obs.Fresh)
	}
	body := scrapeMetrics(t, collector)
	if want := `atlas_calibration_freshness_ok{artifact="parameters"} 0.000000`; !strings.Contains(body, want) {
		t.Errorf("/metrics 缺少 %q\n%s", want, body)
	}
	if want := `atlas_calibration_freshness_run_ok{artifact="parameters"} 1.000000`; !strings.Contains(body, want) {
		t.Errorf("/metrics 缺少 %q\n%s", want, body)
	}
}

// 負向案例 2（fail-closed）：產物不存在／讀不到 ⇒ 必須留下明確訊號，不得靜默通過。
func TestExportCalibrationFreshnessMetrics_UnreadableArtifactIsNotSilent(t *testing.T) {
	workDir := t.TempDir() // 刻意不建立 configs/parameters.json
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)

	prev := config.GetParametersConfigPath()
	t.Cleanup(func() { config.SetParametersConfigPath(prev) })
	config.SetParametersConfigPath("")

	collector := monitoring.NewMetricsCollector()
	obs := exportCalibrationFreshnessMetrics(workDir, collector, now)
	if obs.RunOK {
		t.Fatal("產物不存在時 run_ok 必須是 0")
	}
	body := scrapeMetrics(t, collector)
	for _, want := range []string{
		`atlas_calibration_freshness_run_ok{artifact="parameters"} 0.000000`,
		`atlas_calibration_freshness_ok{artifact="parameters"} 0.000000`,
		`atlas_calibration_freshness_checked_timestamp_seconds{artifact="parameters"}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics 缺少 %q（fail-closed 訊號不得缺席）\n%s", want, body)
		}
	}
}

// 端到端：登記到 manager 的那個 closure 真的會輸出序列（不是只有註冊名稱存在）。
func TestCalibrationFreshnessTaskClosure_EmitsMetrics(t *testing.T) {
	workDir := t.TempDir()
	now := time.Now().UTC()
	path := writeCalibrationParameters(t, workDir, now.Add(-time.Hour))

	prev := config.GetParametersConfigPath()
	t.Cleanup(func() { config.SetParametersConfigPath(prev) })
	config.SetParametersConfigPath(path)

	mgr := apigateway.NewBackgroundTaskManager(nil)
	collector := monitoring.NewMetricsCollector()
	registerBackfillTasks(backfillDeps{
		taskMgr:   mgr,
		cfg:       config.Config{WorkDir: workDir},
		collector: collector,
	})
	task, ok := mgr.Get("calibration_freshness_metrics_export")
	if !ok {
		t.Fatal("任務未註冊")
	}
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("任務必須永遠回 nil（狀態由序列與規則表達，不是 error）: %v", err)
	}
	body := scrapeMetrics(t, collector)
	if want := `atlas_calibration_freshness_ok{artifact="parameters"} 1.000000`; !strings.Contains(body, want) {
		t.Errorf("任務 closure 沒有輸出預期的序列 %q\n%s", want, body)
	}
}
