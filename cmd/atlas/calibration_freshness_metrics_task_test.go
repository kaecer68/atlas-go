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
// parameters.json（只有 ValidateCalibration 真的會讀的欄位），並把**檔案 mtime
// 也釘在 updatedAt**。
//
// ⚠️ 為什麼要一起釘 mtime（2026-09-29 修）：驗證器對 mtime **與** updated_at 各發
// 一個 freshness finding。只寫 updated_at 而讓 mtime 停在真實寫入時間，就等於在
// fixture 裡留下一條 wall-clock 依賴——測試跑到真實時間離 `updatedAt` 超過契約
// （48h）的環境時，mtime 那一半會自己翻紅。本檔其餘測試都用固定時戳，因此 mtime
// 必須是可指定的；mtime 與 updated_at **不一致**的形狀（`cp -p` 還原舊檔）由
// `internal/monitoring` 的測試覆蓋。
func writeCalibrationParameters(t *testing.T, workDir string, updatedAt time.Time) string {
	t.Helper()
	path := writeCalibrationParametersRaw(t, workDir, updatedAt)
	if !updatedAt.IsZero() {
		if err := os.Chtimes(path, updatedAt, updatedAt); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	return path
}

// writeCalibrationParametersRaw 只寫內容，不動 mtime（mtime 由呼叫端決定）。
func writeCalibrationParametersRaw(t *testing.T, workDir string, updatedAt time.Time) string {
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

// ⚠️ 這一條就是「日期炸彈」的回歸守衛，也是本 PR 的驗收核心。
//
// 為什麼需要它：`exportCalibrationFreshnessMetrics` 收一個 `now`，但那個 `now` 一度
// 只影響它自己報出的 `age` —— 真正的 freshness 判定在 `config.ValidateCalibration`
// 內用 `time.Since(...)`（**真實時鐘**）。於是本檔前面那幾條用固定時戳
// （2026-09-26）的案例會隨真實時間自行翻紅：當 wall clock 走過
// `fixture_updated_at + 48h`（= 2026-09-28T10:55Z）的那一天，`_FreshArtifact` 就
// 開始紅，而且紅燈訊息完全指不出原因（它是什麼程式碼都沒改的情況下紅的）。
//
// 斷言方式：把注入的時鐘放到**真實時鐘的 ±100 天**，fixture（updated_at 與 mtime）
// 跟著注入時鐘釘死。兩種錯法各紅一格、方向相反：
//
//	· 實作若用真實時鐘 ⇒ 「−100 天」那格會把 65 分鐘前的產物誤判成過期；
//	· 反之「+100 天」那格會把 72 小時前的產物誤判成新鮮。
//
// 同時斷言 `checked_timestamp` 與 `age_seconds` 都等於注入時鐘導出的值 ——
// 讓「注入的 now 真的走到底」是可觀測的，而不是只有判定恰好對上。
func TestExportCalibrationFreshnessMetrics_VerdictIsIndependentOfWallClock(t *testing.T) {
	realNow := time.Now().UTC() // 只用來把注入時鐘推離真實時鐘，不用來判定

	cases := []struct {
		name      string
		offset    time.Duration
		age       time.Duration
		wantFresh bool
	}{
		{"注入時鐘在真實時鐘前 100 天，產物 65 分鐘前寫的 ⇒ 新鮮", -100 * 24 * time.Hour, 65 * time.Minute, true},
		{"注入時鐘在真實時鐘後 100 天，產物 72 小時前寫的 ⇒ 不新鮮", +100 * 24 * time.Hour, 72 * time.Hour, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := realNow.Add(tc.offset).Truncate(time.Second)
			workDir := t.TempDir()
			path := writeCalibrationParameters(t, workDir, now.Add(-tc.age))

			prev := config.GetParametersConfigPath()
			t.Cleanup(func() { config.SetParametersConfigPath(prev) })
			config.SetParametersConfigPath(path)

			collector := monitoring.NewMetricsCollector()
			obs := exportCalibrationFreshnessMetrics(workDir, collector, now)

			if !obs.RunOK {
				t.Fatalf("run_ok 必須是 1（檔案可讀、JSON 合法），got code=%q", obs.UnverifiableCode)
			}
			if obs.Fresh != tc.wantFresh {
				t.Fatalf("fresh=%v，want %v（freshness_code=%q）—— 判定跟隨了真實時鐘而不是注入的 now=%s",
					obs.Fresh, tc.wantFresh, obs.FreshnessCode, now.Format(time.RFC3339))
			}
			if got, want := obs.CheckedAt, now; !got.Equal(want) {
				t.Errorf("CheckedAt=%v，want %v（注入的 now 必須原樣成為檢查時刻）", got, want)
			}

			body := scrapeMetrics(t, collector)
			for _, want := range []string{
				fmt.Sprintf(`atlas_calibration_freshness_checked_timestamp_seconds{artifact="parameters"} %d.000000`, now.Unix()),
				fmt.Sprintf(`atlas_calibration_freshness_age_seconds{artifact="parameters"} %.6f`, tc.age.Seconds()),
			} {
				if !strings.Contains(body, want) {
					t.Errorf("/metrics 缺少 %q\n--- body ---\n%s", want, body)
				}
			}
			okLine := fmt.Sprintf(`atlas_calibration_freshness_ok{artifact="parameters"} %.6f`, map[bool]float64{true: 1, false: 0}[tc.wantFresh])
			if !strings.Contains(body, okLine) {
				t.Errorf("/metrics 缺少 %q\n%s", okLine, body)
			}
		})
	}
}
