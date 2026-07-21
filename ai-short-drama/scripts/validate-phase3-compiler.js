'use strict';

const fs = require('fs');
const path = require('path');
const {execFileSync} = require('child_process');

const root = path.resolve(__dirname, '..');
const read = (name) => fs.readFileSync(path.join(root, name), 'utf8').replace(/^\uFEFF/, '');
const assert = (condition, message) => { if (!condition) throw new Error(`Phase 3 validation failed: ${message}`); };

const migration = read('database/07-adaptation-compiler-audit.sql');
assert(/^BEGIN;\s*$/m.test(migration) && /COMMIT;\s*$/.test(migration), '07 migration transaction boundary missing');
assert(/pg_advisory_xact_lock/.test(migration) && /schema_migrations/.test(migration), '07 migration lock/ledger missing');
assert(/adaptation-compiler-audit-v1-20260721/.test(migration), '07 checksum missing');
assert(!/\bDROP\s+(?:TABLE|COLUMN)\b|\bDELETE\s+FROM\b|\bTRUNCATE\b/i.test(migration), '07 migration is not additive-first');
for (const column of ['source_event_ids', 'source_chapter_ids', 'added_adaptation_content', 'merged_content', 'deviation_notes']) {
  assert(new RegExp(`ADD COLUMN IF NOT EXISTS ${column}`).test(migration), `audit column ${column} missing`);
}
assert(/Safe compatibility backfill/.test(migration), 'legacy episode audit backfill missing');

const verify = read('database/07-verify-adaptation-compiler.sql');
assert(verify.includes('episode source_event_ids disagree with normalized assignments'), 'assignment/audit verification missing');
assert(verify.includes("'PASS' AS result"), 'verification PASS result missing');
assert(read('database/bootstrap.sh').includes('/opt/drama/07-adaptation-compiler-audit.sql'), 'bootstrap does not apply 07');
const compose = read('docker-compose.yml');
assert(compose.includes('07-adaptation-compiler-audit.sql:/opt/drama/07-adaptation-compiler-audit.sql:ro'), '07 compose mount missing');
assert(compose.includes('07-verify-adaptation-compiler.sql:/opt/drama/07-verify-adaptation-compiler.sql:ro'), '07 verify mount missing');

const schema = JSON.parse(read('contracts/json-schema/compiler-plan.v2.json'));
const episodeRequired = schema.$defs.episode.required;
for (const field of ['source_event_ids', 'source_chapter_ids', 'added_adaptation_content', 'merged_content', 'deviation_notes']) {
  assert(episodeRequired.includes(field), `compiler-plan.v2 does not require ${field}`);
}
assert(schema.$defs.mergedContent.properties.source_event_ids.minItems === 2, 'merge audit must reference at least two events');
assert(schema.$defs.addedContent.properties.rule_ids.minItems === 1, 'added content must carry an authorizing rule');

const handler = read('cms/backend/internal/httpapi/v2_source.go');
for (const marker of ['/adaptation-projects/:projectID/compiler-runs', '/adaptation-plans/:adaptationPlanID', 'StartCompilerRun', 'GetAdaptationPlan']) {
  assert(handler.includes(marker), `CMS compiler marker ${marker} missing`);
}
const store = read('cms/backend/internal/store/v2_compiler.go');
for (const marker of ["'adaptation_compile','project'", "specStatus != \"active\"", 'irStatus != "published"', 'compiler_runs', 'compiler-plan.v2']) {
  assert(store.includes(marker), `compiler store contract ${marker} missing`);
}

const compiler = read('scripts/adaptation-compiler.js');
const orderedStages = [
  'source_scope_resolution', 'event_selection', 'prerequisite_ordering', 'event_compression_merge',
  'episode_allocation', 'character_state_validation', 'foreshadow_validation', 'duration_validation', 'reviewable_plan',
];
let cursor = -1;
for (const stage of orderedStages) {
  const next = compiler.indexOf(`'${stage}'`, cursor + 1);
  assert(next > cursor, `compiler stage ${stage} missing or out of order`);
  cursor = next;
}
assert(!/httpRequest|LITELLM|TEXT_ANALYSIS_MODEL|EPISODE_PLANNING_MODEL/i.test(compiler), 'constraint compiler must not call a generative model');
for (const marker of ['PREREQUISITE_CYCLE', 'CHARACTER_STATE_DISCONTINUITY', 'FORESHADOW_RESOLUTION_WITHOUT_PLANT', 'EPISODE_DURATION_EXCEEDED', 'MUST_PRESERVE_OUTSIDE_SCOPE']) {
  assert(compiler.includes(marker), `business validation ${marker} missing`);
}

const workflowPath = path.join(root, 'workflows', '04a-adaptation-compiler.json');
const workflow = JSON.parse(fs.readFileSync(workflowPath, 'utf8'));
assert(workflow.active === false, 'compiler workflow must remain inactive before deployment approval');
assert(workflow.settings?.saveDataErrorExecution === 'none' && workflow.settings?.saveDataSuccessExecution === 'none', 'workflow execution payload persistence must be disabled');
const nodeNames = workflow.nodes.map((node) => node.name);
for (const name of ['Claim and Load Frozen Compiler Inputs', 'Run Nine-stage Constraint Compiler', 'Validate compiler-plan.v2 and Business Audit', 'Checkpoint Validated Reviewable Plan', 'Atomic Publish Reviewable Episode Plan']) {
  assert(nodeNames.includes(name), `workflow node ${name} missing`);
}
const workflowText = JSON.stringify(workflow);
for (const marker of ['assert_operation_claim', 'compiler_checkpoints', 'artifact_dependencies', 'artifact_source_evidence', 'finish_operation']) {
  assert(workflowText.includes(marker), `atomic compiler persistence marker ${marker} missing`);
}
const generated = execFileSync(process.execPath, [path.join(__dirname, 'build-adaptation-compiler-workflow.js')], {encoding: 'utf8'});
assert(generated === fs.readFileSync(workflowPath, 'utf8'), 'generated workflow drifted from compiler source/builder');

const fixtures = fs.readdirSync(path.join(root, 'test-data')).filter((name) => name.startsWith('phase3-compiler-') && name.endsWith('.json'));
assert(fixtures.length === 3, `expected 3 compiler fixtures, found ${fixtures.length}`);

console.log(`PASS Phase 3 compiler validation: ${orderedStages.length} stages, 5 audit fields, ${fixtures.length} fixtures`);
