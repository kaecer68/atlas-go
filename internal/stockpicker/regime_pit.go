package stockpicker

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// TWSE 收盤時間（Asia/Taipei）。台股正規盤 13:30 為官方收盤。
const (
	twseCloseHour   = 13
	twseCloseMinute = 30
)

// taipeiZone 回傳 Asia/Taipei；以固定 +08:00 為後備（scratch 容器無 tzdata 時
// LoadLocation 會失敗，而台灣全年無 DST ⇒ 固定位移是正確的）。
func taipeiZone() *time.Location {
	if loc, err := time.LoadLocation("Asia/Taipei"); err == nil {
		return loc
	}
	return time.FixedZone("UTC+8", 8*3600)
}

// PITRegimeBySession 把 regime 快照對應到「它**真正可用**的交易日」。
//
// 為什麼需要（FU-20261012-02）：快照的擷取時間在**該交易日收盤之後**（美股
// session 的資料在台北時間隔日凌晨才落地——實測 2026-09-23 的快照擷取於
// 2026-09-24 02:59–07:55 TPE，而 outcome 的進場價是 09-23 13:30 TPE 收盤），
// 因此**不能**用快照自己的 date 當標籤，否則該欄位會帶有「1 個美股 session 的
// 未來資訊」——它曾是 stock_signal_outcomes 唯一非 PIT 的欄位。
//
// 規則：一列快照可用於「**第一個收盤時間嚴格晚於其 capturedAt 的交易日**」；
// 同一交易日多筆時以 capturedAt 最新者為準（與原實作一致）。
// 非交易日（週末/假日）會被跳過；擷取後超過 maxRollForwardDays 天仍找不到
// 交易日則該列不參與（寧可缺標籤，不回填推測）。
func PITRegimeBySession(rows []ledger.RegimeRow) map[string]string {
	const maxRollForwardDays = 30
	tpe := taipeiZone()
	out := make(map[string]string, len(rows))
	best := make(map[string]time.Time, len(rows))

	for _, row := range rows {
		if row.Regime == "" || row.CapturedAt.IsZero() {
			continue
		}
		// 只採 live 快照（is_synthetic = 0）。回填列
		// （cmd/backfill-period-history）不得標註任何 session，理由有兩個且互相獨立：
		//   1. 它用**另一套公式**（legacy VIX/DXY/US10Y）⇒ 同一天可能給出與 live
		//      相反的標籤（2026-09-23：live 7.81 RISK_ON vs 回填 36.68 NEUTRAL）。
		//   2. 它的 CapturedAt 是**回填執行當下**，不是該日的事實時間 ⇒ 對過去的
		//      session 而言天生非 PIT。
		// 回填列仍留在表內供 period 分析；這裡只是不讓它們當 outcome 標籤。
		if row.IsSynthetic != 0 {
			continue
		}
		captured := row.CapturedAt.In(tpe)
		day := time.Date(captured.Year(), captured.Month(), captured.Day(), 0, 0, 0, 0, tpe)
		closeToday := time.Date(day.Year(), day.Month(), day.Day(), twseCloseHour, twseCloseMinute, 0, 0, tpe)

		// 當天收盤是否仍晚於擷取時間？是 ⇒ 當天即為第一個可用交易日。
		candidate := day
		if !closeToday.After(captured) {
			candidate = day.AddDate(0, 0, 1)
		}
		found := false
		for i := 0; i < maxRollForwardDays; i++ {
			if taiwanholidays.IsTradingDay(candidate) {
				found = true
				break
			}
			candidate = candidate.AddDate(0, 0, 1)
		}
		if !found {
			continue
		}
		key := candidate.Format("2006-01-02")
		if ts, ok := best[key]; ok && !captured.After(ts) {
			continue // 同交易日已有更新（或同時）的快照
		}
		best[key] = captured
		out[key] = row.Regime
	}
	return out
}
