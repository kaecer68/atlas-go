package recommender

import (
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

type mockNarrative struct {
	stress float64
	regime string
}

func (m *mockNarrative) GetCurrentStressIndex(ctx context.Context) (StressIndexInfo, error) {
	return StressIndexInfo{Value: m.stress, Regime: m.regime, HasData: true}, nil
}

func (m *mockNarrative) BuildMarketNarrativeData(ctx context.Context) (MarketNarrativeInfo, error) {
	return MarketNarrativeInfo{}, nil
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
	summary   string
	resonance float64
}

func (m *mockCapitalFlow) LatestDaily(ctx context.Context) (CapitalFlowDailyInfo, error) {
	return CapitalFlowDailyInfo{Summary: m.summary, Resonance: m.resonance}, nil
}

type mockEventPredictor struct {
	events []string
}

func (m *mockEventPredictor) PredictToday(ctx context.Context) ([]EventPredictionInfo, error) {
	out := make([]EventPredictionInfo, len(m.events))
	for i, e := range m.events {
		out[i] = EventPredictionInfo{Date: "today", Direction: e, Magnitude: 0.5, Confidence: 0.8}
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
	mock := &mockEventPredictor{events: []string{"MSCI 調整", "ETF 換股", "月營收公告"}}
	h := NewHandlerWithServices(*store, nil, nil, nil, mock, nil)

	req, _ := http.NewRequest(http.MethodGet, "/api/recommendations", nil)
	code, data := h.HandleRecommendations(req)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	rec := data.(TierRecommendation)
	if len(rec.Market.EventsToday) != 3 {
		t.Fatalf("EventsToday len = %d, want 3 (from predictor mock)", len(rec.Market.EventsToday))
	}
	if rec.Market.EventsToday[0] != "MSCI 調整" {
		t.Errorf("EventsToday[0] = %q, want MSCI 調整", rec.Market.EventsToday[0])
	}
}

type mockComparisonEngine struct {
	score       float64
	entrySignal string
	stopLoss    float64
	takeProfit  float64
}

func (m *mockComparisonEngine) GetScore(strategyID string) (StrategyScoreInfo, error) {
	return StrategyScoreInfo{Score: m.score, EntrySignal: m.entrySignal, StopLoss: m.stopLoss, TakeProfit: m.takeProfit}, nil
}

func TestHandleRecommendations_EntrySignalFromComparisonEngine(t *testing.T) {
	t.Setenv("ATLAS_DEV_MODE", "true")
	dir, _ := os.MkdirTemp("", "rec-test")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)
	store.Register("premium@test.com", "pass")
	mock := &mockComparisonEngine{
		score:       0.85,
		entrySignal: "等回測 1000 元支撐進場",
		stopLoss:    0.05,
		takeProfit:  0.15,
	}
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
	if rec.Strategies.EntrySignal != "等回測 1000 元支撐進場" {
		t.Errorf("EntrySignal = %q, want from ComparisonEngine", rec.Strategies.EntrySignal)
	}
	if rec.Strategies.StopLoss != "-5.0%" {
		t.Errorf("StopLoss = %q, want from ComparisonEngine", rec.Strategies.StopLoss)
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

func (f *failingNarrative) GetCurrentStressIndex(ctx context.Context) (StressIndexInfo, error) {
	return StressIndexInfo{}, f.err
}

func (f *failingNarrative) BuildMarketNarrativeData(ctx context.Context) (MarketNarrativeInfo, error) {
	return MarketNarrativeInfo{}, f.err
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

func TestHandleRecommendations_RegimeChange_ConcurrentSafety(t *testing.T) {
	dir, _ := os.MkdirTemp("", "rec-race")
	defer os.RemoveAll(dir)
	store, _ := subscription.NewStore(dir)

	var fireCount int32
	listener := func(oldRegime, newRegime string) {
		atomic.AddInt32(&fireCount, 1)
	}

	mock := &mockNarrative{stress: 20.0, regime: "RISK_ON"}
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil).
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
