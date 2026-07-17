package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// mockCtrlOutcomeStore is a minimal mock for testing ControlService.
type mockCtrlOutcomeStore struct {
	interventions []domain.HumanIntervention
	recordErr     error
	loadErr       error
}

func (m *mockCtrlOutcomeStore) RecordHumanIntervention(iv domain.HumanIntervention) error {
	if m.recordErr != nil {
		return m.recordErr
	}
	m.interventions = append(m.interventions, iv)
	return nil
}

func (m *mockCtrlOutcomeStore) LoadHumanInterventions() ([]domain.HumanIntervention, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.interventions, nil
}

// Unused methods - implement the full interface for compilation
func (m *mockCtrlOutcomeStore) RecordOutcomes(outcomes []domain.RecommendationOutcome) error {
	return nil
}

func (m *mockCtrlOutcomeStore) RecordSessionOutcomes(session domain.ReplaySession, outcomes []domain.RecommendationOutcome) error {
	return nil
}

func (m *mockCtrlOutcomeStore) LoadOutcomes() ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (m *mockCtrlOutcomeStore) LoadSessionOutcomes(sessionID string) ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (m *mockCtrlOutcomeStore) LoadOutcomesFromSessions() ([]domain.RecommendationOutcome, error) {
	return nil, nil
}

func (m *mockCtrlOutcomeStore) RecordSessionScreeningRejects(sessionID string, rejects []domain.ScreeningReject) error {
	return nil
}

func (m *mockCtrlOutcomeStore) LoadSessionScreeningRejects(sessionID string) ([]domain.ScreeningReject, error) {
	return nil, nil
}

func (m *mockCtrlOutcomeStore) RecordSessionTrades(sessionID string, trades []domain.TradeRecord) error {
	return nil
}

func (m *mockCtrlOutcomeStore) LoadSessionTrades(sessionID string) ([]domain.TradeRecord, error) {
	return nil, nil
}

func (m *mockCtrlOutcomeStore) LoadAllSessionTrades() ([]domain.TradeRecord, error)   { return nil, nil }
func (m *mockCtrlOutcomeStore) RecordExperiment(record domain.ExperimentRecord) error { return nil }
func (m *mockCtrlOutcomeStore) RecordSessionExperiment(session domain.ReplaySession, record domain.ExperimentRecord) error {
	return nil
}

func (m *mockCtrlOutcomeStore) RecordSessionSummary(session domain.ReplaySession, summary domain.SessionSummary) error {
	return nil
}

func (m *mockCtrlOutcomeStore) LoadSessionSummaries() ([]domain.SessionSummary, error) {
	return nil, nil
}

func (m *mockCtrlOutcomeStore) LoadAllSessionScorecards() ([]domain.Scorecard, []domain.RecommendationOutcome, error) {
	return nil, nil, nil
}

// TestControlService_RecordIntervention tests recording an intervention.
func TestControlService_RecordIntervention(t *testing.T) {
	store := &mockCtrlOutcomeStore{}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	iv := domain.HumanIntervention{
		ID:            "test-iv-1",
		Type:          "pause_agent",
		TargetAgentID: "agent-007",
		Reason:        "test pause",
		Operator:      "test-admin",
		RecordedAt:    time.Now(),
		TTLHours:      24,
	}

	err := svc.RecordIntervention(iv)
	if err != nil {
		t.Errorf("RecordIntervention error = %v", err)
	}

	if len(store.interventions) != 1 {
		t.Fatalf("expected 1 intervention, got %d", len(store.interventions))
	}
	if store.interventions[0].ID != "test-iv-1" {
		t.Errorf("expected intervention ID 'test-iv-1', got %q", store.interventions[0].ID)
	}
}

// TestControlService_RecordIntervention_PassesThroughError tests that store errors propagate.
func TestControlService_RecordIntervention_PassesThroughError(t *testing.T) {
	store := &mockCtrlOutcomeStore{recordErr: errTestStoreFailure}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	iv := domain.HumanIntervention{ID: "test-iv"}

	err := svc.RecordIntervention(iv)
	if err == nil {
		t.Error("expected error from store, got nil")
	}
}

// TestControlService_WithRegistryProvider tests the builder pattern.
func TestControlService_WithRegistryProvider(t *testing.T) {
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, nil)

	provider := func() (domain.AgentRegistry, error) {
		return orchestrator.SeedRegistry(), nil
	}

	result := svc.WithRegistryProvider(provider)
	if result != svc {
		t.Error("WithRegistryProvider should return the same service instance")
	}

	// Check that registryProvider is set
	if svc.registryProvider == nil {
		t.Error("registryProvider should be set after WithRegistryProvider")
	}
}

