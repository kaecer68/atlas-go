package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type Store struct {
	baseDir string
}

func NewStore(baseDir string) *Store {
	return &Store{baseDir: baseDir}
}

func (s *Store) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(s.baseDir, "recommendation_outcomes.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, outcome := range outcomes {
		if err := enc.Encode(outcome); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o755); err != nil {
		return err
	}

	path := filepath.Join(s.sessionDir(session.ID), "recommendation_outcomes.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, outcome := range outcomes {
		if err := enc.Encode(outcome); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	path := filepath.Join(s.baseDir, "recommendation_outcomes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	outcomes := make([]domain.RecommendationOutcome, 0)
	for scanner.Scan() {
		var outcome domain.RecommendationOutcome
		if err := json.Unmarshal(scanner.Bytes(), &outcome); err != nil {
			return nil, fmt.Errorf("decode outcome: %w", err)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, scanner.Err()
}

func (s *Store) RecordExperiment(record domain.ExperimentRecord) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return err
	}

	path := filepath.Join(s.baseDir, "experiments.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(record)
}

func (s *Store) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o755); err != nil {
		return err
	}

	path := filepath.Join(s.sessionDir(session.ID), "experiments.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(record)
}

func (s *Store) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o755); err != nil {
		return err
	}

	path := filepath.Join(s.sessionDir(session.ID), "summary.json")
	bytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

func (s *Store) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	root := filepath.Join(s.baseDir, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	outcomes := make([]domain.RecommendationOutcome, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "recommendation_outcomes.jsonl")
		fileOutcomes, err := loadOutcomeFile(path)
		if err != nil {
			return nil, nil, err
		}
		outcomes = append(outcomes, fileOutcomes...)
	}

	return BuildScorecards(outcomes), outcomes, nil
}

func (s *Store) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	dir := filepath.Join(s.baseDir, "windows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, summary.WindowID+".json")
	bytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

func (s *Store) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	dir := filepath.Join(s.baseDir, "windows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, windowID+"-mutation-brief.json")
	bytes, err := json.MarshalIndent(brief, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

// SpawnRecord captures the lifecycle audit trail for a spawned agent.
type SpawnRecord struct {
	AgentID         string    `json:"agent_id"`
	GapID           string    `json:"gap_id"`
	GapPattern      string    `json:"gap_pattern"`
	CreatedAt       time.Time `json:"created_at"`
	TrainingSharpe  float64   `json:"training_sharpe"`
	TrainingHitRate float64   `json:"training_hit_rate"`
	FinalFate       string    `json:"final_fate"` // active / extinct / promoted
	ExtinctAt       time.Time `json:"extinct_at,omitempty"`
	PromotedAt      time.Time `json:"promoted_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *Store) RecordSpawnRecord(record SpawnRecord) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.baseDir, "spawn_records.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	record.UpdatedAt = time.Now()
	return json.NewEncoder(f).Encode(record)
}

func (s *Store) LoadSpawnRecords() ([]SpawnRecord, error) {
	path := filepath.Join(s.baseDir, "spawn_records.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	records := make([]SpawnRecord, 0)
	for scanner.Scan() {
		var rec SpawnRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decode spawn record: %w", err)
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

func (s *Store) RecordHumanIntervention(intervention domain.HumanIntervention) error {
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(s.baseDir, "human_interventions.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(intervention)
}

func (s *Store) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	path := filepath.Join(s.baseDir, "human_interventions.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	records := make([]domain.HumanIntervention, 0)
	for scanner.Scan() {
		var rec domain.HumanIntervention
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decode human intervention: %w", err)
		}
		records = append(records, rec)
	}
	return records, scanner.Err()
}

func (s *Store) RecordPromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	dir := filepath.Join(s.baseDir, "experiments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, experimentID+".json")
	bytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes, 0o644)
}

func (s *Store) UpdatePromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	return s.RecordPromptExperimentResult(experimentID, result)
}

func BuildScorecards(outcomes []domain.RecommendationOutcome) []domain.Scorecard {
	type agg struct {
		agentID string
		skill   string
		returns []float64
		hits    int
		windows map[string]struct{}
	}

	byAgent := map[string]*agg{}
	for _, outcome := range outcomes {
		key := outcome.AgentID
		entry, ok := byAgent[key]
		if !ok {
			entry = &agg{
				agentID: outcome.AgentID,
				skill:   outcome.Skill,
				windows: map[string]struct{}{},
			}
			byAgent[key] = entry
		}
		entry.returns = append(entry.returns, outcome.ForwardReturn)
		if outcome.Hit {
			entry.hits++
		}
		if outcome.Window != "" {
			entry.windows[outcome.Window] = struct{}{}
		}
	}

	scorecards := make([]domain.Scorecard, 0, len(byAgent))
	for _, entry := range byAgent {
		avg := mean(entry.returns)
		scorecards = append(scorecards, domain.Scorecard{
			AgentID:       entry.agentID,
			Skill:         entry.skill,
			WindowCount:   len(entry.windows),
			Observations:  len(entry.returns),
			HitRate:       ratio(entry.hits, len(entry.returns)),
			AverageReturn: avg,
			SharpeLike:    sharpeLike(entry.returns),
			MaxDrawdown:   maxDrawdown(entry.returns),
			LastUpdatedAt: time.Now(),
		})
	}

	slices.SortFunc(scorecards, func(a, b domain.Scorecard) int {
		switch {
		case a.SharpeLike < b.SharpeLike:
			return -1
		case a.SharpeLike > b.SharpeLike:
			return 1
		default:
			return 0
		}
	})

	return scorecards
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func ratio(hitCount, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(hitCount) / float64(total)
}

func sharpeLike(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	avg := mean(values)
	var variance float64
	for _, v := range values {
		diff := v - avg
		variance += diff * diff
	}
	variance /= float64(len(values))
	if variance == 0 {
		return avg
	}
	return avg / (variance + 1e-9)
}

func maxDrawdown(values []float64) float64 {
	equity := 1.0
	peak := 1.0
	maxDD := 0.0
	for _, v := range values {
		equity *= 1 + v
		if equity > peak {
			peak = equity
		}
		dd := (peak - equity) / peak
		if dd > maxDD {
			maxDD = dd
		}
	}
	return maxDD
}

func (s *Store) sessionDir(sessionID string) string {
	return filepath.Join(s.baseDir, "sessions", sessionID)
}

// LoadSessionSummaries reads all session summaries stored in the ledger.
func (s *Store) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	root := filepath.Join(s.baseDir, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	summaries := make([]domain.SessionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "summary.json")
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read summary %s: %w", path, err)
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(data, &summary); err != nil {
			return nil, fmt.Errorf("decode summary %s: %w", path, err)
		}
		summaries = append(summaries, summary)
	}

	return summaries, nil
}

func loadOutcomeFile(path string) ([]domain.RecommendationOutcome, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	outcomes := make([]domain.RecommendationOutcome, 0)
	for scanner.Scan() {
		var outcome domain.RecommendationOutcome
		if err := json.Unmarshal(scanner.Bytes(), &outcome); err != nil {
			return nil, fmt.Errorf("decode outcome: %w", err)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, scanner.Err()
}
