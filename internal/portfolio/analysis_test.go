package portfolio

import (
	"testing"
	"time"
)

func TestTradeRecordIsWin(t *testing.T) {
	win := TradeRecord{Pnl: 100}
	if !win.IsWin() {
		t.Error("expected IsWin=true for positive PnL")
	}

	loss := TradeRecord{Pnl: -50}
	if loss.IsWin() {
		t.Error("expected IsWin=false for negative PnL")
	}
}

func TestTradeRecordHoldingPeriod(t *testing.T) {
	now := time.Now()
	trade := TradeRecord{
		EntryTime: now.Add(-2 * time.Hour),
		ExitTime:  now,
	}
	if trade.HoldingPeriod() != 2*time.Hour {
		t.Errorf("expected 2h holding period, got %v", trade.HoldingPeriod())
	}
}

func TestNewPostTradeAnalyzer(t *testing.T) {
	a := NewPostTradeAnalyzer()
	if a == nil {
		t.Fatal("expected non-nil analyzer")
	}
	if len(a.GetTrades()) != 0 {
		t.Error("expected empty trades")
	}
}

func TestPostTradeAnalyzerAddTrade(t *testing.T) {
	a := NewPostTradeAnalyzer()
	trade := TradeRecord{Symbol: "2330.TW", Pnl: 100}
	a.AddTrade(trade)
	if len(a.GetTrades()) != 1 {
		t.Fatalf("expected 1 trade, got %d", len(a.GetTrades()))
	}
	if a.GetTrades()[0].Symbol != "2330.TW" {
		t.Errorf("expected symbol 2330.TW, got %s", a.GetTrades()[0].Symbol)
	}
}

func TestPostTradeAnalyzerCalculateMetrics(t *testing.T) {
	a := NewPostTradeAnalyzer()
	m := a.CalculateMetrics()
	if m.TotalTrades != 0 {
		t.Errorf("expected 0 trades for empty analyzer, got %d", m.TotalTrades)
	}

	a.AddTrade(TradeRecord{Symbol: "A", Pnl: 100, PnlPct: 0.05, EntryTime: time.Now(), ExitTime: time.Now()})
	a.AddTrade(TradeRecord{Symbol: "B", Pnl: -50, PnlPct: -0.02, EntryTime: time.Now(), ExitTime: time.Now()})
	a.AddTrade(TradeRecord{Symbol: "C", Pnl: 200, PnlPct: 0.10, EntryTime: time.Now(), ExitTime: time.Now()})

	m = a.CalculateMetrics()
	if m.TotalTrades != 3 {
		t.Errorf("expected 3 trades, got %d", m.TotalTrades)
	}
	if m.WinningTrades != 2 {
		t.Errorf("expected 2 wins, got %d", m.WinningTrades)
	}
	if m.LosingTrades != 1 {
		t.Errorf("expected 1 loss, got %d", m.LosingTrades)
	}
	if m.TotalPnL != 250 {
		t.Errorf("expected total PnL 250, got %f", m.TotalPnL)
	}
	if m.WinRate != 2.0/3.0 {
		t.Errorf("expected win rate %.2f, got %f", 2.0/3.0, m.WinRate)
	}
}

func TestPostTradeAnalyzerAttributionByAgent(t *testing.T) {
	a := NewPostTradeAnalyzer()
	a.AddTrade(TradeRecord{Symbol: "A", Agent: "agent-1", Pnl: 100})
	a.AddTrade(TradeRecord{Symbol: "B", Agent: "agent-1", Pnl: 50})
	a.AddTrade(TradeRecord{Symbol: "C", Agent: "agent-2", Pnl: -30})

	attr := a.AttributionByAgent()
	if attr["agent-1"] != 150 {
		t.Errorf("expected agent-1 contribution 150, got %f", attr["agent-1"])
	}
	if attr["agent-2"] != -30 {
		t.Errorf("expected agent-2 contribution -30, got %f", attr["agent-2"])
	}
}

func TestPostTradeAnalyzerAttributionBySymbol(t *testing.T) {
	a := NewPostTradeAnalyzer()
	a.AddTrade(TradeRecord{Symbol: "2330.TW", Pnl: 100})
	a.AddTrade(TradeRecord{Symbol: "2317.TW", Pnl: -50})

	attr := a.AttributionBySymbol()
	if attr["2330.TW"] != 100 {
		t.Errorf("expected 2330.TW contribution 100, got %f", attr["2330.TW"])
	}
	if attr["2317.TW"] != -50 {
		t.Errorf("expected 2317.TW contribution -50, got %f", attr["2317.TW"])
	}
}

