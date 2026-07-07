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
}

// NewHandler creates a recommendation handler.
func NewHandler(store subscription.Store) *Handler {
	return &Handler{subStore: store}
}

// HandleRecommendations returns tier-appropriate recommendations.
func (h *Handler) HandleRecommendations(r *http.Request) (int, any) {
	email := r.Header.Get("X-User-Email")
	tier := subscription.TierFree
	if email != "" {
		if user, err := h.subStore.GetByEmail(email); err == nil {
			tier = user.EffectiveTier()
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

// RegisterRoutes registers recommendation endpoints.
func RegisterRoutes(mux *http.ServeMux, store subscription.Store) {
	h := NewHandler(store)
	mux.HandleFunc("GET /api/recommendations", func(w http.ResponseWriter, r *http.Request) {
		code, data := h.HandleRecommendations(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if err := json.NewEncoder(w).Encode(data); err != nil {
			log.Printf("[Recommender] encode response: %v", err)
		}
	})
}
