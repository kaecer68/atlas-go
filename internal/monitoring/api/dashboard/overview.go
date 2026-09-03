package dashboard

import (
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// OverviewResponse is the JSON contract of GET /api/dashboard/overview
// (SSOT plan P1-7, for the Phase-2 merged dashboard page):
//
//	{
//	  "generated_at": "<RFC3339 UTC>",
//	  "source": "postgres" | "jsonl" | "",   // L-cold history backend
//	  "degraded": false,                      // PG unavailable → JSONL fallback
//	  "live_status":     { … same shape as GET /api/dashboard/live-status … },
//	  "portfolio_state": { … same shape as GET /api/dashboard/portfolio-state,
//	                        essence: no equity_curve array … },
//	  "risk_exposure":   { … same shape as GET /api/dashboard/risk-exposure … }
//	}
//
// The three sections are assembled by invoking the same handlers that serve
// the standalone endpoints, so the aggregate can never drift from the
// per-endpoint answers. Field names are identical to the existing endpoints
// (snake_case); consumers that need fields missing from the essence payload
// (e.g. the full equity curve) fall back to the dedicated endpoint.
type OverviewResponse struct {
	GeneratedAt time.Time `json:"generated_at"`
	Source      string    `json:"source,omitempty"`
	Degraded    bool      `json:"degraded,omitempty"`
	// LiveStatus / PortfolioState / RiskExposure mirror the standalone
	// endpoint payloads (typed structs marshal to their own JSON shapes).
	LiveStatus     any `json:"live_status"`
	PortfolioState any `json:"portfolio_state"`
	RiskExposure   any `json:"risk_exposure"`
}

// overviewCacheTTL mirrors the agent-observatory cache cadence (PR #1813):
// the merged dashboard page fires one aggregate request per load and the
// three sub-loads (live-status + portfolio-state + risk-exposure) each touch
// state files, so a 60s TTL keeps repeated page loads cheap.
const overviewCacheTTL = 60 * time.Second

// HandleOverview serves the aggregate dashboard payload with a 60s TTL cache.
func (h *Handlers) HandleOverview(r *http.Request) (int, any) {
	if h.Live == nil {
		return http.StatusServiceUnavailable, map[string]any{
			"status":  "service_unavailable",
			"message": "overview live handlers not wired (RegisterLiveRoutes must run first)",
		}
	}

	h.overviewMu.Lock()
	defer h.overviewMu.Unlock()
	if h.overviewHit != nil && time.Since(h.overviewAt) < overviewCacheTTL {
		return http.StatusOK, h.overviewHit
	}

	payload := h.buildOverview(r)
	h.overviewHit = payload
	h.overviewAt = time.Now()
	return http.StatusOK, payload
}

// buildOverview assembles the three sections from the live handlers.
func (h *Handlers) buildOverview(r *http.Request) *OverviewResponse {
	_, liveStatus := h.Live.HandleLiveStatus(r)
	_, portfolioState := h.Live.HandlePortfolioState(r)
	_, riskExposure := h.Live.HandleRiskExposure(r)

	payload := &OverviewResponse{
		GeneratedAt:    time.Now().UTC(),
		LiveStatus:     liveStatus,
		PortfolioState: portfolioState,
		RiskExposure:   riskExposure,
	}
	// Top-level source/degraded describe the L-cold history backend; each
	// section also carries its own source/degraded like the standalone
	// endpoints do (P1-3). When no SSoT provider is wired (tests), the
	// fields stay empty/omitted.
	svc := h.Live.Svc
	if svc == nil {
		svc = service.NewLiveService(h.Live.WorkDir, h.Live.LedgerDir)
	}
	payload.Source = svc.HistorySource()
	payload.Degraded = svc.HistoryDegraded()
	return payload
}

// RegisterOverviewRoutes mounts the overview endpoint. Kept separate so the
// handler can be registered independently of the live handlers in tests.
func (h *Handlers) RegisterOverviewRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/overview", shared.Get(h.HandleOverview))
}
