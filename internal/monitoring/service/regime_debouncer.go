package service

import (
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const RegimeStabilityWindow = 30 * time.Second

type RegimeDebouncer interface {
	Start() error
	Stop() error
}

type regimeDebouncer struct {
	bus          eventbus.EventBus
	mu           sync.Mutex
	lastRegime   string
	lastChangeAt time.Time
}

func NewRegimeDebouncer(bus eventbus.EventBus) RegimeDebouncer {
	return &regimeDebouncer{bus: bus}
}

func (d *regimeDebouncer) Start() error { return nil }
func (d *regimeDebouncer) Stop() error  { return nil }
