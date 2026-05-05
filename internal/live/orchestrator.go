package live

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

type Orchestrator struct {
	stateStore             *StateStore
	eventBus               *ChannelEventBus
	marketData             marketdata.Provider
	broker                 Broker
	orderMgr               *OrderManager
	registry               domain.AgentRegistry
	executionInputProvider interface {
		Produce(ctx context.Context, symbols []string) (*domain.ExecutionInput, error)
	}
	circuitBreaker *CircuitBreaker

	config OrchestratorConfig

	isRunning bool
	mutex     sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	watchlist []string

	requestedBrokerMode string
	effectiveBrokerMode string
	executionAuditMsg   string

	metrics MetricsRecorder

	scheduler    *Scheduler
	agentRunner  *AgentRunner
	executionMgr *ExecutionManager
}

type MetricsRecorder interface {
	RecordOrder(order domain.Order, status string)
	RecordPosition(position domain.Position)
	RecordPortfolio(cash, totalValue float64)
	RecordCircuitBreakerState(state string)
	RecordRiskEvent(eventType, symbol string)
	RecordCounter(name string, value float64, labels map[string]string)
	RecordGauge(name string, value float64, labels map[string]string)
}

type OrchestratorConfig struct {
	MarketOpenTime             string
	MarketCloseTime            string
	IntradayInterval           time.Duration
	QuotePollInterval          time.Duration
	PreMarketCheck             bool
	MaxDailyLossPct            float64
	MaxPositionLossPct         float64
	StopLossEnabled            bool
	TakeProfitEnabled          bool
	BrokerMode                 string
	BrokerMaxRetries           int
	BrokerAdapter              string
	BrokerAPIBaseURL           string
	BrokerAPIKey               string
	BrokerAPISecret            string
	BrokerHTTPTimeoutS         int
	BrokerHTTPAttempts         int
	BrokerHTTPRetryStatusCodes []int
	BrokerMaxClockSkewS        int
	BrokerNonceTTLS            int
	BrokerNonceStore           string
	BrokerNonceStorePath       string
	BrokerNonceRedisURL        string
	BrokerNonceRedisKeyPrefix  string
	BrokerSigner               string
	BrokerKeyID                string
}

func DefaultOrchestratorConfig() OrchestratorConfig {
	return OrchestratorConfig{
		MarketOpenTime:             "09:00",
		MarketCloseTime:            "13:30",
		IntradayInterval:           5 * time.Minute,
		QuotePollInterval:          30 * time.Second,
		PreMarketCheck:             true,
		MaxDailyLossPct:            2.0,
		MaxPositionLossPct:         5.0,
		StopLossEnabled:            true,
		TakeProfitEnabled:          false,
		BrokerMode:                 "dry-run",
		BrokerMaxRetries:           1,
		BrokerAdapter:              "guarded",
		BrokerHTTPTimeoutS:         5,
		BrokerHTTPAttempts:         2,
		BrokerHTTPRetryStatusCodes: []int{408, 425, 429, 500, 502, 503, 504},
		BrokerMaxClockSkewS:        300,
		BrokerNonceTTLS:            300,
		BrokerNonceStore:           "memory",
		BrokerNonceRedisKeyPrefix:  "atlas:nonce:",
		BrokerSigner:               "placeholder",
	}
}

