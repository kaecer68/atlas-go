package monitoring

import (
	"math"
	"sort"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// SLAParameters holds the per-severity alert SLA thresholds (in seconds).
// Per Decision 9 (alert-redesign-v2.md Part 3.7): the meta-alert replaces
// the original SMS/Slack-page plan with a measurable SLA compliance metric.
// Each severity has its own threshold; alert_acknowledged-within > sla
// is a compliance violation that becomes a meta-alert.
type SLAParameters struct {
	CriticalSec int // CRITICAL alerts must be acknowledged within this many seconds
	ErrorSec    int // ERROR alerts must be acknowledged within this many seconds
	WarningSec  int // WARNING alerts must be acknowledged within this many seconds
}

// SLABucket holds per-severity SLA stats: total alerts, violation count,
// and compliance rate (1.0 = all compliant, 0.0 = all violations).
type SLABucket struct {
	Total          int     `json:"total"`
	Violations     int     `json:"violations"`
	ComplianceRate float64 `json:"compliance_rate"`
}

// SLAStats is the aggregated result returned by AlertStore.GetSLAStats.
type SLAStats struct {
	Critical            SLABucket `json:"critical"`
	Error               SLABucket `json:"error"`
	Warning             SLABucket `json:"warning"`
	AggregateLatencyP50 int       `json:"aggregate_latency_p50"`
	AggregateLatencyP95 int       `json:"aggregate_latency_p95"`
}

// GetSLAStats computes SLA compliance and latency statistics across all
// alerts in the store. Per Decision 9 (alert-redesign-v2.md Part 3.7):
//   - Unacknowledged alerts count as violations (treated as max latency)
//   - ComplianceRate = (Total - Violations) / Total (0 if Total == 0)
//   - Latency percentiles are over acknowledged alerts only (legacy alerts
//     with nil AcknowledgedWithinSec are excluded from latency stats)
func (s *AlertStore) GetSLAStats(params SLAParameters) SLAStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	all, err := s.loadFromFile()
	if err != nil {
		return SLAStats{}
	}

	var latencies []int
	stats := SLAStats{}
	for _, rec := range all {
		// Skip silenced/resolved — they are no longer in the SLA window.
		if rec.Status == domain.AlertStatusSilenced || rec.Status == domain.AlertStatusResolved {
			continue
		}
		var threshold int
		var bucket *SLABucket
		switch rec.Severity {
		case "critical":
			threshold = params.CriticalSec
			bucket = &stats.Critical
		case "error":
			threshold = params.ErrorSec
			bucket = &stats.Error
		case "warning":
			threshold = params.WarningSec
			bucket = &stats.Warning
		default:
			bucket = &stats.Warning
			threshold = params.WarningSec
		}
		bucket.Total++

		if !rec.Acknowledged {
			// Unacknowledged: treat as violation (max latency).
			bucket.Violations++
			continue
		}
		// Acknowledged: check latency vs threshold.
		if rec.AcknowledgedWithinSec == nil {
			// Legacy alert acknowledged before SLA tracking existed; skip
			// from latency stats, count as compliant (no measured latency).
			continue
		}
		latency := *rec.AcknowledgedWithinSec
		latencies = append(latencies, latency)
		if latency > threshold {
			bucket.Violations++
		}
	}
	// Compute compliance rate per bucket.
	stats.Critical.ComplianceRate = complianceRate(stats.Critical.Total, stats.Critical.Violations)
	stats.Error.ComplianceRate = complianceRate(stats.Error.Total, stats.Error.Violations)
	stats.Warning.ComplianceRate = complianceRate(stats.Warning.Total, stats.Warning.Violations)
	// Compute latency percentiles.
	stats.AggregateLatencyP50 = percentile(latencies, 50)
	stats.AggregateLatencyP95 = percentile(latencies, 95)
	return stats
}

// complianceRate returns (Total - Violations) / Total, or 0 if Total == 0.
func complianceRate(total, violations int) float64 {
	if total == 0 {
		return 0
	}
	return float64(total-violations) / float64(total)
}

// percentile returns the p-th percentile of vals (0 if empty). Uses
// nearest-rank method (simpler than linear interpolation; good enough
// for SLA dashboard display).
func percentile(vals []int, p int) int {
	if len(vals) == 0 {
		return 0
	}
	sort.Ints(vals)
	// nearest-rank: rank = ceil(p/100 * N), clamped to [1, N]
	rank := min(max(int(math.Ceil(float64(p)/100.0*float64(len(vals)))), 1), len(vals))
	return vals[rank-1]
}

// Compile-time interface guard.
var _ time.Time = time.Time{}
