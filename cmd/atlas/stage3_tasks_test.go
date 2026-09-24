package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func TestRegisterStage3Tasks_RegistersAllFiveTasksInBTM(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{
		WorkDir:            tmp,
		LedgerDir:          tmp,
		ReplayMode:         "disabled",
		Stage3TasksEnabled: true,
	}

	store := ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)

	d := stage3Deps{
		taskMgr:          btm,
		cfg:              cfg,
		gateway:          nil,
		monitor:          nil,
		dashboard:        nil,
		eventCalendar:    nil,
		predictionLedger: store,
	}
	registerStage3Tasks(d)

	registered := btm.List()
	want := []string{
		"sync-events-daily",
		"sync-macro-daily",
		"sync-capital-daily",
		"sync-regime-weekly",
		"recalibrate-templates-monthly",
	}
	for _, name := range want {
		found := slices.Contains(registered, name)
		if !found {
			t.Fatalf("expected %q registered in BTM; got %v", name, registered)
		}
	}
}

func TestRegisterStage3AlertTasks_RegistersThreeEvaluatorsInBTM(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{
		WorkDir:             tmp,
		LedgerDir:           tmp,
		ReplayMode:          "disabled",
		Stage3TasksEnabled:  true,
		Stage3AlertsEnabled: true,
	}
	store := ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)

	d := stage3Deps{
		taskMgr:          btm,
		cfg:              cfg,
		monitor:          monitoring.NewMonitor(),
		eventCalendar:    nil,
		predictionLedger: store,
	}
	registerStage3AlertTasks(d)

	registered := btm.List()
	want := []string{
		"stage3-alert-staleness",
		"stage3-alert-daily",
		"stage3-alert-market-close",
	}
	for _, name := range want {
		found := slices.Contains(registered, name)
		if !found {
			t.Fatalf("expected %q registered in BTM; got %v", name, registered)
		}
	}
}

func TestRegisterStage3Tasks_RespectsOptOutFlagFalse(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{
		WorkDir:            tmp,
		LedgerDir:          tmp,
		ReplayMode:         "disabled",
		Stage3TasksEnabled: false,
	}
	store := ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)

	d := stage3Deps{
		taskMgr:          btm,
		cfg:              cfg,
		predictionLedger: store,
	}
	registerStage3Tasks(d)

	if registered := btm.List(); len(registered) != 0 {
		t.Fatalf("expected 0 tasks registered when Stage3TasksEnabled=false; got %v", registered)
	}
}

func TestRegisterStage3AlertTasks_RespectsOptOutFlagFalse(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{
		WorkDir:             tmp,
		LedgerDir:           tmp,
		ReplayMode:          "disabled",
		Stage3TasksEnabled:  true,
		Stage3AlertsEnabled: false,
	}
	store := ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)
	monitor := monitoring.NewMonitor()

	d := stage3Deps{
		taskMgr:          btm,
		cfg:              cfg,
		monitor:          monitor,
		predictionLedger: store,
	}
	registerStage3AlertTasks(d)

	if registered := btm.List(); len(registered) != 0 {
		t.Fatalf("expected 0 alerts registered when Stage3AlertsEnabled=false; got %v", registered)
	}
}

// ===========================================================================
// Issue #1941 — Stage 3 wiring + shared-store capital-flow actuals.
//
// Root cause being guarded: registerStage3Tasks / registerStage3AlertTasks
// were fully implemented but never called from main.go, so the
// STAGE3_TASKS_ENABLED / STAGE3_ALERTS_ENABLED config flags gated nothing;
// and LatestCapitalFlowActual built a throwaway capitalflow service whose
// rolling window was empty (every Z = 0), making the prediction-vs-actual
// comparison meaningless.
// ===========================================================================

// TestWireStage3_RegistersThreeTasksAndFiveAlerts proves the single production
// entry point registers the full Stage 3 surface (5 scheduled tasks + 3 alert
// evaluators) when both config flags are on.
func TestWireStage3_RegistersThreeTasksAndFiveAlerts(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)
	d := stage3Deps{
		taskMgr: btm,
		cfg: config.Config{
			WorkDir:             tmp,
			LedgerDir:           tmp,
			ReplayMode:          "disabled",
			Stage3TasksEnabled:  true,
			Stage3AlertsEnabled: true,
		},
		monitor:          monitoring.NewMonitor(),
		predictionLedger: ledger.NewJSONLEventFlowPredictionStore(tmp),
	}

	wireStage3(d)

	registered := btm.List()
	for _, name := range []string{
		"sync-events-daily",
		"sync-macro-daily",
		"sync-capital-daily",
		"sync-regime-weekly",
		"recalibrate-templates-monthly",
		"stage3-alert-staleness",
		"stage3-alert-daily",
		"stage3-alert-market-close",
	} {
		if !slices.Contains(registered, name) {
			t.Errorf("wireStage3 must register %q; registered=%v", name, registered)
		}
	}
	if len(registered) != 8 {
		t.Errorf("wireStage3 registered %d tasks, want 8: %v", len(registered), registered)
	}
}

