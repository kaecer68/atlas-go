package anomaly

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kaecer68/atlas-go/internal/alerting"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// Test_Integration_anomaly_to_alert_to_eventbus_to_metrics drives the
// full T1.4 pipeline with real components: a real detector, a real
// WebhookPublisher pointed at httptest.Server, a real ChannelEventBus
// with a subscriber, an in-package metrics counter, and a real
// MemoryStore. Triggers a burst, calls Emitter.ProcessOnce, verifies
// the alert webhook + bus + metrics + ack store all move.
func Test_Integration_anomaly_to_alert_to_eventbus_to_metrics(t *testing.T) {
	// 1. Webhook target.
	var webhookHits atomic.Int32
	var webhookMu sync.Mutex
	var webhookPayloads []alerting.AlertmanagerPayload
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		webhookHits.Add(1)
		var p alerting.AlertmanagerPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("decode webhook payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		webhookMu.Lock()
		webhookPayloads = append(webhookPayloads, p)
		webhookMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	// 2. Event bus.
	bus := eventbus.NewChannelEventBus(100)
	defer func() { _ = bus.Close() }()
	var busHits atomic.Int32
	var busMu sync.Mutex
	var busEvents []eventbus.BusEvent
	sub := bus.SubscribeAll(func(_ context.Context, ev eventbus.BusEvent) error {
		if ev.Type == eventbus.EventMCPAnomalyDetected {
			busHits.Add(1)
			busMu.Lock()
			busEvents = append(busEvents, ev)
			busMu.Unlock()
		}
		return nil
	})
	defer sub.Cancel()

	// 3. Metrics (in-package — uses the existing fakeScoreRecorder which
	// captures ScoreRecorder.SetAnomalyScore calls; we add a separate
	// counter for the emitter's AnomalyObserver contract).
	scoreRec := &fakeScoreRecorder{}
	emitCount := &countingMetrics{}

	// 4. Detector with tight burst threshold.
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	detector := NewDetector(Config{
		ShortWindow:          time.Hour,
		LongWindow:           24 * time.Hour,
		BurstZScoreThreshold: 1.0,
	}, scoreRec, nil)
	detector.now = fixedClock(now)
	for range 20 {
		detector.Observe(testAuditEntry{version: 2, ts: now, tool: "mcp_x", tenant: "tenant-a", status: "ok"})
	}

	// 5. Ack store + publisher + emitter.
	ackStore := NewMemoryStore(100)
	pub := alerting.NewWebhookPublisher(alerting.WebhookPublisherConfig{
		URL:         webhookSrv.URL,
		HTTPTimeout: 2 * time.Second,
	})
	em := NewEmitter(EmitterConfig{
		Detector:  detector,
		Publisher: pub,
		AckStore:  ackStore,
		Bus:       bus,
		Observer:  emitCount,
	})

	// 6. Drive the pipeline.
	require.NoError(t, em.ProcessOnce(context.Background()))
	// Bus has a single dispatcher goroutine — give it a moment.
	time.Sleep(50 * time.Millisecond)

	// 7. Webhook assertions.
	require.Equal(t, int32(1), webhookHits.Load())
	webhookMu.Lock()
	require.Len(t, webhookPayloads, 1)
	wp := webhookPayloads[0]
	webhookMu.Unlock()
	require.Equal(t, "4", wp.Version)
	require.Equal(t, "firing", wp.Status)
	require.Equal(t, "atlas-mcp", wp.Receiver)
	require.Len(t, wp.Alerts, 1)
	require.Equal(t, "burst", wp.Alerts[0].Labels["anomaly_type"])
	require.Equal(t, "tenant-a", wp.Alerts[0].Labels["tenant_id"])
	require.NotEmpty(t, wp.Alerts[0].Annotations["anomaly_id"])

	// 8. Event bus assertions.
	require.Equal(t, int32(1), busHits.Load())
	busMu.Lock()
	require.Len(t, busEvents, 1)
	busEvent := busEvents[0]
	busMu.Unlock()
	payload, ok := busEvent.Payload.(eventbus.MCPAnomalyEventPayload)
	require.True(t, ok, "expected MCPAnomalyEventPayload, got %T", busEvents[0].Payload)
	require.Equal(t, "tenant-a", payload.TenantID)
	require.Equal(t, "burst", payload.AnomalyType)
	require.NotEmpty(t, payload.AnomalyID)
	require.NotEmpty(t, payload.DetectedAt)

	// 9. Metrics assertions.
	require.Len(t, scoreRec.calls, 1, "ScoreRecorder.SetAnomalyScore should fire once")
	require.Equal(t, "tenant-a", scoreRec.calls[0].tenantID)
	require.Equal(t, "burst", scoreRec.calls[0].anomalyType)
	require.Len(t, emitCount.Calls, 1, "AnomalyObserver.ObserveAnomaly should fire once")
	require.Equal(t, "tenant-a", emitCount.Calls[0].tenantID)

	// 10. Ack store assertions.
	all := ackStore.ListAll(10)
	require.Len(t, all, 1)
	require.Equal(t, "tenant-a", all[0].Event.TenantID)
	require.Equal(t, "burst", all[0].Event.AnomalyType)
	require.False(t, all[0].Acked)
	require.Equal(t, payload.AnomalyID, all[0].AnomalyID, "AnomalyID on bus == AnomalyID in store")
}

// Test_Integration_idempotency_under_repeated_process verifies the
// emitter does not re-publish on every tick — critical for the
// polling goroutine.
func Test_Integration_idempotency_under_repeated_process(t *testing.T) {
	var hits atomic.Int32
	webhookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer webhookSrv.Close()

	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	detector := NewDetector(Config{
		ShortWindow:          time.Hour,
		LongWindow:           24 * time.Hour,
		BurstZScoreThreshold: 1.0,
	}, &fakeScoreRecorder{}, nil)
	detector.now = fixedClock(now)
	for range 20 {
		detector.Observe(testAuditEntry{version: 2, ts: now, tool: "mcp_x", tenant: "tenant-a", status: "ok"})
	}

	em := NewEmitter(EmitterConfig{
		Detector:  detector,
		Publisher: alerting.NewWebhookPublisher(alerting.WebhookPublisherConfig{URL: webhookSrv.URL, HTTPTimeout: time.Second}),
		AckStore:  NewMemoryStore(100),
	})
	for range 5 {
		require.NoError(t, em.ProcessOnce(context.Background()))
	}
	require.Equal(t, int32(1), hits.Load(), "expected exactly 1 webhook hit across 5 ticks")
}

// Test_Integration_acked_anomaly_still_in_audit_but_excluded_from_unacked
// verifies the operator-dashboard contract: ack keeps the audit row but
// removes it from the unacked list.
func Test_Integration_acked_anomaly_still_in_audit_but_excluded_from_unacked(t *testing.T) {
	store := NewMemoryStore(10)
	sa, err := store.Save(AnomalyEvent{TenantID: "t", AnomalyType: "burst"})
	require.NoError(t, err)
	require.Len(t, store.ListUnacked(10), 1)
	require.Len(t, store.ListAll(10), 1)

	require.NoError(t, store.Ack(sa.AnomalyID, "operator-7"))
	require.Empty(t, store.ListUnacked(10))
	require.Len(t, store.ListAll(10), 1)
}
