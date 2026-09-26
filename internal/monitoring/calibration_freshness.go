package monitoring

// Package monitoring — 校準產物新鮮度指標（issue #1944 I31 的 production 半邊）。
//
// 為什麼存在（2026-09-26）
// -----------------------
// PR #1991 把 `calibration-validate` 的 CI 半邊補起來了（step exit code = 驗證程式
// exit code、零設定就有 ::error:: annotation），但它自己在「未完成項」明示：
//
//	production 側的 freshness 檢查尚未接上監控（本 PR 只提供命令與政策）。
//
// 也就是說「`configs/parameters.json` 已經不新鮮」這件事在生產上**只有人記得去跑
// 那條命令**才會被發現 —— 與 issue #1944 整批的 false-green 主題同族。
// 本檔把那個檢查的結果變成 Prometheus 序列，讓它可以被既有監控（scrape + 規則 +
// Alertmanager）看見，**不新造任何監控系統**。
//
// 掛載點（為什麼是這裡，而不是別的）
// ---------------------------------
//  1. 指標面：沿用既有的 `monitoring.MetricsCollector`（`RecordGauge`）與
//     `cmd/atlas/api_routes.go` 的 `/metrics`（`PrometheusHandler`）——與
//     `atlas_channel_*`、`atlas_universe_*` 同一條管線，Prometheus 的重貼標/抓取
//     設定完全不用改（`monitoring/prometheus.yml` 的 `atlas-go` job 已是 10s 抓取）。
//  2. 排程面：沿用既有的 `apigateway.BackgroundTaskManager`（`cmd/atlas` 註冊），
//     與 `channel_health_metrics_export` 同一個形狀（見
//     `cmd/atlas/calibration_freshness_metrics_task.go`）。
//  3. 判定面：**重用** `config.ValidateCalibration`（`internal/config/integrity.go`）
//     ——也就是 `cmd/calibration-validate` 本身用的那一個函式。本檔不重寫任何
//     staleness/結構判定邏輯（重寫就會出現第二套語意，兩個地方遲早不一致）。
//  4. 告警面：新增 `monitoring/rules/calibration_freshness_alerts.yml`，
//     與既有規則同一棵權威樹（`scripts/ci/check_monitoring_single_source.sh` 會擋
//     把規則寫到 repo 外的第二棵樹）。
//
// 評估過但**不採用**的掛載點：`channel health` 契約
// --------------------------------------------------
// `atlas_channel_*` 已經有 staleness / overage / status 三個 gauge 與
// `ChannelDataStale` 告警，看起來最省事，但契約語意不合：
//  * `ChannelContracts()`（`internal/apigateway/channel_contract.go`）描述的是
//    「gateway 抓取的外部資料通道」，其記錄由 adapter 寫入 `data/state/channel_health.json`；
//    `configs/parameters.json` 不是任何 adapter 抓回來的資料，而是**應用自己寫的校準產物**。
//  * 硬塞一個合成 channel 記錄進去，等於在 channel 健康報表上宣稱一個不存在的抓取通道
//    （`DeriveChannelStatus` / 後台通道清單 / `cmd/check-channel-consistency` 都會把它當真）
//    —— 這正是本 session 反覆在修的「宣告與事實不符」。
//  ⇒ 改用獨立的小型 gauge 族（本檔），語意單一且不污染既有契約。
//
// 指標族的設計約束（每一條都對應一個已發生過的缺陷）
// --------------------------------------------------
//  (a) **不得等到第一次 increment 才存在**（issue #1995）：本族全部是 gauge，
//      由背景任務**每一次執行**無條件輸出 ⇒ 服務啟動後數十秒內就存在，沒有
//      「counter 第一次 Add 才建立 series」那種缺席視窗。任務的第一次執行延遲受
//      `startupStaggerDelay` 限制（interval=5m ⇒ 上限 30s），不是 1 天。
//  (b) **缺席要有規則負責**：仍然加了一條 `absent()` 規則（以 `up{job="atlas-go"}` 為閘門），
//      覆蓋「任務停止執行」與「指標被改名/移除」兩種缺席，且不與其他規則重複 paging。
//  (c) **fail-closed**：檢查本身失敗（stat/讀取/JSON 解析失敗）時，`..._run_ok` = 0
//      且 `..._ok` = 0（未知**不得**被當成新鮮），並由專門的規則以 severity=error 發出。
//  (d) **狀態必須持續覆寫**：`MetricsCollector` 的 gauge 是 last-write-wins 且不清序列，
//      條件消失就跳過輸出會讓舊樣本永久凍結（`atlas_channel_staleness_overage_seconds`
//      的檔內註解記載過這個坑）。因此 `..._ok` / `..._run_ok` / `..._checked_timestamp_seconds`
//      每一輪都輸出；只有「值真的不存在」的兩個量（age / last_calibrated_timestamp）
//      在無法判定時刻意不輸出，並由該狀態自己的規則負責（見 (c)）。
//
// 與 CLI 的關係（避免兩套政策）
// ---------------------------
// production 的命令（`docs/specs/industry-allocation-inert-audit-20260924.md` §10.2）是
// `atlas-validate --path=configs/parameters.json --max-age=48h --format=json`。
// 本檔的 `CalibrationFreshnessContract` 就是那個 max-age，兩者必須一致，否則會出現
// 「CLI 說不新鮮、監控說新鮮」的矛盾。`docs/operations/calibration-freshness-runbook.md`
// 是這條契約的落地說明，並有 Go 測試（`TestCalibrationFreshnessContractMatchesRunbook`）
// 釘住「文件寫的 max-age」與「Go 常數」不得漂移。

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// Calibration freshness gauge 族（Prometheus 命名慣例：gauge 不加 _total）。
const (
	// MetricCalibrationFreshnessOK 是「這個校準產物新鮮嗎」的判定值：
	// 1 = 新鮮（可評估、且有記錄的校準時間落在契約內）；0 = 其他一切情況
	// （超過契約 / 從未記錄校準時間 / **檢查本身無法評估**）。
	//
	// 0 刻意包含「無法評估」：未知不得被當成新鮮（fail-closed）。要區分
	// 「真的舊」與「不知道」請看 MetricCalibrationFreshnessRunOK。
	MetricCalibrationFreshnessOK = "atlas_calibration_freshness_ok"
	// MetricCalibrationFreshnessRunOK 是「檢查本身能不能評估這個產物」：
	// 1 = 可以（檔案讀得到、JSON 合法）；0 = 不行（stat/讀取/解析失敗）。
	// 這是 fail-closed 的來源：只有它 =1 時，MetricCalibrationFreshnessOK 才
	// 具有「是否新鮮」的意義。
	MetricCalibrationFreshnessRunOK = "atlas_calibration_freshness_run_ok"
	// MetricCalibrationFreshnessAgeSeconds 是「落後多少」：產物自述的最後校準
	// 時間距今幾秒。**只在有記錄時間時輸出**（updated_at 為零 ⇒ 沒有可量的落後量，
	// 該狀態由 MetricCalibrationLastCalibratedTimestamp 的 0 值負責）。
	MetricCalibrationFreshnessAgeSeconds = "atlas_calibration_freshness_age_seconds"
	// MetricCalibrationLastCalibratedTimestamp 是「最後一次成功校準時間」的
	// Unix 秒數（產物自己記的 `updated_at`）。**0 是明確的哨兵值：「從未記錄校準時間」**
	// —— 不是 1970 年，也不是「沒有資料」；該序列在可評估時一定會輸出。
	MetricCalibrationLastCalibratedTimestamp = "atlas_calibration_last_calibrated_timestamp_seconds"
	// MetricCalibrationFreshnessCheckedTimestamp 是「這個檢查最後一次執行」的
	// Unix 秒數（探針心跳）。用來讓規則分辨「產物舊」與「檢查沒在跑」。
	MetricCalibrationFreshnessCheckedTimestamp = "atlas_calibration_freshness_checked_timestamp_seconds"
)

