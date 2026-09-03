// Package periodmatrix serves GET /api/strategy/period-matrix — the
// (agent × seven-period) performance heatmap payload for the admin console
// (capital-flow Phase 2 PR-2b).
//
// The endpoint is a thin HTTP shell over service.PeriodMatrixService: the
// service owns the SSoT store read and the 60s TTL cache (#1813 pattern),
// and this package only maps errors to HTTP semantics.
package periodmatrix

import (
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// Handlers holds the service dependency for the period-matrix endpoint.
type Handlers struct {
	Svc *service.PeriodMatrixService
}

// NewHandlers creates the endpoint handlers.
func NewHandlers(svc *service.PeriodMatrixService) *Handlers {
	return &Handlers{Svc: svc}
}

// RegisterRoutes mounts GET /api/strategy/period-matrix.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/strategy/period-matrix", shared.Get(h.HandlePeriodMatrix))
}

// periodMatrixResponse is the wire contract of GET /api/strategy/period-matrix:
//
//	{
//	  "generated_at": "<RFC3339>",
//	  "source": "postgres" | "jsonl" | "",
//	  "degraded": false,
//	  "min_samples": 30,
//	  "periods": ["downturn", …, "black_swan"],
//	  "cells": [ { "agent_id", "market_period", "sample_count",
//	               "win_rate", "sharpe", "avg_return", "status" }, … ]
//	}
//
// Cells below min_samples report "status":"insufficient_data" with null
// win_rate/sharpe/avg_return (the sample_count is still truthful). The
// heatmap greys such cells out ("資料不足").
type periodMatrixResponse struct {
	GeneratedAt time.Time              `json:"generated_at"`
	Source      string                 `json:"source,omitempty"`
	Degraded    bool                   `json:"degraded"`
	MinSamples  int                    `json:"min_samples"`
	Periods     []string               `json:"periods"`
	Cells       []portfolio.PeriodCell `json:"cells"`
}

// HandlePeriodMatrix serves the matrix payload.
func (h *Handlers) HandlePeriodMatrix(r *http.Request) (int, any) {
	if h == nil || h.Svc == nil {
		return http.StatusServiceUnavailable, map[string]string{
			"error": "period-matrix service not wired",
		}
	}
	matrix, err := h.Svc.Matrix()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": err.Error()}
	}
	return http.StatusOK, periodMatrixResponse{
		GeneratedAt: matrix.GeneratedAt,
		Source:      h.Svc.Source(),
		Degraded:    h.Svc.Degraded(),
		MinSamples:  matrix.MinSamples,
		Periods:     matrix.Periods,
		Cells:       matrix.Cells,
	}
}
