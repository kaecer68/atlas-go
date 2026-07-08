package recommender

import (
	"context"
	"net/http"
	"os"
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

	h := NewHandler(*store, nil)

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
	h := NewHandler(*store, nil)

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
	h := NewHandler(*store, nil)

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
	h := NewHandlerWithServices(*store, nil, mock, nil, nil, nil)

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
