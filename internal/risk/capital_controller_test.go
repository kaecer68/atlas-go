package risk

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestNewCapitalPhaseController(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	ctrl := NewCapitalPhaseController(cfg)

	if ctrl.GetSnapshot().Phase != domain.PhaseSimulation {
		t.Errorf("expected phase %q, got %q", domain.PhaseSimulation, ctrl.GetSnapshot().Phase)
	}
	if ctrl.GetSnapshot().CanAdvance {
		t.Error("expected CanAdvance to be false initially")
	}
}

func TestGetCapitalLimit(t *testing.T) {
	tests := []struct {
		phase  domain.CapitalPhase
		expect float64
	}{
		{domain.PhaseSimulation, 1.0},
		{domain.PhasePaper, 0.10},
		{domain.PhaseLive, 0.30},
		{domain.PhaseFull, 1.0},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			cfg := domain.DefaultCapitalPhaseConfig()
			cfg.CurrentPhase = tt.phase
			ctrl := NewCapitalPhaseController(cfg)

			got := ctrl.GetCapitalLimit()
			if got != tt.expect {
				t.Errorf("phase %q: expected %.2f, got %.2f", tt.phase, tt.expect, got)
			}
		})
	}
}

func TestGetCapitalLimitUnknownPhase(t *testing.T) {
	cfg := domain.CapitalPhaseConfig{
		CurrentPhase:  domain.CapitalPhase("unknown"),
		CapitalLimits: map[string]float64{},
	}
	ctrl := NewCapitalPhaseController(cfg)

	if got := ctrl.GetCapitalLimit(); got != 1.0 {
		t.Errorf("expected 1.0 for unknown phase, got %.2f", got)
	}
}

func TestCalculateMaxPositionSize(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhasePaper
	ctrl := NewCapitalPhaseController(cfg)

	totalCapital := 1000000.0
	expected := 100000.0

	got := ctrl.CalculateMaxPositionSize(totalCapital)
	if got != expected {
		t.Errorf("expected %.2f, got %.2f", expected, got)
	}
}

func TestCanAdvance_MinDaysNotMet(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 30
	cfg.PhaseStartDate = time.Now().Add(-5 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.05)

	can, reason := ctrl.CanAdvance()
	if can {
		t.Error("expected CanAdvance to be false when min days not met")
	}
	if reason == "" {
		t.Error("expected a reason when cannot advance")
	}
}

func TestCanAdvance_DrawdownExceeded(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 10
	cfg.MaxDrawdownLimit = 0.10
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.15)

	can, reason := ctrl.CanAdvance()
	if can {
		t.Error("expected CanAdvance to be false when drawdown exceeded")
	}
	if reason == "" {
		t.Error("expected a reason when cannot advance")
	}
}

func TestCanAdvance_SharpeNotMet(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 10
	cfg.SharpeThreshold = 1.0
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(0.5, 0.05)

	can, _ := ctrl.CanAdvance()
	if can {
		t.Error("expected CanAdvance to be false when sharpe not met")
	}
}

func TestCanAdvance_AllCriteriaMet(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 10
	cfg.MaxDrawdownLimit = 0.10
	cfg.SharpeThreshold = 1.0
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.05)

	can, reason := ctrl.CanAdvance()
	if !can {
		t.Errorf("expected CanAdvance to be true, got reason: %s", reason)
	}
}

func TestAdvancePhase_Success(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 10
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.05)

	err := ctrl.AdvancePhase()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctrl.GetSnapshot().Phase != domain.PhasePaper {
		t.Errorf("expected phase %q, got %q", domain.PhasePaper, ctrl.GetSnapshot().Phase)
	}
	if ctrl.GetSnapshot().DaysInPhase != 0 {
		t.Errorf("expected DaysInPhase to reset to 0, got %d", ctrl.GetSnapshot().DaysInPhase)
	}
	if ctrl.GetSnapshot().CanAdvance {
		t.Error("expected CanAdvance to be false after phase transition")
	}
}

func TestAdvancePhase_CannotAdvance(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 30
	cfg.PhaseStartDate = time.Now().Add(-5 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.05)

	err := ctrl.AdvancePhase()
	if err == nil {
		t.Fatal("expected error when advancing without meeting criteria")
	}
}

func TestAdvancePhase_AtFinalPhase(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhaseFull
	cfg.MinDaysPerPhase = 1
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.05)

	err := ctrl.AdvancePhase()
	if err == nil {
		t.Fatal("expected error when at final phase")
	}
}

