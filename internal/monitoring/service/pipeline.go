package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// PipelineService encapsulates dashboard pipeline data loading operations.
type PipelineService struct {
	WorkDir          string
	LedgerDir        string
	registryProvider RegistryProviderFunc
}

type RegistryProviderFunc func() (domain.AgentRegistry, error)

func (s *PipelineService) WithRegistryProvider(fn RegistryProviderFunc) *PipelineService {
	s.registryProvider = fn
	return s
}

func NewPipelineService(workDir, ledgerDir string) *PipelineService {
	return &PipelineService{
		WorkDir:   workDir,
		LedgerDir: ledgerDir,
	}
}

func (s *PipelineService) loadRegistry() (domain.AgentRegistry, error) {
	if s.registryProvider != nil {
		return s.registryProvider()
	}
	registryPath := filepath.Join(s.WorkDir, "configs/agents.json")
	return orchestrator.LoadRegistry(registryPath)
}

// LoadMacroRadar loads macro radar summary for the given session.
func (s *PipelineService) LoadMacroRadar(sessionID string) (*MacroRadarData, error) {
	summary, err := LoadSessionSummary(s.LedgerDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load macro radar data: %w", err)
	}
	if summary == nil {
		return nil, nil
	}

	return &MacroRadarData{
		SessionID:     summary.SessionID,
		Regime:        summary.Regime,
		GuardOutcomes: append([]domain.GuardOutcome(nil), summary.GuardOutcomes...),
		BrokerRuntime: summary.BrokerRuntime,
		RecordedAt:    summary.RecordedAt,
	}, nil
}

// MacroRadarData is the internal representation for macro radar response.
type MacroRadarData struct {
	SessionID     string
	Regime        domain.Regime
	GuardOutcomes []domain.GuardOutcome
	BrokerRuntime domain.BrokerRuntimeAudit
	RecordedAt    time.Time
}

// LoadAgentObservatory loads agent observatory data with scorecards.
func (s *PipelineService) LoadAgentObservatory(sessionID string, limit int) (*AgentObservatoryData, error) {
	summary, err := LoadSessionSummary(s.LedgerDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load agent observatory summary: %w", err)
	}

	store := ledger.NewStore(s.LedgerDir)
	outcomes, err := store.LoadOutcomes()
	if err != nil {
		return nil, fmt.Errorf("load recommendation outcomes: %w", err)
	}
	scorecards := ledger.BuildScorecards(outcomes)
	if len(scorecards) > limit {
		scorecards = scorecards[:limit]
	}

	data := &AgentObservatoryData{
		WeakestAgentScorecards: scorecards,
	}
	if summary != nil {
		data.SessionID = summary.SessionID
		data.NextExperimentAgentID = summary.NextExperimentAgentID
		data.BrokerRuntime = summary.BrokerRuntime
		data.RecordedAt = summary.RecordedAt
	}
	return data, nil
}

// AgentObservatoryData is the internal representation for agent observatory response.
type AgentObservatoryData struct {
	SessionID              string
	NextExperimentAgentID  string
	WeakestAgentScorecards []domain.Scorecard
	BrokerRuntime          domain.BrokerRuntimeAudit
	RecordedAt             time.Time
}

// LoadForecastVsReality loads forecast vs reality experiment data.
func (s *PipelineService) LoadForecastVsReality(agentID string, limit int) (*ForecastVsRealityData, error) {
	items, err := s.loadForecastVsRealityItems(agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("load forecast-vs-reality data: %w", err)
	}

	predictions, err := s.loadSymbolPredictions(limit)
	if err != nil {
		logging.Warn("pipeline_service", "load_symbol_predictions", logging.Err(err))
		predictions = nil
	}

	data := &ForecastVsRealityData{Items: items, SymbolPredictions: predictions}
	summary, err := LoadSessionSummary(s.LedgerDir, "")
	if err != nil {
		return data, nil
	}
	if summary != nil {
		data.BrokerRuntime = summary.BrokerRuntime
	}
	return data, nil
}

