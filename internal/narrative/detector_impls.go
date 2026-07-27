package narrative

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ============================================================================
// Stage 5 PR#2 — 24 Detector implementations
// ----------------------------------------------------------------------------
// Each Detector below is a thin wrapper over one (or, for seasonal themes,
// one filtered) legacy detect function in narrative_detectors.go / ingestor.go.
// The wrap is intentionally trivial so that:
//
//   1. No behavior change for existing callers (NarrativeEngine.DetectEvents,
//      MacroIngestor.Ingest) — they still call the legacy detect functions
//      directly and get back *NarrativeEvent as before.
//   2. Each Detector can be enabled / disabled independently via Registry.
//   3. The new template_detector_scan scheduler (PR#4) can drive them via
//      DetectorRegistry.RunAll without coupling to KB / ingestor pipelines.
//
// Source attribution (SourceKB vs SourceIngestor) follows the contract
// documented in narrative_detectors.go:108-113 — KB is authoritative when
// populated, ingestor is degraded-mode proxy. Tariff_shock is the one
// exception: a KB-pipeline equivalent would need a TradeNews data source
// that does not exist yet, so we route through the snapshot ingestor
// pipeline and mark the gap with a TODO.
// ============================================================================

// baseDetector supplies the Theme / Enabled / SetEnabled boilerplate shared
// by every concrete detector. Concrete detectors embed it and only need to
// implement Detect(ctx, in).
type baseDetector struct {
	theme   string
	source  Source
	enabled bool
}

func (d *baseDetector) Theme() string                                   { return d.theme }
func (d *baseDetector) Enabled() bool                                   { return d.enabled }
func (d *baseDetector) SetEnabled(b bool)                               { d.enabled = b }
func (d *baseDetector) PeriodWeight(period domain.MarketPeriod) float64 { return 1.0 }

// narrativeEventToResult projects a *NarrativeEvent from the legacy detect
// functions into the new DetectionResult shape. Returns nil for nil input
// so callers can `return narrativeEventToResult(evt, d.source), nil` without
// an explicit nil check.
func narrativeEventToResult(evt *NarrativeEvent, source Source) *DetectionResult {
	if evt == nil {
		return nil
	}
	return &DetectionResult{
		Theme:      evt.Theme,
		Severity:   severityFromString(evt.Severity),
		Confidence: evt.Confidence,
		DetectedAt: evt.Timestamp,
		Source:     source,
		Metadata:   sourceDataToMetadata(evt.SourceData),
	}
}

func severityFromString(s string) Severity {
	switch s {
	case "critical":
		return SeverityCritical
	case "high":
		return SeverityHigh
	case "low":
		return SeverityLow
	default:
		return SeverityMedium
	}
}

func sourceDataToMetadata(m map[string]float64) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func detectNow(in DetectorInput) time.Time {
	if !in.Now.IsZero() {
		return in.Now
	}
	return time.Now().UTC()
}

// ----------------------------------------------------------------------------
// KB-pipeline detectors (16) — wrap narrative_detectors.go functions that
// take MarketNarrativeData and return *NarrativeEvent.
// ----------------------------------------------------------------------------

type usRatesUpDetector struct{ baseDetector }

func newUSRatesUpDetector() *usRatesUpDetector {
	return &usRatesUpDetector{baseDetector{theme: "US_rates_up", source: SourceKB, enabled: true}}
}

func (d *usRatesUpDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectUSRatesEvent(in.MarketData), d.source), nil
}

// PeriodWeight: US_rates_up ×2 during plateau and turnaround_down.
func (d *usRatesUpDetector) PeriodWeight(period domain.MarketPeriod) float64 {
	switch period {
	case domain.PeriodPlateau, domain.PeriodTurnaroundDown:
		return 2.0
	default:
		return 1.0
	}
}

type usRatesDownDetector struct{ baseDetector }

func newUSRatesDownDetector() *usRatesDownDetector {
	return &usRatesDownDetector{baseDetector{theme: "US_rates_down", source: SourceKB, enabled: true}}
}

func (d *usRatesDownDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectUSRatesDownEvent(in.MarketData), d.source), nil
}

// PeriodWeight: JPY_carry_unwind ×2 during bull and black_swan.
func (d *jpyCarryUnwindDetector) PeriodWeight(period domain.MarketPeriod) float64 {
	switch period {
	case domain.PeriodBull, domain.PeriodBlackSwan:
		return 2.0
	default:
		return 1.0
	}
}

type jpyCarryUnwindDetector struct{ baseDetector }

