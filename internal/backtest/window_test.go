package backtest

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/config"
)

func TestRunWindow(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner(config.Config{
		ReplayMode:        "daily",
		PrimaryMarket:     "TW",
		ReplayDataPath:    "../../samples/replay/twse_stock_day_all_sample.csv",
		LedgerDir:         dir,
		AgentRegistryPath: "../../configs/agents.json",
		ReplaySessionDate: "2026-03-26",
	})

	start, _ := time.Parse("2006-01-02", "2026-03-26")
	end, _ := time.Parse("2006-01-02", "2026-03-27")
	summary, err := runner.Run(start, end)
	if err != nil {
		t.Fatalf("run window: %v", err)
	}
	if summary.SessionCount == 0 {
		t.Fatalf("expected sessions to run")
	}
}
