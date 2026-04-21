package live

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// Orchestrator 实时交易编排器
type Orchestrator struct {
	stateStore     *StateStore
	eventBus       *ChannelEventBus
	marketData     marketdata.Provider
	broker         Broker
	orderMgr       *OrderManager
	registry       domain.AgentRegistry
	system         *orchestrator.System
	circuitBreaker *CircuitBreaker

	// 配置
	config OrchestratorConfig

	// 运行状态
	isRunning bool
	mutex     sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup

	// 调度器
	intradayTicker *time.Ticker
	quoteTicker    *time.Ticker

	// 观测标的
	watchlist []string

	requestedBrokerMode string
	effectiveBrokerMode string
	executionAuditMsg   string

	metrics MetricsRecorder
}

// MetricsRecorder defines the interface for live trading metrics.
type MetricsRecorder interface {
	RecordOrder(order domain.Order, status string)
	RecordPosition(position domain.Position)
	RecordPortfolio(cash, totalValue float64)
	RecordCircuitBreakerState(state string)
	RecordRiskEvent(eventType, symbol string)
	RecordCounter(name string, value float64, labels map[string]string)
	RecordGauge(name string, value float64, labels map[string]string)
}

// OrchestratorConfig 编排器配置
type OrchestratorConfig struct {
	MarketOpenTime             string        // 09:00
	MarketCloseTime            string        // 13:30
	IntradayInterval           time.Duration // 5m
	QuotePollInterval          time.Duration // 30s
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

// DefaultOrchestratorConfig 默认配置
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

// NewOrchestrator 创建新的编排器
func NewOrchestrator(
	ctx context.Context,
	stateStore *StateStore,
	eventBus *ChannelEventBus,
	marketData marketdata.Provider,
	registry domain.AgentRegistry,
	system *orchestrator.System,
	config OrchestratorConfig,
) *Orchestrator {
	ctx, cancel := context.WithCancel(ctx)
	requestedMode, effectiveMode, broker, audit := resolveBrokerMode(config)
	maxRetries := config.BrokerMaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	return &Orchestrator{
		stateStore:          stateStore,
		eventBus:            eventBus,
		marketData:          marketData,
		broker:              broker,
		orderMgr:            NewOrderManager(broker, eventBus, maxRetries, 100*time.Millisecond),
		registry:            registry,
		system:              system,
		circuitBreaker:      NewCircuitBreaker("", ""),
		config:              config,
		ctx:                 ctx,
		cancel:              cancel,
		watchlist:           make([]string, 0),
		requestedBrokerMode: requestedMode,
		effectiveBrokerMode: effectiveMode,
		executionAuditMsg:   audit,
	}
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

// SetTradingMetrics 注入交易指标收集器
func (o *Orchestrator) SetTradingMetrics(metrics MetricsRecorder) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.metrics = metrics
}

// SetCircuitBreaker 注入自定义断路器（用于测试与演练）
func (o *Orchestrator) SetCircuitBreaker(cb *CircuitBreaker) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.circuitBreaker = cb
}

// SetBroker 注入自定义券商执行器，传入 nil 时回退到 dry-run。
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
}

// SetWatchlist 设置观测标的列表
func (o *Orchestrator) SetWatchlist(symbols []string) {
	o.mutex.Lock()
	defer o.mutex.Unlock()
	o.watchlist = symbols
}

// Start 启动编排器
func (o *Orchestrator) Start() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if o.isRunning {
		return fmt.Errorf("orchestrator already running")
	}

	o.isRunning = true

	if o.orderMgr == nil {
		retries := o.config.BrokerMaxRetries
		if retries < 0 {
			retries = 0
		}
		o.orderMgr = NewOrderManager(o.broker, o.eventBus, retries, 100*time.Millisecond)
	}

	// 启动事件订阅
	o.setupEventHandlers()

	// 启动市场时间调度器
	o.wg.Add(1)
	go o.marketTimeScheduler()

	// 启动数据轮询
	o.quoteTicker = time.NewTicker(o.config.QuotePollInterval)
	o.wg.Add(1)
	go o.quotePoller()

	// 发布系统启动事件
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

	fmt.Printf("[Orchestrator] Started with %d symbols, poll interval: %v, broker=%s\n",
		len(o.watchlist), o.config.QuotePollInterval, o.effectiveBrokerMode)

	return nil
}

