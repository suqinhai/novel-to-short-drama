'use strict';

const crypto = require('node:crypto');
const fs = require('node:fs');
const fsp = require('node:fs/promises');
const http = require('node:http');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { Pool } = require('pg');
const {
  TemplateError,
  buildRenderPlan,
  normalizeSettings,
} = require('./ffmpeg-templates');

const ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/;
const AUDIO_KINDS = new Set(['dialogue', 'narration', 'bgm', 'sound_effect', 'ambience', 'other']);
const MASTER_TYPES = new Set(['preview', 'clean', 'subtitled', 'final']);
const TERMINAL_JOB_STATUSES = new Set(['succeeded', 'failed', 'timeout', 'cancelled']);

function envBoolean(name, fallback) {
  const value = process.env[name];
  if (value === undefined || value === '') return fallback;
  return String(value).toLowerCase() === 'true';
}

function envInteger(name, fallback, minimum, maximum) {
  const value = process.env[name] === undefined || process.env[name] === ''
    ? fallback
    : Number(process.env[name]);
  if (!Number.isInteger(value) || value < minimum || value > maximum) {
    throw new Error(`${name} must be an integer between ${minimum} and ${maximum}`);
  }
  return value;
}

function validateToolPath(value, expectedBaseName, envName) {
  const command = String(value || expectedBaseName);
  if (command.length > 512 || /[\0\r\n]/.test(command) || !/^[A-Za-z0-9_./\\:-]+$/.test(command)) {
    throw new Error(`${envName} is not a safe executable path`);
  }
  const base = path.basename(command).toLowerCase().replace(/\.exe$/, '');
  if (base !== expectedBaseName) throw new Error(`${envName} must point to ${expectedBaseName}`);
  return command;
}

const config = Object.freeze({
  port: envInteger('MEDIA_WORKER_PORT', 8090, 1, 65535),
  enabled: envBoolean('MEDIA_WORKER_ENABLED', true),
  storagePath: path.resolve(process.env.MEDIA_STORAGE_PATH || '/data/storage'),
  publicBaseUrl: String(process.env.MEDIA_PUBLIC_BASE_URL || 'http://localhost:8088').replace(/\/+$/, ''),
  pollIntervalMs: envInteger('MEDIA_WORKER_POLL_INTERVAL_SECONDS', 5, 1, 3600) * 1000,
  heartbeatMs: envInteger('MEDIA_WORKER_HEARTBEAT_SECONDS', 10, 2, 3600) * 1000,
  renderTimeoutMs: envInteger('MEDIA_RENDER_TIMEOUT_MINUTES', 30, 1, 1440) * 60000,
  qcTimeoutMs: envInteger('MEDIA_QC_TIMEOUT_MINUTES', 10, 1, 120) * 60000,
  batchSize: envInteger('MEDIA_WORKER_BATCH_SIZE', 1, 1, 32),
  concurrency: envInteger('MEDIA_WORKER_MAX_CONCURRENCY', 1, 1, 16),
  qcConcurrency: envInteger('MEDIA_QC_MAX_CONCURRENCY', 1, 1, 8),
  maxManifestBytes: envInteger('MEDIA_MAX_MANIFEST_BYTES', 10485760, 1024, 104857600),
  maxSubtitleBytes: envInteger('MEDIA_MAX_SUBTITLE_BYTES', 5242880, 1024, 52428800),
  maxInputBytes: envInteger('MEDIA_MAX_INPUT_MB', 2048, 1, 1048576) * 1024 * 1024,
  maxLogBytes: envInteger('MEDIA_MAX_LOG_MB', 20, 1, 1024) * 1024 * 1024,
  ffmpeg: validateToolPath(process.env.FFMPEG_PATH, 'ffmpeg', 'FFMPEG_PATH'),
  ffprobe: validateToolPath(process.env.FFPROBE_PATH, 'ffprobe', 'FFPROBE_PATH'),
  mockMode: envBoolean('MOCK_MODE', true),
  burnSubtitles: envBoolean('BURN_SUBTITLES', true),
  token: String(process.env.MEDIA_WORKER_TOKEN || ''),
  blackSeconds: Number(process.env.QC_MAX_BLACK_SECONDS || 1),
  silenceSeconds: Number(process.env.QC_MAX_SILENCE_SECONDS || 2),
});

if (!Number.isFinite(config.blackSeconds) || config.blackSeconds < 0 || config.blackSeconds > 3600) {
  throw new Error('QC_MAX_BLACK_SECONDS is invalid');
}
if (!Number.isFinite(config.silenceSeconds) || config.silenceSeconds < 0 || config.silenceSeconds > 3600) {
  throw new Error('QC_MAX_SILENCE_SECONDS is invalid');
}

const workerId = (`media-${os.hostname()}-${process.pid}-${crypto.randomBytes(4).toString('hex')}`)
  .replace(/[^A-Za-z0-9_.:-]/g, '_')
  .slice(0, 128);

const poolConfig = process.env.DATABASE_URL
  ? { connectionString: process.env.DATABASE_URL }
  : {
      host: process.env.PGHOST || 'postgres',
      port: Number(process.env.PGPORT || 5432),
      user: process.env.PGUSER || process.env.POSTGRES_USER || 'n8n',
      password: process.env.PGPASSWORD || process.env.POSTGRES_PASSWORD,
      database: process.env.PGDATABASE || process.env.DRAMA_DB || 'short_drama',
    };
poolConfig.max = Math.max(4, config.concurrency + config.qcConcurrency + 2);
poolConfig.idleTimeoutMillis = 30000;
poolConfig.application_name = workerId;
if (String(process.env.PGSSLMODE || '').toLowerCase() === 'require') {
  poolConfig.ssl = { rejectUnauthorized: envBoolean('PGSSL_REJECT_UNAUTHORIZED', true) };
}
const pool = new Pool(poolConfig);
pool.on('error', (error) => console.error(JSON.stringify({ level: 'error', event: 'postgres_pool_error', message: safeMessage(error) })));

let storageRoot = null;
let shuttingDown = false;
let polling = false;
let qcRunning = 0;
let lastStaleRecoveryAt = 0;
const activeJobs = new Map();
const tools = {
  ffmpeg: { available: false, version: null, checked_at: null, error: null },
  ffprobe: { available: false, version: null, checked_at: null, error: null },
};

class WorkerError extends Error {
  constructor(code, message, retryable = false, details = {}) {
    super(message);
    this.name = 'WorkerError';
    this.code = code;
    this.retryable = retryable;
    this.details = details;
  }
}

class ProcessError extends WorkerError {
  constructor(message, details) {
    super(details.timedOut ? 'RENDER_TIMEOUT' : details.aborted ? 'RENDER_CANCELLED' : 'FFMPEG_FAILED', message, !details.aborted, details);
    this.name = 'ProcessError';
  }
}

function safeMessage(error) {
  const raw = String(error?.message || error || 'unknown error');
  return redact(raw).slice(0, 2000);
}

