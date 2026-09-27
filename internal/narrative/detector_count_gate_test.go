// Package narrative — detector/template count truth gate.
//
// Background (2026-09-27): the default registry grew from 24 to 29 detectors, but
// code comments, the cmd/atlas startup log and several specs kept saying "24".
// The startup log line leaked that stale number into a downstream atlas-wiki CI
// audit. Hand-written numbers drift; assertions do not. This file turns the claim
// "the docs and the registry agree on how many detectors exist" into two
// executable assertions:
//
//  1. TestDetectorCount_RegistryMatchesDocumentedCount — the registry size must
//     equal documentedDetectorCount, the number the documents state. Add or
//     remove a detector without bumping the constant and this test is red.
//  2. TestDetectorCount_NoStaleCountClaims — every "N detectors" / "N trigger
//     themes" claim inside countClaimCarriers must carry the live registry size,
//     so a comment or spec that keeps a stale number is caught here instead of in
//     someone's audit log.
//
// The cmd/atlas startup log is NOT part of the text gate: it computes the count
// from the registry at runtime (templateDetectorRouteLog in
// cmd/atlas/template_detector.go, asserted in
// cmd/atlas/template_detector_count_log_test.go).
//

package narrative

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// documentedDetectorCount is the number the current-state documents state for
// NewDefaultDetectorRegistry(). It is what a reader is told, so it must equal
// the registry. When a detector is added or removed:
//
//  1. bump this constant;
//  2. fix the count claims in countClaimCarriers — test 2 below names the files
//     that still carry a stale number;
//  3. re-run the narrative package tests.
const documentedDetectorCount = 29

// countClaimCarriers are the files whose "N detectors" / "N trigger themes"
// claims describe the CURRENT default registry. Every such claim must equal the
// live registry size.
//
// Files that intentionally keep a dated number are deliberately not here:
// CHANGELOG.md (historical PR record), docs/decisions/* (dated decision
// records), docs/ATLAS_CONSTITUTION_AUDIT.md (v1.1 audit snapshot rows quoting
// the then-current requirement), docs/specs/macro-first-principles-causal-gap-spec.md
// (change record of the 24 -> 29 move), client_web/tests/capital-causality.spec.ts
// (its mock builds 24 rows on purpose).
var countClaimCarriers = []string{
	"cmd/atlas/template_detector.go",
	"cmd/atlas-mcp/server/tools_template_detector.go",
	"internal/narrative/detector.go",
	"internal/narrative/detector_impls.go",
	"internal/narrative/detector_impls_test.go",
	"internal/narrative/detector_e2e_test.go",
	"internal/narrative/knowledge_base_test.go",
	"internal/narrative/AGENTS.md",
	"internal/eventdriven/predictor.go",
	"internal/eventdriven/type_theme_mapping.go",
	"internal/eventdriven/type_theme_mapping_test.go",
	"internal/orchestrator/regime_inference.go",
	"docs/specs/eventdriven-spec.md",
	"docs/specs/template-detector-category-spec.md",
	"docs/ATLAS_METHODOLOGY.md",
}

// detectorCountClaim matches a phrase that states how many detectors, trigger
// themes or templates the default registry has. The leading digits are the
// claim.
var detectorCountClaim = regexp.MustCompile(
	`(?i)(?:all\s+)?\d+\s*個?\s*(?:template[-\s]trigger\s*)?(?:detectors?|templates?|trigger[_ ]themes?|themes?)\b`)

// unrelatedCountContexts mark numbers that count something else: Wave 9
// detectors, the Stage 5 detector subsystem, "PR#2 shipped 24" history, and
// "＋5 個 detector struct" change records.
var unrelatedCountContexts = []string{"wave", "stage", "pr#", "＋", "新增", "增加"}

