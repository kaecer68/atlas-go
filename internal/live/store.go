package live

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// StateStore 管理实时交易状态
type StateStore struct {
	basePath string
	mutex    sync.RWMutex

	// 内存状态
	portfolio PortfolioState
	positions map[string]domain.Position
	regime    RegimeState
	events    []Event
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
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
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
	portfolioPath := filepath.Join(s.basePath, "state", "portfolio_state.jsonl")
	if data, err := os.ReadFile(portfolioPath); err == nil && len(data) > 0 {
		lines := splitLines(string(data))
		if len(lines) > 0 {
			lastLine := lines[len(lines)-1]
			if err := json.Unmarshal([]byte(lastLine), &s.portfolio); err != nil {
				return fmt.Errorf("load portfolio state: %w", err)
			}
		}
	}

	// 加载持仓状态
	positionsPath := filepath.Join(s.basePath, "state", "positions_current.jsonl")
	if data, err := os.ReadFile(positionsPath); err == nil && len(data) > 0 {
		lines := splitLines(string(data))
		if len(lines) > 0 {
			lastLine := lines[len(lines)-1]
			var positions []domain.Position
			if err := json.Unmarshal([]byte(lastLine), &positions); err == nil {
				s.positions = make(map[string]domain.Position)
				for _, p := range positions {
					s.positions[p.Symbol] = p
				}
			}
		}
	}

	// 加载市场状态
	regimePath := filepath.Join(s.basePath, "state", "regime_state.jsonl")
	if data, err := os.ReadFile(regimePath); err == nil && len(data) > 0 {
		lines := splitLines(string(data))
		if len(lines) > 0 {
			lastLine := lines[len(lines)-1]
			if err := json.Unmarshal([]byte(lastLine), &s.regime); err != nil {
				return fmt.Errorf("load regime state: %w", err)
			}
		}
	}

	return nil
}

// Save 保存状态到磁盘
func (s *StateStore) Save() error {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// 确保目录存在
	stateDir := filepath.Join(s.basePath, "state")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	// 保存投资组合状态
	portfolioPath := filepath.Join(stateDir, "portfolio_state.jsonl")
	portfolioJSON, _ := json.Marshal(s.portfolio)
	if err := appendToFile(portfolioPath, string(portfolioJSON)); err != nil {
		return fmt.Errorf("save portfolio: %w", err)
	}

	// 保存持仓状态
	positionsPath := filepath.Join(stateDir, "positions_current.jsonl")
	positions := make([]domain.Position, 0, len(s.positions))
	for _, p := range s.positions {
		positions = append(positions, p)
	}
	positionsJSON, _ := json.Marshal(positions)
	if err := appendToFile(positionsPath, string(positionsJSON)); err != nil {
		return fmt.Errorf("save positions: %w", err)
	}

	// 保存市场状态
	regimePath := filepath.Join(stateDir, "regime_state.jsonl")
	regimeJSON, _ := json.Marshal(s.regime)
	if err := appendToFile(regimePath, string(regimeJSON)); err != nil {
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
	for k, v := range s.positions {
		result[k] = v
	}
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
func (s *StateStore) RecordEvent(eventType string, payload interface{}) {
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
	os.MkdirAll(eventsDir, 0755)

	dateStr := time.Now().Format("2006-01-02")
	eventsPath := filepath.Join(eventsDir, fmt.Sprintf("events_%s.jsonl", dateStr))

	eventJSON, _ := json.Marshal(event)
	appendToFile(eventsPath, string(eventJSON))
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

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func appendToFile(path, content string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(content + "\n")
	return err
}
