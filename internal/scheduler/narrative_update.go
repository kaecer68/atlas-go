package scheduler

import (
	"context"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/narrative"
)

// RegisterNarrativeWeightUpdateSchedule registers an hourly task that calls
// engine.UpdateModelWeights() so narrative model weights are periodically
// rebalanced based on recent prediction errors.
//
// Runs independently of template_detector_scan.go (which calls RunAll
// hourly): scan results feed hit rates, weight updates consume them. The
// same cadence (1h) ensures the next tick absorbs any recent hit-rate
// changes. A combined task would be cleaner but would change the existing
// contract.
func RegisterNarrativeWeightUpdateSchedule(
	btm *apigateway.BackgroundTaskManager,
	engine *narrative.NarrativeEngine,
) {
	if btm == nil || engine == nil {
		return
	}
	_ = btm.Register(&apigateway.ScheduledTask{
		Name:     "narrative_weight_update",
		Interval: time.Hour,
		Task: func(ctx context.Context) error {
			engine.UpdateModelWeights()
			return nil
		},
		Enabled: true,
	})
}
