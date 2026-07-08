package recommender

import (
	"encoding/json"
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
			Regime:      "NEUTRAL",
			RegimeLabel: "盤勢中性",
			StressIndex: 0.0,
			CapitalFlow: "資金流向均衡",
			EventsToday: nil,
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
			EntrySignal: "等待回測支撐區間",
			StopLoss:    "-5%",
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
