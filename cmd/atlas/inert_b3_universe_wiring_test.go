package main

// inert_b3_universe_wiring_test.go pins the production wiring fixed for issue
// #1944 Batch 3:
//
//   - I25 / N-U1: the universe pipeline and the -build-universe sub-command
//     must receive a real quote provider, not nil and not a mock.
//   - N-U7: the Layer 2.5 risk filter must receive a quote provider too, and it
//     must be the same instance the pipeline uses.
//   - N-U3: the -build-universe status reader must read the canonical snapshot
//     schema and reject anything else loudly instead of printing zeros.

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// inertStubQuoteProvider satisfies monitoring.QuoteProvider without touching
// the network, so wiring can be asserted without a market-data call.
type inertStubQuoteProvider struct{}

func (inertStubQuoteProvider) GetQuotes(context.Context, time.Time, []string) ([]domain.Quote, error) {
	return nil, nil
}

// TestNewUniverseBuilderDeps_WiresRealQuoteProvider asserts the production
// entry point hands the pipeline a gateway-backed quote provider. A nil here is
// exactly the I25 defect: every symbol then fails the volume/price filter and
// the snapshot publishes symbols_ranked=0 as if it were a market verdict.
func TestNewUniverseBuilderDeps_WiresRealQuoteProvider(t *testing.T) {
	cfg := config.Config{WorkDir: t.TempDir()}
	classTree := industry.DefaultClassification()
	adapter := monitoring.AdaptClassificationTree(classTree)

	deps := newUniverseBuilderDeps(cfg, adapter, nil, nil, config.SmartUniverseConfig{}, nil)

	if deps.Quotes == nil {
		t.Fatal("UniverseBuilderDeps.Quotes is nil: the pipeline would publish symbols_ranked=0 (I25)")
	}
	cp, ok := deps.Quotes.(*marketdata.CoverageCompletingProvider)
	if !ok {
		t.Fatalf("Quotes = %T, want *marketdata.CoverageCompletingProvider", deps.Quotes)
	}
	if _, ok := cp.Primary().(*orchestrator.GatewayBackedProvider); !ok {
		t.Errorf("wrapped provider = %T, want *orchestrator.GatewayBackedProvider (the same provider the simulation path uses)", cp.Primary())
	}
	if deps.RiskFilter == nil {
		t.Fatal("RiskFilter is nil: Layer 2.5 would be skipped entirely")
	}
}

