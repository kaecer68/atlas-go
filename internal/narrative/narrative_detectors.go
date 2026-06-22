package narrative

import (
	"fmt"
	"math"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func (ne *NarrativeEngine) DetectEvents(data MarketNarrativeData) []NarrativeEvent {
	var events []NarrativeEvent
	if evt := detectUSRatesEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectUSRatesDownEvent(data); evt != nil {
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
	if evt := detectUSDTWDEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectTaiwanPoliticalRiskEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectSemiconductorDownturnEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectSeasonalEvent(); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectRetailDivergenceEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectGoldRallyKBEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectDollarSurgeKBEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectEarningsSurpriseKBEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectInflationSpikeKBEvent(data); evt != nil {
		events = append(events, *evt)
	}
	// Extended macro event detectors (from previously unused snapshot data)
	if evt := detectCPIInflationEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectBDIShippingEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectCopperIndustrialEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectTaiwanExportBoomEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectSOXSemiconductorCycleEvent(data); evt != nil {
		events = append(events, *evt)
	}
	if evt := detectDRAMMemoryCycleEvent(data); evt != nil {
		events = append(events, *evt)
	}
	return events
}

type MarketNarrativeData struct {
	US10YChangeBps                float64
	DXYChangePct                  float64
	VIXLevel                      float64
	USD_TWD_ChangePct             float64
	OilChangePct                  float64
	GoldChangePct                 float64
	GoldLevel                     float64 // absolute gold price level (e.g. 2300)
	JPY_ChangePct                 float64
	JPYLevel                      float64 // absolute USD/JPY level (e.g. 155)
	AICapexSentiment              float64 // +1 bullish, -1 bearish
	GeopoliticalGPR               float64 // Geopolitical risk index level
	RetailInstitutionalDivergence float64 // + retail bullish, - retail bearish
	MarginZScore                  float64 // how extreme current margin balance is (reverse indicator)
	EarningsSurprisePct           float64 // actual earnings surprise percentage
	// Extended macro inputs (from snapshot data previously unused by narrative)
	CPIYoY                     float64 // consumer price index YoY %
	BDIChangePct               float64 // Baltic Dry Index change %
	CopperChangePct            float64 // copper price change %
	ExportElectronicsChangePct float64 // Taiwan electronics export change %
	SOXIndexChangePct          float64 // Philadelphia Semiconductor Index change %
	DRAMSpotPriceChangePct     float64 // DRAM spot price change %
	SPXIndexChangePct          float64 // S&P 500 index change %
	NDXIndexChangePct          float64 // Nasdaq Composite index change %
	DJIIndexChangePct          float64 // Dow Jones index change %
	TSMADRChangePct            float64 // TSMC ADR (TSM) change %
}

// detectUSRatesEvent is the KB-pipeline detector (MarketNarrativeData input).
// INTENTIONALLY NOT MERGED with detectUSRatesEventFromSnapshot in ingestor.go.
// Reason: The KB version uses actual DXY input from MarketNarrativeData,
// while the ingestor version uses a single MacroDataPoint with ChangePct as
// a proxy. They are semantically different — KB is the authoritative signal,
// ingestor is a degraded-mode proxy when the full narrative data isn't available.
func detectUSRatesEvent(data MarketNarrativeData) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if data.US10YChangeBps > params.US10YChangeBpsThreshold.Value || data.DXYChangePct > params.DXYChangePctThreshold.Value {
		confidenceUS10Y := computeDeviationConfidence(data.US10YChangeBps, params.US10YChangeBpsThreshold.Value, params.ConfidenceBaseUSRates.Value, params.ConfidenceDeviationCeiling.Value)
		confidenceDXY := computeDeviationConfidence(data.DXYChangePct, params.DXYChangePctThreshold.Value, params.ConfidenceBaseUSRates.Value, params.ConfidenceDeviationCeiling.Value)
		confidence := confidenceUS10Y
		if confidenceDXY > confidence {
			confidence = confidenceDXY
		}
		now := time.Now().UTC()
		dur := getThemeDuration("US_rates_up")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-us-rates-%d", nowUnix()),
			Theme:            "US_rates_up",
			Region:           "US",
			Sentiment:        -0.6,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("US_rates_up"),
			CapitalFlow:      "flight_to_USD",
			TimeWindow:       "1_week",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "medium",
			Status:           "active",
			SourceData: map[string]float64{
				"us10y_change_bps": data.US10YChangeBps,
				"dxy_change_pct":   data.DXYChangePct,
			},
		}
	}
	return nil
}