// TestWireStage3_BothFlagsOffRegistersNothing locks the config-flag contract:
// the flags are read (the pre-#1941 bug was that nobody read them).
func TestWireStage3_BothFlagsOffRegistersNothing(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)
	d := stage3Deps{
		taskMgr: btm,
		cfg: config.Config{
			WorkDir:             tmp,
			LedgerDir:           tmp,
			Stage3TasksEnabled:  false,
			Stage3AlertsEnabled: false,
		},
		monitor: monitoring.NewMonitor(),
	}

	wireStage3(d)

	if registered := btm.List(); len(registered) != 0 {
		t.Fatalf("expected 0 registrations with both flags off; got %v", registered)
	}
}

// TestWireStage3_NilMonitorKeepsTasksSkipsAlerts documents the nil-monitor
// guard: NewStage3AlertEvaluator panics on a nil monitor, so alert wiring is
// skipped while the scheduled tasks still register.
func TestWireStage3_NilMonitorKeepsTasksSkipsAlerts(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)
	d := stage3Deps{
		taskMgr: btm,
		cfg: config.Config{
			WorkDir:             tmp,
			LedgerDir:           tmp,
			Stage3TasksEnabled:  true,
			Stage3AlertsEnabled: true,
		},
		monitor: nil,
	}

	wireStage3(d)

	registered := btm.List()
	if len(registered) != 5 {
		t.Fatalf("expected the 5 scheduled tasks with a nil monitor; got %v", registered)
	}
	for _, name := range registered {
		if slices.Contains([]string{"stage3-alert-staleness", "stage3-alert-daily", "stage3-alert-market-close"}, name) {
			t.Errorf("alert task %q must not register without a monitor", name)
		}
	}
}

// TestMainGoCallsWireStage3 is the regression guard for the #1941 root cause:
// the Stage 3 registrations existed but main.go never called them. A single
// source-level assertion keeps a future refactor from silently dropping the
// call site again.
func TestMainGoCallsWireStage3(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if !bytes.Contains(src, []byte("wireStage3(stage3Deps{")) {
		t.Fatal("main.go must call wireStage3(stage3Deps{...}) — without it STAGE3_TASKS_ENABLED / STAGE3_ALERTS_ENABLED gate nothing and the Stage 3 tasks never run (issue #1941)")
	}
}

// TestBuildStage3AlertDeps_ActualUsesSharedRollingStore reproduces the #1941
// defect and its fix: with a shared service backed by the persisted rolling
// store the realized QualityScore is non-zero (Z is computed from the seeded
// window), while the old throwaway service returns exactly 0 because its
// window is empty.
func TestBuildStage3AlertDeps_ActualUsesSharedRollingStore(t *testing.T) {
	tmp := t.TempDir()
	store := capitalflow.NewFileRollingSampleStore(filepath.Join(tmp, "capital_flow_rolling.json"), 252)
	if err := seedRollingStore(store, "2026-09-17"); err != nil {
		t.Fatalf("seed rolling store: %v", err)
	}

	provider := stage3StubMacroProvider{snap: stage3StubSnapshot()}
	shared := capitalflow.NewServiceWithStore(provider, 0, store, nil)

	deps := buildStage3AlertDeps(stage3Deps{
		cfg:         config.Config{LedgerDir: tmp},
		capitalFlow: shared,
	}, time.UTC)

	sig, ok := deps.LatestCapitalFlowActual()
	if !ok {
		t.Fatal("LatestCapitalFlowActual() unavailable with a shared service wired")
	}
	if sig.Value == 0 {
		t.Fatal("LatestCapitalFlowActual().Value = 0 with a seeded shared rolling store — the comparison is against a fabricated zero (issue #1941)")
	}
	if sig.Direction == "neutral" {
		t.Errorf("expected a non-neutral actual direction from the seeded window; got %q (value=%v)", sig.Direction, sig.Value)
	}

	// The pre-#1941 wiring (throwaway service, no store) yields exactly zero —
	// which is what made the drift comparison meaningless.
	throwaway := capitalflow.NewService(provider, 0, nil)
	report, err := throwaway.LatestDaily(context.Background())
	if err != nil {
		t.Fatalf("throwaway LatestDaily: %v", err)
	}
	if report.QualityScore != 0 {
		t.Fatalf("throwaway service QualityScore = %v, want 0 (that is the defect being fixed)", report.QualityScore)
	}
}

