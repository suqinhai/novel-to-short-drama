'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fsp = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');
const { Pool } = require('pg');
const { RebuildConsumer, RebuildError, localConformanceProvider, publishSuccess } = require('../rebuild-consumer');

const databaseURL = process.env.REBUILD_STATE_DATABASE_URL;

function makeConsumer(pool, storagePath, workerId, options = {}) {
  return new RebuildConsumer({
    pool,
    storagePath,
    publicBaseUrl: 'http://local.invalid/storage',
    workerId,
    leaseSeconds: options.leaseSeconds || 30,
    heartbeatSeconds: 2,
    providerTimeoutSeconds: options.timeoutSeconds || 30,
    retryDelaySeconds: 1,
    localConformanceEnabled: true,
    providerImplementations: options.providerImplementations,
  });
}

async function status(pool, taskId) {
  return (await pool.query(`SELECT status,attempt_count,error_code,error_message,claim_token,lease_owner,
    successor_artifact_id FROM drama.incremental_rebuild_tasks WHERE rebuild_task_id=$1`, [taskId])).rows[0];
}

async function prioritize(pool, taskId) {
  await pool.query(`UPDATE drama.incremental_rebuild_tasks SET
    next_attempt_at=CASE WHEN rebuild_task_id=$1 THEN now() ELSE now()+interval '1 hour' END
    WHERE status IN('pending','retry_wait')`, [taskId]);
}

test('claim uses SKIP LOCKED and lease expiry is safely recovered', { skip: !databaseURL }, async (t) => {
  const pool = new Pool({ connectionString: databaseURL, max: 12 });
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-state-'));
  t.after(async () => { await pool.end(); await fsp.rm(root, { recursive: true, force: true }); });
  await pool.query(`UPDATE drama.incremental_rebuild_tasks SET next_attempt_at=now()+interval '1 hour'
    WHERE rebuild_task_id<>'rebuild_concurrency' AND status IN('pending','retry_wait')`);
  const consumers = Array.from({ length: 8 }, (_, index) => makeConsumer(pool, root, `claim-worker-${index}`));
  const claims = await Promise.all(consumers.map((consumer) => consumer.claimOne()));
  assert.equal(claims.filter(Boolean).length, 1, 'concurrent workers claimed the same task more than once');
  assert.equal(claims.find(Boolean).rebuild_task_id, 'rebuild_concurrency');
  const secondClaim = await consumers[0].claimOne();
  assert.equal(secondClaim, null, 'an active lease was claimed twice');

  await pool.query(`UPDATE drama.incremental_rebuild_tasks SET lease_expires_at=now()-interval '1 second'
    WHERE rebuild_task_id='rebuild_concurrency'`);
  const recovered = await makeConsumer(pool, root, 'lease-recovery-worker').claimOne();
  assert.equal(recovered.rebuild_task_id, 'rebuild_concurrency');
  assert.equal(recovered.attempt_count, 2);
  assert.equal(recovered.lease_owner, 'lease-recovery-worker');
  const events = await pool.query(`SELECT event_type FROM drama.rebuild_task_events
    WHERE rebuild_task_id='rebuild_concurrency'`);
  assert.ok(events.rows.some((row) => row.event_type === 'lease_recovered'));

  await pool.query(`UPDATE drama.incremental_rebuild_tasks SET status='failed',completed_at=now(),
    claim_token=NULL,lease_owner=NULL,lease_expires_at=NULL,heartbeat_at=NULL,
    error_code='TEST_COMPLETE',error_message='claim test complete'
    WHERE rebuild_task_id='rebuild_concurrency'`);
  await pool.query(`UPDATE drama.incremental_rebuild_tasks SET next_attempt_at=NULL
    WHERE rebuild_task_id<>'rebuild_concurrency' AND status IN('pending','retry_wait')`);
});

