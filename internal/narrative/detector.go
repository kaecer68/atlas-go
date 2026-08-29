package narrative

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// ============================================================================
// Stage 5 — Template Trigger Detector abstraction layer
// ----------------------------------------------------------------------------
// Goal: provide a uniform Detector interface so each of the 24 trigger themes
// in templates.go can be detected by an independently-registerable,
// individually-enabled/disabled Detector, without losing the existing
// detectXxxEvent (KB pipeline) / detectXxxEventFromSnapshot (ingestor pipeline)
// functions in narrative_detectors.go / ingestor.go.
//
// Backward compatibility:
//   - All existing functions in narrative_detectors.go and ingestor.go are
//     kept intact; PR#2 will wrap them with thin Detector impls.
//   - NarrativeEngine.DetectEvents(MarketNarrativeData) still returns
//     []NarrativeEvent directly (its caller chain is unchanged).
//   - MacroIngestor.Ingest(ctx) still returns NarrativeEvent slice directly.
//   - DetectorRegistry.RunAll is consumed by the new template_detector_scan
//     scheduler (PR#4) and writes to a separate SQLite scan log — it does
//     NOT feed back into the KB or ingestor pipelines in this stage.
//
// Pipeline distinction (preserved from narrative_detectors.go:108-113):
//   - SourceKB: uses full MarketNarrativeData (authoritative when populated)
//   - SourceIngestor: uses MacroDataSnapshot (degraded-mode proxy)
//   Each Detector declares which Source it belongs to; both can coexist.
// ============================================================================

// Severity classification for a detection result.
// Mirrors the convention used by FactorWeightEngine.applyEventAdjustment
// (internal/portfolio/factor_weight_engine.go): critical/high/medium/low.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

// Source identifies which pipeline produced the DetectionResult.
// Kept as a string for parity with NarrativeEvent.ConfidenceSource.
type Source string

const (
	SourceKB       Source = "kb_pipeline"       // narrative_detectors.go: MarketNarrativeData
	SourceIngestor Source = "snapshot_ingestor" // ingestor.go: MacroDataSnapshot
)

// DetectorInput unifies the inputs to both detection pipelines. Either
// MarketData or MacroSnapshot may be zero-valued; individual detectors pick
// whichever fields they need (this mirrors the existing intentional split
// between detectXxxEvent and detectXxxEventFromSnapshot).
type DetectorInput struct {
	MarketData    MarketNarrativeData
	MacroSnapshot marketdata.MacroDataSnapshot
	Now           time.Time
	CurrentPeriod domain.MarketPeriod // zero = unknown (backward compatible)
}

// DetectionResult is the unified output of any Detector.
// Callers that still need the legacy NarrativeEvent shape (e.g. eventdriven
// via eventbusPublisher) can call ToNarrativeEvent() to project.
type DetectionResult struct {
	Theme      string         `json:"theme"`      // trigger_theme, e.g. "US_rates_up"
	Severity   Severity       `json:"severity"`   // critical|high|medium|low
	Confidence float64        `json:"confidence"` // 0.0-1.0
	DetectedAt time.Time      `json:"detected_at"`
	Source     Source         `json:"source"` // kb_pipeline | snapshot_ingestor
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// ToNarrativeEvent projects a DetectionResult back to the legacy
// NarrativeEvent shape so existing KB / ingestor / eventdriven callers that
// already operate on NarrativeEvent keep working without modification.
// Fields that are owned by other systems (ID, HitRate, Lifecycle, Taxonomy,
// Explanation) are left at their zero values; the caller is expected to
// populate them via the normal lifecycle pipeline (KnowledgeBase.MatchChains,
// EventLifecycleManager, RegimeExplainer).
func (r DetectionResult) ToNarrativeEvent() NarrativeEvent {
	return NarrativeEvent{
		Theme:            r.Theme,
		Confidence:       r.Confidence,
		ConfidenceSource: string(r.Source),
		Severity:         string(r.Severity),
		Timestamp:        r.DetectedAt,
		SourceData:       numericMetadata(r.Metadata),
		Duration:         getThemeDuration(r.Theme),
		ExpiresAt:        r.DetectedAt.Add(getThemeDuration(r.Theme)),
		Status:           "active",
	}
}

// numericMetadata extracts float64 values from a generic metadata map so they
// can populate NarrativeEvent.SourceData (map[string]float64). Non-numeric
// entries are silently dropped — the consumer does not have a use for them.
func numericMetadata(m map[string]any) map[string]float64 {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]float64, len(m))
	for k, v := range m {
		if f, ok := v.(float64); ok {
			out[k] = f
		}
	}
	return out
}

