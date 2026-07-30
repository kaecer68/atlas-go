package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/domain/recommendation"
	"github.com/kaecer68/atlas-go/internal/domain/shared"
	"github.com/kaecer68/atlas-go/internal/industry"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// =============================================================================
// Pure helpers (no I/O, no dependencies)
// =============================================================================

func TestComputeRegimeStability_NilBreakdown(t *testing.T) {
	if got := computeRegimeStability(nil); got != nil {
		t.Errorf("nil breakdown: got %v, want nil", got)
	}
}

func TestComputeRegimeStability_SingleRegime(t *testing.T) {
	rb := &recommendation.RegimeBreakdown{Regimes: map[string]recommendation.RegimePerformance{
		"bull": {AvgReturn: 0.05},
	}}
	if got := computeRegimeStability(rb); got != nil {
		t.Errorf("single regime: got %v, want nil (need >= 2 regimes)", got)
	}
}

func TestComputeRegimeStability_TwoRegimes(t *testing.T) {
	rb := &recommendation.RegimeBreakdown{Regimes: map[string]recommendation.RegimePerformance{
		"bull": {AvgReturn: 0.05},
		"bear": {AvgReturn: -0.03},
	}}
	got := computeRegimeStability(rb)
	if got == nil {
		t.Fatal("expected non-nil std deviation for 2 regimes")
	}
	// Mean = 0.01, variance = ((0.05-0.01)^2 + (-0.03-0.01)^2) / 2 = (0.0016+0.0016)/2 = 0.0016
	// std = sqrt(0.0016) = 0.04
	want := 0.04
	if *got < want-1e-6 || *got > want+1e-6 {
		t.Errorf("std = %v, want %v", *got, want)
	}
}

func TestComputeRegimeStability_ManyRegimes(t *testing.T) {
	rb := &recommendation.RegimeBreakdown{Regimes: map[string]recommendation.RegimePerformance{
		"r1": {AvgReturn: 1.0},
		"r2": {AvgReturn: 2.0},
		"r3": {AvgReturn: 3.0},
		"r4": {AvgReturn: 4.0},
	}}
	got := computeRegimeStability(rb)
	if got == nil {
		t.Fatal("expected non-nil std")
	}
	// Mean=2.5, variance = ((1-2.5)^2 + (2-2.5)^2 + (3-2.5)^2 + (4-2.5)^2)/4 = 5/4 = 1.25
	// std = sqrt(1.25) ≈ 1.118
	if *got < 1.11 || *got > 1.12 {
		t.Errorf("std ≈ %v, want ≈ 1.118", *got)
	}
}

func TestBothNonZeroAndDivergent(t *testing.T) {
	tests := []struct {
		name string
		a, b float64
		want bool
	}{
		{"both zero", 0, 0, false},
		{"one zero", 0, 1.0, false},
		{"near zero (< epsilon)", 0.01, 1.0, false},
		{"equal non-zero", 1.0, 1.0, false},
		{"small relative diff (5%)", 1.0, 1.05, false},
		{"large relative diff (50%)", 1.0, 1.5, true},
		{"negative large diff", 1.0, -1.5, true},
		{"very small but non-zero (boundary)", 0.05, 1.0, true}, // 0.05 == epsilon, NOT below → proceeds; relDiff = 0.95/1.0 = 0.95 > 0.10
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := bothNonZeroAndDivergent(tt.a, tt.b); got != tt.want {
				t.Errorf("bothNonZeroAndDivergent(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// =============================================================================
// LoadMacroRadar: disk + session provider
// =============================================================================

func TestLoadMacroRadar_NoSession_NoDir(t *testing.T) {
	// Sessions dir doesn't exist → FindLatestSessionSummary returns nil, no error.
	baseDir := t.TempDir()
	svc := NewPipelineService(baseDir, baseDir, nil)
	data, err := svc.LoadMacroRadar("")
	if err != nil {
		t.Fatalf("expected nil error for empty ledger, got %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data when no sessions exist, got %+v", data)
	}
}

func TestLoadMacroRadar_ExplicitSession_Missing(t *testing.T) {
	baseDir := t.TempDir()
	svc := NewPipelineService(baseDir, baseDir, nil)
	data, err := svc.LoadMacroRadar("session-20990101-daily")
	if err != nil {
		t.Fatalf("missing session: expected nil error (LoadSessionSummary returns nil for missing), got %v", err)
	}
	if data != nil {
		t.Errorf("expected nil data, got %+v", data)
	}
}

func TestLoadMacroRadar_ExplicitSession_Found(t *testing.T) {
	baseDir := t.TempDir()
	recordedAt := time.Date(2026, time.April, 22, 4, 2, 30, 0, time.UTC)
	sessionID := "session-20260422-daily"
	writeTestSessionSummaryOnly(t, baseDir, sessionID, domain.SessionSummary{
		SessionID:    sessionID,
		Regime:       domain.RegimeRiskOn,
		RecordedAt:   recordedAt,
		OutcomeCount: 3,
		GuardOutcomes: []domain.GuardOutcome{
			{GuardID: "g1", Passed: true, InputCount: 10, OutputCount: 5},
		},
	})

	svc := NewPipelineService(baseDir, baseDir, nil)
	data, err := svc.LoadMacroRadar(sessionID)
	if err != nil {
		t.Fatalf("LoadMacroRadar: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if data.SessionID != sessionID {
		t.Errorf("SessionID = %q, want %q", data.SessionID, sessionID)
	}
	if data.Regime != domain.RegimeRiskOn {
		t.Errorf("Regime = %q, want %q", data.Regime, domain.RegimeRiskOn)
	}
	if len(data.GuardOutcomes) != 1 {
		t.Errorf("GuardOutcomes len = %d, want 1", len(data.GuardOutcomes))
	}
	if data.GuardOutcomes[0].GuardID != "g1" {
		t.Errorf("GuardID = %q, want g1", data.GuardOutcomes[0].GuardID)
	}
}

// =============================================================================
// LoadSessions
// =============================================================================

func TestLoadSessions_NoDir(t *testing.T) {
	baseDir := t.TempDir()
	svc := NewPipelineService(baseDir, baseDir, nil)
	sessions, err := svc.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions with no dir: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty, got %d sessions", len(sessions))
	}
}

func TestLoadSessions_EmptyDir(t *testing.T) {
	baseDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(baseDir, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewPipelineService(baseDir, baseDir, nil)
	sessions, err := svc.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty, got %d", len(sessions))
	}
}

func TestLoadSessions_MultipleSessions(t *testing.T) {
	baseDir := t.TempDir()
	dates := []time.Time{
		time.Date(2026, 4, 20, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 22, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 21, 4, 0, 0, 0, time.UTC),
	}
	sessionIDs := []string{"session-20260420-daily", "session-20260422-daily", "session-20260421-daily"}
	for i, id := range sessionIDs {
		writeTestSessionSummaryOnly(t, baseDir, id, domain.SessionSummary{
			SessionID:    id,
			Regime:       domain.RegimeRiskOn,
			RecordedAt:   dates[i],
			OutcomeCount: i + 1,
		})
	}

	svc := NewPipelineService(baseDir, baseDir, nil)
	sessions, err := svc.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	// Sorted by trading date DESC: 22, 21, 20. Input written in order [20, 22, 21] → sorted OutcomeCount [2, 3, 1].
	wantOrder := []string{"session-20260422-daily", "session-20260421-daily", "session-20260420-daily"}
	wantOCs := []int{2, 3, 1}
	for i, s := range sessions {
		if s.SessionID != wantOrder[i] {
			t.Errorf("position %d: got %q, want %q", i, s.SessionID, wantOrder[i])
		}
		if s.OutcomeCount != wantOCs[i] {
			t.Errorf("position %d: OutcomeCount = %d, want %d", i, s.OutcomeCount, wantOCs[i])
		}
	}
}

func TestLoadSessions_OrphanSession_FallbackOutcomeCountFromJSONL(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260614-daily"
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "agent-1", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 88, PassedGuards: true, RecordedAt: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)},
		{AgentID: "agent-2", Symbol: "2454.TW", Side: domain.SideBuy, Conviction: 73, PassedGuards: true, RecordedAt: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)},
		{AgentID: "agent-3", Symbol: "3008.TW", Side: domain.SideBuy, Conviction: 95, PassedGuards: true, RecordedAt: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC)},
	}
	var buf []byte
	for _, o := range outcomes {
		b, _ := json.Marshal(o)
		buf = append(buf, b...)
		buf = append(buf, '\n')
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), buf, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewPipelineService(baseDir, baseDir, nil)
	sessions, err := svc.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].OutcomeCount != 3 {
		t.Errorf("orphan session OutcomeCount = %d, want 3 (fallback from outcomes.jsonl)", sessions[0].OutcomeCount)
	}
}

