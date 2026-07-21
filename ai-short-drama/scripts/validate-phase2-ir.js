const crypto = require('crypto');
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const extractionSchemaPath = path.join(root, 'contracts', 'json-schema', 'narrative-extraction.v1.json');
const workflowPaths = [
  path.join(root, 'workflows', '02a-narrative-ir-extract.json'),
  path.join(root, 'workflows', '02b-narrative-ir-reconcile.json'),
];
const failures = [];

function fail(message) { failures.push(message); }
function readJson(file) {
  try { return JSON.parse(fs.readFileSync(file, 'utf8').replace(/^\uFEFF/, '')); }
  catch (error) { fail(`${path.relative(root, file)}: invalid JSON: ${error.message}`); return null; }
}

const schema = readJson(extractionSchemaPath);

function resolveRef(ref) {
  if (!ref.startsWith('#/')) throw new Error(`unsupported external $ref ${ref}`);
  return ref.slice(2).split('/').reduce((value, token) => value[token.replace(/~1/g, '/').replace(/~0/g, '~')], schema);
}

function schemaErrors(value, rule, location = '$') {
  if (rule === true) return [];
  if (!rule || typeof rule !== 'object') return [`${location}: invalid schema rule`];
  if (rule.$ref) return schemaErrors(value, resolveRef(rule.$ref), location);
  let errors = [];
  if (rule.allOf) for (const child of rule.allOf) errors.push(...schemaErrors(value, child, location));
  if (rule.anyOf && !rule.anyOf.some((child) => schemaErrors(value, child, location).length === 0)) errors.push(`${location}: does not match anyOf`);
  if (rule.oneOf && rule.oneOf.filter((child) => schemaErrors(value, child, location).length === 0).length !== 1) errors.push(`${location}: does not match exactly one oneOf branch`);
  if (Object.prototype.hasOwnProperty.call(rule, 'const') && value !== rule.const) errors.push(`${location}: expected const ${JSON.stringify(rule.const)}`);
  if (rule.enum && !rule.enum.some((item) => item === value)) errors.push(`${location}: value is outside enum`);
  const types = rule.type === undefined ? [] : (Array.isArray(rule.type) ? rule.type : [rule.type]);
  const matchesType = (type) => type === 'null' ? value === null
    : type === 'array' ? Array.isArray(value)
      : type === 'object' ? value !== null && typeof value === 'object' && !Array.isArray(value)
        : type === 'integer' ? Number.isInteger(value)
          : type === 'number' ? typeof value === 'number' && Number.isFinite(value)
            : typeof value === type;
  if (types.length && !types.some(matchesType)) return [...errors, `${location}: wrong type`];
  if (typeof value === 'string') {
    if (rule.minLength !== undefined && [...value].length < rule.minLength) errors.push(`${location}: shorter than minLength`);
    if (rule.maxLength !== undefined && [...value].length > rule.maxLength) errors.push(`${location}: longer than maxLength`);
    if (rule.pattern && !(new RegExp(rule.pattern)).test(value)) errors.push(`${location}: pattern mismatch`);
  }
  if (typeof value === 'number') {
    if (rule.minimum !== undefined && value < rule.minimum) errors.push(`${location}: below minimum`);
    if (rule.maximum !== undefined && value > rule.maximum) errors.push(`${location}: above maximum`);
  }
  if (Array.isArray(value)) {
    if (rule.minItems !== undefined && value.length < rule.minItems) errors.push(`${location}: fewer than minItems`);
    if (rule.maxItems !== undefined && value.length > rule.maxItems) errors.push(`${location}: more than maxItems`);
    if (rule.uniqueItems && new Set(value.map((item) => JSON.stringify(item))).size !== value.length) errors.push(`${location}: duplicate array items`);
    if (rule.items) value.forEach((item, index) => { errors.push(...schemaErrors(item, rule.items, `${location}[${index}]`)); });
  }
  if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
    for (const key of rule.required || []) if (!(key in value)) errors.push(`${location}: missing ${key}`);
    if (rule.additionalProperties === false && rule.properties) {
      for (const key of Object.keys(value)) if (!(key in rule.properties)) errors.push(`${location}: additional property ${key}`);
    }
    for (const [key, child] of Object.entries(rule.properties || {})) if (key in value) errors.push(...schemaErrors(value[key], child, `${location}.${key}`));
  }
  return errors;
}

function sha256(value) { return crypto.createHash('sha256').update(value, 'utf8').digest('hex'); }

