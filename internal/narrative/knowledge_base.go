package narrative

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// KnowledgeBase holds causal templates and produces instantiated chains.
type KnowledgeBase struct {
	mu        sync.RWMutex
	templates map[string]CausalTemplate
}

// NewKnowledgeBase creates a knowledge base preloaded with default templates.
func NewKnowledgeBase() *KnowledgeBase {
	kb := &KnowledgeBase{
		templates: make(map[string]CausalTemplate),
	}
	for _, t := range DefaultTemplates() {
		kb.templates[t.ID] = t
	}
	return kb
}

// RegisterTemplate adds or replaces a template.
func (kb *KnowledgeBase) RegisterTemplate(t CausalTemplate) {
	kb.mu.Lock()
	defer kb.mu.Unlock()
	kb.templates[t.ID] = t
}

// GetTemplate returns a template by ID.
func (kb *KnowledgeBase) GetTemplate(id string) (CausalTemplate, bool) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	t, ok := kb.templates[id]
	return t, ok
}

// ListTemplates returns all registered templates.
func (kb *KnowledgeBase) ListTemplates() []CausalTemplate {
	kb.mu.RLock()
	defer kb.mu.RUnlock()
	out := make([]CausalTemplate, 0, len(kb.templates))
	for _, t := range kb.templates {
		out = append(out, t)
	}
	return out
}

// MatchChains finds all causal templates that match a given event and instantiates chains.
func (kb *KnowledgeBase) MatchChains(event NarrativeEvent) []CausalChain {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	var chains []CausalChain
	for _, tmpl := range kb.templates {
		if !strings.EqualFold(tmpl.TriggerTheme, event.Theme) {
			continue
		}
		if tmpl.RequiredRegion != "" && !strings.EqualFold(tmpl.RequiredRegion, event.Region) {
			continue
		}

		score := event.Confidence * tmpl.HistoricalHitRate
		steps := make([]CausalStep, len(tmpl.Steps))
		copy(steps, tmpl.Steps)

		chains = append(chains, CausalChain{
			EventID:    event.ID,
			TemplateID: tmpl.ID,
			Steps:      steps,
			Score:      score,
		})
	}
	return chains
}

// NarrativeEngine orchestrates event detection and causal chain matching.
type NarrativeEngine struct {
	kb     *KnowledgeBase
	models []InvestmentModel
}

// NewNarrativeEngine creates a narrative engine with default templates and models.
func NewNarrativeEngine() *NarrativeEngine {
	return &NarrativeEngine{
		kb: NewKnowledgeBase(),
		models: []InvestmentModel{
			{
				ID:             "hawkish_fed_model",
				Name:           "Hawkish Fed Model",
				Description:    "Assumes persistently high US rates and USD strength; favors defensive TW sectors.",
				ActiveThemes:   []string{"US_rates_up", "JPY_carry_unwind"},
				FavoredSectors: []string{"financials", "high_dividend", "etf_rotation"},
				AvoidedSectors: []string{"ai_supply_chain", "small_cap"},
				Weight:         1.0,
			},
			{
				ID:             "ai_supercycle_model",
				Name:           "AI Supercycle Model",
				Description:    "Assumes AI capex cycle overrides macro headwinds; favors Taiwan tech supply chain.",
				ActiveThemes:   []string{"AI_capex_surge"},
				FavoredSectors: []string{"ai_supply_chain", "semiconductor", "pcb", "thermal"},
				AvoidedSectors: []string{"consumer", "tourism"},
				Weight:         1.0,
			},
			{
				ID:             "geopolitical_hedge_model",
				Name:           "Geopolitical Hedge Model",
				Description:    "Assumes elevated geopolitical risk and risk-off flows; favors gold, USD hedges, and TW defensive sectors.",
				ActiveThemes:   []string{"geopolitical_risk_spike", "oil_price_shock"},
				FavoredSectors: []string{"financials", "high_dividend", "shipping"},
				AvoidedSectors: []string{"ai_supply_chain", "small_cap"},
				Weight:         1.0,
			},
		},
	}
}

// DetectEvents accepts raw market/narrative data and returns events.
func (ne *NarrativeEngine) DetectEvents(data MarketNarrativeData) []NarrativeEvent {
	var events []NarrativeEvent
	if evt := detectUSRatesEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectAICapexEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectGeopoliticalRiskEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectOilShockEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectJPYCarryUnwindEvent(data); evt != nil {
		events = append(events, *evt)
	}
	return events
}

// MatchChains returns all causal chains matching the given events.
func (ne *NarrativeEngine) MatchChains(events []NarrativeEvent) []CausalChain {
	var all []CausalChain
	for _, evt := range events {
		chains := ne.kb.MatchChains(evt)
		all = append(all, chains...)
	}
	return all
}