func NewOrchestrator(
	ctx context.Context,
	stateStore *StateStore,
	eventBus *ChannelEventBus,
	marketData marketdata.Provider,
	registry domain.AgentRegistry,
	inputProvider interface {
		Produce(ctx context.Context, symbols []string) (*domain.ExecutionInput, error)
	},
	config OrchestratorConfig,
) *Orchestrator {
	ctx, cancel := context.WithCancel(ctx)
	requestedMode, effectiveMode, broker, audit := resolveBrokerMode(config)
	maxRetries := config.BrokerMaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	o := &Orchestrator{
		stateStore:             stateStore,
		eventBus:               eventBus,
		marketData:             marketData,
		broker:                 broker,
		orderMgr:               NewOrderManager(broker, eventBus, maxRetries, 100*time.Millisecond),
		registry:               registry,
		executionInputProvider: inputProvider,
		circuitBreaker:         NewCircuitBreaker("", ""),
		config:                 config,
		ctx:                    ctx,
		cancel:                 cancel,
		watchlist:              make([]string, 0),
		requestedBrokerMode:    requestedMode,
		effectiveBrokerMode:    effectiveMode,
		executionAuditMsg:      audit,
	}

	o.scheduler = NewScheduler(ctx, marketData, stateStore, o.circuitBreaker, config, effectiveMode)
	o.agentRunner = NewAgentRunner(stateStore, marketData, nil, effectiveMode)
	o.executionMgr = NewExecutionManager(broker, o.circuitBreaker, config, nil)

	o.scheduler.SetMetrics(o.metrics)
	o.scheduler.SetEventBus(eventBus)
	o.agentRunner.SetEventBus(eventBus)
	o.agentRunner.SetMetrics(o.metrics)
	o.executionMgr.SetEventBus(eventBus)

	o.scheduler.SetCycleCallbacks(o.onMarketOpen, o.onIntradayCycle, o.onMarketClose, o.fetchAndDispatchQuotes)

	o.setupEventHandlers()

	return o
}

func (o *Orchestrator) applyExecutionInput() error {
	if o.executionInputProvider == nil {
		return nil
	}
	input, err := o.executionInputProvider.Produce(o.ctx, o.watchlist)
	if err != nil {
		return fmt.Errorf("produce execution input: %w", err)
	}
	return o.agentRunner.ApplyExecutionInput(o.ctx, *input)
}

func (o *Orchestrator) SetTradingMetrics(metrics MetricsRecorder) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.metrics = metrics
	o.scheduler.SetMetrics(metrics)
	o.executionMgr.metrics = metrics
}

func (o *Orchestrator) SetCircuitBreaker(cb *CircuitBreaker) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.circuitBreaker = cb
	o.scheduler.circuitBreaker = cb
}

func (o *Orchestrator) SetBroker(broker Broker) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	retries := o.config.BrokerMaxRetries
	if retries < 0 {
		retries = 0
	}
	if broker == nil {
		o.broker = NewDryRunBroker()
		o.orderMgr = NewOrderManager(o.broker, o.eventBus, retries, 100*time.Millisecond)
		o.requestedBrokerMode = "dry-run"
		o.effectiveBrokerMode = "dry-run"
		o.executionAuditMsg = ""
		return
	}
	o.broker = broker
	o.orderMgr = NewOrderManager(o.broker, o.eventBus, retries, 100*time.Millisecond)
	o.requestedBrokerMode = broker.Mode()
	o.effectiveBrokerMode = broker.Mode()
	o.executionAuditMsg = ""
	o.executionMgr.broker = broker
}

func (o *Orchestrator) SetWatchlist(symbols []string) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.watchlist = symbols
	o.scheduler.SetWatchlist(symbols)
}

