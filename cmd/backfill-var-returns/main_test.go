package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/risk"
	"github.com/kaecer68/atlas-go/internal/sim"
)

// sessionFixture describes one session directory on disk.
type sessionFixture struct {
	Dir       string
	SessionID string
	PV        float64
	Recorded  time.Time
}

// writeSession creates <root>/<dir>/summary.json with the fields the command
// reads. Summary files in the wild carry more (regime, orders, positions);
// this command only needs these three.
func writeSession(t *testing.T, root string, f sessionFixture) {
	t.Helper()
	dir := filepath.Join(root, f.Dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	summary := map[string]any{
		"session_id":      f.SessionID,
		"portfolio_value": f.PV,
		"recorded_at":     f.Recorded,
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal summary: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), data, 0o644); err != nil {
		t.Fatalf("write summary: %v", err)
	}
}

// engineShapedState is a state file of the shape sim.SavePersistentState
// writes: non-nil positions/curves/previous_values, and the last close carried
// in previous_values["_portfolio_"]. cash 900k + 1000 shares marked at
// portfolioValue-900k gives the requested total.
func engineShapedState(portfolioValue float64) domain.SimulationState {
	state := domain.NewSimulationState(1_000_000)
	state.Cash = 900_000
	state.Positions = []domain.Position{{
		Symbol:       "2330.TW",
		Quantity:     1000,
		AverageCost:  100,
		CurrentPrice: (portfolioValue - 900_000) / 1000,
		MarketValue:  portfolioValue - 900_000,
	}}
	state.EquityCurve = []float64{portfolioValue}
	state.DailyReturns = []float64{}
	state.PreviousValues = map[string]float64{"_portfolio_": portfolioValue}
	return state
}

func writeState(t *testing.T, path string, state domain.SimulationState) {
	t.Helper()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write state: %v", err)
	}
}

func readState(t *testing.T, path string) domain.SimulationState {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var state domain.SimulationState
	if err := json.Unmarshal(data, &state); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	return state
}

// runBackfill runs the command the way main does and fails the test on error.
func runBackfill(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	if err := run(args, &out, &errOut); err != nil {
		t.Fatalf("run(%v) = %v (stderr: %s)", args, err, errOut.String())
	}
	return out.String(), errOut.String()
}

func almostEqual(a, b float64) bool {
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < 1e-12
}

