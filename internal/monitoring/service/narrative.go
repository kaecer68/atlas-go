package service

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type NarrativeService struct {
	WorkDir         string
	NarrativeEngine *narrative.NarrativeEngine
	ReportGenerator *narrative.ReportGenerator
	macroProvider   marketdata.MacroDataProvider
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

// BuildMarketNarrativeData fetches the latest macro snapshot and converts it
// into the narrative detection input struct.  Fields not available in the
// snapshot (GeopoliticalGPR, RetailInstitutionalDivergence, MarginZScore)
// are zeroed; the caller should overlay query-param overrides if desired.
func (s *NarrativeService) BuildMarketNarrativeData(ctx context.Context) (narrative.MarketNarrativeData, error) {
	if s.macroProvider == nil {
		return narrative.MarketNarrativeData{}, fmt.Errorf("macro provider not set")
	}
	snap, err := s.macroProvider.FetchSnapshot(ctx)
	if err != nil {
		return narrative.MarketNarrativeData{}, fmt.Errorf("fetch snapshot: %w", err)
	}
	return narrative.MarketNarrativeDataFromSnapshot(snap), nil
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
	return s.NarrativeEngine.GetStressIndexHistory(days)
}

func (s *NarrativeService) GetStressIndexThresholds() narrative.StressIndexThresholds {
	return s.NarrativeEngine.GetStressIndexThresholds()
}
