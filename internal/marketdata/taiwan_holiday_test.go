package marketdata

import (
	"testing"
	"time"
)

// B05：台灣國定假日判定測試。假日表與 internal/industry/event_calendar.go
// 的 taiwanPublicHolidays 對齊（2023-2030 lunar 表）— 更新一側需同步另一側。

func TestIsTaiwanHoliday(t *testing.T) {
	cases := []struct {
		name string
		date time.Time
		want bool
	}{
		// 固定日期假日
		{"元旦 2026", time.Date(2026, 1, 1, 0, 0, 0, 0, twseLocation), true},
		{"228 和平紀念日 2026", time.Date(2026, 2, 28, 0, 0, 0, 0, twseLocation), true},
		{"勞動節 2026", time.Date(2026, 5, 1, 0, 0, 0, 0, twseLocation), true},
		{"國慶日 2026", time.Date(2026, 10, 10, 0, 0, 0, 0, twseLocation), true},
		// 農曆/節氣假日（2026 年）
		{"春節初一 2026", time.Date(2026, 2, 17, 0, 0, 0, 0, twseLocation), true},
		{"清明節 2026", time.Date(2026, 4, 5, 0, 0, 0, 0, twseLocation), true},
		{"端午節 2026", time.Date(2026, 6, 19, 0, 0, 0, 0, twseLocation), true},
		{"中秋節 2026", time.Date(2026, 9, 25, 0, 0, 0, 0, twseLocation), true},
		// 一般交易日（非假日）
		{"一般週三 2026-07-01", time.Date(2026, 7, 1, 0, 0, 0, 0, twseLocation), false},
		{"假日隔日 2026-06-22", time.Date(2026, 6, 22, 0, 0, 0, 0, twseLocation), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTaiwanHoliday(tc.date); got != tc.want {
				t.Errorf("isTaiwanHoliday(%v) = %v, want %v", tc.date, got, tc.want)
			}
		})
	}
}

func TestIsTaiwanTradingDay_Holidays(t *testing.T) {
	// 週末 + 假日都不是交易日；一般週五是交易日。
	cases := []struct {
		name string
		date time.Time
		want bool
	}{
		{"週六", time.Date(2026, 7, 4, 0, 0, 0, 0, twseLocation), false},
		{"週日", time.Date(2026, 7, 5, 0, 0, 0, 0, twseLocation), false},
		{"春節初一 2026（平日）", time.Date(2026, 2, 17, 0, 0, 0, 0, twseLocation), false},
		{"國慶日 2026（週六）", time.Date(2026, 10, 10, 0, 0, 0, 0, twseLocation), false},
		{"一般週五 2026-07-10", time.Date(2026, 7, 10, 0, 0, 0, 0, twseLocation), true},
		{"228補假 2026-02-27", time.Date(2026, 2, 27, 0, 0, 0, 0, twseLocation), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTaiwanTradingDay(tc.date); got != tc.want {
				t.Errorf("isTaiwanTradingDay(%v) = %v, want %v", tc.date, got, tc.want)
			}
		})
	}
}