func TestUpdateMetrics_UpdatesSnapshot(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.PhaseStartDate = time.Now().Add(-10 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.2, 0.08)

	snap := ctrl.GetSnapshot()
	if snap.RollingSharpe != 1.2 {
		t.Errorf("expected RollingSharpe 1.2, got %.2f", snap.RollingSharpe)
	}
	if snap.MaxDrawdown != 0.08 {
		t.Errorf("expected MaxDrawdown 0.08, got %.2f", snap.MaxDrawdown)
	}
	if snap.DaysInPhase < 9 || snap.DaysInPhase > 11 {
		t.Errorf("expected DaysInPhase around 10, got %d", snap.DaysInPhase)
	}
}

func TestCalculateSharpeRatio(t *testing.T) {
	tests := []struct {
		name    string
		returns []float64
		want    float64
	}{
		{
			name:    "empty returns",
			returns: []float64{},
			want:    0.0,
		},
		{
			name:    "single return",
			returns: []float64{0.01},
			want:    0.0,
		},
		{
			name:    "positive returns",
			returns: []float64{0.01, 0.02, 0.015, 0.01, 0.02},
			want:    1,
		},
		{
			name:    "zero variance",
			returns: []float64{0.01, 0.01, 0.01, 0.01},
			want:    0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateSharpeRatio(tt.returns)
			if tt.want == 0.0 {
				if got != 0.0 {
					t.Errorf("expected 0.0, got %.4f", got)
				}
			} else if got <= 0 {
				t.Errorf("expected positive sharpe, got %.4f", got)
			}
		})
	}
}

func TestCanAdvance_ConsecutiveLossesExceeded(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 10
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.05)
	ctrl.snapshot.ConsecutiveLosses = 5

	can, reason := ctrl.CanAdvance()
	if can {
		t.Error("expected CanAdvance to be false when consecutive losses exceeded")
	}
	if reason == "" {
		t.Error("expected a reason when cannot advance due to consecutive losses")
	}
}

func TestRecordLoss_IncrementsCounter(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	ctrl := NewCapitalPhaseController(cfg)

	if ctrl.snapshot.ConsecutiveLosses != 0 {
		t.Fatalf("expected initial ConsecutiveLosses=0, got %d", ctrl.snapshot.ConsecutiveLosses)
	}

	ctrl.RecordLoss()
	if ctrl.snapshot.ConsecutiveLosses != 1 {
		t.Errorf("expected ConsecutiveLosses=1 after one loss, got %d", ctrl.snapshot.ConsecutiveLosses)
	}

	ctrl.RecordLoss()
	ctrl.RecordLoss()
	if ctrl.snapshot.ConsecutiveLosses != 3 {
		t.Errorf("expected ConsecutiveLosses=3 after three losses, got %d", ctrl.snapshot.ConsecutiveLosses)
	}
}

func TestRecordWin_ResetsCounter(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	ctrl := NewCapitalPhaseController(cfg)

	ctrl.RecordLoss()
	ctrl.RecordLoss()
	ctrl.RecordLoss()
	if ctrl.snapshot.ConsecutiveLosses != 3 {
		t.Fatalf("expected ConsecutiveLosses=3 after three losses, got %d", ctrl.snapshot.ConsecutiveLosses)
	}

	ctrl.RecordWin()
	if ctrl.snapshot.ConsecutiveLosses != 0 {
		t.Errorf("expected ConsecutiveLosses=0 after win, got %d", ctrl.snapshot.ConsecutiveLosses)
	}
}

func TestRecordLoss_BlocksAdvance(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 10
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.05)

	for range 4 {
		ctrl.RecordLoss()
	}

	can, _ := ctrl.CanAdvance()
	if !can {
		t.Error("expected CanAdvance=true with 4 consecutive losses (limit is 5)")
	}

	ctrl.RecordLoss()

	can, reason := ctrl.CanAdvance()
	if can {
		t.Error("expected CanAdvance=false with 5 consecutive losses")
	}
	if reason == "" {
		t.Error("expected a reason when cannot advance due to consecutive losses")
	}
}

func TestRecordWin_AllowsRecovery(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 10
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseController(cfg)
	ctrl.UpdateMetrics(1.5, 0.05)

	for range 5 {
		ctrl.RecordLoss()
	}

	can, _ := ctrl.CanAdvance()
	if can {
		t.Error("expected CanAdvance=false with 5 consecutive losses")
	}

	ctrl.RecordWin()

	can, _ = ctrl.CanAdvance()
	if !can {
		t.Error("expected CanAdvance=true after recovery (win resets counter)")
	}
}

func TestNextPhase_Progression(t *testing.T) {
	expectedOrder := []domain.CapitalPhase{
		domain.PhaseSimulation,
		domain.PhasePaper,
		domain.PhaseLive,
		domain.PhaseFull,
	}

	for i := 0; i < len(expectedOrder)-1; i++ {
		cfg := domain.DefaultCapitalPhaseConfig()
		cfg.CurrentPhase = expectedOrder[i]
		cfg.MinDaysPerPhase = 1
		cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
		ctrl := NewCapitalPhaseController(cfg)
		ctrl.UpdateMetrics(1.5, 0.05)

		err := ctrl.AdvancePhase()
		if err != nil {
			t.Fatalf("phase %q: unexpected error: %v", expectedOrder[i], err)
		}

		if ctrl.GetSnapshot().Phase != expectedOrder[i+1] {
			t.Errorf("expected next phase %q, got %q", expectedOrder[i+1], ctrl.GetSnapshot().Phase)
		}
	}
}

