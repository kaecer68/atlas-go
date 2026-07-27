package service

import "github.com/kaecer68/atlas-go/internal/apigateway"

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
// Behavior:
//   - healthStore == nil → returns fileStatus / fileUpdated (no override)
//   - no record for channelID → returns fileStatus / fileUpdated
//   - record.Status == "ok" → returns "ok" with record.LastFetchAt
//   - record.Status == "error" → returns "error" with formatted LastError
//   - any other record.Status (e.g. "warn") → returns fileStatus with record.LastError attached
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

	switch rec.Status {
	case "ok":
		// Last fetch succeeded — channel is healthy regardless of data age.
		return "ok", rec.LastFetchAt, ""
	case "degraded":
		// Last fetch failed but cache has valid data — between ok and error.
		return "degraded", "使用快取: " + rec.LastError, rec.LastError
	case "error":
		// Last fetch failed — report the actual error.
		return "error", "上次失敗: " + rec.LastError, rec.LastError
	default:
		return fileStatus, fileUpdated, rec.LastError
	}
}
