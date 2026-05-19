package system

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring/api/shared"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/portfolio"
	"github.com/kaecer68/atlas-go/internal/risk"
)

type Handlers struct {
	Svc *service.SystemService
}

func NewHandlers(svc *service.SystemService) *Handlers {
	return &Handlers{Svc: svc}
}

func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/dashboard/phase3-status", shared.Get(h.HandlePhase3Status))
	mux.Handle("GET /api/dashboard/system-health", shared.Get(h.HandleSystemHealth))
	mux.Handle("GET /api/dashboard/clamping-events", shared.Get(h.HandleClampingEvents))
	mux.Handle("GET /api/dashboard/conviction-clamping-events", shared.Get(h.HandleConvictionClampingEvents))
	mux.Handle("GET /api/dashboard/capital-phase", shared.Get(h.HandleCapitalPhase))
	mux.Handle("GET /api/dashboard/retail-sentiment", shared.Get(h.HandleRetailSentiment))
}

func (h *Handlers) HandlePhase3Status(r *http.Request) (int, any) {
	metrics, err := h.Svc.LoadPhase3Status()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load phase3 metrics: %v", err)}
	}
	return http.StatusOK, metrics
}

func (h *Handlers) HandleSystemHealth(r *http.Request) (int, any) {
	health, err := h.Svc.LoadSystemHealth()
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load system health: %v", err)}
	}
	return http.StatusOK, health
}

func (h *Handlers) HandleClampingEvents(r *http.Request) (int, any) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000
			}
			limit = parsed
		}
	}

	events, err := h.Svc.LoadClampingEvents(limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load clamping events: %v", err)}
	}

	return http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	}
}

func (h *Handlers) HandleConvictionClampingEvents(r *http.Request) (int, any) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			if parsed > 1000 {
				parsed = 1000
			}
			limit = parsed
		}
	}

	events, err := h.Svc.LoadConvictionClampingEvents(limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("load conviction clamping events: %v", err)}
	}

	return http.StatusOK, map[string]any{
		"events": events,
		"count":  len(events),
	}
}

func (h *Handlers) HandleCapitalPhase(r *http.Request) (int, any) {
	ctrl := risk.NewCapitalPhaseController(domain.DefaultCapitalPhaseConfig())
	return http.StatusOK, ctrl.GetSnapshot()
}

type RetailSentimentResponse struct {
	SentimentScore   float64 `json:"sentiment_score"`
	MarginChangePct  float64 `json:"margin_change_pct"`
	MarginBalance    float64 `json:"margin_balance"`
	ShortBalance     float64 `json:"short_balance"`
	ShortChangePct   float64 `json:"short_change_pct"`
	DayTradingRatio  float64 `json:"day_trading_ratio"`
	MarginPercentile float64 `json:"margin_percentile"`
	ExtremeReading   string  `json:"extreme_reading"`
	Score            float64 `json:"score"`
	ChangePct        float64 `json:"change_pct"`
	Interpretation   string  `json:"interpretation"`
}

func extremeReadingFromScore(score float64) string {
	switch {
	case score >= 0.5:
		return "frenzy"
	case score <= -0.5:
		return "fear"
	default:
		return "neutral"
	}
}

func (h *Handlers) HandleRetailSentiment(r *http.Request) (int, any) {
	snap, err := loadLatestMacroSnapshot(h.Svc.WorkDir)
	if err != nil {
		return http.StatusOK, RetailSentimentResponse{
			SentimentScore:   0,
			MarginChangePct:  0,
			MarginBalance:    0,
			ShortBalance:     0,
			ShortChangePct:   0,
			DayTradingRatio:  0,
			MarginPercentile: 0,
			ExtremeReading:   "neutral",
			Score:            0,
			ChangePct:        0,
			Interpretation:   "no macro snapshot available",
		}
	}

	fb := portfolio.NewFactorBridge()
	input := fb.Convert(snap)

	marginPercentile := calculateMarginPercentile(h.Svc.WorkDir, snap.RetailMarginBalance.Value)

	dayTradingRatio := 0.0
	// TODO: Migrate to Gateway for direct day trading provider instantiation.
	provider := marketdata.NewDayTradingProvider()
	if stats, err := provider.FetchLatest(r.Context()); err == nil {
		dayTradingRatio = stats.VolumeRatio
	}

	interpretation := interpretRetailSentiment(input.RetailSentimentScore)
	return http.StatusOK, RetailSentimentResponse{
		SentimentScore:   input.RetailSentimentScore,
		MarginChangePct:  snap.RetailMarginBalance.ChangePct / 100,
		MarginBalance:    snap.RetailMarginBalance.Value,
		ShortBalance:     snap.RetailShortBalance.Value,
		ShortChangePct:   snap.RetailShortBalance.ChangePct,
		DayTradingRatio:  dayTradingRatio,
		MarginPercentile: marginPercentile,
		ExtremeReading:   extremeReadingFromScore(input.RetailSentimentScore),
		Score:            input.RetailSentimentScore,
		ChangePct:        snap.RetailMarginBalance.ChangePct,
		Interpretation:   interpretation,
	}
}

func loadLatestMacroSnapshot(workDir string) (marketdata.MacroDataSnapshot, error) {
	path := filepath.Join(workDir, "data/state/macro/latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	var snap marketdata.MacroDataSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return marketdata.MacroDataSnapshot{}, err
	}
	return snap, nil
}

type marginHistoryEntry struct {
	Date          string  `json:"date"`
	MarginBalance float64 `json:"margin_balance"`
}

func calculateMarginPercentile(workDir string, currentValue float64) float64 {
	if currentValue <= 0 {
		return 0
	}

	pattern := filepath.Join(workDir, "data/state/margin", "*.json")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return 0
	}

	var values []float64
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var entry marginHistoryEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		if entry.MarginBalance > 0 {
			values = append(values, entry.MarginBalance)
		}
	}

	if len(values) < 2 {
		return 0.5
	}

	lessThan := 0
	for _, v := range values {
		if v < currentValue {
			lessThan++
		}
	}

	return float64(lessThan) / float64(len(values))
}

func interpretRetailSentiment(score float64) string {
	switch {
	case score >= 0.8:
		return "extremely bullish retail sentiment"
	case score >= 0.5:
		return "bullish retail sentiment"
	case score >= 0.2:
		return "mildly bullish retail sentiment"
	case score > -0.2:
		return "neutral retail sentiment"
	case score > -0.5:
		return "mildly bearish retail sentiment"
	case score > -0.8:
		return "bearish retail sentiment"
	default:
		return "extremely bearish retail sentiment"
	}
}