// Stop 停止编排器
func (o *Orchestrator) Stop() error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if !o.isRunning {
		return nil
	}

	o.isRunning = false
	o.cancel()

	if o.quoteTicker != nil {
		o.quoteTicker.Stop()
	}
	if o.intradayTicker != nil {
		o.intradayTicker.Stop()
	}

	o.wg.Wait()

	// 保存最终状态
	if err := o.stateStore.Save(); err != nil {
		return fmt.Errorf("save final state: %w", err)
	}

	fmt.Println("[Orchestrator] Stopped")
	return nil
}

// setupEventHandlers 设置事件处理器
func (o *Orchestrator) setupEventHandlers() {
	// 监听市场快照事件
	o.eventBus.Subscribe(EventMarketSnapshot, func(ctx context.Context, event BusEvent) error {
		// 更新持仓价格
		if payload, ok := event.Payload.(MarketEventPayload); ok {
			quotes := []domain.Quote{payload.Quote}
			o.stateStore.UpdatePositionPrices(quotes)

			// 检查止损止盈
			if o.config.StopLossEnabled || o.config.TakeProfitEnabled {
				o.checkRiskTriggers(payload.Symbol, payload.Quote.Last)
			}
		}
		return nil
	})

	// 监听持仓更新事件
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

	// 监听订单事件
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

// marketTimeScheduler 市场时间调度器
func (o *Orchestrator) marketTimeScheduler() {
	defer o.wg.Done()

	for {
		select {
		case <-o.ctx.Done():
			return
		default:
		}

		now := time.Now()

		// 解析市场时间
		marketOpen, _ := time.Parse("15:04", o.config.MarketOpenTime)
		marketClose, _ := time.Parse("15:04", o.config.MarketCloseTime)

		todayOpen := time.Date(now.Year(), now.Month(), now.Day(),
			marketOpen.Hour(), marketOpen.Minute(), 0, 0, now.Location())
		todayClose := time.Date(now.Year(), now.Month(), now.Day(),
			marketClose.Hour(), marketClose.Minute(), 0, 0, now.Location())

		// 市场开盘处理
		if now.After(todayOpen) && now.Before(todayClose) {
			// 检查是否是开盘后的第一次
			if o.intradayTicker == nil {
				o.handleMarketOpen()
				o.intradayTicker = time.NewTicker(o.config.IntradayInterval)
				o.wg.Add(1)
				go o.intradayProcessor()
			}
		}

		// 市场收盘处理
		if now.After(todayClose) && o.intradayTicker != nil {
			o.handleMarketClose()
			o.intradayTicker.Stop()
			o.intradayTicker = nil
		}

		// 每分钟检查一次（可取消）
		select {
		case <-o.ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

// quotePoller 行情轮询器
func (o *Orchestrator) quotePoller() {
	defer o.wg.Done()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-o.quoteTicker.C:
			o.fetchAndProcessQuotes()
		}
	}
}

// fetchAndProcessQuotes 获取并处理行情
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

	// 发布行情事件
	for _, quote := range quotes {
		o.publishMarketSnapshot(quote)
	}
}

// intradayProcessor 盘中处理器
func (o *Orchestrator) intradayProcessor() {
	defer o.wg.Done()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-o.intradayTicker.C:
			o.handleIntradayCycle()
		}
	}
}

// handleMarketOpen 处理市场开盘
func (o *Orchestrator) handleMarketOpen() {
	fmt.Println("[Orchestrator] Market OPEN")

	// 重置每日状态
	o.stateStore.ResetDayState()

	// 重置 circuit breaker
	portfolio := o.stateStore.GetPortfolio()
	startingValue := portfolio.Cash + portfolio.UnrealizedPnL
	o.circuitBreaker.ResetDayState(startingValue)

	// 加载持仓
	positions := o.stateStore.GetPositions()
	fmt.Printf("[Orchestrator] Loaded %d positions\n", len(positions))

	// 发布开盘事件
	o.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventMarketOpen,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"positions_count": len(positions),
			"cash":            o.stateStore.GetPortfolio().Cash,
		},
	})

	// 运行 Context Agent 判断市场状态
	if err := o.runContextAgent(); err != nil {
		if o.effectiveBrokerMode == "live" {
			log.Printf("[Orchestrator] CRITICAL: %v", err)
		} else {
			log.Printf("[Orchestrator] WARNING: %v (continuing in %s mode)", err, o.effectiveBrokerMode)
		}
	}
}

