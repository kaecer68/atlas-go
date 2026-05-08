package autobacktest

import (
	"fmt"
	"log"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/ledger"
	"github.com/kaecer68/atlas-go/internal/replay"
)

type Runner struct {
	btRunner *backtest.Runner
	cfg      config.Config
}

func NewRunner(cfg config.Config) *Runner {
	store := ledger.NewStore(cfg.LedgerDir)
	return &Runner{
		btRunner: backtest.NewRunner(cfg, store),
		cfg:      cfg,
	}
}

func (r *Runner) RunAndStore() error {
	targetDate, err := r.mostRecentTradingDay()
	if err != nil {
		return fmt.Errorf("mostRecentTradingDay: %w", err)
	}

	latest, err := NewHistory(r.cfg.LedgerDir).LatestN(1)
	if err == nil && len(latest) > 0 && latest[0].Date.Equal(targetDate) {
		log.Printf("[Autobacktest] snapshot for %s already exists; skipping", targetDate.Format("2006-01-02"))
		return nil
	}

	summary, err := r.btRunner.Run(targetDate, targetDate)
	if err != nil {
		return fmt.Errorf("backtest run for %s: %w", targetDate.Format("2006-01-02"), err)
	}

	if _, err := r.btRunner.GenerateReport(summary); err != nil {
		return fmt.Errorf("generate report: %w", err)
	}

	if err := r.recordSnapshot(targetDate); err != nil {
		return fmt.Errorf("record snapshot: %w", err)
	}

	return nil
}

func (r *Runner) recordSnapshot(date time.Time) error {
	cmp := NewComparator(r.cfg.LedgerDir)
	eng := NewSignalEngine(r.cfg.LedgerDir)

	sig, err := eng.Evaluate()
	if err != nil {
		return err
	}

	var active []string
	for _, s := range sig.Active {
		active = append(active, string(s))
	}

	portComp, err := cmp.ComparePortfolio()
	if err != nil {
		return err
	}

	sharpeComp, err := cmp.CompareSharpe()
	if err != nil {
		return err
	}

	summaries, err := cmp.store.LoadSessionSummaries()
	if err != nil {
		return err
	}

	var pv float64
	if len(summaries) > 0 {
		last := summaries[len(summaries)-1]
		pv = last.PortfolioValue
		if pv == 0 {
			pv = last.EndingCash
		}
	}

	hist := NewHistory(r.cfg.LedgerDir)
	snap := AutoSnapshot{
		Date:          date,
		PortfolioVal:  pv,
		VaR95:         sig.VaR95,
		SharpeShort:   sharpeComp.ShortTermAvg,
		SharpeLong:    sharpeComp.LongTermAvg,
		DrawdownPct:   sig.DrawdownPct,
		SignalCount:   len(active),
		ActiveSignals: active,
		ShortTermAvg:  portComp.ShortTermAvg,
		LongTermAvg:   portComp.LongTermAvg,
		DeltaPct:      portComp.DeltaPct,
	}

	return hist.Append(snap)
}

func (r *Runner) mostRecentTradingDay() (time.Time, error) {
	ds, err := replay.LoadTWSEOpenDataCSV(r.cfg.ReplayDataPath)
	if err != nil {
		return time.Time{}, fmt.Errorf("load replay data: %w", err)
	}

	if len(ds.Dates) == 0 {
		return time.Time{}, fmt.Errorf("no dates found in replay data")
	}

	return ds.Dates[len(ds.Dates)-1], nil
}
