// Package narrative — detector/template count truth gate.
//
// Background (2026-09-27): the default registry grew from 24 to 29 detectors, but
// code comments, the cmd/atlas startup log and several specs kept saying "24".
// The startup log line leaked that stale number into a downstream atlas-wiki CI
// audit. Hand-written numbers drift; assertions do not. This file turns the claim
// "the docs and the registry agree on how many detectors exist" into five
// executable assertions:
//
//  1. TestDetectorCount_RegistryMatchesDocumentedCount — the registry size must
//     equal documentedDetectorCount, the number the documents state.
//  2. TestDetectorCount_NoStaleCountClaims — every detector/theme/template count
//     claim inside countClaimCarriers must carry the live registry size.
//  3. TestDetectorCount_LegacyAllowlistIsNarrow — the legacy allowlist is
//     file-scoped and fragment-exact: it cannot excuse a current-state claim.
//  4. TestDetectorCount_ClaimFilesAreClassified — every file in the packages that
//     own the registry and the startup log that states a count must be either a
//     carrier or an explicitly listed exception, so a new unclassified claimant
//     (how cmd/atlas/main.go was missed the first time) fails the build instead
//     of hiding.
//  5. TestDetectorCount_ClaimClassifier_Contract — pins the pattern layer in both
//     directions, including the forms it deliberately does not cover.
//
// The cmd/atlas startup log is covered twice: cmd/atlas/main.go is a carrier (a
// re-hardcoded literal there is a stale claim), and
// cmd/atlas/template_detector_count_log_test.go asserts the emitted line follows
// the registry.
//
// Design note — why there is no "it looks historical, skip it" heuristic:
// the first version of this gate skipped a claim when a legacy marker (Wave,
// Stage, PR#) appeared within N characters. An independent adversarial review
// walked straight through it with
//
//	// Stage 5 detector registry 現況：24 detectors 全啟用。
//
// which is a *current-state* claim wearing a historical marker. Heuristics that
// silently skip text are how "24" survived five detector additions. What is
// skipped now is only what is written down in legacyCountLines, per file.
//
// Known blind spots of the pattern layer — deliberate, documented, not accidental:
//   - a claim only counts when the number and the keyword sit together on ONE
//     line ("detectors\ntotal: 24" is invisible);
//   - spelled-out numbers ("twenty-four detectors") are invisible;
//   - numbers separated from the keyword by other words ("24 of the templates")
//     are invisible;
//   - files outside countClaimCarriers and outside ownerPackageRoots are not
//     scanned. When you add a file that states the count, add it to
//     countClaimCarriers.
package narrative

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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
// describe the CURRENT default registry. Every non-allowlisted claim in them
// must equal the live registry size.
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
	"internal/narrative/AGENTS.md",
	"internal/narrative/detector.go",
	"internal/narrative/detector_impls.go",
	"internal/narrative/detector_impls_test.go",
	"internal/narrative/detector_e2e_test.go",
	"internal/narrative/knowledge_base_test.go",
	"internal/narrative/narrative_test.go",
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

// legacyCountLines lists, per carrier, the exact line fragments whose numbers
// count something other than the default registry: Wave 9 detectors, the Stage 5
// detector subsystem, PR#2 history, and the constitution requirement label
// "19/24 themes". A line is exempt only in the file it is listed under — the same
// sentence in another file is judged.
//
// Adding an entry is a deliberate act. If the gate flags a legitimate line, add
// its fragment here and say why; do not reintroduce a heuristic.
var legacyCountLines = map[string][]string{
	"cmd/atlas/main.go": {
		"Wave 9 detector",
	},
	"cmd/atlas/template_detector.go": {
		"Stage 5 detector",
	},
	"internal/narrative/detector_impls_test.go": {
		"Stage 5 PR#2 detector_impls_test.go",
	},
	"internal/narrative/detector_e2e_test.go": {
		"歷史 stage5 detector plan",
	},
	"docs/specs/eventdriven-spec.md": {
		"Stage 5 起 detector",
		"Stage 5 完整規劃",
	},
	"docs/ATLAS_METHODOLOGY.md": {
		"Narrative 19/24 themes",
	},
}

// ownerPackageRoots are the package trees that own the default registry, the
// module document of record and the cmd/atlas startup log. Test 4 requires every
// file below them that states a count to be classified.
//
// The tree is deliberately narrow: widening it repo-wide would mean classifying
// ~40 files about unrelated detectors (Wave 9, drift, ledger), which is noise,
// not safety. Files outside these trees still need an explicit entry in
// countClaimCarriers when they state the registry count.
var ownerPackageRoots = []string{"cmd/atlas/", "internal/narrative/"}

