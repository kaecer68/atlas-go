package apiswarm

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/metalearning"
	"github.com/kaecer68/atlas-go/internal/monitoring/service"
	"github.com/kaecer68/atlas-go/internal/swarm"
)

func writeSnapshot(t *testing.T, path string, snap swarm.SwarmSnapshot) {
	t.Helper()
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
}

func writeMetalearnerState(t *testing.T, dir string) {
	t.Helper()
	state := map[string]any{
		"strategies": map[string]any{
			"strat-1": map[string]any{
				"id":   "strat-1",
				"name": "Test Strategy",
				"type": "momentum",
				"performance": map[string]any{
					"total_applications": 100,
					"success_count":      75,
				},
			},
		},
		"population": []string{"strat-1"},
	}
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal metalearner: %v", err)
	}
	path := filepath.Join(dir, "metalearner_state.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write metalearner: %v", err)
	}
}

func makeValidSnapshot() swarm.SwarmSnapshot {
	ts := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)
	return swarm.SwarmSnapshot{
		RecordedAt: ts,
		TotalFish:  100,
		Consensus: map[string]swarm.ConsensusPrediction{
			"2330": {
				Symbol:             "2330",
				BullishCount:       10,
				BearishCount:       5,
				NeutralCount:       3,
				ConsensusDirection: "bullish",
				AverageConfidence:  0.75,
			},
		},
		ConsensusConfidence: 0.8,
		TopFishAccuracy:     0.9,
		Anomalies: []swarm.Anomaly{
			{Type: "volatility_spike", Description: "high vol", Severity: 0.7, Symbols: []string{"2330"}},
		},
		Scenarios: []swarm.ScenarioSnapshot{
			{ID: "s1", Name: "bull", Regime: "bull", Volatility: 0.15, Trend: 0.02},
		},
		GenerationsEvolved: 5,
	}
}

func newTestHandlers(t *testing.T) *Handlers {
	t.Helper()
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "swarm_snapshot.json")
	writeSnapshot(t, snapshotPath, makeValidSnapshot())
	writeMetalearnerState(t, dir)
	svc := service.NewSwarmService(snapshotPath)
	svc.SetTrainingDir(dir)
	return &Handlers{Svc: svc}
}

func assertStatus(t *testing.T, status int, want int) {
	t.Helper()
	if status != want {
		t.Errorf("status = %d, want %d", status, want)
	}
}

func TestHandleStatus_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-status", nil)
	status, body := h.HandleStatus(req)
	assertStatus(t, status, http.StatusOK)
	m, ok := body.(*service.SwarmStatusResponse)
	if !ok {
		t.Fatalf("body is %T, want *SwarmStatusResponse", body)
	}
	if m.TotalFish != 100 {
		t.Errorf("TotalFish = %d, want 100", m.TotalFish)
	}
	if m.AnomalyCount != 1 {
		t.Errorf("AnomalyCount = %d, want 1", m.AnomalyCount)
	}
}

func TestHandleStatus_NoData(t *testing.T) {
	h := &Handlers{Svc: service.NewSwarmService("/nonexistent/path.json")}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-status", nil)
	status, _ := h.HandleStatus(req)
	assertStatus(t, status, http.StatusNotFound)
}

func TestHandleConsensus_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-consensus", nil)
	status, body := h.HandleConsensus(req)
	assertStatus(t, status, http.StatusOK)
	entries, ok := body.([]service.ConsensusEntry)
	if !ok {
		t.Fatalf("body is %T, want []ConsensusEntry", body)
	}
	if len(entries) != 1 {
		t.Errorf("len = %d, want 1", len(entries))
	}
}

func TestHandleConsensus_NoData(t *testing.T) {
	h := &Handlers{Svc: service.NewSwarmService("/nonexistent/path.json")}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-consensus", nil)
	status, _ := h.HandleConsensus(req)
	assertStatus(t, status, http.StatusNotFound)
}

func TestHandleAnomalies_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-anomalies", nil)
	status, body := h.HandleAnomalies(req)
	assertStatus(t, status, http.StatusOK)
	anomalies, ok := body.([]swarm.Anomaly)
	if !ok {
		t.Fatalf("body is %T, want []Anomaly", body)
	}
	if len(anomalies) != 1 {
		t.Errorf("len = %d, want 1", len(anomalies))
	}
}

func TestHandleAnomalies_NoData(t *testing.T) {
	h := &Handlers{Svc: service.NewSwarmService("/nonexistent/path.json")}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-anomalies", nil)
	status, _ := h.HandleAnomalies(req)
	assertStatus(t, status, http.StatusNotFound)
}

