package narrative

// ── Hit-rate provenance (issue #1944 Batch 3, item I23) ──────────────────
//
// The narrative module owns several outward fields literally named "hit rate"
// (InvestmentModel.HitRate, CausalTemplate.HistoricalHitRate,
// NarrativeEvent.HitRate, StructuralTrend.HitRate). Before this change they
// were indistinguishable from measured figures on the API surface: the values
// are hand-authored priors written in knowledge_base.go / templates.go /
// structural_trend.go, while the only measurement path (replay evaluation via
// EvaluateModels → updateTemplateHitRates) recomputes them **in process memory
// only** and never persists them.
//
// Every such field now carries a sibling `hit_rate_source` string so that no
// consumer (API, MCP, factor engine, UI) has to guess. The labels are the
// contract; they are asserted by
// internal/narrative/hitrate_provenance_test.go.
const (
	// HitRateSourceHandwrittenPrior — the value is a hand-authored prior
	// literal (code) or an equivalent optimistic seed. There is NO
	// measurement behind it. Callers MUST NOT render it as a backtest,
	// historical or realized hit rate.
	HitRateSourceHandwrittenPrior = "handwritten_prior"

	// HitRateSourceReplayEvalInMemory — the value was recomputed from the
	// replay dataset by EvaluateModels()/updateTemplateHitRates() during this
	// process' lifetime. It is NOT persisted and NOT auditable across
	// restarts, so it is still weaker than a durable backtest; the evidence
	// size travels separately in InvestmentModel.SampleCount.
	HitRateSourceReplayEvalInMemory = "replay_eval_in_memory"

	// HitRateSourceUnavailableNoSamples — the value carries the neutral
	// sentinel produced when evaluation ran but found zero usable
	// favored-vs-avoided comparisons. It is not a measurement.
	HitRateSourceUnavailableNoSamples = "unavailable_no_samples"

	// HitRateSourceUnavailableNoTemplate — the trigger theme has no causal
	// template in DefaultTemplates(), so no prior exists at all and the
	// reported rate is the 0.0 placeholder. Such an event also matches no
	// causal chain, i.e. it cannot move any sector.
	HitRateSourceUnavailableNoTemplate = "unavailable_no_template"

	// HitRateSourceNotPopulated — HitRate was never populated by a producer
	// (the Stage 5 DetectionResult projection declares HitRate as owned by
	// another system and leaves it at 0). 0 here means "unknown", not "zero
	// accuracy"; consumers must not average it.
	HitRateSourceNotPopulated = "not_populated"
)

// HitRateSourceMixed — an aggregate whose contributors do not share one
// provenance label. It signals that a consumer must inspect the contributors
// instead of treating the average as a single measurement.
const HitRateSourceMixed = "mixed"

// AggregateHitRateSourceForEvents returns the provenance label for an average
// computed over the supplied events' HitRate values. Today every detector event
// carries HitRateSourceHandwrittenPrior, so any aggregate over them is a prior
// average and MUST be labeled as such — this is the helper the factor engine
// uses for exactly that purpose (#1944 Batch 3, item I23). Returns "" for an
// empty set and "mixed" when contributors disagree.
func AggregateHitRateSourceForEvents(events []*NarrativeEvent) string {
	if len(events) == 0 {
		return ""
	}
	label := ""
	for _, e := range events {
		if e == nil {
			continue
		}
		s := e.HitRateSource
		if s == "" {
			// Unlabeled events predate the provenance field; they are
			// detector events, i.e. priors.
			s = HitRateSourceHandwrittenPrior
		}
		if label == "" {
			label = s
			continue
		}
		if label != s {
			return HitRateSourceMixed
		}
	}
	return label
}

// ThemesWithoutTemplate lists the trigger themes that narrative detectors emit
// but that DefaultTemplates() does not cover. Events on these themes always
// carry HitRate=0 + HitRateSourceUnavailableNoTemplate and
// KnowledgeBase.MatchChains() returns no chain for them, so they contribute
// nothing to SectorBias or to any factor aggregate (issue #1944 Batch 3, item
// N-P4). The list is asserted against the actual detectors by
// TestThemesWithoutTemplateMatchesDetectors; adding a template for a theme
// requires removing it here.
var ThemesWithoutTemplate = []string{
	"semiconductor_cycle_peak", // detectSOXSemiconductorCycleEvent / detectDRAMMemoryCycleEvent
}

// hitRateSourceForTheme returns the provenance label implied by
// hitRateForTheme(theme): a theme covered by DefaultTemplates() yields the
// hand-authored prior, anything else yields the 0.0 placeholder.
func hitRateSourceForTheme(theme string) string {
	if _, ok := templateHitRates[theme]; ok {
		return HitRateSourceHandwrittenPrior
	}
	return HitRateSourceUnavailableNoTemplate
}

// stampDetectorHitRateProvenance fills HitRateSource on events produced by the
// narrative detectors. Every detector sources its HitRate from
// hitRateForTheme(), i.e. the hand-authored prior table captured at package
// init from DefaultTemplates(); the runtime EMA update in
// updateTemplateHitRates() mutates the engine's private KnowledgeBase copy and
// therefore never reaches these events. Events that already carry a source
// (e.g. projected from a persisted record) are left untouched.
//
// Both detector pipelines (NarrativeEngine.DetectEvents and
// detectEventsFromSnapshot) call this before returning, so no event can leave
// the package with an unlabeled HitRate.
func stampDetectorHitRateProvenance(events []NarrativeEvent) {
	for i := range events {
		if events[i].HitRateSource == "" {
			events[i].HitRateSource = hitRateSourceForTheme(events[i].Theme)
		}
	}
}