function businessErrors(fixture) {
  const out = fixture.extraction;
  const window = fixture.window;
  const errors = [];
  const entities = new Map();
  const facts = new Map();
  const events = new Map();
  const arcs = new Set();
  const sources = [];
  if (Array.isArray(fixture.chapter_ids) && fixture.chapter_ids.length && !fixture.chapter_ids.includes(window.chapter_id)) {
    errors.push('CHAPTER_OUTSIDE_SCOPE: window chapter is not in checkpoint chapter_ids');
  }
  if (fixture.checkpoint) {
    const selected = fixture.checkpoint.selected_chapter_ids || [];
    const checkpointWindows = [...(fixture.checkpoint.windows || [])].sort((a, b) => a.window.ordinal - b.window.ordinal || a.window.start_codepoint - b.window.start_codepoint);
    const covered = new Set(); const totals = new Map(); let previous = null;
    for (const item of checkpointWindows) {
      const current = item.window;
      if (!Number.isInteger(current.chapter_total_codepoints) || current.chapter_total_codepoints < 1 || current.end_codepoint > current.chapter_total_codepoints) errors.push('CHAPTER_TOTAL_INVALID');
      if (totals.has(current.chapter_id) && totals.get(current.chapter_id) !== current.chapter_total_codepoints) errors.push('CHAPTER_TOTAL_MISMATCH');
      totals.set(current.chapter_id, current.chapter_total_codepoints);
      if (previous && current.chapter_id === previous.chapter_id && current.start_codepoint !== previous.end_codepoint) errors.push('WINDOW_COVERAGE_GAP');
      if (previous && current.chapter_id !== previous.chapter_id && previous.end_codepoint !== previous.chapter_total_codepoints) errors.push('CHAPTER_TAIL_MISSING');
      if ((!previous || current.chapter_id !== previous.chapter_id) && current.start_codepoint !== 0) errors.push('WINDOW_COVERAGE_GAP');
      covered.add(current.chapter_id); previous = current;
    }
    if (previous && previous.end_codepoint !== previous.chapter_total_codepoints) errors.push('CHAPTER_TAIL_MISSING');
    const expected = new Set(selected);
    if (covered.size !== expected.size || [...expected].some((id) => !covered.has(id))) errors.push('SELECTED_CHAPTER_COVERAGE_MISMATCH');
  }
  const register = (map, id, code) => { if (map.has(id)) errors.push(`${code}: ${id}`); else map.set(id, true); };
  for (const entity of out.entities || []) { register(entities, entity.local_id, 'DUPLICATE_ENTITY_LOCAL_ID'); sources.push(entity.source); }
  for (const fact of out.facts || []) {
    register(facts, fact.local_id, 'DUPLICATE_FACT_LOCAL_ID');
    if (fact.fact_kind === 'event') events.set(fact.local_id, fact);
    sources.push(fact.source, ...(fact.supporting_sources || []));
    if (fact.event) {
      if (fact.event.location_entity_local_id && !entities.has(fact.event.location_entity_local_id)) errors.push(`UNKNOWN_ENTITY_REFERENCE: ${fact.event.location_entity_local_id}`);
      for (const participant of fact.event.participants || []) {
        if (!entities.has(participant.entity_local_id)) errors.push(`UNKNOWN_ENTITY_REFERENCE: ${participant.entity_local_id}`);
        sources.push(participant.source);
      }
    }
  }
  for (const fact of out.facts || []) {
    const state = fact.character_state;
    if (state) {
      if (!entities.has(state.character_entity_local_id)) errors.push(`UNKNOWN_ENTITY_REFERENCE: ${state.character_entity_local_id}`);
      if (state.trigger_event_local_id && !events.has(state.trigger_event_local_id)) errors.push(`UNKNOWN_EVENT_REFERENCE: ${state.trigger_event_local_id}`);
    }
    const timeline = fact.timeline;
    if (timeline) {
      if (timeline.subject_entity_local_id && !entities.has(timeline.subject_entity_local_id)) errors.push(`UNKNOWN_ENTITY_REFERENCE: ${timeline.subject_entity_local_id}`);
      if (timeline.event_local_id && !events.has(timeline.event_local_id)) errors.push(`UNKNOWN_EVENT_REFERENCE: ${timeline.event_local_id}`);
    }
    const foreshadowing = fact.foreshadowing;
    if (foreshadowing?.event_local_id && !events.has(foreshadowing.event_local_id)) errors.push(`UNKNOWN_EVENT_REFERENCE: ${foreshadowing.event_local_id}`);
  }
  const causal = new Map();
  for (const relation of out.event_relations || []) {
    if (!events.has(relation.from_event_local_id)) errors.push(`UNKNOWN_EVENT_REFERENCE: ${relation.from_event_local_id}`);
    if (!events.has(relation.to_event_local_id)) errors.push(`UNKNOWN_EVENT_REFERENCE: ${relation.to_event_local_id}`);
    if (relation.from_event_local_id === relation.to_event_local_id) errors.push(`SELF_EVENT_RELATION: ${relation.from_event_local_id}`);
    sources.push(relation.source);
    if (['causes', 'enables'].includes(relation.relation_type)) {
      if (!causal.has(relation.from_event_local_id)) causal.set(relation.from_event_local_id, []);
      causal.get(relation.from_event_local_id).push(relation.to_event_local_id);
    }
    const from = events.get(relation.from_event_local_id)?.event?.narrative_order;
    const to = events.get(relation.to_event_local_id)?.event?.narrative_order;
    if (relation.relation_type === 'before' && from !== undefined && to !== undefined && from >= to) errors.push(`TIMELINE_ORDER_CONFLICT: ${relation.from_event_local_id}`);
    if (relation.relation_type === 'after' && from !== undefined && to !== undefined && from <= to) errors.push(`TIMELINE_ORDER_CONFLICT: ${relation.from_event_local_id}`);
  }
  const visiting = new Set(); const visited = new Set();
  function visit(node) {
    if (visiting.has(node)) return true;
    if (visited.has(node)) return false;
    visiting.add(node);
    for (const next of causal.get(node) || []) if (visit(next)) return true;
    visiting.delete(node); visited.add(node); return false;
  }
  for (const node of causal.keys()) if (visit(node)) { errors.push('CAUSAL_CYCLE: causes/enables graph must be acyclic'); break; }
  const lifecycleRank = { planted: 0, reinforced: 1, partially_resolved: 2, resolved: 3, abandoned: 3 };
  const threads = new Map();
  for (const fact of (out.facts || []).filter((item) => item.foreshadowing)) {
    const value = fact.foreshadowing;
    const prior = threads.get(value.thread_local_id);
    if (prior && value.occurrence_order > prior.order && lifecycleRank[value.stage] < lifecycleRank[prior.stage]) errors.push(`FORESHADOW_LIFECYCLE_REGRESSION: ${value.thread_local_id}`);
    if (!prior || value.occurrence_order >= prior.order) threads.set(value.thread_local_id, { order: value.occurrence_order, stage: value.stage });
  }
  for (const arc of out.story_arcs || []) {
    if (arcs.has(arc.local_id)) errors.push(`DUPLICATE_ARC_LOCAL_ID: ${arc.local_id}`); arcs.add(arc.local_id);
    for (const eventId of arc.event_local_ids || []) if (!events.has(eventId)) errors.push(`UNKNOWN_EVENT_REFERENCE: ${eventId}`);
    sources.push(arc.source);
  }
  const chapterChars = Array.from(window.content || '');
  const baseCp = Number(window.start_codepoint || 0);
  const baseBytes = Number(window.start_utf8_byte || 0);
  for (const source of sources.filter(Boolean)) {
    if (source.source_version_id !== window.source_version_id || source.chapter_id !== window.chapter_id || source.chapter_revision_id !== window.chapter_revision_id) {
      errors.push('SOURCE_ID_MISMATCH: source must belong to the bounded window'); continue;
    }
    const start = source.start_codepoint - baseCp; const end = source.end_codepoint - baseCp;
    if (start < 0 || end > chapterChars.length || start >= end) { errors.push('SOURCE_SPAN_OUT_OF_WINDOW: invalid codepoint range'); continue; }
    const quote = chapterChars.slice(start, end).join('');
    const startBytes = baseBytes + Buffer.byteLength(chapterChars.slice(0, start).join(''), 'utf8');
    const endBytes = baseBytes + Buffer.byteLength(chapterChars.slice(0, end).join(''), 'utf8');
    if (quote !== source.quote || startBytes !== source.start_utf8_byte || endBytes !== source.end_utf8_byte || sha256(quote) !== source.quote_hash) errors.push('SOURCE_SPAN_MISMATCH: quote, byte/codepoint range or hash is incorrect');
  }
  return errors;
}