func TestNewCapitalPhaseControllerWithPersistence_LoadsSavedState(t *testing.T) {
	dir := t.TempDir()

	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.MinDaysPerPhase = 1
	cfg.PhaseStartDate = time.Now().Add(-20 * 24 * time.Hour)
	ctrl := NewCapitalPhaseControllerWithPersistence(cfg, dir)
	ctrl.UpdateMetrics(1.5, 0.05)

	if err := ctrl.AdvancePhase(); err != nil {
		t.Fatalf("AdvancePhase failed: %v", err)
	}
	if ctrl.GetSnapshot().Phase != domain.PhasePaper {
		t.Fatalf("expected phase paper after advance, got %q", ctrl.GetSnapshot().Phase)
	}

	freshCfg := domain.DefaultCapitalPhaseConfig()
	freshCfg.CurrentPhase = domain.PhaseSimulation
	restored := NewCapitalPhaseControllerWithPersistence(freshCfg, dir)

	if restored.GetSnapshot().Phase != domain.PhasePaper {
		t.Errorf("expected restored phase paper, got %q", restored.GetSnapshot().Phase)
	}
	if restored.config.CurrentPhase != domain.PhasePaper {
		t.Errorf("expected restored config phase paper, got %q", restored.config.CurrentPhase)
	}
}

func TestNewCapitalPhaseControllerWithPersistence_EmptyDir(t *testing.T) {
	cfg := domain.DefaultCapitalPhaseConfig()
	cfg.CurrentPhase = domain.PhaseLive
	ctrl := NewCapitalPhaseControllerWithPersistence(cfg, "")

	if ctrl.GetSnapshot().Phase != domain.PhaseLive {
		t.Errorf("expected phase live when persist dir empty, got %q", ctrl.GetSnapshot().Phase)
	}
}

func TestCapitalPhaseController_LoadState_NoPersistPath(t *testing.T) {
	ctrl := NewCapitalPhaseController(domain.DefaultCapitalPhaseConfig())
	if _, err := ctrl.LoadState(); err == nil {
		t.Error("expected error when LoadState called with no persist path")
	}
}

func TestCapitalPhaseController_LoadState_MissingFile(t *testing.T) {
	dir := t.TempDir()
	ctrl := NewCapitalPhaseControllerWithPersistence(domain.DefaultCapitalPhaseConfig(), dir)
	_ = os.Remove(ctrl.persistPath)

	if _, err := ctrl.LoadState(); err == nil {
		t.Error("expected error when state file is missing")
	}
}

func TestCapitalPhaseController_LoadState_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	ctrl := NewCapitalPhaseControllerWithPersistence(domain.DefaultCapitalPhaseConfig(), dir)
	if err := os.WriteFile(ctrl.persistPath, []byte("not json"), 0o644); err != nil {
		t.Fatalf("setup invalid json failed: %v", err)
	}

	if _, err := ctrl.LoadState(); err == nil {
		t.Error("expected error when state file contains invalid JSON")
	}
}

func TestCapitalPhaseController_LoadState_CorruptedConfig(t *testing.T) {
	dir := t.TempDir()
	_ = NewCapitalPhaseControllerWithPersistence(domain.DefaultCapitalPhaseConfig(), dir)

	state := PersistedState{
		Config: domain.CapitalPhaseConfig{
			CurrentPhase:   domain.PhaseFull,
			PhaseStartDate: time.Now().Add(-90 * 24 * time.Hour),
			CapitalLimits: map[string]float64{
				string(domain.PhaseSimulation): 1.0,
				string(domain.PhasePaper):      0.10,
				string(domain.PhaseLive):       0.30,
				string(domain.PhaseFull):       1.0,
			},
		},
		Snapshot: domain.CapitalSnapshot{
			Phase:         domain.PhaseFull,
			RollingSharpe: 1.8,
			MaxDrawdown:   0.04,
		},
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	path := filepath.Join(dir, "capital_phase_state.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}

	restored := NewCapitalPhaseControllerWithPersistence(domain.DefaultCapitalPhaseConfig(), dir)
	if restored.GetSnapshot().Phase != domain.PhaseFull {
		t.Errorf("expected restored phase full, got %q", restored.GetSnapshot().Phase)
	}
	if restored.GetSnapshot().RollingSharpe != 1.8 {
		t.Errorf("expected restored RollingSharpe 1.8, got %.2f", restored.GetSnapshot().RollingSharpe)
	}
}