func newJPYCarryUnwindDetector() *jpyCarryUnwindDetector {
	return &jpyCarryUnwindDetector{baseDetector{theme: "JPY_carry_unwind", source: SourceKB, enabled: true}}
}

func (d *jpyCarryUnwindDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectJPYCarryUnwindEvent(in.MarketData), d.source), nil
}

type aiCapexSurgeDetector struct{ baseDetector }

func newAICapexSurgeDetector() *aiCapexSurgeDetector {
	return &aiCapexSurgeDetector{baseDetector{theme: "AI_capex_surge", source: SourceKB, enabled: true}}
}

func (d *aiCapexSurgeDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectAICapexEvent(in.MarketData), d.source), nil
}

// PeriodWeight: AI_capex_surge ×2 during bull and turnaround_up.
func (d *aiCapexSurgeDetector) PeriodWeight(period domain.MarketPeriod) float64 {
	switch period {
	case domain.PeriodBull, domain.PeriodTurnaroundUp:
		return 2.0
	default:
		return 1.0
	}
}

type geopoliticalRiskDetector struct{ baseDetector }

func newGeopoliticalRiskDetector() *geopoliticalRiskDetector {
	return &geopoliticalRiskDetector{baseDetector{theme: "geopolitical_risk_spike", source: SourceKB, enabled: true}}
}

func (d *geopoliticalRiskDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectGeopoliticalRiskEvent(in.MarketData), d.source), nil
}

type oilShockDetector struct{ baseDetector }

func newOilShockDetector() *oilShockDetector {
	return &oilShockDetector{baseDetector{theme: "oil_price_shock", source: SourceKB, enabled: true}}
}

func (d *oilShockDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectOilShockEvent(in.MarketData), d.source), nil
}

type taiwanPoliticalRiskDetector struct{ baseDetector }

func newTaiwanPoliticalRiskDetector() *taiwanPoliticalRiskDetector {
	return &taiwanPoliticalRiskDetector{baseDetector{theme: "taiwan_political_risk", source: SourceKB, enabled: true}}
}

func (d *taiwanPoliticalRiskDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectTaiwanPoliticalRiskEvent(in.MarketData), d.source), nil
}

type usdTwdVolatilityDetector struct{ baseDetector }

func newUSDTWDVolatilityDetector() *usdTwdVolatilityDetector {
	return &usdTwdVolatilityDetector{baseDetector{theme: "USD_TWD_volatility", source: SourceKB, enabled: true}}
}

func (d *usdTwdVolatilityDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectUSDTWDEvent(in.MarketData), d.source), nil
}

type semiconductorDownturnDetector struct{ baseDetector }

func newSemiconductorDownturnDetector() *semiconductorDownturnDetector {
	return &semiconductorDownturnDetector{baseDetector{theme: "semiconductor_downturn", source: SourceKB, enabled: true}}
}

func (d *semiconductorDownturnDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectSemiconductorDownturnEvent(in.MarketData), d.source), nil
}

type retailInstitutionalDivergenceDetector struct{ baseDetector }

func newRetailInstitutionalDivergenceDetector() *retailInstitutionalDivergenceDetector {
	return &retailInstitutionalDivergenceDetector{baseDetector{theme: "retail_institutional_divergence", source: SourceKB, enabled: true}}
}

func (d *retailInstitutionalDivergenceDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectRetailDivergenceEvent(in.MarketData), d.source), nil
}

type goldRallyDetector struct{ baseDetector }

func newGoldRallyDetector() *goldRallyDetector {
	return &goldRallyDetector{baseDetector{theme: "gold_rally", source: SourceKB, enabled: true}}
}

func (d *goldRallyDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectGoldRallyKBEvent(in.MarketData), d.source), nil
}

type dollarSurgeDetector struct{ baseDetector }

func newDollarSurgeDetector() *dollarSurgeDetector {
	return &dollarSurgeDetector{baseDetector{theme: "dollar_surge", source: SourceKB, enabled: true}}
}

func (d *dollarSurgeDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectDollarSurgeKBEvent(in.MarketData), d.source), nil
}

type inflationSpikeDetector struct{ baseDetector }

func newInflationSpikeDetector() *inflationSpikeDetector {
	return &inflationSpikeDetector{baseDetector{theme: "inflation_spike", source: SourceKB, enabled: true}}
}

func (d *inflationSpikeDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectInflationSpikeKBEvent(in.MarketData), d.source), nil
}

type earningsSurpriseDetector struct{ baseDetector }