func utcDay(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// TestRebuildWritesSessionDateSemantics is the core of #1935: after the
// rebuild the file must carry the date semantics sim.Engine.RunDay maintains —
// last_session_date names the trading day of the newest entry, and
// session_base_value is the close that entry is measured against.
func TestRebuildWritesSessionDateSemantics(t *testing.T) {
	sessionsDir := t.TempDir()
	writeSession(t, sessionsDir, sessionFixture{"session-20260922-daily", "session-20260922-daily", 1_000_000, utcDay(2026, 9, 22)})
	writeSession(t, sessionsDir, sessionFixture{"session-20260923-daily", "session-20260923-daily", 1_010_000, utcDay(2026, 9, 23)})
	writeSession(t, sessionsDir, sessionFixture{"session-20260924-daily", "session-20260924-daily", 1_005_000, utcDay(2026, 9, 24)})

	stateFile := filepath.Join(t.TempDir(), "simulation_state.json")
	original := engineShapedState(1_010_000)
	writeState(t, stateFile, original)

	runBackfill(t, sessionsDir, stateFile)

	state := readState(t, stateFile)
	if state.LastSessionDate != "2026-09-24" {
		t.Errorf("last_session_date = %q, want 2026-09-24", state.LastSessionDate)
	}
	if !almostEqual(state.SessionBaseValue, 1_010_000) {
		t.Errorf("session_base_value = %v, want the 2026-09-23 close 1010000", state.SessionBaseValue)
	}
	t.Logf("after rebuild: last_session_date=%s session_base_value=%v daily_returns=%v",
		state.LastSessionDate, state.SessionBaseValue, state.DailyReturns)
	want := []float64{0.01, (1_005_000.0 - 1_010_000.0) / 1_010_000.0}
	if len(state.DailyReturns) != len(want) {
		t.Fatalf("daily_returns = %v, want %d entries", state.DailyReturns, len(want))
	}
	for i, w := range want {
		if !almostEqual(state.DailyReturns[i], w) {
			t.Errorf("daily_returns[%d] = %v, want %v", i, state.DailyReturns[i], w)
		}
	}

	// Everything this command does not own must survive the rewrite.
	if state.Cash != original.Cash {
		t.Errorf("cash = %v, want %v (not owned by this command)", state.Cash, original.Cash)
	}
	if len(state.Positions) != len(original.Positions) || state.Positions[0] != original.Positions[0] {
		t.Errorf("positions = %v, want %v (not owned by this command)", state.Positions, original.Positions)
	}
	if !almostEqual(state.PreviousValues["_portfolio_"], 1_010_000) {
		t.Errorf("previous_values[_portfolio_] = %v, want 1010000", state.PreviousValues["_portfolio_"])
	}
	if len(state.EquityCurve) != 1 || !almostEqual(state.EquityCurve[0], 1_010_000) {
		t.Errorf("equity_curve = %v, want the engine-written curve untouched", state.EquityCurve)
	}
	if state.StartingCash != original.StartingCash || state.MaxEquity != original.MaxEquity {
		t.Errorf("starting_cash/max_equity = %v/%v, want %v/%v",
			state.StartingCash, state.MaxEquity, original.StartingCash, original.MaxEquity)
	}
}

// TestRebuildDedupesSameTradingDay pins the de-duplication rule: several
// session directories can describe one trading day
// (session-<YYYYMMDD>-<replay mode>), and feeding all of them in used to emit
// one return per directory, i.e. same-day duplicates.
func TestRebuildDedupesSameTradingDay(t *testing.T) {
	sessionsDir := t.TempDir()
	writeSession(t, sessionsDir, sessionFixture{"session-20260101-daily", "session-20260101-daily", 1_000_000, utcDay(2026, 1, 1)})
	// Same trading day, recorded later, different close: the "last" rule must
	// keep this one and drop the earlier directory.
	writeSession(t, sessionsDir, sessionFixture{"session-20260101-stress", "session-20260101-stress", 900_000, utcDay(2026, 1, 1).Add(2 * time.Hour)})
	writeSession(t, sessionsDir, sessionFixture{"session-20260102-daily", "session-20260102-daily", 1_010_000, utcDay(2026, 1, 2)})

	newStateFile := func() string {
		path := filepath.Join(t.TempDir(), "simulation_state.json")
		writeState(t, path, engineShapedState(1_010_000))
		return path
	}

	stateFile := newStateFile()
	stdout, _ := runBackfill(t, sessionsDir, stateFile)
	if !strings.Contains(stdout, "de-duplicated trading day 2026-01-01: kept session-20260101-stress, dropped session-20260101-daily") {
		t.Errorf("stdout does not report the de-duplication:\n%s", stdout)
	}

	state := readState(t, stateFile)
	if len(state.DailyReturns) != 1 {
		t.Fatalf("daily_returns = %v, want exactly 1 entry (one per trading day)", state.DailyReturns)
	}
	// One day pair (01-01 -> 01-02) against the kept 900k close.
	if want := (1_010_000.0 - 900_000.0) / 900_000.0; !almostEqual(state.DailyReturns[0], want) {
		t.Errorf("daily_returns[0] = %v, want %v (measured against the kept 2026-01-01 close)", state.DailyReturns[0], want)
	}
	if state.LastSessionDate != "2026-01-02" {
		t.Errorf("last_session_date = %q, want 2026-01-02", state.LastSessionDate)
	}
	if !almostEqual(state.SessionBaseValue, 900_000) {
		t.Errorf("session_base_value = %v, want the kept 2026-01-01 close 900000", state.SessionBaseValue)
	}

	// The explicit override keeps the earliest directory of the day instead.
	firstStateFile := newStateFile()
	runBackfill(t, "-sessions-dir", sessionsDir, "-state-file", firstStateFile, "-on-duplicate", duplicateFirst)
	first := readState(t, firstStateFile)
	if want := (1_010_000.0 - 1_000_000.0) / 1_000_000.0; len(first.DailyReturns) != 1 || !almostEqual(first.DailyReturns[0], want) {
		t.Errorf("daily_returns = %v, want [%v] with -on-duplicate=first", first.DailyReturns, want)
	}
	if !almostEqual(first.SessionBaseValue, 1_000_000) {
		t.Errorf("session_base_value = %v, want the earliest 2026-01-01 close 1000000", first.SessionBaseValue)
	}
}

// TestRebuildRunTwiceKeepsSeriesUnchanged covers the idempotence the issue
// asks for: a second run on the same trading day must not lengthen the series.
func TestRebuildRunTwiceKeepsSeriesUnchanged(t *testing.T) {
	sessionsDir := t.TempDir()
	writeSession(t, sessionsDir, sessionFixture{"session-20260101-daily", "session-20260101-daily", 1_000_000, utcDay(2026, 1, 1)})
	writeSession(t, sessionsDir, sessionFixture{"session-20260102-daily", "session-20260102-daily", 1_010_000, utcDay(2026, 1, 2)})

	stateFile := filepath.Join(t.TempDir(), "simulation_state.json")
	writeState(t, stateFile, engineShapedState(1_010_000))

	runBackfill(t, sessionsDir, stateFile)
	first, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	runBackfill(t, sessionsDir, stateFile)
	second, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}

	if string(first) != string(second) {
		t.Errorf("second run changed the file:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	state := readState(t, stateFile)
	if len(state.DailyReturns) != 1 {
		t.Errorf("daily_returns = %v, want the series to keep its length", state.DailyReturns)
	}
}

// TestRebuildRejectsASingleTradingDay: after de-duplication a directory tree
// that only describes one day cannot produce a return, and the command must say
// so instead of emitting a same-day return (and must not touch the file).
func TestRebuildRejectsASingleTradingDay(t *testing.T) {
	sessionsDir := t.TempDir()
	writeSession(t, sessionsDir, sessionFixture{"session-20260101-daily", "session-20260101-daily", 1_000_000, utcDay(2026, 1, 1)})
	writeSession(t, sessionsDir, sessionFixture{"session-20260101-stress", "session-20260101-stress", 999_000, utcDay(2026, 1, 1).Add(time.Hour)})

	stateFile := filepath.Join(t.TempDir(), "simulation_state.json")
	writeState(t, stateFile, engineShapedState(1_000_000))
	before, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}

	var out, errOut bytes.Buffer
	err = run([]string{sessionsDir, stateFile}, &out, &errOut)
	if err == nil {
		t.Fatal("run() succeeded, want an error for a single trading day")
	}
	if !strings.Contains(err.Error(), "distinct trading days") {
		t.Errorf("error = %v, want it to explain that two distinct trading days are needed", err)
	}
	after, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if string(before) != string(after) {
		t.Error("state file was modified even though the rebuild failed")
	}
}

// TestRebuildDryRunDoesNotWrite keeps the operator escape hatch honest.
func TestRebuildDryRunDoesNotWrite(t *testing.T) {
	sessionsDir := t.TempDir()
	writeSession(t, sessionsDir, sessionFixture{"session-20260101-daily", "session-20260101-daily", 1_000_000, utcDay(2026, 1, 1)})
	writeSession(t, sessionsDir, sessionFixture{"session-20260102-daily", "session-20260102-daily", 1_010_000, utcDay(2026, 1, 2)})

	stateFile := filepath.Join(t.TempDir(), "simulation_state.json")
	writeState(t, stateFile, engineShapedState(1_010_000))
	before, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}

	stdout, _ := runBackfill(t, sessionsDir, stateFile, "-dry-run")
	if !strings.Contains(stdout, "dry-run") {
		t.Errorf("stdout = %q, want a dry-run notice", stdout)
	}
	after, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if string(before) != string(after) {
		t.Error("dry-run modified the state file")
	}
}

