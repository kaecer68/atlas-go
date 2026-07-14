/**
 * demo-data.js
 * Static demo portfolio data for the retail investor landing page.
 * Used when no real portfolio data is available (fresh install, no simulation run).
 */

/**
 * @typedef {Object} DemoPosition
 * @property {string} symbol
 * @property {string} name
 * @property {number} shares
 * @property {number} avgCost
 * @property {number} price
 * @property {number} weight
 * @property {string} sector
 */

/** @returns {DemoPosition[]} */
export function getDemoPortfolio() {
  return [
    { symbol: '2330', name: '台積電', shares: 1000, avgCost: 875.0, price: 920.0, weight: 0.42, sector: '半導體' },
    { symbol: '2317', name: '鴻海', shares: 2000, avgCost: 198.5, price: 205.0, weight: 0.18, sector: '電子零組件' },
    { symbol: '2454', name: '聯發科', shares: 300, avgCost: 1180.0, price: 1150.0, weight: 0.15, sector: '半導體' },
    { symbol: '2308', name: '台達電', shares: 400, avgCost: 320.0, price: 335.0, weight: 0.10, sector: '電子零組件' },
    { symbol: '2881', name: '富邦金', shares: 800, avgCost: 88.0, price: 92.0, weight: 0.08, sector: '金融保險' },
    { symbol: '2603', name: '長榮', shares: 500, avgCost: 165.0, price: 158.0, weight: 0.07, sector: '航運' },
  ];
}

export function getDemoRiskSnapshot() {
  return {
    level: 'medium',
    var95: 0.028,
    maxDrawdown: 0.095,
    hhi: 0.22,
    beta: 1.05,
  };
}

export function getDemoTradeHistory() {
  return [
    { date: '2026-06-25', symbol: '2330', side: 'BUY', shares: 500, price: 870.0 },
    { date: '2026-06-18', symbol: '2317', side: 'BUY', shares: 1000, price: 195.0 },
    { date: '2026-06-10', symbol: '2454', side: 'BUY', shares: 300, price: 1180.0 },
  ];
}