// detectUSRatesDownEvent detects dovish Fed signals: US10Y yield drop OR DXY weakness.
func detectUSRatesDownEvent(data MarketNarrativeData) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	// Thresholds: US10Y drops > 5 bps OR DXY drops > 0.8%.
	const us10yDownThreshold = -5.0
	const dxyDownThreshold = -0.8
	triggered := data.US10YChangeBps < us10yDownThreshold || data.DXYChangePct < dxyDownThreshold
	if !triggered {
		return nil
	}
	confidenceUS10Y := computeDeviationConfidence(math.Abs(data.US10YChangeBps), math.Abs(us10yDownThreshold), params.ConfidenceBaseUSRates.Value, params.ConfidenceDeviationCeiling.Value)
	confidenceDXY := computeDeviationConfidence(math.Abs(data.DXYChangePct), math.Abs(dxyDownThreshold), params.ConfidenceBaseUSRates.Value, params.ConfidenceDeviationCeiling.Value)
	confidence := confidenceUS10Y
	if confidenceDXY > confidence {
		confidence = confidenceDXY
	}
	now := time.Now().UTC()
	dur := getThemeDuration("US_rates_down")
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-us-rates-down-%d", nowUnix()),
		Theme:            "US_rates_down",
		Region:           "US",
		Sentiment:        0.5,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("US_rates_down"),
		CapitalFlow:      "risk_on_liquidity",
		TimeWindow:       "1_week",
		Timestamp:        now,
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Severity:         "medium",
		Status:           "active",
		SourceData: map[string]float64{
			"us10y_change_bps": data.US10YChangeBps,
			"dxy_change_pct":   data.DXYChangePct,
		},
	}
}

// buildAICapexSurgeEvent creates an AI_capex_surge NarrativeEvent from a sentiment score.
// Shared by both the KB pipeline (via MarketNarrativeData) and the ingestor pipeline.
func buildAICapexSurgeEvent(sentiment float64, ts time.Time) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if sentiment > params.AICapexSentimentThreshold.Value {
		confidence := computeDeviationConfidence(sentiment, params.AICapexSentimentThreshold.Value, params.ConfidenceBaseAICapex.Value, params.ConfidenceDeviationCeiling.Value)
		dur := getThemeDuration("AI_capex_surge")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-ai-capex-%d", ts.UnixNano()),
			Theme:            "AI_capex_surge",
			Region:           "US",
			Sentiment:        0.8,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("AI_capex_surge"),
			CapitalFlow:      "tech_capex_inflow",
			TimeWindow:       "1_month",
			Timestamp:        ts,
			Duration:         dur,
			ExpiresAt:        ts.Add(dur),
			Severity:         "high",
			Status:           "active",
			SourceData: map[string]float64{
				"ai_capex_sentiment": sentiment,
			},
		}
	}
	return nil
}

func detectAICapexEvent(data MarketNarrativeData) *NarrativeEvent {
	return buildAICapexSurgeEvent(data.AICapexSentiment, time.Now().UTC())
}

func NewEarningsSurpriseEvent(surprisePct float64) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	confidence := computeDeviationConfidence(math.Abs(surprisePct), 5.0, params.EarningsSurpriseConfidence.Value, params.ConfidenceDeviationCeiling.Value)

	sentiment := 0.0
	capitalFlow := ""
	if surprisePct > 0 {
		sentiment = 0.7
		capitalFlow = "earnings_beat"
	} else {
		sentiment = -0.7
		capitalFlow = "earnings_miss"
	}

	now := time.Now().UTC()
	dur := getThemeDuration("earnings_surprise")
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-earn-%d", nowUnix()),
		Theme:            "earnings_surprise",
		Region:           "TW",
		Sentiment:        sentiment,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("earnings_surprise"),
		CapitalFlow:      capitalFlow,
		TimeWindow:       "1_week",
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Timestamp:        now,
		Severity:         "high",
		Status:           "active",
		SourceData: map[string]float64{
			"surprise_pct": surprisePct,
		},
	}
}

// detectGeopoliticalRiskEvent is the KB-pipeline detector (GPR + Gold input).
// INTENTIONALLY NOT MERGED with detectGeopoliticalRiskEventFromSnapshot in ingestor.go.
// Reason: The KB version uses the GeopoliticalGPR index and GoldChangePct directly
// from MarketNarrativeData, while the ingestor version is a proxy-based v1 detector
// that substitutes gold/VIX/USDTWD for the missing GPR data. Different signal
// quality — KB is authoritative when GPR is available, ingestor is fallback.
func detectGeopoliticalRiskEvent(data MarketNarrativeData) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if data.GeopoliticalGPR > params.GeopoliticalGPRThreshold.Value || data.GoldChangePct > params.GoldChangePctThreshold.Value {
		confidenceGPR := computeDeviationConfidence(data.GeopoliticalGPR, params.GeopoliticalGPRThreshold.Value, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
		confidenceGold := computeDeviationConfidence(data.GoldChangePct, params.GoldChangePctThreshold.Value, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
		confidence := confidenceGPR
		if confidenceGold > confidence {
			confidence = confidenceGold
		}
		now := time.Now().UTC()
		dur := getThemeDuration("geopolitical_risk_spike")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-geo-%d", nowUnix()),
			Theme:            "geopolitical_risk_spike",
			Region:           "Global",
			Sentiment:        -0.8,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("geopolitical_risk_spike"),
			CapitalFlow:      "risk_off",
			TimeWindow:       "immediate",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "high",
			Status:           "active",
			SourceData: map[string]float64{
				"geopolitical_gpr": data.GeopoliticalGPR,
				"gold_change_pct":  data.GoldChangePct,
			},
		}
	}
	return nil
}

