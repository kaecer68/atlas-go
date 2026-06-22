package service

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const (
	ChannelHealthDedupWindow    = 5 * time.Second
	ChannelHealthPollInterval   = 30 * time.Second
	ChannelHealthEventSchemaVer = 1
)

type ChannelHealthProvider interface {
	ChannelErrors() map[string]string
}

type ChannelHealthSynthesizer interface {
	Start(ctx context.Context) error
	Stop() error
}

type channelHealthSynthesizer struct {
	bus      eventbus.EventBus
	provider ChannelHealthProvider
	interval time.Duration

	mu       sync.Mutex
	lastSeen map[string]time.Time

	cancel context.CancelFunc
	done   chan struct{}
}

func NewChannelHealthSynthesizer(bus eventbus.EventBus, provider ChannelHealthProvider) ChannelHealthSynthesizer {
	return &channelHealthSynthesizer{
		bus:      bus,
		provider: provider,
		interval: ChannelHealthPollInterval,
		lastSeen: make(map[string]time.Time),
		done:     make(chan struct{}),
	}
}

func (c *channelHealthSynthesizer) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.run(runCtx)
	return nil
}

func (c *channelHealthSynthesizer) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	<-c.done
	return nil
}

func (c *channelHealthSynthesizer) run(ctx context.Context) {
	defer close(c.done)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.check(time.Now())
		}
	}
}

func (c *channelHealthSynthesizer) check(now time.Time) {
	if c.provider == nil {
		return
	}
	errors := c.provider.ChannelErrors()
	if len(errors) == 0 {
		return
	}
	for channelID, errMsg := range errors {
		key := channelID + "|" + errMsg
		c.mu.Lock()
		last, seen := c.lastSeen[key]
		if seen && now.Sub(last) < ChannelHealthDedupWindow {
			c.mu.Unlock()
			continue
		}
		c.lastSeen[key] = now
		c.mu.Unlock()

		// 首次出現時 `last` 為 time.Time 零值，避免將 `0001-01-01T00:00:00Z`
		// 寫入 payload；改用 `now` 作為首次偵測時間。code review PR #632 issue #1。
		firstSeen := last
		if !seen {
			firstSeen = now
		}
		c.bus.Publish(eventbus.BusEvent{
			Type:          eventbus.EventChannelIndividualHealth,
			Timestamp:     now,
			Severity:      "info",
			SchemaVersion: ChannelHealthEventSchemaVer,
			Payload: map[string]any{
				"channel_id":    channelID,
				"error_message": errMsg,
				"first_seen_at": firstSeen,
				"detected_at":   now,
			},
		})
	}
}
