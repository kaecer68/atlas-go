package orchestrator

import (
	"fmt"

	"github.com/kaecer68/atlas-go/internal/domain"
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

func QuotesToNarrativeData(quotes []domain.Quote) narrative.MarketNarrativeData {
	data := narrative.MarketNarrativeData{}
	for _, q := range quotes {
		switch q.Symbol {
		case "DXY", "^DXY":
			data.DXYChangePct = (q.Last - q.Open) / q.Open * 100
		case "US10Y", "^TNX":
			data.US10YChangeBps = q.Last
		case "VIX", "^VIX":
			data.VIXLevel = q.Last
		case "OIL", "CL=F":
			data.OilChangePct = (q.Last - q.Open) / q.Open * 100
		case "GOLD", "GC=F":
			data.GoldChangePct = (q.Last - q.Open) / q.Open * 100
		case "JPY=X", "USDJPY=X":
			data.JPY_ChangePct = (q.Last - q.Open) / q.Open * 100
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
