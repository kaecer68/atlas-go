// shared_web/static/js/__tests__/sector-display.test.mjs
//
// FU-7 Phase E: contract tests for sector-display.js. Validates parity with
// backend internal/industry.DisplayZHTw (PR #1159).

import { test } from 'node:test';
import assert from 'node:assert/strict';

import {
	SECTOR_IDS,
	SECTOR_DISPLAY_ZH,
	displayZH,
} from '../shared/sector-display.js';

test('SECTOR_DISPLAY_ZH covers all 20 canonical sectors', () => {
	const expectedCount = 20;
	assert.equal(
		Object.keys(SECTOR_DISPLAY_ZH).length,
		expectedCount,
		`expected ${expectedCount} canonical sector IDs, got ${Object.keys(SECTOR_DISPLAY_ZH).length}`,
	);
});

test('SECTOR_IDS list matches keys of SECTOR_DISPLAY_ZH', () => {
	const idsSet = new Set(SECTOR_IDS);
	const mapKeys = new Set(Object.keys(SECTOR_DISPLAY_ZH));
	assert.equal(idsSet.size, mapKeys.size);
	for (const id of idsSet) {
		assert.ok(mapKeys.has(id), `SECTOR_IDS has ${id} but SECTOR_DISPLAY_ZH missing`);
	}
});

test('every label is a non-empty Traditional-Chinese string', () => {
	const chineseRegex = /[\u4e00-\u9fff]/;
	for (const [id, label] of Object.entries(SECTOR_DISPLAY_ZH)) {
		assert.ok(typeof label === 'string' && label.length > 0, `${id}: empty label`);
		assert.ok(
			chineseRegex.test(label),
			`${id}: label "${label}" should contain at least one Chinese character`,
		);
	}
});

test('displayZH resolves canonical IDs to full Chinese labels', () => {
	assert.equal(displayZH('semiconductor'), '半導體');
	assert.equal(displayZH('financials'), '金融保險');
	assert.equal(displayZH('shipping'), '航運');
	assert.equal(displayZH('telecom'), '通信網路');
});

test('displayZH returns input unchanged for unknown strings (forward-compat)', () => {
	// Legacy/truncated Chinese forms should pass through until Phase B-F migrate
	// them to canonical IDs.
	assert.equal(displayZH('金融'), '金融');
	assert.equal(displayZH('半導體'), '半導體');
});

test('displayZH handles empty and undefined gracefully', () => {
	assert.equal(displayZH(''), '');
	assert.equal(displayZH(undefined), '');
	assert.equal(displayZH(null), '');
});

// Lock known canonical key values. Adds a regression guard: changing any
// sector's snake_case ID breaks the frontend → backend wire on the same PR.
test('canonical ID values match backend DisplayZHTw keys (lock values)', () => {
	const expected = {
		semiconductor:     '半導體',
		electronics:       '電子零組件',
		optoelectronics:   '光電',
		financials:        '金融保險',
		cement:            '水泥',
		plastics:          '塑膠',
		textiles:          '紡織',
		steel:             '鋼鐵',
		shipping:          '航運',
		food:              '食品',
		auto:              '汽車',
		telecom:           '通信網路',
		chemicals:         '化學',
		biotech:           '生技醫療',
		construction:      '營建',
		other_electronics: '其他電子',
		machinery:         '電機機械',
		tourism:           '觀光',
		retail:            '百貨',
		energy:            '油電燃氣',
	};
	for (const [id, zh] of Object.entries(expected)) {
		assert.equal(SECTOR_DISPLAY_ZH[id], zh, `mismatch for ${id}`);
	}
});
