package health

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

type Handlers struct{}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /health", shared.Get(h.HandleHealth))
}

func (h *Handlers) HandleHealth(r *http.Request) (int, any) {
	return http.StatusOK, map[string]string{"status": "ok"}
}
