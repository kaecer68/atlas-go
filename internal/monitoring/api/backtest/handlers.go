package backtest

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
)

type Handlers struct {
	mu              sync.Mutex
	running         bool
	status          map[string]interface{}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/backtest/run", h.HandleBacktestRun)
	mux.HandleFunc("/api/backtest/status", h.HandleBacktestStatus)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func (h *Handlers) HandleBacktestRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Start string `json:"start"`
		End   string `json:"end"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Start == "" || req.End == "" {
		writeJSONError(w, http.StatusBadRequest, "start and end dates required")
		return
	}
	startDate, err := time.Parse("2006-01-02", req.Start)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid start date format (YYYY-MM-DD)")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.End)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid end date format (YYYY-MM-DD)")
		return
	}

	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		writeJSONError(w, http.StatusConflict, "backtest already running")
		return
	}
	h.running = true
	h.status = map[string]interface{}{
		"running":    true,
		"started_at": time.Now().UTC(),
		"start":      req.Start,
		"end":        req.End,
	}
	h.mu.Unlock()

	go func() {
		cfg := config.Load()
		if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
			cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
		}
		runner := backtest.NewRunner(cfg)
		summary, err := runner.Run(startDate, endDate)
		if err == nil {
			if rerr := runner.GenerateReport(summary); rerr != nil {
				log.Printf("[DashboardAPI] backtest report generation failed: %v", rerr)
			}
		}

		h.mu.Lock()
		h.running = false
		h.status["running"] = false
		h.status["finished_at"] = time.Now().UTC()
		if err != nil {
			h.status["error"] = err.Error()
		} else {
			h.status["window_id"] = summary.WindowID
			h.status["sessions"] = summary.SessionCount
			h.status["outcomes"] = summary.OutcomeCount
			h.status["worst_agent"] = summary.WorstAgentID
		}
		h.mu.Unlock()
	}()

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"running":      true,
		"check_status": "/api/backtest/status",
		"start":        req.Start,
		"end":          req.End,
	})
}

func (h *Handlers) HandleBacktestStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	h.mu.Lock()
	status := make(map[string]interface{}, len(h.status))
	for k, v := range h.status {
		status[k] = v
	}
	h.mu.Unlock()
	writeJSON(w, http.StatusOK, status)
}
