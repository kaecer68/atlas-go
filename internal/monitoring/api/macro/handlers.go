package macro

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type Handlers struct {
	WorkDir          string
	MacroIngestor    *narrative.MacroIngestor
	TaiwanStressCalc *narrative.TaiwanStressCalculator
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/macro/ingest", h.HandleMacroIngest)
	mux.HandleFunc("/api/macro/snapshot/latest", h.HandleMacroSnapshotLatest)
	mux.HandleFunc("/api/macro/snapshot/history", h.HandleMacroSnapshotHistory)
	mux.HandleFunc("/api/macro/capital-flow/latest", h.HandleCapitalFlowLatest)
	mux.HandleFunc("/api/taiwan/stress-index", h.HandleTaiwanStressIndex)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (h *Handlers) HandleMacroIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	events, snap, err := h.MacroIngestor.Ingest(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("ingest failed: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":   events,
		"snapshot": snap,
	})
}

func (h *Handlers) HandleMacroSnapshotLatest(w http.ResponseWriter, r *http.Request) {
	path := filepath.Join(h.MacroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "no macro snapshot available")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read snapshot: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (h *Handlers) HandleMacroSnapshotHistory(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" {
		writeJSONError(w, http.StatusBadRequest, "date query param required (YYYY-MM-DD)")
		return
	}
	path := filepath.Join(h.MacroIngestor.SnapshotDir(), date+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "snapshot not found for date")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read snapshot: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func (h *Handlers) HandleCapitalFlowLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	path := filepath.Join(h.MacroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "no macro snapshot available")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read snapshot: %v", err))
		return
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("decode snapshot: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"foreign_investor_net": snap.ForeignInvestorNet,
		"domestic_fund_net":    snap.DomesticFundNet,
		"dealer_net":           snap.DealerNet,
		"recorded_at":          snap.RecordedAt,
	})
}

func (h *Handlers) HandleTaiwanStressIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var snap marketdata.MacroDataSnapshot
	path := filepath.Join(h.MacroIngestor.SnapshotDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			_, snap, err = h.MacroIngestor.Ingest(r.Context())
			if err != nil {
				writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("ingest failed: %v", err))
				return
			}
		} else {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read snapshot: %v", err))
			return
		}
	} else {
		if err := json.Unmarshal(data, &snap); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("decode snapshot: %v", err))
			return
		}
	}

	geoStore := narrative.NewGeopoliticalStore(filepath.Join(h.WorkDir, "data/state/geopolitical"))
	index, err := h.TaiwanStressCalc.CalculateFromSnapshotWithStore(r.Context(), snap, geoStore)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("calculate stress index: %v", err))
		return
	}
	writeJSON(w, http.StatusOK, index)
}
