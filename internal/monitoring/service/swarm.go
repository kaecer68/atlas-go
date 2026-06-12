package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/metalearning"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

// SwarmStatusResponse summarizes the swarm's latest state for the dashboard.
type SwarmStatusResponse struct {
	RecordedAt          *time.Time `json:"recorded_at"`
	TotalFish           int        `json:"total_fish"`
	ConsensusSymbols    int        `json:"consensus_symbols"`
	ConsensusConfidence float64    `json:"consensus_confidence"`
	TopAccuracy         float64    `json:"top_accuracy"`
	AnomalyCount        int        `json:"anomaly_count"`
	ScenarioCount       int        `json:"scenario_count"`
	GenerationsEvolved  int        `json:"generations_evolved"`
	TrainingScenarios   int        `json:"training_scenarios"`
}

// ConsensusEntry is a per-symbol consensus breakdown.
type ConsensusEntry struct {
	Symbol             string  `json:"symbol"`
	BullishCount       int     `json:"bullish_count"`
	BearishCount       int     `json:"bearish_count"`
	NeutralCount       int     `json:"neutral_count"`
	ConsensusDirection string  `json:"consensus_direction"`
	AverageConfidence  float64 `json:"average_confidence"`
}

// SwarmService provides access to persisted swarm simulation state.
type SwarmService struct {
	snapshotPath string
	trainingDir  string
}

// NewSwarmService creates a SwarmService reading from the given snapshot path.
func NewSwarmService(snapshotPath string) *SwarmService {
	return &SwarmService{snapshotPath: snapshotPath}
}

// SetTrainingDir attaches a training data directory for summary reporting.
func (s *SwarmService) SetTrainingDir(dir string) {
	s.trainingDir = dir
}

// loadSnapshot reads the persisted swarm snapshot from disk.
func (s *SwarmService) loadSnapshot() (*swarm.SwarmSnapshot, error) {
	data, err := os.ReadFile(s.snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("read swarm snapshot: %w", err)
	}
	var snap swarm.SwarmSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal swarm snapshot: %w", err)
	}
	return &snap, nil
}

// LoadStatus returns a summary of the latest swarm simulation.
func (s *SwarmService) LoadStatus() (*SwarmStatusResponse, error) {
	snap, err := s.loadSnapshot()
	if err != nil {
		return nil, err
	}
	return &SwarmStatusResponse{
		RecordedAt:          &snap.RecordedAt,
		TotalFish:           snap.TotalFish,
		ConsensusSymbols:    len(snap.Consensus),
		ConsensusConfidence: snap.ConsensusConfidence,
		TopAccuracy:         snap.TopFishAccuracy,
		AnomalyCount:        len(snap.Anomalies),
		ScenarioCount:       len(snap.Scenarios),
		GenerationsEvolved:  snap.GenerationsEvolved,
		TrainingScenarios:   s.countTrainingScenarios(),
	}, nil
}

// LoadConsensus returns per-symbol consensus breakdown.
func (s *SwarmService) LoadConsensus() ([]ConsensusEntry, error) {
	snap, err := s.loadSnapshot()
	if err != nil {
		return nil, err
	}
	entries := make([]ConsensusEntry, 0, len(snap.Consensus))
	for sym, cp := range snap.Consensus {
		entries = append(entries, ConsensusEntry{
			Symbol:             sym,
			BullishCount:       cp.BullishCount,
			BearishCount:       cp.BearishCount,
			NeutralCount:       cp.NeutralCount,
			ConsensusDirection: cp.ConsensusDirection,
			AverageConfidence:  cp.AverageConfidence,
		})
	}
	return entries, nil
}

// LoadAnomalies returns anomalies from the latest swarm simulation.
func (s *SwarmService) LoadAnomalies() ([]swarm.Anomaly, error) {
	snap, err := s.loadSnapshot()
	if err != nil {
		return nil, err
	}
	return snap.Anomalies, nil
}

// LoadScenarios returns scenario parameters from the latest swarm simulation.
func (s *SwarmService) LoadScenarios() ([]swarm.ScenarioSnapshot, error) {
	snap, err := s.loadSnapshot()
	if err != nil {
		return nil, err
	}
	return snap.Scenarios, nil
}

// StrategySummary represents a single learning strategy for the dashboard.
type StrategySummary struct {
	ID          string                            `json:"id"`
	Name        string                            `json:"name"`
	Type        string                            `json:"type"`
	Score       float64                           `json:"score"`
	Performance *metalearning.StrategyPerformance `json:"performance,omitempty"`
}

// LoadRecommendedStrategies returns top strategies from the MetaLearner state file.
func (s *SwarmService) LoadRecommendedStrategies() ([]StrategySummary, error) {
	mlPath := filepath.Join(filepath.Dir(s.snapshotPath), "metalearner_state.json")
	data, err := os.ReadFile(mlPath)
	if err != nil {
		return nil, fmt.Errorf("read metalearner state: %w", err)
	}
	var state struct {
		Strategies map[string]struct {
			ID          string                            `json:"id"`
			Name        string                            `json:"name"`
			Type        string                            `json:"type"`
			Performance *metalearning.StrategyPerformance `json:"performance,omitempty"`
		} `json:"strategies"`
		Population []string `json:"population"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal metalearner: %w", err)
	}
	results := make([]StrategySummary, 0, 5)
	for _, id := range state.Population {
		s, ok := state.Strategies[id]
		if !ok {
			continue
		}
		perf := s.Performance
		score := 0.0
		if perf != nil && perf.TotalApplications > 0 {
			score = float64(perf.SuccessCount) / float64(perf.TotalApplications)
		}
		results = append(results, StrategySummary{
			ID:          s.ID,
			Name:        s.Name,
			Type:        s.Type,
			Score:       score,
			Performance: perf,
		})
		if len(results) >= 5 {
			break
		}
	}
	if results == nil {
		return []StrategySummary{}, nil
	}
	return results, nil
}

// countTrainingScenarios counts total persisted training entries across all scenario files.
func (s *SwarmService) countTrainingScenarios() int {
	if s.trainingDir == "" {
		return 0
	}
	entries, err := os.ReadDir(s.trainingDir)
	if err != nil {
		return 0
	}
	total := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".jsonl") {
			f, err := os.Open(filepath.Join(s.trainingDir, entry.Name()))
			if err != nil {
				continue
			}
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				if strings.TrimSpace(scanner.Text()) != "" {
					total++
				}
			}
			_ = f.Close()
		}
	}
	return total
}