// buildOilShockEvent creates an oil_price_shock NarrativeEvent from a change percentage.
// Shared by both the KB pipeline (via MarketNarrativeData) and the ingestor pipeline (via MacroDataPoint).
func buildOilShockEvent(changePct float64, ts time.Time) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	threshold := params.OilChangePctThreshold.Value
	if changePct > threshold || changePct < -threshold {
		confidence := computeDeviationConfidence(changePct, threshold, params.ConfidenceBaseOilShock.Value, params.ConfidenceDeviationCeiling.Value)
		dur := getThemeDuration("oil_price_shock")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-oil-%d", ts.UnixNano()),
			Theme:            "oil_price_shock",
			Region:           "Global",
			Sentiment:        -0.5,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("oil_price_shock"),
			CapitalFlow:      "inflation_reprice",
			TimeWindow:       "1_week",
			Timestamp:        ts,
			Duration:         dur,
			ExpiresAt:        ts.Add(dur),
			Severity:         "medium",
			Status:           "active",
			SourceData: map[string]float64{
				"oil_change_pct": changePct,
			},
		}
	}
	return nil
}

func detectOilShockEvent(data MarketNarrativeData) *NarrativeEvent {
	return buildOilShockEvent(data.OilChangePct, time.Now().UTC())
}

// buildJPYCarryUnwindEvent creates a JPY_carry_unwind NarrativeEvent from JPY change and VIX level.
// Shared by both the KB pipeline (via MarketNarrativeData) and the ingestor pipeline (via MacroDataPoint).
func buildJPYCarryUnwindEvent(jpyChangePct, vixLevel float64, ts time.Time) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if jpyChangePct > params.JPYChangePctThreshold.Value || vixLevel > params.VIXLevelThreshold.Value {
		confidenceJPY := computeDeviationConfidence(jpyChangePct, params.JPYChangePctThreshold.Value, params.ConfidenceBaseJPYCarry.Value, params.ConfidenceDeviationCeiling.Value)
		confidenceVIX := computeDeviationConfidence(vixLevel, params.VIXLevelThreshold.Value, params.ConfidenceBaseJPYCarry.Value, params.ConfidenceDeviationCeiling.Value)
		confidence := confidenceJPY
		if confidenceVIX > confidence {
			confidence = confidenceVIX
		}
		dur := getThemeDuration("JPY_carry_unwind")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-jpy-%d", ts.UnixNano()),
			Theme:            "JPY_carry_unwind",
			Region:           "JP",
			Sentiment:        -0.6,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("JPY_carry_unwind"),
			CapitalFlow:      "global_liquidity_drain",
			TimeWindow:       "immediate",
			Timestamp:        ts,
			Duration:         dur,
			ExpiresAt:        ts.Add(dur),
			Severity:         "medium",
			Status:           "active",
			SourceData: map[string]float64{
				"jpy_change_pct": jpyChangePct,
				"vix_level":      vixLevel,
			},
		}
	}
	return nil
}

func detectJPYCarryUnwindEvent(data MarketNarrativeData) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	// Hybrid trigger: JPY level > 155 (structural carry risk) OR change > threshold OR VIX > threshold.
	const jpyLevelThreshold = 155.0
	triggered := data.JPYLevel > jpyLevelThreshold ||
		data.JPY_ChangePct > params.JPYChangePctThreshold.Value ||
		data.VIXLevel > params.VIXLevelThreshold.Value
	if !triggered {
		return nil
	}
	confidenceLevel := computeDeviationConfidence(data.JPYLevel, jpyLevelThreshold, params.ConfidenceBaseJPYCarry.Value, params.ConfidenceDeviationCeiling.Value)
	confidenceJPY := computeDeviationConfidence(data.JPY_ChangePct, params.JPYChangePctThreshold.Value, params.ConfidenceBaseJPYCarry.Value, params.ConfidenceDeviationCeiling.Value)
	confidenceVIX := computeDeviationConfidence(data.VIXLevel, params.VIXLevelThreshold.Value, params.ConfidenceBaseJPYCarry.Value, params.ConfidenceDeviationCeiling.Value)
	confidence := confidenceLevel
	if confidenceJPY > confidence {
		confidence = confidenceJPY
	}
	if confidenceVIX > confidence {
		confidence = confidenceVIX
	}
	ts := time.Now().UTC()
	dur := getThemeDuration("JPY_carry_unwind")
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-jpy-%d", ts.UnixNano()),
		Theme:            "JPY_carry_unwind",
		Region:           "JP",
		Sentiment:        -0.6,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("JPY_carry_unwind"),
		CapitalFlow:      "global_liquidity_drain",
		TimeWindow:       "immediate",
		Timestamp:        ts,
		Duration:         dur,
		ExpiresAt:        ts.Add(dur),
		Severity:         "medium",
		Status:           "active",
		SourceData: map[string]float64{
			"jpy_level":      data.JPYLevel,
			"jpy_change_pct": data.JPY_ChangePct,
			"vix_level":      data.VIXLevel,
		},
	}
}

