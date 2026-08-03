package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata/realtime"
)

var (
	symbol       = flag.String("symbol", "2330", "stock symbol to subscribe")
	redisAddr    = flag.String("redis", "localhost:6379", "redis address")
	redisChannel = flag.String("channel", "atlas:quotes", "redis pubsub channel")
)

func main() {
	flag.Parse()

	// config.GetSecret reads env then Keychain (envOrKeychain). Fixes the
	// constitution Article 1 violation of reading FUGLE_API_KEY via raw
	// os.Getenv (a5-violations.json:138-140).
	apiKey := config.GetSecret("FUGLE_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "FUGLE_API_KEY not set\n")
		os.Exit(1)
	}

	fmt.Printf("Starting realtime quote stream for %s\n", *symbol)
	fmt.Printf("Redis: %s (channel: %s)\n", *redisAddr, *redisChannel)

	// Setup Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr: *redisAddr,
	})
	defer func() { _ = redisClient.Close() }()

	// Test Redis connection
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Redis connection failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Redis connected")

	// Create provider
	config := realtime.DefaultFugleWSConfig(apiKey)
	provider := realtime.NewFugleWebSocketProvider(config)

	// Create router with Redis
	router := realtime.NewRealtimeRouter(
		realtime.DefaultRouterConfig(),
		[]realtime.RealtimeProvider{provider},
	)
	router.WithRedis(redisClient, *redisChannel)

	// Subscribe to quotes
	quoteCount := 0
	startTime := time.Now()

	router.OnQuote(func(quote domain.Quote) {
		quoteCount++
		elapsed := time.Since(startTime)
		fmt.Printf(
			"[%s] #%d %s @ %.2f (vol: %d) [source: %s]\n",
			elapsed.Round(time.Second),
			quoteCount,
			quote.Symbol,
			quote.Last,
			quote.Volume,
			quote.Source,
		)
	})

	// Start
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := router.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start router: %v\n", err)
		os.Exit(1)
	}

	// Subscribe to symbol
	if err := router.Subscribe([]string{*symbol}); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to subscribe: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Subscribed to %s\n", *symbol)

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to stop")
	<-sigCh

	fmt.Println("\nShutting down...")
	if err := router.Stop(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Stop error: %v\n", err)
	}

	fmt.Printf("Total quotes received: %d\n", quoteCount)
}
