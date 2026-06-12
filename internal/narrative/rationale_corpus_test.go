package narrative

// Test suite for rationale_corpus.go — the static translation corpus plus
// the 6-step TranslateReason dispatcher.
//
// Strategy:
//   - Structural tests (reasonCorpus, rationaleTemplates,
//     rationaleSuffixMarkers) lock in the internal API surface so a careless
//     refactor can't silently drop entries or rename fields.
//   - Behavior tests for TranslateReason cover every step in the documented
//     lookup order: empty → CJK → exact → prefix → suffix → passthrough.
//   - Backward-compat tests assert that the entries inherited from the
//     pre-expansion corpus (3-step version, merged via PR #508) still
//     translate byte-for-byte. The LEO entry translation is pinned to
//     `部署週期` (R1 mitigation in the research report) — if a future
//     expansion flips it to `部署循環`, this test fails loudly.

import (
	"strings"
	"testing"
)

// ─── Structural tests ──────────────────────────────────────────────────

func TestRationaleCorpus_NotEmpty(t *testing.T) {
	if len(reasonCorpus) == 0 {
		t.Fatal("reasonCorpus must not be empty — the static map is the source of truth")
	}
}

func TestRationaleCorpus_NoDuplicateKeys(t *testing.T) {
	// Defensive: Go map literals silently overwrite on duplicate keys. A
	// refactor that adds an entry twice would be invisible at runtime.
	// Detect it by counting unique lowercased+trimmed values and comparing
	// against the map size.
	seen := make(map[string]int, len(reasonCorpus))
	for k := range reasonCorpus {
		k = strings.ToLower(strings.TrimSpace(k))
		seen[k]++
	}
	for k, n := range seen {
		if n > 1 {
			t.Errorf("duplicate key (after normalization) in reasonCorpus: %q appears %d times", k, n)
		}
	}
}

func TestRationaleCorpus_AllValuesNonEmpty(t *testing.T) {
	for k, v := range reasonCorpus {
		if strings.TrimSpace(v) == "" {
			t.Errorf("reasonCorpus[%q] has empty translation", k)
		}
	}
}

func TestRationaleCorpus_HasMinimumEntryCount(t *testing.T) {
	// The expansion wave adds 14 new non-CJK entries (9 ETF + 5
	// superinvestor) on top of the pre-existing 18 non-CJK entries.
	// 6 unreachable CJK identity entries were removed. Total non-CJK
	// entries should be at least 32. If someone accidentally drops a
	// section, this trips.
	const minExpected = 32
	if got := len(reasonCorpus); got < minExpected {
		t.Errorf("reasonCorpus has %d entries, expected at least %d (14 new + 18 inherited, excluding 6 removed CJK identity)", got, minExpected)
	}
}

func TestRationaleCorpus_NewETFEntries_Present(t *testing.T) {
	// The 9 new ETF-rotation entries (regime-aware). Each must be present
	// AND translate to a non-empty Chinese string.
	etfKeys := []string{
		"safe-haven gold etf in risk-off regime",
		"defensive dividend etf in risk-off regime",
		"equity etf penalized in risk-off regime",
		"broad market etf in risk-on regime",
		"dividend etf in risk-on regime",
		"gold etf penalized in risk-on regime",
		"diversified etf in risk-on regime",
		"balanced etf allocation with positive momentum in neutral regime",
		"balanced etf allocation in neutral regime",
	}
	for _, k := range etfKeys {
		k = strings.ToLower(strings.TrimSpace(k))
		v, ok := reasonCorpus[k]
		if !ok {
			t.Errorf("missing ETF rotation entry: %q", k)
			continue
		}
		if !containsCJK(v) {
			t.Errorf("ETF rotation entry %q translated to non-CJK value: %q", k, v)
		}
	}
}

func TestRationaleCorpus_NewSuperinvestorEntries_Present(t *testing.T) {
	// The 5 new superinvestor-theme entries.
	siKeys := []string{
		"superinvestor thematic conviction",
		"macro momentum asymmetric thesis",
		"ai compute cycle durable demand thesis",
		"deep tech ip moat differentiation thesis",
		"quality compounder catalyst thesis",
	}
	for _, k := range siKeys {
		k = strings.ToLower(strings.TrimSpace(k))
		v, ok := reasonCorpus[k]
		if !ok {
			t.Errorf("missing superinvestor entry: %q", k)
			continue
		}
		if !containsCJK(v) {
			t.Errorf("superinvestor entry %q translated to non-CJK value: %q", k, v)
		}
	}
}