// TestControlService_LoadInterventions tests loading interventions.
func TestControlService_LoadInterventions(t *testing.T) {
	store := &mockCtrlOutcomeStore{
		interventions: []domain.HumanIntervention{
			{ID: "iv-1", Type: "pause_agent", RecordedAt: time.Now().Add(-1 * time.Hour)},
			{ID: "iv-2", Type: "resume_agent", RecordedAt: time.Now()},
		},
	}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	ivs, err := svc.LoadInterventions()
	if err != nil {
		t.Fatalf("LoadInterventions error = %v", err)
	}

	// Should be reversed (most recent first)
	if len(ivs) != 2 {
		t.Fatalf("expected 2 interventions, got %d", len(ivs))
	}
	if ivs[0].ID != "iv-2" {
		t.Errorf("expected first intervention to be 'iv-2' (most recent), got %q", ivs[0].ID)
	}
}

// TestControlService_LoadInterventions_LoadError tests error handling in LoadInterventions.
func TestControlService_LoadInterventions_LoadError(t *testing.T) {
	store := &mockCtrlOutcomeStore{loadErr: errTestStoreFailure}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	_, err := svc.LoadInterventions()
	if err == nil {
		t.Error("expected error from store.LoadHumanInterventions, got nil")
	}
}

// TestControlService_GetActiveOverrides_NoInterventions tests that no interventions returns empty results.
func TestControlService_GetActiveOverrides_NoInterventions(t *testing.T) {
	store := &mockCtrlOutcomeStore{}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	paused, banned, weights := svc.GetActiveOverrides()

	if len(paused) != 0 {
		t.Errorf("expected empty paused agents, got %v", paused)
	}
	if len(banned) != 0 {
		t.Errorf("expected empty banned sectors, got %v", banned)
	}
	if len(weights) != 0 {
		t.Errorf("expected empty weights, got %v", weights)
	}
}

// TestControlService_GetActiveOverrides_PauseAgent tests pause_agent intervention.
func TestControlService_GetActiveOverrides_PauseAgent(t *testing.T) {
	futureTime := time.Now().Add(24 * time.Hour)
	store := &mockCtrlOutcomeStore{
		interventions: []domain.HumanIntervention{
			{ID: "iv-1", Type: "pause_agent", TargetAgentID: "agent-007", ExpiresAt: futureTime},
		},
	}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	paused, _, _ := svc.GetActiveOverrides()

	if len(paused) != 1 || paused[0] != "agent-007" {
		t.Errorf("expected paused ['agent-007'], got %v", paused)
	}
}

// TestControlService_GetActiveOverrides_ResumeAgent tests that resume_agent removes pause.
func TestControlService_GetActiveOverrides_ResumeAgent(t *testing.T) {
	futureTime := time.Now().Add(24 * time.Hour)
	store := &mockCtrlOutcomeStore{
		interventions: []domain.HumanIntervention{
			{ID: "iv-1", Type: "pause_agent", TargetAgentID: "agent-007", ExpiresAt: futureTime},
			{ID: "iv-2", Type: "resume_agent", TargetAgentID: "agent-007", ExpiresAt: futureTime},
		},
	}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	paused, _, _ := svc.GetActiveOverrides()

	if len(paused) != 0 {
		t.Errorf("expected no paused agents after resume, got %v", paused)
	}
}

// TestControlService_GetActiveOverrides_SectorBan tests sector_ban intervention.
func TestControlService_GetActiveOverrides_SectorBan(t *testing.T) {
	futureTime := time.Now().Add(24 * time.Hour)
	store := &mockCtrlOutcomeStore{
		interventions: []domain.HumanIntervention{
			{ID: "iv-1", Type: "sector_ban", TargetSector: "semiconductor", ExpiresAt: futureTime},
		},
	}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	_, banned, _ := svc.GetActiveOverrides()

	if len(banned) != 1 || banned[0] != "semiconductor" {
		t.Errorf("expected banned ['semiconductor'], got %v", banned)
	}
}

// TestControlService_GetActiveOverrides_ModelWeight tests set_model_weight intervention.
func TestControlService_GetActiveOverrides_ModelWeight(t *testing.T) {
	futureTime := time.Now().Add(72 * time.Hour)
	store := &mockCtrlOutcomeStore{
		interventions: []domain.HumanIntervention{
			{ID: "iv-1", Type: "set_model_weight", TargetModelID: "model-x", Value: 1.5, ExpiresAt: futureTime},
		},
	}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	_, _, weights := svc.GetActiveOverrides()

	if weights == nil {
		t.Fatal("expected non-nil weights map")
	}
	if weights["model-x"] != 1.5 {
		t.Errorf("expected weights['model-x'] = 1.5, got %f", weights["model-x"])
	}
}

