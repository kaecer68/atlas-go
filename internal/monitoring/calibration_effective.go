package monitoring

// 校準產物的「權威標的」新鮮度 ＋ image-vs-effective drift（issue #2007 的兩個 bounded 子項）。
//
// 為什麼原本的檢查量錯標的（2026-09-27 實證）
// ------------------------------------------
// `calibration_freshness.go` 只評估 `configs/parameters.json`——image 內、受版控、
// 經人工審查的**基準**（下稱 SSOT / image baseline）。那在 #2013 之前是正確標的：
// 校準迴圈會改寫它，所以「它多久沒被改寫」＝「校準迴圈還有沒有在動」。
//
// #2013（FU-20260926-07）把容器內校準的寫入改到 bind mount 上的 overlay
// （`constants.StateParametersCalibrated`，生產 `/app/data/state/parameters.calibrated.json`），
// SSOT 保持 pristine。生產實測（2026-09-27，容器已跑 ~33h）：
//
//	atlas_calibration_freshness_ok{artifact="parameters"}      0
//	atlas_calibration_freshness_run_ok{artifact="parameters"}  1
//	atlas_calibration_freshness_age_seconds{artifact="parameters"}  7206752   ≈ 83 天
//
// 83 天 = SSOT 的 `updated_at`（2026-07-06，= image 建置日）；而 overlay 是**當天**
// 11:27 寫的（32,113 bytes）。也就是說 `CalibrationArtifactStale` 是**永久誤報**：
// 它監控的產物在生產上根本不會被寫（`SaveWithRollback` 每次都更新 `updated_at`，
// 故 `updated_at` 停在建置日等價於「這個檔自 image 之後沒被寫過」）。
//
// 本檔補上兩個 bounded 子項，**不改變任何既有 gauge 名稱的語意**：
//
//	(1) 權威產物的新鮮度：新增 artifact 標籤值 `parameters_overlay`
//	    （`CalibrationArtifactParametersOverlay`），評估 overlay 自己的 `updated_at`。
//	    既有 `artifact="parameters"`（SSOT）序列與語意完全不動——它仍然回答
//	    「image 內基準多久沒被換過」，只是**不再有告警規則**（見下）。
//	(2) image-vs-effective drift：把「基準值」與「overlay 疊加後生效值」的差異變成
//	    序列（`atlas_calibration_drift_*`），讓 #2007 那個「repo 0.03 / effective 0.0108
//	    而沒有任何痕跡」的狀態從此是**可查詢**的事實。
//
// 為什麼 drift 本身**不是**告警（重要，這是一個刻意的政策決定）
// ------------------------------------------------------------
// 業主已定案：runtime 校準是 **intended**，採「持久化 overlay ＋ 漂移偵測」。因此
// `atlas_calibration_drift_keys > 0` 是**健康系統的正常狀態**，對它開告警等於複製
// 我們剛剛修掉的永久誤報。drift 的呈現分工是：
//
//	· 可見性（不 page）：`atlas_calibration_drift_keys` —— 現在有幾個 tunable 的生效值
//	  與基準不同。值班/儀表板/runbook 用它回答「這台機器偏離憲章多少」。
//	· 告警（會 page）：只有**無法解釋**的偏離才 page
//	  （`CalibrationEffectiveDriftUnexplained`：比值落在單步窗 [1/3, 3] 外，
//	  或 overlay 宣告了**套不上**的 entry）。兩者在 promtool 測試裡都有正反案例，
//	  其中「單純 drift 不許 firing」是被寫成可執行規格的（案例 N）。
//
// 讀不動時 fail-closed，但**不**假造 0
// ------------------------------------
// drift 無法計算（基準讀不到／overlay 解不開）時 `atlas_calibration_drift_run_ok` = 0，
// 且數值序列**不輸出**（凍結在最後一次可計算的值）——與既有 age/last_calibrated 的
// 處置一致：0 是「沒有偏離」這個**事實**，不是「量不到」的代名詞，把兩者混用就是
// 本 repo 反覆在修的 false-green。規則面則以 drift 的 run_ok 當閘門
// （`and on() ... == 1`），未知狀態交由 overlay 的
// `CalibrationFreshnessUnverifiable`（error）單獨負責，同一個根因不重複 paging。

