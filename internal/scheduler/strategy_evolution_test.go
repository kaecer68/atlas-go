package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func TestStrategyEvolutionTaskFunc_FactoryReturnsClosure(t *testing.T) {
	deps := StrategyEvolutionDeps{
		SectorDataDir: t.TempDir(),
	}
	task := StrategyEvolutionTaskFunc(deps)
	if task == nil {
		t.Fatal("StrategyEvolutionTaskFunc returned nil")
	}
}

func TestStrategyEvolutionTaskFunc_BurnInSkips(t *testing.T) {
	// Burn-in 期間 evolver 不應被觸發,避免冷啟動 false signal 污染 history。
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-10 * 24 * time.Hour))
	if tr.Current() != domain.MaturityBurnIn {
		t.Fatalf("precondition failed: expected burn_in, got %s", tr.Current())
	}

	deps := StrategyEvolutionDeps{
		// Dashboard 故意保持 nil:若 burn-in gate 正確短路,函式不應碰到
		// dashboard nil check,因此不應回傳錯誤。
		SectorDataDir:   t.TempDir(),
		MaturityTracker: tr,
	}
	task := StrategyEvolutionTaskFunc(deps)

	if err := task(context.Background()); err != nil {
		t.Errorf("burn-in 期間應回傳 nil (graceful skip),got err=%v", err)
	}
}

func TestStrategyEvolutionTaskFunc_NilDashboardErrors(t *testing.T) {
	// 非 burn-in 期間 + nil Dashboard:應回傳明確錯誤,不應 panic。
	tr := domain.NewMaturityTrackerWithStart(time.Now().UTC().Add(-300 * 24 * time.Hour))
	if tr.Current() != domain.MaturityFullAuto {
		t.Fatalf("precondition failed: expected full_auto, got %s", tr.Current())
	}

	deps := StrategyEvolutionDeps{
		SectorDataDir:   t.TempDir(),
		MaturityTracker: tr,
	}
	task := StrategyEvolutionTaskFunc(deps)

	err := task(context.Background())
	if err == nil {
		t.Error("expected error when Dashboard is nil, got nil")
	}
}

func TestStrategyEvolutionTaskFunc_NilMaturityTrackerRuns(t *testing.T) {
	// MaturityTracker 為 nil 時應視為「不啟用閘門」,函式應執行至
	// dashboard nil check 並回傳錯誤(此處即預期錯誤)。
	// 此測試確保 nil tracker 不會被誤判為 burn-in。
	deps := StrategyEvolutionDeps{
		SectorDataDir:   t.TempDir(),
		MaturityTracker: nil,
	}
	task := StrategyEvolutionTaskFunc(deps)

	err := task(context.Background())
	if err == nil {
		t.Error("expected error from nil dashboard path, got nil")
	}
}