func TestRationaleCorpus_BackwardCompat_KeySet(t *testing.T) {
	// Pre-expansion corpus (PR #508, 20 entries). Pin the exact key set so
	// the additive expansion cannot silently drop one of the originals.
	required := []string{
		// Sector agents
		"semiconductor leadership and supply-chain role",
		"ai infrastructure order-flow sensitivity",
		"leo satellite infrastructure and deployment cycle",
		"financial carry with resilient balance-sheet posture",
		"shipping beta used as tactical cycle exposure",
		"robotics automation capex cycle",
		"mining and precious metals cycle",
		"energy commodity cycle",
		"electronics component demand cycle",
		"consumer staples and retail cycle",
		"industrial manufacturing and infrastructure cycle",
		// Position evaluation
		"momentum decay: significant weakness (last << open)",
		"signal weakening: reduce exposure",
		"position evaluation: maintain holding",
		// Style agents
		"price persistence with style overlay",
		"defensive yield lens with valuation discipline",
		"earnings quality and forward visibility support",
		"breakout structure confirmed by volume and close",
	}
	for _, k := range required {
		k = strings.ToLower(strings.TrimSpace(k))
		if _, ok := reasonCorpus[k]; !ok {
			t.Errorf("backward-compat entry dropped from reasonCorpus: %q", k)
		}
	}
}

func TestRationaleCorpus_BackwardCompat_LEOTranslationPreserved(t *testing.T) {
	// R1 mitigation: the LEO entry was changed by the WIP to `部署循環`,
	// but we reverted it back to the established `部署週期`. Pin it here
	// so any future re-introduction of `部署循環` is caught.
	const leoKey = "leo satellite infrastructure and deployment cycle"
	const wantZh = "低軌衛星基礎建設與部署週期"
	got, ok := reasonCorpus[leoKey]
	if !ok {
		t.Fatalf("LEO entry missing from reasonCorpus")
	}
	if got != wantZh {
		t.Errorf("LEO entry translation drifted: got %q, want %q (R1: original was 部署週期)", got, wantZh)
	}
}

// ─── rationaleTemplates structural tests ──────────────────────────────

func TestRationaleTemplates_WellFormed(t *testing.T) {
	if len(rationaleTemplates) != 3 {
		t.Errorf("rationaleTemplates has %d entries, expected 3 ([crowded:, [weighted:, [superinvestor:)", len(rationaleTemplates))
	}
	seen := make(map[string]bool, len(rationaleTemplates))
	for i, tpl := range rationaleTemplates {
		if tpl.Prefix == "" {
			t.Errorf("rationaleTemplates[%d].Prefix is empty", i)
		}
		if !strings.HasPrefix(tpl.Prefix, "[") {
			t.Errorf("rationaleTemplates[%d].Prefix %q must start with [", i, tpl.Prefix)
		}
		if !strings.HasSuffix(tpl.Prefix, ":") {
			t.Errorf("rationaleTemplates[%d].Prefix %q must end with :", i, tpl.Prefix)
		}
		lower := strings.ToLower(tpl.Prefix)
		if seen[lower] {
			t.Errorf("duplicate prefix in rationaleTemplates: %q", tpl.Prefix)
		}
		seen[lower] = true
	}
}

func TestRationaleTemplates_RequiredPrefixes(t *testing.T) {
	// The 3 required prefixes — pin them so a typo in a future edit
	// (e.g. `[crowded ` without the colon) is caught.
	required := []string{"[crowded:", "[weighted:", "[superinvestor:"}
	have := make(map[string]bool, len(rationaleTemplates))
	for _, tpl := range rationaleTemplates {
		have[strings.ToLower(tpl.Prefix)] = true
	}
	for _, p := range required {
		if !have[p] {
			t.Errorf("rationaleTemplates missing required prefix %q", p)
		}
	}
}

// ─── rationaleSuffixMarkers structural tests ──────────────────────────

