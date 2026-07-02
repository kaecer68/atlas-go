package strategy

import (
	"sync"

	"github.com/kaecer68/atlas-go/internal/forecast"
)

type DirectionalTradeLayer struct {
	mu              sync.RWMutex
	directionWeight map[string]float64
}

func NewDirectionalTradeLayer() *DirectionalTradeLayer {
	return &DirectionalTradeLayer{directionWeight: make(map[string]float64)}
}

func (l *DirectionalTradeLayer) ApplySignal(signal forecast.TradeSignal) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.directionWeight[signal.Symbol] = signal.WeightMultiplier
}

func (l *DirectionalTradeLayer) WeightFor(symbol string) float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if w, ok := l.directionWeight[symbol]; ok {
		return w
	}
	return 1.0
}

func (l *DirectionalTradeLayer) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.directionWeight = make(map[string]float64)
}