const fixtures = fs.readdirSync(path.join(root, 'test-data'))
  .filter((file) => /^phase2-ir-.*\.json$/.test(file)).sort();
if (fixtures.length < 4) fail('expected at least four phase2-ir fixtures');
for (const name of fixtures) {
  const fixture = readJson(path.join(root, 'test-data', name));
  if (!fixture) continue;
  const errors = [...schemaErrors(fixture.extraction, schema), ...businessErrors(fixture)];
  if (fixture.expected_valid && errors.length) fail(`${name}: expected valid, got ${errors.join('; ')}`);
  if (!fixture.expected_valid && !errors.some((error) => error.includes(fixture.expected_error))) fail(`${name}: expected ${fixture.expected_error}, got ${errors.join('; ') || 'no error'}`);
}

function requireNode(workflow, name) {
  const node = workflow.nodes.find((item) => item.name === name);
  if (!node) fail(`${workflow.name}: missing node ${name}`);
  return node;
}
function connected(workflow, from, to) {
  return (workflow.connections[from]?.main || []).flat().some((edge) => edge.node === to);
}
for (const workflowPath of workflowPaths) {
  const workflow = readJson(workflowPath);
  if (!workflow) continue;
  const rel = path.relative(root, workflowPath).replace(/\\/g, '/');
  if (workflow.active !== false) fail(`${rel}: workflow must remain inactive`);
  const text = JSON.stringify(workflow);
  for (const forbidden of ['raw_response', 'provider_response', 'request_body', 'response_body']) if (text.includes(forbidden)) fail(`${rel}: forbidden provider payload key ${forbidden}`);
  for (const node of workflow.nodes.filter((item) => item.type === 'n8n-nodes-base.postgres')) {
    if (!node.parameters?.options?.queryReplacement) fail(`${rel}/${node.name}: PostgreSQL query must use queryReplacement`);
  }
}

