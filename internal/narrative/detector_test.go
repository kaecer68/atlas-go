package narrative

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// mockDetector is a test-only Detector that records calls and returns
// configurable results/errors so we can exercise Registry.RunAll semantics
// without depending on any of the 24 real detectors (which arrive in PR#2).
type mockDetector struct {
	theme   string
	enabled bool
	result  *DetectionResult
	err     error
	delay   time.Duration
	mu      sync.Mutex
	lastIn  DetectorInput
}

func newMockDetector(theme string, enabled bool, res *DetectionResult, err error) *mockDetector {
	return &mockDetector{theme: theme, enabled: enabled, result: res, err: err}
}

func (m *mockDetector) Theme() string                                   { return m.theme }
func (m *mockDetector) Enabled() bool                                   { return m.enabled }
func (m *mockDetector) SetEnabled(b bool)                               { m.enabled = b }
func (m *mockDetector) PeriodWeight(period domain.MarketPeriod) float64 { return 1.0 }

func (m *mockDetector) Detect(ctx context.Context, in DetectorInput) (*DetectionResult, error) {
	if m.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(m.delay):
		}
	}
	m.mu.Lock()
	m.lastIn = in
	m.mu.Unlock()
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

// emptyThemeDetector is a minimal Detector used solely to verify that
// Registry.Register rejects empty theme strings. It is defined standalone
// (not embedded into mockDetector) because embedding would pull in the
// mockDetector's sync.Mutex into a value-receiver method, tripping go vet's
// "passes lock by value" warning.
type emptyThemeDetector struct{}

func (*emptyThemeDetector) Theme() string                                   { return "" }
func (*emptyThemeDetector) Enabled() bool                                   { return false }
func (*emptyThemeDetector) SetEnabled(bool)                                 {}
func (*emptyThemeDetector) PeriodWeight(period domain.MarketPeriod) float64 { return 1.0 }
func (*emptyThemeDetector) Detect(context.Context, DetectorInput) (*DetectionResult, error) {
	return nil, nil
}

// ----------------------------------------------------------------------------
// Construction & basic invariants
// ----------------------------------------------------------------------------

func TestNewDetectorRegistry_Empty(t *testing.T) {
	r := NewDetectorRegistry()
	if got := r.Len(); got != 0 {
		t.Fatalf("new registry Len = %d, want 0", got)
	}
	if got := r.List(); len(got) != 0 {
		t.Fatalf("new registry List len = %d, want 0", len(got))
	}
	if got := r.ListEnabled(); len(got) != 0 {
		t.Fatalf("new registry ListEnabled len = %d, want 0", len(got))
	}
	if got := r.Themes(); len(got) != 0 {
		t.Fatalf("new registry Themes len = %d, want 0", len(got))
	}
}

// ----------------------------------------------------------------------------
// Register / Get / duplicate / nil / empty-theme
// ----------------------------------------------------------------------------

func TestDetectorRegistry_RegisterAndGet(t *testing.T) {
	r := NewDetectorRegistry()
	d := newMockDetector("US_rates_up", true, &DetectionResult{Theme: "US_rates_up"}, nil)
	if err := r.Register(d); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if got := r.Len(); got != 1 {
		t.Fatalf("Len after Register = %d, want 1", got)
	}
	got, ok := r.Get("US_rates_up")
	if !ok {
		t.Fatalf("Get(US_rates_up) ok = false, want true")
	}
	if got.Theme() != "US_rates_up" {
		t.Fatalf("Get returned theme %q, want US_rates_up", got.Theme())
	}
}

func TestDetectorRegistry_RegisterNil(t *testing.T) {
	r := NewDetectorRegistry()
	if err := r.Register(nil); err == nil {
		t.Fatalf("Register(nil) err = nil, want error")
	}
}

