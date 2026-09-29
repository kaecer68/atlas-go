package orchestrator

// executor_skip_source_test.go — FU-20260929-14 (a) 支：#1944 T2 的「注入 vs 自帶」來源拆帳。
//
// 為什麼需要（複核結論）：有自帶 universe 的 agent 會被注入**整份 replay CSV** 的符號
// （見 executor_collection.go 的 ExpandUniverse 呼叫），因此 `no_quote` 的絕大多數是
// **注入基線**而非該 agent 自己的股票 —— 生產實測每個被注入的 agent 都是恰好 44、而唯一的
// 未被注入者是 0。把兩個來源分開計數，才能回答「這個 agent 自己掉了幾個候選」。
//
// 本檔釘住四件事：
//  1. 假 CSV fixture 下「注入」與「自帶」可分辨（連各閘門都能分辨）；
//  2. 既有鍵的語意與數值不變（沒有注入時，四個 reason 的總和與注入欄位為 0）；
//  3. 走 DefaultSymbols()（空 universe）的 agent **不會**被注入 ⇒ 注入欄位為 0；
//  4. 歸屬規則＝own 優先：兩邊都有的符號算 own（與既有去重一致 ⇒ 良定義）。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
)

// useReplayCSVFixture 把 replayUniverseCSVPath 指向一個臨時 fixture（含 Code 欄），
// 並在測試結束還原成生產常數（護欄：預設值必須是常數本身、測試必須 t.Cleanup 還原）。
func useReplayCSVFixture(t *testing.T, symbols ...string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "replay_fixture.csv")
	body := "Date,Code,Name,TradeVolume,Open,High,Low,Close\n"
	for _, s := range symbols {
		body += "2026-09-29," + s + ",測試,1000,10,11,9,10.5\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write replay fixture: %v", err)
	}
	prev := replayUniverseCSVPath
	replayUniverseCSVPath = path
	t.Cleanup(func() { replayUniverseCSVPath = prev })
}

// useMissingReplayCSV 指向一個不存在的路徑 ⇒ 沒有注入（等同「CSV 缺失」的環境）。
func useMissingReplayCSV(t *testing.T) {
	t.Helper()
	prev := replayUniverseCSVPath
	replayUniverseCSVPath = filepath.Join(t.TempDir(), "does-not-exist.csv")
	t.Cleanup(func() { replayUniverseCSVPath = prev })
}

// srcAttrRegistry 是一個「skill 不被任何 executor 認領」的 agent（⇒ executor_declined），
// 自帶 universe 只有 2330.TW。
func srcAttrRegistry(universe []string) domain.AgentRegistry {
	return domain.AgentRegistry{Agents: []domain.AgentSpec{{
		ID:       "srcattr-desk-01",
		Skill:    "srcattr_skill_no_executor_claims",
		Layer:    domain.LayerSector,
		Enabled:  true,
		Universe: universe,
	}}}
}

// TestCollectRecommendations_SplitsOwnVersusInjectedSkips 是核心釘子（①＋②）：
//
//	fixture 注入 {1111.TW, 2222.TW, 3333.TW}；agent 自帶 {2330.TW}；
//	quote set 只有 {2330.TW, 2222.TW}（皆 tradable）⇒
//	  no_quote        = 1111, 3333            = 2（**兩個都是注入**）
//	  not_tradable    = 0
//	  executor_declined = 2330（own）, 2222（injected） = 2
//	  ⇒ skips_total = 4；且 injected_executor_declined = 1（只有 2222 是注入）
func TestCollectRecommendations_SplitsOwnVersusInjectedSkips(t *testing.T) {
	buf := captureOrchestratorLogs(t)
	useReplayCSVFixture(t, "1111.TW", "2222.TW", "3333.TW")

	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 100, High: 105, Low: 99, Last: 104, Volume: 5_000_000, IsTradable: true},
		"2222.TW": {Symbol: "2222.TW", Open: 50, High: 55, Low: 49, Last: 54, Volume: 3_000_000, IsTradable: true},
	}
	recs, _ := collectRecommendations(context.Background(), srcAttrRegistry([]string{"2330.TW"}), quotes,
		NewPluginRegistry(), nil, domain.RegimeNeutral, nil, "session-srcattr", nil)
	if len(recs) != 0 {
		t.Fatalf("fixture premise: the unwired skill must produce no recommendations, got %d", len(recs))
	}

	logs := buf.String()
	for _, want := range []string{
		"session_id=session-srcattr",
		"no_quote=2",
		"injected_no_quote=2",
		"not_tradable=0",
		"injected_not_tradable=0",
		"executor_declined=2",
		"injected_executor_declined=1",
		"factor_quality_gate=0",
		"injected_factor_quality_gate=0",
		"skips_total=4",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("skip line must contain %q\n--- log ---\n%s", want, logs)
		}
	}
}

// TestCollectRecommendations_NoInjectionLeavesExistingKeysUnchanged（②）：
// 沒有注入（CSV 缺失）時，**既有四個 reason 的語意與數值不變**，且注入欄位全為 0。
func TestCollectRecommendations_NoInjectionLeavesExistingKeysUnchanged(t *testing.T) {
	buf := captureOrchestratorLogs(t)
	useMissingReplayCSV(t)

	quotes := map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 100, High: 105, Low: 99, Last: 104, Volume: 5_000_000, IsTradable: true},
	}
	_, _ = collectRecommendations(context.Background(), srcAttrRegistry([]string{"2330.TW", "8888.TW"}), quotes,
		NewPluginRegistry(), nil, domain.RegimeNeutral, nil, "session-srcattr-none", nil)

	logs := buf.String()
	for _, want := range []string{
		"no_quote=1",          // 8888.TW（自帶、不在 quote set）
		"injected_no_quote=0", // 沒有注入
		"executor_declined=1", // 2330.TW
		"injected_executor_declined=0",
		"skips_total=2",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("skip line must contain %q\n--- log ---\n%s", want, logs)
		}
	}
}

