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

	"github.com/kaecer68/atlas-go/internal/config"
)

// testNow 回傳「現在」並截到秒。
//
// ⚠️ 為什麼測試用真時鐘而不是固定的假時間：`config.ValidateCalibration` 內部用
// `time.Since(...)`（真實時鐘）判定 mtime/updated_at 是否過期，本檔的 `Fresh` 就是
// 「有沒有 freshness finding」⇒ 若測試注入一個與真時鐘差好幾個小時的 `now`，
// 判定會跟著真時鐘漂移（假 now 在過去 ⇒ 相對於真 now 變成「過期」）。測試必須讓
// fixture 與真時鐘對齊；`now` 只用來計算本檔自己報出的 `age`（可精確對齊到秒）。
func testNow() time.Time { return time.Now().UTC().Truncate(time.Second) }

// calibrationFreshnessFixture 產生一份「結構上有效」的最小 parameters.json。
// 只放 ValidateCalibration 真的會讀的欄位（updated_at +
// industry.classification_tree.value.segments），不複製整份出貨設定 ——
// 這一族測的是**新鮮度**，結構性 finding 由 internal/config 的測試負責。
//
// mtime 非零時會把檔案時間戳設成該值（模擬 `cp -p` 還原出來的舊 mtime）。
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

// gaugeLine 產生 Prometheus exposition 的單行預期值（`PrometheusHandler` 的
// 數值格式是 `%.6f`，所以斷言要跟著那個格式，不能只比整數）。
func gaugeLine(name string, v float64) string {
	return fmt.Sprintf(`%s{artifact="%s"} %.6f`, name, CalibrationArtifactParameters, v)
}