// unclassifiedClaimFiles lists files under ownerPackageRoots that state a count
// which is NOT the default registry, with the reason. Every other claimant file
// there must be a countClaimCarrier.
var unclassifiedClaimFiles = map[string]string{
	"cmd/atlas/template_detector_count_log_test.go":        "asserts that an EMPTY registry reports 0 detectors, so the number is deliberately not the registry size",
	"internal/narrative/detector_count_gate_test.go":       "this gate — it quotes counts on purpose, including the stale ones it must catch",
	"internal/narrative/detector_test.go":                  "counts the stub detectors the test builds locally (4), not the default registry",
	"internal/narrative/frontend_theme_label_sync_test.go": "quotes the stale frontend numbers (24 / 26) it was written to replace",
	"internal/narrative/ingestor.go":                       "counts the single detector an ingestion path wires",
	"internal/narrative/knowledge_base_api_test.go":        "18 = InvestmentModel sector coverage, not detectors",
	"internal/narrative/narrative_detectors.go":            "counts the single detector a KB pipeline path wires",
}

// forwardClaimPatterns match a phrase where the NUMBER LEADS: "24 detectors",
// "29 個 detector", "all 24 themes", "29-template", "26 個主題", "(24) detectors".
var forwardClaimPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\d+\)?\s*個?\s*(?:template[-\s]trigger\s*)?(?:detectors?|templates?|trigger[_ ]themes?|themes?|偵測器|模板|主題)`),
	regexp.MustCompile(`(?i)\d+-(?:detectors?|templates?)`),
}

// reverseClaimPatterns match a phrase where the NUMBER TRAILS: "detector
// count: 24", "number of detectors = 24", "detector 總數 24". The \b before an
// ASCII keyword keeps Go identifiers such as themeCount out of the net; CJK
// keywords get their own pattern because ASCII word boundaries do not exist
// next to them.
var reverseClaimPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)number\s+of\s+detectors?\b[^0-9\n]{0,16}\d+`),
	regexp.MustCompile(`(?i)(?:detector|template|theme)s?\s*\bcount\b[^0-9\n]{0,16}\d+`),
	regexp.MustCompile(`(?:detector|template|theme|偵測器|模板|主題)s?\s*(?:總數|數量)[^0-9\n]{0,16}\d+`),
	regexp.MustCompile(`(?i)(?:detectors?|templates?|themes?|偵測器|模板|主題)\s*(?:總數|數量|數|count)?\s*[:=：]\s*\d+`),
}

// countClaim is one count statement the pattern layer found.
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

// staleClaimsIn reports the claims in one file's content that must equal the
// registry size but do not.
func staleClaimsIn(rel, content string, live int) []string {
	var out []string
	for i, line := range strings.Split(content, "\n") {
		if legacyLineAllowed(rel, line) {
			continue
		}
		for _, claim := range rawCountClaims(line) {
			if claim.number != live {
				out = append(out, rel+":"+strconv.Itoa(i+1)+" states "+strconv.Quote(strings.TrimSpace(claim.text))+
					" but the registry has "+strconv.Itoa(live)+" detectors — fix the file (or the registry)")
			}
		}
	}
	return out
}

// TestDetectorCount_NoStaleCountClaims fails when a file in countClaimCarriers
// states a detector/theme/template count that the registry does not have.
func TestDetectorCount_NoStaleCountClaims(t *testing.T) {
	live := len(NewDefaultDetectorRegistry().List())
	root := repoRootDir(t)

	for _, rel := range countClaimCarriers {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("count-claim carrier %s is unreadable (%v) — if the file moved, update countClaimCarriers", rel, err)
			continue
		}
		for _, msg := range staleClaimsIn(rel, string(raw), live) {
			t.Error(msg)
		}
	}
}

