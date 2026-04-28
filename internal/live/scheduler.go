package live

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type Scheduler struct {
	marketData     marketdata.Provider
	watchlist      []string
	config         OrchestratorConfig
	eventBus       *ChannelEventBus
	stateStore     *StateStore
	circuitBreaker *CircuitBreaker
	metrics        MetricsRecorder
	system         interface {
		Registry() domain.AgentRegistry
		GetPlugins() interface{}
	}
	effectiveBrokerMode string

	intradayTicker *time.Ticker
	quoteTicker    *time.Ticker
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.RWMutex

	onMarketOpen    func()
	onIntradayCycle func()
	onMarketClose   func()
	onFetchQuotes   func()
}

func NewScheduler(
	ctx context.Context,
	marketData marketdata.Provider,
	stateStore *StateStore,
	circuitBreaker *CircuitBreaker,
	config OrchestratorConfig,
	effectiveBrokerMode string,
) *Scheduler {
	ctx, cancel := context.WithCancel(ctx)
	return &Scheduler{
		marketData:          marketData,
		stateStore:          stateStore,
		circuitBreaker:      circuitBreaker,
		config:              config,
		effectiveBrokerMode: effectiveBrokerMode,
		ctx:                 ctx,
		cancel:              cancel,
	}
}

func (s *Scheduler) SetSystem(system interface {
	Registry() domain.AgentRegistry
	GetPlugins() interface{}
}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.system = system
}

func (s *Scheduler) SetWatchlist(symbols []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchlist = symbols
}

func (s *Scheduler) SetMetrics(m MetricsRecorder) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.metrics = m
}

func (s *Scheduler) SetEventBus(eb *ChannelEventBus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventBus = eb
}

func (s *Scheduler) SetCycleCallbacks(onMarketOpen, onIntradayCycle, onMarketClose, onFetchQuotes func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onMarketOpen = onMarketOpen
	s.onIntradayCycle = onIntradayCycle
	s.onMarketClose = onMarketClose
	s.onFetchQuotes = onFetchQuotes
}

func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.quoteTicker = time.NewTicker(s.config.QuotePollInterval)
	s.wg.Add(1)
	go s.quotePoller()

	s.wg.Add(1)
	go s.marketTimeScheduler()

	return nil
}

func (s *Scheduler) Stop() error {
	s.mu.Lock()
	if s.ctx.Err() == nil {
		s.cancel()
	}
	if s.quoteTicker != nil {
		s.quoteTicker.Stop()
	}
	if s.intradayTicker != nil {
		s.intradayTicker.Stop()
	}
	s.mu.Unlock()

	s.wg.Wait()
	return nil
}

func (s *Scheduler) marketTimeScheduler() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		now := time.Now()
		marketOpen, _ := time.Parse("15:04", s.config.MarketOpenTime)
		marketClose, _ := time.Parse("15:04", s.config.MarketCloseTime)

		todayOpen := time.Date(now.Year(), now.Month(), now.Day(),
			marketOpen.Hour(), marketOpen.Minute(), 0, 0, now.Location())
		todayClose := time.Date(now.Year(), now.Month(), now.Day(),
			marketClose.Hour(), marketClose.Minute(), 0, 0, now.Location())

		if now.After(todayOpen) && now.Before(todayClose) {
			s.mu.RLock()
			hasTicker := s.intradayTicker != nil
			s.mu.RUnlock()
			if !hasTicker {
				s.onMarketOpen()
				s.mu.Lock()
				s.intradayTicker = time.NewTicker(s.config.IntradayInterval)
				s.mu.Unlock()
				s.wg.Add(1)
				go s.intradayProcessor()
			}
		}

		if now.After(todayClose) {
			s.mu.RLock()
			hasTicker := s.intradayTicker != nil
			s.mu.RUnlock()
			if hasTicker {
				s.onMarketClose()
				s.mu.Lock()
				s.intradayTicker.Stop()
				s.intradayTicker = nil
				s.mu.Unlock()
			}
		}

		select {
		case <-s.ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

func (s *Scheduler) intradayProcessor() {
	defer s.wg.Done()

	for {
		tickerCh := func() <-chan time.Time {
			s.mu.RLock()
			ticker := s.intradayTicker
			s.mu.RUnlock()
			if ticker == nil {
				return nil
			}
			return ticker.C
		}()

		select {
		case <-s.ctx.Done():
			return
		case <-tickerCh:
			s.onIntradayCycle()
		}
	}
}

func (s *Scheduler) quotePoller() {
	defer s.wg.Done()

	for {
		tickerCh := func() <-chan time.Time {
			s.mu.RLock()
			ticker := s.quoteTicker
			s.mu.RUnlock()
			if ticker == nil {
				return nil
			}
			return ticker.C
		}()

		select {
		case <-s.ctx.Done():
			return
		case <-tickerCh:
			s.onFetchQuotes()
		}
	}
}

func (s *Scheduler) publishEvent(event BusEvent) {
	s.mu.RLock()
	eb := s.eventBus
	s.mu.RUnlock()
	if eb == nil {
		return
	}
	_ = eb.Publish(event)
}

func (s *Scheduler) Status() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]interface{}{
		"quote_ticker":     s.quoteTicker != nil,
		"intraday_ticker":  s.intradayTicker != nil,
		"watchlist_size":   len(s.watchlist),
		"effective_broker": s.effectiveBrokerMode,
	}
}
