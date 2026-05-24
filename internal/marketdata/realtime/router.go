package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// RedisConfig Redis PubSub 設定
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Addr     string `yaml:"addr"`
	Channel  string `yaml:"channel"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

type RouterConfig struct {
	Providers          []ProviderConfig `yaml:"providers"`
	FailoverTimeoutS   int              `yaml:"failover_timeout_s"`
	HealthCheckPeriodS int              `yaml:"health_check_period_s"`
	Redis              RedisConfig      `yaml:"redis"`
}

type ProviderConfig struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	APIKey  string `yaml:"api_key"`
	WSURL   string `yaml:"ws_url"`
	Enabled bool   `yaml:"enabled"`
}

func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		FailoverTimeoutS:   30,
		HealthCheckPeriodS: 60,
	}
}

type RealtimeRouter struct {
	providers    []RealtimeProvider
	activeIdx    atomic.Int32
	callbacks    []QuoteCallback
	cbMu         sync.RWMutex
	config       RouterConfig
	running      bool
	runningMu    sync.Mutex
	cancelCtx    context.Context
	cancelFunc   context.CancelFunc
	failoverCh   chan int32
	redisClient  *redis.Client
	redisChannel string
}

func NewRealtimeRouter(config RouterConfig, providers []RealtimeProvider) *RealtimeRouter {
	return &RealtimeRouter{
		providers:  providers,
		config:     config,
		failoverCh: make(chan int32, len(providers)),
	}
}

// WithRedis 設定 Redis PubSub 發布，啟用後 emitQuote 會將報價同步發布到指定頻道
func (r *RealtimeRouter) WithRedis(client *redis.Client, channel string) *RealtimeRouter {
	r.redisClient = client
	r.redisChannel = channel
	return r
}

func (r *RealtimeRouter) Start(ctx context.Context) error {
	r.runningMu.Lock()
	if r.running {
		r.runningMu.Unlock()
		return nil
	}
	r.running = true
	r.runningMu.Unlock()

	routerCtx, cancel := context.WithCancel(ctx)
	r.cancelCtx = routerCtx
	r.cancelFunc = cancel

	for i, p := range r.providers {
		if err := p.Connect(routerCtx); err != nil {
			continue
		}
		p.OnQuote(func(quote domain.Quote) {
			r.emitQuote(quote)
		})
		if i == 0 {
			r.activeIdx.Store(0)
		}
	}

	go r.healthCheckLoop()
	go r.failoverLoop()

	return nil
}

func (r *RealtimeRouter) Stop(ctx context.Context) error {
	r.runningMu.Lock()
	r.running = false
	r.runningMu.Unlock()

	if r.cancelFunc != nil {
		r.cancelFunc()
	}

	var firstErr error
	for _, p := range r.providers {
		if err := p.Disconnect(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *RealtimeRouter) Subscribe(symbols []string) error {
	active := r.activeProvider()
	if active == nil {
		return fmt.Errorf("realtime router: no active provider")
	}
	return active.Subscribe(symbols)
}

func (r *RealtimeRouter) Unsubscribe(symbols []string) error {
	active := r.activeProvider()
	if active == nil {
		return fmt.Errorf("realtime router: no active provider")
	}
	return active.Unsubscribe(symbols)
}

func (r *RealtimeRouter) OnQuote(callback QuoteCallback) {
	r.cbMu.Lock()
	r.callbacks = append(r.callbacks, callback)
	r.cbMu.Unlock()
}

func (r *RealtimeRouter) ActiveProvider() RealtimeProvider {
	return r.activeProvider()
}

func (r *RealtimeRouter) SwitchToNext() bool {
	currentIdx := r.activeIdx.Load()
	nextIdx := int32(0)
	if currentIdx < int32(len(r.providers))-1 {
		nextIdx = currentIdx + 1
	}

	if nextIdx == currentIdx {
		return false
	}

	next := r.providers[nextIdx]
	if !next.IsConnected() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(r.config.FailoverTimeoutS)*time.Second)
		defer cancel()
		if err := next.Connect(ctx); err != nil {
			return false
		}
	}

	old := r.activeProvider()
	r.activeIdx.Store(nextIdx)

	if old != nil {
		r.failoverCh <- currentIdx
	}

	return true
}

func (r *RealtimeRouter) Status() []ProviderStatus {
	statuses := make([]ProviderStatus, len(r.providers))
	for i, p := range r.providers {
		if s, ok := p.(interface{ Status() ProviderStatus }); ok {
			statuses[i] = s.Status()
		} else {
			state := ProviderStateDisconnected
			if p.IsConnected() {
				state = ProviderStateConnected
			}
			statuses[i] = ProviderStatus{
				Name:  p.Name(),
				State: state,
			}
		}
	}
	return statuses
}

func (r *RealtimeRouter) activeProvider() RealtimeProvider {
	idx := r.activeIdx.Load()
	if int(idx) >= len(r.providers) {
		return nil
	}
	return r.providers[idx]
}

func (r *RealtimeRouter) emitQuote(quote domain.Quote) {
	r.cbMu.RLock()
	callbacks := r.callbacks
	r.cbMu.RUnlock()

	for _, cb := range callbacks {
		cb(quote)
	}

	if r.redisClient != nil && r.redisChannel != "" {
		data, err := json.Marshal(quote)
		if err == nil {
			r.redisClient.Publish(r.cancelCtx, r.redisChannel, data)
		}
	}
}

func (r *RealtimeRouter) healthCheckLoop() {
	period := time.Duration(r.config.HealthCheckPeriodS) * time.Second
	if period == 0 {
		period = 60 * time.Second
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			active := r.activeProvider()
			if active != nil && !active.IsConnected() {
				r.SwitchToNext()
			}
		case <-r.cancelCtx.Done():
			return
		}
	}
}

func (r *RealtimeRouter) failoverLoop() {
	for {
		select {
		case failedIdx := <-r.failoverCh:
			if int(failedIdx) < len(r.providers) {
				r.providers[failedIdx].Disconnect(r.cancelCtx)
			}
		case <-r.cancelCtx.Done():
			return
		}
	}
}
