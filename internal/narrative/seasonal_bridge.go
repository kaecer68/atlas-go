package narrative

// SeasonalBridge implements industry.NarrativeSeasonalProvider, bridging
// the macro-narrative event system into seasonal pattern adjustment calculations.
// It maps active narrative themes to industry-specific seasonal multipliers.
type SeasonalBridge struct {
	engine       *NarrativeEngine
	activeEvents []NarrativeEvent
}

// NewSeasonalBridge creates a bridge from a NarrativeEngine.
func NewSeasonalBridge(engine *NarrativeEngine) *SeasonalBridge {
	return &SeasonalBridge{engine: engine}
}

// SetActiveEvents updates the active event list. Call this before computing
// seasonal adjustments to reflect the latest narrative state.
func (sb *SeasonalBridge) SetActiveEvents(events []NarrativeEvent) {
	sb.activeEvents = events
}

// ActiveThemes returns all active narrative theme identifiers.
// If active events have been explicitly set via SetActiveEvents, those are used.
// Otherwise, the bridge queries the narrative engine directly for current events.
func (sb *SeasonalBridge) ActiveThemes() []string {
	events := sb.activeEvents
	if len(events) == 0 && sb.engine != nil {
		events = sb.engine.DetectEvents(MarketNarrativeData{})
	}

	seen := make(map[string]bool)
	var themes []string
	for _, e := range events {
		if !seen[e.Theme] {
			seen[e.Theme] = true
			themes = append(themes, e.Theme)
		}
	}
	return themes
}

// SeasonalMultiplier returns the seasonal adjustment multiplier for a given
// narrative theme and industry. Themes are mapped based on known macro→sector
// causal relationships from the narrative knowledge base:
//
//	oil_price_shock → amplifies energy (+), dampens industrial/shipping (−)
//	AI_capex_surge → amplifies semiconductor and ai_supply_chain
//	US_rates_up     → dampens dividend-themed sectors (financials, high_dividend)
//	JPY_carry_unwind → dampens risk-on sectors (ai_supply_chain, growth)
//	geopolitical_risk_spike → amplifies defensive (consumer, financials), dampens export
func (sb *SeasonalBridge) SeasonalMultiplier(theme string, industryID string) float64 {
	switch theme {
	case "oil_price_shock":
		switch industryID {
		case "energy":
			return 1.12
		case "shipping", "industrial":
			return 0.92
		default:
			return 1.0
		}
	case "AI_capex_surge":
		switch industryID {
		case "semiconductor", "ai_supply_chain", "electronics":
			return 1.15
		default:
			return 1.0
		}
	case "US_rates_up":
		switch industryID {
		case "financials":
			return 1.08
		case "consumer", "electronics", "industrial":
			return 0.95
		default:
			return 1.0
		}
	case "JPY_carry_unwind":
		switch industryID {
		case "semiconductor", "ai_supply_chain", "shipping":
			return 0.90
		case "financials", "consumer":
			return 1.05
		default:
			return 1.0
		}
	case "geopolitical_risk_spike":
		switch industryID {
		case "consumer", "financials":
			return 1.06
		case "semiconductor", "ai_supply_chain", "shipping":
			return 0.94
		default:
			return 1.0
		}
	default:
		return 1.0
	}
}

// CorrelationMultiplier returns the correlation adjustment multiplier for a
// given narrative theme and industry pair. This enables dynamic supply chain
// linkage correlation modulation based on active macro events:
//
//   - oil_price_shock       → amplifies energy↔shipping (cost pass-through)
//   - AI_capex_surge         → amplifies semiconductor↔ai_supply_chain (demand pull)
//   - US_rates_up            → amplifies financials↔shipping (rate sensitivity)
//   - JPY_carry_unwind       → dampens tech supply chain (risk-off rotation)
//   - geopolitical_risk_spike → dampens export-oriented correlations (supply disruption)
//
// Returns 1.0 when the theme has no effect on the given pair.
func (sb *SeasonalBridge) CorrelationMultiplier(theme string, industryA, industryB string) float64 {
	// Normalize order for symmetric pair matching
	a, b := industryA, industryB
	if a > b {
		a, b = b, a
	}

	switch theme {
	case "oil_price_shock":
		if (a == "energy" && b == "shipping") || (a == "energy" && b == "industrial") {
			return 1.15
		}
		if a == "shipping" && b == "industrial" {
			return 0.92
		}
		return 1.0

	case "AI_capex_surge":
		if (a == "semiconductor" && b == "ai_supply_chain") ||
			(a == "semiconductor" && b == "electronics") ||
			(a == "ai_supply_chain" && b == "electronics") {
			return 1.12
		}
		return 1.0

	case "US_rates_up":
		if a == "financials" && b == "shipping" {
			return 1.10
		}
		if a == "financials" && b == "industrial" {
			return 1.08
		}
		return 1.0

	case "JPY_carry_unwind":
		if (a == "semiconductor" && b == "ai_supply_chain") ||
			(a == "ai_supply_chain" && b == "shipping") {
			return 0.90
		}
		return 1.0

	case "geopolitical_risk_spike":
		if (a == "semiconductor" && b == "shipping") ||
			(a == "electronics" && b == "shipping") {
			return 0.88
		}
		if a == "consumer" && b == "financials" {
			return 1.08
		}
		return 1.0

	default:
		return 1.0
	}
}
