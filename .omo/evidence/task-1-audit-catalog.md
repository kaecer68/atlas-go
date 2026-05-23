# Hardcoded Investment Parameters Catalog

Generated: 2026-05-20
Scope: `internal/portfolio/`, `internal/orchestrator/`, `internal/industry/`, `internal/narrative/`, `internal/risk/`
Exclusions: `*_test.go`, `cmd/experimental/`, mathematical constants (pi, sqrt), buffer sizes, timeouts
Litmus test: "Would changing this value in a backtest produce different investment decisions?"

**Summary**: 60 CONFIG_CANDIDATE, 5 LEGITIMATE_CONSTANT

---

## File: `internal/portfolio/factor_weight_engine.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 23 | 0.25 | FactorMomentum base weight | CONFIG_CANDIDATE |
| 24 | 0.20 | FactorValue base weight | CONFIG_CANDIDATE |
| 25 | 0.20 | FactorQuality base weight | CONFIG_CANDIDATE |
| 26 | 0.15 | FactorAgent base weight | CONFIG_CANDIDATE |
| 27 | 0.10 | FactorInstSent base weight | CONFIG_CANDIDATE |
| 28 | 0.05 | FactorLiquidity base weight | CONFIG_CANDIDATE |
| 29 | 0.05 | FactorNarrative base weight | CONFIG_CANDIDATE |
| 30 | 0.00 | FactorIndustryCycle base weight | CONFIG_CANDIDATE |
| 46 | +0.05 | Bull regime: Momentum boost | CONFIG_CANDIDATE |
| 47 | -0.03 | Bull regime: Quality reduction | CONFIG_CANDIDATE |
| 48 | -0.02 | Bull regime: Value reduction | CONFIG_CANDIDATE |
| 50 | +0.05 | Bear regime: Quality boost | CONFIG_CANDIDATE |
| 51 | +0.03 | Bear regime: Value boost | CONFIG_CANDIDATE |
| 52 | -0.05 | Bear regime: Momentum reduction | CONFIG_CANDIDATE |
| 54 | +0.05 | High_vol regime: Liquidity boost | CONFIG_CANDIDATE |
| 55 | -0.03 | High_vol regime: Momentum reduction | CONFIG_CANDIDATE |
| 56 | -0.02 | High_vol regime: InstSent reduction | CONFIG_CANDIDATE |
| 68 | 0.02 | Single factor weight floor (clamp lower bound) | CONFIG_CANDIDATE |
| 71 | 0.50 | Single factor weight ceiling (clamp upper bound) | CONFIG_CANDIDATE |
| 105 | 0.001 | Normalization tolerance threshold | CONFIG_CANDIDATE |
| 117 | +0.05/-0.03 | RISK_ON event: Momentum & Quality delta | CONFIG_CANDIDATE |
| 122 | -0.05/+0.05/+0.03 | RISK_OFF event: Momentum/Quality/Liquidity delta | CONFIG_CANDIDATE |
| 145 | 0.10 | Severity=critical → delta | CONFIG_CANDIDATE |
| 147 | 0.05 | Severity=high → delta | CONFIG_CANDIDATE |
| 149 | 0.02 | Severity=medium → delta | CONFIG_CANDIDATE |
| 151 | 0.01 | Severity=low → delta | CONFIG_CANDIDATE |
| 153 | 0.02 | Severity default → delta | CONFIG_CANDIDATE |
| 156-175 | delta/delta | Theme-to-factor adjustment maps (AI_capex_surge, US_rates_up, oil_price_shock) | CONFIG_CANDIDATE |
| 210-221 | 0.05/-0.05/0.03/-0.03 | Strategy risk appetite adjustments (Conservative/Aggressive) | CONFIG_CANDIDATE |

---

