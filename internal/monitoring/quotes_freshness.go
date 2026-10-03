package monitoring

// Package monitoring — quotes 新鮮度指標（F54 phase 1，2026-10-03）。
//
// 為什麼存在
// ----------
// 2026-10-02 實損：quotes 沒有任何排程結構性保障（無 cron、無契約檢查），
// 當日 universe build 只有 **2/117** 個標的有 quotes 卻無人發現；週一 10-06
// 早晨的 universe build 是下個風險點。本檔把 `quotes.max(date)` 變成
// Prometheus 序列，讓既有監控（scrape + 規則 + Alertmanager）可以看見
// 「quotes 落後」，與 calibration_freshness.go 同一條管線。
//
// 設計約束（沿用 calibration_freshness 的紀律）
// ------------------------------------------
//  (a) gauge 每輪無條件輸出 `_ok` / `_run_ok` / `_checked_timestamp_seconds`
//      ⇒ 啟動後數十秒內序列就存在，沒有「第一次 increment 才建 series」的
//      缺席視窗；「檢查沒在跑」由規則檔的 absent/心跳規則負責。
//  (b) fail-closed：查詢失敗時 `_ok` = 0 且 `_run_ok` = 0（未知不得當新鮮），
//      由專門的 error 規則負責，與「真的舊」的 warning 規則互不重複 paging。
//  (c) 交易日語意**在 Go 側判定**（重用 internal/taiwanholidays），
//      PromQL 只讀 `_ok` ⇒ 規則簡單、不會在 PromQL 裡重寫第二套交易日曆。
//      成本：taiwanholidays 是純記憶體查表（2021–2040 實測資料），
//      每 5 分鐘一次 O(1) 查表，可忽略。
//
// 判定（ExpectedQuoteDate）
// -----------------------
//   · now（Asia/Taipei）落在**交易日**且小時 >= QuotesFreshnessCutoffHour(18)
//     ⇒ 期望今天的 quotes（收盤 13:45 + backfill 餘量後理應已落地）。
//   · 其他情況（非交易日、或交易日 18:00 前）⇒ 期望「今天之前最近一個交易日」
//     的 quotes。這讓「週一早晨 universe build」場景（週一 08:00 檢查，
//     期望週五的 quotes）被覆蓋：週五 quotes 缺席 ⇒ 週一 00:00 起即不新鮮，
//     在 universe build 之前就觸發。
//   · 硬護欄：max(date) 比今天（日曆日）舊超過 quotesHardStalenessWindow(7d)
//     ⇒ 無論交易日判定如何都不新鮮（防假日表缺新假日時 PreviousTradingDay
//      walks 回一個其實沒資料的日期而永遠誤報「新鮮」之外，也防任何
//     「該有資料卻整週沒有」的沉默）。
//
// ⚠️ 已知的假陽性形狀（誠實聲明，見規則檔 annotation 的 known_issue）：
// backfill（auto_quote_backfill，24h interval）在交易日 18:00 後才落地時，
// 18:00–落地時刻之間會短暫 `_ok`=0；規則 `for: 1h` 吸收單輪抖動。
// 連續假日（如春休 5+ 天）期間期望值停留在假日前最後交易日，`_ok` 維持 1。

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// Quotes 新鮮度 gauge 族（Prometheus 命名慣例：gauge 不加 _total）。
const (
	// MetricQuotesFreshnessOK 是「quotes 新鮮嗎」的判定值（交易日感知）：
	// 1 = max(date) >= 期望交易日；0 = 其他一切情況（落後 / 空表 /
	// **查詢失敗**，fail-closed）。
	MetricQuotesFreshnessOK = "atlas_quotes_freshness_ok"
	// MetricQuotesFreshnessRunOK 是「max(date) 查詢本身成功嗎」：
	// 1 = 查得到（含空表回 zero time）；0 = 查詢/解析失敗。
	MetricQuotesFreshnessRunOK = "atlas_quotes_freshness_run_ok"
	// MetricQuotesMaxDateTimestamp 是 max(date) 的 Unix 秒（該日期
	// Asia/Taipei 午夜）。只在查詢成功且表非空時輸出；供儀表板/值班判讀，
	// **沒有任何規則拿它做判定**（判定一律看 _ok，凍結值不會製造誤報）。
	MetricQuotesMaxDateTimestamp = "atlas_quotes_max_date_timestamp_seconds"
	// MetricQuotesFreshnessCheckedTimestamp 是探針心跳（這個檢查最後一次執行）。
	MetricQuotesFreshnessCheckedTimestamp = "atlas_quotes_freshness_checked_timestamp_seconds"
)

const (
	// QuotesFreshnessCutoffHour 是「交易日幾點起要求當日 quotes 已落地」
	// （Asia/Taipei）。收盤 13:45；18:00 給 backfill / 延遲資料源數小時餘量。
	// 這是**簡化的固定門檻**（不是 backfill 的實際落地時刻觀測），
	// 規則 annotation 已標 known_issue。
	QuotesFreshnessCutoffHour = 18

	// quotesHardStalenessWindow 是「max(date) 比今天舊超過這個日曆窗就
	// 一定不新鮮」的硬護欄（防假日表缺口造成永遠新鮮的誤判）。
	quotesHardStalenessWindow = 7 * 24 * time.Hour
)

