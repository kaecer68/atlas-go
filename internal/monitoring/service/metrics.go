package service

import (
	"context"
	"time"
)

// MetricsService encapsulates metrics collection and trend analysis
type MetricsService struct {
	collector MetricsCollector
	history   MetricsHistory
}

type MetricsCollector interface {
	GetScreeningRate() float64
	GetMetricsSnapshot() MetricsSnapshot
}

type MetricsHistory interface {
	GetTrend(metric string) []TrendPoint
}

type MetricsCollectorAdapter struct {
	GetScreeningRateFunc   func() float64
	GetMetricsSnapshotFunc func() MetricsSnapshot
}

func (a *MetricsCollectorAdapter) GetScreeningRate() float64 {
	return a.GetScreeningRateFunc()
}

func (a *MetricsCollectorAdapter) GetMetricsSnapshot() MetricsSnapshot {
	return a.GetMetricsSnapshotFunc()
}

type MetricsHistoryAdapter struct {
	GetTrendFunc func(metric string) []TrendPoint
}

func (a *MetricsHistoryAdapter) GetTrend(metric string) []TrendPoint {
	return a.GetTrendFunc(metric)
}

// NewMetricsService creates a new MetricsService
func NewMetricsService(collector MetricsCollector, history MetricsHistory) *MetricsService {
	return &MetricsService{
		collector: collector,
		history:   history,
	}
}

// GetMetrics returns metrics based on the requested type
func (s *MetricsService) GetMetrics(metricType string) any {
	switch metricType {
	case "screening":
		return map[string]any{
			"screening_rate":   s.collector.GetScreeningRate(),
			"screening_total":  s.collector.GetMetricsSnapshot().ScreeningTotal,
			"screening_passed": s.collector.GetMetricsSnapshot().ScreeningPassed,
		}
	case "alerts":
		snapshot := s.collector.GetMetricsSnapshot()
		return map[string]any{
			"alerts_triggered":    snapshot.AlertsTriggered,
			"alerts_acknowledged": snapshot.AlertsAcknowledged,
			"alerts_by_type":      snapshot.AlertsByType,
		}
	case "all", "":
		return s.collector.GetMetricsSnapshot()
	default:
		return s.collector.GetMetricsSnapshot()
	}
}

// GetMetricsTrend returns trend data for a metric over a time period
func (s *MetricsService) GetMetricsTrend(metric, period string) map[string]any {
	if metric == "" {
		metric = "screening_rate"
	}
	if period == "" {
		period = "24h"
	}

	var duration time.Duration
	switch period {
	case "24h":
		duration = 24 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	case "30d":
		duration = 30 * 24 * time.Hour
	default:
		duration = 24 * time.Hour
	}

	snapshot := s.collector.GetMetricsSnapshot()
	trend := s.history.GetTrend(metric)

	var filteredTrend []TrendPoint
	cutoff := time.Now().Add(-duration)
	for _, point := range trend {
		if point.Timestamp.After(cutoff) {
			filteredTrend = append(filteredTrend, point)
		}
	}

	return map[string]any{
		"metric":      metric,
		"period":      period,
		"duration":    duration.String(),
		"current":     snapshot,
		"trend":       filteredTrend,
		"data_points": len(filteredTrend),
	}
}

type DataQualityCheckerInterface interface {
	RunAll(ctx context.Context) *DataQualityReport
}

func (s *MetricsService) CheckDataQuality(checker DataQualityCheckerInterface) *DataQualityReport {
	if checker == nil {
		return &DataQualityReport{
			Checks:      make([]DataQualityCheck, 0),
			Overall:     StatusOK,
			Score:       100.0,
			GeneratedAt: time.Now(),
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return checker.RunAll(ctx)
}
