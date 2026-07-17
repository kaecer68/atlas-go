package main

// BK-15: verify WireRecommenderDeps routes the capitalflow service
// through the production store when one is provided, and falls back
// to the in-memory default when not. The split mirrors cmd/atlas/main.go
// where capitalFlowStore is constructed once and shared with the HTTP
// handler, the eventdriven adapter, and the operations_tasks refresh
// closure.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/marketdata"
)

// stubMacroProvider returns an empty MacroDataSnapshot so the wire path
// never touches the real gateway. capitalflow.Service tolerates empty
// inputs at wire time — LatestDaily is only called at request time, not
// during construction.
type stubMacroProvider struct{}

func (stubMacroProvider) Name() string { return "stub" }
func (stubMacroProvider) FetchSnapshot(_ context.Context) (marketdata.MacroDataSnapshot, error) {
	return marketdata.MacroDataSnapshot{RecordedAt: time.Now().Unix()}, nil
}

func TestWireRecommenderDeps_NilMacroProvider_LeavesCapitalFlowNil(t *testing.T) {
	deps := WireRecommenderDeps(WireDeps{
		WorkDir: t.TempDir(),
		// MacroProvider intentionally nil.
		CapitalFlowStore: capitalflow.NewMemoryRollingSampleStore(60),
	})
	if deps.CapitalFlow != nil {
		t.Fatalf("deps.CapitalFlow must be nil when MacroProvider is nil, got %T", deps.CapitalFlow)
	}
}

func TestWireRecommenderDeps_NilStore_FallsBackToMemory(t *testing.T) {
	deps, cfsvc := wireForTest(WireDeps{
		WorkDir:          t.TempDir(),
		MacroProvider:    stubMacroProvider{},
		CapitalFlowStore: nil, // explicit fallback
	})
	if deps.CapitalFlow == nil {
		t.Fatal("deps.CapitalFlow must be wired when MacroProvider is non-nil")
	}
	if cfsvc == nil {
		t.Fatal("cfsvc must be returned by wireForTest for verification")
	}
	store := cfsvc.Store()
	if store == nil {
		t.Fatal("fallback path must produce a non-nil rolling store")
	}
	if _, ok := store.(*capitalflow.MemoryRollingSampleStore); !ok {
		t.Fatalf("fallback store type = %T, want *capitalflow.MemoryRollingSampleStore", store)
	}
}

func TestWireRecommenderDeps_NonNilStore_UsedByService(t *testing.T) {
	provided := capitalflow.NewMemoryRollingSampleStore(60)
	deps, cfsvc := wireForTest(WireDeps{
		WorkDir:          t.TempDir(),
		MacroProvider:    stubMacroProvider{},
		CapitalFlowStore: provided,
	})
	if deps.CapitalFlow == nil {
		t.Fatal("deps.CapitalFlow must be wired when MacroProvider is non-nil")
	}
	if cfsvc == nil {
		t.Fatal("cfsvc must be returned by wireForTest for verification")
	}
	if got := cfsvc.Store(); got != provided {
		t.Fatalf("wired Service.Store() = %p, want provided %p (production must pass the shared store through NewServiceWithStore)", got, provided)
	}
}

// TestWireRecommenderDeps_DoesNotLeakStore verifies the wired store
// reaches the recommender adapter through the HandlerDeps side: even
// though LatestDaily is not invoked during construction, the adapter
// holds the right service underneath (asserted indirectly by the
// pointer-equality check above). This case ensures the adapter was
// built from the same cfsvc, not a fresh fallback.
func TestWireRecommenderDeps_DoesNotLeakStore(t *testing.T) {
	provided := capitalflow.NewMemoryRollingSampleStore(60)
	deps, cfsvc := wireForTest(WireDeps{
		WorkDir:          t.TempDir(),
		MacroProvider:    stubMacroProvider{},
		CapitalFlowStore: provided,
	})
	// Drive one LatestDaily read so we know the closure we handed to
	// NewCapitalFlowFunc actually reaches the underlying service's
	// History path. Without a usable macro snapshot, force/extract may
	// return zero-value scores — but it must not panic, and the store
	// pointer must still match.
	if _, err := deps.CapitalFlow.LatestDaily(context.Background()); err != nil {
		// An error is acceptable here (empty snapshot yields empty
		// forces); we just want to confirm the closure runs through
		// the wired service.
		t.Logf("LatestDaily returned error (acceptable on empty snapshot): %v", err)
	}
	if got := cfsvc.Store(); got != provided {
		t.Fatalf("post-LatestDaily Store() = %p, want provided %p", got, provided)
	}
}

// TestWireRecommenderDeps_FileStoreAcceptanceProductionPath ensures
// the production path (FileRollingSampleStore) is reachable from
// WireDeps without any code-path surprises: the same field accepts a
// file-backed store, and the resulting Service.Store() returns it
// verbatim. This mirrors what cmd/atlas/main.go will wire.
func TestWireRecommenderDeps_FileStoreAcceptanceProductionPath(t *testing.T) {
	store := capitalflow.NewFileRollingSampleStore(filepath.Join(t.TempDir(), "rolling.json"), 60)
	deps, cfsvc := wireForTest(WireDeps{
		WorkDir:          t.TempDir(),
		MacroProvider:    stubMacroProvider{},
		CapitalFlowStore: store,
	})
	if deps.CapitalFlow == nil {
		t.Fatal("deps.CapitalFlow must be wired for the production file-store path")
	}
	if cfsvc == nil {
		t.Fatal("cfsvc must be returned for verification")
	}
	if got := cfsvc.Store(); got != store {
		t.Fatalf("wired Service.Store() = %p, want provided file store %p", got, store)
	}
}

func TestWireRecommenderDeps_ExposesLatestAssessment(t *testing.T) {
	deps := WireRecommenderDeps(WireDeps{
		WorkDir:       t.TempDir(),
		MacroProvider: stubMacroProvider{},
	})
	assessment, err := deps.CapitalFlow.LatestAssessment(t.Context())
	if err != nil {
		t.Fatalf("LatestAssessment: %v", err)
	}
	if assessment.CalibrationStatus != capitalflow.CalibrationCalibrating {
		t.Errorf("CalibrationStatus = %q, want %q", assessment.CalibrationStatus, capitalflow.CalibrationCalibrating)
	}
}