func newEarningsSurpriseDetector() *earningsSurpriseDetector {
	return &earningsSurpriseDetector{baseDetector{theme: "earnings_surprise", source: SourceKB, enabled: true}}
}

func (d *earningsSurpriseDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectEarningsSurpriseKBEvent(in.MarketData), d.source), nil
}

type shippingRateSpikeDetector struct{ baseDetector }

func newShippingRateSpikeDetector() *shippingRateSpikeDetector {
	return &shippingRateSpikeDetector{baseDetector{theme: "shipping_rate_spike", source: SourceKB, enabled: true}}
}

func (d *shippingRateSpikeDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectBDIShippingEvent(in.MarketData), d.source), nil
}

// china_slowdown uses copper-industrial change as the canonical proxy —
// copper demand is a well-known leading indicator of Chinese industrial
// activity. TODO(Stage 6+): add a dedicated China PMI / property-investment
// detector once that data source is wired.
type chinaSlowdownDetector struct{ baseDetector }

func newChinaSlowdownDetector() *chinaSlowdownDetector {
	return &chinaSlowdownDetector{baseDetector{theme: "china_slowdown", source: SourceKB, enabled: true}}
}

func (d *chinaSlowdownDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectCopperIndustrialEvent(in.MarketData), d.source), nil
}

type taiwanExportBoomDetector struct{ baseDetector }

func newTaiwanExportBoomDetector() *taiwanExportBoomDetector {
	return &taiwanExportBoomDetector{baseDetector{theme: "taiwan_export_boom", source: SourceKB, enabled: true}}
}

func (d *taiwanExportBoomDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	return narrativeEventToResult(detectTaiwanExportBoomEvent(in.MarketData), d.source), nil
}

// ----------------------------------------------------------------------------
// Seasonal detectors (6) — all wrap detectSeasonalEvent() which returns at
// most one event per call. Each detector filters by its own theme string
// so the 24-detector scan can independently report which seasonal window
// is active today.
// ----------------------------------------------------------------------------

func newSeasonalDetector(theme string) *baseDetector {
	return &baseDetector{theme: theme, source: SourceKB, enabled: true}
}

type seasonalThemeDetector struct{ baseDetector }

func (d *seasonalThemeDetector) Detect(_ context.Context, _ DetectorInput) (*DetectionResult, error) {
	evt := detectSeasonalEvent()
	if evt == nil || evt.Theme != d.theme {
		return nil, nil
	}
	return narrativeEventToResult(evt, d.source), nil
}

// PeriodWeight for seasonal detectors: earnings_blackout ×2 during plateau and consolidation.
func (d *seasonalThemeDetector) PeriodWeight(period domain.MarketPeriod) float64 {
	if d.theme == "earnings_blackout" {
		switch period {
		case domain.PeriodPlateau, domain.PeriodConsolidation:
			return 2.0
		}
	}
	return 1.0
}

func newSpringFestivalSeasonDetector() Detector {
	return &seasonalThemeDetector{*newSeasonalDetector("spring_festival_season")}
}

func newElectionCycleDetector() Detector {
	return &seasonalThemeDetector{*newSeasonalDetector("election_cycle")}
}

func newEarningsBlackoutDetector() Detector {
	return &seasonalThemeDetector{*newSeasonalDetector("earnings_blackout")}
}

func newTechPeakSeasonDetector() Detector {
	return &seasonalThemeDetector{*newSeasonalDetector("tech_peak_season")}
}

func newYearEndWindowDressingDetector() Detector {
	return &seasonalThemeDetector{*newSeasonalDetector("year_end_window_dressing")}
}

func newDividendSeasonDetector() Detector {
	return &seasonalThemeDetector{*newSeasonalDetector("dividend_season")}
}

// ----------------------------------------------------------------------------
// Tariff shock — snapshot-pipeline only (no KB equivalent exists yet).
// TODO(Stage 6+): add detectTariffShockKBEvent(data MarketNarrativeData) once
// a TradeNews data source is wired; until then this detector covers tariff
// shocks through the MacroDataSnapshot path (VIX spike + DXY vol + SPX
// selloff), which is the same signal as the legacy ingestor call.
// ----------------------------------------------------------------------------

type tariffShockDetector struct{ baseDetector }

func newTariffShockDetector() *tariffShockDetector {
	return &tariffShockDetector{baseDetector{theme: "tariff_shock", source: SourceIngestor, enabled: true}}
}

