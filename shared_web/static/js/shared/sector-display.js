// shared_web/static/js/shared/sector-display.js
//
// FU-7 Phase E: canonical sector display map for the frontend.
//
// Mirrors Go's `internal/industry.DisplayZHTw` (PR #1159). The backend
// Flutter / MCP / JSON responses will eventually deliver sector IDs as
// snake_case English IDs (e.g. "semiconductor"). This module provides the
// canonical → 中文 full label mapping for UI rendering.
//
// Kept intentionally minimal — additive only. Subsequent Phases (B-F)
// will migrate existing legacy Chinese strings in demo-data.js et al.
// to canonical IDs, then look up display via this map.

// Canonical snake_case English sector IDs. Order matches backend.
// Use only as keys; never use the Chinese label as a map key in business
// logic — always go through SECTOR_DISPLAY_ZH.
export const SECTOR_IDS = Object.freeze([
	'auto',
	'biotech',
	'cement',
	'chemicals',
	'construction',
	'electronics',
	'energy',
	'financials',
	'food',
	'machinery',
	'optoelectronics',
	'other_electronics',
	'plastics',
	'retail',
	'semiconductor',
	'shipping',
	'steel',
	'telecom',
	'textiles',
	'tourism',
]);

// Canonical → 中文 full label (Traditional Chinese). Mirrors Go's
// internal/industry.DisplayZHTw. Adding a new sector requires touching both.
export const SECTOR_DISPLAY_ZH = Object.freeze({
	auto:               '汽車',
	biotech:            '生技醫療',
	cement:             '水泥',
	chemicals:          '化學',
	construction:       '營建',
	electronics:        '電子零組件',
	energy:             '油電燃氣',
	financials:         '金融保險',
	food:               '食品',
	machinery:          '電機機械',
	optoelectronics:    '光電',
	other_electronics:  '其他電子',
	plastics:           '塑膠',
	retail:             '百貨',
	semiconductor:      '半導體',
	shipping:           '航運',
	steel:              '鋼鐵',
	telecom:            '通信網路',
	textiles:           '紡織',
	tourism:            '觀光',
});

// DisplayZH returns the Traditional-Chinese label for canonical id, or the
// raw input if unknown. Forward-compatible: passing a legacy Chinese string
// (e.g. "金融" instead of "financials") falls through unchanged for now;
// later Phases will tighten this once the backend emits canonical IDs.
export function displayZH(input) {
	if (!input) return '';
	if (Object.prototype.hasOwnProperty.call(SECTOR_DISPLAY_ZH, input)) {
		return SECTOR_DISPLAY_ZH[input];
	}
	return input;
}
