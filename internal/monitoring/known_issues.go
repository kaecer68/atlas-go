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
	"twse_etf": {
		Key:          "twse_etf_upstream_60d",
		Title:        "TWSE ETF subscription data: upstream unresponsive",
		Description:  "TWSE's ETF subscription endpoint (www.twse.com.tw / ETF) has been returning 403/timeout for 60+ days. We are likely rate-limited at the IP level after a spike of backfill traffic. Workaround: skip ETF flow data in agent recommendations; the rest of the simulation pipeline operates without it.",
		DocumentedAt: "2026-08-05T00:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues?q=is%3Aissue+twse_etf",
	},
	"twse_oddlot": {
		Key:          "twse_oddlot_upstream_60d",
		Title:        "TWSE odd-lot trading data: upstream schema changed",
		Description:  "TWSE's odd-lot trading data endpoint (www.twse.com.tw / oddlot) silently changed its response schema in 2026-Q2. Our parser at internal/marketdata/twse_oddlot_provider.go expects the legacy shape and returns zero rows. Workaround: odd-lot flow is excluded from retail signal calculations; primary flow data uses the twse_capital_flow channel which is healthy.",
		DocumentedAt: "2026-08-05T00:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues?q=is%3Aissue+twse_oddlot",
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