// TestNewUniverseBuilderDepsWithQuotes_SharesProviderWithRiskFilter asserts the
// Layer 2.5 risk filter holds a quote provider, and that the provider is the
// same instance passed to the pipeline (N-U7).
//
// The sharing is proved behaviourally: with a provider wired, the liquidity
// rule reports "no quote returned"; only a *nil* provider produces the
// "quote provider not configured" message. Since the injected value is passed
// to both the deps struct and the filter by a single variable, seeing the
// non-nil branch proves both received it.
func TestNewUniverseBuilderDepsWithQuotes_SharesProviderWithRiskFilter(t *testing.T) {
	cfg := config.Config{WorkDir: t.TempDir()}
	classTree := industry.DefaultClassification()
	adapter := monitoring.AdaptClassificationTree(classTree)

	provider := inertStubQuoteProvider{}
	deps := newUniverseBuilderDepsWithQuotes(
		cfg, adapter, nil, nil, config.SmartUniverseConfig{}, nil, provider)

	if deps.Quotes == nil {
		t.Fatal("UniverseBuilderDeps.Quotes is nil although a provider was injected")
	}

	results, err := deps.RiskFilter.Filter([]string{"2330"})
	if err != nil {
		t.Fatalf("RiskExclusionFilter.Filter: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	var liquidity, others int
	for _, rr := range results[0].RuleResults {
		if rr.RuleName != "liquidity" {
			others++
			continue
		}
		liquidity++
		if strings.Contains(rr.Message, "quote provider not configured") {
			t.Errorf("liquidity rule reports no provider, so the filter did not receive the injected one: %q", rr.Message)
		}
		if !strings.Contains(rr.Message, "skipped: no quote returned") {
			t.Errorf("liquidity Message = %q, want the no-quote skip", rr.Message)
		}
		if rr.Severity != "INFO" || !rr.Passed {
			t.Errorf("skipped liquidity rule = %+v, want an INFO pass", rr)
		}
	}
	if liquidity != 1 {
		t.Errorf("liquidity RuleDetail count = %d, want exactly 1 (N-U7: the skip must be visible)", liquidity)
	}
	if others == 0 {
		t.Error("expected the other Layer 2.5 rules to be recorded alongside liquidity")
	}
}

// TestNewUniverseQuoteProvider_IsGatewayBacked asserts the shared factory
// returns the gateway-backed provider rather than the legacy mock.
func TestNewUniverseQuoteProvider_IsGatewayBacked(t *testing.T) {
	p := newUniverseQuoteProvider(config.Config{})
	if p == nil {
		t.Fatal("newUniverseQuoteProvider returned nil")
	}
	cp, ok := p.(*marketdata.CoverageCompletingProvider)
	if !ok {
		t.Fatalf("provider = %T, want *marketdata.CoverageCompletingProvider wrapping the gateway-backed provider", p)
	}
	if _, ok := cp.Primary().(*orchestrator.GatewayBackedProvider); !ok {
		t.Fatalf("wrapped provider = %T, want *orchestrator.GatewayBackedProvider", cp.Primary())
	}
	// issue #1986: the coverage layer must actually carry the first-party
	// whole-market source that closes the 上櫃 gap. An empty source list would
	// silently degrade to TWSE-only coverage (904/1599 measured 2026-09-24).
	sources := cp.MarketWideSources()
	if len(sources) == 0 {
		t.Fatal("coverage layer has no whole-market source: the 上櫃 gap would stay open")
	}
	if sources[0].Name() != "tpex_daily_close" {
		t.Errorf("first whole-market source = %q, want tpex_daily_close", sources[0].Name())
	}
}

// TestBuildUniverseStatus_ReadsCanonicalSnapshot verifies the CLI status reader
// understands the canonical schema written by monitoring.SaveUniverseSnapshot
// (N-U3). Before the fix the CLI wrote and read its own schema, so the two
// directions silently reported zero.
func TestBuildUniverseStatus_ReadsCanonicalSnapshot(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Config{WorkDir: workDir}

	result := &monitoring.UniverseBuildResult{
		SymbolsBuilt:      27,
		SymbolsFiltered:   27,
		SymbolsRanked:     3,
		SymbolsExcluded:   1,
		Timestamp:         time.Now(),
		QuotesStatus:      monitoring.QuotesStatusOK,
		QuotesReturned:    27,
		RankedTrustworthy: true,
	}
	ranked := []monitoring.RankedSymbol{
		{Symbol: "2330", Score: 91, Industry: "半導體"},
		{Symbol: "2317", Score: 88, Industry: "科技"},
		{Symbol: "2454", Score: 84, Industry: "半導體"},
	}
	if err := monitoring.SaveUniverseSnapshot(workDir, result, ranked); err != nil {
		t.Fatalf("SaveUniverseSnapshot: %v", err)
	}

	var buf bytes.Buffer
	restore := redirectLog(&buf)
	defer restore()

	if err := buildUniverseStatus(cfg); err != nil {
		t.Fatalf("buildUniverseStatus: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "no snapshot found") {
		t.Fatalf("status did not find the canonical snapshot:\n%s", out)
	}
	for _, want := range []string{"Total symbols: 27", "Ranked:        3", "status=ok", "2330", "2454"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "NOT trustworthy") {
		t.Errorf("healthy snapshot reported as untrustworthy:\n%s", out)
	}
}

// TestBuildUniverseStatus_RejectsLegacySchema verifies the retired
// (build_time / ranked_count) schema is no longer read as an empty universe:
// the reader errors out instead of printing zeros.
func TestBuildUniverseStatus_RejectsLegacySchema(t *testing.T) {
	workDir := t.TempDir()
	cfg := config.Config{WorkDir: workDir}

	path := monitoring.UniverseSnapshotPath(workDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacy := `{"build_time":"2026-09-24T06:00:23Z","total_symbols":27,"symbols_in_universe":0,"filtered_count":27,"ranked_count":0,"excluded_count":0,"top_symbols":[]}`
	if err := os.WriteFile(path, []byte(legacy), 0o640); err != nil {
		t.Fatalf("write legacy snapshot: %v", err)
	}

	var buf bytes.Buffer
	restore := redirectLog(&buf)
	defer restore()

	err := buildUniverseStatus(cfg)
	if err == nil {
		t.Fatalf("legacy schema accepted silently; output was:\n%s", buf.String())
	}
	if !strings.Contains(err.Error(), "incompatible schema") {
		t.Errorf("error = %v, want it to name the incompatible schema", err)
	}
}

// TestBuildUniverseStatus_MissingSnapshotIsSoft verifies the first-run path
// still returns nil with an explanatory message (no behaviour regression).
func TestBuildUniverseStatus_MissingSnapshotIsSoft(t *testing.T) {
	workDir := t.TempDir()

	var buf bytes.Buffer
	restore := redirectLog(&buf)
	defer restore()

	if err := buildUniverseStatus(config.Config{WorkDir: workDir}); err != nil {
		t.Fatalf("missing snapshot must stay a soft failure, got: %v", err)
	}
	if !strings.Contains(buf.String(), "no snapshot found") {
		t.Errorf("expected an explanatory message, got:\n%s", buf.String())
	}
}

// redirectLog temporarily routes the standard logger into buf.
func redirectLog(buf *bytes.Buffer) func() {
	prev := log.Writer()
	log.SetOutput(buf)
	return func() { log.SetOutput(prev) }
}

// TestMarketdataMockProvider_IsDetectableByPipeline pins the structural
// contract the mock guard relies on: marketdata.MockProvider (the provider
// GatewayBackedProvider falls back to when
// ATLAS_MARKET_DATA_PROVIDER=fugle has no key) must expose IsMock so
// BuildUniverse can refuse to label its quotes a market verdict.
func TestMarketdataMockProvider_IsDetectableByPipeline(t *testing.T) {
	var p monitoring.QuoteProvider = marketdata.NewMockProvider()
	detector, ok := p.(interface{ IsMock() bool })
	if !ok {
		t.Fatalf("%T does not expose IsMock; the universe pipeline would publish fabricated quotes as ranked_trustworthy=true", p)
	}
	if !detector.IsMock() {
		t.Fatalf("%T.IsMock() = false, want true", p)
	}
}