// buildUSDTWDVolatilityEvent creates a USD_TWD_volatility NarrativeEvent from a change percentage.
// Shared by both the KB pipeline (via MarketNarrativeData) and the ingestor pipeline (via MacroDataPoint).
func buildUSDTWDVolatilityEvent(changePct float64, ts time.Time) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if math.Abs(changePct) > params.USDTWDChangePctThreshold.Value {
		sentiment := -0.5
		if changePct > 0 {
			sentiment = -0.7
		}
		confidence := computeDeviationConfidence(changePct, params.USDTWDChangePctThreshold.Value, params.ConfidenceBaseTaiwanStress.Value, params.ConfidenceDeviationCeiling.Value)
		dur := getThemeDuration("USD_TWD_volatility")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-usd-twd-%d", ts.UnixNano()),
			Theme:            "USD_TWD_volatility",
			Region:           "TW",
			Sentiment:        sentiment,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("USD_TWD_volatility"),
			CapitalFlow:      "fx_driven_outflow",
			TimeWindow:       "1_week",
			Timestamp:        ts,
			Duration:         dur,
			ExpiresAt:        ts.Add(dur),
			Severity:         "medium",
			Status:           "active",
			SourceData: map[string]float64{
				"usd_twd_change_pct": changePct,
			},
		}
	}
	return nil
}

func detectUSDTWDEvent(data MarketNarrativeData) *NarrativeEvent {
	return buildUSDTWDVolatilityEvent(data.USD_TWD_ChangePct, time.Now().UTC())
}

func detectTaiwanPoliticalRiskEvent(data MarketNarrativeData) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if data.GeopoliticalGPR > params.GeopoliticalGPRThreshold.Value {
		confidence := computeDeviationConfidence(data.GeopoliticalGPR, params.GeopoliticalGPRThreshold.Value, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
		now := time.Now().UTC()
		dur := getThemeDuration("taiwan_political_risk")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-tw-pol-%d", nowUnix()),
			Theme:            "taiwan_political_risk",
			Region:           "TW",
			Sentiment:        -0.8,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("taiwan_political_risk"),
			CapitalFlow:      "risk_off",
			TimeWindow:       "immediate",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "high",
			Status:           "active",
			SourceData: map[string]float64{
				"geopolitical_gpr": data.GeopoliticalGPR,
			},
		}
	}
	return nil
}

// detectSemiconductorDownturnEvent is the KB-pipeline detector (multi-factor).
// INTENTIONALLY NOT MERGED with detectSemiconductorEventFromSnapshot in ingestor.go.
// Reason: These are COMPLETELY DIFFERENT detectors. The KB version uses VIX + DXY +
// AICapexSentiment as a multi-factor macro signal of semiconductor stress, while
// the ingestor version uses ExportElectronics ChangePct as a single export-data
// indicator. They detect different aspects of the semiconductor cycle.
func detectSemiconductorDownturnEvent(data MarketNarrativeData) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	// Relaxed from 3-factor AND to 2-of-3 to avoid silent failure during AI capex boom.
	condVIX := data.VIXLevel > params.VIXLevelThreshold.Value
	condDXY := data.DXYChangePct > params.DXYChangePctThreshold.Value
	condAI := data.AICapexSentiment < params.AICapexNegativeSentimentThreshold.Value
	triggered := (condVIX && condDXY) || (condVIX && condAI) || (condDXY && condAI)
	if !triggered {
		return nil
	}
	confidenceVIX := computeDeviationConfidence(data.VIXLevel, params.VIXLevelThreshold.Value, params.ConfidenceBaseTSMCRevenue.Value, params.ConfidenceDeviationCeiling.Value)
	confidenceDXY := computeDeviationConfidence(data.DXYChangePct, params.DXYChangePctThreshold.Value, params.ConfidenceBaseTSMCRevenue.Value, params.ConfidenceDeviationCeiling.Value)
	confidenceAI := computeDeviationConfidence(math.Abs(data.AICapexSentiment), math.Abs(params.AICapexNegativeSentimentThreshold.Value), params.ConfidenceBaseTSMCRevenue.Value, params.ConfidenceDeviationCeiling.Value)
	confidence := confidenceVIX
	if confidenceDXY > confidence {
		confidence = confidenceDXY
	}
	if confidenceAI > confidence {
		confidence = confidenceAI
	}
	now := time.Now().UTC()
	dur := getThemeDuration("semiconductor_downturn")
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-semi-dt-%d", nowUnix()),
		Theme:            "semiconductor_downturn",
		Region:           "TW",
		Sentiment:        -0.6,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("semiconductor_downturn"),
		CapitalFlow:      "tech_capex_slowdown",
		TimeWindow:       "1_month",
		Timestamp:        now,
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Severity:         "high",
		Status:           "active",
		SourceData: map[string]float64{
			"vix_level":          data.VIXLevel,
			"dxy_change_pct":     data.DXYChangePct,
			"ai_capex_sentiment": data.AICapexSentiment,
		},
	}
}

