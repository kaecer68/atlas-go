package server

import (
	"context"
	"fmt"
	"testing"

	"github.com/kaecer68/atlas-go/internal/stockpicker"
)

// callStockpickerWalkforward is a tiny wrapper reducing boilerplate.
func callStockpickerWalkforward(s *server, in stockpickerWalkforwardInput) (stockpickerWalkforwardOutput, error) {
	_, out, err := s.handleStockpickerWalkforward(context.Background(), nil, in)
	return out, err
}

func TestHandleStockpickerWalkforward_HasData(t *testing.T) {
	s, db := stockWinRateHarness(t)
	outStore := stockpicker.NewSignalOutcomeStore(db)
	var outcomes []stockpicker.SignalOutcome
	for d := 1; d <= 30; d++ {
		r := 0.03
		if d%2 == 0 {
			r = -0.01
		}
		outcomes = append(outcomes, stockpicker.SignalOutcome{
			Symbol: "2330", TriggerDate: fmt.Sprintf("2026-09-%02d", d),
			ForwardReturn: r, Source: retainedScanSource, CostRate: 0.00585,
		})
	}
	if err := outStore.RecordOutcomes(context.Background(), outcomes); err != nil {
		t.Fatalf("record outcomes: %v", err)
	}

	out, err := callStockpickerWalkforward(s, stockpickerWalkforwardInput{
		ConditionID: retainedScanCond, Folds: 3, MinTrain: 7, ForwardDays: 5,
		AsOf: "2026-10-01",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !out.Found {
		t.Fatalf("found=false, want true (message=%q)", out.Message)
	}
	if len(out.Folds) != 2 {
		t.Fatalf("folds = %d, want 2: %+v", len(out.Folds), out.Folds)
	}
	if out.Folds[0].ValStart != "2026-09-15" {
		t.Errorf("fold0 val_start = %s, want 2026-09-15", out.Folds[0].ValStart)
	}
	if out.Distribution.N != 2 {
		t.Errorf("distribution.n = %d, want 2", out.Distribution.N)
	}
	if out.Distribution.Mean <= 0 || out.Distribution.Mean >= 1 {
		t.Errorf("distribution.mean = %v, want in (0,1)", out.Distribution.Mean)
	}
}

func TestHandleStockpickerWalkforward_NoData(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	out, err := callStockpickerWalkforward(s, stockpickerWalkforwardInput{
		ConditionID: "no-such-condition", AsOf: "2026-10-01",
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if out.Found {
		t.Fatalf("found=true, want false")
	}
}

func TestHandleStockpickerWalkforward_RequiresCondition(t *testing.T) {
	s, _ := stockWinRateHarness(t)
	if _, err := callStockpickerWalkforward(s, stockpickerWalkforwardInput{}); err == nil {
		t.Fatal("empty condition_id must error")
	}
}
