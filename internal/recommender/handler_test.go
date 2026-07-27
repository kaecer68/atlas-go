package recommender

import (
	"context"
	"errors"
	"net/http"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/methodology"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/subscription"
)

type mockNarrative struct {
	stress float64
	regime string
	err    error
}

func (m *mockNarrative) GetCurrentStressIndex() narrative.TaiwanStressIndex {
	if m.err != nil {
		return narrative.TaiwanStressIndex{}
	}
	return narrative.TaiwanStressIndex{
		Score:  m.stress,
		Regime: m.regime,
	}
}

func (m *mockNarrative) BuildMarketNarrativeData(ctx context.Context) (narrative.MarketNarrativeData, error) {
	return narrative.MarketNarrativeData{}, nil
}

func TestHandleRecommendations(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)

	store, err := subscription.NewStore(dir)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	h := NewHandler(*store, nil).WithDevMode(true)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Tier != string(subscription.TierFree) {
		t.Errorf("expected free tier, got %s", rec.Tier)
	}
}

func TestHandleLoggedInUser(t *testing.T) {
	t.Setenv("ATLAS_DEV_MODE", "true")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)

	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	h := NewHandler(*store, nil).WithDevMode(true)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "premium@test.com")
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Tier != string(subscription.TierPremium) {
		t.Errorf("expected premium (trial), got %s", rec.Tier)
	}
	if rec.Strategies == nil {
		t.Error("premium tier should have strategy recommendations")
	}
}

// T1 RED: P0-2 X-User-Email 偽造 tier 漏洞修復
// DEV_MODE=false (預設) 時, 沒有 JWT + 只帶 X-User-Email 必須被拒絕 (401)
// 不是 free tier fallback, 因為這會讓攻擊者只換 header 就升級 tier
func TestHandleRecommendations_DevModeFallback_Disabled(t *testing.T) {
	os.Unsetenv("ATLAS_DEV_MODE")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)

	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	h := NewHandler(*store, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "premium@test.com")
	// 沒帶 Authorization header → 應該 401
	code, _ := h.HandleRecommendations(req)
	if code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with DEV_MODE disabled, got %d", code)
	}
}

// T1 GREEN 目標: DEV_MODE=true 時, X-User-Email 仍可用 (向後相容 dev/CI)
func TestHandleRecommendations_DevModeFallback_Enabled(t *testing.T) {
	t.Setenv("ATLAS_DEV_MODE", "true")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)

	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	h := NewHandler(*store, nil).WithDevMode(true)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "premium@test.com")
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 with DEV_MODE=true, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Tier != string(subscription.TierPremium) {
		t.Errorf("expected premium tier in DEV_MODE, got %s", rec.Tier)
	}
}

func TestHandleRecommendations_NarrativeIntegration_PopulatesStressIndex(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	mock := &mockNarrative{stress: 15.5, regime: "RISK_ON"}
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil, nil).WithDevMode(true)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Market.StressIndex != 15.5 {
		t.Errorf("StressIndex = %f, want 15.5 (from narrative mock)", rec.Market.StressIndex)
	}
	if rec.Market.Regime != "RISK_ON" {
		t.Errorf("Regime = %q, want %q (from narrative mock)", rec.Market.Regime, "RISK_ON")
	}
}

type mockCapitalFlow struct {
	summary         string
	assessment      capitalflow.CapitalFlowAssessment
	dailyCalls      int
	summaryCalls    int
	assessmentCalls int
}

func (m *mockCapitalFlow) LatestDaily(ctx context.Context) (capitalflow.DailyReport, error) {
	m.dailyCalls++
	return capitalflow.DailyReport{
		Date:          time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC),
		Summary:       m.summary,
		QualityScore:  0.75,
		QualityLabel:  "inflow",
		Resonance:     capitalflow.ResonanceResult{Direction: "aligned"},
		Assessment:    m.assessment,
		DominantActor: capitalflow.ForceForeign,
	}, nil
}

