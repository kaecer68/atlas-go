package methodology

// ─── Skill → charter strategy category mapping ──────────────────────────
//
// Charter (ATLAS_METHODOLOGY.md §4) strategy IDs are coarse portfolio-level
// categories: growth / momentum / value / all_weather / event_arbitrage.
// Agent recommendations, however, are produced by skills (growth_momentum,
// technical_breakout, sector desks, ...). This mapping bridges the two so the
// advisor's period→strategy gating can be applied to raw recommendations.
//
// Mapping rationale (per configs/methodology_rules.yaml strategy definitions):
//   - growth          → 高營收成長、產業前景佳（適用 turnaround_up / bull）
//   - momentum        → 強勢股追價、技術突破（適用 turnaround_up / bull）
//   - value           → 低本益比、低股價淨值比、穩定獲利（適用 downturn / plateau）
//   - all_weather     → 風險平價、低 beta、高股息、低波動（防禦為先）
//   - event_arbitrage → 事件驅動短期錯價（ETF調整/MSCI/營收公告/除權息）
//
// Unmapped skills default to all_weather — the conservative keep: an unknown
// skill is never dropped by period gating.
var skillStrategyCategories = map[string]string{
	// ── Style ──
	"growth_momentum":    "growth",
	"technical_breakout": "momentum",
	"value_yield":        "value",
	"earnings_quality":   "value",

	// ── Sector desks ──
	"semiconductor_desk":   "momentum",
	"ai_supply_chain_desk": "growth",
	"etf_rotation_desk":    "all_weather",
	"financials_desk":      "value",
	"shipping_desk":        "momentum",
	"leo_satellite_desk":   "growth",
	"mining_desk":          "value",
	"energy_desk":          "value",
	"electronics_desk":     "momentum",
	"consumer_desk":        "value",
	"industrial_desk":      "value",
	"robotics_desk":        "momentum",

	// ── Superinvestor ──
	"druckenmiller_macro":      "momentum",
	"aschenbrenner_ai_compute": "growth",
	"baker_deep_tech":          "growth",
	"ackman_quality":           "value",
}

// SkillToStrategyCategory maps an agent skill to its charter strategy
// category. Skills without an explicit mapping default to "all_weather"
// (conservative keep — never gated out by period filtering).
func SkillToStrategyCategory(skill string) string {
	if cat, ok := skillStrategyCategories[skill]; ok {
		return cat
	}
	return "all_weather"
}
