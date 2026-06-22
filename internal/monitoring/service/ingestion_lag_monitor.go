package service

import (
	"context"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const (
	IngestionLagP99Threshold   = 5 * time.Second
	IngestionLagCheckInterval  = 30 * time.Second
	IngestionLagDedupWindow    = 60 * time.Second
	IngestionLagEventSchemaVer = 1
)

type IngestionLagProvider interface {
	P99LatencySeconds() float64
}

type IngestionLagMonitor interface {
	Start(ctx context.Context) error
	Stop() error
}

type ingestionLagMonitor struct {
	bus      eventbus.EventBus
	provider IngestionLagProvider
	interval time.Duration

	mu          sync.Mutex
	lastEmitAt  time.Time
	hasLastEmit bool

	cancel context.CancelFunc
	done   chan struct{}
}

func NewIngestionLagMonitor(bus eventbus.EventBus, provider IngestionLagProvider) IngestionLagMonitor {
	return &ingestionLagMonitor{
		bus:      bus,
		provider: provider,
		interval: IngestionLagCheckInterval,
		done:     make(chan struct{}),
	}
}

func (m *ingestionLagMonitor) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	go m.run(runCtx)
	return nil
}

func (m *ingestionLagMonitor) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	<-m.done
	return nil
}

func (m *ingestionLagMonitor) run(ctx context.Context) {
	defer close(m.done)
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case t := <-ticker.C:
			m.check(t)
		}
	}
}

func (m *ingestionLagMonitor) check(now time.Time) {
	if m.provider == nil {
		return
	}
	p99 := m.provider.P99LatencySeconds()
	if p99 < IngestionLagP99Threshold.Seconds() {
		return
	}

	m.mu.Lock()
	if m.hasLastEmit && now.Sub(m.lastEmitAt) < IngestionLagDedupWindow {
		m.mu.Unlock()
		return
	}
	m.lastEmitAt = now
	m.hasLastEmit = true
	m.mu.Unlock()

	m.bus.Publish(eventbus.BusEvent{
		Type:          eventbus.EventIngestionLagSpike,
		Timestamp:     now,
		Severity:      "warning",
		SchemaVersion: IngestionLagEventSchemaVer,
		Payload: map[string]any{
			"p99_latency_seconds": p99,
			"threshold_seconds":   IngestionLagP99Threshold.Seconds(),
			"detected_at":         now,
		},
	})
}