## File: `internal/portfolio/darwinian_weights.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 21 | 0.3 | DarwinianWeightMin — agent weight floor | CONFIG_CANDIDATE |
| 23 | 2.5 | DarwinianWeightMax — agent weight ceiling | CONFIG_CANDIDATE |
| 25 | 1.0 | DarwinianNeutralWeight | CONFIG_CANDIDATE |
| 27 | 1.05 | TopQuartileMultiplier | CONFIG_CANDIDATE |
| 29 | 0.95 | BottomQuartileMultiplier | CONFIG_CANDIDATE |
| 31 | 20h | DailyAdjustmentCooldown | CONFIG_CANDIDATE |
| 70 | 60 | DarwinianConfig.RollingWindowDays | CONFIG_CANDIDATE |
| 71 | true | DarwinianConfig.UseExponentialDecay | CONFIG_CANDIDATE |
| 72 | 10 | DarwinianConfig.DecayHalfLifeDays | CONFIG_CANDIDATE |
| 73 | 30 | DarwinianConfig.NewAgentProtectionDays | CONFIG_CANDIDATE |
| 74 | 1.0 | DarwinianConfig.NewAgentFixedWeight | CONFIG_CANDIDATE |
| 75 | 3 | DarwinianConfig.MinAdjustmentInterval (days) | CONFIG_CANDIDATE |
| 76 | 0.2 | DarwinianConfig.WeightMomentumFactor | CONFIG_CANDIDATE |
| 854 | 1.5 | Report: "top performer" weight threshold | LEGITIMATE_CONSTANT (reporting) |
| 856 | 0.7 | Report: "bottom performer" weight threshold | LEGITIMATE_CONSTANT (reporting) |
| 864-868 | 2.0/1.5/0.8/0.5 | Weight distribution bucket boundaries | LEGITIMATE_CONSTANT (reporting) |

---

## File: `internal/portfolio/sizing.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 25 | 0.25 | KellyFraction — Kelly criteria conservative fraction | CONFIG_CANDIDATE |
| 26 | 20 | VolLookback — volatility lookback window (days) | CONFIG_CANDIDATE |
| 27 | 0.01 | MaxPositionByADV — 1% of daily volume limit | CONFIG_CANDIDATE |
| 28 | 0.10 | MaxDrawdownLimit — 10% max drawdown | CONFIG_CANDIDATE |
| 29 | 2.0 | ATRMultiplier — ATR stop-loss multiplier | CONFIG_CANDIDATE |
| 30 | 0.5 | CorrelationPenalty — correlation penalty coefficient | CONFIG_CANDIDATE |

---

## File: `internal/portfolio/risk_manager.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 328 | 1.5 (150%) | ShouldStopTrading: maxDrawdownPct × 1.5 threshold | CONFIG_CANDIDATE |
| 340 | 3 | ShouldStopTrading: critical alert count threshold | CONFIG_CANDIDATE |
| 349 | 0.01 | CalculatePositionSize: volatility max loss divisor | CONFIG_CANDIDATE |

---

## File: `internal/portfolio/volatility_manager.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 68 | 0.3 | Default smoothingFactor (EMA) | CONFIG_CANDIDATE |
| 69 | 0.05 | Default rebalanceThreshold (5% deviation) | CONFIG_CANDIDATE |
| 304 | 7 days | WeeklyRebalanceDays interval | CONFIG_CANDIDATE |

---

## File: `internal/portfolio/regime.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 47-50 | 0.40/0.20/0.30/0.10 | RISK_ON: Growth/Value/Momentum/Quality allocation | CONFIG_CANDIDATE |
| 52 | 0.95 | RISK_ON: MaxExposure | CONFIG_CANDIDATE |
| 53 | 0.05 | RISK_ON: CashReserve | CONFIG_CANDIDATE |
| 59-62 | 0.25/0.25/0.25/0.25 | NEUTRAL: Growth/Value/Momentum/Quality allocation | CONFIG_CANDIDATE |
| 64 | 0.80 | NEUTRAL: MaxExposure | CONFIG_CANDIDATE |
| 65 | 0.20 | NEUTRAL: CashReserve | CONFIG_CANDIDATE |
| 71-74 | 0.10/0.40/0.15/0.35 | RISK_OFF: Growth/Value/Momentum/Quality allocation | CONFIG_CANDIDATE |
| 76 | 0.50 | RISK_OFF: MaxExposure | CONFIG_CANDIDATE |
| 77 | 0.50 | RISK_OFF: CashReserve | CONFIG_CANDIDATE |
| 239-242 | 60/40/25/15 | RegimeThresholds: RSI high/low, VIX high/low | CONFIG_CANDIDATE |
| 152 | 0.80 | GetMaxExposure: neutral fallback | CONFIG_CANDIDATE |
| 165 | 0.20 | GetCashReserve: neutral fallback | CONFIG_CANDIDATE |

