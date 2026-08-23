package apigateway

import (
	"context"
	"fmt"
	"time"
)

// ChannelContract is the per-channel data-source contract that makes the
// gateway's multi-provider behavior explicit instead of emergent.
//
// Background (2026-08-23): gateway/crossmarket was fixed 11+10 times since
// 2026-06 (TWSE/Yahoo fallback, stale-if-error cache, twse_replay 誤報,
// correlation warmup, degraded 記錄) — all symptoms of "many providers, no
// unified contract". ChannelRegistry (cache_registry.go) held only a
// providers map + RateLimitManager + CircuitBreakerManager; there was no
// per-channel statement of source priority, expected refresh cadence,
// freshness semantics, or what a successful fetch actually guarantees.
//
// A contract answers four questions per channel:
//  1. SourcePriority — what is the fallback chain when the primary fails?
//  2. ExpectedRefresh / FreshnessWindow — how often should data update,
//     and when is a successful fetch considered stale?
//  3. SuccessCriteria — what does a successful fetch actually guarantee
//     (any payload vs non-zero values vs persisted file)?
//  4. HealthSource — what should a health check verify (live API ping vs
//     persisted file state vs data freshness)?
type ChannelContract struct {
	// ChannelID is the canonical channel ID (aligned with channelIDs()).
	ChannelID string `json:"channel_id"`
	// SourcePriority is the fallback chain, highest priority first
	// (e.g. [TWSE, FinMind]).
	SourcePriority []string `json:"source_priority,omitempty"`
	// ExpectedRefresh is the expected update cadence for this channel's data.
	// Used for fresh-vs-stale judgments. Default: 24h.
	ExpectedRefresh time.Duration `json:"expected_refresh"`
	// FreshnessWindow is the maximum age of a successful fetch before the
	// channel is considered stale. Zero inherits StaleDataThreshold (48h).
	FreshnessWindow time.Duration `json:"freshness_window"`
	// SuccessCriteria declares what a fetch must produce to count as success:
	//   - "data_present"  — any non-empty payload is success
	//   - "value_nonzero" — the payload must contain a non-zero value
	//     (e.g. government_broker total_net != 0)
	//   - "file_exists"   — the persisted output file must exist
	SuccessCriteria string `json:"success_criteria"`
	// HealthSource declares what the health check should verify:
	//   - "live_ping"       — the upstream API answers
	//   - "file_state"      — the persisted data file(s) are in a valid state
	//   - "data_freshness"  — the persisted data is fresh, not just present
	HealthSource string `json:"health_source"`
	// DegradedOnEmpty reports whether empty data should be classified as
	// "degraded" instead of "ok" with an empty payload.
	DegradedOnEmpty bool `json:"degraded_on_empty,omitempty"`
	// Aliases are alternate names for the channel (e.g. "twse-etf" vs the
	// canonical "twse_etf") accepted by alias resolution.
	Aliases []string `json:"aliases,omitempty"`
}

// SuccessCriteria values.
const (
	// SuccessCriteriaDataPresent: any non-empty payload is success.
	SuccessCriteriaDataPresent = "data_present"
	// SuccessCriteriaValueNonzero: the payload must contain a non-zero value.
	SuccessCriteriaValueNonzero = "value_nonzero"
	// SuccessCriteriaFileExists: the persisted output file must exist.
	SuccessCriteriaFileExists = "file_exists"
)

// HealthSource values.
const (
	// HealthSourceLivePing: the upstream API answers.
	HealthSourceLivePing = "live_ping"
	// HealthSourceFileState: the persisted data file(s) are in a valid state.
	HealthSourceFileState = "file_state"
	// HealthSourceDataFreshness: the persisted data is fresh, not just present.
	HealthSourceDataFreshness = "data_freshness"
)

const (
	// DefaultExpectedRefresh is the fallback update cadence when a contract
	// does not declare one. The slowest legitimate refresh interval in the
	// system is 24h (etf_nav_refresh, auto_daily_simulation).
	DefaultExpectedRefresh = 24 * time.Hour
)

// DefaultChannelContract returns the contract applied to a channel when the
// registry has no explicit entry. ExpectedRefresh=24h, FreshnessWindow
// inherits StaleDataThreshold (48h), SuccessCriteria=data_present,
// HealthSource=live_ping.
func DefaultChannelContract(channelID string) ChannelContract {
	return ChannelContract{
		ChannelID:       channelID,
		ExpectedRefresh: DefaultExpectedRefresh,
		FreshnessWindow: StaleDataThreshold,
		SuccessCriteria: SuccessCriteriaDataPresent,
		HealthSource:    HealthSourceLivePing,
	}
}

