package apigateway

import (
	"testing"
	"time"
)

// TestExpectedChannelDataDate_TradingDayAnchored — F58 (2026-10-04 週末假陽性).
//
// 契約（見 ChannelContract.PublishCalendar）：上游只在交易日發布時，新鮮度
// 必須以「有沒有發布機會」判定，而不是牆鐘窗口。本測試把 2026-10-04 的實損
// 形狀（週日晚上、最後成功 10-02T07:20Z）與三個對照點寫成可執行規格。
func TestExpectedChannelDataDate_TradingDayAnchored(t *testing.T) {
	tpe := time.FixedZone("CST", 8*3600)
	midnight := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, tpe)
	}
	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			// 週六：上一個交易日仍是週五 ⇒ 週五的資料就是最新可得。
			name: "週六 2026-10-03 12:00 台北",
			now:  time.Date(2026, 10, 3, 12, 0, 0, 0, tpe),
			want: midnight(2026, 10, 2),
		},
		{
			// 週日（實損時刻）：期望值仍停在週五。
			name: "週日 2026-10-04 22:28 台北",
			now:  time.Date(2026, 10, 4, 22, 28, 0, 0, tpe),
			want: midnight(2026, 10, 2),
		},
		{
			// 交易日 18:00 前：當日資料還沒被要求 ⇒ 期望值仍是前一交易日。
			name: "週一 2026-10-05 10:30 台北（cutoff 前）",
			now:  time.Date(2026, 10, 5, 10, 30, 0, 0, tpe),
			want: midnight(2026, 10, 2),
		},
		{
			// 交易日 18:00 起：要求當日。
			name: "週一 2026-10-05 18:30 台北（cutoff 後）",
			now:  time.Date(2026, 10, 5, 18, 30, 0, 0, tpe),
			want: midnight(2026, 10, 5),
		},
		{
			// 假日（2026-10-09 國慶補假，週五）：不是交易日 ⇒ 期望值回 10-08。
			name: "假日 2026-10-09 20:00 台北",
			now:  time.Date(2026, 10, 9, 20, 0, 0, 0, tpe),
			want: midnight(2026, 10, 8),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExpectedChannelDataDate(tc.now)
			if !got.Equal(tc.want) {
				t.Fatalf("ExpectedChannelDataDate = %s, want %s", got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

// TestStalenessOverageSeconds_TradingDayChannelsWeekendFalsePositive —
// F58 核心驗收：同一個真實記錄（2026-10-02T07:20Z 最後成功）在週日晚上
// **不得**產生 overage（＝不得 firing ChannelDataStale），但同一筆記錄在
// 週一 18:00 後（真的漏一個交易日的發布）**必須**產生 overage。
//
// 反向對照（同時斷言）：同一筆記錄在「日曆無關」的預設契約下 overage > 0
// ⇒ 證明原本的假陽性確實來自牆鐘窗口，而不是資料真的舊。
func TestStalenessOverageSeconds_TradingDayChannelsWeekendFalsePositive(t *testing.T) {
	// 生產實證（F58 已知事實）：twse_sbl last_success 10-02T07:20Z、
	// government_broker last_success 10-02T07:18Z（皆台北週五 15:2x）。
	lastSuccess := time.Date(2026, 10, 2, 7, 20, 0, 0, time.UTC)
	sundayNight := time.Date(2026, 10, 4, 14, 28, 0, 0, time.UTC) // 台北週日 22:28

	staleSec := sundayNight.Sub(lastSuccess).Seconds()
	if staleSec <= StaleDataThreshold.Seconds() {
		t.Fatalf("fixture 不成立: age=%s 必須已超過預設窗口 %s（否則測不到假陽性）",
			time.Duration(staleSec)*time.Second, StaleDataThreshold)
	}

	for _, id := range []string{"twse_sbl", "government_broker"} {
		c := ChannelContracts().Contract(id)
		if c.PublishCalendar != PublishCalendarTWTradingDay {
			t.Fatalf("%s 的契約必須宣告 PublishCalendarTWTradingDay（實際 %q）", id, c.PublishCalendar)
		}
		if got := c.StalenessOverageSeconds(staleSec, lastSuccess, sundayNight); got != 0 {
			t.Errorf("%s 週日晚上 overage = %.0fs, want 0（通道正常，上游週末無發布機會）", id, got)
		}
		// 同一筆記錄、同一個 now，但走「日曆無關」的預設契約 ⇒ 會 firing。
		daily := DefaultChannelContract(id)
		if got := daily.StalenessOverageSeconds(staleSec, lastSuccess, sundayNight); got <= 0 {
			t.Errorf("%s 以預設（牆鐘）契約 overage = %.0fs, want > 0 —— 反向對照不成立", id, got)
		}
	}

	// 週一 18:30 台北（cutoff 後）仍未見 10-05 的資料 ⇒ 真的漏了一次發布。
	mondayEvening := time.Date(2026, 10, 5, 10, 30, 0, 0, time.UTC) // 台北 18:30
	staleness := mondayEvening.Sub(lastSuccess).Seconds()
	got := ChannelContracts().Contract("twse_sbl").StalenessOverageSeconds(staleness, lastSuccess, mondayEvening)
	want := mondayEvening.Sub(time.Date(2026, 10, 5, 18, 0, 0, 0, time.FixedZone("CST", 8*3600))).Seconds()
	if got != want {
		t.Fatalf("週一 18:30 overage = %.0fs, want %.0fs（＝距當日發布截止 18:00 的秒數）", got, want)
	}

	// 週一 18:30 但資料已是當日（正常情況）⇒ 0。
	fresh := time.Date(2026, 10, 5, 7, 40, 0, 0, time.UTC) // 台北 15:40
	if got := ChannelContracts().Contract("twse_sbl").StalenessOverageSeconds(
		mondayEvening.Sub(fresh).Seconds(), fresh, mondayEvening); got != 0 {
		t.Errorf("當日資料已落地時 overage = %.0fs, want 0", got)
	}
}

// TestStalenessOverageSeconds_DefaultCalendarUnchanged —
// 未宣告 PublishCalendar 的通道必須完全保持原語意（staleSec - 有效窗口，
// 夾到 0），否則這個欄位就成了全體通道的靜默放寬。
func TestStalenessOverageSeconds_DefaultCalendarUnchanged(t *testing.T) {
	now := time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		id    string
		stale time.Duration
		want  float64
	}{
		// 契約窗 72h（twse_replay）：6 天 ⇒ overage = 6d - 72h。
		{name: "twse_replay 6d", id: "twse_replay", stale: 6 * 24 * time.Hour, want: (6 * 24 * time.Hour).Seconds() - (72 * time.Hour).Seconds()},
		// 契約窗 8d（tdcc 週快照）：3 天 ⇒ 0。
		{name: "tdcc 3d", id: "tdcc_equity_dispersion", stale: 3 * 24 * time.Hour, want: 0},
		// 預設 48h：27h ⇒ 0。
		{name: "fugle 27h", id: "fugle", stale: 27 * time.Hour, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := ChannelContracts().Contract(tc.id)
			if c.PublishCalendar != PublishCalendarDaily {
				t.Fatalf("%s 不該宣告交易日曆（實際 %q）", tc.id, c.PublishCalendar)
			}
			if got := c.StalenessOverageSeconds(tc.stale.Seconds(), now.Add(-tc.stale), now); got != tc.want {
				t.Fatalf("overage = %.0fs, want %.0fs", got, tc.want)
			}
		})
	}
}