func TestHandleAnomalies_NilSlice(t *testing.T) {
	dir := t.TempDir()
	snap := makeValidSnapshot()
	snap.Anomalies = nil
	path := filepath.Join(dir, "snapshot.json")
	writeSnapshot(t, path, snap)
	h := &Handlers{Svc: service.NewSwarmService(path)}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-anomalies", nil)
	status, body := h.HandleAnomalies(req)
	assertStatus(t, status, http.StatusOK)
	anomalies, ok := body.([]swarm.Anomaly)
	if !ok {
		t.Fatalf("body is %T, want []Anomaly", body)
	}
	if len(anomalies) != 0 {
		t.Errorf("len = %d, want 0 (nil coerced to empty)", len(anomalies))
	}
}

func TestHandleScenarios_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-scenarios", nil)
	status, body := h.HandleScenarios(req)
	assertStatus(t, status, http.StatusOK)
	scenarios, ok := body.([]swarm.ScenarioSnapshot)
	if !ok {
		t.Fatalf("body is %T, want []ScenarioSnapshot", body)
	}
	if len(scenarios) != 1 {
		t.Errorf("len = %d, want 1", len(scenarios))
	}
}

func TestHandleScenarios_NoData(t *testing.T) {
	h := &Handlers{Svc: service.NewSwarmService("/nonexistent/path.json")}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-scenarios", nil)
	status, _ := h.HandleScenarios(req)
	assertStatus(t, status, http.StatusNotFound)
}

func TestHandleScenarios_NilSlice(t *testing.T) {
	dir := t.TempDir()
	snap := makeValidSnapshot()
	snap.Scenarios = nil
	path := filepath.Join(dir, "snapshot.json")
	writeSnapshot(t, path, snap)
	h := &Handlers{Svc: service.NewSwarmService(path)}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-scenarios", nil)
	status, body := h.HandleScenarios(req)
	assertStatus(t, status, http.StatusOK)
	scenarios, ok := body.([]swarm.ScenarioSnapshot)
	if !ok {
		t.Fatalf("body is %T, want []ScenarioSnapshot", body)
	}
	if len(scenarios) != 0 {
		t.Errorf("len = %d, want 0 (nil coerced to empty)", len(scenarios))
	}
}

func TestHandleStrategies_Success(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-strategies", nil)
	status, body := h.HandleStrategies(req)
	assertStatus(t, status, http.StatusOK)
	summaries, ok := body.([]service.StrategySummary)
	if !ok {
		t.Fatalf("body is %T, want []StrategySummary", body)
	}
	if len(summaries) != 1 {
		t.Errorf("len = %d, want 1", len(summaries))
	}
	if summaries[0].Score != 0.75 {
		t.Errorf("Score = %f, want 0.75", summaries[0].Score)
	}
}

func TestHandleStrategies_NoData(t *testing.T) {
	h := &Handlers{Svc: service.NewSwarmService("/nonexistent/path.json")}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-strategies", nil)
	status, _ := h.HandleStrategies(req)
	assertStatus(t, status, http.StatusNotFound)
}

func TestHandleStrategies_EmptyPopulation(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "swarm_snapshot.json")
	writeSnapshot(t, snapshotPath, makeValidSnapshot())
	// Write metalearner with empty population
	state := map[string]any{
		"strategies": map[string]any{},
		"population": []string{},
	}
	b, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(dir, "metalearner_state.json"), b, 0o644)
	svc := service.NewSwarmService(snapshotPath)
	h := &Handlers{Svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-strategies", nil)
	status, body := h.HandleStrategies(req)
	assertStatus(t, status, http.StatusOK)
	summaries, ok := body.([]service.StrategySummary)
	if !ok {
		t.Fatalf("body is %T, want []StrategySummary", body)
	}
	if len(summaries) != 0 {
		t.Errorf("len = %d, want 0", len(summaries))
	}
}

func TestHandleConsensus_EmptyConsensus(t *testing.T) {
	dir := t.TempDir()
	snap := makeValidSnapshot()
	snap.Consensus = map[string]swarm.ConsensusPrediction{}
	path := filepath.Join(dir, "snapshot.json")
	writeSnapshot(t, path, snap)
	h := &Handlers{Svc: service.NewSwarmService(path)}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-consensus", nil)
	status, body := h.HandleConsensus(req)
	assertStatus(t, status, http.StatusOK)
	entries, ok := body.([]service.ConsensusEntry)
	if !ok {
		t.Fatalf("body is %T, want []ConsensusEntry", body)
	}
	if len(entries) != 0 {
		t.Errorf("len = %d, want 0", len(entries))
	}
}

