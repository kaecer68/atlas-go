package orchestrator

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// PR3 — narrative integration extracted from system.go (Issue #611 sub-issue-4):
//   - detectNarrativeEvents: scan quotes for narrative signals via NarrativeEngine.
//   - applyNarrativeContextWithEvents: adjust recommendation conviction using
//     detected narrative events.
//   - QuotesToNarrativeData: pure data-transformation helper (no System receiver).
//   - applyAlphaDiscovery: surface alpha-discovered tickers into finalRecs.
//
// These bridge raw simulation quotes with narrative/alpha enrichment layers.

func (s *System) detectNarrativeEvents(quotes []domain.Quote) []narrative.NarrativeEvent {
	if s.narrativeEngine == nil {
		return nil
	}
	data := QuotesToNarrativeData(quotes)
	return s.narrativeEngine.DetectEvents(data)
}

func (s *System) applyNarrativeContextWithEvents(recs []domain.Recommendation, events []narrative.NarrativeEvent) []domain.Recommendation {
	if s.narrativeEngine == nil || len(events) == 0 {
		return recs
	}
	chains := s.narrativeEngine.MatchChains(events)

	enriched := make([]domain.Recommendation, len(recs))
	for i, rec := range recs {
		enriched[i] = rec
		// Attach narrative context to every recommendation layer so the
		// pipeline UI and alerts can show why a rec was made.
		enriched[i].SupportingEvents = make([]string, len(events))
		for j, e := range events {
			enriched[i].SupportingEvents[j] = e.ID
		}
		enriched[i].ReasoningChain = []string{}
		for _, e := range events {
			enriched[i].ReasoningChain = append(enriched[i].ReasoningChain, fmt.Sprintf("%s (%s, confidence %.2f)", e.Theme, e.Region, e.Confidence))
		}
		for _, c := range chains {
			if len(c.Steps) > 0 {
				enriched[i].ReasoningChain = append(enriched[i].ReasoningChain, fmt.Sprintf("Chain %s: %s", c.TemplateID, c.Steps[0].Description))
			}
		}
		if enriched[i].Reason != "" {
			enriched[i].Reason = fmt.Sprintf("%s | Narrative: %d event(s)", enriched[i].Reason, len(events))
		}
	}
	return enriched
}

// quoteChangePct returns the intraday change of q in percent and reports
// whether the observation may be used (k3 decision 3-3).
//
// The divisor is the only guarded input: a non-positive or non-finite Open
// cannot divide, and the raw expression (Last-Open)/Open yields +-Inf or NaN,
// which then reaches the narrative detectors and the macro-flow engine as if it
// were a real session move. Such a quote is a MISSING observation, so the field
// keeps its zero value and the skip is logged for audit. Normal quotes take the
// same arithmetic as before, bit for bit.
func quoteChangePct(component string, q domain.Quote) (float64, bool) {
	if !shared.IsUsablePrice(q.Open) {
		logging.Warn(component, "quote_change_skipped",
			"reason", "unusable_open", "symbol", q.Symbol, "open", q.Open, "last", q.Last)
		return 0, false
	}
	return (q.Last - q.Open) / q.Open * 100, true
}

func QuotesToNarrativeData(quotes []domain.Quote) narrative.MarketNarrativeData {
	data := narrative.MarketNarrativeData{}
	const component = "orchestrator.quotes_to_narrative"
	for _, q := range quotes {
		switch q.Symbol {
		case "DXY", "^DXY":
			if pct, ok := quoteChangePct(component, q); ok {
				data.DXYChangePct = pct
			}
		case "US10Y", "^TNX":
			data.US10YChangeBps = q.Last
		case "VIX", "^VIX":
			data.VIXLevel = q.Last
		case "OIL", "CL=F":
			if pct, ok := quoteChangePct(component, q); ok {
				data.OilChangePct = pct
			}
		case "GOLD", "GC=F":
			if pct, ok := quoteChangePct(component, q); ok {
				data.GoldChangePct = pct
			}
		case "JPY=X", "USDJPY=X":
			if pct, ok := quoteChangePct(component, q); ok {
				data.JPY_ChangePct = pct
			}
		case "SPX", "^GSPC":
			if pct, ok := quoteChangePct(component, q); ok {
				data.SPXIndexChangePct = pct
			}
		case "NDX", "^IXIC":
			if pct, ok := quoteChangePct(component, q); ok {
				data.NDXIndexChangePct = pct
			}
		case "DJI", "^DJI":
			if pct, ok := quoteChangePct(component, q); ok {
				data.DJIIndexChangePct = pct
			}
		case "SOX", "^SOX":
			if pct, ok := quoteChangePct(component, q); ok {
				data.SOXIndexChangePct = pct
			}
		case "BDI":
			if pct, ok := quoteChangePct(component, q); ok {
				data.BDIChangePct = pct
			}
		case "COPPER", "HG=F":
			if pct, ok := quoteChangePct(component, q); ok {
				data.CopperChangePct = pct
			}
		case "TSM", "TSMADR":
			if pct, ok := quoteChangePct(component, q); ok {
				data.TSMADRChangePct = pct
			}
		}
	}
	return data
}

func (s *System) applyAlphaDiscovery(quotes []domain.Quote, recs []domain.Recommendation) []domain.Recommendation {
	if s.Port().alphaDiscovery == nil {
		return nil
	}
	symbols := RegistrySymbols(s.Sim().registry)
	quoteMap := make(map[string]domain.Quote, len(quotes))
	for _, q := range quotes {
		quoteMap[q.Symbol] = q
	}
	return s.Port().alphaDiscovery.Discover(s.Sim().ctx, symbols, quoteMap, recs)
}
