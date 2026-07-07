// shared_web/static/js/__tests__/contract.test.mjs
//
// Schema contract validation tests: verify JSON Schema files are well-formed
// and contain the expected field definitions. Catches field-name drift early.
//
// 执行：node --test shared_web/static/js/__tests__/contract.test.mjs

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

// Load all schemas
const macroSnapshotSchema = loadSchema('macro-snapshot.schema.json');
const usIndicesSchema = loadSchema('us-indices.schema.json');
const stressIndexSchema = loadSchema('stress-index.schema.json');
const recommendationPipelineSchema = loadSchema('recommendation-pipeline.schema.json');
const narrativeBundleSchema = loadSchema('narrative-bundle.schema.json');
const calendarEventsSchema = loadSchema('calendar-events.schema.json');
const portfolioStateSchema = loadSchema('portfolio-state.schema.json');

// ============================================================================
// Helper: validate basic JSON Schema structure
// ============================================================================

function assertValidSchemaStructure(schema, name) {
  assert.ok(schema && typeof schema === 'object', `${name} must be a non-null object`);
  assert.ok('$schema' in schema, `${name} must have $schema`);
  assert.ok('$id' in schema, `${name} must have $id`);
  assert.equal(schema.type, 'object', `${name} must have type:"object"`);
  assert.ok(Array.isArray(schema.required), `${name} must have required array`);
  assert.ok(schema.properties && typeof schema.properties === 'object', `${name} must have properties object`);
}

// ============================================================================
// P0: All schemas are valid JSON and have required JSON Schema keys
// ============================================================================

test('macroSnapshotSchema: valid JSON Schema structure', () => {
  assertValidSchemaStructure(macroSnapshotSchema, 'macroSnapshotSchema');
});

test('usIndicesSchema: valid JSON Schema structure', () => {
  assertValidSchemaStructure(usIndicesSchema, 'usIndicesSchema');
});

test('stressIndexSchema: valid JSON Schema structure', () => {
  assertValidSchemaStructure(stressIndexSchema, 'stressIndexSchema');
});

test('recommendationPipelineSchema: valid JSON Schema structure', () => {
  assertValidSchemaStructure(recommendationPipelineSchema, 'recommendationPipelineSchema');
});

test('narrativeBundleSchema: valid JSON Schema structure', () => {
  assertValidSchemaStructure(narrativeBundleSchema, 'narrativeBundleSchema');
});

test('calendarEventsSchema: valid JSON Schema structure', () => {
  assertValidSchemaStructure(calendarEventsSchema, 'calendarEventsSchema');
});

test('portfolioStateSchema: valid JSON Schema structure', () => {
  assertValidSchemaStructure(portfolioStateSchema, 'portfolioStateSchema');
});

// ============================================================================
// P0: macro-snapshot schema has the 5 required market indicator fields
// ============================================================================

test('macroSnapshotSchema: has required top-level fields', () => {
  const required = macroSnapshotSchema.required;
  assert.ok(required.includes('recorded_at'), 'recorded_at must be required');
});

test('macroSnapshotSchema: has all 5 required market indicator properties', () => {
  // Phase 0 audit: these 5 fields must exist to render the home dashboard
  const props = macroSnapshotSchema.properties;
  assert.ok('taiex' in props, 'taiex must exist in macro snapshot');
  assert.ok('tsm_adr' in props, 'tsm_adr must exist in macro snapshot');
  assert.ok('sox_index' in props, 'sox_index must exist in macro snapshot');
  assert.ok('ndx_index' in props, 'ndx_index must exist in macro snapshot');
  assert.ok('foreign_investor_net' in props, 'foreign_investor_net must exist in macro snapshot');
});

test('macroSnapshotSchema: tsm_adr field uses "tsm_adr" NOT "tsm" (Phase 0 regression guard)', () => {
  // Phase 0 fix: backend was using "tsm" but frontend expected "tsm_adr".
  // This test ensures we never regress to the wrong field name.
  const props = macroSnapshotSchema.properties;
  assert.ok('tsm_adr' in props, 'field must be named tsm_adr');
  assert.ok(!('tsm' in props && props.tsm !== props.tsm_adr), 'tsm must NOT exist as a separate field (should be tsm_adr)');
});

test('macroSnapshotSchema: each indicator field is an object with value/change_pct', () => {
  const props = macroSnapshotSchema.properties;
  for (const field of ['taiex', 'tsm_adr', 'sox_index', 'ndx_index', 'foreign_investor_net']) {
    const indicator = props[field];
    assert.equal(indicator.type, 'object', `${field} must be type:object`);
    assert.ok(indicator.required.includes('value'), `${field} must require value`);
    assert.ok(indicator.required.includes('change_pct'), `${field} must require change_pct`);
    assert.ok(indicator.required.includes('timestamp'), `${field} must require timestamp`);
  }
});

// ============================================================================
// P0: us-indices schema has indices array and tech_stocks array
// ============================================================================

test('usIndicesSchema: has required arrays', () => {
  const required = usIndicesSchema.required;
  assert.ok(required.includes('indices'), 'indices must be required');
  assert.ok(required.includes('tech_stocks'), 'tech_stocks must be required');
});

test('usIndicesSchema: indices is an array of objects with symbol/value/change_pct', () => {
  const indices = usIndicesSchema.properties.indices;
  assert.equal(indices.type, 'array', 'indices must be type:array');
  assert.equal(indices.items.type, 'object', 'indices items must be objects');
  assert.ok(indices.items.required.includes('symbol'), 'indices items must require symbol');
  assert.ok(indices.items.required.includes('value'), 'indices items must require value');
  assert.ok(indices.items.required.includes('change_pct'), 'indices items must require change_pct');
});

test('usIndicesSchema: tech_stocks is an array of objects with symbol/value/change_pct', () => {
  const techStocks = usIndicesSchema.properties.tech_stocks;
  assert.equal(techStocks.type, 'array', 'tech_stocks must be type:array');
  assert.equal(techStocks.items.type, 'object', 'tech_stocks items must be objects');
  assert.ok(techStocks.items.required.includes('symbol'), 'tech_stocks items must require symbol');
  assert.ok(techStocks.items.required.includes('value'), 'tech_stocks items must require value');
  assert.ok(techStocks.items.required.includes('change_pct'), 'tech_stocks items must require change_pct');
});

// ============================================================================
// P1: stress-index schema structure
// ============================================================================

test('stressIndexSchema: has required fields score/regime/timestamp', () => {
  const required = stressIndexSchema.required;
  assert.ok(required.includes('score'), 'score must be required');
  assert.ok(required.includes('regime'), 'regime must be required');
  assert.ok(required.includes('timestamp'), 'timestamp must be required');
  assert.equal(stressIndexSchema.properties.score.type, 'number', 'score must be number');
  assert.equal(stressIndexSchema.properties.regime.type, 'string', 'regime must be string');
  assert.equal(stressIndexSchema.properties.timestamp.type, 'number', 'timestamp must be number');
});

// ============================================================================
// P1: recommendation-pipeline schema structure
// ============================================================================

test('recommendationPipelineSchema: items is array with required item fields', () => {
  const items = recommendationPipelineSchema.properties.items;
  assert.equal(items.type, 'array', 'items must be type:array');
  assert.equal(items.items.type, 'object', 'items items must be objects');
  assert.ok(items.items.required.includes('theme'), 'item must require theme');
  assert.ok(items.items.required.includes('sentiment'), 'item must require sentiment');
  assert.ok(items.items.required.includes('confidence'), 'item must require confidence');
  assert.ok(items.items.required.includes('severity'), 'item must require severity');
});