func TestRationaleSuffixMarkers_WellFormed(t *testing.T) {
	if len(rationaleSuffixMarkers) != 2 {
		t.Errorf("rationaleSuffixMarkers has %d entries, expected 2", len(rationaleSuffixMarkers))
	}
	for i, m := range rationaleSuffixMarkers {
		if m == "" {
			t.Errorf("rationaleSuffixMarkers[%d] is empty", i)
		}
		if !strings.HasPrefix(m, " ") {
			t.Errorf("rationaleSuffixMarkers[%d] %q should start with a space (to be safely droppable from the trimmed head)", i, m)
		}
	}
}

func TestRationaleSuffixMarkers_RequiredMarkers(t *testing.T) {
	required := []string{" | narrative:", " [prism:"}
	have := make(map[string]bool, len(rationaleSuffixMarkers))
	for _, m := range rationaleSuffixMarkers {
		have[m] = true
	}
	for _, m := range required {
		if !have[m] {
			t.Errorf("rationaleSuffixMarkers missing required marker %q", m)
		}
	}
}

// ─── TranslateReason step 1: empty / whitespace ───────────────────────

func TestTranslateReason_Empty(t *testing.T) {
	if got := TranslateReason(""); got != "" {
		t.Errorf("TranslateReason(\"\") = %q, want \"\"", got)
	}
}

func TestTranslateReason_WhitespaceOnly(t *testing.T) {
	cases := []string{" ", "\t", "\n", "  \t \n "}
	for _, in := range cases {
		if got := TranslateReason(in); got != "" {
			t.Errorf("TranslateReason(%q) = %q, want \"\"", in, got)
		}
	}
}

// ─── TranslateReason step 2: CJK passthrough ──────────────────────────

func TestTranslateReason_CJKPassthrough(t *testing.T) {
	// CJK short-circuit fires regardless of corpus contents. Any
	// string with at least one CJK char returns trimmed input unchanged.
	cases := []string{
		"低軌衛星基礎建設與部署週期",
		"半導體領導地位",
		"superinvestor 風格", // mixed CJK + ASCII should also passthrough
	}
	for _, in := range cases {
		if got := TranslateReason(in); got != in {
			t.Errorf("TranslateReason(%q) = %q, want passthrough", in, got)
		}
	}
}

// ─── TranslateReason step 3: exact match (backward compat) ────────────