function redact(value) {
  let text = String(value || '');
  if (storageRoot) text = text.split(storageRoot).join('$MEDIA_STORAGE');
  text = text
    .replace(/(authorization\s*[:=]\s*bearer\s+)[^\s,;]+/gi, '$1[REDACTED]')
    .replace(/((?:api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*)[^\s,;]+/gi, '$1[REDACTED]')
    .replace(/([?&](?:token|key|signature|password)=)[^&\s]+/gi, '$1[REDACTED]');
  return text;
}

function assertId(value, name, optional = false) {
  if ((value === undefined || value === null || value === '') && optional) return null;
  const normalized = String(value || '');
  if (!ID_PATTERN.test(normalized)) throw new WorkerError('TIMELINE_VALIDATION_FAILED', `${name} is invalid`, false);
  return normalized;
}

function numberField(value, fallback, minimum, maximum, name, integer = false) {
  const parsed = value === undefined || value === null || value === '' ? fallback : Number(value);
  if (!Number.isFinite(parsed) || parsed < minimum || parsed > maximum || (integer && !Number.isInteger(parsed))) {
    throw new WorkerError('TIMELINE_VALIDATION_FAILED', `${name} is invalid`, false);
  }
  return parsed;
}

function isInsideRoot(candidate) {
  const relative = path.relative(storageRoot, candidate);
  return relative === '' || (!relative.startsWith(`..${path.sep}`) && relative !== '..' && !path.isAbsolute(relative));
}

function rawPath(value, name) {
  if (typeof value !== 'string' || value.length === 0 || value.length > 4096 || /[\0\r\n]/.test(value)) {
    throw new WorkerError('MEDIA_PATH_NOT_ALLOWED', `${name} is not a valid storage path`, false);
  }
  const candidate = path.isAbsolute(value) ? path.resolve(value) : path.resolve(storageRoot, value);
  if (!isInsideRoot(candidate)) throw new WorkerError('MEDIA_PATH_NOT_ALLOWED', `${name} is outside MEDIA_STORAGE_PATH`, false);
  return candidate;
}

async function resolveExistingFile(value, name) {
  const candidate = rawPath(value, name);
  let real;
  try {
    real = await fsp.realpath(candidate);
  } catch (error) {
    if (error.code === 'ENOENT') throw new WorkerError('MEDIA_FILE_NOT_FOUND', `${name} does not exist`, false);
    throw error;
  }
  if (!isInsideRoot(real)) throw new WorkerError('MEDIA_PATH_NOT_ALLOWED', `${name} resolves outside MEDIA_STORAGE_PATH`, false);
  const stat = await fsp.stat(real);
  if (!stat.isFile() || stat.size <= 0 || stat.size > config.maxInputBytes) {
    throw new WorkerError('MEDIA_FILE_INVALID', `${name} is not a valid bounded regular file`, false);
  }
  return { path: real, stat };
}

async function resolveOutputFile(value, name) {
  const candidate = rawPath(value, name);
  const parent = path.dirname(candidate);
  await fsp.mkdir(parent, { recursive: true, mode: 0o750 });
  const realParent = await fsp.realpath(parent);
  if (!isInsideRoot(realParent)) throw new WorkerError('MEDIA_PATH_NOT_ALLOWED', `${name} parent resolves outside MEDIA_STORAGE_PATH`, false);
  const resolved = path.join(realParent, path.basename(candidate));
  try {
    const stat = await fsp.lstat(resolved);
    if (stat.isSymbolicLink() || !stat.isFile()) {
      throw new WorkerError('MEDIA_PATH_NOT_ALLOWED', `${name} cannot be a link or non-file`, false);
    }
    const real = await fsp.realpath(resolved);
    if (!isInsideRoot(real)) throw new WorkerError('MEDIA_PATH_NOT_ALLOWED', `${name} resolves outside MEDIA_STORAGE_PATH`, false);
  } catch (error) {
    if (error.code !== 'ENOENT') throw error;
  }
  return resolved;
}

function storageRelative(filePath) {
  if (!filePath || !isInsideRoot(filePath)) return null;
  return path.relative(storageRoot, filePath).split(path.sep).join('/');
}

function publicUrl(filePath) {
  const relative = storageRelative(filePath);
  if (relative === null) return null;
  return `${config.publicBaseUrl}/${relative.split('/').map(encodeURIComponent).join('/')}`;
}

function appendTail(current, chunk, maximum = 65536) {
  const combined = current + String(chunk);
  return combined.length <= maximum ? combined : combined.slice(combined.length - maximum);
}

function createBoundedLog(logPath) {
  const stream = fs.createWriteStream(logPath, { flags: 'a', mode: 0o600 });
  let bytes = 0;
  let truncated = false;
  let failed = null;
  stream.on('error', (error) => { failed = error; });
  return {
    write(value) {
      if (failed || truncated) return;
      const buffer = Buffer.from(String(value));
      if (bytes + buffer.length > config.maxLogBytes) {
        truncated = true;
        stream.write('\n[media-worker log truncated at configured maximum]\n');
        return;
      }
      bytes += buffer.length;
      stream.write(buffer);
    },
    close() {
      return new Promise((resolve) => stream.end(resolve));
    },
    get error() { return failed; },
  };
}

function killChildProcess(child, signal = 'SIGTERM') {
  if (!child || child.exitCode !== null) return;
  try {
    if (process.platform !== 'win32' && child.pid) process.kill(-child.pid, signal);
    else child.kill(signal);
  } catch (error) {
    if (error.code !== 'ESRCH') console.error(JSON.stringify({ level: 'warn', event: 'child_kill_failed', message: safeMessage(error) }));
  }
}

function runProcess(command, args, options = {}) {
  if (!Array.isArray(args) || args.some((arg) => typeof arg !== 'string')) {
    return Promise.reject(new WorkerError('TIMELINE_VALIDATION_FAILED', 'process arguments must be a string array', false));
  }
  return new Promise((resolve, reject) => {
    let child;
    let settled = false;
    let timedOut = false;
    let aborted = false;
    let stdout = '';
    let stderr = '';
    const startedAt = Date.now();
    try {
      child = spawn(command, args, {
        shell: false,
        windowsHide: true,
        detached: process.platform !== 'win32',
        stdio: ['ignore', 'pipe', 'pipe'],
      });
    } catch (error) {
      reject(error);
      return;
    }
    options.onChild?.(child, () => {
      aborted = true;
      killChildProcess(child, 'SIGTERM');
      setTimeout(() => killChildProcess(child, 'SIGKILL'), 5000).unref();
    });
    const timeout = setTimeout(() => {
      timedOut = true;
      killChildProcess(child, 'SIGTERM');
      setTimeout(() => killChildProcess(child, 'SIGKILL'), 5000).unref();
    }, options.timeoutMs || config.renderTimeoutMs);
    timeout.unref();

    child.stdout.on('data', (chunk) => {
      if (options.captureStdout !== false) stdout = appendTail(stdout, chunk, options.maxCapture || 2097152);
      options.onStdoutChunk?.(chunk);
      if (options.logStdout) options.log?.write(`[stdout] ${chunk}`);
    });
    child.stderr.on('data', (chunk) => {
      stderr = appendTail(stderr, chunk, options.maxCapture || 2097152);
      options.onStderrChunk?.(chunk);
      options.log?.write(`[stderr] ${chunk}`);
    });
    child.on('error', (error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      reject(error);
    });
    child.on('close', (code, signal) => {
      if (settled) return;
      settled = true;
      clearTimeout(timeout);
      const result = { code, signal, stdout, stderr, timedOut, aborted, durationMs: Date.now() - startedAt };
      if (code === 0 && !timedOut && !aborted) resolve(result);
      else reject(new ProcessError(`media process exited with code ${code ?? 'null'}${signal ? ` (${signal})` : ''}`, result));
    });
  });
}

async function checkTool(command) {
  try {
    const result = await runProcess(command, ['-version'], { timeoutMs: 5000, maxCapture: 32768 });
    return { available: true, version: result.stdout.split(/\r?\n/, 1)[0].slice(0, 200), checked_at: new Date().toISOString(), error: null };
  } catch (error) {
    return { available: false, version: null, checked_at: new Date().toISOString(), error: safeMessage(error) };
  }
}

async function refreshToolState() {
  tools.ffmpeg = await checkTool(config.ffmpeg);
  tools.ffprobe = await checkTool(config.ffprobe);
}

async function parseProbe(filePath, options = {}) {
  if (!tools.ffprobe.available) throw new WorkerError('FFPROBE_NOT_AVAILABLE', 'ffprobe is not available', false);
  let result;
  try {
    result = await runProcess(config.ffprobe, [
      '-v', 'error', '-print_format', 'json', '-show_format', '-show_streams', filePath,
    ], { timeoutMs: options.timeoutMs || 60000, maxCapture: 4194304, log: options.log });
  } catch (error) {
    if (error instanceof ProcessError) {
      throw new WorkerError('MEDIA_FILE_INVALID', `ffprobe could not parse media: ${redact(error.details.stderr).slice(-1000)}`, false, { exit_code: error.details.code });
    }
    throw error;
  }
  try {
    return JSON.parse(result.stdout);
  } catch {
    throw new WorkerError('MEDIA_FILE_INVALID', 'ffprobe returned invalid JSON', false);
  }
}

function fraction(value) {
  if (typeof value !== 'string' || !value.includes('/')) return Number(value) || null;
  const [left, right] = value.split('/').map(Number);
  return Number.isFinite(left) && Number.isFinite(right) && right !== 0 ? left / right : null;
}

function probeSummary(probe) {
  const streams = Array.isArray(probe.streams) ? probe.streams : [];
  const video = streams.find((stream) => stream.codec_type === 'video') || null;
  const audio = streams.find((stream) => stream.codec_type === 'audio') || null;
  const durations = [probe.format?.duration, video?.duration, audio?.duration]
    .map(Number)
    .filter((value) => Number.isFinite(value) && value > 0);
  return {
    format: String(probe.format?.format_name || ''),
    durationMs: durations.length ? Math.round(Math.max(...durations) * 1000) : 0,
    fileSizeBytes: Number(probe.format?.size || 0),
    width: Number(video?.width || 0),
    height: Number(video?.height || 0),
    fps: fraction(video?.avg_frame_rate || video?.r_frame_rate),
    videoCodec: video?.codec_name || null,
    audioCodec: audio?.codec_name || null,
    sampleRate: audio?.sample_rate ? Number(audio.sample_rate) : null,
    channels: audio?.channels ? Number(audio.channels) : null,
    videoDurationMs: Number(video?.duration || 0) * 1000 || null,
    audioDurationMs: Number(audio?.duration || 0) * 1000 || null,
    hasVideo: Boolean(video),
    hasAudio: Boolean(audio),
  };
}

async function sha256File(filePath) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash('sha256');
    const input = fs.createReadStream(filePath);
    input.on('error', reject);
    input.on('data', (chunk) => hash.update(chunk));
    input.on('end', () => resolve(hash.digest('hex')));
  });
}

