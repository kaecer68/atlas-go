package store

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/constants"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// Well-known filenames used by StateStore for persistence.
const (
	PortfolioStateFile = "portfolio_state.json"
	PositionsStateFile = "positions_current.json"
	RegimeStateFile    = "regime_state.json"
	LiveStateSubDir    = "state"
)

// DefaultCircuitBreakerStatePath is the default relative path for circuit breaker state.
const DefaultCircuitBreakerStatePath = "data/state/circuit_breaker_state.json"

// DefaultLiveStateBasePath is the default base directory for live trading state.
const DefaultLiveStateBasePath = constants.StateLive

// StateStore 管理实时交易状态
type StateStore struct {
	basePath string
	mutex    sync.RWMutex

	// 内存状态
	portfolio       PortfolioState
	positions       map[string]domain.Position
	regime          RegimeState
	events          []Event
	recommendations []domain.Recommendation
	filteredRecs    []domain.Recommendation
}

// PortfolioState 投资组合状态
type PortfolioState struct {
	Cash          float64   `json:"cash"`
	TotalExposure float64   `json:"total_exposure"`
	AvailableCash float64   `json:"available_cash"`
	DayPnL        float64   `json:"day_pnl"`
	UnrealizedPnL float64   `json:"unrealized_pnl"`
	RealizedPnL   float64   `json:"realized_pnl"`
	LastUpdated   time.Time `json:"last_updated"`
}

// RegimeState 市场状态
type RegimeState struct {
	CurrentRegime domain.Regime `json:"current_regime"`
	Confidence    float64       `json:"confidence"`
	LastChangedAt time.Time     `json:"last_changed_at"`
	DeterminedBy  string        `json:"determined_by"`
}

// Event 事件记录
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

// NewStateStore 创建新的状态存储
func NewStateStore(basePath string) *StateStore {
	return &StateStore{
		basePath:  basePath,
		positions: make(map[string]domain.Position),
		events:    make([]Event, 0),
		portfolio: PortfolioState{
			Cash:          3000000, // 默认初始资金
			AvailableCash: 3000000,
			LastUpdated:   time.Now(),
		},
		regime: RegimeState{
			CurrentRegime: domain.RegimeNeutral,
			Confidence:    0.5,
			LastChangedAt: time.Now(),
			DeterminedBy:  "system",
		},
	}
}

// Load 从磁盘加载状态
func (s *StateStore) Load() error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// 加载投资组合状态
	if p, err := LoadLastPortfolioState(s.basePath); err == nil {
		s.portfolio = p
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("load portfolio state: %w", err)
	}

	// 加载持仓状态（格式错误不阻断启动）
	if positions, err := LoadLastPositions(s.basePath); err == nil {
		s.positions = positions
	}

	// 加载市场状态
	if r, err := LoadLastRegime(s.basePath); err == nil {
		s.regime = r
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("load regime state: %w", err)
	}

	return nil
}

// Save 保存状态到磁盘
func (s *StateStore) Save() error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// 确保目录存在
	stateDir := filepath.Join(s.basePath, "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	// 保存投资组合状态
	portfolioPath := filepath.Join(stateDir, PortfolioStateFile)
	portfolioJSON, err := json.Marshal(s.portfolio)
	if err != nil {
		return fmt.Errorf("marshal portfolio: %w", err)
	}
	if err := WriteFileAtomic(portfolioPath, string(portfolioJSON)); err != nil {
		return fmt.Errorf("save portfolio: %w", err)
	}

	// 保存持仓状态
	positionsPath := filepath.Join(stateDir, PositionsStateFile)
	positions := make([]domain.Position, 0, len(s.positions))
	for _, p := range s.positions {
		positions = append(positions, p)
	}
	positionsJSON, err := json.Marshal(positions)
	if err != nil {
		return fmt.Errorf("marshal positions: %w", err)
	}
	if err := WriteFileAtomic(positionsPath, string(positionsJSON)); err != nil {
		return fmt.Errorf("save positions: %w", err)
	}

	// 保存市场状态
	regimePath := filepath.Join(stateDir, RegimeStateFile)
	regimeJSON, err := json.Marshal(s.regime)
	if err != nil {
		return fmt.Errorf("marshal regime: %w", err)
	}
	if err := WriteFileAtomic(regimePath, string(regimeJSON)); err != nil {
		return fmt.Errorf("save regime: %w", err)
	}

	return nil
}

