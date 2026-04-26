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
