package events

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// TestRegisterDashboardBufferSubs_NilBusNoop verifies that passing a nil bus
// does not panic and silently no-ops. This matches the production callsite
// where the helper is invoked unconditionally.
func TestRegisterDashboardBufferSubs_NilBusNoop(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil bus must not panic, got %v", r)
		}
	}()
	RegisterDashboardBufferSubs(nil)
}

// TestRegisterDashboardBufferSubs_AllEventTypesPopulated verifies that all 14
// monitored event types are wired to their respective buffer hooks. Before
// this helper existed, the wiring was duplicated inline in cmd/atlas/main.go
// against the simulation bus only, which meant the live-trading bus (where
// Wave 9 detectors actually publish) had zero buffer subscribers and SSE
// catchup was permanently empty.
func TestRegisterDashboardBufferSubs_AllEventTypesPopulated(t *testing.T) {
	resetNarrativeBuffer()
	resetPromotionBuffer()
	resetHealthAlertBuffer()
	resetRiskGateBuffer()
	resetIndustryCalendarBuffer()
	resetBacktestCompletedBuffer()
	resetCalibrationCompletedBuffer()
	resetTradeSlippageBuffer()
	resetChannelIndividualHealthBuffer()
	resetRegimeChangeConfirmedBuffer()
	resetFactorWeightRegressionBuffer()
	resetDriftDetectedBuffer()
	resetIngestionLagSpikeBuffer()

	bus := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus.Close() }()

	RegisterDashboardBufferSubs(bus)
	// Allow the per-handler goroutines that the bus dispatches into to register.
	time.Sleep(50 * time.Millisecond)

	cases := []struct {
		name    string
		event   eventbus.EventType
		payload map[string]any
		verify  func() int
	}{
		{
			name:    "narrative",
			event:   eventbus.EventNarrative,
			payload: map[string]any{"event_id": "n-1"},
			verify:  func() int { return len(GetBufferedNarrativeEvents()) },
		},
		{
			name:    "promotion",
			event:   eventbus.EventPromotionRecorded,
			payload: map[string]any{"experiment_id": "exp-1"},
			verify:  func() int { return len(GetBufferedPromotionEvents()) },
		},
		{
			name:    "health_alert",
			event:   eventbus.EventHealthAlert,
			payload: map[string]any{"category": "sharpe_trend"},
			verify:  func() int { return len(GetBufferedHealthAlerts()) },
		},
		{
			name:    "risk_gate_rejected",
			event:   eventbus.EventRiskGateRejected,
			payload: map[string]any{"verdict": "BLOCK"},
			verify:  func() int { return len(GetBufferedRiskGateEvents()) },
		},
		{
			name:    "risk_gate_allowed",
			event:   eventbus.EventRiskGateAllowed,
			payload: map[string]any{"verdict": "ALLOW"},
			verify:  func() int { return len(GetBufferedRiskGateEvents()) },
		},
		{
			name:    "risk_gate_overridden",
			event:   eventbus.EventRiskGateOverridden,
			payload: map[string]any{"verdict": "REDUCE"},
			verify:  func() int { return len(GetBufferedRiskGateEvents()) },
		},
		{
			name:    "industry_calendar",
			event:   eventbus.EventIndustryCalendar,
			payload: map[string]any{"event_id": "ic-1"},
			verify:  func() int { return len(GetBufferedIndustryCalendarEvents()) },
		},
		{
			name:    "backtest_completed",
			event:   eventbus.EventBacktestCompleted,
			payload: map[string]any{"window_id": "bt-1"},
			verify:  func() int { return len(GetBufferedBacktestCompletedEvents()) },
		},
		{
			name:    "calibration_completed",
			event:   eventbus.EventCalibrationCompleted,
			payload: map[string]any{"module": "linkage"},
			verify:  func() int { return len(GetBufferedCalibrationCompletedEvents()) },
		},
		{
			name:    "trade_slippage",
			event:   eventbus.EventTradeSlippage,
			payload: map[string]any{"order_id": "ord-1"},
			verify:  func() int { return len(GetBufferedTradeSlippageEvents()) },
		},
		{
			name:    "channel_individual_health",
			event:   eventbus.EventChannelIndividualHealth,
			payload: map[string]any{"channel_id": "spx"},
			verify:  func() int { return len(GetBufferedChannelIndividualHealthEvents()) },
		},
		{
			name:    "regime_change_confirmed",
			event:   eventbus.EventRegimeChangeConfirmed,
			payload: map[string]any{"new_regime": "BULL"},
			verify:  func() int { return len(GetBufferedRegimeChangeConfirmedEvents()) },
		},
		{
			name:    "factor_weight_regression",
			event:   eventbus.EventFactorWeightRegression,
			payload: map[string]any{"regime": "BULL"},
			verify:  func() int { return len(GetBufferedFactorWeightRegressionEvents()) },
		},
		{
			name:    "drift_detected",
			event:   eventbus.EventDriftDetected,
			payload: map[string]any{"drift_type": "concentration"},
			verify:  func() int { return len(GetBufferedDriftDetectedEvents()) },
		},
		{
			name:    "ingestion_lag_spike",
			event:   eventbus.EventIngestionLagSpike,
			payload: map[string]any{"lag_seconds": 10.0},
			verify:  func() int { return len(GetBufferedIngestionLagSpikeEvents()) },
		},
	}

	// RiskGate subs share the same buffer; reset and snapshot before/after per case.
	riskGateInitial := len(GetBufferedRiskGateEvents())

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.verify()
			bus.Publish(eventbus.BusEvent{
				Type:    tc.event,
				Payload: tc.payload,
			})
			// Allow the bus dispatcher to invoke the buffer hook.
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if tc.verify() > before {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Errorf("event %s did not populate its buffer within timeout (before=%d, after=%d)",
				tc.event, before, tc.verify())
		})
	}

	// Sanity: at least one of the three RiskGate variants incremented the shared buffer.
	if got := len(GetBufferedRiskGateEvents()); got <= riskGateInitial {
		t.Errorf("expected risk-gate buffer to grow after 3 sub-cases, before=%d after=%d",
			riskGateInitial, got)
	}
}

// TestRegisterDashboardBufferSubs_SecondCallAlsoPopulates verifies that
// registering the helper twice (against the same or different bus instances)
// is safe. The cmd/atlas wiring path calls the helper in both `run` and
// `runLiveTrading`, so a duplicate registration from those callsites must
// not panic or leak subscriptions.
func TestRegisterDashboardBufferSubs_SecondCallAlsoPopulates(t *testing.T) {
	resetRegimeChangeConfirmedBuffer()
	resetDriftDetectedBuffer()

	bus1 := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus1.Close() }()
	bus2 := eventbus.NewChannelEventBus(256)
	defer func() { _ = bus2.Close() }()

	RegisterDashboardBufferSubs(bus1)
	RegisterDashboardBufferSubs(bus2)
	time.Sleep(50 * time.Millisecond)

	bus1.Publish(eventbus.BusEvent{
		Type:    eventbus.EventRegimeChangeConfirmed,
		Payload: map[string]any{"new_regime": "A"},
	})
	bus2.Publish(eventbus.BusEvent{
		Type:    eventbus.EventDriftDetected,
		Payload: map[string]any{"drift_type": "turnover"},
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(GetBufferedRegimeChangeConfirmedEvents()) > 0 && len(GetBufferedDriftDetectedEvents()) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("second-bus events did not reach the shared buffer (regime=%d, drift=%d)",
		len(GetBufferedRegimeChangeConfirmedEvents()), len(GetBufferedDriftDetectedEvents()))
}
