package health

import (
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"net/http"
)

type Handlers struct{}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.HandleHealth)
}

func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	shared.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
