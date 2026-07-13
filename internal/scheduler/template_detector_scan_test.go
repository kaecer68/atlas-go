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
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

type stubDetector struct {
	theme  string
	result *narrative.DetectionResult
	err    error
	calls  atomic.Int32
}

func (d *stubDetector) Theme() string   { return d.theme }
func (d *stubDetector) Enabled() bool   { return true }
func (d *stubDetector) SetEnabled(bool) {}
func (d *stubDetector) Detect(_ context.Context, _ narrative.DetectorInput) (*narrative.DetectionResult, error) {
	d.calls.Add(1)
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

	RegisterTemplateDetectorScanTasks(btm, registry, store)

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

	RegisterTemplateDetectorScanTasks(btm, registry, store)
	RegisterTemplateDetectorScanTasks(btm, registry, store)

	if _, ok := btm.Get("template_detector_scan"); !ok {
		t.Fatal("task should still be registered after second call (first wins)")
	}
}

func TestRegisterTemplateDetectorScanTasks_NilDeps_NoOp(t *testing.T) {
	btm := apigateway.NewBackgroundTaskManager(nil)
	registry := narrative.NewDetectorRegistry()
	store := &stubScanStore{batchID: "x"}

	RegisterTemplateDetectorScanTasks(nil, registry, store)
	RegisterTemplateDetectorScanTasks(btm, nil, store)
	RegisterTemplateDetectorScanTasks(btm, registry, nil)

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

	RegisterTemplateDetectorScanTasks(btm, registry, store)

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

	RegisterTemplateDetectorScanTasks(btm, registry, store)

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

	RegisterTemplateDetectorScanTasks(btm, registry, store)
	task, _ := btm.Get("template_detector_scan")

	err := task.Task(context.Background())
	if err == nil {
		t.Fatal("expected error from store.AppendScan to propagate, got nil")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error = %v, want to contain 'disk full'", err)
	}
}