const extract = readJson(workflowPaths[0]);
if (extract) {
  for (const name of ['Claim IR Operation', 'Load One Bounded Chapter Window', 'Call Narrative Extraction Model', 'JSON Schema Validate', 'Business Validate Provenance and References', 'Checkpoint Validated Window']) requireNode(extract, name);
  if (!connected(extract, 'Parse Model JSON', 'JSON Schema Validate') || !connected(extract, 'JSON Schema Validate', 'Business Validate Provenance and References')) fail('02a: JSON Schema validation must precede business validation');
  const text = JSON.stringify(extract);
  for (const marker of ['IR_WINDOW_MAX_CODEPOINTS', 'narrative-extraction.v1', 'json_schema', 'claim_operation', 'checkpoint_operation', "checkpoint_data->'chapter_ids'", 'last_selected_ordinal', 'chapter_total_codepoints', 'selected_chapter_ids']) if (!text.includes(marker)) fail(`02a: missing ${marker}`);
  if (!/LIMIT 1/i.test(text)) fail('02a: chapter selection must be bounded to one chapter');
}

const reconcile = readJson(workflowPaths[1]);
if (reconcile) {
  for (const name of ['Assert Fenced Claim and Load Windows', 'Revalidate All Windows', 'Deterministic Cross-window Reconcile', 'Atomic Publish Narrative IR', 'Published Result']) requireNode(reconcile, name);
  const publish = requireNode(reconcile, 'Atomic Publish Narrative IR');
  const sql = publish?.parameters?.query || '';
  for (const marker of ['assert_operation_claim', 'source_spans', 'narrative_entity_revisions', 'narrative_fact_revisions', 'narrative_event_revisions', 'event_participants', 'event_relations', 'character_state_changes', 'timeline_facts', 'foreshadow_occurrences', 'story_arc_revisions', 'finish_operation']) if (!sql.includes(marker)) fail(`02b atomic publish: missing ${marker}`);
  if (!/WITH\s+claim\s+AS/i.test(sql)) fail('02b: fenced claim and publication must share one SQL statement');
  if ((reconcile.nodes || []).filter((node) => node.type === 'n8n-nodes-base.postgres' && /INSERT INTO drama\.narrative_/i.test(node.parameters?.query || '')).length !== 1) fail('02b: Narrative IR publication must use one atomic PostgreSQL node');
  const reconcileText = JSON.stringify(reconcile);
  for (const marker of ['CHAPTER_TAIL_MISSING', 'CHAPTER_TOTAL_MISMATCH', 'SELECTED_CHAPTER_COVERAGE_MISMATCH']) if (!reconcileText.includes(marker)) fail(`02b coverage validation: missing ${marker}`);
}

if (failures.length) {
  failures.forEach((message) => console.error(`ERROR ${message}`));
  console.error(`FAILED Phase 2 Narrative IR validation: ${failures.length} error(s)`);
  process.exitCode = 1;
} else {
  console.log(`PASS Phase 2 Narrative IR validation: ${workflowPaths.length} workflows, ${fixtures.length} fixtures`);
}
