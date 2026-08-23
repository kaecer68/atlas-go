package apigateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestChannelContracts_Coverage verifies every channel in channelIDs() has an
// explicit contract. A missing contract means the channel silently falls back
// to the default (24h refresh, 48h window, live_ping, data_present) — which is
// exactly the "no per-channel contract" failure mode this work removes.
func TestChannelContracts_Coverage(t *testing.T) {
	registry := ChannelContracts()
	missing := 0
	for _, id := range channelIDs() {
		if _, ok := registry.Lookup(id); !ok {
			t.Errorf("channel %q has no ChannelContract", id)
			missing++
		}
	}
	if missing > 0 {
		t.Fatalf("%d channels missing contracts (add them to buildChannelContractRegistry)", missing)
	}
}

// TestChannelContracts_NoExtras verifies the registry contains no contract
// for a channel that is not in channelIDs() — an extra entry means the
// registry drifted from the canonical list.
func TestChannelContracts_NoExtras(t *testing.T) {
	registry := ChannelContracts()
	known := make(map[string]bool, len(channelIDs()))
	for _, id := range channelIDs() {
		known[id] = true
	}
	for id := range registry.All() {
		if !known[id] {
			t.Errorf("contract for %q is not in channelIDs()", id)
		}
	}
}

// TestChannelContracts_ValidateClean verifies the authoritative registry
// passes its own semantic validation (no violations).
func TestChannelContracts_ValidateClean(t *testing.T) {
	violations := ChannelContracts().Validate()
	if len(violations) > 0 {
		for _, v := range violations {
			t.Errorf("violation: %s %s: %s", v.ChannelID, v.Check, v.Detail)
		}
		t.Fatalf("ChannelContracts() has %d validation violations", len(violations))
	}
}

// TestChannelContracts_KeyFields verifies the explicitly-called-out contracts
// from the 2026-08-23 investigation are present and carry the right semantics:
//   - twse_replay: data_freshness (not live ping)
//   - government_broker: file_state + value_nonzero + DegradedOnEmpty (ok 假象 fix)
//   - twse_etf: alias "twse-etf" resolves to canonical "twse_etf"
//   - finmind: explicit freshness expectations
//   - taifex_daily: live_ping
func TestChannelContracts_KeyFields(t *testing.T) {
	registry := ChannelContracts()

	replay, _ := registry.Lookup("twse_replay")
	if replay.HealthSource != HealthSourceDataFreshness {
		t.Errorf("twse_replay HealthSource = %q, want %q (it verifies local CSV freshness, not TWSE liveness)", replay.HealthSource, HealthSourceDataFreshness)
	}
	if !replay.DegradedOnEmpty {
		t.Error("twse_replay should DegradedOnEmpty=true (empty replay dataset is a failure, not ok+empty)")
	}

	broker, _ := registry.Lookup("government_broker")
	if broker.HealthSource != HealthSourceFileState {
		t.Errorf("government_broker HealthSource = %q, want %q", broker.HealthSource, HealthSourceFileState)
	}
	if broker.SuccessCriteria != SuccessCriteriaValueNonzero {
		t.Errorf("government_broker SuccessCriteria = %q, want %q (ok 假象 fix: file must contain non-zero total_net)", broker.SuccessCriteria, SuccessCriteriaValueNonzero)
	}
	if !broker.DegradedOnEmpty {
		t.Error("government_broker should DegradedOnEmpty=true (empty/no_data must surface as degraded, not ok)")
	}

	etf, _ := registry.Lookup("twse_etf")
	canonical, ok := registry.ResolveAlias("twse-etf")
	if !ok {
		t.Fatal("twse-etf alias not resolvable")
	}
	if canonical != "twse_etf" {
		t.Errorf("ResolveAlias(twse-etf) = %q, want twse_etf", canonical)
	}
	if etf.ChannelID != "twse_etf" {
		t.Errorf("twse_etf contract ChannelID = %q", etf.ChannelID)
	}

	finmind, _ := registry.Lookup("finmind")
	if finmind.ExpectedRefresh != 24*time.Hour {
		t.Errorf("finmind ExpectedRefresh = %s, want 24h", finmind.ExpectedRefresh)
	}

	taifex, _ := registry.Lookup("taifex_daily")
	if taifex.HealthSource != HealthSourceLivePing {
		t.Errorf("taifex_daily HealthSource = %q, want %q", taifex.HealthSource, HealthSourceLivePing)
	}
}