// TestDetectorCount_LegacyAllowlistIsNarrow guarantees that the allowlist cannot
// be used as cover for a current-state claim: the exempt fragment must be present
// in the file it is listed under, and the classic escape sentence — a stale
// current-state claim wearing a historical marker — must still be reported.
func TestDetectorCount_LegacyAllowlistIsNarrow(t *testing.T) {
	live := len(NewDefaultDetectorRegistry().List())

	// Every listed fragment must actually appear in its file, otherwise the entry
	// has rotted into a blanket exemption nobody notices.
	root := repoRootDir(t)
	for rel, fragments := range legacyCountLines {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("legacyCountLines lists %s, which cannot be read: %v", rel, err)
			continue
		}
		for _, fragment := range fragments {
			if !strings.Contains(string(raw), fragment) {
				t.Errorf("legacyCountLines[%s] exempts %q, but that text is gone — delete the stale entry", rel, fragment)
			}
		}
	}

	// The escape an adversarial review demonstrated: a marker in the sentence must
	// not exempt a current-state claim.
	for _, tc := range []struct {
		rel  string
		line string
	}{
		{"cmd/atlas/main.go", "// Stage 5 detector registry 現況：24 detectors 全啟用。"},
		{"cmd/atlas/main.go", "// Wave 12：24 detectors 已註冊。"},
		{"cmd/atlas/main.go", "// PR#2099：24 detectors 已註冊。"},
		{"internal/narrative/detector.go", "// Stage 5 detector registry 現況：24 detectors 全啟用。"},
	} {
		got := strings.Join(staleClaimsIn(tc.rel, tc.line, live), " | ")
		if !strings.Contains(got, `"24 detectors"`) {
			t.Errorf("%s: %q produced %q, want a report of the stale 24-detector claim — a legacy marker must not excuse a current-state claim",
				tc.rel, tc.line, got)
		}
	}

	// The allowlist is file-scoped: the exempted fragment under its own file is
	// not exempt anywhere else.
	exemptLine := "// but in runLiveTrading every Wave 9 detector (and the rest of the live"
	if got := staleClaimsIn("cmd/atlas/main.go", exemptLine, live); len(got) != 0 {
		t.Errorf("allowlisted line in its own file reported %v, want none", got)
	}
	if got := staleClaimsIn("internal/narrative/detector.go", exemptLine, live); len(got) == 0 {
		t.Error("an allowlisted fragment must not exempt the same line in another file")
	}
}

// TestDetectorCount_ClaimFilesAreClassified fails when a file under
// ownerPackageRoots states a count but is neither a countClaimCarrier nor listed
// in unclassifiedClaimFiles.
//
// This is the assertion that makes the omission which hid the original bug
// impossible to repeat silently: cmd/atlas/main.go emitted the stale count and
// was not a carrier, so nothing looked at it.
func TestDetectorCount_ClaimFilesAreClassified(t *testing.T) {
	root := repoRootDir(t)

	carriers := make(map[string]bool, len(countClaimCarriers))
	for _, rel := range countClaimCarriers {
		carriers[rel] = true
	}

	claimants := map[string]int{}
	for _, tree := range ownerPackageRoots {
		err := filepath.WalkDir(filepath.Join(root, tree), func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if ext := filepath.Ext(path); ext != ".go" && ext != ".md" {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			rel = filepath.ToSlash(rel)
			n := 0
			for _, line := range strings.Split(string(raw), "\n") {
				if legacyLineAllowed(rel, line) {
					continue
				}
				n += len(rawCountClaims(line))
			}
			if n > 0 {
				claimants[rel] = n
			}
			return nil
		})
		if err != nil {
			t.Fatalf("cannot walk %s: %v", tree, err)
		}
	}

	if len(claimants) == 0 {
		t.Fatalf("no count claimants found under %v — the walk is broken and this gate checks nothing", ownerPackageRoots)
	}

	for rel, n := range claimants {
		if carriers[rel] {
			continue
		}
		if _, ok := unclassifiedClaimFiles[rel]; ok {
			continue
		}
		t.Errorf("%s states %d detector/theme/template count(s) but is classified nowhere — add it to countClaimCarriers (the claims must equal %d) or to unclassifiedClaimFiles with a reason",
			rel, n, documentedDetectorCount)
	}

	// Exceptions must not rot: every listed file must still exist and still claim.
	for rel := range unclassifiedClaimFiles {
		if _, ok := claimants[rel]; !ok {
			t.Errorf("unclassifiedClaimFiles lists %s, which no longer states any count — remove the exception", rel)
		}
	}
}

