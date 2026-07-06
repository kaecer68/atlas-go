import { test as base } from '@playwright/test';

// Check if atlas server is reachable. If not, skip all tests.
export async function skipIfAtlasOffline(test: typeof base) {
  try {
    const res = await fetch('http://localhost:18080/health');
    if (!res.ok) throw new Error('unhealthy');
  } catch {
    test.skip(true, 'atlas server not running — skipping E2E tests');
  }
}