// TestBuildStage3AlertDeps_NilServiceIsUnavailable asserts the nil-service path
// degrades to "unavailable" instead of reporting a fabricated zero.
func TestBuildStage3AlertDeps_NilServiceIsUnavailable(t *testing.T) {
	deps := buildStage3AlertDeps(stage3Deps{cfg: config.Config{LedgerDir: t.TempDir()}}, time.UTC)
	if _, ok := deps.LatestCapitalFlowActual(); ok {
		t.Fatal("LatestCapitalFlowActual() must report unavailable when no shared service is wired")
	}
}

// TestStage3DriftRecorder_WritesPredictionVsActualRecord covers the Stage-3
// predicted-vs-actual observation artifact (hit and miss).
func TestStage3DriftRecorder_WritesPredictionVsActualRecord(t *testing.T) {
	tmp := t.TempDir()
	rec := newStage3DriftRecorder(tmp)
	now := time.Date(2026, 9, 24, 5, 45, 0, 0, time.UTC)

	if err := rec.record(now, monitoring.CapitalFlowSignal{Direction: "bullish", Value: 0.7},
		monitoring.CapitalFlowSignal{Direction: "bullish", Value: 1.25}); err != nil {
		t.Fatalf("Record(hit): %v", err)
	}
	if err := rec.record(now, monitoring.CapitalFlowSignal{Direction: "bullish", Value: 0.7},
		monitoring.CapitalFlowSignal{Direction: "bearish", Value: -1.1}); err != nil {
		t.Fatalf("Record(miss): %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, stage3DriftRecordFile))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 records, got %d: %q", len(lines), string(data))
	}
	var first, second stage3DriftRecord
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal record 1: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("unmarshal record 2: %v", err)
	}
	if !first.Hit || second.Hit {
		t.Errorf("Hit flags = (%v, %v), want (true, false)", first.Hit, second.Hit)
	}
	if first.PredictedDirection != "bullish" || first.ActualDirection != "bullish" || first.ActualValue != 1.25 {
		t.Errorf("record 1 = %+v, want the predicted/actual pair preserved", first)
	}
	if first.TradingDate != "2026-09-24" {
		t.Errorf("TradingDate = %q, want 2026-09-24 (Asia/Taipei)", first.TradingDate)
	}
	if first.ActualSource == "" {
		t.Error("ActualSource must name where the realized value came from")
	}
}

// TestStage3DriftRecorder_CapsArtifactSize keeps the JSONL artifact bounded.
func TestStage3DriftRecorder_CapsArtifactSize(t *testing.T) {
	tmp := t.TempDir()
	rec := newStage3DriftRecorder(tmp)
	now := time.Date(2026, 9, 24, 5, 45, 0, 0, time.UTC)
	for i := 0; i < stage3DriftRecordCap+5; i++ {
		if err := rec.record(now, monitoring.CapitalFlowSignal{Direction: "bullish", Value: float64(i)},
			monitoring.CapitalFlowSignal{Direction: "bullish", Value: float64(i)}); err != nil {
			t.Fatalf("Record(%d): %v", i, err)
		}
	}
	lines, err := readJSONLLines(filepath.Join(tmp, stage3DriftRecordFile))
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	if len(lines) != stage3DriftRecordCap {
		t.Fatalf("artifact holds %d records, want the cap %d", len(lines), stage3DriftRecordCap)
	}
}

// TestNewStage3DriftRecorder_EmptyLedgerDirIsNoop keeps a misconfigured
// process from writing into the repo root.
func TestNewStage3DriftRecorder_EmptyLedgerDirIsNoop(t *testing.T) {
	if rec := newStage3DriftRecorder(""); rec != nil {
		t.Fatal("empty ledger dir must yield a nil (no-op) recorder")
	}
	var nilRec *stage3DriftRecorder
	if err := nilRec.record(time.Now(), monitoring.CapitalFlowSignal{}, monitoring.CapitalFlowSignal{}); err != nil {
		t.Fatalf("nil recorder must be a no-op; got %v", err)
	}
}

// --- helpers -------------------------------------------------------------

// stage3StubMacroProvider serves a fixed snapshot to the capital-flow service.
type stage3StubMacroProvider struct{ snap marketdata.MacroDataSnapshot }