func (d *tariffShockDetector) Detect(_ context.Context, in DetectorInput) (*DetectionResult, error) {
	snap := in.MacroSnapshot
	if snap.VIX.Symbol == "" && snap.DXY.Symbol == "" && snap.SPXIndex.Symbol == "" {
		return nil, nil
	}
	evt := detectTariffShockEventFromSnapshot(snap.VIX, snap.DXY, snap.SPXIndex, detectNow(in))
	return narrativeEventToResult(evt, d.source), nil
}

// PeriodWeight: tariff_shock is universally important (1.5×) — can trigger
// direct transition to turnaround_down or black_swan in any period.
func (d *tariffShockDetector) PeriodWeight(period domain.MarketPeriod) float64 { return 1.5 }

// ----------------------------------------------------------------------------
// Public constructor — registers all 24 detectors, default-enabled.
// ----------------------------------------------------------------------------

// NewDefaultDetectorRegistry returns a registry populated with the 24
// trigger-theme detectors from templates.go. Detectors start enabled; callers
// can Disable() individual themes via Registry.Enable/Disable without
// affecting the others.
//
// Order of registration is not semantically meaningful (Registry is map-
// keyed), but we group: KB-pipeline detectors, then seasonal, then the one
// snapshot-pipeline detector, so a debugger walking the list reads top-to-
// bottom in pipeline order.
func NewDefaultDetectorRegistry() *DetectorRegistry {
	r := NewDetectorRegistry()
	// KB pipeline — macro / micro narrative detectors
	r.MustRegister(newUSRatesUpDetector())
	r.MustRegister(newUSRatesDownDetector())
	r.MustRegister(newJPYCarryUnwindDetector())
	r.MustRegister(newAICapexSurgeDetector())
	r.MustRegister(newGeopoliticalRiskDetector())
	r.MustRegister(newOilShockDetector())
	r.MustRegister(newTaiwanPoliticalRiskDetector())
	r.MustRegister(newUSDTWDVolatilityDetector())
	r.MustRegister(newSemiconductorDownturnDetector())
	r.MustRegister(newRetailInstitutionalDivergenceDetector())
	r.MustRegister(newGoldRallyDetector())
	r.MustRegister(newDollarSurgeDetector())
	r.MustRegister(newInflationSpikeDetector())
	r.MustRegister(newEarningsSurpriseDetector())
	r.MustRegister(newShippingRateSpikeDetector())
	r.MustRegister(newChinaSlowdownDetector())
	r.MustRegister(newTaiwanExportBoomDetector())
	// Seasonal — all share detectSeasonalEvent(), filtered by theme
	r.MustRegister(newSpringFestivalSeasonDetector())
	r.MustRegister(newElectionCycleDetector())
	r.MustRegister(newEarningsBlackoutDetector())
	r.MustRegister(newTechPeakSeasonDetector())
	r.MustRegister(newYearEndWindowDressingDetector())
	r.MustRegister(newDividendSeasonDetector())
	// Snapshot pipeline — only theme without a KB equivalent
	r.MustRegister(newTariffShockDetector())
	return r
}

// Compile-time interface compliance checks. If a detector struct drifts
// away from the Detector interface, the build fails here rather than at
// the MustRegister call site, which makes the failure obvious.
var (
	_ Detector = (*usRatesUpDetector)(nil)
	_ Detector = (*usRatesDownDetector)(nil)
	_ Detector = (*jpyCarryUnwindDetector)(nil)
	_ Detector = (*aiCapexSurgeDetector)(nil)
	_ Detector = (*geopoliticalRiskDetector)(nil)
	_ Detector = (*oilShockDetector)(nil)
	_ Detector = (*taiwanPoliticalRiskDetector)(nil)
	_ Detector = (*usdTwdVolatilityDetector)(nil)
	_ Detector = (*semiconductorDownturnDetector)(nil)
	_ Detector = (*retailInstitutionalDivergenceDetector)(nil)
	_ Detector = (*goldRallyDetector)(nil)
	_ Detector = (*dollarSurgeDetector)(nil)
	_ Detector = (*inflationSpikeDetector)(nil)
	_ Detector = (*earningsSurpriseDetector)(nil)
	_ Detector = (*shippingRateSpikeDetector)(nil)
	_ Detector = (*chinaSlowdownDetector)(nil)
	_ Detector = (*taiwanExportBoomDetector)(nil)
	_ Detector = (*seasonalThemeDetector)(nil)
	_ Detector = (*tariffShockDetector)(nil)
	// marketdata import kept referenced so go.mod / IDE stays consistent
	_ = marketdata.MacroDataSnapshot{}
)
