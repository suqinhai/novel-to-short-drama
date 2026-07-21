const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const read = (relative) => fs.readFileSync(path.join(root, relative), 'utf8').replace(/^\uFEFF/, '');
const assert = (condition, message) => {
  if (!condition) throw new Error(`Phase 2 validation failed: ${message}`);
};

const handler = read('cms/backend/internal/httpapi/v2_source.go');
for (const route of [
  '/source-works', '/source-versions/*resourcePath', '/operations/:operationID',
]) assert(handler.includes(route), `CMS v2 route ${route} missing`);
for (const contract of [
  'Idempotency-Key', 'If-Match', 'ETag', 'DisallowUnknownFields',
]) assert(handler.includes(contract), `CMS HTTP contract ${contract} missing`);

const sourceStore = read('cms/backend/internal/store/v2_source.go');
for (const marker of [
  'CreateSourceWork', 'CreateSourceVersion', 'ApplyImport', 'ReviseChapter',
  'PublishSourceVersion', 'StartIRRun', "'ir_extraction','ir_revision'",
  'INSERT INTO drama.narrative_ir_revisions',
]) assert(sourceStore.includes(marker), `CMS source store marker ${marker} missing`);

const sourceViews = [
  'cms/frontend/src/views/SourceWorksView.vue',
  'cms/frontend/src/views/SourceWorkDetailView.vue',
  'cms/frontend/src/views/SourceVersionView.vue',
  'cms/frontend/src/views/AdaptationScopeView.vue',
  'cms/frontend/src/components/OperationTracker.vue',
  'cms/frontend/src/services/narrativeApi.js',
];
for (const file of sourceViews) assert(fs.existsSync(path.join(root, file)), `${file} missing`);
const narrativeApi = read('cms/frontend/src/services/narrativeApi.js');
for (const marker of ['Idempotency-Key', 'If-Match', 'ETAG_REQUIRED', '/source-works', '/operations/']) {
  assert(narrativeApi.includes(marker), `frontend API marker ${marker} missing`);
}
const adaptationView = read('cms/frontend/src/views/AdaptationScopeView.vue');
assert(adaptationView.includes("VITE_ADAPTATION_SPEC_WRITES_ENABLED === 'true'"), 'Adaptation Spec write feature gate missing');
assert(adaptationView.includes("mode: 'chapters_only'"), 'safe chapters_only fallback missing');

const workflowFiles = [
  'workflows/02a-narrative-ir-extract.json',
  'workflows/02b-narrative-ir-reconcile.json',
];
for (const file of workflowFiles) {
  const workflow = JSON.parse(read(file));
  assert(workflow.active === false, `${file} must remain inactive until deployment approval`);
  assert(workflow.settings?.saveDataErrorExecution === 'none', `${file} must not persist raw error execution data`);
  assert(workflow.settings?.saveDataSuccessExecution === 'none', `${file} must not persist raw success execution data`);
}

const extract = read(workflowFiles[0]);
for (const marker of [
  'IR_WINDOW_MAX_CODEPOINTS', 'LIMIT 1', 'chapter_ids', 'JSON Schema Validate',
  'Business Validate Provenance and References', 'checkpoint_operation',
]) assert(extract.includes(marker), `bounded extraction marker ${marker} missing`);
const reconcile = read(workflowFiles[1]);
for (const marker of [
  'assert_operation_claim', 'Deterministic Cross-window Reconcile',
  'Atomic Publish Narrative IR', 'finish_operation', 'Atomically Quarantine Failed Batch',
]) assert(reconcile.includes(marker), `reconcile marker ${marker} missing`);

const fixtures = fs.readdirSync(path.join(root, 'test-data')).filter((name) => name.startsWith('phase2-ir-') && name.endsWith('.json'));
assert(fixtures.length === 7, `expected 7 Narrative IR fixtures, found ${fixtures.length}`);

console.log(`PASS Phase 2 integration validation: ${sourceViews.length} frontend files, ${workflowFiles.length} workflows, ${fixtures.length} IR fixtures`);
