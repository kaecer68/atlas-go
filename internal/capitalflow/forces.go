package capitalflow

import (
	"math"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ForceExtractor extracts force scores from a MacroDataSnapshot.
//
// Per docs/reference/product-positioning.md §7 (manifest #E05), the taxonomy
// is:
//
//   - 5 subjects (participate in resonance): foreign, institutional, dealer,
//     government, retail.
//   - 1 leading_indicator: foreign futures OI — surfaces on the foreign
//     ForceScore via LeadingZ / LeadingTrend; also exposed as a deprecated
//     "futures" score for back-compat.
//   - 1 sentiment: TSM ADR — exposed as a deprecated score; never enters
//     resonance.
type ForceExtractor struct {
	windows map[ForceName]*rollingWindow
	// Foreign uses two windows: spot (現貨買賣超) and futures (台指期未平倉).
	// The spot window feeds ZScore; the futures window feeds LeadingZ.
	foreignSpotWindow    *rollingWindow
	foreignFuturesWindow *rollingWindow
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
	return &ForceExtractor{
		windows:              wins,
		foreignSpotWindow:    newRollingWindow(60),
		foreignFuturesWindow: newRollingWindow(60),
	}
}

// Extract computes force scores from a market data snapshot and updates
// rolling windows. Returns 7 ForceScore entries for backward compatibility:
// 5 subjects + 1 leading_indicator (futures, deprecated) + 1 sentiment (TSM
// ADR, deprecated). Only the 5 subjects participate in resonance.
func (e *ForceExtractor) Extract(snap marketdata.MacroDataSnapshot) []ForceScore {
	forces := make([]ForceScore, 0, 7)
	forces = append(forces, e.extractForeign(snap))
	forces = append(forces, e.extractFuturesDeprecated(snap))
	forces = append(forces, e.extractTSMADR(snap))
	forces = append(forces, e.extractInstitutional(snap))
	forces = append(forces, e.extractDealer(snap))
	forces = append(forces, e.extractGovernment(snap))
	forces = append(forces, e.extractRetail(snap))
	return forces
}

func (e *ForceExtractor) extractForeign(snap marketdata.MacroDataSnapshot) ForceScore {
	spotRaw := snap.ForeignInvestorNet.Value
	spotZ := round(zScore(e.windows[ForceForeign], spotRaw), 3)
	spotTrend := trendFor(spotZ)

	// Leading signal from TAIFEX 三大法人期貨 OI (manifest #E01).
	// When the channel has no data (Symbol empty), the leading fields stay
	// at zero and trend is neutral.
	var leadingZ float64
	var leadingTrend string
	if snap.ForeignFuturesOINet.Symbol != "" {
		futRaw := snap.ForeignFuturesOINet.Value
		leadingZ = round(zScore(e.foreignFuturesWindow, futRaw), 3)
		leadingTrend = trendFor(leadingZ)
	} else {
		leadingTrend = "neutral"
	}

	return ForceScore{
		Force:        ForceForeign,
		Role:         ForceRoleSubject,
		RawValue:     spotRaw,
		ZScore:       spotZ,
		Trend:        spotTrend,
		LeadingZ:     leadingZ,
		LeadingTrend: leadingTrend,
	}
}

// extractFuturesDeprecated preserves the legacy "futures" ForceScore entry
// for API back-compat. It uses the same data source as the foreign
// LeadingZ, so consumers reading either get the same numbers. Marked
// deprecated + leading_indicator; not a resonance subject.
func (e *ForceExtractor) extractFuturesDeprecated(snap marketdata.MacroDataSnapshot) ForceScore {
	var raw float64
	if snap.ForeignFuturesOINet.Symbol != "" {
		raw = snap.ForeignFuturesOINet.Value
	}
	z := round(zScore(e.windows[ForceFutures], raw), 3)
	return ForceScore{
		Force:      ForceFutures,
		Role:       ForceRoleLeadingIndicator,
		Deprecated: true,
		RawValue:   raw,
		ZScore:     z,
		Trend:      trendFor(z),
	}
}

func (e *ForceExtractor) extractTSMADR(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.TSMADR.ChangePct
	z := round(zScore(e.windows[ForceTSMADR], raw), 3)
	return ForceScore{
		Force:      ForceTSMADR,
		Role:       ForceRoleSentiment,
		Deprecated: true,
		RawValue:   raw,
		ZScore:     z,
		Trend:      trendFor(z),
	}
}

func (e *ForceExtractor) extractInstitutional(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.DomesticFundNet.Value
	return e.scoreSubject(ForceInstitutional, raw)
}

func (e *ForceExtractor) extractDealer(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.DealerNet.Value
	return e.scoreSubject(ForceDealer, raw)
}

// extractGovernment reads the E04 官股行庫 reading from the snapshot.
// When no data is present (Symbol empty), the score stays neutral with
// DataAvailable=false so the resonance model can distinguish "no data"
// from "data says neutral".
func (e *ForceExtractor) extractGovernment(snap marketdata.MacroDataSnapshot) ForceScore {
	if snap.GovernmentNet.Symbol == "" {
		w := e.windows[ForceGovernment]
		w.push(0)
		return ForceScore{
			Force:         ForceGovernment,
			Role:          ForceRoleSubject,
			RawValue:      0,
			ZScore:        0,
			Trend:         "neutral",
			DataAvailable: false,
		}
	}
	raw := snap.GovernmentNet.Value
	z := round(zScore(e.windows[ForceGovernment], raw), 3)
	return ForceScore{
		Force:         ForceGovernment,
		Role:          ForceRoleSubject,
		RawValue:      raw,
		ZScore:        z,
		Trend:         trendFor(z),
		DataAvailable: true,
	}
}

func (e *ForceExtractor) extractRetail(snap marketdata.MacroDataSnapshot) ForceScore {
	raw := snap.RetailMarginBalance.ChangePct + snap.RetailShortBalance.ChangePct
	return e.scoreSubject(ForceRetail, raw)
}

func (e *ForceExtractor) scoreSubject(name ForceName, raw float64) ForceScore {
	z := round(zScore(e.windows[name], raw), 3)
	return ForceScore{
		Force:    name,
		Role:     ForceRoleSubject,
		RawValue: raw,
		ZScore:   z,
		Trend:    trendFor(z),
	}
}

// zScore pushes the raw value into the window then returns its Z-score.
// The window maintains 60-day history so Z normalizes against recent
// context (not process-lifetime: restarts reset the window — known
// limitation tracked as BK-15).
func zScore(w *rollingWindow, raw float64) float64 {
	w.push(raw)
	return w.zScore(raw)
}

func trendFor(z float64) string {
	switch {
	case z > 0.5:
		return "bullish"
	case z < -0.5:
		return "bearish"
	default:
		return "neutral"
	}
}

func round(v float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10
	}
	return math.Round(v*pow) / pow
}
