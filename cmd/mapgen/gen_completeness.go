package main

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/cmd/mapgen/astutil"
	"github.com/kaecer68/atlas-go/cmd/mapgen/maps"
)

// generateCompleteness generates .omo/maps/module-completeness.md.
// It walks internal/, analyses each module via AST for function/stub counts,
// counts TODO/FIXME comments, runs coverage tests, and computes a
// completeness score per module.
func generateCompleteness() error {
	repoRoot, err := astutil.FindRepoRoot(".")
	if err != nil {
		return fmt.Errorf("find repo root: %w", err)
	}

	internalDir := filepath.Join(repoRoot, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return fmt.Errorf("read internal dir: %w", err)
	}

	// Collect module directories for parallel analysis.
	type modEntry struct {
		dir  string
		name string
	}
	var modules []modEntry
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "testdata" {
			continue
		}
		modules = append(modules, modEntry{
			dir:  filepath.Join(internalDir, e.Name()),
			name: e.Name(),
		})
	}

	// Parallel analysis with bounded concurrency.
	const maxConcurrent = 4
	sem := make(chan struct{}, maxConcurrent)
	type result struct {
		report maps.CompletenessReport
		err    error
	}
	results := make([]result, len(modules))

	var wg sync.WaitGroup
	for i, m := range modules {
		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(idx int, mod modEntry) {
			defer wg.Done()
			defer func() { <-sem }() // release
			report, err := analyseModule(repoRoot, mod.dir, mod.name)
			results[idx] = result{report, err}
		}(i, m)
	}
	wg.Wait()

	var reports []maps.CompletenessReport
	for _, r := range results {
		if r.err != nil {
			return fmt.Errorf("analyze module: %w", r.err)
		}
		reports = append(reports, r.report)
	}

	// Sort alphabetically.
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Module < reports[j].Module
	})

	content := renderCompletenessReport(reports)
	if err := maps.WriteMap("module-completeness.md", content); err != nil {
		return fmt.Errorf("write completeness map: %w", err)
	}

	return nil
}

// moduleAnalysis holds intermediate per-module metrics.
type moduleAnalysis struct {
	totalFuncs   int
	stubCount    int
	todoCount    int
	fixmeCount   int
	hackCount    int
	goFiles      int
	totalFiles   int
	testCoverage float64
	notes        string
}

// analyseModule performs AST analysis, comment counting, and coverage testing
// for a single module directory.
func analyseModule(repoRoot, modDir, modName string) (maps.CompletenessReport, error) {
	analysis := &moduleAnalysis{}

	// 1. AST analysis: count functions, stubs, and go files.
	if err := walkModuleAST(modDir, analysis); err != nil {
		return maps.CompletenessReport{}, fmt.Errorf("ast walk: %w", err)
	}

	// 2. Count TODO/FIXME/HACK comments.
	countCommentPatterns(modDir, analysis)

	// 3. Run coverage test.
	analysis.runCoverage(repoRoot, modName)

	// 4. Calculate completeness score.
	score := calcCompletenessScore(analysis)

	// 5. Load previous percentage for delta.
	prevPct := loadPreviousPct(modName)

	return maps.CompletenessReport{
		Module:          modName,
		GoFiles:         analysis.goFiles,
		TotalFiles:      analysis.totalFiles,
		TotalFuncs:      analysis.totalFuncs,
		StubCount:       analysis.stubCount,
		TODOCount:       analysis.todoCount,
		FIXMECount:      analysis.fixmeCount,
		TestCoverage:    analysis.testCoverage,
		CompletenessPct: score,
		PreviousPct:     prevPct,
		Notes:           analysis.notes,
	}, nil
}

// walkModuleAST walks all non-test Go files in modDir, parses each into an
// AST, and counts total function declarations and stub functions.
func walkModuleAST(modDir string, a *moduleAnalysis) error {
	fset := token.NewFileSet()

	return filepath.Walk(modDir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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

		a.totalFiles++
		if !strings.HasSuffix(info.Name(), "_test.go") {
			a.goFiles++
		}

		f := astutil.ParseGoFile(fset, p)
		if f == nil {
			return nil
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			a.totalFuncs++
			if isStub(fn) {
				a.stubCount++
			}
		}

		return nil
	})
}

