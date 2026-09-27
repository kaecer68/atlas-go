// Package narrative — detector/template count truth gate.
//
// Background (2026-09-27): the default registry grew from 24 to 29 detectors, but
// code comments, the cmd/atlas startup log and several specs kept saying "24".
// The startup log line leaked that stale number into a downstream atlas-wiki CI
// audit. Hand-written numbers drift; assertions do not. This file turns the claim
// "the docs and the registry agree on how many detectors exist" into three
// executable assertions:
//
//  1. TestDetectorCount_RegistryMatchesDocumentedCount — the registry size must
//     equal documentedDetectorCount, the number the documents state. Add or
//     remove a detector without bumping the constant and this test is red.
//  2. TestDetectorCount_NoStaleCountClaims — every detector/theme/template count
//     claim inside countClaimCarriers must carry the live registry size, so a
//     comment or spec that keeps a stale number is caught here instead of in
//     someone's audit log.
//  3. TestDetectorCount_ClaimClassifier_Contract — pins what the text gate
//     catches and what it deliberately lets through, so a future "simplification"
//     of the classifier cannot silently reopen a hole.
//
// The cmd/atlas startup log is covered twice: cmd/atlas/main.go is in
// countClaimCarriers (a re-hardcoded literal there is a stale claim), and
// cmd/atlas/template_detector_count_log_test.go asserts that the emitted line
// follows the registry.
//
// Known blind spots of the text gate — deliberate, documented, not accidental:
//   - a claim only counts when the number and the keyword sit together on ONE
//     line ("detectors\ntotal: 24" is invisible);
//   - spelled-out numbers ("twenty-four detectors") are invisible;
//   - numbers separated from the keyword by other words ("24 of the templates")
//     are invisible;
//   - files outside countClaimCarriers are not scanned at all. When you add a
//     file that states the count, add it to countClaimCarriers.
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

// countClaimCarriers are the files whose detector/theme/template count claims
// describe the CURRENT default registry. Every such claim must equal the live
// registry size.
//
// Files that intentionally keep a dated number are deliberately not here:
// CHANGELOG.md (historical PR record), docs/decisions/* (dated decision
// records), docs/ATLAS_CONSTITUTION_AUDIT.md (v1.1 audit snapshot rows quoting
// the then-current requirement label), docs/specs/macro-first-principles-causal-gap-spec.md
// (change record of the 24 -> 29 move), docs/specs/industry-allocation-inert-audit-20260924.md
// (dated audit), client_web/tests/capital-causality.spec.ts (its mock builds 24
// rows on purpose).
var countClaimCarriers = []string{
	"cmd/atlas/main.go",
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
	"shared_web/static/js/shared/theme-labels.js",
	"shared_web/static/js/shared/constants.js",
	"docs/specs/eventdriven-spec.md",
	"docs/specs/template-detector-category-spec.md",
	"docs/ATLAS_METHODOLOGY.md",
}

// forwardClaimPatterns match a phrase where the NUMBER LEADS: "24 detectors",
// "29 個 detector", "all 24 themes", "29-template", "26 個主題".
var forwardClaimPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\d+\s*個?\s*(?:template[-\s]trigger\s*)?(?:detectors?|templates?|trigger[_ ]themes?|themes?|偵測器|模板|主題)`),
	regexp.MustCompile(`(?i)\d+-(?:detectors?|templates?)`),
}

// reverseClaimPatterns match a phrase where the NUMBER TRAILS: "detector
// count: 24", "number of detectors = 24", "detector 總數 24".
var reverseClaimPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:number\s+of\s+detectors?|(?:detector|template|theme)s?\s*(?:count|總數|數量))[^0-9\n]{0,16}\d+`),
}

// legacyCountContexts mark a number that counts something else: Wave 9
// detectors, the Stage 5 detector subsystem, "PR#2 shipped N" history, and
// "＋N 個 detector struct" change records. A marker only counts when it sits
// within legacyContextWindow characters of the number — an adversarial review
// found that a wide window let a current-state claim hide behind a distant
// "Stage 5" mention.
var legacyCountContexts = []string{"wave", "stage", "pr#", "＋", "新增", "增加"}

// legacyContextWindow is how close a legacyCountContexts marker must be.
const legacyContextWindow = 16

// countClaim is one count statement the gate judges.
type countClaim struct {
	number int
	text   string
}

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
			for _, claim := range staleCountClaims(line) {
				if claim.number != live {
					t.Errorf("%s:%d states %q but the registry has %d detectors — fix the file (or the registry)", rel, i+1, claim.text, live)
				}
			}
		}
	}
}

// staleCountClaims returns the count claims in one line that describe the
// default registry. Numbers that belong to another detector family (Wave 9, the
// Stage 5 subsystem), to a dated change record, or to a requirement label like
// "19/24 themes" are not claims and are dropped.
func staleCountClaims(line string) []countClaim {
	var out []countClaim

	for _, pat := range forwardClaimPatterns {
		for _, loc := range pat.FindAllStringIndex(line, -1) {
			if !judgeable(line, loc[0]) {
				continue
			}
			if n, ok := nthInt(line[loc[0]:loc[1]], 0); ok {
				out = append(out, countClaim{number: n, text: line[loc[0]:loc[1]]})
			}
		}
	}

	for _, pat := range reverseClaimPatterns {
		for _, loc := range pat.FindAllStringIndex(line, -1) {
			if !judgeable(line, loc[0]) {
				continue
			}
			nums := intsIn(line[loc[0]:loc[1]])
			if len(nums) == 0 {
				continue
			}
			out = append(out, countClaim{number: nums[len(nums)-1], text: line[loc[0]:loc[1]]})
		}
	}

	return out
}

