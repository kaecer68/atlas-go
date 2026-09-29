package orchestrator

// executor_skip_observability_test.go — issue #1944 T1.
//
// The three `continue`s that drop a candidate before it ever becomes a
// recommendation used to write NOTHING: no ScreeningReject row, no metric, no log.
// That is why, in the I36 audit, four agents with zero signals could not be
// attributed at all — the only evidence left behind was an absence. These tests
// pin the new accounting: what it counts, how it is bounded, and that it prints
// once per session instead of once per skip.
//
// Semantics under test (ruling of 2026-09-30):
//   - every skip EVENT increments a counter — no de-duplication by
//     (agent, symbol, reason): the question is "how many candidates did this
//     agent lose in this session";
//   - the output is ONE aggregate line per session, never one per skip;
//   - a symbol with NO factor scores at all is labelled executor_declined, not
//     factor_quality_gate, because the quality gate deliberately requires
//     preCount > 0 (it means "we had factor evidence and it was too weak").

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/logging"
)

// captureOrchestratorLogs swaps the global logger for the duration of the test
// at Debug level (the most permissive capture).
func captureOrchestratorLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	return captureOrchestratorLogsAt(t, slog.LevelDebug)
}

// captureOrchestratorLogsAt is captureOrchestratorLogs with an explicit level, so a
// test can assert what PRODUCTION sees (INFO) rather than what a debugging session
// sees — the distinction the level rule depends on.
func captureOrchestratorLogsAt(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := logging.Default()
	logging.SetLogger(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { logging.SetLogger(prev) })
	return buf
}

func TestSessionSkipCounts_CountsEveryEventWithoutDeduplication(t *testing.T) {
	s := newSessionSkipCounts()

	// The SAME (agent, reason) twice: the counter must advance twice. De-duplicating
	// here would hide how many candidates were lost, which is the whole measurement.
	s.record("ai-desk-01", skipReasonFactorQualityGate)
	s.record("ai-desk-01", skipReasonFactorQualityGate)
	s.record("ai-desk-01", skipReasonNoQuote)
	s.record("leo-satellite-desk-01", skipReasonExecutorDeclined)

	if got := s.byReason[skipReasonFactorQualityGate]; got != 2 {
		t.Errorf("factor_quality_gate = %d, want 2 (one per skip event)", got)
	}
	if got := s.byReason[skipReasonNoQuote]; got != 1 {
		t.Errorf("no_tradable_quote = %d, want 1", got)
	}
	if got := s.byReason[skipReasonExecutorDeclined]; got != 1 {
		t.Errorf("executor_declined = %d, want 1", got)
	}
	if got := s.total(); got != 4 {
		t.Errorf("total = %d, want 4 (sum over reasons)", got)
	}
	totals := s.agentTotals()
	if got := totals["ai-desk-01"]; got != 3 {
		t.Errorf("byAgentTotals[ai-desk-01] = %d, want 3", got)
	}
	if got := totals["leo-satellite-desk-01"]; got != 1 {
		t.Errorf("byAgentTotals[leo-satellite-desk-01] = %d, want 1", got)
	}
	if _, ok := totals["never-skipped-01"]; ok {
		t.Error("byAgentTotals must only contain agents that actually skipped something (bounded payload)")
	}
}

func TestSessionSkipCounts_TopSummaryIsDeterministicAndCapped(t *testing.T) {
	s := newSessionSkipCounts()
	// 8 distinct (agent, reason) pairs, with deliberate count ties to exercise the
	// tie-breakers (agent asc, then reason asc).
	for i := range 4 {
		agent := []string{"a-desk-01", "b-desk-01", "c-desk-01", "d-desk-01"}[i]
		s.record(agent, skipReasonExecutorDeclined)
		s.record(agent, skipReasonFactorQualityGate)
	}

	first := s.topAgentSkipSummary(5)
	second := s.topAgentSkipSummary(5)
	if first != second {
		t.Fatalf("summary is not deterministic: %q vs %q", first, second)
	}
	if n := len(strings.Split(first, ",")); n != 5 {
		t.Fatalf("summary = %q, want exactly 5 entries (cap)", first)
	}
	// Ties are broken by agent ID ascending, so the first entries are the a/b desks.
	if !strings.HasPrefix(first, "a-desk-01:") {
		t.Errorf("summary = %q, want it to start with a-desk-01 (count desc, agent asc)", first)
	}
	// No trailing comma / empty entry when capping.
	if strings.HasSuffix(first, ",") {
		t.Errorf("summary = %q has a trailing comma", first)
	}
}