func (o *Orchestrator) Start() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.isRunning {
		return fmt.Errorf("orchestrator already running")
	}
	o.isRunning = true

	o.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventSystemStart,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"market_data_provider":           o.marketData.Name(),
			"watchlist_count":                len(o.watchlist),
			"broker_mode_requested":          o.requestedBrokerMode,
			"broker_mode_effective":          o.effectiveBrokerMode,
			"broker_adapter":                 o.config.BrokerAdapter,
			"broker_signer":                  o.config.BrokerSigner,
			"broker_key_id":                  o.config.BrokerKeyID,
			"broker_http_attempts":           o.config.BrokerHTTPAttempts,
			"broker_http_timeout_sec":        o.config.BrokerHTTPTimeoutS,
			"broker_http_retry_status_codes": o.config.BrokerHTTPRetryStatusCodes,
			"broker_max_clock_skew_sec":      o.config.BrokerMaxClockSkewS,
			"broker_nonce_ttl_sec":           o.config.BrokerNonceTTLS,
			"broker_nonce_store":             o.config.BrokerNonceStore,
			"broker_nonce_store_path":        o.config.BrokerNonceStorePath,
			"broker_nonce_redis_key_prefix":  o.config.BrokerNonceRedisKeyPrefix,
			"broker_max_retries":             o.config.BrokerMaxRetries,
		},
	})

	if o.executionAuditMsg != "" {
		o.publishEvent(BusEvent{
			ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
			Type:      EventSystemError,
			Timestamp: time.Now(),
			Payload: map[string]string{
				"error": o.executionAuditMsg,
			},
		})
	}

	if err := o.scheduler.Start(); err != nil {
		return fmt.Errorf("start scheduler: %w", err)
	}

	fmt.Printf("[Orchestrator] Started with %d symbols, poll interval: %v, broker=%s\n",
		len(o.watchlist), o.config.QuotePollInterval, o.effectiveBrokerMode)

	return nil
}

func (o *Orchestrator) Stop() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if !o.isRunning {
		return nil
	}
	o.isRunning = false
	o.cancel()

	if err := o.scheduler.Stop(); err != nil {
		fmt.Printf("[Orchestrator] scheduler stop error: %v\n", err)
	}

	o.wg.Wait()

	if err := o.stateStore.Save(); err != nil {
		return fmt.Errorf("save final state: %w", err)
	}

	fmt.Println("[Orchestrator] Stopped")
	return nil
}

func (o *Orchestrator) setupEventHandlers() {
	o.eventBus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error {
		if payload, ok := event.Payload.(MarketEventPayload); ok {
			quotes := []domain.Quote{payload.Quote}
			o.stateStore.UpdatePositionPrices(quotes)

			if o.config.StopLossEnabled || o.config.TakeProfitEnabled {
				o.checkRiskTriggers(payload.Symbol, payload.Quote.Last)
			}
		}
		return nil
	})

	o.eventBus.Subscribe(EventPositionUpdate, func(ctx context.Context, event BusEvent) error {
		if payload, ok := event.Payload.(PositionEventPayload); ok {
			fmt.Printf("[Position] %s: %s %s x%d @ %.2f\n",
				payload.ChangeType,
				payload.Symbol,
				payload.Position.Symbol,
				payload.Position.Quantity,
				payload.Position.AverageCost)
		}
		return nil
	})

	o.eventBus.Subscribe(EventOrderPlaced, func(ctx context.Context, event BusEvent) error {
		if payload, ok := event.Payload.(OrderEventPayload); ok {
			fmt.Printf("[Order] Placed: %s %s x%d @ %.2f\n",
				payload.Order.Side,
				payload.Order.Symbol,
				payload.Order.Quantity,
				payload.Order.Price)
		}
		return nil
	})
}

func (o *Orchestrator) onMarketOpen() {
	fmt.Println("[Orchestrator] Market OPEN")

	o.stateStore.ResetDayState()
	portfolio := o.stateStore.GetPortfolio()
	startingValue := portfolio.Cash + portfolio.UnrealizedPnL
	o.circuitBreaker.ResetDayState(startingValue)

	positions := o.stateStore.GetPositions()
	fmt.Printf("[Orchestrator] Loaded %d positions\n", len(positions))

	o.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventMarketOpen,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"positions_count": len(positions),
			"cash":            o.stateStore.GetPortfolio().Cash,
		},
	})

	if err := o.applyExecutionInput(); err != nil {
		if o.effectiveBrokerMode == "live" {
			logging.Error("live_orchestrator", "critical_error", logging.Err(err))
		} else {
			logging.Warn("live_orchestrator", "warning_continuing", logging.Err(err), "broker_mode", o.effectiveBrokerMode)
		}
	}
}

