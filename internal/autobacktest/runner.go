package autobacktest

import (
	"fmt"
	"time"

	"github.com/kaecer68/atlas-go/internal/backtest"
	"github.com/kaecer68/atlas-go/internal/config"
	"github.com/kaecer68/atlas-go/internal/eventbus"
	"github.com/kaecer68/atlas-go/internal/ledger"
	livestore "github.com/kaecer68/atlas-go/internal/live/store"
	"github.com/kaecer68/atlas-go/internal/logging"
	"github.com/kaecer68/atlas-go/internal/replay"
)

type Runner struct {
	btRunner *backtest.Runner
	cfg      config.Config
	eventBus *eventbus.ChannelEventBus
}

func NewRunner(cfg config.Config) *Runner {
	store := ledger.NewStore(cfg.LedgerDir)
	return &Runner{
		btRunner: backtest.NewRunner(cfg, store),
		cfg:      cfg,
	}
}

func NewRunnerWithEventBus(cfg config.Config, eventBus *eventbus.ChannelEventBus) *Runner {
	store := ledger.NewStore(cfg.LedgerDir)
	btRunner := backtest.NewRunner(cfg, store)
	if eventBus != nil {
		btRunner.WithEventBus(eventBus)
	}
	return &Runner{
		btRunner: btRunner,
		cfg:      cfg,
		eventBus: eventBus,
	}
}

func (r *Runner) RunAndStore() error {
	targetDate, err := r.mostRecentTradingDay()
	if err != nil {
		return fmt.Errorf("mostRecentTradingDay: %w", err)
	}

	latest, err := NewHistory(r.cfg.LedgerDir).LatestN(1)
	if err == nil && len(latest) > 0 && latest[0].Date.Equal(targetDate) {
		logging.Info("autobacktest", "snapshot_exists_skip", "target_date", targetDate.Format("2006-01-02"))
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

	r.syncToLiveStore()

	if r.eventBus != nil {
		r.eventBus.PublishBacktestCompleted(eventbus.BacktestCompletedEventPayload{
			WindowID:              summary.WindowID,
			StartDate:             summary.StartDate,
			EndDate:               summary.EndDate,
			SessionCount:          summary.SessionCount,
			OutcomeCount:          summary.OutcomeCount,
			WorstAgentID:          summary.WorstAgentID,
			WorstAgentSkill:       summary.WorstAgentSkill,
			WorstAgentLayer:       string(summary.WorstAgentLayer),
			WorstAgentWindowCount: summary.WorstAgentWindowCount,
			WorstAgentSharpeLike:  summary.WorstAgentSharpeLike,
			GeneratedAt:           summary.GeneratedAt,
			TargetDate:            targetDate,
			SyncSucceeded:         true,
		})
	}

	return nil
}

func (r *Runner) syncToLiveStore() {
	state := r.btRunner.LastState()
	if state == nil {
		return
	}

	store := livestore.NewStateStore(livestore.DefaultLiveStateBasePath)
	if err := store.Load(); err != nil {
		logging.Warn("autobacktest", "load_live_state_failed", "err", err.Error())
	}

	for symbol := range store.GetPositions() {
		store.RemovePosition(symbol)
	}

	var totalExposure, totalUnrealizedPnL float64
	for _, pos := range state.Positions {
		totalExposure += pos.MarketValue
		totalUnrealizedPnL += pos.UnrealizedPnL
		store.UpdatePosition(pos)
	}

	store.UpdatePortfolio(livestore.PortfolioState{
		Cash:          state.Cash,
		TotalExposure: totalExposure,
		AvailableCash: state.Cash,
		UnrealizedPnL: totalUnrealizedPnL,
		LastUpdated:   time.Now(),
	})

	if err := store.Save(); err != nil {
		logging.Warn("autobacktest", "sync_live_state_failed", "err", err.Error())
	} else {
		logging.Info("autobacktest", "synced_to_live_store",
			"positions", len(state.Positions),
			"exposure", totalExposure,
			"cash", state.Cash)
	}
}

func (r *Runner) recordSnapshot(date time.Time) error {
	cmp := NewComparator(r.cfg.LedgerDir)
	eng, err := NewSignalEngine(r.cfg.LedgerDir)
	if err != nil {
		return fmt.Errorf("create signal engine: %w", err)
	}

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

	now := time.Now()
	for i := len(ds.Dates) - 1; i >= 0; i-- {
		if ds.Dates[i].After(now) {
			continue
		}
		if _, ok := ds.NextDate(ds.Dates[i], 1); ok {
			return ds.Dates[i], nil
		}
	}

	return ds.Dates[len(ds.Dates)-1], nil
}
