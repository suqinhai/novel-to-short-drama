'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');

const workflowPath = path.resolve(__dirname, '..', 'workflows', '07-visual-assets.json');
const workflow = JSON.parse(fs.readFileSync(workflowPath, 'utf8').replace(/^\uFEFF/, ''));
const node = (name) => {
  const result = workflow.nodes.find((entry) => entry.name === name);
  assert.ok(result, `missing workflow node: ${name}`);
  return result;
};

test('asset persistence uses a pre-built parameter array', () => {
  const normalize = node('Normalize Asset Result');
  const save = node('Save Asset and Review');
  assert.equal(save.parameters.options.queryReplacement, '={{$json.db_values}}');

  const source = {
    project_id: 'project_test',
    profile_id: 'profile_test',
    asset_type: 'character_full_body',
    entity_type: 'character',
    entity_id: 'character_test',
    generation_version: 2,
    prompt: 'test prompt',
    negative_prompt: 'test negative prompt',
    reference_image_urls: ['/reference.png'],
    regenerated_from_asset_id: 'asset_source',
    generation_mode: 'replace',
    style: { width: 1024, height: 1792 },
  };
  const providerResult = {
    status: 'succeeded',
    provider: 'test-provider',
    model: 'test-model',
    task_id: 'image_task_test',
    images: [{
      url: 'http://localhost/image.png',
      storage_url: '/data/image.png',
      width: 1024,
      height: 1792,
      content_hash: 'hash',
    }],
  };
  const execute = new Function('require', '$', '$env', '$json', normalize.parameters.jsCode);
  const [result] = execute(
    require,
    () => ({ item: { json: source } }),
    { IMAGE_PROVIDER: 'fallback-provider', IMAGE_MODEL: 'fallback-model' },
    providerResult,
  );

  assert.equal(result.json.status, 'succeeded');
  assert.equal(result.json.db_values.length, 28);
  assert.equal(result.json.db_values[24], 'image_task_test');
  assert.equal(result.json.db_values[26], 'asset_source');
  assert.equal(result.json.db_values[27], 'replace');
  assert.deepEqual(JSON.parse(result.json.db_values[13]), ['asset_source']);
});

test('batch success requires every generated asset to be persisted', () => {
  const stats = node('Collect Visual Asset Stats').parameters.query;
  const result = node('Build Batch Result').parameters.jsCode;

  assert.match(stats, /persisted_count/);
  assert.match(stats, /LEFT JOIN drama\.generated_assets/);
  assert.match(result, /persistedCount===taskCount/);
  assert.match(result, /ASSET_PERSISTENCE_FAILED/);
});