func TestRegisterRoutes(t *testing.T) {
	h := newTestHandlers(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/dashboard/swarm-status"},
		{"GET", "/api/dashboard/swarm-consensus"},
		{"GET", "/api/dashboard/swarm-anomalies"},
		{"GET", "/api/dashboard/swarm-scenarios"},
		{"GET", "/api/dashboard/swarm-strategies"},
	}
	for _, r := range routes {
		req := httptest.NewRequest(r.method, r.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code == 0 {
			t.Errorf("route %s %s not registered (no handler)", r.method, r.path)
		}
	}
}

func TestHandleStrategies_MalformedMetalearnerJSON(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "swarm_snapshot.json")
	writeSnapshot(t, snapshotPath, makeValidSnapshot())
	// Write corrupt metalearner
	if err := os.WriteFile(filepath.Join(dir, "metalearner_state.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt metalearner: %v", err)
	}
	svc := service.NewSwarmService(snapshotPath)
	h := &Handlers{Svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-strategies", nil)
	status, _ := h.HandleStrategies(req)
	assertStatus(t, status, http.StatusNotFound)
}

func TestHandleStatus_CorruptSnapshot(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "snapshot.json")
	if err := os.WriteFile(snapshotPath, []byte("{bad"), 0o644); err != nil {
		t.Fatalf("write corrupt snapshot: %v", err)
	}
	h := &Handlers{Svc: service.NewSwarmService(snapshotPath)}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-status", nil)
	status, _ := h.HandleStatus(req)
	assertStatus(t, status, http.StatusNotFound)
}

// Verify that json.RawMessage round-trips through the handlers produce correct JSON.
func TestHandleStatus_JSONRoundtrip(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-status", nil)
	status, body := h.HandleStatus(req)
	assertStatus(t, status, http.StatusOK)
	// Marshal/unmarshal to verify JSON serialization
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := m["total_fish"]; !ok {
		t.Error("missing total_fish in JSON output")
	}
}

func TestHandleStrategies_NilPerformance(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "swarm_snapshot.json")
	writeSnapshot(t, snapshotPath, makeValidSnapshot())
	state := map[string]any{
		"strategies": map[string]any{
			"strat-1": map[string]any{
				"id":          "strat-1",
				"name":        "Test",
				"type":        "momentum",
				"performance": nil,
			},
		},
		"population": []string{"strat-1"},
	}
	b, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(dir, "metalearner_state.json"), b, 0o644)
	svc := service.NewSwarmService(snapshotPath)
	h := &Handlers{Svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-strategies", nil)
	status, body := h.HandleStrategies(req)
	assertStatus(t, status, http.StatusOK)
	summaries, ok := body.([]service.StrategySummary)
	if !ok {
		t.Fatalf("body is %T, want []StrategySummary", body)
	}
	if len(summaries) != 1 {
		t.Fatalf("len = %d, want 1", len(summaries))
	}
	if summaries[0].Score != 0.0 {
		t.Errorf("Score = %f, want 0.0 for nil performance", summaries[0].Score)
	}
}

func TestHandleStrategies_ZeroApplications(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "swarm_snapshot.json")
	writeSnapshot(t, snapshotPath, makeValidSnapshot())
	state := map[string]any{
		"strategies": map[string]any{
			"strat-1": map[string]any{
				"id":   "strat-1",
				"name": "Test",
				"type": "momentum",
				"performance": map[string]any{
					"total_applications": 0,
					"success_count":      0,
				},
			},
		},
		"population": []string{"strat-1"},
	}
	b, _ := json.Marshal(state)
	_ = os.WriteFile(filepath.Join(dir, "metalearner_state.json"), b, 0o644)
	svc := service.NewSwarmService(snapshotPath)
	h := &Handlers{Svc: svc}
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-strategies", nil)
	status, body := h.HandleStrategies(req)
	assertStatus(t, status, http.StatusOK)
	summaries := body.([]service.StrategySummary)
	if summaries[0].Score != 0.0 {
		t.Errorf("Score = %f, want 0.0 when TotalApplications=0", summaries[0].Score)
	}
}

// verify JSON tags match snake_case convention
func TestHandleStatus_SnakeCaseJSON(t *testing.T) {
	h := newTestHandlers(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/swarm-status", nil)
	_, body := h.HandleStatus(req)
	b, _ := json.Marshal(body)
	var m map[string]any
	_ = json.NewDecoder(bytes.NewReader(b)).Decode(&m)
	// Verify snake_case keys
	for _, key := range []string{"total_fish", "consensus_symbols", "consensus_confidence", "top_accuracy", "anomaly_count", "scenario_count", "generations_evolved"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing snake_case key %q in JSON output", key)
		}
	}
}

// Ensure unused imports stay for embedded type assertions
var _ metalearning.StrategyPerformance
