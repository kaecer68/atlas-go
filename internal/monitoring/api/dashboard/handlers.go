package dashboard

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/retail"
)

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