async function loadManifest(job) {
  const resolved = await resolveExistingFile(job.input_manifest_path, 'input_manifest_path');
  if (resolved.stat.size > config.maxManifestBytes) {
    throw new WorkerError('MEDIA_FILE_INVALID', 'render manifest exceeds MEDIA_MAX_MANIFEST_BYTES', false);
  }
  const bytes = await fsp.readFile(resolved.path);
  if (bytes.includes(0)) throw new WorkerError('MEDIA_FILE_INVALID', 'render manifest contains NUL bytes', false);
  let manifest;
  try {
    manifest = JSON.parse(bytes.toString('utf8'));
  } catch {
    throw new WorkerError('MEDIA_FILE_INVALID', 'render manifest is not valid UTF-8 JSON', false);
  }
  if (!manifest || typeof manifest !== 'object' || Array.isArray(manifest)) {
    throw new WorkerError('MEDIA_FILE_INVALID', 'render manifest root must be an object', false);
  }
  const associations = [
    ['project_id', job.project_id],
    ['episode_id', job.episode_id],
    ['timeline_id', job.timeline_id],
  ];
  for (const [field, expected] of associations) {
    if (manifest[field] !== undefined && String(manifest[field]) !== String(expected)) {
      throw new WorkerError('SOURCE_VERSION_MISMATCH', `${field} does not match render_jobs`, false);
    }
  }
  const manifestVersion = manifest.timeline_version ?? manifest.generation_version ?? manifest.version;
  if (manifestVersion !== undefined && Number(manifestVersion) !== Number(job.timeline_version)) {
    throw new WorkerError('SOURCE_VERSION_MISMATCH', 'timeline_version does not match render_jobs', false);
  }
  if (manifest.render_type !== undefined && String(manifest.render_type) !== String(job.render_type)) {
    throw new WorkerError('SOURCE_VERSION_MISMATCH', 'render_type does not match render_jobs', false);
  }
  return { manifest, manifestPath: resolved.path };
}

function listFrom(value) {
  if (value === undefined || value === null) return [];
  if (!Array.isArray(value)) throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'manifest track collections must be arrays', false);
  return value;
}

function firstPath(item) {
  return item?.path || item?.source_path || item?.local_path || item?.storage_path || null;
}

function boolValue(value, fallback) {
  if (value === undefined || value === null || value === '') return fallback;
  if (typeof value === 'boolean') return value;
  return String(value).toLowerCase() === 'true';
}

async function prepareSubtitle(manifest) {
  const configNode = manifest.subtitles || manifest.subtitle_config || {};
  const burn = boolValue(
    configNode.burned ?? configNode.burn ?? manifest.subtitle_burned,
    config.burnSubtitles,
  );
  if (!burn) return null;
  const sourceValue = configNode.path || configNode.source_path || manifest.subtitle_path;
  if (!sourceValue) {
    const masterType = String(manifest.master_type || manifest.output?.master_type || '');
    if (masterType === 'subtitled' || masterType === 'final') {
      throw new WorkerError('SUBTITLE_CUE_INVALID', 'subtitle burn-in is enabled but no subtitle path was supplied', false);
    }
    return null;
  }
  const source = await resolveExistingFile(sourceValue, 'subtitle path');
  if (source.stat.size > config.maxSubtitleBytes) {
    throw new WorkerError('SUBTITLE_CUE_INVALID', 'subtitle file exceeds MEDIA_MAX_SUBTITLE_BYTES', false);
  }
  const extension = path.extname(source.path).toLowerCase();
  if (!['.srt', '.ass'].includes(extension)) {
    throw new WorkerError('SUBTITLE_CUE_INVALID', 'only .srt and .ass subtitle files are accepted', false);
  }
  const bytes = await fsp.readFile(source.path);
  const text = bytes.toString('utf8');
  if (text.includes('\uFFFD') || /\0/.test(text) || text.split(/\r?\n/).some((line) => line.length > 20000)) {
    throw new WorkerError('SUBTITLE_CUE_INVALID', 'subtitle file contains invalid UTF-8 or unsafe oversized lines', false);
  }
  const digest = crypto.createHash('sha256').update(bytes).digest('hex');
  const directory = await resolveOutputFile(path.join('.worker', 'subtitles', `${digest}${extension}`), 'worker subtitle copy');
  try {
    await fsp.access(directory, fs.constants.R_OK);
  } catch {
    await fsp.writeFile(directory, text.replace(/\r\n/g, '\n'), { mode: 0o600, flag: 'wx' }).catch(async (error) => {
      if (error.code !== 'EEXIST') throw error;
    });
  }
  return directory;
}

async function normalizeMedia(manifest) {
  const inputs = manifest.inputs || {};
  const tracks = manifest.tracks || {};
  const videoRaw = listFrom(inputs.video ?? tracks.video ?? manifest.video_segments);
  const combinedAudio = [];
  const genericAudio = listFrom(inputs.audio);
  for (const item of genericAudio) combinedAudio.push({ ...item, kind: item.kind || item.track_type || 'other' });
  for (const kind of ['dialogue', 'narration', 'bgm', 'sound_effect', 'ambience']) {
    for (const item of listFrom(tracks[kind])) combinedAudio.push({ ...item, kind });
  }
  if (videoRaw.length > 500 || combinedAudio.length > 1000) {
    throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'manifest has too many media inputs', false);
  }

  const videos = [];
  for (let index = 0; index < videoRaw.length; index += 1) {
    const item = videoRaw[index];
    if (!item || typeof item !== 'object' || Array.isArray(item)) {
      throw new WorkerError('TIMELINE_VALIDATION_FAILED', `video[${index}] is invalid`, false);
    }
    const source = await resolveExistingFile(firstPath(item), `video[${index}].path`);
    const timelineStartMs = numberField(item.timeline_start_ms ?? item.timelineStartMs, videos.at(-1)?.timelineEndMs || 0, 0, 86400000, `video[${index}].timeline_start_ms`, true);
    const sourceInMs = numberField(item.source_in_ms ?? item.sourceInMs, 0, 0, 86400000, `video[${index}].source_in_ms`, true);
    const explicitDuration = item.duration_ms ?? item.durationMs;
    const timelineEndMs = numberField(
      item.timeline_end_ms ?? item.timelineEndMs,
      explicitDuration === undefined ? NaN : timelineStartMs + Number(explicitDuration),
      timelineStartMs + 1,
      86400000,
      `video[${index}].timeline_end_ms`,
      true,
    );
    const durationMs = timelineEndMs - timelineStartMs;
    const sourceOutMs = numberField(
      item.source_out_ms ?? item.sourceOutMs,
      sourceInMs + durationMs,
      sourceInMs + 1,
      86400000,
      `video[${index}].source_out_ms`,
      true,
    );
    videos.push({
      path: source.path,
      shotId: item.shot_id || item.entity_id || null,
      sequenceNumber: numberField(item.sequence_number ?? item.sequenceNumber, index + 1, 1, 100000, `video[${index}].sequence_number`, true),
      timelineStartMs,
      timelineEndMs,
      durationMs,
      sourceInMs,
      sourceOutMs,
      stat: source.stat,
    });
  }
  videos.sort((left, right) => left.sequenceNumber - right.sequenceNumber);
  videos.forEach((item, index) => {
    if (item.sequenceNumber !== index + 1) throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'video sequence_number must be continuous', false);
  });

  const audio = [];
  for (let index = 0; index < combinedAudio.length; index += 1) {
    const item = combinedAudio[index];
    if (!item || typeof item !== 'object' || Array.isArray(item)) {
      throw new WorkerError('TIMELINE_VALIDATION_FAILED', `audio[${index}] is invalid`, false);
    }
    const kind = String(item.kind || 'other');
    if (!AUDIO_KINDS.has(kind)) throw new WorkerError('TIMELINE_VALIDATION_FAILED', `audio[${index}].kind is unsupported`, false);
    if (kind === 'bgm' && item.authorized !== true && item.licensed !== true && !boolValue(manifest.mock, false)) {
      throw new WorkerError('MEDIA_ASSETS_INCOMPLETE', 'BGM must be explicitly authorized before rendering', false);
    }
    const source = await resolveExistingFile(firstPath(item), `audio[${index}].path`);
    const timelineStartMs = numberField(item.timeline_start_ms ?? item.timelineStartMs, 0, 0, 86400000, `audio[${index}].timeline_start_ms`, true);
    const sourceInMs = numberField(item.source_in_ms ?? item.sourceInMs, 0, 0, 86400000, `audio[${index}].source_in_ms`, true);
    const durationMs = numberField(
      item.duration_ms ?? item.durationMs,
      Number(item.timeline_end_ms ?? item.timelineEndMs) - timelineStartMs,
      1,
      86400000,
      `audio[${index}].duration_ms`,
      true,
    );
    audio.push({
      path: source.path,
      kind,
      timelineStartMs,
      durationMs,
      sourceInMs,
      sourceOutMs: sourceInMs + durationMs,
      volume: numberField(item.volume, 1, 0, 4, `audio[${index}].volume`),
      fadeInMs: numberField(item.fade_in_ms ?? item.fadeInMs, 0, 0, durationMs, `audio[${index}].fade_in_ms`, true),
      fadeOutMs: numberField(item.fade_out_ms ?? item.fadeOutMs, 0, 0, durationMs, `audio[${index}].fade_out_ms`, true),
      stat: source.stat,
    });
  }
  const subtitlePath = await prepareSubtitle(manifest);
  const videoDurationMs = videos.length ? videos.at(-1).timelineEndMs : 0;
  if (videoDurationMs && audio.some((track) => track.timelineStartMs + track.durationMs > videoDurationMs + 100)) {
    throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'audio track exceeds the video timeline duration', false);
  }
  const transitions = listFrom(manifest.transitions);
  if (transitions.length > Math.max(0, videos.length - 1)) {
    throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'too many transitions for the video timeline', false);
  }
  return {
    videos,
    audio,
    subtitlePath,
    transitions,
    duckingEnabled: boolValue(manifest.audio?.ducking_enabled ?? manifest.render_config?.bgm_ducking_enabled, true),
  };
}

