// Package snapshot provides two complementary snapshot strategies for the
// #611 refactor safety net (Layer 3):
//
//  1. APISnapshot — uses go/parser + go/ast to extract exported symbols
//     (funcs, types, consts, vars) from a Go source file. Detects accidental
//     additions, removals, or signature changes during refactor.
//
//  2. GoldenSnapshot — serializes a runtime value to deterministic JSON and
//     compares against a snapshot file. Locks behavior of pure functions.
//
// Both strategies fail the calling test on mismatch. The first mismatch run
// writes the actual output to testdata/<file>.actual for review. To accept a
// change, copy testdata/<file>.actual over testdata/<file>.golden (and update
// the test's intent comment).
package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// APISnapshot captures the exported surface of a Go source file.
type APISnapshot struct {
	Package string     `json:"package"`
	Funcs   []FuncSig  `json:"funcs"`
	Types   []TypeDecl `json:"types"`
	Consts  []Const    `json:"consts"`
	Vars    []Var      `json:"vars"`
}

// FuncSig is one exported function or method signature.
type FuncSig struct {
	Name     string `json:"name"`
	Receiver string `json:"receiver,omitempty"`
	Params   string `json:"params"`
	Results  string `json:"results"`
}

// TypeDecl describes one exported type declaration.
type TypeDecl struct {
	Name   string   `json:"name"`
	Kind   string   `json:"kind"` // "struct" | "interface" | "alias"
	Fields []string `json:"fields,omitempty"`
}

// Const is one exported constant (or constant block entry).
type Const struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	Value string `json:"value"`
}

// Var is one exported package-level variable.
type Var struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CaptureAPI parses the Go source file at path and returns its exported API
// snapshot. Package name is read from the file's package clause.
func CaptureAPI(path string) (APISnapshot, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return APISnapshot{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return captureFromAST(f)
}

// CaptureAPIs parses multiple Go source files in the same package and returns
// the merged exported API snapshot. All files must share the same package name.
func CaptureAPIs(paths ...string) (APISnapshot, error) {
	merged := APISnapshot{}
	for _, path := range paths {
		snap, err := CaptureAPI(path)
		if err != nil {
			return APISnapshot{}, err
		}
		if merged.Package == "" {
			merged.Package = snap.Package
		} else if merged.Package != snap.Package {
			return APISnapshot{}, fmt.Errorf("package mismatch: %s vs %s", merged.Package, snap.Package)
		}
		merged.Funcs = append(merged.Funcs, snap.Funcs...)
		merged.Types = append(merged.Types, snap.Types...)
		merged.Consts = append(merged.Consts, snap.Consts...)
		merged.Vars = append(merged.Vars, snap.Vars...)
	}
	sort.Slice(merged.Funcs, func(i, j int) bool { return funcKey(merged.Funcs[i]) < funcKey(merged.Funcs[j]) })
	sort.Slice(merged.Types, func(i, j int) bool { return merged.Types[i].Name < merged.Types[j].Name })
	sort.Slice(merged.Consts, func(i, j int) bool { return merged.Consts[i].Name < merged.Consts[j].Name })
	sort.Slice(merged.Vars, func(i, j int) bool { return merged.Vars[i].Name < merged.Vars[j].Name })
	return merged, nil
}

func captureFromAST(f *ast.File) (APISnapshot, error) {
	snap := APISnapshot{Package: f.Name.Name}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if !d.Name.IsExported() {
				continue
			}
			sig := FuncSig{
				Name:     d.Name.Name,
				Params:   fieldListText(d.Type.Params),
				Results:  fieldListText(d.Type.Results),
				Receiver: receiverText(d.Recv),
			}
			snap.Funcs = append(snap.Funcs, sig)
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if !s.Name.IsExported() {
						continue
					}
					td := TypeDecl{
						Name: s.Name.Name,
						Kind: typeKind(s.Type),
					}
					if st, ok := s.Type.(*ast.StructType); ok {
						td.Fields = structFields(st)
					}
					snap.Types = append(snap.Types, td)
				case *ast.ValueSpec:
					for i, name := range s.Names {
						if !name.IsExported() {
							continue
						}
						entry := Const{
							Name: name.Name,
						}
						if s.Type != nil {
							entry.Type = exprText(s.Type)
						}
						if i < len(s.Values) {
							entry.Value = exprText(s.Values[i])
						}
						if d.Tok == token.VAR {
							snap.Vars = append(snap.Vars, Var{
								Name: entry.Name,
								Type: entry.Type,
							})
						} else {
							snap.Consts = append(snap.Consts, entry)
						}
					}
				}
			}
		}
	}

	// Deterministic ordering for stable snapshots.
	sort.Slice(snap.Funcs, func(i, j int) bool { return funcKey(snap.Funcs[i]) < funcKey(snap.Funcs[j]) })
	sort.Slice(snap.Types, func(i, j int) bool { return snap.Types[i].Name < snap.Types[j].Name })
	sort.Slice(snap.Consts, func(i, j int) bool { return snap.Consts[i].Name < snap.Consts[j].Name })
	sort.Slice(snap.Vars, func(i, j int) bool { return snap.Vars[i].Name < snap.Vars[j].Name })

	return snap, nil
}