import (
	"encoding/json"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

// CalibrationArtifactParametersOverlay 是**權威的 effective 校準產物**：校準 overlay。
//
// 新增這個標籤值（而不是把 `artifact="parameters"` 改指向 overlay）是刻意的：
// 既有序列的語意不得改變（既有消費者/儀表板會把它讀成「SSOT 的新鮮度」），
// 而兩者回答的是不同的問題——「image 內基準多久沒換」vs「runtime 有沒有在校準」。
// 規則面的對應見 `monitoring/rules/calibration_freshness_alerts.yml` 檔頭。
const CalibrationArtifactParametersOverlay = "parameters_overlay"

// Overlay 新鮮度的 finding codes。
//
// 為什麼另立一套 code，而不用 `config.CalibrationFindingCode` 既有的
// PARAMS_STAT_FAILED / PARAMS_READ_FAILED / PARAMS_INVALID_JSON：那三個是
// `config.ValidateCalibration` 對「一份 parameters 文件」發出的判定，而 overlay
// **不是** parameters 文件（它不會有 industry.classification_tree）。
// 把 overlay 的失敗借用 params.json 的 code，會讓「哪個檔案壞了」在日誌上失真。
// 型別共用（`config.CalibrationFindingCode`）只是為了讓觀測結果有一個穩定的
// machine-readable 欄位；值域各自獨立。
const (
	// CalibrationOverlayStatFailed：overlay 路徑 stat 失敗（權限、父目錄型別錯誤…）。
	CalibrationOverlayStatFailed = config.CalibrationFindingCode("OVERLAY_STAT_FAILED")
	// CalibrationOverlayReadFailed：overlay 存在但讀不到。
	CalibrationOverlayReadFailed = config.CalibrationFindingCode("OVERLAY_READ_FAILED")
	// CalibrationOverlayInvalidJSON：overlay 讀得到但不是合法的 overlay 文件。
	CalibrationOverlayInvalidJSON = config.CalibrationFindingCode("OVERLAY_INVALID_DOCUMENT")
	// CalibrationOverlayAbsent：overlay 檔案不存在。這是**可評估的確定狀態**
	// （不是「不知道」）：在生產它代表「沒有任何 runtime 校準被持久化」——
	// 正是 #2013 要消滅的狀態，所以走 Fresh=false（warning）而不是 run_ok=0（error）。
	CalibrationOverlayAbsent = config.CalibrationFindingCode("OVERLAY_ABSENT")
	// CalibrationOverlayZeroUpdatedAt：文件沒有 `updated_at`（從未由
	// `SaveCalibrationOverlay` 寫過，或被人手動編輯成缺欄位）⇒ 無法判定年齡，
	// 不得當成新鮮（fail-closed）。
	CalibrationOverlayZeroUpdatedAt = config.CalibrationFindingCode("OVERLAY_ZERO_UPDATED_AT")
	// CalibrationOverlayStale：`updated_at` 超過 `CalibrationFreshnessContract`。
	CalibrationOverlayStale = config.CalibrationFindingCode("OVERLAY_STALE")
)

// Drift 指標族。命名沿用 `atlas_calibration_*`。
//
// ⚠️ 這裡刻意**不**用 `_total` 後綴：本套件檔頭已定調「gauge 不加 `_total`」，
// 而這四個都是 gauge（last-write-wins 的**現況**，不是單調累加的事件計數）。
// 用 `_total` 會讓 Prometheus 使用者把它當 counter 做 `rate()`，得到無意義的結果。
const (
	// MetricCalibrationDriftRunOK：「這個比較能不能做」。
	// 1 = 基準與 overlay 都讀得到；0 = 不能（基準讀不到/解析失敗、overlay 解不開）。
	// fail-closed：0 時下面的數值序列是**凍結值**，不代表現在的偏離量。
	MetricCalibrationDriftRunOK = "atlas_calibration_drift_run_ok"
	// MetricCalibrationDriftKeys：**現在**有幾個 tunable 的生效值與 image 內基準不同
	// （`config.CalibrationOverlayReport.DriftedKeys`）。> 0 是 intended 的正常狀態。
	MetricCalibrationDriftKeys = "atlas_calibration_drift_keys"
	// MetricCalibrationDriftOutOfWindowKeys：其中比值落在校準迴圈單步窗
	// （`config.CalibrationOverlaySingleStepWindowMin/Max`）之外的數量。
	// > 0 代表「這個生效值不是單一輪被接受的校準能解釋的」——告警條件之一。
	MetricCalibrationDriftOutOfWindowKeys = "atlas_calibration_drift_out_of_window_keys"
	// MetricCalibrationDriftDroppedKeys：overlay 宣告了但**沒生效**的 entry 數量，
	// 以 `reason` 區分（見 DriftDropReason*）。同樣是「宣告與事實不符」的可見化。
	MetricCalibrationDriftDroppedKeys = "atlas_calibration_drift_dropped_keys"
)

// MetricCalibrationDriftDroppedKeys 的 reason 標籤值。
const (
	// DriftDropReasonSSOTMoved：entry 記錄的 baseline 已與現在的 SSOT 不同 ⇒
	// loader 以「受審查的憲章編輯優先」為由讓它失效（**設計行為**，只呈現不告警）。
	DriftDropReasonSSOTMoved = "ssot_moved"
	// DriftDropReasonNotApplicable：entry 指到不存在的參數/路徑、或值不是可套用的型別
	// ⇒ 它永遠不會生效（**缺陷**：校準的產物被靜默忽略，告警條件之一）。
	DriftDropReasonNotApplicable = "not_applicable"
)

// Drift 觀測本身失敗的 codes（語意同 CalibrationOverlay* 那一組：fail-closed 的原因）。
const (
	// CalibrationDriftBaselineUnreadable：image 內基準 stat 不到、讀不到或解析失敗。
	// 「沒有基準」不等於「沒有偏離」。
	CalibrationDriftBaselineUnreadable = config.CalibrationFindingCode("DRIFT_BASELINE_UNREADABLE")
	// CalibrationDriftOverlayPathUnresolved：連 overlay 的路徑都解析不出來
	// （workDir 為空且製程沒有註冊 overlay）⇒ 沒有東西可以比。
	CalibrationDriftOverlayPathUnresolved = config.CalibrationFindingCode("DRIFT_OVERLAY_PATH_UNRESOLVED")
	// CalibrationDriftOverlayUnreadable：overlay 讀不到或解不開（與 overlay 新鮮度的
	// CalibrationOverlayReadFailed/InvalidJSON 同一個根因，這裡只是 drift 這一側的代碼）。
	CalibrationDriftOverlayUnreadable = config.CalibrationFindingCode("DRIFT_OVERLAY_UNREADABLE")
)

// ObserveCalibrationOverlayFreshness 評估**權威產物**（overlay）的新鮮度並輸出
// gauge 族（artifact=`parameters_overlay`）。
//
// 與 `ObserveCalibrationFreshness` 的關係：同一個 observation 型別、同一組 gauge 名稱、
// 同一個 `CalibrationFreshnessContract`（48h），差別只在「量什麼」：
//   - SSOT 那一條跑 `config.ValidateCalibration`（結構 + mtime + updated_at，CLI 同判）；
//   - 這一條量 overlay 自己的 `updated_at`——overlay 不是 parameters 文件，
//     沒有 classification_tree 可以驗結構，而它的 `updated_at` 由
//     `SaveCalibrationOverlay` 在**每一次**寫入時蓋章（tmp→rename 的原子替換），
//     所以「它的年齡」＝「校準迴圈最後一次交出東西是多久以前」。
//
// `obs.Findings` 一律為 nil（這一條不經 `ValidateCalibration`，沒有結構性 finding）；
// 失敗原因一律看 `UnverifiableCode`（run_ok=0）與 `FreshnessCode`（run_ok=1 但 Fresh=false）。
func ObserveCalibrationOverlayFreshness(collector *MetricsCollector, path string, now time.Time) CalibrationFreshnessObservation {
	obs := CalibrationFreshnessObservation{
		Artifact:  CalibrationArtifactParametersOverlay,
		Path:      path,
		CheckedAt: now,
	}

	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			// 檔案不存在是**確定**狀態（不是「讀不到」）：沒有任何 runtime 校準被持久化。
			obs.RunOK = true
			obs.Fresh = false
			obs.FreshnessCode = CalibrationOverlayAbsent
		} else {
			obs.RunOK = false
			obs.UnverifiableCode = CalibrationOverlayStatFailed
		}
		emitCalibrationOverlayFreshnessGauges(collector, obs)
		return obs
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		obs.RunOK = false
		obs.UnverifiableCode = CalibrationOverlayReadFailed
		emitCalibrationOverlayFreshnessGauges(collector, obs)
		return obs
	}

	// 型別用 config 的權威定義（schema 只有一份），錯誤分類自己來——這樣
	// 「讀不到」與「解不開」在日誌與 metrics 上是兩個不同的 code。
	var ov config.CalibrationOverlay
	if err := json.Unmarshal(raw, &ov); err != nil {
		obs.RunOK = false
		obs.UnverifiableCode = CalibrationOverlayInvalidJSON
		emitCalibrationOverlayFreshnessGauges(collector, obs)
		return obs
	}

	obs.RunOK = true
	obs.Fresh = true
	if ov.UpdatedAt.IsZero() {
		// 沒有 updated_at ⇒ 沒有可信年齡。與 SSOT 的 UPDATED_AT_ZERO 同語意：
		// 可評估（run_ok=1）但不新鮮。
		obs.Fresh = false
		obs.FreshnessCode = CalibrationOverlayZeroUpdatedAt
		emitCalibrationOverlayFreshnessGauges(collector, obs)
		return obs
	}

	obs.LastCalibrated = ov.UpdatedAt
	age := now.Sub(ov.UpdatedAt)
	if age < 0 {
		// 產物自述的時間落在未來（時鐘偏移或人為編輯）：與既有
		// computeChannelStalenessSeconds / observeCalibrationFreshness 的處置一致，夾到 0。
		age = 0
	}
	obs.AgeSeconds = age.Seconds()
	obs.HaveAge = true
	if age > CalibrationFreshnessContract {
		obs.Fresh = false
		obs.FreshnessCode = CalibrationOverlayStale
	}

	emitCalibrationOverlayFreshnessGauges(collector, obs)
	return obs
}