func TestCollectRecommendations_CountsSkipsAndLogsOncePerSession(t *testing.T) {
	buf := captureOrchestratorLogs(t)

	// One agent whose skill NO executor claims (⇒ executor_declined), screening the
	// same two symbols: one with a tradable quote, one without a quote at all
	// (⇒ no_tradable_quote). Deterministic and independent of the shipped registry.
	registry := domain.AgentRegistry{Agents: []domain.AgentSpec{{
		ID:       "unwired-desk-01",
		Skill:    "unwired_desk_skill_that_no_executor_claims",
		Layer:    domain.LayerSector,
		Enabled:  true,
		Universe: []string{"2330.TW", "9999.TW", "8888.TW"},
	}}}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 100, High: 105, Low: 99, Last: 104, Volume: 5_000_000, IsTradable: true},
		// Present in the quote set but flagged non-tradable ⇒ not_tradable.
		"9999.TW": {Symbol: "9999.TW", Open: 0, High: 0, Low: 0, Last: 0, Volume: 0, IsTradable: false},
		// 8888.TW is deliberately ABSENT from the quote set ⇒ no_quote.
	}
	plugins := NewPluginRegistry()

	recs, _ := collectRecommendations(context.Background(), registry, quotes, plugins, nil,
		domain.RegimeNeutral, nil, "session-t1", nil)

	if len(recs) != 0 {
		t.Fatalf("fixture premise: the unwired skill must produce no recommendations, got %d", len(recs))
	}

	logs := buf.String()
	if n := strings.Count(logs, "recommendation_skips"); n != 1 {
		t.Fatalf("expected exactly ONE aggregate skip line per session, got %d\n--- log ---\n%s", n, logs)
	}
	for _, want := range []string{
		"session_id=session-t1",
		"no_quote=1",
		"not_tradable=1",
		"executor_declined=1",
		"factor_quality_gate=0",
		"skips_total=3",
		// slog quotes a value containing '='; the assertion stays exact about the
		// agent/reason pair so a future rename of either is caught.
		`top_agents="unwired-desk-01:executor_declined=1`,
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("skip line must contain %q\n--- log ---\n%s", want, logs)
		}
	}
}

func TestCollectRecommendations_NoSkipsMeansNoLine(t *testing.T) {
	buf := captureOrchestratorLogs(t)

	// A registry with no sector/style/superinvestor agent at all: nothing is iterated,
	// so nothing is skipped and no summary may be printed (a line per session with
	// zeros everywhere would be pure noise).
	registry := domain.AgentRegistry{Agents: []domain.AgentSpec{{
		ID: "control-01", Skill: "control_thing", Layer: domain.LayerControl, Enabled: true,
	}}}
	quotes := map[string]domain.Quote{"2330.TW": {Symbol: "2330.TW", Last: 100, IsTradable: true}}
	plugins := NewPluginRegistry()

	_, _ = collectRecommendations(context.Background(), registry, quotes, plugins, nil,
		domain.RegimeNeutral, nil, "session-t1-quiet", nil)

	if logs := buf.String(); strings.Contains(logs, "recommendation_skips") {
		t.Fatalf("a session with zero skips must not emit a skip summary\n--- log ---\n%s", logs)
	}
}