// judgeable reports whether a claim starting at start describes the default
// registry: not preceded nearby by a legacy marker, and not part of a fraction
// label such as "Narrative 19/24 themes".
func judgeable(line string, start int) bool {
	before := strings.ToLower(line[:start])

	window := before
	if len(window) > legacyContextWindow {
		window = window[len(window)-legacyContextWindow:]
	}
	for _, marker := range legacyCountContexts {
		if strings.Contains(window, marker) {
			return false
		}
	}

	return !precededByFractionOrDigit(line[:start])
}

// precededByFractionOrDigit reports whether a number is part of a fraction label
// ("19/24 themes") or continues a longer number. Both name something other than
// the registry size of today.
func precededByFractionOrDigit(before string) bool {
	trimmed := strings.TrimRight(before, " \t")
	if len(trimmed) == 0 {
		return false
	}
	last := trimmed[len(trimmed)-1]
	if last >= '0' && last <= '9' {
		return true
	}
	return last == '/' && len(trimmed) >= 2 && isASCIIDigit(trimmed[len(trimmed)-2])
}

// intsIn returns every decimal number in s, in order.
func intsIn(s string) []int {
	var out []int
	for i := 0; i < len(s); i++ {
		if !isASCIIDigit(s[i]) {
			continue
		}
		end := i
		for end < len(s) && isASCIIDigit(s[end]) {
			end++
		}
		if n, err := strconv.Atoi(s[i:end]); err == nil {
			out = append(out, n)
		}
		i = end
	}
	return out
}

// nthInt returns the idx-th decimal number in s.
func nthInt(s string, idx int) (int, bool) {
	nums := intsIn(s)
	if idx < 0 || idx >= len(nums) {
		return 0, false
	}
	return nums[idx], true
}

func isASCIIDigit(b byte) bool { return b >= '0' && b <= '9' }

// TestDetectorCount_ClaimClassifier_Contract pins the text gate's behaviour in
// both directions. The "caught" cases include the escapes an independent
// adversarial review of the first version of this gate actually demonstrated;
// the "skipped" cases are the legacy forms that must NOT fail the build.
//
// If you change the patterns or the legacy window: fix this table first (it
// encodes the intent), then the carrier files.
func TestDetectorCount_ClaimClassifier_Contract(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []int
	}{
		// Must be caught — these all describe the current registry.
		{"english plural", "//\tGET /api/detector/registry/list → 24 detectors + enable/disable", []int{24}},
		{"all N themes", "// used (all 24 themes registered)", []int{24}},
		{"hyphenated template", "// the full 24-template set", []int{24}},
		{"hyphenated detector", "// so the 24-detector scan reports the window", []int{24}},
		{"chinese 個 detector", "// 註冊所有 24 個 detector", []int{24}},
		{"chinese 偵測器", "// 共 24 個偵測器", []int{24}},
		{"chinese 主題", "// 與 templates.go 24 個主題同步", []int{24}},
		{"reverse count", "// detector count: 24", []int{24}},
		{"reverse number of", "// Number of detectors = 24", []int{24}},
		{"reverse chinese", "// detector 總數 24", []int{24}},
		{"distant stage mention does not excuse a current claim", "// Stage 5 detector registry 現況：24 detectors 全啟用。", []int{24}},
		{"distant PR mention does not excuse a current claim", "// PR#2 之後，本系統目前共有 24 detectors 全啟用。", []int{24}},
		{"correct number is also a claim", "// registers all 29 detectors, default-enabled.", []int{29}},

		// Must be skipped — other detector families, change records, labels.
		{"wave 9 detectors", "// but in runLiveTrading every Wave 9 detector (and the rest of the live", nil},
		{"stage 5 subsystem", "// Exposes two read-only HTTP endpoints that proxy the Stage 5 detector", nil},
		{"pr 2 history", "// PR#2 shipped 24 detectors and 23 unit tests.", nil},
		{"plus-N change record", "// — ＋5 個 detector struct + NewDefaultDetectorRegistry()", nil},
		{"requirement fraction label", "| D4 | Narrative 19/24 themes 進入 regime inference | ✅ | #1372 |", nil},
		{"legacy theme subset is not a registry claim", "// the full set (replacing the legacy 5-theme subset that", nil},
		{"no number at all", "// the registry decides how many detectors exist", nil},
		{"spelled-out number (documented blind spot)", "// twenty-four detectors ship by default", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := staleCountClaims(tc.line)
			gotNums := make([]int, 0, len(got))
			for _, c := range got {
				gotNums = append(gotNums, c.number)
			}
			if len(gotNums) != len(tc.want) {
				t.Fatalf("staleCountClaims(%q) = %v, want %v", tc.line, gotNums, tc.want)
			}
			for i := range gotNums {
				if gotNums[i] != tc.want[i] {
					t.Fatalf("staleCountClaims(%q) = %v, want %v", tc.line, gotNums, tc.want)
				}
			}
		})
	}
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
			if abs, absErr := filepath.Abs(cand); absErr == nil {
				return abs
			}
			return cand
		}
	}
	t.Fatalf("cannot locate the repository root from this test file; tried %v", candidates)
	return ""
}
