// Command check-maturity validates that every internal/ Go package has a
// doc.go with a valid Maturity: tag, and that the MATURITY.md reference
// table is consistent with the doc.go tags.
//
// Usage:
//
//	go run ./cmd/check-maturity          # full check
//	go run ./cmd/check-maturity --json   # JSON output for CI
//
// Exit code: 0 = all checks pass, 1 = violations found.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var validTiers = map[string]bool{
	"stable":       true,
	"evolving":     true,
	"experimental": true,
	"utility":      true,
}

func main() {
	jsonMode := len(os.Args) > 1 && os.Args[1] == "--json"

	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal("cannot find repo root: %v", err)
	}

	violations := runChecks(repoRoot)

	if jsonMode {
		outputJSON(violations)
	} else {
		outputText(violations)
	}

	if len(violations) > 0 {
		os.Exit(1)
	}
}

type Violation struct {
	Check  string `json:"check"`
	File   string `json:"file"`
	Detail string `json:"detail"`
}

func runChecks(repoRoot string) []Violation {
	var all []Violation
	all = append(all, checkDocGoExists(repoRoot)...)
	all = append(all, checkMaturityTags(repoRoot)...)
	all = append(all, checkMaturityMD(repoRoot)...)
	all = append(all, checkCrossConsistency(repoRoot)...)
	return all
}

// getGoPackages returns all directories under internal/ that contain .go files,
// excluding testdata (which is a Go test fixture directory, not a package).
func getGoPackages(repoRoot string) ([]string, error) {
	internalDir := filepath.Join(repoRoot, "internal")
	entries, err := os.ReadDir(internalDir)
	if err != nil {
		return nil, err
	}
	var pkgs []string
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "testdata" {
			continue
		}
		matches, _ := filepath.Glob(filepath.Join(internalDir, e.Name(), "*.go"))
		if len(matches) > 0 {
			pkgs = append(pkgs, e.Name())
		}
	}
	sort.Strings(pkgs)
	return pkgs, nil
}

// checkDocGoExists verifies every Go package has a doc.go file.
func checkDocGoExists(repoRoot string) []Violation {
	var v []Violation
	pkgs, err := getGoPackages(repoRoot)
	if err != nil {
		v = append(v, Violation{"docgo_missing", "internal/", err.Error()})
		return v
	}
	for _, pkg := range pkgs {
		docPath := filepath.Join(repoRoot, "internal", pkg, "doc.go")
		if _, err := os.Stat(docPath); os.IsNotExist(err) {
			v = append(v, Violation{
				Check:  "docgo_missing",
				File:   "internal/" + pkg + "/",
				Detail: "no doc.go found",
			})
		}
	}
	return v
}

// checkMaturityTags verifies every doc.go has exactly one valid Maturity: tag.
func checkMaturityTags(repoRoot string) []Violation {
	var v []Violation
	pkgs, err := getGoPackages(repoRoot)
	if err != nil {
		return v
	}
	for _, pkg := range pkgs {
		docPath := filepath.Join(repoRoot, "internal", pkg, "doc.go")
		data, err := os.ReadFile(docPath)
		if err != nil {
			v = append(v, Violation{"maturity_missing", docPath, "cannot read doc.go"})
			continue
		}
		lines := strings.Split(string(data), "\n")
		var maturityLines []string
		for _, line := range lines {
			if strings.Contains(line, "Maturity:") {
				maturityLines = append(maturityLines, line)
			}
		}
		relPath := "internal/" + pkg + "/doc.go"
		switch len(maturityLines) {
		case 0:
			v = append(v, Violation{"maturity_missing", relPath, "no Maturity: tag"})
		case 1:
			tier := strings.TrimSpace(strings.TrimPrefix(maturityLines[0], "// Maturity:"))
			if !validTiers[tier] {
				v = append(v, Violation{
					Check:  "maturity_invalid",
					File:   relPath,
					Detail: fmt.Sprintf("invalid Maturity value: '%s' (valid: stable/evolving/experimental/utility)", tier),
				})
			}
		default:
			v = append(v, Violation{
				Check:  "maturity_multiple",
				File:   relPath,
				Detail: fmt.Sprintf("%d Maturity: tags found, expected 1", len(maturityLines)),
			})
		}
	}
	return v
}