async function validateInputMedia(media, log) {
  for (let index = 0; index < media.videos.length; index += 1) {
    const item = media.videos[index];
    const summary = probeSummary(await parseProbe(item.path, { log }));
    if (!summary.hasVideo || summary.durationMs <= 0) {
      throw new WorkerError('MEDIA_FILE_INVALID', `video[${index}] has no decodable video stream`, false);
    }
    if (item.sourceOutMs > summary.durationMs + 100) {
      throw new WorkerError('TIMELINE_VALIDATION_FAILED', `video[${index}].source_out_ms exceeds source duration`, false);
    }
  }
  for (let index = 0; index < media.audio.length; index += 1) {
    const item = media.audio[index];
    const summary = probeSummary(await parseProbe(item.path, { log }));
    if (!summary.hasAudio || summary.durationMs <= 0) {
      throw new WorkerError('MEDIA_FILE_INVALID', `audio[${index}] has no decodable audio stream`, false);
    }
    if (item.sourceOutMs > summary.durationMs + 100) {
      throw new WorkerError('TIMELINE_VALIDATION_FAILED', `audio[${index}] source range exceeds source duration`, false);
    }
  }
}

function mapMasterType(job, manifest) {
  const explicit = String(manifest.master_type || manifest.output?.master_type || '');
  if (explicit) {
    if (!MASTER_TYPES.has(explicit)) throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'master_type is unsupported', false);
    return explicit;
  }
  if (job.render_type === 'preview') return 'preview';
  if (job.render_type === 'subtitle_preview') return 'subtitled';
  if (job.render_type === 'master') return boolValue(
    manifest.subtitle_burned ?? manifest.subtitles?.burned ?? manifest.subtitles?.burn,
    config.burnSubtitles,
  ) ? 'final' : 'clean';
  throw new WorkerError('TIMELINE_VALIDATION_FAILED', `render_type ${job.render_type} does not produce an episode master`, false);
}

function aspectRatio(width, height) {
  const gcd = (left, right) => (right ? gcd(right, left % right) : left);
  const divisor = gcd(width, height);
  return `${width / divisor}:${height / divisor}`;
}

function normalizeProcessError(error) {
  if (error instanceof WorkerError || error instanceof TemplateError) return error;
  if (error?.code === 'ENOENT') {
    return new WorkerError('FFMPEG_NOT_AVAILABLE', 'media executable is not available', false);
  }
  return new WorkerError('FFMPEG_FAILED', safeMessage(error), true);
}

async function defaultLogPath(job) {
  const relative = path.join('logs', 'render', String(job.project_id).replace(/[^A-Za-z0-9_-]/g, '_'), `${job.render_job_id}.log`);
  return resolveOutputFile(job.log_path || relative, 'log_path');
}

async function writeRenderStatus(job, manifestPath, data) {
  try {
    const filePath = await resolveOutputFile(path.join('logs', 'render-status', `${job.render_job_id}.json`), 'render status manifest');
    const body = {
      schema_version: '1.0',
      render_job_id: job.render_job_id,
      project_id: job.project_id,
      episode_id: job.episode_id,
      timeline_id: job.timeline_id,
      timeline_version: Number(job.timeline_version),
      render_type: job.render_type,
      dynamic_render_completed: data.status === 'succeeded',
      status: data.status,
      manifest_path: manifestPath ? storageRelative(manifestPath) : storageRelative(rawPath(job.input_manifest_path, 'input_manifest_path')),
      output_path: data.outputPath ? storageRelative(data.outputPath) : null,
      master_id: data.masterId || null,
      content_hash: data.contentHash || null,
      error: data.error ? {
        code: data.error.code,
        message: safeMessage(data.error),
        retryable: Boolean(data.error.retryable),
      } : null,
      updated_at: new Date().toISOString(),
    };
    await fsp.writeFile(filePath, `${JSON.stringify(body, null, 2)}\n`, { mode: 0o600 });
    return filePath;
  } catch (error) {
    console.error(JSON.stringify({ level: 'error', event: 'render_status_write_failed', render_job_id: job.render_job_id, message: safeMessage(error) }));
    return null;
  }
}

async function recoverStaleJobs() {
  const staleSeconds = Math.ceil(config.renderTimeoutMs / 1000) + Math.ceil(config.heartbeatMs / 1000) * 2;
  const result = await pool.query(`
    UPDATE drama.render_jobs
       SET retry_count = LEAST(retry_count + 1, max_retries),
           status = CASE WHEN retry_count < max_retries THEN 'pending' ELSE 'timeout' END,
           worker_id = NULL,
           progress = CASE WHEN retry_count < max_retries THEN 0 ELSE progress END,
           started_at = CASE WHEN retry_count < max_retries THEN NULL ELSE started_at END,
           heartbeat_at = NULL,
           completed_at = CASE WHEN retry_count < max_retries THEN NULL ELSE now() END,
           error_code = 'RENDER_TIMEOUT',
           error_message = 'worker heartbeat expired; task recovered safely',
           updated_at = now()
     WHERE status IN ('claimed','processing')
       AND COALESCE(heartbeat_at, started_at, created_at) < now() - make_interval(secs => $1)
       AND (worker_id IS NULL OR worker_id <> $2)
    RETURNING render_job_id, status`, [staleSeconds, workerId]);
  if (result.rowCount) {
    console.warn(JSON.stringify({ level: 'warn', event: 'stale_jobs_recovered', count: result.rowCount }));
  }
}

async function claimJobs(renderJobId, batchSize) {
  const client = await pool.connect();
  try {
    await client.query('BEGIN');
    const result = await client.query(`
      WITH candidates AS (
        SELECT id
          FROM drama.render_jobs
         WHERE status = 'pending'
           AND ($1::text IS NULL OR render_job_id = $1)
         ORDER BY created_at, id
         LIMIT $2
         FOR UPDATE SKIP LOCKED
      )
      UPDATE drama.render_jobs AS job
         SET status = 'claimed',
             worker_id = $3,
             progress = 0,
             started_at = now(),
             heartbeat_at = now(),
             completed_at = NULL,
             error_code = NULL,
             error_message = NULL,
             updated_at = now()
        FROM candidates
       WHERE job.id = candidates.id
      RETURNING job.*`, [renderJobId, batchSize, workerId]);
    await client.query('COMMIT');
    return result.rows;
  } catch (error) {
    await client.query('ROLLBACK').catch(() => {});
    throw error;
  } finally {
    client.release();
  }
}

async function requeueForExplicitRetry(renderJobId) {
  await pool.query(`
    UPDATE drama.render_jobs
       SET status = 'pending',
           worker_id = NULL,
           progress = 0,
           started_at = NULL,
           heartbeat_at = NULL,
           completed_at = NULL,
           error_code = NULL,
           error_message = NULL,
           updated_at = now()
     WHERE render_job_id = $1
       AND status IN ('failed','timeout')
       AND retry_count <= max_retries`, [renderJobId]);
}

