package eventdriven

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/narrative"
)

func TestNarrativeProvider_ListModels_NilEngine(t *testing.T) {
	p := &NarrativeProvider{Engine: nil}
	got := p.ListModels()
	if got != nil {
		t.Errorf("nil engine should return nil, got %v", got)
	}
}

func TestNarrativeProvider_ListModels_NeutralDefaults(t *testing.T) {
	ne := narrative.NewNarrativeEngine()
	p := &NarrativeProvider{Engine: ne}
	views := p.ListModels()
	if len(views) == 0 {
		t.Fatal("expected default models from NewNarrativeEngine")
	}
	for i, v := range views {
		if v.Direction != "neutral" {
			t.Errorf("model %d (%s): default RecentPrediction=0 should derive Direction=neutral, got %s",
				i, v.ID, v.Direction)
		}
	}
}

func TestNarrativeProvider_ListModels_PreservesFields(t *testing.T) {
	ne := narrative.NewNarrativeEngine()
	src := ne.ListModels()
	if len(src) == 0 {
		t.Skip("engine returned no models")
	}

	p := &NarrativeProvider{Engine: ne}
	got := p.ListModels()

	for i, m := range got {
		if m.ID != src[i].ID {
			t.Errorf("model %d: ID mismatch %s != %s", i, m.ID, src[i].ID)
		}
		if m.Name != src[i].Name {
			t.Errorf("model %d: Name mismatch %s != %s", i, m.Name, src[i].Name)
		}
		if m.Weight != src[i].Weight {
			t.Errorf("model %d: Weight mismatch %v != %v", i, m.Weight, src[i].Weight)
		}
		if len(m.ActiveThemes) != len(src[i].ActiveThemes) {
			t.Errorf("model %d: ActiveThemes length mismatch %d != %d",
				i, len(m.ActiveThemes), len(src[i].ActiveThemes))
		}
	}
}

func TestDirectionFromPrediction(t *testing.T) {
	cases := []struct {
		pred float64
		want string
	}{
		{0.5, "bullish"},
		{0.001, "bullish"},
		{0, "neutral"},
		{-0.001, "bearish"},
		{-0.5, "bearish"},
	}
	for _, c := range cases {
		got := directionFromPrediction(c.pred)
		if got != c.want {
			t.Errorf("directionFromPrediction(%v) = %s, want %s", c.pred, got, c.want)
		}
	}
}
