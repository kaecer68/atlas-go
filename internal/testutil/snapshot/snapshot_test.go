package snapshot_test

import (
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/testutil/snapshot"
)

// TestCaptureAPI_SelfParse exercises CaptureAPI on snapshot.go itself.
// This is a round-trip sanity check: the parser must successfully extract
// its own public API without crashing, and the result must be deterministic.
func TestCaptureAPI_SelfParse(t *testing.T) {
	const selfPath = "snapshot.go"

	snap, err := snapshot.CaptureAPI(selfPath)
	if err != nil {
		t.Fatalf("CaptureAPI failed: %v", err)
	}

	if snap.Package != "snapshot" {
		t.Errorf("Package = %q, want %q", snap.Package, "snapshot")
	}

	// Must contain the public entry points we exported.
	wantFuncs := map[string]bool{
		"CaptureAPI":       false,
		"AssertAPI":        false,
		"AssertGoldenJSON": false,
	}
	for _, f := range snap.Funcs {
		if _, ok := wantFuncs[f.Name]; ok {
			wantFuncs[f.Name] = true
		}
	}
	for name, found := range wantFuncs {
		if !found {
			t.Errorf("missing public function %q in captured API", name)
		}
	}

	// Must contain the public types.
	wantTypes := map[string]bool{
		"APISnapshot": false,
		"FuncSig":     false,
		"TypeDecl":    false,
		"Const":       false,
		"Var":         false,
	}
	for _, td := range snap.Types {
		if _, ok := wantTypes[td.Name]; ok {
			wantTypes[td.Name] = true
		}
	}
	for name, found := range wantTypes {
		if !found {
			t.Errorf("missing public type %q in captured API", name)
		}
	}
}

// TestCaptureAPI_Determinism verifies that two consecutive parses of the
// same file produce byte-identical snapshots. Required for golden-file
// comparison to be meaningful.
func TestCaptureAPI_Determinism(t *testing.T) {
	const selfPath = "snapshot.go"

	first, err := snapshot.CaptureAPI(selfPath)
	if err != nil {
		t.Fatalf("first parse: %v", err)
	}
	second, err := snapshot.CaptureAPI(selfPath)
	if err != nil {
		t.Fatalf("second parse: %v", err)
	}

	if strings.Compare(formatForCompare(first), formatForCompare(second)) != 0 {
		t.Errorf("non-deterministic parse output — golden-file comparison would be unreliable")
	}
}

// TestCaptureAPI_Nonexistent ensures we get a clean error on bad input.
func TestCaptureAPI_Nonexistent(t *testing.T) {
	_, err := snapshot.CaptureAPI("/does/not/exist/file.go")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error %q should mention 'parse'", err.Error())
	}
}

func formatForCompare(s snapshot.APISnapshot) string {
	var b strings.Builder
	b.WriteString("package=" + s.Package + "\n")
	for _, f := range s.Funcs {
		b.WriteString("func " + f.Name + f.Params + " " + f.Results + "\n")
	}
	for _, td := range s.Types {
		b.WriteString("type " + td.Name + " kind=" + td.Kind + " fields=" + strings.Join(td.Fields, ",") + "\n")
	}
	for _, c := range s.Consts {
		b.WriteString("const " + c.Name + " " + c.Type + " = " + c.Value + "\n")
	}
	for _, v := range s.Vars {
		b.WriteString("var " + v.Name + " " + v.Type + "\n")
	}
	return b.String()
}
