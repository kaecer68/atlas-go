package orchestrator

// executor_injected_quote_gate_test.go — FU-20260929-14 (b):
// 「只把該場真的有報價的注入符號加進掃描清單」。
//
// 為什麼（2026-09-30 實測）：ExpandUniverse 回傳 CSV 的 Code 欄**原樣**（裸 code，如 "0050"），
// 而報價表的鍵帶 .TW（"0050.TW"）⇒ 兩者永不相交 ⇒ 每個 agent 被注入 ~44 個**無法報價**的符號
// （每場 881 個 skip 事件），它們在 quote 檢查就被丟掉、**永遠不可能變成候選**。CSV 的 44 個
// 代碼全部是 ETF。
//
// 本檔釘住四件事：
//  1. ★ 零行為變更的**直接**證明：把送進 screener 的呼叫序列（symbol ＋ criteria 指紋）
//     在 gate on／off 兩側**逐位元比對** ⇒ 相同（候選集合不變）；
//  2. counter 的差異（`injected_no_quote` 881 → 0、`skips_total` 少掉注入基線）與
//     rejects／recommendations 逐項相同；
//  3. **不變量**：gate on 時 `injected_no_quote` 結構上必為 0（多組 quote set 變化都成立）；
//  4. 靜默變顯式：每場**恰好一行** `injected_symbols_unquoted`，且帶 `dropped` 與 `likely_cause`。

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/kaecer68/atlas-go/internal/domain"
	"github.com/kaecer68/atlas-go/internal/screener"
)

// gateMinVolume is a deliberately lenient criterion: it EXISTS (so the registry
// actually delegates to the screener instead of short-circuiting on "no filters")
// and it PASSES for the fixture quotes, so the collector walks past screening and
// on to the executor.
func gateMinVolume(v int64) *int64 { return &v }

func gateAgent(id string, minVolume int64) domain.AgentSpec {
	return domain.AgentSpec{
		ID:       id,
		Skill:    "gate_skill_no_executor_claims",
		Layer:    domain.LayerSector,
		Enabled:  true,
		Universe: []string{"2330.TW"},
		ScreeningCriteria: domain.ScreeningCriteria{
			VolumeIntraday: &domain.MinFilter{Min: gateMinVolume(minVolume)},
		},
	}
}

func gateRegistry() domain.AgentRegistry {
	return domain.AgentRegistry{Agents: []domain.AgentSpec{
		gateAgent("gate-a", 1_000),
		gateAgent("gate-b", 2_000),
	}}
}

// recordingScreener wraps the real screener and records every screening call.
//
// The Screener interface receives the agent's ScreeningCriteria rather than the
// agent, so the tuple recorded here is (criteria fingerprint, symbol) — with two
// agents carrying different criteria that identifies the agent just as well, and
// it is enough to compare the two runs element by element.
type recordingScreener struct {
	inner screener.Screener
	mu    sync.Mutex
	calls []string
}

// criteriaFingerprint renders an agent's criteria BY VALUE. The criteria struct
// holds pointers to filter bounds, so fmt would print addresses and two runs of the
// same fixture would look different for no reason; dereferencing (recursively, via
// reflection) keeps the fingerprint stable and comparable.
func criteriaFingerprint(c domain.ScreeningCriteria) string {
	return fmt.Sprintf("%v", derefValue(reflect.ValueOf(c)))
}

func derefValue(v reflect.Value) any {
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return derefValue(v.Elem())
	case reflect.Struct:
		parts := make([]string, 0, v.NumField())
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			parts = append(parts, fmt.Sprintf("%s=%v", t.Field(i).Name, derefValue(v.Field(i))))
		}
		return strings.Join(parts, ",")
	case reflect.Slice, reflect.Array:
		parts := make([]string, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			parts = append(parts, fmt.Sprintf("%v", derefValue(v.Index(i))))
		}
		return "[" + strings.Join(parts, ",") + "]"
	default:
		return v.Interface()
	}
}