func TestDetectorRegistry_RegisterEmptyTheme(t *testing.T) {
	r := NewDetectorRegistry()
	err := r.Register(&emptyThemeDetector{})
	if err == nil {
		t.Fatalf("Register(empty theme) err = nil, want error")
	}
}

func TestDetectorRegistry_RegisterDuplicate(t *testing.T) {
	r := NewDetectorRegistry()
	a := newMockDetector("US_rates_up", true, nil, nil)
	b := newMockDetector("US_rates_up", true, nil, nil)
	if err := r.Register(a); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(b); err == nil {
		t.Fatalf("duplicate Register err = nil, want error")
	}
	if got := r.Len(); got != 1 {
		t.Fatalf("Len after duplicate Register = %d, want 1", got)
	}
}

func TestDetectorRegistry_GetUnregistered(t *testing.T) {
	r := NewDetectorRegistry()
	if _, ok := r.Get("does_not_exist"); ok {
		t.Fatalf("Get(unregistered) ok = true, want false")
	}
}

func TestDetectorRegistry_MustRegisterPanicsOnDuplicate(t *testing.T) {
	r := NewDetectorRegistry()
	r.MustRegister(newMockDetector("X", true, nil, nil))
	defer func() {
		if recover() == nil {
			t.Fatalf("MustRegister duplicate did not panic")
		}
	}()
	r.MustRegister(newMockDetector("X", true, nil, nil))
}

// ----------------------------------------------------------------------------
// Enable / Disable
// ----------------------------------------------------------------------------