// handleIntradayCycle 处理盘中周期
func (o *Orchestrator) handleIntradayCycle() {
	fmt.Printf("[Orchestrator] Intraday cycle at %s\n", time.Now().Format("15:04:05"))

	// 获取最新行情
	o.fetchAndProcessQuotes()

	// Circuit breaker evaluation
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
			return // skip entire cycle
		}
		// Paused: still run risk checks but skip new buy orders later
	}

	// 更新持仓盈亏
	dayPnL := portfolio.DayPnL
	dayPnLPct := (dayPnL / portfolio.Cash) * 100

	// 检查每日最大亏损 (legacy event publishing, circuit breaker handles action)
	if o.config.MaxDailyLossPct > 0 && dayPnLPct < -o.config.MaxDailyLossPct {
		fmt.Printf("[Risk] Daily loss limit hit: %.2f%%\n", dayPnLPct)
		o.publishRiskEvent(EventRiskAlert, "", domain.Position{},
			"max_daily_loss", 0)
	}

	// 运行 Agent 生成推荐
	if err := o.runStyleAndSectorAgents(); err != nil {
		if o.effectiveBrokerMode == "live" {
			log.Printf("[Orchestrator] CRITICAL: %v", err)
		} else {
			log.Printf("[Orchestrator] WARNING: %v (continuing in %s mode)", err, o.effectiveBrokerMode)
		}
	}

	// 应用 CRO 风险过滤
	if err := o.applyRiskFilters(); err != nil {
		if o.effectiveBrokerMode == "live" {
			log.Printf("[Orchestrator] CRITICAL: %v", err)
		} else {
			log.Printf("[Orchestrator] WARNING: %v (continuing in %s mode)", err, o.effectiveBrokerMode)
		}
	}

	// 模拟订单执行 (circuit breaker is checked inside executeOrder)
	o.simulateOrderExecution()
}

// handleMarketClose 处理市场收盘
func (o *Orchestrator) handleMarketClose() {
	fmt.Println("[Orchestrator] Market CLOSE")

	// 获取最终行情
	o.fetchAndProcessQuotes()

	// 计算最终盈亏
	portfolio := o.stateStore.GetPortfolio()

	// 保存状态
	if err := o.stateStore.Save(); err != nil {
		fmt.Printf("[Error] Save state: %v\n", err)
	}

	// 发布收盘事件
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

// runContextAgent 运行 Context Agent 判断市场状态
// 使用与 simulation 模式相同的 regime inference 逻辑
func (o *Orchestrator) runContextAgent() error {
	if o.system == nil {
		return fmt.Errorf("system not initialized")
	}

	// Fetch quotes for regime inference
	ctx, cancel := context.WithTimeout(o.ctx, 10*time.Second)
	defer cancel()

	// Use watchlist or default symbols
	symbols := o.watchlist
	if len(symbols) == 0 {
		symbols = []string{"0050.TW", "0056.TW", "2330.TW", "2317.TW", "2454.TW"}
	}

	quotes, err := o.marketData.GetQuotes(ctx, time.Now(), symbols)
	if err != nil {
		return fmt.Errorf("fetch quotes for regime inference: %w", err)
	}

	// Convert to map for registry processing
	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}

	// Get registry and plugins from system
	registry := o.system.Registry()
	plugins := o.system.GetPlugins()
	if plugins == nil {
		plugins = orchestrator.NewPluginRegistry()
	}

	// Infer regime using same logic as simulation
	regime := o.inferRegime(registry, quoteMap, plugins)

	// Store current regime in state
	o.stateStore.SetCurrentRegime(regime)

	// Publish regime change event
	o.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventSystemStart, // Reuse type or create EventRegimeChange
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"regime": string(regime),
			"type":   "regime_inference",
		},
	})

	fmt.Printf("[ContextAgent] Regime inferred: %s\n", regime)
	return nil
}