func (r *recordingScreener) record(symbol string, criteria domain.ScreeningCriteria) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, criteriaFingerprint(criteria)+"|"+symbol)
}

func (r *recordingScreener) sequence() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

func (r *recordingScreener) Screen(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (bool, error) {
	r.record(symbol, criteria)
	return r.inner.Screen(ctx, symbol, criteria, quotes)
}

func (r *recordingScreener) ScreenDetailed(ctx context.Context, symbol string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) (screener.ScreenResult, error) {
	r.record(symbol, criteria)
	return r.inner.ScreenDetailed(ctx, symbol, criteria, quotes)
}

func (r *recordingScreener) ScreenUniverse(ctx context.Context, symbols []string, criteria domain.ScreeningCriteria, quotes map[string]domain.Quote) ([]string, error) {
	return r.inner.ScreenUniverse(ctx, symbols, criteria, quotes)
}

// useInjectedQuoteGate sets the gate for one test (and restores it).
func useInjectedQuoteGate(t *testing.T, on bool) {
	t.Helper()
	previous := injectOnlyQuotedSymbols
	injectOnlyQuotedSymbols = on
	t.Cleanup(func() { injectOnlyQuotedSymbols = previous })
}

// instrumentedRegistry builds a registry whose screener records its calls.
func instrumentedRegistry(t *testing.T) (*PluginRegistry, *recordingScreener) {
	t.Helper()
	plugins := NewPluginRegistry().WithScreener(screener.NewEngine(nil, nil))
	if plugins.screener == nil {
		t.Fatal("fixture premise: the registry has no screener to wrap")
	}
	rec := &recordingScreener{inner: plugins.screener}
	plugins.screener = rec
	return plugins, rec
}

// gateFixtureQuotes returns the quote set used by the gate tests: the agent's own
// symbol and one injected symbol are priceable, the other injected symbol is not.
func gateFixtureQuotes() map[string]domain.Quote {
	return map[string]domain.Quote{
		"2330.TW": {Symbol: "2330.TW", Open: 100, High: 105, Low: 99, Last: 104, Volume: 5_000_000, IsTradable: true},
		"1111.TW": {Symbol: "1111.TW", Open: 20, High: 21, Low: 19, Last: 20, Volume: 2_000_000, IsTradable: true},
	}
}

// TestInjectedSymbolsGate_ScreeningInputIsUnchanged is the zero-behavior proof:
// the symbols that reach the screener are identical with the gate on and off, so
// the injected symbols were already incapable of producing candidates.
func TestInjectedSymbolsGate_ScreeningInputIsUnchanged(t *testing.T) {
	const sessionID = "session-gate-equal"
	registry := gateRegistry

	// fixture: CSV injects 1111.TW (quoted) and 2222.TW (unquoted).
	useReplayCSVFixture(t, "1111.TW", "2222.TW")

	run := func(gateOn bool) ([]string, []domain.Recommendation, []domain.ScreeningReject, string) {
		useInjectedQuoteGate(t, gateOn)
		buf := captureOrchestratorLogs(t)
		plugins, rec := instrumentedRegistry(t)
		recs, rejects := collectRecommendations(context.Background(), registry(), gateFixtureQuotes(),
			plugins, nil, domain.RegimeNeutral, nil, sessionID, nil)
		return rec.sequence(), recs, rejects, buf.String()
	}

	onSeq, onRecs, onRejects, onLogs := run(true)
	offSeq, offRecs, offRejects, offLogs := run(false)

	// Anti-vacuity: two agents, each scanning its own symbol plus the one quoted
	// injected symbol ⇒ four screening calls. If this were 0 the equality below
	// would prove nothing.
	if len(onSeq) != 4 || len(offSeq) != 4 {
		t.Fatalf("screening calls = %d (gate on) / %d (gate off), want 4 each (2 agents × {own 2330.TW, injected 1111.TW})\n on=%v\noff=%v",
			len(onSeq), len(offSeq), onSeq, offSeq)
	}

	// ★ the screening input, element by element
	if len(onSeq) != len(offSeq) {
		t.Fatalf("screening calls: gate on = %d, gate off = %d — the gate must not change which symbols are screened\n on=%v\noff=%v",
			len(onSeq), len(offSeq), onSeq, offSeq)
	}
	for i := range onSeq {
		if onSeq[i] != offSeq[i] {
			t.Fatalf("screening call %d differs: gate on = %q, gate off = %q (order and content must be identical)", i, onSeq[i], offSeq[i])
		}
	}
	if !strings.Contains(strings.Join(onSeq, "\n"), "2330.TW") {
		t.Fatalf("fixture premise: the agent's own symbol must reach the screener, calls = %v", onSeq)
	}
	// The unquoted injected symbol must NOT be screened in either run: it is
	// dropped at the quote check, which is why excluding it cannot matter.
	for _, seq := range [][]string{onSeq, offSeq} {
		if strings.Contains(strings.Join(seq, "\n"), "2222.TW") {
			t.Fatalf("an unquoted injected symbol must never reach the screener, calls = %v", seq)
		}
	}

	// candidate sets, item by item
	if len(onRecs) != len(offRecs) || len(onRejects) != len(offRejects) {
		t.Fatalf("candidates differ: recs %d/%d, rejects %d/%d", len(onRecs), len(offRecs), len(onRejects), len(offRejects))
	}
	for i := range onRejects {
		if onRejects[i].Symbol != offRejects[i].Symbol || onRejects[i].AgentID != offRejects[i].AgentID {
			t.Errorf("reject %d differs: on = %s/%s, off = %s/%s", i,
				onRejects[i].AgentID, onRejects[i].Symbol, offRejects[i].AgentID, offRejects[i].Symbol)
		}
	}

	// counters: the ONLY thing that moves is the injected no_quote baseline
	for _, want := range []string{"injected_no_quote=0", "no_quote=0", "executor_declined=4", "skips_total=4"} {
		if !strings.Contains(onLogs, want) {
			t.Errorf("gate on: skip line must contain %q\n--- log ---\n%s", want, onLogs)
		}
	}
	for _, want := range []string{"injected_no_quote=2", "no_quote=2", "executor_declined=4", "skips_total=6"} {
		if !strings.Contains(offLogs, want) {
			t.Errorf("gate off (pre-fix baseline): skip line must contain %q\n--- log ---\n%s", want, offLogs)
		}
	}
}

// TestInjectedSymbolsGate_InjectedNoQuoteIsStructurallyZero（不變量）：
// gate on 時，注入的符號一定有報價 ⇒ `injected_no_quote` **必為 0**；三組 quote set 都成立。
func TestInjectedSymbolsGate_InjectedNoQuoteIsStructurallyZero(t *testing.T) {
	useInjectedQuoteGate(t, true)
	useReplayCSVFixture(t, "1111.TW", "2222.TW", "3333.TW")

	cases := map[string]map[string]domain.Quote{
		"no quotes at all": {},
		"only own quoted": {
			"2330.TW": {Symbol: "2330.TW", Open: 1, High: 1, Low: 1, Last: 1, Volume: 1, IsTradable: true},
		},
		"own + one injected quoted": gateFixtureQuotes(),
	}
	for name, quotes := range cases {
		t.Run(name, func(t *testing.T) {
			buf := captureOrchestratorLogs(t)
			_, _ = collectRecommendations(context.Background(), srcAttrRegistry([]string{"2330.TW"}), quotes,
				NewPluginRegistry(), nil, domain.RegimeNeutral, nil, "session-gate-invariant", nil)
			logs := buf.String()
			if strings.Contains(logs, "recommendation_skips") {
				if !strings.Contains(logs, "injected_no_quote=0") {
					t.Errorf("injected_no_quote must be structurally 0 with the gate on\n--- log ---\n%s", logs)
				}
				for _, reason := range []string{"not_tradable", "factor_quality_gate", "executor_declined"} {
					_ = reason // the injected_* fields for other reasons may be non-zero; only no_quote is invariant
				}
			}
		})
	}
}

// TestInjectedSymbolsGate_UnquotedInjectionIsReportedOncePerSession（靜默變顯式）：
// 恰好一行、帶 dropped 與 likely_cause，且**不隨 agent 數成長**。
func TestInjectedSymbolsGate_UnquotedInjectionIsReportedOncePerSession(t *testing.T) {
	useInjectedQuoteGate(t, true)
	useReplayCSVFixture(t, "1111.TW", "2222.TW", "3333.TW")

	buf := captureOrchestratorLogs(t)
	_, _ = collectRecommendations(context.Background(), gateRegistry(), gateFixtureQuotes(),
		NewPluginRegistry().WithScreener(screener.NewEngine(nil, nil)), nil, domain.RegimeNeutral, nil, "session-gate-warn", nil)

	logs := buf.String()
	if n := strings.Count(logs, "injected_symbols_unquoted"); n != 1 {
		t.Fatalf("injected_symbols_unquoted lines = %d, want exactly 1 per session (not per agent)\n--- log ---\n%s", n, logs)
	}
	// 三個注入符號中只有 1111.TW 有報價 ⇒ 兩個被丟；兩個 agent ⇒ dropped=4、agents=2
	for _, want := range []string{"dropped=4", "agents=2", "likely_cause=", ".TW", "normalizing the suffix would make them enter candidate sets"} {
		if !strings.Contains(logs, want) {
			t.Errorf("the report must contain %q\n--- log ---\n%s", want, logs)
		}
	}
}

// TestInjectedSymbolsGate_NothingDroppedIsSilent：沒有被丟的符號時不輸出噪音。
func TestInjectedSymbolsGate_NothingDroppedIsSilent(t *testing.T) {
	useInjectedQuoteGate(t, true)
	useReplayCSVFixture(t, "1111.TW") // 這個注入符號有報價 ⇒ 不被丟

	quotes := gateFixtureQuotes()
	buf := captureOrchestratorLogs(t)
	_, _ = collectRecommendations(context.Background(), srcAttrRegistry([]string{"2330.TW"}), quotes,
		NewPluginRegistry(), nil, domain.RegimeNeutral, nil, "session-gate-silent", nil)

	if logs := buf.String(); strings.Contains(logs, "injected_symbols_unquoted") {
		t.Errorf("no line may be emitted when nothing was dropped\n--- log ---\n%s", logs)
	}
}

// TestInjectedSymbolsGate_DefaultIsOn pins the SHIPPED default. Without this, the
// gate could be flipped to off in production and every other test here would still
// pass (they set the switch explicitly), so the regression would be invisible.
//
// The behavioral half matters more than the constant: it runs the collector the
// way production does, without touching the switch.
func TestInjectedSymbolsGate_DefaultIsOn(t *testing.T) {
	if !injectOnlyQuotedSymbols {
		t.Fatal("injectOnlyQuotedSymbols default is false: production would again scan unpriceable injected symbols (the pre-fix baseline of ~881 no_quote skips per session). If this is an intentional rollback, revert this test with the change")
	}

	useReplayCSVFixture(t, "1111.TW", "2222.TW") // 2222 is injected and unquoted
	buf := captureOrchestratorLogs(t)
	_, _ = collectRecommendations(context.Background(), gateRegistry(), gateFixtureQuotes(),
		NewPluginRegistry().WithScreener(screener.NewEngine(nil, nil)), nil, domain.RegimeNeutral, nil, "session-gate-default", nil)

	logs := buf.String()
	for _, want := range []string{"injected_no_quote=0", "injected_symbols_unquoted", "dropped=2"} {
		if !strings.Contains(logs, want) {
			t.Errorf("with the shipped default the collector must not count injected no_quote skips and must report the drop (%q)\n--- log ---\n%s", want, logs)
		}
	}
}
