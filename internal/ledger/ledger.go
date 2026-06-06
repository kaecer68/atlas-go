package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type Store struct {
	baseDir string
	mu      sync.Mutex
}

func NewStore(baseDir string) OutcomeStore {
	return &Store{baseDir: baseDir}
}

func (s *Store) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}

	path := filepath.Join(s.baseDir, "recommendation_outcomes.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, outcome := range outcomes {
		if err := outcome.Validate(); err != nil {
			logging.Warn(
				"ledger", "outcome_validation_failed",
				logging.AgentID(outcome.AgentID),
				"symbol", outcome.Symbol,
				"error", err.Error(),
			)
		}
		if err := enc.Encode(outcome); err != nil {
			_ = f.Close()
			return fmt.Errorf("encode outcome: %w", err)
		}
	}
	return f.Close()
}

func (s *Store) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o755); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	path := filepath.Join(s.sessionDir(session.ID), "recommendation_outcomes.jsonl")
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	enc := json.NewEncoder(f)
	for _, outcome := range outcomes {
		if err := outcome.Validate(); err != nil {
			logging.Warn(
				"ledger", "outcome_validation_failed",
				logging.AgentID(outcome.AgentID),
				"symbol", outcome.Symbol,
				"error", err.Error(),
			)
		}
		if err := enc.Encode(outcome); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("encode outcome: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close file: %w", err)
	}
	return os.Rename(tmp, path)
}

// LoadSessionOutcomes reads per-session recommendation outcomes from the session directory.
func (s *Store) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.sessionDir(sessionID), "recommendation_outcomes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

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

// LoadOutcomesFromSessions aggregates outcomes from all session directories.
// This is the richest data source with per-agent, per-symbol forward returns.
// Prefer this over LoadOutcomes() which reads from the sparse global file.
func (s *Store) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}
	var allOutcomes []domain.RecommendationOutcome
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "session-") {
			continue
		}
		outcomePath := filepath.Join(s.baseDir, entry.Name(), "recommendation_outcomes.jsonl")
		f, err := os.Open(outcomePath)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var outcome domain.RecommendationOutcome
			if err := json.Unmarshal(scanner.Bytes(), &outcome); err != nil {
				continue
			}
			allOutcomes = append(allOutcomes, outcome)
		}
		_ = f.Close()
	}
	return allOutcomes, nil
}

func (s *Store) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "recommendation_outcomes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}

	path := filepath.Join(s.baseDir, "experiments.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return json.NewEncoder(f).Encode(record)
}

// LoadExperiments reads all experiment records from experiments.jsonl.
func (s *Store) LoadExperiments() ([]domain.ExperimentRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "experiments.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open experiments file: %w", err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	records := make([]domain.ExperimentRecord, 0)
	for scanner.Scan() {
		var record domain.ExperimentRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode experiment record: %w", err)
		}
		records = append(records, record)
	}
	return records, scanner.Err()
}

func (s *Store) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o755); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	path := filepath.Join(s.sessionDir(session.ID), "experiments.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return json.NewEncoder(f).Encode(record)
}

func (s *Store) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o755); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}

	path := filepath.Join(s.sessionDir(session.ID), "summary.json")
	bytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return os.Rename(tmp, path)
}

func (s *Store) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := filepath.Join(s.baseDir, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read dir: %w", err)
	}

	outcomes := make([]domain.RecommendationOutcome, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "recommendation_outcomes.jsonl")
		fileOutcomes, err := loadOutcomeFile(path)
		if err != nil {
			return nil, nil, fmt.Errorf("load outcome file %s: %w", path, err)
		}
		outcomes = append(outcomes, fileOutcomes...)
	}

	return BuildScorecards(outcomes), outcomes, nil
}

func (s *Store) RecordWindowSummary(summary domain.BacktestWindowSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.baseDir, "windows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir window dir: %w", err)
	}
	path := filepath.Join(dir, summary.WindowID+".json")
	bytes, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return os.Rename(tmp, path)
}

