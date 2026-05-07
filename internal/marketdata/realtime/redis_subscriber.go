package realtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/redis/go-redis/v9"
)

// RedisSubscriber 訂閱 Redis PubSub 頻道並將收到的報價轉發至回呼
type RedisSubscriber struct {
	client   *redis.Client
	channel  string
	callback QuoteCallback

	running   bool
	runningMu sync.Mutex
	cancelCtx context.Context
	cancelFn  context.CancelFunc
}

func NewRedisSubscriber(client *redis.Client, channel string, callback QuoteCallback) *RedisSubscriber {
	return &RedisSubscriber{
		client:   client,
		channel:  channel,
		callback: callback,
	}
}

func (s *RedisSubscriber) Start(ctx context.Context) error {
	s.runningMu.Lock()
	if s.running {
		s.runningMu.Unlock()
		return nil
	}
	s.running = true
	s.runningMu.Unlock()

	subCtx, cancel := context.WithCancel(ctx)
	s.cancelCtx = subCtx
	s.cancelFn = cancel

	pubsub := s.client.Subscribe(subCtx, s.channel)

	go func() {
		defer pubsub.Close()
		ch := pubsub.Channel()
		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var quote domain.Quote
				if err := json.Unmarshal([]byte(msg.Payload), &quote); err != nil {
					continue
				}
				s.callback(quote)
			case <-subCtx.Done():
				return
			}
		}
	}()

	return nil
}

func (s *RedisSubscriber) Stop() error {
	s.runningMu.Lock()
	s.running = false
	s.runningMu.Unlock()

	if s.cancelFn != nil {
		s.cancelFn()
	}
	return nil
}

// NewRedisClientFromConfig 從 RedisConfig 建立 *redis.Client
func NewRedisClientFromConfig(cfg RedisConfig) (*redis.Client, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("redis: not enabled in config")
	}
	if cfg.Addr == "" {
		return nil, fmt.Errorf("redis: addr is required")
	}
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	return client, nil
}