// scrapeCalibrationFreshness 回傳 /metrics 的輸出（走生產同一條 handler 路徑）。
func scrapeCalibrationFreshness(t *testing.T, c *MetricsCollector) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	PrometheusHandler(c).ServeHTTP(rec, req)
	return rec.Body.String()
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
	now := testNow()
	updatedAt := now.Add(-65 * time.Minute) // 生產實測的健康值（FU-20260926-07）
	path := calibrationFreshnessFixture(t, updatedAt, time.Time{})

	c := NewMetricsCollector()
	obs := ObserveCalibrationFreshness(c, path, now)

	if !obs.RunOK {
		t.Fatalf("RunOK=false (code=%q)，預期可評估", obs.UnverifiableCode)
	}
	if !obs.Fresh {
		t.Fatalf("Fresh=false (code=%q)，預期新鮮（age=%.0fs）", obs.FreshnessCode, obs.AgeSeconds)
	}
	if obs.FreshnessCode != "" {
		t.Errorf("新鮮時 FreshnessCode 應為空，got %q", obs.FreshnessCode)
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
	now := testNow()
	// 版控那份 parameters.json 的實測年齡：updated_at 2026-07-06T01:43:01+08:00 ≈ 82.8 天。
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
	if got := string(obs.FreshnessCode); got != "UPDATED_AT_STALE" {
		t.Errorf("FreshnessCode=%q，want UPDATED_AT_STALE", got)
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

// 「CLI 說不新鮮、監控說新鮮」是這條接線最不能出現的矛盾（本檔頭與 runbook 都
// 把它列為單一政策的要求）。mtime 舊但 updated_at 新（`cp -p` 還原一份舊檔）是
// 唯一會讓兩邊分開的形狀 ⇒ 判定必須跟 CLI 一樣是 **不新鮮**（mtimes 也是
// `ValidateCalibration` 的 freshness finding 之一）。
func TestObserveCalibrationFreshness_MtimeStaleMatchesCLI(t *testing.T) {
	now := testNow()
	path := calibrationFreshnessFixture(t, now.Add(-30*time.Minute), now.Add(-72*time.Hour))

	cli, err := config.ValidateCalibration(path, CalibrationFreshnessContract)
	if err != nil {
		t.Fatal(err)
	}
	obs := ObserveCalibrationFreshness(NewMetricsCollector(), path, now)

	if obs.Fresh != cli.OK {
		t.Errorf("gauge 判定（fresh=%v）與 CLI（OK=%v）不一致 —— 兩者必須同源", obs.Fresh, cli.OK)
	}
	if obs.Fresh {
		t.Fatal("mtime 過期時必須判為不新鮮（與 CLI 一致）")
	}
	if got := string(obs.FreshnessCode); got != "MTIME_STALE" {
		t.Errorf("FreshnessCode=%q，want MTIME_STALE", got)
	}
	// age 只反映 updated_at ⇒ 在這種形狀下 age 小是**正確**的，但它不是判定值。
	if obs.AgeSeconds > 3600 {
		t.Errorf("AgeSeconds=%v，updated_at 只有 30 分鐘前，age 應小", obs.AgeSeconds)
	}
	c := NewMetricsCollector()
	ObserveCalibrationFreshness(c, path, now)
	if want := gaugeLine(MetricCalibrationFreshnessAgeSeconds, 1800); !strings.Contains(scrapeCalibrationFreshness(t, c), want) {
		t.Errorf("/metrics 缺少 %q（age 仍只反映 updated_at）", want)
	}
}

// 這是 fail-closed 的核心案例之一：產物說「我從來沒有被校準過」時，
// 既不能算新鮮、也不能算「不知道」。
func TestObserveCalibrationFreshness_NeverCalibratedIsNotFresh(t *testing.T) {
	now := testNow()
	path := calibrationFreshnessFixture(t, time.Time{}, time.Time{})

	c := NewMetricsCollector()
	obs := ObserveCalibrationFreshness(c, path, now)

	if !obs.RunOK {
		t.Fatal("RunOK 應為 true：JSON 讀得到也解析得動，缺的是 updated_at")
	}
	if obs.Fresh {
		t.Fatal("Fresh 應為 false：沒有記錄校準時間不得算新鮮")
	}
	if got := string(obs.FreshnessCode); got != "UPDATED_AT_ZERO" {
		t.Errorf("FreshnessCode=%q，want UPDATED_AT_ZERO", got)
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
	now := testNow()
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
	// 第一次執行（全新的 collector）時這兩個量必須缺席而不是輸出假的 0/巨大值。
	for _, name := range []string{MetricCalibrationFreshnessAgeSeconds, MetricCalibrationLastCalibratedTimestamp} {
		if line := seriesLine(body, name); line != "" {
			t.Errorf("%s 在無法評估時不得**新寫入**，got %q", name, line)
		}
	}
}

// ⚠️ 這是**已知且刻意**的行為（檔頭「凍結樣本的語意」）：gauge 是 last-write-wins
// 且 collector 不會移除序列 ⇒ 由「可評估」變成「無法評估」之後，`/metrics` 上仍留著
// 最後一次可評估時的 age / last_calibrated。
//
// 為什麼可以接受：判定（`_ok` / `_run_ok`）每一輪都被覆寫，且**沒有任何規則**拿
// age / last_calibrated 做判定 ⇒ 凍結值不會製造誤報；它只會被值班的人讀到，所以
// runbook 明確要求「先看 run_ok 再讀 age」。
//
// 本測試存在的目的：把這個取捨釘成可執行的規格（有人若以為「不輸出 = 序列消失」
// 而據此改文案或改規則，這裡會紅）。
func TestObserveCalibrationFreshness_UnverifiableFreezesLastKnownSeries(t *testing.T) {
	now := testNow()
	path := calibrationFreshnessFixture(t, now.Add(-65*time.Minute), time.Time{})
	c := NewMetricsCollector()

	ObserveCalibrationFreshness(c, path, now) // 第一輪：可評估、新鮮
	body1 := scrapeCalibrationFreshness(t, c)
	if !strings.Contains(body1, gaugeLine(MetricCalibrationFreshnessAgeSeconds, 3900)) {
		t.Fatalf("第一輪必須輸出 age=3900\n%s", body1)
	}

	// 第二輪：檔案消失（無法評估）。collector 仍持有第一輪的 age 序列。
	ObserveCalibrationFreshness(c, filepath.Join(t.TempDir(), "gone.json"), now.Add(6*time.Minute))
	body2 := scrapeCalibrationFreshness(t, c)

	if want := gaugeLine(MetricCalibrationFreshnessRunOK, 0); !strings.Contains(body2, want) {
		t.Errorf("run_ok 必須被覆寫成 0，缺少 %q\n%s", want, body2)
	}
	if want := gaugeLine(MetricCalibrationFreshnessOK, 0); !strings.Contains(body2, want) {
		t.Errorf("ok 在無法評估時必須是 0（fail-closed），缺少 %q\n%s", want, body2)
	}
	if want := gaugeLine(MetricCalibrationFreshnessCheckedTimestamp, float64(now.Add(6*time.Minute).Unix())); !strings.Contains(body2, want) {
		t.Errorf("心跳必須被覆寫成最新的執行時間，缺少 %q\n%s", want, body2)
	}
	// 凍結樣本：值仍是第一輪的 3900（**不是**重算過的 4260）。
	if !strings.Contains(body2, gaugeLine(MetricCalibrationFreshnessAgeSeconds, 3900)) {
		t.Errorf("已知行為：age 序列會凍結在第一輪的值，缺少 %q\n%s",
			gaugeLine(MetricCalibrationFreshnessAgeSeconds, 3900), body2)
	}
	t.Logf("已知行為：無法評估後 age 序列仍以凍結值留在 /metrics（判讀順序見 runbook §2）")
}

func TestObserveCalibrationFreshness_InvalidJSONIsUnverifiable(t *testing.T) {
	now := testNow()
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
	now := testNow()
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
	now := testNow()
	path := calibrationFreshnessFixture(t, now.Add(-time.Hour), time.Time{})

	obs := ObserveCalibrationFreshness(nil, path, now)
	if !obs.Fresh {
		t.Fatal("nil collector 仍必須完成評估")
	}
}

func TestObserveCalibrationFreshness_ContractBoundary(t *testing.T) {
	now := testNow()
	// 用可注入的 maxAge 走到契約邊界，不必等 48 小時。
	// ⚠️ 刻意不測「剛好等於契約」：判定在 `ValidateCalibration` 內用真時鐘做
	// `time.Since(...) > maxAge`，相差幾個微秒就會落在另一側 ⇒ 那種案例必然 flaky。
	cases := []struct {
		name   string
		age    time.Duration
		maxAge time.Duration
		fresh  bool
	}{
		{"契約內", 47 * time.Hour, 48 * time.Hour, true},
		{"超過契約 1 分鐘", 48*time.Hour + time.Minute, 48 * time.Hour, false},
		{"未來時間戳夾到 0", -time.Hour, 48 * time.Hour, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := calibrationFreshnessFixture(t, now.Add(-tc.age), time.Time{})
			obs := observeCalibrationFreshness(NewMetricsCollector(), CalibrationArtifactParameters, path, tc.maxAge, now)
			if obs.Fresh != tc.fresh {
				t.Errorf("Fresh=%v，want %v（age=%.0fs, contract=%.0fs, code=%q）",
					obs.Fresh, tc.fresh, obs.AgeSeconds, tc.maxAge.Seconds(), obs.FreshnessCode)
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
// 用 `-v` 執行會印出目前這份的觀察值（run_ok / fresh / age / last_calibrated），
// 是 runbook §4「驗收」那組命令的本機對照。
func TestObserveCalibrationFreshness_ShippedArtifactIsEvaluable(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "parameters.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("出貨設定檔不存在（非完整 checkout）: %v", err)
	}
	obs := ObserveCalibrationFreshness(NewMetricsCollector(), path, testNow())
	if !obs.RunOK {
		t.Fatalf("出貨的 parameters.json 必須可評估，got code=%q", obs.UnverifiableCode)
	}
	t.Logf("shipped artifact: fresh=%v freshness_code=%q age_seconds=%.0f last_calibrated=%v findings=%d",
		obs.Fresh, obs.FreshnessCode, obs.AgeSeconds, obs.LastCalibrated.UTC().Format(time.RFC3339), len(obs.Findings))
}

// 出貨契約必須只有一個數字：`CalibrationFreshnessContract` = `config.DefaultCalibrationMaxAge`
// = CLI 的 `--max-age` 預設值。這條測試同時讀 CLI 原始碼與 runbook，兩邊任一漂移即紅燈。
func TestCalibrationFreshnessContractMatchesCLIAndRunbook(t *testing.T) {
	if CalibrationFreshnessContract != config.DefaultCalibrationMaxAge {
		t.Fatalf("CalibrationFreshnessContract=%v，必須等於 config.DefaultCalibrationMaxAge=%v",
			CalibrationFreshnessContract, config.DefaultCalibrationMaxAge)
	}
	if CalibrationFreshnessContract != 48*time.Hour {
		t.Fatalf("CalibrationFreshnessContract=%v，want 48h（改這個值＝改政策，必須同時更新 runbook 與 CLI）", CalibrationFreshnessContract)
	}

	cli, err := os.ReadFile(filepath.Join("..", "..", "cmd", "calibration-validate", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cli), `flag.Duration("max-age", config.DefaultCalibrationMaxAge`) {
		t.Error("CLI 的 --max-age 預設值必須引用 config.DefaultCalibrationMaxAge（不得再寫一份字面值）")
	}
	if strings.Contains(string(cli), "48 * time.Hour") {
		t.Error("CLI 不得再保有第二份 48h 字面值（政策只有一個數字）")
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

// 規則檔**expr** 引用的每一個 atlas_calibration_* 指標名都必須真的由本檔輸出。
// promtool 抓不到這一類缺陷：規則引用一個**不存在**的指標時不會語法錯誤，
// 只會永遠沉默（false-green 的經典形狀）。
//
// ⚠️ 掃描前先去掉 YAML 註解：註解裡提到指標名（triage 文案）不算「被規則引用」，
// 否則測試會誤以為 age / last_calibrated 有判定側消費者。
func TestCalibrationFreshnessRulesReferenceEmittedMetrics(t *testing.T) {
	rulesPath := filepath.Join("..", "..", "monitoring", "rules", "calibration_freshness_alerts.yml")
	data, err := os.ReadFile(rulesPath)
	if err != nil {
		t.Fatalf("讀取規則檔失敗: %v", err)
	}
	var exprLines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		exprLines = append(exprLines, line)
	}
	exprs := strings.Join(exprLines, "\n")

	emitted := map[string]bool{
		MetricCalibrationFreshnessOK:               true,
		MetricCalibrationFreshnessRunOK:            true,
		MetricCalibrationFreshnessAgeSeconds:       true,
		MetricCalibrationLastCalibratedTimestamp:   true,
		MetricCalibrationFreshnessCheckedTimestamp: true,
	}
	refs := regexp.MustCompile(`atlas_calibration_[a-z_]+`).FindAllString(exprs, -1)
	if len(refs) == 0 {
		t.Fatal("規則檔（非註解部分）沒有引用任何 atlas_calibration_* 指標")
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
	t.Logf("規則 expr 引用的指標: %v", got)
}

// 規則檔必須真的有牙齒（不是只有指標名對得上）：至少一條規則的 expr 引用
// fail-closed 訊號（run_ok），且至少一條以 absent() 覆蓋「檢查沒在跑」。
func TestCalibrationFreshnessRulesCoverFailClosedAndAbsence(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "monitoring", "rules", "calibration_freshness_alerts.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var keep []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		keep = append(keep, line)
	}
	text := strings.Join(keep, "\n")
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
