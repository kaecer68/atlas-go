package main

import (
	"os"
	"path/filepath"

	"github.com/kaecer68/atlas-go/internal/portfolio"
)

// loadCalibrationOrders walks every session directory under
// workDir/data/state/sessions/ and aggregates all calibrated orders
// stored in recommendation_outcomes.jsonl. Missing sessions dir is
// returned as an error; per-file parse failures are silently skipped
// (intentional — partial history is better than no history).
func loadCalibrationOrders(workDir string) ([]portfolio.CalibratedOrder, error) {
	sessionsDir := filepath.Join(workDir, "data", "state", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return nil, err
	}
	var all []portfolio.CalibratedOrder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		orders, err := portfolio.LoadOrdersFromJSONL(filepath.Join(sessionsDir, e.Name(), "recommendation_outcomes.jsonl"))
		if err != nil {
			continue
		}
		all = append(all, orders...)
	}
	return all, nil
}