func TestLoadSessions_OrphanSession_EmptyOutcomesKeepsZero(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260615-daily"
	if err := os.MkdirAll(filepath.Join(baseDir, "sessions", sessionID), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewPipelineService(baseDir, baseDir, nil)
	sessions, err := svc.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].OutcomeCount != 0 {
		t.Errorf("empty orphan OutcomeCount = %d, want 0", sessions[0].OutcomeCount)
	}
}

func TestLoadSessions_SummaryAuthoritativeNotOverriddenByOutcomes(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260616-daily"
	writeTestSessionArtifacts(
		t, baseDir, sessionID,
		domain.SessionSummary{SessionID: sessionID, Regime: domain.RegimeRiskOn, RecordedAt: time.Date(2026, 6, 16, 4, 0, 0, 0, time.UTC), OutcomeCount: 10},
		domain.RecommendationOutcome{AgentID: "agent-1", Symbol: "2330.TW", RecordedAt: time.Date(2026, 6, 16, 4, 0, 0, 0, time.UTC)},
	)

	svc := NewPipelineService(baseDir, baseDir, nil)
	sessions, err := svc.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if sessions[0].OutcomeCount != 10 {
		t.Errorf("summary.OutcomeCount should be authoritative, got %d, want 10", sessions[0].OutcomeCount)
	}
}

