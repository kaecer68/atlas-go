package orchestrator

import (
	"fmt"
	"sync"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type convictionBuilder struct {
	mu    sync.RWMutex
	base  int
	floor int
	final int
	steps []domain.ConvictionStep
}

func newConvictionBuilder(base, floor int) *convictionBuilder {
	return &convictionBuilder{base: base, floor: floor, final: base}
}

func (b *convictionBuilder) add(rule string, delta int, reason string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.final += delta
	b.steps = append(b.steps, domain.ConvictionStep{Rule: rule, Delta: delta, Reason: reason})
}

// addWithProvenance is a future replacement for add() that includes parameter
// provenance fields. Not yet called — existing callers use b.add().
// When migrated, switch to this method for config traceability.
//
//lint:ignore U1000 utility for future provenance-aware callers
func (b *convictionBuilder) addWithProvenance(rule string, delta int, reason string, source string, paramRef string, paramValue string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.final += delta
	b.steps = append(b.steps, domain.ConvictionStep{
		Rule:       rule,
		Delta:      delta,
		Reason:     reason,
		Source:     source,
		ParamRef:   paramRef,
		ParamValue: paramValue,
	})
}

func (b *convictionBuilder) cycleAdjustment(modulator *IndustryCycleModulator, industryID string) {
	if modulator == nil {
		return
	}
	card := modulator.GetCycleCard()
	if card == nil {
		return
	}
	conf := modulator.CycleConfidenceFromCard(industryID)
	if conf <= 0.5 {
		return
	}
	delta := int((conf - 0.5) * 20)
	if delta > 10 {
		delta = 10
	}
	if delta > 0 {
		b.addWithProvenance(
			"modulator:cycle_status_card:cycle_adjustment",
			delta,
			fmt.Sprintf("週期綜合情緒 %s (%.3f) - %s 產業信心%.0f%%",
				card.SentimentLabel, card.CompositeCoefficient, industryID, conf*100),
			"CycleStatusCard",
			"industry.CycleStatusCard.CompositeCoefficient",
			fmt.Sprintf("%.3f", card.CompositeCoefficient),
		)
	}
}

func (b *convictionBuilder) cap(max int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.final > max {
		delta := max - b.final
		b.final = max
		b.steps = append(b.steps, domain.ConvictionStep{Rule: "cap", Delta: delta, Reason: fmt.Sprintf("capped at %d", max)})
	}
}

func (b *convictionBuilder) floorCheck() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.final >= b.floor
}

func (b *convictionBuilder) build() (int, *domain.ConvictionBreakdown) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	stepsCopy := make([]domain.ConvictionStep, len(b.steps))
	copy(stepsCopy, b.steps)
	return b.final, &domain.ConvictionBreakdown{
		Base:  b.base,
		Floor: b.floor,
		Final: b.final,
		Steps: stepsCopy,
	}
}