---

## File: `internal/portfolio/sector_rotator.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 107 | 0.05 | Yellow: defensive allocation boost | CONFIG_CANDIDATE |
| 108 | 0.03 | Yellow: cash allocation boost | CONFIG_CANDIDATE |
| 109-110 | -0.04/-0.04 | Yellow: ai_supply_chain & semiconductor reduction | CONFIG_CANDIDATE |
| 114-119 | 0.10/0.08/0.05/-0.08/-0.08/-0.05 | Orange: moderate defensive positioning deltas | CONFIG_CANDIDATE |
| 123-129 | 0.25/0.15/0.10/-0.15/-0.15/-0.10/-0.05 | Red: severe risk-off allocation deltas | CONFIG_CANDIDATE |
| 136-140 | 0.10/0.08/0.07/-0.10/0.0 | Flow "risk_off": allocation changes | CONFIG_CANDIDATE |
| 143-148 | 0.30/0.15/0.05/0.02/0.03/-0.10 | Flow "carry_trade_unwind": allocation changes | CONFIG_CANDIDATE |
| 151-156 | 0.15/0.08/0.05/0.05/0.02/-0.08 | Flow "sector_rotation": allocation changes | CONFIG_CANDIDATE |

---

## File: `internal/orchestrator/narrative_conviction_modulator.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 22 | 0.81 | hitRate: AI_capex_surge | CONFIG_CANDIDATE |
| 23 | 0.72 | hitRate: US_rates_up | CONFIG_CANDIDATE |
| 24 | 0.68 | hitRate: JPY_carry_unwind | CONFIG_CANDIDATE |
| 25 | 0.65 | hitRate: geopolitical_risk_spike | CONFIG_CANDIDATE |
| 26 | 0.58 | hitRate: oil_price_shock | CONFIG_CANDIDATE |
| 31-39 | skill→theme | Agent skill-to-narrative-theme mapping (9 entries) | CONFIG_CANDIDATE |
| 109 | ×10 | Hit rate to conviction adjustment multiplier | CONFIG_CANDIDATE |

---

## File: `internal/industry/cycle.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 127 | 0.25 | semiconductor: RevenueGrowthYoY default | CONFIG_CANDIDATE |
| 128 | 0.30 | semiconductor: ProfitGrowthYoY default | CONFIG_CANDIDATE |
| 129 | 5.5 | semiconductor: InventoryTurnover default | CONFIG_CANDIDATE |
| 130 | 0.85 | semiconductor: CapacityUtilization default | CONFIG_CANDIDATE |
| 134-137 | 0.45/0.50/6.0/0.90 | ai_supply_chain: default metrics | CONFIG_CANDIDATE |
| 140-143 | 0.15/0.12/4.0/0.70 | robotics: default metrics | CONFIG_CANDIDATE |
| 147-150 | 0.08/0.10/0.0/0.75 | financials: default metrics | CONFIG_CANDIDATE |
| 154-157 | -0.05/-0.10/3.0/0.65 | shipping: default metrics | CONFIG_CANDIDATE |
| 161-164 | 0.05/0.03/4.5/0.70 | energy: default metrics | CONFIG_CANDIDATE |
| 168-171 | 0.12/0.15/5.0/0.75 | electronics: default metrics | CONFIG_CANDIDATE |
| 175-178 | 0.03/0.05/6.0/0.70 | consumer: default metrics | CONFIG_CANDIDATE |
| 183-186 | 0.06/0.08/4.0/0.68 | industrial: default metrics | CONFIG_CANDIDATE |
| 190-193 | 0.22/0.28/5.0/0.88 | foundry: default metrics | CONFIG_CANDIDATE |
| 197-200 | 0.40/0.45/6.5/0.85 | server_assembly: default metrics | CONFIG_CANDIDATE |
| 204-207 | 0.20/0.22/5.5/0.80 | cooling: default metrics | CONFIG_CANDIDATE |
| 305-311 | 0.20/0.20/0.05/0.05/-0.05/-0.05 | Fallback CycleThresholdConfig (when not in parameters) | CONFIG_CANDIDATE |
| 507-513 | 0.20/0.20/0.05/0.05/-0.05/-0.05 | boundaryConfidence: fallback thresholds (duplicate) | CONFIG_CANDIDATE |
| 532 | 0.25 | boundaryConfidence: fallback range denominator | CONFIG_CANDIDATE |
| 552 | 0.30 | Leading indicator: Revenue Growth weight | CONFIG_CANDIDATE |
| 563 | 0.25 | Leading indicator: Inventory Turnover weight | CONFIG_CANDIDATE |
| 581 | 0.35 | Lagging indicator: Profit Growth weight | CONFIG_CANDIDATE |
| 593 | 0.30 | Lagging indicator: Capacity Utilization weight | CONFIG_CANDIDATE |
| 766 | 0.70 | CycleTransition: Recession→Recovery probability | CONFIG_CANDIDATE |
| 767 | 0.80 | CycleTransition: Recovery→Expansion probability | CONFIG_CANDIDATE |
| 768 | 0.60 | CycleTransition: Expansion→Mature probability | CONFIG_CANDIDATE |
| 769 | 0.50 | CycleTransition: Mature→Recession probability | CONFIG_CANDIDATE |
| 766 | 180 | Transition Recession→Recovery: typical duration days | CONFIG_CANDIDATE |
| 767 | 270 | Transition Recovery→Expansion: typical duration days | CONFIG_CANDIDATE |
| 768 | 360 | Transition Expansion→Mature: typical duration days | CONFIG_CANDIDATE |
| 769 | 180 | Transition Mature→Recession: typical duration days | CONFIG_CANDIDATE |

