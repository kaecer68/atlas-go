package ledger

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/portfolio"
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
	entries, err := os.ReadDir(filepath.Join(s.baseDir, "sessions"))
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}
	var allOutcomes []domain.RecommendationOutcome
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "session-") {
			continue
		}
		outcomePath := filepath.Join(s.baseDir, "sessions", entry.Name(), "recommendation_outcomes.jsonl")
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
		outcomes     []domain.RecommendationOutcome
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
		entry.outcomes = append(entry.outcomes, outcome)
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
		sharpe := portfolio.ComputeSharpe(entry.returns, portfolio.SharpeConfig{
			Frequency:  portfolio.FrequencyPerOutcome,
			MinSamples: 2,
		})
		n := len(entry.returns)
		hitRate := ratio(entry.hits, n)
		var tStat, hitRateTStat, confLow, confHigh float64
		if n >= 2 {
			tStat = sharpe * math.Sqrt(float64(n))
			if hitRate > 0 && hitRate < 1 {
				hitRateTStat = (hitRate - 0.5) / math.Sqrt(hitRate*(1-hitRate)/float64(n))
			}
			se := 1.96 / math.Sqrt(float64(n))
			confLow = sharpe - se
			confHigh = sharpe + se
		}

		// Phase 3: IS/OOS split. Per-agent outcomes sorted by RecordedAt
		// before splitting so chronological order is enforced even when
		// callers pass unsorted input.
		sortedOutcomes := portfolio.SortOutcomesByTime(entry.outcomes)
		isTrainOut, isTestOut := portfolio.Split(sortedOutcomes, portfolio.SplitConfig{
			Method:     portfolio.SplitChronological,
			TrainRatio: 0.8,
		})
		trainReturns := make([]float64, len(isTrainOut))
		for i, o := range isTrainOut {
			trainReturns[i] = o.ForwardReturn
		}
		testReturns := make([]float64, len(isTestOut))
		for i, o := range isTestOut {
			testReturns[i] = o.ForwardReturn
		}
		var isSharpe, oosSharpe, oosRatio float64
		var overfitWarning bool
		var overfitReason, oosSampleWarning string
		if len(testReturns) < 5 {
			oosSampleWarning = fmt.Sprintf("insufficient_test_samples: %d < 5", len(testReturns))
			oosSharpe = 0
		} else {
			oosSharpe = portfolio.ComputeSharpe(testReturns, portfolio.SharpeConfig{
				Frequency:  portfolio.FrequencyPerOutcome,
				MinSamples: 2,
			})
		}
		if len(trainReturns) < 10 {
			if oosSampleWarning != "" {
				oosSampleWarning += "; "
			}
			oosSampleWarning += fmt.Sprintf("insufficient_train_samples: %d < 10", len(trainReturns))
			isSharpe = 0
		} else {
			isSharpe = portfolio.ComputeSharpe(trainReturns, portfolio.SharpeConfig{
				Frequency:  portfolio.FrequencyPerOutcome,
				MinSamples: 2,
			})
		}
		if isSharpe != 0 || oosSharpe != 0 {
			oosRatio = math.Abs(isSharpe) / math.Max(math.Abs(oosSharpe), 0.01)
			if portfolio.IsOOSDivergent(isSharpe, oosSharpe, 2.0) {
				overfitWarning = true
				if oosSharpe <= 0 && isSharpe > 0 {
					overfitReason = fmt.Sprintf("is_positive_oos_non_positive:is=%.3f oos=%.3f", isSharpe, oosSharpe)
				} else {
					overfitReason = fmt.Sprintf("is_oos_ratio=%.2f>2.0:is=%.3f oos=%.3f", oosRatio, isSharpe, oosSharpe)
				}
			}
		}

		// Phase 3: rolling Sharpe trend (per-window Sharpe linear slope).
		var trendSlope float64
		if len(daily) >= 2 {
			trendSlope = sharpeTrendSlope(daily)
		}

		scorecards = append(scorecards, domain.Scorecard{
			AgentID:                  entry.agentID,
			Skill:                    entry.skill,
			Layer:                    domain.AgentLayer(entry.layer),
			WindowCount:              len(entry.windows),
			Observations:             n,
			HitRate:                  hitRate,
			AverageReturn:            avg,
			SharpeLike:               sharpe,
			MaxDrawdown:              maxDrawdown(daily),
			TStat:                    tStat,
			HitRateTStat:             hitRateTStat,
			ConfidenceLow:            confLow,
			ConfidenceHigh:           confHigh,
			StatisticallySignificant: len(entry.windows) >= 20,
			LastUpdatedAt:            time.Now(),
			IsSharpe:                 isSharpe,
			OosSharpe:                oosSharpe,
			IsOosRatio:               oosRatio,
			OverfitWarning:           overfitWarning,
			OverfitReason:            overfitReason,
			RollingSharpeTrend:       trendSlope,
			OosSampleWarning:         oosSampleWarning,
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

func sharpeTrendSlope(values []float64) float64 {
	n := len(values)
	if n < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	for i, v := range values {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumXX += x * x
	}
	denom := float64(n)*sumXX - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (float64(n)*sumXY - sumX*sumY) / denom
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
