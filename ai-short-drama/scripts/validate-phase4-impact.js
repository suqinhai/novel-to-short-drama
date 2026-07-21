'use strict';

const fs = require('fs');
const path = require('path');
const root = path.resolve(__dirname, '..');
const read = (name) => fs.readFileSync(path.join(root, name), 'utf8').replace(/^\uFEFF/, '');
const assert = (value, message) => { if (!value) throw new Error(`Phase 4 impact validation failed: ${message}`); };

const migration = read('database/08-chapter-impact-analysis.sql');
assert(/^BEGIN;$/m.test(migration) && /COMMIT;\s*$/.test(migration), 'migration transaction boundary missing');
assert(/pg_advisory_xact_lock/.test(migration) && /chapter-impact-analysis-v1-20260721/.test(migration), 'migration lock or ledger checksum missing');
assert(!/\bDROP\s+(?:TABLE|COLUMN)\b|\bTRUNCATE\b|\bDELETE\s+FROM\b/i.test(migration), 'migration is not additive-first');
for (const marker of ['revision_scope', 'base_ir_revision_id', 'changed_chapter_ids', 'regeneration_requests',
  'guard_incremental_ir_publish', 'enqueue_incremental_impact', 'analyze_chapter_impact']) {
  assert(migration.includes(marker), `migration marker ${marker} missing`);
}
for (const marker of ["details->>'subtype'='event'", "details->>'subtype'='character_state'", "details->>'subtype'='story_arc'",
  "validity_status='stale'", 'artifact_dependencies', 'episode_event_assignments', 'source_chapter_ids']) {
  assert(migration.includes(marker), `exact impact selector ${marker} missing`);
}
assert(!/vector|embedding|cosine/i.test(migration), 'impact analysis must not use vector similarity');

const sourceStore = read('cms/backend/internal/store/v2_source.go');
for (const marker of ['chapter-impact-ir:', 'revision_scope', 'base_ir_revision_id', 'changed_chapter_ids']) {
  assert(sourceStore.includes(marker), `automatic incremental extraction marker ${marker} missing`);
}
const impactStore = read('cms/backend/internal/store/v2_impact.go');
for (const marker of ['GetProjectImpact', 'CreateRegenerationRequest', "artifact.validity_status='stale'", 'invalidation_impacts']) {
  assert(impactStore.includes(marker), `CMS impact contract ${marker} missing`);
}

const workflow = JSON.parse(read('workflows/02c-chapter-impact-analysis.json'));
assert(workflow.active === false, 'workflow must remain inactive before deployment');
assert(workflow.settings.saveDataErrorExecution === 'none' && workflow.settings.saveDataSuccessExecution === 'none', 'workflow execution payload persistence must be disabled');
const workflowText = JSON.stringify(workflow);
for (const marker of ['claim_operation', 'invalidation_scan', 'analyze_chapter_impact']) {
  assert(workflowText.includes(marker), `workflow marker ${marker} missing`);
}
const reconcileWorkflow = read('workflows/02b-narrative-ir-reconcile.json');
for (const marker of ["ctx.revision_scope='full'", "is_current=(ctx.revision_scope='full')"]) {
  assert(reconcileWorkflow.includes(marker), `02b incremental publish guard ${marker} missing`);
}

const api = read('contracts/openapi/narrative-api.v2.yaml');
for (const marker of ['/adaptation-projects/{project_id}/impact:', 'createRegenerationRequest', 'changed_character_states:', 'affected_story_arcs:']) {
  assert(api.includes(marker), `OpenAPI marker ${marker} missing`);
}
const ui = read('cms/frontend/src/views/ImpactAnalysisView.vue');
for (const marker of ['selectedArtifactIds', 'requestRegeneration', '不会覆盖现有审核产物', '不以向量相似度']) {
  assert(ui.includes(marker), `CMS impact marker ${marker} missing`);
}
assert(read('database/bootstrap.sh').includes('/opt/drama/08-chapter-impact-analysis.sql'), 'bootstrap migration missing');
assert(read('docker-compose.yml').includes('08-chapter-impact-analysis.sql:/opt/drama/08-chapter-impact-analysis.sql:ro'), 'compose migration mount missing');
assert(read('test-data/phase4-chapter-impact-e2e.sql').includes("'PASS' result"), 'database E2E fixture missing');

console.log('PASS Phase 4 chapter impact static validation: incremental IR, exact diff, stale lineage and explicit regeneration decision');
