// Command gentags generates JavaScript and TypeScript type definitions from Go structs.
//
// Usage: go run ./cmd/gentags
//
// Reads:
//
//	internal/domain/**/*.go             — domain types (Scorecard, GuardOutcome, Regime, ...)
//	internal/monitoring/api/**/*.go     — API response types (AgentObservatoryResponse, ...)
//	internal/monitoring/service/**/*.go — service response types (SystemHealthResponse, ...)
//	internal/reporting/**/*.go          — performance report types (PerformanceReport, AgentPerformance, ...)
//
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
	"encoding/json"
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
	outValidFields := filepath.Join(rootDir, "web/static/js/shared/valid_fields.json")

	// Scan domain types first (foundational), then API response types (depend on domain types).
	structs := parseStructs(domainDir)
	apiDir := findMonitoringAPIDir(rootDir)
	svcDir := findMonitoringServiceDir(rootDir)
	reportDir := findReportingDir(rootDir)
	configDir := findConfigDir(rootDir)
	if apiDir != "" || svcDir != "" || reportDir != "" || configDir != "" {
		// Build merged struct names from all directories so cross-package
		// type references resolve correctly.
		allNames := preScanStructNames(domainDir)
		for _, d := range []string{apiDir, svcDir, configDir} {
			if d == "" {
				continue
			}
			for k := range preScanStructNames(d) {
				allNames[k] = true
			}
		}
		// Merge API structs.
		if apiDir != "" {
			apiStructs := parseStructsWithNames(apiDir, allNames)
			for k, v := range apiStructs {
				if _, exists := structs[k]; exists {
					fmt.Fprintf(os.Stderr, "gentags: struct %q exists in both domain and api; using api version\n", k)
				}
				structs[k] = v
			}
		}
		// Merge service structs (e.g. CircuitBreakerStateResponse, SystemHealthResponse).
		if svcDir != "" {
			svcStructs := parseStructsWithNames(svcDir, allNames)
			for k, v := range svcStructs {
				if _, exists := structs[k]; exists {
					fmt.Fprintf(os.Stderr, "gentags: struct %q exists in both domain and service; using service version\n", k)
				}
				structs[k] = v
			}
		}
		// Merge reporting structs (e.g. PerformanceReport, AgentPerformance).
		if reportDir != "" {
			reportStructs := parseStructsWithNames(reportDir, allNames)
			for k, v := range reportStructs {
				if _, exists := structs[k]; exists {
					fmt.Fprintf(os.Stderr, "gentags: struct %q exists in both domain and reporting; using reporting version\n", k)
				}
				structs[k] = v
			}
		}
		// Merge config structs (e.g. ParametersConfig, CalibrationEvidence).
		if configDir != "" {
			configStructs := parseStructsWithNames(configDir, allNames)
			for k, v := range configStructs {
				structs[k] = v
			}
		}
	}

	writeFieldNames(structs, outJS)
	writeTypeScriptInterfaces(structs, outTS, false)
	writeTypeScriptInterfaces(structs, outDTS, true)
	writeValidFields(structs, outValidFields)
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

func findMonitoringAPIDir(rootDir string) string {
	candidate := filepath.Join(rootDir, "internal", "monitoring", "api")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func findMonitoringServiceDir(rootDir string) string {
	candidate := filepath.Join(rootDir, "internal", "monitoring", "service")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func findReportingDir(rootDir string) string {
	candidate := filepath.Join(rootDir, "internal", "reporting")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func findConfigDir(rootDir string) string {
	candidate := filepath.Join(rootDir, "internal", "config")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func parseStructs(domainDir string) map[string][]structField {
	return parseStructsWithNames(domainDir, nil)
}

func parseStructsWithNames(dir string, baseStructNames map[string]bool) map[string][]structField {
	fset := token.NewFileSet()
	structs := make(map[string][]structField)

	// Two-pass scan:
	//   1. Pre-pass: collect all Go struct type names so goTypeToTS can emit
	//      proper TypeScript interface references (e.g., ParameterSnapshot | null
	//      instead of string | null) while keeping string type aliases as string.
	//   2. Main pass: parse fields with type resolution.
	rawNames := preScanStructNames(dir)
	// Merge with base struct names for cross-directory type resolution
	// (e.g. API structs referencing domain types like domain.GuardOutcome).
	structNames := rawNames
	if baseStructNames != nil {
		structNames = make(map[string]bool, len(rawNames)+len(baseStructNames))
		for k := range rawNames {
			structNames[k] = true
		}
		for k := range baseStructNames {
			structNames[k] = true
		}
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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

	_ = filepath.Walk(domainDir, func(path string, info os.FileInfo, err error) error {
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
	b.WriteString("// Source: internal/domain/**/*.go, internal/monitoring/api/**/*.go, internal/monitoring/service/**/*.go\n")
	b.WriteString("\n")
	b.WriteString("export const FIELD = {\n")

	names := make([]string, 0, len(structs))
	for name := range structs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, structName := range names {
		fields := structs[structName]
		fmt.Fprintf(&b, "  %s: {\n", structName)
		for _, f := range fields {
			fmt.Fprintf(&b, "    %s: '%s',\n", f.JSONName, f.JSONName)
		}
		b.WriteString("  },\n")
	}
	b.WriteString("};\n")

	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d structs to %s\n", len(structs), out)
}

func writeTypeScriptInterfaces(structs map[string][]structField, out string, ambient bool) {
	var b strings.Builder
	b.WriteString("// Auto-generated by cmd/gentags. DO NOT EDIT.\n")
	b.WriteString("// Run: go generate ./...  or  go run ./cmd/gentags\n")
	b.WriteString("// Source: internal/domain/**/*.go, internal/monitoring/api/**/*.go, internal/monitoring/service/**/*.go\n")
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
		fmt.Fprintf(&b, "%sinterface %s {\n", prefix, structName)
		for _, f := range fields {
			optionalMark := ""
			if f.Optional {
				optionalMark = "?"
			}
			fmt.Fprintf(&b, "  %s%s: %s;\n", f.JSONName, optionalMark, f.TSType)
		}
		b.WriteString("}\n\n")
	}

	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
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

func writeValidFields(structs map[string][]structField, out string) {
	seen := make(map[string]bool)
	var fields []string
	for _, fs := range structs {
		for _, f := range fs {
			if !seen[f.JSONName] {
				seen[f.JSONName] = true
				fields = append(fields, f.JSONName)
			}
		}
	}
	sort.Strings(fields)
	data, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal valid_fields: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d fields to %s\n", len(fields), out)
}