// emitCalibrationOverlayFreshnessGauges 輸出 overlay 這一組。
//
// 與 `emitCalibrationFreshnessGauges` 的唯一差別：**不輸出探針心跳**
// （`atlas_calibration_freshness_checked_timestamp_seconds`）。心跳回答的是
// 「這個匯出任務有沒有在跑」，那是**任務級**的事實，不是產物級的：兩個 artifact
// 由同一次函式呼叫、同一個任務寫入，不可能一個停一個跑。若給每個 artifact 各一份
// 心跳，規則 3（`CalibrationFreshnessExporterDown`）的
// `time() - checked > 1800` 就會同時匹配兩條序列、對同一個根因發兩次通知。
// 因此心跳維持單一序列（artifact="parameters"），本函式只輸出產物級的量。
func emitCalibrationOverlayFreshnessGauges(collector *MetricsCollector, obs CalibrationFreshnessObservation) {
	if collector == nil {
		return
	}
	labels := map[string]string{"artifact": obs.Artifact}
	collector.RecordGauge(MetricCalibrationFreshnessRunOK, boolGauge(obs.RunOK), labels)
	collector.RecordGauge(MetricCalibrationFreshnessOK, boolGauge(obs.Fresh), labels)
	if !obs.RunOK {
		return
	}
	lastCalibrated := float64(0) // 0 = 從未記錄（明確哨兵值）
	if !obs.LastCalibrated.IsZero() {
		lastCalibrated = float64(obs.LastCalibrated.Unix())
	}
	collector.RecordGauge(MetricCalibrationLastCalibratedTimestamp, lastCalibrated, labels)
	if obs.HaveAge {
		collector.RecordGauge(MetricCalibrationFreshnessAgeSeconds, obs.AgeSeconds, labels)
	}
}