---

## File: `internal/industry/linkage.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 244 | 0.5 | Default base correlation (when no available data) | CONFIG_CANDIDATE |
| 258 | 0.30 | Recession correlation boost factor | CONFIG_CANDIDATE |
| 290 | 15 | RecalculateFromReturns: min observations | CONFIG_CANDIDATE |
| 404 | 0.8 | ShockPropagation: downstream decay fallback | CONFIG_CANDIDATE |
| 415 | 0.6 | ShockPropagation: upstream decay fallback | CONFIG_CANDIDATE |
| 595 | 0.85 | Default correlation: semiconductor↔ai_supply_chain | CONFIG_CANDIDATE |
| 596 | 0.72 | Default correlation: semiconductor↔electronics | CONFIG_CANDIDATE |
| 597 | 0.45 | Default correlation: semiconductor↔robotics | CONFIG_CANDIDATE |
| 598 | 0.15 | Default correlation: semiconductor↔financials | CONFIG_CANDIDATE |
| 599 | -0.10 | Default correlation: semiconductor↔shipping | CONFIG_CANDIDATE |
| 602 | 0.65 | Default correlation: ai_supply_chain↔electronics | CONFIG_CANDIDATE |
| 603 | 0.55 | Default correlation: ai_supply_chain↔robotics | CONFIG_CANDIDATE |
| 604 | 0.20 | Default correlation: ai_supply_chain↔financials | CONFIG_CANDIDATE |
| 605 | 0.05 | Default correlation: ai_supply_chain↔shipping | CONFIG_CANDIDATE |
| 608 | 0.48 | Default correlation: robotics↔electronics | CONFIG_CANDIDATE |
| 609 | 0.60 | Default correlation: robotics↔industrial | CONFIG_CANDIDATE |
| 610 | 0.10 | Default correlation: robotics↔financials | CONFIG_CANDIDATE |
| 613 | 0.35 | Default correlation: financials↔consumer | CONFIG_CANDIDATE |
| 614 | 0.25 | Default correlation: financials↔industrial | CONFIG_CANDIDATE |
| 615 | 0.05 | Default correlation: financials↔shipping | CONFIG_CANDIDATE |
| 616 | 0.10 | Default correlation: financials↔energy | CONFIG_CANDIDATE |
| 619 | 0.40 | Default correlation: shipping↔energy | CONFIG_CANDIDATE |
| 620 | 0.30 | Default correlation: shipping↔industrial | CONFIG_CANDIDATE |
| 623 | 0.20 | Default correlation: consumer↔industrial | CONFIG_CANDIDATE |
| 624 | 0.15 | Default correlation: consumer↔energy | CONFIG_CANDIDATE |

