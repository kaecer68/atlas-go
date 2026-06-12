package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeMLFixture writes a metalearner_state.json (and a stub snapshot.json) in dir
// and returns the snapshot path suitable for NewSwarmService.
func writeMLFixture(t *testing.T, dir, mlContent string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "metalearner_state.json"), []byte(mlContent), 0o644); err != nil {
		t.Fatalf("write metalearner_state.json: %v", err)
	}
	snapPath := filepath.Join(dir, "snapshot.json")
	if err := os.WriteFile(snapPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write snapshot.json: %v", err)
	}
	return snapPath
}

func TestLoadRecommendedStrategies_HappyPath(t *testing.T) {
	dir := t.TempDir()
	state := `{
		"strategies": {
			"a": {"id":"a","name":"A","type":"momentum","performance":{"success_count":8,"total_applications":10,"avg_improvement":0.05,"convergence_rate":0.7,"stability_score":0.9}},
			"b": {"id":"b","name":"B","type":"adaptive","performance":{"success_count":5,"total_applications":10,"avg_improvement":0.02,"convergence_rate":0.4,"stability_score":0.6}}
		},
		"population": ["a", "b"]
	}`
	s := NewSwarmService(writeMLFixture(t, dir, state))

	got, err := s.LoadRecommendedStrategies()
	if err != nil {
		t.Fatalf("LoadRecommendedStrategies: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d results, want 2", len(got))
	}
	if got[0].ID != "a" || got[0].Score != 0.8 {
		t.Errorf("first strategy: id=%s score=%v, want a/0.8", got[0].ID, got[0].Score)
	}
	if got[0].Performance == nil {
		t.Fatal("Performance should be populated from metalearner JSON")
	}
	if got[0].Performance.AvgImprovement != 0.05 {
		t.Errorf("AvgImprovement = %v, want 0.05", got[0].Performance.AvgImprovement)
	}
	if got[0].Performance.StabilityScore != 0.9 {
		t.Errorf("StabilityScore = %v, want 0.9", got[0].Performance.StabilityScore)
	}
	if got[0].Performance.ConvergenceRate != 0.7 {
		t.Errorf("ConvergenceRate = %v, want 0.7", got[0].Performance.ConvergenceRate)
	}
}

func TestLoadRecommendedStrategies_NilPerformance(t *testing.T) {
	dir := t.TempDir()
	state := `{"strategies":{"a":{"id":"a","name":"A","type":"momentum"}},"population":["a"]}`
	s := NewSwarmService(writeMLFixture(t, dir, state))

	got, err := s.LoadRecommendedStrategies()
	if err != nil {
		t.Fatalf("LoadRecommendedStrategies: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1", len(got))
	}
	if got[0].Performance != nil {
		t.Errorf("Performance should be nil when source JSON has none, got %+v", got[0].Performance)
	}
	if got[0].Score != 0 {
		t.Errorf("Score = %v, want 0 (no performance data → no rate)", got[0].Score)
	}
}

func TestLoadRecommendedStrategies_TotalApplicationsZero(t *testing.T) {
	dir := t.TempDir()
	state := `{"strategies":{"a":{"id":"a","name":"A","type":"momentum","performance":{"success_count":0,"total_applications":0,"avg_improvement":0.05,"convergence_rate":0.7,"stability_score":0.9}}},"population":["a"]}`
	s := NewSwarmService(writeMLFixture(t, dir, state))

	got, err := s.LoadRecommendedStrategies()
	if err != nil {
		t.Fatalf("LoadRecommendedStrategies: %v", err)
	}
	if got[0].Score != 0 {
		t.Errorf("Score = %v, want 0 (div-by-zero guard)", got[0].Score)
	}
	// Performance should still propagate the non-rate fields.
	if got[0].Performance == nil {
		t.Fatal("Performance should be populated even when TotalApplications=0")
	}
	if got[0].Performance.AvgImprovement != 0.05 {
		t.Errorf("AvgImprovement = %v, want 0.05", got[0].Performance.AvgImprovement)
	}
	if got[0].Performance.StabilityScore != 0.9 {
		t.Errorf("StabilityScore = %v, want 0.9", got[0].Performance.StabilityScore)
	}
}