// CalibrationDriftObservation 是 image-vs-effective 比較的結果，回傳給呼叫端
// 做日誌與測試斷言（測試不需要去 scrape /metrics）。
type CalibrationDriftObservation struct {
	Artifact string
	// BaselinePath 是 image 內基準（`configs/parameters.json`）。
	BaselinePath string
	// OverlayPath 是被拿來疊加的 overlay。
	OverlayPath string
	CheckedAt   time.Time
	// RunOK 為 false 代表這個比較**沒有完成**（此時所有 key 清單都必須當成未知，
	// 不得讀成 0）。
	RunOK bool
	// UnverifiableCode 是導致 RunOK=false 的原因（可比較時為空字串）。
	UnverifiableCode config.CalibrationFindingCode
	// DriftedKeys 是生效值與基準不同的 key（>= 0 是正常的；見檔頭的政策段）。
	DriftedKeys []string
	// OutOfWindowKeys 是 DriftedKeys 中比值超出單步窗的那些（異常）。
	OutOfWindowKeys []string
	// InvalidatedKeys 是「基準已移動因此不生效」的 key（設計行為，只呈現）。
	InvalidatedKeys []string
	// UnknownKeys 是「宣告了但套不上」的 key（缺陷，告警條件之一）。
	UnknownKeys []string
}

// ObserveCalibrationDrift 比較 image 內基準與 overlay 疊加後的生效值，並輸出
// `atlas_calibration_drift_*` 族（artifact=`parameters_overlay`）。
//
// 實作刻意**重用** `config.InspectCalibratedOverlayLayer`——也就是 runtime 載入
// 用的同一套判定（哪些 entry 生效、哪些失效、哪些套不上）。監控不會有第二套語意，
// 而且那個入口保證唯讀（監控任務不得寫檔：一個 read-modify-write 循環會與校準
// 寫入者的 merge 競爭）。
func ObserveCalibrationDrift(collector *MetricsCollector, baselinePath, overlayPath string, now time.Time) CalibrationDriftObservation {
	return observeCalibrationDrift(collector, CalibrationArtifactParametersOverlay, baselinePath, overlayPath, now)
}