async function jobState(renderJobId) {
  const result = await pool.query(`
    SELECT render_job_id, trace_id, project_id, episode_id, timeline_id, timeline_version,
           render_type, status, progress, worker_id, command_template_id,
           input_manifest_path, output_path, output_url, log_path,
           retry_count, max_retries, started_at, heartbeat_at, completed_at,
           error_code, error_message, created_at, updated_at
      FROM drama.render_jobs
     WHERE render_job_id = $1`, [renderJobId]);
  return result.rows[0] || null;
}

async function heartbeat(job, progress) {
  await pool.query(`
    UPDATE drama.render_jobs
       SET heartbeat_at = now(), progress = LEAST(99, GREATEST(progress, $3)), updated_at = now()
     WHERE render_job_id = $1 AND worker_id = $2 AND status = 'processing'`,
  [job.render_job_id, workerId, Math.max(1, Math.min(99, Math.round(progress)))]);
}

async function persistSuccess({ job, manifest, outputPath, logPath, probe, contentHash, templateId, subtitlePath }) {
  const summary = probeSummary(probe);
  if (!summary.hasVideo || !summary.hasAudio || summary.durationMs <= 0 || summary.fileSizeBytes < 1024) {
    throw new WorkerError('OUTPUT_FILE_INVALID', 'render output is missing a valid video/audio stream', true);
  }
  const masterType = mapMasterType(job, manifest);
  const suppliedMasterId = manifest.master_id || manifest.output?.master_id;
  const masterId = suppliedMasterId
    ? assertId(suppliedMasterId, 'master_id')
    : `master_${crypto.createHash('sha256').update(job.render_job_id).digest('hex').slice(0, 24)}`;
  const subtitleUrl = subtitlePath ? publicUrl(subtitlePath) : null;
  const outputUrl = publicUrl(outputPath);
  const settings = normalizeSettings(manifest);
  const client = await pool.connect();
  try {
    await client.query('BEGIN');
    const locked = await client.query('SELECT status FROM drama.render_jobs WHERE render_job_id=$1 FOR UPDATE', [job.render_job_id]);
    if (!locked.rows[0]) throw new WorkerError('DATABASE_WRITE_FAILED', 'render job disappeared before completion', true);
    if (locked.rows[0].status === 'cancelled') throw new WorkerError('RENDER_CANCELLED', 'render job was cancelled', false);
    await client.query(`
      UPDATE drama.episode_masters
         SET is_current = false, updated_at = now()
       WHERE project_id = $1 AND episode_id = $2 AND master_type = $3 AND is_current`,
    [job.project_id, job.episode_id, masterType]);
    await client.query(`
      INSERT INTO drama.episode_masters(
        master_id, render_job_id, project_id, episode_id, timeline_id, generation_version,
        master_type, storage_url, local_path, thumbnail_url, subtitle_url, subtitle_burned,
        width, height, aspect_ratio, fps, duration_ms, file_size_bytes,
        video_codec, audio_codec, sample_rate, loudness_lufs, peak_db, content_hash,
        status, is_current, created_at, updated_at
      ) VALUES (
        $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,
        $13,$14,$15,$16,$17,$18,$19,$20,$21,NULL,NULL,$22,'ready',true,now(),now()
      )
      ON CONFLICT(master_id) DO UPDATE SET
        render_job_id=EXCLUDED.render_job_id, storage_url=EXCLUDED.storage_url,
        local_path=EXCLUDED.local_path, thumbnail_url=EXCLUDED.thumbnail_url,
        subtitle_url=EXCLUDED.subtitle_url, subtitle_burned=EXCLUDED.subtitle_burned,
        width=EXCLUDED.width, height=EXCLUDED.height, aspect_ratio=EXCLUDED.aspect_ratio,
        fps=EXCLUDED.fps, duration_ms=EXCLUDED.duration_ms,
        file_size_bytes=EXCLUDED.file_size_bytes, video_codec=EXCLUDED.video_codec,
        audio_codec=EXCLUDED.audio_codec, sample_rate=EXCLUDED.sample_rate,
        content_hash=EXCLUDED.content_hash, status='ready', is_current=true, updated_at=now()`, [
      masterId,
      job.render_job_id,
      job.project_id,
      job.episode_id,
      job.timeline_id,
      Number(job.timeline_version),
      masterType,
      outputUrl,
      outputPath,
      null,
      subtitleUrl,
      Boolean(subtitlePath),
      summary.width,
      summary.height,
      manifest.settings?.aspect_ratio || manifest.output?.aspect_ratio || aspectRatio(summary.width, summary.height),
      summary.fps || settings.fps,
      summary.durationMs,
      summary.fileSizeBytes,
      summary.videoCodec,
      summary.audioCodec,
      summary.sampleRate,
      contentHash,
    ]);
    await client.query(`
      UPDATE drama.render_jobs
         SET status='succeeded', progress=100, output_path=$2, output_url=$3, log_path=$4,
             command_template_id=$5, heartbeat_at=now(), completed_at=now(),
             error_code=NULL, error_message=NULL, updated_at=now()
       WHERE render_job_id=$1`, [job.render_job_id, outputPath, outputUrl, logPath, templateId]);
    await client.query('COMMIT');
    return { masterId, masterType, outputUrl, summary };
  } catch (error) {
    await client.query('ROLLBACK').catch(() => {});
    throw error;
  } finally {
    client.release();
  }
}

async function persistFailure(job, error, logPath) {
  const normalized = normalizeProcessError(error);
  if (normalized.code === 'RENDER_CANCELLED') {
    await pool.query(`
      UPDATE drama.render_jobs
         SET status='cancelled', completed_at=COALESCE(completed_at,now()), heartbeat_at=now(),
             error_code='RENDER_CANCELLED', error_message=$2, log_path=$3, updated_at=now()
       WHERE render_job_id=$1 AND status <> 'succeeded'`,
    [job.render_job_id, safeMessage(normalized).slice(0, 1000), logPath]);
    return { normalized, status: 'cancelled' };
  }
  const currentRetry = Number(job.retry_count || 0);
  const maxRetries = Number(job.max_retries || 0);
  const retry = normalized.retryable && currentRetry < maxRetries;
  const attempt = retry ? currentRetry + 1 : Math.min(currentRetry, maxRetries);
  const terminalStatus = normalized.code === 'RENDER_TIMEOUT' ? 'timeout' : 'failed';
  const status = retry ? 'pending' : terminalStatus;
  const exitSuffix = normalized.details?.exit_code ?? normalized.details?.code;
  const message = `${safeMessage(normalized)}${exitSuffix !== undefined ? ` [exit=${exitSuffix}]` : ''}`.slice(0, 1000);
  await pool.query(`
    UPDATE drama.render_jobs
       SET status=$2, retry_count=$3, worker_id=CASE WHEN $4 THEN NULL ELSE worker_id END,
           progress=CASE WHEN $4 THEN 0 ELSE progress END,
           started_at=CASE WHEN $4 THEN NULL ELSE started_at END,
           heartbeat_at=CASE WHEN $4 THEN NULL ELSE now() END,
           completed_at=CASE WHEN $4 THEN NULL ELSE now() END,
           error_code=$5, error_message=$6, log_path=$7, updated_at=now()
     WHERE render_job_id=$1 AND status <> 'cancelled'`,
  [job.render_job_id, status, attempt, retry, normalized.code || 'FFMPEG_FAILED', message, logPath]);
  return { normalized, status };
}

