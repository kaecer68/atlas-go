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
	if err := store.Record("twse_capital_flow", "error", "rate limit exceeded"); err != nil {
		t.Fatalf("record error: %v", err)
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

// TestResolveChannelStatusFromStore_OtherStatus covers store statuses the
// resolver has no dedicated mapping for (e.g. "inactive"). FileStatus should
// pass through, but LastError from the record should be attached for
// visibility.
func TestResolveChannelStatusFromStore_OtherStatus(t *testing.T) {
	store := apigateway.NewChannelHealthStoreWithPool(t.TempDir(), nil)
	if err := store.Record("ch-inactive", "inactive", "channel disabled"); err != nil {
		t.Fatalf("record inactive: %v", err)
	}
	status, updated, lastErr := resolveChannelStatusFromStore(store, "ch-inactive", "ok", "20260611")
	if status != "ok" {
		t.Fatalf("expected fileStatus to pass through for unmapped store status, got %q", status)
	}
	if updated != "20260611" {
		t.Fatalf("expected fileUpdated to pass through, got %q", updated)
	}
	if lastErr != "channel disabled" {
		t.Fatalf("expected store LastError to be attached, got %q", lastErr)
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
