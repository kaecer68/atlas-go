package stockpicker

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// 以下測試把「快照擷取時間 → 可標註 session」的規則釘死（FU-20261012-02）。
// 錨點用生產實際數字：2026-09-23 的 regime 快照擷取於 2026-09-24 07:50 TPE
// （美股 session 資料隔日凌晨落地），而 outcome 進場價是 09-23 13:30 TPE 收盤。

func mustTradingDay(t *testing.T, iso string) {
	t.Helper()
	d, err := time.Parse("2006-01-02", iso)
	if err != nil {
		t.Fatalf("parse %s: %v", iso, err)
	}
	if !taiwanholidays.IsTradingDay(d) {
		t.Fatalf("測試前提不成立：%s 不是交易日", iso)
	}
}

func TestPITRegimeBySession_CaptureAfterCloseDoesNotLabelThatSession(t *testing.T) {
	mustTradingDay(t, "2026-09-23")
	tpe := taipeiZone()
	captured := time.Date(2026, 9, 24, 7, 50, 0, 0, tpe) // 生產實測：09-24 07:50 TPE
	got := PITRegimeBySession([]ledger.RegimeRow{{
		Date: "2026-09-23", Regime: "RISK_ON", CapturedAt: captured.UTC(),
	}})

	if v, labelled := got["2026-09-23"]; labelled {
		t.Fatalf("09-23 不得被「收盤後才擷取」的快照標註（非 PIT）；got 2026-09-23=%q", v)
	}
	// 下一個交易日（09-24）收盤晚於該擷取時間 ⇒ 該列可用於 09-24。
	next := time.Date(2026, 9, 24, 0, 0, 0, 0, tpe)
	if !taiwanholidays.IsTradingDay(next) {
		t.Skip("2026-09-24 非交易日，此環境日曆與生產不同")
	}
	if got["2026-09-24"] != "RISK_ON" {
		t.Fatalf("expected 2026-09-24 = RISK_ON, got %v", got)
	}
}

func TestPITRegimeBySession_CaptureBeforeCloseLabelsSameSession(t *testing.T) {
	mustTradingDay(t, "2026-09-23")
	tpe := taipeiZone()
	captured := time.Date(2026, 9, 23, 10, 0, 0, 0, tpe) // 收盤前 ⇒ 當日即可用
	got := PITRegimeBySession([]ledger.RegimeRow{{
		Date: "2026-09-23", Regime: "NEUTRAL", CapturedAt: captured.UTC(),
	}})
	if got["2026-09-23"] != "NEUTRAL" {
		t.Fatalf("收盤前擷取應可標註當日，got %v", got)
	}
}

func TestPITRegimeBySession_WeekendCaptureRollsToNextTradingDay(t *testing.T) {
	tpe := taipeiZone()
	sat := time.Date(2026, 9, 26, 10, 0, 0, 0, tpe) // 週六
	if taiwanholidays.IsTradingDay(sat) {
		t.Skip("2026-09-26 是交易日，日曆與預期不同")
	}
	got := PITRegimeBySession([]ledger.RegimeRow{{
		Date: "2026-09-25", Regime: "RISK_OFF", CapturedAt: sat.UTC(),
	}})
	if _, ok := got["2026-09-26"]; ok {
		t.Fatalf("週六不得被標註，got %v", got)
	}
	found := false
	for k, v := range got {
		if v == "RISK_OFF" {
			found = true
			d, _ := time.Parse("2006-01-02", k)
			if !taiwanholidays.IsTradingDay(d) {
				t.Fatalf("可用 session %s 必須是交易日", k)
			}
		}
	}
	if !found {
		t.Fatal("週六擷取的快照必須前滾到之後的交易日，不能整列丟失")
	}
}

func TestPITRegimeBySession_LatestCaptureWinsPerSession(t *testing.T) {
	mustTradingDay(t, "2026-09-23")
	tpe := taipeiZone()
	early := time.Date(2026, 9, 23, 9, 0, 0, 0, tpe) // 收盤前（同日可用）
	late := time.Date(2026, 9, 23, 11, 0, 0, 0, tpe) // 收盤前、較晚
	got := PITRegimeBySession([]ledger.RegimeRow{
		{Date: "2026-09-23", Regime: "NEUTRAL", CapturedAt: early.UTC()},
		{Date: "2026-09-23", Regime: "RISK_ON", CapturedAt: late.UTC()},
	})
	if got["2026-09-23"] != "RISK_ON" {
		t.Fatalf("同一 session 應以最新 capturedAt 為準，got %v", got)
	}
}

func TestPITRegimeBySession_SkipsIncompleteRows(t *testing.T) {
	got := PITRegimeBySession([]ledger.RegimeRow{
		{Date: "2026-09-23", Regime: "", CapturedAt: time.Now()},         // 空 regime
		{Date: "2026-09-23", Regime: "RISK_ON", CapturedAt: time.Time{}}, // 零時間
	})
	if len(got) != 0 {
		t.Fatalf("不完整列必須略過（不回填推測），got %v", got)
	}
}
