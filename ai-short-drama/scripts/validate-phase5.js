'use strict';

const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const workflowDir = path.join(root, 'workflows');
const requiredWorkflows = new Map([
  ['11-edit-compose.json', 'wf_edit_compose'],
  ['11a-media-processing-worker.json', 'wf_media_processing_worker'],
  ['12-qc-review-publish.json', 'wf_qc_review_publish'],
  ['12a-publish-provider-adapter.json', 'wf_publish_provider_adapter'],
  ['12b-publish-task-poller.json', 'wf_publish_task_poller'],
  ['00-project-orchestrator.json', 'wf_project_orchestrator'],
]);
const requiredTables = [
  'edit_timelines', 'edit_timeline_items', 'render_jobs', 'episode_masters', 'qc_jobs',
  'qc_reports', 'final_reviews', 'publication_metadata', 'publication_tasks', 'workflow_notifications',
];
const requiredEnv = [
  'MEDIA_WORKER_ENABLED', 'MEDIA_WORKER_URL', 'MEDIA_WORKER_POLL_INTERVAL_SECONDS',
  'MEDIA_WORKER_BATCH_SIZE', 'MEDIA_WORKER_HEARTBEAT_SECONDS', 'MEDIA_RENDER_TIMEOUT_MINUTES',
  'MEDIA_MAX_RETRIES', 'MEDIA_MAX_THREADS', 'OUTPUT_WIDTH', 'OUTPUT_HEIGHT', 'OUTPUT_ASPECT_RATIO',
  'OUTPUT_FPS', 'OUTPUT_VIDEO_CODEC', 'OUTPUT_AUDIO_CODEC', 'OUTPUT_AUDIO_BITRATE',
  'OUTPUT_SAMPLE_RATE', 'OUTPUT_PIXEL_FORMAT', 'OUTPUT_CRF', 'OUTPUT_PRESET', 'OUTPUT_FASTSTART',
  'TARGET_LOUDNESS_LUFS', 'TARGET_TRUE_PEAK_DB', 'BGM_DUCKING_ENABLED', 'BGM_DUCKING_DB',
  'DEFAULT_TRANSITION', 'DEFAULT_TRANSITION_DURATION_MS', 'ENABLE_INTRO', 'ENABLE_OUTRO',
  'BURN_SUBTITLES', 'GENERATE_CLEAN_MASTER', 'GENERATE_PREVIEW_MASTER',
  'QC_TECHNICAL_ENABLED', 'QC_SUBTITLE_ENABLED', 'QC_CONTENT_ENABLED', 'QC_COMPLIANCE_ENABLED',
  'QC_VISION_MODEL', 'QC_TEXT_MODEL', 'QC_FRAME_SAMPLE_PER_SHOT', 'QC_BLOCKING_SCORE',
  'QC_WARNING_SCORE', 'QC_MAX_BLACK_SECONDS', 'QC_MAX_SILENCE_SECONDS',
  'QC_DURATION_TOLERANCE_PERCENT', 'QC_AUDIO_VIDEO_DRIFT_MS',
  'PUBLISH_PROVIDER', 'PUBLISH_PLATFORM', 'PUBLISH_API_BASE_URL', 'PUBLISH_API_KEY',
  'PUBLISH_ACCOUNT_REFERENCE', 'PUBLISH_MAX_RETRIES', 'PUBLISH_POLL_INTERVAL_SECONDS',
  'PUBLISH_MAX_POLL_COUNT', 'PUBLISH_MAX_WAIT_MINUTES', 'PUBLISH_DEFAULT_VISIBILITY',
  'ALLOW_REAL_PUBLISH', 'TEST_MAX_EPISODES', 'TEST_PUBLISH_MODE', 'MOCK_MODE',
];
const requiredFixtures = [
  '11-compose-episode.json', '11-recompose-episode.json', '12-run-qc.json',
  '12-final-review.json', '12-publish-episode.json', 'mock-publish-provider-responses.json',
];
const errors = [];
const warnings = [];
let codeNodes = 0;
let expressions = 0;

const assert = (condition, message) => { if (!condition) errors.push(message); };
const read = (...segments) => fs.readFileSync(path.join(root, ...segments), 'utf8').replace(/^\uFEFF/, '');

function walk(value, visitor, keyPath = []) {
  visitor(value, keyPath);
  if (Array.isArray(value)) value.forEach((entry, index) => walk(entry, visitor, keyPath.concat(index)));
  else if (value && typeof value === 'object') {
    Object.entries(value).forEach(([key, entry]) => walk(entry, visitor, keyPath.concat(key)));
  }
}

function checkExpression(value, file, node, keyPath) {
  if (typeof value !== 'string' || !value.startsWith('={{') || !value.endsWith('}}')) return;
  expressions += 1;
  const body = value.slice(3, -2).trim();
  try { new Function(`return (${body});`); }
  catch (error) { errors.push(`${file}/${node}: expression ${keyPath.join('.')} does not parse: ${error.message}`); }
}

