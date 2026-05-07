package data

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/kaecer68/atlas-go/internal/janus"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type Handlers struct {
	WorkDir           string
	Pool              *pgxpool.Pool
	MacroIngestor     *narrative.MacroIngestor
	GeoProvider       narrative.GeopoliticalRiskProvider
	TaiwanGeoProvider *narrative.CompositeTaiwanGeopoliticalProvider
	JanusEngine       *janus.Engine
	HealthRecorder    ChannelHealthRecorder
	dataChannelSvc    *service.DataChannelService
	channelIngestSvc  *service.ChannelIngestService
}

func NewHandlers(workDir string, pool *pgxpool.Pool, macroIngestor *narrative.MacroIngestor, geoProvider narrative.GeopoliticalRiskProvider, taiwanGeoProvider *narrative.CompositeTaiwanGeopoliticalProvider, janusEngine *janus.Engine, healthRecorder ChannelHealthRecorder) *Handlers {
	h := &Handlers{
		WorkDir:           workDir,
		Pool:              pool,
		MacroIngestor:     macroIngestor,
		GeoProvider:       geoProvider,
		TaiwanGeoProvider: taiwanGeoProvider,
		JanusEngine:       janusEngine,
		HealthRecorder:    healthRecorder,
	}
	h.dataChannelSvc = service.NewDataChannelService(workDir, pool, macroIngestor, geoProvider, taiwanGeoProvider, janusEngine)
	h.channelIngestSvc = service.NewChannelIngestService(workDir, pool, macroIngestor, geoProvider, taiwanGeoProvider, janusEngine)
	return h
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/dashboard/data-channels", h.HandleDataChannels)
	mux.HandleFunc("/api/channels/ingest", h.HandleChannelsIngest)
}

func (h *Handlers) HandleDataChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	channels, err := h.dataChannelSvc.GetAllChannelStatuses(r.Context())
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	alerts, err := h.dataChannelSvc.GetAlerts(r.Context())
	if err != nil {
		shared.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	shared.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"channels":  channels,
		"alerts":    alerts,
		"generated": time.Now().Format("2006-01-02 15:04:05"),
	})
}

func (h *Handlers) HandleChannelsIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		shared.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result := h.channelIngestSvc.TriggerAllIngests(r.Context())

	response := map[string]any{
		"macro_ok":    result.MacroOK,
		"geo_ok":      result.GeoOK,
		"cap_flow_ok": result.CapFlowOK,
		"export_ok":   result.ExportOK,
		"tsmc_ok":     result.TsmcOK,
		"tw_geo_ok":   result.TwGeoOK,
		"janus_ok":    result.JanusOK,
		"tej_ok":      result.TejOK,
	}
	if result.MacroErr != "" {
		response["macro_error"] = result.MacroErr
	}
	if result.GeoErr != "" {
		response["geo_error"] = result.GeoErr
	}
	if result.CapFlowErr != "" {
		response["cap_flow_error"] = result.CapFlowErr
	}
	if result.ExportErr != "" {
		response["export_error"] = result.ExportErr
	}
	if result.TsmcErr != "" {
		response["tsmc_error"] = result.TsmcErr
	}
	if result.TwGeoErr != "" {
		response["tw_geo_error"] = result.TwGeoErr
	}
	if result.JanusErr != "" {
		response["janus_error"] = result.JanusErr
	}
	if result.TejErr != "" {
		response["tej_error"] = result.TejErr
	}

	if !result.MacroOK && !result.GeoOK && !result.CapFlowOK {
		shared.WriteJSONError(w, http.StatusInternalServerError, "all core ingests failed")
		return
	}

	shared.WriteJSON(w, http.StatusOK, response)
}