// checkMaturityMD verifies MATURITY.md exists with all four tier sections.
func checkMaturityMD(repoRoot string) []Violation {
	var v []Violation
	mdPath := filepath.Join(repoRoot, "internal", "MATURITY.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		v = append(v, Violation{"maturity_md_missing", mdPath, "reference file not found"})
		return v
	}
	data, err := os.ReadFile(mdPath)
	if err != nil {
		v = append(v, Violation{"maturity_md_missing", mdPath, "cannot read MATURITY.md"})
		return v
	}
	content := string(data)
	for _, label := range []string{"S · Stable", "E · Evolving", "X · Experimental", "U · Utility"} {
		if !strings.Contains(content, label) {
			v = append(v, Violation{
				Check:  "maturity_md_structure",
				File:   mdPath,
				Detail: fmt.Sprintf("missing section: %s", label),
			})
		}
	}
	return v
}

// checkCrossConsistency verifies doc.go tags match MATURITY.md entries.
func checkCrossConsistency(repoRoot string) []Violation {
	var v []Violation
	mdPath := filepath.Join(repoRoot, "internal", "MATURITY.md")
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		return v // already reported by checkMaturityMD
	}

	// Build doc.go → tier map
	docTiers := make(map[string]string)
	pkgs, _ := getGoPackages(repoRoot)
	for _, pkg := range pkgs {
		docPath := filepath.Join(repoRoot, "internal", pkg, "doc.go")
		data, err := os.ReadFile(docPath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.Contains(line, "Maturity:") {
				docTiers[pkg] = strings.TrimSpace(strings.TrimPrefix(line, "// Maturity:"))
				break
			}
		}
	}

	// Build MATURITY.md → tier map
	mdTiers := parseMaturityMD(mdPath)

	// doc.go packages must be in MATURITY.md with same tier
	for pkg, docTier := range docTiers {
		mdTier, ok := mdTiers[pkg]
		if !ok {
			v = append(v, Violation{
				Check:  "cross_missing_md",
				File:   "internal/" + pkg + "/",
				Detail: fmt.Sprintf("doc.go tier=%s, not found in MATURITY.md", docTier),
			})
		} else if docTier != mdTier {
			v = append(v, Violation{
				Check:  "cross_mismatch",
				File:   "internal/" + pkg + "/",
				Detail: fmt.Sprintf("doc.go=%s, MATURITY.md=%s", docTier, mdTier),
			})
		}
	}

	// MATURITY.md packages must have a doc.go with same tier
	for pkg, mdTier := range mdTiers {
		docTier, ok := docTiers[pkg]
		if !ok {
			v = append(v, Violation{
				Check:  "cross_orphan_md",
				File:   "internal/" + pkg + "/",
				Detail: fmt.Sprintf("listed in MATURITY.md as %s, but no doc.go", mdTier),
			})
		} else if docTier != mdTier {
			// Already reported above
			_ = docTier
		}
	}

	return v
}