// AssertAPI compares the captured API to the golden file. On mismatch it
// writes the actual snapshot to testdata/<base>.actual and fails the test.
func AssertAPI(t *testing.T, got APISnapshot, goldenPath string) {
	t.Helper()
	want := APISnapshot{}
	if data, err := os.ReadFile(goldenPath); err == nil {
		if err := json.Unmarshal(data, &want); err != nil {
			t.Fatalf("golden file %s is not valid JSON: %v", goldenPath, err)
		}
	} else if !os.IsNotExist(err) {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	} else {
		// First run — write the golden file and pass.
		if err := writeJSONAtomic(goldenPath, got); err != nil {
			t.Fatalf("write initial golden %s: %v", goldenPath, err)
		}
		t.Logf("created initial golden file: %s", goldenPath)
		return
	}

	if !reflect.DeepEqual(got, want) {
		actualPath := strings.TrimSuffix(goldenPath, ".golden.json") + ".actual.json"
		if err := writeJSONAtomic(actualPath, got); err != nil {
			t.Fatalf("write actual %s: %v", actualPath, err)
		}
		t.Fatalf("API snapshot mismatch.\n  golden: %s\n  actual: %s\n  To accept, diff and copy actual → golden.", goldenPath, actualPath)
	}
}

// AssertGoldenJSON serializes got and compares to goldenPath. On mismatch it
// writes the actual file and fails. First run creates the golden.
func AssertGoldenJSON(t *testing.T, got any, goldenPath string) {
	t.Helper()
	actual, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Deterministic JSON: sort map keys via custom marshal.
	actual = canonicalizeJSON(actual)

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		if !os.IsNotExist(err) {
			t.Fatalf("read golden %s: %v", goldenPath, err)
		}
		if err := os.WriteFile(goldenPath, append(actual, '\n'), 0o644); err != nil {
			t.Fatalf("write initial golden %s: %v", goldenPath, err)
		}
		t.Logf("created initial golden file: %s", goldenPath)
		return
	}

	if !bytes.Equal(append(actual, '\n'), want) {
		actualPath := strings.TrimSuffix(goldenPath, ".golden.json") + ".actual.json"
		if err := os.WriteFile(actualPath, append(actual, '\n'), 0o644); err != nil {
			t.Fatalf("write actual %s: %v", actualPath, err)
		}
		t.Fatalf("golden snapshot mismatch.\n  golden: %s\n  actual: %s", goldenPath, actualPath)
	}
}

// --- internal helpers ---

func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = canonicalizeJSON(data)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func canonicalizeJSON(b []byte) []byte {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return b // fallback — leave as-is
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return out
}

func funcKey(f FuncSig) string {
	if f.Receiver != "" {
		return f.Receiver + "." + f.Name
	}
	return f.Name
}

func receiverText(r *ast.FieldList) string {
	if r == nil || len(r.List) == 0 {
		return ""
	}
	var parts []string
	for _, f := range r.List {
		typ := exprText(f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, strings.TrimPrefix(typ, "*"))
			continue
		}
		for range f.Names {
			parts = append(parts, strings.TrimPrefix(typ, "*"))
		}
	}
	return strings.Join(parts, ",")
}