func detectSeasonalEvent() *NarrativeEvent {
	now := time.Now().UTC()
	month := now.Month()
	day := now.Day()
	params := config.GetParametersConfig().Narrative

	if (month == 1 && day >= 15) || (month == 2 && day <= 15) {
		dur := getThemeDuration("spring_festival_season")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-spring-%d", nowUnix()),
			Theme:            "spring_festival_season",
			Region:           "TW",
			Sentiment:        0.3,
			Confidence:       params.SpringFestivalConfidence.Value,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          hitRateForTheme("spring_festival_season"),
			CapitalFlow:      "seasonal_rotation",
			TimeWindow:       "1_month",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "low",
			Status:           "active",
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	if (month == 1 && day <= 15) || (month == 12 && day >= 20) {
		dur := getThemeDuration("election_cycle")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-election-%d", nowUnix()),
			Theme:            "election_cycle",
			Region:           "TW",
			Sentiment:        -0.2,
			Confidence:       params.ElectionCycleConfidence.Value,
			ConfidenceSource: "calendar_political",
			HitRate:          hitRateForTheme("election_cycle"),
			CapitalFlow:      "policy_uncertainty",
			TimeWindow:       "1_month",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "low",
			Status:           "active",
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	if (month == 3 && day >= 1) || (month == 4 && day <= 15) {
		hitRate := params.EarningsBlackoutConfidence.Value
		dur := getThemeDuration("earnings_blackout")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-blackout-%d", nowUnix()),
			Theme:            "earnings_blackout",
			Region:           "TW",
			Sentiment:        0.1,
			Confidence:       hitRate,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          hitRate,
			CapitalFlow:      "pre_earnings_positioning",
			TimeWindow:       "1_month",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "low",
			Status:           "active",
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	if (month == 7 && day >= 1) || (month == 8) || (month == 9 && day <= 15) {
		hitRate := params.TechPeakSeasonConfidence.Value
		dur := getThemeDuration("tech_peak_season")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-tech-peak-%d", nowUnix()),
			Theme:            "tech_peak_season",
			Region:           "TW",
			Sentiment:        0.5,
			Confidence:       hitRate,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          hitRate,
			CapitalFlow:      "tech_capex_inflow",
			TimeWindow:       "2_months",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "low",
			Status:           "active",
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	// Dividend season: Jun–Aug (除權息旺季)
	if month >= 6 && month <= 8 {
		const dividendConfidence = 0.60
		dur := getThemeDuration("dividend_season")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-dividend-%d", nowUnix()),
			Theme:            "dividend_season",
			Region:           "TW",
			Sentiment:        0.3,
			Confidence:       dividendConfidence,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          hitRateForTheme("dividend_season"),
			CapitalFlow:      "dividend_rotation",
			TimeWindow:       "2_months",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "low",
			Status:           "active",
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	if month == 11 || month == 12 {
		hitRate := params.YearEndWindowDressingConfidence.Value
		dur := getThemeDuration("year_end_window_dressing")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-yearend-%d", nowUnix()),
			Theme:            "year_end_window_dressing",
			Region:           "TW",
			Sentiment:        0.2,
			Confidence:       hitRate,
			ConfidenceSource: "calendar_seasonal",
			HitRate:          hitRate,
			CapitalFlow:      "institutional_rebalancing",
			TimeWindow:       "2_months",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "low",
			Status:           "active",
			SourceData: map[string]float64{
				"month": float64(month),
				"day":   float64(day),
			},
		}
	}

	return nil
}

// detectRetailDivergenceEvent is the KB-pipeline detector (divergence quantification).
// INTENTIONALLY NOT MERGED with detectRetailDivergenceEventFromSnapshot in ingestor.go.
// Reason: The KB version uses MarginZScore + RetailInstitutionalDivergence from
// MarketNarrativeData (true divergence quantification), while the ingestor version
// uses a simpler heuristic (marginZScore > threshold && foreignNet < 0). The KB
// version provides more precise divergence measurement.
func detectRetailDivergenceEvent(data MarketNarrativeData) *NarrativeEvent {
	params := config.GetParametersConfig().Narrative
	if data.MarginZScore > params.RetailMarginZScoreThreshold.Value && data.RetailInstitutionalDivergence > params.RetailInstitutionalDivergenceThreshold.Value {
		confidenceZScore := computeDeviationConfidence(data.MarginZScore, params.RetailMarginZScoreThreshold.Value, params.ConfidenceBaseTaiwanStress.Value, params.ConfidenceDeviationCeiling.Value)
		confidenceDivergence := computeDeviationConfidence(data.RetailInstitutionalDivergence, params.RetailInstitutionalDivergenceThreshold.Value, params.ConfidenceBaseTaiwanStress.Value, params.ConfidenceDeviationCeiling.Value)
		confidence := confidenceZScore
		if confidenceDivergence > confidence {
			confidence = confidenceDivergence
		}
		now := time.Now().UTC()
		dur := getThemeDuration("retail_institutional_divergence")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-retail-div-%d", nowUnix()),
			Theme:            "retail_institutional_divergence",
			Region:           "TW",
			Sentiment:        -0.5,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("retail_institutional_divergence"),
			CapitalFlow:      "crowding_risk",
			TimeWindow:       "immediate",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "medium",
			Status:           "active",
			SourceData: map[string]float64{
				"margin_zscore":                   data.MarginZScore,
				"retail_institutional_divergence": data.RetailInstitutionalDivergence,
			},
		}
	}
	return nil
}

// detectGoldRallyKBEvent is the KB-pipeline detector for gold_rally (GoldChangePct input).
// INTENTIONALLY NOT MERGED with detectGoldRallyEventFromSnapshot in ingestor.go.
// Reason: The KB version uses GoldChangePct from MarketNarrativeData directly, while the
// ingestor version uses a MacroDataPoint with ChangePct. Different input shapes, same theme.
func detectGoldRallyKBEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.GoldChangePct > 3.0 {
		confidence := data.GoldChangePct / 5.0
		if confidence > 1.0 {
			confidence = 1.0
		}
		now := time.Now().UTC()
		dur := getThemeDuration("gold_rally")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-gold-rally-%d", nowUnix()),
			Theme:            "gold_rally",
			Region:           "COM",
			Sentiment:        0.6,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("gold_rally"),
			CapitalFlow:      "flight_to_gold",
			TimeWindow:       "1_week",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "medium",
			Status:           "active",
			SourceData: map[string]float64{
				"gold_change_pct": data.GoldChangePct,
			},
		}
	}
	return nil
}

// detectDollarSurgeKBEvent is the KB-pipeline detector for dollar_surge (DXYChangePct input).
// INTENTIONALLY NOT MERGED with detectDollarSurgeEventFromSnapshot in ingestor.go.
// Reason: The KB version uses DXYChangePct from MarketNarrativeData directly, while the
// ingestor version uses a MacroDataPoint with ChangePct. Different input shapes, same theme.
func detectDollarSurgeKBEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.DXYChangePct > 1.5 {
		confidence := data.DXYChangePct / 3.0
		if confidence > 1.0 {
			confidence = 1.0
		}
		now := time.Now().UTC()
		dur := getThemeDuration("dollar_surge")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-dollar-surge-%d", nowUnix()),
			Theme:            "dollar_surge",
			Region:           "US",
			Sentiment:        -0.5,
			Confidence:       confidence,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("dollar_surge"),
			CapitalFlow:      "flight_to_USD",
			TimeWindow:       "1_week",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "medium",
			Status:           "active",
			SourceData: map[string]float64{
				"dxy_change_pct": data.DXYChangePct,
			},
		}
	}
	return nil
}