test('failure, timeout, invalid output and hash mismatch preserve predecessor current', { skip: !databaseURL }, async (t) => {
  const pool = new Pool({ connectionString: databaseURL, max: 8 });
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-failures-'));
  t.after(async () => { await pool.end(); await fsp.rm(root, { recursive: true, force: true }); });
  const implementations = {
    test_failure: async () => { throw new RebuildError('TEST_PROVIDER_FAILURE', 'provider failed', false); },
	test_timeout: async (context) => new Promise((resolve, reject) => {
	  context.signal.addEventListener('abort', () => {
		const error = new Error('provider call was aborted by the consumer deadline');
		error.name = 'AbortError';
		reject(error);
	  }, { once: true });
	}),
    test_invalid: async () => ({ bad: true }),
    test_hash_mismatch: async (context) => {
      const output = await localConformanceProvider({ ...context, localConformanceEnabled: true });
      output.provider = 'test_hash_mismatch';
      output.provenance.provider = 'test_hash_mismatch';
      output.provenance.execution_mode = 'external';
      output.artifact.content_hash = '0'.repeat(64);
      return output;
    },
  };
  for (const [taskId, expectedCode] of [
    ['rebuild_failure', 'TEST_PROVIDER_FAILURE'],
    ['rebuild_timeout', 'REBUILD_PROVIDER_TIMEOUT'],
    ['rebuild_invalid', 'REBUILD_OUTPUT_SCHEMA_INVALID'],
    ['rebuild_hash_mismatch', 'REBUILD_OUTPUT_HASH_MISMATCH'],
  ]) {
    await prioritize(pool, taskId);
	const consumer = makeConsumer(pool, root, `failure-${taskId}`, {
	  providerImplementations: implementations, timeoutSeconds: taskId === 'rebuild_timeout' ? 0.05 : 30,
	});
    await assert.rejects(consumer.runOnce());
    const state = await status(pool, taskId);
    assert.equal(state.status, 'failed');
    assert.equal(state.error_code, expectedCode);
    assert.equal(state.successor_artifact_id, null);
    const current = await pool.query(`SELECT is_current,validity_status FROM drama.artifacts
      WHERE artifact_id='artifact_phase5_timeline'`);
    assert.deepEqual(current.rows[0], { is_current: true, validity_status: 'valid' });
  }
});

test('retry succeeds and duplicate publication callback is idempotent', { skip: !databaseURL }, async (t) => {
  const pool = new Pool({ connectionString: databaseURL, max: 8 });
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-retry-'));
  t.after(async () => { await pool.end(); await fsp.rm(root, { recursive: true, force: true }); });
  let calls = 0;
  await prioritize(pool, 'rebuild_retry_success');
  const consumer = makeConsumer(pool, root, 'retry-worker', { providerImplementations: {
    test_retry: async (context) => {
      calls += 1;
      if (calls === 1) throw new RebuildError('TEST_TRANSIENT', 'transient provider error', true);
      const output = await localConformanceProvider({ ...context, localConformanceEnabled: true });
      output.provider = 'test_retry';
      output.provenance.provider = 'test_retry';
      output.provenance.execution_mode = 'external';
      return output;
    },
  } });
  await assert.rejects(consumer.runOnce(), (error) => error.code === 'TEST_TRANSIENT');
  assert.equal((await status(pool, 'rebuild_retry_success')).status, 'retry_wait');
  await pool.query(`UPDATE drama.incremental_rebuild_tasks SET next_attempt_at=now()
    WHERE rebuild_task_id='rebuild_retry_success'`);
  const published = await consumer.runOnce();
  assert.match(published.successorArtifactId, /^artifact_rb_/);
  const state = await pool.query(`SELECT * FROM drama.incremental_rebuild_tasks
    WHERE rebuild_task_id='rebuild_retry_success'`);
  assert.equal(state.rows[0].status, 'succeeded');
  assert.equal(state.rows[0].attempt_count, 2);
  const duplicate = await publishSuccess(pool, state.rows[0], state.rows[0].output);
  assert.deepEqual(duplicate, { successorArtifactId: published.successorArtifactId, duplicate: true });
  assert.equal((await pool.query(`SELECT count(*)::int count FROM drama.rebuild_publications
    WHERE rebuild_task_id='rebuild_retry_success'`)).rows[0].count, 1);
});