// Detector is the Stage 5 abstraction that wraps a single trigger-theme
// detection. Each of the 24 templates in templates.go maps to one Detector
// impl (instantiated in PR#2).
//
// Contract:
//   - Theme() returns the trigger_theme string; it must be stable across calls.
//   - Enabled() / SetEnabled(b) toggle this detector's active flag. The
//     registry honors this flag in ListEnabled() and RunAll().
//   - Detect(ctx, in) returns (nil, nil) when no trigger fired. Errors are
//     reserved for genuine failures (bad config, internal panic, ctx.Err()).
type Detector interface {
	Theme() string
	Enabled() bool
	SetEnabled(enabled bool)
	Detect(ctx context.Context, in DetectorInput) (*DetectionResult, error)
	// PeriodWeight returns a multiplier for this detector's confidence
	// in the given market period. Default is 1.0 (no adjustment).
	// Per ATLAS_METHODOLOGY.md appendix B, certain detectors have
	// elevated sensitivity in specific periods (e.g., US_rates_up ×2
	// during plateau/turnaround_down).
	PeriodWeight(period domain.MarketPeriod) float64
}

// DetectorRegistry holds all registered Detectors keyed by trigger_theme.
// All exported methods are safe for concurrent use.
type DetectorRegistry struct {
	mu        sync.RWMutex
	detectors map[string]Detector
}

// NewDetectorRegistry returns an empty registry. Callers (typically
// NewNarrativeEngine in PR#2) Register() each Detector individually so that
// a missing registration surfaces as a startup-time error rather than a
// silent "no detector" at scan time.
func NewDetectorRegistry() *DetectorRegistry {
	return &DetectorRegistry{
		detectors: make(map[string]Detector),
	}
}

// Register inserts a Detector. Duplicate theme registration returns an error
// to catch accidental double-registration in tests and production wiring.
func (r *DetectorRegistry) Register(d Detector) error {
	if d == nil {
		return fmt.Errorf("narrative: cannot register nil detector")
	}
	theme := d.Theme()
	if theme == "" {
		return fmt.Errorf("narrative: detector has empty theme")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.detectors[theme]; exists {
		return fmt.Errorf("narrative: detector for theme %q already registered", theme)
	}
	r.detectors[theme] = d
	return nil
}

// MustRegister is the panic-on-error variant used in engine init paths where
// duplicate registration indicates a programmer bug (not a runtime condition).
func (r *DetectorRegistry) MustRegister(d Detector) {
	if err := r.Register(d); err != nil {
		panic(err)
	}
}

// Get returns the Detector registered for the given theme, if any.
func (r *DetectorRegistry) Get(theme string) (Detector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.detectors[theme]
	return d, ok
}

// List returns a snapshot of all registered detectors (enabled or not).
// Order is unspecified.
func (r *DetectorRegistry) List() []Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Detector, 0, len(r.detectors))
	for _, d := range r.detectors {
		out = append(out, d)
	}
	return out
}

// ListEnabled returns only detectors whose Enabled() reports true.
// Order is unspecified.
func (r *DetectorRegistry) ListEnabled() []Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Detector, 0, len(r.detectors))
	for _, d := range r.detectors {
		if d.Enabled() {
			out = append(out, d)
		}
	}
	return out
}

// Themes returns the set of registered trigger themes.
// Order is unspecified (map iteration).
func (r *DetectorRegistry) Themes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.detectors))
	for theme := range r.detectors {
		out = append(out, theme)
	}
	return out
}

// Enable activates the detector for the given theme. Returns an error if the
// theme is not registered (so callers don't silently no-op on typos).
func (r *DetectorRegistry) Enable(theme string) error {
	return r.setEnabled(theme, true)
}

// Disable deactivates the detector for the given theme.
func (r *DetectorRegistry) Disable(theme string) error {
	return r.setEnabled(theme, false)
}

func (r *DetectorRegistry) setEnabled(theme string, enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	d, ok := r.detectors[theme]
	if !ok {
		return fmt.Errorf("narrative: detector for theme %q not registered", theme)
	}
	d.SetEnabled(enabled)
	return nil
}

// Len returns the number of registered detectors.
func (r *DetectorRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.detectors)
}

// RunAll executes every enabled Detector in parallel and returns:
//   - results: DetectionResult slice (nil entries filtered out)
func (r *DetectorRegistry) RunAll(ctx context.Context, in DetectorInput) ([]DetectionResult, []error) {
	detectors := r.ListEnabled()
	if len(detectors) == 0 {
		return nil, nil
	}

	type out struct {
		theme  string
		result *DetectionResult
		err    error
		d      Detector
	}
	ch := make(chan out, len(detectors))

	for _, d := range detectors {
		d := d // capture
		go func() {
			res, err := d.Detect(ctx, in)
			ch <- out{theme: d.Theme(), result: res, err: err, d: d}
		}()
	}

	results := make([]DetectionResult, 0, len(detectors))
	var errs []error
	for range detectors {
		o := <-ch
		if o.err != nil {
			errs = append(errs, fmt.Errorf("narrative: detector %s: %w", o.theme, o.err))
			continue
		}
		if o.result != nil {
			// Apply period sensitivity weight (ATLAS_METHODOLOGY.md Appendix B).
			if in.CurrentPeriod != "" {
				w := o.d.PeriodWeight(in.CurrentPeriod)
				if w != 1.0 {
					o.result.Confidence *= w
					if o.result.Confidence > 1.0 {
						o.result.Confidence = 1.0
					}
				}
			}
			results = append(results, *o.result)
		}
	}
	return results, errs
}