// CalibrationArtifactParameters 是唯一目前受監控的校準產物
// （`configs/parameters.json`）。保留 label 是因為同族的檢查未來可能要涵蓋
// 其他產物（例如 `configs/sector_symbols.json`），屆時不必改指標名。
const CalibrationArtifactParameters = "parameters"

// CalibrationFreshnessContract 是「校準產物多久算不新鮮」的權威門檻。
//
// 與 production CLI 的 `--max-age=48h` 是**同一個政策**（見檔頭「與 CLI 的關係」）。
// 為什麼是 48h（實測基線，不是照抄別的規則）：
//   - 生產實測的「健康」值：2026-09-26 唯讀盤查，容器 `Created=02:04:02Z`、
//     檔案 `updated_at=2026-09-26T03:08:44.90062791Z` ⇒ 啟動後約 **65 分鐘**就被
//     校準任務群改寫（`docs/operations/FOLLOWUPS.md` FU-20260926-07）。
//   - 生產實測的「壞掉」值：同一份盤查指出 image 內（=版控那份）的
//     `updated_at=2026-07-06` ⇒ 約 **82 天**，且那是 issue #1944 I30 的形狀
//     （nightly backfill 的寫入被丟棄，永遠不會刷新）。
//   - 兩者之間有 ~48×（對健康值）與 ~40×（對壞值）的差距；48h 落在這個空隙裡，
//     並且與 CLI 契約一致（同一個政策只有一個數字）。
//   - 校準任務群的主要 cadence 是 24h（`cmd/atlas/calibration_tasks.go` 的
//     17 個 top-level 任務；`auto_cycle_update` 6h、`regime_calibrate` 1h），
//     48h = 兩個週期，可以吸收一次失敗的週期與週末（`ValidateCalibration` 的
//     既有註解即為此而設）。
//
// ⚠️ 已知的假陽性形狀（誠實聲明，不要當成 bug 回報）：校準寫入是**有變更才寫**
// （`internal/risk/self_calibrate.go`：`if len(report.Changes) > 0` 才
// `LockedSaveWithRollback`），因此「已收斂、連續多輪 verdict=stable」的系統
// 可能合法地超過 48h 不改寫檔案。規則的註解與 runbook 把這一格列為第一順位的
// 誤報排查（缺一個真正的「校準任務已執行」心跳指標，屬另一張票）。
const CalibrationFreshnessContract = 48 * time.Hour

