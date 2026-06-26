package experiment

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/domain/experiment"
	"github.com/kaecer68/atlas-go/internal/eventbus"
)

// LifecyclePublisher wraps domain.TransitionExperimentStatus with event
// publishing. Decision 5 (alert-redesign-v2.md Part 3.6) reverses the
// original Decision 5: instead of dropping experiment lifecycle to log,
// it publishes EventExperimentAccepted (info) or EventExperimentRejected
// (error) so baseline promotion/rejection becomes visible on the
// dashboard.
//
// Why a wrapper rather than modifying TransitionExperimentStatus itself:
// per internal/domain/AGENTS.md the domain package is pure types with
// "no business logic, no coordination logic, no I/O". Event publishing
// is I/O, so it lives in the business-logic layer (internal/experiment/).
type LifecyclePublisher struct {
	bus *eventbus.ChannelEventBus
}

func NewLifecyclePublisher(bus *eventbus.ChannelEventBus) *LifecyclePublisher {
	return &LifecyclePublisher{bus: bus}
}

// TransitionAndPublish performs the state-machine transition and then
// publishes the corresponding EventExperiment{Accepted,Rejected} event.
// Returns the transition error if invalid (e.g. Accepted → Running).
// nil bus is allowed (transition still happens; publish becomes no-op).
func (p *LifecyclePublisher) TransitionAndPublish(
	record *experiment.ExperimentRecord,
	next experiment.ExperimentStatus,
) error {
	if err := experiment.TransitionExperimentStatus(record, next); err != nil {
		return err
	}
	if p.bus == nil {
		return nil
	}
	payload := eventbus.ExperimentLifecyclePayload{
		ExperimentID:  record.ID,
		ProposalID:    record.ProposalID,
		TargetAgentID: record.TargetAgentID,
		Skill:         record.Skill,
		RevertReason:  record.RevertReason,
		Timestamp:     time.Now(),
	}
	switch next {
	case experiment.ExperimentAccepted:
		p.bus.PublishExperimentLifecycle(eventbus.EventExperimentAccepted, payload, "info")
	case experiment.ExperimentRejected:
		p.bus.PublishExperimentLifecycle(eventbus.EventExperimentRejected, payload, "error")
	}
	return nil
}
