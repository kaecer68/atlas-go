package service

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
)

// resolveChannelStatusFromStore merges the Gateway health store record with a
// file-age-based health check. The health store takes priority because it
// reflects the actual result of the last fetch attempt. File-age alone can
// produce false "待更新" warnings on weekends/holidays when no fetch is
// expected but the channel itself is healthy.
//
// This is the shared resolver used by both SystemService (home page overview)
// and DataChannelService (data channel management page) so the two pages
// always agree on channel status.
//
// 2026-09-24 (fix/20260924-channel-status-truth): the record's status is a
// VERDICT and the store's verdict is only final after the contract freshness
// window is applied. Previously this function returned "ok" for any record
// whose status was "ok", so a channel whose last successful fetch was 17 days
// old (twse_oddlot: upstream removed) still read "正常" on /admin/datachannels
// while the health summary logged "stale" for the same channel at the same
// second. The window is now applied through the single shared judgment
// apigateway.DeriveChannelStatus, the same function behind
// Gateway.Summary / StatusSummary / the dashboard payload / the metrics gauge /
// the channel_health DB mirror — one channel, one verdict.
//
// Behavior:
//   - healthStore == nil → returns fileStatus / fileUpdated (no override)
//   - no record for channelID → returns fileStatus / fileUpdated
//   - record ok AND within the contract window → "ok" with record.LastFetchAt
//   - record ok AND older than the contract window → "stale" with the reason
//   - record.Status warn / degraded / error / inactive → that status with the
//     record's error text (inactive is no longer treated as unmapped: falling
//     through to an empty fileStatus rendered 未知 while the record said 未啟用)
//   - any other record.Status → returns fileStatus with record.LastError attached
func resolveChannelStatusFromStore(
	healthStore *apigateway.ChannelHealthStore,
	channelID string,
	fileStatus string,
	fileUpdated string,
) (status, updated, lastError string) {
	if healthStore == nil {
		return fileStatus, fileUpdated, ""
	}
	rec := healthStore.Get(channelID)
	if rec == nil {
		return fileStatus, fileUpdated, ""
	}

	now := time.Now()
	contract := apigateway.ChannelContracts().Contract(channelID)
	if derived := apigateway.DeriveChannelStatus(rec, contract, now); derived != rec.Status {
		// Freshness verdict (ok → stale): report the derived status with the
		// record's real fetch time and an explicit reason, never a bare "ok".
		return derived, rec.LastFetchAt, apigateway.DeriveChannelStatusReason(rec, contract, now)
	}

	switch rec.Status {
	case "ok":
		// Last fetch succeeded and the data is within the contract window.
		return "ok", rec.LastFetchAt, ""
	case "warn":
		// Transient waiting state (e.g. FinMind daily quota exhausted — the
		// Gateway records ErrQuotaExhausted as warn, not error). The
		// registered-channel fallback passes an empty fileStatus, so without
		// this case a warn record displayed as "未知" on /admin/datachannels.
		// Report "warn" (待更新) with the stored error attached; the next
		// scheduled fetch success flips the record to ok and clears it.
		return "warn", rec.LastFetchAt, rec.LastError
	case "degraded":
		// Last fetch failed but cache has valid data — between ok and error.
		return "degraded", "使用快取: " + rec.LastError, rec.LastError
	case "error":
		// Last fetch failed — report the actual error.
		return "error", "上次失敗: " + rec.LastError, rec.LastError
	case "stale":
		// A record written as stale directly (e.g. adapter_government_broker
		// marks an unusable snapshot "stale") must never fall through to the
		// file-age fallback — that silently dropped the record's verdict and
		// rendered the channel as 未知.
		return "stale", rec.LastFetchAt, rec.LastError
	case "inactive":
		// Registered but intentionally off (twse_etf: upstream removed, opt-in
		// via key). Report the record's own verdict with its reason text so the
		// page shows 未啟用 + why, instead of an empty status rendered as 未知.
		return "inactive", rec.LastFetchAt, rec.LastError
	default:
		return fileStatus, fileUpdated, rec.LastError
	}
}
