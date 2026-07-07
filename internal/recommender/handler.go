package recommender

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/subscription"
)

// TierRecommendation wraps tier-specific recommendation content.
type TierRecommendation struct {
	Tier       string      `json:"tier"`
	Market     MarketLight `json:"market"`
	Strategies any         `json:"strategies,omitempty"`
	Signals    any         `json:"signals,omitempty"`
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
	subStore subscription.Store
	jwtMgr   *subscription.JWTManager
}

// NewHandler creates a recommendation handler with optional JWT verification.
// If jwtMgr is non-nil, the handler verifies JWT tokens before reading tier.
func NewHandler(store subscription.Store, jwtMgr *subscription.JWTManager) *Handler {
	return &Handler{subStore: store, jwtMgr: jwtMgr}
}

// HandleRecommendations returns tier-appropriate recommendations.
// JWT from cookie/Authorization header is preferred; falls back to
// X-User-Email header only when no JWT is present (legacy/dev).
func (h *Handler) HandleRecommendations(r *http.Request) (int, any) {
	tier := subscription.TierFree

	if h.jwtMgr != nil {
		if token := subscription.ExtractToken(r); token != "" {
			if claims, err := h.jwtMgr.Verify(token); err == nil {
				if user, err := h.subStore.GetByEmail(claims.Email); err == nil {
					tier = user.EffectiveTier()
				}
			}
		}
	}

	// Legacy fallback for dev/testing without JWT
	if tier == subscription.TierFree {
		if email := r.Header.Get("X-User-Email"); email != "" {
			if user, err := h.subStore.GetByEmail(email); err == nil {
				tier = user.EffectiveTier()
			}
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
		rec.Strategies = map[string]any{
			"active":    "all_weather",
			"available": []string{"all_weather", "defensive"},
		}
		return http.StatusOK, rec

	case subscription.TierPremium:
		rec.Strategies = map[string]any{
			"active":       "growth",
			"ranked":       []string{"growth", "momentum", "all_weather", "value", "defensive"},
			"entry_signal": "等待回測支撐區間",
			"stop_loss":    "-5%",
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
