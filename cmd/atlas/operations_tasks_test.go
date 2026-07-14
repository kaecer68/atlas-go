package main

import (
	"testing"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/janus"
)

// registerOpsForTest 隔離 registerOperationsTasks 的可測部分：只注入
// taskMgr 與本次 PR 新增的 optional dep，避免其他 9 個既有 task 因
// nil dashboard/repo/healthMonitor 而失敗。
func registerOpsForTest(mgr *apigateway.BackgroundTaskManager, janusEngine *janus.Engine) {
	registerOperationsTasks(operationsDeps{
		taskMgr:     mgr,
		janusEngine: janusEngine,
	})
}

func TestRegisterOperationsTasks_JanusRefreshRegistered(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTest(mgr, janus.NewEngine())

	task, ok := mgr.Get("janus_regime_refresh")
	if !ok {
		t.Fatal("janus_regime_refresh must be registered when janusEngine is non-nil")
	}
	if task.Interval.Hours() != 6 {
		t.Errorf("janus_regime_refresh interval = %v, want 6h", task.Interval)
	}
	if !task.Enabled {
		t.Error("janus_regime_refresh must be Enabled by default")
	}
	if task.Task == nil {
		t.Fatal("janus_regime_refresh Task func must not be nil")
	}
}

func TestRegisterOperationsTasks_JanusRefreshSkippedWhenNil(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTest(mgr, nil)

	if _, ok := mgr.Get("janus_regime_refresh"); ok {
		t.Fatal("janus_regime_refresh must NOT be registered when janusEngine is nil")
	}
}

func TestRegisterOperationsTasks_CapitalFlowRefreshSkippedWhenNil(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerOpsForTest(mgr, nil)

	if _, ok := mgr.Get("capital_flow_refresh"); ok {
		t.Fatal("capital_flow_refresh must NOT be registered when capitalFlow is nil")
	}
}
