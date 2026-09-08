package apigateway

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestChannelHealthStore_Alerts_FilterStale verifies that non-ok records
// older than the configured staleThreshold are filtered from Alerts().
//
// Real-world context: dashboard showed a 27-day-old `fubon` "no such host"
// error alongside fresh channel states. The stale record masked the real
// channel health — without filtering, Alerts() returns every historical
// error and the dashboard can't distinguish "currently broken" from
// "was broken once".
func TestChannelHealthStore_Alerts_FilterStale(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	// Inject stale record: error timestamp is 2 hours before "now".
	s.mu.Lock()
	s.data["ch_stale_2h"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
		LastError:   "old dns failure",
	}
	// Inject stale record from 27 days ago (the actual production scenario).
	s.data["ch_stale_27d"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-27 * 24 * time.Hour).Format(time.RFC3339),
		LastError:   "very old transient failure",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 0 {
		t.Errorf("expected stale alerts to be filtered, got %d: %+v", len(alerts), alerts)
	}
}

// TestChannelHealthStore_Alerts_KeepFresh verifies that non-ok records
// within the staleThreshold window still surface as alerts.
func TestChannelHealthStore_Alerts_KeepFresh(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	s.mu.Lock()
	s.data["ch_fresh_30m"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
		LastError:   "recent failure",
	}
	s.data["ch_fresh_59m"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-59 * time.Minute).Format(time.RFC3339),
		LastError:   "boundary case",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 2 {
		t.Errorf("expected 2 fresh alerts, got %d: %+v", len(alerts), alerts)
	}
}