---

## File: `internal/industry/dynamic_env.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 28 | 90 | DynamicEnvModulator windowDays default | CONFIG_CANDIDATE |
| 58 | 90 | UpdateRollingBaseline: fallback window | CONFIG_CANDIDATE |

---

## File: `internal/industry/seasonality.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 81 | 1.15 | spring_festival: AdjustmentFactor | CONFIG_CANDIDATE |
| 82 | 0.70 | spring_festival: HistoricalAccuracy | CONFIG_CANDIDATE |
| 83 | 0.032 | spring_festival: AvgMarketReturn | CONFIG_CANDIDATE |
| 96 | 1.10 | earnings_window: AdjustmentFactor | CONFIG_CANDIDATE |
| 97 | 0.55 | earnings_window: HistoricalAccuracy | CONFIG_CANDIDATE |
| 98 | 0.015 | earnings_window: AvgMarketReturn | CONFIG_CANDIDATE |
| 111 | 1.20 | dividend_season: AdjustmentFactor | CONFIG_CANDIDATE |
| 112 | 0.65 | dividend_season: HistoricalAccuracy | CONFIG_CANDIDATE |
| 113 | 0.025 | dividend_season: AvgMarketReturn | CONFIG_CANDIDATE |
| 126 | 1.25 | tech_peak_season: AdjustmentFactor | CONFIG_CANDIDATE |
| 127 | 0.75 | tech_peak_season: HistoricalAccuracy | CONFIG_CANDIDATE |
| 128 | 0.085 | tech_peak_season: AvgMarketReturn | CONFIG_CANDIDATE |
| 141 | 1.10 | earnings_verification: AdjustmentFactor | CONFIG_CANDIDATE |
| 142 | 0.60 | earnings_verification: HistoricalAccuracy | CONFIG_CANDIDATE |
| 143 | 0.020 | earnings_verification: AvgMarketReturn | CONFIG_CANDIDATE |
| 156 | 1.12 | year_end_rally: AdjustmentFactor | CONFIG_CANDIDATE |
| 157 | 0.58 | year_end_rally: HistoricalAccuracy | CONFIG_CANDIDATE |
| 158 | 0.018 | year_end_rally: AvgMarketReturn | CONFIG_CANDIDATE |
| 171 | 1.08 | summer_electricity: AdjustmentFactor | CONFIG_CANDIDATE |
| 172 | 0.62 | summer_electricity: HistoricalAccuracy | CONFIG_CANDIDATE |
| 173 | 0.012 | summer_electricity: AvgMarketReturn | CONFIG_CANDIDATE |
| 270 | 0.01 | GetPatternAdjustment: adjustment floor | CONFIG_CANDIDATE |

---

## File: `internal/industry/risk.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 81 | -0.03 | AsymmetricRiskConfig: BadNewsThreshold | CONFIG_CANDIDATE |
| 82 | 0.05 | AsymmetricRiskConfig: GoodNewsThreshold | CONFIG_CANDIDATE |
| 83 | 30 | AsymmetricRiskConfig: ReactionTimeMinutes | CONFIG_CANDIDATE |
| 84 | 2.0 | AsymmetricRiskConfig: VolumeSpikeMultiplier | CONFIG_CANDIDATE |
| 97-107 | 0.95/0.93/0.88/... | DefaultNewsSources: reliability scores | CONFIG_CANDIDATE |
| 258 | 24h | News latency: max risk at 24h gap | CONFIG_CANDIDATE |
| 274 | -0.05 | NewsLatency: ImpactEstimate multiplier | CONFIG_CANDIDATE |
| 275 | 0.80 | NewsLatency: hardcoded Confidence | CONFIG_CANDIDATE |
| 297-303 | 0.10/0.07/0.05 | Asymmetric: price drop severity thresholds | CONFIG_CANDIDATE |
| 313 | /3.0 | Asymmetric: confidence volume normalization | CONFIG_CANDIDATE |

