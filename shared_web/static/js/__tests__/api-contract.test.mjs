// shared_web/static/js/__tests__/api-contract.test.mjs
//
// Integration contract tests: validate live API responses against JSON Schemas.
// These tests hit the running Docker server (localhost:18080).
// If the server is unavailable, tests are skipped (not failed).
//
// 执行：node --test shared_web/static/js/__tests__/api-contract.test.mjs

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const SCHEMAS_DIR = resolve(__dirname, '../schemas');

function loadSchema(name) {
  const content = readFileSync(resolve(SCHEMAS_DIR, name), 'utf8');
  return JSON.parse(content);
}

const macroSnapshotSchema = loadSchema('macro-snapshot.schema.json');
const usIndicesSchema = loadSchema('us-indices.schema.json');
const stressIndexSchema = loadSchema('stress-index.schema.json');

const BASE_URL = 'http://localhost:18080';

// ============================================================================
// Helper: check if server is reachable
// ============================================================================

/** Returns true if the Atlas server is running on BASE_URL. */
async function isServerRunning() {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 3000);
    const res = await fetch(`${BASE_URL}/health`, { signal: controller.signal });
    clearTimeout(timeout);
    return res.ok;
  } catch (_e) {
    return false;
  }
}

/**
 * Validate a value against a JSON Schema (draft-07 subset).
 * Throws AssertionError on validation failure.
 */
function validateAgainstSchema(value, schema, path = 'root') {
  if (schema.type === 'object') {
    if (typeof value !== 'object' || value === null || Array.isArray(value)) {
      throw new assert.AssertionError({ message: `${path}: expected object, got ${typeof value}` });
    }
    if (Array.isArray(schema.required)) {
      for (const req of schema.required) {
        if (!(req in value)) {
          throw new assert.AssertionError({ message: `${path}: missing required field "${req}"` });
        }
      }
    }
    if (schema.properties) {
      for (const [key, propSchema] of Object.entries(schema.properties)) {
        if (key in value) {
          validateAgainstSchema(value[key], propSchema, `${path}.${key}`);
        }
      }
    }
  } else if (schema.type === 'array') {
    if (!Array.isArray(value)) {
      throw new assert.AssertionError({ message: `${path}: expected array, got ${typeof value}` });
    }
    if (schema.items) {
      value.forEach((item, i) => validateAgainstSchema(item, schema.items, `${path}[${i}]`));
    }
  } else if (schema.type === 'number') {
    if (typeof value !== 'number') {
      throw new assert.AssertionError({ message: `${path}: expected number, got ${typeof value}` });
    }
  } else if (schema.type === 'string') {
    if (typeof value !== 'string') {
      throw new assert.AssertionError({ message: `${path}: expected string, got ${typeof value}` });
    }
  }
}

// ============================================================================
// P0: /api/macro/snapshot/latest validates against macro-snapshot schema
// ============================================================================

test('/api/macro/snapshot/latest: response validates against macro-snapshot schema', async () => {
  if (!await isServerRunning()) {
    assert.skip('Server not running on localhost:18080 — skipping integration test');
    return;
  }

  const res = await fetch(`${BASE_URL}/api/macro/snapshot/latest`);
  assert.ok(res.ok, `HTTP ${res.status} for /api/macro/snapshot/latest`);

  const data = await res.json();
  validateAgainstSchema(data, macroSnapshotSchema);
});

// ============================================================================
// P0: /api/dashboard/us-indices validates against us-indices schema
// ============================================================================

test('/api/dashboard/us-indices: response validates against us-indices schema', async () => {
  if (!await isServerRunning()) {
    assert.skip('Server not running on localhost:18080 — skipping integration test');
    return;
  }

  const res = await fetch(`${BASE_URL}/api/dashboard/us-indices`);
  assert.ok(res.ok, `HTTP ${res.status} for /api/dashboard/us-indices`);

  const data = await res.json();
  validateAgainstSchema(data, usIndicesSchema);

  // Extra structural assertions
  assert.ok(Array.isArray(data.indices), 'indices must be an array');
  assert.ok(Array.isArray(data.tech_stocks), 'tech_stocks must be an array');
  if (data.indices.length > 0) {
    const item = data.indices[0];
    assert.ok('symbol' in item, 'index item must have symbol');
    assert.ok('value' in item, 'index item must have value');
    assert.ok('change_pct' in item, 'index item must have change_pct');
  }
  if (data.tech_stocks.length > 0) {
    const item = data.tech_stocks[0];
    assert.ok('symbol' in item, 'tech_stocks item must have symbol');
    assert.ok('value' in item, 'tech_stocks item must have value');
    assert.ok('change_pct' in item, 'tech_stocks item must have change_pct');
  }
});

// ============================================================================
// P0: /api/taiwan/stress-index validates against stress-index schema
// ============================================================================

test('/api/taiwan/stress-index: response validates against stress-index schema', async () => {
  if (!await isServerRunning()) {
    assert.skip('Server not running on localhost:18080 — skipping integration test');
    return;
  }

  const res = await fetch(`${BASE_URL}/api/taiwan/stress-index`);
  assert.ok(res.ok, `HTTP ${res.status} for /api/taiwan/stress-index`);

  const data = await res.json();
  validateAgainstSchema(data, stressIndexSchema);

  // Extra type assertions
  assert.ok(typeof data.index === 'number', 'index must be a number');
  assert.ok(typeof data.regime === 'string', 'regime must be a string');
  assert.ok(typeof data.updated_at === 'string', 'updated_at must be a string');
});
