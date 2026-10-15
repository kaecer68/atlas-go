package calibration

// calibrate_darwinian_store_test.go — Q2b2 ledger seam (station 3).
//
// CalibrateDarwinianWithStore takes an injected ledger.OutcomeStore so the weekly
// auto_calibrate job reads PostgreSQL (the source of truth) instead of the flat
// JSONL mirror that session_summary_reconcile backfills every 12h. These tests
// pin the resolution order: injected store first, the mirror only as a fallback,
// and never as a silent second guess when the store succeeded.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/ledger"
)

// writeSessionOutcomes writes perAgent outcomes for each of six agents into
// <root>/data/state/sessions/<session>/recommendation_outcomes.jsonl — the
// layout both the legacy mirror and a JSONL outcome store read. It returns the
// ledger directory.
func writeSessionOutcomes(t *testing.T, root string, scale float64, perAgent int) string {
	t.Helper()
	ledgerDir := filepath.Join(root, "data", "state")
	sessionDir := filepath.Join(ledgerDir, "sessions", "session-20261016-daily")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	f, err := os.Create(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"))
	if err != nil {
		t.Fatalf("create outcomes: %v", err)
	}
	enc := json.NewEncoder(f)
	agents := []string{"agent-a", "agent-b", "agent-c", "agent-d", "agent-e", "agent-f"}
	for i, agent := range agents {
		for j := range perAgent {
			outcome := recommendation.RecommendationOutcome{
				AgentID:       agent,
				Symbol:        "2330",
				Side:          "buy",
				Conviction:    5,
				Window:        "1d",
				Hit:           i%2 == 0,
				ForwardReturn: scale * (float64(i+1) + 0.1*float64(j)),
			}
			if err := enc.Encode(outcome); err != nil {
				t.Fatalf("encode outcome: %v", err)
			}
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close outcomes: %v", err)
	}
	return ledgerDir
}

// jsonlOutcomeStore opens a real JSONL outcome store on dir — the backend
// production uses for the mirror.
func jsonlOutcomeStore(t *testing.T, dir string) ledger.OutcomeStore {
	t.Helper()
	store, err := ledger.NewOutcomeStore(config.Config{StoreBackend: "jsonl", LedgerDir: dir})
	if err != nil {
		t.Fatalf("open jsonl outcome store: %v", err)
	}
	return store
}

// fixedOutcomeStore returns its configured outcomes (or error) unchanged and
// counts the reads. The embedded nil interface means any *other* OutcomeStore
// method panics — which is the signal we want if the calibrator ever starts
// calling more of the interface than it needs.
type fixedOutcomeStore struct {
	ledger.OutcomeStore
	outcomes []recommendation.RecommendationOutcome
	err      error
	calls    int
}

func (s *fixedOutcomeStore) LoadOutcomesFromSessions() ([]recommendation.RecommendationOutcome, error) {
	s.calls++
	return s.outcomes, s.err
}

func hasInsufficientDataError(res CalibrationResult) bool {
	for _, e := range res.Errors {
		if strings.Contains(e, "insufficient outcome data") {
			return true
		}
	}
	return false
}

// TestCalibrateDarwinianWithStore_PrefersInjectedStore pins ①: the data comes
// from the injected store, not from the mirror in the working directory.
func TestCalibrateDarwinianWithStore_PrefersInjectedStore(t *testing.T) {
	// The store sees a calibratable fixture.
	storeRoot := t.TempDir()
	storeDir := writeSessionOutcomes(t, storeRoot, 0.01, 5)
	store := jsonlOutcomeStore(t, storeDir)

	// The working directory holds a DIFFERENT, non-calibratable mirror (6
	// outcomes < the 10 needed): reading cwd instead of the store would report
	// "insufficient outcome data" and write the heuristic defaults.
	cwdRoot := t.TempDir()
	writeSessionOutcomes(t, cwdRoot, 0.01, 1)

	// Reference: the legacy nil-store path reading the store's fixture.
	t.Chdir(storeRoot)
	refEngine, refCfg := newDarwinianCfg() // defined in darwinian_risk_vol_band_test.go
	want := CalibrateDarwinian(refEngine, 100, refCfg)

	t.Chdir(cwdRoot)
	legacyEngine, legacyCfg := newDarwinianCfg()
	legacy := CalibrateDarwinian(legacyEngine, 100, legacyCfg)
	if !hasInsufficientDataError(legacy) {
		t.Fatalf("cwd fixture must be non-calibratable, otherwise this test proves nothing; errors=%v", legacy.Errors)
	}

	storeEngine, storeCfg := newDarwinianCfg()
	got := CalibrateDarwinianWithStore(storeEngine, 100, storeCfg, store)

	if hasInsufficientDataError(got) {
		t.Fatalf("injected store hits the cwd mirror instead: %v", got.Errors)
	}
	if reflect.DeepEqual(got.Parameters, legacy.Parameters) {
		t.Fatal("result equals the cwd-mirror result; the injected store was ignored")
	}
	if !reflect.DeepEqual(got.Parameters, want.Parameters) {
		t.Fatalf("parameters from the store = %+v, want the same as reading that fixture = %+v", got.Parameters, want.Parameters)
	}
}

// TestCalibrateDarwinianWithStore_ParityWithLegacyMirror pins ②: the same
// outcomes served through the injected store and through the legacy mirror
// produce identical parameters — the seam changes the source, not the maths.
func TestCalibrateDarwinianWithStore_ParityWithLegacyMirror(t *testing.T) {
	root := t.TempDir()
	dir := writeSessionOutcomes(t, root, 0.013, 5)
	store := jsonlOutcomeStore(t, dir)
	t.Chdir(root)

	storeEngine, storeCfg := newDarwinianCfg()
	withStore := CalibrateDarwinianWithStore(storeEngine, 100, storeCfg, store)

	legacyEngine, legacyCfg := newDarwinianCfg()
	legacy := CalibrateDarwinian(legacyEngine, 100, legacyCfg)

	if len(withStore.Parameters) == 0 {
		t.Fatal("fixture produced no parameters; the parity assertion would be vacuous")
	}
	if !reflect.DeepEqual(withStore.Parameters, legacy.Parameters) {
		t.Fatalf("store parameters = %+v, mirror parameters = %+v", withStore.Parameters, legacy.Parameters)
	}
	if !reflect.DeepEqual(withStore.Errors, legacy.Errors) {
		t.Fatalf("store errors = %v, mirror errors = %v", withStore.Errors, legacy.Errors)
	}
}

// TestCalibrateDarwinianWithStore_NilStoreReadsTheMirror pins ③: the legacy entry
// point (and an explicit nil store) still reads <cwd>/data/state, and its
// tolerance for a missing mirror is unchanged.
func TestCalibrateDarwinianWithStore_NilStoreReadsTheMirror(t *testing.T) {
	root := t.TempDir()
	writeSessionOutcomes(t, root, 0.01, 5)
	t.Chdir(root)

	engine, cfg := newDarwinianCfg()
	legacy := CalibrateDarwinian(engine, 100, cfg)
	if hasInsufficientDataError(legacy) {
		t.Fatalf("CalibrateDarwinian must keep reading the cwd mirror: %v", legacy.Errors)
	}

	engine, cfg = newDarwinianCfg()
	explicitNil := CalibrateDarwinianWithStore(engine, 100, cfg, nil)
	if !reflect.DeepEqual(legacy.Parameters, explicitNil.Parameters) {
		t.Fatalf("CalibrateDarwinian == CalibrateDarwinianWithStore(nil) is required: %+v vs %+v",
			legacy.Parameters, explicitNil.Parameters)
	}

	// No mirror at all: report the deficiency instead of panicking.
	t.Chdir(t.TempDir())
	engine, cfg = newDarwinianCfg()
	empty := CalibrateDarwinian(engine, 100, cfg)
	if !hasInsufficientDataError(empty) {
		t.Fatalf("a missing mirror must report insufficient data, got errors=%v", empty.Errors)
	}
	if len(empty.Parameters) == 0 {
		t.Fatal("the insufficient-data path must still return the heuristic defaults")
	}
}

// TestCalibrateDarwinianWithStore_StoreErrorFallsBackToMirror pins ③-2: only a
// store READ ERROR degrades to the mirror, and the fallback result is exactly the
// nil-store result.
func TestCalibrateDarwinianWithStore_StoreErrorFallsBackToMirror(t *testing.T) {
	root := t.TempDir()
	writeSessionOutcomes(t, root, 0.01, 5)
	t.Chdir(root)

	engine, cfg := newDarwinianCfg()
	want := CalibrateDarwinian(engine, 100, cfg)
	if len(want.Parameters) == 0 || hasInsufficientDataError(want) {
		t.Fatalf("fixture must be calibratable, otherwise this test proves nothing: errors=%v", want.Errors)
	}

	broken := &fixedOutcomeStore{err: errors.New("pg: connection refused")}
	engine, cfg = newDarwinianCfg()
	got := CalibrateDarwinianWithStore(engine, 100, cfg, broken)

	if broken.calls != 1 {
		t.Fatalf("store read attempts = %d, want exactly 1", broken.calls)
	}
	if !reflect.DeepEqual(got.Parameters, want.Parameters) {
		t.Fatalf("fallback parameters = %+v, want the mirror parameters %+v", got.Parameters, want.Parameters)
	}
}

// TestCalibrateDarwinianWithStore_EmptyStoreIsTakenAtFaceValue pins the other
// half of the degradation rule: a store that SUCCEEDS with zero outcomes must not
// be second-guessed by reading the mirror (that would resurrect the exact
// stale-mirror defect this seam removes).
func TestCalibrateDarwinianWithStore_EmptyStoreIsTakenAtFaceValue(t *testing.T) {
	root := t.TempDir()
	writeSessionOutcomes(t, root, 0.01, 5) // calibratable mirror that must NOT be read
	t.Chdir(root)

	empty := &fixedOutcomeStore{}
	engine, cfg := newDarwinianCfg()
	res := CalibrateDarwinianWithStore(engine, 100, cfg, empty)

	if empty.calls != 1 {
		t.Fatalf("store read attempts = %d, want exactly 1", empty.calls)
	}
	if !hasInsufficientDataError(res) {
		t.Fatalf("an empty store result must be reported as insufficient data, got errors=%v", res.Errors)
	}
}
