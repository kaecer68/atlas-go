package main

import (
	"testing"
	"time"

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

// TestCurrentTaipeiTradingDate verifies the trading-day boundary
// derivation used by the capital_flow_refresh closure:
//   - Mon–Fri before 15:30 Taipei → previous weekday (Friday or earlier)
//   - Mon–Fri at/after 15:30 Taipei → today
//   - Saturday → Friday, Sunday → Friday
//
// Reference dates are 2026-07-13 (Mon), 2026-07-17 (Fri),
// 2026-07-18 (Sat), 2026-07-19 (Sun), 2026-07-20 (Mon).
func TestCurrentTaipeiTradingDate(t *testing.T) {
	tz, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		t.Skipf("Asia/Taipei tzdata unavailable: %v", err)
	}

	cases := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "Friday 16:00 Taipei → today (Friday)",
			now:  time.Date(2026, 7, 17, 16, 0, 0, 0, tz),
			want: time.Date(2026, 7, 17, 0, 0, 0, 0, tz),
		},
		{
			name: "Friday 15:30 Taipei boundary → today (Friday)",
			now:  time.Date(2026, 7, 17, 15, 30, 0, 0, tz),
			want: time.Date(2026, 7, 17, 0, 0, 0, 0, tz),
		},
		{
			name: "Friday 15:29 Taipei (just before cutoff) → Thursday",
			now:  time.Date(2026, 7, 17, 15, 29, 0, 0, tz),
			want: time.Date(2026, 7, 16, 0, 0, 0, 0, tz),
		},
		{
			name: "Friday 09:00 Taipei → Thursday",
			now:  time.Date(2026, 7, 17, 9, 0, 0, 0, tz),
			want: time.Date(2026, 7, 16, 0, 0, 0, 0, tz),
		},
		{
			name: "Monday 09:00 Taipei → Friday",
			now:  time.Date(2026, 7, 13, 9, 0, 0, 0, tz),
			want: time.Date(2026, 7, 10, 0, 0, 0, 0, tz),
		},
		{
			name: "Monday 16:00 Taipei → today (Monday)",
			now:  time.Date(2026, 7, 13, 16, 0, 0, 0, tz),
			want: time.Date(2026, 7, 13, 0, 0, 0, 0, tz),
		},
		{
			name: "Saturday 12:00 Taipei → previous Friday",
			now:  time.Date(2026, 7, 18, 12, 0, 0, 0, tz),
			want: time.Date(2026, 7, 17, 0, 0, 0, 0, tz),
		},
		{
			name: "Sunday 12:00 Taipei → previous Friday",
			now:  time.Date(2026, 7, 19, 12, 0, 0, 0, tz),
			want: time.Date(2026, 7, 17, 0, 0, 0, 0, tz),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := currentTaipeiTradingDate(tc.now)
			if !got.Equal(tc.want) {
				t.Errorf("currentTaipeiTradingDate(%v) = %v, want %v", tc.now, got, tc.want)
			}
		})
	}
}