// TestDetectorCount_ClaimClassifier_Contract pins the pattern layer in both
// directions. The "caught" cases include escapes an independent adversarial
// review of the first version of this gate actually demonstrated; the "blind
// spot" cases are forms the patterns deliberately do not cover, written down so
// nobody has to guess.
//
// If you change the patterns: fix this table first (it encodes the intent), then
// the carrier files.
func TestDetectorCount_ClaimClassifier_Contract(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []int
	}{
		// Must be caught — these all describe the current registry.
		{"english plural", "//\tGET /api/detector/registry/list → 24 detectors + enable/disable", []int{24}},
		{"all N themes", "// used (all 24 themes registered)", []int{24}},
		{"hyphenated template", "// the full 29-template set (replacing the legacy 5-theme subset that", []int{29}},
		{"hyphenated detector", "// so the 24-detector scan reports the window", []int{24}},
		{"chinese 個 detector", "// 註冊所有 24 個 detector", []int{24}},
		{"chinese 偵測器", "// 共 24 個偵測器", []int{24}},
		{"chinese 主題", "// 與 templates.go 24 個主題同步", []int{24}},
		{"parenthesised", "// (24) detectors are registered", []int{24}},
		{"fullwidth digits", "// 共 ２４ 個 detector", []int{24}},
		{"reverse count", "// detector count: 24", []int{24}},
		{"reverse number of", "// Number of detectors = 24", []int{24}},
		{"reverse chinese", "// detector 總數 24", []int{24}},
		{"colon form", "// total detectors: 24", []int{24}},
		{"colon form short", "// detectors = 24", []int{24}},
		{"colon form chinese", "// 偵測器：24", []int{24}},
		{"colon form chinese keyword", "// detector 數: 24", []int{24}},
		{"chinese total form", "// 偵測器總數 24", []int{24}},
		{"chinese total form 2", "// 模板總數：24", []int{24}},
		{"chinese quantity form", "// 主題數量 24", []int{24}},
		{"current claim wearing a marker", "// Stage 5 detector registry 現況：24 detectors 全啟用。", []int{5, 24}},
		{"correct number is also a claim", "// registers all 29 detectors, default-enabled.", []int{29}},

		// Deliberately not claims (no allowance needed).
		{"identifier named themeCount", "if themeCount[theme] != 1 {", nil},
		{"legacy theme subset", "// the legacy 5-theme subset that", nil},
		{"no number at all", "// the registry decides how many detectors exist", nil},
		{"files and line numbers", "internal/narrative/detector.go:124 and 2026-09-27", nil},

		// Documented blind spots — if you make one of these work, move the row up.
		{"blind spot: spelled-out number", "// twenty-four detectors ship by default", nil},
		{"blind spot: number split from keyword", "// 24 of the templates are KB-backed", nil},
		{"subset phrasing is judged too", "// 21 個 detector themes today", []int{21}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rawCountClaims(tc.line)
			gotNums := make([]int, 0, len(got))
			for _, c := range got {
				gotNums = append(gotNums, c.number)
			}
			if len(gotNums) != len(tc.want) {
				t.Fatalf("rawCountClaims(%q) = %v, want %v", tc.line, gotNums, tc.want)
			}
			for i := range gotNums {
				if gotNums[i] != tc.want[i] {
					t.Fatalf("rawCountClaims(%q) = %v, want %v", tc.line, gotNums, tc.want)
				}
			}
		})
	}
}

// rawCountClaims returns every count claim in one line. It applies patterns only
// — no "looks historical" heuristic; exemption is decided by
// legacyLineAllowed, which reads the explicit per-file list.
func rawCountClaims(line string) []countClaim {
	line = normalizeDigits(line)

	type span struct {
		start, end, number int
	}
	var spans []span

	for _, pat := range forwardClaimPatterns {
		for _, loc := range pat.FindAllStringIndex(line, -1) {
			if n, ok := nthInt(line[loc[0]:loc[1]], 0); ok {
				spans = append(spans, span{loc[0], loc[1], n})
			}
		}
	}
	for _, pat := range reverseClaimPatterns {
		for _, loc := range pat.FindAllStringIndex(line, -1) {
			nums := intsIn(line[loc[0]:loc[1]])
			if len(nums) == 0 {
				continue
			}
			spans = append(spans, span{loc[0], loc[1], nums[len(nums)-1]})
		}
	}

	// Several patterns can describe the same statement ("detector count: 24" is
	// both a count-phrase and a colon form). Keep the widest span per position so
	// one statement is reported once.
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end > spans[j].end
	})

	var out []countClaim
	lastEnd := -1
	for _, s := range spans {
		if s.start < lastEnd {
			continue
		}
		out = append(out, countClaim{number: s.number, text: line[s.start:s.end]})
		lastEnd = s.end
	}
	return out
}

// legacyLineAllowed reports whether a line is exempt in that specific file.
func legacyLineAllowed(rel, line string) bool {
	for _, fragment := range legacyCountLines[rel] {
		if strings.Contains(line, fragment) {
			return true
		}
	}
	return false
}

// normalizeDigits rewrites fullwidth digits (２４) to ASCII so the patterns see
// the same number a human reads.
func normalizeDigits(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r >= '０' && r <= '９' }) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return '0' + (r - '０')
		}
		return r
	}, s)
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
