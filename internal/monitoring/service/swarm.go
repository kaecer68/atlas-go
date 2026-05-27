package service

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

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
}

// NewSwarmService creates a SwarmService reading from the given snapshot path.
func NewSwarmService(snapshotPath string) *SwarmService {
	return &SwarmService{snapshotPath: snapshotPath}
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