async function processRenderJob(job) {
  const context = { job, child: null, cancelled: false, progress: 1, cancel: null };
  activeJobs.set(job.render_job_id, context);
  let log = null;
  let logPath = null;
  let manifest = null;
  let manifestPath = null;
  let heartbeatTimer = null;
  try {
    logPath = await defaultLogPath(job);
    log = createBoundedLog(logPath);
    log.write(`\n${new Date().toISOString()} render_job=${job.render_job_id} worker=${workerId}\n`);
    const processing = await pool.query(`
      UPDATE drama.render_jobs
         SET status='processing', log_path=$3, heartbeat_at=now(), progress=1, updated_at=now()
       WHERE render_job_id=$1 AND worker_id=$2 AND status='claimed'
      RETURNING render_job_id`, [job.render_job_id, workerId, logPath]);
    if (!processing.rowCount) throw new WorkerError('RENDER_JOB_ALREADY_RUNNING', 'claimed job no longer belongs to this worker', false);
    heartbeatTimer = setInterval(() => {
      heartbeat(job, context.progress).catch((error) => {
        log?.write(`[heartbeat-error] ${safeMessage(error)}\n`);
      });
    }, config.heartbeatMs);
    heartbeatTimer.unref();

    ({ manifest, manifestPath } = await loadManifest(job));
    const explicitMock = boolValue(manifest.mock, false) || String(manifest.mode || '').toLowerCase() === 'mock';
    if (explicitMock && !config.mockMode) {
      throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'mock rendering is disabled for this worker', false);
    }
    if (!tools.ffmpeg.available) {
      throw new WorkerError('FFMPEG_NOT_AVAILABLE', 'ffmpeg is not available; a static render-status manifest was preserved', false);
    }
    if (!tools.ffprobe.available) {
      throw new WorkerError('FFPROBE_NOT_AVAILABLE', 'ffprobe is not available; render success cannot be verified', false);
    }

    const outputValue = manifest.output?.path || manifest.output_path || job.output_path;
    if (!outputValue) throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'output_path is required', false);
    const outputPath = await resolveOutputFile(outputValue, 'output_path');
    const jobOutput = job.output_path ? await resolveOutputFile(job.output_path, 'render_jobs.output_path') : outputPath;
    if (outputPath !== jobOutput) throw new WorkerError('SOURCE_VERSION_MISMATCH', 'manifest output path does not match render_jobs.output_path', false);
    if (path.extname(outputPath).toLowerCase() !== '.mp4') {
      throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'video master output must be an .mp4 file', false);
    }
    const settings = normalizeSettings(manifest);
    let media = { videos: [], audio: [], subtitlePath: null, duckingEnabled: true };
    if (!explicitMock) {
      media = await normalizeMedia(manifest);
      await validateInputMedia(media, log);
    }
    const partialPath = await resolveOutputFile(
      path.join(path.dirname(outputPath), `.${path.basename(outputPath)}.${job.render_job_id}.partial.mp4`),
      'partial output path',
    );
    const plan = buildRenderPlan({ manifest, media, outputPath: partialPath, settings, mock: explicitMock });
    await pool.query('UPDATE drama.render_jobs SET command_template_id=$2, updated_at=now() WHERE render_job_id=$1', [job.render_job_id, plan.templateId]);
    log.write(`[template] ${plan.templateId} duration_ms=${plan.totalDurationMs}\n`);

    let existing = false;
    try {
      const stat = await fsp.stat(outputPath);
      existing = stat.isFile() && stat.size > 0;
    } catch (error) {
      if (error.code !== 'ENOENT') throw error;
    }
    if (existing && Number(job.retry_count || 0) === 0) {
      throw new WorkerError('OUTPUT_FILE_INVALID', 'output path already exists for a first render attempt', false);
    }

    let outputProbe;
    if (existing) {
      log.write('[recovery] validating existing output from an earlier attempt\n');
      outputProbe = await parseProbe(outputPath, { log });
    } else {
      if (manifest.simulate === 'ffmpeg_failure') {
        throw new WorkerError('FFMPEG_FAILED', 'simulated FFmpeg failure requested by mock manifest', true);
      }
      const simulateTimeout = manifest.simulate === 'ffmpeg_timeout';
      let progressBuffer = '';
      let outTimeUs = 0;
      await runProcess(config.ffmpeg, plan.args, {
        timeoutMs: simulateTimeout ? 25 : config.renderTimeoutMs,
        log,
        onChild(child, cancel) {
          context.child = child;
          context.cancel = cancel;
          if (context.cancelled) cancel();
        },
        onStdoutChunk(chunk) {
          progressBuffer += String(chunk);
          const lines = progressBuffer.split(/\r?\n/);
          progressBuffer = lines.pop() || '';
          for (const line of lines) {
            const separator = line.indexOf('=');
            if (separator < 0) continue;
            const key = line.slice(0, separator);
            const value = line.slice(separator + 1);
            if (key === 'out_time_us' || key === 'out_time_ms') outTimeUs = Math.max(outTimeUs, Number(value) || 0);
            if (key === 'progress' && value === 'end') context.progress = 95;
          }
          if (plan.totalDurationMs > 0 && outTimeUs > 0) {
            context.progress = Math.max(context.progress, Math.min(94, 5 + (outTimeUs / (plan.totalDurationMs * 1000)) * 89));
          }
        },
      });
      if (context.cancelled) throw new WorkerError('RENDER_CANCELLED', 'render cancelled by request', false);
      outputProbe = await parseProbe(partialPath, { log });
      const partialSummary = probeSummary(outputProbe);
      if (!partialSummary.hasVideo || !partialSummary.hasAudio || partialSummary.durationMs <= 0 || partialSummary.fileSizeBytes < 1024) {
        throw new WorkerError('OUTPUT_FILE_INVALID', 'FFmpeg output failed post-render validation', true);
      }
      await fsp.rename(partialPath, outputPath);
    }
    context.progress = 97;
    const finalStat = await fsp.stat(outputPath);
    if (!finalStat.isFile() || finalStat.size < 1024) throw new WorkerError('OUTPUT_FILE_INVALID', 'final output file is invalid', true);
    const contentHash = await sha256File(outputPath);
    const persisted = await persistSuccess({
      job,
      manifest,
      outputPath,
      logPath,
      probe: outputProbe,
      contentHash,
      templateId: plan.templateId,
      subtitlePath: media.subtitlePath,
    });
    await writeRenderStatus(job, manifestPath, {
      status: 'succeeded', outputPath, masterId: persisted.masterId, contentHash,
    });
    log.write(`[success] master_id=${persisted.masterId} sha256=${contentHash}\n`);
    console.info(JSON.stringify({ level: 'info', event: 'render_succeeded', render_job_id: job.render_job_id, master_id: persisted.masterId }));
  } catch (caught) {
    const error = normalizeProcessError(caught);
    log?.write(`[failure] code=${error.code} retryable=${Boolean(error.retryable)} message=${safeMessage(error)}\n`);
    let state = 'failed';
    try {
      const persisted = await persistFailure(job, error, logPath);
      state = persisted.status;
    } catch (dbError) {
      log?.write(`[database-write-failure] ${safeMessage(dbError)}\n`);
      console.error(JSON.stringify({ level: 'error', event: 'render_failure_not_persisted', render_job_id: job.render_job_id, message: safeMessage(dbError) }));
    }
    await writeRenderStatus(job, manifestPath, { status: state, error });
    console.error(JSON.stringify({ level: 'error', event: 'render_failed', render_job_id: job.render_job_id, code: error.code, retryable: Boolean(error.retryable), status: state }));
  } finally {
    if (heartbeatTimer) clearInterval(heartbeatTimer);
    if (log) await log.close().catch(() => {});
    activeJobs.delete(job.render_job_id);
  }
}

function dispatchJobs(jobs) {
  for (const job of jobs) {
    if (activeJobs.has(job.render_job_id)) continue;
    processRenderJob(job).catch((error) => {
      console.error(JSON.stringify({ level: 'error', event: 'unhandled_render_error', render_job_id: job.render_job_id, message: safeMessage(error) }));
    });
  }
}

async function pollOnce() {
  if (!config.enabled || shuttingDown || polling) return;
  polling = true;
  try {
    const now = Date.now();
    if (now - lastStaleRecoveryAt > Math.max(config.renderTimeoutMs, 60000)) {
      await recoverStaleJobs();
      lastStaleRecoveryAt = now;
    }
    const capacity = Math.max(0, config.concurrency - activeJobs.size);
    if (capacity > 0) {
      const jobs = await claimJobs(null, Math.min(capacity, config.batchSize));
      dispatchJobs(jobs);
    }
  } catch (error) {
    console.error(JSON.stringify({ level: 'error', event: 'poll_failed', message: safeMessage(error) }));
  } finally {
    polling = false;
  }
}

function parseBlackAndSilence(stderr) {
  const black = [...String(stderr).matchAll(/black_start:([0-9.]+)\s+black_end:([0-9.]+)\s+black_duration:([0-9.]+)/g)]
    .map((match) => ({ start_seconds: Number(match[1]), end_seconds: Number(match[2]), duration_seconds: Number(match[3]) }));
  const silenceStarts = [...String(stderr).matchAll(/silence_start:\s*([0-9.]+)/g)].map((match) => Number(match[1]));
  const silenceEnds = [...String(stderr).matchAll(/silence_end:\s*([0-9.]+)\s*\|\s*silence_duration:\s*([0-9.]+)/g)]
    .map((match, index) => ({ start_seconds: silenceStarts[index] ?? null, end_seconds: Number(match[1]), duration_seconds: Number(match[2]) }));
  const meanMatch = [...String(stderr).matchAll(/mean_volume:\s*(-?(?:inf|[0-9.]+))\s*dB/gi)].at(-1);
  const maxMatch = [...String(stderr).matchAll(/max_volume:\s*(-?(?:inf|[0-9.]+))\s*dB/gi)].at(-1);
  const parseVolume = (match) => {
    if (!match) return null;
    return match[1].toLowerCase() === '-inf' ? null : Number(match[1]);
  };
  return { black, silence: silenceEnds, meanVolumeDb: parseVolume(meanMatch), maxVolumeDb: parseVolume(maxMatch) };
}

