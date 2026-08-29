package service

import (
	"context"
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/narrative/geopolitical"
)

type NarrativeService struct {
	WorkDir         string
	NarrativeEngine *narrative.NarrativeEngine
	ReportGenerator *narrative.ReportGenerator
	macroProvider   marketdata.MacroDataProvider
	geoProvider     geopolitical.GeopoliticalRiskProvider
	historicalStore ledger.HistoricalStore
}

func NewNarrativeService(workDir string, narrativeEngine *narrative.NarrativeEngine, reportGenerator *narrative.ReportGenerator) *NarrativeService {
	return &NarrativeService{
		WorkDir:         workDir,
		NarrativeEngine: narrativeEngine,
		ReportGenerator: reportGenerator,
	}
}

// WithHistoricalStore injects the SQLite ledger so the service can read/write
// historical time-series (stress index, geopolitical risk, etc.).
func (s *NarrativeService) WithHistoricalStore(hs ledger.HistoricalStore) *NarrativeService {
	s.historicalStore = hs
	return s
}

func (s *NarrativeService) SetMacroProvider(p marketdata.MacroDataProvider) {
	s.macroProvider = p
}

func (s *NarrativeService) SetGeoProvider(p geopolitical.GeopoliticalRiskProvider) {
	s.geoProvider = p
}

// BuildMarketNarrativeData fetches the latest macro snapshot and converts it
// into the narrative detection input struct.  GeopoliticalGPR is fetched from
// the geoProvider if available; other fields not available in the snapshot
// (RetailInstitutionalDivergence, MarginZScore, EarningsSurprisePct) remain
// zeroed and should be overlaid by query-param overrides if desired.
func (s *NarrativeService) BuildMarketNarrativeData(ctx context.Context) (narrative.MarketNarrativeData, error) {
	if s.macroProvider == nil {
		return narrative.MarketNarrativeData{}, fmt.Errorf("macro provider not set")
	}
	snap, err := s.macroProvider.FetchSnapshot(ctx)
	if err != nil {
		return narrative.MarketNarrativeData{}, fmt.Errorf("fetch snapshot: %w", err)
	}
	data := narrative.MarketNarrativeDataFromSnapshot(snap)

	// Overlay geopolitical risk score if provider is available.
	if s.geoProvider != nil {
		geoCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if score, err := s.geoProvider.FetchScore(geoCtx); err == nil {
			data.GeopoliticalGPR = score.Intensity
		} else {
			logging.Warn("narrative_service", "geo_provider_fallback", logging.Err(err))
		}
	}

	return data, nil
}

func (s *NarrativeService) DetectEvents(data narrative.MarketNarrativeData) []narrative.NarrativeEvent {
	return s.NarrativeEngine.DetectEvents(data)
}

func (s *NarrativeService) MatchChains(events []narrative.NarrativeEvent) []narrative.CausalChain {
	return s.NarrativeEngine.MatchChains(events)
}

func (s *NarrativeService) GetActiveModels(themes []string) []narrative.InvestmentModel {
	replayPath := config.GetReplayDataPath(s.WorkDir)
	if err := s.NarrativeEngine.EvaluateModels(replayPath); err != nil {
		logging.Warn("narrative_service", "evaluate_models_warning", logging.Err(err))
	}
	return s.NarrativeEngine.ActiveModels(themes)
}

func (s *NarrativeService) GetTemplates() []narrative.CausalTemplate {
	kb := s.NarrativeEngine.KnowledgeBase()
	return kb.ListTemplates()
}

// ListModels returns the full InvestmentModel library (all models, not just
// currently-active ones). ACI: lets agents inventory the whole module —
// every model's sector bets, themes, weight, hit-rate — regardless of which
// themes are currently detected.
func (s *NarrativeService) ListModels() []narrative.InvestmentModel {
	return s.NarrativeEngine.ListModels()
}

// ModelInventory returns the full module picture for agents: every model,
// the currently-active subset, and the theme→model / theme→template
// cross-reference (表裡結構: causal templates 的 trigger_theme ↔ models 的
// active_theme). This is the ACI entry point for understanding what the
// capital-models module contains and how it links to the causality KB.
func (s *NarrativeService) ModelInventory(ctx context.Context) map[string]any {
	allModels := s.NarrativeEngine.ListModels()

	// Active models = those whose themes are currently detected.
	data, _ := s.BuildMarketNarrativeData(ctx)
	events := s.DetectEvents(data)
	themes := make([]string, 0, len(events))
	for _, e := range events {
		themes = append(themes, e.Theme)
	}
	activeModels := s.NarrativeEngine.ActiveModels(themes)

	// theme → models (from active_theme) and theme → templates (trigger_theme).
	templates := s.GetTemplates()
	themeToModels := make(map[string][]string)
	for _, m := range allModels {
		for _, t := range m.ActiveThemes {
			themeToModels[t] = append(themeToModels[t], m.ID)
		}
	}
	themeToTemplates := make(map[string][]string)
	for _, t := range templates {
		themeToTemplates[t.TriggerTheme] = append(themeToTemplates[t.TriggerTheme], t.ID)
	}

	return map[string]any{
		"all_models":         allModels,
		"active_models":      activeModels,
		"active_themes":      themes,
		"theme_to_models":    themeToModels,
		"theme_to_templates": themeToTemplates,
		"workflow": "SectorBias derives a sector allocation multiplier per " +
			"industry from active models: each model's FavoredSectors (+)/" +
			"AvoidedSectors (−) × Darwinian weight × event confidence×hit-rate. " +
			"Templates (causality KB) explain the causal chain but do not drive " +
			"allocation directly; models are the executable sector bets.",
	}
}

