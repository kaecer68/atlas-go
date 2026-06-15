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

func TestSwarmService_NewSwarmService(t *testing.T) {
	svc := NewSwarmService("/path/to/snapshot")
	if svc == nil {
		t.Fatal("NewSwarmService returned nil")
	}
	if svc.snapshotPath != "/path/to/snapshot" {
		t.Errorf("snapshotPath = %q, want /path/to/snapshot", svc.snapshotPath)
	}
}

func TestSwarmService_SetTrainingDir(t *testing.T) {
	svc := NewSwarmService("/path/to/snapshot")
	svc.SetTrainingDir("/training/dir")
	if svc.trainingDir != "/training/dir" {
		t.Errorf("trainingDir = %q, want /training/dir", svc.trainingDir)
	}
}

func writeSwarmSnapshotFixture(t *testing.T, dir, snapshotJSON string) string {
	snapPath := filepath.Join(dir, "swarm_snapshot.json")
	if err := os.WriteFile(snapPath, []byte(snapshotJSON), 0o644); err != nil {
		t.Fatalf("write swarm_snapshot.json: %v", err)
	}
	return snapPath
}

func TestSwarmService_LoadStatus_OK(t *testing.T) {
	dir := t.TempDir()
	snap := `{
		"recorded_at": "2026-06-15T10:00:00Z",
		"total_fish": 20,
		"consensus_confidence": 0.75,
		"top_fish_accuracy": 0.68,
		"generations_evolved": 5,
		"consensus": {"2330": {"bullish_count": 8, "bearish_count": 2, "neutral_count": 2, "consensus_direction": "bullish", "average_confidence": 0.72}},
		"anomalies": [{"type": "flash_rally", "symbol": "2330"}],
		"scenarios": [{"id": "bull_market", "name": "Bull Market", "regime": "bull", "volatility": 0.15, "trend": 0.3}]
	}`
	svc := NewSwarmService(writeSwarmSnapshotFixture(t, dir, snap))

	status, err := svc.LoadStatus()
	if err != nil {
		t.Fatalf("LoadStatus error = %v", err)
	}
	if status.TotalFish != 20 {
		t.Errorf("TotalFish = %d, want 20", status.TotalFish)
	}
	if status.ConsensusSymbols != 1 {
		t.Errorf("ConsensusSymbols = %d, want 1", status.ConsensusSymbols)
	}
	if status.ConsensusConfidence != 0.75 {
		t.Errorf("ConsensusConfidence = %v, want 0.75", status.ConsensusConfidence)
	}
	if status.AnomalyCount != 1 {
		t.Errorf("AnomalyCount = %d, want 1", status.AnomalyCount)
	}
	if status.ScenarioCount != 1 {
		t.Errorf("ScenarioCount = %d, want 1", status.ScenarioCount)
	}
	if status.GenerationsEvolved != 5 {
		t.Errorf("GenerationsEvolved = %d, want 5", status.GenerationsEvolved)
	}
}

func TestSwarmService_LoadStatus_NotFound(t *testing.T) {
	svc := NewSwarmService("/nonexistent/path/snapshot.json")
	_, err := svc.LoadStatus()
	if err == nil {
		t.Error("expected error for nonexistent snapshot")
	}
}

func TestSwarmService_LoadStatus_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	svc := NewSwarmService(writeSwarmSnapshotFixture(t, dir, `{not valid json`))
	_, err := svc.LoadStatus()
	if err == nil {
		t.Error("expected error for malformed JSON")
	}
}

func TestSwarmService_LoadConsensus_OK(t *testing.T) {
	dir := t.TempDir()
	snap := `{
		"consensus": {
			"2330": {"bullish_count": 8, "bearish_count": 2, "neutral_count": 2, "consensus_direction": "bullish", "average_confidence": 0.72},
			"2317": {"bullish_count": 3, "bearish_count": 5, "neutral_count": 1, "consensus_direction": "bearish", "average_confidence": 0.65}
		}
	}`
	svc := NewSwarmService(writeSwarmSnapshotFixture(t, dir, snap))

	entries, err := svc.LoadConsensus()
	if err != nil {
		t.Fatalf("LoadConsensus error = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("len(entries) = %d, want 2", len(entries))
	}
}

func TestSwarmService_LoadConsensus_Empty(t *testing.T) {
	dir := t.TempDir()
	svc := NewSwarmService(writeSwarmSnapshotFixture(t, dir, `{"consensus": {}}`))

	entries, err := svc.LoadConsensus()
	if err != nil {
		t.Fatalf("LoadConsensus error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0", len(entries))
	}
}

func TestSwarmService_LoadAnomalies_OK(t *testing.T) {
	dir := t.TempDir()
	snap := `{"anomalies": [{"type": "flash_rally", "symbol": "2330"}, {"type": "liquidity_crisis", "symbol": "2317"}]}`
	svc := NewSwarmService(writeSwarmSnapshotFixture(t, dir, snap))

	anomalies, err := svc.LoadAnomalies()
	if err != nil {
		t.Fatalf("LoadAnomalies error = %v", err)
	}
	if len(anomalies) != 2 {
		t.Errorf("len(anomalies) = %d, want 2", len(anomalies))
	}
}

func TestSwarmService_LoadScenarios_OK(t *testing.T) {
	dir := t.TempDir()
	snap := `{"scenarios": [{"id": "bull", "name": "Bull Market", "regime": "bull", "volatility": 0.15, "trend": 0.3}]}`
	svc := NewSwarmService(writeSwarmSnapshotFixture(t, dir, snap))

	scenarios, err := svc.LoadScenarios()
	if err != nil {
		t.Fatalf("LoadScenarios error = %v", err)
	}
	if len(scenarios) != 1 {
		t.Errorf("len(scenarios) = %d, want 1", len(scenarios))
	}
}

func TestSwarmService_CountTrainingScenarios_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	svc := NewSwarmService(writeSwarmSnapshotFixture(t, dir, `{}`))
	svc.SetTrainingDir(dir)

	count := svc.countTrainingScenarios()
	if count != 0 {
		t.Errorf("countTrainingScenarios = %d, want 0 for empty dir", count)
	}
}

func TestSwarmService_CountTrainingScenarios_NoTrainingDir(t *testing.T) {
	svc := NewSwarmService("/some/path")
	count := svc.countTrainingScenarios()
	if count != 0 {
		t.Errorf("countTrainingScenarios = %d, want 0 when trainingDir not set", count)
	}
}

func TestSwarmService_CountTrainingScenarios_WithJSONLFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "scenario1.jsonl"), []byte(`{"a":1}
{"b":2}
{"c":3}
`), 0o644); err != nil {
		t.Fatalf("write scenario1.jsonl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "scenario2.jsonl"), []byte(`{"d":4}
`), 0o644); err != nil {
		t.Fatalf("write scenario2.jsonl: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notjsonl.txt"), []byte(`ignore me`), 0o644); err != nil {
		t.Fatalf("write notjsonl.txt: %v", err)
	}

	svc := NewSwarmService(writeSwarmSnapshotFixture(t, dir, `{}`))
	svc.SetTrainingDir(dir)

	count := svc.countTrainingScenarios()
	if count != 4 {
		t.Errorf("countTrainingScenarios = %d, want 4 (3 lines + 1 line)", count)
	}
}

func TestSwarmService_LoadRecommendedStrategies_MalformedJSON(t *testing.T) {
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
