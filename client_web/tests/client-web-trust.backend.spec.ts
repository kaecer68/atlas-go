/**
 * Trust & clarity audit for the investor-facing client_web — backend half.
 *
 * Runs ONLY via `npm run test:e2e:backend` (playwright.backend.config.ts)
 * which starts the real atlas-go server on port 18080. CI's frontend-tests
 * job does not provide Go or PostgreSQL, so this file is excluded from the
 * default config via testIgnore patterns in both configs.
 *
 * The single raw-backend assertion (`request.get('/api/stock/quote')`) lives
 * here; the rest of the trust checks (page route + content) live in
 * client-web-trust.spec.ts and run against the static SPA server.
 */
import { test, expect } from '@playwright/test';
import { skipIfAtlasOffline } from '../../tests-shared/atlas-check';

test.beforeAll(async () => { await skipIfAtlasOffline(test); });

test('backend API /api/stock/quote penetrates to frontend', async ({ request }) => {
  const res = await request.get('/api/stock/quote?symbol=2330');
  expect(res.status()).toBe(200);
  const body = await res.json();
  expect(body).toHaveProperty('symbol', '2330');
  expect(body).toHaveProperty('last');
});