func (m *mockCapitalFlow) Summary(ctx context.Context) (capitalflow.SummaryReport, error) {
	m.summaryCalls++
	return capitalflow.SummaryReport{
		Date:          time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC),
		Summary:       m.summary,
		DominantForce: capitalflow.ForceForeign,
		QualityLabel:  "inflow",
		ResonanceDir:  "aligned",
	}, nil
}

// LatestAssessment exposes the E07 assessment contract for direct adapter
// consumers. HandleRecommendations derives the same assessment from its one
// DailyReport fetch and must not invoke this method separately.
func (m *mockCapitalFlow) LatestAssessment(ctx context.Context) (capitalflow.CapitalFlowAssessment, error) {
	m.assessmentCalls++
	return m.assessment, nil
}

func TestHandleRecommendations_CapitalFlowUsesOneDailyFetchAndWarnsWhileCalibrating(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test-capital-flow-fetch")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	cf := &mockCapitalFlow{
		summary: "法人資金偏多，校準中",
		assessment: capitalflow.CapitalFlowAssessment{
			CalibrationStatus: capitalflow.CalibrationCalibrating,
		},
	}
	h := NewHandlerWithServices(*store, nil, nil, cf, nil, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if cf.dailyCalls != 1 {
		t.Errorf("LatestDaily calls = %d, want 1", cf.dailyCalls)
	}
	if cf.summaryCalls != 0 {
		t.Errorf("Summary calls = %d, want 0 (detail must derive from DailyReport)", cf.summaryCalls)
	}
	if cf.assessmentCalls != 0 {
		t.Errorf("LatestAssessment calls = %d, want 0 (assessment must derive from DailyReport)", cf.assessmentCalls)
	}
	wantWarning := "regime_unavailable; stress_index_unavailable; capital_flow_assessment_calibrating"
	if rec.Warning != wantWarning {
		t.Errorf("Warning = %q, want %q", rec.Warning, wantWarning)
	}
}

type mockEventPredictor struct {
	direction string
}

func (m *mockEventPredictor) PredictToday() (eventdriven.FlowPrediction, error) {
	return eventdriven.FlowPrediction{
		Date:       time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		Direction:  m.direction,
		Confidence: 0.8,
	}, nil
}

func (m *mockEventPredictor) NextNDays(n int) ([]eventdriven.FlowPrediction, error) {
	out := make([]eventdriven.FlowPrediction, n)
	for i := 0; i < n; i++ {
		out[i] = eventdriven.FlowPrediction{
			Date:       time.Date(2026, 7, 8+i+1, 0, 0, 0, 0, time.UTC),
			Direction:  m.direction,
			Confidence: 0.8,
		}
	}
	return out, nil
}