const parsed = new Map();
for (const [file, expectedId] of requiredWorkflows) {
  const full = path.join(workflowDir, file);
  if (!fs.existsSync(full)) { errors.push(`${file}: missing`); continue; }
  let workflow;
  try { workflow = JSON.parse(fs.readFileSync(full, 'utf8').replace(/^\uFEFF/, '')); }
  catch (error) { errors.push(`${file}: invalid JSON: ${error.message}`); continue; }
  parsed.set(file, workflow);
  assert(workflow.id === expectedId, `${file}: workflow id must be ${expectedId}, got ${workflow.id}`);
  assert(Array.isArray(workflow.nodes) && workflow.nodes.length > 0, `${file}: nodes missing`);
  assert(workflow.settings?.executionOrder === 'v1', `${file}: executionOrder must be v1`);
  const ids = workflow.nodes.map((node) => node.id);
  const names = workflow.nodes.map((node) => node.name);
  const nameSet = new Set(names);
  assert(new Set(ids).size === ids.length, `${file}: duplicate node id`);
  assert(nameSet.size === names.length, `${file}: duplicate node name`);
  for (const [source, outputs] of Object.entries(workflow.connections || {})) {
    assert(nameSet.has(source), `${file}: connection source missing: ${source}`);
    for (const lanes of Object.values(outputs || {})) {
      for (const lane of lanes || []) {
        for (const edge of lane || []) assert(nameSet.has(edge.node), `${file}: connection target missing: ${edge.node}`);
      }
    }
  }
  for (const node of workflow.nodes) {
    if (node.type === 'n8n-nodes-base.code') {
      codeNodes += 1;
      try { new Function(node.parameters?.jsCode || ''); }
      catch (error) { errors.push(`${file}/${node.name}: Code node does not parse: ${error.message}`); }
    }
    walk(node.parameters, (value, keyPath) => checkExpression(value, file, node.name, keyPath));
    if (node.type === 'n8n-nodes-base.switch') {
      const rules = node.parameters?.rules?.values;
      const lanes = workflow.connections?.[node.name]?.main || [];
      if (Array.isArray(rules)) {
        const expected = rules.length + (node.parameters?.options?.fallbackOutput === 'extra' ? 1 : 0);
        assert(lanes.length === expected, `${file}/${node.name}: Switch has ${lanes.length} lanes, expected ${expected}`);
      }
    }
    assert(node.type !== 'n8n-nodes-base.wait', `${file}/${node.name}: unbounded Wait node is forbidden`);
    if (node.type === 'n8n-nodes-base.executeWorkflow') {
      assert(node.typeVersion >= 1.1, `${file}/${node.name}: incompatible Execute Sub-workflow version`);
      const value = node.parameters?.workflowId?.value;
      assert(typeof value === 'string' && value.length > 0, `${file}/${node.name}: workflow target id missing`);
    }
  }
  console.log(`OK workflow ${file}: ${workflow.nodes.length} nodes`);
}

const allWorkflowIds = new Set(fs.readdirSync(workflowDir).filter((file) => file.endsWith('.json')).flatMap((file) => {
  try { return [JSON.parse(fs.readFileSync(path.join(workflowDir, file), 'utf8').replace(/^\uFEFF/, '')).id]; }
  catch { return []; }
}));
for (const [file, workflow] of parsed) {
  for (const node of workflow.nodes.filter((entry) => entry.type === 'n8n-nodes-base.executeWorkflow')) {
    const target = node.parameters?.workflowId?.value;
    assert(allWorkflowIds.has(target), `${file}/${node.name}: target workflow ${target} is not present`);
  }
}

const sql = read('database', '05-edit-qc-publish.sql');
assert(/^BEGIN;/m.test(sql) && /COMMIT;\s*$/.test(sql), '05 SQL: transaction boundary missing');
assert(!/\b(DROP\s+TABLE|TRUNCATE|DELETE\s+FROM)\b/i.test(sql), '05 SQL: destructive table/data statement found');
assert(!/\bBYTEA\b/i.test(sql), '05 SQL: media binary column is forbidden');
for (const table of requiredTables) {
  assert(new RegExp(`CREATE TABLE IF NOT EXISTS drama\\.${table}\\b`, 'i').test(sql), `05 SQL: missing ${table}`);
  assert(sql.includes(`'${table}'`), `05 SQL: updated_at trigger list missing ${table}`);
}
assert(/uq_episode_current_final_master/i.test(sql), '05 SQL: single current final master invariant missing');
assert(/uq_publication_delivery/i.test(sql), '05 SQL: semantic publication idempotency index missing');
assert(/validate_final_review_approval/i.test(sql), '05 SQL: blocking QC approval guard missing');

