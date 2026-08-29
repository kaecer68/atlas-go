package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kaecer68/atlas-go/cmd/mapgen/astutil"
	"github.com/kaecer68/atlas-go/cmd/mapgen/maps"
)

const modulePrefix = "github.com/kaecer68/atlas-go"

// generateArchitecture generates .omo/maps/architecture.md.
// It walks internal/, collects module stats, parses AGENTS.md for role
// descriptions, builds an import dependency graph, and writes the full
// architecture map as markdown.
func generateArchitecture() error {
	repoRoot, err := astutil.FindRepoRoot(".")
	if err != nil {
		return fmt.Errorf("find repo root: %w", err)
	}

	agentsPath := filepath.Join(repoRoot, "AGENTS.md")
	internalDir := filepath.Join(repoRoot, "internal")

	// 1. Parse AGENTS.md to extract module role descriptions.
	roleMap, err := parseAgentRoles(agentsPath)
	if err != nil {
		return fmt.Errorf("parse AGENTS.md: %w", err)
	}

	// 2. Walk internal/ and collect module metadata.
	modules, err := collectModules(internalDir, roleMap)
	if err != nil {
		return fmt.Errorf("collect modules: %w", err)
	}

	// Sort alphabetically for deterministic output.
	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Name < modules[j].Name
	})

	// 3. Build import dependency graph.
	importGraph := buildImportGraph(internalDir, modules)

	// 4. Render and write the map.
	totalGoFiles := 0
	totalLOC := 0
	for _, m := range modules {
		totalGoFiles += m.GoFiles + m.TestFiles
		totalLOC += m.LOC
	}

	content := renderArchitectureMap(modules, importGraph, totalGoFiles, totalLOC)
	if err := maps.WriteMap("architecture.md", content); err != nil {
		return fmt.Errorf("write architecture map: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// AGENTS.md parsing
// ---------------------------------------------------------------------------

// parseAgentRoles reads AGENTS.md and extracts module role descriptions
// from the markdown tables in the "核心架構" section.
// Returns a map from module short-name (e.g., "orchestrator") to its role
// description string.
func parseAgentRoles(agentsPath string) (map[string]string, error) {
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		return nil, fmt.Errorf("read AGENTS.md: %w", err)
	}

	roleMap := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	inTable := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of a module table.
		if strings.Contains(trimmed, "| 目錄 | 職責 |") ||
			strings.Contains(trimmed, "| 目錄 | 職責") {
			inTable = true
			continue
		}

		// Skip separator lines.
		if inTable && strings.Contains(trimmed, "|---") {
			continue
		}

		// End of table: empty line outside a row, or a section heading.
		if inTable && trimmed == "" {
			inTable = false
			continue
		}
		if inTable && strings.HasPrefix(trimmed, "#") {
			inTable = false
			continue
		}

		if !inTable {
			continue
		}

		// Must be a table row starting with |
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}

		// Parse: | `internal/name/` | role description |
		cols := splitTableRow(trimmed)
		if len(cols) < 2 {
			continue
		}

		name := extractModuleName(cols[0])
		if name == "" {
			continue
		}

		role := strings.TrimSpace(cols[1])
		if role == "" {
			role = "_(no description)_"
		}

		// Keep only the first entry for each module (AGENTS.md lists them once).
		if _, exists := roleMap[name]; !exists {
			roleMap[name] = role
		}
	}

	return roleMap, nil
}

// splitTableRow tokenises a markdown table row into cells.
func splitTableRow(s string) []string {
	// Trim leading/trailing |
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	parts := strings.Split(s, "|")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		result = append(result, strings.TrimSpace(p))
	}
	return result
}

// extractModuleName extracts the short module name from a table cell like
// "`internal/orchestrator/`" → "orchestrator".
func extractModuleName(cell string) string {
	cell = strings.TrimSpace(cell)
	cell = strings.Trim(cell, "`")
	cell = strings.TrimPrefix(cell, "internal/")
	cell = strings.TrimSuffix(cell, "/")
	if cell == "" {
		return ""
	}
	// Take only the first segment (e.g., "portfolio/optimizer" → "portfolio")
	if idx := strings.Index(cell, "/"); idx >= 0 {
		cell = cell[:idx]
	}
	return cell
}

// ---------------------------------------------------------------------------
// Module discovery
// ---------------------------------------------------------------------------

// collectModules walks internal/ and returns a slice of ModuleInfo for each
// subdirectory that contains Go source files.
func collectModules(internalDir string, roleMap map[string]string) ([]maps.ModuleInfo, error) {
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return nil, fmt.Errorf("read internal dir: %w", err)
	}

	var modules []maps.ModuleInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()

		// Skip hidden directories.
		if strings.HasPrefix(name, ".") {
			continue
		}
		// Skip testdata / fixtures.
		if name == "testdata" {
			continue
		}

		dirPath := filepath.Join(internalDir, name)

		goFiles, testFiles, loc, err := countModuleFiles(dirPath)
		if err != nil {
			return nil, fmt.Errorf("count %s: %w", name, err)
		}

		// Skip directories without any Go files.
		if goFiles == 0 && testFiles == 0 {
			continue
		}

		role := roleMap[name]
		if role == "" {
			role = "_(no description)_"
		}

		modules = append(modules, maps.ModuleInfo{
			Name:        name,
			Path:        dirPath,
			GoFiles:     goFiles,
			TestFiles:   testFiles,
			LOC:         loc,
			Role:        role,
			HasAgentsMD: hasAgentsMD(dirPath),
		})
	}
	return modules, nil
}