func TestHandleRecommendations_CapitalFlowFromService(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	mock := &mockCapitalFlow{summary: "外資連續買超 3 日，共振 0.85"}
	h := NewHandlerWithServices(*store, nil, nil, mock, nil, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Market.CapitalFlow != "外資連續買超 3 日，共振 0.85" {
		t.Errorf("CapitalFlow = %q, want from capitalFlow mock", rec.Market.CapitalFlow)
	}
	if rec.Market.CapitalFlowDetail == nil {
		t.Fatalf("CapitalFlowDetail should be populated when SummaryReport has QualityLabel, got nil")
	}
	if rec.Market.CapitalFlowDetail.QualityLabel != "inflow" {
		t.Errorf("CapitalFlowDetail.QualityLabel = %q, want %q", rec.Market.CapitalFlowDetail.QualityLabel, "inflow")
	}
	if rec.Market.CapitalFlowDetail.DominantForce != "foreign" {
		t.Errorf("CapitalFlowDetail.DominantForce = %q, want %q", rec.Market.CapitalFlowDetail.DominantForce, "foreign")
	}
	if rec.Market.CapitalFlowDetail.ResonanceDir != "aligned" {
		t.Errorf("CapitalFlowDetail.ResonanceDir = %q, want %q", rec.Market.CapitalFlowDetail.ResonanceDir, "aligned")
	}
	if rec.Market.CapitalFlowDetail.Date == "" {
		t.Errorf("CapitalFlowDetail.Date should not be empty")
	}
}

func TestHandleRecommendations_EventsFromPredictor(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	mock := &mockEventPredictor{direction: "inflow"}
	h := NewHandlerWithServices(*store, nil, nil, nil, mock, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if len(rec.Market.EventsToday) != 1 {
		t.Fatalf("EventsToday len = %d, want 1 (today's prediction)", len(rec.Market.EventsToday))
	}
	if rec.Market.EventsToday[0] != "today:inflow" {
		t.Errorf("EventsToday[0] = %q, want %q", rec.Market.EventsToday[0], "today:inflow")
	}
}

type mockComparisonEngine struct {
	score float64
}

func (m *mockComparisonEngine) GetScore(strategyID string) (float64, error) {
	return m.score, nil
}

func (m *mockComparisonEngine) RankedStrategies() ([]string, error) {
	return []string{"growth", "momentum", "all_weather"}, nil
}

func TestHandleRecommendations_EntrySignalFromComparisonEngine(t *testing.T) {
	t.Setenv("ATLAS_DEV_MODE", "true")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	mock := &mockComparisonEngine{score: 0.85}
	h := NewHandlerWithServices(*store, nil, nil, nil, nil, mock, nil).WithDevMode(true)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "premium@test.com")
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Strategies == nil {
		t.Fatal("premium tier should have strategy recommendations")
	}
	if rec.Strategies.EntrySignal != "Score=0.85 — 排名第1" {
		t.Errorf("EntrySignal = %q, want real (F06) 'Score=0.85 — 排名第1'", rec.Strategies.EntrySignal)
	}
	if rec.Strategies.StopLoss != "-5%" {
		t.Errorf("StopLoss = %q, want hardcoded '-5%%'", rec.Strategies.StopLoss)
	}
}

func TestHandleRecommendations_ServiceFailure_AddsWarning(t *testing.T) {
	t.Setenv("ATLAS_DEV_MODE", "true")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	mock := &failingNarrative{err: errors.New("taiwan_stress_calc transient failure")}
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil, nil).WithDevMode(true)
	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "premium@test.com")
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200 (degraded, not 503) on service failure, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Warning == "" {
		t.Error("expected Warning field populated when narrative service fails")
	}
}

type failingNarrative struct{ err error }

func (f *failingNarrative) GetCurrentStressIndex() narrative.TaiwanStressIndex {
	return narrative.TaiwanStressIndex{}
}

func (f *failingNarrative) BuildMarketNarrativeData(ctx context.Context) (narrative.MarketNarrativeData, error) {
	return narrative.MarketNarrativeData{}, f.err
}

func TestHandleRecommendations_RegimeChange_FiresListener(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)

	var oldR, newR string
	listener := func(oldRegime, newRegime string) {
		oldR, newR = oldRegime, newRegime
	}

	mock := &mockNarrative{stress: 20.0, regime: "RISK_OFF"}
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil, nil).WithDevMode(true).WithRegimeListener(listener)

	t.Setenv("ATLAS_DEV_MODE", "true")
	store.Register("free@test.com", "pass")

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	req.Header.Set("X-User-Email", "free@test.com")
	_, _ = h.HandleRecommendations(req)

	if oldR != "" {
		t.Errorf("first call: oldRegime should be empty, got %q", oldR)
	}
	if newR != "RISK_OFF" {
		t.Errorf("first call: newRegime should be RISK_OFF, got %q", newR)
	}
}

