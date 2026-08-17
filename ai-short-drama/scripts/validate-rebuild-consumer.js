'use strict';

const fs = require('node:fs');
const assert = require('node:assert/strict');
const read = (file) => fs.readFileSync(file, 'utf8');
const migration = read('database/33-rebuild-consumer-closure.sql');
const renderIdentityMigration = read('database/34-render-artifact-version-identity.sql');
const worker = read('scripts/media-worker/rebuild-consumer.js');
const host = read('scripts/media-worker/worker.js');
const localEdit = read('cms/backend/internal/store/local_edit.go');
const shotEditor = read('cms/backend/internal/store/shot_editor.go');
const exportStore = read('cms/backend/internal/store/professional_export.go');
const runner = read('scripts/run-phase5-acceptance.js');
const compose = read('docker-compose.yml');

for (const marker of ['FOR UPDATE SKIP LOCKED', 'lease_expires_at', 'attempt_count', 'max_attempts',
  'REBUILD_PUBLICATION_REQUIRED', 'rebuild_provider_executions', 'rebuild_publications',
  'trg_prepare_incremental_rebuild_task', 'local_conformance', 'publish_render_artifact_successors']) {
  assert(migration.includes(marker), `migration rebuild marker missing: ${marker}`);
}
for (const marker of ["convert_to('episode_master:'||master.master_id", "convert_to('edit_timeline:'||NEW.timeline_id",
  'render-artifact-version-identity-v1-20260817']) {
  assert(renderIdentityMigration.includes(marker), `render artifact identity marker missing: ${marker}`);
}
assert(!renderIdentityMigration.includes("master_artifact_id:='artifact_master_'||substr(master.content_hash"),
  'master artifact identity still depends on content hash');
for (const marker of ['regenerate_voice', 'update_subtitle', 'regenerate_image', 'regenerate_video',
  'update_continuity', 'recompose_timeline', 'validateProviderOutput', 'probeMediaOutput',
  'REBUILD_PROVIDER_UNSUPPORTED', 'REBUILD_OUTPUT_HASH_MISMATCH', 'publishSuccess', 'persistFailure']) {
  assert(worker.includes(marker), `worker rebuild marker missing: ${marker}`);
}
assert(!worker.includes("else output = await localConformanceProvider"), 'provider router contains a silent conformance fallback');
assert(host.includes('rebuildConsumer.runOnce()') && host.includes("'/rebuild-tasks/claim-or-run'"),
  'media worker does not schedule and expose the rebuild consumer');
assert(host.includes('REBUILD_PROVIDER_ENDPOINTS_JSON') && compose.includes('REBUILD_PROVIDER_ENDPOINTS_JSON'),
  'external provider routing configuration is not wired end-to-end');
assert(localEdit.includes('this endpoint only accepts cancelled') && shotEditor.includes('this endpoint only accepts cancelled'),
  'public API can still forge rebuild execution success');
assert(exportStore.includes('rebuild_provenance'), 'professional export omits rebuild provenance');
assert(runner.includes('Generic rebuild consumer full delivery closure E2E') &&
  runner.includes('REBUILD_STATE_DATABASE_URL'), 'top-level acceptance does not execute the rebuild closure/state tests');
console.log('PASS generic rebuild consumer static/API/workflow acceptance');
