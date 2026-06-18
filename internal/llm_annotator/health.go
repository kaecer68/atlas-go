package llm_annotator

import (
	"encoding/json"
	"net/http"
)

// HandleHealth returns an http.HandlerFunc that responds with the client's
// current Snapshot as JSON. The handler reads the snapshot on every request
// so callers can poll for live state.
func HandleHealth(client *KimiClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := client.Snapshot()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(snap)
	}
}