func TestPostTradeAnalyzerCalculateAgentStats(t *testing.T) {
	a := NewPostTradeAnalyzer()
	now := time.Now()
	a.AddTrade(TradeRecord{Symbol: "A", Agent: "agent-1", Pnl: 100, ExitTime: now})
	a.AddTrade(TradeRecord{Symbol: "B", Agent: "agent-1", Pnl: -50, ExitTime: now})
	a.AddTrade(TradeRecord{Symbol: "C", Agent: "agent-2", Pnl: 200, ExitTime: now})

	stats := a.CalculateAgentStats()
	if len(stats) != 2 {
		t.Fatalf("expected 2 agent stats, got %d", len(stats))
	}

	foundAgent1 := false
	for _, s := range stats {
		if s.AgentID == "agent-1" {
			foundAgent1 = true
			if s.TotalTrades != 2 {
				t.Errorf("expected 2 trades for agent-1, got %d", s.TotalTrades)
			}
			if s.WinCount != 1 {
				t.Errorf("expected 1 win for agent-1, got %d", s.WinCount)
			}
			if s.WinRate != 0.5 {
				t.Errorf("expected win rate 0.5, got %f", s.WinRate)
			}
		}
	}
	if !foundAgent1 {
		t.Error("expected agent-1 in stats")
	}
}

func TestPostTradeAnalyzerCalculateDailyPnL(t *testing.T) {
	a := NewPostTradeAnalyzer()
	now := time.Now()
	a.AddTrade(TradeRecord{Symbol: "A", Pnl: 100, ExitTime: now})
	a.AddTrade(TradeRecord{Symbol: "B", Pnl: -50, ExitTime: now})

	daily := a.CalculateDailyPnL()
	if len(daily) != 1 {
		t.Fatalf("expected 1 daily entry, got %d", len(daily))
	}
	if daily[0].PnL != 50 {
		t.Errorf("expected daily PnL 50, got %f", daily[0].PnL)
	}
	if daily[0].Trades != 2 {
		t.Errorf("expected 2 trades, got %d", daily[0].Trades)
	}
}

func TestPostTradeAnalyzerCalculateExecutionQuality(t *testing.T) {
	a := NewPostTradeAnalyzer()
	now := time.Now()
	a.AddTrade(TradeRecord{Symbol: "A", Slippage: 0.5, Commission: 10, EntryTime: now.Add(-time.Hour), ExitTime: now})

	eq := a.CalculateExecutionQuality()
	if eq.AvgSlippage != 0.5 {
		t.Errorf("expected avg slippage 0.5, got %f", eq.AvgSlippage)
	}
	if eq.AvgCommission != 10 {
		t.Errorf("expected avg commission 10, got %f", eq.AvgCommission)
	}

	empty := NewPostTradeAnalyzer()
	eq2 := empty.CalculateExecutionQuality()
	if eq2.AvgSlippage != 0 {
		t.Errorf("expected 0 slippage for empty, got %f", eq2.AvgSlippage)
	}
}

func TestPostTradeAnalyzerClear(t *testing.T) {
	a := NewPostTradeAnalyzer()
	a.AddTrade(TradeRecord{Symbol: "A", Pnl: 100})
	a.Clear()
	if len(a.GetTrades()) != 0 {
		t.Errorf("expected 0 trades after clear, got %d", len(a.GetTrades()))
	}
}

func TestPostTradeAnalyzerFilterByPeriod(t *testing.T) {
	a := NewPostTradeAnalyzer()
	now := time.Now()
	a.AddTrade(TradeRecord{Symbol: "A", Pnl: 100, ExitTime: now.Add(-48 * time.Hour)})
	a.AddTrade(TradeRecord{Symbol: "B", Pnl: 200, ExitTime: now})

	filtered := a.FilterByPeriod(now.Add(-24*time.Hour), now.Add(time.Hour))
	if len(filtered) != 1 {
		t.Fatalf("expected 1 trade in period, got %d", len(filtered))
	}
	if filtered[0].Symbol != "B" {
		t.Errorf("expected trade B, got %s", filtered[0].Symbol)
	}
}

func TestPostTradeAnalyzerGenerateSuggestions(t *testing.T) {
	a := NewPostTradeAnalyzer()
	now := time.Now()
	for range 20 {
		a.AddTrade(TradeRecord{Symbol: "A", Pnl: -10, ExitTime: now})
	}
	suggestions := a.GenerateSuggestions()
	if len(suggestions) == 0 {
		t.Error("expected suggestions for losing streak")
	}
}

func TestPostTradeAnalyzerGenerateReport(t *testing.T) {
	a := NewPostTradeAnalyzer()
	now := time.Now()
	a.AddTrade(TradeRecord{Symbol: "A", Pnl: 100, ExitTime: now})

	report := a.GenerateReport("test-period")
	if report.Metrics.TotalTrades != 1 {
		t.Errorf("expected 1 trade in report, got %d", report.Metrics.TotalTrades)
	}
}
