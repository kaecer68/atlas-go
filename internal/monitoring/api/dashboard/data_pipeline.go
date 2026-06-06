package dashboard

import (
	"fmt"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/monitoring/service"
)

// HandleDataPipeline returns the data pipeline freshness status.
func (h *Handlers) HandleDataPipeline(r *http.Request) (int, any) {
	pipelineSvc := service.NewDataPipelineService(h.WorkDir, h.LedgerDir)
	sources, err := pipelineSvc.GetPipelineStatus()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("load data pipeline: %v", err),
		}
	}
	return http.StatusOK, map[string]any{
		"sources":   sources,
		"generated": time.Now().Format(time.RFC3339),
	}
}