// calibrationUnverifiableCodes 是「檢查無法評估這個產物」的 finding 集合。
// 這三個 code 由 `config.collectCalibrationFindings` 在早期失敗路徑發出
// （stat 失敗 / 讀取失敗 / JSON 不合法），此時 updated_at 與 mtime 都沒有可信值。
var calibrationUnverifiableCodes = map[config.CalibrationFindingCode]bool{
	config.CalibrationFindingParamsStatFailed:  true,
	config.CalibrationFindingParamsReadFailed:  true,
	config.CalibrationFindingParamsInvalidJSON: true,
}

// CalibrationFreshnessObservation 是觀察結果，回傳給呼叫端做日誌與測試斷言
// （測試不需要去 scrape /metrics 才能驗證判定）。
type CalibrationFreshnessObservation struct {
	Artifact string
	Path     string
	// CheckedAt 是本次檢查的時間（注入的 now，測試可重現）。
	CheckedAt time.Time
	// RunOK 為 false 代表檢查無法評估產物（檔案不存在/讀不到/JSON 不合法）。
	RunOK bool
	// Fresh 只在 RunOK 為 true 時才有「新鮮」的意思，語意同 MetricCalibrationFreshnessOK。
	Fresh bool
	// LastCalibrated 是產物自述的最後校準時間；zero 代表沒有記錄。
	LastCalibrated time.Time
	// AgeSeconds / HaveAge：有記錄時間才會 true。
	AgeSeconds float64
	HaveAge    bool
	// UnverifiableCode 是導致 RunOK=false 的 finding code（可讀失敗為空字串）。
	UnverifiableCode config.CalibrationFindingCode
	// Findings 是底層檢查的完整結果（含結構性 finding），供日誌使用。
	Findings []config.CalibrationFinding
}

