'use strict';

const path = require('node:path');

const VIDEO_CODECS = new Set(['libx264', 'libx265']);
const AUDIO_CODECS = new Set(['aac']);
const PIXEL_FORMATS = new Set(['yuv420p', 'yuv422p']);
const PRESETS = new Set([
  'ultrafast', 'superfast', 'veryfast', 'faster', 'fast',
  'medium', 'slow', 'slower', 'veryslow',
]);
const TRANSITION_TYPES = new Map([
  ['fade', 'fade'],
  ['crossfade', 'fade'],
  ['dissolve', 'fade'],
  ['wipeleft', 'wipeleft'],
  ['wiperight', 'wiperight'],
  ['slideleft', 'slideleft'],
  ['slideright', 'slideright'],
]);

class TemplateError extends Error {
  constructor(code, message) {
    super(message);
    this.name = 'TemplateError';
    this.code = code;
    this.retryable = false;
  }
}

function finiteNumber(value, fallback, minimum, maximum, name) {
  const parsed = value === undefined || value === null || value === '' ? fallback : Number(value);
  if (!Number.isFinite(parsed) || parsed < minimum || parsed > maximum) {
    throw new TemplateError('TIMELINE_VALIDATION_FAILED', `${name} must be between ${minimum} and ${maximum}`);
  }
  return parsed;
}

function integer(value, fallback, minimum, maximum, name) {
  const parsed = finiteNumber(value, fallback, minimum, maximum, name);
  if (!Number.isInteger(parsed)) {
    throw new TemplateError('TIMELINE_VALIDATION_FAILED', `${name} must be an integer`);
  }
  return parsed;
}

function choice(value, fallback, choices, name) {
  const normalized = String(value || fallback);
  if (!choices.has(normalized)) {
    throw new TemplateError('TIMELINE_VALIDATION_FAILED', `${name} is not supported`);
  }
  return normalized;
}

function seconds(milliseconds) {
  return (milliseconds / 1000).toFixed(6).replace(/0+$/, '').replace(/\.$/, '');
}

function filterNumber(value) {
  if (!Number.isFinite(value)) {
    throw new TemplateError('TIMELINE_VALIDATION_FAILED', 'non-finite filter value');
  }
  return Number(value).toFixed(6).replace(/0+$/, '').replace(/\.$/, '');
}

