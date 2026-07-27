// Package scheduler — Stage 5 PR#4 template_detector_scan tests.
package scheduler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type stubDetector struct {
	theme  string
	result *narrative.DetectionResult
	err    error
	calls  atomic.Int32
	lastIn narrative.DetectorInput
}

func (d *stubDetector) Theme() string                                   { return d.theme }
func (d *stubDetector) Enabled() bool                                   { return true }
func (d *stubDetector) SetEnabled(bool)                                 {}
func (d *stubDetector) PeriodWeight(period domain.MarketPeriod) float64 { return 1.0 }
func (d *stubDetector) Detect(_ context.Context, in narrative.DetectorInput) (*narrative.DetectionResult, error) {
	d.calls.Add(1)
	d.lastIn = in
	return d.result, d.err
}

type stubScanStore struct {
	mu        sync.Mutex
	calls     int
	results   [][]narrative.DetectionResult
	batchID   string
	appendErr error
	loadRows  []ledger.ScanResultRow
	loadErr   error
}

func (s *stubScanStore) AppendScan(_ context.Context, results []narrative.DetectionResult) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.results = append(s.results, results)
	return s.batchID, s.appendErr
}

func (s *stubScanStore) LoadRecentScans(_ context.Context, _ int) ([]ledger.ScanResultRow, error) {
	return s.loadRows, s.loadErr
}

func TestRegisterTemplateDetectorScanTasks_WiresTask(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{batchID: "test-batch"}

	RegisterTemplateDetectorScanTasks(btm, registry, store, nil, nil)

	task, ok := btm.Get("template_detector_scan")
	if !ok {
		t.Fatal("template_detector_scan not registered")
	}
	if task.Interval != time.Hour {
		t.Errorf("Interval = %v, want 1h", task.Interval)
	}
	if task.Jitter != 6*time.Minute {
		t.Errorf("Jitter = %v, want 6min (auto-set by BackgroundTaskManager)", task.Jitter)
	}
	if !task.Enabled {
		t.Error("task should be enabled by default")
	}
	if task.Task == nil {
		t.Error("Task func should not be nil")
	}
}

func TestRegisterTemplateDetectorScanTasks_DuplicateNameErrorIgnored(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{batchID: "x"}

	RegisterTemplateDetectorScanTasks(btm, registry, store, nil, nil)
	RegisterTemplateDetectorScanTasks(btm, registry, store, nil, nil)

	if _, ok := btm.Get("template_detector_scan"); !ok {
		t.Fatal("task should still be registered after second call (first wins)")
	}
}

func TestRegisterTemplateDetectorScanTasks_NilDeps_NoOp(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{batchID: "x"}

	RegisterTemplateDetectorScanTasks(nil, registry, store, nil, nil)
	RegisterTemplateDetectorScanTasks(btm, nil, store, nil, nil)
	RegisterTemplateDetectorScanTasks(btm, registry, nil, nil, nil)

	if names := btm.List(); len(names) != 0 {
		t.Errorf("expected 0 tasks after nil-dep calls, got %v", names)
	}
}

func TestRegisterTemplateDetectorScanTasks_TaskBodyWritesResults(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{batchID: "batch-abc"}

	expected := &narrative.DetectionResult{
		Theme:      "test_theme",
		Severity:   narrative.SeverityHigh,
		Confidence: 0.9,
		DetectedAt: time.Now().UTC(),
		Source:     narrative.SourceKB,
		Metadata:   map[string]any{"k": "v"},
	}
	if err := registry.Register(&stubDetector{theme: "test_theme", result: expected}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	RegisterTemplateDetectorScanTasks(btm, registry, store, nil, nil)

	task, _ := btm.Get("template_detector_scan")
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task body returned error: %v", err)
	}

	if store.calls != 1 {
		t.Errorf("store.AppendScan called %d times, want 1", store.calls)
	}
	if len(store.results) != 1 || len(store.results[0]) != 1 {
		t.Fatalf("store.results = %+v, want [[<1 result>]]", store.results)
	}
	got := store.results[0][0]
	if got.Theme != expected.Theme || got.Severity != expected.Severity {
		t.Errorf("stored result = %+v, want theme=%s severity=%s", got, expected.Theme, expected.Severity)
	}
}

func TestRegisterTemplateDetectorScanTasks_TaskBodyNoResultsNoWrite(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{batchID: "x"}

	if err := registry.Register(&stubDetector{theme: "silent", result: nil}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	RegisterTemplateDetectorScanTasks(btm, registry, store, nil, nil)

	task, _ := btm.Get("template_detector_scan")
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task body returned error: %v", err)
	}

	if store.calls != 0 {
		t.Errorf("store.AppendScan should not be called when RunAll returns no results, got %d calls", store.calls)
	}
}

