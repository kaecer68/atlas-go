package monitoring

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// calibrationFreshnessFixture 產生一份「結構上有效」的最小 parameters.json。
// 只放 ValidateCalibration 真的會讀的欄位（updated_at +
// industry.classification_tree.value.segments），不複製整份出貨設定 ——
// 這一族測的是**新鮮度**，結構性 finding 由 internal/config 的測試負責。
func calibrationFreshnessFixture(t *testing.T, updatedAt time.Time, mtime time.Time) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "parameters.json")

	updatedAtField := "null"
	if !updatedAt.IsZero() {
		updatedAtField = fmt.Sprintf("%q", updatedAt.Format(time.RFC3339Nano))
	}
	body := fmt.Sprintf(`{
	  "version": "test",
	  "updated_at": %s,
	  "industry": {
	    "classification_tree": {
	      "value": {
	        "segments": [
	          {"id": "semiconductor", "name": "半導體產業", "level": 1,
	           "representative_stocks": ["2330.TW"]},
	          {"id": "ic_design", "name": "IC 設計", "level": 2, "parent_id": "semiconductor",
	           "representative_stocks": ["2454.TW"]}
	        ]
	      }
	    }
	  }
	}`, updatedAtField)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if !mtime.IsZero() {
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// scrapeCalibrationFreshness 回傳 /metrics 的輸出（走生產同一條 handler 路徑）。
func scrapeCalibrationFreshness(t *testing.T, c *MetricsCollector) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	PrometheusHandler(c).ServeHTTP(rec, req)
	return rec.Body.String()
}

// gaugeLine 產生 Prometheus exposition 的單行預期值（`PrometheusHandler` 的
// 數值格式是 `%.6f`，所以斷言要跟著那個格式，不能只比整數）。
func gaugeLine(name string, v float64) string {
	return fmt.Sprintf(`%s{artifact="%s"} %.6f`, name, CalibrationArtifactParameters, v)
}

// seriesLine 取出某個 metric 的單行輸出；不存在時回空字串（= 缺席，這在
// 本檔是**要斷言的行為**，所以刻意不 Fatal）。
func seriesLine(body, name string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, name+"{") || line == name {
			return line
		}
	}
	return ""
}

func TestObserveCalibrationFreshness_FreshArtifactIsFresh(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	updatedAt := now.Add(-65 * time.Minute) // 生產實測的健康值（FU-20260926-07）
	path := calibrationFreshnessFixture(t, updatedAt, time.Time{})

	c := NewMetricsCollector()
	obs := ObserveCalibrationFreshness(c, path, now)

	if !obs.RunOK {
		t.Fatalf("RunOK=false (code=%q)，預期可評估", obs.UnverifiableCode)
	}
	if !obs.Fresh {
		t.Fatalf("Fresh=false，預期新鮮（age=%.0fs）", obs.AgeSeconds)
	}
	if got, want := obs.AgeSeconds, 65*60.0; got != want {
		t.Errorf("AgeSeconds=%v，want %v", got, want)
	}

	body := scrapeCalibrationFreshness(t, c)
	mustContain := []string{
		gaugeLine(MetricCalibrationFreshnessOK, 1),
		gaugeLine(MetricCalibrationFreshnessRunOK, 1),
		gaugeLine(MetricCalibrationFreshnessCheckedTimestamp, float64(now.Unix())),
		gaugeLine(MetricCalibrationFreshnessAgeSeconds, 3900),
		gaugeLine(MetricCalibrationLastCalibratedTimestamp, float64(updatedAt.Unix())),
	}
	for _, want := range mustContain {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics 缺少 %q\n--- body ---\n%s", want, body)
		}
	}
}

