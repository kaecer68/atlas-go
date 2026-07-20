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
)

type NarrativeService struct {
	WorkDir         string
	NarrativeEngine *narrative.NarrativeEngine
	ReportGenerator *narrative.ReportGenerator
	macroProvider   marketdata.MacroDataProvider
	geoProvider     narrative.GeopoliticalRiskProvider
	historicalStore ledger.HistoricalStore
}

func NewNarrativeService(workDir string, narrativeEngine *narrative.NarrativeEngine, reportGenerator *narrative.ReportGenerator) *NarrativeService {
	return &NarrativeService{
		WorkDir:         workDir,
		NarrativeEngine: narrativeEngine,
		ReportGenerator: reportGenerator,
	}
}

func (s *NarrativeService) SetMacroProvider(p marketdata.MacroDataProvider) {
	s.macroProvider = p
}

func (s *NarrativeService) SetGeoProvider(p narrative.GeopoliticalRiskProvider) {
	s.geoProvider = p
}

// WithHistoricalStore injects the SQLite-backed historical store so that
// GetStressIndexHistory can read from the persistent ledger instead of the
// process-local in-memory ring buffer. Safe to call with nil (keeps fallback).
func (s *NarrativeService) WithHistoricalStore(hs ledger.HistoricalStore) *NarrativeService {
	s.historicalStore = hs
	return s
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
	kb := narrative.NewKnowledgeBase()
	return kb.ListTemplates()
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
		rows, err := s.historicalStore.LoadStressHistory(ctx, days)
		if err != nil {
			logging.Warn("narrative_service", "load_stress_history_failed", logging.Err(err))
		} else if len(rows) > 0 {
			return stressRowsToIndex(rows)
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
			Regime:    r.Regime,
			Timestamp: r.CapturedAt.Unix(),
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
