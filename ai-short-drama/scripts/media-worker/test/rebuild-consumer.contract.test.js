'use strict';

const assert = require('node:assert/strict');
const crypto = require('node:crypto');
const fsp = require('node:fs/promises');
const os = require('node:os');
const path = require('node:path');
const test = require('node:test');
const { ACTIONS, OUTPUT_SCHEMA, RebuildError, validateProviderOutput } = require('../rebuild-consumer');

function hash(value) {
  return crypto.createHash('sha256').update(value).digest('hex');
}

function taskFor(action) {
  return {
    rebuild_task_id: `rebuild-contract-${action}`,
    action,
    provider: 'local_conformance',
    provider_execution_id: `rbx-contract-${action}`,
    artifact_id: `artifact-contract-${action}`,
    target_entity_type: action === 'recompose_timeline' ? 'edit_timeline' : 'entity',
    target_entity_id: `entity-${action}`,
    target_entity_version_id: null,
    attempt_count: 1,
	input: {
	  predecessor_content_hash: 'a'.repeat(64),
	  from_source_version_id: 'source-v1',
	  to_source_version_id: 'source-v2',
	  source_change_set_id: 'change-set-contract',
	},
  };
}

async function outputFor(root, action) {
  const task = taskFor(action);
  const spec = ACTIONS[action];
  const filePath = path.join(root, `${action}.${spec.extension}`);
  let bytes;
  if (spec.format === 'json') bytes = Buffer.from(`${JSON.stringify({ action, materialized: true, padding: 'x'.repeat(80) })}\n`);
  if (spec.format === 'wav') bytes = Buffer.concat([Buffer.from('RIFF'), Buffer.alloc(4), Buffer.from('WAVE'), Buffer.alloc(2048)]);
  if (spec.format === 'png') bytes = Buffer.concat([Buffer.from('89504e470d0a1a0a', 'hex'), Buffer.alloc(256)]);
  if (spec.format === 'mp4') bytes = Buffer.concat([Buffer.alloc(4), Buffer.from('ftyp'), Buffer.alloc(2048)]);
  await fsp.writeFile(filePath, bytes);
  return {
    task,
    output: {
      schema_version: OUTPUT_SCHEMA,
      task_id: task.rebuild_task_id,
      action,
      provider: task.provider,
      execution_id: task.provider_execution_id,
      artifact: {
        artifact_type: spec.artifactType,
        native_entity_id: `native-${action}`,
        storage_path: filePath,
        storage_url: `http://local.invalid/${action}.${spec.extension}`,
        content_hash: hash(bytes),
        size_bytes: bytes.length,
        mime_type: spec.mimeType,
        format: spec.format,
        version: 1,
        duration_ms: spec.format === 'wav' || spec.format === 'mp4' ? 1000 : null,
      },
      source: {
        predecessor_artifact_id: task.artifact_id,
        predecessor_content_hash: 'a'.repeat(64),
        entity_type: task.target_entity_type,
        entity_id: task.target_entity_id,
        entity_version_id: null,
        from_source_version_id: 'source-v1',
        to_source_version_id: 'source-v2',
      },
      provenance: {
        execution_mode: 'local_conformance',
        provider: task.provider,
        model_version: `local-conformance-${action}-v1`,
        rebuild_task_id: task.rebuild_task_id,
        attempt: 1,
        request_hash: 'b'.repeat(64),
        source_change_set_id: 'change-set-contract',
        generated_at: new Date().toISOString(),
      },
    },
  };
}

test('all six rebuild actions have strict physical output contracts', async (t) => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-contract-'));
  t.after(() => fsp.rm(root, { recursive: true, force: true }));
  for (const action of Object.keys(ACTIONS)) {
    const { task, output } = await outputFor(root, action);
    const validated = await validateProviderOutput(output, task, root, 10 * 1024 * 1024, {
      probeMedia: async () => ({ durationMs: 1000, formatName: output.artifact.format === 'wav' ? 'wav' : 'mov,mp4',
        streams: [{ codec_type: output.artifact.format === 'wav' ? 'audio' : 'video' }] }),
    });
    assert.equal(validated.artifact.content_hash, output.artifact.content_hash);
  }
});

test('schema, file, length, hash, MIME and provenance defects are rejected', async (t) => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-negative-'));
  t.after(() => fsp.rm(root, { recursive: true, force: true }));
  const cases = [
    ['unknown field', (value) => { value.output.unexpected = true; }, 'REBUILD_OUTPUT_SCHEMA_INVALID'],
    ['wrong hash', (value) => { value.output.artifact.content_hash = '0'.repeat(64); }, 'REBUILD_OUTPUT_HASH_MISMATCH'],
    ['wrong MIME', (value) => { value.output.artifact.mime_type = 'text/plain'; }, 'REBUILD_OUTPUT_FORMAT_INVALID'],
    ['wrong task', (value) => { value.output.task_id = 'rebuild-wrong'; }, 'REBUILD_OUTPUT_SCHEMA_INVALID'],
    ['wrong provenance', (value) => { value.output.provenance.attempt = 2; }, 'REBUILD_OUTPUT_PROVENANCE_INVALID'],
	['wrong source version', (value) => { value.output.source.to_source_version_id = 'source-wrong'; }, 'REBUILD_OUTPUT_PROVENANCE_INVALID'],
    ['missing file', (value) => { value.output.artifact.storage_path = path.join(root, 'missing.json'); }, 'REBUILD_OUTPUT_FILE_MISSING'],
  ];
  for (const [name, mutate, code] of cases) {
    const value = await outputFor(root, 'recompose_timeline');
    mutate(value);
    await assert.rejects(validateProviderOutput(value.output, value.task, root, 1024 * 1024),
      (error) => error instanceof RebuildError && error.code === code, name);
  }
});

test('output path cannot escape the storage root', async (t) => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-root-'));
  const outside = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-outside-'));
  t.after(async () => {
    await fsp.rm(root, { recursive: true, force: true });
    await fsp.rm(outside, { recursive: true, force: true });
  });
  const value = await outputFor(outside, 'update_continuity');
  await assert.rejects(validateProviderOutput(value.output, value.task, root, 1024 * 1024),
    (error) => error.code === 'REBUILD_OUTPUT_PATH_INVALID');
});

test('media probe duration must match provider metadata', async (t) => {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'rebuild-duration-'));
  t.after(() => fsp.rm(root, { recursive: true, force: true }));
  const value = await outputFor(root, 'regenerate_video');
  await assert.rejects(validateProviderOutput(value.output, value.task, root, 10 * 1024 * 1024, {
    probeMedia: async () => ({ durationMs: 4000, formatName: 'mov,mp4', streams: [{ codec_type: 'video' }] }),
  }), (error) => error.code === 'REBUILD_OUTPUT_FORMAT_INVALID');
});