// TestDefaultChannelContract verifies the fallback contract values.
func TestDefaultChannelContract(t *testing.T) {
	c := DefaultChannelContract("bogus_channel")
	if c.ChannelID != "bogus_channel" {
		t.Errorf("ChannelID = %q", c.ChannelID)
	}
	if c.ExpectedRefresh != DefaultExpectedRefresh {
		t.Errorf("ExpectedRefresh = %s, want %s", c.ExpectedRefresh, DefaultExpectedRefresh)
	}
	if c.FreshnessWindow != StaleDataThreshold {
		t.Errorf("FreshnessWindow = %s, want %s (inherit global stale threshold)", c.FreshnessWindow, StaleDataThreshold)
	}
	if c.SuccessCriteria != SuccessCriteriaDataPresent {
		t.Errorf("SuccessCriteria = %q, want %q", c.SuccessCriteria, SuccessCriteriaDataPresent)
	}
	if c.HealthSource != HealthSourceLivePing {
		t.Errorf("HealthSource = %q, want %q", c.HealthSource, HealthSourceLivePing)
	}
}

// TestChannelContractRegistry_ContractFallsBackToDefault verifies Contract()
// returns the default for unknown channels instead of a zero-value struct
// (a zero struct would make FreshnessWindow=0 → inherits 48h, but would
// misreport ExpectedRefresh=0 and SuccessCriteria="").
func TestChannelContractRegistry_ContractFallsBackToDefault(t *testing.T) {
	r := NewChannelContractRegistry()
	c := r.Contract("not_registered")
	if c.ChannelID != "not_registered" {
		t.Errorf("ChannelID = %q", c.ChannelID)
	}
	if c.SuccessCriteria != SuccessCriteriaDataPresent {
		t.Errorf("SuccessCriteria = %q, want default", c.SuccessCriteria)
	}
	if c.ExpectedRefresh <= 0 {
		t.Error("ExpectedRefresh should fall back to a positive default")
	}
}

// TestChannelContractRegistry_LookupByAlias verifies Lookup resolves aliases.
func TestChannelContractRegistry_LookupByAlias(t *testing.T) {
	r := NewChannelContractRegistry()
	r.Register(ChannelContract{ChannelID: "twse_etf", Aliases: []string{"twse-etf"}})
	c, ok := r.Lookup("twse-etf")
	if !ok {
		t.Fatal("Lookup(twse-etf) should resolve through alias")
	}
	if c.ChannelID != "twse_etf" {
		t.Errorf("alias-resolved ChannelID = %q, want twse_etf", c.ChannelID)
	}
}

