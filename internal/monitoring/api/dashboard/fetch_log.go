package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	defaultFetchLogLimit = 10
	maxFetchLogLimit     = 50
	fetchLogFileName     = "channel_fetch_log.json"
)

// fetchLogEntry mirrors the ChannelFetchLogEntry shape persisted by
// monitoring.ChannelHealthStore in channel_fetch_log.json. We redeclare it
// here to avoid an import cycle (monitoring package imports api/dashboard
// via dashboard_api.go).
type fetchLogEntry struct {
	Channel   string `json:"channel"`
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Timestamp string `json:"timestamp"`
	Error     string `json:"error,omitempty"`
}

// HandleChannelFetchLog returns the most recent channel fetch events from the
// persistent ring buffer (data/state/channel_fetch_log.json). The ring buffer
// is appended to on every monitoring.RecordChannelFetch(WithPool) call across
// all CLI tools (geo-ingest, macro-ingest, backfill-replay, etc.).
//
// Query params:
//   - limit (optional): number of entries to return (1-50, default 10).
//
// Response shape:
//   - 200 with {entries, count} on success
//   - 200 with {entries: [], empty_state: "..."} when the log is empty
func (h *Handlers) HandleChannelFetchLog(r *http.Request) (int, any) {
	limit := defaultFetchLogLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("invalid limit %q: must be a positive integer", raw),
			}
		}
		if parsed > maxFetchLogLimit {
			parsed = maxFetchLogLimit
		}
		limit = parsed
	}

	entries, err := readFetchLog(filepath.Join(h.WorkDir, "data/state"), limit)
	if err != nil {
		return http.StatusInternalServerError, map[string]string{
			"error": fmt.Sprintf("read fetch log: %v", err),
		}
	}

	resp := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		resp = append(resp, map[string]any{
			"time":    formatFetchTime(e.Timestamp),
			"channel": e.Channel,
			"result":  e.Status,
			"latency": formatFetchLatency(e.LatencyMs),
			"error":   e.Error,
		})
	}

	if len(resp) == 0 {
		return http.StatusOK, map[string]any{
			"entries":     resp,
			"count":       0,
			"empty_state": "尚無 fetch 紀錄 (CLI 工具下次抓取時會自動寫入)",
		}
	}

	return http.StatusOK, map[string]any{
		"entries": resp,
		"count":   len(resp),
	}
}

func readFetchLog(stateDir string, limit int) ([]fetchLogEntry, error) {
	path := filepath.Join(stateDir, fetchLogFileName)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return nil, nil
	}
	var all []fetchLogEntry
	if err := json.Unmarshal(b, &all); err != nil {
		return nil, err
	}
	if limit > 0 && limit < len(all) {
		all = all[len(all)-limit:]
	}
	return all, nil
}

func formatFetchTime(rfc3339 string) string {
	if rfc3339 == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		return rfc3339
	}
	return t.Format("15:04:05")
}

func formatFetchLatency(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000.0)
}
