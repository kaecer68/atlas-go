package apigateway

import (
	"strings"
	"testing"
	"time"
)

// ① 前端改用與告警同一契約（2026-10-05）。
//
// 由來：ChannelDataStale 的輸入是 atlas_channel_staleness_overage_seconds，而
// 告警側已於 #2201（F58）改成「交易日感知」——但前端這一側
// （DeriveChannelStatus → /admin/datachannels、首頁資料品質、DB mirror）仍用
// 牆鐘窗口 age > EffectiveFreshnessWindow()，所以同一個通道在週末有兩種判定：
// 告警說 overage = 0（正常），頁面說「資料過期」。本檔把「同一根因的第二個
// 消費者也修好」寫成可執行規格。
//
// 實損（F58 已登錄讀數，未重查生產）:
//
//	twse_sbl          last_success 10-02T07:20Z
//	government_broker last_success 10-02T07:18Z（2026-10-02 週五，台北 15:20）
//	⇒ 10-02T07:22Z + 48h 起過窗，至週日晚約 7h 顯示為「資料過期」，而兩個通道
//	  當日/當週皆正常（上游週末沒有發布機會）。
var (
	tpeZone   = time.FixedZone("CST", 8*3600)
	tpeSunday = time.Date(2026, 10, 4, 22, 28, 0, 0, tpeZone)
)

// TestDeriveChannelStatus_TradingDayChannelWeekendIsNotStale 是 ① 的驗收主體：
// 用 10-02～10-04 的真實形狀（週五最後成功、週日晚上檢視）餵進判定，週末不得
// 再判 stale；而下一個交易日的發布截止（台北 18:00）過後必須判 stale。
func TestDeriveChannelStatus_TradingDayChannelWeekendIsNotStale(t *testing.T) {
	contracts := ChannelContracts()
	lastSuccess := time.Date(2026, 10, 2, 7, 20, 0, 0, time.UTC) // 台北 15:20，週五

	calendarChannels := []string{"twse_sbl", "government_broker"}
	for _, id := range calendarChannels {
		c := contracts.Contract(id)
		if c.PublishCalendar != PublishCalendarTWTradingDay {
			t.Fatalf("%s 必須宣告 PublishCalendarTWTradingDay（實際 %q）", id, c.PublishCalendar)
		}
		rec := &ChannelHealthRecord{
			Status:        StatusOK,
			LastFetchAt:   lastSuccess.Format(time.RFC3339),
			LastSuccessAt: lastSuccess.Format(time.RFC3339),
			LastDataAt:    lastSuccess.Format(time.RFC3339),
		}

		weekend := []time.Time{
			time.Date(2026, 10, 3, 12, 0, 0, 0, tpeZone), // 週六
			tpeSunday, // 週日（實損時刻）
			time.Date(2026, 10, 4, 23, 59, 0, 0, tpeZone),
		}
		for _, now := range weekend {
			if got := DeriveChannelStatus(rec, c, now); got != StatusOK {
				t.Errorf("%s @ %s = %q, want ok（週末上游沒有發布機會，牆鐘窗口不得判 stale）",
					id, now.In(tpeZone).Format(time.RFC3339), got)
			}
			if r := DeriveChannelStatusReason(rec, c, now); r != "" {
				t.Errorf("%s @ %s 在週末必須沒有理由文字，got %q", id, now.In(tpeZone).Format(time.RFC3339), r)
			}
		}

		// 週一 10:30（當日 18:00 截止前）：期望值仍是上一個交易日 ⇒ 仍 ok。
		mondayMorning := time.Date(2026, 10, 5, 10, 30, 0, 0, tpeZone)
		if got := DeriveChannelStatus(rec, c, mondayMorning); got != StatusOK {
			t.Errorf("%s @ 週一 10:30 = %q, want ok（當日發布窗口尚未到）", id, got)
		}

		// 週一 18:30（截止後）：週五的資料已經早於應有之交易日 ⇒ stale，
		// 而且理由必須是日曆語意（不是「超過合約更新窗口 48 小時」）。
		mondayEvening := time.Date(2026, 10, 5, 18, 30, 0, 0, tpeZone)
		if got := DeriveChannelStatus(rec, c, mondayEvening); got != StatusStale {
			t.Errorf("%s @ 週一 18:30 = %q, want stale（真過期，不是日曆假象）", id, got)
		}
		reason := DeriveChannelStatusReason(rec, c, mondayEvening)
		if !strings.Contains(reason, "2026-10-05") || !strings.Contains(reason, "交易日") {
			t.Errorf("%s 的 stale 理由必須說明應有之交易日，got %q", id, reason)
		}
	}
}