// countModuleFiles walks a module directory recursively and returns:
//
//	goFiles  — count of .go files (excluding _test.go)
//	testFiles — count of _test.go files
//	loc       — total lines across all .go files
func countModuleFiles(dirPath string) (goFiles, testFiles, loc int, err error) {
	err = filepath.Walk(dirPath, func(p string, info os.FileInfo, wErr error) error {
		if wErr != nil {
			return wErr
		}
		if info.IsDir() {
			base := filepath.Base(p)
			if strings.HasPrefix(base, ".") || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		lines := astutil.CountLines(p)
		loc += lines

		if strings.HasSuffix(info.Name(), "_test.go") {
			testFiles++
		} else {
			goFiles++
		}
		return nil
	})
	return
}

// hasAgentsMD returns true if the directory contains an AGENTS.md file.
func hasAgentsMD(dirPath string) bool {
	_, err := os.Stat(filepath.Join(dirPath, "AGENTS.md"))
	return err == nil
}

// ---------------------------------------------------------------------------
// Import dependency graph
// ---------------------------------------------------------------------------

// buildImportGraph walks every Go file in each module and records internal
// imports. Returns a map from module name → sorted, deduplicated list of
// imported modules.
func buildImportGraph(internalDir string, modules []maps.ModuleInfo) map[string][]string {
	graph := make(map[string][]string)

	for _, mod := range modules {
		imports := extractImports(filepath.Join(internalDir, mod.Name), mod.Name)
		if len(imports) > 0 {
			graph[mod.Name] = imports
		}
	}
	return graph
}

// extractImports parses all Go files in dirPath and returns a deduplicated,
// sorted list of internal module names that this directory imports.
func extractImports(dirPath, selfName string) []string {
	seen := make(map[string]bool)
	fset := token.NewFileSet()

	_ = filepath.Walk(dirPath, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip on error
		}
		if info.IsDir() {
			base := filepath.Base(p)
			if strings.HasPrefix(base, ".") || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}

		for _, imp := range f.Imports {
			if imp.Path == nil {
				continue
			}
			path := strings.Trim(imp.Path.Value, "\"")
			if !strings.HasPrefix(path, modulePrefix+"/internal/") {
				continue
			}

			modName := moduleNameFromImport(path)
			if modName == "" || modName == selfName {
				continue
			}
			seen[modName] = true
		}
		return nil
	})

	var result []string
	for m := range seen {
		result = append(result, m)
	}
	sort.Strings(result)
	return result
}

// moduleNameFromImport extracts the top-level module name from an internal
// import path. For example:
//
//	github.com/kaecer68/atlas-go/internal/domain         → "domain"
//	github.com/kaecer68/atlas-go/internal/portfolio/opt  → "portfolio"
func moduleNameFromImport(importPath string) string {
	trimmed := strings.TrimPrefix(importPath, modulePrefix+"/internal/")
	if before, _, ok := strings.Cut(trimmed, "/"); ok {
		return before
	}
	return trimmed
}

// ---------------------------------------------------------------------------
// Markdown rendering
// ---------------------------------------------------------------------------

// renderArchitectureMap builds the full architecture.md content.
func renderArchitectureMap(modules []maps.ModuleInfo, importGraph map[string][]string, totalGoFiles, totalLOC int) string {
	var sb strings.Builder

	// Header with metadata.
	ts := timestampUTC()
	sb.WriteString("# Atlas System Architecture Map\n")
	fmt.Fprintf(
		&sb,
		"> Generated: %s | Modules: %d | Go Files: %d | Total LOC: %s\n\n",
		ts,
		len(modules),
		totalGoFiles,
		formatNum(totalLOC),
	)

	// Module Inventory table.
	sb.WriteString("## Module Inventory\n\n")
	header := []string{"Module", "Go Files", "Test Files", "LOC", "Role"}
	var rows [][]string
	for _, m := range modules {
		rows = append(rows, []string{
			m.Name,
			fmt.Sprintf("%d", m.GoFiles),
			fmt.Sprintf("%d", m.TestFiles),
			formatNum(m.LOC),
			m.Role,
		})
	}
	sb.WriteString(maps.MarkdownTable(header, rows))
	sb.WriteString("\n")

	// Import Dependency Graph.
	sb.WriteString("## Import Dependency Graph\n\n")
	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	for _, m := range modules {
		deps, ok := importGraph[m.Name]
		if !ok || len(deps) == 0 {
			continue
		}
		for _, dep := range deps {
			// Skip domain — it's imported by nearly everyone and clutters the graph.
			if dep == "domain" {
				continue
			}
			fmt.Fprintf(&sb, "  %s --> %s\n", m.Name, dep)
		}
	}
	sb.WriteString("```\n\n")

	// Footer.
	fmt.Fprintf(&sb, "_Generated by cmd/mapgen. Last updated: %s_\n", timestampUTC())

	return sb.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// timestampUTC returns the current time in UTC as "2006-01-02 15:04 MST".
func timestampUTC() string {
	return time.Now().UTC().Format("2006-01-02 15:04 MST")
}

// formatNum inserts commas for readability (e.g., 123456 → "123,456").
func formatNum(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result = append(result, ',')
		}
		//nolint:gosec // ASCII digits only, no overflow risk
		result = append(result, byte(c))
	}
	return string(result)
}
