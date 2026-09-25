package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/sectorallocation"
)

// TestClosureStorePathMatchesFileClosureStoreWriteTarget is the regression test
// for issue #1944 N-A4. The preflight used to validate
// <work_dir>/data/state/sector_closure_policy.jsonl, a path no writer ever
// creates, while the production store lives under
// <work_dir>/data/sector/allocation. This test compares the path the checklist
// validates with the path FileClosureStore actually writes on disk.
func TestClosureStorePathMatchesFileClosureStoreWriteTarget(t *testing.T) {
	workDir := t.TempDir()

	if res := checkClosureStore(workDir); !res.OK {
		t.Fatalf("checkClosureStore(%q) failed: %s", workDir, res.Message)
	}

	// The production construction path: ResolveClosureStoreDir + the store.
	store := sectorallocation.NewFileClosureStore(sectorallocation.ResolveClosureStoreDir(workDir))
	if _, err := store.Store(sectorallocation.SectorAllocationSnapshot{
		AsOfTradingDate:   "2026-09-25",
		EffectiveFrom:     "2026-09-26",
		Target:            map[industry.SectorID]float64{"semiconductor": 0.30},
		ModelVersion:      "1.0.0",
		CalibrationStatus: "calibrating",
		WeightSource:      "heuristic",
	}); err != nil {
		t.Fatalf("FileClosureStore.Store: %v", err)
	}

	// The preflight writes (and cleans up) an empty probe file, so after the
	// real Store the work dir must hold exactly the one store file.
	var written []string
	if err := filepath.WalkDir(workDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		written = append(written, path)
		return nil
	}); err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if len(written) != 1 {
		t.Fatalf("expected exactly the closure policy file on disk, got %v", written)
	}

	checked := closureStorePath(workDir)
	if written[0] != checked {
		t.Fatalf("preflight validates %s but FileClosureStore wrote %s", checked, written[0])
	}
	want := filepath.Join(workDir, "data", "sector", "allocation", "sector_closure_policy.jsonl")
	if checked != want {
		t.Fatalf("closureStorePath = %s, want %s", checked, want)
	}

	// The pre-fix location must NOT be what the checklist validates.
	stale := filepath.Join(workDir, "data", "state", "sector_closure_policy.jsonl")
	if checked == stale {
		t.Fatalf("preflight still validates the stale data/state path %s", stale)
	}
}

// TestClosureStorePathIsSharedWithStoreConstruction: the exported helper is the
// single source of truth for the write target (1 line per caller).
func TestClosureStorePathIsSharedWithStoreConstruction(t *testing.T) {
	workDir := "/tmp/atlas-workdir-probe"
	if got, want := sectorallocation.ClosureStoreDirRel(), filepath.Join("data", "sector", "allocation"); got != want {
		t.Errorf("ClosureStoreDirRel() = %q, want %q", got, want)
	}
	if got, want := sectorallocation.ResolveClosureStoreDir(workDir), filepath.Join(workDir, "data", "sector", "allocation"); got != want {
		t.Errorf("ResolveClosureStoreDir() = %q, want %q", got, want)
	}
	if got, want := closureStorePath(workDir), filepath.Join(workDir, "data", "sector", "allocation", "sector_closure_policy.jsonl"); got != want {
		t.Errorf("closureStorePath() = %q, want %q", got, want)
	}
}

// TestClosureStoreConstructionUsesSharedResolver guards against the N-A4 class
// of drift resurfacing: every NON-TEST construction of a FileClosureStore must
// resolve its directory through ResolveClosureStoreDir, so the path the
// preflight validates stays the path the writer uses. Tests are excluded (they
// use t.TempDir()).
func TestClosureStoreConstructionUsesSharedResolver(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("abs root: %v", err)
	}

	fset := token.NewFileSet()
	var offenders []string
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "dist":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil || !bytes.Contains(src, []byte("NewFileClosureStore")) {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, src, 0)
		if parseErr != nil {
			offenders = append(offenders, fmt.Sprintf("%s: parse: %v", path, parseErr))
			return nil
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || calleeName(call.Fun) != "NewFileClosureStore" {
				return true
			}
			arg, ok := call.Args[0].(*ast.CallExpr)
			if len(call.Args) != 1 || !ok || calleeName(arg.Fun) != "ResolveClosureStoreDir" {
				offenders = append(offenders, fmt.Sprintf("%s:%d", path, fset.Position(call.Pos()).Line))
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk: %v", walkErr)
	}
	if len(offenders) > 0 {
		t.Fatalf("FileClosureStore constructed outside ResolveClosureStoreDir at %v; "+
			"the closure preflight resolves the store path through that helper, so a hardcoded "+
			"directory here re-creates issue #1944 N-A4", offenders)
	}
}

func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	default:
		return ""
	}
}
