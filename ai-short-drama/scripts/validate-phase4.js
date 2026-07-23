const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const workflowDir = path.join(root, 'workflows');
const requiredWorkflows = [
  '09a-video-provider-adapter.json',
  '09b-video-task-poller.json',
  '09-image-to-video.json',
  '10a-tts-provider-adapter.json',
  '10b-audio-task-poller-process.json',
  '10-voice-audio.json',
  '00-project-orchestrator.json',
];
const requiredWorkflowIds = new Map([
  ['09a-video-provider-adapter.json', 'wf_video_provider_adapter'],
  ['09b-video-task-poller.json', 'wf_video_task_poller'],
  ['09-image-to-video.json', 'wf_image_to_video'],
  ['10a-tts-provider-adapter.json', 'wf_tts_provider_adapter'],
  ['10b-audio-task-poller-process.json', 'wf_audio_task_poller_process'],
  ['10-voice-audio.json', 'wf_voice_audio'],
  ['00-project-orchestrator.json', 'wf_project_orchestrator'],
]);
const requiredEnv = [
  'VIDEO_PROMPT_MODEL','VIDEO_QC_MODEL','VIDEO_PROVIDER','VIDEO_MODEL','VIDEO_API_BASE_URL','VIDEO_API_KEY','VIDEO_USE_GENERATED_AUDIO',
  'VIDEO_PROVIDER_MODE','VIDEO_REQUEST_TIMEOUT_SECONDS','VIDEO_MAX_RETRIES','VIDEO_MAX_CONCURRENCY',
  'VIDEO_REQUEST_INTERVAL_MS','VIDEO_POLL_INTERVAL_SECONDS','VIDEO_MAX_POLL_COUNT','VIDEO_MAX_WAIT_MINUTES',
  'VIDEO_POLLER_BATCH_SIZE','VIDEO_DEFAULT_DURATION_SECONDS','VIDEO_DEFAULT_ASPECT_RATIO','VIDEO_DEFAULT_RESOLUTION',
  'VIDEO_DEFAULT_FPS','VIDEO_DURATION_TOLERANCE_PERCENT','TEST_MAX_VIDEO_SHOTS',
  'TTS_PROVIDER','TTS_MODEL','TTS_API_BASE_URL','TTS_API_KEY','TTS_PROVIDER_MODE','TTS_REQUEST_TIMEOUT_SECONDS',
  'TTS_MAX_RETRIES','TTS_MAX_CONCURRENCY','TTS_REQUEST_INTERVAL_MS','TTS_POLL_INTERVAL_SECONDS','TTS_MAX_POLL_COUNT',
  'TTS_MAX_WAIT_MINUTES','TTS_DEFAULT_LANGUAGE','TTS_DEFAULT_FORMAT','TTS_DEFAULT_SAMPLE_RATE','TTS_TARGET_LOUDNESS_LUFS',
  'TTS_MAX_PEAK_DB','TTS_MAX_TEXT_LENGTH','DEFAULT_NARRATOR_VOICE_ID','TEST_MAX_DIALOGUES',
  'MEDIA_PROCESSING_ENABLED','FFMPEG_PATH','FFPROBE_PATH','MEDIA_MAX_DOWNLOAD_MB','MOCK_MODE',
];
const requiredTables = [
  'video_generation_tasks','shot_videos','voice_profiles','tts_generation_tasks','dialogue_audio',
  'subtitle_cues','episode_audio_plans','media_processing_jobs',
];
const requiredFixtures = [
  '09-generate-shot-videos.json','09-review-shot-video.json','09-regenerate-shot-video.json',
  '10-generate-episode-audio.json','10-review-audio.json',
  'mock-video-provider-responses.json','mock-tts-provider-responses.json',
];

const errors = [];
const warnings = [];
const parsed = new Map();
const assert = (condition, message) => { if (!condition) errors.push(message); };

function walk(value, visit, keyPath = []) {
  visit(value, keyPath);
  if (Array.isArray(value)) value.forEach((v, i) => walk(v, visit, keyPath.concat(i)));
  else if (value && typeof value === 'object') Object.entries(value).forEach(([k, v]) => walk(v, visit, keyPath.concat(k)));
}