// TestEngineRerunAfterBackfillReplacesInsteadOfAppending is the end-to-end
// #1935 acceptance test. It rebuilds the series for a session the engine then
// re-runs, and shows the two behaviors side by side:
//
//   - pre-fix output (series rewritten without the date fields): the engine
//     cannot tell the session is already recorded and appends a second,
//     near-flat entry for the same trading day;
//   - fixed output (date fields written): the engine treats the session as a
//     re-run, keeps the series length and recomputes the entry against
//     session_base_value — exactly the semantics of RunDay.
func TestEngineRerunAfterBackfillReplacesInsteadOfAppending(t *testing.T) {
	sessionsDir := t.TempDir()
	writeSession(t, sessionsDir, sessionFixture{"session-20260922-daily", "session-20260922-daily", 1_000_000, utcDay(2026, 9, 22)})
	writeSession(t, sessionsDir, sessionFixture{"session-20260923-daily", "session-20260923-daily", 1_010_000, utcDay(2026, 9, 23)})

	sessionDay := utcDay(2026, 9, 23)
	quotes := []domain.Quote{{
		Symbol:     "2330.TW",
		Last:       121, // 900k cash + 1000 shares => 1.021M
		Volume:     1_000_000,
		IsTradable: true,
		AsOf:       sessionDay,
	}}
	engine := sim.NewEngine(domain.SimulationConstraints{
		StartingCash:                1_000_000,
		MaxPositionWeight:           1.0,
		MaxOpenPositions:            5,
		MinTradableVolume:           1,
		MinRecommendationConviction: 0,
		TransactionCostBPS:          0,
		SlippageBPS:                 0,
		ReserveCashFraction:         0,
	})

	// Pre-fix shape: the same series, but written without the date fields.
	legacyFile := filepath.Join(t.TempDir(), "simulation_state.json")
	legacy := engineShapedState(1_010_000)
	legacy.DailyReturns = []float64{0.01}
	writeState(t, legacyFile, legacy)
	legacyState := readState(t, legacyFile)
	legacyResult := engine.RunDay(&legacyState, sessionDay, domain.RegimeRiskOn, quotes, nil)
	if legacyResult.SessionRerun {
		t.Error("control: a file without last_session_date cannot report a re-run")
	}
	if len(legacyState.DailyReturns) != 2 {
		t.Fatalf("control: daily_returns = %v, want the same-day duplicate the issue reported (2 entries)",
			legacyState.DailyReturns)
	}
	duplicate := legacyState.DailyReturns[1]
	if want := (1_021_000.0 - 1_010_000.0) / 1_010_000.0; !almostEqual(duplicate, want) {
		t.Errorf("control: appended entry = %v, want the flat same-day delta %v", duplicate, want)
	}

	// Fixed shape: the command rebuilds the series and the date fields.
	fixedFile := filepath.Join(t.TempDir(), "simulation_state.json")
	writeState(t, fixedFile, engineShapedState(1_010_000))
	runBackfill(t, sessionsDir, fixedFile)
	fixedState := readState(t, fixedFile)

	result := engine.RunDay(&fixedState, sessionDay, domain.RegimeRiskOn, quotes, nil)
	if !result.SessionRerun {
		t.Error("the engine did not recognise the backfilled session as already recorded")
	}
	if len(fixedState.DailyReturns) != 1 {
		t.Fatalf("daily_returns = %v, want the series length preserved", fixedState.DailyReturns)
	}
	// 1.021M against the previous session's close (session_base_value = 1.0M),
	// not against the stale 1.010M left by the earlier run.
	if want := (1_021_000 - 1_000_000) / 1_000_000.0; !almostEqual(fixedState.DailyReturns[0], want) {
		t.Errorf("daily_returns[0] = %v, want %v (recomputed against session_base_value)", fixedState.DailyReturns[0], want)
	}
	if !result.SessionReturnRecorded || !almostEqual(result.SessionReturn, fixedState.DailyReturns[0]) {
		t.Errorf("result = %+v, want the recomputed session return", result)
	}
	t.Logf("pre-fix output: daily_returns=%v (session_rerun=%v)", legacyState.DailyReturns, legacyResult.SessionRerun)
	t.Logf("fixed output:   daily_returns=%v (session_rerun=%v, session_return=%.6f)",
		fixedState.DailyReturns, result.SessionRerun, result.SessionReturn)
}