// GetPortfolio 获取投资组合状态
func (s *StateStore) GetPortfolio() PortfolioState {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.portfolio
}

// UpdatePortfolio 更新投资组合状态
func (s *StateStore) UpdatePortfolio(update PortfolioState) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.portfolio = update
	s.portfolio.LastUpdated = time.Now()
}

// GetPositions 获取所有持仓
func (s *StateStore) GetPositions() map[string]domain.Position {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	result := make(map[string]domain.Position, len(s.positions))
	maps.Copy(result, s.positions)
	return result
}

// GetPosition 获取指定持仓
func (s *StateStore) GetPosition(symbol string) (domain.Position, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	p, ok := s.positions[symbol]
	return p, ok
}

// UpdatePosition 更新或添加持仓
func (s *StateStore) UpdatePosition(position domain.Position) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.positions[position.Symbol] = position
}

// RemovePosition 移除持仓
func (s *StateStore) RemovePosition(symbol string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.positions, symbol)
}

// GetRegime 获取市场状态
func (s *StateStore) GetRegime() RegimeState {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.regime
}

// UpdateRegime 更新市场状态
func (s *StateStore) UpdateRegime(regime domain.Regime, confidence float64, determinedBy string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.regime = RegimeState{
		CurrentRegime: regime,
		Confidence:    confidence,
		LastChangedAt: time.Now(),
		DeterminedBy:  determinedBy,
	}
}

// RecordEvent 记录事件
func (s *StateStore) RecordEvent(eventType string, payload any) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	event := Event{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      eventType,
		Timestamp: time.Now(),
		Payload:   payload,
	}
	s.events = append(s.events, event)

	// 异步保存到磁盘
	go s.persistEvent(event)
}

// persistEvent 持久化事件
func (s *StateStore) persistEvent(event Event) {
	eventsDir := filepath.Join(s.basePath, "events")
	if err := os.MkdirAll(eventsDir, 0o755); err != nil {
		return
	}

	dateStr := time.Now().Format("2006-01-02")
	eventsPath := filepath.Join(eventsDir, fmt.Sprintf("events_%s.jsonl", dateStr))

	eventJSON, err := json.Marshal(event)
	if err != nil {
		logging.Warn("state_store", "marshal_event_failed", logging.Err(err))
		return
	}
	if err := appendToFile(eventsPath, string(eventJSON)); err != nil {
		logging.Warn("state_store", "append_event_failed", logging.Err(err))
	}
}

// UpdatePositionPrices 更新所有持仓价格
func (s *StateStore) UpdatePositionPrices(quotes []domain.Quote) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	quoteMap := make(map[string]domain.Quote)
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}

	unrealizedPnL := 0.0
	totalExposure := 0.0

	for symbol, position := range s.positions {
		if quote, ok := quoteMap[symbol]; ok {
			position.CurrentPrice = quote.Last
			position.MarketValue = float64(position.Quantity) * quote.Last
			position.UnrealizedPnL = float64(position.Quantity) * (quote.Last - position.AverageCost)
			s.positions[symbol] = position

			unrealizedPnL += position.UnrealizedPnL
			totalExposure += position.MarketValue
		}
	}

	s.portfolio.UnrealizedPnL = unrealizedPnL
	s.portfolio.TotalExposure = totalExposure
	s.portfolio.LastUpdated = time.Now()
}

// CalculateDayPnL 计算当日盈亏
func (s *StateStore) CalculateDayPnL(dayOpenPrices map[string]float64) float64 {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	dayPnL := 0.0
	for symbol, position := range s.positions {
		if dayOpen, ok := dayOpenPrices[symbol]; ok {
			dayPnL += float64(position.Quantity) * (position.CurrentPrice - dayOpen)
		}
	}

	s.portfolio.DayPnL = dayPnL
	return dayPnL
}