// isStub reports whether fn is a stub function — i.e. its body has 0-1
// statements where the only statement is `return nil, nil` (or any number
// of nils, or a bare `return`).
func isStub(fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return true
	}
	if len(fn.Body.List) == 0 {
		return true
	}
	if len(fn.Body.List) > 1 {
		return false
	}

	ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
	if !ok {
		return false
	}

	// Bare return.
	if len(ret.Results) == 0 {
		return true
	}

	// All results must be nil identifiers.
	for _, r := range ret.Results {
		ident, ok := r.(*ast.Ident)
		if !ok || ident.Name != "nil" {
			return false
		}
	}
	return true
}

// countCommentPatterns walks all Go files in modDir and counts TODO/FIXME/HACK
// comments using astutil.CountPatterns.
func countCommentPatterns(modDir string, a *moduleAnalysis) {
	fset := token.NewFileSet()

	patterns := map[string]*regexp.Regexp{
		"TODO":  astutil.ReTODO,
		"FIXME": astutil.ReFIXME,
		"HACK":  astutil.ReHACK,
	}

	_ = filepath.Walk(modDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") {
			return nil
		}

		f := astutil.ParseGoFile(fset, p)
		if f == nil {
			return nil
		}

		counts := astutil.CountPatterns(fset, f, patterns)
		a.todoCount += counts["TODO"]
		a.fixmeCount += counts["FIXME"]
		a.hackCount += counts["HACK"]

		return nil
	})
}

// runCoverage executes `go test -cover ./internal/<modName>/...` with a 30s
// timeout and parses the coverage percentage from stdout.
func (a *moduleAnalysis) runCoverage(repoRoot, modName string) {
	// Check if the module has any test files.
	hasTests := false
	modDir := filepath.Join(repoRoot, "internal", modName)
	_ = filepath.Walk(modDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), "_test.go") {
			hasTests = true
			return filepath.SkipDir
		}
		return nil
	})

	if !hasTests {
		a.testCoverage = 0
		a.notes = "no tests"
		return
	}

	pkgPath := "github.com/kaecer68/atlas-go/internal/" + modName
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "test", "-cover", "-coverprofile=/dev/null", pkgPath+"/...")
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = stderr // captured but not used

	_ = cmd.Run() // ignore exit code; coverage may still be present

	// Coverage line may be in stdout or stderr depending on Go version/test output.
	combined := stdout.String() + "\n" + stderr.String()
	coverage := parseCoverage(combined)
	if coverage < 0 {
		a.testCoverage = 0
		a.notes = "coverage parse failed"
		return
	}

	a.testCoverage = coverage
}

// parseCoverage extracts the percentage from a line like:
// "coverage: 68.3% of statements"
// Returns -1 if not found.
func parseCoverage(output string) float64 {
	for line := range strings.SplitSeq(output, "\n") {
		idx := strings.Index(line, "coverage:")
		if idx < 0 {
			continue
		}
		rest := strings.TrimSpace(line[idx:])
		parts := strings.Fields(rest)
		if len(parts) < 2 {
			continue
		}
		pctStr := strings.TrimSuffix(parts[1], "%")
		var pct float64
		if _, err := fmt.Sscanf(pctStr, "%f", &pct); err != nil {
			continue
		}
		return pct / 100.0
	}
	return -1
}

// calcCompletenessScore computes the completeness score for a module.
//
//	score = 100
//	  - stubPenalty = (stubCount / max(totalFunctions, 1)) * 40   // capped at 40
//	  - todoPenalty = (todoCount + fixmeCount) * 2                 // capped at 20
//	  - testPenalty = (1 - testCoverage%) * 40                      // capped at 40
//	// score clamped to [0, 100]
func calcCompletenessScore(a *moduleAnalysis) int {
	// Stub penalty: proportion of stubs * 40, capped at 40.
	totalFuncs := math.Max(float64(a.totalFuncs), 1)
	stubPenalty := (float64(a.stubCount) / totalFuncs) * 40.0
	if stubPenalty > 40 {
		stubPenalty = 40
	}

	// TODO/FIXME penalty: count * 2, capped at 20.
	todoPenalty := float64(a.todoCount+a.fixmeCount) * 2.0
	if todoPenalty > 20 {
		todoPenalty = 20
	}

	// Test penalty: (1 - coverage) * 40, capped at 40.
	testPenalty := (1.0 - a.testCoverage) * 40.0
	if testPenalty > 40 {
		testPenalty = 40
	}

	score := 100.0 - stubPenalty - todoPenalty - testPenalty
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	return int(math.Round(score))
}

