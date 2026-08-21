package main

// Phase B7: maturity tracker Save wiring.
//
// The tracker is created at startup and persisted whenever the phase
// transitions. A daily background task also refreshes and saves it so
// `data/state/maturity_tracker.json` always has a recent `last_checked`
// timestamp even when no transition occurs.

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// registerMaturityTrackerSaveTask wires the "maturity_tracker_save" daily
// background task. It is a no-op when the tracker is nil (e.g. bootstrap
// failed to load it), so a missing maturity file never blocks startup.
func registerMaturityTrackerSaveTask(taskMgr *apigateway.BackgroundTaskManager, tracker *domain.MaturityTracker, statePath string) {
	if tracker == nil {
		logging.Warn("bootstrap", "maturity_tracker_save_skipped", "reason", "tracker_nil")
		return
	}

	_ = taskMgr.Register(&apigateway.ScheduledTask{
		Name:     "maturity_tracker_save",
		Interval: 24 * time.Hour,
		Enabled:  true,
		Task: func(ctx context.Context) error {
			tracker.Refresh()
			if err := tracker.Save(statePath); err != nil {
				logging.Warn("maturity_tracker_save", "save_failed",
					"err", err,
					"state_path", statePath)
				return err
			}
			logging.Info("maturity_tracker_save", "saved",
				"maturity", string(tracker.Current()),
				"days_since_start", tracker.DaysSinceStart(),
				"state_path", statePath)
			return nil
		},
	})
	logging.Info("bootstrap", "maturity_tracker_save_registered", "interval", "24h", "state_path", statePath)
}