// legacyRebuildSeries reproduces the pre-fix algorithm of this command: every
// session directory contributes, sorted by trading day only, with no
// de-duplication and no date fields written. It exists so the risk regression
// below can be measured against the reported production signature (#1935).
func legacyRebuildSeries(t *testing.T, sessionsDir string) []float64 {
	t.Helper()
	dirents, err := os.ReadDir(sessionsDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	type item struct {
		date string
		pv   float64
	}
	var items []item
	for _, e := range dirents {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "session-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sessionsDir, e.Name(), "summary.json"))
		if err != nil {
			continue
		}
		var s sessionSummary
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		date := tradingDay(s.SessionID, e.Name())
		if date == "" {
			continue
		}
		items = append(items, item{date, s.PortfolioValue})
	}
	// Pre-fix sort: by trading day only, no de-duplication.
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].date < items[j-1].date; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
	var returns []float64
	for i := 1; i < len(items); i++ {
		prev, curr := items[i-1].pv, items[i].pv
		if prev > 0 && curr > 0 {
			returns = append(returns, (curr-prev)/prev)
		}
	}
	return returns
}

// TestRiskSnapshotIsNotDominatedBySameDayDuplicates is the risk half of the
// acceptance: same-day duplicate returns at 0 fill the 5% tail
// ComputeRiskSnapshot reads, so the snapshot reports "no risk". The rebuilt
// series has one entry per trading day and the tail shows the real losses.
func TestRiskSnapshotIsNotDominatedBySameDayDuplicates(t *testing.T) {
	const days = 300 // > risk.MinObservationsForVaR, and the duplicate series is longer still

	sessionsDir := t.TempDir()
	value := 1_000_000.0
	equity := []float64{value}
	base := utcDay(2025, 1, 1)
	for i := 0; i < days; i++ {
		if i > 0 {
			// 29 losing days (i % 10 == 0) among 299 traded days: more than 5%
			// of the rebuilt series, so the real losses own its tail — but only
			// 29 of 599 entries once every day appears twice, i.e. less than 5%
			// of the duplicated series, which is the shape in which the zeros
			// take the tail over.
			if i%10 == 0 {
				value *= 0.98
			} else {
				value *= 1.002
			}
			equity = append(equity, value)
		}
		when := base.AddDate(0, 0, i)
		// Session ids and directory names use the compact trading day form,
		// session-YYYYMMDD-<mode> (domain.SessionDateFromID).
		id := "session-" + when.Format("20060102")
		writeSession(t, sessionsDir, sessionFixture{id + "-daily", id + "-daily", value, when})
		// A second directory for the same trading day: the same close, i.e. a
		// flat re-run — this is what produced the reported zero returns.
		writeSession(t, sessionsDir, sessionFixture{id + "-stress", id + "-stress", value, when.Add(time.Hour)})
	}

	legacyReturns := legacyRebuildSeries(t, sessionsDir)
	zeroes := 0
	for _, r := range legacyReturns {
		if r == 0 {
			zeroes++
		}
	}
	if zeroes == 0 {
		t.Fatal("control: the pre-fix rebuild produced no zero return, so it cannot reproduce the defect")
	}
	legacy := risk.ComputeRiskSnapshot(legacyReturns, equity)
	t.Logf("pre-fix series: %d entries (%d same-day zeros) var95=%.4f cvar95=%.4f",
		len(legacyReturns), zeroes, legacy.VaR95, legacy.CVaR95)
	if legacy.VaR95 != 0 {
		t.Errorf("control: var95 = %v over %d entries including %d same-day zeros, want the reported 0 signature",
			legacy.VaR95, len(legacyReturns), zeroes)
	}
	if legacy.CVaR95 == 0 {
		t.Fatalf("control: cvar95 = 0, want the diluted non-zero value the zeros produce")
	}

	stateFile := filepath.Join(t.TempDir(), "simulation_state.json")
	writeState(t, stateFile, engineShapedState(value))
	stdout, stderr := runBackfill(t, sessionsDir, stateFile)
	if stderr != "" {
		t.Errorf("stderr = %q, want no skipped session directory", stderr)
	}
	state := readState(t, stateFile)

	if len(state.DailyReturns) != days-1 {
		t.Fatalf("daily_returns has %d entries, want %d (one per trading day, no same-day duplicates)",
			len(state.DailyReturns), days-1)
	}
	if got := strings.Count(stdout, "de-duplicated trading day"); got != days {
		t.Errorf("stdout reported %d de-duplicated trading days, want %d", got, days)
	}
	fixed := risk.ComputeRiskSnapshot(state.DailyReturns, equity)
	t.Logf("rebuilt series: %d entries var95=%.4f cvar95=%.4f",
		len(state.DailyReturns), fixed.VaR95, fixed.CVaR95)
	if fixed.VaR95 == 0 {
		t.Errorf("var95 = 0 after the rebuild, want the real 5%% quantile (%v over %d daily returns)",
			fixed.VaR95, len(state.DailyReturns))
	}
	if almostEqual(fixed.VaR95, legacy.VaR95) {
		t.Errorf("var95 unchanged by the rebuild: %v", fixed.VaR95)
	}
	// The zeros did not only move var95 to 0, they diluted CVaR as well.
	if abs(fixed.CVaR95) <= abs(legacy.CVaR95) {
		t.Errorf("cvar95 = %v after the rebuild vs %v with the same-day zeros, want the rebuilt tail to show the larger loss",
			fixed.CVaR95, legacy.CVaR95)
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// TestParseOptionsForms pins both accepted argument forms.
func TestParseOptionsForms(t *testing.T) {
	positional, err := parseOptions([]string{"/tmp/sessions", "/tmp/state.json"})
	if err != nil {
		t.Fatalf("positional form: %v", err)
	}
	if positional.sessionsDir != "/tmp/sessions" || positional.stateFile != "/tmp/state.json" {
		t.Errorf("positional form parsed as %+v", positional)
	}
	if positional.onDuplicate != duplicateLast {
		t.Errorf("default on-duplicate = %q, want %q", positional.onDuplicate, duplicateLast)
	}

	flagged, err := parseOptions([]string{"-sessions-dir", "/tmp/s", "-state-file", "/tmp/st", "-dry-run", "-on-duplicate", duplicateFirst})
	if err != nil {
		t.Fatalf("flag form: %v", err)
	}
	if flagged.sessionsDir != "/tmp/s" || flagged.stateFile != "/tmp/st" || !flagged.dryRun || flagged.onDuplicate != duplicateFirst {
		t.Errorf("flag form parsed as %+v", flagged)
	}

	if _, err := parseOptions([]string{"-on-duplicate", "bogus", "/tmp/s", "/tmp/st"}); err == nil {
		t.Error("parseOptions accepted an unknown -on-duplicate value")
	}
	if _, err := parseOptions([]string{"/tmp/s"}); err == nil {
		t.Error("parseOptions accepted a missing state file")
	}
}
