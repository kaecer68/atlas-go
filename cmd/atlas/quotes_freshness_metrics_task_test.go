package main

// quotes_freshness_metrics_export 的註冊/跳過行為測試（F54 phase 1）。

import (
	"context"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/apigateway"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/monitoring"
)

// stubQuoteStore 實作 ledger.QuoteStore + ledger.QuoteMaxDater。
type stubQuoteStore struct {
	maxDate time.Time
	err     error
	called  int
}

func (s *stubQuoteStore) RecordQuotes([]domain.DailyBar) error { return nil }
func (s *stubQuoteStore) LoadQuotes(string, time.Time, time.Time) ([]domain.DailyBar, error) {
	return nil, nil
}
func (s *stubQuoteStore) LoadLatestQuotes([]string) (map[string]domain.DailyBar, error) {
	return nil, nil
}
func (s *stubQuoteStore) MaxQuoteDate(context.Context) (time.Time, error) {
	s.called++
	return s.maxDate, s.err
}

// jsonlLikeStore 只實作 QuoteStore（不實作 QuoteMaxDater），模擬 JSONL 後端。
type jsonlLikeStore struct{}

func (jsonlLikeStore) RecordQuotes([]domain.DailyBar) error { return nil }
func (jsonlLikeStore) LoadQuotes(string, time.Time, time.Time) ([]domain.DailyBar, error) {
	return nil, nil
}
func (jsonlLikeStore) LoadLatestQuotes([]string) (map[string]domain.DailyBar, error) {
	return nil, nil
}

func TestRegisterQuotesFreshnessMetricsTask_UnsupportedBackendSkipped(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerQuotesFreshnessMetricsTask(backfillDeps{
		taskMgr:    mgr,
		collector:  monitoring.NewMetricsCollector(),
		quoteStore: jsonlLikeStore{},
	})
	if taskRegistered(mgr) {
		t.Fatalf("unsupported backend must not register the task")
	}
}

func TestRegisterQuotesFreshnessMetricsTask_NilStoreSkipped(t *testing.T) {
	mgr := apigateway.NewBackgroundTaskManager(nil)
	registerQuotesFreshnessMetricsTask(backfillDeps{
		taskMgr:   mgr,
		collector: monitoring.NewMetricsCollector(),
	})
	if taskRegistered(mgr) {
		t.Fatalf("nil quote store must not register the task")
	}
}

func TestExportQuotesFreshnessMetrics_EmitsGauges(t *testing.T) {
	collector := monitoring.NewMetricsCollector()
	store := &stubQuoteStore{maxDate: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)}
	now := time.Date(2026, 10, 2, 20, 0, 0, 0, time.UTC)
	obs := exportQuotesFreshnessMetrics(context.Background(), store, collector, now)
	if store.called != 1 {
		t.Fatalf("MaxQuoteDate called %d times, want 1", store.called)
	}
	if !obs.RunOK {
		t.Fatalf("RunOK = false, want true")
	}
	if _, ok := collector.GetMetric(monitoring.MetricQuotesFreshnessCheckedTimestamp, nil); !ok {
		t.Fatalf("heartbeat gauge %s not emitted", monitoring.MetricQuotesFreshnessCheckedTimestamp)
	}
	if _, ok := collector.GetMetric(monitoring.MetricQuotesMaxDateTimestamp, nil); !ok {
		t.Fatalf("gauge %s not emitted", monitoring.MetricQuotesMaxDateTimestamp)
	}
}

func taskRegistered(mgr *apigateway.BackgroundTaskManager) bool {
	_, ok := mgr.Get("quotes_freshness_metrics_export")
	return ok
}

var _ ledger.QuoteStore = (*stubQuoteStore)(nil)
var _ ledger.QuoteMaxDater = (*stubQuoteStore)(nil)
