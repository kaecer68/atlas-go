package parameters

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

type Handlers struct {
	params     *config.ParametersConfig
	paramsPath string
}

func NewHandlers(paramsPath string) *Handlers {
	params, _ := config.LoadParametersConfig(paramsPath)
	if params == nil {
		params = config.DefaultParametersConfig()
	}
	return &Handlers{
		params:     params,
		paramsPath: paramsPath,
	}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/parameters", h.HandleParameters)
	mux.HandleFunc("/api/parameters/categories", h.HandleCategories)
	mux.HandleFunc("/api/parameters/infer-garch", h.HandleInferGARCH)
	mux.HandleFunc("/api/parameters/sweep", h.HandleSweep)
	mux.HandleFunc("/api/parameters/snapshots", h.HandleSnapshots)
	mux.HandleFunc("/api/parameters/audit-log", h.HandleAuditLog)
	mux.HandleFunc("/api/parameters/rollback", h.HandleRollback)
}

// HandleParameters returns the current parameters or updates them.
func (h *Handlers) HandleParameters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleGetParameters(w, r)
	case http.MethodPost:
		h.handlePostParameters(w, r)
	default:
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handlers) handleGetParameters(w http.ResponseWriter, r *http.Request) {
	shared.WriteJSON(w, http.StatusOK, h.params)
}

func (h *Handlers) handlePostParameters(w http.ResponseWriter, r *http.Request) {
	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}

	engine := config.NewInferenceEngine(h.params)
	for name, value := range updates {
		if v, ok := value.(float64); ok {
			if err := engine.SetParameter(name, v); err != nil {
				shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("set %s: %v", name, err))
				return
			}
		} else if v, ok := value.(int); ok {
			if err := engine.SetParameter(name, float64(v)); err != nil {
				shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("set %s: %v", name, err))
				return
			}
		} else {
			shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("parameter %s must be numeric", name))
			return
		}
	}

	if err := h.params.Validate(); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("validation failed: %v", err))
		return
	}

	if h.paramsPath != "" {
		if err := h.params.Save(h.paramsPath); err != nil {
			shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("save parameters: %v", err))
			return
		}
	}

	shared.WriteJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// HandleCategories returns available parameter categories.
func (h *Handlers) HandleCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	categories := []map[string]any{
		{"id": "darwinian", "name": "Darwinian Weights", "description": "Dynamic weight adjustment parameters"},
		{"id": "factor", "name": "Factor Engine", "description": "Multi-factor scoring parameters"},
		{"id": "optimizer", "name": "Portfolio Optimizer", "description": "Optimization constraint parameters"},
		{"id": "sizing", "name": "Position Sizing", "description": "Kelly criterion and risk parameters"},
		{"id": "health", "name": "Agent Health", "description": "Agent health monitoring thresholds"},
		{"id": "garch", "name": "Volatility Forecasting", "description": "GARCH model parameters"},
		{"id": "experiment", "name": "Experiment Evaluation", "description": "Experiment acceptance thresholds"},
		{"id": "baseline", "name": "Baseline Policy", "description": "Default baseline policy values"},
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{"categories": categories})
}

// HandleInferGARCH runs GARCH parameter inference from provided returns.
func (h *Handlers) HandleInferGARCH(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Returns []float64 `json:"returns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}

	engine := config.NewInferenceEngine(h.params)
	garch, err := engine.InferGARCH(req.Returns)
	if err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("infer GARCH: %v", err))
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"omega": garch.Omega,
		"alpha": garch.Alpha,
		"beta":  garch.Beta,
	})
}

// HandleSweep runs a parameter sweep with the given evaluator.
func (h *Handlers) HandleSweep(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Parameter    string    `json:"parameter"`
		Values       []float64 `json:"values"`
		CurrentValue float64   `json:"current_value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}

	result := map[string]any{
		"parameter":     req.Parameter,
		"values_tested": req.Values,
		"current_value": req.CurrentValue,
		"note":          "parameter sweep requires backtest integration — use /api/backtest with different parameter sets",
	}

	shared.WriteJSON(w, http.StatusOK, result)
}

// HandleSnapshots lists all parameter snapshots.
func (h *Handlers) HandleSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	store := config.NewSnapshotStore("data/state/parameter-snapshots")
	snaps, err := store.ListSnapshots()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("list snapshots: %v", err))
		return
	}

	var summaries []map[string]any
	for _, snap := range snaps {
		summaries = append(summaries, map[string]any{
			"id":           snap.ID,
			"timestamp":    snap.Timestamp,
			"reason":       snap.Reason,
			"user":         snap.User,
			"change_count": len(snap.Changes),
		})
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{"snapshots": summaries})
}

func (h *Handlers) HandleRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		SnapshotID string `json:"snapshot_id"`
		Reason     string `json:"reason"`
		User       string `json:"user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		shared.WriteJSONError(w, http.StatusBadRequest, fmt.Sprintf("decode request: %v", err))
		return
	}

	if req.SnapshotID == "" {
		shared.WriteJSONError(w, http.StatusBadRequest, "snapshot_id is required")
		return
	}

	store := config.NewSnapshotStore("data/state/parameter-snapshots")
	rollbackSnap, err := store.RollbackToSnapshot(req.SnapshotID, req.Reason, req.User)
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("rollback failed: %v", err))
		return
	}

	cfg := config.GetParametersConfig()
	if err := cfg.Save(h.paramsPath); err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("save config: %v", err))
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{
		"message":      "rollback successful",
		"rollback_id":  rollbackSnap.ID,
		"restored_to":  req.SnapshotID,
		"change_count": len(rollbackSnap.Changes),
	})
}

func (h *Handlers) HandleAuditLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	store := config.NewSnapshotStore("data/state/parameter-snapshots")
	snaps, err := store.ListSnapshots()
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, fmt.Sprintf("list snapshots: %v", err))
		return
	}

	var changes []map[string]any
	for _, snap := range snaps {
		for _, change := range snap.Changes {
			changes = append(changes, map[string]any{
				"parameter":   change.Parameter,
				"old_value":   change.OldValue,
				"new_value":   change.NewValue,
				"reason":      change.Reason,
				"timestamp":   change.Timestamp,
				"snapshot_id": snap.ID,
			})
		}
	}

	shared.WriteJSON(w, http.StatusOK, map[string]any{"changes": changes})
}