async function resolveQcTarget(request) {
  const masterId = request.master_id ? assertId(request.master_id, 'master_id') : null;
  if (masterId) {
    const result = await pool.query(`
      SELECT master_id, project_id, episode_id, local_path, storage_url, content_hash, status
        FROM drama.episode_masters WHERE master_id=$1`, [masterId]);
    if (!result.rows[0]) throw new WorkerError('MEDIA_FILE_NOT_FOUND', 'master_id was not found', false);
    if (result.rows[0].status !== 'ready') {
      throw new WorkerError('MEDIA_FILE_INVALID', 'master is not ready for technical QC', false);
    }
    const resolved = await resolveExistingFile(result.rows[0].local_path, 'episode_masters.local_path');
    return { ...result.rows[0], local_path: resolved.path };
  }
  if (!request.local_path) throw new WorkerError('MEDIA_FILE_NOT_FOUND', 'master_id or local_path is required', false);
  const resolved = await resolveExistingFile(request.local_path, 'local_path');
  return { master_id: null, project_id: null, episode_id: null, local_path: resolved.path, content_hash: null, status: 'ready' };
}

async function runTechnicalQc(request) {
  if (qcRunning >= config.qcConcurrency) throw new WorkerError('QC_TECHNICAL_FAILED', 'technical QC worker is busy', true);
  if (!tools.ffmpeg.available) throw new WorkerError('FFMPEG_NOT_AVAILABLE', 'ffmpeg is not available', false);
  if (!tools.ffprobe.available) throw new WorkerError('FFPROBE_NOT_AVAILABLE', 'ffprobe is not available', false);
  qcRunning += 1;
  try {
    const target = await resolveQcTarget(request);
    const probe = await parseProbe(target.local_path, { timeoutMs: config.qcTimeoutMs });
    const summary = probeSummary(probe);
    const analysisArgs = ['-hide_banner', '-nostdin', '-v', 'info', '-i', target.local_path];
    if (summary.hasVideo) analysisArgs.push('-vf', `blackdetect=d=${config.blackSeconds}:pic_th=0.98:pix_th=0.10`);
    if (summary.hasAudio) analysisArgs.push('-af', `silencedetect=n=-50dB:d=${config.silenceSeconds},volumedetect`);
    analysisArgs.push('-f', 'null', '-');
    const analysis = await runProcess(config.ffmpeg, analysisArgs, { timeoutMs: config.qcTimeoutMs, maxCapture: 4194304 });
    const parsed = parseBlackAndSilence(analysis.stderr);

    const frameHash = crypto.createHash('sha256');
    let frameBuffer = '';
    let frameCount = 0;
    let duplicateFrames = 0;
    let previousHash = null;
    if (summary.hasVideo) {
      await runProcess(config.ffmpeg, [
        '-hide_banner', '-nostdin', '-v', 'error', '-i', target.local_path,
        '-map', '0:v:0', '-an', '-f', 'framemd5', '-',
      ], {
        timeoutMs: config.qcTimeoutMs,
        captureStdout: false,
        onStdoutChunk(chunk) {
          frameHash.update(chunk);
          frameBuffer += String(chunk);
          const lines = frameBuffer.split(/\r?\n/);
          frameBuffer = lines.pop() || '';
          for (const line of lines) {
            if (!line || line.startsWith('#')) continue;
            const fields = line.split(',').map((value) => value.trim());
            const digest = fields.at(-1);
            if (digest && digest === previousHash) duplicateFrames += 1;
            if (digest) previousHash = digest;
            frameCount += 1;
          }
        },
      });
    }
    const maxBlack = parsed.black.reduce((maximum, item) => Math.max(maximum, item.duration_seconds), 0);
    const maxSilence = parsed.silence.reduce((maximum, item) => Math.max(maximum, item.duration_seconds), 0);
    const avDriftMs = summary.videoDurationMs && summary.audioDurationMs
      ? Math.abs(summary.videoDurationMs - summary.audioDurationMs)
      : null;
    const blockingIssues = [];
    if (!summary.hasVideo) blockingIssues.push({ code: 'VIDEO_STREAM_MISSING', severity: 'failed' });
    if (!summary.hasAudio) blockingIssues.push({ code: 'AUDIO_STREAM_MISSING', severity: 'failed' });
    if (summary.durationMs <= 0 || summary.fileSizeBytes < 1024) blockingIssues.push({ code: 'MEDIA_FILE_INVALID', severity: 'failed' });
    if (maxBlack > config.blackSeconds) blockingIssues.push({ code: 'BLACK_FRAME_TOO_LONG', severity: 'failed', duration_seconds: maxBlack });
    if (maxSilence > config.silenceSeconds) blockingIssues.push({ code: 'SILENCE_TOO_LONG', severity: 'failed', duration_seconds: maxSilence });
    const warnings = [];
    if (avDriftMs !== null && avDriftMs > Number(process.env.QC_AUDIO_VIDEO_DRIFT_MS || 200)) {
      warnings.push({ code: 'AUDIO_VIDEO_DRIFT', drift_ms: Math.round(avDriftMs) });
    }
    if (frameCount && duplicateFrames / frameCount > 0.5) {
      warnings.push({ code: 'HIGH_DUPLICATE_FRAME_RATIO', ratio: duplicateFrames / frameCount });
    }
    return {
      success: blockingIssues.length === 0,
      status: blockingIssues.length ? 'failed' : warnings.length ? 'warning' : 'passed',
      master_id: target.master_id,
      project_id: target.project_id,
      episode_id: target.episode_id,
      technical_report: {
        tool_versions: { ffmpeg: tools.ffmpeg.version, ffprobe: tools.ffprobe.version },
        stream: summary,
        black_frames: parsed.black,
        silence: parsed.silence,
        audio: { mean_volume_db: parsed.meanVolumeDb, max_volume_db: parsed.maxVolumeDb },
        frames: {
          count: frameCount,
          consecutive_duplicate_count: duplicateFrames,
          framemd5_sha256: frameHash.digest('hex'),
        },
        av_drift_ms: avDriftMs === null ? null : Math.round(avDriftMs),
        blocking_issues: blockingIssues,
        warnings,
      },
      error: blockingIssues.length ? {
        code: 'QC_TECHNICAL_FAILED',
        message: 'technical QC found blocking media issues',
        retryable: false,
      } : null,
    };
  } finally {
    qcRunning -= 1;
  }
}

function jsonResponse(response, statusCode, body) {
  const payload = Buffer.from(JSON.stringify(body));
  response.writeHead(statusCode, {
    'Content-Type': 'application/json; charset=utf-8',
    'Content-Length': payload.length,
    'Cache-Control': 'no-store',
    'X-Content-Type-Options': 'nosniff',
  });
  response.end(payload);
}

function requestAuthorized(request) {
  if (!config.token) return true;
  const provided = String(request.headers['x-media-worker-token'] || '');
  const expectedBuffer = Buffer.from(config.token);
  const providedBuffer = Buffer.from(provided);
  return expectedBuffer.length === providedBuffer.length && crypto.timingSafeEqual(expectedBuffer, providedBuffer);
}

async function readJsonBody(request) {
  const chunks = [];
  let size = 0;
  for await (const chunk of request) {
    size += chunk.length;
    if (size > 131072) throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'request body is too large', false);
    chunks.push(chunk);
  }
  if (!chunks.length) return {};
  try {
    const value = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('object required');
    return value;
  } catch {
    throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'request body must be a JSON object', false);
  }
}

function errorResponse(error) {
  const normalized = normalizeProcessError(error);
  let statusCode = normalized.retryable ? 503 : 400;
  if (normalized.code === 'MEDIA_FILE_NOT_FOUND') statusCode = 404;
  if (normalized.code === 'RENDER_JOB_ALREADY_RUNNING') statusCode = 409;
  if (normalized.code === 'RENDER_CANCELLED') statusCode = 409;
  return {
    statusCode,
    body: {
      success: false,
      worker_id: workerId,
      error: {
        code: normalized.code || 'MEDIA_WORKER_ERROR',
        message: safeMessage(normalized),
        retryable: Boolean(normalized.retryable),
      },
    },
  };
}

