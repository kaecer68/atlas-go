package scheduler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/baseline"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/portfolio"
)

func TestAutoRollback_BurnInSkips(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-10 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")
	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results during burn_in, got %d", len(results))
	}
}

func TestAutoRollback_AgentDisable(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sick_agent", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})

	// Inject negative returns so RollingSharpe < -1.0 (prevents count reset).
	for i := range 60 {
		// 10 unique values (>= minUniqueReturnsForSharpe) so the degenerate-window
		// guard does not zero the Sharpe; negative mean keeps it well below -1.0.
		dw.RecordOutcome("sick_agent", -0.03-float64(i%10)*0.001, false)
	}

	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)

	// Simulate 30 days of catastrophic Sharpe
	ar.agentFailCount["sick_agent"] = 30

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 rollback result, got %d", len(results))
	}
	if results[0].Action != "disable_agent" {
		t.Errorf("expected action=disable_agent, got %s", results[0].Action)
	}
	if results[0].TargetID != "sick_agent" {
		t.Errorf("expected target=sick_agent, got %s", results[0].TargetID)
	}

	// Verify agent was actually removed
	_, ok := dw.GetAgentWeightData("sick_agent")
	if ok {
		t.Error("expected sick_agent to be removed after rollback execution")
	}
}

func TestAutoRollback_PromotionDegradation(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")
	bm := baseline.NewManager("/tmp/test_baseline_rollback.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent_1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	for i := range 60 {
		ret := 0.005 + float64(i%10)*0.002
		dw.RecordOutcome("agent_1", ret, true)
	}

	ar := NewAutoRollback(bm, dw, nil).WithMaturityTracker(tr)

	preSharpe := ar.computeSystemSharpe()
	if preSharpe <= 0 {
		t.Fatalf("pre-promotion sharpe should be > 0, got %f", preSharpe)
	}
	ar.RecordPromotion("exp-001", preSharpe)

	dw.Reset()
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent_1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	for i := range 60 {
		ret := -0.03 - float64(i%10)*0.001
		dw.RecordOutcome("agent_1", ret, false)
	}

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 rollback result for degraded promotion, got %d", len(results))
	}
	if results[0].Action != "revert_baseline" {
		t.Errorf("expected action=revert_baseline, got %s", results[0].Action)
	}
	if results[0].TargetID != "exp-001" {
		t.Errorf("expected target=exp-001, got %s", results[0].TargetID)
	}
}

func TestAutoRollback_PromotionDegradation_ExecutesActualRevert(t *testing.T) {
	dir := t.TempDir()
	policyPath := filepath.Join(dir, "baseline_policy.json")
	resultPath := filepath.Join(dir, "result.json")
	candidatePath := filepath.Join(dir, "v2.md")

	if err := os.WriteFile(candidatePath, []byte("candidate prompt body v2"), 0o644); err != nil {
		t.Fatalf("write candidate: %v", err)
	}
	result := domain.PromptExperimentResult{
		Experiment: domain.ExperimentRecord{
			ID:            "exp-revert-001",
			TargetAgentID: "growth-momentum-01",
			Skill:         "growth_momentum",
			MutationType:  "prompt_tightening",
			Status:        domain.ExperimentAccepted,
		},
		CandidatePrompt: candidatePath,
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if err := os.WriteFile(resultPath, resultBytes, 0o644); err != nil {
		t.Fatalf("write result: %v", err)
	}

	bm := baseline.NewManager(policyPath)
	if _, err := bm.PromoteResult(resultPath); err != nil {
		t.Fatalf("first promote: %v", err)
	}

	pre, err := baseline.Load(policyPath)
	if err != nil {
		t.Fatalf("load after first promote: %v", err)
	}
	if pre.Version != 2 {
		t.Fatalf("expected policy version 2 after first promote, got %d", pre.Version)
	}
	if len(pre.Promotions) != 1 {
		t.Fatalf("expected 1 promotion record, got %d", len(pre.Promotions))
	}

	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager(filepath.Join(dir, "dw.json"))
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent_1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	for i := range 60 {
		ret := 0.005 + float64(i%10)*0.002
		dw.RecordOutcome("agent_1", ret, true)
	}

	ar := NewAutoRollback(bm, dw, nil).WithMaturityTracker(tr)
	preSharpe := ar.computeSystemSharpe()
	if preSharpe <= 0 {
		t.Fatalf("pre-promotion sharpe should be > 0, got %f", preSharpe)
	}
	ar.RecordPromotion("exp-revert-001", preSharpe)

	dw.Reset()
	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "agent_1", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})
	for i := range 60 {
		ret := -0.03 - float64(i%10)*0.001
		dw.RecordOutcome("agent_1", ret, false)
	}

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 rollback result, got %d", len(results))
	}
	if results[0].Action != "revert_baseline" {
		t.Fatalf("expected action=revert_baseline, got %s", results[0].Action)
	}

	history := ar.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry after actual revert, got %d", len(history))
	}

	post, err := baseline.Load(policyPath)
	if err != nil {
		t.Fatalf("load after revert: %v", err)
	}
	if post.Version != 1 {
		t.Errorf("expected policy version 1 after revert (was 2, reverted to before exp-revert-001), got %d", post.Version)
	}
	if len(post.Promotions) != 0 {
		t.Errorf("expected 0 promotions after revert, got %d", len(post.Promotions))
	}
	if len(post.RevertHistory) != 1 {
		t.Errorf("expected 1 revert record, got %d", len(post.RevertHistory))
	}
}