func (s *PipelineService) loadSymbolPredictions(limit int) ([]SymbolPredictionItem, error) {
	path := filepath.Join(s.LedgerDir, "recommendation_outcomes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	type rawOutcome struct {
		AgentID       string  `json:"agent_id"`
		Symbol        string  `json:"symbol"`
		Side          string  `json:"side"`
		Conviction    int     `json:"conviction"`
		TargetPrice   float64 `json:"target_price"`
		ForwardReturn float64 `json:"forward_return"`
		Hit           bool    `json:"hit"`
		PassedGuards  bool    `json:"passed_guards"`
		RecordedAt    string  `json:"recorded_at"`
		SessionID     string  `json:"session_id"`
	}

	var outcomes []rawOutcome
	var dropped int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var out rawOutcome
		if err := json.Unmarshal(scanner.Bytes(), &out); err != nil {
			dropped++
			continue
		}
		outcomes = append(outcomes, out)
	}
	if dropped > 0 {
		logging.Warn("pipeline_service", "dropped_corrupted_outcomes", logging.FInt("count", dropped))
	}

	slices.SortFunc(outcomes, func(a, b rawOutcome) int {
		if a.SessionID > b.SessionID {
			return -1
		}
		if a.SessionID < b.SessionID {
			return 1
		}
		return 0
	})

	if len(outcomes) > limit {
		outcomes = outcomes[:limit]
	}

	result := make([]SymbolPredictionItem, len(outcomes))
	for i, o := range outcomes {
		result[i] = SymbolPredictionItem{
			AgentID:       o.AgentID,
			Symbol:        o.Symbol,
			Side:          o.Side,
			Conviction:    o.Conviction,
			TargetPrice:   o.TargetPrice,
			ForwardReturn: o.ForwardReturn,
			Hit:           o.Hit,
			PassedGuards:  o.PassedGuards,
			RecordedAt:    o.RecordedAt,
			SessionID:     o.SessionID,
		}
	}
	return result, nil
}

// ForecastVsRealityData is the internal representation for forecast vs reality response.
type ForecastVsRealityData struct {
	Items             []ForecastVsRealityItem
	SymbolPredictions []SymbolPredictionItem
	BrokerRuntime     domain.BrokerRuntimeAudit
}

// SymbolPredictionItem represents a single symbol's prediction vs actual outcome.
type SymbolPredictionItem struct {
	AgentID       string  `json:"agent_id"`
	Symbol        string  `json:"symbol"`
	Side          string  `json:"side"`
	Conviction    int     `json:"conviction"`
	TargetPrice   float64 `json:"target_price"`
	ForwardReturn float64 `json:"forward_return"`
	Hit           bool    `json:"hit"`
	PassedGuards  bool    `json:"passed_guards"`
	RecordedAt    string  `json:"recorded_at"`
	SessionID     string  `json:"session_id"`
}

// ForecastVsRealityItem represents a single experiment result.
type ForecastVsRealityItem struct {
	ExperimentID   string
	ProposalID     string
	CommitID       string
	ApprovalID     string
	TargetAgentID  string
	Skill          string
	MutationType   string
	Status         domain.ExperimentStatus
	BaselineValue  float64
	CandidateValue float64
	RecordedAt     time.Time
}

