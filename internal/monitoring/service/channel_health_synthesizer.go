package service

import (
	"time"

	"github.com/kaecer68/atlas-go/internal/eventbus"
)

const ChannelHealthDedupWindow = 5 * time.Second

type ChannelHealthSynthesizer interface {
	Start() error
	Stop() error
}

type channelHealthSynthesizer struct {
	bus eventbus.EventBus
}

func NewChannelHealthSynthesizer(bus eventbus.EventBus) ChannelHealthSynthesizer {
	return &channelHealthSynthesizer{bus: bus}
}

func (c *channelHealthSynthesizer) Start() error { return nil }
func (c *channelHealthSynthesizer) Stop() error  { return nil }
