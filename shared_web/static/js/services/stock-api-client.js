import { getToken } from './auth.js';

async function fetchWithAuth(url) {
  const token = getToken();
  const headers = {};
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  const res = await fetch(url, { headers, credentials: 'include' });
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`);
  }
  return res.json();
}

function resultToState(result) {
  if (result.status === 'fulfilled') {
    return { status: 'loaded', data: result.value, error: null };
  }
  return { status: 'error', data: null, error: result.reason.message };
}

export async function fetchStockBundle(symbol) {
  const results = await Promise.allSettled([
    fetchWithAuth(`/api/stock/quote?symbol=${symbol}`),
    fetchWithAuth(`/api/stock/fundamentals?symbol=${symbol}`),
    fetchWithAuth(`/api/stock/chips?symbol=${symbol}`),
    fetchWithAuth(`/api/stock/technical?symbol=${symbol}&days=90`)
  ]);

  return {
    quote: resultToState(results[0]),
    fundamentals: resultToState(results[1]),
    chips: resultToState(results[2]),
    technical: resultToState(results[3])
  };
}
