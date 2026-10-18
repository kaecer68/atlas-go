package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// positionsDeterminismFixture returns a fixed, non-trivial position set.
//
// Row-order contract (FU-20261016-35): the fixture deliberately uses 32 distinct
// symbols. Two independent Go map iterations agreeing on the same order has
// probability 1/32! (= 0 for any practical purpose), so a byte-identity
// assertion built on this fixture has discriminating power. A 2-3 element
// fixture would flake green roughly half of the runs and therefore would NOT
// be a probe.
func positionsDeterminismFixture() []domain.Position {
	symbols := []string{
		"0050.TW", "1101.TW", "1216.TW", "1301.TW", "1303.TW", "1402.TW",
		"1605.TW", "2002.TW", "2105.TW", "2201.TW", "2207.TW", "2301.TW",
		"2303.TW", "2308.TW", "2317.TW", "2330.TW", "2345.TW", "2357.TW",
		"2379.TW", "2382.TW", "2408.TW", "2412.TW", "2454.TW", "2603.TW",
		"2609.TW", "2615.TW", "2801.TW", "2881.TW", "2882.TW", "2891.TW",
		"2912.TW", "3008.TW",
	}
	out := make([]domain.Position, 0, len(symbols))
	for i, sym := range symbols {
		qty := 1000 + i*10
		avg := 50 + float64(i)*3.5
		price := 55 + float64(i)*3.25
		out = append(out, domain.Position{
			Symbol:        sym,
			Quantity:      qty,
			AverageCost:   avg,
			CurrentPrice:  price,
			MarketValue:   float64(qty) * price,
			UnrealizedPnL: float64(qty) * (price - avg),
			EntryDate:     time.Date(2026, 9, 1+i%20, 0, 0, 0, 0, time.UTC),
		})
	}
	return out
}

func newStoreWithFixturePositions(t *testing.T, dir string, positions []domain.Position) *StateStore {
	t.Helper()
	st := NewStateStore(dir)
	for _, p := range positions {
		st.UpdatePosition(p)
	}
	return st
}

func sha256OfFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func readSavedPositions(t *testing.T, dir string) []domain.Position {
	t.Helper()
	path := filepath.Join(dir, LiveStateSubDir, PositionsStateFile)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved positions %s: %v", path, err)
	}
	var got []domain.Position
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal saved positions %s: %v", path, err)
	}
	return got
}

// TestStateStoreSavePositionsDeterminism_ByteIdenticalAcrossRuns pins the
// serialization contract of StateStore.Save: the SAME input must produce the
// SAME bytes for state/positions_current.json.
//
// Before FU-20261016-35 the positions slice was built straight from the
// `map[string]domain.Position` field, so the file order (and therefore the
// file bytes) changed from run to run.
func TestStateStoreSavePositionsDeterminism_ByteIdenticalAcrossRuns(t *testing.T) {
	fixture := positionsDeterminismFixture()

	dirA, dirB := t.TempDir(), t.TempDir()
	stA := newStoreWithFixturePositions(t, dirA, fixture)
	stB := newStoreWithFixturePositions(t, dirB, fixture)

	if err := stA.Save(); err != nil {
		t.Fatalf("Save (A): %v", err)
	}
	if err := stB.Save(); err != nil {
		t.Fatalf("Save (B): %v", err)
	}

	pathA := filepath.Join(dirA, LiveStateSubDir, PositionsStateFile)
	pathB := filepath.Join(dirB, LiveStateSubDir, PositionsStateFile)
	sumA, sumB := sha256OfFile(t, pathA), sha256OfFile(t, pathB)
	if sumA != sumB {
		t.Errorf("positions file is not byte-identical across two runs of the same input (sha256 %s vs %s)", sumA, sumB)
	}
}

// TestStateStoreSavePositionsDeterminism_OrderIsSymbolAscending pins the
// canonical order itself, so the contract is stated and not merely implied by
// "two runs happen to agree".
func TestStateStoreSavePositionsDeterminism_OrderIsSymbolAscending(t *testing.T) {
	fixture := positionsDeterminismFixture()
	dir := t.TempDir()
	st := newStoreWithFixturePositions(t, dir, fixture)
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := readSavedPositions(t, dir)
	if len(got) != len(fixture) {
		t.Fatalf("saved position count = %d, want %d", len(got), len(fixture))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Symbol >= got[i].Symbol {
			t.Fatalf("saved positions are not in ascending symbol order: index %d = %q, index %d = %q",
				i-1, got[i-1].Symbol, i, got[i].Symbol)
		}
	}
}

// TestStateStoreSavePositionsDeterminism_ContentSetUnchanged guards the other
// half of the contract: sorting must not drop, add or alter any value. Each
// position is compared as its own JSON document, so the assertion is
// order-insensitive but value-exact.
func TestStateStoreSavePositionsDeterminism_ContentSetUnchanged(t *testing.T) {
	fixture := positionsDeterminismFixture()
	dir := t.TempDir()
	st := newStoreWithFixturePositions(t, dir, fixture)
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got := readSavedPositions(t, dir)
	bySymbol := make(map[string]domain.Position, len(got))
	for _, p := range got {
		if _, dup := bySymbol[p.Symbol]; dup {
			t.Fatalf("saved positions contain duplicate symbol %q", p.Symbol)
		}
		bySymbol[p.Symbol] = p
	}
	if len(bySymbol) != len(fixture) {
		t.Fatalf("saved symbol set size = %d, want %d", len(bySymbol), len(fixture))
	}

	for _, want := range fixture {
		gotPos, ok := bySymbol[want.Symbol]
		if !ok {
			t.Fatalf("symbol %q is missing from the saved positions file", want.Symbol)
		}
		wantJSON, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("marshal want %q: %v", want.Symbol, err)
		}
		gotJSON, err := json.Marshal(gotPos)
		if err != nil {
			t.Fatalf("marshal got %q: %v", want.Symbol, err)
		}
		if !bytes.Equal(wantJSON, gotJSON) {
			t.Errorf("position %q changed value: want %s, got %s", want.Symbol, wantJSON, gotJSON)
		}
	}
}

// TestStateStoreGetStateDeterminism_PositionsOrderIsSymbolAscending covers the
// in-memory sibling of Save (same root cause, same file): State.GetState()
// feeds internal/monitoring/rules.go, whose position rules return the FIRST
// matching position, so an unordered slice made the alert message symbol
// non-deterministic too.
func TestStateStoreGetStateDeterminism_PositionsOrderIsSymbolAscending(t *testing.T) {
	fixture := positionsDeterminismFixture()
	dir := t.TempDir()
	st := newStoreWithFixturePositions(t, dir, fixture)

	state := st.GetState()
	if state == nil {
		t.Fatal("GetState returned nil")
	}
	if len(state.Positions) != len(fixture) {
		t.Fatalf("snapshot position count = %d, want %d", len(state.Positions), len(fixture))
	}
	for i := 1; i < len(state.Positions); i++ {
		if state.Positions[i-1].Symbol >= state.Positions[i].Symbol {
			t.Fatalf("snapshot positions are not in ascending symbol order: index %d = %q, index %d = %q",
				i-1, state.Positions[i-1].Symbol, i, state.Positions[i].Symbol)
		}
	}
}