func TestLoadSessions_FallbackToSessionIDDate(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260422-daily"
	// Write a session dir without a summary.json → RecordedAt falls back to sessionID date.
	if err := os.MkdirAll(filepath.Join(baseDir, "sessions", sessionID), 0o755); err != nil {
		t.Fatal(err)
	}
	svc := NewPipelineService(baseDir, baseDir, nil)
	sessions, err := svc.LoadSessions()
	if err != nil {
		t.Fatalf("LoadSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != sessionID {
		t.Errorf("SessionID = %q", sessions[0].SessionID)
	}
	if sessions[0].RecordedAt.IsZero() {
		t.Error("RecordedAt should fall back to sessionID date, not zero")
	}
}

// =============================================================================
// LoadDarwinianHistory
// =============================================================================

func TestLoadDarwinianHistory_NoFile(t *testing.T) {
	baseDir := t.TempDir()
	svc := NewPipelineService(baseDir, baseDir, nil)
	points, err := svc.LoadDarwinianHistory(10)
	if err != nil {
		t.Fatalf("LoadDarwinianHistory with no file: %v", err)
	}
	if points == nil {
		t.Error("expected non-nil empty slice on missing file, got nil")
	}
	if len(points) != 0 {
		t.Errorf("expected 0 points, got %d", len(points))
	}
}

func TestLoadDarwinianHistory_ReadsAndLimits(t *testing.T) {
	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write 3 history lines, oldest first (file order); reader iterates reverse.
	lines := []string{
		`{"timestamp":"2026-04-20T04:00:00Z","weights":{"agent-a":{"weight":0.9,"rolling_sharpe":1.0,"hit_rate":0.5}}}`,
		`{"timestamp":"2026-04-21T04:00:00Z","weights":{"agent-a":{"weight":0.95,"rolling_sharpe":1.1,"hit_rate":0.55},"agent-b":{"weight":1.0,"rolling_sharpe":0.8,"hit_rate":0.45}}}`,
		`{"timestamp":"2026-04-22T04:00:00Z","weights":{"agent-b":{"weight":1.1,"rolling_sharpe":0.9,"hit_rate":0.5}}}`,
	}
	if err := os.WriteFile(filepath.Join(baseDir, "darwinian_history.jsonl"),
		[]byte(lines[0]+"\n"+lines[1]+"\n"+lines[2]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewPipelineService(baseDir, baseDir, nil)
	points, err := svc.LoadDarwinianHistory(10)
	if err != nil {
		t.Fatalf("LoadDarwinianHistory: %v", err)
	}
	if len(points) != 4 {
		t.Fatalf("expected 4 points (3 entries: 1+2+1 weights), got %d", len(points))
	}
	// First point should be the latest (reverse iteration).
	if points[0].Timestamp != "2026-04-22T04:00:00Z" {
		t.Errorf("first point timestamp = %q, want latest", points[0].Timestamp)
	}
	if points[0].AgentID != "agent-b" {
		t.Errorf("first point agent = %q, want agent-b", points[0].AgentID)
	}
}

func TestLoadDarwinianHistory_LimitRespected(t *testing.T) {
	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// 5 lines × 1 weight each = 5 points; limit=2 → only 2 returned.
	var lines []string
	for i := 0; i < 5; i++ {
		lines = append(lines,
			`{"timestamp":"2026-04-2`+string(rune('0'+i))+`T04:00:00Z","weights":{"a":{"weight":1.0,"rolling_sharpe":1.0,"hit_rate":0.5}}}`)
	}
	content := ""
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(filepath.Join(baseDir, "darwinian_history.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewPipelineService(baseDir, baseDir, nil)
	points, err := svc.LoadDarwinianHistory(2)
	if err != nil {
		t.Fatalf("LoadDarwinianHistory: %v", err)
	}
	if len(points) != 2 {
		t.Errorf("expected 2 points (limit), got %d", len(points))
	}
}

func TestLoadDarwinianHistory_CorruptedLinesSkipped(t *testing.T) {
	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `not valid json
{"timestamp":"2026-04-22T04:00:00Z","weights":{"agent-a":{"weight":1.0,"rolling_sharpe":1.0,"hit_rate":0.5}}}
also not valid
`
	if err := os.WriteFile(filepath.Join(baseDir, "darwinian_history.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewPipelineService(baseDir, baseDir, nil)
	points, err := svc.LoadDarwinianHistory(10)
	if err != nil {
		t.Fatalf("LoadDarwinianHistory: %v", err)
	}
	if len(points) != 1 {
		t.Errorf("expected 1 valid point, got %d", len(points))
	}
	if len(points) > 0 && points[0].AgentID != "agent-a" {
		t.Errorf("AgentID = %q", points[0].AgentID)
	}
}

// =============================================================================
// LoadDarwinianStatus
// =============================================================================

func TestLoadDarwinianStatus_NoFile(t *testing.T) {
	baseDir := t.TempDir()
	svc := NewPipelineService(baseDir, baseDir, nil)
	data, err := svc.LoadDarwinianStatus()
	if err != nil {
		t.Fatalf("LoadDarwinianStatus with no file: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if data.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", data.Status)
	}
	if data.AgentCount != 0 {
		t.Errorf("AgentCount = %d, want 0", data.AgentCount)
	}
}

func TestLoadDarwinianStatus_Valid(t *testing.T) {
	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := map[string]any{
		"saved_at": "2026-04-22T04:00:00Z",
		"weights": map[string]any{
			"agent-a": map[string]any{
				"weight": 1.2, "rolling_sharpe": 1.5, "hit_rate": 0.6,
				"total_signals": 100, "win_count": 60, "loss_count": 40,
				"avg_return": 0.02, "last_updated_at": "2026-04-22T03:55:00Z",
			},
		},
	}
	data, _ := json.Marshal(payload)
	if err := os.WriteFile(filepath.Join(baseDir, "darwinian_weights.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewPipelineService(baseDir, baseDir, nil)
	status, err := svc.LoadDarwinianStatus()
	if err != nil {
		t.Fatalf("LoadDarwinianStatus: %v", err)
	}
	if status == nil {
		t.Fatal("expected non-nil status")
	}
	if status.Status != "ok" {
		t.Errorf("Status = %q, want ok", status.Status)
	}
	if status.LastComputed != "2026-04-22T04:00:00Z" {
		t.Errorf("LastComputed = %q", status.LastComputed)
	}
	if status.AgentCount != 1 {
		t.Errorf("AgentCount = %d, want 1", status.AgentCount)
	}
	agent, ok := status.Agents["agent-a"]
	if !ok {
		t.Fatal("agent-a missing from Agents")
	}
	if agent.Weight != 1.2 {
		t.Errorf("Weight = %v, want 1.2", agent.Weight)
	}
	if agent.RollingSharpe != 1.5 {
		t.Errorf("RollingSharpe = %v, want 1.5", agent.RollingSharpe)
	}
	if agent.HitRate != 0.6 {
		t.Errorf("HitRate = %v, want 0.6", agent.HitRate)
	}
	if agent.TotalSignals != 100 {
		t.Errorf("TotalSignals = %d, want 100", agent.TotalSignals)
	}
	if agent.WinCount != 60 {
		t.Errorf("WinCount = %d, want 60", agent.WinCount)
	}
	if agent.LastUpdated != "2026-04-22T03:55:00Z" {
		t.Errorf("LastUpdated = %q", agent.LastUpdated)
	}
}

func TestLoadDarwinianStatus_MalformedJSON(t *testing.T) {
	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "data", "state")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "darwinian_weights.json"),
		[]byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewPipelineService(baseDir, baseDir, nil)
	_, err := svc.LoadDarwinianStatus()
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestLoadDarwinianStatus_StatusLabels(t *testing.T) {
	baseDir := t.TempDir()
	stateDir := filepath.Join(baseDir, "data", "state")
	cfgDir := filepath.Join(baseDir, "configs")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	registry := map[string]any{
		"version": 1,
		"agents": []map[string]any{
			{"id": "registered-zero"},
			{"id": "active-agent"},
		},
	}
	regData, _ := json.Marshal(registry)
	if err := os.WriteFile(filepath.Join(cfgDir, "agents.json"), regData, 0o644); err != nil {
		t.Fatal(err)
	}

	weights := map[string]any{
		"saved_at": "2026-04-22T04:00:00Z",
		"weights": map[string]any{
			"registered-zero": map[string]any{
				"weight": 1.0, "rolling_sharpe": 0.0, "hit_rate": 0.0,
				"total_signals": 0, "win_count": 0, "loss_count": 0,
				"avg_return": 0.0, "last_updated_at": "2026-04-22T03:55:00Z",
			},
			"ghost-agent": map[string]any{
				"weight": 1.2, "rolling_sharpe": 0.0, "hit_rate": 0.0,
				"total_signals": 0, "win_count": 0, "loss_count": 0,
				"avg_return": 0.0, "last_updated_at": "2026-04-22T03:55:00Z",
			},
			"active-agent": map[string]any{
				"weight": 1.5, "rolling_sharpe": 2.0, "hit_rate": 0.6,
				"total_signals": 10, "win_count": 6, "loss_count": 4,
				"avg_return": 0.02, "last_updated_at": "2026-04-22T03:55:00Z",
			},
			"legacy-agent": map[string]any{
				"weight": 0.9, "rolling_sharpe": 0.0, "hit_rate": 0.0,
				"win_count": 0, "loss_count": 0,
				"avg_return": 0.0, "last_updated_at": "2026-04-22T03:55:00Z",
			},
		},
	}
	data, _ := json.Marshal(weights)
	if err := os.WriteFile(filepath.Join(baseDir, "darwinian_weights.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewPipelineService(baseDir, baseDir, nil)
	status, err := svc.LoadDarwinianStatus()
	if err != nil {
		t.Fatalf("LoadDarwinianStatus: %v", err)
	}

	cases := map[string]string{
		"registered-zero": "dormant",
		"ghost-agent":     "ghost",
		"active-agent":    "active",
		"legacy-agent":    "ghost",
	}
	for id, want := range cases {
		a, ok := status.Agents[id]
		if !ok {
			t.Fatalf("agent %s missing from response", id)
		}
		if a.Status != want {
			t.Errorf("agent %s Status = %q, want %q", id, a.Status, want)
		}
	}
}

// =============================================================================
// LoadRegimeHistory: uses OutcomeStore.LoadSessionSummaries
// =============================================================================

// mockOutcomeStore implements ledger.OutcomeStore minimally for LoadRegimeHistory.
type mockOutcomeStore struct {
	summaries []domain.SessionSummary
	err       error
}

func (m *mockOutcomeStore) RecordOutcomes(_ []domain.RecommendationOutcome) error {
	return nil
}

func (m *mockOutcomeStore) RecordSessionOutcomes(_ domain.ReplaySession, _ []domain.RecommendationOutcome) error {
	return nil
}

func (m *mockOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (m *mockOutcomeStore) LoadSessionOutcomes(_ string) ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (m *mockOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (m *mockOutcomeStore) RecordSessionScreeningRejects(_ string, _ []domain.ScreeningReject) error {
	return nil
}

func (m *mockOutcomeStore) LoadSessionScreeningRejects(_ string) ([]domain.ScreeningReject, error) {
	return nil, nil
}

func (m *mockOutcomeStore) RecordSessionTrades(_ string, _ []domain.TradeRecord) error {
	return nil
}

func (m *mockOutcomeStore) LoadSessionTrades(_ string) ([]domain.TradeRecord, error) {
	return nil, nil
}

func (m *mockOutcomeStore) LoadAllSessionTrades() ([]domain.TradeRecord, error) {
	return nil, nil
}
func (m *mockOutcomeStore) RecordExperiment(_ domain.ExperimentRecord) error { return nil }
func (m *mockOutcomeStore) RecordSessionExperiment(_ domain.ReplaySession, _ domain.ExperimentRecord) error {
	return nil
}

func (m *mockOutcomeStore) RecordSessionSummary(_ domain.ReplaySession, _ domain.SessionSummary) error {
	return nil
}

func (m *mockOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return m.summaries, m.err
}

func (m *mockOutcomeStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}
func (m *mockOutcomeStore) RecordHumanIntervention(_ domain.HumanIntervention) error { return nil }
func (m *mockOutcomeStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	return nil, nil
}

func TestLoadRegimeHistory_StoreError(t *testing.T) {
	svc := NewPipelineService("/tmp", "/tmp", &mockOutcomeStore{
		err: errors.New("ledger unavailable"),
	})
	_, err := svc.LoadRegimeHistory(10)
	if err == nil {
		t.Fatal("expected error from store, got nil")
	}
}

func TestLoadRegimeHistory_Empty(t *testing.T) {
	svc := NewPipelineService("/tmp", "/tmp", &mockOutcomeStore{})
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if len(data.Sessions) != 0 {
		t.Errorf("Sessions len = %d, want 0", len(data.Sessions))
	}
	if len(data.Transitions) != 0 {
		t.Errorf("Transitions len = %d, want 0", len(data.Transitions))
	}
	if data.Current != "" {
		t.Errorf("Current = %q, want empty", data.Current)
	}
}

func TestLoadRegimeHistory_DetectsTransitions(t *testing.T) {
	now := time.Date(2026, 4, 22, 4, 0, 0, 0, time.UTC)
	summaries := []domain.SessionSummary{
		{SessionID: "session-20260420-daily", Regime: domain.RegimeRiskOn, RecordedAt: now.AddDate(0, 0, -2)},
		{SessionID: "session-20260421-daily", Regime: domain.RegimeRiskOn, RecordedAt: now.AddDate(0, 0, -1)},
		{SessionID: "session-20260422-daily", Regime: domain.RegimeRiskOff, RecordedAt: now},
		{SessionID: "session-20260423-daily", Regime: domain.RegimeNeutral, RecordedAt: now.AddDate(0, 0, 1)},
	}
	svc := NewPipelineService("/tmp", "/tmp", &mockOutcomeStore{summaries: summaries})
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if len(data.Sessions) != 4 {
		t.Errorf("Sessions len = %d, want 4", len(data.Sessions))
	}
	// 3 transitions: 21→22 (risk_on→risk_off), 22→23 (risk_off→transition).
	// (20→21 same regime, no transition.)
	if len(data.Transitions) != 2 {
		t.Fatalf("Transitions len = %d, want 2 (got: %+v)", len(data.Transitions), data.Transitions)
	}
	if data.Transitions[0].From != string(domain.RegimeRiskOn) || data.Transitions[0].To != string(domain.RegimeRiskOff) {
		t.Errorf("first transition: %+v", data.Transitions[0])
	}
	if data.Current != string(domain.RegimeNeutral) {
		t.Errorf("Current = %q, want %q", data.Current, domain.RegimeNeutral)
	}
}

func TestLoadRegimeHistory_LimitRespected(t *testing.T) {
	now := time.Date(2026, 4, 22, 4, 0, 0, 0, time.UTC)
	summaries := []domain.SessionSummary{
		{SessionID: "session-20260420-daily", Regime: domain.RegimeRiskOn, RecordedAt: now.AddDate(0, 0, -2)},
		{SessionID: "session-20260421-daily", Regime: domain.RegimeRiskOn, RecordedAt: now.AddDate(0, 0, -1)},
		{SessionID: "session-20260422-daily", Regime: domain.RegimeRiskOn, RecordedAt: now},
	}
	svc := NewPipelineService("/tmp", "/tmp", &mockOutcomeStore{summaries: summaries})
	data, err := svc.LoadRegimeHistory(2)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if len(data.Sessions) != 2 {
		t.Errorf("Sessions len = %d, want 2 (limit)", len(data.Sessions))
	}
	// Last 2 summaries kept: 20260421, 20260422.
	if data.Sessions[0].SessionID != "session-20260421-daily" {
		t.Errorf("first session = %q", data.Sessions[0].SessionID)
	}
	if data.Sessions[1].SessionID != "session-20260422-daily" {
		t.Errorf("second session = %q", data.Sessions[1].SessionID)
	}
}

func TestLoadRegimeHistory_TimestampsAreUTC(t *testing.T) {
	cst := time.FixedZone("CST", 8*3600)
	recordedAt := time.Date(2026, 4, 22, 12, 0, 0, 0, cst)
	summaries := []domain.SessionSummary{
		{SessionID: "session-cst", Regime: domain.RegimeRiskOn, RecordedAt: recordedAt},
		{
			SessionID: "session-utc", Regime: domain.RegimeRiskOff,
			RecordedAt: time.Date(2026, 4, 23, 4, 0, 0, 0, time.UTC),
		},
	}
	svc := NewPipelineService("/tmp", "/tmp", &mockOutcomeStore{summaries: summaries})
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	for i, sess := range data.Sessions {
		parsed, parseErr := time.Parse(time.RFC3339, sess.RecordedAt)
		if parseErr != nil {
			t.Fatalf("session %d: not RFC3339-parseable: %v", i, parseErr)
		}
		if parsed.Location().String() != "UTC" {
			t.Errorf("session %d: timezone %s, want UTC (raw=%s)",
				i, parsed.Location().String(), sess.RecordedAt)
		}
	}
	if data.Transitions != nil {
		for i, tr := range data.Transitions {
			parsed, parseErr := time.Parse(time.RFC3339, tr.Timestamp)
			if parseErr != nil {
				t.Fatalf("transition %d: not RFC3339-parseable: %v", i, parseErr)
			}
			if parsed.Location().String() != "UTC" {
				t.Errorf("transition %d: timezone %s, want UTC (raw=%s)",
					i, parsed.Location().String(), tr.Timestamp)
			}
		}
	}
}

// =============================================================================
// LoadRegimeHistory: HistoricalStore path (CL-3 A01)
// =============================================================================

// mockHistoricalStore implements ledger.HistoricalStore minimally for the
// LoadRegimeHistory SQLite-path test. Other methods panic if invoked — they
// are intentionally not used by the code under test.
type mockHistoricalStore struct {
	rows []ledger.RegimeRow
	err  error
}

func (m *mockHistoricalStore) UpsertRegime(_ context.Context, _ ledger.RegimeRow) error {
	panic("mockHistoricalStore: UpsertRegime not implemented")
}

func (m *mockHistoricalStore) LoadRegimeByDate(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	panic("mockHistoricalStore: LoadRegimeByDate not implemented")
}

func (m *mockHistoricalStore) LoadRegimeByDateAll(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	panic("mockHistoricalStore: LoadRegimeByDateAll not implemented")
}

func (m *mockHistoricalStore) LoadRegimeHistory(_ context.Context, limit int) ([]ledger.RegimeRow, error) {
	if m.err != nil {
		return nil, m.err
	}
	if limit > 0 && len(m.rows) > limit {
		return m.rows[:limit], nil
	}
	return m.rows, nil
}

func (m *mockHistoricalStore) LoadRegimeHistoryAll(_ context.Context, _ int) ([]ledger.RegimeRow, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.rows, nil
}

func (m *mockHistoricalStore) UpsertStress(_ context.Context, _ ledger.StressRow) error {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadStressByDate(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadStressByDateAll(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadStressHistory(_ context.Context, _ int) ([]ledger.StressRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadStressHistoryAll(_ context.Context, _ int) ([]ledger.StressRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) UpsertGeopolitical(_ context.Context, _ ledger.GeopoliticalRow) error {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadGeopoliticalByDate(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadGeopoliticalByDateAll(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadGeopoliticalHistory(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadGeopoliticalHistoryAll(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) UpsertEventCalendar(_ context.Context, _ ledger.EventCalendarRow) error {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadEventCalendarByDate(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadEventCalendarByDateAll(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadEventCalendarRange(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadEventCalendarRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) UpsertPredictionBacktest(_ context.Context, _ ledger.PredictionBacktestRow) error {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadPredictionBacktestRange(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) LoadPredictionBacktestRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	panic("not implemented")
}

func (m *mockHistoricalStore) CountSynthetic(_ context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

func (m *mockHistoricalStore) UpsertPeriod(_ context.Context, _ ledger.PeriodRow) error {
	return nil
}

func (m *mockHistoricalStore) LoadPeriodByDate(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockHistoricalStore) LoadPeriodByDateAll(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}

func (m *mockHistoricalStore) LoadPeriodHistory(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

func (m *mockHistoricalStore) LoadPeriodHistoryAll(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return nil, nil
}

func (m *mockHistoricalStore) Close() error {
	return nil
}

func TestLoadRegimeHistory_HistoricalStore_OK(t *testing.T) {
	rows := []ledger.RegimeRow{
		{Date: "2026-06-29", Regime: "RISK_OFF", RecordedAt: time.Date(2026, 6, 29, 6, 0, 0, 0, time.UTC)},
		{Date: "2026-06-28", Regime: "NEUTRAL", RecordedAt: time.Date(2026, 6, 28, 6, 0, 0, 0, time.UTC)},
		{Date: "2026-06-27", Regime: "TRANSITIONAL", RecordedAt: time.Date(2026, 6, 27, 6, 0, 0, 0, time.UTC)},
	}
	svc := NewPipelineService("/tmp", "/tmp", nil).
		WithHistoricalStore(&mockHistoricalStore{rows: rows})
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if len(data.Sessions) != 3 {
		t.Fatalf("Sessions len = %d, want 3", len(data.Sessions))
	}
	if data.Current != "RISK_OFF" {
		t.Errorf("Current = %q, want RISK_OFF (latest by RecordedAt)", data.Current)
	}
	if len(data.Transitions) != 2 {
		t.Errorf("Transitions len = %d, want 2 (NEUTRAL->TRANSITIONAL, TRANSITIONAL->RISK_OFF)", len(data.Transitions))
	}
}

func TestLoadRegimeHistory_HistoricalStore_StoreError(t *testing.T) {
	svc := NewPipelineService("/tmp", "/tmp", nil).
		WithHistoricalStore(&mockHistoricalStore{err: errors.New("sqlite unavailable")})
	_, err := svc.LoadRegimeHistory(10)
	if err == nil {
		t.Fatal("expected error from HistoricalStore, got nil")
	}
}

func TestLoadRegimeHistory_HistoricalStore_Empty(t *testing.T) {
	svc := NewPipelineService("/tmp", "/tmp", nil).
		WithHistoricalStore(&mockHistoricalStore{})
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if len(data.Sessions) != 0 {
		t.Errorf("Sessions len = %d, want 0", len(data.Sessions))
	}
	if data.Current != "" {
		t.Errorf("Current should be empty, got %q", data.Current)
	}
}

// TestLoadRegimeHistoryDays_CalendarWindow covers E6: the `days` parameter
// must be interpreted as a calendar window (today-days+1 .. today), not a
// row limit. Rows outside the window are excluded even if the store returns
// more rows.
func TestLoadRegimeHistoryDays_CalendarWindow(t *testing.T) {
	now := time.Now().UTC()
	date := func(offset int) string { return now.AddDate(0, 0, offset).Format("2006-01-02") }
	rows := []ledger.RegimeRow{
		{Date: date(0), Regime: "RISK_OFF", RecordedAt: now},
		{Date: date(-2), Regime: "NEUTRAL", RecordedAt: now.AddDate(0, 0, -2)},
		{Date: date(-10), Regime: "RISK_ON", RecordedAt: now.AddDate(0, 0, -10)},
	}
	svc := NewPipelineService("/tmp", "/tmp", nil).
		WithHistoricalStore(&mockHistoricalStore{rows: rows})
	data, err := svc.LoadRegimeHistoryDays(5)
	if err != nil {
		t.Fatalf("LoadRegimeHistoryDays: %v", err)
	}
	if len(data.Sessions) != 2 {
		t.Fatalf("Sessions len = %d, want 2 (rows within 5-day window)", len(data.Sessions))
	}
	if data.Sessions[0].Date != date(0) || data.Sessions[1].Date != date(-2) {
		t.Errorf("unexpected session dates: got %+v", data.Sessions)
	}
	if data.Sessions[0].Regime != "RISK_OFF" || data.Sessions[1].Regime != "NEUTRAL" {
		t.Errorf("unexpected regime order: got %+v", data.Sessions)
	}
	if data.Current != "RISK_OFF" {
		t.Errorf("Current = %q, want RISK_OFF", data.Current)
	}
	if len(data.Transitions) != 1 {
		t.Errorf("Transitions len = %d, want 1", len(data.Transitions))
	}
}

// =============================================================================
// extractPipelineMetrics (pure helper)
// =============================================================================

func TestExtractPipelineMetrics_AllFieldsPresent(t *testing.T) {
	pe, pb, dy := 12.5, 1.8, 3.2
	bt := 0.045
	outcome := domain.RecommendationOutcome{
		ForwardReturn: bt,
		FactorScores: shared.FactorScores{
			Breakdown: &shared.FactorScoreBreakdown{
				Value:   shared.FactorScoreItem{RawInputs: map[string]float64{"pe": pe, "pb": pb}},
				Quality: shared.FactorScoreItem{RawInputs: map[string]float64{"dividend_yield": dy}},
			},
		},
	}
	m := extractPipelineMetrics(outcome)
	if m.PriceToEarnings == nil || *m.PriceToEarnings != pe {
		t.Errorf("P/E = %v, want %v", m.PriceToEarnings, pe)
	}
	if m.PriceToBook == nil || *m.PriceToBook != pb {
		t.Errorf("P/B = %v, want %v", m.PriceToBook, pb)
	}
	if m.DividendYield == nil || *m.DividendYield != dy {
		t.Errorf("DividendYield = %v, want %v", m.DividendYield, dy)
	}
	if m.BacktestReturn == nil || *m.BacktestReturn != bt {
		t.Errorf("BacktestReturn = %v, want %v", m.BacktestReturn, bt)
	}
}

func TestExtractPipelineMetrics_MissingBreakdown(t *testing.T) {
	outcome := domain.RecommendationOutcome{ForwardReturn: 0.05}
	m := extractPipelineMetrics(outcome)
	if m.PriceToEarnings != nil || m.PriceToBook != nil || m.DividendYield != nil {
		t.Errorf("expected nil financial fields when Breakdown is nil, got %+v", m)
	}
	if m.BacktestReturn == nil || *m.BacktestReturn != 0.05 {
		t.Errorf("BacktestReturn = %v, want 0.05", m.BacktestReturn)
	}
}

func TestExtractPipelineMetrics_PartialInputs(t *testing.T) {
	pe := 15.0
	outcome := domain.RecommendationOutcome{
		ForwardReturn: 0.01,
		FactorScores: shared.FactorScores{
			Breakdown: &shared.FactorScoreBreakdown{
				Value: shared.FactorScoreItem{RawInputs: map[string]float64{"pe": pe}},
			},
		},
	}
	m := extractPipelineMetrics(outcome)
	if m.PriceToEarnings == nil || *m.PriceToEarnings != pe {
		t.Errorf("P/E = %v", m.PriceToEarnings)
	}
	if m.PriceToBook != nil {
		t.Errorf("P/B should be nil, got %v", *m.PriceToBook)
	}
	if m.DividendYield != nil {
		t.Errorf("DividendYield should be nil, got %v", *m.DividendYield)
	}
}

// =============================================================================
// readOutcomeFile (pure JSONL reader)
// =============================================================================

func TestReadOutcomeFile_NoFile(t *testing.T) {
	_, err := readOutcomeFile(filepath.Join(t.TempDir(), "missing.jsonl"))
	if err != nil {
		t.Errorf("missing file: expected nil error, got %v", err)
	}
}

func TestReadOutcomeFile_ValidAndCorrupt(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "outcomes.jsonl")
	content := `{"agent_id":"a1","symbol":"2330","side":"buy","forward_return":0.05,"hit":true}
not json
{"agent_id":"a2","symbol":"2317","side":"sell","forward_return":-0.02,"hit":false}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readOutcomeFile(path)
	if err != nil {
		t.Fatalf("readOutcomeFile: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 valid outcomes, got %d", len(got))
	}
	if len(got) > 0 && got[0].AgentID != "a1" {
		t.Errorf("first = %q", got[0].AgentID)
	}
}

// =============================================================================
// loadRegistry: graceful degradation when registry missing
// =============================================================================

func TestLoadRegistry_ProviderTakesPrecedence(t *testing.T) {
	svc := NewPipelineService("/tmp", "/tmp", nil)
	called := false
	svc.WithRegistryProvider(func() (domain.AgentRegistry, error) {
		called = true
		return domain.AgentRegistry{}, nil
	})
	_, err := svc.loadRegistry()
	if err != nil {
		t.Fatalf("provider-based load: %v", err)
	}
	if !called {
		t.Error("expected custom provider to be called")
	}
}

func TestLoadRegistry_FallbackSeedsOnMissingConfig(t *testing.T) {
	baseDir := t.TempDir()
	configsDir := filepath.Join(baseDir, "configs")
	if err := os.MkdirAll(configsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configsDir, "agents.json"),
		[]byte(`{"version":1,"agents":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewPipelineService(baseDir, baseDir, nil)
	reg, err := svc.loadRegistry()
	if err != nil {
		t.Fatalf("loadRegistry with empty seed file: %v", err)
	}
	if len(reg.Agents) != 0 {
		t.Errorf("expected empty Agents from empty seed file, got %d", len(reg.Agents))
	}
}

func TestLoadRegistry_ProviderReturnsError(t *testing.T) {
	svc := NewPipelineService("/tmp", "/tmp", nil)
	wantErr := errors.New("provider failed")
	svc.WithRegistryProvider(func() (domain.AgentRegistry, error) {
		return domain.AgentRegistry{}, wantErr
	})
	_, err := svc.loadRegistry()
	if err == nil || !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped provider error, got %v", err)
	}
}

// =============================================================================
// ComputeScorecardMetrics + Provider injection (set/get)
// =============================================================================

func TestPipelineService_ProviderSetters(t *testing.T) {
	svc := NewPipelineService("/tmp", "/tmp", nil)
	if svc.WithRegistryProvider(func() (domain.AgentRegistry, error) { return domain.AgentRegistry{}, nil }) != svc {
		t.Error("WithRegistryProvider should return svc for chaining")
	}
	if svc.WithNarrativeProvider(func(_ []string) *NarrativeContextData { return nil }) != svc {
		t.Error("WithNarrativeProvider should return svc for chaining")
	}
	if svc.WithCycleProvider(func(_ string) *IndustryContextData { return nil }) != svc {
		t.Error("WithCycleProvider should return svc for chaining")
	}
	if svc.WithCycleCardProvider(func() *industry.CycleStatusCard { return nil }) != svc {
		t.Error("WithCycleCardProvider should return svc for chaining")
	}
}

// =============================================================================
// computeAgentRegimeBreakdown (regression + edge cases)
// =============================================================================

func TestComputeAgentRegimeBreakdown(t *testing.T) {
	agentA := "agent-a"
	bull := "bull"
	bear := "bear"
	unknown := "unknown"

	cases := []struct {
		name          string
		outcomes      []domain.RecommendationOutcome
		agentID       string
		defaultRegime string
		wantNil       bool
		wantRegimes   map[string]domain.RegimePerformance
	}{
		{
			name:          "nil outcomes",
			outcomes:      nil,
			agentID:       agentA,
			defaultRegime: bull,
			wantNil:       true,
		},
		{
			name: "no matching agent",
			outcomes: []domain.RecommendationOutcome{
				{AgentID: "other-agent", ForwardReturn: 0.05, Hit: true, Regime: bull},
			},
			agentID:       agentA,
			defaultRegime: bull,
			wantNil:       true,
		},
		{
			name: "single regime from outcome",
			outcomes: []domain.RecommendationOutcome{
				{AgentID: agentA, ForwardReturn: 0.10, Hit: true, Regime: bull},
				{AgentID: agentA, ForwardReturn: -0.05, Hit: false, Regime: bull},
				{AgentID: agentA, ForwardReturn: 0.03, Hit: true, Regime: bull},
				{AgentID: "other", ForwardReturn: 0.99, Hit: true, Regime: bull},
			},
			agentID:       agentA,
			defaultRegime: unknown,
			wantRegimes: map[string]domain.RegimePerformance{
				bull: {
					Regime:       bull,
					SessionCount: 3,
					TotalReturn:  0.10 - 0.05 + 0.03,
					WinRate:      2.0 / 3.0,
					AvgReturn:    (0.10 - 0.05 + 0.03) / 3.0,
				},
			},
		},
		{
			name: "multiple regimes",
			outcomes: []domain.RecommendationOutcome{
				{AgentID: agentA, ForwardReturn: 0.10, Hit: true, Regime: bull},
				{AgentID: agentA, ForwardReturn: -0.05, Hit: false, Regime: bear},
				{AgentID: agentA, ForwardReturn: 0.03, Hit: true, Regime: bull},
				{AgentID: agentA, ForwardReturn: -0.02, Hit: false, Regime: bear},
			},
			agentID:       agentA,
			defaultRegime: unknown,
			wantRegimes: map[string]domain.RegimePerformance{
				bull: {
					Regime:       bull,
					SessionCount: 2,
					TotalReturn:  0.13,
					WinRate:      1.0,
					AvgReturn:    0.065,
				},
				bear: {
					Regime:       bear,
					SessionCount: 2,
					TotalReturn:  -0.07,
					WinRate:      0.0,
					AvgReturn:    -0.035,
				},
			},
		},
		{
			name: "empty regime falls back to default",
			outcomes: []domain.RecommendationOutcome{
				{AgentID: agentA, ForwardReturn: 0.10, Hit: true, Regime: ""},
				{AgentID: agentA, ForwardReturn: -0.05, Hit: false, Regime: ""},
				{AgentID: agentA, ForwardReturn: 0.03, Hit: true, Regime: bull},
			},
			agentID:       agentA,
			defaultRegime: unknown,
			wantRegimes: map[string]domain.RegimePerformance{
				unknown: {
					Regime:       unknown,
					SessionCount: 2,
					TotalReturn:  0.05,
					WinRate:      0.5,
					AvgReturn:    0.025,
				},
				bull: {
					Regime:       bull,
					SessionCount: 1,
					TotalReturn:  0.03,
					WinRate:      1.0,
					AvgReturn:    0.03,
				},
			},
		},
		{
			name: "all outcomes default regime when no outcome regime",
			outcomes: []domain.RecommendationOutcome{
				{AgentID: agentA, ForwardReturn: 0.10, Hit: true, Regime: ""},
				{AgentID: agentA, ForwardReturn: -0.05, Hit: false, Regime: ""},
				{AgentID: agentA, ForwardReturn: 0.03, Hit: true, Regime: ""},
			},
			agentID:       agentA,
			defaultRegime: bull,
			wantRegimes: map[string]domain.RegimePerformance{
				bull: {
					Regime:       bull,
					SessionCount: 3,
					TotalReturn:  0.08,
					WinRate:      2.0 / 3.0,
					AvgReturn:    0.08 / 3.0,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeAgentRegimeBreakdown(tc.outcomes, tc.agentID, tc.defaultRegime)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil breakdown")
			}
			if len(got.Regimes) != len(tc.wantRegimes) {
				t.Fatalf("regime count = %d, want %d", len(got.Regimes), len(tc.wantRegimes))
			}
			for regime, want := range tc.wantRegimes {
				perf, ok := got.Regimes[regime]
				if !ok {
					t.Fatalf("missing regime %q", regime)
				}
				if perf.Regime != want.Regime {
					t.Errorf("Regime = %q, want %q", perf.Regime, want.Regime)
				}
				if perf.SessionCount != want.SessionCount {
					t.Errorf("SessionCount = %d, want %d", perf.SessionCount, want.SessionCount)
				}
				if math.Abs(perf.TotalReturn-want.TotalReturn) > 1e-9 {
					t.Errorf("TotalReturn = %v, want %v", perf.TotalReturn, want.TotalReturn)
				}
				if math.Abs(perf.WinRate-want.WinRate) > 1e-9 {
					t.Errorf("WinRate = %v, want %v", perf.WinRate, want.WinRate)
				}
				if math.Abs(perf.AvgReturn-want.AvgReturn) > 1e-9 {
					t.Errorf("AvgReturn = %v, want %v", perf.AvgReturn, want.AvgReturn)
				}
			}
		})
	}
}

func TestComputeAgentRegimeBreakdown_PerOutcomeRegime(t *testing.T) {
	outcomes := []domain.RecommendationOutcome{
		{AgentID: "target", Regime: string(domain.RegimeRiskOn), ForwardReturn: 0.10, Hit: true},
		{AgentID: "target", Regime: string(domain.RegimeRiskOn), ForwardReturn: -0.02, Hit: false},
		{AgentID: "target", Regime: string(domain.RegimeRiskOff), ForwardReturn: 0.04, Hit: true},
		{AgentID: "target", Regime: "", ForwardReturn: 0.05, Hit: true},                         // fallback regime
		{AgentID: "other", Regime: string(domain.RegimeRiskOn), ForwardReturn: 0.99, Hit: true}, // ignored
	}
	got := computeAgentRegimeBreakdown(outcomes, "target", "default")
	if got == nil {
		t.Fatal("expected non-nil breakdown")
	}
	if len(got.Regimes) != 3 {
		t.Fatalf("expected 3 regime entries, got %d", len(got.Regimes))
	}

	on, ok := got.Regimes[string(domain.RegimeRiskOn)]
	if !ok {
		t.Fatal("expected 'RISK_ON' regime entry")
	}
	if on.SessionCount != 2 {
		t.Errorf("RISK_ON SessionCount = %d, want 2", on.SessionCount)
	}
	wantReturnRiskOn := 0.10 - 0.02
	if on.TotalReturn < wantReturnRiskOn-1e-9 || on.TotalReturn > wantReturnRiskOn+1e-9 {
		t.Errorf("RISK_ON TotalReturn = %v, want %v", on.TotalReturn, wantReturnRiskOn)
	}

	off, ok := got.Regimes[string(domain.RegimeRiskOff)]
	if !ok {
		t.Fatal("expected 'RISK_OFF' regime entry")
	}
	if off.SessionCount != 1 {
		t.Errorf("RISK_OFF SessionCount = %d, want 1", off.SessionCount)
	}
	if off.TotalReturn < 0.04-1e-9 || off.TotalReturn > 0.04+1e-9 {
		t.Errorf("RISK_OFF TotalReturn = %v, want 0.04", off.TotalReturn)
	}

	fallback, ok := got.Regimes["default"]
	if !ok {
		t.Fatal("expected 'default' regime entry for empty Regime fallback")
	}
	if fallback.SessionCount != 1 {
		t.Errorf("default SessionCount = %d, want 1", fallback.SessionCount)
	}
	if fallback.TotalReturn < 0.05-1e-9 || fallback.TotalReturn > 0.05+1e-9 {
		t.Errorf("default TotalReturn = %v, want 0.05", fallback.TotalReturn)
	}
}

// =============================================================================
// Smoke test: ensure coverage tools see the new tests run.
// (No assertions; this is a marker that the test file was compiled and run.)
// =============================================================================

func TestLoadRecommendationPipeline_DegradedWhenSummaryMissing(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260614-daily"
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	outcome := domain.RecommendationOutcome{
		AgentID:      "agent-1",
		Skill:        "growth_momentum",
		Layer:        domain.LayerStyle,
		Symbol:       "2330.TW",
		Side:         domain.SideBuy,
		Conviction:   88,
		PassedGuards: true,
	}
	outcomeBytes, err := json.Marshal(outcome)
	if err != nil {
		t.Fatalf("marshal outcome: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "recommendation_outcomes.jsonl"), append(outcomeBytes, '\n'), 0o644); err != nil {
		t.Fatalf("write recommendation_outcomes.jsonl: %v", err)
	}

	svc := NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	data, err := svc.LoadRecommendationPipeline(sessionID, true)
	if err != nil {
		t.Fatalf("load recommendation pipeline: %v", err)
	}
	if data == nil {
		t.Fatalf("expected pipeline data")
	}
	if data.Status != PipelineStatusDegraded {
		t.Errorf("expected Status=%q, got %q", PipelineStatusDegraded, data.Status)
	}
	if data.StatusMessage == "" {
		t.Error("expected non-empty StatusMessage when degraded")
	}
	if len(data.Items) != 1 {
		t.Errorf("expected 1 item (outcomes still render when degraded), got %d", len(data.Items))
	}
}

func TestLoadRecommendationPipeline_MinimalWhenNoOutcomes(t *testing.T) {
	baseDir := t.TempDir()
	sessionID := "session-20260614-daily"
	sessionDir := filepath.Join(baseDir, "sessions", sessionID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	summary := domain.SessionSummary{
		SessionID: sessionID,
		Regime:    domain.RegimeRiskOn,
	}
	summaryBytes, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "summary.json"), summaryBytes, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	svc := NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	data, err := svc.LoadRecommendationPipeline(sessionID, true)
	if err != nil {
		t.Fatalf("load recommendation pipeline: %v", err)
	}
	if data.Status != PipelineStatusMinimal {
		t.Errorf("expected Status=%q when outcomes JSONL missing, got %q", PipelineStatusMinimal, data.Status)
	}
}

func TestLoadRecommendationPipeline_NoSession(t *testing.T) {
	baseDir := t.TempDir()

	svc := NewPipelineService(baseDir, baseDir, ledger.NewStore(baseDir))
	data, err := svc.LoadRecommendationPipeline("", true)
	if err != nil {
		t.Fatalf("expected no error for empty sessions dir, got: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data even with no sessions")
	}
	if data.Status != PipelineStatusNoSession {
		t.Errorf("expected Status=%q when no sessions exist, got %q", PipelineStatusNoSession, data.Status)
	}
}

func TestPipelineService_LoadUniverseOverlap_WithSeedRegistry(t *testing.T) {
	registry := orchestrator.SeedRegistry()
	provider := func() (domain.AgentRegistry, error) { return registry, nil }

	svc := NewPipelineService("/tmp/nonexistent", "/tmp/nonexistent", nil)
	svc.registryProvider = provider

	data, err := svc.LoadUniverseOverlap()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil data")
	}
	if len(data.Agents) == 0 {
		t.Fatal("expected at least one agent from SeedRegistry")
	}
	if data.Matrix == nil {
		t.Fatal("expected non-nil matrix")
	}
}

func TestPipelineService_LoadUniverseOverlap_OverlapCalculation(t *testing.T) {
	agentA := recommendation.AgentSpec{
		ID:       "agent-a",
		Name:     "Agent A",
		Layer:    domain.LayerSector,
		Enabled:  true,
		Universe: []string{"2330", "2317", "2454"},
	}
	agentB := recommendation.AgentSpec{
		ID:       "agent-b",
		Name:     "Agent B",
		Layer:    domain.LayerStyle,
		Enabled:  true,
		Universe: []string{"2330", "2454", "2498"},
	}
	registry := domain.AgentRegistry{Agents: []recommendation.AgentSpec{agentA, agentB}}
	provider := func() (domain.AgentRegistry, error) { return registry, nil }

	svc := NewPipelineService("/tmp/nonexistent", "/tmp/nonexistent", nil)
	svc.registryProvider = provider

	data, err := svc.LoadUniverseOverlap()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if data.Matrix["agent-a"]["agent-b"] != 2 {
		t.Errorf("expected overlap=2 between agent-a and agent-b, got %d", data.Matrix["agent-a"]["agent-b"])
	}
	if data.Matrix["agent-b"]["agent-a"] != 2 {
		t.Errorf("expected overlap=2 between agent-b and agent-a, got %d", data.Matrix["agent-b"]["agent-a"])
	}
	if _, ok := data.Matrix["agent-a"]["agent-a"]; ok {
		t.Error("diagonal entry agent-a->agent-a should be absent")
	}
}

func TestPipelineService_LoadUniverseOverlap_Warnings(t *testing.T) {
	agentA := recommendation.AgentSpec{
		ID:       "context-agent",
		Name:     "Context Agent",
		Layer:    domain.LayerContext,
		Enabled:  true,
		Universe: []string{},
	}
	agentB := recommendation.AgentSpec{
		ID:       "sector-agent",
		Name:     "Sector Agent",
		Layer:    domain.LayerSector,
		Enabled:  true,
		Universe: []string{"2330", "2317", "2454"},
	}
	registry := domain.AgentRegistry{Agents: []recommendation.AgentSpec{agentA, agentB}}
	provider := func() (domain.AgentRegistry, error) { return registry, nil }

	svc := NewPipelineService("/tmp/nonexistent", "/tmp/nonexistent", nil)
	svc.registryProvider = provider

	data, err := svc.LoadUniverseOverlap()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	for _, w := range data.Warnings {
		t.Logf("warning: %s", w)
	}
}

func TestPipelineService_LoadUniverseOverlap_SmallUniverse(t *testing.T) {
	agent := recommendation.AgentSpec{
		ID:       "tiny-agent",
		Name:     "Tiny Agent",
		Layer:    domain.LayerSector,
		Enabled:  true,
		Universe: []string{"2330"},
	}
	registry := domain.AgentRegistry{Agents: []recommendation.AgentSpec{agent}}
	provider := func() (domain.AgentRegistry, error) { return registry, nil }

	svc := NewPipelineService("/tmp/nonexistent", "/tmp/nonexistent", nil)
	svc.registryProvider = provider

	data, err := svc.LoadUniverseOverlap()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(data.Warnings) == 0 {
		t.Error("expected warning for universe < 3 symbols")
	}
}

func TestPipelineService_LoadUniverseOverlap_RegistryError_FallsBackToSeed(t *testing.T) {
	called := false
	provider := func() (domain.AgentRegistry, error) {
		called = true
		return domain.AgentRegistry{}, errors.New("registry load failed")
	}

	svc := NewPipelineService("/tmp/nonexistent", "/tmp/nonexistent", nil)
	svc.registryProvider = provider

	data, err := svc.LoadUniverseOverlap()
	if err != nil {
		t.Fatalf("expected no error on fallback, got: %v", err)
	}
	if !called {
		t.Error("expected registry provider to be called")
	}
	if len(data.Agents) == 0 {
		t.Fatal("expected non-empty agents from SeedRegistry fallback")
	}
}

// =============================================================================
// LoadRegimeHistory: period sourced from period_history (Hermes MCP fix)
// =============================================================================
//
// These tests guard the contract change: buildRegimeHistoryData reads
// period/period_name_zh from period_history (PeriodDetector truth), NOT
// from RegimeToPeriod(regime). The previous behavior produced fabricated
// values whenever regime and period disagreed (e.g. 2026-07-29: regime=
// RISK_ON, real period=consolidation). The new contract:
//   - period_history has a row for date D     → Period = p.Period
//   - period_history missing D                → Period = "" (no fallback)
//   - current_period for the latest row      → same rule as above
//   - market_period mirrors period (deprecated alias)
//   - source is split into regime_source + period_source (no conflation)

// stubHistoricalStoreWithPeriod is a ledger.HistoricalStore stub that
// serves both regime_history and period_history from in-memory slices.
// All other methods panic — the code under test only uses LoadRegimeHistory
// / LoadRegimeHistoryAll and LoadPeriodHistoryAll.
type stubHistoricalStoreWithPeriod struct {
	regimeRows []ledger.RegimeRow
	periodRows []ledger.PeriodRow
	periodErr  error
	regimeErr  error
}

func (m *stubHistoricalStoreWithPeriod) UpsertRegime(_ context.Context, _ ledger.RegimeRow) error {
	panic("stubHistoricalStoreWithPeriod: UpsertRegime not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadRegimeByDate(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	panic("stubHistoricalStoreWithPeriod: LoadRegimeByDate not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadRegimeByDateAll(_ context.Context, _ string) (ledger.RegimeRow, bool, error) {
	panic("stubHistoricalStoreWithPeriod: LoadRegimeByDateAll not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadRegimeHistory(_ context.Context, limit int) ([]ledger.RegimeRow, error) {
	if m.regimeErr != nil {
		return nil, m.regimeErr
	}
	if limit > 0 && len(m.regimeRows) > limit {
		return m.regimeRows[:limit], nil
	}
	return m.regimeRows, nil
}
func (m *stubHistoricalStoreWithPeriod) LoadRegimeHistoryAll(_ context.Context, _ int) ([]ledger.RegimeRow, error) {
	if m.regimeErr != nil {
		return nil, m.regimeErr
	}
	return m.regimeRows, nil
}
func (m *stubHistoricalStoreWithPeriod) UpsertStress(_ context.Context, _ ledger.StressRow) error {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadStressByDate(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadStressByDateAll(_ context.Context, _ string) (ledger.StressRow, bool, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadStressHistory(_ context.Context, _ int) ([]ledger.StressRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadStressHistoryAll(_ context.Context, _ int) ([]ledger.StressRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) UpsertGeopolitical(_ context.Context, _ ledger.GeopoliticalRow) error {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadGeopoliticalByDate(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadGeopoliticalByDateAll(_ context.Context, _ string) (ledger.GeopoliticalRow, bool, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadGeopoliticalHistory(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadGeopoliticalHistoryAll(_ context.Context, _ int) ([]ledger.GeopoliticalRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) UpsertEventCalendar(_ context.Context, _ ledger.EventCalendarRow) error {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadEventCalendarByDate(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadEventCalendarByDateAll(_ context.Context, _ string) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadEventCalendarRange(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadEventCalendarRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.EventCalendarRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) UpsertPredictionBacktest(_ context.Context, _ ledger.PredictionBacktestRow) error {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadPredictionBacktestRange(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) LoadPredictionBacktestRangeAll(_ context.Context, _, _ string, _ int) ([]ledger.PredictionBacktestRow, error) {
	panic("not implemented")
}
func (m *stubHistoricalStoreWithPeriod) CountSynthetic(_ context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}
func (m *stubHistoricalStoreWithPeriod) UpsertPeriod(_ context.Context, _ ledger.PeriodRow) error {
	return nil
}
func (m *stubHistoricalStoreWithPeriod) LoadPeriodByDate(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}
func (m *stubHistoricalStoreWithPeriod) LoadPeriodByDateAll(_ context.Context, _ string) (ledger.PeriodRow, bool, error) {
	return ledger.PeriodRow{}, false, nil
}
func (m *stubHistoricalStoreWithPeriod) LoadPeriodHistory(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	return m.periodRows, nil
}
func (m *stubHistoricalStoreWithPeriod) LoadPeriodHistoryAll(_ context.Context, _ int) ([]ledger.PeriodRow, error) {
	if m.periodErr != nil {
		return nil, m.periodErr
	}
	return m.periodRows, nil
}
func (m *stubHistoricalStoreWithPeriod) Close() error {
	return nil
}

// TestBuildRegimeHistoryData_PeriodFromHistory proves the bug fix:
// when period_history has consolidation for 2026-07-29 but
// regime_history says RISK_ON, the public Period field MUST read
// "consolidation" (PeriodDetector truth) and NOT "bull" (which is
// what RegimeToPeriod(RISK_ON) would yield). This is the canonical
// reproduction of the Hermes MCP confusion.
func TestBuildRegimeHistoryData_PeriodFromHistory(t *testing.T) {
	rows := []ledger.RegimeRow{
		{Date: "2026-07-29", Regime: "RISK_ON", Source: "macro_ingest", RecordedAt: time.Date(2026, 7, 29, 6, 0, 0, 0, time.UTC)},
		{Date: "2026-07-28", Regime: "NEUTRAL", Source: "macro_ingest", RecordedAt: time.Date(2026, 7, 28, 6, 0, 0, 0, time.UTC)},
	}
	periods := []ledger.PeriodRow{
		{Date: "2026-07-29", Period: "consolidation", Source: "period_detector"},
		{Date: "2026-07-28", Period: "black_swan", Source: "period_detector"},
	}
	store := &stubHistoricalStoreWithPeriod{regimeRows: rows, periodRows: periods}
	svc := NewPipelineService("/tmp", "/tmp", nil).WithHistoricalStore(store)
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if len(data.Sessions) != 2 {
		t.Fatalf("Sessions len = %d, want 2", len(data.Sessions))
	}
	// rows are newest-first, so index 0 is 2026-07-29.
	if data.Sessions[0].Period != "consolidation" {
		t.Errorf("Sessions[0].Period = %q, want %q (PeriodDetector truth, not RegimeToPeriod(RISK_ON)=bull)",
			data.Sessions[0].Period, "consolidation")
	}
	if data.Sessions[0].PeriodNameZH != "盤整" {
		t.Errorf("Sessions[0].PeriodNameZH = %q, want %q",
			data.Sessions[0].PeriodNameZH, "盤整")
	}
	// market_period is the deprecated alias and must mirror Period.
	if data.Sessions[0].MarketPeriod != "consolidation" {
		t.Errorf("Sessions[0].MarketPeriod = %q, want %q (deprecated alias must equal Period)",
			data.Sessions[0].MarketPeriod, "consolidation")
	}
	// Source split: regime_source from regime_history, period_source from period_history.
	if data.Sessions[0].RegimeSource != "macro_ingest" {
		t.Errorf("Sessions[0].RegimeSource = %q, want %q",
			data.Sessions[0].RegimeSource, "macro_ingest")
	}
	if data.Sessions[0].PeriodSource != "period_history" {
		t.Errorf("Sessions[0].PeriodSource = %q, want %q",
			data.Sessions[0].PeriodSource, "period_history")
	}
	// second row: 2026-07-28 with black_swan (this date is the post-
	// TAIEX backfill black_swan day, so we know the PeriodNameZH mapping).
	if data.Sessions[1].Period != "black_swan" {
		t.Errorf("Sessions[1].Period = %q, want %q", data.Sessions[1].Period, "black_swan")
	}
	if data.Sessions[1].PeriodNameZH != "黑天鵝" {
		t.Errorf("Sessions[1].PeriodNameZH = %q, want %q", data.Sessions[1].PeriodNameZH, "黑天鵝")
	}
	// regime remains authoritative for the 3-state contract.
	if data.Sessions[0].Regime != "RISK_ON" {
		t.Errorf("Sessions[0].Regime = %q, want RISK_ON (regime untouched)", data.Sessions[0].Regime)
	}
	// current_period follows the same rule.
	if data.CurrentPeriod != "consolidation" {
		t.Errorf("CurrentPeriod = %q, want %q (latest row = 2026-07-29 = consolidation)",
			data.CurrentPeriod, "consolidation")
	}
}

// TestBuildRegimeHistoryData_PeriodMissingIsEmpty proves the honest-
// degradation contract: when period_history has no row for a regime
// date, Period stays empty. We MUST NOT see a RegimeToPeriod-fabricated
// value (e.g. RISK_ON → bull). This guards against the regression that
// motivated the fix.
func TestBuildRegimeHistoryData_PeriodMissingIsEmpty(t *testing.T) {
	rows := []ledger.RegimeRow{
		{Date: "2026-07-29", Regime: "RISK_ON", Source: "macro_ingest", RecordedAt: time.Date(2026, 7, 29, 6, 0, 0, 0, time.UTC)},
	}
	// periodByDate is intentionally empty: the date has no period_history row.
	store := &stubHistoricalStoreWithPeriod{regimeRows: rows}
	svc := NewPipelineService("/tmp", "/tmp", nil).WithHistoricalStore(store)
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if len(data.Sessions) != 1 {
		t.Fatalf("Sessions len = %d, want 1", len(data.Sessions))
	}
	if data.Sessions[0].Period != "" {
		t.Errorf("Sessions[0].Period = %q, want \"\" (no period_history row — must NOT fallback to RegimeToPeriod)",
			data.Sessions[0].Period)
	}
	if data.Sessions[0].PeriodNameZH != "" {
		t.Errorf("Sessions[0].PeriodNameZH = %q, want \"\"", data.Sessions[0].PeriodNameZH)
	}
	if data.Sessions[0].MarketPeriod != "" {
		t.Errorf("Sessions[0].MarketPeriod = %q, want \"\" (deprecated alias must equal Period)", data.Sessions[0].MarketPeriod)
	}
	if data.Sessions[0].PeriodSource != "" {
		t.Errorf("Sessions[0].PeriodSource = %q, want \"\" (no period → no period_source claim)", data.Sessions[0].PeriodSource)
	}
	// regime must still be the truthful RISK_ON — 3-state contract preserved.
	if data.Sessions[0].Regime != "RISK_ON" {
		t.Errorf("Sessions[0].Regime = %q, want RISK_ON", data.Sessions[0].Regime)
	}
	// current_period follows the same empty-when-missing rule.
	if data.CurrentPeriod != "" {
		t.Errorf("CurrentPeriod = %q, want \"\" (no period_history row for latest date)", data.CurrentPeriod)
	}
}

// TestBuildRegimeHistoryData_PeriodStoreErrorIsEmpty ensures that a
// failing period_history load also degrades honestly (period = "").
// This is the no-PostgreSQL / transient-error scenario.
func TestBuildRegimeHistoryData_PeriodStoreErrorIsEmpty(t *testing.T) {
	rows := []ledger.RegimeRow{
		{Date: "2026-07-29", Regime: "RISK_ON", Source: "macro_ingest", RecordedAt: time.Date(2026, 7, 29, 6, 0, 0, 0, time.UTC)},
	}
	store := &stubHistoricalStoreWithPeriod{regimeRows: rows, periodErr: errors.New("sqlite busy")}
	svc := NewPipelineService("/tmp", "/tmp", nil).WithHistoricalStore(store)
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory must not error on period_history failure, got: %v", err)
	}
	if len(data.Sessions) != 1 {
		t.Fatalf("Sessions len = %d, want 1", len(data.Sessions))
	}
	if data.Sessions[0].Period != "" {
		t.Errorf("Sessions[0].Period = %q, want \"\" (period_history error → honest empty)", data.Sessions[0].Period)
	}
	if data.CurrentPeriod != "" {
		t.Errorf("CurrentPeriod = %q, want \"\"", data.CurrentPeriod)
	}
}

// TestLoadRegimeHistoryFromSessions_PeriodEmpty proves the legacy path
// (no HistoricalStore) reports empty period fields instead of falling
// back to RegimeToPeriod. This matches the honest-degradation
// contract for callers still on the simulation-summary path.
func TestLoadRegimeHistoryFromSessions_PeriodEmpty(t *testing.T) {
	now := time.Date(2026, 7, 29, 6, 0, 0, 0, time.UTC)
	summaries := []domain.SessionSummary{
		{SessionID: "session-20260729", Regime: domain.RegimeRiskOn, RecordedAt: now},
	}
	svc := NewPipelineService("/tmp", "/tmp", &mockOutcomeStore{summaries: summaries})
	data, err := svc.LoadRegimeHistory(10)
	if err != nil {
		t.Fatalf("LoadRegimeHistory: %v", err)
	}
	if len(data.Sessions) != 1 {
		t.Fatalf("Sessions len = %d, want 1", len(data.Sessions))
	}
	if data.Sessions[0].Regime != "RISK_ON" {
		t.Errorf("Sessions[0].Regime = %q, want RISK_ON (3-state preserved on legacy path)", data.Sessions[0].Regime)
	}
	if data.Sessions[0].Period != "" {
		t.Errorf("Sessions[0].Period = %q, want \"\" (no HistoricalStore → no period → no fabricated fallback)", data.Sessions[0].Period)
	}
	if data.Sessions[0].PeriodNameZH != "" {
		t.Errorf("Sessions[0].PeriodNameZH = %q, want \"\"", data.Sessions[0].PeriodNameZH)
	}
	if data.CurrentPeriod != "" {
		t.Errorf("CurrentPeriod = %q, want \"\"", data.CurrentPeriod)
	}
}
