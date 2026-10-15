package service

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/autobacktest"
	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type BacktestService struct {
	mu       sync.Mutex
	running  bool
	status   map[string]any
	cfg      config.Config
	eventBus *eventbus.ChannelEventBus

	// store backs the L-cold session-summary read of the backtest runner
	// (Q2 batch 2 unit 2) through the same backend-aware factory as the
	// performance report (PostgreSQL first, JSONL fallback, degraded
	// tracked). When nil the runner keeps the legacy JSONL ledger store
	// under cfg.LedgerDir, so this seam is additive.
	store ledger.OutcomeStore
}

func NewBacktestService(cfg config.Config) *BacktestService {
	return &BacktestService{
		status: make(map[string]any),
		cfg:    cfg,
	}
}

func (s *BacktestService) WithEventBus(eventBus *eventbus.ChannelEventBus) *BacktestService {
	s.eventBus = eventBus
	return s
}

// WithStore wires the backend-aware ledger store used by the backtest runner
// for its session-summary read (PG first, JSONL fallback). When nil the runner
// keeps the legacy JSONL ledger read (tests/legacy).
func (s *BacktestService) WithStore(store ledger.OutcomeStore) *BacktestService {
	s.store = store
	return s
}

// summariesStore resolves the store used by the backtest runner: the injected
// backend-aware store when wired, the legacy JSONL ledger otherwise.
func (s *BacktestService) summariesStore() ledger.OutcomeStore {
	if s.store != nil {
		return s.store
	}
	return ledger.NewStore(s.cfg.LedgerDir)
}

// logReadSource records which backend actually served the runner's summary
// read (SourceBackend()/Degraded() of the PG-first store), so a JSONL
// fallback on a PG deployment is visible in the log.
func (s *BacktestService) logReadSource(event string) {
	if s.store == nil {
		return
	}
	logging.Info("backtest_service", event,
		logging.FStr("source", storeSourceBackend(s.store)),
		logging.FBool("degraded", storeDegraded(s.store)))
}

func (s *BacktestService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *BacktestService) Start(startDate, endDate time.Time) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("backtest already running")
	}
	s.running = true
	s.status = map[string]any{
		"running":    true,
		"started_at": time.Now().UTC(),
		"start":      startDate.Format("2006-01-02"),
		"end":        endDate.Format("2006-01-02"),
	}
	s.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		type result struct {
			summary domain.BacktestWindowSummary
			err     error
		}
		ch := make(chan result, 1)
		go func() {
			store := s.summariesStore()
			runner := backtest.NewRunner(s.cfg, store)
			if s.eventBus != nil {
				runner.WithEventBus(s.eventBus)
			}
			summary, err := runner.Run(startDate, endDate)
			// The runner read its session summaries from this store; label the
			// backend that answered (PG-first vs JSONL fallback).
			s.logReadSource("backtest_summaries_read")
			if err == nil {
				if _, rerr := runner.GenerateReport(summary); rerr != nil {
					logging.Error("backtest_service", "report_generation_failed", logging.Err(rerr))
				}
			}
			ch <- result{summary: summary, err: err}
		}()

		var res result
		select {
		case <-ctx.Done():
			res.err = fmt.Errorf("backtest timed out after 10 minutes")
		case res = <-ch:
		}

		s.mu.Lock()
		s.running = false
		s.status["running"] = false
		s.status["finished_at"] = time.Now().UTC()
		if res.err != nil {
			s.status["error"] = res.err.Error()
		} else {
			s.status["window_id"] = res.summary.WindowID
			s.status["sessions"] = res.summary.SessionCount
			s.status["outcomes"] = res.summary.OutcomeCount
			s.status["worst_agent"] = res.summary.WorstAgentID
		}
		s.mu.Unlock()
	}()

	return nil
}

func (s *BacktestService) GetStatus() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := make(map[string]any, len(s.status))
	maps.Copy(status, s.status)

	hist := autobacktest.NewHistory(s.cfg.LedgerDir)
	if snaps, err := hist.LatestN(1); err == nil && len(snaps) > 0 {
		status["last_auto_date"] = snaps[0].Date.Format("2006-01-02")
		status["last_auto_portfolio_val"] = snaps[0].PortfolioVal
	}

	return status
}

func (s *BacktestService) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.status["running"] = false
}
