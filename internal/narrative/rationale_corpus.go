package narrative

import "strings"

// reasonCorpus maps English reason strings to their Chinese translations.
// Keys are normalized (lowercase + trimmed) for case-insensitive matching.
// This is a deterministic lookup — no LLM involvement.
var reasonCorpus = map[string]string{
	// ── Sector agents ──
	"semiconductor leadership and supply-chain role":       "半導體領導地位與供應鏈角色",
	"ai infrastructure order-flow sensitivity":             "AI 基礎建設訂單流敏感度",
	"leo satellite infrastructure and deployment cycle":    "低軌衛星基礎建設與部署週期",
	"financial carry with resilient balance-sheet posture": "金融利差與穩健資產負債表",
	"shipping beta used as tactical cycle exposure":        "航運 Beta 作為戰術週期曝險",
	"robotics automation capex cycle":                      "機器人自動化資本支出週期",
	"mining and precious metals cycle":                     "礦業與貴金屬週期",
	"energy commodity cycle":                               "能源商品週期",
	"electronics component demand cycle":                   "電子零組件需求週期",
	"consumer staples and retail cycle":                    "民生消費與零售週期",
	"industrial manufacturing and infrastructure cycle":    "工業製造與基礎建設週期",

	// ── Style agents ──
	"price persistence with style overlay":             "價格持續性與風格疊加",
	"defensive yield lens with valuation discipline":   "防禦性殖利率視角與估值紀律",
	"earnings quality and forward visibility support":  "盈餘品質與前瞻能見度支撐",
	"breakout structure confirmed by volume and close": "突破結構經量價確認",

	// ── Position evaluation (shared across sector + style) ──
	"momentum decay: significant weakness (last << open)": "動能衰減：顯著弱勢（收盤遠低於開盤）",
	"signal weakening: reduce exposure":                   "信號減弱：降低曝險",
	"position evaluation: maintain holding":               "持倉評估：維持持有",

	// ── Control layer (already Chinese — pass through) ──
	"控制層已略過（未啟用 cro 檢查）": "控制層已略過（未啟用 CRO 檢查）",
	"未通過控制層過濾":           "未通過控制層過濾",
}

// TranslateReason returns the Chinese translation of an English reason string.
// If no translation exists, the original string is returned unchanged.
// Matching is case-insensitive and whitespace-insensitive after normalization.
func TranslateReason(english string) string {
	if english == "" {
		return ""
	}
	key := strings.ToLower(strings.TrimSpace(english))
	if translated, ok := reasonCorpus[key]; ok {
		return translated
	}
	// Check if it's already Chinese (contains CJK characters).
	// Pass through without attempting translation — avoids double-translation.
	if containsCJK(english) {
		return english
	}
	// Unknown English string — return as-is so it's visible for future corpus expansion.
	return english
}

// containsCJK returns true if the string contains at least one CJK character
// (Unicode blocks: CJK Unified Ideographs, CJK Compatibility, CJK Extension A).
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