function parseExpression(raw, file, nodeName, keyPath) {
  if (typeof raw !== 'string' || !raw.startsWith('={{') || !raw.endsWith('}}')) return;
  const body = raw.slice(3, -2).trim();
  try { new Function(`return (${body});`); }
  catch (error) { errors.push(`${file}: ${nodeName} expression ${keyPath.join('.')} does not parse: ${error.message}`); }
}

for (const file of requiredWorkflows) {
  const full = path.join(workflowDir, file);
  if (!fs.existsSync(full)) { errors.push(`${file}: missing`); continue; }
  let workflow;
  try { workflow = JSON.parse(fs.readFileSync(full, 'utf8').replace(/^\uFEFF/, '')); }
  catch (error) { errors.push(`${file}: invalid JSON: ${error.message}`); continue; }
  parsed.set(file, workflow);
  assert(workflow.id === requiredWorkflowIds.get(file), `${file}: workflow id ${workflow.id} does not match ${requiredWorkflowIds.get(file)}`);
  assert(Array.isArray(workflow.nodes) && workflow.nodes.length > 0, `${file}: nodes missing`);
  const ids = workflow.nodes.map((n) => n.id);
  const names = workflow.nodes.map((n) => n.name);
  const nameSet = new Set(names);
  assert(new Set(ids).size === ids.length, `${file}: duplicate node id`);
  assert(nameSet.size === names.length, `${file}: duplicate node name`);
  for (const [source, outputs] of Object.entries(workflow.connections || {})) {
    assert(nameSet.has(source), `${file}: connection source not found: ${source}`);
    for (const lanes of Object.values(outputs || {})) {
      for (const lane of lanes || []) for (const edge of lane || []) assert(nameSet.has(edge.node), `${file}: connection target not found: ${edge.node}`);
    }
  }
  for (const node of workflow.nodes) {
    if (node.type === 'n8n-nodes-base.code' && typeof node.parameters?.jsCode === 'string') {
      try { new Function(node.parameters.jsCode); }
      catch (error) { errors.push(`${file}: Code node ${node.name} does not parse: ${error.message}`); }
    }
    walk(node.parameters, (value, keyPath) => parseExpression(value, file, node.name, keyPath));
    if (node.type === 'n8n-nodes-base.switch') {
      const ruleCount = node.parameters?.rules?.values?.length;
      const laneCount = workflow.connections?.[node.name]?.main?.length || 0;
      if (Number.isInteger(ruleCount)) {
        const expected = ruleCount + (node.parameters?.options?.fallbackOutput === 'extra' ? 1 : 0);
        assert(laneCount === expected, `${file}: Switch ${node.name} has ${laneCount} lanes, expected ${expected}`);
      }
    }
    if (node.type === 'n8n-nodes-base.wait') {
      const quotaPacingWait = file === '09-image-to-video.json'
        && node.name === 'Rate Limit Video Submissions'
        && node.parameters?.resume === 'timeInterval'
        && node.parameters?.unit === 'seconds';
      if (!quotaPacingWait) errors.push(`${file}: Wait node is not allowed outside bounded video submission pacing`);
    }
  }
  console.log(`OK workflow ${file}: ${workflow.nodes.length} nodes`);
}

const allWorkflowIds = new Set(
  fs.readdirSync(workflowDir).filter((f) => f.endsWith('.json')).map((f) => {
    try { return JSON.parse(fs.readFileSync(path.join(workflowDir, f), 'utf8').replace(/^\uFEFF/, '')).id; }
    catch { return null; }
  }).filter(Boolean),
);
for (const [file, workflow] of parsed) {
  for (const node of workflow.nodes) if (node.type === 'n8n-nodes-base.executeWorkflow') {
    const target = node.parameters?.workflowId?.value;
    assert(typeof target === 'string' && allWorkflowIds.has(target), `${file}: Execute Sub-workflow target not found: ${target}`);
  }
}

