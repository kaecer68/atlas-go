package narrative

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The two frontend maps below claim (in comments) to mirror Go sources. A
// comment cannot keep itself true, so these tests compare the actual key sets:
//
//   - shared_web/static/js/shared/theme-labels.js  THEME_LABELS     == DefaultTemplates() themes
//   - shared_web/static/js/shared/constants.js     NARRATIVE_THEME_LABELS == DefaultThemeDurations() keys
//
// Both files used to carry a stale hand-written count ("24 個主題" while 29
// templates existed, "26 theme codes" while 31 codes existed). Add a theme on
// either side and the mismatch names the file to fix.

// jsMapKeys returns the top-level keys of `export const <name> = { ... }` in a
// JS module. It fails loudly when the map cannot be found, so a renamed map
// cannot turn these tests into silent no-ops.
func jsMapKeys(t *testing.T, relPath, name string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(repoRootDir(t), relPath))
	if err != nil {
		t.Fatalf("%s is unreadable: %v", relPath, err)
	}
	src := string(raw)

	header := regexp.MustCompile(`(?m)^\s*export\s+const\s+` + regexp.QuoteMeta(name) + `\s*=\s*\{`)
	loc := header.FindStringIndex(src)
	if loc == nil {
		t.Fatalf("%s: could not find `export const %s = {` — if the map moved or was renamed, update this test", relPath, name)
	}

	start := loc[1] - 1
	depth := 0
	end := -1
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatalf("%s: unbalanced braces while scanning %s", relPath, name)
	}

	body := src[start : end+1]
	keys := regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:`).FindAllStringSubmatch(body, -1)

	out := make([]string, 0, len(keys))
	for _, m := range keys {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatalf("%s: %s parsed as empty — the key syntax changed and this gate stopped checking anything", relPath, name)
	}
	return out
}

func themeSet(detectors []Detector) []string {
	out := make([]string, 0, len(detectors))
	for _, d := range detectors {
		out = append(out, d.Theme())
	}
	sort.Strings(out)
	return out
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func diffSets(got, want []string) string {
	g, w := map[string]bool{}, map[string]bool{}
	for _, s := range got {
		g[s] = true
	}
	for _, s := range want {
		w[s] = true
	}
	var onlyGot, onlyWant []string
	for s := range g {
		if !w[s] {
			onlyGot = append(onlyGot, s)
		}
	}
	for s := range w {
		if !g[s] {
			onlyWant = append(onlyWant, s)
		}
	}
	sort.Strings(onlyGot)
	sort.Strings(onlyWant)
	return "in frontend only: " + strings.Join(onlyGot, ",") + " | in Go only: " + strings.Join(onlyWant, ",")
}

func TestFrontendThemeLabels_MatchDefaultRegistryThemes(t *testing.T) {
	const rel = "shared_web/static/js/shared/theme-labels.js"

	got := sortedCopy(jsMapKeys(t, rel, "THEME_LABELS"))
	want := themeSet(NewDefaultDetectorRegistry().List())

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s THEME_LABELS no longer mirrors NewDefaultDetectorRegistry() (%d vs %d keys): %s", rel, len(got), len(want), diffSets(got, want))
	}
}

func TestFrontendNarrativeThemeLabels_MatchLifecycleDurations(t *testing.T) {
	const rel = "shared_web/static/js/shared/constants.js"

	got := sortedCopy(jsMapKeys(t, rel, "NARRATIVE_THEME_LABELS"))
	want := make([]string, 0)
	for theme := range DefaultThemeDurations() {
		want = append(want, theme)
	}
	sort.Strings(want)

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s NARRATIVE_THEME_LABELS no longer mirrors DefaultThemeDurations() (%d vs %d keys): %s", rel, len(got), len(want), diffSets(got, want))
	}
}
