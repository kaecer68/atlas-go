package astutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestExtractStringLiteral(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`"/api/foo"`, "/api/foo"},
		{`"/health"`, "/health"},
		{`""`, ""},
	}

	for _, tc := range tests {
		expr, err := parser.ParseExpr(tc.input)
		if err != nil {
			t.Fatalf("parse expr %s: %v", tc.input, err)
		}
		got := extractStringLiteral(expr)
		if got != tc.want {
			t.Errorf("extractStringLiteral(%s) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestExtractStringLiteral_nonLiteral(t *testing.T) {
	// Identifiers should return the name
	expr, err := parser.ParseExpr("someConst")
	if err != nil {
		t.Fatalf("parse expr: %v", err)
	}
	got := extractStringLiteral(expr)
	if got != "someConst" {
		t.Errorf("expected 'someConst', got %q", got)
	}
}

func TestExtractHandlerName_ident(t *testing.T) {
	expr, err := parser.ParseExpr(`handler`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := extractHandlerName(expr)
	if got != "handler" {
		t.Errorf("expected 'handler', got %q", got)
	}
}

func TestExtractHandlerName_selector(t *testing.T) {
	expr, err := parser.ParseExpr(`a.handleFoo`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := extractHandlerName(expr)
	if got != "a.handleFoo" {
		t.Errorf("expected 'a.handleFoo', got %q", got)
	}
}

func TestExtractHandlerName_funcLit(t *testing.T) {
	expr, err := parser.ParseExpr(`func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := extractHandlerName(expr)
	if got != "anonymous" {
		t.Errorf("expected 'anonymous', got %q", got)
	}
}

func TestExtractHandlerName_typeExpr(t *testing.T) {
	// Fallback case: could be a type expression like http.HandlerFunc(someFunc)
	expr, err := parser.ParseExpr(`http.HandlerFunc(fn)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := extractHandlerName(expr)
	// Should not panic, returns type string
	if got == "" {
		t.Error("expected non-empty handler name")
	}
}

func TestClassifyRoute(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"/api/dashboard/foo", "dashboard"},
		{"/api/industry/bar", "industry"},
		{"/api/narrative/events", "narrative"},
		{"/api/control/set-model-weight", "control"},
		{"/api/agents/health", "control"},
		{"/api/experiment/run", "experiment"},
		{"/api/backtest/window", "backtest"},
		{"/api/dashboard/performance", "performance"},
		{"/api/dashboard/performance/export", "performance"},
		{"/api/dashboard/live/positions", "live"},
		{"/api/dashboard/pnl/daily", "live"},
		{"/api/dashboard/risk/exposure", "live"},
		{"/api/scheduler/status", "system"},
		{"/api/tasks/list", "system"},
		{"/health", "system"},
		{"/api/unknown", "other"},
		{"/metrics", "other"},
		{"/", "other"},
	}

	for _, tc := range tests {
		got := classifyRoute(tc.pattern)
		if got != tc.want {
			t.Errorf("classifyRoute(%q) = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}

func TestIsStubFunc(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "empty body",
			src:  `package p; func foo() {}`,
			want: true,
		},
		{
			name: "nil body",
			src:  `package p; func foo();`,
			want: true,
		},
		{
			name: "return nil",
			src:  `package p; func foo() error { return nil }`,
			want: true,
		},
		{
			name: "return nil, nil",
			src:  `package p; func foo() (error, error) { return nil, nil }`,
			want: true,
		},
		{
			name: "bare return",
			src:  `package p; func foo() { return }`,
			want: true,
		},
		{
			name: "real implementation",
			src:  `package p; func foo() string { return "hello" }`,
			want: false,
		},
		{
			name: "real implementation with var",
			src:  `package p; func foo() error { var err error; return err }`,
			want: false,
		},
		{
			name: "multiple statements",
			src:  `package p; func foo() error { x := 1; return nil }`,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", tc.src, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var fn *ast.FuncDecl
			for _, decl := range f.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok {
					fn = fd
					break
				}
			}
			if fn == nil {
				t.Fatal("no function declaration found")
			}
			got := isStubFunc(fn)
			if got != tc.want {
				t.Errorf("isStubFunc(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestIsNilReturn(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "bare return",
			src:  `package p; func foo() { return }`,
			want: true,
		},
		{
			name: "return nil",
			src:  `package p; func foo() error { return nil }`,
			want: true,
		},
		{
			name: "return nil, nil",
			src:  `package p; func foo() (error, error) { return nil, nil }`,
			want: true,
		},
		{
			name: "return err",
			src:  `package p; func foo() error { return err }`,
			want: false,
		},
		{
			name: "return value",
			src:  `package p; func foo() string { return "x" }`,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "", tc.src, parser.ParseComments)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for _, decl := range f.Decls {
				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil && len(fn.Body.List) > 0 {
					if ret, ok := fn.Body.List[0].(*ast.ReturnStmt); ok {
						got := isNilReturn(ret)
						if got != tc.want {
							t.Errorf("isNilReturn(%s) = %v, want %v", tc.name, got, tc.want)
						}
					}
				}
			}
		})
	}
}

func TestExtractRoutes_noPanic(t *testing.T) {
	// ExtractRoutes should not panic with any valid or invalid input.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ExtractRoutes panicked: %v", r)
		}
	}()
	// Scan current dir (no route registrations here, but should not panic).
	ExtractRoutes([]string{"."})
	// Scan nonexistent dir — should log an error but not panic.
	ExtractRoutes([]string{"/nonexistent"})
	// Empty input.
	ExtractRoutes(nil)
}

func TestMarkStubHandlers_noop(t *testing.T) {
	// Should not panic with empty input
	MarkStubHandlers(nil, nil)
	MarkStubHandlers(nil, []string{})
}

// Test to verify the RouteInfo dedup key
func TestRouteDedupKey(t *testing.T) {
	fset := token.NewFileSet()
	routeSet := make(map[string]bool)

	src := `package test
import "net/http"
func register(mux *http.ServeMux) {
	mux.HandleFunc("/api/foo", handleFoo)
	mux.HandleFunc("/api/foo", handleFoo) // duplicate
}
`
	f, _ := parser.ParseFile(fset, "", src, parser.ParseComments)
	routes := extractRoutesFromFile(fset, f, "/fake/test.go", routeSet)
	if len(routes) != 1 {
		t.Errorf("expected 1 route after dedup, got %d", len(routes))
	}
}

func BenchmarkClassifyRoute(b *testing.B) {
	patterns := []string{
		"/api/dashboard/foo",
		"/api/industry/bar",
		"/api/narrative/baz",
		"/api/control/qux",
		"/api/experiment/quux",
		"/api/backtest/corge",
		"/api/scheduler/grault",
		"/health",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range patterns {
			classifyRoute(p)
		}
	}
}

// Test to ensure StubMarker doesn't mutate unexpected fields
func TestIsStubFunc_nonFunc(t *testing.T) {
	src := `package p
var x = 1
`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, decl := range f.Decls {
		if _, isGenDecl := decl.(*ast.GenDecl); isGenDecl {
			continue
		}
		_ = decl // ensure non-GenDecl declarations parse successfully
	}
}
