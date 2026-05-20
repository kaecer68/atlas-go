// Command gentags generates JavaScript and TypeScript type definitions from Go domain structs.
//
// Usage: go run ./cmd/gentags
//
// Reads: internal/domain/**/*.go  (recursively)
// Writes:
//
//	web/static/js/shared/field_names.js  — snake_case field name constants
//	web/static/js/shared/field_types.ts   — TypeScript interfaces for explicit import
//	web/static/js/shared/field_types.d.ts — ambient declarations, IDE auto-discovers
//
// This eliminates manual synchronization between Go backend structs
// and frontend field access — a single source of truth.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type structField struct {
	Name     string
	JSONName string
	TSType   string
	Optional bool
}

func main() {
	domainDir := findDomainDir()
	rootDir := filepath.Dir(domainDir)
	if filepath.Base(domainDir) == "domain" {
		rootDir = filepath.Dir(rootDir)
	}
	outJS := filepath.Join(rootDir, "web/static/js/shared/field_names.js")
	outTS := filepath.Join(rootDir, "web/static/js/shared/field_types.ts")
	outDTS := filepath.Join(rootDir, "web/static/js/shared/field_types.d.ts")

	structs := parseStructs(domainDir)

	writeFieldNames(structs, outJS)
	writeTypeScriptInterfaces(structs, outTS, false)
	writeTypeScriptInterfaces(structs, outDTS, true)
}

func findDomainDir() string {
	if _, err := os.Stat("internal/domain"); err == nil {
		return "internal/domain"
	}
	cwd, _ := os.Getwd()
	for {
		candidate := filepath.Join(cwd, "internal", "domain")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			break
		}
		cwd = parent
	}
	return "internal/domain"
}

func parseStructs(domainDir string) map[string][]structField {
	fset := token.NewFileSet()
	structs := make(map[string][]structField)

	// Two-pass scan:
	//   1. Pre-pass: collect all Go struct type names so goTypeToTS can emit
	//      proper TypeScript interface references (e.g., ParameterSnapshot | null
	//      instead of string | null) while keeping string type aliases as string.
	//   2. Main pass: parse fields with type resolution.
	structNames := preScanStructNames(domainDir)

	err := filepath.Walk(domainDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			fmt.Fprintf(os.Stderr, "parse %s: %v\n", path, err)
			return nil
		}

		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}

				var fields []structField
				for _, field := range st.Fields.List {
					if field.Tag == nil || len(field.Names) == 0 {
						continue
					}
					jsonName := extractJSONName(field.Tag.Value)
					if jsonName == "" || jsonName == "-" {
						continue
					}
					optional := strings.Contains(field.Tag.Value, ",omitempty")
					tsType := goTypeToTSWithStructs(field.Type, structNames)
					fields = append(fields, structField{
						Name:     field.Names[0].Name,
						JSONName: jsonName,
						TSType:   tsType,
						Optional: optional,
					})
				}
				if len(fields) > 0 {
					structs[ts.Name.Name] = fields
				}
			}
		}
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk dir: %v\n", err)
		os.Exit(1)
	}
	return structs
}

// preScanStructNames walks the domain directory and collects all Go struct
// type names (ast.StructType) for use by goTypeToTSWithStructs.
func preScanStructNames(domainDir string) map[string]bool {
	structNames := make(map[string]bool)
	fset := token.NewFileSet()

	filepath.Walk(domainDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if parseErr != nil {
			return nil
		}
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, isStruct := ts.Type.(*ast.StructType); isStruct {
					structNames[ts.Name.Name] = true
				}
			}
		}
		return nil
	})
	return structNames
}

func goTypeToTSWithStructs(expr ast.Expr, structNames map[string]bool) string {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "string":
			return "string"
		case "bool":
			return "boolean"
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"float32", "float64", "complex64", "complex128":
			return "number"
		default:
			// Custom types (enums like Regime, GuardSeverity) → string
			return "string"
		}
	case *ast.StarExpr:
		return goTypeToTSWithStructs(t.X, structNames) + " | null"
	case *ast.ArrayType:
		return goTypeToTSWithStructs(t.Elt, structNames) + "[]"
	case *ast.SelectorExpr:
		// time.Time → string (serialized as ISO8601)
		if ident, ok := t.X.(*ast.Ident); ok && ident.Name == "time" {
			return "string"
		}
		// For cross-package struct types, emit the type name
		// (e.g., shared.ParameterSnapshot → ParameterSnapshot).
		// Non-struct type aliases (e.g., shared.AgentLayer, shared.Side)
		// fall through to "string".
		if structNames[t.Sel.Name] {
			return t.Sel.Name
		}
		return "string"
	case *ast.MapType:
		return fmt.Sprintf("Record<%s, %s>", goTypeToTSWithStructs(t.Key, structNames), goTypeToTSWithStructs(t.Value, structNames))
	case *ast.InterfaceType:
		return "unknown"
	default:
		return "string"
	}
}

func writeFieldNames(structs map[string][]structField, out string) {
	var b strings.Builder
	b.WriteString("// Auto-generated by cmd/gentags. DO NOT EDIT.\n")
	b.WriteString("// Run: go generate ./...  or  go run ./cmd/gentags\n")
	b.WriteString("// Source: internal/domain/*.go json tags\n")
	b.WriteString("\n")
	b.WriteString("export const FIELD = {\n")

	names := make([]string, 0, len(structs))
	for name := range structs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, structName := range names {
		fields := structs[structName]
		b.WriteString(fmt.Sprintf("  %s: {\n", structName))
		for _, f := range fields {
			b.WriteString(fmt.Sprintf("    %s: '%s',\n", f.JSONName, f.JSONName))
		}
		b.WriteString("  },\n")
	}
	b.WriteString("};\n")

	if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d structs to %s\n", len(structs), out)
}

func writeTypeScriptInterfaces(structs map[string][]structField, out string, ambient bool) {
	var b strings.Builder
	b.WriteString("// Auto-generated by cmd/gentags. DO NOT EDIT.\n")
	b.WriteString("// Run: go generate ./...  or  go run ./cmd/gentags\n")
	b.WriteString("// Source: internal/domain/*.go json tags\n")
	if !ambient {
		b.WriteString("//\n")
		b.WriteString("// Use: import type { GuardOutcome, RecommendationOutcome } from './field_types.js';\n")
		b.WriteString("//      const item: RecommendationOutcome = apiResponse.items[0];\n")
	}
	b.WriteString("\n")

	names := make([]string, 0, len(structs))
	for name := range structs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, structName := range names {
		fields := structs[structName]
		prefix := "export "
		if ambient {
			prefix = "declare "
		}
		b.WriteString(fmt.Sprintf("%sinterface %s {\n", prefix, structName))
		for _, f := range fields {
			optionalMark := ""
			if f.Optional {
				optionalMark = "?"
			}
			b.WriteString(fmt.Sprintf("  %s%s: %s;\n", f.JSONName, optionalMark, f.TSType))
		}
		b.WriteString("}\n\n")
	}

	if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d interfaces to %s\n", len(structs), out)
}

func extractJSONName(tag string) string {
	tag = strings.Trim(tag, "`")
	_, after, ok := strings.Cut(tag, `json:"`)
	if !ok {
		return ""
	}
	rest := after
	before, _, ok := strings.Cut(rest, "\"")
	if !ok {
		return ""
	}
	name := before
	if comma := strings.IndexByte(name, ','); comma >= 0 {
		name = name[:comma]
	}
	return name
}
