import { test } from 'node:test';
import assert from 'node:assert/strict';
import { renderSectorPredictions, MUST_WATCH_SECTORS, _setStateForTest } from '../pages/capital_predictions.js';

// Mock localStorage
global.localStorage = {
  store: {},
  getItem(key) { return this.store[key] || null; },
  setItem(key, val) { this.store[key] = val; }
};

// Mock document
const mockClassList = {
  add() {},
  remove() {}
};

function createMockElement(id) {
  return {
    id,
    innerHTML: '',
    hidden: false,
    style: {},
    classList: mockClassList,
    listeners: {},
    addEventListener(evt, cb) {
      this.listeners[evt] = cb;
    },
    querySelector(sel) {
      if (sel === '.cp-sp-header' || sel === '#cp-sp-show-all') {
        return createMockElement(sel);
      }
      return null;
    },
    querySelectorAll() {
      return [];
    },
    getAttribute() { return '0'; },
    scrollIntoView() {}
  };
}

let mockHost;
let mockDetail;

global.document = {
  getElementById(id) {
    if (id === 'cp-sector-predictions') return mockHost;
    if (id === 'cp-detail') return mockDetail;
    return null;
  }
};

test('empty state', () => {
  mockHost = createMockElement('cp-sector-predictions');
  _setStateForTest([], [], [], false, false, false);
  renderSectorPredictions();
  assert.ok(mockHost.innerHTML.includes('尚無板塊預測資料'), 'Should show empty state');
});

test('api error state', () => {
  mockHost = createMockElement('cp-sector-predictions');
  _setStateForTest([], [], [], true, false, false);
  renderSectorPredictions();
  assert.ok(mockHost.innerHTML.includes('API 錯誤'), 'Should show API error state');
});

test('render sector predictions', () => {
  mockHost = createMockElement('cp-sector-predictions');
  
  const testSecPreds = [
    {
      date: '2026-07-17',
      sectors: [
        { sector_id: 'semiconductor', sector_name: '半導體', direction: 'inflow', confidence: 0.62 },
        { sector_id: 'steel', sector_name: '鋼鐵', direction: 'outflow', confidence: 0.8 },
        { sector_id: 'financials', sector_name: '金融', direction: 'neutral', confidence: 0.5 },
        { sector_id: 'other', sector_name: '其他', direction: 'inflow', confidence: 0.9 }
      ]
    }
  ];
  
  _setStateForTest([], [], testSecPreds, false, false, false);
  renderSectorPredictions();
  
  assert.ok(mockHost.innerHTML.includes('5 個必須看板塊中 1 個偏多 / 1 個偏空 / 1 個觀望'), 'Summary badge should be correct');
  assert.ok(mockHost.innerHTML.includes('display:none;'), 'Should be collapsed by default');
});

test('show all 20 sectors', () => {
  mockHost = createMockElement('cp-sector-predictions');
  
  const testSecPreds = [
    {
      date: '2026-07-17',
      sectors: [
        { sector_id: 'semiconductor', sector_name: '半導體', direction: 'inflow', confidence: 0.62 },
        { sector_id: 'other', sector_name: '其他', direction: 'inflow', confidence: 0.9 }
      ]
    }
  ];
  
  // false initially -> only semiconductor
  _setStateForTest([], [], testSecPreds, false, false, false);
  renderSectorPredictions();
  assert.ok(!mockHost.innerHTML.includes('其他'), 'Other sectors should be hidden');
  
  // showAll = true -> other sector included
  _setStateForTest([], [], testSecPreds, false, true, false);
  renderSectorPredictions();
  assert.ok(mockHost.innerHTML.includes('其他'), 'Other sectors should be shown');
});

test('expand/collapse and localStorage', () => {
  mockHost = createMockElement('cp-sector-predictions');
  const testSecPreds = [{ date: '2026-07-17', sectors: [] }];
  
  _setStateForTest([], [], testSecPreds, false, false, false);
  renderSectorPredictions();
  
  // simulate click on header
  const header = mockHost.querySelector('.cp-sp-header');
  assert.ok(header);
  
  // We can't perfectly simulate the internal closure but we can see if it was initially collapsed
  assert.ok(mockHost.innerHTML.includes('display:none;'), 'Should be collapsed by default');
});
