// Package monitoring — startup eager warmup (開機熱機)。
//
// RunWarmup 在背景依序呼叫五個快取層的讀取入口，讓第一位使用者到達前
// TTL cache 已填好。每項獨立 timeout（預設 30s）、單項失敗不致命，
// 僅 log 耗時。ATLAS_WARMUP=0 可停用（預設啟用）。
//
// P0 single-flight 保證：熱機期間的請求與熱機共用同一個 gateway/macro
// cache 的 double-check locking（不會重複 fan-out）。capitalflow Summary
// 共用同一個 in-memory cachedReport。事件預測共用 Handler 的 60s TTL
// cache。narrative bundle 共用 Handlers 的 BuildBundle。recommender
// PredictToday 共用 eventPredictorAdapter 的 60s TTL cache。
package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/kaecer68/atlas-go/internal/capitalflow"
	"github.com/kaecer68/atlas-go/internal/eventdriven"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/marketdata"
	apinarrative "github.com/kaecer68/atlas-go/internal/monitoring/api/narrative"
	narrativepkg "github.com/kaecer68/atlas-go/internal/narrative"
	"github.com/kaecer68/atlas-go/internal/recommender"
)

const warmupTimeout = 30 * time.Second

// WarmupDeps bundles the five service-layer entry points for startup warmup.
// Every field is required; nil fields cause the corresponding step to log
// a skip and continue.
type WarmupDeps struct {
	// MacroProvider for FetchSnapshot (gateway_adapter TTL cache).
	MacroProvider marketdata.MacroDataProvider

	// CapitalFlowSvc for Summary (reads macro data + rolling store).
	CapitalFlowSvc *capitalflow.Service

	// EventHandler for HandlePrediction (60s PredictionCacheTTL).
	EventHandler *eventdriven.Handler

	// NarrativeHandler for BuildBundle (narrative events + chains + models).
	NarrativeHandler *apinarrative.Handlers

	// EventPredictor for PredictToday (60s predictTodayCacheTTL in eventPredictorAdapter).
	EventPredictor recommender.EventPredictor
}

// RunWarmup executes all five warmup steps sequentially in the calling
// goroutine. Each step gets an independent 30s timeout and logs its
// duration. A step failure is logged but does not abort the sequence.
//
// ATLAS_WARMUP=0 disables warmup entirely (the function returns immediately).
func RunWarmup(deps WarmupDeps) {
	if os.Getenv("ATLAS_WARMUP") == "0" {
		logging.Info("warmup", "disabled", "reason", "ATLAS_WARMUP=0")
		return
	}
	logging.Info("warmup", "started", "steps", 5)

	start := time.Now()
	steps := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"macro_snapshot", func(ctx context.Context) error {
			if deps.MacroProvider == nil {
				return fmt.Errorf("skipped: MacroProvider is nil")
			}
			_, err := deps.MacroProvider.FetchSnapshot(ctx)
			return err
		}},
		{"capitalflow_summary", func(ctx context.Context) error {
			if deps.CapitalFlowSvc == nil {
				return fmt.Errorf("skipped: CapitalFlowSvc is nil")
			}
			_, err := deps.CapitalFlowSvc.Summary(ctx)
			return err
		}},
		{"events_prediction", func(ctx context.Context) error {
			if deps.EventHandler == nil {
				return fmt.Errorf("skipped: EventHandler is nil")
			}
			// Use httptest to construct a minimal *http.Request so the
			// warmup exercises the full HandlePrediction code path,
			// including the 60s PredictionCacheTTL cache check/write.
			req := httptest.NewRequest(http.MethodGet, "/api/events/prediction", nil).
				WithContext(ctx)
			status, _ := deps.EventHandler.HandlePrediction(req)
			if status != http.StatusOK {
				return fmt.Errorf("handler returned status %d", status)
			}
			return nil
		}},
		{"narrative_bundle", func(ctx context.Context) error {
			if deps.NarrativeHandler == nil {
				return fmt.Errorf("skipped: NarrativeHandler is nil")
			}
			// Zero data for warmup — no query-param overrides in this path.
			_, err := deps.NarrativeHandler.BuildBundle(ctx, narrativepkg.MarketNarrativeData{})
			return err
		}},
		{"recommender_predict_today", func(ctx context.Context) error {
			if deps.EventPredictor == nil {
				return fmt.Errorf("skipped: EventPredictor is nil")
			}
			_, err := deps.EventPredictor.PredictToday()
			return err
		}},
	}

	for _, step := range steps {
		ctx, cancel := context.WithTimeout(context.Background(), warmupTimeout)
		t0 := time.Now()
		err := step.fn(ctx)
		cancel()
		elapsed := time.Since(t0).Truncate(time.Millisecond)
		if err != nil {
			logging.Warn("warmup", "step_failed",
				"name", step.name,
				"elapsed", elapsed.String(),
				logging.Err(err))
		} else {
			logging.Info("warmup", "step_done",
				"name", step.name,
				"elapsed", elapsed.String())
		}
	}

	total := time.Since(start).Truncate(time.Millisecond)
	logging.Info("warmup", "done", "total", total.String())
}