func (s *Store) RecordMutationBrief(windowID string, brief domain.MutationBrief) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.baseDir, "windows")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir window dir: %w", err)
	}
	path := filepath.Join(dir, windowID+"-mutation-brief.json")
	bytes, err := json.MarshalIndent(brief, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal brief: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return os.Rename(tmp, path)
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
	ExtinctAt       time.Time `json:"extinct_at"`
	PromotedAt      time.Time `json:"promoted_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (s *Store) RecordSpawnRecord(record SpawnRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}
	path := filepath.Join(s.baseDir, "spawn_records.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	record.UpdatedAt = time.Now()
	return json.NewEncoder(f).Encode(record)
}

func (s *Store) LoadSpawnRecords() ([]SpawnRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "spawn_records.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.baseDir, 0o755); err != nil {
		return fmt.Errorf("mkdir all: %w", err)
	}
	path := filepath.Join(s.baseDir, "human_interventions.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	return json.NewEncoder(f).Encode(intervention)
}

func (s *Store) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.baseDir, "human_interventions.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.baseDir, "experiments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir experiment dir: %w", err)
	}
	path := filepath.Join(dir, experimentID+".json")
	bytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return os.Rename(tmp, path)
}

func (s *Store) UpdatePromptExperimentResult(experimentID string, result domain.PromptExperimentResult) error {
	return s.RecordPromptExperimentResult(experimentID, result)
}

func BuildScorecards(outcomes []domain.RecommendationOutcome) []domain.Scorecard {
	type agg struct {
		agentID      string
		skill        string
		layer        string
		returns      []float64
		hits         int
		windows      map[string]struct{}
		dailyReturns map[string]float64
		dailyCounts  map[string]int
	}

	byAgent := map[string]*agg{}
	for _, outcome := range outcomes {
		key := outcome.AgentID
		entry, ok := byAgent[key]
		if !ok {
			entry = &agg{
				agentID:      outcome.AgentID,
				skill:        outcome.Skill,
				layer:        string(outcome.Layer),
				windows:      map[string]struct{}{},
				dailyReturns: map[string]float64{},
				dailyCounts:  map[string]int{},
			}
			byAgent[key] = entry
		}
		entry.returns = append(entry.returns, outcome.ForwardReturn)
		if outcome.Hit {
			entry.hits++
		}
		if outcome.Window != "" {
			entry.windows[outcome.Window] = struct{}{}
			entry.dailyReturns[outcome.Window] += outcome.ForwardReturn
			entry.dailyCounts[outcome.Window]++
		}
	}

	scorecards := make([]domain.Scorecard, 0, len(byAgent))
	for _, entry := range byAgent {
		avg := mean(entry.returns)
		daily := make([]float64, 0, len(entry.windows))
		for w := range entry.windows {
			if c := entry.dailyCounts[w]; c > 0 {
				daily = append(daily, entry.dailyReturns[w]/float64(c))
			}
		}
		scorecards = append(scorecards, domain.Scorecard{
			AgentID:       entry.agentID,
			Skill:         entry.skill,
			Layer:         domain.AgentLayer(entry.layer),
			WindowCount:   len(entry.windows),
			Observations:  len(entry.returns),
			HitRate:       ratio(entry.hits, len(entry.returns)),
			AverageReturn: avg,
			SharpeLike:    sharpeLike(entry.returns),
			MaxDrawdown:   maxDrawdown(daily),
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

func (s *Store) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.sessionDir(sessionID), 0o755); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}
	path := filepath.Join(s.sessionDir(sessionID), "screened_symbols.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, r := range rejects {
		if err := enc.Encode(r); err != nil {
			return fmt.Errorf("encode reject: %w", err)
		}
	}
	return nil
}

func (s *Store) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.sessionDir(sessionID), "screened_symbols.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	rejects := make([]domain.ScreeningReject, 0)
	for scanner.Scan() {
		var rec domain.ScreeningReject
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decode screening reject: %w", err)
		}
		rejects = append(rejects, rec)
	}
	return rejects, scanner.Err()
}

func (s *Store) RecordSessionTrades(sessionID string, trades []domain.TradeRecord) error {
	if len(trades) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.sessionDir(sessionID), 0o755); err != nil {
		return fmt.Errorf("mkdir session dir: %w", err)
	}
	path := filepath.Join(s.sessionDir(sessionID), "trades.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, trade := range trades {
		if err := enc.Encode(trade); err != nil {
			return fmt.Errorf("encode trade: %w", err)
		}
	}
	return nil
}

func (s *Store) LoadSessionTrades(sessionID string) ([]domain.TradeRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := filepath.Join(s.sessionDir(sessionID), "trades.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	trades := make([]domain.TradeRecord, 0)
	for scanner.Scan() {
		var rec domain.TradeRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			return nil, fmt.Errorf("decode trade: %w", err)
		}
		trades = append(trades, rec)
	}
	return trades, scanner.Err()
}

func (s *Store) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	root := filepath.Join(s.baseDir, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}
	all := make([]domain.TradeRecord, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		trades, err := s.LoadSessionTrades(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("load session trades %s: %w", entry.Name(), err)
		}
		all = append(all, trades...)
	}
	slices.SortFunc(all, func(a, b domain.TradeRecord) int {
		return b.Timestamp.Compare(a.Timestamp)
	})
	return all, nil
}

func (s *Store) sessionDir(sessionID string) string {
	return filepath.Join(s.baseDir, "sessions", sessionID)
}

// LoadSessionSummaries reads all session summaries stored in the ledger.
func (s *Store) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := filepath.Join(s.baseDir, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
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
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

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
