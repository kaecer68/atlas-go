// Package-level rationale translation corpus for the investment pipeline.
//
// Purpose:
//
//	The investment pipeline UI (web/static/js/pages/pipeline.js, line 275)
//	renders the `reason` and `guard_reason` fields verbatim. Agent files
//	produce English rationales (see internal/orchestrator/plugin_sector.go,
//	plugin_style.go, plugin_control.go). Translating at the agent layer
//	would scatter translations across the 17 sector/style/superinvestor
//	files; we instead translate at the handler serialization layer
//	(internal/monitoring/api/pipeline/handlers.go) so the agents remain
//	English-native and any other consumer (audit logs, narrative events,
//	the recommendation_outcomes.jsonl file) keeps working unchanged.
//
// Invariant:
//
//	This file MUST NOT depend on an LLM, a translation API, or any external
//	network resource. The corpus is a pure Go map. Future waves may add a
//	fallback LLM translator for unmatched strings, but the corpus is
//	authoritative for everything it covers — agents that want to ship a
//	new reason string should add an entry here, not bypass it.
//
// Coverage:
//   - 32 static English literal rationales emitted by 17 sector/style/
//     superinvestor executors in internal/orchestrator.
//   - 3 prefix templates for the control-layer "crowded", "weighted", and
//     "superinvestor" badges. The English structural prefix is preserved
//     in the translated output because the frontend pipeline.js:247
//     string-matches on `[crowded:` to render the "擁擠" badge; the
//     hard rule of this wave is "no frontend change".
//   - 2 suffix markers for the runtime-appended ` | Narrative: N event(s)`
//     and ` [PRISM:X.XX]` suffixes. The leading base is translated, the
//     dynamic suffix is preserved (audit-relevant).
//   - CJK pass-through: any input already containing Chinese characters
//     is returned unchanged, so the already-Chinese control-layer and
//     guard-layer strings don't get double-translated.
package narrative

import "strings"