func TestCollectRecommendations_SkipCountsReachTheScratchpadTrace(t *testing.T) {
	registry := domain.AgentRegistry{Agents: []domain.AgentSpec{{
		ID:       "unwired-desk-01",
		Skill:    "unwired_desk_skill_that_no_executor_claims",
		Layer:    domain.LayerSector,
		Enabled:  true,
		Universe: []string{"2330.TW"},
	}}}
	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 100, High: 105, Low: 99, Last: 104, Volume: 5_000_000, IsTradable: true},
	}
	scratchpad := NewScratchpad("session-t1-trace", t.TempDir())

	_, _ = collectRecommendations(context.Background(), registry, quotes, NewPluginRegistry(), nil,
		domain.RegimeNeutral, nil, "session-t1-trace", scratchpad)

	var found bool
	for _, trace := range scratchpad.Traces() {
		if trace.Action != "collect_recommendations" {
			continue
		}
		found = true
		data, ok := trace.Data.(map[string]any)
		if !ok {
			t.Fatalf("trace.Data = %T, want map[string]any", trace.Data)
		}
		if got := data["skips_executor_declined"]; got != 1 {
			t.Errorf("trace skips_executor_declined = %v, want 1", got)
		}
		if got := data["skips_total"]; got != 1 {
			t.Errorf("trace skips_total = %v, want 1", got)
		}
		if got := data["skips_factor_quality_gate"]; got != 0 {
			t.Errorf("trace skips_factor_quality_gate = %v, want 0", got)
		}
		byAgent, ok := data["skips_by_agent"].(map[string]map[string]int)
		if !ok {
			t.Fatalf("trace skips_by_agent = %v (%T), want map[string]map[string]int (per agent AND reason)", data["skips_by_agent"], data["skips_by_agent"])
		}
		if byAgent["unwired-desk-01"][skipReasonExecutorDeclined] != 1 {
			t.Errorf("trace skips_by_agent[unwired-desk-01][executor_declined] = %d, want 1", byAgent["unwired-desk-01"][skipReasonExecutorDeclined])
		}
	}
	if !found {
		t.Fatal("no collect_recommendations trace was recorded")
	}
}

// TestLogRecommendationSkips_LevelFollowsSessionID pins the 2026-09-30 level rule.
//
// Production evidence: 4272 of 4288 recommendation_skips lines in 12h carried an
// EMPTY session_id — those are inner/batch calls that write no reasoning trace, and
// at INFO they are pure noise (they are not paging: no alert reads this event). The
// rule is therefore log hygiene, not suppression: no session ID ⇒ the same summary
// at DEBUG, so raising the level still reveals the path.
func TestLogRecommendationSkips_LevelFollowsSessionID(t *testing.T) {
	counts := newSessionSkipCounts()
	counts.record("unwired-desk-01", skipReasonNoQuote)
	counts.record("unwired-desk-01", skipReasonNotTradable)

	t.Run("no session id goes out at DEBUG only", func(t *testing.T) {
		// Capture at INFO: a DEBUG line must be filtered out, so the buffer stays
		// empty. This is the assertion that would have caught the production noise.
		infoBuf := captureOrchestratorLogsAt(t, slog.LevelInfo)
		logRecommendationSkips("", 21, 3, counts)
		if got := infoBuf.String(); strings.Contains(got, "recommendation_skips") {
			t.Fatalf("a call without a session id must not emit an INFO line\n--- log ---\n%s", got)
		}
		if got := infoBuf.String(); got != "" {
			t.Errorf("expected no INFO output at all, got %q", got)
		}

		debugBuf := captureOrchestratorLogsAt(t, slog.LevelDebug)
		logRecommendationSkips("", 21, 3, counts)
		if got := debugBuf.String(); !strings.Contains(got, "recommendation_skips") {
			t.Fatalf("the summary must still be observable at DEBUG\n--- log ---\n%s", got)
		}
	})

	t.Run("a session id emits exactly one INFO line", func(t *testing.T) {
		buf := captureOrchestratorLogsAt(t, slog.LevelInfo)
		logRecommendationSkips("session-20261001-daily", 21, 44, counts)
		logRecommendationSkips("session-20261001-daily", 21, 44, nil) // nil guard: no line
		logs := buf.String()
		if n := strings.Count(logs, "recommendation_skips"); n != 1 {
			t.Fatalf("expected exactly one INFO line per session, got %d\n--- log ---\n%s", n, logs)
		}
		for _, want := range []string{"no_quote=1", "not_tradable=1", "skips_total=2", "session_id=session-20261001-daily"} {
			if !strings.Contains(logs, want) {
				t.Errorf("INFO line must contain %q\n--- log ---\n%s", want, logs)
			}
		}
	})
}
