package recommender

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
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
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil).WithDevMode(true)

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
	summary string
}

func (m *mockCapitalFlow) LatestDaily(ctx context.Context) (capitalflow.DailyReport, error) {
	return capitalflow.DailyReport{
		Date:    time.Now(),
		Summary: m.summary,
	}, nil
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
	h := NewHandlerWithServices(*store, nil, nil, mock, nil, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if rec.Market.CapitalFlow != "外資連續買超 3 日，共振 0.85" {
		t.Errorf("CapitalFlow = %q, want from capitalFlow mock", rec.Market.CapitalFlow)
	}
}

func TestHandleRecommendations_EventsFromPredictor(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	mock := &mockEventPredictor{direction: "inflow"}
	h := NewHandlerWithServices(*store, nil, nil, nil, mock, nil)

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

func TestHandleRecommendations_EntrySignalFromComparisonEngine(t *testing.T) {
	t.Setenv("ATLAS_DEV_MODE", "true")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	mock := &mockComparisonEngine{score: 0.85}
	h := NewHandlerWithServices(*store, nil, nil, nil, nil, mock).WithDevMode(true)

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
	if rec.Strategies.EntrySignal != "Score=0.85 — 等回測支撐區間" {
		t.Errorf("EntrySignal = %q, want hardcoded 'Score=0.85 — 等回測支撐區間'", rec.Strategies.EntrySignal)
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
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil).WithDevMode(true)
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
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil).WithDevMode(true).WithRegimeListener(listener)

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