// TestControlService_GetActiveOverrides_ExpiredIntervention tests that expired interventions are ignored.
func TestControlService_GetActiveOverrides_ExpiredIntervention(t *testing.T) {
	pastTime := time.Now().Add(-1 * time.Hour) // expired
	store := &mockCtrlOutcomeStore{
		interventions: []domain.HumanIntervention{
			{ID: "iv-1", Type: "pause_agent", TargetAgentID: "agent-expired", ExpiresAt: pastTime},
		},
	}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	paused, _, _ := svc.GetActiveOverrides()

	if len(paused) != 0 {
		t.Errorf("expected no paused agents (expired), got %v", paused)
	}
}

// TestControlService_GetAgentHealth_NilHealthManager tests nil HealthManager handling.
func TestControlService_GetAgentHealth_NilHealthManager(t *testing.T) {
	store := &mockCtrlOutcomeStore{}
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, store)

	agents, mutedCount, err := svc.GetAgentHealth()
	if err != nil {
		t.Fatalf("GetAgentHealth error = %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents with nil HealthManager, got %d", len(agents))
	}
	if mutedCount != 0 {
		t.Errorf("expected 0 muted count with nil HealthManager, got %d", mutedCount)
	}
}

// TestControlService_GetAgentHealth_WithRegistryProvider tests custom registry provider.
func TestControlService_GetAgentHealth_WithRegistryProvider(t *testing.T) {
	store := &mockCtrlOutcomeStore{}
	healthManager := portfolio.NewAgentHealthManager()
	svc := NewControlService("/tmp/work", "/tmp/ledger", healthManager, store)

	// Use custom registry provider that returns a known registry
	customRegistry := domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent-custom-1", Enabled: true},
			{ID: "agent-custom-2", Enabled: false},
		},
	}
	svc.WithRegistryProvider(func() (domain.AgentRegistry, error) {
		return customRegistry, nil
	})

	agents, _, err := svc.GetAgentHealth()
	if err != nil {
		t.Fatalf("GetAgentHealth error = %v", err)
	}

	// Only enabled agents should be included
	if len(agents) != 1 {
		t.Errorf("expected 1 agent (only enabled), got %d", len(agents))
	}
	if agents[0].AgentID != "agent-custom-1" {
		t.Errorf("expected agent 'agent-custom-1', got %q", agents[0].AgentID)
	}
}

// TestControlService_GetAgentHealth_FallbackToSeedRegistry tests fallback when registry loading fails.
func TestControlService_GetAgentHealth_FallbackToSeedRegistry(t *testing.T) {
	store := &mockCtrlOutcomeStore{}
	healthManager := portfolio.NewAgentHealthManager()
	svc := NewControlService("/tmp/work", "/tmp/ledger", healthManager, store)

	// No registry provider set and no valid registry file - should fallback to seed
	agents, _, err := svc.GetAgentHealth()
	if err != nil {
		t.Fatalf("GetAgentHealth error = %v", err)
	}
	// Seed registry should have some agents
	if len(agents) == 0 {
		t.Error("expected non-empty agents from seed registry fallback")
	}
}

// TestControlService_CreateIntervention_PauseAgent tests pause_agent intervention creation.
func TestControlService_CreateIntervention_PauseAgent(t *testing.T) {
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, nil)
	iv := svc.CreateIntervention("pause_agent", "agent-007", "test reason", "admin", 0)

	if iv.Type != "pause_agent" {
		t.Errorf("Type = %q, want pause_agent", iv.Type)
	}
	if iv.TargetAgentID != "agent-007" {
		t.Errorf("TargetAgentID = %q, want agent-007", iv.TargetAgentID)
	}
	if iv.Reason != "test reason" {
		t.Errorf("Reason = %q, want test reason", iv.Reason)
	}
	if iv.Operator != "admin" {
		t.Errorf("Operator = %q, want admin", iv.Operator)
	}
	if iv.TTLHours != 24 {
		t.Errorf("TTLHours = %d, want 24", iv.TTLHours)
	}
	if iv.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should be set for pause_agent")
	}
	if !strings.HasPrefix(iv.ID, "int-pause-agent-007-") {
		t.Errorf("ID = %q, want prefix int-pause-agent-007-", iv.ID)
	}
}