async function healthResponse() {
  let database = { available: false, error: null };
  try {
    await pool.query('SELECT 1');
    database = { available: true, error: null };
  } catch (error) {
    database.error = safeMessage(error);
  }
  const healthy = database.available && tools.ffmpeg.available && tools.ffprobe.available;
  return {
    healthy,
    body: {
      success: healthy,
      status: !config.enabled ? 'disabled' : healthy ? 'healthy' : 'degraded',
      worker_id: workerId,
      enabled: config.enabled,
      active_jobs: activeJobs.size,
      max_concurrency: config.concurrency,
      qc_running: qcRunning,
      database,
      tools,
      storage: { root: '$MEDIA_STORAGE', writable: true },
      time: new Date().toISOString(),
    },
  };
}

async function handleRequest(request, response) {
  if (!requestAuthorized(request)) {
    jsonResponse(response, 401, { success: false, error: { code: 'MEDIA_WORKER_UNAUTHORIZED', message: 'invalid worker token', retryable: false } });
    return;
  }
  const url = new URL(request.url, `http://${request.headers.host || 'localhost'}`);
  try {
    if (request.method === 'GET' && url.pathname === '/health') {
      const health = await healthResponse();
      jsonResponse(response, health.healthy || !config.enabled ? 200 : 503, health.body);
      return;
    }
    if (request.method === 'POST' && url.pathname === '/jobs/claim-or-run') {
      if (!config.enabled) throw new WorkerError('MEDIA_WORKER_DISABLED', 'media worker is disabled', false);
      const body = await readJsonBody(request);
      const renderJobId = assertId(body.render_job_id, 'render_job_id', true);
      const action = String(body.action || 'run');
      if (!['run', 'render', 'resume', 'retry'].includes(action)) {
        throw new WorkerError('TIMELINE_VALIDATION_FAILED', 'claim action is unsupported', false);
      }
      if (renderJobId && action === 'retry') await requeueForExplicitRetry(renderJobId);
      const capacity = Math.max(0, config.concurrency - activeJobs.size);
      if (capacity === 0) {
        jsonResponse(response, 202, { success: true, accepted: false, status: 'busy', worker_id: workerId, claimed: [] });
        return;
      }
      const requestedBatch = numberField(body.batch_size, 1, 1, config.batchSize, 'batch_size', true);
      const jobs = await claimJobs(renderJobId, Math.min(capacity, requestedBatch));
      dispatchJobs(jobs);
      if (jobs.length) {
        jsonResponse(response, 202, {
          success: true,
          accepted: true,
          status: 'claimed',
          worker_id: workerId,
          claimed: jobs.map((job) => ({ render_job_id: job.render_job_id, status: job.status })),
        });
        return;
      }
      if (renderJobId) {
        const current = await jobState(renderJobId);
        if (!current) throw new WorkerError('MEDIA_FILE_NOT_FOUND', 'render_job_id was not found', false);
        jsonResponse(response, TERMINAL_JOB_STATUSES.has(current.status) ? 200 : 409, {
          success: current.status === 'succeeded',
          accepted: false,
          status: current.status,
          worker_id: workerId,
          job: current,
          error: current.status === 'succeeded' ? null : {
            code: current.status === 'claimed' || current.status === 'processing' ? 'RENDER_JOB_ALREADY_RUNNING' : current.error_code || 'RENDER_JOB_NOT_PENDING',
            message: current.error_message || `render job is ${current.status}`,
            retryable: current.status === 'claimed' || current.status === 'processing',
          },
        });
        return;
      }
      jsonResponse(response, 200, { success: true, accepted: false, status: 'idle', worker_id: workerId, claimed: [] });
      return;
    }
    const jobMatch = url.pathname.match(/^\/jobs\/([A-Za-z0-9][A-Za-z0-9_.:-]{0,127})$/);
    if (request.method === 'GET' && jobMatch) {
      const current = await jobState(jobMatch[1]);
      if (!current) throw new WorkerError('MEDIA_FILE_NOT_FOUND', 'render_job_id was not found', false);
      jsonResponse(response, 200, { success: true, worker_id: workerId, job: current });
      return;
    }
    const cancelMatch = url.pathname.match(/^\/jobs\/([A-Za-z0-9][A-Za-z0-9_.:-]{0,127})\/cancel$/);
    if (request.method === 'POST' && cancelMatch) {
      const renderJobId = cancelMatch[1];
      const active = activeJobs.get(renderJobId);
      if (active) {
        active.cancelled = true;
        active.cancel?.();
      }
      const result = await pool.query(`
        UPDATE drama.render_jobs
           SET status='cancelled', completed_at=now(), heartbeat_at=now(),
               error_code='RENDER_CANCELLED', error_message='cancelled by explicit request', updated_at=now()
         WHERE render_job_id=$1 AND status IN ('pending','claimed','processing')
        RETURNING render_job_id, status`, [renderJobId]);
      if (!result.rowCount) {
        const current = await jobState(renderJobId);
        if (!current) throw new WorkerError('MEDIA_FILE_NOT_FOUND', 'render_job_id was not found', false);
        jsonResponse(response, 409, { success: false, status: current.status, error: { code: 'RENDER_CANCELLED', message: `job cannot be cancelled from ${current.status}`, retryable: false } });
        return;
      }
      jsonResponse(response, 202, { success: true, status: 'cancelled', render_job_id: renderJobId, worker_id: workerId });
      return;
    }
    if (request.method === 'POST' && url.pathname === '/qc/technical') {
      const body = await readJsonBody(request);
      const allowed = new Set(['master_id', 'local_path', 'trace_id']);
      const unexpected = Object.keys(body).filter((key) => !allowed.has(key));
      if (unexpected.length) throw new WorkerError('QC_TECHNICAL_FAILED', 'technical QC accepts only master_id, local_path and trace_id', false);
      const result = await runTechnicalQc(body);
      jsonResponse(response, 200, result);
      return;
    }
    jsonResponse(response, 404, { success: false, error: { code: 'MEDIA_WORKER_ROUTE_NOT_FOUND', message: 'route not found', retryable: false } });
  } catch (error) {
    const failure = errorResponse(error);
    jsonResponse(response, failure.statusCode, failure.body);
  }
}

async function initialize() {
  await fsp.mkdir(config.storagePath, { recursive: true, mode: 0o750 });
  storageRoot = await fsp.realpath(config.storagePath);
  await fsp.mkdir(path.join(storageRoot, 'logs', 'render'), { recursive: true, mode: 0o750 });
  await fsp.mkdir(path.join(storageRoot, 'logs', 'render-status'), { recursive: true, mode: 0o750 });
  await fsp.mkdir(path.join(storageRoot, '.worker', 'subtitles'), { recursive: true, mode: 0o750 });
  const writeTest = path.join(storageRoot, '.worker', `.write-test-${process.pid}`);
  await fsp.writeFile(writeTest, 'ok', { mode: 0o600 });
  await fsp.unlink(writeTest);
  await refreshToolState();

  const server = http.createServer((request, response) => {
    handleRequest(request, response).catch((error) => {
      console.error(JSON.stringify({ level: 'error', event: 'http_unhandled', message: safeMessage(error) }));
      if (!response.headersSent) {
        const failure = errorResponse(error);
        jsonResponse(response, failure.statusCode, failure.body);
      } else response.destroy();
    });
  });
  server.requestTimeout = Math.max(config.qcTimeoutMs + 10000, 30000);
  server.headersTimeout = 10000;
  server.keepAliveTimeout = 5000;
  server.listen(config.port, '0.0.0.0', () => {
    console.info(JSON.stringify({
      level: 'info',
      event: 'media_worker_started',
      worker_id: workerId,
      port: config.port,
      enabled: config.enabled,
      storage: '$MEDIA_STORAGE',
      ffmpeg_available: tools.ffmpeg.available,
      ffprobe_available: tools.ffprobe.available,
    }));
  });

  const pollTimer = setInterval(pollOnce, config.pollIntervalMs);
  pollTimer.unref();
  const toolTimer = setInterval(() => refreshToolState().catch((error) => {
    console.error(JSON.stringify({ level: 'error', event: 'tool_check_failed', message: safeMessage(error) }));
  }), 60000);
  toolTimer.unref();
  setImmediate(pollOnce);

  const shutdown = async (signal) => {
    if (shuttingDown) return;
    shuttingDown = true;
    console.info(JSON.stringify({ level: 'info', event: 'media_worker_shutdown', signal, active_jobs: activeJobs.size }));
    clearInterval(pollTimer);
    clearInterval(toolTimer);
    server.close();
    for (const context of activeJobs.values()) {
      context.cancelled = true;
      context.cancel?.();
    }
    const deadline = Date.now() + 10000;
    while (activeJobs.size && Date.now() < deadline) {
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    await pool.end().catch(() => {});
    process.exit(activeJobs.size ? 1 : 0);
  };
  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('SIGINT', () => shutdown('SIGINT'));
}

initialize().catch((error) => {
  console.error(JSON.stringify({ level: 'fatal', event: 'media_worker_start_failed', message: safeMessage(error) }));
  process.exit(1);
});
