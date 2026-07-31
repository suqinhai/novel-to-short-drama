'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const { buildRenderPlan, TemplateError } = require('../ffmpeg-templates');

function baseOptions(audio) {
  return {
    manifest: { audio: { ducking_enabled: true } },
    media: {
      videos: [{
        path: '/media/shot.mp4', shotId: 'shot_1', timelineStartMs: 0,
        durationMs: 5000, sourceInMs: 0, sourceOutMs: 5000,
      }],
      audio,
      transitions: [],
      subtitlePath: null,
      duckingEnabled: true,
      ducking: { threshold: 0.03, ratio: 6, attackMs: 15, releaseMs: 300 },
    },
    outputPath: '/output/master.mp4',
    settings: {
      width: 1080, height: 1920, fps: 24, sampleRate: 48000, crf: 20,
      threads: 2, videoCodec: 'libx264', audioCodec: 'aac', pixelFormat: 'yuv420p',
      preset: 'medium', bitrate: '192k', loudness: -16, truePeak: -1,
      minSpeedRatio: .8, maxSpeedRatio: 1.25,
    },
  };
}

test('dialogue limited speed, BGM pitch, fades and ducking compile into one safe filtergraph', () => {
  const plan = buildRenderPlan(baseOptions([
    {
      path: '/media/dialogue.wav', kind: 'dialogue', timelineStartMs: 500,
      durationMs: 1000, sourceDurationMs: 1100, sourceInMs: 0, volume: 1,
      fadeInMs: 10, fadeOutMs: 20, speedRatio: 1.1, pitchSemitones: 0,
    },
    {
      path: '/media/bgm.wav', kind: 'bgm', timelineStartMs: 0,
      durationMs: 5000, sourceDurationMs: 5000, sourceInMs: 0, volume: .4,
      fadeInMs: 300, fadeOutMs: 500, speedRatio: 1, pitchSemitones: 2,
    },
  ]));
  const graph = plan.args[plan.args.indexOf('-filter_complex') + 1];
  assert.match(graph, /atempo=1\.1/);
  assert.match(graph, /asetrate=/);
  assert.match(graph, /sidechaincompress=threshold=0\.03:ratio=6:attack=15:release=300/);
  assert.match(graph, /afade=t=in/);
});

test('media template rejects excessive dialogue acceleration instead of hiding overrun', () => {
  assert.throws(() => buildRenderPlan(baseOptions([{
    path: '/media/dialogue.wav', kind: 'dialogue', timelineStartMs: 0,
    durationMs: 1000, sourceDurationMs: 1200, sourceInMs: 0, volume: 1,
    fadeInMs: 0, fadeOutMs: 0, speedRatio: 1.2, pitchSemitones: 0,
  }])), error => error instanceof TemplateError && error.code === 'TIMELINE_VALIDATION_FAILED');
});
