package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/methodology"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/replay"
)

// PipelineService encapsulates dashboard pipeline data loading operations.
type PipelineService struct {
	WorkDir           string
	LedgerDir         string
	registryProvider  RegistryProviderFunc
	narrativeProvider NarrativeProviderFunc
	cycleProvider     CycleProviderFunc
	cardProvider      CycleCardProviderFunc
	store             ledger.OutcomeStore
	historicalStore   ledger.HistoricalStore
}

type NarrativeContextData struct {
	ActiveThemes   []string
	PrimaryTheme   string
	PrimaryHitRate float64
	DirectionHint  string
}

type IndustryContextData struct {
	IndustryID         string
	BusinessCycle      string
	CycleConfidence    float64
	SeasonalMultiplier float64
	SystemicImportance float64
}

func NewPipelineService(workDir, ledgerDir string, store ledger.OutcomeStore) *PipelineService {
	return &PipelineService{
		WorkDir:   workDir,
		LedgerDir: ledgerDir,
		store:     store,
	}
}

// WithHistoricalStore injects the HistoricalStore (regime_history SQLite
// table) so LoadRegimeHistory can serve true regime time-series instead of
// simulation session metadata. Builder pattern preserves backward compat:
// existing 43 NewPipelineService test callers (nil 3rd arg) keep working.
// See docs/manifests/2026-07-20-cl3-regime-history.md A01.
func (s *PipelineService) WithHistoricalStore(hs ledger.HistoricalStore) *PipelineService {
	s.historicalStore = hs
	return s
}

// PipelineLoadStatus indicates the health of the loaded recommendation pipeline data.
type PipelineLoadStatus string

const (
	PipelineStatusOK        PipelineLoadStatus = "ok"
	PipelineStatusDegraded  PipelineLoadStatus = "degraded"
	PipelineStatusMinimal   PipelineLoadStatus = "minimal"
	PipelineStatusNoSession PipelineLoadStatus = "no_session"
	PipelineStatusError     PipelineLoadStatus = "error"
)

func (s *PipelineService) loadRegistry() (domain.AgentRegistry, error) {
	if s.registryProvider != nil {
		return s.registryProvider()
	}
	registryPath := filepath.Join(s.WorkDir, constants.AgentsConfigPath)
	return orchestrator.LoadRegistry(registryPath)
}

// LoadMacroRadar loads macro radar summary for the given session.

// MacroRadarData is the internal representation for macro radar response.

// LoadAgentObservatory loads agent observatory data with scorecards.

// AgentObservatoryData is the internal representation for agent observatory response.

// LoadForecastVsReality loads forecast vs reality experiment data.
func (s *PipelineService) LoadForecastVsReality(agentID string, limit int) (*ForecastVsRealityData, error) {
	items, err := s.loadForecastVsRealityItems(agentID, limit)
	if err != nil {
		return nil, fmt.Errorf("load forecast-vs-reality data: %w", err)
	}

	summary, err := FindLatestSessionSummary(s.store, s.LedgerDir)
	if err != nil {
		logging.Warn("pipeline_service", "load_session_summary", logging.Err(err))
		summary = nil
	}

	predictions := s.loadSymbolPredictions(limit, summary)

	data := &ForecastVsRealityData{Items: items, SymbolPredictions: predictions}
	if summary != nil {
		data.BrokerRuntime = summary.BrokerRuntime
	}
	return data, nil
}

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

func readOutcomeFile(path string) ([]rawOutcome, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var results []rawOutcome
	var dropped int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var r rawOutcome
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			dropped++
			continue
		}
		results = append(results, r)
	}
	if dropped > 0 {
		logging.Warn("pipeline_service", "dropped_corrupted_outcomes", logging.FInt("count", dropped))
	}
	return results, scanner.Err()
}

