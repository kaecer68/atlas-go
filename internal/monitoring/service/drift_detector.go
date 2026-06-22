package service

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const (
	DriftMaxConcentrationThreshold = 0.25
	DriftTurnoverThreshold         = 0.15
	DriftCheckInterval             = 5 * time.Minute
)

type DriftDetector interface {
	Start() error
	Stop() error
}

type driftDetector struct {
	bus eventbus.EventBus
}

func NewDriftDetector(bus eventbus.EventBus) DriftDetector {
	return &driftDetector{bus: bus}
}

func (d *driftDetector) Start() error { return nil }
func (d *driftDetector) Stop() error  { return nil }