// TestDeriveChannelStatus_UndeclaredChannelKeepsWallClockRule 是「未宣告
// PublishCalendar 的通道語意逐字不變」的可執行證明：同一批邊界輸入（窗口內、
// 窗口邊緣、窗口外）在改動前後必須得到相同的判定與相同的理由文字。
func TestDeriveChannelStatus_UndeclaredChannelKeepsWallClockRule(t *testing.T) {
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	// us_yahoo 是預設契約（48h 窗口、無日曆）的通道。
	c := ChannelContracts().Contract("us_yahoo")
	if c.PublishCalendar != PublishCalendarDaily {
		t.Fatalf("us_yahoo 不該宣告發布日曆，got %q", c.PublishCalendar)
	}
	if c.EffectiveFreshnessWindow() != StaleDataThreshold {
		t.Fatalf("us_yahoo 窗口 = %s, want %s", c.EffectiveFreshnessWindow(), StaleDataThreshold)
	}

	cases := []struct {
		name string
		age  time.Duration
		want string
	}{
		{"窗口內 47h59m", 47*time.Hour + 59*time.Minute, StatusOK},
		{"窗口邊緣 48h（不大於 ⇒ ok，與舊 age > window 相同）", 48 * time.Hour, StatusOK},
		{"窗口外 48h01m", 48*time.Hour + time.Minute, StatusStale},
	}
	for _, tc := range cases {
		rec := &ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-tc.age))}
		if got := DeriveChannelStatus(rec, c, now); got != tc.want {
			t.Errorf("%s: DeriveChannelStatus = %q, want %q", tc.name, got, tc.want)
		}
	}

	// 理由文字逐字不變（未宣告日曆的通道）。
	rec := &ChannelHealthRecord{Status: StatusOK, LastFetchAt: mustRFC(now.Add(-17 * 24 * time.Hour))}
	// 逐字沿用現行文字（含 "17 天 未更新" 既有空白）：本測證明改動沒有動到它。
	want := "資料已 17 天 未更新，超過合約更新窗口 48 小時（最後一次成功抓取 " + rec.LastFetchAt + "）"
	if got := DeriveChannelStatusReason(rec, c, now); got != want {
		t.Errorf("未宣告通道的 stale 理由 = %q, want %q（語意必須逐字不變）", got, want)
	}
}

// TestDeriveChannelStatus_DegradedEscalationUsesTheSameContract 是 2b（degraded
// → error 升級）與 ① 一致性的證明：升級判定也必須走契約，而不是第二套牆鐘。
func TestDeriveChannelStatus_DegradedEscalationUsesTheSameContract(t *testing.T) {
	lastData := time.Date(2026, 10, 2, 7, 18, 0, 0, time.UTC) // 週五 15:18 台北
	rec := &ChannelHealthRecord{
		Status:        StatusDegraded,
		LastFetchAt:   mustRFC(tpeSunday.Add(-2 * time.Minute)),
		LastSuccessAt: lastData.Format(time.RFC3339),
		LastDataAt:    lastData.Format(time.RFC3339),
	}
	contract := ChannelContracts().Contract("government_broker")

	// 週末：資料來自最新應發布的交易日 ⇒ 不升級（維持 degraded，不是 error）。
	if got := DeriveChannelStatus(rec, contract, tpeSunday); got != StatusDegraded {
		t.Errorf("週日 = %q, want degraded（資料並未逾約）", got)
	}

	// 週一 18:30：發布截止已過而資料還停在週五 ⇒ 升級為 error。
	mondayEvening := time.Date(2026, 10, 5, 18, 30, 0, 0, tpeZone)
	if got := DeriveChannelStatus(rec, contract, mondayEvening); got != StatusError {
		t.Errorf("週一 18:30 = %q, want error（真過期，必須是可告警的判定）", got)
	}

	// 未宣告日曆的通道維持牆鐘語意（48h 窗口）。
	plain := ChannelContracts().Contract("us_yahoo")
	if got := DeriveChannelStatus(rec, plain, tpeSunday); got != StatusError {
		t.Errorf("未宣告通道 = %q, want error（牆鐘窗口語意不變）", got)
	}
}

// TestDeriveChannelStatus_RetiredContractOutranksEveryRecord 是 ②（退役標記）在
// 判定層的規格：契約宣告退役後，任何 record 都不能讓它回到可告警的判定。
func TestDeriveChannelStatus_RetiredContractOutranksEveryRecord(t *testing.T) {
	c := ChannelContracts().Contract("twse_oddlot")
	if !c.IsRetired() {
		t.Fatal("twse_oddlot 的契約必須宣告 Retirement（設計退休）")
	}

	recs := map[string]*ChannelHealthRecord{
		"nil record": nil,
		"ok 且剛抓完":    {Status: StatusOK, LastFetchAt: mustRFC(tpeSunday)},
		"17 天前的 ok":  {Status: StatusOK, LastFetchAt: mustRFC(tpeSunday.Add(-17 * 24 * time.Hour))},
		"degraded（修前會升級為 error 的形狀）": {
			Status: StatusDegraded, LastFetchAt: mustRFC(tpeSunday),
			LastSuccessAt: mustRFC(tpeSunday.Add(-17 * 24 * time.Hour)),
		},
		"error 殘留 record": {Status: StatusError, LastFetchAt: mustRFC(tpeSunday)},
	}
	for name, rec := range recs {
		if got := DeriveChannelStatus(rec, c, tpeSunday); got != StatusRetired {
			t.Errorf("%s = %q, want %q", name, got, StatusRetired)
		}
	}

	// 別名（runtime 的 dash 形式）必須解析到同一份契約，否則那個拼法會退回
	// DefaultChannelContract 而變回「一般通道」。
	if got := DeriveChannelStatusForID(&ChannelHealthRecord{Status: StatusError}, "twse-oddlot", tpeSunday); got != StatusRetired {
		t.Errorf("dash alias = %q, want %q", got, StatusRetired)
	}

	// 退休理由必須自我說明，且指名替代輸入（資訊級文字，不是待辦）。
	reason := DeriveChannelStatusReason(nil, c, tpeSunday)
	if !strings.Contains(reason, "已退役") || !strings.Contains(reason, "twse_capital_flow") {
		t.Errorf("退役理由 = %q, want 已退役 ＋ 替代輸入", reason)
	}

	// 負向控制：沒宣告退役的通道（tej 是 operator opt-in 未開通）不得變成 retired。
	if got := DeriveChannelStatus(&ChannelHealthRecord{Status: StatusInactive}, ChannelContracts().Contract("tej"), tpeSunday); got != StatusInactive {
		t.Errorf("tej = %q, want inactive（未啟用是可逆的狀態，不是退役）", got)
	}
}
