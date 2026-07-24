package events

import (
	"context"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// RegisterDashboardBufferSubs subscribes the 16 dashboard buffer hooks to the
// given bus so that SSE reconnecting clients can receive a catchup of recent
// events for each monitored event type.
//
// The catchup buffer lives in this package, but the *bus* that feeds it is
// owned by the caller (typically the top-level `run` and `runLiveTrading`
// entry points in cmd/atlas/main.go). Callers must invoke this function on
// the SAME bus that the SSE handler is subscribed to — otherwise published
// events go to the live bus while the buffer subscribes to a different one
// and the buffer stays empty forever. The cmd/atlas wiring calls this once
// per run-mode to guarantee parity with the bus passed to
// DashboardAPI.SetEventBus.
//
// The parameter is the eventbus.EventBus interface (not the concrete
// *ChannelEventBus) so the helper is reusable for any bus implementation
// that satisfies the interface, matching the rest of the codebase (e.g.
// Wave9Observability).
func RegisterDashboardBufferSubs(bus eventbus.EventBus) {
	if bus == nil {
		return
	}
	bus.Subscribe(eventbus.EventNarrative, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferNarrativeEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventPromotionRecorded, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferPromotionRecordedEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventHealthAlert, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferHealthAlertEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventRiskGateRejected, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferRiskGateEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventRiskGateAllowed, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferRiskGateEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventRiskGateOverridden, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferRiskGateEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventIndustryCalendar, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferIndustryCalendarEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventBacktestCompleted, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferBacktestCompletedEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventCalibrationCompleted, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferCalibrationCompletedEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventTradeSlippage, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferTradeSlippageEvent(ev)
		return nil
	})
	// Wave 9 yellow observability outputs (per internal/monitoring/AGENTS.md).
	bus.Subscribe(eventbus.EventChannelIndividualHealth, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferChannelIndividualHealthEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventRegimeChangeConfirmed, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferRegimeChangeConfirmedEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventFactorWeightRegression, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferFactorWeightRegressionEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventDriftDetected, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferDriftDetectedEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventIngestionLagSpike, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferIngestionLagSpikeEvent(ev)
		return nil
	})
	bus.Subscribe(eventbus.EventAgentHealthChange, func(_ context.Context, ev eventbus.BusEvent) error {
		BufferAgentHealthChangeEvent(ev)
		return nil
	})
}
