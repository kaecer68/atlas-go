package monitoring

import (
	"context"
	"strings"
	"testing"
)

// This file pins the LOG evidence of the Step 1 empty-universe path
// (RankedFallbackEmptyUniverse), because the alert rule that covers that path
// tells the operator which log line to grep for: alert AtlasUniverseEmptyUniverse
// in monitoring/rules/atlas_universe_scoring_alerts.yml, "先查什麼" step 2.
//
// Why it needs a test (FU-20260926-13, closed 2026-10-16): that step used to read
// "(Step 1 的 `symbols_gathered count=0` 是本條的直接日誌證據)" — but
// logging.Info("universe_scheduler", "symbols_gathered", …) sits AFTER
// `if result.SymbolsBuilt == 0 { return }` in BuildUniverse, so the line can
// never carry count=0. An on-call reader following the step greps, finds the
// previous healthy run's `count=1599`, and is sent after the wrong thing. The
// evidence that DOES exist on this path is the WARN markRankedUntrustworthy
// emits, and that is what the rule now names.
//
// The negative half is the important half: it is the assertion that fails if
// someone later moves the `symbols_gathered` Info line above the early return
// (which would make the rule's warning stale) or renames the WARN event.
func TestBuildUniverseEmptyUniverseLogEvidence(t *testing.T) {
	t.Run("empty universe logs the reason WARN and not symbols_gathered", func(t *testing.T) {
		buf := captureLogs(t)
		deps := buildDepsFixture(t, tempDir(t))
		// gatherAllSymbols reads exactly these three; nil-ing them is the
		// "Step 1 就 0 檔" shape this alert exists for.
		deps.Tree = nil
		deps.Mapper = nil
		deps.Substrate = nil

		result, _, err := BuildUniverse(context.Background(), deps, false)
		if err != nil {
			t.Fatalf("BuildUniverse: %v", err)
		}
		if result.SymbolsBuilt != 0 || result.RankedFallbackReason != RankedFallbackEmptyUniverse {
			t.Fatalf("fixture did not produce the empty-universe shape: symbols_built=%d reason=%q",
				result.SymbolsBuilt, result.RankedFallbackReason)
		}

		logs := buf.String()
		if !strings.Contains(logs, "msg=ranked_not_trustworthy") ||
			!strings.Contains(logs, "reason="+RankedFallbackEmptyUniverse) {
			t.Errorf("the empty-universe path must log the reason it returned; got:\n%s", logs)
		}
		if strings.Contains(logs, "symbols_gathered") {
			t.Errorf("`symbols_gathered` is written after the SymbolsBuilt == 0 early return, "+
				"so it must not appear on this path (the alert rule's triage step 2 documents exactly "+
				"this); got:\n%s", logs)
		}
	})

	t.Run("a non-empty run does log symbols_gathered", func(t *testing.T) {
		buf := captureLogs(t)
		deps := buildDepsFixture(t, tempDir(t))

		result, _, err := BuildUniverse(context.Background(), deps, false)
		if err != nil {
			t.Fatalf("BuildUniverse: %v", err)
		}
		if result.SymbolsBuilt == 0 {
			t.Fatal("fixture produced no symbols; the positive half of this test needs a non-empty run")
		}

		logs := buf.String()
		if !strings.Contains(logs, "symbols_gathered") {
			t.Errorf("a non-empty run must log symbols_gathered (count=%d); got:\n%s",
				result.SymbolsBuilt, logs)
		}
		if strings.Contains(logs, "reason="+RankedFallbackEmptyUniverse) {
			t.Errorf("a non-empty run must not report %s; got:\n%s", RankedFallbackEmptyUniverse, logs)
		}
	})
}
