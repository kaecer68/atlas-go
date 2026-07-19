package recommender

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	monitoringservice "github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/strategy"
	"github.com/kaecer68/atlas-go/internal/subscription"
)

// cannedMacroProvider returns a fixed MacroDataSnapshot on every FetchSnapshot
// call. Implements marketdata.MacroDataProvider. Used so the real
// capitalflow.Service and narrative service exercise their full pipelines
// (not the nil-safe fallback path) without depending on external data.
type cannedMacroProvider struct {
	snap marketdata.MacroDataSnapshot
}

func (c *cannedMacroProvider) Name() string { return "canned" }

func (c *cannedMacroProvider) FetchSnapshot(ctx context.Context) (marketdata.MacroDataSnapshot, error) {
	return c.snap, nil
}

// buildRISKOFFSnapshot constructs a snapshot with foreign net selling + dealer
// buying + elevated VIX. Resonance rules will classify foreign+dealer as
// adversarial → coefficient = 0.5, direction = "mixed".
func buildRISKOFFSnapshot(now int64) marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		VIX:                marketdata.MacroDataPoint{Symbol: "VIX", Value: 28.5, ChangePct: 0.18, Timestamp: now},
		US10Y:              marketdata.MacroDataPoint{Symbol: "US10Y", Value: 4.85, ChangePct: 0.04, Timestamp: now},
		DXY:                marketdata.MacroDataPoint{Symbol: "DXY", Value: 107.2, ChangePct: 0.012, Timestamp: now},
		USD_TWD:            marketdata.MacroDataPoint{Symbol: "USD_TWD", Value: 32.6, ChangePct: 0.008, Timestamp: now},
		ForeignInvestorNet: marketdata.MacroDataPoint{Symbol: "FI", Value: -180.0, ChangePct: -1.2, Timestamp: now},
		DomesticFundNet:    marketdata.MacroDataPoint{Symbol: "DII", Value: 25.0, ChangePct: 0.05, Timestamp: now},
		DealerNet:          marketdata.MacroDataPoint{Symbol: "DEALER", Value: 155.0, ChangePct: 0.3, Timestamp: now},
		SOXIndex:           marketdata.MacroDataPoint{Symbol: "SOX", Value: 4850.0, ChangePct: -0.025, Timestamp: now},
		Oil:                marketdata.MacroDataPoint{Symbol: "OIL", Value: 92.0, ChangePct: 0.06, Timestamp: now},
		Gold:               marketdata.MacroDataPoint{Symbol: "GOLD", Value: 2450.0, ChangePct: 0.015, Timestamp: now},
		DataStatus:         "ok",
		RecordedAt:         now,
	}
}

// realWiredDeps constructs the same producer set that cmd/atlas/wire_recommender.go
// builds in production. Returns the 4 real services wrapped in the recommender
// adapter functions, ready to pass to NewHandlerWithServices.
//
// This replaces PR #998's mock-based helpers. The integration smoke test's
// job is to verify the wire doesn't break the handler — NOT to assert
// specific business values, which are covered by unit tests in
// handler_test.go and adapters_test.go.
func realWiredDeps(t *testing.T) (NarrativeProvider, CapitalFlowProvider, EventPredictor, ComparisonEngine) {
	t.Helper()

	workDir := t.TempDir()

	mp := &cannedMacroProvider{snap: buildRISKOFFSnapshot(time.Now().Unix())}

	// 1. capitalflow: real service backed by canned macro provider.
	cfsvc := capitalflow.NewService(mp, 0, nil)
	capflowAdapter := NewCapitalFlowFunc(cfsvc.LatestDaily, cfsvc.Summary, cfsvc.LatestAssessment)

	// 2. event-driven: real predictor with the standard event calendar.
	// RefreshEvents evaluates the built-in rule set. Custom event injection
	// is not part of the public API — the test verifies wiring + response
	// shape, not content.
	cal := industry.NewEventCalendar()
	cal.RefreshEvents(time.Now())
	predictor := eventdriven.NewPredictor(cal)
	eventAdapter := NewEventPredictorAdapter(predictor)

	// 3. narrative: real engine + report generator + monitoring service.
	narrativeEng := narrative.NewNarrativeEngine()
	reportGen := narrative.NewReportGenerator()
	narrativeSvc := monitoringservice.NewNarrativeService(workDir, narrativeEng, reportGen)
	if narrativeSvc == nil {
		t.Fatal("NewNarrativeService returned nil")
	}
	narrativeSvc.SetMacroProvider(mp)
	narrativeAdapter := NewNarrativeAdapterFunc(
		narrativeSvc.GetCurrentStressIndex,
		narrativeSvc.BuildMarketNarrativeData,
	)

	// 4. comparison engine: real instance, 30-day window. No recorded trades
	// → GetScore returns 0. This exercises the nil-safe fallback for strategy
	// while still verifying the adapter wire works.
	cmpEng := strategy.NewComparisonEngine(30, nil)
	strategyAdapter := NewComparisonEngineAdapter(cmpEng)

	return narrativeAdapter, capflowAdapter, eventAdapter, strategyAdapter
}

