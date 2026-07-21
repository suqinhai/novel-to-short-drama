const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const read = (name) => fs.readFileSync(path.join(root, name), 'utf8').replace(/^\uFEFF/, '');
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

const sql = read('database/06-narrative-ir-foundation.sql');
assert(/^BEGIN;\s*$/m.test(sql) && /COMMIT;\s*$/.test(sql), '06 migration must have one transaction boundary');
assert(/pg_advisory_xact_lock/.test(sql), '06 migration advisory lock missing');
assert(/lock_timeout/.test(sql), '06 migration lock timeout missing');
assert(/schema_migrations/.test(sql) && /phase1-contract-v2-20260721/.test(sql), '06 migration ledger/checksum missing');
assert(/\\if :phase1_apply/.test(sql) && /already applied with matching checksum; no-op/.test(sql), '06 migration must short-circuit on a matching ledger row');
assert(!/\bDROP\s+(?:TABLE|COLUMN)\b/i.test(sql), '06 migration contains destructive DROP TABLE/COLUMN');
assert(!/\bTRUNCATE\b/i.test(sql), '06 migration contains TRUNCATE');
assert(!/\bDELETE\s+FROM\b/i.test(sql), '06 migration contains DELETE FROM');
assert(!/\bBYTEA\b/i.test(sql), '06 migration must not store source/provider blobs in BYTEA');

const requiredTables = [
  'schema_migrations', 'migration_audit', 'source_works', 'source_versions',
  'source_chapters', 'chapter_revisions', 'source_version_chapters', 'source_spans',
  'operations', 'source_import_jobs', 'source_import_items', 'project_source_bindings',
  'legacy_source_bindings', 'narrative_ir_revisions', 'narrative_entities',
  'narrative_entity_revisions', 'narrative_entity_aliases', 'narrative_entity_mentions',
  'narrative_facts', 'narrative_fact_revisions', 'fact_evidence',
  'narrative_event_revisions', 'event_participants', 'event_relations',
  'character_state_changes', 'timeline_facts', 'foreshadow_threads',
  'foreshadow_occurrences', 'story_arcs', 'story_arc_revisions', 'story_arc_events',
  'adaptation_specs', 'adaptation_spec_versions', 'adaptation_scope_chapters',
  'adaptation_scope_arcs', 'adaptation_rules', 'compiler_runs', 'compiler_checkpoints',
  'compiler_diagnostics', 'adaptation_plans', 'adaptation_episode_plans',
  'episode_event_assignments', 'artifact_types', 'artifacts', 'artifact_dependencies',
  'artifact_source_evidence', 'source_change_sets', 'source_change_items',
  'invalidation_tasks', 'invalidation_impacts'
];
for (const table of requiredTables) {
  assert(new RegExp(`CREATE TABLE IF NOT EXISTS drama\\.${table}\\b`, 'i').test(sql), `06 migration missing table ${table}`);
}

for (const token of [
  'source_version_id', 'chapter_revision_id', 'primary_source_span_id', 'confidence',
  'adaptation_spec_version_id', 'observed_upstream_hash', 'invalidation_task_id',
  'lease_expires_at', 'idempotency_key', 'updated_at'
]) {
  assert(sql.includes(token), `06 migration missing public-contract token ${token}`);
}
assert(/FOREIGN KEY\(primary_source_span_id, work_id, source_version_id, chapter_id, primary_chapter_revision_id\)/.test(sql), 'fact/entity composite provenance FK missing');
assert(/guard_published_source_version/.test(sql) && /guard_published_source_child/.test(sql), 'published-source immutability guards missing');
assert(!/UPDATE drama\.projects SET display_name/.test(sql), 'migration must not rewrite legacy project timestamps for display_name');
assert(/CREATE OR REPLACE FUNCTION drama\.claim_operation/.test(sql) && /CREATE OR REPLACE FUNCTION drama\.heartbeat_operation/.test(sql) && /CREATE OR REPLACE FUNCTION drama\.assert_operation_claim/.test(sql) && /CREATE OR REPLACE FUNCTION drama\.finish_operation/.test(sql), 'atomic operation lease/finalization functions missing');
assert(/guard_published_ir_revision/.test(sql) && /guard_active_spec_version/.test(sql) && /validate_episode_event_assignment/.test(sql) && /guard_episode_plan_reparent/.test(sql), 'IR/spec/compiler output freeze guards missing');
assert(/BEFORE INSERT OR UPDATE OR DELETE ON drama\.source_version_chapters/.test(sql), 'sealed source membership insert guard missing');
assert(/FROM drama\.novels n[\s\S]+ON CONFLICT\(legacy_novel_id\) DO NOTHING/.test(sql), 'legacy novel mapping backfill missing');