func TestObserveCalibrationFreshness_StaleArtifactFlipsOK(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	// 版控那份 parameters.json 的實測年齡：updated_at 2026-07-06T01:43:01+08:00。
	updatedAt := now.Add(-72 * time.Hour)
	path := calibrationFreshnessFixture(t, updatedAt, time.Time{})

	c := NewMetricsCollector()
	obs := ObserveCalibrationFreshness(c, path, now)

	if !obs.RunOK {
		t.Fatal("RunOK 應為 true：檔案讀得到、JSON 合法")
	}
	if obs.Fresh {
		t.Fatal("Fresh 應為 false：72h > 48h 契約")
	}
	body := scrapeCalibrationFreshness(t, c)
	if want := gaugeLine(MetricCalibrationFreshnessOK, 0); !strings.Contains(body, want) {
		t.Errorf("/metrics 缺少 %q\n%s", want, body)
	}
	// 判定值為 0，但「檢查本身可以評估」必須仍然是 1（否則無法分辨
	// 「真的舊」與「不知道」）。
	if want := gaugeLine(MetricCalibrationFreshnessRunOK, 1); !strings.Contains(body, want) {
		t.Errorf("/metrics 缺少 %q\n%s", want, body)
	}
	if want := gaugeLine(MetricCalibrationFreshnessAgeSeconds, 259200); !strings.Contains(body, want) {
		t.Errorf("/metrics 缺少 %q\n%s", want, body)
	}
}

// 這是 fail-closed 的核心案例：產物說「我從來沒有被校準過」時，
// 既不能算新鮮、也不能算「不知道」。
func TestObserveCalibrationFreshness_NeverCalibratedIsNotFresh(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	path := calibrationFreshnessFixture(t, time.Time{}, time.Time{})

	c := NewMetricsCollector()
	obs := ObserveCalibrationFreshness(c, path, now)

	if !obs.RunOK {
		t.Fatal("RunOK 應為 true：JSON 讀得到也解析得動，缺的是 updated_at")
	}
	if obs.Fresh {
		t.Fatal("Fresh 應為 false：沒有記錄校準時間不得算新鮮")
	}
	if !obs.LastCalibrated.IsZero() {
		t.Errorf("LastCalibrated=%v，want zero", obs.LastCalibrated)
	}
	body := scrapeCalibrationFreshness(t, c)
	if want := gaugeLine(MetricCalibrationLastCalibratedTimestamp, 0); !strings.Contains(body, want) {
		t.Errorf("「從未校準」必須以明確的 0 哨兵值輸出，缺少 %q\n%s", want, body)
	}
	if line := seriesLine(body, MetricCalibrationFreshnessAgeSeconds); line != "" {
		t.Errorf("沒有可量的時間戳時不得輸出 age（避免假造量測值），got %q", line)
	}
}

func TestObserveCalibrationFreshness_MissingFileIsUnverifiable(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	c := NewMetricsCollector()
	obs := ObserveCalibrationFreshness(c, filepath.Join(t.TempDir(), "absent.json"), now)

	if obs.RunOK {
		t.Fatal("檔案不存在時 RunOK 必須是 false（fail-closed）")
	}
	if obs.Fresh {
		t.Fatal("無法評估時 Fresh 必須是 false")
	}
	if obs.UnverifiableCode == "" {
		t.Fatal("必須記錄導致無法評估的 finding code，讓日誌/盤查可分辨原因")
	}
	body := scrapeCalibrationFreshness(t, c)
	for _, want := range []string{
		gaugeLine(MetricCalibrationFreshnessRunOK, 0),
		gaugeLine(MetricCalibrationFreshnessOK, 0),
		gaugeLine(MetricCalibrationFreshnessCheckedTimestamp, float64(now.Unix())),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics 缺少 %q\n%s", want, body)
		}
	}
	// 沒有可信時間時，這兩個量必須缺席而不是輸出假的 0/巨大值。
	for _, name := range []string{MetricCalibrationFreshnessAgeSeconds, MetricCalibrationLastCalibratedTimestamp} {
		if line := seriesLine(body, name); line != "" {
			t.Errorf("%s 在無法評估時不得輸出，got %q", name, line)
		}
	}
}