// inferRegime replicates the simulation regime inference logic
func (o *Orchestrator) inferRegime(registry domain.AgentRegistry, quotes map[string]domain.Quote, plugins *orchestrator.PluginRegistry) domain.Regime {
	score := 0
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerContext {
			continue
		}
		prompt := plugins.ResolvePrompt(agent, nil)
		score += plugins.RegimeScore(agent, quotes, prompt)
	}

	switch {
	case score > 0:
		return domain.RegimeRiskOn
	case score < 0:
		return domain.RegimeRiskOff
	default:
		return domain.RegimeNeutral
	}
}

// runStyleAndSectorAgents 运行 Style 和 Sector Agents
// 生成股票推荐并存储在 state 中供后续执行
func (o *Orchestrator) runStyleAndSectorAgents() error {
	if o.system == nil {
		return fmt.Errorf("system not initialized")
	}

	// Fetch quotes for all watchlist symbols
	ctx, cancel := context.WithTimeout(o.ctx, 10*time.Second)
	defer cancel()

	if len(o.watchlist) == 0 {
		return nil // No symbols to analyze
	}

	quotes, err := o.marketData.GetQuotes(ctx, time.Now(), o.watchlist)
	if err != nil {
		return fmt.Errorf("fetch quotes for agent analysis: %w", err)
	}

	// Convert to map
	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}

	// Get registry and plugins
	registry := o.system.Registry()
	plugins := o.system.GetPlugins()
	if plugins == nil {
		plugins = orchestrator.NewPluginRegistry()
	}

	// Get current regime
	regime := o.stateStore.GetCurrentRegime()

	// Collect recommendations using same logic as simulation
	var recommendations []domain.Recommendation
	for _, agent := range registry.Agents {
		if !agent.Enabled {
			continue
		}
		if agent.Layer != domain.LayerSector && agent.Layer != domain.LayerStyle && agent.Layer != domain.LayerSuperinvestor {
			continue
		}

		prompt := plugins.ResolvePrompt(agent, nil)
		symbols := agent.Universe
		if len(symbols) == 0 {
			symbols = o.watchlist
		}

		for _, symbol := range symbols {
			quote, ok := quoteMap[symbol]
			if !ok || !quote.IsTradable {
				continue
			}

			// Apply screening
			passed, err := plugins.Screen(ctx, agent, symbol, quoteMap)
			if err != nil || !passed {
				continue
			}

			rec, ok := plugins.Recommendation(agent, quote, prompt, regime)
			if !ok {
				continue
			}
			recommendations = append(recommendations, rec)
		}
	}

	// Store recommendations
	if len(recommendations) > 0 {
		o.stateStore.SetPendingRecommendations(recommendations)
		fmt.Printf("[StyleAndSectorAgents] Generated %d recommendations\n", len(recommendations))
	}

	// Publish event
	o.publishEvent(BusEvent{
		ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
		Type:      EventSystemStart,
		Timestamp: time.Now(),
		Payload: map[string]interface{}{
			"recommendation_count": len(recommendations),
			"type":                 "agent_recommendations",
		},
	})

	return nil
}

// applyRiskFilters 应用 CRO Agent 风险过滤
// 使用与 simulation 模式相同的控制层逻辑
func (o *Orchestrator) applyRiskFilters() error {
	if o.system == nil {
		return fmt.Errorf("system not initialized")
	}

	// Get pending recommendations
	recommendations := o.stateStore.GetPendingRecommendations()
	if len(recommendations) == 0 {
		return nil // No recommendations to filter
	}

	// Get registry and plugins
	registry := o.system.Registry()
	plugins := o.system.GetPlugins()
	if plugins == nil {
		plugins = orchestrator.NewPluginRegistry()
	}

	// Get execution policy from system
	policy := o.system.GetExecutionPolicy()

	// Apply control layer filters
	filtered := recommendations
	for _, agent := range registry.Agents {
		if !agent.Enabled || agent.Layer != domain.LayerControl {
			continue
		}
		filtered = plugins.ApplyControl(agent, filtered, policy)
	}

	// Store filtered recommendations
	o.stateStore.SetFilteredRecommendations(filtered)

	// Calculate blocked count
	blockedCount := len(recommendations) - len(filtered)

	fmt.Printf("[RiskFilters] Applied CRO/CIO filters: %d passed, %d blocked\n", len(filtered), blockedCount)

	// Publish event
	if blockedCount > 0 {
		o.publishEvent(BusEvent{
			ID:        fmt.Sprintf("evt-%d", time.Now().UnixNano()),
			Type:      EventRiskAlert,
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"blocked_count": blockedCount,
				"passed_count":  len(filtered),
				"type":          "risk_filter",
			},
		})
	}

	return nil
}

