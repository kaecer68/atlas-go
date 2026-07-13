package main

import (
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
		WorkDir:    tmp,
		LedgerDir:  tmp,
		ReplayMode: "disabled",
	}

	store, err := ledger.NewEventFlowPredictionStore(cfg)
	if err != nil {
		t.Fatalf("NewEventFlowPredictionStore: %v", err)
	}

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
		found := false
		for _, got := range registered {
			if got == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q registered in BTM; got %v", name, registered)
		}
	}
}

func TestRegisterStage3AlertTasks_RegistersThreeEvaluatorsInBTM(t *testing.T) {
	tmp := t.TempDir()
	btm := apigateway.NewBackgroundTaskManager(nil)

	cfg := config.Config{WorkDir: tmp, LedgerDir: tmp, ReplayMode: "disabled"}
	store, err := ledger.NewEventFlowPredictionStore(cfg)
	if err != nil {
		t.Fatalf("NewEventFlowPredictionStore: %v", err)
	}

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
		found := false
		for _, got := range registered {
			if got == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q registered in BTM; got %v", name, registered)
		}
	}
}