func TestObserveCalibrationFreshness_InvalidJSONIsUnverifiable(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "parameters.json")
	if err := os.WriteFile(path, []byte("{ this is not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := NewMetricsCollector()
	obs := ObserveCalibrationFreshness(c, path, now)

	if obs.RunOK {
		t.Fatal("JSON 不合法時 RunOK 必須是 false")
	}
	if got := string(obs.UnverifiableCode); got != "PARAMS_INVALID_JSON" {
		t.Errorf("UnverifiableCode=%q，want PARAMS_INVALID_JSON", got)
	}
}

// issue #1995 的回歸守衛：整族必須在**第一次執行**就存在，且不隨前一次失敗而缺。
// 這裡模擬「啟動後第一次跑（檔案還沒被校準任務產生）→ 之後補上檔案」，
// 並斷言兩輪之間 series 只有被覆寫、沒有消失或增生。
func TestObserveCalibrationFreshness_WholeFamilyPresentFromFirstRun(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	c := NewMetricsCollector()

	missing := filepath.Join(t.TempDir(), "absent.json")
	ObserveCalibrationFreshness(c, missing, now)

	family := []string{
		MetricCalibrationFreshnessOK,
		MetricCalibrationFreshnessRunOK,
		MetricCalibrationFreshnessCheckedTimestamp,
	}
	for _, name := range family {
		if line := seriesLine(scrapeCalibrationFreshness(t, c), name); line == "" {
			t.Errorf("第一次執行（檔案缺席）就必須有 %s，否則 Prometheus 會看到缺席而非 0", name)
		}
	}

	path := calibrationFreshnessFixture(t, now.Add(-time.Hour), time.Time{})
	ObserveCalibrationFreshness(c, path, now.Add(time.Minute))

	body := scrapeCalibrationFreshness(t, c)
	// 五個系列都必須存在（前一次失敗的 run_ok=0 必須被覆寫成 1）。
	for _, name := range []string{
		MetricCalibrationFreshnessOK,
		MetricCalibrationFreshnessRunOK,
		MetricCalibrationFreshnessCheckedTimestamp,
		MetricCalibrationFreshnessAgeSeconds,
		MetricCalibrationLastCalibratedTimestamp,
	} {
		if line := seriesLine(body, name); line == "" {
			t.Errorf("成功執行後必須有 %s\n%s", name, body)
		}
	}
	if want := gaugeLine(MetricCalibrationFreshnessRunOK, 1); !strings.Contains(body, want) {
		t.Errorf("失敗狀態必須被後續成功覆寫，缺少 %q\n%s", want, body)
	}
	// 標籤基數：整族只有 artifact 一個標籤值。
	if n := strings.Count(body, `atlas_calibration_freshness_ok{`); n != 1 {
		t.Errorf("freshness_ok 只應有一個 series（label artifact=parameters），got %d\n%s", n, body)
	}
}

func TestObserveCalibrationFreshness_NilCollectorIsSafe(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	path := calibrationFreshnessFixture(t, now.Add(-time.Hour), time.Time{})

	obs := ObserveCalibrationFreshness(nil, path, now)
	if !obs.Fresh {
		t.Fatal("nil collector 仍必須完成評估")
	}
}

func TestObserveCalibrationFreshness_ContractBoundary(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	// 用可注入的 maxAge 走到契約邊界，不必等 48 小時。
	cases := []struct {
		name   string
		age    time.Duration
		maxAge time.Duration
		fresh  bool
	}{
		{"契約內", 47 * time.Hour, 48 * time.Hour, true},
		{"剛好等於契約", 48 * time.Hour, 48 * time.Hour, true}, // <= 才算新鮮
		{"超過契約 1 秒", 48*time.Hour + time.Second, 48 * time.Hour, false},
		{"未來時間戳夾到 0", -time.Hour, 48 * time.Hour, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := calibrationFreshnessFixture(t, now.Add(-tc.age), time.Time{})
			obs := observeCalibrationFreshness(NewMetricsCollector(), CalibrationArtifactParameters, path, tc.maxAge, now)
			if obs.Fresh != tc.fresh {
				t.Errorf("Fresh=%v，want %v（age=%.0fs, contract=%.0fs）", obs.Fresh, tc.fresh, obs.AgeSeconds, tc.maxAge.Seconds())
			}
			if obs.AgeSeconds < 0 {
				t.Errorf("AgeSeconds=%v，不得為負", obs.AgeSeconds)
			}
		})
	}
}

// 真輸入 smoke test：出貨的 configs/parameters.json 必須**可評估**
// （讀得到、解析得動）。不斷言它的新鮮度 —— 那個值屬於生產事實、會隨校準改變，
// 斷言它會讓測試與 repo 內容耦合。
//
// 為什麼要跑真檔：其餘案例都是合成 fixture，只有這一條會在使用者真的把
// parameters.json 改成讀不動/解不開（例如巨大的結構變更或編碼問題）時紅燈。
// 用 `-v` 執行會印出目前這份的觀察值（run_ok / age / last_calibrated），
// 是 runbook §4「驗收」那組命令的本機對照。
func TestObserveCalibrationFreshness_ShippedArtifactIsEvaluable(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "parameters.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("出貨設定檔不存在（非完整 checkout）: %v", err)
	}
	obs := ObserveCalibrationFreshness(NewMetricsCollector(), path, time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC))
	if !obs.RunOK {
		t.Fatalf("出貨的 parameters.json 必須可評估，got code=%q", obs.UnverifiableCode)
	}
	t.Logf("shipped artifact: fresh=%v age_seconds=%.0f last_calibrated=%v findings=%d",
		obs.Fresh, obs.AgeSeconds, obs.LastCalibrated.UTC().Format(time.RFC3339), len(obs.Findings))
}

