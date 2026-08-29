package main

import (
	"slices"
	"testing"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

func TestRegisterStage3Tasks_RegistersAllFiveTasksInBTM(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{
		WorkDir:            tmp,
		LedgerDir:          tmp,
		ReplayMode:         "disabled",
		Stage3TasksEnabled: true,
	}

	store := ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)

	d := stage3Deps{
		taskMgr:          btm,
		cfg:              cfg,
		gateway:          nil,
		monitor:          nil,
		dashboard:        nil,
		eventCalendar:    nil,
		macroProvider:    nil,
		predictionLedger: store,
	}
	registerStage3Tasks(d)

	registered := btm.List()
	want := []string{
		"sync-events-daily",
		"sync-macro-daily",
		"sync-capital-daily",
		"sync-regime-weekly",
		"recalibrate-templates-monthly",
	}
	for _, name := range want {
		found := slices.Contains(registered, name)
		if !found {
			t.Fatalf("expected %q registered in BTM; got %v", name, registered)
		}
	}
}

func TestRegisterStage3AlertTasks_RegistersThreeEvaluatorsInBTM(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{
		WorkDir:             tmp,
		LedgerDir:           tmp,
		ReplayMode:          "disabled",
		Stage3TasksEnabled:  true,
		Stage3AlertsEnabled: true,
	}
	store := ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)

	var macroProvider marketdata.MacroDataProvider
	d := stage3Deps{
		taskMgr:          btm,
		cfg:              cfg,
		monitor:          monitoring.NewMonitor(),
		eventCalendar:    nil,
		macroProvider:    macroProvider,
		predictionLedger: store,
	}
	registerStage3AlertTasks(d)

	registered := btm.List()
	want := []string{
		"stage3-alert-staleness",
		"stage3-alert-daily",
		"stage3-alert-market-close",
	}
	for _, name := range want {
		found := slices.Contains(registered, name)
		if !found {
			t.Fatalf("expected %q registered in BTM; got %v", name, registered)
		}
	}
}

func TestRegisterStage3Tasks_RespectsOptOutFlagFalse(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{
		WorkDir:            tmp,
		LedgerDir:          tmp,
		ReplayMode:         "disabled",
		Stage3TasksEnabled: false,
	}
	store := ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)

	d := stage3Deps{
		taskMgr:          btm,
		cfg:              cfg,
		predictionLedger: store,
	}
	registerStage3Tasks(d)

	if registered := btm.List(); len(registered) != 0 {
		t.Fatalf("expected 0 tasks registered when Stage3TasksEnabled=false; got %v", registered)
	}
}

func TestRegisterStage3AlertTasks_RespectsOptOutFlagFalse(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{
		WorkDir:             tmp,
		LedgerDir:           tmp,
		ReplayMode:          "disabled",
		Stage3TasksEnabled:  true,
		Stage3AlertsEnabled: false,
	}
	store := ledger.NewJSONLEventFlowPredictionStore(cfg.LedgerDir)
	monitor := monitoring.NewMonitor()

	d := stage3Deps{
		taskMgr:          btm,
		cfg:              cfg,
		monitor:          monitor,
		predictionLedger: store,
	}
	registerStage3AlertTasks(d)

	if registered := btm.List(); len(registered) != 0 {
		t.Fatalf("expected 0 alerts registered when Stage3AlertsEnabled=false; got %v", registered)
	}
}
