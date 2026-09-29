package marketdata

import (
	"testing"
	"time"
)

// TestTradingDaysBetween 是 #2145 的形狀表：**(from, to] 的交易日數**。
//
// 為什麼端點約定非對稱：replay 轉檔的穩態是「CSV 有今日交易日、JSONL 有上一個資料日」
// ⇒ `to`（CSV 最新資料日）是**下一輪才會轉**的那一天，必須算進去（穩態＝1，不是 0），
// `from`（JSONL 最新日）已經轉過，故不含。
//
// 期望值一律由**權威** `taiwanholidays.IsTradingDay` 決定，這裡逐一註明該跳過哪些非交易日
// （2026-09-25 中秋節、2026-09-28 調整放假、2027-01-01 元旦 ⇒ 皆由 HolidayTables 提供）。
func TestTradingDaysBetween(t *testing.T) {
	d := func(s string) time.Time {
		v, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("parse %s: %v", s, err)
		}
		return v
	}
	cases := []struct {
		name     string
		from, to string
		want     int
		why      string
	}{
		{"同一天", "2026-09-29", "2026-09-29", 0, "(from, to] 為空"},
		{"端點皆非交易日", "2026-09-25", "2026-09-28", 0, "09-25 中秋節、09-28 調整放假、中間為週末"},
		{"長連假且已跟上（本次誤報形狀）", "2026-09-24", "2026-09-29", 1, "09-25／09-28 皆休市；只算 09-29"},
		{"CSV 端點本身是假日", "2026-09-24", "2026-09-25", 0, "09-25 中秋節 ⇒ 不算"},
		{"單一交易日落後", "2026-09-23", "2026-09-25", 1, "只算 09-24"},
		{"兩個交易日落後", "2026-09-22", "2026-09-25", 2, "算 09-23、09-24"},
		{"跨年（非平凡）", "2026-12-30", "2027-01-04", 2, "12-31 交易日 ＋ 01-04 交易日；01-01 元旦、01-02/03 週末"},
		{"跨年（端點皆非交易日）", "2026-12-31", "2027-01-02", 0, "01-01 元旦、01-02 週六"},
		{"from 晚於 to", "2026-09-29", "2026-09-01", 0, "反向 ⇒ 0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := TradingDaysBetween(d(c.from), d(c.to))
			if got != c.want {
				t.Fatalf("TradingDaysBetween(%s, %s) = %d, want %d（%s）", c.from, c.to, got, c.want, c.why)
			}
		})
	}
}

// TestTradingDaysBetween_UsesTaipeiCalendarDay 釘住「以台北日曆日判定」：
// 這一對時間戳在 **UTC** 日曆下是 (10-01 週四, 10-02 週五] ⇒ 1（10-02 是交易日），
// 在 **Asia/Taipei** 下是 (10-02 週五, 10-03 週六] ⇒ **0**（10-03 是週六）⇒ 必須回 0。
// （生產 cron 容器不設 TZ ⇒ time.Now() 是 UTC；此測試防止有人改成 UTC 比較。）
func TestTradingDaysBetween_UsesTaipeiCalendarDay(t *testing.T) {
	from := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC) // 台北 10-02 04:00（週五）
	to := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)   // 台北 10-03 04:00（週六）
	if got := TradingDaysBetween(from, to); got != 0 {
		t.Fatalf("TradingDaysBetween over the UTC/Taipei boundary = %d, want 0 (Taipei calendar: (10-02 Fri, 10-03 Sat])", got)
	}
}

// TestTradingDaysBetween_ScanIsBounded 病態輸入不得無限迴圈：掃描上限 maxTradingDayScan 天，
// 回傳的是「該窗口內的交易日數」（仍遠大於任何使用它的門檻）。
func TestTradingDaysBetween_ScanIsBounded(t *testing.T) {
	got := TradingDaysBetween(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC))
	if got <= 0 || got > maxTradingDayScan {
		t.Fatalf("bounded scan = %d, want 0 < n <= %d", got, maxTradingDayScan)
	}
}