// ValidatesData reports whether the contract requires data-level validation
// beyond a liveness ping. Live-ping channels with plain data_present success
// have nothing to validate at the data layer — the adapter's own HealthCheck
// is authoritative.
func (c ChannelContract) ValidatesData() bool {
	return c.HealthSource != HealthSourceLivePing || c.SuccessCriteria != SuccessCriteriaDataPresent
}

// dataStateSatisfies reports whether a persisted data state meets the
// contract's SuccessCriteria.
func (c ChannelContract) dataStateSatisfies(ds DataState) bool {
	switch c.SuccessCriteria {
	case SuccessCriteriaValueNonzero:
		return ds.Present && ds.NonZero
	default: // file_exists, data_present
		return ds.Present
	}
}

// DataState describes the persisted data of a file-backed channel so that
// contract evaluation can verify data validity (not just liveness).
type DataState struct {
	// Present reports whether the underlying data file(s) exist.
	Present bool `json:"present"`
	// NonZero reports whether the payload contains a non-zero value
	// (e.g. government_broker total_net != 0).
	NonZero bool `json:"non_zero,omitempty"`
	// RecordedAt is the data timestamp (file mtime or embedded date).
	RecordedAt time.Time `json:"recorded_at,omitempty"`
	// Detail is a human-readable description (file path, date, value).
	Detail string `json:"detail,omitempty"`
}

// DataStateProvider is implemented by adapters whose HealthSource is
// file_state or data_freshness and that can report their persisted data
// state. Contract evaluation uses it to verify data validity instead of
// trusting a successful fetch (the government_broker "ok 假象": a fetch
// can succeed while every upstream symbol failed and no reading file was
// written).
type DataStateProvider interface {
	DataState(ctx context.Context) (DataState, error)
}

// EvaluateContractHealth applies the contract's SuccessCriteria / HealthSource
// semantics on top of the adapter's own health result.
//
// base is the adapter's HealthCheck result (or "ok" when called from the
// fetch path right after a successful Fetch). Rules:
//   - Non-ok base statuses pass through unchanged (they are already alerting).
//   - Contracts that do not validate data (live_ping + data_present) pass
//     through — the adapter's HealthCheck is authoritative.
//   - Providers that do not implement DataStateProvider pass through.
//   - When the persisted data state fails SuccessCriteria, an "ok" base is
//     downgraded to "degraded" (if DegradedOnEmpty) or kept "ok" (empty is a
//     legitimate outcome per contract).
func EvaluateContractHealth(ctx context.Context, contract ChannelContract, provider DataProvider, base HealthStatus) HealthStatus {
	if base.Status != "ok" {
		return base
	}
	if !contract.ValidatesData() {
		return base
	}
	dsp, ok := provider.(DataStateProvider)
	if !ok {
		return base
	}
	ds, err := dsp.DataState(ctx)
	if err != nil {
		return degradedHealth(base, fmt.Sprintf("%s data state unreadable: %v", contract.ChannelID, err))
	}
	if contract.dataStateSatisfies(ds) {
		return base
	}
	if contract.DegradedOnEmpty {
		return degradedHealth(base, fmt.Sprintf("%s data missing or empty: %s", contract.ChannelID, ds.Detail))
	}
	return base
}

func degradedHealth(base HealthStatus, reason string) HealthStatus {
	base.Status = "degraded"
	base.LastError = reason
	if base.UpdatedAt == "" {
		base.UpdatedAt = time.Now().Format(time.RFC3339)
	}
	return base
}

// ChannelContractRegistry is the per-channel contract table. It is a static
// configuration map; production code reads it through ChannelContracts().
type ChannelContractRegistry struct {
	contracts map[string]ChannelContract
}

// NewChannelContractRegistry creates an empty registry.
func NewChannelContractRegistry() *ChannelContractRegistry {
	return &ChannelContractRegistry{contracts: make(map[string]ChannelContract)}
}

// Register adds a contract to the registry.
func (r *ChannelContractRegistry) Register(c ChannelContract) {
	r.contracts[c.ChannelID] = c
}

// Size returns the number of registered contracts.
func (r *ChannelContractRegistry) Size() int {
	return len(r.contracts)
}

// Lookup returns the contract for a channel ID or one of its aliases.
func (r *ChannelContractRegistry) Lookup(channelID string) (ChannelContract, bool) {
	if c, ok := r.contracts[channelID]; ok {
		return c, true
	}
	if canonical, ok := r.ResolveAlias(channelID); ok {
		c, ok := r.contracts[canonical]
		return c, ok
	}
	return ChannelContract{}, false
}