func TestLoadRecommendedStrategies_MissingPopulationID(t *testing.T) {
	dir := t.TempDir()
	state := `{"strategies":{"a":{"id":"a","name":"A","type":"momentum","performance":{"success_count":5,"total_applications":10}}},"population":["a","missing"]}`
	s := NewSwarmService(writeMLFixture(t, dir, state))

	got, err := s.LoadRecommendedStrategies()
	if err != nil {
		t.Fatalf("LoadRecommendedStrategies: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d results, want 1 (missing ID should be skipped), got=%v", len(got), got)
	}
	if got[0].ID != "a" {
		t.Errorf("ID = %q, want a", got[0].ID)
	}
}

func TestLoadRecommendedStrategies_CapAt5(t *testing.T) {
	dir := t.TempDir()
	strategiesJSON := `{` +
		`"a":{"id":"a","name":"A","type":"momentum","performance":{"success_count":1,"total_applications":1}},` +
		`"b":{"id":"b","name":"B","type":"momentum","performance":{"success_count":1,"total_applications":1}},` +
		`"c":{"id":"c","name":"C","type":"momentum","performance":{"success_count":1,"total_applications":1}},` +
		`"d":{"id":"d","name":"D","type":"momentum","performance":{"success_count":1,"total_applications":1}},` +
		`"e":{"id":"e","name":"E","type":"momentum","performance":{"success_count":1,"total_applications":1}},` +
		`"f":{"id":"f","name":"F","type":"momentum","performance":{"success_count":1,"total_applications":1}},` +
		`"g":{"id":"g","name":"G","type":"momentum","performance":{"success_count":1,"total_applications":1}}` +
		`}`
	state := `{"strategies":` + strategiesJSON + `,"population":["a","b","c","d","e","f","g"]}`
	s := NewSwarmService(writeMLFixture(t, dir, state))

	got, err := s.LoadRecommendedStrategies()
	if err != nil {
		t.Fatalf("LoadRecommendedStrategies: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("got %d results, want 5 (cap-at-5)", len(got))
	}
	// Verify ordering preserved from population slice.
	wantIDs := []string{"a", "b", "c", "d", "e"}
	for i, want := range wantIDs {
		if got[i].ID != want {
			t.Errorf("got[%d].ID = %q, want %q", i, got[i].ID, want)
		}
	}
}

func TestLoadRecommendedStrategies_EmptyState(t *testing.T) {
	dir := t.TempDir()
	state := `{"strategies":{},"population":[]}`
	s := NewSwarmService(writeMLFixture(t, dir, state))

	got, err := s.LoadRecommendedStrategies()
	if err != nil {
		t.Fatalf("LoadRecommendedStrategies: %v", err)
	}
	if got == nil {
		t.Error("got nil, want empty non-nil slice (frontend iterates .length)")
	}
	if len(got) != 0 {
		t.Errorf("got %d results, want 0", len(got))
	}
}

func TestLoadRecommendedStrategies_MissingStateFile(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snapshot.json")
	if err := os.WriteFile(snapPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write snapshot.json: %v", err)
	}
	// Intentionally do NOT write metalearner_state.json.
	s := NewSwarmService(snapPath)

	_, err := s.LoadRecommendedStrategies()
	if err == nil {
		t.Fatal("expected error for missing metalearner_state.json")
	}
	if !strings.Contains(err.Error(), "metalearner state") {
		t.Errorf("err = %v, want wrap containing 'metalearner state'", err)
	}
}

func TestLoadRecommendedStrategies_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	s := NewSwarmService(writeMLFixture(t, dir, `{not valid json`))

	_, err := s.LoadRecommendedStrategies()
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "metalearner") {
		t.Errorf("err = %v, want wrap containing 'metalearner'", err)
	}
}
