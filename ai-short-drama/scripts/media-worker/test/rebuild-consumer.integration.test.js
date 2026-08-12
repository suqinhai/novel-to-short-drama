'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fs = require('node:fs');
const fsp = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');
const { Pool } = require('pg');
const { RebuildConsumer } = require('../rebuild-consumer');

const databaseURL = process.env.REBUILD_E2E_DATABASE_URL;

async function fileHash(filePath) {
  return crypto.createHash('sha256').update(await fsp.readFile(filePath)).digest('hex');
}

test('impact task is claimed, executed, validated and atomically published', { skip: !databaseURL }, async (t) => {
  const pool = new Pool({ connectionString: databaseURL, max: 8 });
  const storagePath = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-consumer-e2e-'));
  t.after(async () => {
    await pool.end();
    await fsp.rm(storagePath, { recursive: true, force: true });
  });
  const beforeUnrelated = await pool.query(`SELECT content_hash,validity_status,is_current
    FROM drama.artifacts WHERE artifact_id='artifact_phase5_sound_bgm'`);
  const oldExport = await pool.query(`SELECT export_id,status FROM drama.professional_export_jobs
    WHERE project_id='p_phase1_legacy' ORDER BY created_at LIMIT 1`);
  const oldQA = await pool.query(`SELECT gate_run_id,status FROM drama.quality_gate_runs
    WHERE project_id='p_phase1_legacy' AND target_timeline_id='timeline_phase5_v1' ORDER BY created_at DESC LIMIT 1`);
  if (process.env.REBUILD_E2E_EXPECT_IMPACT === 'true') {
	if (oldExport.rowCount) assert.equal(oldExport.rows[0].status, 'stale', 'upstream impact must stale an existing old export before rebuilding');
	assert.equal((await pool.query(`SELECT validity_status FROM drama.artifacts
	  WHERE artifact_id='artifact_phase5_timeline'`)).rows[0]?.validity_status, 'stale',
	'upstream impact must make the old timeline stale before rebuilding');
  }
  assert.notEqual(oldQA.rows[0]?.status, 'approved', 'upstream impact must revoke/supersede old QA');

  const consumer = new RebuildConsumer({
    pool,
    storagePath,
    publicBaseUrl: 'http://local.invalid/storage',
    workerId: 'rebuild-e2e-worker',
    leaseSeconds: 30,
    heartbeatSeconds: 2,
    providerTimeoutSeconds: 30,
    retryDelaySeconds: 1,
    localConformanceEnabled: true,
  });
  const result = await consumer.runOnce();
  assert.match(result.successorArtifactId, /^artifact_rb_[0-9a-f]{28}$/);

  const task = await pool.query(`SELECT * FROM drama.incremental_rebuild_tasks
    WHERE regeneration_request_item_id='regeni_rebuild_consumer_e2e'`);
  assert.equal(task.rows[0].status, 'succeeded');
  assert.equal(task.rows[0].successor_artifact_id, result.successorArtifactId);
  assert.equal(task.rows[0].claim_token, null);
  assert.equal(task.rows[0].lease_owner, null);
  assert.equal(task.rows[0].attempt_count, 1);
  const output = task.rows[0].output;
  assert.equal(output.schema_version, 'rebuild-provider-output.v1');
  assert.equal(output.provider, 'local_conformance');
  assert.equal(output.provenance.execution_mode, 'local_conformance');
  assert.equal(output.provenance.source_change_set_id,
    task.rows[0].input.source_change_set_id);
  assert.equal(output.artifact.size_bytes, fs.statSync(output.artifact.storage_path).size);
  assert.equal(output.artifact.content_hash, await fileHash(output.artifact.storage_path));
  assert.doesNotThrow(() => JSON.parse(fs.readFileSync(output.artifact.storage_path, 'utf8')));

  const events = await pool.query(`SELECT event_type FROM drama.rebuild_task_events
    WHERE rebuild_task_id=$1 ORDER BY created_at,rebuild_task_event_id`, [task.rows[0].rebuild_task_id]);
  for (const event of ['created', 'claimed', 'running', 'provider_called', 'output_validated', 'published']) {
    assert.ok(events.rows.some((row) => row.event_type === event), `missing state event ${event}`);
  }
  const execution = await pool.query(`SELECT status,provider,request_hash,output
    FROM drama.rebuild_provider_executions WHERE rebuild_task_id=$1`, [task.rows[0].rebuild_task_id]);
  assert.equal(execution.rows[0].status, 'succeeded');
  assert.equal(execution.rows[0].provider, 'local_conformance');
  assert.match(execution.rows[0].request_hash, /^[0-9a-f]{64}$/);

  const artifacts = await pool.query(`SELECT artifact_id,native_entity_id,content_hash,validity_status,is_current
    FROM drama.artifacts WHERE artifact_id IN('artifact_phase5_timeline',$1) ORDER BY artifact_id`,
  [result.successorArtifactId]);
  const oldArtifact = artifacts.rows.find((row) => row.artifact_id === 'artifact_phase5_timeline');
  const successor = artifacts.rows.find((row) => row.artifact_id === result.successorArtifactId);
  assert.deepEqual([oldArtifact.validity_status, oldArtifact.is_current], ['superseded', false]);
  assert.deepEqual([successor.validity_status, successor.is_current], ['valid', true]);
  assert.equal(successor.content_hash, output.artifact.content_hash);
  assert.equal(successor.native_entity_id, output.artifact.native_entity_id);
  const native = await pool.query(`SELECT timeline_id,parent_timeline_id,is_current,approval_state,status,
    source_versions->>'rebuild_task_id' rebuild_task_id FROM drama.edit_timelines
    WHERE timeline_id IN('timeline_phase5_v1',$1) ORDER BY version`, [output.artifact.native_entity_id]);
  assert.equal(native.rows[0].timeline_id, 'timeline_phase5_v1');
  assert.equal(native.rows[0].is_current, false);
  assert.equal(native.rows[1].parent_timeline_id, 'timeline_phase5_v1');
  assert.equal(native.rows[1].is_current, true);
  assert.equal(native.rows[1].approval_state, 'draft');
  assert.equal(native.rows[1].rebuild_task_id, task.rows[0].rebuild_task_id);
  const binding = await pool.query(`SELECT current_artifact_id FROM drama.artifact_current_bindings
    WHERE current_artifact_id=$1`, [result.successorArtifactId]);
  assert.equal(binding.rowCount, 1);
  const publication = await pool.query(`SELECT * FROM drama.rebuild_publications WHERE rebuild_task_id=$1`,
    [task.rows[0].rebuild_task_id]);
  assert.equal(publication.rows[0].predecessor_artifact_id, 'artifact_phase5_timeline');
  assert.equal(publication.rows[0].successor_artifact_id, result.successorArtifactId);
  const regeneration = await pool.query(`SELECT request.status,item.status FROM drama.regeneration_requests request
    JOIN drama.regeneration_request_items item USING(regeneration_request_id)
    WHERE item.regeneration_request_item_id='regeni_rebuild_consumer_e2e'`);
  assert.deepEqual(regeneration.rows[0], { status: 'completed' });

  const afterUnrelated = await pool.query(`SELECT content_hash,validity_status,is_current
    FROM drama.artifacts WHERE artifact_id='artifact_phase5_sound_bgm'`);
  assert.deepEqual(afterUnrelated.rows[0], beforeUnrelated.rows[0], 'unrelated sound artifact changed');
});
