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
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type BacktestService struct {
	mu      sync.Mutex
	running bool
	status  map[string]any
	cfg     config.Config
}

func NewBacktestService(cfg config.Config) *BacktestService {
	return &BacktestService{
		status: make(map[string]any),
		cfg:    cfg,
	}
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
			store := ledger.NewStore(s.cfg.LedgerDir)
			runner := backtest.NewRunner(s.cfg, store)
			summary, err := runner.Run(startDate, endDate)
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