// parseMaturityMD extracts a pkg→tier map from MATURITY.md.
func parseMaturityMD(path string) map[string]string {
	result := make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return result
	}
	defer func() { _ = f.Close() }()

	tierMap := map[string]string{
		"S · Stable":       "stable",
		"E · Evolving":     "evolving",
		"X · Experimental": "experimental",
		"U · Utility":      "utility",
	}

	scanner := bufio.NewScanner(f)
	currentTier := ""
	for scanner.Scan() {
		line := scanner.Text()
		for label, tier := range tierMap {
			if strings.Contains(line, label) {
				currentTier = tier
				break
			}
		}
		// Reset tier on non-package sections
		if strings.Contains(line, "非 Package 目錄") || strings.Contains(line, "Non-Package") {
			currentTier = ""
		}
		if currentTier == "" {
			continue
		}
		// Extract `pkgname` from table row: | `pkgname` | ...
		// Skip entries that look like directories (contain /) — those are non-package references.
		if strings.HasPrefix(strings.TrimSpace(line), "| `") {
			start := strings.Index(line, "`")
			end := strings.Index(line[start+1:], "`")
			if start >= 0 && end >= 0 {
				pkg := line[start+1 : start+1+end]
				if strings.Contains(pkg, "/") {
					continue
				}
				result[pkg] = currentTier
			}
		}
	}
	return result
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func outputText(violations []Violation) {
	checks := map[string]int{"docgo_missing": 0, "maturity_missing": 0, "maturity_multiple": 0, "maturity_invalid": 0, "maturity_md_missing": 0, "maturity_md_structure": 0, "cross_missing_md": 0, "cross_mismatch": 0, "cross_orphan_md": 0}
	totalPassed := 4 // docgo, tags, md, cross
	checkFailed := map[string]bool{}

	for _, v := range violations {
		checks[v.Check]++
		switch v.Check {
		case "docgo_missing":
			checkFailed["docgo"] = true
		case "maturity_missing", "maturity_multiple", "maturity_invalid":
			checkFailed["tags"] = true
		case "maturity_md_missing", "maturity_md_structure":
			checkFailed["md"] = true
		case "cross_missing_md", "cross_mismatch", "cross_orphan_md":
			checkFailed["cross"] = true
		}
	}

	fmt.Println("Atlas Maturity Label Check")
	fmt.Println("===========================")
	fmt.Println()

	// Check 1
	if checkFailed["docgo"] {
		fmt.Printf("❌ Check 1/4: doc.go existence — %d package(s) missing doc.go\n", checks["docgo_missing"])
	} else {
		pkgs, _ := getGoPackages(".")
		fmt.Printf("✅ Check 1/4: doc.go existence — all %d packages have doc.go\n", len(pkgs))
	}

	// Check 2
	failed := checks["maturity_missing"] + checks["maturity_multiple"] + checks["maturity_invalid"]
	if checkFailed["tags"] {
		fmt.Printf("❌ Check 2/4: Maturity tag validity — %d issue(s)\n", failed)
	} else {
		fmt.Println("✅ Check 2/4: Maturity tag validity — all valid")
	}

	// Check 3
	if checkFailed["md"] {
		fmt.Printf("❌ Check 3/4: MATURITY.md — %d issue(s)\n", checks["maturity_md_missing"]+checks["maturity_md_structure"])
	} else {
		fmt.Println("✅ Check 3/4: MATURITY.md — present and structurally complete")
	}

	// Check 4
	failedCross := checks["cross_missing_md"] + checks["cross_mismatch"] + checks["cross_orphan_md"]
	if checkFailed["cross"] {
		fmt.Printf("❌ Check 4/4: doc.go ↔ MATURITY.md consistency — %d mismatch(es)\n", failedCross)
	} else {
		fmt.Println("✅ Check 4/4: doc.go ↔ MATURITY.md consistency — fully aligned")
	}

	if len(violations) > 0 {
		fmt.Println()
		for _, v := range violations {
			fmt.Printf("  • [%s] %s — %s\n", v.Check, v.File, v.Detail)
		}
	}

	passed := totalPassed
	for range checkFailed {
		passed--
	}
	if passed < 0 {
		passed = 0
	}
	fmt.Printf("\n═════════════════════════════\nResult: %d/4 checks passed\n", passed)
	if len(violations) > 0 {
		fmt.Printf("Found %d violation(s)\n\n", len(violations))
		fmt.Println("Fix suggestions:")
		fmt.Println("  1. Missing doc.go → create internal/<pkg>/doc.go with Maturity: <tier>")
		fmt.Println("  2. Invalid/Missing tag → use stable/evolving/experimental/utility")
		fmt.Println("  3. Inconsistency → update MATURITY.md or doc.go to match")
	} else {
		fmt.Println("All checks passed ✅")
	}
}

func outputJSON(violations []Violation) {
	passed := 4
	checkFailed := map[string]bool{}
	for _, v := range violations {
		switch v.Check {
		case "docgo_missing":
			checkFailed["docgo"] = true
		case "maturity_missing", "maturity_multiple", "maturity_invalid":
			checkFailed["tags"] = true
		case "maturity_md_missing", "maturity_md_structure":
			checkFailed["md"] = true
		case "cross_missing_md", "cross_mismatch", "cross_orphan_md":
			checkFailed["cross"] = true
		}
	}
	passed -= len(checkFailed)
	if passed < 0 {
		passed = 0
	}

	out := map[string]any{
		"status":           "passed",
		"total_violations": len(violations),
		"checks_passed":    passed,
		"checks_total":     4,
		"violations":       violations,
	}
	if len(violations) > 0 {
		out["status"] = "violations_found"
	}
	b, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(b))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
