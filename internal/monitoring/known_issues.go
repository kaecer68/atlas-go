package monitoring

// KnownIssue describes a long-standing channel failure that has been
// investigated, documented, and consciously deferred (workaround in place,
// root cause external to atlas). The /api/dashboard/channel-health endpoint
// surfaces these as a `known_issue` field on each channel so the dashboard
// UI can render a "known issue" badge instead of a raw error.
//
// PR-C (kaecer 2026-08-05 dispatch) added this for two channels that have
// been failing 60+ days without a fix landing: twse_etf and twse_oddlot.
// Both go through TWSE upstream endpoints that have either rate-limited
// our outbound IP or changed their response shape — neither root cause
// is on the atlas side, so we surface the issue as "known" rather than
// acting like atlas is broken.
type KnownIssue struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
	// DocumentedAt is the RFC3339 timestamp of when this issue was
	// first recorded. Used by the UI to show "Known issue for 60+ days"
	// as a hint that the team is aware.
	DocumentedAt string `json:"documented_at"`
	// TrackingURL points to the upstream issue tracker or notes file
	// for the team to follow up. Optional.
	TrackingURL string `json:"tracking_url,omitempty"`
}

// knownIssues is the static registry of channel-level known issues.
//
// To add a new known issue:
//  1. Append a KnownIssue entry here.
//  2. Make sure the upstream investigation has been documented in
//     ~/workspace/atlas-notes/05-decisions/ with a clear root-cause
//     analysis showing the failure is NOT on the atlas side, OR has a
//     atlas-side fix that is consciously deferred.
//  3. Update the matching channel's title in the dashboard UI to
//     reference the known issue.
//
// The registry is intentionally static (no config file, no DB) because
// the bar for declaring a "known issue" should be high — it requires
// explicit human sign-off, not a runtime heuristic.
var knownIssues = map[string]KnownIssue{
	// Note (PR-D, 2026-08-05): some channel_health records come in with
	// dash-separated IDs (e.g. "twse-oddlot", "twse-etf") instead of
	// underscore-separated ones (e.g. "twse_oddlot", "twse_etf"). Both
	// refer to the same upstream TWSE issue, so we register both forms
	// with the same description. The dash-separated form is the
	// runtime-observed ID; the underscore-separated form is the
	// provider-returned ID (see internal/marketdata/twse_*_provider.go).
	// A separate issue should investigate why the runtime pipeline
	// produces two different IDs for the same logical channel.
	"twse_etf": {
		Key:          "twse_etf_upstream_60d",
		Title:        "TWSE ETF subscription aggregate report removed (TWT44U → 404)",
		Description:  "TWSE's ETF net-subscription aggregate report (www.twse.com.tw/exchangeReport/TWT44U) was removed. Container-probed 2026-08-10: HTTP 307 → page-not-found.html (404) for any date/params, while STOCK_DAY_ALL returns 200 — NOT an IP block (the earlier 403/rate-limit hypothesis was wrong). No public equivalent for the 申購贖回淨額 aggregate exists as of 2026-08: TWSE OpenAPI opendata (44 datasets) has no ETF-subscription dataset; FinMind has only ETF holdings; the ETFortune portal publicizes NAV/PCF/premium-discount but not net-subscription statistics. NOTE: this is a gap in the aggregate statistic specifically — ETF investor information (NAV, PCF, premium/discount) remains public. Impact: ETFNetSubscription (RSI-TW Part C subC3) is permanently unavailable; subC3 returns IsFallback without contributing (B03, 2026-08-10).",
		DocumentedAt: "2026-08-05T00:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues?q=is%3Aissue+twse_etf",
	},
	// dash alias of twse_etf — same upstream issue, different channel
	// ID observed at runtime. See note above.
	"twse-etf": {
		Key:          "twse_etf_upstream_60d_dash_alias",
		Title:        "TWSE ETF subscription data: upstream unresponsive (dash alias)",
		Description:  "Same upstream issue as twse_etf. The runtime channel_health record carries the channel ID \"twse-etf\" (dash-separated) instead of \"twse_etf\" (underscore-separated). This alias exists so the dashboard can render the known-issue badge on both forms until the channel-ID naming inconsistency is investigated and unified upstream.",
		DocumentedAt: "2026-08-05T01:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues?q=is%3Aissue+twse_etf",
	},
	"twse_oddlot": {
		Key:          "twse_oddlot_upstream_60d",
		Title:        "TWSE odd-lot trading report removed (BFI84U repurposed)",
		Description:  "TWSE's odd-lot trading report has been removed. Confirmed 2026-08: exchangeReport/BFI84U now returns the 得為融資融券有價證券停券預告表 (margin suspension notice) report with a flat {stat,title,fields,data} shape, and MI_INDEX type=ODDLOT returns an empty data set — no public equivalent remains. Workaround (short-term redirect): the a6_odd_lot retail sub-indicator derives a contrarian proxy from twse_capital_flow's total institutional net instead of showing 0 (internal/monitoring/gateway_adapter.go NewOddLotFetcher → oddLotFromCapitalFlow).",
		DocumentedAt: "2026-08-05T00:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues?q=is%3Aissue+twse_oddlot",
	},
	// dash alias of twse_oddlot — same upstream issue, different channel
	// ID observed at runtime. See note above.
	"twse-oddlot": {
		Key:          "twse_oddlot_upstream_60d_dash_alias",
		Title:        "TWSE odd-lot trading data: upstream schema changed (dash alias)",
		Description:  "Same upstream issue as twse_oddlot. The runtime channel_health record carries the channel ID \"twse-oddlot\" (dash-separated) instead of \"twse_oddlot\" (underscore-separated). This alias exists so the dashboard can render the known-issue badge on both forms until the channel-ID naming inconsistency is investigated and unified upstream.",
		DocumentedAt: "2026-08-05T01:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues?q=is%3Aissue+twse_oddlot",
	},

	// PR-G (kaecer 2026-08-05). The runtime channel_health record
	// \"taifex-daily\" (dash-separated) is a dead alias — it has not been
	// the canonical channel ID since taifex_daily was registered with the
	// underscore form in apigateway/register_adapters.go. No atlas code
	// path writes to \"taifex-daily\" today, so its last_success timestamp
	// is frozen at 2026-06-04 (when the alias was last touched) and the
	// record stays at status=\"error\" with a stale \"i/o timeout\" DNS
	// error message even though openapi.taifex.com.tw resolves and
	// responds 200 from inside the atlas container.
	//
	// This entry is the dead-alias counterpart of the twse-etf / twse-oddlot
	// entries above: the underlying taifex_daily channel is healthy (last
	// success 2026-08-05 03:54 UTC), so the dashboard badge exists to
	// mark the dead alias as a non-actionable stale record rather than a
	// real upstream failure.
	"taifex-daily": {
		Key:          "taifex_daily_dead_alias",
		Title:        "TAIFEX daily (dash alias): dead channel ID",
		Description:  "The channel ID \"taifex-daily\" (dash-separated) is a dead alias — atlas registered the canonical channel as \"taifex_daily\" (underscore-separated) in apigateway/register_adapters.go and no code path writes to the dash form. The last_success timestamp is frozen at 2026-06-04 with a stale \"i/o timeout\" DNS error, but openapi.taifex.com.tw currently resolves and returns 200 from inside the atlas container. The canonical taifex_daily channel is healthy. This entry exists so the dashboard renders a known-issue badge on the dead alias instead of a confusing red error. The root cause of the dead alias should be investigated separately (likely an early-version registration that was never cleaned up when the channel was renamed).",
		DocumentedAt: "2026-08-05T03:50:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues?q=is%3Aissue+taifex_daily_alias",
	},
}

// LookupKnownIssue returns the KnownIssue for the given channelID, or
// nil if the channel is not a known-issue channel. Frontend code can
// safely render `if (channel.known_issue) { showBadge(...) }`.
func LookupKnownIssue(channelID string) *KnownIssue {
	if issue, ok := knownIssues[channelID]; ok {
		// Return a copy so callers can't mutate the registry.
		issueCopy := issue
		return &issueCopy
	}
	return nil
}

// KnownIssueChannelIDs returns the set of channel IDs that have a
// known issue declared. Used by ops scripts to enumerate
// "intentionally degraded" channels for status reports.
func KnownIssueChannelIDs() []string {
	ids := make([]string, 0, len(knownIssues))
	for id := range knownIssues {
		ids = append(ids, id)
	}
	return ids
}