func TestHandleRecommendations_RegimeChange_ConcurrentSafety(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-race")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)

	var fireCount int32
	listener := func(oldRegime, newRegime string) {
		atomic.AddInt32(&fireCount, 1)
	}

	mock := &mockNarrative{stress: 20.0, regime: "RISK_ON"}
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil, nil).
		WithRegimeListener(listener).
		WithDevMode(true)
	store.Register("free@test.com", "pass")

	// detectRegimeChange no-ops when lastSeenRegime is ""; pre-populate
	// so concurrent goroutines see a real regime transition.
	h.lastSeenRegime = "RISK_OFF"

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
			req.Header.Set("X-User-Email", "free@test.com")
			_, _ = h.HandleRecommendations(req)
		}()
	}
	wg.Wait()

	// reproduce-flagged: racy read/write in detectRegimeChange lets N goroutines
	// observe RISK_OFF simultaneously and each fire the listener
	count := atomic.LoadInt32(&fireCount)
	if count != 1 {
		t.Errorf("listener fired %d times, want 1 (race condition)", count)
	}
}

// TestHandlerRecommendations_AssessmentUnchanged — spec §9.5 / CF-INV-08.
//
// The recommender handler must remain stable across consecutive capital-flow
// reads. HandleRecommendations derives the assessment from each DailyReport
// and must produce deeply equal responses for identical provider data. The
// direct LatestAssessment call below only verifies the provider contract; the
// handler itself must not call LatestAssessment separately.
func TestHandlerRecommendations_AssessmentUnchanged(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test-assessment")
	defer os.RemoveAll(dir)

	store, _ := subscription.NewStore(dir)
	cf := &mockCapitalFlow{summary: "外資連三買超 800 億"}
	h := NewHandlerWithServices(*store, nil, nil, cf, nil, nil, nil).WithDevMode(true)

	// Sanity: the mock's direct assessment path returns the Go zero value,
	// whose empty CalibrationStatus keeps EligibleForAutomation closed.
	assessment, err := cf.LatestAssessment(t.Context())
	if err != nil {
		t.Fatalf("LatestAssessment: %v", err)
	}
	if !reflect.DeepEqual(assessment, capitalflow.CapitalFlowAssessment{}) {
		t.Errorf("LatestAssessment returned non-zero assessment %+v; stub contract is zero-value", assessment)
	}

	// Two handler calls must be deeply equal — neither must regress
	// because assessment status changed between calls.
	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	_, first := h.HandleRecommendations(req)
	_, second := h.HandleRecommendations(req)

	firstRec, ok := first.(TierRecommendation)
	if !ok {
		t.Fatalf("first: expected TierRecommendation, got %T", first)
	}
	secondRec, ok := second.(TierRecommendation)
	if !ok {
		t.Fatalf("second: expected TierRecommendation, got %T", second)
	}
	if !recommendationsDeepEqual(firstRec, secondRec) {
		t.Errorf("recommendation drifted between two consecutive reads (CF-INV-08):\n  first=%+v\n  second=%+v",
			firstRec, secondRec)
	}
}