const verifySQL = read('database/06-verify-narrative-foundation.sql');
for (const check of [
  'migration 06 ledger row/checksum missing',
  'legacy novel backfill incomplete',
  'legacy chapter backfill incomplete',
  'narrative facts without exact source provenance',
  'adaptation chapter scope crosses source works',
  "'PASS' AS result"
]) {
  assert(verifySQL.includes(check), `verification SQL missing check: ${check}`);
}
assert(!/\b(?:INSERT|UPDATE|DELETE|TRUNCATE|DROP|ALTER|CREATE)\b/i.test(verifySQL.replace(/CREATE|UPDATE/g, (x) => x)), 'verification SQL should remain read-only');

const bootstrap = read('database/bootstrap.sh');
assert(bootstrap.includes('/opt/drama/06-narrative-ir-foundation.sql'), 'bootstrap does not apply migration 06');
const compose = read('docker-compose.yml');
assert(compose.includes('06-narrative-ir-foundation.sql:/opt/drama/06-narrative-ir-foundation.sql:ro'), 'compose migration mount missing');
assert(compose.includes('06-verify-narrative-foundation.sql:/opt/drama/06-verify-narrative-foundation.sql:ro'), 'compose verification mount missing');

const schemaDir = path.join(root, 'contracts', 'json-schema');
const schemas = new Map();
for (const file of fs.readdirSync(schemaDir).filter((name) => name.endsWith('.json'))) {
  const parsed = JSON.parse(fs.readFileSync(path.join(schemaDir, file), 'utf8'));
  assert(parsed.$schema === 'https://json-schema.org/draft/2020-12/schema', `${file}: wrong JSON Schema draft`);
  assert(parsed.$id && parsed.title, `${file}: $id/title required`);
  schemas.set(file, parsed);
}

function valueType(value) {
  if (value === null) return 'null';
  if (Array.isArray(value)) return 'array';
  if (Number.isInteger(value)) return 'integer';
  if (typeof value === 'number') return 'number';
  return typeof value;
}

function resolveLocal(rootSchema, ref) {
  assert(ref.startsWith('#/'), `unsupported fixture $ref ${ref}`);
  return ref.slice(2).split('/').reduce((value, part) => value[part.replace(/~1/g, '/').replace(/~0/g, '~')], rootSchema);
}

function validate(schema, value, rootSchema, at = '$') {
  if (schema === true) return [];
  if (schema === false) return [`${at}: forbidden by schema`];
  if (schema.$ref) return validate(resolveLocal(rootSchema, schema.$ref), value, rootSchema, at);
  let errors = [];
  if (schema.allOf) for (const child of schema.allOf) errors.push(...validate(child, value, rootSchema, at));
  if (schema.anyOf && !schema.anyOf.some((child) => validate(child, value, rootSchema, at).length === 0)) errors.push(`${at}: anyOf failed`);
  if (schema.oneOf) {
    const matches = schema.oneOf.filter((child) => validate(child, value, rootSchema, at).length === 0).length;
    if (matches !== 1) errors.push(`${at}: oneOf matched ${matches}`);
  }
  if (schema.if) {
    const conditionMatches = validate(schema.if, value, rootSchema, at).length === 0;
    if (conditionMatches && schema.then) errors.push(...validate(schema.then, value, rootSchema, at));
    if (!conditionMatches && schema.else) errors.push(...validate(schema.else, value, rootSchema, at));
  }
  if (Object.prototype.hasOwnProperty.call(schema, 'const') && JSON.stringify(value) !== JSON.stringify(schema.const)) errors.push(`${at}: const mismatch`);
  if (schema.enum && !schema.enum.some((item) => JSON.stringify(item) === JSON.stringify(value))) errors.push(`${at}: enum mismatch`);
  if (schema.type) {
    const allowed = Array.isArray(schema.type) ? schema.type : [schema.type];
    const actual = valueType(value);
    if (!allowed.includes(actual) && !(actual === 'integer' && allowed.includes('number'))) return [...errors, `${at}: expected ${allowed.join('|')}, got ${actual}`];
  }
  if (typeof value === 'string') {
    if (schema.minLength !== undefined && [...value].length < schema.minLength) errors.push(`${at}: minLength`);
    if (schema.maxLength !== undefined && [...value].length > schema.maxLength) errors.push(`${at}: maxLength`);
    if (schema.pattern && !new RegExp(schema.pattern).test(value)) errors.push(`${at}: pattern`);
    if (schema.format === 'uuid' && !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value)) errors.push(`${at}: uuid format`);
    if (schema.format === 'date-time' && Number.isNaN(Date.parse(value))) errors.push(`${at}: date-time format`);
  }
  if (typeof value === 'number') {
    if (schema.minimum !== undefined && value < schema.minimum) errors.push(`${at}: minimum`);
    if (schema.maximum !== undefined && value > schema.maximum) errors.push(`${at}: maximum`);
  }
  if (Array.isArray(value)) {
    if (schema.minItems !== undefined && value.length < schema.minItems) errors.push(`${at}: minItems`);
    if (schema.maxItems !== undefined && value.length > schema.maxItems) errors.push(`${at}: maxItems`);
    if (schema.uniqueItems && new Set(value.map((item) => JSON.stringify(item))).size !== value.length) errors.push(`${at}: uniqueItems`);
    if (schema.items) value.forEach((item, index) => errors.push(...validate(schema.items, item, rootSchema, `${at}[${index}]`)));
  }
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    for (const key of schema.required || []) if (!Object.prototype.hasOwnProperty.call(value, key)) errors.push(`${at}: missing ${key}`);
    for (const [key, child] of Object.entries(schema.properties || {})) {
      if (Object.prototype.hasOwnProperty.call(value, key)) errors.push(...validate(child, value[key], rootSchema, `${at}.${key}`));
    }
    if (schema.additionalProperties === false) {
      for (const key of Object.keys(value)) if (!Object.prototype.hasOwnProperty.call(schema.properties || {}, key)) errors.push(`${at}: additional property ${key}`);
    }
  }
  return errors;
}