func (p stage3StubMacroProvider) Name() string { return "stage3-stub" }

func (p stage3StubMacroProvider) FetchSnapshot(context.Context) (marketdata.MacroDataSnapshot, error) {
	return p.snap, nil
}

// stage3StubSnapshot yields available foreign / institutional / retail
// dimensions with raw values well outside the seeded reference window.
func stage3StubSnapshot() marketdata.MacroDataSnapshot {
	return marketdata.MacroDataSnapshot{
		ForeignInvestorNet:  marketdata.MacroDataPoint{Symbol: "T86", Value: 500},
		DomesticFundNet:     marketdata.MacroDataPoint{Symbol: "T86", Value: 400},
		RetailMarginBalance: marketdata.MacroDataPoint{Symbol: "MI_MARGN", ChangePct: 2},
		RetailShortBalance:  marketdata.MacroDataPoint{Symbol: "MI_MARGN", ChangePct: 1},
		RecordedAt:          time.Date(2026, 9, 17, 5, 30, 0, 0, time.UTC).Unix(),
	}
}

// seedRollingStore writes 30 prior trading-day samples per dimension, strictly
// before asOf, with a small dispersion so the Z-score is finite and clearly
// non-zero against the stub snapshot.
func seedRollingStore(store capitalflow.RollingSampleStore, asOf string) error {
	ctx := context.Background()
	base := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	dims := map[capitalflow.ForceName]float64{
		capitalflow.ForceForeign:       100,
		capitalflow.ForceInstitutional: 80,
		capitalflow.ForceRetail:        1,
	}
	for i := 0; i < 30; i++ {
		date := base.AddDate(0, 0, i).Format("2006-01-02")
		if date >= asOf {
			break
		}
		samples := make([]capitalflow.RollingSample, 0, len(dims))
		for dim, center := range dims {
			drift := float64(i%5) - 2 // -2..+2 spread keeps stddev > 0
			samples = append(samples, capitalflow.RollingSample{
				TradingDate: date,
				Dimension:   dim,
				RawValue:    center + drift,
				Unit:        "test",
				SourceID:    "SRC-TEST",
			})
		}
		if err := store.UpsertDay(ctx, date, samples); err != nil {
			return err
		}
	}
	return nil
}

// TestBuildStage3AlertDeps_EmptySharedStoreIsUnavailable closes the remaining
// hole in the #1941 fix: a shared service wired to a rolling store that was
// never written still returns QualityScore=0 for every dimension (Z=0), which
// would look like a real "neutral" actual. The closure must report
// unavailable instead of comparing against that zero.
func TestBuildStage3AlertDeps_EmptySharedStoreIsUnavailable(t *testing.T) {
	tmp := t.TempDir()
	// Note: no seedRollingStore call — the store exists but holds no samples.
	store := capitalflow.NewFileRollingSampleStore(filepath.Join(tmp, "capital_flow_rolling.json"), 252)
	provider := stage3StubMacroProvider{snap: stage3StubSnapshot()}
	shared := capitalflow.NewServiceWithStore(provider, 0, store, nil)

	deps := buildStage3AlertDeps(stage3Deps{
		cfg:         config.Config{LedgerDir: tmp},
		capitalFlow: shared,
	}, time.UTC)

	if sig, ok := deps.LatestCapitalFlowActual(); ok {
		t.Fatalf("empty rolling store must not produce an actual reading; got ok=true value=%v direction=%q", sig.Value, sig.Direction)
	}
}

// TestHasCalibrationEvidence documents the guard: only dimensions whose source
// is present and whose reference window is non-empty count as evidence.
func TestHasCalibrationEvidence(t *testing.T) {
	cases := []struct {
		name   string
		forces []capitalflow.ForceScore
		want   bool
	}{
		{name: "empty", forces: nil, want: false},
		{
			name: "unavailable_source_only",
			forces: []capitalflow.ForceScore{
				{Force: capitalflow.ForceForeign, DataAvailable: false, SampleCount: 40},
			},
			want: false,
		},
		{
			name: "available_but_no_samples",
			forces: []capitalflow.ForceScore{
				{Force: capitalflow.ForceForeign, DataAvailable: true, SampleCount: 0},
			},
			want: false,
		},
		{
			name: "available_with_samples",
			forces: []capitalflow.ForceScore{
				{Force: capitalflow.ForceForeign, DataAvailable: true, SampleCount: 3},
			},
			want: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasCalibrationEvidence(tc.forces); got != tc.want {
				t.Errorf("hasCalibrationEvidence(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