// Contract returns the contract for a channel ID, falling back to
// DefaultChannelContract when the registry has no entry. Alias names are
// resolved to their canonical channel first.
func (r *ChannelContractRegistry) Contract(channelID string) ChannelContract {
	if c, ok := r.Lookup(channelID); ok {
		return c
	}
	return DefaultChannelContract(channelID)
}

// ResolveAlias maps an alias name to its canonical channel ID.
// The second return value is false when name is not an alias of any channel.
func (r *ChannelContractRegistry) ResolveAlias(name string) (string, bool) {
	for id, c := range r.contracts {
		for _, a := range c.Aliases {
			if a == name {
				return id, true
			}
		}
	}
	return "", false
}

// All returns a copy of the registry's contracts keyed by channel ID.
func (r *ChannelContractRegistry) All() map[string]ChannelContract {
	out := make(map[string]ChannelContract, len(r.contracts))
	for k, v := range r.contracts {
		out[k] = v
	}
	return out
}

// ContractViolation is a single validation finding produced by Validate().
type ContractViolation struct {
	ChannelID string `json:"channel_id"`
	Check     string `json:"check"`
	Detail    string `json:"detail"`
}

// Validate checks the registry for:
//   - coverage: every channel in channelIDs() has a contract (no missing)
//   - no extras: no contract references a channel outside channelIDs()
//   - alias hygiene: aliases are non-empty, distinct, and do not collide
//     with canonical IDs or other channels' aliases
//   - semantic consistency: HealthSource/SuccessCriteria combinations that
//     contradict the channel's nature (e.g. live_ping + file_exists,
//     file_state + plain data_present) are flagged
func (r *ChannelContractRegistry) Validate() []ContractViolation {
	var violations []ContractViolation

	known := make(map[string]bool, len(channelIDs()))
	for _, id := range channelIDs() {
		known[id] = true
	}

	aliasOwner := make(map[string]string)

	// Coverage: every canonical channel must have an explicit contract.
	for _, id := range channelIDs() {
		if _, ok := r.contracts[id]; !ok {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "missing_contract",
				Detail:    "channel in channelIDs() has no ChannelContract (registry relies on default)",
			})
		}
	}

	for id, c := range r.contracts {
		// No extras.
		if !known[id] {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "unknown_channel",
				Detail:    "contract ChannelID is not in channelIDs()",
			})
		}
		if c.ChannelID != id {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "channel_id_mismatch",
				Detail:    fmt.Sprintf("registry key %q but contract.ChannelID = %q", id, c.ChannelID),
			})
		}

		// Cadence sanity.
		if c.ExpectedRefresh <= 0 {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "invalid_refresh",
				Detail:    "ExpectedRefresh must be > 0",
			})
		}
		if c.FreshnessWindow < 0 {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "invalid_window",
				Detail:    "FreshnessWindow must be >= 0 (0 inherits StaleDataThreshold)",
			})
		}
		if c.FreshnessWindow > 0 && c.ExpectedRefresh > 0 && c.FreshnessWindow < c.ExpectedRefresh {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "window_below_refresh",
				Detail:    fmt.Sprintf("FreshnessWindow (%s) < ExpectedRefresh (%s): data would be stale before the next refresh", c.FreshnessWindow, c.ExpectedRefresh),
			})
		}

		// Criteria / source vocabulary.
		switch c.SuccessCriteria {
		case SuccessCriteriaDataPresent, SuccessCriteriaValueNonzero, SuccessCriteriaFileExists:
		default:
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "invalid_criteria",
				Detail:    fmt.Sprintf("unknown SuccessCriteria %q", c.SuccessCriteria),
			})
		}
		switch c.HealthSource {
		case HealthSourceLivePing, HealthSourceFileState, HealthSourceDataFreshness:
		default:
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "invalid_health_source",
				Detail:    fmt.Sprintf("unknown HealthSource %q", c.HealthSource),
			})
		}

		// Semantic consistency.
		if c.HealthSource == HealthSourceLivePing &&
			(c.SuccessCriteria == SuccessCriteriaFileExists || c.SuccessCriteria == SuccessCriteriaValueNonzero) {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "incompatible_criteria",
				Detail:    fmt.Sprintf("HealthSource=live_ping cannot verify SuccessCriteria=%q (file existence / values need file-state inspection)", c.SuccessCriteria),
			})
		}
		if c.HealthSource == HealthSourceFileState && c.SuccessCriteria == SuccessCriteriaDataPresent {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "weak_criteria",
				Detail:    "file_state channel with data_present criteria: file existence alone cannot prove data validity (the government_broker ok 假象)",
			})
		}
		if c.SuccessCriteria == SuccessCriteriaValueNonzero && !c.DegradedOnEmpty {
			violations = append(violations, ContractViolation{
				ChannelID: id,
				Check:     "missing_degraded_flag",
				Detail:    "value_nonzero criteria requires DegradedOnEmpty=true (zero/empty data must surface as degraded, not ok)",
			})
		}

		// Alias hygiene.
		seen := make(map[string]bool, len(c.Aliases))
		for _, a := range c.Aliases {
			if a == "" {
				violations = append(violations, ContractViolation{
					ChannelID: id,
					Check:     "alias_conflict",
					Detail:    "empty alias",
				})
				continue
			}
			if a == id {
				violations = append(violations, ContractViolation{
					ChannelID: id,
					Check:     "alias_conflict",
					Detail:    fmt.Sprintf("alias %q equals the canonical channel ID", a),
				})
			}
			if known[a] {
				violations = append(violations, ContractViolation{
					ChannelID: id,
					Check:     "alias_conflict",
					Detail:    fmt.Sprintf("alias %q collides with another canonical channel ID", a),
				})
			}
			if seen[a] {
				violations = append(violations, ContractViolation{
					ChannelID: id,
					Check:     "alias_conflict",
					Detail:    fmt.Sprintf("duplicate alias %q within the same channel", a),
				})
			}
			seen[a] = true
			if owner, ok := aliasOwner[a]; ok && owner != id {
				violations = append(violations, ContractViolation{
					ChannelID: id,
					Check:     "alias_conflict",
					Detail:    fmt.Sprintf("alias %q is also used by channel %q", a, owner),
				})
			}
			aliasOwner[a] = id
		}
	}

	return violations
}