// TestDetectorCount_RegistryMatchesDocumentedCount pins the documented number to
// the live registry from four angles, so a partial change cannot slip through.
func TestDetectorCount_RegistryMatchesDocumentedCount(t *testing.T) {
	reg := NewDefaultDetectorRegistry()

	if got := len(reg.List()); got != documentedDetectorCount {
		t.Errorf("len(NewDefaultDetectorRegistry().List()) = %d, documentedDetectorCount = %d — a detector was added or removed without updating the documented count", got, documentedDetectorCount)
	}
	if got := reg.Len(); got != documentedDetectorCount {
		t.Errorf("registry.Len() = %d, documentedDetectorCount = %d", got, documentedDetectorCount)
	}
	if got := len(reg.Themes()); got != documentedDetectorCount {
		t.Errorf("len(registry.Themes()) = %d, documentedDetectorCount = %d", got, documentedDetectorCount)
	}
	if got := len(DefaultTemplates()); got != documentedDetectorCount {
		t.Errorf("len(DefaultTemplates()) = %d, documentedDetectorCount = %d — templates and detectors must move together", got, documentedDetectorCount)
	}
}

// TestDetectorCount_NoStaleCountClaims fails when a file in countClaimCarriers
// states a detector/theme/template count that the registry does not have.
func TestDetectorCount_NoStaleCountClaims(t *testing.T) {
	live := len(NewDefaultDetectorRegistry().List())
	root := repoRootDir(t)

	for _, rel := range countClaimCarriers {
		path := filepath.Join(root, rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("count-claim carrier %s is unreadable (%v) — if the file moved, update countClaimCarriers", rel, err)
			continue
		}

		for i, line := range strings.Split(string(raw), "\n") {
			for _, loc := range detectorCountClaim.FindAllStringIndex(line, -1) {
				claim := line[loc[0]:loc[1]]
				if hasUnrelatedCountContext(line[:loc[0]]) || precededByFractionOrDigit(line[:loc[0]]) {
					continue
				}
				got, convErr := firstInt(claim)
				if convErr != nil {
					t.Errorf("%s:%d claim %q has no leading number: %v", rel, i+1, claim, convErr)
					continue
				}
				if got != live {
					t.Errorf("%s:%d states %q but the registry has %d detectors — fix the file (or the registry)", rel, i+1, strings.TrimSpace(claim), live)
				}
			}
		}
	}
}

// hasUnrelatedCountContext reports whether the text before a claim names a
// different detector family or a change record.
func hasUnrelatedCountContext(before string) bool {
	window := strings.ToLower(before)
	if len(window) > 40 {
		window = window[len(window)-40:]
	}
	for _, marker := range unrelatedCountContexts {
		if strings.Contains(window, marker) {
			return true
		}
	}
	return false
}

// precededByFractionOrDigit reports whether a claim is part of a fraction label
// such as "Narrative 19/24 themes", which names a requirement, not the registry
// size of today.
func precededByFractionOrDigit(before string) bool {
	trimmed := strings.TrimRight(before, " \t")
	if len(trimmed) == 0 {
		return false
	}
	last := trimmed[len(trimmed)-1]
	if last >= '0' && last <= '9' {
		return true
	}
	// "19/24 themes" names a requirement label, not today's registry size.
	return last == '/' && len(trimmed) >= 2 && trimmed[len(trimmed)-2] >= '0' && trimmed[len(trimmed)-2] <= '9'
}

// firstInt returns the first decimal number in claim.
func firstInt(claim string) (int, error) {
	idx := strings.IndexFunc(claim, func(r rune) bool { return r >= '0' && r <= '9' })
	if idx < 0 {
		return 0, strconv.ErrSyntax
	}
	end := idx
	for end < len(claim) && claim[end] >= '0' && claim[end] <= '9' {
		end++
	}
	return strconv.Atoi(claim[idx:end])
}

// repoRootDir locates the repository root from this test file, falling back to
// the working directory walk (robust under -trimpath).
func repoRootDir(t *testing.T) string {
	t.Helper()

	candidates := []string{}
	if _, thisFile, _, ok := runtime.Caller(0); ok {
		candidates = append(candidates, filepath.Join(filepath.Dir(thisFile), "..", ".."))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "..", ".."))
	}
	for _, cand := range candidates {
		if _, err := os.Stat(filepath.Join(cand, "go.mod")); err == nil {
			abs, absErr := filepath.Abs(cand)
			if absErr == nil {
				return abs
			}
			return cand
		}
	}
	t.Fatalf("cannot locate the repository root from this test file; tried %v", candidates)
	return ""
}