func TestDetectorRegistry_EnableDisable(t *testing.T) {
	r := NewDetectorRegistry()
	d := newMockDetector("US_rates_up", false, nil, nil)
	r.MustRegister(d)

	if err := r.Enable("US_rates_up"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !d.Enabled() {
		t.Fatalf("after Enable detector.Enabled() = false, want true")
	}

	if err := r.Disable("US_rates_up"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	if d.Enabled() {
		t.Fatalf("after Disable detector.Enabled() = true, want false")
	}
}

func TestDetectorRegistry_EnableUnregistered(t *testing.T) {
	r := NewDetectorRegistry()
	if err := r.Enable("ghost"); err == nil {
		t.Fatalf("Enable(unregistered) err = nil, want error")
	}
	if err := r.Disable("ghost"); err == nil {
		t.Fatalf("Disable(unregistered) err = nil, want error")
	}
}

func TestDetectorRegistry_ListEnabledFilters(t *testing.T) {
	r := NewDetectorRegistry()
	r.MustRegister(newMockDetector("a", true, nil, nil))
	r.MustRegister(newMockDetector("b", false, nil, nil))
	r.MustRegister(newMockDetector("c", true, nil, nil))

	got := r.ListEnabled()
	if len(got) != 2 {
		t.Fatalf("ListEnabled len = %d, want 2", len(got))
	}
	seen := map[string]bool{}
	for _, d := range got {
		seen[d.Theme()] = true
	}
	if !seen["a"] || !seen["c"] || seen["b"] {
		t.Fatalf("ListEnabled themes = %v, want {a, c}", seen)
	}
}

// ----------------------------------------------------------------------------
// RunAll — happy path, error isolation, parallel execution, no-op cases
// ----------------------------------------------------------------------------

func TestDetectorRegistry_RunAll_NoEnabled(t *testing.T) {
	r := NewDetectorRegistry()
	r.MustRegister(newMockDetector("a", false, nil, nil))
	results, errs := r.RunAll(context.Background(), DetectorInput{})
	if len(results) != 0 || len(errs) != 0 {
		t.Fatalf("RunAll with no enabled detectors = (%v, %v), want (nil, nil)", results, errs)
	}
}

func TestDetectorRegistry_RunAll_CollectsResults(t *testing.T) {
	r := NewDetectorRegistry()
	r.MustRegister(newMockDetector("a", true,
		&DetectionResult{Theme: "a", Severity: SeverityLow, Confidence: 0.3, Source: SourceKB, DetectedAt: time.Now()},
		nil))
	r.MustRegister(newMockDetector("b", true,
		&DetectionResult{Theme: "b", Severity: SeverityHigh, Confidence: 0.8, Source: SourceIngestor, DetectedAt: time.Now()},
		nil))
	r.MustRegister(newMockDetector("c", false, nil, nil)) // disabled — should be skipped

	results, errs := r.RunAll(context.Background(), DetectorInput{})
	if len(errs) != 0 {
		t.Fatalf("RunAll errs = %v, want nil", errs)
	}
	if len(results) != 2 {
		t.Fatalf("RunAll results len = %d, want 2", len(results))
	}
}

func TestDetectorRegistry_RunAll_ErrorIsolation(t *testing.T) {
	r := NewDetectorRegistry()
	boom := errors.New("boom")
	r.MustRegister(newMockDetector("good", true,
		&DetectionResult{Theme: "good", Confidence: 0.5, Source: SourceKB, DetectedAt: time.Now()}, nil))
	r.MustRegister(newMockDetector("bad", true, nil, boom))
	r.MustRegister(newMockDetector("also_good", true,
		&DetectionResult{Theme: "also_good", Confidence: 0.7, Source: SourceKB, DetectedAt: time.Now()}, nil))

	results, errs := r.RunAll(context.Background(), DetectorInput{})
	if len(results) != 2 {
		t.Fatalf("results len = %d, want 2 (one detector errored but did not abort)", len(results))
	}
	if len(errs) != 1 {
		t.Fatalf("errs len = %d, want 1", len(errs))
	}
	if errs[0].Error() != "narrative: detector bad: boom" {
		t.Fatalf("err message = %q, want %q", errs[0].Error(), "narrative: detector bad: boom")
	}
}

func TestDetectorRegistry_RunAll_NilResultFiltered(t *testing.T) {
	r := NewDetectorRegistry()
	r.MustRegister(newMockDetector("silent", true, nil, nil)) // returns (nil, nil) — no trigger
	results, errs := r.RunAll(context.Background(), DetectorInput{})
	if len(results) != 0 {
		t.Fatalf("results len = %d, want 0", len(results))
	}
	if len(errs) != 0 {
		t.Fatalf("errs = %v, want nil", errs)
	}
}

func TestDetectorRegistry_RunAll_ParallelExecution(t *testing.T) {
	// 4 detectors each sleeping 50ms — if RunAll serialized, total >= 200ms.
	// In parallel, total should be ~50ms. Allow generous slack for CI.
	const n = 4
	const sleep = 50 * time.Millisecond
	r := NewDetectorRegistry()
	for i := 0; i < n; i++ {
		theme := string(rune('a' + i))
		d := newMockDetector(theme, true,
			&DetectionResult{Theme: theme, DetectedAt: time.Now()}, nil)
		d.delay = sleep
		r.MustRegister(d)
	}

	start := time.Now()
	results, errs := r.RunAll(context.Background(), DetectorInput{})
	elapsed := time.Since(start)

	if len(results) != n || len(errs) != 0 {
		t.Fatalf("results=%d errs=%d, want %d/0", len(results), len(errs), n)
	}
	// Parallelism floor: must be << n*sleep. Use n*sleep/2 as a conservative
	// bound; in practice it should be ~sleep.
	if elapsed >= n*sleep/2 {
		t.Fatalf("RunAll elapsed = %v, expected ~%v (parallel execution)", elapsed, sleep)
	}
}

func TestDetectorRegistry_RunAll_RespectsContext(t *testing.T) {
	r := NewDetectorRegistry()
	d := newMockDetector("slow", true,
		&DetectionResult{Theme: "slow", DetectedAt: time.Now()}, nil)
	d.delay = 200 * time.Millisecond
	r.MustRegister(d)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, errs := r.RunAll(ctx, DetectorInput{})
	if len(errs) == 0 {
		t.Fatalf("expected context-cancellation error, got none")
	}
	if !errors.Is(errs[0], context.DeadlineExceeded) {
		t.Fatalf("err = %v, want wraps context.DeadlineExceeded", errs[0])
	}
}

// ----------------------------------------------------------------------------
// ToNarrativeEvent projection
// ----------------------------------------------------------------------------

func TestDetectionResult_ToNarrativeEvent(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	res := DetectionResult{
		Theme:      "US_rates_up",
		Severity:   SeverityCritical,
		Confidence: 0.85,
		DetectedAt: now,
		Source:     SourceKB,
		Metadata: map[string]any{
			"US10YChangeBps": 25.0,
			"DXYChangePct":   0.015,
			"label":          "non-numeric should be dropped",
		},
	}
	evt := res.ToNarrativeEvent()

	if evt.Theme != "US_rates_up" {
		t.Errorf("Theme = %q, want US_rates_up", evt.Theme)
	}
	if evt.Confidence != 0.85 {
		t.Errorf("Confidence = %v, want 0.85", evt.Confidence)
	}
	if evt.ConfidenceSource != "kb_pipeline" {
		t.Errorf("ConfidenceSource = %q, want kb_pipeline", evt.ConfidenceSource)
	}
	if evt.Severity != "critical" {
		t.Errorf("Severity = %q, want critical", evt.Severity)
	}
	if evt.Status != "active" {
		t.Errorf("Status = %q, want active", evt.Status)
	}
	if !evt.Timestamp.Equal(now) {
		t.Errorf("Timestamp = %v, want %v", evt.Timestamp, now)
	}
	if evt.SourceData["US10YChangeBps"] != 25.0 {
		t.Errorf("SourceData[US10YChangeBps] = %v, want 25.0", evt.SourceData["US10YChangeBps"])
	}
	if evt.SourceData["DXYChangePct"] != 0.015 {
		t.Errorf("SourceData[DXYChangePct] = %v, want 0.015", evt.SourceData["DXYChangePct"])
	}
	if _, hasLabel := evt.SourceData["label"]; hasLabel {
		t.Errorf("non-numeric metadata leaked into SourceData")
	}
	if evt.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0 (getThemeDuration should provide a value)", evt.Duration)
	}
	if !evt.ExpiresAt.Equal(now.Add(evt.Duration)) {
		t.Errorf("ExpiresAt = %v, want %v", evt.ExpiresAt, now.Add(evt.Duration))
	}
}

func TestDetectionResult_ToNarrativeEvent_NilMetadata(t *testing.T) {
	res := DetectionResult{Theme: "X", DetectedAt: time.Now(), Source: SourceKB}
	evt := res.ToNarrativeEvent()
	if evt.SourceData != nil {
		t.Errorf("SourceData = %v, want nil when metadata is empty", evt.SourceData)
	}
}

// ----------------------------------------------------------------------------
// Concurrent access safety
// ----------------------------------------------------------------------------

func TestDetectorRegistry_ConcurrentAccess(t *testing.T) {
	// Smoke test: many goroutines doing Register / Get / Enable / Disable /
	// ListEnabled / RunAll concurrently must not race or panic.
	r := NewDetectorRegistry()
	r.MustRegister(newMockDetector("a", true, nil, nil))
	r.MustRegister(newMockDetector("b", true, nil, nil))

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			ctx := context.Background()
			for j := 0; j < 50; j++ {
				switch (i + j) % 5 {
				case 0:
					_, _ = r.Get("a")
					_ = r.Themes()
				case 1:
					_ = r.Enable("a")
					_ = r.Disable("a")
				case 2:
					_ = r.List()
					_ = r.ListEnabled()
				case 3:
					_, _ = r.RunAll(ctx, DetectorInput{})
				case 4:
					_ = r.Len()
				}
			}
		}(i)
	}
	wg.Wait()
}