func TestRegisterTemplateDetectorScanTasks_TaskBodyPropagatesStoreError(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{
		batchID:   "x",
		appendErr: fmt.Errorf("disk full"),
	}

	if err := registry.Register(&stubDetector{
		theme: "always_fires",
		result: &narrative.DetectionResult{
			Theme: "always_fires", Severity: narrative.SeverityLow,
			DetectedAt: time.Now(), Source: narrative.SourceKB, Confidence: 0.5,
		},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	RegisterTemplateDetectorScanTasks(btm, registry, store, nil, nil)
	task, _ := btm.Get("template_detector_scan")

	err := task.Task(context.Background())
	if err == nil {
		t.Fatal("expected error from store.AppendScan to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error = %v, want to contain 'disk full'", err)
	}
}

func TestRegisterTemplateDetectorScanTasks_ProvidersInjectedIntoDetectorInput(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{batchID: "test-batch"}
	d := &stubDetector{
		theme:  "test_theme",
		result: &narrative.DetectionResult{Theme: "test_theme", Severity: narrative.SeverityLow, Confidence: 0.5},
	}
	if err := registry.Register(d); err != nil {
		t.Fatalf("Register: %v", err)
	}

	wantMacro := marketdata.MacroDataSnapshot{DataStatus: "ok"}
	wantMacro.DXY.Value = 1.5
	wantMarket := narrative.MarketNarrativeData{US10YChangeBps: 42.0, VIXLevel: 18.5}

	macroProvider := func() marketdata.MacroDataSnapshot { return wantMacro }
	marketProvider := func() narrative.MarketNarrativeData { return wantMarket }

	RegisterTemplateDetectorScanTasks(btm, registry, store, macroProvider, marketProvider)
	task, _ := btm.Get("template_detector_scan")
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}

	if d.calls.Load() != 1 {
		t.Fatalf("expected detector called once, got %d", d.calls.Load())
	}
	if d.lastIn.MacroSnapshot.DataStatus != wantMacro.DataStatus {
		t.Errorf("MacroSnapshot.DataStatus = %q, want %q",
			d.lastIn.MacroSnapshot.DataStatus, wantMacro.DataStatus)
	}
	if d.lastIn.MacroSnapshot.DXY.Value != wantMacro.DXY.Value {
		t.Errorf("MacroSnapshot.DXY.Value = %v, want %v",
			d.lastIn.MacroSnapshot.DXY.Value, wantMacro.DXY.Value)
	}
	if d.lastIn.MarketData.US10YChangeBps != wantMarket.US10YChangeBps {
		t.Errorf("MarketData.US10YChangeBps = %v, want %v",
			d.lastIn.MarketData.US10YChangeBps, wantMarket.US10YChangeBps)
	}
	if d.lastIn.MarketData.VIXLevel != wantMarket.VIXLevel {
		t.Errorf("MarketData.VIXLevel = %v, want %v",
			d.lastIn.MarketData.VIXLevel, wantMarket.VIXLevel)
	}
	if d.lastIn.Now.IsZero() {
		t.Error("Now must remain populated regardless of providers")
	}
}

func TestRegisterTemplateDetectorScanTasks_NilProviders_LeavesFieldsZero(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{batchID: "test-batch"}
	d := &stubDetector{
		theme:  "test_theme",
		result: &narrative.DetectionResult{Theme: "test_theme", Severity: narrative.SeverityLow, Confidence: 0.5},
	}
	if err := registry.Register(d); err != nil {
		t.Fatalf("Register: %v", err)
	}

	RegisterTemplateDetectorScanTasks(btm, registry, store, nil, nil)
	task, _ := btm.Get("template_detector_scan")
	if err := task.Task(context.Background()); err != nil {
		t.Fatalf("task: %v", err)
	}

	if d.lastIn.Now.IsZero() {
		t.Error("Now must be populated even when providers are nil")
	}
	if d.lastIn.MacroSnapshot.DataStatus != "" {
		t.Errorf("nil macroProvider should leave MacroSnapshot.DataStatus empty (got %q)",
			d.lastIn.MacroSnapshot.DataStatus)
	}
	if d.lastIn.MarketData.US10YChangeBps != 0 {
		t.Errorf("nil marketProvider should leave MarketData.US10YChangeBps = 0 (got %v)",
			d.lastIn.MarketData.US10YChangeBps)
	}
}