// ActiveModels returns investment models whose active themes intersect with the given event themes.
func (ne *NarrativeEngine) ActiveModels(eventThemes []string) []InvestmentModel {
	themeSet := make(map[string]struct{})
	for _, t := range eventThemes {
		themeSet[strings.ToLower(t)] = struct{}{}
	}

	var active []InvestmentModel
	for _, m := range ne.models {
		for _, t := range m.ActiveThemes {
			if _, ok := themeSet[strings.ToLower(t)]; ok {
				active = append(active, m)
				break
			}
		}
	}
	return active
}

// UpdateModelWeights adjusts model weights based on recent prediction errors.
func (ne *NarrativeEngine) UpdateModelWeights() {
	// Simple inverse-error weighting.
	var totalInvErr float64
	for i := range ne.models {
		err := ne.models[i].RecentError
		if err <= 0 {
			err = 0.001
		}
		totalInvErr += 1.0 / err
	}
	for i := range ne.models {
		err := ne.models[i].RecentError
		if err <= 0 {
			err = 0.001
		}
		ne.models[i].Weight = (1.0 / err) / totalInvErr
	}
}

// ListModels returns all investment models.
func (ne *NarrativeEngine) ListModels() []InvestmentModel {
	out := make([]InvestmentModel, len(ne.models))
	copy(out, ne.models)
	return out
}

// MarketNarrativeData carries raw inputs for narrative detection.
type MarketNarrativeData struct {
	US10YChangeBps    float64
	DXYChangePct      float64
	VIXLevel          float64
	USD_TWD_ChangePct float64
	OilChangePct      float64
	GoldChangePct     float64
	JPY_ChangePct     float64
	AICapexSentiment  float64 // +1 bullish, -1 bearish
	GeopoliticalGPR   float64 // Geopolitical risk index level
}

func detectUSRatesEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.US10YChangeBps > 10 || data.DXYChangePct > 1.5 {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-us-rates-%d", nowUnix()),
			Theme:       "US_rates_up",
			Region:      "US",
			Sentiment:   -0.6,
			Confidence:  0.75,
			CapitalFlow: "flight_to_USD",
			TimeWindow:  "1_week",
			Timestamp:   time.Now().UTC(),
			SourceData: map[string]float64{
				"us10y_change_bps": data.US10YChangeBps,
				"dxy_change_pct":   data.DXYChangePct,
			},
		}
	}
	return nil
}

func detectAICapexEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.AICapexSentiment > 0.5 {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-ai-capex-%d", nowUnix()),
			Theme:       "AI_capex_surge",
			Region:      "US",
			Sentiment:   0.8,
			Confidence:  0.70,
			CapitalFlow: "tech_capex_inflow",
			TimeWindow:  "1_month",
			Timestamp:   time.Now().UTC(),
			SourceData: map[string]float64{
				"ai_capex_sentiment": data.AICapexSentiment,
			},
		}
	}
	return nil
}

func detectGeopoliticalRiskEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.GeopoliticalGPR > 150 || data.GoldChangePct > 2.0 {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-geo-%d", nowUnix()),
			Theme:       "geopolitical_risk_spike",
			Region:      "Global",
			Sentiment:   -0.8,
			Confidence:  0.65,
			CapitalFlow: "risk_off",
			TimeWindow:  "immediate",
			Timestamp:   time.Now().UTC(),
			SourceData: map[string]float64{
				"geopolitical_gpr": data.GeopoliticalGPR,
				"gold_change_pct":  data.GoldChangePct,
			},
		}
	}
	return nil
}

func detectOilShockEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.OilChangePct > 5.0 || data.OilChangePct < -5.0 {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-oil-%d", nowUnix()),
			Theme:       "oil_price_shock",
			Region:      "Global",
			Sentiment:   -0.5,
			Confidence:  0.60,
			CapitalFlow: "inflation_reprice",
			TimeWindow:  "1_week",
			Timestamp:   time.Now().UTC(),
			SourceData: map[string]float64{
				"oil_change_pct": data.OilChangePct,
			},
		}
	}
	return nil
}

func detectJPYCarryUnwindEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.JPY_ChangePct > 2.0 || data.VIXLevel > 25 {
		return &NarrativeEvent{
			ID:          fmt.Sprintf("evt-jpy-%d", nowUnix()),
			Theme:       "JPY_carry_unwind",
			Region:      "JP",
			Sentiment:   -0.6,
			Confidence:  0.65,
			CapitalFlow: "global_liquidity_drain",
			TimeWindow:  "immediate",
			Timestamp:   time.Now().UTC(),
			SourceData: map[string]float64{
				"jpy_change_pct": data.JPY_ChangePct,
				"vix_level":      data.VIXLevel,
			},
		}
	}
	return nil
}

var nowUnix = func() int64 {
	// Overridden in tests.
	return time.Now().UnixNano()
}