// recommendationsDeepEqual compares the recommendation-shaped fields
// that the recommender handler emits. We avoid reflect.DeepEqual on the
// whole struct because TierRecommendation embeds an `any` Signals
// field which may carry non-comparable types across call paths; this
// helper keeps the assertion scope to the CF-INV-08 contract.
func recommendationsDeepEqual(a, b TierRecommendation) bool {
	if a.Tier != b.Tier || a.Warning != b.Warning {
		return false
	}
	if a.Market.Regime != b.Market.Regime ||
		a.Market.RegimeLabel != b.Market.RegimeLabel ||
		a.Market.StressIndex != b.Market.StressIndex ||
		a.Market.CapitalFlow != b.Market.CapitalFlow {
		return false
	}
	if (a.Market.CapitalFlowDetail == nil) != (b.Market.CapitalFlowDetail == nil) {
		return false
	}
	if a.Market.CapitalFlowDetail != nil && b.Market.CapitalFlowDetail != nil {
		if a.Market.CapitalFlowDetail.QualityLabel != b.Market.CapitalFlowDetail.QualityLabel ||
			a.Market.CapitalFlowDetail.QualityScore != b.Market.CapitalFlowDetail.QualityScore ||
			a.Market.CapitalFlowDetail.ResonanceDir != b.Market.CapitalFlowDetail.ResonanceDir ||
			a.Market.CapitalFlowDetail.DominantForce != b.Market.CapitalFlowDetail.DominantForce ||
			a.Market.CapitalFlowDetail.Date != b.Market.CapitalFlowDetail.Date {
			return false
		}
	}
	if len(a.Market.EventsToday) != len(b.Market.EventsToday) {
		return false
	}
	for i := range a.Market.EventsToday {
		if a.Market.EventsToday[i] != b.Market.EventsToday[i] {
			return false
		}
	}
	if (a.Strategies == nil) != (b.Strategies == nil) {
		return false
	}
	if a.Strategies != nil && b.Strategies != nil {
		if a.Strategies.Active != b.Strategies.Active ||
			a.Strategies.EntrySignal != b.Strategies.EntrySignal ||
			a.Strategies.StopLoss != b.Strategies.StopLoss {
			return false
		}
	}
	return true
}

// TestBuildRankedBrief verifies that RankedBrief preserves ranked order,
// maps YAML strategy IDs to correct categories (defensive/aggressive/tactical),
// and leaves non-YAML IDs with an empty category.
func TestBuildRankedBrief(t *testing.T) {
	rules := config.TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	advisor := methodology.NewAdvisor(rules)

	// "growth" is YAML (aggressive), "defensive" is NOT a YAML strategy
	// (it lives in strategy/registry.go, not methodology_rules.yaml).
	ranked := []string{"growth", "defensive"}

	briefs := buildRankedBrief(ranked, advisor)

	if len(briefs) != len(ranked) {
		t.Fatalf("len(RankedBrief) = %d, want %d", len(briefs), len(ranked))
	}

	// Order must match.
	if briefs[0].ID != "growth" {
		t.Errorf("RankedBrief[0].ID = %q, want growth", briefs[0].ID)
	}
	if briefs[0].Category != "aggressive" {
		t.Errorf("RankedBrief[0].Category = %q, want aggressive", briefs[0].Category)
	}

	if briefs[1].ID != "defensive" {
		t.Errorf("RankedBrief[1].ID = %q, want defensive", briefs[1].ID)
	}
	if briefs[1].Category != "" {
		t.Errorf("RankedBrief[1].Category = %q, want empty (not in YAML)", briefs[1].Category)
	}
}

// TestBuildRankedBrief_NilAdvisor verifies that a nil advisor produces
// empty categories for all entries (no panic).
func TestBuildRankedBrief_NilAdvisor(t *testing.T) {
	ranked := []string{"growth", "momentum"}
	briefs := buildRankedBrief(ranked, nil)

	if len(briefs) != 2 {
		t.Fatalf("len(RankedBrief) = %d, want 2", len(briefs))
	}
	for i, b := range briefs {
		if b.Category != "" {
			t.Errorf("RankedBrief[%d].Category = %q with nil advisor, want empty", i, b.Category)
		}
	}
}

// TestBuildRankedBrief_DefensiveNotInFallback verifies the ghost-id fix:
// "defensive" is NOT in the YAML six-strategy set and must not appear
// in fallback ranked lists.
func TestBuildRankedBrief_DefensiveNotInYAML(t *testing.T) {
	rules := config.TryLoadMethodologyRules("../../configs/methodology_rules.yaml")
	advisor := methodology.NewAdvisor(rules)

	cat := advisor.StrategyCategory("defensive")
	if cat != "" {
		t.Errorf("StrategyCategory(defensive) = %q, want empty — defensive is NOT a YAML six-strategy ID", cat)
	}
}
