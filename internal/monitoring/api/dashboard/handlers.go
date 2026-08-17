package dashboard

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/liveness"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/retail"
)

// TaskLivenessProvider reads the cross-restart task-liveness table.
// Satisfied directly by *liveness.Store and by *monitoring.DashboardAPI
// (late-bound via SetTaskLivenessProvider).
type TaskLivenessProvider interface {
	List(ctx context.Context) ([]liveness.Row, error)
}

// SchedulerStatusProvider exposes live BackgroundTaskManager status.
// Satisfied directly by *apigateway.BackgroundTaskManager (Status()).
type SchedulerStatusProvider interface {
	Status() []apigateway.TaskStatus
}

// taskLivenessTask is one merged row of the task-liveness API response.
type taskLivenessTask struct {
	Name                string     `json:"name"`
	LastRunAt           *time.Time `json:"last_run_at,omitempty"`
	LastSuccessAt       *time.Time `json:"last_success_at,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastDurationMs      int64      `json:"last_duration_ms"`
	// Runtime fields merged from /api/scheduler/status state when the task is
	// registered in the BackgroundTaskManager; omitted for cron-only rows.
	Interval     string     `json:"interval,omitempty"`
	IntervalSecs int64      `json:"interval_seconds,omitempty"`
	Enabled      *bool      `json:"enabled,omitempty"`
	NextRunAt    *time.Time `json:"next_run_at,omitempty"`
	Stale        bool       `json:"stale"`
	StaleReason  string     `json:"stale_reason,omitempty"`
	Source       string     `json:"source"` // "btm" (registered task) or "cron" (ping-only row)
}

type taskLivenessResponse struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Total       int                `json:"total"`
	StaleCount  int                `json:"stale_count"`
	Tasks       []taskLivenessTask `json:"tasks"`
}

// DrawdownProvider is an interface for retrieving the latest drawdown result.
// DashboardAPI satisfies this interface via its GetLatestDrawdown() method.
type DrawdownProvider interface {
	GetLatestDrawdown() *portfolio.DrawdownResult
}

// Handlers holds dependencies for dashboard management center API endpoints.
type Handlers struct {
	WorkDir           string
	LedgerDir         string
	Pool              *pgxpool.Pool
	MacroIngestor     *narrative.MacroIngestor
	GeoProvider       geopolitical.GeopoliticalRiskProvider
	TaiwanGeoProvider geopolitical.GeopoliticalRiskProvider
	JanusEngine       *janus.Engine
	DrawdownProvider  DrawdownProvider
	// RegisteredChannelIDs, when set, makes the data-channels page list every
	// ID from the ChannelRegistry (manifest #G05). May be nil in tests.
	RegisteredChannelIDs []string

	// TaskLivenessProvider / SchedulerStatusProvider back the
	// GET /api/dashboard/task-liveness endpoint. Both are late-bound (set
	// after RegisterAllRoutes because the BackgroundTaskManager is created
	// later in cmd/atlas/main.go); nil → the endpoint reports 503.
	TaskLivenessProvider TaskLivenessProvider
	SchedulerStatus      SchedulerStatusProvider

	// channel state management — initialized by LoadChannelStates
	channelStates   map[string]channelState
	channelStatesMu sync.RWMutex
}

// NewHandlers creates a new Handlers with the required directory paths.
// Channel states are loaded from the state file on initialization.
func NewHandlers(workDir, ledgerDir string) *Handlers {
	h := &Handlers{
		WorkDir:       workDir,
		LedgerDir:     ledgerDir,
		channelStates: make(map[string]channelState),
	}
	h.LoadChannelStates()
	return h
}

// HandleRSITwCalibration serves the latest RSI-tw calibration report.
func (h *Handlers) HandleRSITwCalibration(r *http.Request) (int, any) {
	report, err := retail.LoadLastCalibrationReport(h.WorkDir)
	if err != nil {
		// File not found is expected on fresh systems — keep 200 with not_available.
		if errors.Is(err, os.ErrNotExist) {
			return http.StatusOK, map[string]any{
				"status":  "not_available",
				"message": "no calibration report available yet. The first calibration runs on system startup and every 24h thereafter.",
			}
		}
		logging.Error("dashboard", "rsi_tw_calibration_report_load_failed", "err", err)
		return http.StatusInternalServerError, map[string]any{
			"status": "error",
			"error":  "failed to load calibration report",
		}
	}
	return http.StatusOK, map[string]any{
		"report": report,
	}
}

// HandleTaskLiveness returns the aggregated task-liveness snapshot: for each
// task, the persisted last run / last success / failure count (survives
// restarts) merged with live runtime state (interval, enabled, next run) and
// a staleness flag computed as now-lastRun > interval x 3.
func (h *Handlers) HandleTaskLiveness(r *http.Request) (int, any) {
	if h.TaskLivenessProvider == nil {
		return http.StatusServiceUnavailable, map[string]any{
			"status": "error",
			"error":  "task liveness provider not configured",
		}
	}
	rows, err := h.TaskLivenessProvider.List(r.Context())
	if err != nil {
		// Graceful degrade: liveness is an observability endpoint, a DB hiccup
		// must not 500 the whole admin dashboard. Report degraded and return an
		// empty snapshot (frontend shows "活性資料暫不可用" instead of erroring).
		logging.Error("dashboard", "task_liveness_list_failed", "err", err.Error())
		return http.StatusOK, map[string]any{
			"status":       "degraded",
			"error":        "task liveness store unavailable: " + err.Error(),
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"tasks":        []taskLivenessTask{},
		}
	}

	runtime := make(map[string]apigateway.TaskStatus)
	if h.SchedulerStatus != nil {
		for _, st := range h.SchedulerStatus.Status() {
			runtime[st.Name] = st
		}
	}

	now := time.Now()
	resp := taskLivenessResponse{GeneratedAt: now, Tasks: make([]taskLivenessTask, 0, len(rows))}
	for _, row := range rows {
		t := taskLivenessTask{
			Name:                row.TaskName,
			LastError:           row.LastError,
			ConsecutiveFailures: row.ConsecutiveFailures,
			LastDurationMs:      row.LastDurationMs,
			Source:              "cron",
		}
		if !row.LastRunAt.IsZero() {
			lr := row.LastRunAt
			t.LastRunAt = &lr
		}
		if !row.LastSuccessAt.IsZero() {
			ls := row.LastSuccessAt
			t.LastSuccessAt = &ls
		}
		if st, ok := runtime[row.TaskName]; ok {
			t.Source = "btm"
			iv := st.Interval
			t.Interval = iv.String()
			t.IntervalSecs = int64(iv.Seconds())
			enabled := st.Enabled
			t.Enabled = &enabled
			if !st.NextRun.IsZero() {
				nr := st.NextRun
				t.NextRunAt = &nr
			}
			if iv > 0 && liveness.IsStale(row.LastRunAt, iv, now) {
				t.Stale = true
				t.StaleReason = fmt.Sprintf("not run for %s (> 3x %s interval)",
					now.Sub(row.LastRunAt).Round(time.Second), iv.String())
			}
		}
		resp.Tasks = append(resp.Tasks, t)
		if t.Stale {
			resp.StaleCount++
		}
	}
	resp.Total = len(resp.Tasks)
	return http.StatusOK, resp
}

// HandleMaturity returns the system's current maturity phase and progress.
func (h *Handlers) HandleMaturity(r *http.Request) (int, any) {
	statePath := filepath.Join(h.WorkDir, "data", "state", "maturity_tracker.json")
	// Seeded constructor: ATLAS_MATURITY_FIRST_START carries the original
	// first_start across data-dir loss (see internal/domain/maturity.go).
	tracker, err := domain.NewMaturityTrackerSeeded(statePath, config.Load().MaturityFirstStart)
	if err != nil {
		logging.Error("dashboard", "maturity_tracker_load_failed", "err", err)
		tracker = domain.NewMaturityTrackerWithStart(time.Now().UTC())
	}
	return http.StatusOK, map[string]any{
		"phase":                  tracker.Current(),
		"days_since_start":       tracker.DaysSinceStart(),
		"days_until_calibrating": tracker.DaysUntil(domain.MaturityCalibrating),
		"days_until_full_auto":   tracker.DaysUntil(domain.MaturityFullAuto),
		"first_start_date":       tracker.FirstStartDate().Format(time.RFC3339),
		"thresholds": map[string]int{
			"burn_in":     0,
			"calibrating": 60,
			"full_auto":   252,
		},
	}
}

// RegisterRoutes registers all dashboard management center routes.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/data-channels", shared.Get(h.HandleDataChannels))
	mux.Handle("GET /api/dashboard/data-channels/{name}", shared.Get(h.HandleDataChannelDetail))
	mux.Handle("GET /api/dashboard/data-pipeline", shared.Get(h.HandleDataPipeline))
	mux.Handle("GET /api/dashboard/drawdown", shared.Get(h.HandleDrawdown))
	mux.Handle("GET /api/dashboard/channel-fetch-log", shared.Get(h.HandleChannelFetchLog))
	mux.Handle("GET /api/traces/sim-latest", shared.Get(h.HandleSimLatest))
	mux.Handle("POST /api/dashboard/channels/", shared.Adapt(h.HandleChannelAction))
	mux.Handle("POST /api/dashboard/api-keys/update", shared.Post(h.HandleAPIKeyUpdate))
	// Deprecated: internal calibration; not for web UI or MCP. See docs/operations/tier-boundary.md.
	mux.Handle("GET /api/dashboard/rsi-tw-calibration", shared.Get(h.HandleRSITwCalibration))
	mux.Handle("GET /api/dashboard/maturity", shared.Get(h.HandleMaturity))
	mux.Handle("GET /api/dashboard/task-liveness", shared.Get(h.HandleTaskLiveness))
	mux.Handle("GET /api/janus/regime-score", shared.Get(h.HandleJanusRegimeScore))
}

// HandleJanusRegimeScore returns the current regime score from JANUS engine.
// Prefers real Sharpe (from PRISM tracker) over macro-synthesized fallback.
// is_synthetic=true means the score is macro-derived, not from PRISM training.
func (h *Handlers) HandleJanusRegimeScore(r *http.Request) (int, any) {
	if h.JanusEngine == nil {
		return http.StatusServiceUnavailable, map[string]string{
			"error": "janus engine not available",
		}
	}
	score, isSynthetic := h.JanusEngine.GetCurrentRegimeScore()
	return http.StatusOK, map[string]any{
		"score":        score,
		"is_synthetic": isSynthetic,
	}
}
