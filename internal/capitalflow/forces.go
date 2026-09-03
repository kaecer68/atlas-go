package capitalflow

import (
	"math"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ForceExtractor extracts force scores from a MacroDataSnapshot.
//
// Per docs/specs/capital-flow-seven-dimension-spec.md §7 (manifest
// #E05), the taxonomy is:
//
//   - 5 subjects (participate in resonance): foreign, institutional,
//     dealer, government, retail.
//   - 1 leading_indicator: foreign futures OI — surfaces on the
//     foreign ForceScore via LeadingZ / LeadingTrend; also exposed
//     as a deprecated "futures" score for back-compat.
//   - 1 sentiment: TSM ADR — exposed as a deprecated score; never
//     enters resonance.
//
// The extractor is stateless with respect to the rolling window
// (BK-15 / spec §8.1 / CF-INV-04): every read path receives the
// history it should score against via the Score method's `history`
// parameter, and the only writer of the rolling sample store is
// Service.Refresh. That separation is what lets the read path
// (LatestDaily, Summary, QualityScore) stay idempotent across
// repeated calls — a snapshot scored twice against the same
// history must produce the same report.
type ForceExtractor struct{}

// NewForceExtractor creates a stateless ForceExtractor. Window
// state is supplied at scoring time via the history argument of
// Score, not held inside the struct.
func NewForceExtractor() *ForceExtractor {
	return &ForceExtractor{}
}

// Score computes force scores from a market data snapshot using
// the caller-supplied rolling history. The extractor never mutates
// the history, never calls any store method, and never holds onto
// window state of its own.
//
// Dimensions whose source channel is missing (Symbol == "") return
// DataAvailable=false with Z=0 / Trend="neutral"; they do not look
// up history and therefore contribute nothing to the rolling store
// when Service.Refresh persists today's reading. This satisfies
// spec §8.3 / CF-INV-06 (no zero-valued samples in the rolling
// store, no "neutral" interpretation of missing data).
//
// tradingDate is the as-of trading day (YYYY-MM-DD); it is threaded
// into every ForceScore.AsOfTradingDate so the E07 assessment can
// carry an explicit timestamp without growing any caller signature.
func (e *ForceExtractor) Score(
	snap marketdata.MacroDataSnapshot,
	tradingDate string,
	history map[ForceName][]RollingSample,
) []ForceScore {
	raw := []ForceScore{
		e.scoreForeign(snap, history),
		e.scoreFuturesDeprecated(snap, history),
		e.scoreTSMADR(snap, history),
		e.scoreInstitutional(snap, history),
		e.scoreDealer(snap, history),
		e.scoreGovernment(snap, history),
		e.scoreRetail(snap, history),
	}
	forces := make([]ForceScore, 0, len(raw))
	for _, f := range raw {
		prov := ComputeForceProvenance(f.Force)
		f.DimensionRole = prov.DimensionRole
		f.SourceID = prov.SourceID
		f.Unit = prov.Unit
		f.ParticipatesInActorConsensus = prov.ParticipatesInActorConsensus
		f.AsOfTradingDate = tradingDate
		f.SampleCount = len(history[f.Force])
		// E07 / spec §7.2 / CF-INV-07: the legacy cross-unit weight
		// is suppressed. We set Weight=0 and WeightDeprecated=true
		// here (in addition to applyForceWeights) so callers that
		// read ForceScore directly — bypassing the report
		// generator, e.g. Score() tests — still see the suppressed
		// shape rather than an uninitialised Weight.
		f.Weight = 0
		f.WeightDeprecated = true
		f.DisplayName = f.Force.DisplayName()
		// H-CF-02 / spec §8.4 — a force becomes calibration-eligible once
		// the rolling history has at least 30 samples. Below the threshold
		// we keep the conservative "calibrating" label so automation stays
		// gated (CF-INV-13); once SampleCount crosses 30 the per-force
		// status flips to "eligible". The "degraded" path lives in the
		// calibration pipeline and is not set here.
		if f.SampleCount >= 30 {
			f.CalibrationStatus = CalibrationEligible
		} else {
			f.CalibrationStatus = CalibrationCalibrating
		}
		switch f.Force {
		case ForceForeign, ForceInstitutional, ForceDealer:
			f.EvidenceClass = EvidenceOfficial
		case ForceGovernment:
			f.EvidenceClass = EvidenceProxy
		case ForceRetail:
			f.EvidenceClass = EvidenceOfficialDerived
		case ForceFutures:
			f.EvidenceClass = EvidenceOfficial
		case ForceTSMADR:
			f.EvidenceClass = EvidenceCrossMarket
		}
		forces = append(forces, f)
	}
	return forces
}

// Extract is a thin wrapper kept for callers that only need the
// extract stage (no rolling history). It delegates to Score with
// an empty history, so Z-scores are computed from process-lifetime
// zero history (Z=raw when no samples are loaded; see
// zScoreFromSamples). Production readers should use Score directly
// via Service.LatestDaily, which threads the rolling history
// through from the store.
//
// Spec §8.4 — Z is computed from prior samples only — is honored
// by zScoreFromSamples, which never pushes `raw` into the window.
func (e *ForceExtractor) Extract(snap marketdata.MacroDataSnapshot) []ForceScore {
	return e.Score(snap, "", nil)
}

// scoreForeign computes the foreign subject Z from the spot window
// and the foreign leading Z from the futures window. Per spec §8.3,
// when the spot channel is missing (Symbol empty) the subject is
// reported as DataAvailable=false so the rolling store never
// receives a zero-valued foreign sample. The leading field follows
// the same rule on the futures channel.
func (e *ForceExtractor) scoreForeign(snap marketdata.MacroDataSnapshot, history map[ForceName][]RollingSample) ForceScore {
	spotRaw := snap.ForeignInvestorNet.Value
	spotZ := round(zScoreFromSamples(history[ForceForeign], spotRaw), 3)
	spotTrend := trendFor(spotZ)

	// Leading signal from TAIFEX 三大法人期貨 OI (manifest #E01).
	// When the channel has no data (Symbol empty), the leading
	// fields stay at zero and trend is neutral — no history
	// lookup, no sample is generated for the rolling store
	// (spec §8.3 / CF-INV-06).
	var leadingZ float64
	var leadingTrend string
	if snap.ForeignFuturesOINet.Symbol != "" {
		futRaw := snap.ForeignFuturesOINet.Value
		leadingZ = round(zScoreFromSamples(history[ForceFutures], futRaw), 3)
		leadingTrend = trendFor(leadingZ)
	} else {
		leadingTrend = "neutral"
	}

	return ForceScore{
		Force:         ForceForeign,
		Role:          ForceRoleSubject,
		RawValue:      spotRaw,
		ZScore:        spotZ,
		Trend:         spotTrend,
		LeadingZ:      leadingZ,
		LeadingTrend:  leadingTrend,
		DataAvailable: snap.ForeignInvestorNet.Symbol != "",
	}
}

// scoreFuturesDeprecated preserves the legacy "futures" ForceScore
// entry for API back-compat. It uses the same data source as the
// foreign LeadingZ, so consumers reading either get the same
// numbers. Marked deprecated + leading_indicator; not a resonance
// subject. When the underlying channel is empty, the entry is
// reported as DataAvailable=false (no history lookup, no sample).
func (e *ForceExtractor) scoreFuturesDeprecated(snap marketdata.MacroDataSnapshot, history map[ForceName][]RollingSample) ForceScore {
	if snap.ForeignFuturesOINet.Symbol == "" {
		return ForceScore{
			Force:         ForceFutures,
			Role:          ForceRoleLeadingIndicator,
			Deprecated:    true,
			RawValue:      0,
			ZScore:        0,
			Trend:         "neutral",
			DataAvailable: false,
		}
	}
	raw := snap.ForeignFuturesOINet.Value
	z := round(zScoreFromSamples(history[ForceFutures], raw), 3)
	return ForceScore{
		Force:         ForceFutures,
		Role:          ForceRoleLeadingIndicator,
		Deprecated:    true,
		RawValue:      raw,
		ZScore:        z,
		Trend:         trendFor(z),
		DataAvailable: true,
	}
}

func (e *ForceExtractor) scoreTSMADR(snap marketdata.MacroDataSnapshot, history map[ForceName][]RollingSample) ForceScore {
	if snap.TSMADR.Symbol == "" {
		return ForceScore{
			Force:         ForceTSMADR,
			Role:          ForceRoleSentiment,
			Deprecated:    true,
			RawValue:      0,
			ZScore:        0,
			Trend:         "neutral",
			DataAvailable: false,
		}
	}
	raw := snap.TSMADR.ChangePct
	z := round(zScoreFromSamples(history[ForceTSMADR], raw), 3)
	return ForceScore{
		Force:         ForceTSMADR,
		Role:          ForceRoleSentiment,
		Deprecated:    true,
		RawValue:      raw,
		ZScore:        z,
		Trend:         trendFor(z),
		DataAvailable: true,
	}
}

func (e *ForceExtractor) scoreInstitutional(snap marketdata.MacroDataSnapshot, history map[ForceName][]RollingSample) ForceScore {
	if snap.DomesticFundNet.Symbol == "" {
		return ForceScore{
			Force:         ForceInstitutional,
			Role:          ForceRoleSubject,
			RawValue:      0,
			ZScore:        0,
			Trend:         "neutral",
			DataAvailable: false,
		}
	}
	raw := snap.DomesticFundNet.Value
	z := round(zScoreFromSamples(history[ForceInstitutional], raw), 3)
	return ForceScore{
		Force:         ForceInstitutional,
		Role:          ForceRoleSubject,
		RawValue:      raw,
		ZScore:        z,
		Trend:         trendFor(z),
		DataAvailable: true,
	}
}

func (e *ForceExtractor) scoreDealer(snap marketdata.MacroDataSnapshot, history map[ForceName][]RollingSample) ForceScore {
	if snap.DealerNet.Symbol == "" {
		return ForceScore{
			Force:         ForceDealer,
			Role:          ForceRoleSubject,
			RawValue:      0,
			ZScore:        0,
			Trend:         "neutral",
			DataAvailable: false,
		}
	}
	raw := snap.DealerNet.Value
	z := round(zScoreFromSamples(history[ForceDealer], raw), 3)
	return ForceScore{
		Force:         ForceDealer,
		Role:          ForceRoleSubject,
		RawValue:      raw,
		ZScore:        z,
		Trend:         trendFor(z),
		DataAvailable: true,
	}
}

// scoreGovernment reads the E04 官股行庫 reading from the snapshot.
// When no data is present (Symbol empty), the score stays neutral
// with DataAvailable=false so the resonance model can distinguish
// "no data" from "data says neutral" — and so the rolling store
// never receives a zero-valued government sample (CF-INV-06).
func (e *ForceExtractor) scoreGovernment(snap marketdata.MacroDataSnapshot, history map[ForceName][]RollingSample) ForceScore {
	if snap.GovernmentNet.Symbol == "" {
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
	z := round(zScoreFromSamples(history[ForceGovernment], raw), 3)
	return ForceScore{
		Force:         ForceGovernment,
		Role:          ForceRoleSubject,
		RawValue:      raw,
		ZScore:        z,
		Trend:         trendFor(z),
		DataAvailable: true,
	}
}

// scoreRetail combines the E05 retail margin + short balance
// readings. When either channel is missing (Symbol empty), the
// retail force is reported as DataAvailable=false with no history
// lookup, so today's missing source cannot poison the rolling
// store with a zero-valued retail sample.
func (e *ForceExtractor) scoreRetail(snap marketdata.MacroDataSnapshot, history map[ForceName][]RollingSample) ForceScore {
	if snap.RetailMarginBalance.Symbol == "" || snap.RetailShortBalance.Symbol == "" {
		return ForceScore{
			Force:         ForceRetail,
			Role:          ForceRoleSubject,
			RawValue:      0,
			ZScore:        0,
			Trend:         "neutral",
			DataAvailable: false,
		}
	}
	raw := snap.RetailMarginBalance.ChangePct + snap.RetailShortBalance.ChangePct
	z := round(zScoreFromSamples(history[ForceRetail], raw), 3)
	return ForceScore{
		Force:         ForceRetail,
		Role:          ForceRoleSubject,
		RawValue:      raw,
		ZScore:        z,
		Trend:         trendFor(z),
		DataAvailable: true,
	}
}

// ComputeForceProvenance is a thin lookup helper that returns the
// 4-field provenance row for a given dimension (spec §6 / §7 /
// CF-INV-01). Score() calls it once per dimension to populate the
// matching ForceScore fields; tests call it directly to anchor the
// 7×4 provenance matrix without going through a full Extract run.
//
// Values are static per the spec — the source registry
// (rolling_store.go) and the E07 dimension-role constants
// (types.go) are the source of truth. Behavioral proxy dimensions
// (government / retail) report ParticipatesInActorConsensus=false
// so the actor consensus filter (CF-INV-09) excludes them from
// Aligned/Opposing — the test contract is that only the three
// official_actor dimensions vote in actor consensus, and a
// behavioral_proxy dimension's "true" reading still does not.
//
// Unknown dimensions get the zero-value row so an unrecognized
// caller cannot trigger a panic.
func ComputeForceProvenance(force ForceName) ForceProvenance {
	switch force {
	case ForceForeign, ForceInstitutional, ForceDealer:
		return ForceProvenance{
			DimensionRole:                DimensionRoleOfficialActor,
			SourceID:                     SourceTWSET86,
			Unit:                         "hundred_million_shares",
			ParticipatesInActorConsensus: true,
		}
	case ForceGovernment:
		return ForceProvenance{
			DimensionRole:                DimensionRoleBehavioralProxy,
			SourceID:                     SourceGovernmentOperator,
			Unit:                         "twd",
			ParticipatesInActorConsensus: false,
		}
	case ForceRetail:
		return ForceProvenance{
			DimensionRole:                DimensionRoleBehavioralProxy,
			SourceID:                     SourceTWSEODDLOT,
			Unit:                         "pct_composite",
			ParticipatesInActorConsensus: false,
		}
	case ForceFutures:
		return ForceProvenance{
			DimensionRole:                DimensionRolePositioningIndicator,
			SourceID:                     SourceTAIFEXInst,
			Unit:                         "contracts",
			ParticipatesInActorConsensus: false,
		}
	case ForceTSMADR:
		return ForceProvenance{
			DimensionRole:                DimensionRoleCrossMarketSignal,
			SourceID:                     SourceSECTSMC,
			Unit:                         "pct",
			ParticipatesInActorConsensus: false,
		}
	}
	return ForceProvenance{}
}

// zScoreFromSamples computes (raw - mean) / stddev across the given
// rolling samples. It is pure: it never mutates the input slice and
// never calls any store. BK-15 makes this the only Z-score helper
// in the package — the previous zScore(w, raw) helper which pushed
// `raw` into a shared window has been removed so that the read path
// is provably free of side effects (spec §8.1 / CF-INV-04).
//
// Per spec §8.4 the window contains only prior samples — `raw` is
// excluded. With fewer than two prior samples the sample standard
// deviation is undefined, so Z is pinned to 0 (neutral) instead of
// emitting a pseudo-value (CF-INV-04 / C1a).
func zScoreFromSamples(samples []RollingSample, raw float64) float64 {
	if len(samples) < 2 {
		return 0
	}
	w := newRollingWindow(len(samples))
	for _, s := range samples {
		w.push(s.RawValue)
	}
	return w.zScore(raw)
}

// trendFor labels a Z-score's direction using the configured
// capitalflow trend thresholds (audit M8): bullish when z strictly
// exceeds the bullish cutoff, bearish when z strictly undercuts the
// bearish cutoff, neutral otherwise. The cutoffs come from
// config.GetCapitalflowTrendBullishThreshold /
// GetCapitalflowTrendBearishThreshold — defaults +0.5 / -0.5 reproduce
// the pre-parameterization hardcoded behavior exactly (same call path
// pattern as ComputeResonance's coefficient bounds).
func trendFor(z float64) string {
	return classifyTrend(z,
		config.GetCapitalflowTrendBullishThreshold(),
		config.GetCapitalflowTrendBearishThreshold())
}

// classifyTrend is the pure threshold comparator behind trendFor. It
// exists so tests (and future callers that already hold thresholds)
// can exercise the mapping without round-tripping through the config
// singleton.
func classifyTrend(z, bullishThreshold, bearishThreshold float64) string {
	switch {
	case z > bullishThreshold:
		return "bullish"
	case z < bearishThreshold:
		return "bearish"
	default:
		return "neutral"
	}
}

func round(v float64, decimals int) float64 {
	pow := 1.0
	for range decimals {
		pow *= 10
	}
	return math.Round(v*pow) / pow
}
