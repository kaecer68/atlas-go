package dashboard

import (
	"errors"
	"net/http"
	"os"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/narrative"
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
	GeoProvider       narrative.GeopoliticalRiskProvider
	TaiwanGeoProvider narrative.GeopoliticalRiskProvider
	JanusEngine       *janus.Engine
	DrawdownProvider  DrawdownProvider

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

// RegisterRoutes registers all dashboard management center routes.
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/data-channels", shared.Get(h.HandleDataChannels))
	mux.Handle("GET /api/dashboard/data-pipeline", shared.Get(h.HandleDataPipeline))
	mux.Handle("GET /api/dashboard/drawdown", shared.Get(h.HandleDrawdown))
	mux.Handle("GET /api/traces/sim-latest", shared.Get(h.HandleSimLatest))
	mux.Handle("POST /api/dashboard/channels/", shared.Adapt(h.HandleChannelAction))
	mux.Handle("POST /api/dashboard/api-keys/update", shared.Post(h.HandleAPIKeyUpdate))
	mux.Handle("GET /api/dashboard/rsi-tw-calibration", shared.Get(h.HandleRSITwCalibration))
}
