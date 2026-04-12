package narrative

// DefaultTemplates returns the built-in causal narrative templates.
func DefaultTemplates() []CausalTemplate {
	return []CausalTemplate{
		{
			ID:             "us_rates_up",
			Name:           "US Rates Up / Hawkish Fed",
			TriggerTheme:   "US_rates_up",
			RequiredRegion: "US",
			Steps: []CausalStep{
				{Description: "US Treasury yields rise → USD strengthens", Affected: []string{"DXY", "EM_currencies"}, Impact: 0.6},
				{Description: "Stronger USD + higher rates trigger foreign outflows from EM Asia", Affected: []string{"foreign_flow_TW"}, Impact: -0.7},
				{Description: "Taiwan TAIEX faces pressure; defensive sectors relatively resilient", Affected: []string{"financials", "high_dividend", "etf_rotation"}, Impact: 0.4},
				{Description: "High-beta sectors (AI supply chain, small cap) face valuation compression", Affected: []string{"ai_supply_chain", "small_cap"}, Impact: -0.6},
			},
			HistoricalHitRate: 0.72,
			SourceReferences:  []string{"IMF Working Paper WP/19/128", "BIS Quarterly Review Dec 2022"},
		},
		{
			ID:             "jpy_carry_unwind",
			Name:           "JPY Carry Trade Unwind",
			TriggerTheme:   "JPY_carry_unwind",
			RequiredRegion: "JP",
			Steps: []CausalStep{
				{Description: "BOJ rate hike or balance-sheet reduction strengthens JPY", Affected: []string{"JPY", "global_liquidity"}, Impact: 0.5},
				{Description: "Unwinding of JPY-funded leveraged positions reduces global risk appetite", Affected: []string{"global_equities", "VIX"}, Impact: -0.6},
				{Description: "Taiwan high-leverage / high-valuation names face short-term liquidation pressure", Affected: []string{"ai_supply_chain", "small_cap"}, Impact: -0.5},
				{Description: "Safe-haven flows into JPY and quality assets", Affected: []string{"JPY", "gold", "high_dividend"}, Impact: 0.4},
			},
			HistoricalHitRate: 0.68,
			SourceReferences:  []string{"BIS Annual Economic Report 2024", "Goldman Sachs Global Macro Note Aug 2024"},
		},
		{
			ID:             "ai_capex_surge",
			Name:           "AI Capex Surge",
			TriggerTheme:   "AI_capex_surge",
			RequiredRegion: "US",
			Steps: []CausalStep{
				{Description: "NVIDIA / MSFT / hyperscalers raise capex guidance", Affected: []string{"US_tech_capex"}, Impact: 0.8},
				{Description: "CoWoS and advanced packaging demand tightens supply", Affected: []string{"semiconductor", "foundry"}, Impact: 0.7},
				{Description: "Taiwan AI supply chain (foundry, packaging, PCB, thermal) benefits", Affected: []string{"ai_supply_chain", "semiconductor", "pcb", "thermal"}, Impact: 0.8},
				{Description: "Upstream equipment and materials see increased order visibility", Affected: []string{"semi_equipment", "materials"}, Impact: 0.5},
			},
			HistoricalHitRate: 0.81,
			SourceReferences:  []string{"Morgan Stanley Taiwan Tech Outlook 2024", "Goldman Sachs Asia Pacific Strategy 2024"},
		},
		{
			ID:             "geopolitical_risk_spike",
			Name:           "Geopolitical Risk Spike",
			TriggerTheme:   "geopolitical_risk_spike",
			RequiredRegion: "Global",
			Steps: []CausalStep{
				{Description: "Escalation in Middle East or Taiwan Strait raises uncertainty", Affected: []string{"GPR_index", "oil"}, Impact: -0.8},
				{Description: "Flight to safety: USD and gold rise in tandem", Affected: []string{"DXY", "gold"}, Impact: 0.7},
				{Description: "Taiwan equity risk premium expands; capital flows turn defensive", Affected: []string{"TAIEX"}, Impact: -0.5},
				{Description: "Defensive sectors (financials, high dividend, shipping hedges) outperform cyclicals", Affected: []string{"financials", "high_dividend", "shipping"}, Impact: 0.4},
				{Description: "High-beta tech and export-oriented names face multiple compression", Affected: []string{"ai_supply_chain", "small_cap"}, Impact: -0.6},
			},
			HistoricalHitRate: 0.65,
			SourceReferences:  []string{"Caldara-Iacoviello GPR Dataset", "Fed Finance and Economics Discussion Series 2023"},
		},
		{
			ID:             "oil_price_shock",
			Name:           "Oil Price Shock",
			TriggerTheme:   "oil_price_shock",
			RequiredRegion: "Global",
			Steps: []CausalStep{
				{Description: "Sharp oil price move alters inflation expectations", Affected: []string{"breakeven_inflation", "oil"}, Impact: -0.6},
				{Description: "Fed policy path is repriced (hawkish if supply shock, dovish if demand collapse)", Affected: []string{"Fed_funds_futures", "US_rates"}, Impact: -0.5},
				{Description: "Rate-sensitive assets revalued; Taiwan transport and petrochemical sectors directly impacted", Affected: []string{"shipping", "petrochemicals"}, Impact: -0.4},
				{Description: "Consumer discretionary and margin-sensitive sectors face cost pressure", Affected: []string{"consumer", "tourism"}, Impact: -0.3},
			},
			HistoricalHitRate: 0.58,
			SourceReferences:  []string{"Hamilton (1983) JPE - Oil and Macroeconomy", "IMF World Economic Outlook 2022"},
		},
		{
			ID:             "middle_east_escalation",
			Name:           "Middle East Conflict Escalation",
			TriggerTheme:   "middle_east_escalation",
			RequiredRegion: "Middle East",
			Steps: []CausalStep{
				{Description: "Conflict escalation threatens oil supply and Red Sea shipping lanes", Affected: []string{"oil", "shipping"}, Impact: -0.8},
				{Description: "Oil price surge raises global inflation expectations", Affected: []string{"breakeven_inflation", "US_rates"}, Impact: -0.6},
				{Description: "Fed forced to maintain hawkish stance to combat supply-side inflation", Affected: []string{"Fed_funds_futures", "DXY"}, Impact: 0.5},
				{Description: "Safe-haven flows into gold and USD strengthen the dollar", Affected: []string{"gold", "DXY"}, Impact: 0.7},
				{Description: "Emerging markets face capital outflows as risk appetite drops", Affected: []string{"EM_currencies", "foreign_flow_TW"}, Impact: -0.7},
				{Description: "Taiwan high-beta tech and export-oriented names face multiple compression", Affected: []string{"ai_supply_chain", "semiconductor", "small_cap"}, Impact: -0.6},
				{Description: "Defensive sectors (financials, high dividend, shipping hedges) relatively resilient", Affected: []string{"financials", "high_dividend", "shipping"}, Impact: 0.3},
			},
			HistoricalHitRate: 0.68,
			SourceReferences:  []string{"Caldara-Iacoviello GPR Dataset", "BIS Quarterly Review Dec 2023 - Geopolitical risk and commodity prices", "Goldman Sachs Global Macro Research 2024"},
		},
	}
}
