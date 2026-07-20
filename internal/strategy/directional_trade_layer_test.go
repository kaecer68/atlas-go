package strategy

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/forecast"
)

func TestDirectionalTradeLayer_New(t *testing.T) {
	l := NewDirectionalTradeLayer()
	if l == nil {
		t.Fatal("NewDirectionalTradeLayer returned nil")
	}
	if l.directionWeight == nil {
		t.Error("directionWeight map not initialized")
	}
}

func TestDirectionalTradeLayer_WeightFor_Default(t *testing.T) {
	l := NewDirectionalTradeLayer()

	w := l.WeightFor("2330.TW")
	if w != 1.0 {
		t.Errorf("WeightFor unknown symbol = %f, want 1.0", w)
	}
}

func TestDirectionalTradeLayer_ApplySignal(t *testing.T) {
	l := NewDirectionalTradeLayer()

	// Apply a bullish signal
	l.ApplySignal(forecast.TradeSignal{Symbol: "2330.TW", WeightMultiplier: 1.5})
	w := l.WeightFor("2330.TW")
	if w != 1.5 {
		t.Errorf("WeightFor after ApplySignal = %f, want 1.5", w)
	}
}

func TestDirectionalTradeLayer_ApplySignalOverride(t *testing.T) {
	l := NewDirectionalTradeLayer()

	l.ApplySignal(forecast.TradeSignal{Symbol: "2330.TW", WeightMultiplier: 1.5})
	l.ApplySignal(forecast.TradeSignal{Symbol: "2330.TW", WeightMultiplier: 0.5})

	w := l.WeightFor("2330.TW")
	if w != 0.5 {
		t.Errorf("WeightFor after override = %f, want 0.5", w)
	}
}

func TestDirectionalTradeLayer_Reset(t *testing.T) {
	l := NewDirectionalTradeLayer()

	l.ApplySignal(forecast.TradeSignal{Symbol: "2330.TW", WeightMultiplier: 1.5})
	l.Reset()

	// After reset, all symbols return to default weight
	w := l.WeightFor("2330.TW")
	if w != 1.0 {
		t.Errorf("WeightFor after Reset = %f, want 1.0", w)
	}
}

func TestDirectionalTradeLayer_ConcurrentSafety(t *testing.T) {
	l := NewDirectionalTradeLayer()

	done := make(chan bool, 2)
	go func() {
		for i := 0; i < 100; i++ {
			l.ApplySignal(forecast.TradeSignal{Symbol: "2330.TW", WeightMultiplier: 1.0})
		}
		done <- true
	}()
	go func() {
		for i := 0; i < 100; i++ {
			l.WeightFor("2330.TW")
		}
		done <- true
	}()

	<-done
	<-done
	// No race condition = pass
}

func TestDirectionalTradeLayer_MultipleSymbols(t *testing.T) {
	l := NewDirectionalTradeLayer()

	symbols := []string{"2330.TW", "2317.TW", "2454.TW", "2412.TW"}
	weights := []float64{1.5, 0.8, 1.2, 0.6}

	for i, sym := range symbols {
		l.ApplySignal(forecast.TradeSignal{Symbol: sym, WeightMultiplier: weights[i]})
	}

	for i, sym := range symbols {
		w := l.WeightFor(sym)
		if w != weights[i] {
			t.Errorf("%s weight = %f, want %f", sym, w, weights[i])
		}
	}
}
