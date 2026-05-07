package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
)

type Handlers struct {
	Svc *service.SystemService
}

func NewHandlers(svc *service.SystemService) *Handlers {
	return &Handlers{Svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/phase3-status", h.HandlePhase3Status)
	mux.HandleFunc("/api/dashboard/system-health", h.HandleSystemHealth)
	mux.HandleFunc("/api/dashboard/clamping-events", h.HandleClampingEvents)
	mux.HandleFunc("/api/dashboard/conviction-clamping-events", h.HandleConvictionClampingEvents)
	mux.HandleFunc("/api/dashboard/capital-phase", h.HandleCapitalPhase)
	mux.HandleFunc("/api/dashboard/retail-sentiment", h.HandleRetailSentiment)
}

// HandlePhase3Status returns Phase 3 metrics (Swarm, PRISM, Spawning, Reflexivity, Adversarial).
func (h *Handlers) HandlePhase3Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	metrics, err := h.Svc.LoadPhase3Status()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load phase3 metrics: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, metrics)
}

// HandleSystemHealth returns system health including baseline version, replay data status,
// last window, crowding warnings, regime, and data channel status.
func (h *Handlers) HandleSystemHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	health, err := h.Svc.LoadSystemHealth()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load system health: %v", err))
		return
	}
	shared.WriteJSON(w, http.StatusOK, health)
}

func (h *Handlers) HandleClampingEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000
			}
			limit = parsed
		}
	}

	events, err := h.Svc.LoadClampingEvents(limit)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load clamping events: %v", err))
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	})
}

func (h *Handlers) HandleConvictionClampingEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000
			}
			limit = parsed
		}
	}

	events, err := h.Svc.LoadConvictionClampingEvents(limit)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("load conviction clamping events: %v", err))
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	})
}

// HandleCapitalPhase returns the current capital phase snapshot.
func (h *Handlers) HandleCapitalPhase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	ctrl := risk.NewCapitalPhaseController(domain.DefaultCapitalPhaseConfig())
	snapshot := ctrl.GetSnapshot()

	shared.WriteJSON(w, http.StatusOK, snapshot)
}

// RetailSentimentResponse is the API response for retail sentiment.
type RetailSentimentResponse struct {
	Score          float64 `json:"score"`
	ChangePct      float64 `json:"change_pct"`
	Interpretation string  `json:"interpretation"`
}

// HandleRetailSentiment returns computed retail sentiment scores from the latest macro snapshot.
func (h *Handlers) HandleRetailSentiment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	snap, err := loadLatestMacroSnapshot(h.Svc.WorkDir)
	if err != nil {
		shared.WriteJSON(w, http.StatusOK, RetailSentimentResponse{
			Score:          0,
			ChangePct:      0,
			Interpretation: "no macro snapshot available",
		})
		return
	}

	fb := portfolio.NewFactorBridge()
	input := fb.Convert(snap)

	interpretation := interpretRetailSentiment(input.RetailSentimentScore)
	shared.WriteJSON(w, http.StatusOK, RetailSentimentResponse{
		Score:          input.RetailSentimentScore,
		ChangePct:      snap.RetailMarginBalance.ChangePct,
		Interpretation: interpretation,
	})
}

// loadLatestMacroSnapshot reads the latest macro snapshot from disk.
func loadLatestMacroSnapshot(workDir string) (marketdata.MacroDataSnapshot, error) {
	path := filepath.Join(workDir, "data/state/macro/latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	return snap, nil
}

// interpretRetailSentiment returns a human-readable interpretation of the retail sentiment score.
func interpretRetailSentiment(score float64) string {
	switch {
	case score >= 0.8:
		return "extremely bullish retail sentiment"
	case score >= 0.5:
		return "bullish retail sentiment"
	case score >= 0.2:
		return "mildly bullish retail sentiment"
	case score > -0.2:
		return "neutral retail sentiment"
	case score > -0.5:
		return "mildly bearish retail sentiment"
	case score > -0.8:
		return "bearish retail sentiment"
	default:
		return "extremely bearish retail sentiment"
	}
}