// simulateOrderExecution 模拟订单执行
func (o *Orchestrator) simulateOrderExecution() {
	if o.broker == nil {
		o.broker = NewDryRunBroker()
	}
	if o.orderMgr == nil {
		retries := o.config.BrokerMaxRetries
		if retries < 0 {
			retries = 0
		}
		o.orderMgr = NewOrderManager(o.broker, o.eventBus, retries, 100*time.Millisecond)
	}

	// Phase 6 起步：先建立可审计的执行通道，默认 dry-run，不触发真实下单。
	fmt.Printf("[Trading] Execution channel ready (mode=%s)\n", o.orderMgr.Mode())
	if o.metrics != nil {
		o.metrics.RecordCounter("execution_cycles_total", 1, map[string]string{
			"broker_mode": o.effectiveBrokerMode,
		})
	}
}

func (o *Orchestrator) executeOrder(ctx context.Context, order domain.Order) error {
	if !o.circuitBreaker.CanPlaceOrder(order.Side) {
		if o.metrics != nil {
			o.metrics.RecordCounter("orders_blocked_total", 1, map[string]string{
				"symbol": order.Symbol,
				"side":   string(order.Side),
				"reason": string(o.circuitBreaker.State()),
			})
		}
		return fmt.Errorf("circuit breaker blocks %s order for %s (state=%s)", order.Side, order.Symbol, o.circuitBreaker.State())
	}
	if o.orderMgr == nil {
		if o.broker == nil {
			o.broker = NewDryRunBroker()
		}
		retries := o.config.BrokerMaxRetries
		if retries < 0 {
			retries = 0
		}
		o.orderMgr = NewOrderManager(o.broker, o.eventBus, retries, 100*time.Millisecond)
	}
	if err := o.orderMgr.Execute(ctx, order); err != nil {
		if o.metrics != nil {
			o.metrics.RecordCounter("orders_failed_total", 1, map[string]string{
				"symbol": order.Symbol,
				"side":   string(order.Side),
			})
		}
		return fmt.Errorf("execute order via manager: %w", err)
	}
	if o.metrics != nil {
		o.metrics.RecordOrder(order, "submitted")
	}
	return nil
}

// checkRiskTriggers 检查风险触发条件
func (o *Orchestrator) checkRiskTriggers(symbol string, currentPrice float64) {
	position, ok := o.stateStore.GetPosition(symbol)
	if !ok {
		return
	}

	// 计算盈亏百分比
	pnlPct := (currentPrice - position.AverageCost) / position.AverageCost * 100

	// 检查止损
	if o.config.StopLossEnabled && pnlPct < -o.config.MaxPositionLossPct {
		o.circuitBreaker.RecordStopLoss()
		if o.metrics != nil {
			o.metrics.RecordRiskEvent("stop_loss", symbol)
		}
		o.publishRiskEvent(EventStopLossTriggered, symbol, position,
			"stop_loss", currentPrice)
		fmt.Printf("[Risk] Stop loss triggered for %s at %.2f (loss: %.2f%%)\n",
			symbol, currentPrice, pnlPct)
	}

	// 检查止盈 (简化: 2倍止损距离)
	if o.config.TakeProfitEnabled && pnlPct > o.config.MaxPositionLossPct*2 {
		if o.metrics != nil {
			o.metrics.RecordRiskEvent("take_profit", symbol)
		}
		o.publishRiskEvent(EventTakeProfitTriggered, symbol, position,
			"take_profit", currentPrice)
		fmt.Printf("[Risk] Take profit triggered for %s at %.2f (gain: %.2f%%)\n",
			symbol, currentPrice, pnlPct)
	}
}

// Status 获取运行状态
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

func (o *Orchestrator) publishEvent(event BusEvent) {
	if o.eventBus == nil {
		return
	}
	if err := o.eventBus.Publish(event); err != nil {
		// Event bus errors are non-fatal; best-effort delivery only.
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