func TestAutoRollback_CalibrationDegradation(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")

	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)

	ar.RecordCalibration(1.0)

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 rollback result, got %d", len(results))
	}
	if results[0].Action != "revert_calibration" {
		t.Errorf("expected action=revert_calibration, got %s", results[0].Action)
	}
	if results[0].TargetID != "last_calibration" {
		t.Errorf("expected target=last_calibration, got %s", results[0].TargetID)
	}
}

func TestAutoRollback_CalibrationDegradation_RestoresFromSnapshot(t *testing.T) {
	dir := t.TempDir()
	paramsPath := filepath.Join(dir, "parameters.json")
	config.SetParametersConfigPath(paramsPath)
	t.Cleanup(func() { config.SetParametersConfigPath("") })
	config.ResetParametersConfig()

	originalCfg := config.DefaultParametersConfig()
	originalCfg.Version = "pre-calibration-v1"
	if err := originalCfg.Save(paramsPath); err != nil {
		t.Fatalf("save initial: %v", err)
	}
	if err := config.SnapshotToBackup(paramsPath); err != nil {
		t.Fatalf("SnapshotToBackup: %v", err)
	}

	if _, err := config.LoadParametersConfig(paramsPath); err != nil {
		t.Fatalf("LoadParametersConfig: %v", err)
	}
	if config.GetParametersConfig().Version != "pre-calibration-v1" {
		t.Fatalf("singleton should be pre-calibration-v1 after load, got %s", config.GetParametersConfig().Version)
	}

	modifiedCfg := config.DefaultParametersConfig()
	modifiedCfg.Version = "post-calibration-v2"
	if err := modifiedCfg.LockedSaveWithRollback(paramsPath); err != nil {
		t.Fatalf("LockedSaveWithRollback: %v", err)
	}

	dataBeforeRevert, err := os.ReadFile(paramsPath)
	if err != nil {
		t.Fatalf("read before revert: %v", err)
	}
	if !strings.Contains(string(dataBeforeRevert), `"post-calibration-v2"`) {
		t.Fatalf("expected post-calibration-v2 in file before revert, got: %s", string(dataBeforeRevert))
	}

	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager(filepath.Join(dir, "dw.json"))

	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)
	ar.RecordCalibration(1.0)

	results, err := ar.RunDaily(context.Background())
	if err != nil {
		t.Fatalf("RunDaily: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 rollback result, got %d", len(results))
	}
	if results[0].Action != "revert_calibration" {
		t.Fatalf("expected action=revert_calibration, got %s", results[0].Action)
	}

	history := ar.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry after actual restore, got %d", len(history))
	}

	dataAfterRevert, err := os.ReadFile(paramsPath)
	if err != nil {
		t.Fatalf("read after revert: %v", err)
	}
	if !strings.Contains(string(dataAfterRevert), `"pre-calibration-v1"`) {
		t.Errorf("expected pre-calibration-v1 in file after actual restore, got: %s", string(dataAfterRevert))
	}
	if strings.Contains(string(dataAfterRevert), `"post-calibration-v2"`) {
		t.Errorf("post-calibration-v2 should be gone after restore, got: %s", string(dataAfterRevert))
	}

	if got := config.GetParametersConfig().Version; got != "pre-calibration-v1" {
		t.Errorf("singleton should be reloaded to pre-calibration-v1 after revert, got: %s", got)
	}
}

func TestAutoRollback_History(t *testing.T) {
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	dw := portfolio.NewDarwinianWeightManager("/tmp/test_rollback.json")

	dw.InitializeFromRegistry(domain.AgentRegistry{
		Agents: []domain.AgentSpec{
			{ID: "sick_agent", Enabled: true, Layer: domain.LayerSector, Skill: "tech"},
		},
	})

	// Inject negative returns so RollingSharpe < -1.0.
	for i := range 60 {
		// 10 unique values (>= minUniqueReturnsForSharpe) so the degenerate-window
		// guard does not zero the Sharpe; negative mean keeps it well below -1.0.
		dw.RecordOutcome("sick_agent", -0.03-float64(i%10)*0.001, false)
	}

	ar := NewAutoRollback(nil, dw, nil).WithMaturityTracker(tr)
	ar.agentFailCount["sick_agent"] = 30

	ar.RunDaily(context.Background())

	history := ar.History()
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Action != "disable_agent" {
		t.Errorf("expected history action=disable_agent, got %s", history[0].Action)
	}
}

func TestAutoRollback_RecordPromotion_EmitsEvent(t *testing.T) {
	bus := eventbus.NewChannelEventBus(64)
	defer bus.Close()

	var received atomic.Int32
	bus.Subscribe(eventbus.EventPromotionRecorded, func(_ context.Context, _ eventbus.BusEvent) error {
		received.Add(1)
		return nil
	})

	ar := NewAutoRollback(nil, nil, nil).WithEventBus(bus)
	ar.RecordPromotion("exp-event-001", 1.42)

	time.Sleep(100 * time.Millisecond)
	if got := received.Load(); got != 1 {
		t.Fatalf("expected 1 EventPromotionRecorded emitted, got %d", got)
	}
}