func TestTranslateReason_ExactMatch_BackwardCompat(t *testing.T) {
	// Pre-expansion entries must still translate byte-for-byte.
	cases := []struct {
		in, want string
	}{
		{"semiconductor leadership and supply-chain role", "半導體領導地位與供應鏈角色"},
		{"ai infrastructure order-flow sensitivity", "AI 基礎建設訂單流敏感度"},
		{"leo satellite infrastructure and deployment cycle", "低軌衛星基礎建設與部署週期"},
		{"momentum decay: significant weakness (last << open)", "動能衰減：顯著弱勢（收盤遠低於開盤）"},
		{"breakout structure confirmed by volume and close", "突破結構經量價確認"},
	}
	for _, c := range cases {
		got := TranslateReason(c.in)
		if got != c.want {
			t.Errorf("TranslateReason(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTranslateReason_ExactMatch_NewEntries(t *testing.T) {
	// The 14 new entries — both key and expected translation pinned.
	cases := []struct {
		in, want string
	}{
		// ETF rotation (9)
		{"safe-haven gold etf in risk-off regime", "避險型黃金 ETF（風險趨避盤勢）"},
		{"defensive dividend etf in risk-off regime", "防禦型高股息 ETF（風險趨避盤勢）"},
		{"equity etf penalized in risk-off regime", "股票型 ETF（風險趨避盤勢遭壓抑）"},
		{"broad market etf in risk-on regime", "大盤型 ETF（風險偏好盤勢）"},
		{"dividend etf in risk-on regime", "高股息 ETF（風險偏好盤勢）"},
		{"gold etf penalized in risk-on regime", "黃金 ETF（風險偏好盤勢遭壓抑）"},
		{"diversified etf in risk-on regime", "多元化配置 ETF（風險偏好盤勢）"},
		{"balanced etf allocation with positive momentum in neutral regime", "中性盤勢下平衡型 ETF 配置且動能正向"},
		{"balanced etf allocation in neutral regime", "中性盤勢下平衡型 ETF 配置"},
		// Superinvestor themes (5)
		{"superinvestor thematic conviction", "超級投資者主題式信念"},
		{"macro momentum asymmetric thesis", "總經動能不對稱報酬論點"},
		{"ai compute cycle durable demand thesis", "AI 算力循環耐久需求論點"},
		{"deep tech ip moat differentiation thesis", "深度科技 IP 護城河差異化論點"},
		{"quality compounder catalyst thesis", "品質複利型企業觸發論點"},
	}
	for _, c := range cases {
		got := TranslateReason(c.in)
		if got != c.want {
			t.Errorf("TranslateReason(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTranslateReason_ExactMatch_CaseInsensitive(t *testing.T) {
	// Corpus keys are stored lowercased; TranslateReason lowercases the
	// lookup input. Mixed-case input must hit the same translation.
	cases := []struct {
		in, want string
	}{
		{"Semiconductor Leadership and Supply-Chain Role", "半導體領導地位與供應鏈角色"},
		{"LEO SATELLITE INFRASTRUCTURE AND DEPLOYMENT CYCLE", "低軌衛星基礎建設與部署週期"},
		{"  Broad Market ETF in Risk-On Regime  ", "大盤型 ETF（風險偏好盤勢）"}, // also trims
	}
	for _, c := range cases {
		got := TranslateReason(c.in)
		if got != c.want {
			t.Errorf("TranslateReason(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ─── TranslateReason step 4: prefix template ──────────────────────────

func TestTranslateReason_PrefixTemplate_Crowded(t *testing.T) {
	// "[Crowded:2 agents] price persistence with style overlay"
	//   → "[Crowded:2 agents] 價格持續性與風格疊加"
	got := TranslateReason("[Crowded:2 agents] price persistence with style overlay")
	want := "[Crowded:2 agents] 價格持續性與風格疊加"
	if got != want {
		t.Errorf("TranslateReason prefix crowded: got %q, want %q", got, want)
	}
}

func TestTranslateReason_PrefixTemplate_Weighted(t *testing.T) {
	got := TranslateReason("[Weighted:3 agents] earnings quality and forward visibility support")
	want := "[Weighted:3 agents] 盈餘品質與前瞻能見度支撐"
	if got != want {
		t.Errorf("TranslateReason prefix weighted: got %q, want %q", got, want)
	}
}

func TestTranslateReason_PrefixTemplate_Superinvestor(t *testing.T) {
	got := TranslateReason("[Superinvestor:ackman_quality] quality compounder catalyst thesis")
	want := "[Superinvestor:ackman_quality] 品質複利型企業觸發論點"
	if got != want {
		t.Errorf("TranslateReason prefix superinvestor: got %q, want %q", got, want)
	}
}

func TestTranslateReason_PrefixTemplate_NoBase(t *testing.T) {
	// "[Weighted:3 agents]" with nothing after — return head unchanged.
	got := TranslateReason("[Weighted:3 agents]")
	want := "[Weighted:3 agents]"
	if got != want {
		t.Errorf("TranslateReason prefix no base: got %q, want %q", got, want)
	}
}

func TestTranslateReason_PrefixTemplate_MalformedNoBracket(t *testing.T) {
	// "[crowded:2 agents price persistence with style overlay" (no `]`)
	// → step 6 passthrough (returns trimmed).
	in := "[crowded:2 agents price persistence with style overlay"
	got := TranslateReason(in)
	if got != in {
		t.Errorf("TranslateReason malformed prefix: got %q, want %q", got, in)
	}
}

func TestTranslateReason_PrefixTemplate_UnknownBase(t *testing.T) {
	// Prefix matches, but the trailing base is NOT in the corpus.
	// Should preserve the head and translate... nothing → just head + " " + passthrough base.
	in := "[Crowded:5 agents] some unmapped english rationale"
	got := TranslateReason(in)
	want := "[Crowded:5 agents] some unmapped english rationale"
	if got != want {
		t.Errorf("TranslateReason prefix unknown base: got %q, want %q", got, want)
	}
}

func TestTranslateReason_PrefixTemplate_CaseInsensitivePrefix(t *testing.T) {
	// Prefix matching is case-insensitive (lowercased lookup), but the
	// ORIGINAL-case english head is preserved in the output.
	got := TranslateReason("[CROWDED:2 agents] breakout structure confirmed by volume and close")
	want := "[CROWDED:2 agents] 突破結構經量價確認"
	if got != want {
		t.Errorf("TranslateReason prefix case-insensitive: got %q, want %q", got, want)
	}
}

// ─── TranslateReason step 5: suffix markers ───────────────────────────

func TestTranslateReason_Suffix_Narrative(t *testing.T) {
	// "semiconductor leadership and supply-chain role | Narrative: 2 events"
	//   → "半導體領導地位與供應鏈角色 | narrative: 2 events"
	in := "semiconductor leadership and supply-chain role | narrative: 2 events"
	got := TranslateReason(in)
	want := "半導體領導地位與供應鏈角色 | narrative: 2 events"
	if got != want {
		t.Errorf("TranslateReason suffix narrative: got %q, want %q", got, want)
	}
}

func TestTranslateReason_Suffix_Prism(t *testing.T) {
	// "ai infrastructure order-flow sensitivity [PRISM:0.85]"
	//   → "AI 基礎建設訂單流敏感度 [prism:0.85]"
	// Note: the suffix is preserved in its original case as found in the
	// input (lowercased `narrative:` / `prism:` markers will only match
	// if the input also has them lowercased; mixed-case input is fine
	// because step 5 uses a case-insensitive find).
	in := "ai infrastructure order-flow sensitivity [PRISM:0.85]"
	got := TranslateReason(in)
	want := "AI 基礎建設訂單流敏感度 [PRISM:0.85]"
	if got != want {
		t.Errorf("TranslateReason suffix prism: got %q, want %q", got, want)
	}
}

func TestTranslateReason_Suffix_UnknownBase(t *testing.T) {
	// Suffix marker present, base not in corpus → translate nothing for
	// the base (passthrough), preserve suffix.
	in := "some unknown english | narrative: 1 event"
	got := TranslateReason(in)
	want := "some unknown english | narrative: 1 event"
	if got != want {
		t.Errorf("TranslateReason suffix unknown base: got %q, want %q", got, want)
	}
}

// ─── TranslateReason combo: prefix + suffix ───────────────────────────

func TestTranslateReason_Combo_PrefixAndSuffix(t *testing.T) {
	// Prefix wraps a base that is in the corpus; suffix appended.
	// Step 4 (prefix) fires first; the recursive TranslateReason on the
	// trimmed base returns the Chinese translation (no suffix in base),
	// then the outer join is `head + " " + translated`. The suffix
	// marker is NOT split off in this path because step 5 only runs if
	// step 4 didn't match. The expected output is therefore:
	//   "[Crowded:2 agents] 半導體領導地位與供應鏈角色 | narrative: 1 event"
	in := "[Crowded:2 agents] semiconductor leadership and supply-chain role | narrative: 1 event"
	got := TranslateReason(in)
	want := "[Crowded:2 agents] 半導體領導地位與供應鏈角色 | narrative: 1 event"
	if got != want {
		t.Errorf("TranslateReason combo prefix+suffix: got %q, want %q", got, want)
	}
}

// ─── TranslateReason step 6: passthrough ──────────────────────────────

func TestTranslateReason_Passthrough_UnknownEnglish(t *testing.T) {
	in := "some unmapped english rationale"
	got := TranslateReason(in)
	if got != in {
		t.Errorf("TranslateReason passthrough: got %q, want %q", got, in)
	}
}

func TestTranslateReason_Passthrough_TrimsWhitespace(t *testing.T) {
	// Whitespace is trimmed before returning, even for unmapped English.
	in := "  some unmapped english rationale  "
	want := "some unmapped english rationale"
	got := TranslateReason(in)
	if got != want {
		t.Errorf("TranslateReason passthrough trim: got %q, want %q", got, want)
	}
}

// ─── containsCJK tests ────────────────────────────────────────────────

func TestContainsCJK(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"hello", false},
		{"hello world 123", false},
		{"半導體", true},
		{"低軌衛星", true},
		{"未過濾", true},
		// Compatibility Ideographs
		{"豈", true},
		// Mixed
		{"superinvestor 風格", true},
		{"半導體 leadership", true},
	}
	for _, c := range cases {
		if got := containsCJK(c.in); got != c.want {
			t.Errorf("containsCJK(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
