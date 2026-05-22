package service

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

type ControlService struct {
	WorkDir          string
	LedgerDir        string
	HealthManager    *portfolio.AgentHealthManager
	registryProvider RegistryProviderFunc
	store            ledger.OutcomeStore
}

func NewControlService(workDir, ledgerDir string, healthManager *portfolio.AgentHealthManager, store ledger.OutcomeStore) *ControlService {
	return &ControlService{
		WorkDir:       workDir,
		LedgerDir:     ledgerDir,
		HealthManager: healthManager,
		store:         store,
	}
}

func (s *ControlService) RecordIntervention(intervention domain.HumanIntervention) error {
	return s.store.RecordHumanIntervention(intervention)
}

func (s *ControlService) WithRegistryProvider(fn RegistryProviderFunc) *ControlService {
	s.registryProvider = fn
	return s
}

func (s *ControlService) loadRegistry() (domain.AgentRegistry, error) {
	if s.registryProvider != nil {
		return s.registryProvider()
	}
	registryPath := filepath.Join(s.WorkDir, "configs/agents.json")
	return orchestrator.LoadRegistry(registryPath)
}

func (s *ControlService) LoadInterventions() ([]domain.HumanIntervention, error) {
	interventions, err := s.store.LoadHumanInterventions()
	if err != nil {
		return nil, err
	}
	slices.Reverse(interventions)
	return interventions, nil
}

func (s *ControlService) GetActiveOverrides() (pausedAgents, bannedSectors []string, modelWeights map[string]float64) {
	interventions, err := s.store.LoadHumanInterventions()
	if err != nil {
		return nil, nil, nil
	}

	paused := make(map[string]bool)
	banned := make(map[string]bool)
	weights := make(map[string]float64)

	for _, iv := range interventions {
		if iv.IsExpired() {
			continue
		}
		switch iv.Type {
		case "pause_agent":
			paused[iv.TargetAgentID] = true
		case "resume_agent":
			delete(paused, iv.TargetAgentID)
		case "sector_ban":
			banned[iv.TargetSector] = true
		case "sector_unban":
			delete(banned, iv.TargetSector)
		case "set_model_weight":
			weights[iv.TargetModelID] = iv.Value
		}
	}

	return mapKeys(paused), mapKeys(banned), weights
}

func (s *ControlService) GetAgentHealth() ([]*portfolio.AgentHealth, int, error) {
	if s.HealthManager == nil {
		return []*portfolio.AgentHealth{}, 0, nil
	}

	registry, err := s.loadRegistry()
	if err != nil {
		registry = orchestrator.SeedRegistry()
	}

	agents := make([]*portfolio.AgentHealth, 0)
	mutedCount := 0

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		h := s.HealthManager.GetHealth(agent.ID)
		if h == nil {
			h = &portfolio.AgentHealth{
				AgentID: agent.ID,
				Status:  portfolio.HealthStatusHealthy,
			}
		}
		agents = append(agents, h)
		if h.Status == portfolio.HealthStatusMuted {
			mutedCount++
		}
	}

	return agents, mutedCount, nil
}

func (s *ControlService) CreateIntervention(interventionType, targetID, reason, operator string, value float64) domain.HumanIntervention {
	now := time.Now().UTC()
	var id, targetAgentID, targetSymbol, targetSector, targetModelID string
	var ttlHours int

	switch interventionType {
	case "pause_agent":
		id = fmt.Sprintf("int-pause-%s-%d", targetID, now.UnixNano())
		targetAgentID = targetID
		ttlHours = 24
	case "resume_agent":
		id = fmt.Sprintf("int-resume-%s-%d", targetID, now.UnixNano())
		targetAgentID = targetID
	case "set_model_weight":
		id = fmt.Sprintf("int-model-%s-%d", targetID, now.UnixNano())
		targetModelID = targetID
		ttlHours = 72
	case "sector_ban", "sector_unban":
		id = fmt.Sprintf("int-sector-%s-%d", targetID, now.UnixNano())
		targetSector = targetID
		ttlHours = 24
	case "approve_rec":
		id = fmt.Sprintf("int-approve-%s-%d", targetID, now.UnixNano())
		if parts := strings.SplitN(targetID, ":", 2); len(parts) == 2 {
			targetAgentID = parts[0]
			targetSymbol = parts[1]
		} else {
			targetSymbol = targetID
		}
		ttlHours = 48
	case "reject_rec":
		id = fmt.Sprintf("int-reject-%s-%d", targetID, now.UnixNano())
		if parts := strings.SplitN(targetID, ":", 2); len(parts) == 2 {
			targetAgentID = parts[0]
			targetSymbol = parts[1]
		} else {
			targetSymbol = targetID
		}
		ttlHours = 48
	}

	hi := domain.HumanIntervention{
		ID:            id,
		Type:          interventionType,
		TargetAgentID: targetAgentID,
		TargetSymbol:  targetSymbol,
		TargetSector:  targetSector,
		TargetModelID: targetModelID,
		Value:         value,
		Reason:        reason,
		Operator:      operator,
		RecordedAt:    now,
		TTLHours:      ttlHours,
	}
	if ttlHours > 0 {
		hi.ExpiresAt = now.Add(time.Duration(ttlHours) * time.Hour)
	}
	return hi
}

func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