func (o *Orchestrator) onIntradayCycle() {
	fmt.Printf("[Orchestrator] Intraday cycle at %s\n", time.Now().Format("15:04:05"))

	o.fetchAndProcessQuotes()

	portfolio := o.stateStore.GetPortfolio()
	o.circuitBreaker.Evaluate(portfolio, o.stateStore.GetPositions(), nil)
	state := o.circuitBreaker.State()
	if o.metrics != nil {
		o.metrics.RecordCircuitBreakerState(string(state))
		o.metrics.RecordPortfolio(portfolio.Cash, portfolio.Cash+portfolio.UnrealizedPnL)
		o.metrics.RecordGauge("portfolio_day_pnl", portfolio.DayPnL, nil)
	}
	if state != CircuitNormal {
		fmt.Printf("[CircuitBreaker] Trading restricted: state=%s\n", state)
		if state == CircuitHalted {
			return
		}
	}

	dayPnL := portfolio.DayPnL
	dayPnLPct := (dayPnL / portfolio.Cash) * 100
	if o.config.MaxDailyLossPct > 0 && dayPnLPct < -o.config.MaxDailyLossPct {
		fmt.Printf("[Risk] Daily loss limit hit: %.2f%%\n", dayPnLPct)
		o.publishRiskEvent(EventRiskAlert, "", domain.Position{}, "max_daily_loss", 0)
	}

	if err := o.applyExecutionInput(); err != nil {
		if o.effectiveBrokerMode == "live" {
			logging.Error("live_orchestrator", "critical_error", logging.Err(err))
		} else {
			logging.Warn("live_orchestrator", "warning_continuing", logging.Err(err), "broker_mode", o.effectiveBrokerMode)
		}
	}

	o.executionMgr.SimulateExecution()
}

func (o *Orchestrator) onMarketClose() {
	fmt.Println("[Orchestrator] Market CLOSE")

	o.fetchAndProcessQuotes()

	portfolio := o.stateStore.GetPortfolio()

	if err := o.stateStore.Save(); err != nil {
		fmt.Printf("[Error] Save state: %v\n", err)
	}

	o.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventMarketClose,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"day_pnl":        portfolio.DayPnL,
			"unrealized_pnl": portfolio.UnrealizedPnL,
			"total_exposure": portfolio.TotalExposure,
		},
	})
}

func (o *Orchestrator) fetchAndProcessQuotes() {
	if len(o.watchlist) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(o.ctx, 10*time.Second)
	defer cancel()

	quotes, err := o.marketData.GetQuotes(ctx, time.Now(), o.watchlist)
	if err != nil {
		o.publishEvent(BusEvent{
			ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
			Type:      EventSystemError,
			Timestamp: time.Now(),
			Payload: map[string]string{
				"error": fmt.Sprintf("fetch quotes: %v", err),
			},
		})
		return
	}

	for _, quote := range quotes {
		o.publishMarketSnapshot(quote)
	}
}

func (o *Orchestrator) fetchAndDispatchQuotes() {
	o.fetchAndProcessQuotes()
}

func (o *Orchestrator) checkRiskTriggers(symbol string, currentPrice float64) {
	position, ok := o.stateStore.GetPosition(symbol)
	if !ok {
		return
	}

	pnlPct := (currentPrice - position.AverageCost) / position.AverageCost * 100

	if o.config.StopLossEnabled && pnlPct < -o.config.MaxPositionLossPct {
		o.circuitBreaker.RecordStopLoss()
		if o.metrics != nil {
			o.metrics.RecordRiskEvent("stop_loss", symbol)
		}
		o.publishRiskEvent(EventStopLossTriggered, symbol, position, "stop_loss", currentPrice)
		fmt.Printf("[Risk] Stop loss triggered for %s at %.2f (loss: %.2f%%)\n",
			symbol, currentPrice, pnlPct)
	}

	if o.config.TakeProfitEnabled && pnlPct > o.config.MaxPositionLossPct*2 {
		if o.metrics != nil {
			o.metrics.RecordRiskEvent("take_profit", symbol)
		}
		o.publishRiskEvent(EventTakeProfitTriggered, symbol, position, "take_profit", currentPrice)
		fmt.Printf("[Risk] Take profit triggered for %s at %.2f (gain: %.2f%%)\n",
			symbol, currentPrice, pnlPct)
	}
}