// ObserveCalibrationFreshness 以權威契約（CalibrationFreshnessContract）評估
// path 的校準產物並輸出 gauge 族。collector 為 nil 時只做評估、不輸出
// （與 `RecordDBInitFailure` 等既有 helper 的 nil 安全慣例一致）。
//
// 本函式**不**回傳 error：檢查失敗本身就是一個要被觀測的狀態（fail-closed 序列
// 而不是例外），呼叫端不需要（也不應該）用 error 與「不新鮮」混在一起判斷。
func ObserveCalibrationFreshness(collector *MetricsCollector, path string, now time.Time) CalibrationFreshnessObservation {
	return observeCalibrationFreshness(collector, CalibrationArtifactParameters, path, CalibrationFreshnessContract, now)
}

// observeCalibrationFreshness 是可注入 artifact/maxAge 的實作，給測試用
// （讓「剛好在契約邊界」這種案例不必等 48 小時）。
func observeCalibrationFreshness(collector *MetricsCollector, artifact, path string, maxAge time.Duration, now time.Time) CalibrationFreshnessObservation {
	obs := CalibrationFreshnessObservation{Artifact: artifact, Path: path, CheckedAt: now}
	labels := map[string]string{"artifact": artifact}

	// 重用 CLI 用的那一個判定函式（無 policy ⇒ scope=full，全部 finding 都是 error；
	// 這裡只讀 finding 的 **code** 來決定「能不能評估」，不看 severity）。
	res, err := config.ValidateCalibration(path, maxAge)
	if err != nil || res == nil {
		// 無 policy 時 ValidateCalibration 不會回 error；保險起見仍走 fail-closed
		// 路徑，讓「呼叫面壞掉」也留下可觀測的 0，而不是靜默。
		obs.RunOK = false
		obs.Fresh = false
		emitCalibrationFreshnessGauges(collector, labels, now, obs)
		return obs
	}
	obs.Findings = res.Findings
	obs.RunOK = true

	for _, f := range res.Findings {
		if calibrationUnverifiableCodes[f.Code] {
			obs.RunOK = false
			obs.UnverifiableCode = f.Code
			break
		}
	}
	if !obs.RunOK {
		obs.Fresh = false
		emitCalibrationFreshnessGauges(collector, labels, now, obs)
		return obs
	}

	if !res.UpdatedAt.IsZero() {
		obs.LastCalibrated = res.UpdatedAt
		age := now.Sub(res.UpdatedAt).Seconds()
		if age < 0 {
			// 產物自述的時間落在未來（時鐘偏移或人為編輯）：與
			// computeChannelStalenessSeconds 的既有處置一致，夾到 0。
			age = 0
		}
		obs.AgeSeconds = age
		obs.HaveAge = true
	}
	obs.Fresh = obs.HaveAge && obs.AgeSeconds <= maxAge.Seconds()

	emitCalibrationFreshnessGauges(collector, labels, now, obs)
	return obs
}

// emitCalibrationFreshnessGauges 輸出整族。
//
// 每一輪**無條件**輸出 checked/run_ok/ok（見檔頭約束 (a)/(d)）；
// age 與 last_calibrated_timestamp 只在檢查可以評估時輸出
// （無法評估 ⇒ 沒有可信的時間值，硬輸出一個數字就是假造量測值）。
func emitCalibrationFreshnessGauges(collector *MetricsCollector, labels map[string]string, now time.Time, obs CalibrationFreshnessObservation) {
	if collector == nil {
		return
	}
	collector.RecordGauge(MetricCalibrationFreshnessCheckedTimestamp, float64(now.Unix()), labels)
	collector.RecordGauge(MetricCalibrationFreshnessRunOK, boolGauge(obs.RunOK), labels)
	collector.RecordGauge(MetricCalibrationFreshnessOK, boolGauge(obs.Fresh), labels)
	if !obs.RunOK {
		return
	}
	lastCalibrated := float64(0) // 0 = 從未記錄（明確哨兵值，見常數註解）
	if !obs.LastCalibrated.IsZero() {
		lastCalibrated = float64(obs.LastCalibrated.Unix())
	}
	collector.RecordGauge(MetricCalibrationLastCalibratedTimestamp, lastCalibrated, labels)
	if obs.HaveAge {
		collector.RecordGauge(MetricCalibrationFreshnessAgeSeconds, obs.AgeSeconds, labels)
	}
}

func boolGauge(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
