package service

import (
	"testing"
	"time"
)

// Helper: build epoch seconds from a wall-clock time in a location.
func tsAt(y int, m time.Month, d, hh, mm int, loc *time.Location) int64 {
	return time.Date(y, m, d, hh, mm, 0, 0, loc).Unix()
}

func containsSub(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && stringsIndex(s, sub) >= 0)
}

func stringsIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestEvaluateMacroFreshness_WeekendUS covers the #1762 complaint: on Monday
// early morning TW (Sunday evening ET), US-market indicators holding the
// Friday close must be ok with「週末休市」, never「資料待更新」.
func TestEvaluateMacroFreshness_WeekendUS(t *testing.T) {
	nyc := mustLocation("America/New_York")
	fr := tsAt(2026, time.August, 28, 16, 5, nyc)                          // Friday quote at close
	mon := tsAt(2026, time.August, 31, 2, 44, mustLocation("Asia/Taipei")) // Sunday 14:44 ET

	status, text := evaluateMacroFreshness("DX-Y.NYB", fr, time.Unix(mon, 0))
	if status != "ok" {
		t.Errorf("status = %q, want ok", status)
	}
	if !containsSub(text, "週末休市") || !containsSub(text, "8/28") {
		t.Errorf("text = %q, want 週末休市（8/28 收盤）", text)
	}
}

// TestEvaluateMacroFreshness_InSessionFresh — during a live session, data at
// the previous close is current.
func TestEvaluateMacroFreshness_InSessionFresh(t *testing.T) {
	nyc := mustLocation("America/New_York")
	tue := tsAt(2026, time.August, 25, 16, 5, nyc) // Tuesday close
	now := tsAt(2026, time.August, 26, 12, 0, nyc) // Wednesday in-session
	status, text := evaluateMacroFreshness("^VIX", tue, time.Unix(now, 0))
	if status != "ok" || text != "正常" {
		t.Errorf("got (%q,%q), want (ok, 正常)", status, text)
	}
}

// TestEvaluateMacroFreshness_MissedSession — a completed expected session
// without new data is genuine staleness.
func TestEvaluateMacroFreshness_MissedSession(t *testing.T) {
	nyc := mustLocation("America/New_York")
	mon := tsAt(2026, time.August, 24, 16, 5, nyc) // Monday close
	now := tsAt(2026, time.August, 26, 12, 0, nyc) // Wednesday: Tuesday session missed
	status, text := evaluateMacroFreshness("GC=F", mon, time.Unix(now, 0))
	if status != "warn" || !containsSub(text, "資料待更新") {
		t.Errorf("got (%q,%q), want warn 資料待更新", status, text)
	}
}

// TestEvaluateMacroFreshness_TWWeekendAndPreOpen — TW symbol over the weekend
// and in Monday pre-open.
func TestEvaluateMacroFreshness_TWWeekendAndPreOpen(t *testing.T) {
	tpe := mustLocation("Asia/Taipei")
	fr := tsAt(2026, time.August, 28, 13, 45, tpe) // Friday after 13:30 close
	sat := tsAt(2026, time.August, 29, 12, 0, tpe) // Saturday noon
	status, text := evaluateMacroFreshness("USDTWD=X", fr, time.Unix(sat, 0))
	if status != "ok" || !containsSub(text, "週末休市") {
		t.Errorf("got (%q,%q), want ok 週末休市", status, text)
	}

	preOpen := tsAt(2026, time.August, 31, 8, 0, tpe) // Monday 08:00 pre-open
	status, text = evaluateMacroFreshness("USDTWD=X", fr, time.Unix(preOpen, 0))
	if status != "ok" || !containsSub(text, "盤前") {
		t.Errorf("got (%q,%q), want ok 盤前", status, text)
	}
}

// TestEvaluateMacroFreshness_UnknownSymbolFallback — symbols without a
// market mapping keep the legacy 24h wall-clock behavior.
func TestEvaluateMacroFreshness_UnknownSymbolFallback(t *testing.T) {
	tpe := mustLocation("Asia/Taipei")
	now := tsAt(2026, time.August, 31, 2, 44, tpe)
	old := now - 30*3600
	status, text := evaluateMacroFreshness("UNKNOWN.SYM", old, time.Unix(now, 0))
	if status != "warn" || !containsSub(text, "資料待更新") {
		t.Errorf("got (%q,%q), want legacy warn", status, text)
	}
	recent := now - 2*3600
	status, text = evaluateMacroFreshness("UNKNOWN.SYM", recent, time.Unix(now, 0))
	if status != "ok" || text != "正常" {
		t.Errorf("got (%q,%q), want (ok, 正常)", status, text)
	}
}
