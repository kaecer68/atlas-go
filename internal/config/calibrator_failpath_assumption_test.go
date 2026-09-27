package config

import (
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// ASSUMPTION GUARD — not a behaviour test.
//
// Claim it keeps executable (issue #1944 Batch A, PR #2077): the
// `case failedCount > 0` arm of calibratorVerdict — "nothing was written because
// every SetParameter failed ⇒ Verdict=failed" — cannot be reached through
// CalibrateParameters. That is why the arm is covered by the pure-function table
// test in calibrator_verdict_test.go instead of an end-to-end run.
//
// The claim rests on one property of the inference engine: every name the
// calibrate loop can write is a name SetParameter accepts.
//
//  1. GetParameter resolves a name iff it is a parameterTable entry, or it
//     matches a map-parameter prefix AND that sub-key already exists.
//  2. SetParameter rejects a name iff it is neither. So get-ok implies set-ok
//     exactly as long as both handlers recognise the same prefix set.
//  3. A name that resolves in neither is skipped by the loop
//     (`if !ok { continue }`) and never counted as a failure, while an
//     unresolvable name aborts earlier inside the optimizer
//     ("calibrate: optimize: unknown parameter: X") before any report exists.
//
// IF THIS TEST GOES RED, THE ASSUMPTION HAS BROKEN and the Batch A coverage
// decision no longer holds. Then you must:
//   - add a real end-to-end test that drives CalibrateParameters into the
//     failed-write path (the pure-function table test is no longer sufficient
//     coverage for it), and
//   - update docs/reference/inert-registry.md §Batch A 收尾, which currently
//     records that arm as "實測否證：目前不可達".
//
// Do not "fix" a red run by relaxing or deleting this guard: the assumption it
// guards is the reason the coverage decision was sound, so a red run is new
// information about the engine, not a false alarm.
//
// Every source here is live, never a copied list: scalar names come from the
// engine's own parameterTable, map sub-keys from the shipped config data, and
// the map-prefix list from the handler source file itself (so a newly added
// prefix cannot slip past the guard).
func TestCalibratorFailPath_UnreachableAssumptionStillHolds(t *testing.T) {
	ie := NewInferenceEngine(DefaultParametersConfig())

	getPrefixes := mapParameterPrefixes(t, "handleMapGetParameter")
	if len(getPrefixes) == 0 {
		t.Fatal("enumeration sanity: no map-parameter prefixes found in inference.go — the extractor is broken, so this guard would pass vacuously")
	}

	// Structural half. A prefix understood on read but not on write is exactly
	// the shape that would make get-ok without set-ok, so the two handlers must
	// agree on the prefix set.
	setPrefixes := mapParameterPrefixes(t, "handleMapSetParameter")
	if !slices.Equal(getPrefixes, setPrefixes) {
		var getOnly, setOnly []string
		for _, p := range getPrefixes {
			if !slices.Contains(setPrefixes, p) {
				getOnly = append(getOnly, p)
			}
		}
		for _, p := range setPrefixes {
			if !slices.Contains(getPrefixes, p) {
				setOnly = append(setOnly, p)
			}
		}
		t.Fatalf("ASSUMPTION BROKEN: the map-parameter prefixes diverged between the read and write handlers"+
			"\n  read-only prefixes : %v\n  write-only prefixes: %v\n\n%s",
			getOnly, setOnly, failPathConsequence("the two handlers no longer agree on the map-parameter name space"))
	}

	// Behavioural half: enumerate every name the calibrate loop can reach and
	// assert SetParameter accepts each one.
	candidates := make(map[string]bool, len(parameterTable))
	for name := range parameterTable {
		candidates[name] = true
	}
	mapKeys := mapParameterSubKeys(DefaultParametersConfig())
	if len(mapKeys) == 0 {
		t.Fatal("enumeration sanity: the shipped config exposes no map-parameter sub-keys — the map half of this guard would be empty")
	}
	for _, prefix := range getPrefixes {
		for key := range mapKeys {
			candidates[prefix+key] = true
		}
	}

	resolved, fromMap, refused := 0, 0, make([]string, 0)
	for name := range candidates {
		if _, ok := ie.GetParameter(name); !ok {
			continue // the loop skips these: they can never become a failure
		}
		resolved++
		if slices.ContainsFunc(getPrefixes, func(p string) bool { return strings.HasPrefix(name, p) }) {
			fromMap++
		}
		if err := ie.SetParameter(name, 1.0); err != nil {
			refused = append(refused, fmt.Sprintf("%s (%v)", name, err))
		}
	}
	slices.Sort(refused)

	if resolved < 200 {
		t.Fatalf("enumeration sanity: only %d resolvable names (the shipped config resolves ~285) — the enumeration is broken, so this guard would pass vacuously", resolved)
	}
	if fromMap == 0 {
		t.Fatal("enumeration sanity: no resolvable map sub-key was exercised, so the map-prefix half of this guard is untested")
	}
	if len(refused) > 0 {
		t.Fatalf("ASSUMPTION BROKEN: %d of %d resolvable parameter name(s) were refused by SetParameter:"+
			"\n  %s\n\n%s",
			len(refused), resolved, strings.Join(refused, "\n  "),
			failPathConsequence("the failedCount>0 branch of calibratorVerdict became REACHABLE"))
	}

	// Positive control: the probe must be able to observe a refusal at all.
	// Without this, a handler that started accepting every name would leave the
	// assertions above passing for the wrong reason.
	if err := ie.SetParameter("__failpath_assumption_guard_unknown__", 1.0); err == nil {
		t.Fatal("positive control failed: SetParameter accepted a name that resolves nowhere, so this guard cannot detect a rejected parameter")
	}

	t.Logf("assumption holds: %d/%d resolvable names writable (%d from map prefixes, %d from parameterTable), 0 refused",
		resolved, resolved, fromMap, resolved-fromMap)
}

// failPathConsequence phrases what a broken assumption costs, so a red run says
// what to do rather than only what failed.
func failPathConsequence(what string) string {
	return "CONSEQUENCE: " + what + " ⇒ the pure-function-only coverage of that arm is no longer sufficient:" +
		"\n  1. add a real end-to-end case driving CalibrateParameters into the failed-write path, and" +
		"\n  2. update docs/reference/inert-registry.md §Batch A 收尾 (it records the arm as \"實測否證：目前不可達\")."
}

// mapParameterPrefixes reads the recognised map-parameter prefixes out of the
// named handler in inference.go. The source file is the only place that list
// exists; copying it into the test would let the two drift apart.
func mapParameterPrefixes(t *testing.T, handler string) []string {
	t.Helper()
	src, err := os.ReadFile("inference.go")
	if err != nil {
		t.Fatalf("read inference.go: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "func (ie *InferenceEngine) "+handler+"(")
	if start < 0 {
		t.Fatalf("handler %s not found in inference.go — this guard must be updated with the handler", handler)
	}
	end := strings.Index(body[start+10:], "\nfunc ")
	if end < 0 {
		t.Fatalf("could not find the end of handler %s in inference.go", handler)
	}
	body = body[start : start+10+end]

	matches := regexp.MustCompile(`strings\.CutPrefix\(name, "([^"]+)"\)`).FindAllStringSubmatch(body, -1)
	prefixes := make([]string, 0, len(matches))
	for _, m := range matches {
		prefixes = append(prefixes, m[1])
	}
	slices.Sort(prefixes)
	return slices.Compact(prefixes)
}

// mapParameterSubKeys collects every key of every map[string]float64 reachable
// from cfg, i.e. the sub-key space of the map-parameter names (the prefix that
// each key belongs to is checked through GetParameter, not guessed here).
func mapParameterSubKeys(cfg *ParametersConfig) map[string]bool {
	keys := make(map[string]bool)
	var walk func(v reflect.Value, depth int)
	walk = func(v reflect.Value, depth int) {
		if depth > 8 || !v.IsValid() {
			return
		}
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem(), depth+1)
			}
		case reflect.Struct:
			for i := range v.NumField() {
				walk(v.Field(i), depth+1)
			}
		case reflect.Slice, reflect.Array:
			for i := range v.Len() {
				walk(v.Index(i), depth+1)
			}
		case reflect.Map:
			if v.Type().Key().Kind() == reflect.String && v.Type().Elem().Kind() == reflect.Float64 {
				for _, key := range v.MapKeys() {
					keys[key.String()] = true
				}
			}
			for _, key := range v.MapKeys() {
				walk(v.MapIndex(key), depth+1)
			}
		}
	}
	walk(reflect.ValueOf(cfg), 0)
	return keys
}
