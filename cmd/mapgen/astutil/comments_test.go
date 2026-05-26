package astutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"testing"
)

func TestCountPatterns(t *testing.T) {
	src := `
package test

// TODO: implement this
func foo() {}

// FIXME: this is broken
// HACK: workaround for now
func bar() {}

// Not a stub
func baz() {}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	patterns := map[string]*regexp.Regexp{
		"TODO":  ReTODO,
		"FIXME": ReFIXME,
		"HACK":  ReHACK,
	}
	counts := CountPatterns(fset, f, patterns)

	if counts["TODO"] != 1 {
		t.Errorf("expected 1 TODO, got %d", counts["TODO"])
	}
	if counts["FIXME"] != 1 {
		t.Errorf("expected 1 FIXME, got %d", counts["FIXME"])
	}
	if counts["HACK"] != 1 {
		t.Errorf("expected 1 HACK, got %d", counts["HACK"])
	}
}

func TestCountPatterns_noMatch(t *testing.T) {
	src := `
package test

// This is a normal comment
func foo() {}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	patterns := map[string]*regexp.Regexp{
		"TODO":  ReTODO,
		"FIXME": ReFIXME,
	}
	counts := CountPatterns(fset, f, patterns)

	if counts["TODO"] != 0 {
		t.Errorf("expected 0 TODO, got %d", counts["TODO"])
	}
	if counts["FIXME"] != 0 {
		t.Errorf("expected 0 FIXME, got %d", counts["FIXME"])
	}
}

func TestCountPatterns_multiline(t *testing.T) {
	src := `
package test

/*
TODO: big refactor needed
*/
func foo() {}

// TODO: also fix this
func bar() {}
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	counts := CountPatterns(fset, f, map[string]*regexp.Regexp{
		"TODO": ReTODO,
	})

	if counts["TODO"] != 2 {
		t.Errorf("expected 2 TODOs, got %d", counts["TODO"])
	}
}

func TestCountPatterns_emptyFile(t *testing.T) {
	src := `package test
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	counts := CountPatterns(fset, f, map[string]*regexp.Regexp{
		"TODO": ReTODO,
	})
	// Should not panic, should return zero
	if counts == nil {
		t.Fatal("expected non-nil counts map")
	}
}

func BenchmarkCountPatterns(b *testing.B) {
	src := `
package test

// TODO: implement this
func foo() {}

// FIXME: broken
func bar() {}

// HACK: workaround
func baz() {}
`
	fset := token.NewFileSet()
	f, _ := parser.ParseFile(fset, "", src, parser.ParseComments)
	patterns := map[string]*regexp.Regexp{
		"TODO":  ReTODO,
		"FIXME": ReFIXME,
		"HACK":  ReHACK,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = CountPatterns(fset, f, patterns)
	}
}

// Test to ensure exports match expected values
func TestReExports(t *testing.T) {
	if ReTODO == nil {
		t.Error("ReTODO should not be nil")
	}
	if !ReTODO.MatchString("TODO") {
		t.Error("ReTODO should match 'TODO'")
	}
	if ReFIXME == nil {
		t.Error("ReFIXME should not be nil")
	}
	if !ReFIXME.MatchString("FIXME") {
		t.Error("ReFIXME should match 'FIXME'")
	}
	if !ReStub.MatchString("not implemented") {
		t.Error("ReStub should match 'not implemented'")
	}
}

// Helper test to ensure blank identifier works (variable used to prevent unused errors)
func TestBlankNaming(t *testing.T) {
	_ = ast.File{}
}