// TestChannelHealthStore_Alerts_ZeroThresholdDisabled verifies that
// setting staleThreshold to 0 disables filtering entirely — useful for
// historical audit / debugging where all records are wanted.
func TestChannelHealthStore_Alerts_ZeroThresholdDisabled(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(0) // disable filter

	s.mu.Lock()
	s.data["ch_30d_old"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
		LastError:   "very old",
	}
	s.data["ch_ok"] = &ChannelHealthRecord{
		Status:    "ok",
		LastError: "",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "ch_30d_old" {
		t.Errorf("expected 1 old alert when filter disabled, got %+v", alerts)
	}
}

// TestChannelHealthStore_Alerts_BoundaryEdge verifies the > comparison:
// exactly at threshold = included; threshold + 1ns = filtered.
func TestChannelHealthStore_Alerts_BoundaryEdge(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	s.mu.Lock()
	// Exactly 1 hour old: should be included (boundary inclusive)
	s.data["ch_at_threshold"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
		LastError:   "boundary",
	}
	// 1 hour + 1 nanosecond old: should be filtered
	s.data["ch_over_threshold"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Add(-1*time.Hour - time.Nanosecond).Format(time.RFC3339),
		LastError:   "just over",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "ch_at_threshold" {
		t.Errorf("expected exactly ch_at_threshold to surface (boundary inclusive), got %+v", alerts)
	}
}

// TestChannelHealthStore_Alerts_UnparseableTimestampKept verifies the
// defensive fallback: if LastFetchAt cannot be parsed, the record is
// shown rather than silently filtered. Prefer false-positive alerts
// over silently missing a real failure.
func TestChannelHealthStore_Alerts_UnparseableTimestampKept(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	s.mu.Lock()
	s.data["ch_bad_ts"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: "not-a-rfc3339-timestamp",
		LastError:   "bad timestamp",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "ch_bad_ts" {
		t.Errorf("expected unparseable timestamp record to be kept (defensive), got %+v", alerts)
	}
}

// TestChannelHealthStore_Alerts_OKAndInactiveExcluded verifies the
// existing semantics: "ok" and "inactive" records never appear in
// Alerts() regardless of age.
func TestChannelHealthStore_Alerts_OKAndInactiveExcluded(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	now := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	s.WithNowFunc(func() time.Time { return now })
	s.WithStaleThreshold(1 * time.Hour)

	s.mu.Lock()
	s.data["ch_ok"] = &ChannelHealthRecord{
		Status:      "ok",
		LastFetchAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
	}
	s.data["ch_inactive"] = &ChannelHealthRecord{
		Status:      "inactive",
		LastFetchAt: now.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
	}
	s.data["ch_error_fresh"] = &ChannelHealthRecord{
		Status:      "error",
		LastFetchAt: now.Format(time.RFC3339),
		LastError:   "current",
	}
	s.mu.Unlock()

	alerts := s.Alerts()
	if len(alerts) != 1 || alerts[0].ChannelID != "ch_error_fresh" {
		t.Errorf("expected only ch_error_fresh to surface, got %+v", alerts)
	}
}

// TestChannelHealthStore_Alerts_DefaultThresholdApplied verifies that
// the constructor wires DefaultStaleThreshold (1 hour) by default —
// no explicit WithStaleThreshold call needed for production use.
func TestChannelHealthStore_Alerts_DefaultThresholdApplied(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	if s.staleThreshold != DefaultStaleThreshold {
		t.Errorf("expected default staleThreshold = %v, got %v",
			DefaultStaleThreshold, s.staleThreshold)
	}
	if DefaultStaleThreshold != 1*time.Hour {
		t.Errorf("expected DefaultStaleThreshold = 1 hour, got %v", DefaultStaleThreshold)
	}
}

// TestChannelHealthStore_WithStaleThreshold_Chaining verifies the
// setter returns *ChannelHealthStore for fluent API.
func TestChannelHealthStore_WithStaleThreshold_Chaining(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)
	got := s.WithStaleThreshold(2 * time.Hour).WithNowFunc(time.Now)
	if got != s {
		t.Errorf("With* methods must return the store for chaining")
	}
	if s.staleThreshold != 2*time.Hour {
		t.Errorf("WithStaleThreshold did not apply")
	}
	if s.nowFunc == nil {
		t.Errorf("WithNowFunc did not apply")
	}
}

// TestChannelHealthStore_Record_OKClearsStaleErrors verifies that a healthy
// (status=ok) Record() clears the Errors slice left over from a previous
// failure, so the dashboard stops showing stale error text for channels that
// have since recovered (e.g. market_volume / day_trading).
func TestChannelHealthStore_Record_OKClearsStaleErrors(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	if err := s.Record("market_volume", "error", "connection refused"); err != nil {
		t.Fatalf("record error: %v", err)
	}
	rec := s.Get("market_volume")
	if rec == nil || len(rec.Errors) == 0 {
		t.Fatalf("expected stale error to be recorded, got %+v", rec)
	}

	if err := s.Record("market_volume", "ok", ""); err != nil {
		t.Fatalf("record ok: %v", err)
	}
	rec = s.Get("market_volume")
	if rec == nil {
		t.Fatal("expected record after ok")
	}
	if rec.LastError != "" {
		t.Errorf("LastError = %q, want empty after ok", rec.LastError)
	}
	if len(rec.Errors) != 0 {
		t.Errorf("Errors = %v, want empty after ok (stale error text must be cleared)", rec.Errors)
	}
}

// TestChannelHealthStore_RecordWaiting_KeepsOKWithoutAdvancingLastSuccess
// verifies the waiting semantics added in the 2026-09-03 alert-noise fix: a
// channel whose upstream has no NEW data yet (e.g. TDCC weekly snapshot not
// published) keeps status "ok" (no ChannelHealthStatusError alert) but does
// NOT advance LastSuccessAt — consumers treat last_success as the
// data-freshness anchor, so it must stay at the last time data landed.
func TestChannelHealthStore_RecordWaiting_KeepsOKWithoutAdvancingLastSuccess(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)

	// First: a real successful fetch lands data.
	if err := s.Record("tdcc_equity_dispersion", "ok", ""); err != nil {
		t.Fatalf("record ok: %v", err)
	}
	rec := s.Get("tdcc_equity_dispersion")
	if rec == nil || rec.LastSuccessAt == "" {
		t.Fatalf("expected LastSuccessAt after a successful fetch, got %+v", rec)
	}
	firstSuccess := rec.LastSuccessAt

	// Waiting outcome: upstream answered but has no new snapshot yet.
	if err := s.RecordWaiting("tdcc_equity_dispersion"); err != nil {
		t.Fatalf("record waiting: %v", err)
	}
	rec = s.Get("tdcc_equity_dispersion")
	if rec == nil {
		t.Fatal("expected record after waiting")
	}
	if rec.Status != "ok" {
		t.Errorf("Status = %q, want ok (waiting must not surface as error)", rec.Status)
	}
	if rec.LastError != "" {
		t.Errorf("LastError = %q, want empty on waiting (no error text on a healthy record)", rec.LastError)
	}
	if rec.LastSuccessAt != firstSuccess {
		t.Errorf("LastSuccessAt = %q, want unchanged %q (waiting must not advance last_success)", rec.LastSuccessAt, firstSuccess)
	}
	if rec.LastFetchAt < firstSuccess {
		t.Errorf("LastFetchAt = %q should be refreshed on waiting (>= first success %q)", rec.LastFetchAt, firstSuccess)
	}

	// A later real success advances last_success again (the ok record sets
	// LastSuccessAt == LastFetchAt, unlike the waiting record which keeps
	// the old anchor). Cross a second boundary so the RFC3339 (second
	// precision) timestamps differ deterministically.
	time.Sleep(1100 * time.Millisecond)
	if err := s.Record("tdcc_equity_dispersion", "ok", ""); err != nil {
		t.Fatalf("record ok after waiting: %v", err)
	}
	rec = s.Get("tdcc_equity_dispersion")
	if rec == nil {
		t.Fatal("expected record after final ok")
	}
	if rec.Status != "ok" {
		t.Errorf("Status = %q, want ok after final success", rec.Status)
	}
	if rec.LastSuccessAt != rec.LastFetchAt {
		t.Errorf("LastSuccessAt = %q, want == LastFetchAt %q after a real success", rec.LastSuccessAt, rec.LastFetchAt)
	}
	if rec.LastSuccessAt == firstSuccess {
		t.Errorf("expected LastSuccessAt to advance after a real success, still %q", rec.LastSuccessAt)
	}
}

// TestChannelHealthStore_RecordWaiting_NoPriorSuccess stays ok from a cold
// start: a brand-new channel that has never landed data records waiting
// without inventing a last_success.
func TestChannelHealthStore_RecordWaiting_NoPriorSuccess(t *testing.T) {
	s := NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := s.RecordWaiting("tdcc_equity_dispersion"); err != nil {
		t.Fatalf("record waiting: %v", err)
	}
	rec := s.Get("tdcc_equity_dispersion")
	if rec == nil {
		t.Fatal("expected record after waiting")
	}
	if rec.Status != "ok" {
		t.Errorf("Status = %q, want ok", rec.Status)
	}
	if rec.LastSuccessAt != "" {
		t.Errorf("LastSuccessAt = %q, want empty (no data ever landed)", rec.LastSuccessAt)
	}
	if rec.LastFetchAt == "" {
		t.Error("LastFetchAt should be refreshed even on waiting")
	}
	// Waiting records must not appear in Alerts().
	if alerts := s.Alerts(); len(alerts) != 0 {
		t.Errorf("waiting record surfaced as alert: %+v", alerts)
	}
}

// --- 失敗阻尼（k3 audit R1, 2026-09-08）---

// dampingTestClock pins the record clock inside the TW market session
// (Tuesday 10:00 Taipei) so the R2 session cap is deterministically
// INACTIVE for damping tests. Tests exercising the cap itself use their
// own clocks (see TestRecord_SessionCap).
func dampingTestClock() func() time.Time {
	return func() time.Time { return time.Date(2026, 9, 8, 10, 0, 0, 0, taipeiLoc) }
}

// TestRecord_ErrorDamping_FirstFailureIsWarn: 單次 error 嘗試 → derived
// status=warn（gauge 1）→ ChannelHealthStatusError（status==2）不會響。
func TestRecord_ErrorDamping_FirstFailureIsWarn(t *testing.T) {
	dir := t.TempDir()
	s := NewChannelHealthStore(dir).WithRecordClock(dampingTestClock())
	if err := s.Record("fubon", "ok", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Record("fubon", "error", "fubon proxy: status 500, body: {\"detail\":\"503: SDK init timed out\"}"); err != nil {
		t.Fatal(err)
	}
	rec := s.Get("fubon")
	if rec == nil {
		t.Fatal("no record")
	}
	if rec.Status != "warn" {
		t.Errorf("status = %q, want warn (single transient failure must not page)", rec.Status)
	}
	if rec.ConsecutiveFailures != 1 {
		t.Errorf("counter = %d, want 1", rec.ConsecutiveFailures)
	}
	if rec.LastError == "" {
		t.Error("last_error must be kept on warn (diagnostics)")
	}
}

// TestRecord_ErrorDamping_StreakEscalatesToError: 連續失敗達預設 Grace(2)
// → derived status=error。
func TestRecord_ErrorDamping_StreakEscalatesToError(t *testing.T) {
	dir := t.TempDir()
	s := NewChannelHealthStore(dir).WithRecordClock(dampingTestClock())
	_ = s.Record("fubon", "error", "first")
	_ = s.Record("fubon", "error", "second")
	rec := s.Get("fubon")
	if rec.Status != "error" {
		t.Errorf("status = %q, want error after %d consecutive failures", rec.Status, rec.ConsecutiveFailures)
	}
	if rec.ConsecutiveFailures != 2 {
		t.Errorf("counter = %d, want 2", rec.ConsecutiveFailures)
	}
}

// TestRecord_SuccessResetsStreak: 成功歸零 — 恢復後下一次單次失敗又是 warn。
func TestRecord_SuccessResetsStreak(t *testing.T) {
	dir := t.TempDir()
	s := NewChannelHealthStore(dir).WithRecordClock(dampingTestClock())
	_ = s.Record("fubon", "error", "boom")
	_ = s.Record("fubon", "ok", "")
	_ = s.Record("fubon", "error", "transient again")
	rec := s.Get("fubon")
	if rec.Status != "warn" {
		t.Errorf("status = %q, want warn (streak must reset on success)", rec.Status)
	}
	if rec.ConsecutiveFailures != 1 {
		t.Errorf("counter = %d, want 1", rec.ConsecutiveFailures)
	}
}

// TestRecord_WarnAttemptKeepsStreak: breaker-open 的 warn 嘗試不推進、
// 不歸零計數（資料沒落地，streak 語義保持）。
func TestRecord_WarnAttemptKeepsStreak(t *testing.T) {
	dir := t.TempDir()
	s := NewChannelHealthStore(dir).WithRecordClock(dampingTestClock())
	_ = s.Record("fubon", "error", "boom")
	_ = s.Record("fubon", "warn", "breaker open")
	rec := s.Get("fubon")
	if rec.ConsecutiveFailures != 1 {
		t.Errorf("counter = %d, want 1 (warn attempt must not reset streak)", rec.ConsecutiveFailures)
	}
	if rec.Status != "warn" {
		t.Errorf("status = %q, want warn", rec.Status)
	}
}

// TestRecord_GraceFailuresOneImmediateError: 契約顯式 GraceFailures=1 →
// 單次失敗立即 error（時間敏感通道的逃生門）。
func TestRecord_GraceFailuresOneImmediateError(t *testing.T) {
	dir := t.TempDir()
	s := NewChannelHealthStore(dir).WithRecordClock(dampingTestClock())
	orig := ChannelContracts().Contract("fubon") // 快照原契約（含 MarketSession 標籤）
	c := orig
	c.GraceFailures = 1
	ChannelContracts().Register(c)
	t.Cleanup(func() {
		// 還原原契約（Registry 全域單例；不得用 DefaultChannelContract —
		// 會抹掉 session 標籤污染後續測試）
		ChannelContracts().Register(orig)
	})
	_ = s.Record("fubon", "error", "boom")
	rec := s.Get("fubon")
	if rec.Status != "error" {
		t.Errorf("status = %q, want error with GraceFailures=1", rec.Status)
	}
}

// TestRecord_DampingSurvivesReload: 阻尼計數持久化到 disk — 重啟（新
// store 實例）後 streak 延續，不會因重啟歸零而重新放行單次失敗。
func TestRecord_DampingSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	s1 := NewChannelHealthStore(dir).WithRecordClock(dampingTestClock())
	_ = s1.Record("fubon", "error", "boom")
	_ = s1.Record("fubon", "error", "boom2")

	s2 := NewChannelHealthStore(dir).WithRecordClock(dampingTestClock())
	_ = s2.Record("fubon", "error", "boom3")
	rec := s2.Get("fubon")
	if rec.ConsecutiveFailures != 3 {
		t.Errorf("counter = %d, want 3 (streak must survive restart)", rec.ConsecutiveFailures)
	}
	if rec.Status != "error" {
		t.Errorf("status = %q, want error", rec.Status)
	}
}

// --- R2 契約級 session 感知（k3 audit, 2026-09-08）---

// TestRecord_SessionCap_ErrorOutsideMarketSession: tw 通道在非交易時段
// 連續失敗不得升級 error（重放 fubon 08:05 盤前 proxy 503 誤報；k3 audit
// R2）。交易時段內行為不變（streak 達 grace 仍升級）。
func TestRecord_SessionCap_ErrorOutsideMarketSession(t *testing.T) {
	// helper 邊界：週六非 session；交易日 08:05 盤前非 session；交易日 10:00 是 session
	sat := time.Date(2026, 9, 12, 10, 0, 0, 0, taipeiLoc)
	if twMarketSessionActive(sat) {
		t.Fatal("Saturday must not be a market session")
	}
	pre := time.Date(2026, 9, 8, 8, 5, 0, 0, taipeiLoc)
	if twMarketSessionActive(pre) {
		t.Fatal("08:05 on a trading day is pre-market, not session")
	}
	if !twMarketSessionActive(time.Date(2026, 9, 8, 10, 0, 0, 0, taipeiLoc)) {
		t.Fatal("10:00 on a trading day must be session")
	}

	// fubon 是 1h 間距的盤前健康檢查 — 時鐘固定在週六（非 session）：
	// 連續 3 次 error 也不得升級 error。
	dir := t.TempDir()
	clock := sat
	s := NewChannelHealthStore(dir).WithRecordClock(func() time.Time { return clock })
	for i := 0; i < 3; i++ {
		if err := s.Record("fubon", "error", "proxy 503 SDK init timed out"); err != nil {
			t.Fatal(err)
		}
	}
	rec := s.Get("fubon")
	if rec.ConsecutiveFailures != 3 {
		t.Fatalf("counter = %d, want 3", rec.ConsecutiveFailures)
	}
	if rec.Status != "warn" {
		t.Fatalf("status = %q, want warn (session cap on non-trading day)", rec.Status)
	}

	// 時鐘推進到週二 10:00（session 內）：下一次失敗越過 grace → error。
	clock = time.Date(2026, 9, 8, 10, 0, 0, 0, taipeiLoc)
	if err := s.Record("fubon", "error", "proxy 503"); err != nil {
		t.Fatal(err)
	}
	rec = s.Get("fubon")
	if rec.Status != "error" {
		t.Fatalf("status = %q, want error (in session, streak 4 >= grace 2)", rec.Status)
	}
}

// --- R3 provenance（k3 audit, 2026-09-08）---

// TestRecord_UnregisteredIDMarkedDerived: 寫入未註冊 ID → provenance=derived；
// 註冊 ID → 空（regular）。
func TestRecord_UnregisteredIDMarkedDerived(t *testing.T) {
	dir := t.TempDir()
	s := NewChannelHealthStore(dir)
	_ = s.Record("vix", "ok", "") // vix 不在 channelIDs()（us_yahoo 的指標欄位）
	if rec := s.Get("vix"); rec == nil || rec.Provenance != ProvenanceDerived {
		t.Fatalf("vix provenance = %+v, want derived", rec)
	}
	_ = s.Record("fubon", "ok", "") // fubon 已註冊
	if rec := s.Get("fubon"); rec == nil || rec.Provenance != "" {
		t.Fatalf("fubon provenance = %+v, want empty (registered)", rec)
	}
}

// TestLoad_JanitorMarksLegacyOrphans: 舊版 JSON（無 provenance 欄位）載入時
// 孤兒 ID 自動標 derived（冪等）。
func TestLoad_JanitorMarksLegacyOrphans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "channel_health.json")
	payload := `{"channels":{"vix":{"status":"ok","last_fetch_at":"2026-09-01T00:00:00Z"},"fubon":{"status":"ok","last_fetch_at":"2026-09-01T00:00:00Z"}}}`
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewChannelHealthStore(dir)
	if rec := s.Get("vix"); rec == nil || rec.Provenance != ProvenanceDerived {
		t.Fatalf("legacy orphan vix: %+v, want provenance derived", rec)
	}
	if rec := s.Get("fubon"); rec == nil || rec.Provenance != "" {
		t.Fatalf("legacy registered fubon: %+v, want empty provenance", rec)
	}
	// 冪等：再次 load 不變
	_ = s.load()
	if rec := s.Get("vix"); rec == nil || rec.Provenance != ProvenanceDerived {
		t.Fatal("janitor must be idempotent")
	}
}