// detectEarningsSurpriseKBEvent is the KB-pipeline detector for earnings_surprise.
// Uses AICapexSentiment as a proxy: strong AI capex sentiment correlates with earnings strength.
// INTENTIONALLY NOT MERGED with NewEarningsSurpriseEvent (which takes surprisePct).
// Reason: The KB version uses AICapexSentiment (sentiment proxy) while
// NewEarningsSurpriseEvent uses actual earnings surprise percentage. Different signal sources.
func detectEarningsSurpriseKBEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.AICapexSentiment > 0.7 {
		now := time.Now().UTC()
		dur := getThemeDuration("earnings_surprise")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-earn-%d", nowUnix()),
			Theme:            "earnings_surprise",
			Region:           "TW",
			Sentiment:        0.7,
			Confidence:       data.AICapexSentiment,
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("earnings_surprise"),
			CapitalFlow:      "earnings_beat",
			TimeWindow:       "1_week",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "high",
			Status:           "active",
			SourceData: map[string]float64{
				"ai_capex_sentiment": data.AICapexSentiment,
			},
		}
	}
	return nil
}

// detectInflationSpikeKBEvent is the KB-pipeline detector for inflation_spike.
// Triggers when VIX and DXY both signal inflation repricing (VIX > 25 proxy for
// inflation uncertainty, DXY strengthening as confirmation).
// INTENTIONALLY NOT MERGED with detectInflationSpikeEventFromSnapshot in ingestor.go.
// Reason: The KB version uses VIXLevel + DXYChangePct from MarketNarrativeData directly,
// while the ingestor version uses MacroDataPoint structs. Different input shapes, same theme.
func detectInflationSpikeKBEvent(data MarketNarrativeData) *NarrativeEvent {
	if data.VIXLevel > 25 && data.DXYChangePct > 1.0 {
		now := time.Now().UTC()
		dur := getThemeDuration("inflation_spike")
		return &NarrativeEvent{
			ID:               fmt.Sprintf("evt-inflation-spike-%d", nowUnix()),
			Theme:            "inflation_spike",
			Region:           "US",
			Sentiment:        -0.6,
			Confidence:       (data.VIXLevel-25)/15.0 + 0.5, // scales 25→0.5, 40→1.0
			ConfidenceSource: "deviation_based_v1",
			HitRate:          hitRateForTheme("inflation_spike"),
			CapitalFlow:      "inflation_reprice",
			TimeWindow:       "1_week",
			Timestamp:        now,
			Duration:         dur,
			ExpiresAt:        now.Add(dur),
			Severity:         "medium",
			Status:           "active",
			SourceData: map[string]float64{
				"vix_level":      data.VIXLevel,
				"dxy_change_pct": data.DXYChangePct,
			},
		}
	}
	return nil
}

// --- Extended macro event detectors (previously unused snapshot data) ---

