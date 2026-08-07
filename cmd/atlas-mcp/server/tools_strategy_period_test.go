package server

// feat/20260807-m1-m4-period-strategy-tools — M4 strategy_for_period 測試。

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// chdirRepoRoot 讓測試以 repo root 為 cwd（與 production atlas-mcp 相同），
// 確保 configs/methodology_rules.yaml 可被 TryLoadMethodologyRules 讀取。
func chdirRepoRoot(t *testing.T) {
	t.Helper()
	t.Chdir("../../../") // cmd/atlas-mcp/server → repo root
}

func TestStrategyForPeriod_ReturnsAllowedStrategies(t *testing.T) {
	chdirRepoRoot(t)
	s := &server{}
	_, out, err := s.handleStrategyForPeriod(context.Background(), nil, StrategyForPeriodInput{Period: "downturn"})
	if err != nil {
		t.Fatalf("handleStrategyForPeriod: %v", err)
	}

	if out.Period != "downturn" {
		t.Errorf("period = %q, want downturn", out.Period)
	}
	if out.PeriodNameZH != "低迷" {
		t.Errorf("period_name_zh = %q, want 低迷", out.PeriodNameZH)
	}
	if len(out.Allowed) == 0 {
		t.Error("allowed must be non-empty for a known period")
	}
	if len(out.Strategies) == 0 {
		t.Error("strategies must be non-empty for a known period")
	}

	// 每個 strategy brief 要有 category + priority
	for _, s := range out.Strategies {
		if s.Category == "" {
			t.Errorf("strategy %s missing category", s.ID)
		}
		if s.Priority != "primary" && s.Priority != "secondary" {
			t.Errorf("strategy %s priority = %q, want primary/secondary", s.ID, s.Priority)
		}
	}

	// KnownPeriods 完整
	if len(out.KnownPeriods) != 7 {
		t.Errorf("known_periods = %d, want 7", len(out.KnownPeriods))
	}
}

func TestStrategyForPeriod_AllSevenPeriods(t *testing.T) {
	chdirRepoRoot(t)
	s := &server{}
	periods := []string{"downturn", "turnaround_up", "bull", "plateau", "consolidation", "turnaround_down", "black_swan"}
	for _, p := range periods {
		t.Run(p, func(t *testing.T) {
			_, out, err := s.handleStrategyForPeriod(context.Background(), nil, StrategyForPeriodInput{Period: p})
			if err != nil {
				t.Fatalf("handleStrategyForPeriod(%s): %v", p, err)
			}
			if out.PeriodNameZH == "" {
				t.Errorf("period %s: name_zh empty", p)
			}
		})
	}
}

func TestStrategyForPeriod_UnknownPeriod(t *testing.T) {
	s := &server{}
	_, out, err := s.handleStrategyForPeriod(context.Background(), nil, StrategyForPeriodInput{Period: "not_a_period"})
	if err == nil {
		t.Fatal("unknown period must return error")
	}
	var unknown *unknownPeriodError
	if !errors.As(err, &unknown) {
		t.Errorf("error type = %T, want *unknownPeriodError", err)
	}
	if len(out.KnownPeriods) == 0 {
		t.Error("unknown period error should include known_periods for agent recovery")
	}
}

func TestStrategyForPeriod_EmptyPeriod(t *testing.T) {
	s := &server{}
	_, _, err := s.handleStrategyForPeriod(context.Background(), nil, StrategyForPeriodInput{})
	if err == nil {
		t.Fatal("empty period must return error")
	}
}

func TestStrategyForPeriod_JSONSerializable(t *testing.T) {
	chdirRepoRoot(t)
	s := &server{}
	_, out, err := s.handleStrategyForPeriod(context.Background(), nil, StrategyForPeriodInput{Period: "bull"})
	if err != nil {
		t.Fatalf("handleStrategyForPeriod: %v", err)
	}
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundTrip["period"] != "bull" {
		t.Errorf("round-trip period = %v, want bull", roundTrip["period"])
	}
}
