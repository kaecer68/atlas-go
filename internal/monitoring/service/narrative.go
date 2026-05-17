package service

import (
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type NarrativeService struct {
	WorkDir         string
	NarrativeEngine *narrative.NarrativeEngine
	ReportGenerator *narrative.ReportGenerator
}

func NewNarrativeService(workDir string, narrativeEngine *narrative.NarrativeEngine, reportGenerator *narrative.ReportGenerator) *NarrativeService {
	return &NarrativeService{
		WorkDir:         workDir,
		NarrativeEngine: narrativeEngine,
		ReportGenerator: reportGenerator,
	}
}

func (s *NarrativeService) DetectEvents(data narrative.MarketNarrativeData) []narrative.NarrativeEvent {
	return s.NarrativeEngine.DetectEvents(data)
}

func (s *NarrativeService) MatchChains(events []narrative.NarrativeEvent) []narrative.CausalChain {
	return s.NarrativeEngine.MatchChains(events)
}

func (s *NarrativeService) GetActiveModels(themes []string) []narrative.InvestmentModel {
	replayPath := filepath.Join(s.WorkDir, "data/replay/tw_extended_90days.csv")
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
