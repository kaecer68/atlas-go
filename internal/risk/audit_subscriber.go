package risk

import (
	"context"
	"fmt"

	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type AuditSubscriber struct{ bus *eventbus.ChannelEventBus }

func NewAuditSubscriber(bus *eventbus.ChannelEventBus) *AuditSubscriber {
	a := &AuditSubscriber{bus: bus}
	bus.Subscribe(eventbus.EventStopLossTriggered, a.log)
	bus.Subscribe(eventbus.EventTakeProfitTriggered, a.log)
	bus.Subscribe(eventbus.EventRiskAlert, a.log)
	bus.Subscribe(eventbus.EventOrderFilled, a.log)
	bus.Subscribe(eventbus.EventOrderRejected, a.log)
	bus.Subscribe(eventbus.EventOrderPlaced, a.log)
	return a
}

func (a *AuditSubscriber) log(_ context.Context, ev eventbus.BusEvent) error {
	logging.Info("risk_audit", "risk_event",
		logging.FStr("event_type", string(ev.Type)),
		logging.FStr("event_id", ev.ID),
		logging.FStr("payload", fmt.Sprintf("%+v", ev.Payload)),
	)
	return nil
}