func (o *Orchestrator) Status() map[string]interface{} {
	o.mutex.RLock()
	defer o.mutex.RUnlock()

	portfolio := o.stateStore.GetPortfolio()
	positions := o.stateStore.GetPositions()

	return map[string]interface{}{
		"is_running":         o.isRunning,
		"market_data_source": o.marketData.Name(),
		"watchlist_size":     len(o.watchlist),
		"positions_count":    len(positions),
		"portfolio": map[string]interface{}{
			"cash":           portfolio.Cash,
			"available_cash": portfolio.AvailableCash,
			"total_exposure": portfolio.TotalExposure,
			"day_pnl":        portfolio.DayPnL,
			"unrealized_pnl": portfolio.UnrealizedPnL,
		},
		"config": map[string]interface{}{
			"market_open":                    o.config.MarketOpenTime,
			"market_close":                   o.config.MarketCloseTime,
			"intraday_cycle":                 o.config.IntradayInterval.String(),
			"quote_poll":                     o.config.QuotePollInterval.String(),
			"stop_loss_enabled":              o.config.StopLossEnabled,
			"broker_mode_requested":          o.requestedBrokerMode,
			"broker_mode_effective":          o.effectiveBrokerMode,
			"broker_adapter":                 o.config.BrokerAdapter,
			"broker_signer":                  o.config.BrokerSigner,
			"broker_key_id":                  o.config.BrokerKeyID,
			"broker_http_attempts":           o.config.BrokerHTTPAttempts,
			"broker_http_timeout_sec":        o.config.BrokerHTTPTimeoutS,
			"broker_http_retry_status_codes": o.config.BrokerHTTPRetryStatusCodes,
			"broker_max_clock_skew_sec":      o.config.BrokerMaxClockSkewS,
			"broker_nonce_ttl_sec":           o.config.BrokerNonceTTLS,
			"broker_nonce_store":             o.config.BrokerNonceStore,
			"broker_nonce_store_path":        o.config.BrokerNonceStorePath,
			"broker_nonce_redis_key_prefix":  o.config.BrokerNonceRedisKeyPrefix,
			"broker_max_retries":             o.config.BrokerMaxRetries,
		},
	}
}

func (o *Orchestrator) executeOrder(ctx context.Context, order domain.Order) error {
	if o.executionMgr == nil {
		retries := o.config.BrokerMaxRetries
		if retries < 0 {
			retries = 0
		}
		if o.broker == nil {
			o.broker = NewDryRunBroker()
		}
		o.orderMgr = NewOrderManager(o.broker, o.eventBus, retries, 100*time.Millisecond)
		if !o.circuitBreaker.CanPlaceOrder(order.Side) {
			return fmt.Errorf("circuit breaker blocks %s order for %s (state=%s)", order.Side, order.Symbol, o.circuitBreaker.State())
		}
		if err := o.orderMgr.Execute(ctx, order); err != nil {
			return fmt.Errorf("execute order via manager: %w", err)
		}
		return nil
	}
	return o.executionMgr.ExecuteOrder(ctx, order)
}

func (o *Orchestrator) ExecuteOrder(ctx context.Context, order domain.Order) error {
	return o.executionMgr.ExecuteOrder(ctx, order)
}

func (o *Orchestrator) publishEvent(event BusEvent) {
	if o.eventBus == nil {
		return
	}
	if err := o.eventBus.Publish(event); err != nil {
	}
}

