package capitalflow

import (
	"math"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ForceExtractor extracts force scores from a MacroDataSnapshot.
type ForceExtractor struct {
	windows map[ForceName]*rollingWindow
}

// NewForceExtractor creates a ForceExtractor with 60-day rolling windows.
func NewForceExtractor() *ForceExtractor {
	wins := make(map[ForceName]*rollingWindow)
	for _, f := range []ForceName{
		ForceForeign, ForceFutures, ForceTSMADR,
		ForceInstitutional, ForceDealer, ForceGovernment, ForceRetail,
	} {
		wins[f] = newRollingWindow(60)
	}
	return &ForceExtractor{windows: wins}
}

// Extract computes force scores from a market data snapshot and updates rolling windows.
func (e *ForceExtractor) Extract(snap marketdata.MacroDataSnapshot) []ForceScore {
	forces := []ForceScore{
		e.extractForeign(snap),
		e.extractFutures(snap),
		e.extractTSMADR(snap),
		e.extractInstitutional(snap),
		e.extractDealer(snap),
		e.extractGovernment(snap),
		e.extractRetail(snap),
	}
	return forces
}

func (e *ForceExtractor) extractForeign(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.ForeignInvestorNet.Value
	return e.score(ForceForeign, raw)
}

func (e *ForceExtractor) extractFutures(snap marketdata.MacroDataSnapshot) ForceScore {
	// Futures open interest net is not currently in MacroDataSnapshot.
	// It comes from a separate TAIFEX provider (internal/marketdata/taifex_provider.go).
	// Placeholder — will be wired when taifex channel is integrated into MacroDataSnapshot.
	_ = snap
	return e.score(ForceFutures, 0.0)
}

func (e *ForceExtractor) extractTSMADR(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.TSMADR.ChangePct
	return e.score(ForceTSMADR, raw)
}

func (e *ForceExtractor) extractInstitutional(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.DomesticFundNet.Value
	return e.score(ForceInstitutional, raw)
}

func (e *ForceExtractor) extractDealer(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.DealerNet.Value
	return e.score(ForceDealer, raw)
}

func (e *ForceExtractor) extractGovernment(snap marketdata.MacroDataSnapshot) ForceScore {
	_ = snap
	return e.score(ForceGovernment, 0.0)
}

func (e *ForceExtractor) extractRetail(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.RetailMarginBalance.ChangePct + snap.RetailShortBalance.ChangePct
	return e.score(ForceRetail, raw)
}

// score computes Z-score and trend label for a raw value.
func (e *ForceExtractor) score(name ForceName, raw float64) ForceScore {
	w := e.windows[name]
	w.push(raw)
	z := w.zScore(raw)
	trend := "neutral"
	if z > 0.5 {
		trend = "bullish"
	} else if z < -0.5 {
		trend = "bearish"
	}
	return ForceScore{
		Force:    name,
		RawValue: raw,
		ZScore:   round(z, 3),
		Trend:    trend,
	}
}

// round rounds a float to the specified decimal places.
func round(v float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10
	}
	return math.Round(v*pow) / pow
}
