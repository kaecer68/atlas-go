package metrics

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/kaecer68/atlas-go/internal/llm_annotator"
)

type degradedSnapshotJSON struct {
	Timestamp             time.Time        `json:"timestamp"`
	DegradedActivations   []map[string]any `json:"degraded_activations"`
	ProviderErrors        []map[string]any `json:"provider_errors"`
	DegradedCallbackCount []map[string]any `json:"degraded_callback_count"`
}

func sampleToMap(s Sample) map[string]any {
	entry := make(map[string]any, len(s.Labels)+2)
	for k, v := range s.Labels {
		entry[k] = v
	}
	entry["value"] = s.Value
	entry["timestamp"] = s.Timestamp
	return entry
}

// HandleDegraded returns an HTTP handler that exposes the current degraded
// metrics snapshot as JSON. Each sample is serialized as a flat map
// (labels + value + timestamp) for backward compatibility with existing
// dashboards and the Prometheus bridge; the top-level timestamp marks the
// capture moment.
func HandleDegraded(dm *DegradedMetrics) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := dm.Snapshot()
		out := degradedSnapshotJSON{
			Timestamp:             snap.Timestamp,
			DegradedActivations:   make([]map[string]any, 0, len(snap.DegradedActivations)),
			ProviderErrors:        make([]map[string]any, 0, len(snap.ProviderErrors)),
			DegradedCallbackCount: make([]map[string]any, 0, len(snap.DegradedCallbackCount)),
		}
		for _, s := range snap.DegradedActivations {
			out.DegradedActivations = append(out.DegradedActivations, sampleToMap(s))
		}
		for _, s := range snap.ProviderErrors {
			out.ProviderErrors = append(out.ProviderErrors, sampleToMap(s))
		}
		for _, s := range snap.DegradedCallbackCount {
			out.DegradedCallbackCount = append(out.DegradedCallbackCount, sampleToMap(s))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

// HandleCost returns an HTTP handler that exposes a KimiClient's CostReport
// as JSON. costPer1kTokens is the USD price per 1,000 tokens (e.g. 0.001
// = $0.001/1k). Pass 0 to compute token counts without a USD total.
//
// getClient is a late-binding getter so the handler always dereferences
// the latest value — this avoids the nil-closure trap when routes are
// registered before the KimiClient is injected (see P0-2, 2026-07-26).
func HandleCost(getClient func() *llm_annotator.KimiClient, costPer1kTokens float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := getClient()
		if client == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "llm annotator cost endpoint unavailable: no KimiClient wired",
			})
			return
		}
		report := client.CostReport(costPer1kTokens)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(report)
	}
}