const bootstrap = read('database', 'bootstrap.sh');
const migrations = ['init.sql', '02-script-storyboard.sql', '03-visual-assets-images.sql', '04-video-audio.sql', '05-edit-qc-publish.sql'];
const offsets = migrations.map((name) => bootstrap.indexOf(name));
assert(offsets.every((offset) => offset >= 0) && offsets.every((offset, index) => index === 0 || offset > offsets[index - 1]), 'bootstrap migration order must be init -> 02 -> 03 -> 04 -> 05');

const envText = read('.env.example');
const envLines = envText.split(/\r?\n/).filter((line) => line && !line.startsWith('#'));
const envNames = envLines.map((line) => line.split('=', 1)[0]);
assert(new Set(envNames).size === envNames.length, '.env.example: duplicate variable');
for (const name of requiredEnv) assert(envNames.includes(name), `.env.example: missing ${name}`);
const compose = read('docker-compose.yml');
for (const name of requiredEnv.filter((name) => !['TEST_MAX_EPISODES'].includes(name))) {
  assert(compose.includes(name), `docker-compose.yml: missing ${name}`);
}
for (const marker of ['media-worker:', 'scripts/media-worker/Dockerfile', './storage:/data/storage', '05-edit-qc-publish.sql']) {
  assert(compose.includes(marker), `docker-compose.yml: missing ${marker}`);
}

for (const file of ['Dockerfile', 'package.json', 'ffmpeg-templates.js', 'worker.js']) {
  assert(fs.existsSync(path.join(root, 'scripts', 'media-worker', file)), `scripts/media-worker/${file}: missing`);
}
if (fs.existsSync(path.join(root, 'scripts', 'media-worker', 'worker.js'))) {
  const worker = read('scripts', 'media-worker', 'worker.js');
  const templates = read('scripts', 'media-worker', 'ffmpeg-templates.js');
  assert(/spawn\s*\(/.test(worker + templates), 'media-worker: child process spawn is missing');
  assert(!/\bexec(?:Sync|File|FileSync)?\s*\(/.test(worker + templates), 'media-worker: exec-style shell execution is forbidden');
  assert(!/\beval\s*\(/.test(worker + templates), 'media-worker: eval is forbidden');
  assert(/MEDIA_STORAGE_PATH/.test(worker + templates), 'media-worker: storage root enforcement missing');
  assert(/heartbeat/i.test(worker), 'media-worker: heartbeat update missing');
  assert(/FFPROBE_PATH/.test(worker + templates), 'media-worker: FFprobe integration missing');
  assert(/sha256/i.test(worker + templates), 'media-worker: SHA-256 output hashing missing');
  assert(/RENDER_TIMEOUT|timeout/i.test(worker + templates), 'media-worker: timeout guard missing');
}

for (const fixture of requiredFixtures) {
  const full = path.join(root, 'test-data', fixture);
  if (!fs.existsSync(full)) { errors.push(`test-data/${fixture}: missing`); continue; }
  try { JSON.parse(fs.readFileSync(full, 'utf8').replace(/^\uFEFF/, '')); }
  catch (error) { errors.push(`test-data/${fixture}: invalid JSON: ${error.message}`); }
}
for (const file of ['README.md', 'metadata.json', 'qc-report.json', 'upload-instructions.txt']) {
  assert(fs.existsSync(path.join(root, 'output-package', 'example', file)), `output-package/example/${file}: missing`);
}

const readme = read('README.md');
assert((readme.match(/```/g) || []).length % 2 === 0, 'README.md: unbalanced code fence');
for (const topic of ['第五阶段', '05-edit-qc-publish.sql', 'media-worker', 'manual_package', 'waiting_final_review', 'publication_metadata_approved', 'FFprobe']) {
  assert(readme.includes(topic), `README.md: missing phase 5 topic ${topic}`);
}

const secretScan = [
  ...[...requiredWorkflows.keys()].map((file) => [
    file,
    fs.existsSync(path.join(workflowDir, file)) ? read('workflows', file) : '',
  ]),
  ['mock-publish-provider-responses.json', fs.existsSync(path.join(root, 'test-data', 'mock-publish-provider-responses.json')) ? read('test-data', 'mock-publish-provider-responses.json') : ''],
];
for (const [file, content] of secretScan) {
  assert(!/(?:sk|api|pat|ghp)_[A-Za-z0-9_-]{20,}/.test(content), `${file}: possible real secret found`);
  assert(!/Bearer\s+[A-Za-z0-9._~-]{20,}/.test(content), `${file}: literal bearer token found`);
}

if (warnings.length) warnings.forEach((warning) => console.warn(`WARN ${warning}`));
if (errors.length) {
  errors.forEach((error) => console.error(`ERROR ${error}`));
  console.error(`FAILED phase 5 static validation: ${errors.length} error(s), ${warnings.length} warning(s)`);
  process.exitCode = 1;
} else {
  console.log(`PASS phase 5 static validation: ${requiredWorkflows.size} workflows, ${codeNodes} Code nodes, ${expressions} expressions, ${requiredTables.length} tables, ${requiredEnv.length} env vars`);
}
