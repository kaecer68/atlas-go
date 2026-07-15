package eventdriven

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestActiveTriggerThemesForDay_NilRegistry_ReturnsNil(t *testing.T) {
	if got := activeTriggerThemesForDay(nil); got != nil {
		t.Errorf("activeTriggerThemesForDay(nil) = %v, want nil", got)
	}
}

func TestActiveTriggerThemesForDay_EmptyRegistry_ReturnsEmpty(t *testing.T) {
	reg := narrative.NewDetectorRegistry()
	got := activeTriggerThemesForDay(reg)
	if len(got) != 0 {
		t.Errorf("got %d themes, want 0 for empty registry", len(got))
	}
}

func TestActiveTriggerThemesForDay_DefaultRegistry_ReturnsRegisteredThemes(t *testing.T) {
	reg := narrative.NewDefaultDetectorRegistry()
	got := activeTriggerThemesForDay(reg)
	if len(got) == 0 {
		t.Error("DefaultDetectorRegistry should yield at least one theme, got empty slice")
	}
	for _, theme := range got {
		if _, ok := reg.Get(theme); !ok {
			t.Errorf("theme %q returned by activeTriggerThemesForDay is not in registry", theme)
		}
	}
}

func TestActiveTriggerThemesForDay_PartialRegistry_OnlyRegisteredThemes(t *testing.T) {
	reg := narrative.NewDetectorRegistry()
	if err := reg.Register(&minimalDetector{theme: "US_rates_up", enabled: true}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register(&minimalDetector{theme: "JPY_carry_unwind", enabled: true}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got := activeTriggerThemesForDay(reg)
	if len(got) != 2 {
		t.Errorf("got %d themes, want 2", len(got))
	}
	seen := map[string]bool{}
	for _, theme := range got {
		seen[theme] = true
	}
	if !seen["US_rates_up"] || !seen["JPY_carry_unwind"] {
		t.Errorf("expected both themes, got %v", got)
	}
}

func TestComputeNarrativeTilt_NilThemeSet(t *testing.T) {
	models := []ModelView{{ID: "m1", Weight: 1.0, Direction: "bullish", ActiveThemes: []string{"x"}}}
	if got := computeNarrativeTilt(models, nil); got != 0 {
		t.Errorf("got %v, want 0 for nil themeSet", got)
	}
}

func TestComputeNarrativeTilt_EmptyThemeSet(t *testing.T) {
	models := []ModelView{{ID: "m1", Weight: 1.0, Direction: "bullish", ActiveThemes: []string{"x"}}}
	if got := computeNarrativeTilt(models, map[string]struct{}{}); got != 0 {
		t.Errorf("got %v, want 0 for empty themeSet", got)
	}
}

func TestComputeNarrativeTilt_EmptyModels(t *testing.T) {
	themes := map[string]struct{}{"x": {}}
	if got := computeNarrativeTilt(nil, themes); got != 0 {
		t.Errorf("got %v, want 0 for nil models", got)
	}
	if got := computeNarrativeTilt([]ModelView{}, themes); got != 0 {
		t.Errorf("got %v, want 0 for empty models", got)
	}
}

func TestComputeNarrativeTilt_NeutralDirectionIgnored(t *testing.T) {
	themes := map[string]struct{}{"x": {}}
	models := []ModelView{
		{ID: "neut", Weight: 5.0, Direction: "neutral", ActiveThemes: []string{"x"}},
	}
	if got := computeNarrativeTilt(models, themes); got != 0 {
		t.Errorf("got %v, want 0 (neutral direction contributes nothing)", got)
	}
}

func TestComputeNarrativeTilt_WeightSums(t *testing.T) {
	themes := map[string]struct{}{"x": {}}
	models := []ModelView{
		{ID: "a", Weight: 1.0, Direction: "bullish", ActiveThemes: []string{"x"}},
		{ID: "b", Weight: 0.5, Direction: "bearish", ActiveThemes: []string{"x"}},
		{ID: "c", Weight: 2.0, Direction: "neutral", ActiveThemes: []string{"x"}},
	}
	got := computeNarrativeTilt(models, themes)
	want := 1.0*1.0 + 0.5*(-1.0) + 2.0*0
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestComputeNarrativeTilt_NoMatchingTheme(t *testing.T) {
	themes := map[string]struct{}{"other_theme": {}}
	models := []ModelView{
		{ID: "m1", Weight: 1.0, Direction: "bullish", ActiveThemes: []string{"x"}},
	}
	if got := computeNarrativeTilt(models, themes); got != 0 {
		t.Errorf("got %v, want 0 (model themes don't intersect)", got)
	}
}

func TestThemeIntersects(t *testing.T) {
	set := map[string]struct{}{"a": {}, "b": {}}
	cases := []struct {
		themes []string
		want   bool
	}{
		{[]string{"a"}, true},
		{[]string{"b"}, true},
		{[]string{"a", "b"}, true},
		{[]string{"a", "c"}, true},
		{[]string{"c"}, false},
		{[]string{}, false},
		{nil, false},
	}
	for _, tc := range cases {
		if got := themeIntersects(tc.themes, set); got != tc.want {
			t.Errorf("themeIntersects(%v, set) = %v, want %v", tc.themes, got, tc.want)
		}
	}
}