// detectCPIInflationEvent triggers when US CPI YoY exceeds 3% (above target)
// or drops below 1% (deflation risk).
func detectCPIInflationEvent(data MarketNarrativeData) *NarrativeEvent {
	const cpiHighThreshold = 3.0
	const cpiLowThreshold = 1.0
	if data.CPIYoY <= 0 {
		return nil
	}
	triggered := data.CPIYoY > cpiHighThreshold || data.CPIYoY < cpiLowThreshold
	if !triggered {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	now := time.Now().UTC()
	dur := getThemeDuration("inflation_spike")
	sentiment := -0.5
	capitalFlow := "inflation_reprice"
	threshold := cpiHighThreshold
	if data.CPIYoY < cpiLowThreshold {
		threshold = cpiLowThreshold
		sentiment = -0.3
		capitalFlow = "deflation_risk"
	}
	confidence := computeDeviationConfidence(data.CPIYoY, threshold, params.ConfidenceBaseGeopolitical.Value, params.ConfidenceDeviationCeiling.Value)
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-cpi-%d", nowUnix()),
		Theme:            "inflation_spike",
		Region:           "US",
		Sentiment:        sentiment,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("inflation_spike"),
		CapitalFlow:      capitalFlow,
		TimeWindow:       "1_month",
		Timestamp:        now,
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Severity:         "medium",
		Status:           "active",
		SourceData: map[string]float64{
			"cpi_yoy": data.CPIYoY,
		},
	}
}

// detectBDIShippingEvent triggers on Baltic Dry Index moves >10% (global trade signal).
func detectBDIShippingEvent(data MarketNarrativeData) *NarrativeEvent {
	const bdiThreshold = 10.0
	if math.Abs(data.BDIChangePct) <= bdiThreshold {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	now := time.Now().UTC()
	dur := getThemeDuration("shipping_rate_spike")
	sentiment := 0.4
	capitalFlow := "shipping_demand"
	if data.BDIChangePct < 0 {
		sentiment = -0.4
		capitalFlow = "shipping_slump"
	}
	confidence := computeDeviationConfidence(data.BDIChangePct, bdiThreshold, params.ConfidenceBaseTaiwanStress.Value, params.ConfidenceDeviationCeiling.Value)
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-bdi-%d", nowUnix()),
		Theme:            "shipping_rate_spike",
		Region:           "Global",
		Sentiment:        sentiment,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("shipping_rate_spike"),
		CapitalFlow:      capitalFlow,
		TimeWindow:       "1_week",
		Timestamp:        now,
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Severity:         "medium",
		Status:           "active",
		SourceData: map[string]float64{
			"bdi_change_pct": data.BDIChangePct,
		},
	}
}

// detectCopperIndustrialEvent triggers on copper price moves >3% (Dr. Copper signal).
func detectCopperIndustrialEvent(data MarketNarrativeData) *NarrativeEvent {
	const copperThreshold = 3.0
	if math.Abs(data.CopperChangePct) <= copperThreshold {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	now := time.Now().UTC()
	dur := getThemeDuration("china_slowdown")
	sentiment := 0.5
	capitalFlow := "industrial_demand"
	if data.CopperChangePct < 0 {
		sentiment = -0.5
		capitalFlow = "industrial_slowdown"
	}
	confidence := computeDeviationConfidence(data.CopperChangePct, copperThreshold, params.ConfidenceBaseTaiwanStress.Value, params.ConfidenceDeviationCeiling.Value)
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-copper-%d", nowUnix()),
		Theme:            "china_slowdown",
		Region:           "Global",
		Sentiment:        sentiment,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("china_slowdown"),
		CapitalFlow:      capitalFlow,
		TimeWindow:       "1_week",
		Timestamp:        now,
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Severity:         "medium",
		Status:           "active",
		SourceData: map[string]float64{
			"copper_change_pct": data.CopperChangePct,
		},
	}
}

// detectTaiwanExportBoomEvent triggers on Taiwan electronics export moves >5%.
func detectTaiwanExportBoomEvent(data MarketNarrativeData) *NarrativeEvent {
	const exportThreshold = 5.0
	if math.Abs(data.ExportElectronicsChangePct) <= exportThreshold {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	now := time.Now().UTC()
	dur := getThemeDuration("taiwan_export_boom")
	sentiment := 0.6
	capitalFlow := "export_inflow"
	if data.ExportElectronicsChangePct < 0 {
		sentiment = -0.6
		capitalFlow = "export_outflow"
	}
	confidence := computeDeviationConfidence(data.ExportElectronicsChangePct, exportThreshold, params.ConfidenceBaseTaiwanStress.Value, params.ConfidenceDeviationCeiling.Value)
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-export-%d", nowUnix()),
		Theme:            "taiwan_export_boom",
		Region:           "TW",
		Sentiment:        sentiment,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("taiwan_export_boom"),
		CapitalFlow:      capitalFlow,
		TimeWindow:       "1_month",
		Timestamp:        now,
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Severity:         "medium",
		Status:           "active",
		SourceData: map[string]float64{
			"export_electronics_change_pct": data.ExportElectronicsChangePct,
		},
	}
}

// detectSOXSemiconductorCycleEvent triggers on SOX index moves >5%.
func detectSOXSemiconductorCycleEvent(data MarketNarrativeData) *NarrativeEvent {
	const soxThreshold = 5.0
	if math.Abs(data.SOXIndexChangePct) <= soxThreshold {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	now := time.Now().UTC()
	dur := getThemeDuration("semiconductor_cycle_peak")
	sentiment := 0.6
	capitalFlow := "tech_capex_inflow"
	if data.SOXIndexChangePct < 0 {
		sentiment = -0.6
		capitalFlow = "tech_capex_slowdown"
	}
	confidence := computeDeviationConfidence(data.SOXIndexChangePct, soxThreshold, params.ConfidenceBaseTSMCRevenue.Value, params.ConfidenceDeviationCeiling.Value)
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-sox-%d", nowUnix()),
		Theme:            "semiconductor_cycle_peak",
		Region:           "US",
		Sentiment:        sentiment,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("semiconductor_cycle_peak"),
		CapitalFlow:      capitalFlow,
		TimeWindow:       "1_month",
		Timestamp:        now,
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Severity:         "medium",
		Status:           "active",
		SourceData: map[string]float64{
			"sox_index_change_pct": data.SOXIndexChangePct,
		},
	}
}

