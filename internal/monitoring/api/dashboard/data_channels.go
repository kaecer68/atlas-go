package dashboard

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// HandleDataChannels returns data channel health status with alerts.
func (h *Handlers) HandleDataChannels(r *http.Request) (int, any) {
	fugleKey := config.GetSecret("FUGLE_API_KEY")
	if fugleKey == "" {
		fugleKey = config.GetSecret("ATLAS_FUGLE_API_KEY")
	}
	fubonKey := config.GetSecret("FUBON_API_KEY")
	if fubonKey == "" {
		fubonKey = config.GetSecret("ATLAS_FUBON_API_KEY")
	}
	finmindKey := config.GetSecret("FINMIND_API_KEY")
	if finmindKey == "" {
		finmindKey = config.GetSecret("ATLAS_FINMIND_API_KEY")
	}
	tejKey := config.GetSecret("TEJ_API_KEY")

	channelSvc := service.NewDataChannelService(
		h.WorkDir,
		h.Pool,
		h.MacroIngestor,
		h.GeoProvider,
		h.TaiwanGeoProvider,
		h.JanusEngine,
		fugleKey,
		fubonKey,
		finmindKey,
		tejKey,
	)
	channels, err := channelSvc.GetAllChannelStatuses(r.Context())
	if err != nil {
		return http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("load data channels: %v", err),
		}
	}
	alerts, err := channelSvc.GetAlerts(r.Context())
	if err != nil {
		alerts = []service.ChannelAlert{}
	}
	return http.StatusOK, map[string]any{
		"channels":  channels,
		"alerts":    alerts,
		"generated": time.Now().Format(time.RFC3339),
	}
}