// TestControlService_CreateIntervention_ResumeAgent tests resume_agent intervention creation.
func TestControlService_CreateIntervention_ResumeAgent(t *testing.T) {
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, nil)
	iv := svc.CreateIntervention("resume_agent", "agent-007", "test reason", "admin", 0)

	if iv.Type != "resume_agent" {
		t.Errorf("Type = %q, want resume_agent", iv.Type)
	}
	if iv.TargetAgentID != "agent-007" {
		t.Errorf("TargetAgentID = %q, want agent-007", iv.TargetAgentID)
	}
	if iv.TTLHours != 0 {
		t.Errorf("TTLHours = %d, want 0 (no expiry)", iv.TTLHours)
	}
	if !iv.ExpiresAt.IsZero() {
		t.Error("ExpiresAt should not be set for resume_agent")
	}
}

// TestControlService_CreateIntervention_SetModelWeight tests set_model_weight intervention creation.
func TestControlService_CreateIntervention_SetModelWeight(t *testing.T) {
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, nil)
	iv := svc.CreateIntervention("set_model_weight", "model-v2", "下调权重", "admin", 0.5)

	if iv.Type != "set_model_weight" {
		t.Errorf("Type = %q, want set_model_weight", iv.Type)
	}
	if iv.TargetModelID != "model-v2" {
		t.Errorf("TargetModelID = %q, want model-v2", iv.TargetModelID)
	}
	if iv.Value != 0.5 {
		t.Errorf("Value = %v, want 0.5", iv.Value)
	}
	if iv.TTLHours != 72 {
		t.Errorf("TTLHours = %d, want 72", iv.TTLHours)
	}
}

// TestControlService_CreateIntervention_SectorBan tests sector_ban intervention creation.
func TestControlService_CreateIntervention_SectorBan(t *testing.T) {
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, nil)
	iv := svc.CreateIntervention("sector_ban", "semiconductor", "行业风险", "admin", 0)

	if iv.Type != "sector_ban" {
		t.Errorf("Type = %q, want sector_ban", iv.Type)
	}
	if iv.TargetSector != "semiconductor" {
		t.Errorf("TargetSector = %q, want semiconductor", iv.TargetSector)
	}
	if iv.TTLHours != 24 {
		t.Errorf("TTLHours = %d, want 24", iv.TTLHours)
	}
}

// TestControlService_CreateIntervention_ApproveRec tests approve_rec intervention creation.
func TestControlService_CreateIntervention_ApproveRec(t *testing.T) {
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, nil)

	// approve_rec with agent:symbol format
	iv := svc.CreateIntervention("approve_rec", "agent-007:2330", "test", "admin", 0)
	if iv.Type != "approve_rec" {
		t.Errorf("Type = %q, want approve_rec", iv.Type)
	}
	if iv.TargetAgentID != "agent-007" {
		t.Errorf("TargetAgentID = %q, want agent-007", iv.TargetAgentID)
	}
	if iv.TargetSymbol != "2330" {
		t.Errorf("TargetSymbol = %q, want 2330", iv.TargetSymbol)
	}
	if iv.TTLHours != 48 {
		t.Errorf("TTLHours = %d, want 48", iv.TTLHours)
	}

	// approve_rec with symbol only
	iv2 := svc.CreateIntervention("approve_rec", "2330", "test", "admin", 0)
	if iv2.TargetSymbol != "2330" {
		t.Errorf("TargetSymbol = %q, want 2330", iv2.TargetSymbol)
	}
	if iv2.TargetAgentID != "" {
		t.Errorf("TargetAgentID = %q, want empty", iv2.TargetAgentID)
	}
}

// TestControlService_CreateIntervention_RejectRec tests reject_rec intervention creation.
func TestControlService_CreateIntervention_RejectRec(t *testing.T) {
	svc := NewControlService("/tmp/work", "/tmp/ledger", nil, nil)
	iv := svc.CreateIntervention("reject_rec", "agent-007:2330", "test", "admin", 0)

	if iv.Type != "reject_rec" {
		t.Errorf("Type = %q, want reject_rec", iv.Type)
	}
	if iv.TargetAgentID != "agent-007" {
		t.Errorf("TargetAgentID = %q, want agent-007", iv.TargetAgentID)
	}
	if iv.TargetSymbol != "2330" {
		t.Errorf("TargetSymbol = %q, want 2330", iv.TargetSymbol)
	}
	if iv.TTLHours != 48 {
		t.Errorf("TTLHours = %d, want 48", iv.TTLHours)
	}
}

// errTestStoreFailure is a sentinel error for testing.
var errTestStoreFailure = fmt.Errorf("store failure")