func (s *PipelineService) loadForecastVsRealityItems(agentID string, limit int) ([]ForecastVsRealityItem, error) {
	experimentsDir := filepath.Join(s.LedgerDir, "experiments")
	entries, err := os.ReadDir(experimentsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	items := make([]ForecastVsRealityItem, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(experimentsDir, entry.Name())
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var result domain.PromptExperimentResult
		if err := json.Unmarshal(bytes, &result); err != nil {
			return nil, err
		}
		if agentID != "" && result.Experiment.TargetAgentID != agentID {
			continue
		}
		items = append(items, ForecastVsRealityItem{
			ExperimentID:   result.Experiment.ID,
			ProposalID:     result.Experiment.ProposalID,
			CommitID:       result.Experiment.CommitID,
			ApprovalID:     result.Experiment.ApprovalID,
			TargetAgentID:  result.Experiment.TargetAgentID,
			Skill:          result.Experiment.Skill,
			MutationType:   result.Experiment.MutationType,
			Status:         result.Experiment.Status,
			BaselineValue:  result.Experiment.BaselineValue,
			CandidateValue: result.Experiment.CandidateValue,
			RecordedAt:     result.RecordedAt,
		})
	}

	slices.SortFunc(items, func(a, b ForecastVsRealityItem) int {
		switch {
		case a.RecordedAt.After(b.RecordedAt):
			return -1
		case a.RecordedAt.Before(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})

	if len(items) > limit {
		return items[:limit], nil
	}
	return items, nil
}

// LoadRecommendationPipeline loads the recommendation pipeline for a session.
func (s *PipelineService) LoadRecommendationPipeline(sessionID string, showAll bool) (*RecommendationPipelineData, error) {
	summary, err := LoadSessionSummary(s.LedgerDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load recommendation pipeline summary: %w", err)
	}
	if summary == nil {
		return nil, nil
	}

	var ds *replay.Dataset
	cfg := config.Load()
	replayPath := cfg.ReplayDataPath
	if replayPath == "samples/replay/twse_stock_day_all_sample.csv" {
		replayPath = "data/replay/tw_extended_90days.csv"
	}
	if replayPath != "" {
		if !filepath.IsAbs(replayPath) {
			replayPath = filepath.Join(s.WorkDir, replayPath)
		}
		if dsTmp, err := replay.LoadTWSEOpenDataCSV(replayPath); err == nil {
			ds = dsTmp
		}
	}

	sessionsDir := filepath.Join(s.LedgerDir, "sessions")
	outcomesPath := filepath.Join(sessionsDir, summary.SessionID, "recommendation_outcomes.jsonl")
	items := make([]PipelineItemData, 0)
	if data, err := os.ReadFile(outcomesPath); err == nil {
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var outcome struct {
				AgentID             string                      `json:"AgentID"`
				Skill               string                      `json:"Skill"`
				Layer               string                      `json:"Layer"`
				Symbol              string                      `json:"Symbol"`
				Side                string                      `json:"Side"`
				Conviction          int                         `json:"Conviction"`
				TargetPrice         float64                     `json:"TargetPrice"`
				StopLossPrice       float64                     `json:"StopLossPrice"`
				ForwardReturn       float64                     `json:"ForwardReturn"`
				Hit                 bool                        `json:"Hit"`
				Reason              string                      `json:"Reason"`
				Price               float64                     `json:"Price"`
				PassedGuards        bool                        `json:"PassedGuards"`
				GuardReason         string                      `json:"GuardReason"`
				RecordedAt          time.Time                   `json:"RecordedAt"`
				FactorScores        domain.FactorScores         `json:"factor_scores,omitempty"`
				ConvictionBreakdown *domain.ConvictionBreakdown `json:"conviction_breakdown,omitempty"`
			}
			if err := json.Unmarshal([]byte(line), &outcome); err != nil {
				continue
			}
			fr := outcome.ForwardReturn
			price := outcome.Price
			side := outcome.Side
			passedGuards := outcome.PassedGuards
			// Legacy sessions (generated before PassedGuards field existed)
			// should default to true to preserve backward-compatible display.
			if !passedGuards && !strings.Contains(line, `"PassedGuards"`) {
				passedGuards = true
			}
			if ds != nil && !outcome.RecordedAt.IsZero() {
				if fr == 0 {
					if recalculated, ok := ds.ForwardReturn(outcome.Symbol, outcome.RecordedAt, 1); ok {
						fr = recalculated
					}
				}
				if price == 0 {
					if bar, ok := ds.ByDate[outcome.RecordedAt.Format("2006-01-02")][outcome.Symbol]; ok {
						price = bar.Close
					}
				}
			}
			if side == "" {
				side = string(domain.SideBuy)
			}
			tp := outcome.TargetPrice
			slp := outcome.StopLossPrice
			if tp == 0 && slp == 0 && price > 0 {
				tp, slp = fallbackPriceTargets(outcome.Skill, price)
			}
			if !showAll && !passedGuards {
				continue
			}
			tags := computePipelineTags(ds, outcome.Symbol, outcome.RecordedAt)
			items = append(items, PipelineItemData{
				Symbol:              outcome.Symbol,
				AgentID:             outcome.AgentID,
				Skill:               outcome.Skill,
				Layer:               outcome.Layer,
				Side:                side,
				Conviction:          outcome.Conviction,
				TargetPrice:         tp,
				StopLossPrice:       slp,
				ForwardReturn:       fr,
				Hit:                 fr > 0,
				Reason:              outcome.Reason,
				Price:               price,
				PassedGuards:        passedGuards,
				GuardReason:         outcome.GuardReason,
				Tags:                tags,
				RecordedAt:          outcome.RecordedAt,
				FactorScores:        outcome.FactorScores,
				ConvictionBreakdown: outcome.ConvictionBreakdown,
			})
		}
	}

	guards := make([]domain.GuardOutcome, 0, len(summary.GuardOutcomes))
	for _, g := range summary.GuardOutcomes {
		guards = append(guards, domain.GuardOutcome{
			GuardID:     g.GuardID,
			GuardSkill:  g.GuardSkill,
			Severity:    g.Severity,
			Passed:      g.Passed,
			Reason:      g.Reason,
			InputCount:  g.InputCount,
			OutputCount: g.OutputCount,
		})
	}

	store := ledger.NewStore(s.LedgerDir)
	screened, err := store.LoadSessionScreeningRejects(summary.SessionID)
	if err != nil {
		// Log but don't fail
		screened = nil
	}

	return &RecommendationPipelineData{
		SessionID:     summary.SessionID,
		Regime:        summary.Regime,
		Items:         items,
		GuardOutcomes: guards,
		ScreenedItems: screened,
		RecordedAt:    summary.RecordedAt,
	}, nil
}

// RecommendationPipelineData is the internal representation for recommendation pipeline response.
type RecommendationPipelineData struct {
	SessionID     string
	Regime        domain.Regime
	Items         []PipelineItemData
	GuardOutcomes []domain.GuardOutcome
	ScreenedItems []domain.ScreeningReject
	RecordedAt    time.Time
}

// PipelineItemData represents a single recommendation in the pipeline.
type PipelineItemData struct {
	Symbol              string
	AgentID             string
	Skill               string
	Layer               string
	Side                string
	Conviction          int
	TargetPrice         float64
	StopLossPrice       float64
	ForwardReturn       float64
	Hit                 bool
	Reason              string
	Price               float64
	PassedGuards        bool
	GuardReason         string
	Tags                []string
	RecordedAt          time.Time
	FactorScores        domain.FactorScores
	ConvictionBreakdown *domain.ConvictionBreakdown
}

// LoadSessions loads all sessions metadata.
func (s *PipelineService) LoadSessions() ([]SessionMeta, error) {
	sessionsDir := filepath.Join(s.LedgerDir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	sessions := make([]SessionMeta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		summaryPath := filepath.Join(sessionsDir, entry.Name(), "summary.json")
		bytes, err := os.ReadFile(summaryPath)
		if err != nil {
			continue
		}
		var summary domain.SessionSummary
		if err := json.Unmarshal(bytes, &summary); err != nil {
			continue
		}
		sessions = append(sessions, SessionMeta{
			SessionID:    summary.SessionID,
			RecordedAt:   summary.RecordedAt,
			Regime:       string(summary.Regime),
			OutcomeCount: summary.OutcomeCount,
		})
	}

	// Sort by session trading date descending, then RecordedAt tiebreaker.
	slices.SortFunc(sessions, func(a, b SessionMeta) int {
		aDate := sessionDateFromID(a.SessionID)
		bDate := sessionDateFromID(b.SessionID)
		switch {
		case aDate.After(bDate):
			return -1
		case aDate.Before(bDate):
			return 1
		case a.RecordedAt.After(b.RecordedAt):
			return -1
		case a.RecordedAt.Before(b.RecordedAt):
			return 1
		default:
			return 0
		}
	})

	return sessions, nil
}

// SessionMeta represents session metadata.
type SessionMeta struct {
	SessionID    string
	RecordedAt   time.Time
	Regime       string
	OutcomeCount int
}

// LoadUniverseOverlap loads agent universe overlap data.
func (s *PipelineService) LoadUniverseOverlap() (*UniverseOverlapData, error) {
	registry, err := s.loadRegistry()
	if err != nil {
		registry = orchestrator.SeedRegistry()
	}

	agents := make([]AgentUniverseViewData, 0)
	byAgent := make(map[string]map[string]struct{})
	warnings := make([]string, 0)

	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		universe := make([]string, len(agent.Universe))
		copy(universe, agent.Universe)
		if len(universe) == 0 {
			universe = orchestrator.DefaultSymbols()
		}
		agents = append(agents, AgentUniverseViewData{
			AgentID:           agent.ID,
			Name:              agent.Name,
			Layer:             string(agent.Layer),
			Universe:          universe,
			ScreeningCriteria: agent.ScreeningCriteria,
		})
		set := make(map[string]struct{}, len(universe))
		for _, sym := range universe {
			set[sym] = struct{}{}
		}
		byAgent[agent.ID] = set
		// Only stock-picking layers are expected to have a dedicated universe.
		// Context and control layers falling back to defaults is by design, not a warning.
		if len(agent.Universe) == 0 && isStockPickingLayer(string(agent.Layer)) {
			warnings = append(warnings, fmt.Sprintf("%s 未設定標的池（fallback 至預設值）", agent.ID))
		}
		if len(universe) < 3 {
			warnings = append(warnings, fmt.Sprintf("%s 標的池僅有 %d 檔標的", agent.ID, len(universe)))
		}
	}

	// Track which agents use a fallback universe so we can exclude them from
	// meaningful overlap calculations (their wide coverage creates noise).
	fallbackAgents := make(map[string]bool)
	for _, v := range agents {
		// In our registry, an empty Universe means fallback to DefaultSymbols.
		// We look up the original agent config to confirm.
		for _, agent := range registry.Agents {
			if agent.ID == v.AgentID {
				fallbackAgents[v.AgentID] = len(agent.Universe) == 0
				break
			}
		}
	}

	matrix := make(map[string]map[string]int)
	for idA, setA := range byAgent {
		matrix[idA] = make(map[string]int)
		for idB, setB := range byAgent {
			if idA == idB {
				continue
			}
			overlap := 0
			for sym := range setA {
				if _, ok := setB[sym]; ok {
					overlap++
				}
			}
			matrix[idA][idB] = overlap
			// Crowding warnings are only meaningful among stock-picking layers.
			// Also skip if either agent uses a fallback universe (wide default coverage creates noise).
			if overlap >= 3 && isStockPickingLayerByID(idA, agents) && isStockPickingLayerByID(idB, agents) && !fallbackAgents[idA] && !fallbackAgents[idB] {
				warnings = append(warnings, fmt.Sprintf("標的池重疊過高：%s ↔ %s（%d 檔）", idA, idB, overlap))
			}
		}
	}

	return &UniverseOverlapData{
		Agents:   agents,
		Matrix:   matrix,
		Warnings: warnings,
	}, nil
}

// UniverseOverlapData is the internal representation for universe overlap response.
type UniverseOverlapData struct {
	Agents   []AgentUniverseViewData
	Matrix   map[string]map[string]int
	Warnings []string
}

// AgentUniverseViewData represents an agent's universe view.
type AgentUniverseViewData struct {
	AgentID           string
	Name              string
	Layer             string
	Universe          []string
	ScreeningCriteria domain.ScreeningCriteria
}

// DarwinianStatusData holds the current Darwinian weight state for the dashboard.
type DarwinianStatusData struct {
	Status       string                        `json:"status"`
	LastComputed string                        `json:"last_computed,omitempty"`
	AgentCount   int                           `json:"agent_count"`
	Agents       map[string]DarwinianAgentInfo `json:"agents"`
}

// DarwinianAgentInfo holds weight and performance data for a single agent.
type DarwinianAgentInfo struct {
	Weight        float64 `json:"weight"`
	RollingSharpe float64 `json:"rolling_sharpe"`
	HitRate       float64 `json:"hit_rate"`
	TotalSignals  int     `json:"total_signals"`
	WinCount      int     `json:"win_count"`
	LossCount     int     `json:"loss_count"`
	AvgReturn     float64 `json:"avg_return"`
	LastUpdated   string  `json:"last_updated,omitempty"`
}

// LoadDarwinianStatus loads the current Darwinian weight state from disk.
func (s *PipelineService) LoadDarwinianHistory(limit int) ([]DarwinianHistoryPoint, error) {
	historyPath := filepath.Join(s.WorkDir, "data/state/darwinian_history.jsonl")
	data, err := os.ReadFile(historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read darwinian history: %w", err)
	}
	var points []DarwinianHistoryPoint
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for i := len(lines) - 1; i >= 0 && len(points) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var snap struct {
			Timestamp string `json:"timestamp"`
			Weights   map[string]struct {
				Weight        float64 `json:"weight"`
				RollingSharpe float64 `json:"rolling_sharpe"`
				HitRate       float64 `json:"hit_rate"`
			} `json:"weights"`
		}
		if err := json.Unmarshal([]byte(line), &snap); err != nil {
			continue
		}
		ts := snap.Timestamp
		for agentID, w := range snap.Weights {
			points = append(points, DarwinianHistoryPoint{
				AgentID:       agentID,
				Timestamp:     ts,
				Weight:        w.Weight,
				RollingSharpe: w.RollingSharpe,
				HitRate:       w.HitRate,
			})
		}
	}
	return points, nil
}

type DarwinianHistoryPoint struct {
	AgentID       string  `json:"agent_id"`
	Timestamp     string  `json:"timestamp"`
	Weight        float64 `json:"weight"`
	RollingSharpe float64 `json:"rolling_sharpe"`
	HitRate       float64 `json:"hit_rate"`
}

func (s *PipelineService) LoadDarwinianStatus() (*DarwinianStatusData, error) {
	weightsPath := filepath.Join(s.WorkDir, "data/state/darwinian_weights.json")
	data, err := os.ReadFile(weightsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &DarwinianStatusData{Status: "not_found"}, nil
		}
		return nil, fmt.Errorf("read darwinian weights: %w", err)
	}
	var saved struct {
		SavedAt string `json:"saved_at"`
		Weights map[string]struct {
			Weight        float64 `json:"weight"`
			RollingSharpe float64 `json:"rolling_sharpe"`
			HitRate       float64 `json:"hit_rate"`
			TotalSignals  int     `json:"total_signals"`
			WinCount      int     `json:"win_count"`
			LossCount     int     `json:"loss_count"`
			AvgReturn     float64 `json:"avg_return"`
			LastUpdatedAt string  `json:"last_updated_at"`
		} `json:"weights"`
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("parse darwinian weights: %w", err)
	}
	agents := make(map[string]DarwinianAgentInfo, len(saved.Weights))
	for id, w := range saved.Weights {
		agents[id] = DarwinianAgentInfo{
			Weight:        w.Weight,
			RollingSharpe: w.RollingSharpe,
			HitRate:       w.HitRate,
			TotalSignals:  w.TotalSignals,
			WinCount:      w.WinCount,
			LossCount:     w.LossCount,
			AvgReturn:     w.AvgReturn,
			LastUpdated:   w.LastUpdatedAt,
		}
	}
	return &DarwinianStatusData{
		Status:       "ok",
		LastComputed: saved.SavedAt,
		AgentCount:   len(agents),
		Agents:       agents,
	}, nil
}

