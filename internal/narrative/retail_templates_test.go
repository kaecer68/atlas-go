package narrative

import "testing"

func TestRetailTemplatesExist(t *testing.T) {
	templates := DefaultTemplates()

	hasFrenzy := false
	hasFear := false
	for _, tmpl := range templates {
		if tmpl.TriggerTheme == "retail_frenzy" {
			hasFrenzy = true
		}
		if tmpl.TriggerTheme == "retail_fear" {
			hasFear = true
		}
	}

	if !hasFrenzy {
		t.Error("missing retail_frenzy template")
	}
	if !hasFear {
		t.Error("missing retail_fear template")
	}
}

func TestRetailModelExists(t *testing.T) {
	ne := NewNarrativeEngine()
	models := ne.ActiveModels([]string{"retail_frenzy", "retail_fear"})

	found := false
	for _, m := range models {
		if m.ID == "retail_sentiment_model" {
			found = true
			break
		}
	}

	if !found {
		t.Error("expected retail_sentiment_model to be active for retail themes")
	}
}
