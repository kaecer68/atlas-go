package dashboard

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/orchestrator"
)

// HandleSimLatest returns the latest simulation trace records.
func (h *Handlers) HandleSimLatest(r *http.Request) (int, any) {
	pattern := filepath.Join(h.WorkDir, ".omo", "traces", "sim-*.jsonl")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		logging.Error("dashboardapi", "sim_trace_glob_failed",
			"pattern", pattern, "err", err)
		return http.StatusInternalServerError, map[string]string{
			"error": "failed to list trace files",
		}
	}

	if len(matches) == 0 {
		return http.StatusNotFound, map[string]string{
			"error": "no simulation trace files found",
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i] > matches[j]
	})

	latestFile := matches[0]
	f, err := os.Open(latestFile)
	if err != nil {
		logging.Error("dashboardapi", "sim_trace_open_failed",
			"file", latestFile, "err", err)
		return http.StatusInternalServerError, map[string]string{
			"error": "failed to open trace file",
		}
	}
	defer func() { _ = f.Close() }()

	var records []orchestrator.SimTraceRecord
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var record orchestrator.SimTraceRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			logging.Error("dashboardapi", "sim_trace_parse_failed",
				"file", latestFile, "err", err)
			f.Close()
			return http.StatusInternalServerError, map[string]string{
				"error": "failed to parse trace record",
			}
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		logging.Error("dashboardapi", "sim_trace_scan_failed",
			"file", latestFile, "err", err)
		return http.StatusInternalServerError, map[string]string{
			"error": "failed to read trace file",
		}
	}

	return http.StatusOK, records
}
