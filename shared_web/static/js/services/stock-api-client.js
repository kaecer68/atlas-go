import { getToken } from './auth.js';

const TTL_MS = {
  quote: 30 * 1000,
  fundamentals: 24 * 60 * 60 * 1000,
  chips: 24 * 60 * 60 * 1000,
  technical: 5 * 60 * 1000,
  'sector-median-pe': 60 * 60 * 1000,
  // Monthly revenue is published ~10th of each month; 7-day TTL keeps
  // the section fresh across the mid-month publish without hammering
  // the FinMind-backed endpoint (which has a daily quota budget).
  'monthly-revenue': 7 * 24 * 60 * 60 * 1000
};

const STORAGE_PREFIX = 'atlas_stock_cache::';

function cacheKey(kind, symbol) {
  return `${STORAGE_PREFIX}${kind}::${symbol}`;
}

function readCache(kind, symbol) {
  try {
    const raw = localStorage.getItem(cacheKey(kind, symbol));
    if (!raw) return null;
    const entry = JSON.parse(raw);
    if (!entry || typeof entry.expiresAt !== 'number') return null;
    if (Date.now() > entry.expiresAt) {
      localStorage.removeItem(cacheKey(kind, symbol));
      return null;
    }
    return entry.data;
  } catch (e) {
    return null;
  }
}

function writeCache(kind, symbol, data) {
  try {
    const ttl = TTL_MS[kind];
    if (!ttl) return;
    localStorage.setItem(cacheKey(kind, symbol), JSON.stringify({
      data,
      expiresAt: Date.now() + ttl,
    }));
  } catch (e) {
    // quota exceeded or storage unavailable; fail silently
  }
}

async function fetchWithAuth(url) {
  const token = getToken();
  const headers = {};
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  const res = await fetch(url, { headers, credentials: 'include' });
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`;
    try {
      const body = await res.json();
      if (body && typeof body.error === 'string' && body.error) {
        message = body.error;
      }
    } catch (e) {
      // ignore parse failure, keep status-based message
    }
    throw new Error(message);
  }
  return res.json();
}

async function fetchCached(kind, symbol, urlPath) {
  const cached = readCache(kind, symbol);
  if (cached) {
    return { ...cached, _fromCache: true };
  }
  const data = await fetchWithAuth(urlPath);
  writeCache(kind, symbol, data);
  return { ...data, _fromCache: false };
}

function resultToState(result) {
  if (result.status === 'fulfilled') {
    return { status: 'loaded', data: result.value, error: null };
  }
  return { status: 'error', data: null, error: result.reason.message };
}

export async function fetchStockBundle(symbol) {
  const results = await Promise.allSettled([
    fetchCached('quote', symbol, `/api/stock/quote?symbol=${symbol}`),
    fetchCached('fundamentals', symbol, `/api/stock/fundamentals?symbol=${symbol}`),
    fetchCached('chips', symbol, `/api/stock/chips?symbol=${symbol}`),
    fetchCached('technical', symbol, `/api/stock/technical?symbol=${symbol}&days=90`)
  ]);

  return {
    quote: resultToState(results[0]),
    fundamentals: resultToState(results[1]),
    chips: resultToState(results[2]),
    technical: resultToState(results[3])
  };
}

export async function fetchSectorMedianPE(sector) {
  return fetchCached('sector-median-pe', sector, `/api/stock/sector-median-pe?sector=${encodeURIComponent(sector)}`);
}

// fetchStockMonthlyRevenue calls GET /api/stock/monthly_revenue?symbol=X
// (optionally year/month). Returns the most recent published monthly
// revenue plus YoY% (change_pct) and MoM%. Coverage is broader than the
// 4 stocktools endpoints: FinMind TaiwanStockMonthRevenue covers TWSE
// 上市 + TPEX 上櫃 + 興櫃, so TPEX symbols like 3131/3587/6640 return
// data here even though chips/fundamentals mark them NOT_COVERED.
// The endpoint 503s when the FinMind daily quota is nearly exhausted
// (see handler.go monthlyRevenueMinQuota) — the caller treats that as a
// non-fatal missing section.
export async function fetchStockMonthlyRevenue(symbol, year, month) {
  let path = `/api/stock/monthly_revenue?symbol=${encodeURIComponent(symbol)}`;
  if (year) path += `&year=${encodeURIComponent(year)}`;
  if (month) path += `&month=${encodeURIComponent(month)}`;
  // Cache key must include the (year, month) variant — otherwise queries
  // for different reporting months would share the same 7-day cache entry
  // (e.g. 2026-07 vs 2026-08 would return the same revenue). 'latest' is
  // used when no explicit month is requested (server default = last
  // closed month), which changes over time and should not be pinned by
  // a stale cache entry across a month boundary either.
  const variant = year ? `${year}-${month || ''}` : 'latest';
  return fetchCached('monthly-revenue', `${symbol}::${variant}`, path);
}


// fetchStockCoverage calls GET /api/stock/coverage?symbol=X to discover
// whether the 4 stocktools endpoints (quote/fundamentals/chips/technical)
// will return real data for this symbol. Out-of-scope symbols return
// 200 + covered=false (not 404 — coverage is informational, not an error).
// See docs/manifests/2026-08-06-stock-coverage-notice.md.
export async function fetchStockCoverage(symbol) {
  try {
    return await fetchWithAuth(`/api/stock/coverage?symbol=${encodeURIComponent(symbol)}`);
  } catch (e) {
    // Coverage lookup is best-effort; fail open so a coverage outage
    // doesn't break the entire stock-quote page.
    return { symbol, covered: true, listing: 'UNKNOWN', quote_covered: true, reason: '' };
  }
}

// fetchStockBundleWithCoverage runs the existing 4-endpoint bundle AND
// a parallel coverage lookup. When the symbol is out-of-scope, attaches
// the coverage context to the bundle so render components can detect
// `bundle.coverage.covered === false` and show a scope badge instead of
// an error banner.
export async function fetchStockBundleWithCoverage(symbol) {
  const [coverage, bundle] = await Promise.all([
    fetchStockCoverage(symbol),
    fetchStockBundle(symbol)
  ]);
  if (coverage && !coverage.covered) {
    bundle.coverage = coverage;
  }
  return bundle;
}