// RationaleCorpus maps a normalized (lowercased, whitespace-trimmed) English
// rationale string to its Chinese (Traditional, Taiwan-style) translation.
// Keys are stored lowercased; TranslateReason lowercases the lookup input.
//
// To add a new entry: append `english lowercase literal: "中文翻譯"` here.
// The TranslateReason function will pick it up on the next request — no
// registration, no rebuild, no agent-file change required.
var RationaleCorpus = map[string]string{
	// ─── Sector agents: semiconductor / AI supply chain / LEO ───
	"semiconductor leadership and supply-chain role":    "半導體領導地位與供應鏈角色",
	"ai infrastructure order-flow sensitivity":          "AI 基礎建設訂單流敏感度",
	"leo satellite infrastructure and deployment cycle": "低軌衛星基礎建設與部署週期",

	// ─── Sector agent: ETF rotation (regime-aware, 9 strings) ───
	"safe-haven gold etf in risk-off regime":                           "避險型黃金 ETF（風險趨避盤勢）",
	"defensive dividend etf in risk-off regime":                        "防禦型高股息 ETF（風險趨避盤勢）",
	"equity etf penalized in risk-off regime":                          "股票型 ETF（風險趨避盤勢遭壓抑）",
	"broad market etf in risk-on regime":                               "大盤型 ETF（風險偏好盤勢）",
	"dividend etf in risk-on regime":                                   "高股息 ETF（風險偏好盤勢）",
	"gold etf penalized in risk-on regime":                             "黃金 ETF（風險偏好盤勢遭壓抑）",
	"diversified etf in risk-on regime":                                "多元化配置 ETF（風險偏好盤勢）",
	"balanced etf allocation with positive momentum in neutral regime": "中性盤勢下平衡型 ETF 配置且動能正向",
	"balanced etf allocation in neutral regime":                        "中性盤勢下平衡型 ETF 配置",

	// ─── Sector agents: financials / shipping / industry desks ───
	"financial carry with resilient balance-sheet posture": "金融利差與穩健資產負債表",
	"shipping beta used as tactical cycle exposure":        "航運 Beta 作為戰術週期曝險",
	"robotics automation capex cycle":                      "機器人自動化資本支出週期",
	"mining and precious metals cycle":                     "礦業與貴金屬週期",
	"energy commodity cycle":                               "能源商品週期",
	"electronics component demand cycle":                   "電子零組件需求週期",
	"consumer staples and retail cycle":                    "民生消費與零售週期",
	"industrial manufacturing and infrastructure cycle":    "工業製造與基礎建設週期",

	// ─── Shared position-evaluation rationales (across all executors) ───
	"momentum decay: significant weakness (last << open)": "動能衰減：顯著弱勢（收盤遠低於開盤）",
	"signal weakening: reduce exposure":                   "信號減弱：降低曝險",
	"position evaluation: maintain holding":               "持倉評估：維持持有",

	// ─── Style agents ───
	"price persistence with style overlay":             "價格持續性與風格疊加",
	"defensive yield lens with valuation discipline":   "防禦性殖利率視角與估值紀律",
	"earnings quality and forward visibility support":  "盈餘品質與前瞻能見度支撐",
	"breakout structure confirmed by volume and close": "突破結構經量價確認",

	// ─── Superinvestor agents (Portfolio Manager role) ───
	"superinvestor thematic conviction":        "超級投資者主題式信念",
	"macro momentum asymmetric thesis":         "總經動能不對稱報酬論點",
	"ai compute cycle durable demand thesis":   "AI 算力循環耐久需求論點",
	"deep tech ip moat differentiation thesis": "深度科技 IP 護城河差異化論點",
	"quality compounder catalyst thesis":       "品質複利型企業觸發論點",

	// ─── Already-Chinese control/guard strings (passthrough identity map) ───
	// These exist so that an "exactly equal" round-trip preserves the original
	// Chinese strings byte-for-byte (no invisible reformatting).
	"控制層已略過（未啟用 cro 檢查）":                  "控制層已略過（未啟用 CRO 檢查）",
	"未通過控制層過濾":                            "未通過控制層過濾",
	"未過濾任何推薦，全部放行":                        "未過濾任何推薦，全部放行",
	"強制阻擋全部推薦，當日不進場":                      "強制阻擋全部推薦，當日不進場",
	"控制層過濾記錄未載入（summary.json 缺失），推薦清單仍可用": "控制層過濾記錄未載入（summary.json 缺失），推薦清單仍可用",
	"本場次尚無推薦產出記錄":                         "本場次尚無推薦產出記錄",
}

// RationaleTemplate is a prefix-based rule for templated English rationales.
// The English prefix (e.g. "[crowded:") is preserved in the output because
// the pipeline UI (web/static/js/pages/pipeline.js:247) string-matches on
// it to render the "擁擠" badge. The hard rule of this wave is "no
// frontend change" — so the structural marker stays in English and only
// the trailing base reason is translated. The Chinese column below is
// documented for future use (e.g. when the frontend is updated to match
// on the Chinese marker) but is NOT emitted by TranslateReason.
type RationaleTemplate struct {
	Prefix  string // lowercased English prefix (e.g. "[crowded:")
	Chinese string // documented Chinese translation; not emitted by TranslateReason
}

// RationaleTemplates lists the prefix templates for control-layer badges
// (see internal/orchestrator/plugin_control.go lines 87, 171, 295, 298, 482).
// The prefix and dynamic substitution (e.g. "2 agents", "ackman_quality")
// are passed through unchanged; only the trailing base reason is
// translated by recursing into TranslateReason.
var RationaleTemplates = []RationaleTemplate{
	{Prefix: "[crowded:", Chinese: "[擁擠"},
	{Prefix: "[weighted:", Chinese: "[加權"},
	{Prefix: "[superinvestor:", Chinese: "[超級投資者"},
}

// RationaleSuffixMarkers list substring markers that indicate a composite
// reason made of [static base] + [dynamic suffix]. TranslateReason splits
// the input on the first occurrence of any marker, translates the leading
// static base via RationaleCorpus, and preserves the marker + suffix
// unchanged. This covers runtime-appended English suffixes whose dynamic
// numbers are audit-relevant and should NOT be masked by translation.
var RationaleSuffixMarkers = []string{
	" | narrative:", // system.go:966 (NarrativeConvictionModulator)
	" [prism:",      // phase3_controller.go:232 (PRISM conviction boost)
}