// TestE2E_RecommendationsWiredFlow exercises the full HTTP flow against
// /api/recommendations with the SAME producer set that production wires
// via cmd/atlas/wire_recommender.go. This replaces PR #998's mock-based
// TestE2E_RecommendationsEndpoint as the canonical E2E for /api/recommendations.
//
// Scope: integration smoke test.
//   - Verifies the 4 producers (capitalflow, eventdriven, narrative, strategy)
//     can be wired through the production adapter functions and the handler
//     serves requests without crashing.
//   - Verifies the response shape (tier, market, strategies, warning).
//   - Does NOT assert specific business values (regime, stress index, capital
//     flow string) — those are tested in handler_test.go and adapters_test.go
//     where inputs are deterministic.
//
// Companion: PR #998's mock-based e2e_test.go remains in the repo as the
// fast smoke variant; this file is the integration-confidence test that
// exercises real wired services.
func TestE2E_RecommendationsWiredFlow(t *testing.T) {
	dir, err := os.MkdirTemp("", "rec-e2e-wired")
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	defer os.RemoveAll(dir)

	store, err := subscription.NewStore(dir)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	_, _ = store.Register("premium@test.com", "pass")

	narrative, capflow, evts, strategy := realWiredDeps(t)

	handler := NewHandlerWithServices(*store, nil, narrative, capflow, evts, strategy).WithDevMode(true)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/recommendations", func(w http.ResponseWriter, r *http.Request) {
		code, data := handler.HandleRecommendations(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if data != nil {
			_ = json.NewEncoder(w).Encode(data)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		name       string
		userHeader string
		wantTier   string
	}{
		{name: "premium tier via dev X-User-Email", userHeader: "premium@test.com", wantTier: "premium"},
		{name: "free tier anonymous", userHeader: "", wantTier: "free"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/recommendations", nil)
			if tt.userHeader != "" {
				req.Header.Set("X-User-Email", tt.userHeader)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("GET: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status: got %d, want 200", resp.StatusCode)
			}

			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}

			// Tier gate
			if got := body["tier"]; got != tt.wantTier {
				t.Errorf("tier: got %v, want %s", got, tt.wantTier)
			}

			// Response shape — must always have market. strategies is
			// premium-tier-only (free/registered don't get strategy payload).
			mustHaveKey(t, body, "market")
			if tt.wantTier == "premium" {
				mustHaveKey(t, body, "strategies")
			}

			// Market shape — must be a map.
			market, ok := body["market"].(map[string]any)
			if !ok {
				t.Fatalf("market: not a map (got %T)", body["market"])
			}
			mustHaveKey(t, market, "regime")
			mustHaveKey(t, market, "events_today")

			// Wired-path negative assertions: ensure the response didn't come
			// from the nil-safe fallback (which returns "資金流向均衡" / "NEUTRAL").
			if cf, ok := market["capital_flow"].(string); ok {
				if cf == "資金流向均衡" {
					t.Errorf("capital_flow equals nil-safe fallback — wired path not exercised")
				}
			}
			if regime, ok := market["regime"].(string); ok && tt.wantTier != "free" {
				if regime == "NEUTRAL" {
					t.Errorf("regime equals nil-safe fallback — wired path not exercised")
				}
			}

			// events_today must be an array (shape from real predictor).
			if _, ok := market["events_today"].([]any); !ok {
				t.Errorf("events_today: not an array (got %T) — predictor wire may be broken", market["events_today"])
			}

			// Free tier: warning populated.
			if tt.wantTier == "free" {
				if warn, ok := body["warning"].(string); !ok || warn == "" {
					t.Errorf("free tier: expected non-empty warning, got %v", body["warning"])
				}
			}

			// Premium tier: strategies must be non-nil map with the keys the
			// handler actually emits. The struct has an `Available` field but
			// it's JSON-skipped.
			if tt.wantTier == "premium" {
				if strats, ok := body["strategies"].(map[string]any); !ok {
					t.Errorf("premium tier: strategies not a map (got %T)", body["strategies"])
				} else {
					mustHaveKey(t, strats, "active")
					mustHaveKey(t, strats, "ranked")
					mustHaveKey(t, strats, "entry_signal")
					mustHaveKey(t, strats, "stop_loss")
				}
			}

			// Diagnostic: full body under -v.
			t.Logf("body: %s", mustMarshal(t, body))
		})
	}
}

// mustHaveKey fails the test if key k is missing from map m.
func mustHaveKey(t *testing.T, m map[string]any, k string) {
	t.Helper()
	if _, ok := m[k]; !ok {
		keys := make([]string, 0, len(m))
		for key := range m {
			keys = append(keys, key)
		}
		t.Errorf("missing key %q in map (got keys: %v)", k, keys)
	}
}

// mustMarshal JSON-encodes a value for t.Logf diagnostic output.
func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("<marshal error: %v>", err)
	}
	return string(b)
}
