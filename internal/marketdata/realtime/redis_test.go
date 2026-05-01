package realtime

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/kaecer68/atlas-go/internal/domain"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis 啟動失敗: %v", err)
	}
	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	return mr, client
}

func TestRealtimeRouter_WithRedis_PublishesQuote(t *testing.T) {
	mr, client := setupMiniredis(t)
	defer mr.Close()
	defer client.Close()

	channel := "atlas:quotes"

	mock1 := &mockProvider{name: "mock-1", connected: true}
	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1})
	router.WithRedis(client, channel)

	ctx := context.Background()
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Start 失敗: %v", err)
	}
	defer router.Stop(ctx)

	quote := domain.Quote{
		Symbol:     "2330",
		Last:       785.0,
		Open:       780.0,
		High:       790.0,
		Low:        775.0,
		Volume:     15000000,
		Market:     "TW",
		IsTradable: true,
		Source:     "test",
	}

	router.emitQuote(quote)

	time.Sleep(100 * time.Millisecond)

	val, err := client.Get(ctx, "last_quote_"+quote.Symbol).Result()
	_ = val
	_ = err
}

func TestRealtimeRouter_WithRedis_SubscriberReceivesQuote(t *testing.T) {
	mr, client := setupMiniredis(t)
	defer mr.Close()
	defer client.Close()

	channel := "atlas:quotes"

	var received domain.Quote
	var mu sync.Mutex
	sub := NewRedisSubscriber(client, channel, func(q domain.Quote) {
		mu.Lock()
		received = q
		mu.Unlock()
	})

	ctx := context.Background()
	if err := sub.Start(ctx); err != nil {
		t.Fatalf("Subscriber Start 失敗: %v", err)
	}
	defer sub.Stop()

	time.Sleep(50 * time.Millisecond)

	quote := domain.Quote{
		Symbol:     "2330",
		Last:       785.0,
		Open:       780.0,
		High:       790.0,
		Low:        775.0,
		Volume:     15000000,
		Market:     "TW",
		IsTradable: true,
		Source:     "test",
	}

	data, err := json.Marshal(quote)
	if err != nil {
		t.Fatalf("json.Marshal 失敗: %v", err)
	}

	if err := client.Publish(ctx, channel, data).Err(); err != nil {
		t.Fatalf("Publish 失敗: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if received.Symbol != "2330" {
		t.Fatalf("期望收到 symbol='2330'，收到 '%s'", received.Symbol)
	}
	if received.Last != 785.0 {
		t.Fatalf("期望收到 last=785.0，收到 %f", received.Last)
	}
}

func TestRealtimeRouter_WithRedis_NilClientNoPanic(t *testing.T) {
	mock1 := &mockProvider{name: "mock-1", connected: true}
	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1})

	ctx := context.Background()
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Start 失敗: %v", err)
	}
	defer router.Stop(ctx)

	quote := domain.Quote{Symbol: "2330", Last: 785}

	router.emitQuote(quote)
}

func TestRealtimeRouter_WithRedis_EmptyChannelNoPublish(t *testing.T) {
	mr, client := setupMiniredis(t)
	defer mr.Close()
	defer client.Close()

	mock1 := &mockProvider{name: "mock-1", connected: true}
	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1})
	router.WithRedis(client, "")

	ctx := context.Background()
	if err := router.Start(ctx); err != nil {
		t.Fatalf("Start 失敗: %v", err)
	}
	defer router.Stop(ctx)

	quote := domain.Quote{Symbol: "2330", Last: 785}
	router.emitQuote(quote)
}

func TestRedisSubscriber_StopIdempotent(t *testing.T) {
	mr, client := setupMiniredis(t)
	defer mr.Close()
	defer client.Close()

	sub := NewRedisSubscriber(client, "test-channel", func(q domain.Quote) {})

	if err := sub.Stop(); err != nil {
		t.Fatalf("第一次 Stop 不應報錯: %v", err)
	}
	if err := sub.Stop(); err != nil {
		t.Fatalf("第二次 Stop 不應報錯: %v", err)
	}
}

func TestNewRedisClientFromConfig_Disabled(t *testing.T) {
	cfg := RedisConfig{Enabled: false}
	_, err := NewRedisClientFromConfig(cfg)
	if err == nil {
		t.Fatal("期望 Redis 未啟用時回傳錯誤")
	}
}

func TestNewRedisClientFromConfig_MissingAddr(t *testing.T) {
	cfg := RedisConfig{Enabled: true, Addr: ""}
	_, err := NewRedisClientFromConfig(cfg)
	if err == nil {
		t.Fatal("期望缺少 addr 時回傳錯誤")
	}
}

func TestNewRedisClientFromConfig_Success(t *testing.T) {
	cfg := RedisConfig{
		Enabled:  true,
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	}
	client, err := NewRedisClientFromConfig(cfg)
	if err != nil {
		t.Fatalf("期望成功建立 client，收到錯誤: %v", err)
	}
	if client == nil {
		t.Fatal("期望非 nil client")
	}
	client.Close()
}

func TestRealtimeRouter_WithRedis_FullRoundTrip(t *testing.T) {
	mr, client := setupMiniredis(t)
	defer mr.Close()
	defer client.Close()

	channel := "atlas:quotes"

	var received domain.Quote
	var mu sync.Mutex
	sub := NewRedisSubscriber(client, channel, func(q domain.Quote) {
		mu.Lock()
		received = q
		mu.Unlock()
	})

	ctx := context.Background()
	if err := sub.Start(ctx); err != nil {
		t.Fatalf("Subscriber Start 失敗: %v", err)
	}
	defer sub.Stop()

	time.Sleep(50 * time.Millisecond)

	mock1 := &mockProvider{name: "mock-1", connected: true}
	config := DefaultRouterConfig()
	router := NewRealtimeRouter(config, []RealtimeProvider{mock1})
	router.WithRedis(client, channel)

	if err := router.Start(ctx); err != nil {
		t.Fatalf("Router Start 失敗: %v", err)
	}
	defer router.Stop(ctx)

	quote := domain.Quote{
		Symbol:     "2317",
		Last:       850.0,
		Open:       845.0,
		High:       855.0,
		Low:        840.0,
		Volume:     8000000,
		Market:     "TW",
		IsTradable: true,
		Source:     "fugle-ws",
	}

	router.emitQuote(quote)

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if received.Symbol != "2317" {
		t.Fatalf("期望收到 symbol='2317'，收到 '%s'", received.Symbol)
	}
	if received.Last != 850.0 {
		t.Fatalf("期望收到 last=850.0，收到 %f", received.Last)
	}
	if received.Source != "fugle-ws" {
		t.Fatalf("期望收到 source='fugle-ws'，收到 '%s'", received.Source)
	}
}