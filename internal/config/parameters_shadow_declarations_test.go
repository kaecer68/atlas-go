package config

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── Shadow-parameter declarations (#1944 Batch 4 / N-C3) ─────────────────
//
// N-C3 found three industry.* parameters that were declared in config while
// hardcoded copies were the real implementation. Each entry below is either
// WIRED (a non-test consumer exists, and the declaration must say so) or NOT
// WIRED (no consumer, and the declaration must say so). These tests fail as soon
// as the two drift apart, so a knob can never silently change from dead to live
// — or the other way around — without updating the shipped rationale.
type shadowParam struct {
	field        string // Go struct field name on IndustryParameters
	key          string // config JSON key (industry.<key>)
	wantConsumer bool
}

var shadowParams = []shadowParam{
	// N-C3: wired in #1944 Batch 4 — EventCalendar.sentimentCap() reads it.
	{field: "EventSentimentCap", key: "event_sentiment_cap", wantConsumer: true},
	// N-C3: still shadow — the runtime uses EvidenceTier, not this enum map.
	{field: "FreshnessScores", key: "freshness_scores", wantConsumer: false},
	// N-C3: still shadow — EventCalendar uses hardcoded defaultEventRules().
	{field: "EventCalendarRules", key: "event_calendar_rules", wantConsumer: false},
}

const declaredNotWired = "NOT WIRED"

// scanShadowConsumers returns the non-test Go files outside internal/config that
// mention the field name or the JSON key.
func scanShadowConsumers(t *testing.T, repoRoot string, p shadowParam) []string {
	t.Helper()
	var consumers []string
	for _, sub := range []string{"internal", "cmd"} {
		base := filepath.Join(repoRoot, sub)
		walkErr := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			if strings.Contains(filepath.ToSlash(path), "/internal/config/") {
				return nil // the declaration itself
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			text := string(data)
			if strings.Contains(text, p.field) || strings.Contains(text, p.key) {
				rel, relErr := filepath.Rel(repoRoot, path)
				if relErr != nil {
					rel = path
				}
				consumers = append(consumers, rel)
			}
			return nil
		})
		if walkErr != nil {
			t.Fatalf("walk %s: %v", base, walkErr)
		}
	}
	return consumers
}

// TestShadowParametersDeclarationMatchesConsumers couples the code scan with the
// declaration, for both the Go defaults and the shipped config file.
func TestShadowParametersDeclarationMatchesConsumers(t *testing.T) {
	repoRoot, err := moduleRoot()
	if err != nil {
		t.Skipf("module root not found: %v", err)
	}

	shippedPath := filepath.Join(repoRoot, "configs", "parameters.json")
	shippedRaw, err := os.ReadFile(shippedPath)
	if err != nil {
		t.Fatalf("read shipped parameters: %v", err)
	}
	var shippedRoot map[string]json.RawMessage
	if err := json.Unmarshal(shippedRaw, &shippedRoot); err != nil {
		t.Fatalf("parse shipped parameters: %v", err)
	}
	var industry map[string]json.RawMessage
	if err := json.Unmarshal(shippedRoot["industry"], &industry); err != nil {
		t.Fatalf("parse shipped industry section: %v", err)
	}

	defaults := DefaultParametersConfig()

	for _, p := range shadowParams {
		t.Run(p.key, func(t *testing.T) {
			consumers := scanShadowConsumers(t, repoRoot, p)

			if p.wantConsumer && len(consumers) == 0 {
				t.Fatalf("declared WIRED but no non-test consumer reads %s outside internal/config — either wire it or declare %q", p.field, declaredNotWired)
			}
			if !p.wantConsumer && len(consumers) > 0 {
				t.Fatalf("consumer(s) now read %s: %v — update the declarations (configs/parameters.json, configs/parameters/industry.json, defaults) to describe what is actually enforced, then drop %q",
					p.field, consumers, declaredNotWired)
			}

			// Go defaults declaration (event_calendar_rules has no Go default).
			var defaultRationale string
			switch p.field {
			case "EventSentimentCap":
				defaultRationale = defaults.Industry.EventSentimentCap.Rationale
			case "FreshnessScores":
				defaultRationale = defaults.Industry.FreshnessScores.Rationale
			case "EventCalendarRules":
				defaultRationale = "n/a"
			}
			if defaultRationale != "n/a" {
				assertDeclaration(t, "DefaultParametersConfig", p, defaultRationale)
			}

			// Shipped config declaration.
			raw, ok := industry[p.key]
			if !ok {
				t.Fatalf("configs/parameters.json industry.%s is missing", p.key)
			}
			var meta struct {
				Rationale string `json:"rationale"`
			}
			if err := json.Unmarshal(raw, &meta); err != nil {
				t.Fatalf("parse industry.%s: %v", p.key, err)
			}
			assertDeclaration(t, "configs/parameters.json", p, meta.Rationale)
		})
	}
}

// assertDeclaration enforces the only two legal states: a NOT WIRED marker with
// no consumer, or no marker with a consumer.
func assertDeclaration(t *testing.T, where string, p shadowParam, rationale string) {
	t.Helper()
	hasMarker := strings.Contains(rationale, declaredNotWired)
	if !p.wantConsumer && !hasMarker {
		t.Errorf("%s: no consumer reads %s, but the rationale does not say %q — the config would claim a knob that does nothing",
			where, p.field, declaredNotWired)
	}
	if p.wantConsumer && hasMarker {
		t.Errorf("%s: %s now has a consumer, but the rationale still says %q — describe the wiring instead",
			where, p.field, declaredNotWired)
	}
}
