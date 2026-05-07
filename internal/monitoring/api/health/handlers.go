package health

import (
	"net/http"

	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
)

type Handlers struct {
	WorkDir   string
	LedgerDir string
}

func NewHandlers(workDir, ledgerDir string) *Handlers {
	return &Handlers{WorkDir: workDir, LedgerDir: ledgerDir}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.HandleHealth)
	mux.HandleFunc("/api/health/data-integrity", HandleDataIntegrity(h.WorkDir, h.LedgerDir))
}

func (h *Handlers) HandleHealth(w http.ResponseWriter, r *http.Request) {
	shared.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