// loadPreviousPct reads the previous completeness report and extracts the
// percentage for the given module.
func loadPreviousPct(modName string) int {
	prev := maps.LoadPrevious("module-completeness.md")
	if prev == "" {
		return 0
	}

	// Simple line-by-line search for the module row.
	for line := range strings.SplitSeq(prev, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "| "), "|")
		if len(cells) < 1 {
			continue
		}
		if strings.TrimSpace(cells[0]) == modName {
			// Score is in column index 6 (0-based: Module, Funcs, Stubs, TODOs, FIXMEs, Coverage, Score).
			if len(cells) > 6 {
				scoreStr := strings.TrimSpace(cells[6])
				scoreStr = strings.TrimSuffix(scoreStr, "%")
				var score int
				if _, err := fmt.Sscanf(scoreStr, "%d", &score); err == nil {
					return score
				}
			}
		}
	}
	return 0
}

// renderCompletenessReport builds the full markdown content.
func renderCompletenessReport(reports []maps.CompletenessReport) string {
	var sb strings.Builder

	// Compute summary stats.
	totalScore := 0
	bestMod := ""
	bestScore := -1
	worstMod := ""
	worstScore := 101
	var noTestMods []string

	for _, r := range reports {
		totalScore += r.CompletenessPct
		if r.CompletenessPct > bestScore {
			bestScore = r.CompletenessPct
			bestMod = r.Module
		}
		if r.CompletenessPct < worstScore {
			worstScore = r.CompletenessPct
			worstMod = r.Module
		}
		if strings.Contains(r.Notes, "no tests") {
			noTestMods = append(noTestMods, r.Module)
		}
	}

	avgScore := 0
	if len(reports) > 0 {
		avgScore = totalScore / len(reports)
	}

	// Header.
	ts := time.Now().UTC().Format("2006-01-02 15:04 UTC")
	sb.WriteString("# Module Completeness Report\n")
	fmt.Fprintf(&sb, "> Generated: %s | %d modules | Average score: %d%%\n\n",
		ts, len(reports), avgScore)

	// Table.
	header := []string{"Module", "Funcs", "Stubs", "TODOs", "FIXMEs", "Coverage", "Score", "Δ", "Notes"}
	var rows [][]string
	for _, r := range reports {
		delta := ""
		if r.PreviousPct > 0 {
			diff := r.CompletenessPct - r.PreviousPct
			if diff > 0 {
				delta = fmt.Sprintf("+%d%%", diff)
			} else if diff < 0 {
				delta = fmt.Sprintf("%d%%", diff)
			} else {
				delta = "0%"
			}
		}

		rows = append(rows, []string{
			r.Module,
			fmt.Sprintf("%d", r.TotalFuncs),
			fmt.Sprintf("%d", r.StubCount),
			fmt.Sprintf("%d", r.TODOCount),
			fmt.Sprintf("%d", r.FIXMECount),
			maps.Pct(r.TestCoverage),
			fmt.Sprintf("%d%%", r.CompletenessPct),
			delta,
			r.Notes,
		})
	}
	sb.WriteString(maps.MarkdownTable(header, rows))
	sb.WriteString("\n")

	// Summary.
	sb.WriteString("## Summary\n\n")
	fmt.Fprintf(&sb, "- Average completeness: %d%%\n", avgScore)
	if bestMod != "" {
		fmt.Fprintf(&sb, "- Most complete module: %s (%d%%)\n", bestMod, bestScore)
	}
	if worstMod != "" {
		fmt.Fprintf(&sb, "- Needs attention: %s (%d%%)\n", worstMod, worstScore)
	}
	if len(noTestMods) > 0 {
		fmt.Fprintf(&sb, "- Modules with no tests: %s\n", strings.Join(noTestMods, ", "))
	}

	fmt.Fprintf(&sb, "\n_Generated by cmd/mapgen. Last updated: %s_\n", ts)

	return sb.String()
}