func (s *PipelineService) loadSymbolPredictions(limit int, summary *domain.SessionSummary) []SymbolPredictionItem {
	var rawOutcomes []rawOutcome
	if summary != nil {
		path := filepath.Join(s.LedgerDir, "sessions", summary.SessionID, "recommendation_outcomes.jsonl")
		if r, err := readOutcomeFile(path); err != nil {
			logging.Warn("pipeline_service", "read_outcome_file_failed", logging.Err(err))
		} else {
			rawOutcomes = r
		}
	}
	if rawOutcomes == nil {
		path := filepath.Join(s.LedgerDir, "recommendation_outcomes.jsonl")
		if r, err := readOutcomeFile(path); err != nil {
			logging.Warn("pipeline_service", "read_outcome_file_failed", logging.Err(err))
		} else {
			rawOutcomes = r
		}
	}
	if rawOutcomes == nil {
		return nil
	}

	slices.SortFunc(rawOutcomes, func(a, b rawOutcome) int {
		if a.SessionID > b.SessionID {
			return -1
		}
		if a.SessionID < b.SessionID {
			return 1
		}
		return 0
	})

	if len(rawOutcomes) > limit {
		rawOutcomes = rawOutcomes[:limit]
	}

	result := make([]SymbolPredictionItem, len(rawOutcomes))
	for i, o := range rawOutcomes {
		result[i] = SymbolPredictionItem(o)
	}
	return result
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
		name := entry.Name()
		// Skip metadata/index files that are not experiment results.
		// Experiment result files follow the exec-<agent>-<timestamp>.json convention.
		if name == "_metadata.json" || !strings.HasPrefix(name, "exec-") {
			continue
		}
		path := filepath.Join(experimentsDir, name)
		bytes, err := os.ReadFile(path)
		if err != nil {
			logging.Warn("pipeline_service", "read_experiment_file_failed", logging.FStr("file", name), logging.Err(err))
			continue
		}
		var result domain.PromptExperimentResult
		if err := json.Unmarshal(bytes, &result); err != nil {
			logging.Warn("pipeline_service", "experiment_file_unmarshal_failed", logging.FStr("file", name), logging.Err(err))
			continue
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

type sessionData struct {
	Items         []PipelineItemData
	ScreenedItems []domain.ScreeningReject
	Regime        domain.Regime
	RecordedAt    time.Time
	GuardOutcomes []domain.GuardOutcome
	Status        PipelineLoadStatus
	StatusMessage string
}

// extractPipelineMetrics extracts P/E, P/B, DividendYield from FactorScores.Breakdown.RawInputs
// and BacktestReturn from ForwardReturn.
func extractPipelineMetrics(outcome domain.RecommendationOutcome) PipelineItemMetrics {
	var metrics PipelineItemMetrics
	if outcome.FactorScores.Breakdown != nil {
		// P/E from Value factor RawInputs
		if pe, ok := outcome.FactorScores.Breakdown.Value.RawInputs["pe"]; ok {
			metrics.PriceToEarnings = &pe
		}
		// P/B from Value factor RawInputs
		if pb, ok := outcome.FactorScores.Breakdown.Value.RawInputs["pb"]; ok {
			metrics.PriceToBook = &pb
		}
		// DividendYield from Quality factor RawInputs
		if dy, ok := outcome.FactorScores.Breakdown.Quality.RawInputs["dividend_yield"]; ok {
			metrics.DividendYield = &dy
		}
	}
	bt := outcome.ForwardReturn
	metrics.BacktestReturn = &bt
	return metrics
}

func (s *PipelineService) loadSessionPipelineData(sessionID, sessionsDir string, showAll bool, ds *replay.Dataset) *sessionData {
	var summary *domain.SessionSummary
	var guards []domain.GuardOutcome
	var regime domain.Regime
	var recordedAt time.Time

	summaryPath := filepath.Join(sessionsDir, sessionID, "summary.json")
	if summaryBytes, err := os.ReadFile(summaryPath); err == nil {
		var s domain.SessionSummary
		if err := json.Unmarshal(summaryBytes, &s); err == nil {
			summary = &s
			regime = s.Regime
			recordedAt = s.RecordedAt
			guards = make([]domain.GuardOutcome, 0, len(s.GuardOutcomes))
			for _, g := range s.GuardOutcomes {
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
		}
	}

	status := PipelineStatusOK
	statusMessage := ""
	if summary == nil {
		status = PipelineStatusDegraded
		statusMessage = "控制層過濾記錄未載入（summary.json 缺失），推薦清單仍可用"
	}

	outcomesPath := filepath.Join(sessionsDir, sessionID, "recommendation_outcomes.jsonl")
	items := make([]PipelineItemData, 0)
	if data, err := os.ReadFile(outcomesPath); err == nil {
		lines := strings.SplitSeq(strings.TrimSpace(string(data)), "\n")
		for line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var outcome domain.RecommendationOutcome
			if err := json.Unmarshal([]byte(line), &outcome); err != nil {
				logging.Warn("pipeline_service", "corrupted_outcome_skipped", logging.Err(err))
				continue
			}
			if outcome.Symbol == "" {
				var legacy map[string]any
				if err := json.Unmarshal([]byte(line), &legacy); err == nil {
					if v, ok := legacy["Symbol"]; ok {
						outcome.Symbol = fmt.Sprint(v)
					}
					if v, ok := legacy["AgentID"]; ok {
						outcome.AgentID = fmt.Sprint(v)
					}
					if v, ok := legacy["Skill"]; ok {
						outcome.Skill = fmt.Sprint(v)
					}
					if v, ok := legacy["Side"]; ok {
						outcome.Side = domain.Side(fmt.Sprint(v))
					}
					if v, ok := legacy["Conviction"]; ok {
						if n, ok := v.(float64); ok {
							outcome.Conviction = int(n)
						}
					}
				}
			}
			fr := outcome.ForwardReturn
			price := outcome.Price
			side := string(outcome.Side)
			passedGuards := outcome.PassedGuards
			if !passedGuards && !strings.Contains(line, `"PassedGuards"`) {
				passedGuards = true
			}
			if ds != nil && !outcome.RecordedAt.IsZero() {
				// 僅在 synthetic 結果上回填：真實 0% 漲跌不可誤判為「未設定」。
				if fr == 0 && outcome.IsSynthetic {
					if recalculated, ok := ds.ForwardReturn(outcome.Symbol, outcome.RecordedAt, 1); ok {
						fr = recalculated
						outcome.ForwardReturn = fr
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
				tp, slp = fallbackPriceTargets(outcome.Skill, price, outcome.Side)
			}
			if !showAll && !passedGuards {
				continue
			}
			tags := computePipelineTags(ds, outcome.Symbol, outcome.RecordedAt)
			var narCtx *NarrativeContextData
			var indCtx *IndustryContextData
			if s.narrativeProvider != nil {
				narCtx = s.narrativeProvider(outcome.SupportingEvents)
			}
			if s.cycleProvider != nil {
				indCtx = s.cycleProvider(outcome.Skill)
			}
			items = append(items, PipelineItemData{
				Symbol:              outcome.Symbol,
				AgentID:             outcome.AgentID,
				Skill:               outcome.Skill,
				Layer:               string(outcome.Layer),
				Side:                side,
				Conviction:          outcome.Conviction,
				TargetPrice:         tp,
				StopLossPrice:       slp,
				ForwardReturn:       fr,
				Hit:                 (outcome.Side == domain.SideSell && fr < 0) || (outcome.Side == domain.SideBuy && fr > 0),
				Reason:              outcome.Reason,
				Price:               price,
				PassedGuards:        passedGuards,
				GuardReason:         outcome.GuardReason,
				Tags:                tags,
				RecordedAt:          outcome.RecordedAt,
				FactorScores:        outcome.FactorScores,
				ConvictionBreakdown: outcome.ConvictionBreakdown,
				NarrativeEventIDs:   outcome.SupportingEvents,
				NarrativeContext:    narCtx,
				IndustryContext:     indCtx,
				Metrics:             extractPipelineMetrics(outcome),
			})
		}
	} else {
		status = PipelineStatusMinimal
		statusMessage = "本場次尚無推薦產出記錄"
	}

	store := s.store
	screened, _ := store.LoadSessionScreeningRejects(sessionID)

	return &sessionData{
		Items:         items,
		ScreenedItems: screened,
		Regime:        regime,
		RecordedAt:    recordedAt,
		GuardOutcomes: guards,
		Status:        status,
		StatusMessage: statusMessage,
	}
}

// LoadRecommendationPipeline loads the recommendation pipeline for a session.
func (s *PipelineService) LoadRecommendationPipeline(sessionID string, showAll bool) (*RecommendationPipelineData, error) {
	sessionsDir := filepath.Join(s.LedgerDir, "sessions")

	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &RecommendationPipelineData{
				Status:        PipelineStatusNoSession,
				StatusMessage: "尚未執行任何回測場次，請先執行回測",
			}, nil
		}
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	var availableSessions []string
	var sessionDirs []string
	for _, entry := range entries {
		if entry.IsDir() {
			availableSessions = append(availableSessions, entry.Name())
			sessionDirs = append(sessionDirs, entry.Name())
		}
	}

	if len(sessionDirs) == 0 {
		return &RecommendationPipelineData{
			Status:            PipelineStatusNoSession,
			StatusMessage:     "尚未執行任何回測場次，請先執行回測",
			AvailableSessions: availableSessions,
		}, nil
	}

	var targetSession string
	if sessionID != "" {
		if slices.Contains(sessionDirs, sessionID) {
			targetSession = sessionID
		}
	} else {
		slices.SortFunc(sessionDirs, func(a, b string) int {
			aDate := domain.SessionDateFromID(a)
			bDate := domain.SessionDateFromID(b)
			switch {
			case aDate.After(bDate):
				return -1
			case aDate.Before(bDate):
				return 1
			default:
				return 0
			}
		})
		targetSession = sessionDirs[0]
	}

	if targetSession == "" {
		return &RecommendationPipelineData{
			Status:            PipelineStatusNoSession,
			StatusMessage:     "尚未執行任何回測場次，請先執行回測",
			AvailableSessions: availableSessions,
		}, nil
	}

	var ds *replay.Dataset
	cfg := config.Load()
	replayPath := cfg.ReplayDataPath
	if !filepath.IsAbs(replayPath) {
		replayPath = filepath.Join(s.WorkDir, replayPath)
	}
	if dsTmp, err := replay.LoadTWSEOpenDataCSV(replayPath); err == nil {
		ds = dsTmp
	} else {
		logging.Warn("pipeline_service", "load_replay_csv_failed", logging.Err(err))
	}

	sd := s.loadSessionPipelineData(targetSession, sessionsDir, showAll, ds)

	var fallbackMsg string
	if len(sd.Items) == 0 && sessionID == "" {
		for _, dir := range sessionDirs {
			if dir == targetSession {
				continue
			}
			fallbackPath := filepath.Join(sessionsDir, dir, "recommendation_outcomes.jsonl")
			if info, err := os.Stat(fallbackPath); err == nil && info.Size() > 0 {
				fallbackData := s.loadSessionPipelineData(dir, sessionsDir, showAll, ds)
				if len(fallbackData.Items) > 0 {
					targetSession = dir
					sd = fallbackData
					fallbackMsg = fmt.Sprintf("最新場次 %s 尚無數據，已自動切換至 %s", sessionDirs[0], dir)
					logging.Info("pipeline_service", "auto_fallback_session", "from", sessionDirs[0], "to", dir, "items", len(sd.Items))
					break
				}
			}
		}
	}

	if len(sd.Items) == 0 && fallbackMsg == "" {
		sd.Status = PipelineStatusMinimal
		if sd.StatusMessage == "" {
			sd.StatusMessage = "本場次尚無推薦產出記錄"
		}
	}

	return &RecommendationPipelineData{
		SessionID:         targetSession,
		Regime:            sd.Regime,
		Items:             sd.Items,
		GuardOutcomes:     sd.GuardOutcomes,
		ScreenedItems:     sd.ScreenedItems,
		RecordedAt:        sd.RecordedAt,
		Status:            sd.Status,
		StatusMessage:     sd.StatusMessage,
		AvailableSessions: availableSessions,
		IsFallbackSession: fallbackMsg != "",
		FallbackMessage:   fallbackMsg,
		CycleStatus:       s.buildCycleStatus(),
	}, nil
}

func (s *PipelineService) buildCycleStatus() *CycleStatusResponse {
	if s.cardProvider == nil {
		return nil
	}
	card := s.cardProvider()
	if card == nil {
		return nil
	}
	return &CycleStatusResponse{
		SentimentLabel:       card.SentimentLabel,
		CompositeCoefficient: card.CompositeCoefficient,
		CycleConfidence:      card.CycleConfidence,
		BusinessCycle:        card.BusinessCycle,
		SiliconPhaseName:     card.SiliconPhaseName,
		IsFavorable:          card.IsFavorable,
		ActivePatterns:       len(card.ActivePatterns),
		ActiveEvents:         len(card.ActiveEvents),
	}
}

// RecommendationPipelineData is the internal representation for recommendation pipeline response.
type RecommendationPipelineData struct {
	SessionID         string
	Regime            domain.Regime
	Items             []PipelineItemData
	GuardOutcomes     []domain.GuardOutcome
	ScreenedItems     []domain.ScreeningReject
	RecordedAt        time.Time
	Status            PipelineLoadStatus
	StatusMessage     string
	AvailableSessions []string
	IsFallbackSession bool
	FallbackMessage   string
	CycleStatus       *CycleStatusResponse
}

// CycleStatusResponse carries the composite cycle sentiment card data.
type CycleStatusResponse struct {
	SentimentLabel       string  `json:"sentiment_label"`
	CompositeCoefficient float64 `json:"composite_coefficient"`
	CycleConfidence      float64 `json:"cycle_confidence"`
	BusinessCycle        string  `json:"business_cycle"`
	SiliconPhaseName     string  `json:"silicon_phase_name"`
	IsFavorable          bool    `json:"is_favorable"`
	ActivePatterns       int     `json:"active_patterns"`
	ActiveEvents         int     `json:"active_events"`
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
	NarrativeEventIDs   []string
	NarrativeContext    *NarrativeContextData
	IndustryContext     *IndustryContextData
	Metrics             PipelineItemMetrics
}

type PipelineItemMetrics struct {
	PriceToEarnings *float64
	PriceToBook     *float64
	DividendYield   *float64
	BacktestReturn  *float64
}

// countNonEmptyLines 回傳檔案內非空行數(給孤兒 session OutcomeCount fallback 用)。
func countNonEmptyLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return 0
	}
	count := 0
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
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
		sessionID := entry.Name()
		meta := SessionMeta{
			SessionID:    sessionID,
			Regime:       "unknown",
			OutcomeCount: 0,
		}

		summaryPath := filepath.Join(sessionsDir, sessionID, "summary.json")
		if bytes, err := os.ReadFile(summaryPath); err == nil {
			var summary domain.SessionSummary
			if err := json.Unmarshal(bytes, &summary); err == nil {
				// SessionID is authoritative from directory name only.
				// summary.json may contain stale or mismatched format data.
				if !summary.RecordedAt.IsZero() {
					meta.RecordedAt = summary.RecordedAt
				}
				if summary.Regime != "" {
					meta.Regime = string(summary.Regime)
				}
				meta.OutcomeCount = summary.OutcomeCount
			} else {
				logging.Warn("pipeline_service", "parse_session_summary_failed", logging.Err(err))
			}
		} else {
			logging.Warn("pipeline_service", "read_session_summary_failed", logging.Err(err))
		}

		// Fall back to session ID date if RecordedAt was not set from summary.
		if meta.RecordedAt.IsZero() {
			meta.RecordedAt = domain.SessionDateFromID(sessionID)
		}

		// Fall back to outcomes.jsonl line count if summary.json missing or had 0 outcomes.
		// 理由:孤兒 session (寫了 outcomes.jsonl 但 summary.json 未寫入) 應在 sessions
		// 下拉選單顯示實際筆數,而不是 0。僅在 summary 為 0 時覆寫,保留 summary 為「過濾後」語意。
		if meta.OutcomeCount == 0 {
			if count := countNonEmptyLines(filepath.Join(sessionsDir, sessionID, "recommendation_outcomes.jsonl")); count > 0 {
				meta.OutcomeCount = count
			}
		}

		sessions = append(sessions, meta)
	}

	// Sort by session trading date descending, then RecordedAt tiebreaker.
	slices.SortFunc(sessions, func(a, b SessionMeta) int {
		aDate := domain.SessionDateFromID(a.SessionID)
		bDate := domain.SessionDateFromID(b.SessionID)
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

// LoadSessionsWithTopStrategies loads all session metadata AND enriches each
// session with its top N strategies ranked by conviction DESC.
//
// The enrichment performs one extra LoadSessionOutcomes call per session
// (N+1 pattern, acceptable since the session count is bounded ~100). Sessions
// that fail to enrich (no outcomes in SQLite, store error) get an empty
// TopStrategies slice rather than failing the entire list — the metadata
// layer is the primary deliverable.
//
// topN must be positive; values <=0 are clamped to 3.
func (s *PipelineService) LoadSessionsWithTopStrategies(topN int) ([]SessionMeta, error) {
	if topN <= 0 {
		topN = 3
	}

	sessions, err := s.LoadSessions()
	if err != nil {
		return nil, fmt.Errorf("load sessions: %w", err)
	}
	if s.store == nil {
		return sessions, nil
	}

	for i := range sessions {
		outcomes, oerr := s.store.LoadSessionOutcomes(sessions[i].SessionID)
		if oerr != nil {
			// Skip enrichment for this session; metadata layer still useful.
			logging.Warn("pipeline_service", "load_session_outcomes_failed",
				logging.FStr("session_id", sessions[i].SessionID),
				logging.Err(oerr))
			sessions[i].TopStrategies = []domain.RecommendationOutcome{}
			continue
		}
		if len(outcomes) == 0 {
			sessions[i].TopStrategies = []domain.RecommendationOutcome{}
			continue
		}
		// Sort by Conviction DESC, then Symbol ASC as stable tiebreaker.
		slices.SortFunc(outcomes, func(a, b domain.RecommendationOutcome) int {
			if a.Conviction != b.Conviction {
				return b.Conviction - a.Conviction
			}
			if a.Symbol != b.Symbol {
				if a.Symbol < b.Symbol {
					return -1
				}
				return 1
			}
			return 0
		})
		if len(outcomes) > topN {
			outcomes = outcomes[:topN]
		}
		sessions[i].TopStrategies = outcomes
	}
	return sessions, nil
}

// SessionDetail combines a session's summary metadata with its per-strategy
// recommendation outcomes. This is the response shape for the
// /api/dashboard/sessions/{id} drill-down endpoint (CL-4 §18.7.3).
type SessionDetail struct {
	SessionID    string                         `json:"session_id"`
	RecordedAt   time.Time                      `json:"recorded_at"`
	Regime       string                         `json:"regime"`
	OutcomeCount int                            `json:"outcome_count"`
	Summary      domain.SessionSummary          `json:"summary"`
	Outcomes     []domain.RecommendationOutcome `json:"outcomes"`
}

// ErrSessionNotFound is returned by LoadSessionDetail when the requested
// sessionID has no summary.json on disk. Distinguishes "no data" from
// "system error" so the handler can map to HTTP 404.
var ErrSessionNotFound = errors.New("session not found")

// LoadSessionDetail loads the full detail for a single session: metadata +
// summary (from filesystem summary.json) + per-strategy outcomes (from the
// ledger store). Returns ErrSessionNotFound when the sessionID has no
// summary on disk; any other error is treated as a system error.
func (s *PipelineService) LoadSessionDetail(sessionID string) (*SessionDetail, error) {
	// First, verify the session exists by scanning LoadSessions.
	sessions, err := s.LoadSessions()
	if err != nil {
		return nil, fmt.Errorf("load sessions: %w", err)
	}
	var found *SessionMeta
	for i := range sessions {
		if sessions[i].SessionID == sessionID {
			found = &sessions[i]
			break
		}
	}
	if found == nil {
		return nil, ErrSessionNotFound
	}

	detail := &SessionDetail{
		SessionID:    found.SessionID,
		RecordedAt:   found.RecordedAt,
		Regime:       found.Regime,
		OutcomeCount: found.OutcomeCount,
	}

	// Read the full summary.json (may be stale or missing — best-effort).
	summaryPath := filepath.Join(s.LedgerDir, "sessions", sessionID, "summary.json")
	if data, rerr := os.ReadFile(summaryPath); rerr == nil {
		if uerr := json.Unmarshal(data, &detail.Summary); uerr != nil {
			logging.Warn("pipeline_service", "parse_session_summary_failed",
				logging.FStr("session_id", sessionID),
				logging.Err(uerr))
		}
	} else if !os.IsNotExist(rerr) {
		logging.Warn("pipeline_service", "read_session_summary_failed",
			logging.FStr("session_id", sessionID),
			logging.Err(rerr))
	}

	// Per-strategy outcomes from the ledger store. Empty slice (not nil)
	// when the session has no outcomes yet.
	detail.Outcomes = []domain.RecommendationOutcome{}
	if s.store != nil {
		outcomes, oerr := s.store.LoadSessionOutcomes(sessionID)
		if oerr != nil {
			return nil, fmt.Errorf("load session outcomes: %w", oerr)
		}
		if len(outcomes) > 0 {
			detail.Outcomes = outcomes
		}
	}

	return detail, nil
}

// SessionMeta represents session metadata.
//
// TopStrategies is nil when populated by LoadSessions (preserves backward
// compatibility for existing callers). It is populated by
// LoadSessionsWithTopStrategies via per-session SQL queries against the
// outcomes table, then sorted by Conviction DESC and truncated to topN.
type SessionMeta struct {
	SessionID     string
	RecordedAt    time.Time
	Regime        string
	OutcomeCount  int
	TopStrategies []domain.RecommendationOutcome
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
//
// Status is a data-visibility label for the agent and MUST be one of:
//   - "active":  the agent has produced at least one signal (total_signals > 0)
//   - "dormant": the agent is registered in configs/agents.json but has zero signals
//   - "ghost":   the agent is NOT registered and has zero signals (legacy or stale residual)
//
// This explicit marking prevents the frontend from having to infer agent health
// from zero values (atlas-data-visibility L4 spec).
type DarwinianAgentInfo struct {
	Weight        float64 `json:"weight"`
	RollingSharpe float64 `json:"rolling_sharpe"`
	HitRate       float64 `json:"hit_rate"`
	TotalSignals  int     `json:"total_signals"`
	WinCount      int     `json:"win_count"`
	LossCount     int     `json:"loss_count"`
	AvgReturn     float64 `json:"avg_return"`
	LastUpdated   string  `json:"last_updated,omitempty"`
	Status        string  `json:"status"`
}

// LoadDarwinianStatus loads the current Darwinian weight state from disk.
func (s *PipelineService) LoadDarwinianHistory(limit int) ([]DarwinianHistoryPoint, error) {
	historyPath := filepath.Join(s.LedgerDir, "darwinian_history.jsonl")
	data, err := os.ReadFile(historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []DarwinianHistoryPoint{}, nil
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
			logging.Warn("pipeline_service", "corrupted_darwinian_history_skipped", logging.Err(err))
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
	registered, err := s.loadRegisteredAgentIDs()
	if err != nil {
		return nil, fmt.Errorf("load registered agent list: %w", err)
	}

	weightsPath := filepath.Join(s.LedgerDir, "darwinian_weights.json")
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
			TotalSignals  *int    `json:"total_signals"`
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
		totalSignals := 0
		status := "ghost"
		if w.TotalSignals != nil {
			totalSignals = *w.TotalSignals
			if totalSignals > 0 {
				status = "active"
			} else if isInRegisteredAgentList(registered, id) {
				status = "dormant"
			} else {
				status = "ghost"
			}
		}
		agents[id] = DarwinianAgentInfo{
			Weight:        w.Weight,
			RollingSharpe: w.RollingSharpe,
			HitRate:       w.HitRate,
			TotalSignals:  totalSignals,
			WinCount:      w.WinCount,
			LossCount:     w.LossCount,
			AvgReturn:     w.AvgReturn,
			LastUpdated:   w.LastUpdatedAt,
			Status:        status,
		}
	}
	return &DarwinianStatusData{
		Status:       "ok",
		LastComputed: saved.SavedAt,
		AgentCount:   len(agents),
		Agents:       agents,
	}, nil
}

func (s *PipelineService) loadRegisteredAgentIDs() (map[string]bool, error) {
	registryPath := filepath.Join(s.WorkDir, "configs", "agents.json")
	data, err := os.ReadFile(registryPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read agents registry: %w", err)
	}
	var registry struct {
		Agents []struct {
			ID string `json:"id"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("parse agents registry: %w", err)
	}
	ids := make(map[string]bool, len(registry.Agents))
	for _, a := range registry.Agents {
		ids[a.ID] = true
	}
	return ids, nil
}

func isInRegisteredAgentList(registered map[string]bool, agentID string) bool {
	return registered[agentID]
}

type RegimeHistoryData struct {
	Sessions      []RegimeSessionEntry `json:"sessions"`
	Transitions   []RegimeTransition   `json:"transitions"`
	Current       string               `json:"current_regime"`
	CurrentPeriod string               `json:"current_period,omitempty"`
}

type RegimeSessionEntry struct {
	SessionID    string `json:"session_id"`
	Date         string `json:"date"`
	Regime       string `json:"regime"`
	Period       string `json:"period,omitempty"`
	MarketPeriod string `json:"market_period,omitempty"` // detected period from period_history
	PeriodNameZH string `json:"period_name_zh,omitempty"`
	RecordedAt   string `json:"recorded_at"`
	Source       string `json:"source,omitempty"`
}

type RegimeTransition struct {
	From      string `json:"from_regime"`
	To        string `json:"to_regime"`
	Timestamp string `json:"timestamp"`
}

func (s *PipelineService) LoadRegimeHistory(limit int) (*RegimeHistoryData, error) {
	if s.historicalStore != nil {
		return s.loadRegimeHistoryFromStore(limit)
	}
	return s.loadRegimeHistoryFromSessions(limit)
}

// LoadRegimeHistoryDays returns regime history entries whose date falls within
// the last N calendar days (UTC), ending today. It uses the HistoricalStore
// when available and falls back to the legacy session-summary path otherwise.
func (s *PipelineService) LoadRegimeHistoryDays(days int) (*RegimeHistoryData, error) {
	if days <= 0 {
		days = 30
	}
	if days > 365 {
		days = 365
	}
	if s.historicalStore != nil {
		return s.loadRegimeHistoryFromStoreDays(days)
	}
	return s.loadRegimeHistoryFromSessions(days)
}

// loadRegimeHistoryFromStore reads the regime_history SQLite table via
// HistoricalStore and projects RegimeRow into the existing RegimeSessionEntry
// shape. This is the canonical time-series path (see spec §18.6.2).
func (s *PipelineService) loadRegimeHistoryFromStore(limit int) (*RegimeHistoryData, error) {
	rows, err := s.historicalStore.LoadRegimeHistory(context.Background(), limit)
	if err != nil {
		return nil, fmt.Errorf("load regime history: %w", err)
	}
	return buildRegimeHistoryData(rows, s.historicalStore), nil
}

// loadRegimeHistoryFromStoreDays is the calendar-window variant of
// loadRegimeHistoryFromStore: it loads a generous row limit and then filters to
// rows whose date is within the last N days. This makes `?days=N` on the
// HTTP endpoint mean a date window, not a row limit.
func (s *PipelineService) loadRegimeHistoryFromStoreDays(days int) (*RegimeHistoryData, error) {
	limit := days * 2
	if limit < 90 {
		limit = 90
	}
	rows, err := s.historicalStore.LoadRegimeHistory(context.Background(), limit)
	if err != nil {
		return nil, fmt.Errorf("load regime history: %w", err)
	}
	minDate := time.Now().UTC().AddDate(0, 0, -days+1).Format("2006-01-02")
	filtered := make([]ledger.RegimeRow, 0, len(rows))
	for _, r := range rows {
		if r.Date >= minDate {
			filtered = append(filtered, r)
		}
	}
	return buildRegimeHistoryData(filtered, s.historicalStore), nil
}

func buildRegimeHistoryData(rows []ledger.RegimeRow, hs ledger.HistoricalStore) *RegimeHistoryData {
	// Build a date→period map from period_history when available.
	periodByDate := map[string]ledger.PeriodRow{}
	if hs != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if periodRows, err := hs.LoadPeriodHistoryAll(ctx, 365); err == nil {
			for _, p := range periodRows {
				periodByDate[p.Date] = p
			}
		} else {
			logging.Warn("pipeline", "load_period_history_failed", logging.Err(err))
		}
	}

	sessions := make([]RegimeSessionEntry, len(rows))
	var transitions []RegimeTransition
	var prevRegime string
	for i, row := range rows {
		var period domain.MarketPeriod
		var periodStr, periodZH string
		if row.Regime != "" {
			period = methodology.RegimeToPeriod(domain.Regime(row.Regime))
			periodStr = string(period)
			periodZH = period.PeriodNameZH()
		}
		entry := RegimeSessionEntry{
			SessionID:    row.Date,
			Date:         row.Date,
			Regime:       row.Regime,
			Period:       periodStr,
			PeriodNameZH: periodZH,
			RecordedAt:   row.RecordedAt.UTC().Format(time.RFC3339),
			Source:       row.Source,
		}
		// Join period_history: set market_period when available.
		if p, ok := periodByDate[row.Date]; ok && p.Period != "" {
			entry.MarketPeriod = p.Period
		}
		sessions[i] = entry
		if i > 0 && row.Regime != prevRegime {
			transitions = append(transitions, RegimeTransition{
				From:      prevRegime,
				To:        row.Regime,
				Timestamp: row.RecordedAt.UTC().Format(time.RFC3339),
			})
		}
		prevRegime = row.Regime
	}
	current := ""
	currentPeriod := ""
	if len(rows) > 0 {
		current = rows[0].Regime
		if current != "" {
			currentPeriod = string(methodology.RegimeToPeriod(domain.Regime(current)))
		}
	}
	return &RegimeHistoryData{
		Sessions:      sessions,
		Transitions:   transitions,
		Current:       current,
		CurrentPeriod: currentPeriod,
	}
}

// loadRegimeHistoryFromSessions is the legacy fallback that reads simulation
// session summaries from the filesystem. Preserved for backward compatibility
// with the 43 test callers that don't inject HistoricalStore; production
// paths go through loadRegimeHistoryFromStore (see spec §18.6.2).
func (s *PipelineService) loadRegimeHistoryFromSessions(limit int) (*RegimeHistoryData, error) {
	store := s.store
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
		var period domain.MarketPeriod
		var periodStr, periodZH string
		if sum.Regime != "" {
			period = methodology.RegimeToPeriod(sum.Regime)
			periodStr = string(period)
			periodZH = period.PeriodNameZH()
		}
		sessions[i] = RegimeSessionEntry{
			SessionID:    sum.SessionID,
			Date:         sum.RecordedAt.UTC().Format("2006-01-02"),
			Regime:       string(sum.Regime),
			Period:       periodStr,
			PeriodNameZH: periodZH,
			RecordedAt:   sum.RecordedAt.UTC().Format(time.RFC3339),
		}
		if i > 0 && string(sum.Regime) != prevRegime {
			transitions = append(transitions, RegimeTransition{
				From:      prevRegime,
				To:        string(sum.Regime),
				Timestamp: sum.RecordedAt.UTC().Format(time.RFC3339),
			})
		}
		prevRegime = string(sum.Regime)
	}
	current := ""
	currentPeriod := ""
	if len(summaries) > 0 {
		last := summaries[len(summaries)-1]
		current = string(last.Regime)
		if last.Regime != "" {
			currentPeriod = string(methodology.RegimeToPeriod(last.Regime))
		}
	}
	return &RegimeHistoryData{
		Sessions:      sessions,
		Transitions:   transitions,
		Current:       current,
		CurrentPeriod: currentPeriod,
	}, nil
}
