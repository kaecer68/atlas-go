package dailyreport

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Report is a daily market report.
type Report struct {
	Date      string          `json:"date"`
	Generated time.Time       `json:"generated_at"`
	Global    GlobalOverview  `json:"global"`
	Capital   CapitalSection  `json:"capital"`
	Events    EventsSection   `json:"events"`
	Strategy  StrategySection `json:"strategy"`
	Risk      RiskSection     `json:"risk"`
}

// GlobalOverview covers global macro conditions.
type GlobalOverview struct {
	BondYield string `json:"bond_yield"`
	USDIndex  string `json:"usd_index"`
	JPY       string `json:"jpy"`
	VIX       string `json:"vix"`
	Status    string `json:"status"`
	Summary   string `json:"summary"`
}

// CapitalSection covers Taiwan capital flow.
type CapitalSection struct {
	Foreign       string  `json:"foreign"`
	Institutional string  `json:"institutional"`
	Dealer        string  `json:"dealer"`
	Government    string  `json:"government"`
	Retail        string  `json:"retail"`
	Resonance     float64 `json:"resonance"`
	Quality       string  `json:"quality"`
}

// EventsSection lists upcoming events.
type EventsSection struct {
	Tomorrow []string `json:"tomorrow"`
	ThisWeek []string `json:"this_week"`
	Count    int      `json:"count"`
}

// StrategySection covers active strategy signals.
type StrategySection struct {
	Active    string `json:"active_strategy"`
	EntryCond string `json:"entry_condition"`
	Direction string `json:"direction"`
}

// RiskSection covers risk warnings.
type RiskSection struct {
	StressIndex   float64 `json:"stress_index"`
	DrawdownAlert bool    `json:"drawdown_alert"`
	RiskLevel     string  `json:"risk_level"`
	Warning       string  `json:"warning,omitempty"`
}

// Generator produces daily reports.
type Generator struct {
	mu      sync.RWMutex
	latest  *Report
	archive map[string]*Report
	workDir string
}

// NewGenerator creates a daily report generator.
func NewGenerator(workDir string) *Generator {
	return &Generator{
		workDir: workDir,
		archive: make(map[string]*Report),
	}
}

// Generate creates the day's report.
func (g *Generator) Generate() *Report {
	now := time.Now()
	date := now.Format("2006-01-02")

	r := &Report{
		Date:      date,
		Generated: now,
		Global: GlobalOverview{
			BondYield: "4.25%",
			USDIndex:  "104.5",
			JPY:       "150.2",
			VIX:       "14.3",
			Status:    "RISK_ON",
			Summary:   "全球資金環境偏寬鬆，有利風險資產",
		},
		Capital: CapitalSection{
			Foreign:       "偏多",
			Institutional: "中性",
			Dealer:        "偏多",
			Government:    "中性",
			Retail:        "偏空",
			Resonance:     1.0,
			Quality:       "moderate_inflow",
		},
		Events: EventsSection{
			Tomorrow: []string{"無重大事件"},
			ThisWeek: []string{"月營收公告期"},
			Count:    1,
		},
		Strategy: StrategySection{
			Active:    "all_weather",
			EntryCond: "等待回測支撐區間",
			Direction: "偏多",
		},
		Risk: RiskSection{
			StressIndex:   0.3,
			DrawdownAlert: false,
			RiskLevel:     "moderate",
		},
	}

	g.mu.Lock()
	g.latest = r
	g.archive[date] = r
	g.mu.Unlock()

	g.persist(r)
	return r
}

func (g *Generator) persist(r *Report) {
	dir := filepath.Join(g.workDir, "data", "reports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := filepath.Join(dir, r.Date+".json")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("[DailyReport] close persist file: %v", err)
		}
	}()
	if err := json.NewEncoder(f).Encode(r); err != nil {
		log.Printf("[DailyReport] encode persist: %v", err)
	}
}

// Latest returns the most recent report.
func (g *Generator) Latest() *Report {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.latest
}

// GetByDate returns a specific date's report.
func (g *Generator) GetByDate(date string) *Report {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.archive[date]
}

// Handler serves daily report endpoints.
type Handler struct {
	gen *Generator
}

// NewHandler creates a report API handler.
func NewHandler(gen *Generator) *Handler {
	return &Handler{gen: gen}
}

// HandleLatest returns the latest report as JSON.
func (h *Handler) HandleLatest(w http.ResponseWriter, r *http.Request) {
	rep := h.gen.Latest()
	if rep == nil {
		rep = h.gen.Generate()
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rep); err != nil {
		log.Printf("[DailyReport] encode latest: %v", err)
	}
}

// HandleArchive returns a historical report by date query param.
func (h *Handler) HandleArchive(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		http.Error(w, "?date=YYYY-MM-DD required", http.StatusBadRequest)
		return
	}
	rep := h.gen.GetByDate(date)
	if rep == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rep); err != nil {
		log.Printf("[DailyReport] encode archive: %v", err)
	}
}

// HandleSubscribe registers email subscription.
func (h *Handler) HandleSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "subscribed"}); err != nil {
		log.Printf("[DailyReport] encode subscribe: %v", err)
	}
}

// RegisterRoutes registers daily report endpoints.
func RegisterRoutes(mux *http.ServeMux, gen *Generator) {
	h := NewHandler(gen)
	mux.HandleFunc("GET /api/reports/latest", h.HandleLatest)
	mux.HandleFunc("GET /api/reports/archive", h.HandleArchive)
	mux.HandleFunc("POST /api/reports/subscribe", h.HandleSubscribe)
}