// FFmpeg filtergraph escaping is independent of shell escaping. The worker never
// invokes a shell; this function protects the subtitles filter parser itself.
function escapeFilterPath(filePath) {
  return String(filePath)
    .replace(/\\/g, '\\\\')
    .replace(/'/g, "\\'")
    .replace(/:/g, '\\:')
    .replace(/,/g, '\\,')
    .replace(/;/g, '\\;')
    .replace(/\[/g, '\\[')
    .replace(/\]/g, '\\]');
}

function normalizeSettings(manifest, env = process.env) {
  const source = manifest.settings || manifest.output || {};
  const width = integer(source.width, Number(env.OUTPUT_WIDTH || 1080), 64, 4096, 'width');
  const height = integer(source.height, Number(env.OUTPUT_HEIGHT || 1920), 64, 4096, 'height');
  if (width % 2 || height % 2) {
    throw new TemplateError('TIMELINE_VALIDATION_FAILED', 'width and height must be even');
  }
  const fps = finiteNumber(source.fps, Number(env.OUTPUT_FPS || 24), 1, 120, 'fps');
  const sampleRate = integer(
    source.sample_rate || source.sampleRate,
    Number(env.OUTPUT_SAMPLE_RATE || 48000),
    8000,
    192000,
    'sample_rate',
  );
  const crf = integer(source.crf, Number(env.OUTPUT_CRF || 20), 0, 51, 'crf');
  const threads = integer(source.threads, Number(env.MEDIA_MAX_THREADS || 2), 1, 16, 'threads');
  const videoCodec = choice(source.video_codec || source.videoCodec, env.OUTPUT_VIDEO_CODEC || 'libx264', VIDEO_CODECS, 'video_codec');
  const audioCodec = choice(source.audio_codec || source.audioCodec, env.OUTPUT_AUDIO_CODEC || 'aac', AUDIO_CODECS, 'audio_codec');
  const pixelFormat = choice(source.pixel_format || source.pixelFormat, env.OUTPUT_PIXEL_FORMAT || 'yuv420p', PIXEL_FORMATS, 'pixel_format');
  const preset = choice(source.preset, env.OUTPUT_PRESET || 'medium', PRESETS, 'preset');
  const bitrate = String(source.audio_bitrate || source.audioBitrate || env.OUTPUT_AUDIO_BITRATE || '192k');
  if (!/^(?:3[2-9]|[4-9][0-9]|[1-4][0-9]{2}|5(?:0[0-9]|1[0-2]))k$/.test(bitrate)) {
    throw new TemplateError('TIMELINE_VALIDATION_FAILED', 'audio_bitrate must be 32k through 512k');
  }
  const loudness = finiteNumber(source.target_loudness_lufs, Number(env.TARGET_LOUDNESS_LUFS || -16), -36, -5, 'target_loudness_lufs');
  const truePeak = finiteNumber(source.target_true_peak_db, Number(env.TARGET_TRUE_PEAK_DB || -1), -9, 0, 'target_true_peak_db');
  const maxSpeedRatio = finiteNumber(source.max_speed_ratio, Number(env.MEDIA_MAX_SPEED_RATIO || 1.25), 1, 4, 'max_speed_ratio');
  const minSpeedRatio = finiteNumber(source.min_speed_ratio, Number(env.MEDIA_MIN_SPEED_RATIO || 0.8), 0.25, 1, 'min_speed_ratio');
  return {
    width,
    height,
    fps,
    sampleRate,
    crf,
    threads,
    videoCodec,
    audioCodec,
    pixelFormat,
    preset,
    bitrate,
    loudness,
    truePeak,
    maxSpeedRatio,
    minSpeedRatio,
  };
}

function outputArguments(settings, outputPath) {
  if (path.extname(outputPath).toLowerCase() !== '.mp4') {
    throw new TemplateError('TIMELINE_VALIDATION_FAILED', 'render output must use the .mp4 extension');
  }
  return [
    '-c:v', settings.videoCodec,
    '-preset', settings.preset,
    '-crf', String(settings.crf),
    '-pix_fmt', settings.pixelFormat,
    '-r', filterNumber(settings.fps),
    '-c:a', settings.audioCodec,
    '-b:a', settings.bitrate,
    '-ar', String(settings.sampleRate),
    '-ac', '2',
    '-movflags', '+faststart',
    '-max_muxing_queue_size', '2048',
    '-threads', String(settings.threads),
    '-shortest',
    outputPath,
  ];
}

function buildMockPlan({ manifest, outputPath, settings }) {
  const durationMs = integer(
    manifest.total_duration_ms || manifest.duration_ms || manifest.output?.duration_ms,
    5000,
    250,
    3600000,
    'mock duration_ms',
  );
  const duration = seconds(durationMs);
  const videoSource = `testsrc2=size=${settings.width}x${settings.height}:rate=${filterNumber(settings.fps)}:duration=${duration}`;
  const audioSource = `sine=frequency=440:sample_rate=${settings.sampleRate}:duration=${duration}`;
  const filter = `[1:a]volume=0.08,loudnorm=I=${filterNumber(settings.loudness)}:TP=${filterNumber(settings.truePeak)}:LRA=11[aout]`;
  return {
    args: [
      '-hide_banner', '-nostdin', '-y', '-loglevel', 'warning', '-progress', 'pipe:1', '-nostats',
      '-f', 'lavfi', '-i', videoSource,
      '-f', 'lavfi', '-i', audioSource,
      '-filter_complex', filter,
      '-map', '0:v:0', '-map', '[aout]',
      '-t', duration,
      ...outputArguments(settings, outputPath),
    ],
    totalDurationMs: durationMs,
    templateId: 'mock_playable_v1',
  };
}

function addAudioInputFilters({ media, args, filters, firstInputIndex, settings, totalDurationMs }) {
  if (!media.audio.length) {
    const index = firstInputIndex;
    args.push(
      '-f', 'lavfi', '-t', seconds(totalDurationMs), '-i',
      `anullsrc=channel_layout=stereo:sample_rate=${settings.sampleRate}`,
    );
    filters.push(`[${index}:a]atrim=duration=${seconds(totalDurationMs)},asetpts=PTS-STARTPTS[aout]`);
    return;
  }

  const groups = { speech: [], bgm: [], other: [] };
  media.audio.forEach((track, offset) => {
    const inputIndex = firstInputIndex + offset;
    args.push('-ss', seconds(track.sourceInMs), '-t', seconds(track.durationMs), '-i', track.path);
    const label = `a${offset}`;
    const chain = [
      `[${inputIndex}:a]aresample=${settings.sampleRate}`,
      'aformat=sample_fmts=fltp:channel_layouts=stereo',
      'asetpts=PTS-STARTPTS',
      `volume=${filterNumber(track.volume)}`,
    ];
    if (track.fadeInMs > 0) {
      chain.push(`afade=t=in:st=0:d=${seconds(track.fadeInMs)}`);
    }
    if (track.fadeOutMs > 0) {
      chain.push(`afade=t=out:st=${seconds(Math.max(0, track.durationMs - track.fadeOutMs))}:d=${seconds(track.fadeOutMs)}`);
    }
    chain.push(
      `adelay=${track.timelineStartMs}|${track.timelineStartMs}`,
      `apad=whole_dur=${seconds(totalDurationMs)}`,
      `atrim=duration=${seconds(totalDurationMs)}[${label}]`,
    );
    filters.push(chain.join(','));
    if (track.kind === 'dialogue' || track.kind === 'narration') groups.speech.push(label);
    else if (track.kind === 'bgm') groups.bgm.push(label);
    else groups.other.push(label);
  });

  const mixGroup = (labels, name) => {
    if (!labels.length) return null;
    if (labels.length === 1) return labels[0];
    filters.push(`${labels.map((label) => `[${label}]`).join('')}amix=inputs=${labels.length}:normalize=0:dropout_transition=0[${name}]`);
    return name;
  };

  let speech = mixGroup(groups.speech, 'speech');
  let bgm = mixGroup(groups.bgm, 'bgm');
  const other = mixGroup(groups.other, 'other');
  const ducking = manifestBoolean(media.duckingEnabled, true);
  const finalLabels = [];
  if (speech && bgm && ducking) {
    filters.push(`[${speech}]asplit=2[speechmix][sidechain]`);
    filters.push(`[${bgm}][sidechain]sidechaincompress=threshold=0.02:ratio=8:attack=20:release=250[bgmducked]`);
    speech = 'speechmix';
    bgm = 'bgmducked';
  }
  if (speech) finalLabels.push(speech);
  if (bgm) finalLabels.push(bgm);
  if (other) finalLabels.push(other);
  if (finalLabels.length === 1) {
    filters.push(`[${finalLabels[0]}]loudnorm=I=${filterNumber(settings.loudness)}:TP=${filterNumber(settings.truePeak)}:LRA=11,atrim=duration=${seconds(totalDurationMs)}[aout]`);
  } else {
    filters.push(`${finalLabels.map((label) => `[${label}]`).join('')}amix=inputs=${finalLabels.length}:normalize=0:dropout_transition=0,loudnorm=I=${filterNumber(settings.loudness)}:TP=${filterNumber(settings.truePeak)}:LRA=11,atrim=duration=${seconds(totalDurationMs)}[aout]`);
  }
}

function manifestBoolean(value, fallback) {
  if (value === undefined || value === null || value === '') return fallback;
  if (typeof value === 'boolean') return value;
  return String(value).toLowerCase() === 'true';
}

function buildTimelinePlan({ manifest, media, outputPath, settings }) {
  if (!media.videos.length) {
    throw new TemplateError('MEDIA_ASSETS_INCOMPLETE', 'at least one validated video segment is required');
  }
  const args = ['-hide_banner', '-nostdin', '-y', '-loglevel', 'warning', '-progress', 'pipe:1', '-nostats'];
  const filters = [];
  let cursor = 0;
  media.videos.forEach((segment, index) => {
    if (Math.abs(segment.timelineStartMs - cursor) > 100) {
      throw new TemplateError('TIMELINE_VALIDATION_FAILED', 'video segments must form a continuous cut timeline');
    }
    const sourceDurationMs = segment.sourceOutMs - segment.sourceInMs;
    const speedRatio = segment.durationMs / sourceDurationMs;
    if (speedRatio < settings.minSpeedRatio || speedRatio > settings.maxSpeedRatio) {
      throw new TemplateError('TIMELINE_VALIDATION_FAILED', 'video speed adjustment is outside the configured safe range');
    }
    args.push('-ss', seconds(segment.sourceInMs), '-t', seconds(sourceDurationMs), '-i', segment.path);
    const transform = Math.abs(speedRatio - 1) < 0.0005 ? '' : `,setpts=${filterNumber(speedRatio)}*PTS`;
    filters.push(
      `[${index}:v]scale=${settings.width}:${settings.height}:force_original_aspect_ratio=decrease,` +
      `pad=${settings.width}:${settings.height}:(ow-iw)/2:(oh-ih)/2:color=black,` +
      `fps=${filterNumber(settings.fps)},format=${settings.pixelFormat},setsar=1,settb=AVTB,setpts=PTS-STARTPTS${transform}[v${index}]`,
    );
    cursor += segment.durationMs;
  });
  const totalDurationMs = cursor;
  const transitionEntries = Array.isArray(media.transitions) ? media.transitions : [];
  const appliedTransitions = new Set();
  const transitionAt = (boundaryIndex) => {
    const previous = media.videos[boundaryIndex - 1];
    const next = media.videos[boundaryIndex];
    let index = transitionEntries.findIndex((transition, candidateIndex) => {
      if (appliedTransitions.has(candidateIndex) || !transition || typeof transition !== 'object') return false;
      const fromMatches = transition.from_shot_id === undefined || String(transition.from_shot_id) === String(previous.shotId);
      const toMatches = transition.to_shot_id === undefined || String(transition.to_shot_id) === String(next.shotId);
      return fromMatches && toMatches && (transition.from_shot_id !== undefined || transition.to_shot_id !== undefined);
    });
    if (index < 0 && transitionEntries.length === media.videos.length - 1) index = boundaryIndex - 1;
    if (index < 0) return null;
    appliedTransitions.add(index);
    const entry = transitionEntries[index];
    const rawType = String(entry.type || entry.transition_type || 'cut').toLowerCase();
    if (rawType === 'cut') return null;
    const type = TRANSITION_TYPES.get(rawType);
    if (!type) throw new TemplateError('TIMELINE_VALIDATION_FAILED', `unsupported transition type: ${rawType}`);
    const durationMs = integer(entry.duration_ms ?? entry.durationMs, 200, 1, 2000, 'transition duration_ms');
    if (durationMs * 2 >= Math.min(previous.durationMs, next.durationMs)) {
      throw new TemplateError('TIMELINE_VALIDATION_FAILED', 'transition duration is too long for adjacent shots');
    }
    return { type, durationMs };
  };
  let videoChain = 'v0';
  let videoDurationMs = media.videos[0].durationMs;
  for (let index = 1; index < media.videos.length; index += 1) {
    const transition = transitionAt(index);
    const nextChain = `vchain${index}`;
    if (transition) {
      const padded = `vpad${index}`;
      filters.push(`[${videoChain}]tpad=stop_mode=clone:stop_duration=${seconds(transition.durationMs)}[${padded}]`);
      filters.push(
        `[${padded}][v${index}]xfade=transition=${transition.type}:duration=${seconds(transition.durationMs)}:` +
        `offset=${seconds(videoDurationMs)}[${nextChain}]`,
      );
    } else {
      filters.push(`[${videoChain}][v${index}]concat=n=2:v=1:a=0[${nextChain}]`);
    }
    videoChain = nextChain;
    videoDurationMs += media.videos[index].durationMs;
  }
  const unapplied = transitionEntries.filter((transition, index) => {
    const type = String(transition?.type || transition?.transition_type || 'cut').toLowerCase();
    return type !== 'cut' && !appliedTransitions.has(index);
  });
  if (unapplied.length) throw new TemplateError('TIMELINE_VALIDATION_FAILED', 'transition does not match an adjacent video boundary');
  filters.push(`[${videoChain}]null[vcat]`);

  if (media.subtitlePath) {
    filters.push(`[vcat]subtitles=filename='${escapeFilterPath(media.subtitlePath)}':charenc=UTF-8[vout]`);
  } else {
    filters.push('[vcat]null[vout]');
  }

  addAudioInputFilters({
    manifest,
    media,
    args,
    filters,
    firstInputIndex: media.videos.length,
    settings,
    totalDurationMs,
  });
  return {
    args: [
      ...args,
      '-filter_complex', filters.join(';'),
      '-map', '[vout]', '-map', '[aout]',
      '-t', seconds(totalDurationMs),
      ...outputArguments(settings, outputPath),
    ],
    totalDurationMs,
    templateId: media.subtitlePath ? 'timeline_subtitled_v1' : 'timeline_clean_v1',
  };
}

function buildRenderPlan(options) {
  const settings = options.settings || normalizeSettings(options.manifest);
  if (options.mock) return buildMockPlan({ ...options, settings });
  return buildTimelinePlan({ ...options, settings });
}

module.exports = {
  TemplateError,
  buildRenderPlan,
  escapeFilterPath,
  normalizeSettings,
};