// KnownChannelIDs returns the canonical channel ID list (channelIDs()).
// Exported so the check-channel-contracts CLI and external validators can
// compare coverage against the same source of truth the gateway uses.
func KnownChannelIDs() []string {
	return channelIDs()
}

// ChannelContracts returns the authoritative per-channel contract registry.
// Every channel in channelIDs() has an explicit entry (see
// buildChannelContractRegistry); the registry is static configuration.
func ChannelContracts() *ChannelContractRegistry {
	return channelContractRegistry
}

// channelContractRegistry is initialized once at package load.
var channelContractRegistry = buildChannelContractRegistry()

// buildChannelContractRegistry declares the explicit contract for every
// channel in channelIDs(). Channels not listed here fall back to
// DefaultChannelContract — but Validate() flags any such gap, so this table
// must stay in lock-step with channelIDs() (the check-channel-contracts
// CLI and TestChannelContracts_Coverage enforce this).
func buildChannelContractRegistry() *ChannelContractRegistry {
	r := NewChannelContractRegistry()

	// ---- live API channels (HealthSource=live_ping) ----
	live := func(id string, priority []string, refresh time.Duration, aliases ...string) {
		c := DefaultChannelContract(id)
		c.SourcePriority = priority
		c.ExpectedRefresh = refresh
		c.Aliases = aliases
		r.Register(c)
	}

	live("us_yahoo", []string{"Yahoo"}, 24*time.Hour)
	live("twse_capital_flow", []string{"TWSE", "FinMind"}, 24*time.Hour)
	live("fugle", []string{"Fugle"}, time.Hour)
	live("fubon", []string{"Fubon"}, time.Hour)
	live("finmind", []string{"FinMind"}, 24*time.Hour)
	live("frankfurter_fx", []string{"Frankfurter"}, 24*time.Hour)
	live("geopolitical", []string{"GDELT", "RSS"}, 6*time.Hour)
	live("twse_margin", []string{"TWSE", "FinMind"}, 24*time.Hour)
	live("export_statistics", []string{"MOF"}, 24*time.Hour)
	live("tsmc_revenue", []string{"FinMind"}, 24*time.Hour)
	live("geopolitical_taiwan", []string{"RSS"}, 6*time.Hour)
	live("janus_regime", []string{"Compute"}, 6*time.Hour)
	live("tej", []string{"TEJ"}, 24*time.Hour)
	live("exchange_rate", []string{"Frankfurter"}, 24*time.Hour)
	live("sox_index", []string{"SOX"}, 24*time.Hour)
	live("dram_spot_price", []string{"DRAM"}, 24*time.Hour)
	live("twse_sector_index", []string{"TWSE"}, 24*time.Hour)
	live("day_trading", []string{"TWSE"}, 24*time.Hour)
	live("market_volume", []string{"TWSE"}, 24*time.Hour)
	live("bdi", []string{"BDI"}, 24*time.Hour)
	live("taifex_daily", []string{"TAIFEX"}, 24*time.Hour)
	live("taifex_institutional", []string{"TAIFEX"}, 24*time.Hour)
	live("twse_oddlot", []string{"TWSE", "FinMind"}, 24*time.Hour)
	live("twse_insider", []string{"TWSE"}, 24*time.Hour)
	live("us_spx", []string{"Yahoo"}, 24*time.Hour)
	live("us_ndx", []string{"Yahoo"}, 24*time.Hour)
	live("us_dji", []string{"Yahoo"}, 24*time.Hour)
	live("taiex_index", []string{"Yahoo", "TWSE"}, 24*time.Hour)
	live("tw_vol", []string{"Yahoo", "TWSE"}, 24*time.Hour)
	live("us_nvda", []string{"Yahoo"}, 24*time.Hour)
	live("us_aapl", []string{"Yahoo"}, 24*time.Hour)
	live("us_msft", []string{"Yahoo"}, 24*time.Hour)
	live("tsm_adr", []string{"Yahoo"}, 24*time.Hour)

	// twse_etf: upstream TWT44U removed (2026-08-10), registration is
	// opt-in via TWSE_ETF_API_KEY. Contract keeps the historical alias
	// "twse-etf" so operators referencing the old hyphenated name still
	// resolve to the canonical channel.
	live("twse_etf", []string{"TWSE"}, 24*time.Hour, "twse-etf")

	// ---- file-state channels (HealthSource=file_state / data_freshness) ----

	// twse_replay: file-based TWSE replay CSV. HealthCheck reads the CSV
	// and verifies data freshness (not just existence). 72h window aligns
	// with the adapter's <3d ok / <14d warn thresholds.
	c := DefaultChannelContract("twse_replay")
	c.SourcePriority = []string{"TWSE", "FinMind"}
	c.ExpectedRefresh = 24 * time.Hour
	c.FreshnessWindow = 72 * time.Hour
	c.HealthSource = HealthSourceDataFreshness
	c.DegradedOnEmpty = true
	r.Register(c)

	// sector_data: reads data/state/sector_data/sector_data.json. The
	// provider returns an empty snapshot (no error) when the file is
	// missing, so the contract requires file_exists + degraded-on-empty
	// instead of trusting the adapter's ok.
	c = DefaultChannelContract("sector_data")
	c.SourcePriority = []string{"TWSE"}
	c.ExpectedRefresh = 24 * time.Hour
	c.HealthSource = HealthSourceFileState
	c.SuccessCriteria = SuccessCriteriaFileExists
	c.DegradedOnEmpty = true
	r.Register(c)

	// tdcc_equity_dispersion / twse_sbl: stubs (G01/G02) — HealthCheck
	// returns "inactive". Contract declares file_state semantics for when
	// they go live.
	c = DefaultChannelContract("tdcc_equity_dispersion")
	c.SourcePriority = []string{"TDCC"}
	c.HealthSource = HealthSourceFileState
	c.SuccessCriteria = SuccessCriteriaFileExists
	r.Register(c)

	c = DefaultChannelContract("twse_sbl")
	c.SourcePriority = []string{"TWSE"}
	c.HealthSource = HealthSourceFileState
	c.SuccessCriteria = SuccessCriteriaFileExists
	r.Register(c)

	// government_flow: operator-imported daily readings (flat YYYYMMDD.json
	// dir). HealthCheck warns when no reading exists; contract makes the
	// file-state semantics explicit.
	c = DefaultChannelContract("government_flow")
	c.SourcePriority = []string{"Operator", "TWSE"}
	c.HealthSource = HealthSourceFileState
	c.SuccessCriteria = SuccessCriteriaFileExists
	c.DegradedOnEmpty = true
	r.Register(c)

	// government_broker: TWSE broker-level aggregator (C06). The 2026-08-22
	// audit found the channel reported "ok" while data was never fetched —
	// AggregateDate returns (nil, nil) when every upstream symbol fails
	// (e.g. all captcha'd) and the adapter surfaces a no_data stub as a
	// successful fetch. The contract cures the 假象: success requires a
	// persisted reading file with non-zero total_net, and empty data must
	// surface as degraded (implemented via DataStateProvider on the
	// GovernmentBrokerChannelAdapter + EvaluateContractHealth).
	c = DefaultChannelContract("government_broker")
	c.SourcePriority = []string{"TWSE"}
	c.HealthSource = HealthSourceFileState
	c.SuccessCriteria = SuccessCriteriaValueNonzero
	c.DegradedOnEmpty = true
	r.Register(c)

	return r
}
