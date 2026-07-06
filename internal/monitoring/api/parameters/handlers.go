package parameters

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
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
	mux.Handle("GET /api/parameters", shared.Get(h.HandleGetParameters))
	mux.Handle("POST /api/parameters", shared.AdminPost(h.HandlePostParameters))
	mux.Handle("GET /api/parameters/categories", shared.Get(h.HandleCategories))
	mux.Handle("POST /api/parameters/infer-garch", shared.Post(h.HandleInferGARCH))
	mux.Handle("POST /api/parameters/sweep", shared.AdminPost(h.HandleSweep))
	mux.Handle("GET /api/parameters/snapshots", shared.Get(h.HandleSnapshots))
	mux.Handle("GET /api/parameters/audit-log", shared.Get(h.HandleAuditLog))
	mux.Handle("POST /api/parameters/rollback", shared.AdminPost(h.HandleRollback))
	mux.Handle("POST /api/parameters/reload", shared.AdminPost(h.HandleReload))
	mux.Handle("GET /api/parameters/metadata", shared.Get(h.HandleGetMetadata))
}

// HandleGetParameters returns the current parameters.
func (h *Handlers) HandleGetParameters(r *http.Request) (int, any) {
	result, err := h.paramsToFlatMap()
	if err != nil {
		return http.StatusOK, h.params
	}
	return http.StatusOK, result
}

// HandleGetMetadata returns parameters with full provenance metadata (rationale, source, citation, todo, last_calibrated).
func (h *Handlers) HandleGetMetadata(r *http.Request) (int, any) {
	result, err := h.paramsToMetadataMap()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("marshal parameters: %v", err)}
	}
	return http.StatusOK, result
}

func (h *Handlers) paramsToMetadataMap() (map[string]any, error) {
	raw, err := json.Marshal(h.params)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	result := make(map[string]any)
	flattenWithMetadata(m, "", result)
	return result, nil
}

func flattenWithMetadata(src map[string]any, prefix string, dst map[string]any) {
	for k, v := range src {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			if _, exists := sub["value"]; exists {
				dst[key] = sub
			} else {
				flattenWithMetadata(sub, key, dst)
			}
		} else {
			dst[key] = v
		}
	}
}

func (h *Handlers) paramsToFlatMap() (map[string]any, error) {
	raw, err := json.Marshal(h.params)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	result := make(map[string]any)
	flatten(m, "", result)
	return result, nil
}

func flatten(src map[string]any, prefix string, dst map[string]any) {
	for k, v := range src {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if sub, ok := v.(map[string]any); ok {
			if val, exists := sub["value"]; exists && len(sub) <= 5 {
				if deep, ok := val.(map[string]any); ok {
					flatten(deep, key, dst)
				} else {
					dst[key] = val
				}
			} else {
				flatten(sub, key, dst)
			}
		} else {
			dst[key] = v
		}
	}
}

// HandlePostParameters updates parameters.
func (h *Handlers) HandlePostParameters(r *http.Request) (int, any) {
	var updates map[string]any
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode request: %v", err)}
	}

	engine := config.NewInferenceEngine(h.params)
	for name, value := range updates {
		if v, ok := value.(float64); ok {
			if err := engine.SetParameter(name, v); err != nil {
				return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("set %s: %v", name, err)}
			}
		} else if v, ok := value.(int); ok {
			if err := engine.SetParameter(name, float64(v)); err != nil {
				return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("set %s: %v", name, err)}
			}
		} else if v, ok := value.(string); ok {
			if err := engine.SetStringParameter(name, v); err != nil {
				return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("set %s: %v", name, err)}
			}
		} else if v, ok := value.(bool); ok {
			if err := engine.SetBoolParameter(name, v); err != nil {
				return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("set %s: %v", name, err)}
			}
		} else if v, ok := value.(map[string]any); ok {
			fmap := make(map[string]float64, len(v))
			for sk, sv := range v {
				switch nv := sv.(type) {
				case float64:
					fmap[sk] = nv
				case int:
					fmap[sk] = float64(nv)
				default:
					return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("map sub-key %s.%s has non-numeric value", name, sk)}
				}
			}
			if err := engine.SetMapParameter(name, fmap); err != nil {
				return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("set map %s: %v", name, err)}
			}
		} else {
			return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("parameter %s has unsupported type", name)}
		}
	}

	if err := h.params.Validate(); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("validation failed: %v", err)}
	}

	if h.paramsPath != "" {
		if err := h.params.LockedSaveWithRollback(h.paramsPath); err != nil {
			return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("save parameters: %v", err)}
		}
		if err := config.ReloadParametersConfig(); err != nil {
			return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("reload after save: %v", err)}
		}
		h.params = config.GetParametersConfig()
	}

	return http.StatusOK, map[string]string{"status": "updated"}
}

