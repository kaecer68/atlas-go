package monitoring

import "sort"

// KnownIssue describes a channel-level condition that has been investigated and
// documented. The /api/dashboard/channel-health endpoint surfaces these as a
// `known_issue` field on each channel so the dashboard UI can render a
// "known issue" badge instead of a raw error.
//
// Two shapes live in this registry, and the badge must tell them apart:
//
//   - RETIRED (2026-09-29 onward, issue #2134): the upstream is gone for good,
//     the fetch path has been removed, and a replacement input is wired. The
//     channel record reads "inactive", so it neither pages nor looks broken;
//     the badge carries the history plus the reason the channel is off.
//     twse_oddlot (→ twse_capital_flow proxy) and twse_etf (→ Fubon PCF) are
//     both in this shape.
//   - DEAD ALIAS: the canonical channel is healthy and only a leftover runtime
//     ID is stale (taifex-daily).
//
// PR-C (kaecer 2026-08-05 dispatch) added the first entries for two channels
// that had been failing 60+ days without a fix landing: twse_etf and
// twse_oddlot. The early "TWSE rate-limited our outbound IP" hypothesis was
// later FALSIFIED — TWSE removed or repurposed both reports, there is no
// atlas-side fix, and (issue #2134) the two channels were RETIRED instead of
// being left to page forever.
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

	// ── 治理欄位（issue #2138）─────────────────────────────────────────────
	//
	// 這些欄位是「永久損壞 channel 必須退役或修復」判準的機械化輸入。它們刻意
	// **不進 JSON payload**（json:"-"）：對外 API 的 `known_issue` 欄位語意不變，
	// field contract 零變動，也不新增沒有消費者的對外欄位。治理的消費者只有兩個：
	//   · CI 的 cmd/check-channel-consistency（靜態：期限）
	//   · 執行期的 Evaluation（動態：資料齡 × 契約窗）
	// 定義與成本理由見 internal/monitoring/channel_governance.go 與
	// docs/specs/channel-health-status-single-truth-spec.md §8。

	// UpstreamRemovedAt is the RFC3339 date of the FIRST-PARTY evidence that the
	// upstream is permanently unavailable (report removed/repurposed, endpoint
	// gone). Empty means "this entry is not an availability case" — e.g. a dead
	// alias whose canonical channel is healthy (taifex-daily). Leaving it empty
	// is a positive declaration, not an omission: it is how an entry opts out of
	// the retire-or-fix criterion, and it is visible in one field.
	UpstreamRemovedAt string `json:"-"`
	// ReplacementInput names the input that already serves the consumer after
	// the removal (empty = no usable replacement exists). The criterion's second
	// condition is exactly this: without a replacement there is nothing to
	// retire TO, so the channel is a monitor/fix case, not a retire case.
	ReplacementInput string `json:"-"`
	// ActionBy is the RFC3339 deadline by which the retire-or-fix decision must
	// have shipped. Checked by CI: a past deadline with no RetiredAt is a FAILURE
	// (deterministic, date-driven, and intentional — a WARN is what let the
	// original case sit unhandled for 60+ days).
	ActionBy string `json:"-"`
	// RetiredAt is the RFC3339 date the retirement or fix shipped. It is what
	// makes a deadline green again: renewing ActionBy without shipping is allowed
	// (with a documented re-review), but the static report always prints
	// UpstreamRemovedAt so repeated renewals cannot hide how long it has been.
	RetiredAt string `json:"-"`
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
		Title:        "TWSE ETF subscription aggregate report removed (TWT44U → 404) — channel RETIRED (not fetched; input served by Fubon PCF)",
		Description:  "RETIRED (2026-09-29 verification, issue #2134): no atlas code path fetches this channel. Registration is gated on the TWSE_ETF_API_KEY opt-in flag, which production does not set, so the adapter is not in the gateway registry and the record is written as `inactive` at startup (register_adapters.go) — the channel page shows 未啟用 with this reason and no alert rule matches. The consumers are unaffected: ETF net-subscription (RSI-tw subC3) reads the Fubon PCF provider (marketdata.NewFubonETFProvider, wired 2026-08-17). This entry is kept as the historical record and to keep the dashboard badge. Historical investigation follows. TWSE's ETF net-subscription aggregate report (www.twse.com.tw/exchangeReport/TWT44U) was removed. Container-probed 2026-08-10: HTTP 307 → page-not-found.html (404) for any date/params, while STOCK_DAY_ALL returns 200 — NOT an IP block (the earlier 403/rate-limit hypothesis was wrong). No public equivalent for the 申購贖回淨額 aggregate exists as of 2026-08: TWSE OpenAPI opendata (44 datasets) has no ETF-subscription dataset; FinMind has only ETF holdings; the ETFortune portal publicizes NAV/PCF/premium-discount but not net-subscription statistics. NOTE: this is a gap in the aggregate statistic specifically — ETF investor information (NAV, PCF, premium/discount) remains public. Impact: the twse_etf channel cannot serve the full-market aggregate; as of 2026-08-17 subC3 (ETFNetSubscription) consumes the Fubon PCF provider (internal/marketdata/fubon_etf_provider.go — 富邦投信官網申購買回清單, 8 支主力 ETF TWD 加權淨申購) as a directional proxy with real nonzero values (B03 superseded 2026-08-17).",
		DocumentedAt: "2026-08-05T00:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues/1573",

		// 治理（#2138）：永久移除 ＋ 替代已接線 ＋ 已於 2026-08-17 以富邦 PCF 取代
		// （subC3 走 marketdata.NewFubonETFProvider）⇒ 條目帶 RetiredAt，期限綠燈。
		UpstreamRemovedAt: "2026-08-10T00:00:00Z",
		ReplacementInput:  "Fubon PCF（marketdata.NewFubonETFProvider → monitoring.NewETFFetcher；subC3 已接線 2026-08-17）",
		ActionBy:          "2026-08-17T00:00:00Z",
		RetiredAt:         "2026-08-17T00:00:00Z",
	},
	// dash alias of twse_etf — same upstream issue, different channel
	// ID observed at runtime. See note above.
	"twse-etf": {
		Key:          "twse_etf_upstream_60d_dash_alias",
		Title:        "TWSE ETF subscription data: upstream unresponsive (dash alias) — channel RETIRED",
		Description:  "RETIRED with the canonical channel (issue #2134). Same upstream issue as twse_etf. The runtime channel_health record carries the channel ID \"twse-etf\" (dash-separated) instead of \"twse_etf\" (underscore-separated). Like the canonical form, no atlas code path fetches this ID and the ETF net-subscription input comes from the Fubon PCF provider instead; the alias entry exists so the dashboard renders the badge on both spellings until the channel-ID naming inconsistency is investigated and unified upstream.",
		DocumentedAt: "2026-08-05T01:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues/1573",

		// 治理（#2138）：與 canonical 條目同一個上游事件與同一個替代路徑 ⇒ 同步處置。
		UpstreamRemovedAt: "2026-08-10T00:00:00Z",
		ReplacementInput:  "Fubon PCF（marketdata.NewFubonETFProvider → monitoring.NewETFFetcher；subC3 已接線 2026-08-17）",
		ActionBy:          "2026-08-17T00:00:00Z",
		RetiredAt:         "2026-08-17T00:00:00Z",
	},
	"twse_oddlot": {
		Key:          "twse_oddlot_upstream_60d",
		Title:        "TWSE odd-lot trading report removed (BFI84U repurposed) — channel RETIRED 2026-09-29 (input served by twse_capital_flow proxy)",
		Description:  "RETIRED (issue #2134): the fetch path is gone and the record is written status=\"inactive\" at startup (register_adapters.go), so the channel neither fetches nor pages. The retail imbalance input (a6_odd_lot) comes from twse_capital_flow: monitoring.NewOddLotFetcher → oddLotFromCapitalFlow derives a contrarian proxy from the institutional net total and REFUSES (error, no zero value) when the proxy input is unusable, so a6_odd_lot falls back to its neutral parameter instead of a fabricated 0. Why the retirement was needed: the leftover record from the last fetch attempt was status=\"degraded\", which DeriveChannelStatus escalates to ERROR once the data is older than the 48h contract window (E29-3 rule 2b) — measured in production 2026-09-29 (record degraded, last_success 2026-09-07, atlas_channel_health_status=2) it kept ChannelHealthStatusError firing permanently with no possible recovery. Historical investigation follows. TWSE's odd-lot trading report has been removed. Confirmed 2026-08: exchangeReport/BFI84U now returns the 得為融資融券有價證券停券預告表 (margin suspension notice) report with a flat {stat,title,fields,data} shape, and MI_INDEX type=ODDLOT returns an empty data set — no public equivalent remains.",
		DocumentedAt: "2026-08-05T00:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues/2134",

		// 治理（#2138）：判準的 worked example。上游 2026-08 永久移除、替代路徑
		// （twse_capital_flow 代理）已存在，卻因為沒有判準而讓告警連續 firing 直到
		// 2026-09-29 才退役（#2136）。RetiredAt 讓期限轉綠。
		UpstreamRemovedAt: "2026-08-01T00:00:00Z",
		ReplacementInput:  "twse_capital_flow 代理（monitoring.NewOddLotFetcher → oddLotFromCapitalFlow）",
		ActionBy:          "2026-09-29T00:00:00Z",
		RetiredAt:         "2026-09-29T00:00:00Z",
	},
	// dash alias of twse_oddlot — same upstream issue, different channel
	// ID observed at runtime. See note above.
	"twse-oddlot": {
		Key:          "twse_oddlot_upstream_60d_dash_alias",
		Title:        "TWSE odd-lot trading data: upstream schema changed (dash alias) — channel RETIRED 2026-09-29",
		Description:  "RETIRED with the canonical channel (issue #2134): register_adapters.go writes the same \"inactive\" verdict for this dash-separated ID as for twse_oddlot, because an environment that still carries the frozen \"circuit breaker open for channel twse-oddlot\" record would otherwise keep firing on the alias. Same upstream issue as twse_oddlot. The runtime channel_health record carries the channel ID \"twse-oddlot\" (dash-separated) instead of \"twse_oddlot\" (underscore-separated); the retail imbalance input comes from twse_capital_flow (monitoring.NewOddLotFetcher), and the badge is kept on both spellings until the channel-ID naming inconsistency is investigated and unified upstream.",
		DocumentedAt: "2026-08-05T01:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues/2134",

		// 治理（#2138）：與 canonical 條目同一個上游事件與同一個替代路徑 ⇒ 同步處置。
		UpstreamRemovedAt: "2026-08-01T00:00:00Z",
		ReplacementInput:  "twse_capital_flow 代理（monitoring.NewOddLotFetcher → oddLotFromCapitalFlow）",
		ActionBy:          "2026-09-29T00:00:00Z",
		RetiredAt:         "2026-09-29T00:00:00Z",
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

		// 治理（#2138）：**不適用**。這裡沒有「上游被移除」——canonical 的 taifex_daily
		// 健康，只有一個早期版本的 dash 形式 ID 沒被清掉。UpstreamRemovedAt 保持空字串
		// 就是這個宣告（明文寫出「不適用」，避免未來被誤判成該退役的通道）。
		UpstreamRemovedAt: "",
		ReplacementInput:  "",
		ActionBy:          "",
		RetiredAt:         "",
	},

	// BDI / CNBC empty quote (2026-09-23 dispatch). CNBC's quote service keeps
	// answering the Baltic Dry Index symbol `.BADI` with a structurally valid
	// payload that carries no price: no `last` field, open/high/low all
	// "0.00", provider "CNBC Quote Cache". Verified 2026-09-23 against
	// https://quote.cnbc.com/quote-html-webservice/quote.htm?symbols=.BADI&output=json
	// with the production User-Agent (atlas-go/1.0): the SAME request at the
	// SAME minute returns fresh values for .SPX (7764.64), .DJI (51863.69) and
	// .IXIC (27244.278), so this is neither a UA/Akamai block (that returns an
	// HTML "Access Denied") nor a network/atlas problem — it is a data-side
	// upstream outage for one symbol. Root-cause analysis for the gate above:
	// ~/workspace/atlas-notes/05-decisions/2026-09-23-bdi-cnbc-empty-quote-root-cause.md
	//
	// Alternative sources were probed and are unusable: Yahoo `^BDI` and `BDIY`
	// are delisted, stooq has no BDI symbol. The Baltic Exchange's own index is
	// licensed and not publicly scrapable. There is therefore no atlas-side fix;
	// the conscious handling is: provider returns typed marketdata.ErrEmptyQuote
	// → gateway records the channel as "warn" (not "error") → circuit breaker
	// treats it as a no-op (empty quotes must not accumulate failures) →
	// narrative mergeWithPrev keeps the last-known-good Bdi datapoint (3370,
	// 2026-09-20) so downstream consumers (eventdriven sector_predictor BDI
	// factor) keep reading a real value instead of zero.
	//
	// Impact while upstream stays dark: macro snapshots carry BDI = last good
	// value (3370 from 2026-09-20) rather than a fresh print; the channel page
	// shows the warn state with the last-success date, and that badge marks the
	// condition as externally caused so it is not re-investigated every week.
	"bdi": {
		Key:          "bdi_cnbc_empty_quote_2026_09",
		Title:        "BDI: CNBC `.BADI` returns an empty quote (no last price) since 2026-09-20T08:35Z",
		Description:  "CNBC's quote service (quote.cnbc.com/quote-html-webservice/quote.htm?symbols=.BADI&output=json) has answered the Baltic Dry Index symbol `.BADI` with a structurally valid but price-less quote since 2026-09-20T08:35Z: HTTP 200, JSON shape intact, no `last` field, open/high/low all \"0.00\", provider \"CNBC Quote Cache\". Confirmed 2026-09-23 from the atlas host with the production User-Agent (atlas-go/1.0): .SPX=7764.64, .DJI=51863.69, .IXIC=27244.278 came back fresh in the same response window, so this is a per-symbol, data-side upstream outage, NOT an Akamai/UA block (which returns HTML Access Denied) and NOT an atlas-side fault. No usable alternative source exists: Yahoo `^BDI` and `BDIY` are delisted and stooq carries no BDI symbol. Atlas-side handling (2026-09-23 fix): BDIProvider returns typed marketdata.ErrEmptyQuote; the gateway records bdi as status warn (not error, no ChannelHealthStatusError page) and the circuit breaker treats expected-empty as a no-op so repeated empty quotes no longer open the channel breaker (they previously did after 3 consecutive ticks and then served \"circuit breaker open for channel bdi\" while task_liveness.macro_cache_bdi accumulated 800+ consecutive failures). The last-known-good BDI value (3370, 2026-09-20) is preserved in macro snapshots via narrative.mergeWithPrev, so downstream BDI-factor consumers keep a real value. Root-cause notes: ~/workspace/atlas-notes/05-decisions/2026-09-23-bdi-cnbc-empty-quote-root-cause.md",
		DocumentedAt: "2026-09-23T00:00:00Z",
		TrackingURL:  "https://github.com/kaecer68/atlas-go/issues?q=is%3Aissue+bdi_cnbc_empty_quote",

		// 治理（#2138）：**不符退役判準，只登記**。上游（CNBC `.BADI`）確實自
		// 2026-09-20 起永久無價，但**不存在可用的替代來源**（Yahoo 下市、stooq 無此符號、
		// Baltic Exchange 指數有授權）⇒ 判準的第二條件不成立，退役無處可去。正確處置是
		// 現行的 expected-empty 處理（warn ＋ last-known-good）＋ 到期重評估上游是否恢復，
		// 因此 ActionBy 是「重評估期限」而不是「退役期限」。
		UpstreamRemovedAt: "2026-09-20T08:35:00Z",
		ReplacementInput:  "",
		ActionBy:          "2026-12-31T00:00:00Z",
		RetiredAt:         "",
	},
}

// KnownIssueEntry pairs a registry channel ID with its entry. The governance
// report needs the channel ID (that is what an operator acts on) while the
// entry's own Key is the stable identifier of the issue.
type KnownIssueEntry struct {
	ChannelID string
	Issue     KnownIssue
}

// KnownIssueEntries returns every registered known issue with its channel ID,
// sorted by channel ID.
//
// The sorted order is part of the contract: the governance report
// (cmd/check-channel-consistency) prints these entries, and a map iteration
// would make its output — and any test on it — non-deterministic.
func KnownIssueEntries() []KnownIssueEntry {
	ids := make([]string, 0, len(knownIssues))
	for id := range knownIssues {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]KnownIssueEntry, 0, len(ids))
	for _, id := range ids {
		out = append(out, KnownIssueEntry{ChannelID: id, Issue: knownIssues[id]})
	}
	return out
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