test('transaction failure rolls back both native and artifact current switches', { skip: !databaseURL }, async (t) => {
  const pool = new Pool({ connectionString: databaseURL, max: 8 });
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-transaction-'));
  t.after(async () => {
    await pool.query(`DROP TRIGGER IF EXISTS trg_test_fail_rebuild_publication ON drama.rebuild_publications;
      DROP FUNCTION IF EXISTS drama.test_fail_rebuild_publication()`);
    await pool.end();
    await fsp.rm(root, { recursive: true, force: true });
  });
  const current = await pool.query(`SELECT artifact.artifact_id,artifact.native_entity_id,artifact.content_hash,
      timeline.approval_state FROM drama.artifacts artifact
      JOIN drama.edit_timelines timeline ON timeline.timeline_id=artifact.native_entity_id
    WHERE artifact.project_id='p_phase1_legacy' AND artifact.artifact_type='edit_timeline'
      AND artifact.is_current AND artifact.validity_status='valid'
    ORDER BY artifact.updated_at DESC LIMIT 1`);
  await pool.query(`UPDATE drama.incremental_rebuild_tasks SET artifact_id=$2,target_entity_id=$3,
    input=input||jsonb_build_object('predecessor_artifact_id',$2::text,'predecessor_content_hash',$4::text)
    WHERE rebuild_task_id=$1`, ['rebuild_transaction_failure', current.rows[0].artifact_id,
    current.rows[0].native_entity_id, current.rows[0].content_hash]);
  await pool.query(`CREATE OR REPLACE FUNCTION drama.test_fail_rebuild_publication() RETURNS trigger LANGUAGE plpgsql AS $$
    BEGIN IF NEW.rebuild_task_id='rebuild_transaction_failure' THEN RAISE EXCEPTION 'injected publication failure'; END IF; RETURN NEW; END $$;
    CREATE TRIGGER trg_test_fail_rebuild_publication BEFORE INSERT ON drama.rebuild_publications
    FOR EACH ROW EXECUTE FUNCTION drama.test_fail_rebuild_publication()`);
  await prioritize(pool, 'rebuild_transaction_failure');
  const consumer = makeConsumer(pool, root, 'transaction-worker', { providerImplementations: {
    test_transaction: async (context) => {
      const output = await localConformanceProvider({ ...context, localConformanceEnabled: true });
      output.provider = 'test_transaction';
      output.provenance.provider = 'test_transaction';
      output.provenance.execution_mode = 'external';
      return output;
    },
  } });
  await assert.rejects(consumer.runOnce(), (error) => error.code === 'REBUILD_PUBLICATION_FAILED');
  const state = await status(pool, 'rebuild_transaction_failure');
  assert.equal(state.status, 'failed');
  assert.equal(state.successor_artifact_id, null);
  const oldArtifact = await pool.query(`SELECT is_current,validity_status FROM drama.artifacts
    WHERE artifact_id=$1`, [current.rows[0].artifact_id]);
  assert.deepEqual(oldArtifact.rows[0], { is_current: true, validity_status: 'valid' });
  const oldNative = await pool.query(`SELECT is_current,approval_state FROM drama.edit_timelines
    WHERE timeline_id=$1`, [current.rows[0].native_entity_id]);
  assert.equal(oldNative.rows[0].is_current, true);
  assert.equal(oldNative.rows[0].approval_state, current.rows[0].approval_state);
  assert.equal((await pool.query(`SELECT count(*)::int count FROM drama.rebuild_publications
    WHERE rebuild_task_id='rebuild_transaction_failure'`)).rows[0].count, 0);
});
