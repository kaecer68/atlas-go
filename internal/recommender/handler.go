package recommender

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

// TierRecommendation wraps tier-specific recommendation content.
type TierRecommendation struct {
	Tier       string                  `json:"tier"`
	Market     MarketLight             `json:"market"`
	Strategies *StrategyRecommendation `json:"strategies,omitempty"`
	Signals    any                     `json:"signals,omitempty"`
}

// StrategyRecommendation is the structured strategies payload (replaces
// map[string]any so cmd/gentags can extract JSON tags for field-contract CI).
type StrategyRecommendation struct {
	Active      string   `json:"active,omitempty"`
	Available   []string `json:"available,omitempty"`
	Ranked      []string `json:"ranked,omitempty"`
	EntrySignal string   `json:"entry_signal,omitempty"`
	StopLoss    string   `json:"stop_loss,omitempty"`
}

// MarketLight is the free/public tier market overview.
type MarketLight struct {
	Regime      string   `json:"regime"`
	RegimeLabel string   `json:"regime_label"`
	StressIndex float64  `json:"stress_index"`
	CapitalFlow string   `json:"capital_flow_summary"`
	EventsToday []string `json:"events_today"`
}

// Handler serves tier-based recommendations.
type Handler struct {
	subStore       subscription.Store
	jwtMgr         *subscription.JWTManager
	narrative      NarrativeProvider
	capitalFlow    CapitalFlowProvider
	eventPredictor EventPredictor
	strategyComp   ComparisonEngine
}

// NewHandler creates a recommendation handler with optional JWT verification.
// If jwtMgr is non-nil, the handler verifies JWT tokens before reading tier.
func NewHandler(store subscription.Store, jwtMgr *subscription.JWTManager) *Handler {
	return &Handler{subStore: store, jwtMgr: jwtMgr}
}

// NewHandlerWithServices constructs a Handler with Sprint 2 T8-T12 service deps.
// All services may be nil; real integration is T8-T12 work.
func NewHandlerWithServices(
	store subscription.Store,
	jwtMgr *subscription.JWTManager,
	narrative NarrativeProvider,
	capitalFlow CapitalFlowProvider,
	eventPredictor EventPredictor,
	strategy ComparisonEngine,
) *Handler {
	return &Handler{
		subStore:       store,
		jwtMgr:         jwtMgr,
		narrative:      narrative,
		capitalFlow:    capitalFlow,
		eventPredictor: eventPredictor,
		strategyComp:   strategy,
	}
}

// HandleRecommendations returns tier-appropriate recommendations.
// JWT from cookie/Authorization header is preferred; X-User-Email is honored
// ONLY when ATLAS_DEV_MODE=true (production rejects it as spoofing).
func (h *Handler) HandleRecommendations(r *http.Request) (int, any) {
	tier := subscription.TierFree
	authenticated := false

	if h.jwtMgr != nil {
		if token := subscription.ExtractToken(r); token != "" {
			if claims, err := h.jwtMgr.Verify(token); err == nil {
				if user, err := h.subStore.GetByEmail(claims.Email); err == nil {
					tier = user.EffectiveTier()
					authenticated = true
				}
			}
		}
	}

	if !authenticated && devModeEnabled() {
		if email := r.Header.Get("X-User-Email"); email != "" {
			if user, err := h.subStore.GetByEmail(email); err == nil {
				tier = user.EffectiveTier()
				authenticated = true
			}
		}
	}

	if !authenticated && !devModeEnabled() && r.Header.Get("X-User-Email") != "" {
		return http.StatusUnauthorized, map[string]string{
			"error": "X-User-Email header not allowed in production",
		}
	}

	rec := TierRecommendation{
		Tier: string(tier),
		Market: MarketLight{
			Regime:      regimeFromNarrative(h.narrative),
			RegimeLabel: "盤勢中性",
			StressIndex: stressIndexFromNarrative(h.narrative),
			CapitalFlow: capitalFlowFromCapitalFlow(h.capitalFlow),
			EventsToday: eventsFromPredictor(h.eventPredictor),
		},
	}

	switch tier {
	case subscription.TierFree:
		// Free tier: market light only
		return http.StatusOK, rec

	case subscription.TierRegistered:
		rec.Strategies = &StrategyRecommendation{
			Active:    "all_weather",
			Available: []string{"all_weather", "defensive"},
		}
		return http.StatusOK, rec

	case subscription.TierPremium:
		rec.Strategies = &StrategyRecommendation{
			Active:      "growth",
			Ranked:      []string{"growth", "momentum", "all_weather", "value", "defensive"},
			EntrySignal: signalEntry(h.strategyComp, "growth"),
			StopLoss:    signalStopLoss(h.strategyComp, "growth"),
		}
		return http.StatusOK, rec

	default:
		return http.StatusOK, rec
	}
}

// RegisterRoutes registers recommendation endpoints with optional JWT verification.
func RegisterRoutes(mux *http.ServeMux, store subscription.Store, jwtMgr *subscription.JWTManager) {
	h := NewHandler(store, jwtMgr)
	mux.HandleFunc("GET /api/recommendations", func(w http.ResponseWriter, r *http.Request) {
		code, data := h.HandleRecommendations(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("[Recommender] encode response: %v", err)
		}
	})
}

func stressIndexFromNarrative(p NarrativeProvider) float64 {
	if p == nil {
		return 0.0
	}
	info, err := p.GetCurrentStressIndex(context.Background())
	if err != nil || !info.HasData {
		return 0.0
	}
	return info.Value
}

func regimeFromNarrative(p NarrativeProvider) string {
	if p == nil {
		return "NEUTRAL"
	}
	info, err := p.GetCurrentStressIndex(context.Background())
	if err != nil || !info.HasData || info.Regime == "" {
		return "NEUTRAL"
	}
	return info.Regime
}

func capitalFlowFromCapitalFlow(p CapitalFlowProvider) string {
	if p == nil {
		return "資金流向均衡"
	}
	info, err := p.LatestDaily(context.Background())
	if err != nil || info.Summary == "" {
		return "資金流向均衡"
	}
	return info.Summary
}

func eventsFromPredictor(p EventPredictor) []string {
	if p == nil {
		return nil
	}
	preds, err := p.PredictToday(context.Background())
	if err != nil || len(preds) == 0 {
		return nil
	}
	out := make([]string, len(preds))
	for i, p := range preds {
		out[i] = p.Direction
	}
	return out
}

func signalsFromComparisonEngine(p ComparisonEngine, strategyID string) (entrySignal string, stopLoss, takeProfit float64) {
	if p == nil {
		return "", 0, 0
	}
	info, err := p.GetScore(strategyID)
	if err != nil {
		return "", 0, 0
	}
	return info.EntrySignal, info.StopLoss, info.TakeProfit
}

func signalEntry(e ComparisonEngine, strategyID string) string {
	if e == nil {
		return "等待回測支撐區間"
	}
	info, err := e.GetScore(strategyID)
	if err != nil || info.EntrySignal == "" {
		return "等待回測支撐區間"
	}
	return info.EntrySignal
}

func signalStopLoss(e ComparisonEngine, strategyID string) string {
	if e == nil {
		return "-5%"
	}
	info, err := e.GetScore(strategyID)
	if err != nil || info.StopLoss == 0 {
		return "-5%"
	}
	return fmt.Sprintf("-%.1f%%", info.StopLoss*100)
}