const videoGenerationWorkflow = fs.readFileSync(path.join(workflowDir, '09-image-to-video.json'), 'utf8');
const videoAdapterWorkflow = fs.readFileSync(path.join(workflowDir, '09a-video-provider-adapter.json'), 'utf8');
const ttsAdapterWorkflow = fs.readFileSync(path.join(workflowDir, '10a-tts-provider-adapter.json'), 'utf8');
const voiceAudioWorkflow = fs.readFileSync(path.join(workflowDir, '10-voice-audio.json'), 'utf8');
assert(videoGenerationWorkflow.includes('model,provider,prompt:videoPrompt'), '09-image-to-video.json: selected video model is not included in request payload');
assert(videoGenerationWorkflow.includes('"shot_id": "={{$json.shot.shot_id}}"'), '09-image-to-video.json: provider dispatch is missing the top-level shot_id');
assert(videoGenerationWorkflow.includes('Rate Limit Video Submissions'), '09-image-to-video.json: provider dispatch is missing quota-safe request pacing');
assert(videoAdapterWorkflow.includes('model: task.model, ...request'), '09a-video-provider-adapter.json: selected video model is not forwarded to provider');
assert(videoAdapterWorkflow.includes("const { URL: URLCtor } = require('url')"), '09a-video-provider-adapter.json: endpoint validation must use the sandbox-safe URL implementation');
assert(videoAdapterWorkflow.includes('Normalize Provider Response v3'), '09a-video-provider-adapter.json: provider responses must use the nested-response-safe allowlisted normalizer');
assert(videoAdapterWorkflow.includes("createHash('sha256')") && videoAdapterWorkflow.includes('recovered_provider_task_id'), '09a-video-provider-adapter.json: Veo task IDs must be recoverable without persisting raw HTTP objects');
for (const provider of ['google_gemini_speech', 'google_chirp3_hd']) {
  assert(ttsAdapterWorkflow.includes(provider), `10a-tts-provider-adapter.json: missing ${provider} route`);
}
assert(ttsAdapterWorkflow.includes('generativelanguage.googleapis.com') && ttsAdapterWorkflow.includes(':generateContent'), '10a-tts-provider-adapter.json: Gemini Speech endpoint missing');
assert(ttsAdapterWorkflow.includes('texttospeech.googleapis.com') && ttsAdapterWorkflow.includes('/v1/text:synthesize'), '10a-tts-provider-adapter.json: Chirp 3 HD endpoint missing');
assert(ttsAdapterWorkflow.includes('x-goog-api-key') && !ttsAdapterWorkflow.includes('?key='), '10a-tts-provider-adapter.json: Google credentials must use a header, never a URL query');
assert(ttsAdapterWorkflow.includes("Buffer.from(compact,'base64')") && ttsAdapterWorkflow.includes('fs.writeFileSync'), '10a-tts-provider-adapter.json: Google audio must be decoded to media storage');
assert(!ttsAdapterWorkflow.includes('"jsonBody": "={{ (()=>'), '10a-tts-provider-adapter.json: HTTP JSON body expressions must avoid parser-sensitive IIFEs');
assert(ttsAdapterWorkflow.includes("status='failed'AND $14 IN('retry','resume','regenerate')"), '10a-tts-provider-adapter.json: failed TTS tasks must be resumable');
assert(ttsAdapterWorkflow.includes("typeof err==='string'?err"), '10a-tts-provider-adapter.json: provider error strings must be preserved');
assert(voiceAudioWorkflow.includes("output_data->>'status'") && voiceAudioWorkflow.includes("'voice_profiles_created'"), '10-voice-audio.json: waiting-review voice profile cache must be resumable');
assert(voiceAudioWorkflow.includes("'audio_processing'"), '10-voice-audio.json: incomplete audio-processing cache must be resumable');
assert(!voiceAudioWorkflow.includes("THEN NULL ELSE drama.workflow_tasks.output_data"), '10-voice-audio.json: workflow task output_data must stay non-null when resuming');

