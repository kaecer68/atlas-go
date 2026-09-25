package narrative

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ── I23 (#1944 Batch 3): hit-rate provenance ─────────────────────────────
//
// These tests pin the FACT that the narrative module's outward hit-rate fields
// are hand-authored priors, not measured/backtested figures, and that the
// provenance label is present on every outward surface. If a future change
// starts computing a real hit rate, this file must be updated together with
// docs/reference/inert-registry.md.

// TestDefaultTemplatesCarryPriorHitRateSource pins that every shipped causal
// template labels its HistoricalHitRate as a handwritten prior.
func TestDefaultTemplatesCarryPriorHitRateSource(t *testing.T) {
	templates := DefaultTemplates()
	if len(templates) == 0 {
		t.Fatal("DefaultTemplates() returned no templates")
	}
	for _, tmpl := range templates {
		if tmpl.HitRateSource != HitRateSourceHandwrittenPrior {
			t.Errorf("template %q: HitRateSource = %q, want %q",
				tmpl.ID, tmpl.HitRateSource, HitRateSourceHandwrittenPrior)
		}
		if tmpl.HistoricalHitRate <= 0 || tmpl.HistoricalHitRate > 1 {
			t.Errorf("template %q: HistoricalHitRate = %v out of (0,1]", tmpl.ID, tmpl.HistoricalHitRate)
		}
	}
}

// TestNewKnowledgeBaseTemplatesCarryPriorSource pins the same fact on the live
// knowledge base, i.e. the object behind GET /api/narrative/templates.
func TestNewKnowledgeBaseTemplatesCarryPriorSource(t *testing.T) {
	list := NewKnowledgeBase().ListTemplates()
	if len(list) == 0 {
		t.Fatal("ListTemplates() returned no templates")
	}
	for _, tmpl := range list {
		if tmpl.HitRateSource != HitRateSourceHandwrittenPrior {
			t.Errorf("template %q: HitRateSource = %q, want %q",
				tmpl.ID, tmpl.HitRateSource, HitRateSourceHandwrittenPrior)
		}
	}
}

// TestRegisterTemplateDefaultsToPriorSource guards the normalization seam: a
// template registered without an explicit source must not expose an unlabeled
// hit rate (an unlabeled value is what let a prior pass for a measurement).
func TestRegisterTemplateDefaultsToPriorSource(t *testing.T) {
	kb := NewKnowledgeBase()
	kb.RegisterTemplate(CausalTemplate{ID: "tmp-unlabeled", TriggerTheme: "x", HistoricalHitRate: 0.4})
	got, ok := kb.GetTemplate("tmp-unlabeled")
	if !ok {
		t.Fatal("registered template not found")
	}
	if got.HitRateSource != HitRateSourceHandwrittenPrior {
		t.Errorf("HitRateSource = %q, want %q", got.HitRateSource, HitRateSourceHandwrittenPrior)
	}
}

// TestInvestmentModelsStartAsHandwrittenPrior pins that the shipped model
// library is prior-only before any evaluation runs.
func TestInvestmentModelsStartAsHandwrittenPrior(t *testing.T) {
	models := NewNarrativeEngine().ListModels()
	if len(models) == 0 {
		t.Fatal("engine has no models")
	}
	for _, m := range models {
		if m.HitRateSource != HitRateSourceHandwrittenPrior {
			t.Errorf("model %q: HitRateSource = %q, want %q (SampleCount=%d)",
				m.ID, m.HitRateSource, HitRateSourceHandwrittenPrior, m.SampleCount)
		}
		if m.SampleCount != 0 {
			t.Errorf("model %q: expected SampleCount 0 before evaluation, got %d", m.ID, m.SampleCount)
		}
	}
}

// TestEvaluateModelsMarksInMemoryProvenance pins that an evaluation pass
// relabels every model: a recomputed rate is in-memory only (never a durable
// backtest), and a zero-sample pass yields the neutral sentinel, not a prior.
func TestEvaluateModelsMarksInMemoryProvenance(t *testing.T) {
	csvPath, err := writeTestReplayCSV(t.TempDir())
	if err != nil {
		t.Fatalf("write test csv: %v", err)
	}
	ne := NewNarrativeEngine()
	if err := ne.EvaluateModels(csvPath); err != nil {
		t.Fatalf("EvaluateModels: %v", err)
	}
	for _, m := range ne.ListModels() {
		switch m.HitRateSource {
		case HitRateSourceReplayEvalInMemory:
			if m.SampleCount == 0 {
				t.Errorf("model %q labeled %q with 0 samples", m.ID, m.HitRateSource)
			}
		case HitRateSourceUnavailableNoSamples:
			if m.SampleCount != 0 {
				t.Errorf("model %q labeled %q with %d samples", m.ID, m.HitRateSource, m.SampleCount)
			}
		default:
			t.Errorf("model %q kept source %q after evaluation; every evaluated model must be relabeled",
				m.ID, m.HitRateSource)
		}
	}
}

// TestUpdateTemplateHitRatesRelabelsSource pins that an EMA recalculation moves
// templates off the prior label instead of leaving a blended value unlabeled.
func TestUpdateTemplateHitRatesRelabelsSource(t *testing.T) {
	csvPath, err := writeTestReplayCSV(t.TempDir())
	if err != nil {
		t.Fatalf("write test csv: %v", err)
	}
	ne := NewNarrativeEngine()
	if err := ne.EvaluateModels(csvPath); err != nil {
		t.Fatalf("EvaluateModels: %v", err)
	}
	ne.RecalculateTemplateHitRates()
	ne.RecalculateAllTemplateHitRates(0.30)
	relabeled := 0
	for _, tmpl := range ne.KnowledgeBase().ListTemplates() {
		if tmpl.HitRateSource == HitRateSourceReplayEvalInMemory {
			relabeled++
		}
	}
	if relabeled == 0 {
		t.Error("expected at least one template to be relabeled after an EMA recalculation")
	}
}

// TestDetectEventsStampsPriorProvenance pins that events leaving the KB
// detector pipeline are labeled, and that the label agrees with the prior
// table for the event's theme.
func TestDetectEventsStampsPriorProvenance(t *testing.T) {
	events := NewNarrativeEngine().DetectEvents(MarketNarrativeData{US10YChangeBps: 40})
	if len(events) == 0 {
		t.Fatal("expected at least one detected event for a 40bp 10Y move")
	}
	for _, e := range events {
		if _, hasTemplate := templateHitRates[e.Theme]; hasTemplate {
			if e.HitRateSource != HitRateSourceHandwrittenPrior {
				t.Errorf("event theme %q: HitRateSource = %q, want %q",
					e.Theme, e.HitRateSource, HitRateSourceHandwrittenPrior)
			}
			continue
		}
		if e.HitRateSource != HitRateSourceUnavailableNoTemplate {
			t.Errorf("event theme %q (no template): HitRateSource = %q, want %q",
				e.Theme, e.HitRateSource, HitRateSourceUnavailableNoTemplate)
		}
		if e.HitRate != 0 {
			t.Errorf("event theme %q (no template): HitRate = %v, want 0", e.Theme, e.HitRate)
		}
	}
}

// TestIngestorPipelineStampsPriorProvenance pins the second detector pipeline
// (MacroDataSnapshot path used by MacroIngestor).
func TestIngestorPipelineStampsPriorProvenance(t *testing.T) {
	now := time.Now().UTC()
	curr := marketdata.MacroDataSnapshot{US10Y: marketdata.MacroDataPoint{Symbol: "TNX", Value: 60}}
	prev := marketdata.MacroDataSnapshot{US10Y: marketdata.MacroDataPoint{Symbol: "TNX", Value: 0}}
	events := detectEventsFromSnapshot(curr, prev, nil)
	if len(events) == 0 {
		t.Fatal("expected at least one snapshot-detected event for a 60bp 10Y move")
	}
	for _, e := range events {
		if e.HitRateSource == "" {
			t.Errorf("snapshot event theme %q has an unlabeled HitRate", e.Theme)
		}
	}
	_ = now
}

// TestHitRateForThemeOnlyExposesThemesWithTemplates pins that hitRateForTheme
// never invents a prior: an unknown theme yields the 0.0 placeholder, which is
// exactly the signal ThemesWithoutTemplate carries.
func TestHitRateForThemeOnlyExposesThemesWithTemplates(t *testing.T) {
	if got := hitRateForTheme("this_theme_does_not_exist"); got != 0 {
		t.Errorf("hitRateForTheme(unknown) = %v, want 0", got)
	}
	for _, theme := range ThemesWithoutTemplate {
		if got := hitRateForTheme(theme); got != 0 {
			t.Errorf("hitRateForTheme(%q) = %v, want 0 (no template)", theme, got)
		}
	}
}

// ── N-P4: semiconductor_cycle_peak has no template ───────────────────────
//
// The SOX and DRAM detectors fire on real macro moves but their theme has no
// causal template: HitRate stays 0 and MatchChains produces no chain, so the
// signal cannot reach any sector. Rather than silently emitting a dead event,
// the event now carries HitRateSourceUnavailableNoTemplate.

// TestThemesWithoutTemplateMatchesKB pins the registered list against the KB.
func TestThemesWithoutTemplateMatchesKB(t *testing.T) {
	if len(ThemesWithoutTemplate) == 0 {
		t.Fatal("ThemesWithoutTemplate must list at least semiconductor_cycle_peak")
	}
	for _, theme := range ThemesWithoutTemplate {
		if _, ok := templateHitRates[theme]; ok {
			t.Errorf("theme %q is listed as template-less but DefaultTemplates() covers it", theme)
		}
		kb := NewKnowledgeBase()
		if _, ok := kb.GetTemplateByTheme(theme); ok {
			t.Errorf("theme %q is listed as template-less but a template matches it", theme)
		}
		if chains := kb.MatchChains(NarrativeEvent{Theme: theme}); len(chains) != 0 {
			t.Errorf("theme %q produced %d chains; expected 0", theme, len(chains))
		}
	}
}

// TestSemiconductorCyclePeakEventCannotReachAnySector pins the whole dead-end:
// detector output is labeled, no chain matches, and SectorBias stays 0.
func TestSemiconductorCyclePeakEventCannotReachAnySector(t *testing.T) {
	evt := detectSOXSemiconductorCycleEvent(MarketNarrativeData{SOXIndexChangePct: 6.5})
	if evt == nil {
		t.Fatal("expected a semiconductor_cycle_peak event for a 6.5% SOX move")
	}
	if evt.Theme != "semiconductor_cycle_peak" {
		t.Fatalf("theme = %q, want semiconductor_cycle_peak", evt.Theme)
	}
	if evt.HitRate != 0 {
		t.Errorf("HitRate = %v, want 0 (no prior exists for this theme)", evt.HitRate)
	}
	// The raw detector helpers are internal and unlabeled; the label is applied
	// by the pipeline choke points, which the assertions below cover.
	if !slices.Contains(ThemesWithoutTemplate, evt.Theme) {
		t.Errorf("theme %q must be registered in ThemesWithoutTemplate", evt.Theme)
	}

	ne := NewNarrativeEngine()
	events := ne.DetectEvents(MarketNarrativeData{SOXIndexChangePct: 6.5})
	stamped := false
	for _, e := range events {
		if e.Theme != "semiconductor_cycle_peak" {
			continue
		}
		stamped = true
		if e.HitRateSource != HitRateSourceUnavailableNoTemplate {
			t.Errorf("stamped event source = %q, want %q", e.HitRateSource, HitRateSourceUnavailableNoTemplate)
		}
	}
	if !stamped {
		t.Fatal("SOX event missing from DetectEvents output")
	}
	if chains := ne.MatchChains(events); len(chains) != 0 {
		t.Errorf("expected no causal chain, got %d", len(chains))
	}
	for _, sid := range []string{"semiconductor", "electronics"} {
		if bias := ne.SectorBias(sid, events); bias != 0 {
			t.Errorf("SectorBias(%q) = %v, want 0 (dead theme must not move a sector)", sid, bias)
		}
	}
}

// TestAggregateHitRateSourceForEvents pins the aggregate labeling used by the
// factor engine so an average over priors is never presented as a measurement.
func TestAggregateHitRateSourceForEvents(t *testing.T) {
	if got := AggregateHitRateSourceForEvents(nil); got != "" {
		t.Errorf("empty set: got %q, want empty", got)
	}
	prior := []*NarrativeEvent{{Theme: "a", HitRateSource: HitRateSourceHandwrittenPrior}}
	if got := AggregateHitRateSourceForEvents(prior); got != HitRateSourceHandwrittenPrior {
		t.Errorf("prior-only: got %q, want %q", got, HitRateSourceHandwrittenPrior)
	}
	unlabeled := []*NarrativeEvent{{Theme: "a"}, {Theme: "b"}}
	if got := AggregateHitRateSourceForEvents(unlabeled); got != HitRateSourceHandwrittenPrior {
		t.Errorf("unlabeled events: got %q, want %q", got, HitRateSourceHandwrittenPrior)
	}
	mixed := []*NarrativeEvent{
		{Theme: "a", HitRateSource: HitRateSourceHandwrittenPrior},
		{Theme: "b", HitRateSource: HitRateSourceReplayEvalInMemory},
	}
	if got := AggregateHitRateSourceForEvents(mixed); got != HitRateSourceMixed {
		t.Errorf("mixed: got %q, want %q", got, HitRateSourceMixed)
	}
}

// TestStructuralTrendHitRatesArePriors pins the third outward hit-rate surface.
func TestStructuralTrendHitRatesArePriors(t *testing.T) {
	assessment := NewStructuralTrendEngine().Assess(MacroDataSnapshot{}, SectorDataSnapshot{
		AIRevenueGrowth:    60,
		CoWoSUtilization:   90,
		SemiconductorIndex: 5000,
		CapexGrowth:        50,
	})
	if len(assessment.Trends) == 0 {
		t.Fatal("expected structural trends to be detected")
	}
	for _, tr := range assessment.Trends {
		if tr.HitRateSource != HitRateSourceHandwrittenPrior {
			t.Errorf("trend %q: HitRateSource = %q, want %q",
				tr.Name, tr.HitRateSource, HitRateSourceHandwrittenPrior)
		}
	}
}

// TestHitRateProvenanceJSONContract pins the wire contract of the new
// provenance field on every outward hit-rate surface: the JSON key is
// `hit_rate_source` and the shipped value is `handwritten_prior`. This is the
// evidence that /api/narrative/templates and /api/narrative/models can no
// longer present a prior as a backtest figure (#1944 Batch 3, item I23).
func TestHitRateProvenanceJSONContract(t *testing.T) {
	encode := func(t *testing.T, v any) map[string]any {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal %T: %v", v, err)
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal %T: %v", v, err)
		}
		return out
	}

	tmpl := DefaultTemplates()[0]
	tmplJSON := encode(t, tmpl)
	if _, ok := tmplJSON["historical_hit_rate"]; !ok {
		t.Fatal("template no longer exposes historical_hit_rate")
	}
	if got := tmplJSON["hit_rate_source"]; got != HitRateSourceHandwrittenPrior {
		t.Errorf("template hit_rate_source = %v, want %q", got, HitRateSourceHandwrittenPrior)
	}

	models := NewNarrativeEngine().ListModels()
	if len(models) == 0 {
		t.Fatal("engine has no models")
	}
	modelJSON := encode(t, models[0])
	if got := modelJSON["hit_rate_source"]; got != HitRateSourceHandwrittenPrior {
		t.Errorf("model hit_rate_source = %v, want %q", got, HitRateSourceHandwrittenPrior)
	}

	events := NewNarrativeEngine().DetectEvents(MarketNarrativeData{US10YChangeBps: 40})
	if len(events) == 0 {
		t.Fatal("expected a detected event")
	}
	eventJSON := encode(t, events[0])
	if got := eventJSON["hit_rate_source"]; got != HitRateSourceHandwrittenPrior {
		t.Errorf("event hit_rate_source = %v, want %q", got, HitRateSourceHandwrittenPrior)
	}

	assessment := NewStructuralTrendEngine().Assess(MacroDataSnapshot{}, SectorDataSnapshot{
		AIRevenueGrowth: 60, CoWoSUtilization: 90, SemiconductorIndex: 5000, CapexGrowth: 50,
	})
	if len(assessment.Trends) == 0 {
		t.Fatal("expected structural trends")
	}
	trendJSON := encode(t, assessment.Trends[0])
	if got := trendJSON["hit_rate_source"]; got != HitRateSourceHandwrittenPrior {
		t.Errorf("structural trend hit_rate_source = %v, want %q", got, HitRateSourceHandwrittenPrior)
	}
}