// ResetDayState 重置每日状态（市场开盘时调用）
func (s *StateStore) ResetDayState() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.portfolio.DayPnL = 0
	s.portfolio.RealizedPnL = 0
	s.events = make([]Event, 0)
}

// GetState 获取完整状态快照
func (s *StateStore) GetState() *State {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	positions := make([]domain.Position, 0, len(s.positions))
	for _, p := range s.positions {
		positions = append(positions, p)
	}

	return &State{
		Portfolio: s.portfolio,
		Positions: positions,
		Regime:    s.regime,
		Events:    s.events,
	}
}

// State 完整状态快照
type State struct {
	Portfolio PortfolioState
	Positions []domain.Position
	Regime    RegimeState
	Events    []Event
}

// Helper functions

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, err = f.WriteString(content + "\n")
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

// writeFileAtomic writes content to a temp file and renames it to the target path.
// This ensures readers never see a partially written file.
func WriteFileAtomic(path, content string) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*.jsonl")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.WriteString(content + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

// readLastJSONLLine reads the last valid JSON line from a JSONL file.
// It scans backwards from the end of the file to tolerate partially-written final lines.
func readLastJSONLLine(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range slices.Backward(lines) {
		line := strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if err := json.Unmarshal([]byte(line), v); err == nil {
			return nil
		}
	}
	return fmt.Errorf("no valid JSON line found in %s", path)
}

// LoadLastPortfolioState reads the latest persisted portfolio state from the StateStore directory.
func LoadLastPortfolioState(basePath string) (PortfolioState, error) {
	var p PortfolioState
	path := filepath.Join(basePath, LiveStateSubDir, PortfolioStateFile)
	if err := readLastJSONLLine(path, &p); err != nil {
		return p, fmt.Errorf("read portfolio state: %w", err)
	}
	return p, nil
}

// LoadLastPositions reads the latest persisted positions from the StateStore directory.
func LoadLastPositions(basePath string) (map[string]domain.Position, error) {
	path := filepath.Join(basePath, LiveStateSubDir, PositionsStateFile)
	var list []domain.Position
	if err := readLastJSONLLine(path, &list); err != nil {
		return nil, fmt.Errorf("read positions: %w", err)
	}
	m := make(map[string]domain.Position, len(list))
	for _, pos := range list {
		m[pos.Symbol] = pos
	}
	return m, nil
}

// LoadLastRegime reads the latest persisted regime state from the StateStore directory.
func LoadLastRegime(basePath string) (RegimeState, error) {
	var r RegimeState
	path := filepath.Join(basePath, LiveStateSubDir, RegimeStateFile)
	if err := readLastJSONLLine(path, &r); err != nil {
		return r, fmt.Errorf("read regime state: %w", err)
	}
	return r, nil
}

// GetCurrentRegime returns the current market regime.
func (s *StateStore) GetCurrentRegime() domain.Regime {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.regime.CurrentRegime
}

// SetCurrentRegime updates the current market regime.
func (s *StateStore) SetCurrentRegime(regime domain.Regime) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.regime.CurrentRegime = regime
	s.regime.LastChangedAt = time.Now()
	s.regime.DeterminedBy = "context_agent"
}

// GetPendingRecommendations returns the pending recommendations.
func (s *StateStore) GetPendingRecommendations() []domain.Recommendation {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return append([]domain.Recommendation(nil), s.recommendations...)
}

// SetPendingRecommendations sets the pending recommendations.
func (s *StateStore) SetPendingRecommendations(recs []domain.Recommendation) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.recommendations = append([]domain.Recommendation(nil), recs...)
}

// GetFilteredRecommendations returns the filtered recommendations.
func (s *StateStore) GetFilteredRecommendations() []domain.Recommendation {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return append([]domain.Recommendation(nil), s.filteredRecs...)
}

// SetFilteredRecommendations sets the filtered recommendations.
func (s *StateStore) SetFilteredRecommendations(recs []domain.Recommendation) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.filteredRecs = append([]domain.Recommendation(nil), recs...)
}