const validSpec = JSON.parse(fs.readFileSync(path.join(root, 'test-data', 'contracts', 'adaptation-spec.valid.json'), 'utf8'));
const emptyScope = structuredClone(validSpec);
emptyScope.scope.chapter_ids = [];
emptyScope.scope.story_arc_revision_ids = [];
assert(validate(schemas.get('adaptation-spec.v1.json'), emptyScope, schemas.get('adaptation-spec.v1.json')).length > 0, 'adaptation spec empty scope unexpectedly passed');
const untargetedHardRule = structuredClone(validSpec);
delete untargetedHardRule.rules[0].target_id;
assert(validate(schemas.get('adaptation-spec.v1.json'), untargetedHardRule, schemas.get('adaptation-spec.v1.json')).length > 0, 'targeted rule without target_id unexpectedly passed');
const invalidAttributeRule = structuredClone(validSpec);
invalidAttributeRule.rules[0].target_type = 'attribute';
invalidAttributeRule.rules[0].parameters = {};
assert(validate(schemas.get('adaptation-spec.v1.json'), invalidAttributeRule, schemas.get('adaptation-spec.v1.json')).length > 0, 'attribute rule without owner/path unexpectedly passed');
const fixtureMappings = [
  ['workflow-command.v2.json', 'workflow-command'],
  ['narrative-extraction.v1.json', 'narrative-extraction'],
  ['adaptation-spec.v1.json', 'adaptation-spec'],
  ['compiler-plan.v1.json', 'compiler-plan'],
  ['worker-execution.v1.json', 'worker-execution']
];
for (const [schemaFile, fixtureBase] of fixtureMappings) {
  const schema = schemas.get(schemaFile);
  for (const expectation of ['valid', 'invalid']) {
    const fixtureFile = path.join(root, 'test-data', 'contracts', `${fixtureBase}.${expectation}.json`);
    const fixture = JSON.parse(fs.readFileSync(fixtureFile, 'utf8'));
    const errors = validate(schema, fixture, schema);
    if (expectation === 'valid') assert(errors.length === 0, `${fixtureBase}.valid failed: ${errors.slice(0, 5).join('; ')}`);
    else assert(errors.length > 0, `${fixtureBase}.invalid unexpectedly passed`);
  }
}

const openapi = read('contracts/openapi/narrative-api.v2.yaml');
for (const marker of [
  'openapi: 3.1.0', '/source-works:', '/source-versions/{source_version_id}/imports:',
  '/source-versions/{source_version_id}/chapters:batch:', '/adaptation-projects:',
  '/adaptation-projects/{project_id}/compiler-runs:', '/operations/{operation_id}:',
  '/operations:claim:', '/operations/{operation_id}:heartbeat:',
  '/operations/{operation_id}:checkpoint:', '/operations/{operation_id}:finish:',
  '/artifacts/{artifact_id}/lineage:', 'Idempotency-Key', 'If-Match',
  '../json-schema/adaptation-spec.v1.json'
]) assert(openapi.includes(marker), `OpenAPI contract missing ${marker}`);
for (const match of openapi.matchAll(/\$ref:\s+(\.\.\/json-schema\/[^\s]+)/g)) {
  assert(fs.existsSync(path.resolve(root, 'contracts', 'openapi', match[1])), `OpenAPI external ref missing: ${match[1]}`);
}

const changedContent = [sql, verifySQL, openapi, ...schemas.values()].map((value) => typeof value === 'string' ? value : JSON.stringify(value)).join('\n');
assert(!/(?:sk|api|pat|ghp)_[A-Za-z0-9_-]{20,}/.test(changedContent), 'possible real secret in Phase 1 contracts');
assert(!/Bearer\s+[A-Za-z0-9._~-]{20,}/.test(changedContent), 'literal bearer token in Phase 1 contracts');

console.log(`PASS Phase 1 static validation: ${requiredTables.length} tables, ${schemas.size} JSON Schemas, ${fixtureMappings.length * 2} contract fixtures`);