// 出貨契約（48h）必須與 CLI/文件一致：這是「同一個政策只有一個數字」的
// 漂移守門。分兩邊釘住 —— Go 常數本身，以及 runbook 寫的 production 命令。
func TestCalibrationFreshnessContractMatchesRunbook(t *testing.T) {
	if CalibrationFreshnessContract != 48*time.Hour {
		t.Fatalf("CalibrationFreshnessContract=%v，want 48h", CalibrationFreshnessContract)
	}
	runbook := filepath.Join("..", "..", "docs", "operations", "calibration-freshness-runbook.md")
	data, err := os.ReadFile(runbook)
	if err != nil {
		t.Fatalf("讀取 runbook 失敗（新鮮度政策必須有落地文件）: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"atlas_calibration_freshness_ok",
		"atlas_calibration_freshness_run_ok",
		"atlas_calibration_freshness_age_seconds",
		"atlas_calibration_last_calibrated_timestamp_seconds",
		"atlas_calibration_freshness_checked_timestamp_seconds",
		"atlas-validate --path=configs/parameters.json --max-age=48h",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("runbook 缺少 %q（文件漂回舊說法或漏寫指標名即紅燈）", want)
		}
	}
	if strings.Contains(text, "--max-age=72h") {
		t.Error("runbook 不得出現第二個 max-age 值：政策只有一個數字")
	}
}

// 規則檔引用的每一個 atlas_calibration_* 指標名都必須真的由本檔輸出。
// promtool 抓不到這一類缺陷：規則引用一個**不存在**的指標時不會語法錯誤，
// 只會永遠沉默（false-green 的經典形狀）。
func TestCalibrationFreshnessRulesReferenceEmittedMetrics(t *testing.T) {
	rulesPath := filepath.Join("..", "..", "monitoring", "rules", "calibration_freshness_alerts.yml")
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("讀取規則檔失敗: %v", err)
	}
	emitted := map[string]bool{
		MetricCalibrationFreshnessOK:               true,
		MetricCalibrationFreshnessRunOK:            true,
		MetricCalibrationFreshnessAgeSeconds:       true,
		MetricCalibrationLastCalibratedTimestamp:   true,
		MetricCalibrationFreshnessCheckedTimestamp: true,
	}
	refs := regexp.MustCompile(`atlas_calibration_[a-z_]+`).FindAllString(string(data), -1)
	if len(refs) == 0 {
		t.Fatal("規則檔沒有引用任何 atlas_calibration_* 指標")
	}
	seen := map[string]bool{}
	for _, r := range refs {
		seen[r] = true
		if !emitted[r] {
			t.Errorf("規則檔引用了本檔沒有輸出的指標 %q（規則會永遠沉默）", r)
		}
	}
	var got []string
	for r := range seen {
		got = append(got, r)
	}
	sort.Strings(got)
	t.Logf("規則檔引用的指標: %v", got)
}

// 規則檔必須真的有牙齒（不是只有指標名對得上）：至少一條規則的 expr 引用
// fail-closed 訊號（run_ok），且至少一條以 absent() 覆蓋「檢查沒在跑」。
func TestCalibrationFreshnessRulesCoverFailClosedAndAbsence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "monitoring", "rules", "calibration_freshness_alerts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []struct {
		what   string
		needle string
	}{
		{"fail-closed（無法評估 ⇒ 明確告警）", MetricCalibrationFreshnessRunOK + `{artifact="parameters"} == 0`},
		{"缺席（檢查沒在跑 ⇒ 明確告警）", "absent(" + MetricCalibrationFreshnessCheckedTimestamp + ")"},
	} {
		if !strings.Contains(text, want.needle) {
			t.Errorf("%s：規則檔缺少 %q", want.what, want.needle)
		}
	}
}
