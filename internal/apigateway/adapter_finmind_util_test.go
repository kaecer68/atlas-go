package apigateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/taiwanholidays"
)

// TestFinMindProbeDate pins the probe-date rule introduced for issue #1999:
// the finmind channel probe must ask for the most recent Taiwan TRADING day,
// never "yesterday minus weekend". The old rule asked for 2026-09-25 (中秋節,
// 休市日 — verified against internal/taiwanholidays) from the 2026-09-26T01:01Z
// production probe and turned "no session that day" into a channel error.
func TestFinMindProbeDate(t *testing.T) {
	cases := []struct {
		name string
		now  string
		want string
	}{
		{"交易日盤前 → 前一個交易日", "2026-09-24T09:01:00+08:00", "2026-09-23"},
		{"休市日 中秋 09-25 → 前一個交易日", "2026-09-25T09:01:00+08:00", "2026-09-24"},
		{"休市日後週六（生產事故 2026-09-26T01:01Z = 台北 09:01）→ 前一個交易日", "2026-09-26T09:01:00+08:00", "2026-09-24"},
		{"週日 → 前一個交易日", "2026-09-27T09:01:00+08:00", "2026-09-24"},
		{"休市日後的週一（前一個日曆日是假日，須再往前）→ 前一個交易日", "2026-09-28T09:01:00+08:00", "2026-09-24"},
		{"雙十連假 10-09（週五休市）→ 前一個交易日", "2026-10-09T09:01:00+08:00", "2026-10-08"},
		{"UTC 01:01（台北當日 09:01）→ 前一個交易日", "2026-09-24T01:01:00Z", "2026-09-23"},
		// 這一列是「必須以台北時區計算」的牙齒：2026-09-28T17:30Z = 台北 09-29 01:30
		// → 前一個交易日 2026-09-28；若改回用 UTC 時鐘（去掉 .In(taipeiLoc)），
		// UTC 日期 09-28 會往前走到 09-25（中秋休市日）→ 2026-09-24，本列即 FAIL。
		{"UTC 傍晚（台北已跨日）→ 前一個交易日（時區不可省）", "2026-09-28T17:30:00Z", "2026-09-28"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatalf("bad fixture %q: %v", tc.now, err)
			}
			if got := finmindProbeDate(now); got != tc.want {
				t.Errorf("finmindProbeDate(%s) = %s, want %s", tc.now, got, tc.want)
			}
		})
	}

	// Invariant that makes the whole rule work: every probe date is a trading
	// day, so FinMind can always answer it.
	for _, tc := range cases {
		d, err := time.Parse("2006-01-02", tc.want)
		if err != nil {
			t.Fatalf("bad expected date %q: %v", tc.want, err)
		}
		if !taiwanholidays.IsTradingDay(d) {
			t.Errorf("fixture %q expects probe date %s, which is not a Taiwan trading day", tc.name, tc.want)
		}
	}

	// Fixture anchor: 2026-09-25 really is a 休市日 and 2026-09-26/27 a weekend,
	// i.e. this suite reproduces the production incident rather than assuming it.
	for _, day := range []string{"2026-09-25", "2026-09-26", "2026-09-27"} {
		d, _ := time.Parse("2006-01-02", day)
		if taiwanholidays.IsTradingDay(d) {
			t.Errorf("%s must be a non-trading day for this suite to reproduce #1999", day)
		}
	}
	if !strings.Contains(finmindProbeDate(time.Now()), "-") {
		t.Errorf("finmindProbeDate(now) = %q, expected YYYY-MM-DD", finmindProbeDate(time.Now()))
	}
	today := time.Now().In(mustTaipei(t)).Format("2006-01-02")
	if got := finmindProbeDate(time.Now()); got >= today {
		t.Errorf("finmindProbeDate(now) = %q, expected a date before today (%s)", got, today)
	}
}

func mustTaipei(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Fatalf("LoadLocation(Asia/Taipei): %v", err)
	}
	return loc
}

func TestSaveSnapshot(t *testing.T) {
	// saveSnapshot uses "data/state/<channelID>/latest.json" relative path
	// We need to ensure the test runs from a clean state
	channelID := "test_channel_test"
	defer func() {
		_ = os.RemoveAll("data")
	}()

	saveSnapshot(channelID, []byte(`{"test": true}`))

	expectedPath := filepath.Join("data", "state", channelID, "latest.json")
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("saveSnapshot failed to write file: %v", err)
	}
	if string(data) != `{"test": true}` {
		t.Errorf("saveSnapshot wrote %q, want {\"test\": true}", string(data))
	}
}