func fieldListText(fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	var parts []string
	for _, f := range fl.List {
		typ := exprText(f.Type)
		if len(f.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		names := make([]string, len(f.Names))
		for i, n := range f.Names {
			names[i] = n.Name
		}
		parts = append(parts, strings.Join(names, ",")+" "+typ)
	}
	return "(" + strings.Join(parts, ", ") + ")"
}

func structFields(st *ast.StructType) []string {
	if st.Fields == nil {
		return nil
	}
	var fields []string
	for _, f := range st.Fields.List {
		typ := exprText(f.Type)
		var names []string
		for _, n := range f.Names {
			names = append(names, n.Name)
		}
		if len(names) == 0 {
			fields = append(fields, typ)
		} else {
			fields = append(fields, strings.Join(names, ", ")+" "+typ)
		}
	}
	return fields
}

func typeKind(e ast.Expr) string {
	switch e.(type) {
	case *ast.StructType:
		return "struct"
	case *ast.InterfaceType:
		return "interface"
	default:
		return "alias"
	}
}

func exprText(e ast.Expr) string {
	if e == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := formatExpr(&buf, e); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

// formatExpr renders a minimal but stable text for an AST expression.
// Uses simple token-by-token formatting to avoid depending on go/format
// (which can normalize spacing and complicate diffs).
func formatExpr(buf *bytes.Buffer, e ast.Expr) error {
	switch v := e.(type) {
	case *ast.Ident:
		buf.WriteString(v.Name)
	case *ast.StarExpr:
		buf.WriteByte('*')
		return formatExpr(buf, v.X)
	case *ast.SelectorExpr:
		if err := formatExpr(buf, v.X); err != nil {
			return err
		}
		buf.WriteByte('.')
		buf.WriteString(v.Sel.Name)
	case *ast.ArrayType:
		buf.WriteByte('[')
		if v.Len != nil {
			if err := formatExpr(buf, v.Len); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
		return formatExpr(buf, v.Elt)
	case *ast.MapType:
		buf.WriteString("map[")
		if err := formatExpr(buf, v.Key); err != nil {
			return err
		}
		buf.WriteByte(']')
		return formatExpr(buf, v.Value)
	case *ast.ChanType:
		buf.WriteString("chan ")
		if v.Dir == ast.RECV {
			buf.WriteString("<- ")
		}
		return formatExpr(buf, v.Value)
	case *ast.FuncType:
		buf.WriteString("func")
		if err := writeFieldList(buf, v.Params); err != nil {
			return err
		}
		if v.Results != nil && len(v.Results.List) > 0 {
			if len(v.Results.List) == 1 && len(v.Results.List[0].Names) == 0 {
				buf.WriteByte(' ')
				if err := formatExpr(buf, v.Results.List[0].Type); err != nil {
					return err
				}
			} else {
				if err := writeFieldList(buf, v.Results); err != nil {
					return err
				}
			}
		}
	case *ast.BasicLit:
		buf.WriteString(v.Value)
	case *ast.Ellipsis:
		buf.WriteString("...")
		return formatExpr(buf, v.Elt)
	case *ast.InterfaceType:
		buf.WriteString("interface{")
		if v.Methods != nil {
			for _, f := range v.Methods.List {
				if len(f.Names) > 0 {
					for _, n := range f.Names {
						buf.WriteString(n.Name)
					}
				} else {
					if err := formatExpr(buf, f.Type); err != nil {
						return err
					}
				}
				buf.WriteString("; ")
			}
		}
		buf.WriteString("}")
	case *ast.StructType:
		buf.WriteString("struct{")
		if v.Fields != nil {
			for _, f := range v.Fields.List {
				if err := formatExpr(buf, f.Type); err != nil {
					return err
				}
				buf.WriteString("; ")
			}
		}
		buf.WriteString("}")
	case *ast.IndexExpr:
		if err := formatExpr(buf, v.X); err != nil {
			return err
		}
		buf.WriteByte('[')
		if err := formatExpr(buf, v.Index); err != nil {
			return err
		}
		buf.WriteByte(']')
	case *ast.IndexListExpr:
		if err := formatExpr(buf, v.X); err != nil {
			return err
		}
		buf.WriteByte('[')
		for i, idx := range v.Indices {
			if i > 0 {
				buf.WriteString(", ")
			}
			if err := formatExpr(buf, idx); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case *ast.ParenExpr:
		buf.WriteByte('(')
		if err := formatExpr(buf, v.X); err != nil {
			return err
		}
		buf.WriteByte(')')
	default:
		buf.WriteString("<unsupported>")
	}
	return nil
}

func writeFieldList(buf *bytes.Buffer, fl *ast.FieldList) error {
	if fl == nil {
		return nil
	}
	buf.WriteByte('(')
	for i, f := range fl.List {
		if i > 0 {
			buf.WriteString(", ")
		}
		if len(f.Names) > 0 {
			for j, n := range f.Names {
				if j > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(n.Name)
			}
			buf.WriteByte(' ')
		}
		if err := formatExpr(buf, f.Type); err != nil {
			return err
		}
	}
	buf.WriteByte(')')
	return nil
}
