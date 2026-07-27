package system

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/portprobe"
)

// portHealthReport describes one component's port occupancy state at the
// moment /health was queried. State is a JSON-friendly version of
// portprobe.State: "free" | "healthy" | "foreign" | "unknown".
type portHealthReport struct {
	Addr    string `json:"addr"`
	State   string `json:"state"`
	PID     int    `json:"pid,omitempty"`
	Command string `json:"command,omitempty"`
	Error   string `json:"error,omitempty"`
}

// healthResponse is the /health JSON body. Status preserves the legacy
// "ok" string so existing consumers keep working; Ports is additive.
type healthResponse struct {
	Status string                      `json:"status"`
	Ports  map[string]portHealthReport `json:"ports"`
}

// HealthHandlers provides the /health endpoint and /api/health/aggregate Tier 2.
// ChannelHealth is used by checkChannelHealth() for freshness-aware channel status.
type HealthHandlers struct {
	APIAddr       string
	FubonAddr     string
	ChannelHealth *apigateway.ChannelHealthStore
}

func (h *HealthHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /health", shared.Get(h.HandleHealth))
}

func (h *HealthHandlers) HandleHealth(r *http.Request) (int, any) {
	apiAddr := h.APIAddr
	if apiAddr == "" {
		apiAddr = constants.AdminHTTPAddr
	}
	fubonAddr := h.FubonAddr
	if fubonAddr == "" {
		fubonAddr = constants.FubonProxyAddr
	}

	resp := healthResponse{
		Status: "ok",
		Ports: map[string]portHealthReport{
			"atlas_http":  {Addr: apiAddr, State: "healthy"},
			"fubon_proxy": reportPort(fubonAddr),
		},
	}
	return http.StatusOK, resp
}

func reportPort(addr string) portHealthReport {
	state, occ, err := portprobe.Probe(addr)
	if err != nil {
		return portHealthReport{Addr: addr, State: "unknown", Error: err.Error()}
	}
	switch state {
	case portprobe.StateFree:
		return portHealthReport{Addr: addr, State: "free"}
	case portprobe.StateHealthy:
		return portHealthReport{Addr: addr, State: "healthy", PID: occ.PID, Command: occ.Command}
	case portprobe.StateForeign:
		return portHealthReport{Addr: addr, State: "foreign", PID: occ.PID, Command: occ.Command}
	}
	return portHealthReport{Addr: addr, State: "unknown"}
}
