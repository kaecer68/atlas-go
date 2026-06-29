package pipeline

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/kaecer68/atlas-go/internal/config"
)

// L24RouteDeps bundles the dependencies for the L2.4 schedule API.
type L24RouteDeps struct {
	Manager  *L24StateManager
	GetParam func() config.L2_4ScheduleParameters
}

// RegisterL24Routes wires the 5 L2.4 endpoints under
// /api/synergy/l2-4-schedule/* into the given mux.
func RegisterL24Routes(mux *http.ServeMux, deps L24RouteDeps) {
	if deps.Manager == nil {
		panic("pipeline.RegisterL24Routes: deps.Manager is required")
	}
	if deps.GetParam == nil {
		panic("pipeline.RegisterL24Routes: deps.GetParam is required")
	}
	mgr := deps.Manager
	getParam := deps.GetParam

	// GET /api/synergy/l2-4-schedule — current state + effective values
	mux.HandleFunc("GET /api/synergy/l2-4-schedule", func(w http.ResponseWriter, r *http.Request) {
		state := mgr.Get()
		param := getParam()
		resp := buildL24Response(state, param)
		writeJSON(w, http.StatusOK, resp)
	})

	// POST /api/synergy/l2-4-schedule/start
	mux.HandleFunc("POST /api/synergy/l2-4-schedule/start", func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Start(); err != nil {
			writeJSONError(w, http.StatusConflict, err)
			return
		}
		state := mgr.Get()
		param := getParam()
		writeJSON(w, http.StatusOK, buildL24Response(state, param))
	})

	// POST /api/synergy/l2-4-schedule/stop
	mux.HandleFunc("POST /api/synergy/l2-4-schedule/stop", func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Stop(); err != nil {
			writeJSONError(w, http.StatusConflict, err)
			return
		}
		state := mgr.Get()
		param := getParam()
		writeJSON(w, http.StatusOK, buildL24Response(state, param))
	})

	// POST /api/synergy/l2-4-schedule/reset — clear override
	mux.HandleFunc("POST /api/synergy/l2-4-schedule/reset", func(w http.ResponseWriter, r *http.Request) {
		if err := mgr.Reset(); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err)
			return
		}
		state := mgr.Get()
		param := getParam()
		writeJSON(w, http.StatusOK, buildL24Response(state, param))
	})

	// PUT /api/synergy/l2-4-schedule — update override
	mux.HandleFunc("PUT /api/synergy/l2-4-schedule", func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var body struct {
			OverrideStartTime  string `json:"override_start_time"`
			OverridePeriodDays int    `json:"override_period_days"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, errors.New("invalid JSON: "+err.Error()))
			return
		}
		if body.OverrideStartTime == "" {
			writeJSONError(w, http.StatusBadRequest, errors.New("override_start_time is required"))
			return
		}
		if err := mgr.ApplyOverride(body.OverrideStartTime, body.OverridePeriodDays); err != nil {
			writeJSONError(w, http.StatusBadRequest, err)
			return
		}
		state := mgr.Get()
		param := getParam()
		writeJSON(w, http.StatusOK, buildL24Response(state, param))
	})
}

// L24Response is the wire-format returned by all 5 endpoints.
type L24Response struct {
	Status              L24ResponseStatus `json:"status"`
	DefaultStartTime    string            `json:"default_start_time"`
	DefaultPeriodDays   int               `json:"default_period_days"`
	OverrideStartTime   string            `json:"override_start_time,omitempty"`
	OverridePeriodDays  int               `json:"override_period_days,omitempty"`
	EffectiveStartTime  string            `json:"effective_start_time"`
	EffectivePeriodDays int               `json:"effective_period_days"`
	AutoEnabled         bool              `json:"auto_enabled"`
	UpdatedAt           string            `json:"updated_at"`
}

// L24ResponseStatus is a serialisable view of the L24ScheduleStatus
// (avoids leaking time.Time pointer semantics to the JSON client).
type L24ResponseStatus struct {
	Running           bool   `json:"running"`
	StartedAt         string `json:"started_at,omitempty"`
	EndsAt            string `json:"ends_at,omitempty"`
	CurrentPeriodDays int    `json:"current_period_days,omitempty"`
}

func buildL24Response(state L24Schedule, param config.L2_4ScheduleParameters) L24Response {
	defStart := param.DefaultStartTime.Value
	if defStart == "" {
		defStart = "13:45"
	}
	defDays := param.DefaultPeriodDays.Value
	if defDays == 0 {
		defDays = 7
	}

	effStart := defStart
	if state.Config.OverrideStartTime != "" {
		effStart = state.Config.OverrideStartTime
	}
	effDays := defDays
	if state.Config.OverridePeriodDays > 0 {
		effDays = state.Config.OverridePeriodDays
	}

	status := L24ResponseStatus{
		Running:           state.Status.Running,
		CurrentPeriodDays: state.Status.CurrentPeriodDays,
	}
	if state.Status.StartedAt != nil {
		status.StartedAt = state.Status.StartedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if state.Status.EndsAt != nil {
		status.EndsAt = state.Status.EndsAt.Format("2006-01-02T15:04:05Z07:00")
	}

	return L24Response{
		Status:              status,
		DefaultStartTime:    defStart,
		DefaultPeriodDays:   defDays,
		OverrideStartTime:   state.Config.OverrideStartTime,
		OverridePeriodDays:  state.Config.OverridePeriodDays,
		EffectiveStartTime:  effStart,
		EffectivePeriodDays: effDays,
		AutoEnabled:         param.AutoEnabled.Value,
		UpdatedAt:           state.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error(), "status": strconv.Itoa(status)})
}