// detectDRAMMemoryCycleEvent triggers on DRAM spot price moves >5%.
func detectDRAMMemoryCycleEvent(data MarketNarrativeData) *NarrativeEvent {
	const dramThreshold = 5.0
	if math.Abs(data.DRAMSpotPriceChangePct) <= dramThreshold {
		return nil
	}
	params := config.GetParametersConfig().Narrative
	now := time.Now().UTC()
	dur := getThemeDuration("semiconductor_cycle_peak")
	sentiment := 0.5
	capitalFlow := "memory_demand"
	if data.DRAMSpotPriceChangePct < 0 {
		sentiment = -0.5
		capitalFlow = "memory_oversupply"
	}
	confidence := computeDeviationConfidence(data.DRAMSpotPriceChangePct, dramThreshold, params.ConfidenceBaseTSMCRevenue.Value, params.ConfidenceDeviationCeiling.Value)
	return &NarrativeEvent{
		ID:               fmt.Sprintf("evt-dram-%d", nowUnix()),
		Theme:            "semiconductor_cycle_peak",
		Region:           "Global",
		Sentiment:        sentiment,
		Confidence:       confidence,
		ConfidenceSource: "deviation_based_v1",
		HitRate:          hitRateForTheme("semiconductor_cycle_peak"),
		CapitalFlow:      capitalFlow,
		TimeWindow:       "1_month",
		Timestamp:        now,
		Duration:         dur,
		ExpiresAt:        now.Add(dur),
		Severity:         "medium",
		Status:           "active",
		SourceData: map[string]float64{
			"dram_spot_price_change_pct": data.DRAMSpotPriceChangePct,
		},
	}
}

// computeDeviationConfidence calculates event confidence based on how far the observed value deviates from the threshold.
// Uses a base confidence plus a linear interpolation to ceiling.
func computeDeviationConfidence(observed, threshold, base, ceiling float64) float64 {
	if threshold <= 0 {
		return base
	}
	ratio := math.Abs(observed) / math.Abs(threshold)
	if ratio < 1.0 {
		return base
	}
	confidence := base + (ratio-1.0)*(ceiling-base)
	if confidence > ceiling {
		confidence = ceiling
	}
	return confidence
}

// getThemeDuration returns the typical duration for a narrative event theme.
// Uses the same durations as EventLifecycleManager.defaultDurations().
func getThemeDuration(theme string) time.Duration {
	switch theme {
	case "gold_rally":
		return 7 * 24 * time.Hour
	case "dollar_surge":
		return 7 * 24 * time.Hour
	case "earnings_surprise":
		return 10 * 24 * time.Hour
	case "inflation_spike":
		return 15 * 24 * time.Hour
	case "election_cycle":
		return 45 * 24 * time.Hour
	case "spring_festival_season":
		return 30 * 24 * time.Hour
	case "tech_peak_season":
		return 60 * 24 * time.Hour
	case "US_rates_up":
		return 14 * 24 * time.Hour
	case "US_rates_down":
		return 14 * 24 * time.Hour
	case "JPY_carry_unwind":
		return 5 * 24 * time.Hour
	case "geopolitical_risk_spike":
		return 10 * 24 * time.Hour
	case "semiconductor_downturn":
		return 30 * 24 * time.Hour
	case "taiwan_political_risk":
		return 10 * 24 * time.Hour
	case "USD_TWD_volatility":
		return 14 * 24 * time.Hour
	case "oil_price_shock":
		return 10 * 24 * time.Hour
	case "retail_institutional_divergence":
		return 7 * 24 * time.Hour
	case "dividend_season":
		return 60 * 24 * time.Hour
	case "year_end_window_dressing":
		return 45 * 24 * time.Hour
	case "shipping_rate_spike":
		return 14 * 24 * time.Hour
	case "china_slowdown":
		return 21 * 24 * time.Hour
	case "taiwan_export_boom":
		return 30 * 24 * time.Hour
	case "tariff_shock":
		return 14 * 24 * time.Hour
	default:
		return 7 * 24 * time.Hour
	}
}

var nowUnix = func() int64 {
	// Overridden in tests.
	return time.Now().UnixNano()
}

// NarrativeCalibrationReport summarizes the result of a self-calibration run.
type NarrativeCalibrationReport struct {
	Timestamp        time.Time         `json:"timestamp"`
	ModelsUpdated    int               `json:"models_updated"`
	TemplatesUpdated int               `json:"templates_updated"`
	Models           []InvestmentModel `json:"models"`
	Verdict          string            `json:"verdict"`
	Summary          string            `json:"summary"`
}

// SelfCalibrate evaluates model performance against replay data and updates
// model weights and template hit rates. It orchestrates the existing
// EvaluateModels → UpdateModelWeights → updateTemplateHitRates pipeline
// and produces a calibration report.