// HandleCategories returns available parameter categories.
func (h *Handlers) HandleCategories(r *http.Request) (int, any) {
	categories := []map[string]any{
		{"id": "darwinian", "name": "Darwinian Weights", "description": "Dynamic weight adjustment parameters"},
		{"id": "factor", "name": "Factor Engine", "description": "Multi-factor scoring parameters"},
		{"id": "optimizer", "name": "Portfolio Optimizer", "description": "Optimization constraint parameters"},
		{"id": "sizing", "name": "Position Sizing", "description": "Kelly criterion and risk parameters"},
		{"id": "health", "name": "Agent Health", "description": "Agent health monitoring thresholds"},
		{"id": "garch", "name": "Volatility Forecasting", "description": "GARCH model parameters"},
		{"id": "experiment", "name": "Experiment Evaluation", "description": "Experiment acceptance thresholds"},
		{"id": "baseline", "name": "Baseline Policy", "description": "Default baseline policy values"},
		{"id": "cycle", "name": "Industry Cycle Thresholds", "description": "Per-industry business cycle detection thresholds"},
		{"id": "industry", "name": "Industry Analysis", "description": "Sector weights, cycle thresholds, inventory/capex, and risk scoring"},
		{"id": "strategy", "name": "Strategy Selection", "description": "Strategy switching and evaluation parameters"},
		{"id": "engine", "name": "Engine Configuration", "description": "Macro risk, drawdown, executors, simulation parameters (migrated from EngineConfig)"},
	}

	flatParams, _ := h.paramsToFlatMap()
	keys := make(map[string][]string)
	for _, cat := range categories {
		catID := cat["id"].(string)
		for k := range flatParams {
			if k == "version" || k == "updated_at" {
				continue
			}
			matched := strings.HasPrefix(k, catID)
			if catID == "cycle" && strings.Contains(k, "cycle_thresholds") {
				matched = true
			}
			if matched {
				keys[catID] = append(keys[catID], k)
			}
		}
		if keys[catID] == nil {
			keys[catID] = []string{}
		}
	}

	return http.StatusOK, map[string]any{"categories": categories, "keys": keys}
}

// HandleInferGARCH runs GARCH parameter inference from provided returns.
func (h *Handlers) HandleInferGARCH(r *http.Request) (int, any) {
	var req struct {
		Returns []float64 `json:"returns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode request: %v", err)}
	}

	engine := config.NewInferenceEngine(h.params)
	garch, err := engine.InferGARCH(req.Returns)
	if err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("infer GARCH: %v", err)}
	}

	return http.StatusOK, map[string]any{
		"omega": garch.Omega,
		"alpha": garch.Alpha,
		"beta":  garch.Beta,
	}
}

// HandleSweep runs a parameter sweep with the given evaluator.
func (h *Handlers) HandleSweep(r *http.Request) (int, any) {
	var req struct {
		Parameter    string    `json:"parameter"`
		Values       []float64 `json:"values"`
		CurrentValue float64   `json:"current_value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode request: %v", err)}
	}

	result := map[string]any{
		"parameter":     req.Parameter,
		"values_tested": req.Values,
		"current_value": req.CurrentValue,
		"note":          "parameter sweep requires backtest integration — use /api/backtest with different parameter sets",
	}

	return http.StatusOK, result
}

// HandleSnapshots lists all parameter snapshots.
func (h *Handlers) HandleSnapshots(r *http.Request) (int, any) {
	store := config.NewSnapshotStore(constants.StateParameterSnapshots)
	snaps, err := store.ListSnapshots()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list snapshots: %v", err)}
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

	return http.StatusOK, map[string]any{"snapshots": summaries}
}

// HandleRollback restores parameters to a previous snapshot.
func (h *Handlers) HandleRollback(r *http.Request) (int, any) {
	var req struct {
		SnapshotID string `json:"snapshot_id"`
		Reason     string `json:"reason"`
		User       string `json:"user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode request: %v", err)}
	}

	if req.SnapshotID == "" {
		return http.StatusBadRequest, map[string]string{"error": "snapshot_id is required"}
	}

	store := config.NewSnapshotStore(constants.StateParameterSnapshots)
	rollbackSnap, err := store.RollbackToSnapshot(req.SnapshotID, req.Reason, req.User)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("rollback failed: %v", err)}
	}

	cfg := config.GetParametersConfig()
	if err := cfg.LockedSaveWithRollback(h.paramsPath); err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("save config: %v", err)}
	}
	if err := config.ReloadParametersConfig(); err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("reload after rollback: %v", err)}
	}
	h.params = config.GetParametersConfig()

	return http.StatusOK, map[string]any{
		"message":      "rollback successful",
		"rollback_id":  rollbackSnap.ID,
		"restored_to":  req.SnapshotID,
		"change_count": len(rollbackSnap.Changes),
	}
}

// HandleAuditLog returns the audit log of all parameter changes.
func (h *Handlers) HandleAuditLog(r *http.Request) (int, any) {
	store := config.NewSnapshotStore(constants.StateParameterSnapshots)
	snaps, err := store.ListSnapshots()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("list snapshots: %v", err)}
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

	return http.StatusOK, map[string]any{"changes": changes}
}

func formatIntegrityErrors(errs []error) string {
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "; ")
}

// HandleReload reloads parameters from disk and updates the global singleton.
func (h *Handlers) HandleReload(r *http.Request) (int, any) {
	path := h.paramsPath
	if path == "" {
		path = config.GetParametersConfigPath()
	}

	if errs := config.CheckParamsIntegrity(path); len(errs) > 0 {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("integrity check failed: %v", formatIntegrityErrors(errs))}
	}

	cfg, err := config.LoadParametersConfig(path)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("reload failed: %v", err)}
	}

	if err := cfg.Validate(); err != nil {
		return http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("validation failed: %v", err)}
	}

	*config.GetParametersConfig() = *cfg
	h.params = cfg

	return http.StatusOK, map[string]string{"status": "reloaded", "version": cfg.Version}
}
