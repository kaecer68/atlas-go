package service

import (
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/logging"
)

type BacktestService struct {
	mu      sync.Mutex
	running bool
	status  map[string]any
}

func NewBacktestService() *BacktestService {
	return &BacktestService{
		status: make(map[string]any),
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
		cfg := config.Load()
		if cfg.ReplayDataPath == "samples/replay/twse_stock_day_all_sample.csv" {
			cfg.ReplayDataPath = "data/replay/tw_extended_90days.csv"
		}
		store := ledger.NewStore(cfg.LedgerDir)
		runner := backtest.NewRunner(cfg, store)
		summary, err := runner.Run(startDate, endDate)
		if err == nil {
			if _, rerr := runner.GenerateReport(summary); rerr != nil {
				logging.Error("backtest_service", "report_generation_failed", logging.Err(rerr))
			}
		}

		s.mu.Lock()
		s.running = false
		s.status["running"] = false
		s.status["finished_at"] = time.Now().UTC()
		if err != nil {
			s.status["error"] = err.Error()
		} else {
			s.status["window_id"] = summary.WindowID
			s.status["sessions"] = summary.SessionCount
			s.status["outcomes"] = summary.OutcomeCount
			s.status["worst_agent"] = summary.WorstAgentID
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
	return status
}