// TestCollectRecommendations_DefaultSymbolsAgentIsNeverInjected（③）：
// 空 universe ⇒ 走 DefaultSymbols() ⇒ **即使 CSV fixture 存在**也不會被注入 ⇒ 注入欄位為 0。
// （mutation：把歸屬改成「一律 injected」⇒ 本測試必須紅。）
func TestCollectRecommendations_DefaultSymbolsAgentIsNeverInjected(t *testing.T) {
	buf := captureOrchestratorLogs(t)
	useReplayCSVFixture(t, "1111.TW", "2222.TW", "3333.TW") // fixture 存在，但這個 agent 不該被注入

	// 空 universe 的 agent：它的候選來自 DefaultSymbols()，其中沒有任何一個在 quote set ⇒ 全部 no_quote。
	_, _ = collectRecommendations(context.Background(), srcAttrRegistry(nil), map[string]domain.Quote{},
		NewPluginRegistry(), nil, domain.RegimeNeutral, nil, "session-srcattr-default", nil)

	logs := buf.String()
	if !strings.Contains(logs, "recommendation_skips") {
		t.Fatalf("fixture premise: an empty quote set must produce skips\n--- log ---\n%s", logs)
	}
	for _, reason := range []string{"no_quote", "not_tradable", "factor_quality_gate", "executor_declined"} {
		if !strings.Contains(logs, "injected_"+reason+"=0") {
			t.Errorf("a DefaultSymbols() agent must report injected_%s=0 (no injection on that branch)\n--- log ---\n%s", reason, logs)
		}
	}
}

// TestCollectRecommendations_InjectedKeysReachTheTrace 把新增的四個 trace 鍵釘住
// （只加不減：既有鍵仍必須是同一組數值）。
func TestCollectRecommendations_InjectedKeysReachTheTrace(t *testing.T) {
	useReplayCSVFixture(t, "1111.TW", "2222.TW", "3333.TW")
	quotes := map[string]domain.Quote{
		"2222.TW": {Symbol: "2222.TW", Open: 50, High: 55, Low: 49, Last: 54, Volume: 3_000_000, IsTradable: true},
	}
	scratchpad := NewScratchpad("session-srcattr-trace", t.TempDir())
	_, _ = collectRecommendations(context.Background(), srcAttrRegistry([]string{"2330.TW"}), quotes,
		NewPluginRegistry(), nil, domain.RegimeNeutral, nil, "session-srcattr-trace", scratchpad)

	for _, tr := range scratchpad.Traces() {
		if tr.Action != "collect_recommendations" {
			continue
		}
		data, ok := tr.Data.(map[string]any)
		if !ok {
			t.Fatalf("trace.Data = %T, want map[string]any", tr.Data)
		}
		// 由 fixture 手算（不是抄輸出）：quote set = {2222.TW}；own = {2330.TW}；
		// injected = {1111.TW, 2222.TW, 3333.TW} ⇒
		//   no_quote         = 2330(own) + 1111, 3333(injected) = 3
		//   executor_declined = 2222(injected，有報價 ⇒ 進到 executor 才被退) = 1
		//   injected_no_quote = 1111, 3333 = 2
		if got := data["skips_no_quote"]; got != 3 {
			t.Errorf("skips_no_quote = %v, want 3 (2330 own + 1111, 3333 injected)", got)
		}
		if got := data["skips_injected_no_quote"]; got != 2 {
			t.Errorf("skips_injected_no_quote = %v, want 2 (1111, 3333)", got)
		}
		if got := data["skips_injected_not_tradable"]; got != 0 {
			t.Errorf("skips_injected_not_tradable = %v, want 0", got)
		}
		if got := data["skips_injected_executor_declined"]; got != 1 {
			t.Errorf("skips_injected_executor_declined = %v, want 1 (2222 is injected and reached the executor)", got)
		}
		if got := data["skips_total"]; got != 4 {
			t.Errorf("skips_total = %v, want 4", got)
		}
		return
	}
	t.Fatal("no collect_recommendations trace was recorded")
}

// TestRecordSource_OwnWinsWhenSymbolIsInBothLists（④ 歸屬規則）：
// 兩邊都有的符號算 own ⇒ 不是注入。
func TestRecordSource_OwnWinsWhenSymbolIsInBothLists(t *testing.T) {
	s := newSessionSkipCounts()
	// 由 record() 進來的（既有呼叫端）一律記為 own
	s.record("a", skipReasonNoQuote)
	if got := s.injected(skipReasonNoQuote); got != 0 {
		t.Fatalf("record() must attribute to own, injected = %d", got)
	}
	s.recordSource("a", skipReasonNoQuote, skipSourceInjected)
	if got := s.injected(skipReasonNoQuote); got != 1 {
		t.Fatalf("injected = %d, want 1 after one explicit injected event", got)
	}
	if s.byReason[skipReasonNoQuote] != 2 || s.total() != 2 {
		t.Fatalf("totals must keep counting BOTH sources: byReason=%d total=%d, want 2/2",
			s.byReason[skipReasonNoQuote], s.total())
	}
}
