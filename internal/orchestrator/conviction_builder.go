package orchestrator

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
)

type convictionBuilder struct {
	base  int
	floor int
	final int
	steps []domain.ConvictionStep
}

func newConvictionBuilder(base, floor int) *convictionBuilder {
	return &convictionBuilder{base: base, floor: floor, final: base}
}

func (b *convictionBuilder) add(rule string, delta int, reason string) {
	b.final += delta
	b.steps = append(b.steps, domain.ConvictionStep{Rule: rule, Delta: delta, Reason: reason})
}

func (b *convictionBuilder) cap(max int) {
	if b.final > max {
		b.add("cap", max-b.final, fmt.Sprintf("capped at %d", max))
	}
}

func (b *convictionBuilder) floorCheck() bool {
	if b.final < b.floor {
		b.add("floor", b.floor-b.final, fmt.Sprintf("below floor %d", b.floor))
		b.final = b.floor
		return false
	}
	return true
}

func (b *convictionBuilder) build() (int, *domain.ConvictionBreakdown) {
	return b.final, &domain.ConvictionBreakdown{
		Base:  b.base,
		Floor: b.floor,
		Final: b.final,
		Steps: b.steps,
	}
}