const envPath = path.join(root, '.env.example');
const envLines = fs.readFileSync(envPath, 'utf8').replace(/^\uFEFF/, '').split(/\r?\n/).filter((line) => line && !line.startsWith('#'));
const envNames = envLines.map((line) => line.split('=', 1)[0]);
assert(new Set(envNames).size === envNames.length, '.env.example: duplicate variable');
for (const name of requiredEnv) assert(envNames.includes(name), `.env.example: missing ${name}`);
const compose = fs.readFileSync(path.join(root, 'docker-compose.yml'), 'utf8');
for (const name of requiredEnv) assert(compose.includes(name), `docker-compose.yml: missing ${name}`);
assert(compose.includes('./database/04-video-audio.sql:/opt/drama/04-video-audio.sql:ro'), 'docker-compose.yml: 04 migration is not mounted');
assert(compose.includes('Dockerfile.n8n-ffmpeg'), 'docker-compose.yml: fixed FFmpeg image build missing');

const sql = fs.readFileSync(path.join(root, 'database', '04-video-audio.sql'), 'utf8');
for (const table of requiredTables) assert(new RegExp(`CREATE TABLE IF NOT EXISTS drama\\.${table}\\b`, 'i').test(sql), `04-video-audio.sql: missing ${table}`);
assert(/^BEGIN;/m.test(sql) && /COMMIT;\s*$/.test(sql), '04-video-audio.sql: transaction boundary missing');
assert(!/\b(DROP TABLE|TRUNCATE|DELETE FROM)\b/i.test(sql), '04-video-audio.sql: destructive DDL/DML found');
assert(!/\bBYTEA\b/i.test(sql), '04-video-audio.sql: media binary column is forbidden');
const triggerLoop = (sql.match(/FOREACH\s+t\s+IN\s+ARRAY\s+ARRAY\[([\s\S]*?)\]\s+LOOP/i) || [])[1] || '';
assert(/trg_%I_updated/i.test(sql) && /set_updated_at/i.test(sql), '04-video-audio.sql: dynamic updated_at trigger builder missing');
for (const table of requiredTables) assert(triggerLoop.includes(`'${table}'`), `04-video-audio.sql: updated_at trigger list missing ${table}`);
const bootstrap = fs.readFileSync(path.join(root, 'database', 'bootstrap.sh'), 'utf8');
const order = ['init.sql','02-script-storyboard.sql','03-visual-assets-images.sql','04-video-audio.sql'].map((name) => bootstrap.indexOf(name));
assert(order.every((v) => v >= 0) && order.every((v, i) => i === 0 || v > order[i - 1]), 'bootstrap.sh: migration order is not init -> 02 -> 03 -> 04');

for (const fixture of requiredFixtures) {
  const full = path.join(root, 'test-data', fixture);
  if (!fs.existsSync(full)) { errors.push(`test-data/${fixture}: missing`); continue; }
  try { JSON.parse(fs.readFileSync(full, 'utf8').replace(/^\uFEFF/, '')); }
  catch (error) { errors.push(`test-data/${fixture}: invalid JSON: ${error.message}`); }
}
for (const dir of ['shot-videos','voices','dialogue-audio','subtitles','waveforms']) {
  assert(fs.existsSync(path.join(root, 'storage', dir, '.gitkeep')), `storage/${dir}/.gitkeep: missing`);
}
const readme = fs.readFileSync(path.join(root, 'README.md'), 'utf8');
assert((readme.match(/```/g) || []).length % 2 === 0, 'README.md: unbalanced code fence');
for (const term of ['第四阶段','04-video-audio.sql','09a-video-provider-adapter.json','10a-tts-provider-adapter.json','stage_4_completed','FFprobe']) {
  assert(readme.includes(term), `README.md: missing phase 4 topic ${term}`);
}

if (warnings.length) warnings.forEach((warning) => console.warn(`WARN ${warning}`));
if (errors.length) {
  errors.forEach((error) => console.error(`ERROR ${error}`));
  console.error(`FAILED: ${errors.length} error(s), ${warnings.length} warning(s)`);
  process.exitCode = 1;
} else {
  console.log(`PASS phase 4 static validation: ${requiredWorkflows.length} workflows, ${requiredEnv.length} env vars, ${requiredFixtures.length} fixtures`);
}