---

## File: `internal/narrative/knowledge_base.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 159 | 1.0 | InvestmentModel: weight for hawkish_fed_model | CONFIG_CANDIDATE |
| 169 | 1.0 | InvestmentModel: weight for ai_supercycle_model | CONFIG_CANDIDATE |
| 179 | 1.0 | InvestmentModel: weight for geopolitical_hedge_model | CONFIG_CANDIDATE |
| 189 | 1.0 | InvestmentModel: weight for taiwan_political_risk_model | CONFIG_CANDIDATE |
| 199 | 1.0 | InvestmentModel: weight for semiconductor_cycle_model | CONFIG_CANDIDATE |
| 209 | 1.0 | InvestmentModel: weight for seasonal_model | CONFIG_CANDIDATE |
| 219 | 1.0 | InvestmentModel: weight for election_model | CONFIG_CANDIDATE |
| 318 | 30 | EvaluateModels: lookback days | CONFIG_CANDIDATE |
| 319 | 5 | EvaluateModels: holdWindow days | CONFIG_CANDIDATE |
| 503 | -0.6 | US_rates_up: default Sentiment | CONFIG_CANDIDATE |
| 527 | 0.8 | AI_capex_surge: default Sentiment | CONFIG_CANDIDATE |
| 555 | -0.8 | geopolitical_risk_spike: default Sentiment | CONFIG_CANDIDATE |
| 580 | -0.5 | oil_price_shock: default Sentiment | CONFIG_CANDIDATE |
| 608 | -0.6 | JPY_carry_unwind: default Sentiment | CONFIG_CANDIDATE |
| 629 | -0.7 | USD_TWD_volatility: sentiment for depreciation | CONFIG_CANDIDATE |
| 659 | -0.8 | taiwan_political_risk: default Sentiment | CONFIG_CANDIDATE |
| 691 | -0.6 | semiconductor_downturn: default Sentiment | CONFIG_CANDIDATE |
| 718 | 0.3 | spring_festival: Sentiment | CONFIG_CANDIDATE |
| 719 | 0.65 | spring_festival: Confidence | CONFIG_CANDIDATE |
| 738 | 0.60 | election_cycle: Confidence | CONFIG_CANDIDATE |
| 757 | 0.55 | earnings_blackout: Confidence | CONFIG_CANDIDATE |
| 759 | 0.55 | earnings_blackout: HitRate (hardcoded, not from template) | CONFIG_CANDIDATE |
| 776 | 0.5 | tech_peak_season: Sentiment | CONFIG_CANDIDATE |
| 777 | 0.75 | tech_peak_season: Confidence | CONFIG_CANDIDATE |
| 779 | 0.75 | tech_peak_season: HitRate (hardcoded, not from template) | CONFIG_CANDIDATE |
| 795 | 0.2 | year_end_window_dressing: Sentiment | CONFIG_CANDIDATE |
| 796 | 0.58 | year_end_window_dressing: Confidence | CONFIG_CANDIDATE |
| 798 | 0.58 | year_end_window_dressing: HitRate (hardcoded, not from template) | CONFIG_CANDIDATE |
| 824 | -0.5 | retail_institutional_divergence: Sentiment | CONFIG_CANDIDATE |
| 850 | 0.95 | computeDeviationConfidence: confidence ceiling | CONFIG_CANDIDATE |

---