type RegimeHistoryData struct {
	Sessions    []RegimeSessionEntry `json:"sessions"`
	Transitions []RegimeTransition   `json:"transitions"`
	Current     string               `json:"current_regime"`
}

type RegimeSessionEntry struct {
	SessionID  string `json:"session_id"`
	Regime     string `json:"regime"`
	RecordedAt string `json:"recorded_at"`
}

type RegimeTransition struct {
	From      string `json:"from_regime"`
	To        string `json:"to_regime"`
	Timestamp string `json:"timestamp"`
}

func (s *PipelineService) LoadRegimeHistory(limit int) (*RegimeHistoryData, error) {
	store := ledger.NewStore(s.LedgerDir)
	summaries, err := store.LoadSessionSummaries()
	if err != nil {
		return nil, fmt.Errorf("load session summaries: %w", err)
	}
	if len(summaries) > limit {
		summaries = summaries[len(summaries)-limit:]
	}
	sessions := make([]RegimeSessionEntry, len(summaries))
	var transitions []RegimeTransition
	var prevRegime string
	for i, sum := range summaries {
		sessions[i] = RegimeSessionEntry{
			SessionID:  sum.SessionID,
			Regime:     string(sum.Regime),
			RecordedAt: sum.RecordedAt.Format(time.RFC3339),
		}
		if i > 0 && string(sum.Regime) != prevRegime {
			transitions = append(transitions, RegimeTransition{
				From:      prevRegime,
				To:        string(sum.Regime),
				Timestamp: sum.RecordedAt.Format(time.RFC3339),
			})
		}
		prevRegime = string(sum.Regime)
	}
	current := ""
	if len(summaries) > 0 {
		current = string(summaries[len(summaries)-1].Regime)
	}
	return &RegimeHistoryData{
		Sessions:    sessions,
		Transitions: transitions,
		Current:     current,
	}, nil
}