// observeCalibrationDrift 是可注入 artifact 的實作，給測試用。
func observeCalibrationDrift(collector *MetricsCollector, artifact, baselinePath, overlayPath string, now time.Time) CalibrationDriftObservation {
	obs := CalibrationDriftObservation{
		Artifact:     artifact,
		BaselinePath: baselinePath,
		OverlayPath:  overlayPath,
		CheckedAt:    now,
	}

	if overlayPath == "" {
		obs.UnverifiableCode = CalibrationDriftOverlayPathUnresolved
		emitCalibrationDriftGauges(collector, obs)
		return obs
	}
	// 基準必須是一個**存在**的檔案/目錄：`LoadParametersSource` 對不存在的路徑
	// 會靜默回退到內建預設值，那不是「受審查的基準」，拿它當比較對象就是假造基準。
	if _, err := os.Stat(baselinePath); err != nil {
		obs.UnverifiableCode = CalibrationDriftBaselineUnreadable
		emitCalibrationDriftGauges(collector, obs)
		return obs
	}
	src, err := config.LoadParametersSource(baselinePath)
	if err != nil || src == nil || src.Config == nil {
		obs.UnverifiableCode = CalibrationDriftBaselineUnreadable
		emitCalibrationDriftGauges(collector, obs)
		return obs
	}

	// 唯讀分類：與 runtime 相同的判定，但不會刪 entry、不會回寫 overlay。
	_, report := config.InspectCalibratedOverlayLayer(src.Config, src.Raw, overlayPath)
	if report.Err != nil {
		// 讀不到或解不開：這是「量不到」（run_ok=0），不是「沒有偏離」（0）。
		obs.UnverifiableCode = CalibrationDriftOverlayUnreadable
		emitCalibrationDriftGauges(collector, obs)
		return obs
	}

	obs.RunOK = true
	obs.DriftedKeys = report.DriftedKeys()
	for _, d := range report.Applied {
		if config.RatioOutsideSingleStepWindow(d.Ratio) {
			obs.OutOfWindowKeys = append(obs.OutOfWindowKeys, d.Key)
		}
	}
	obs.InvalidatedKeys = append(obs.InvalidatedKeys, report.Invalidated...)
	obs.UnknownKeys = append(obs.UnknownKeys, report.Unknown...)
	emitCalibrationDriftGauges(collector, obs)
	return obs
}

// emitCalibrationDriftGauges 輸出整族。
//
// 每一輪無條件輸出 `_run_ok`（見 `calibration_freshness.go` 檔頭約束 (a)/(d)）；
// 數值序列只在可以比較時輸出——無法比較時硬輸出 0 就是把「不知道」寫成
// 「沒有偏離」（false-green）。此時序列凍結在最後一次可比較的值，判讀順序
// 與既有那一組相同：**先看 run_ok，再看值**。
func emitCalibrationDriftGauges(collector *MetricsCollector, obs CalibrationDriftObservation) {
	if collector == nil {
		return
	}
	labels := map[string]string{"artifact": obs.Artifact}
	collector.RecordGauge(MetricCalibrationDriftRunOK, boolGauge(obs.RunOK), labels)
	if !obs.RunOK {
		return
	}
	collector.RecordGauge(MetricCalibrationDriftKeys, float64(len(obs.DriftedKeys)), labels)
	collector.RecordGauge(MetricCalibrationDriftOutOfWindowKeys, float64(len(obs.OutOfWindowKeys)), labels)
	// 兩個 reason 每一輪都要輸出（即使值為 0）：只輸出非零的那一個會讓另一個序列
	// 在「第一次出現」之前不存在，規則的 `> 0` 就會有一段看不到的窗。
	collector.RecordGauge(MetricCalibrationDriftDroppedKeys, float64(len(obs.InvalidatedKeys)), withLabel(labels, "reason", DriftDropReasonSSOTMoved))
	collector.RecordGauge(MetricCalibrationDriftDroppedKeys, float64(len(obs.UnknownKeys)), withLabel(labels, "reason", DriftDropReasonNotApplicable))
}

// withLabel 回傳 labels 的副本再加上一組 key/value（不修改輸入的 map：
// 觀測結果的 labels 會被同一輪的其他輸出重用）。
func withLabel(labels map[string]string, key, value string) map[string]string {
	out := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		out[k] = v
	}
	out[key] = value
	return out
}
