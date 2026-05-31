package narrative

import "github.com/kaecer68/atlas-go/internal/industry"

// SeasonalBridge implements industry.NarrativeSeasonalProvider, bridging
// the macro-narrative event system into seasonal pattern adjustment calculations.
// It maps active narrative themes to industry-specific seasonal multipliers.
type SeasonalBridge struct {
	engine       *NarrativeEngine
	activeEvents []NarrativeEvent
	cycleCard    *industry.CycleStatusCard
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

func (sb *SeasonalBridge) SetCycleCard(card *industry.CycleStatusCard) {
	sb.cycleCard = card
}

// CycleAmplifiedMultiplier computes a cycle-aware seasonal multiplier. When the
// cycle phase is expansion and at least one seasonal pattern is active, the base
// multiplier is amplified by the composite coefficient. Returns the base multiplier
// unchanged when no card is cached or when the cycle is not expansion.
func (sb *SeasonalBridge) CycleAmplifiedMultiplier(base float64, theme string, industryID string, direction float64) float64 {
	baseMultiplier := sb.SeasonalMultiplier(theme, industryID, direction)
	card := sb.cycleCard
	if card == nil {
		return baseMultiplier
	}
	if card.BusinessCycle != "expansion" {
		return baseMultiplier
	}
	if len(card.ActivePatterns) == 0 {
		return baseMultiplier
	}
	amplification := 1.0 + (card.CompositeCoefficient-1.0)*0.5
	return baseMultiplier * amplification
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
//	direction: +1 = shock up (e.g., oil price rises), -1 = shock down (e.g., oil price falls)
//
//	oil_price_shock → amplifies energy (+), dampens industrial/shipping (−)
//	AI_capex_surge → amplifies semiconductor and ai_supply_chain
//	US_rates_up     → amplifies financials (net interest margin), dampens consumer/industrial
//	JPY_carry_unwind → dampens risk-on sectors (ai_supply_chain, growth)
//	geopolitical_risk_spike → amplifies defensive (consumer, financials), dampens export
func (sb *SeasonalBridge) SeasonalMultiplier(theme string, industryID string, direction float64) float64 {
	switch theme {
	case "oil_price_shock":
		switch industryID {
		case "energy":
			if direction > 0 {
				return 1.12 // oil up = energy benefits
			}
			return 0.88 // oil down = energy hurts
		case "shipping", "industrial":
			if direction > 0 {
				return 0.92 // oil up = shipping/industrial hurt
			}
			return 1.08 // oil down = shipping/industrial benefit
		default:
			return 1.0
		}
	case "AI_capex_surge":
		switch industryID {
		case "semiconductor", "ai_supply_chain", "electronics":
			if direction > 0 {
				return 1.15 // capex surge
			}
			return 0.90 // capex contraction
		default:
			return 1.0
		}
	case "US_rates_up":
		switch industryID {
		case "financials":
			if direction > 0 {
				return 1.08 // rates up = banks benefit
			}
			return 0.95 // rates down = banks hurt
		case "consumer", "electronics", "industrial":
			if direction > 0 {
				return 0.95 // rates up = borrowing costs rise
			}
			return 1.05 // rates down = borrowing costs fall
		default:
			return 1.0
		}
	case "JPY_carry_unwind":
		switch industryID {
		case "semiconductor", "ai_supply_chain", "shipping":
			if direction > 0 {
				return 0.90 // unwind = exporters hurt
			}
			return 1.05 // stable = slight recovery
		case "financials", "consumer":
			if direction > 0 {
				return 1.05 // domestic benefits from repatriation
			}
			return 1.0
		case "leo_satellite":
			if direction > 0 {
				return 0.95 // export-oriented, risk-off dampening
			}
			return 1.02
		case "mining":
			if direction > 0 {
				return 1.03 // commodity safe-haven flow
			}
			return 1.0
		default:
			return 1.0
		}
	case "geopolitical_risk_spike":
		switch industryID {
		case "consumer", "financials":
			if direction > 0 {
				return 1.06 // risk up = safe havens
			}
			return 0.98 // risk down = slight normalization
		case "semiconductor", "ai_supply_chain", "shipping":
			if direction > 0 {
				return 0.94 // risk up = supply chains hurt
			}
			return 1.02 // risk down = slight recovery
		default:
			return 1.0
		}
	case "taiwan_political_risk":
		switch industryID {
		case "semiconductor", "ai_supply_chain", "electronics":
			return 0.92
		case "financials", "consumer":
			return 1.08
		default:
			return 1.0
		}
	case "election_cycle":
		switch industryID {
		case "semiconductor", "ai_supply_chain":
			return 0.97
		case "consumer":
			return 1.03
		default:
			return 1.0
		}
	case "spring_festival_season":
		switch industryID {
		case "consumer":
			return 1.05
		case "semiconductor", "ai_supply_chain":
			return 0.95
		default:
			return 1.0
		}
	case "USD_TWD_volatility":
		switch industryID {
		case "semiconductor", "electronics":
			return 1.05
		case "consumer":
			return 0.97
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
	match := func(x, y string) bool {
		return (industryA == x && industryB == y) || (industryA == y && industryB == x)
	}

	switch theme {
	case "oil_price_shock":
		if match("energy", "shipping") || match("energy", "industrial") {
			return 1.15
		}
		if match("shipping", "industrial") {
			return 0.92
		}
		if match("mining", "energy") {
			return 1.08 // energy cost pass-through
		}
		return 1.0

	case "AI_capex_surge":
		if match("semiconductor", "ai_supply_chain") ||
			match("semiconductor", "electronics") ||
			match("ai_supply_chain", "electronics") {
			return 1.12
		}
		if match("leo_satellite", "semiconductor") {
			return 1.10 // satellite chip demand from AI infrastructure
		}
		if match("leo_satellite", "ai_supply_chain") {
			return 1.08 // space-based AI data processing
		}
		return 1.0

	case "US_rates_up":
		if match("financials", "shipping") {
			return 1.10
		}
		if match("financials", "industrial") {
			return 1.08
		}
		return 1.0

	case "JPY_carry_unwind":
		if match("semiconductor", "ai_supply_chain") ||
			match("ai_supply_chain", "shipping") {
			return 0.90
		}
		if match("leo_satellite", "electronics") {
			return 0.92 // export-oriented risk-off dampening
		}
		if match("mining", "financials") {
			return 1.05 // commodity safe-haven during yen volatility
		}
		return 1.0

	case "geopolitical_risk_spike":
		if match("semiconductor", "shipping") ||
			match("electronics", "shipping") {
			return 0.88
		}
		if match("consumer", "financials") {
			return 1.08
		}
		if match("leo_satellite", "semiconductor") {
			return 1.15 // dual-use defense supply chain disruption
		}
		if match("leo_satellite", "ai_supply_chain") {
			return 1.12 // space-based AI infrastructure risk
		}
		if match("mining", "semiconductor") {
			return 1.12 // rare earth supply disruption
		}
		if match("mining", "electronics") {
			return 1.10 // critical mineral supply chains
		}
		if match("mining", "energy") {
			return 1.08 // energy-commodity linkage
		}
		return 1.0

	case "taiwan_political_risk":
		if match("semiconductor", "shipping") ||
			match("electronics", "shipping") {
			return 1.10
		}
		if match("semiconductor", "ai_supply_chain") {
			return 1.12
		}
		if match("consumer", "financials") {
			return 1.10
		}
		if match("leo_satellite", "semiconductor") {
			return 1.12 // Taiwan semiconductor dependency
		}
		if match("leo_satellite", "ai_supply_chain") {
			return 1.10 // Taiwan space-AI infrastructure
		}
		return 1.0

	case "election_cycle":
		if match("financials", "consumer") {
			return 1.05
		}
		if match("semiconductor", "ai_supply_chain") {
			return 0.95
		}
		return 1.0

	case "spring_festival_season":
		if match("consumer", "financials") {
			return 1.05
		}
		return 1.0

	case "USD_TWD_volatility":
		if match("semiconductor", "electronics") {
			return 1.08
		}
		if match("semiconductor", "shipping") {
			return 1.05
		}
		return 1.0

	default:
		return 1.0
	}
}