func (s *NarrativeService) GenerateDailySummary(date string, events []narrative.NarrativeEvent, recs []domain.Recommendation, risk *domain.RiskSnapshot) *domain.DailySummaryReport {
	return s.ReportGenerator.GenerateDailySummary(date, events, recs, risk)
}

func (s *NarrativeService) GetCurrentStressIndex() narrative.TaiwanStressIndex {
	return s.NarrativeEngine.GetCurrentStressIndex()
}

func (s *NarrativeService) GetStressIndexHistory(days int) []narrative.TaiwanStressIndex {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}

	if s.historicalStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Load enough rows to cover the requested calendar window, then filter by
		// date so `days=N` means "last N calendar days" rather than "last N rows".
		limit := max(days*2, 90)
		rows, err := s.historicalStore.LoadStressHistory(ctx, limit)
		if err != nil {
			logging.Warn("narrative_service", "load_stress_history_failed", logging.Err(err))
		} else {
			rows = filterStressRowsByMinDate(rows, dateNDaysAgo(days))
			if len(rows) > 0 {
				return stressRowsToIndex(rows)
			}
		}
	}

	return s.NarrativeEngine.GetStressIndexHistory(days)
}

func stressRowsToIndex(rows []ledger.StressRow) []narrative.TaiwanStressIndex {
	out := make([]narrative.TaiwanStressIndex, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		idx := narrative.TaiwanStressIndex{
			Score:     r.Score,
			Regime:    narrative.NormalizeRegime(r.Regime),
			Timestamp: r.CapturedAt.Unix(),
			Date:      r.Date,
			Source:    r.Source,
		}
		if r.Components != nil {
			idx.Components = make(map[string]float64, len(r.Components))
			for k, v := range r.Components {
				if f, ok := v.(float64); ok {
					idx.Components[k] = f
				}
			}
		}
		out = append(out, idx)
	}
	return out
}

func (s *NarrativeService) GetStressIndexThresholds() narrative.StressIndexThresholds {
	return s.NarrativeEngine.GetStressIndexThresholds()
}

// GeopoliticalPoint is one date-stamped geopolitical risk reading for API
// consumers.
type GeopoliticalPoint struct {
	Date       string   `json:"date"`
	Intensity  float64  `json:"intensity"`
	Sources    []string `json:"sources,omitempty"`
	Source     string   `json:"source"`
	CapturedAt string   `json:"captured_at"`
}

// GetGeopoliticalHistory reads historical geopolitical risk scores from the
// ledger when available, falling back to the current in-memory score as a
// single-point history. The `days` parameter is interpreted as a calendar
// window (today-days+1 through today).
func (s *NarrativeService) GetGeopoliticalHistory(days int) []GeopoliticalPoint {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	if s.historicalStore != nil {
		limit := max(days*2, 90)
		rows, err := s.historicalStore.LoadGeopoliticalHistory(context.Background(), limit)
		if err != nil {
			logging.Warn("narrative_service", "load_geopolitical_history_failed", logging.Err(err))
		} else {
			rows = filterGeopoliticalRowsByMinDate(rows, dateNDaysAgo(days))
			if len(rows) > 0 {
				out := make([]GeopoliticalPoint, len(rows))
				for i, r := range rows {
					out[i] = GeopoliticalPoint{
						Date:       r.Date,
						Intensity:  r.Intensity,
						Sources:    r.Sources,
						Source:     r.Source,
						CapturedAt: r.CapturedAt.UTC().Format(time.RFC3339),
					}
				}
				return out
			}
		}
	}
	return nil
}

// dateNDaysAgo returns the inclusive start date (UTC) for a window of N days
// ending today. For days=5 the window is today-4 .. today.
func dateNDaysAgo(days int) string {
	return time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
}

func filterStressRowsByMinDate(rows []ledger.StressRow, minDate string) []ledger.StressRow {
	out := make([]ledger.StressRow, 0, len(rows))
	for _, r := range rows {
		if r.Date >= minDate {
			out = append(out, r)
		}
	}
	return out
}

func filterGeopoliticalRowsByMinDate(rows []ledger.GeopoliticalRow, minDate string) []ledger.GeopoliticalRow {
	out := make([]ledger.GeopoliticalRow, 0, len(rows))
	for _, r := range rows {
		if r.Date >= minDate {
			out = append(out, r)
		}
	}
	return out
}