// QuotesFreshnessObservation 是觀察結果，回傳給呼叫端做日誌與測試斷言
// （測試不需要 scrape /metrics 就能驗證判定）。
type QuotesFreshnessObservation struct {
	CheckedAt time.Time
	// RunOK：max(date) 查詢本身成功（false = 查詢/解析失敗）。
	RunOK bool
	// Fresh：交易日感知判定（RunOK=false 時恒為 false，fail-closed）。
	Fresh bool
	// Reason 是判定理由碼（fresh / max_date_before_expected /
	// quotes_table_empty / query_failed），供日誌與測試。
	Reason string
	// MaxDate 是查到的 max(date)（zero = 表空或查詢失敗）。
	MaxDate time.Time
	// ExpectedDate 是本次判定所要求的交易日（Asia/Taipei 日曆日午夜）。
	ExpectedDate time.Time
	// AgeSeconds / HaveAge：查詢成功且表非空才會有值。
	AgeSeconds float64
	HaveAge    bool
}

// taipeiLocation 解析 Asia/Taipei；失敗時回退固定 +8（與 cmd/atlas
// backfill_tasks.go 的既有慣例相同）。
func taipeiLocation() *time.Location {
	if tz, err := time.LoadLocation("Asia/Taipei"); err == nil {
		return tz
	}
	return time.FixedZone("CST", 8*3600)
}

// ExpectedQuoteDate 計算「now 這一刻，quotes 至少要有哪個交易日的資料」。
// 時鐘以 Asia/Taipei 的**日曆日**為準（quotes.date 是台股交易日，非時區瞬時）。
// 匯出供 cmd 任務日誌與測試使用；判定本身在 ObserveQuotesFreshness 內完成。
func ExpectedQuoteDate(now time.Time) time.Time {
	taipei := taipeiLocation()
	t := now.In(taipei)
	today := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, taipei)
	if taiwanholidays.IsTradingDay(today) && t.Hour() >= QuotesFreshnessCutoffHour {
		return today
	}
	// 非交易日，或交易日 cutoff 之前：要求「今天之前最近一個交易日」。
	return taiwanholidays.PreviousTradingDay(today, 1)
}

// ObserveQuotesFreshness 以交易日感知契約評估 quotes.max(date) 並輸出
// gauge 族。collector 為 nil 時只做評估、不輸出（nil 安全慣例，同
// calibration_freshness）。
//
// maxDate 為 zero 且 queryErr 為 nil 代表「表是空的」（可觀測狀態：
// 判定不新鮮，Reason=quotes_table_empty，但 RunOK=true）。
// 本函式不回傳 error：查詢失敗本身就是要被觀測的狀態。
func ObserveQuotesFreshness(collector *MetricsCollector, maxDate time.Time, queryErr error, now time.Time) QuotesFreshnessObservation {
	obs := QuotesFreshnessObservation{CheckedAt: now, ExpectedDate: ExpectedQuoteDate(now)}

	switch {
	case queryErr != nil:
		obs.RunOK = false
		obs.Fresh = false
		obs.Reason = "query_failed"
	case maxDate.IsZero():
		obs.RunOK = true
		obs.Fresh = false
		obs.Reason = "quotes_table_empty"
	default:
		obs.RunOK = true
		obs.MaxDate = maxDate
		obs.HaveAge = true
		obs.AgeSeconds = now.Sub(maxDate).Seconds()
		// 硬護欄：max(date) 比今天（Taipei 日曆日）舊超過 7 天 ⇒ 無論
		// 交易日判定如何都不新鮮。
		today := now.In(taipeiLocation())
		tooOld := today.AddDate(0, 0, -7).After(maxDate)
		if !maxDate.Before(obs.ExpectedDate) && !tooOld {
			obs.Fresh = true
			obs.Reason = "fresh"
		} else {
			obs.Fresh = false
			obs.Reason = "max_date_before_expected"
		}
	}

	emitQuotesFreshnessGauges(collector, now, obs)
	return obs
}

// emitQuotesFreshnessGauges 每一輪都覆寫狀態序列（last-write-wins，不清
// 序列 ⇒ 條件消失必須主動跳回 1，否則舊樣本永久凍結）。
func emitQuotesFreshnessGauges(collector *MetricsCollector, now time.Time, obs QuotesFreshnessObservation) {
	if collector == nil {
		return
	}
	nowUnix := float64(now.Unix())
	runOK, fresh := 0.0, 0.0
	if obs.RunOK {
		runOK = 1
	}
	if obs.Fresh {
		fresh = 1
	}
	collector.RecordGauge(MetricQuotesFreshnessRunOK, runOK, nil)
	collector.RecordGauge(MetricQuotesFreshnessOK, fresh, nil)
	collector.RecordGauge(MetricQuotesFreshnessCheckedTimestamp, nowUnix, nil)
	// max_date 只在「查得到且表非空」時輸出；查詢失敗後留下的是**凍結值**
	// （gauge 不清序列），判讀順序：先看 _run_ok（同 calibration_freshness
	// 檔頭「凍結樣本的語意」）。
	if obs.RunOK && obs.HaveAge {
		taipei := taipeiLocation()
		d := obs.MaxDate.In(taipei)
		midnight := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, taipei)
		collector.RecordGauge(MetricQuotesMaxDateTimestamp, float64(midnight.Unix()), nil)
	}
}
