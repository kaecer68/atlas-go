package orchestrator

import (
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
)

func testDay() time.Time { return time.Date(2026, 9, 24, 0, 0, 0, 0, time.UTC) }

// TestSyntheticPlaceholderReturn_DeterministicAndDayScoped pins the seed
// contract: same (agent, symbol, day) => identical value; a different day => a
// different draw, so a rolling window is not one repeated number.
func TestSyntheticPlaceholderReturn_DeterministicAndDayScoped(t *testing.T) {
	day := testDay()
	first := syntheticPlaceholderReturn("2330.TW", "agent-a", domain.RegimeRiskOn, day)
	for range 5 {
		if got := syntheticPlaceholderReturn("2330.TW", "agent-a", domain.RegimeRiskOn, day); got != first {
			t.Fatalf("not deterministic: %v then %v", first, got)
		}
	}

	seen := map[float64]bool{first: true}
	for d := 1; d <= 5; d++ {
		seen[syntheticPlaceholderReturn("2330.TW", "agent-a", domain.RegimeRiskOn, day.AddDate(0, 0, d))] = true
	}
	if len(seen) < 4 {
		t.Fatalf("expected day-scoped variation, got %d unique values over 6 days", len(seen))
	}
}

// TestSyntheticPlaceholderReturn_AgentScoped keeps the A4 L2 guarantee: agents
// recommending the same symbol on the same day must not all share one value.
func TestSyntheticPlaceholderReturn_AgentScoped(t *testing.T) {
	seen := map[float64]string{}
	for _, agent := range []string{"agent-a", "agent-b", "agent-c", "agent-d", "agent-e"} {
		v := syntheticPlaceholderReturn("2330.TW", agent, domain.RegimeRiskOn, testDay())
		if prev, dup := seen[v]; dup {
			t.Fatalf("agents %q and %q share value %v for the same symbol/day", prev, agent, v)
		}
		seen[v] = agent
	}
}

// TestSyntheticPlaceholderReturn_WithinRegimeBand bounds the placeholder by the
// configured regime band (risk-on +/-5%, risk-off +/-3%).
func TestSyntheticPlaceholderReturn_WithinRegimeBand(t *testing.T) {
	day := testDay()
	for i := range 400 {
		symbol := "SYM" + string(rune('A'+i%26)) + itoa(i)
		if got := syntheticPlaceholderReturn(symbol, "agent-a", domain.RegimeRiskOn, day); got < -0.05 || got > 0.05 {
			t.Fatalf("risk-on value %v outside [-0.05, 0.05]", got)
		}
		if got := syntheticPlaceholderReturn(symbol, "agent-a", domain.RegimeRiskOff, day); got < -0.03 || got > 0.03 {
			t.Fatalf("risk-off value %v outside [-0.03, 0.03]", got)
		}
	}
}

// TestSyntheticPlaceholderReturn_NotSystematicallyMiss ensures the placeholder
// is centred noise rather than a value that is always <= 0 (the old
// flat-day path returned exactly 0 => Hit=false, i.e. a guaranteed miss).
func TestSyntheticPlaceholderReturn_NotSystematicallyMiss(t *testing.T) {
	day := testDay()
	positive := 0
	total := 200
	for i := range total {
		if syntheticPlaceholderReturn("SYM"+itoa(i), "agent-a", domain.RegimeRiskOn, day) > 0 {
			positive++
		}
	}
	share := float64(positive) / float64(total)
	if share < 0.2 || share > 0.8 {
		t.Fatalf("placeholder positive share = %.2f, want roughly balanced (0.2-0.8)", share)
	}
}

// TestBuildSyntheticOutcomes_IgnoresSameDayPriceAction is the regression test
// for issue #1944 (I20): the placeholder must NOT be derived from the same-day
// intraday move, otherwise `Hit` is computed from the same input as the
// signal (self-fulfilling) — an up day must not mechanically turn every held
// recommendation into a hit, and a down day must not turn every one into a miss.
func TestBuildSyntheticOutcomes_IgnoresSameDayPriceAction(t *testing.T) {
	recs := []domain.Recommendation{{Agent: "agent-a", Symbol: "2330.TW", Side: domain.SideBuy, Conviction: 80}}
	upDay := []domain.Quote{{Symbol: "2330.TW", Open: 100, Last: 110}}
	downDay := []domain.Quote{{Symbol: "2330.TW", Open: 100, Last: 90}}

	upOut := buildSyntheticOutcomes(recs, recs, upDay, testDay(), string(domain.RegimeRiskOn), nil)
	downOut := buildSyntheticOutcomes(recs, recs, downDay, testDay(), string(domain.RegimeRiskOn), nil)
	if len(upOut) != 1 || len(downOut) != 1 {
		t.Fatalf("expected 1 outcome each, got %d and %d", len(upOut), len(downOut))
	}

	if upOut[0].ForwardReturn != downOut[0].ForwardReturn {
		t.Fatalf("placeholder still tracks same-day price action: up=%v down=%v",
			upOut[0].ForwardReturn, downOut[0].ForwardReturn)
	}
	if !upOut[0].IsSynthetic {
		t.Error("synthetic outcomes must stay flagged IsSynthetic=true")
	}
	if upOut[0].Hit != (upOut[0].ForwardReturn > 0) {
		t.Errorf("Hit=%v inconsistent with ForwardReturn=%v", upOut[0].Hit, upOut[0].ForwardReturn)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	return string(buf)
}
