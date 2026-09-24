package service

import (
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

// TestResolveChannelStatusFromStore_NilStore covers the case where the caller
// (e.g. SystemService in a unit test) hasn't injected a health store.
func TestResolveChannelStatusFromStore_NilStore(t *testing.T) {
	status, updated, lastErr := resolveChannelStatusFromStore(nil, "any", "warn", "2026-05-13")
	if status != "warn" {
		t.Fatalf("expected fileStatus to pass through, got %q", status)
	}
	if updated != "2026-05-13" {
		t.Fatalf("expected fileUpdated to pass through, got %q", updated)
	}
	if lastErr != "" {
		t.Fatalf("expected empty lastError with nil store, got %q", lastErr)
	}
}

func TestResolveChannelStatusFromStore_NoRecord(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	status, updated, lastErr := resolveChannelStatusFromStore(store, "unrecorded", "warn", "fallback")
	if status != "warn" {
		t.Fatalf("expected fileStatus to pass through, got %q", status)
	}
	if updated != "fallback" {
		t.Fatalf("expected fileUpdated to pass through, got %q", updated)
	}
	if lastErr != "" {
		t.Fatalf("expected empty lastError when no record, got %q", lastErr)
	}
}

// TestResolveChannelStatusFromStore_Ok covers the happy path: the Gateway
// has a recent successful fetch, so the channel is reported as ok
// regardless of file age (this is the bug fix — without store override,
// weekend file mtime would falsely report "待更新").
func TestResolveChannelStatusFromStore_Ok(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := store.Record("twse_capital_flow", "ok", ""); err != nil {
		t.Fatalf("record ok: %v", err)
	}
	// File-age would say "warn" (e.g. file is 3 days old), but the store
	// takes priority and reports the real fetch result.
	status, updated, lastErr := resolveChannelStatusFromStore(store, "twse_capital_flow", "warn", "20260608")
	if status != "ok" {
		t.Fatalf("expected ok override, got %q", status)
	}
	if updated == "20260608" {
		t.Fatalf("expected LastFetchAt to replace fileUpdated, got %q", updated)
	}
	if lastErr != "" {
		t.Fatalf("expected empty lastError on ok, got %q", lastErr)
	}
}

func TestResolveChannelStatusFromStore_Error(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	// twse_capital_flow 為 tw-session 通道（R2）：時鐘固定在交易時段內
	//（UTC 02:00 = 台北 10:00），session cap 不干擾升級斷言。
	store.WithRecordClock(func() time.Time { return time.Date(2026, 9, 8, 2, 0, 0, 0, time.UTC) })
	for i := 0; i < 2; i++ { // 越過 GraceFailures(2) → derived error（k3 audit R1）
		if err := store.Record("twse_capital_flow", "error", "rate limit exceeded"); err != nil {
			t.Fatalf("record error: %v", err)
		}
	}
	status, updated, lastErr := resolveChannelStatusFromStore(store, "twse_capital_flow", "ok", "20260611")
	if status != "error" {
		t.Fatalf("expected error override, got %q", status)
	}
	if updated != "上次失敗: rate limit exceeded" {
		t.Fatalf("expected formatted error message, got %q", updated)
	}
	if lastErr != "rate limit exceeded" {
		t.Fatalf("expected raw error to be exposed, got %q", lastErr)
	}
}

// TestResolveChannelStatusFromStore_Inactive covers the inactive status: the
// record's own verdict must be reported (2026-09-24). It used to fall through
// to the file-age fallback, so /admin/datachannels rendered twse_etf — whose
// record says inactive with a reason — as an empty status / 未知.
func TestResolveChannelStatusFromStore_Inactive(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := store.Record("twse_etf", "inactive", "TWT44U removed upstream (2026-08-10)"); err != nil {
		t.Fatalf("record inactive: %v", err)
	}
	status, updated, lastErr := resolveChannelStatusFromStore(store, "twse_etf", "ok", "20260611")
	if status != "inactive" {
		t.Fatalf("expected inactive to be reported, got %q", status)
	}
	if updated == "20260611" {
		t.Fatalf("expected the record's LastFetchAt, got the file timestamp %q", updated)
	}
	if lastErr != "TWT44U removed upstream (2026-08-10)" {
		t.Fatalf("expected store LastError to be attached, got %q", lastErr)
	}
}

// TestResolveChannelStatusFromStore_UnmappedStatusStillFallsBack pins the
// remaining fallback: a store status the resolver genuinely has no mapping for
// keeps the file-age verdict, with the record's error attached for visibility.
func TestResolveChannelStatusFromStore_UnmappedStatusStillFallsBack(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := store.Record("ch-weird", "quantum", "unmapped"); err != nil {
		t.Fatalf("record: %v", err)
	}
	status, updated, lastErr := resolveChannelStatusFromStore(store, "ch-weird", "warn", "20260611")
	if status != "warn" || updated != "20260611" {
		t.Fatalf("expected fileStatus/fileUpdated fallback, got (%q,%q)", status, updated)
	}
	if lastErr != "unmapped" {
		t.Fatalf("expected store LastError to be attached, got %q", lastErr)
	}
}

// TestResolveChannelStatusFromStore_StaleRecordIsNotDropped covers a record
// written directly as "stale" (adapter_government_broker HealthCheck marks an
// unusable snapshot stale): it must be reported, not silently replaced by the
// file-age verdict.
func TestResolveChannelStatusFromStore_StaleRecordIsNotDropped(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := store.Record("government_broker", "stale", "reading older than 48h"); err != nil {
		t.Fatalf("record stale: %v", err)
	}
	status, _, lastErr := resolveChannelStatusFromStore(store, "government_broker", "ok", "20260611")
	if status != "stale" {
		t.Fatalf("expected stale to be reported, got %q", status)
	}
	if lastErr != "reading older than 48h" {
		t.Fatalf("expected the reason attached, got %q", lastErr)
	}
}

// TestResolveChannelStatusFromStore_ExpiredOkIsStale is the regression test for
// the reported symptom: a channel whose record says ok but whose last successful
// fetch is 17 days old (twse_oddlot, upstream removed) used to read "ok / 正常"
// on /admin/datachannels while the health summary logged "stale" at the same
// second. Both must now say stale.
func TestResolveChannelStatusFromStore_ExpiredOkIsStale(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	fetched := time.Now().Add(-17 * 24 * time.Hour).UTC().Truncate(time.Second)
	store.WithRecordClock(func() time.Time { return fetched })
	if err := store.Record("twse_oddlot", "ok", ""); err != nil {
		t.Fatalf("record ok: %v", err)
	}
	// Back to the real clock for the control case below.
	store.WithRecordClock(time.Now)
	status, updated, lastErr := resolveChannelStatusFromStore(store, "twse_oddlot", "ok", "20260907")
	if status != "stale" {
		t.Fatalf("expected stale for a 17-day-old ok record, got %q", status)
	}
	if updated == "20260907" {
		t.Fatalf("expected the record's LastFetchAt as updated, got the file timestamp %q", updated)
	}
	if lastErr == "" || !strings.Contains(lastErr, "48 小時") {
		t.Fatalf("expected a reason quoting the freshness window, got %q", lastErr)
	}
	// The same clock (now) must not turn a freshly fetched record stale.
	if err := store.Record("twse_margin", "ok", ""); err != nil {
		t.Fatalf("record ok: %v", err)
	}
	if st, _, _ := resolveChannelStatusFromStore(store, "twse_margin", "warn", "20260907"); st != "ok" {
		t.Fatalf("fresh record = %q, want ok", st)
	}
}

// TestResolveChannelStatusFromStore_Warn is the regression test for the
// /admin/datachannels "未知" display bug: the Gateway records a transient
// waiting state (FinMind daily quota exhausted) as "warn", and the
// registered-channel fallback path passes an empty fileStatus. The resolver
// must report "warn" (displayed as 待更新) with the stored error attached —
// not fall through to the empty fileStatus which rendered as "未知".
func TestResolveChannelStatusFromStore_Warn(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := store.Record("twse_sbl", "warn", "twse_sbl: finmind fetch 2026-09-04: finmind: daily quota exhausted (used=14400, remaining=0)"); err != nil {
		t.Fatalf("record warn: %v", err)
	}
	status, updated, lastErr := resolveChannelStatusFromStore(store, "twse_sbl", "", "")
	if status != "warn" {
		t.Fatalf("expected warn override, got %q", status)
	}
	if updated == "" {
		t.Fatalf("expected LastFetchAt as updated, got empty string")
	}
	if lastErr == "" || !strings.Contains(lastErr, "quota exhausted") {
		t.Fatalf("expected quota error attached, got %q", lastErr)
	}
}

// TestResolveChannelStatusFromStore_Degraded covers the degraded state:
// the store recorded a degraded fetch (live API failed, cache served).
// The resolver must show "degraded" — between ok and error.
func TestResolveChannelStatusFromStore_Degraded(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := store.Record("tsmc_revenue", "degraded", "cache_fallback"); err != nil {
		t.Fatalf("mark degraded: %v", err)
	}
	status, updated, lastErr := resolveChannelStatusFromStore(store, "tsmc_revenue", "ok", "20260611")
	if status != "degraded" {
		t.Fatalf("expected degraded override, got %q", status)
	}
	if lastErr != "cache_fallback" {
		t.Fatalf("expected cache_fallback reason, got %q", lastErr)
	}
	if updated != "使用快取: cache_fallback" {
		t.Fatalf("expected cache fallback message, got %q", updated)
	}
}

// TestResolveChannelStatusFromStore_OkFromOldFile is the regression test for
// the original bug: the file mtime is 2 days old (would be "warn") but the
// Gateway recorded a successful fetch today. The resolver must report "ok".
func TestResolveChannelStatusFromStore_OkFromOldFile(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := store.Record("twse_capital_flow", "ok", "", WithLastDataAt(time.Now().Add(-2*24*time.Hour))); err != nil {
		t.Fatalf("record: %v", err)
	}
	status, _, _ := resolveChannelStatusFromStore(store, "twse_capital_flow", "warn", "20260609")
	if status != "ok" {
		t.Fatalf("expected ok to override warn (regression for the original divergence bug), got %q", status)
	}
}