// TestChannelContractRegistry_Validate_DetectsGaps verifies the validator
// flags the failure modes it exists to catch: missing contracts, unknown
// channels, alias collisions, and contradictory criteria.
func TestChannelContractRegistry_Validate_DetectsGaps(t *testing.T) {
	t.Run("missing contract", func(t *testing.T) {
		r := NewChannelContractRegistry()
		v := r.Validate() // empty registry → every canonical channel missing
		found := false
		for _, x := range v {
			if x.Check == "missing_contract" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected missing_contract violation for empty registry")
		}
	})

	t.Run("unknown channel", func(t *testing.T) {
		r := NewChannelContractRegistry()
		r.Register(ChannelContract{ChannelID: "not_a_channel"})
		v := r.Validate()
		found := false
		for _, x := range v {
			if x.Check == "unknown_channel" && x.ChannelID == "not_a_channel" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected unknown_channel violation for not_a_channel")
		}
	})

	t.Run("alias collision with canonical id", func(t *testing.T) {
		r := NewChannelContractRegistry()
		r.Register(ChannelContract{ChannelID: "us_yahoo", Aliases: []string{"twse_margin"}}) // collides with a canonical ID
		v := r.Validate()
		found := false
		for _, x := range v {
			if x.Check == "alias_conflict" && x.ChannelID == "us_yahoo" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected alias_conflict violation when alias equals another canonical channel ID")
		}
	})

	t.Run("live ping cannot verify file_exists", func(t *testing.T) {
		r := NewChannelContractRegistry()
		c := DefaultChannelContract("x")
		c.SuccessCriteria = SuccessCriteriaFileExists // HealthSource stays live_ping
		r.Register(c)
		v := r.Validate()
		found := false
		for _, x := range v {
			if x.Check == "incompatible_criteria" && x.ChannelID == "x" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected incompatible_criteria violation for live_ping + file_exists")
		}
	})

	t.Run("file_state with data_present is weak", func(t *testing.T) {
		r := NewChannelContractRegistry()
		c := DefaultChannelContract("x")
		c.HealthSource = HealthSourceFileState // SuccessCriteria stays data_present
		r.Register(c)
		v := r.Validate()
		found := false
		for _, x := range v {
			if x.Check == "weak_criteria" && x.ChannelID == "x" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected weak_criteria violation for file_state + data_present")
		}
	})

	t.Run("value_nonzero requires degraded flag", func(t *testing.T) {
		r := NewChannelContractRegistry()
		c := DefaultChannelContract("x")
		c.SuccessCriteria = SuccessCriteriaValueNonzero // DegradedOnEmpty stays false
		r.Register(c)
		v := r.Validate()
		found := false
		for _, x := range v {
			if x.Check == "missing_degraded_flag" && x.ChannelID == "x" {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected missing_degraded_flag violation for value_nonzero without DegradedOnEmpty")
		}
	})
}

// ---- EvaluateContractHealth tests ----

// contractTestProvider is a minimal DataProvider used by contract evaluation
// tests. It implements DataStateProvider so callers can toggle data state.
type contractTestProvider struct {
	hs    HealthStatus
	hsErr error
	ds    DataState
	dsErr error
	meta  ChannelMetadata
}

func (p *contractTestProvider) Fetch(context.Context) (*FetchResult, error) {
	return &FetchResult{Data: []byte(`{}`)}, nil
}
func (p *contractTestProvider) HealthCheck(context.Context) (HealthStatus, error) {
	return p.hs, p.hsErr
}
func (p *contractTestProvider) RateLimit() *rate.Limiter                     { return rate.NewLimiter(rate.Inf, 0) }
func (p *contractTestProvider) Metadata() ChannelMetadata                    { return p.meta }
func (p *contractTestProvider) DataState(context.Context) (DataState, error) { return p.ds, p.dsErr }

// nonDataStateProvider is a DataProvider that does NOT implement
// DataStateProvider — contract evaluation must leave its health untouched.
type nonDataStateProvider struct {
	hs HealthStatus
}

func (p *nonDataStateProvider) Fetch(context.Context) (*FetchResult, error) {
	return &FetchResult{Data: []byte(`{}`)}, nil
}
func (p *nonDataStateProvider) HealthCheck(context.Context) (HealthStatus, error) { return p.hs, nil }
func (p *nonDataStateProvider) RateLimit() *rate.Limiter                          { return rate.NewLimiter(rate.Inf, 0) }
func (p *nonDataStateProvider) Metadata() ChannelMetadata                         { return ChannelMetadata{} }

func TestEvaluateContractHealth_ValueNonzero_Satisfied(t *testing.T) {
	contract := ChannelContract{
		ChannelID:       "government_broker",
		SuccessCriteria: SuccessCriteriaValueNonzero,
		HealthSource:    HealthSourceFileState,
		DegradedOnEmpty: true,
	}
	provider := &contractTestProvider{hs: HealthStatus{Status: "ok"}, ds: DataState{Present: true, NonZero: true}}
	got := EvaluateContractHealth(context.Background(), contract, provider, HealthStatus{Status: "ok"})
	if got.Status != "ok" {
		t.Errorf("Status = %q, want ok (reading exists with non-zero total_net)", got.Status)
	}
}

func TestEvaluateContractHealth_ValueNonzero_ZeroValue_Degrades(t *testing.T) {
	contract := ChannelContract{
		ChannelID:       "government_broker",
		SuccessCriteria: SuccessCriteriaValueNonzero,
		HealthSource:    HealthSourceFileState,
		DegradedOnEmpty: true,
	}
	provider := &contractTestProvider{
		hs: HealthStatus{Status: "ok"},
		ds: DataState{Present: true, NonZero: false, Detail: "20260821.json total_net=0"},
	}
	got := EvaluateContractHealth(context.Background(), contract, provider, HealthStatus{Status: "ok"})
	if got.Status != "degraded" {
		t.Errorf("Status = %q, want degraded (zero total_net must not be ok)", got.Status)
	}
	if got.LastError == "" {
		t.Error("degraded result must carry a reason in LastError")
	}
}

func TestEvaluateContractHealth_ValueNonzero_MissingFile_Degrades(t *testing.T) {
	contract := ChannelContract{
		ChannelID:       "government_broker",
		SuccessCriteria: SuccessCriteriaValueNonzero,
		HealthSource:    HealthSourceFileState,
		DegradedOnEmpty: true,
	}
	provider := &contractTestProvider{
		hs: HealthStatus{Status: "ok"},
		ds: DataState{Present: false, Detail: "missing reading file for 20260821"},
	}
	got := EvaluateContractHealth(context.Background(), contract, provider, HealthStatus{Status: "ok"})
	if got.Status != "degraded" {
		t.Errorf("Status = %q, want degraded (no reading file = data never fetched — the ok 假象)", got.Status)
	}
}

func TestEvaluateContractHealth_EmptyButNotDegradedOnEmpty_StaysOk(t *testing.T) {
	contract := ChannelContract{
		ChannelID:       "x",
		SuccessCriteria: SuccessCriteriaFileExists,
		HealthSource:    HealthSourceFileState,
		DegradedOnEmpty: false, // contract explicitly allows ok+empty
	}
	provider := &contractTestProvider{
		hs: HealthStatus{Status: "ok"},
		ds: DataState{Present: false, Detail: "no file"},
	}
	got := EvaluateContractHealth(context.Background(), contract, provider, HealthStatus{Status: "ok"})
	if got.Status != "ok" {
		t.Errorf("Status = %q, want ok (empty is a legitimate outcome when DegradedOnEmpty=false)", got.Status)
	}
}

func TestEvaluateContractHealth_NonOkBase_Passthrough(t *testing.T) {
	contract := ChannelContract{
		ChannelID:       "government_broker",
		SuccessCriteria: SuccessCriteriaValueNonzero,
		HealthSource:    HealthSourceFileState,
		DegradedOnEmpty: true,
	}
	provider := &contractTestProvider{hs: HealthStatus{Status: "error"}, ds: DataState{Present: true, NonZero: true}}
	base := HealthStatus{Status: "error", LastError: "captcha required"}
	got := EvaluateContractHealth(context.Background(), contract, provider, base)
	if got.Status != "error" || got.LastError != "captcha required" {
		t.Errorf("non-ok base must pass through unchanged, got %+v", got)
	}
}

func TestEvaluateContractHealth_DataStateError_Degrades(t *testing.T) {
	contract := ChannelContract{
		ChannelID:       "government_broker",
		SuccessCriteria: SuccessCriteriaValueNonzero,
		HealthSource:    HealthSourceFileState,
		DegradedOnEmpty: true,
	}
	provider := &contractTestProvider{hs: HealthStatus{Status: "ok"}, dsErr: errors.New("permission denied")}
	got := EvaluateContractHealth(context.Background(), contract, provider, HealthStatus{Status: "ok"})
	if got.Status != "degraded" {
		t.Errorf("Status = %q, want degraded (data state unreadable)", got.Status)
	}
}

func TestEvaluateContractHealth_LivePingDataPresent_NoDataCheck(t *testing.T) {
	contract := DefaultChannelContract("us_yahoo") // live_ping + data_present
	provider := &contractTestProvider{hs: HealthStatus{Status: "ok"}, ds: DataState{Present: false}}
	got := EvaluateContractHealth(context.Background(), contract, provider, HealthStatus{Status: "ok"})
	if got.Status != "ok" {
		t.Errorf("Status = %q, want ok (live_ping contracts skip data validation)", got.Status)
	}
}

func TestEvaluateContractHealth_NoDataStateProvider_Passthrough(t *testing.T) {
	contract := ChannelContract{
		ChannelID:       "government_broker",
		SuccessCriteria: SuccessCriteriaValueNonzero,
		HealthSource:    HealthSourceFileState,
		DegradedOnEmpty: true,
	}
	provider := &nonDataStateProvider{hs: HealthStatus{Status: "ok"}}
	got := EvaluateContractHealth(context.Background(), contract, provider, HealthStatus{Status: "ok"})
	if got.Status != "ok" {
		t.Errorf("Status = %q, want ok (provider without DataStateProvider is authoritative)", got.Status)
	}
}