## File: `internal/narrative/taiwan_stress_index.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 43 | 5.0 | stressScaleDXY: DXY change% → pressure score | CONFIG_CANDIDATE |
| 44 | 2.0 | stressScaleUS10Y: yield absolute → pressure score | CONFIG_CANDIDATE |
| 45 | 10.0 | stressScaleForeignFlow: foreign net sell → pressure score | CONFIG_CANDIDATE |
| 46 | 2.5 | stressScaleVIX: VIX → pressure score (100/40) | CONFIG_CANDIDATE |
| 47 | 10.0 | stressScaleJPY: JPY change% → pressure score | CONFIG_CANDIDATE |
| 48 | 1.0 | stressScaleGeopolitical: risk intensity (already 0-100) | CONFIG_CANDIDATE |
| 49 | 2.0 | stressScaleOil: oil change → pressure score | CONFIG_CANDIDATE |
| 50 | 2.0 | stressScaleGold: gold change → pressure score | CONFIG_CANDIDATE |
| 53 | 0.13 | stressWeightDXY: DXY factor weight | CONFIG_CANDIDATE |
| 54 | 0.18 | stressWeightUS10Y: US10Y factor weight | CONFIG_CANDIDATE |
| 55 | 0.22 | stressWeightForeignFlow: foreign flow weight | CONFIG_CANDIDATE |
| 56 | 0.13 | stressWeightVIX: VIX weight | CONFIG_CANDIDATE |
| 57 | 0.08 | stressWeightJPY: JPY weight | CONFIG_CANDIDATE |
| 58 | 0.13 | stressWeightGeopolitical: geopolitical weight | CONFIG_CANDIDATE |
| 59 | 0.07 | stressWeightOil: crude oil weight | CONFIG_CANDIDATE |
| 60 | 0.06 | stressWeightGold: gold weight | CONFIG_CANDIDATE |
| 63 | 70.0 | stressThresholdCrisis: red line | CONFIG_CANDIDATE |
| 64 | 50.0 | stressThresholdHigh: orange line | CONFIG_CANDIDATE |
| 65 | 30.0 | stressThresholdAlert: yellow line | CONFIG_CANDIDATE |

---

## File: `internal/narrative/ingestor.go`

(Hardcoded sentiment and confidence values are covered in `knowledge_base.go` above, as ingestor delegates to detect* functions in knowledge_base.go.)

---

## File: `internal/risk/macro_aware_drawdown.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 49-54 | 0.0/0.15/0.35/0.60/0.90 | DefaultDrawdownLevels: Percentage per action | CONFIG_CANDIDATE |
| 49-54 | 1.0/0.85/0.65/0.40/0.10 | DefaultDrawdownLevels: MaxExposure per action | CONFIG_CANDIDATE |

---

## File: `internal/risk/capital_controller.go`

| Line | Value | Context | Classification |
|------|-------|---------|---------------|
| 79 | 1.0 | GetCapitalLimit: fallback (no phase limit configured) | CONFIG_CANDIDATE |

---

## Statistics

| Category | Count |
|----------|-------|
| Total entries cataloged | 65 |
| CONFIG_CANDIDATE | 60 |
| LEGITIMATE_CONSTANT | 5 |
| Files scanned | 20+ |
| Directories covered | 5 |

## Key Findings

1. **FactorWeightEngine base weights** (lines 23-30): All 8 factor weights are hardcoded and directly impact portfolio construction.

2. **Regime-based adjustments** (lines 46-56): The regime-specific delta adjustments (±0.02 to ±0.05) are small magic numbers that significantly impact factor tilts.

3. **Severity-to-delta mapping** (lines 144-153): The delta values (0.10/0.05/0.02/0.01) for narrative event severity levels are arbitrary and uncalibrated.

4. **Sector rotator macro adjustments** (sector_rotator.go lines 107-129): Large hardcoded allocation shifts (0.25 cash in Red risk) that bypass any config system.

5. **CycleTracker default metrics** (cycle.go lines 124-209): All 12 industries have hardcoded default financial metrics used until real data arrives.

6. **TaiwanStressIndex constants** (taiwan_stress_index.go lines 43-65): 8 scaling factors + 8 weights + 3 thresholds; these determine when the system classifies market as "alert"/"high"/"crisis".

7. **DefaultCorrelationMatrix** (linkage.go lines 595-626): 25+ pair-wise correlation coefficients hardcoded for Taiwan industry relationships.

8. **SeasonalPattern adjustments** (seasonality.go lines 69-177): 7 seasonal patterns with hardcoded AdjustmentFactor (1.08-1.25), HistoricalAccuracy (0.55-0.75), and AvgMarketReturn.

9. **Narrative event sentiments** (knowledge_base.go): Each detect*Event() creates events with hardcoded Sentiment (-0.8 to 0.8) and some with hardcoded Confidence values.

10. **Darwinian weight bounds** (darwinian_weights.go lines 21-31): The [0.3, 2.5] range is hardcoded and silently clamps all agent weights.
