package service

import (
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/industry"
)

// PR5-1 — provider injection extracted from pipeline.go (Issue #611 sub-issue-5):
//   - RegistryProviderFunc + NarrativeProviderFunc + CycleProviderFunc +
//     CycleCardProviderFunc: lazy-loaded dependency injection types.
//   - WithRegistryProvider / WithNarrativeProvider / WithCycleProvider /
//     WithCycleCardProvider: fluent setters for these providers.
//
// The provider types and their fluent setters are extracted to their own file
// to clarify the DI contract separately from the data-loading logic.

type (
	NarrativeProviderFunc func(eventIDs []string) *NarrativeContextData
	CycleProviderFunc     func(skill string) *IndustryContextData
	CycleCardProviderFunc func() *industry.CycleStatusCard
)

type RegistryProviderFunc func() (domain.AgentRegistry, error)

func (s *PipelineService) WithRegistryProvider(fn RegistryProviderFunc) *PipelineService {
	s.registryProvider = fn
	return s
}

func (s *PipelineService) WithNarrativeProvider(fn NarrativeProviderFunc) *PipelineService {
	s.narrativeProvider = fn
	return s
}

func (s *PipelineService) WithCycleProvider(fn CycleProviderFunc) *PipelineService {
	s.cycleProvider = fn
	return s
}

func (s *PipelineService) WithCycleCardProvider(fn CycleCardProviderFunc) *PipelineService {
	s.cardProvider = fn
	return s
}