func (o *Orchestrator) publishMarketSnapshot(quote domain.Quote) {
	if o.eventBus == nil {
		return
	}
	_ = o.eventBus.PublishMarketSnapshot(quote)
}

func (o *Orchestrator) publishRiskEvent(eventType EventType, symbol string, position domain.Position, triggerType string, triggerPrice float64) {
	if o.eventBus == nil {
		return
	}
	_ = o.eventBus.PublishRiskEvent(eventType, symbol, position, triggerType, triggerPrice)
}

func resolveBrokerMode(cfg OrchestratorConfig) (requested string, effective string, broker Broker, auditMsg string) {
	requested = strings.TrimSpace(strings.ToLower(cfg.BrokerMode))
	if requested == "" {
		requested = "dry-run"
	}

	adapterProvider := strings.TrimSpace(strings.ToLower(cfg.BrokerAdapter))
	if adapterProvider == "" {
		adapterProvider = "guarded"
	}

	switch requested {
	case "dry-run", "paper":
		return requested, "dry-run", NewDryRunBroker(), ""
	case "live":
		switch adapterProvider {
		case "guarded":
			return requested, "live-guarded", NewGuardedLiveBroker(nil), "live mode enabled with guarded adapter; orders are rejected until adapter is configured"
		case "mock":
			return requested, "live-mock", NewGuardedLiveBroker(NewMockLiveAdapter()), "live mode uses mock adapter; no real orders are sent"
		case "http":
			nonceStore, err := BuildNonceReplayStoreWithOptions(cfg.BrokerNonceStore, NonceReplayStoreOptions{
				FilePath:       cfg.BrokerNonceStorePath,
				RedisURL:       cfg.BrokerNonceRedisURL,
				RedisKeyPrefix: cfg.BrokerNonceRedisKeyPrefix,
			})
			if err != nil {
				return requested, "live-guarded", NewGuardedLiveBroker(nil), fmt.Sprintf("live+http adapter requested but nonce store config invalid: %v; fallback to guarded", err)
			}
			httpAdapter := NewHTTPBrokerAdapter(HTTPBrokerAdapterConfig{
				BaseURL:              cfg.BrokerAPIBaseURL,
				APIKey:               cfg.BrokerAPIKey,
				APISecret:            cfg.BrokerAPISecret,
				KeyID:                cfg.BrokerKeyID,
				Timeout:              time.Duration(cfg.BrokerHTTPTimeoutS) * time.Second,
				MaxAttempts:          cfg.BrokerHTTPAttempts,
				RetryableStatusCodes: cfg.BrokerHTTPRetryStatusCodes,
				MaxClockSkew:         time.Duration(cfg.BrokerMaxClockSkewS) * time.Second,
				NonceTTL:             time.Duration(cfg.BrokerNonceTTLS) * time.Second,
				NonceStore:           nonceStore,
				Signer:               cfg.BrokerSigner,
			})
			if strings.TrimSpace(cfg.BrokerAPIBaseURL) == "" {
				return requested, "live-guarded", NewGuardedLiveBroker(nil), "live+http adapter requested but ATLAS_BROKER_API_BASE_URL is empty; fallback to guarded"
			}
			if strings.TrimSpace(cfg.BrokerAPIKey) == "" {
				return requested, "live-guarded", NewGuardedLiveBroker(nil), "live+http adapter requested but ATLAS_BROKER_API_KEY is empty; fallback to guarded"
			}
			return requested, "live-http", NewGuardedLiveBroker(httpAdapter), "live mode uses http adapter with signature placeholder; verify credentials and endpoint before production use"
		default:
			return requested, "live-guarded", NewGuardedLiveBroker(nil), fmt.Sprintf("unsupported broker adapter %q for live mode; fallback to guarded", adapterProvider)
		}
	default:
		return requested, "dry-run", NewDryRunBroker(), fmt.Sprintf("unsupported broker mode %q; fallback to dry-run", requested)
	}
}
