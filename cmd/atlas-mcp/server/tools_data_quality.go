package server

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// injectDataQuality adds a data_quality envelope to the response map.
func injectDataQuality(m map[string]any, tool string, fetchErr error) {
	if m == nil {
		return
	}
	dq := domain.DataQuality{
		Source:     tool,
		Provenance: "live",
	}
	if fetchErr != nil {
		dq.Available = false
		dq.FallbackReason = fetchErr.Error()
	} else {
		dq.Available = true
	}
	m["data_quality"] = dq.ToMap()
}

// dataQualityFromIngestStatus derives DataQuality from upstream ingest_status.
func dataQualityFromIngestStatus(m map[string]any, tool string) *domain.DataQuality {
	if m == nil {
		return nil
	}
	raw, ok := m["ingest_status"]
	if !ok {
		return nil
	}
	is, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	dq := &domain.DataQuality{
		Available:  true,
		Source:     tool,
		Provenance: "live",
	}
	if last, ok := is["last_fetch"].(string); ok && last != "" {
		if t, err := parseRFC3339(last); err == nil {
			dq.AsOf = t
		}
	}
	if stale, _ := is["stale"].(bool); stale {
		dq.IsFallback = true
		dq.FallbackReason = "stale cache"
	}
	return dq
}

func parseRFC3339(s string) (time.Time, error) {
	for _, f := range []string{time.RFC3339, time.RFC3339Nano, "2006-01-02T15:04:05"} {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse timestamp: %q", s)
}