// TranslateReason returns the Chinese translation of an English rationale
// string, or the original (trimmed) input if no match is found.
//
// Lookup order:
//  1. Empty / whitespace-only input → empty string.
//  2. Already-CJK input → returned unchanged (avoids double-translation
//     of control-layer and guard-layer Chinese strings).
//  3. Exact match against RationaleCorpus (lowercased + whitespace-trimmed key).
//  4. Prefix match against RationaleTemplates (preserves the English
//     structural prefix and dynamic substitution; recursively translates
//     the trailing base reason).
//  5. Suffix split: if the input contains any RationaleSuffixMarker, the
//     leading base is translated via (3)/(4), and the marker + suffix is
//     preserved unchanged.
//  6. Passthrough: return the original (trimmed) input verbatim so
//     unmapped English strings stay visible for future corpus expansion.
//
// Never panics. Never returns the empty string for a non-empty input.
func TranslateReason(english string) string {
	trimmed := strings.TrimSpace(english)
	if trimmed == "" {
		return ""
	}
	if containsCJK(trimmed) {
		// Already Chinese — pass through to avoid double-translation.
		return trimmed
	}
	normalized := strings.ToLower(trimmed)

	// 3) Exact match against the static corpus.
	if translated, ok := RationaleCorpus[normalized]; ok {
		return translated
	}

	// 4) Prefix template match. Preserves the English structural prefix
	//    and dynamic substitution (e.g. agent count, skill name) so the
	//    frontend's `[crowded:` string-match on pipeline.js:247 keeps
	//    working, and recursively translates the trailing base reason.
	//    Examples:
	//      "[crowded:2 agents] price persistence with style overlay"
	//        → "[crowded:2 agents] 價格持續性與風格疊加"
	//      "[Superinvestor:ackman_quality] quality compounder catalyst thesis"
	//        → "[Superinvestor:ackman_quality] 品質複利型企業觸發論點"
	//      "[Weighted:3 agents]" (no base) → "[Weighted:3 agents]"
	for _, tpl := range RationaleTemplates {
		prefixLower := strings.ToLower(tpl.Prefix)
		if !strings.HasPrefix(normalized, prefixLower) {
			continue
		}
		closeIdx := strings.Index(trimmed, "]")
		if closeIdx < 0 {
			// Malformed template — fall through to passthrough.
			break
		}
		englishHead := trimmed[:closeIdx+1] // e.g. "[crowded:2 agents]"
		rest := strings.TrimSpace(trimmed[closeIdx+1:])
		if rest == "" {
			return englishHead
		}
		return englishHead + " " + TranslateReason(rest)
	}

	// 5) Suffix split. The leading base is translated; the suffix
	//    (audit-relevant metadata) is preserved unchanged.
	for _, marker := range RationaleSuffixMarkers {
		idx := strings.Index(strings.ToLower(trimmed), marker)
		if idx < 0 {
			continue
		}
		base := strings.TrimSpace(trimmed[:idx])
		suffix := trimmed[idx+1:] // marker has a leading space; drop it; rejoin with " "
		// Recursive lookup for the base. If it has its own template or
		// another suffix marker, those still apply.
		translatedBase := TranslateReason(base)
		return translatedBase + " " + suffix
	}

	// 6) Passthrough — unmapped English stays visible for future corpus expansion.
	return trimmed
}

// containsCJK returns true if the string contains at least one CJK
// character. Used to short-circuit TranslateReason on already-Chinese
// inputs so the control-layer / guard-layer Chinese strings don't get
// double-translated or mangled by the lowercased corpus lookup.
func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3400 && r <= 0x4DBF) || // CJK Unified Ideographs Extension A
			(r >= 0xF900 && r <= 0xFAFF) { // CJK Compatibility Ideographs
			return true
		}
	}
	return false
}
