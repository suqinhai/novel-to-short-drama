import test from 'node:test'
import assert from 'node:assert/strict'
import {
  NLE_TRACKS, activeItemsAt, formatNLETimecode, gesturePatch, renderOutcome,
  snapTimeMS, stepPlayhead, subtitlePreviewConfig, visibleTimelineWindow,
} from '../src/services/timelineNle.js'

test('NLE exposes the seven required fixed tracks and unified millisecond timecode', () => {
  assert.deepEqual(NLE_TRACKS.map(item => item.type), [
    'video', 'dialogue', 'narration', 'bgm', 'ambience', 'sound_effect', 'subtitle',
  ])
  assert.equal(formatNLETimecode(62345), '01:02.345')
  assert.equal(stepPlayhead(1000, 1, 'frame', 25), 1040)
  assert.equal(stepPlayhead(1000, -1, '100ms', 25), 900)
})

test('move and trims snap in milliseconds while keeping independent source handles', () => {
  const item = { timeline_item_id: 'a', timeline_start_ms: 1000, timeline_end_ms: 3000, source_in_ms: 200, source_out_ms: 2200 }
  assert.equal(snapTimeMS(1940, [0, 2000], 80), 2000)
  assert.deepEqual(gesturePatch(item, 'move', 940, [2000], 80), { timeline_start_ms: 2000, timeline_end_ms: 4000 })
  assert.deepEqual(gesturePatch(item, 'trim-start', 450, [1500], 80), { timeline_start_ms: 1500, source_in_ms: 700 })
  assert.deepEqual(gesturePatch(item, 'trim-end', -450, [2500], 80), { timeline_end_ms: 2500, source_out_ms: 1700 })
})

test('virtual time window bounds long episodes and only resolves active media', () => {
  assert.deepEqual(visibleTimelineWindow(6000, 1200, 60, 3600000), { start_ms: 95000, end_ms: 125000 })
  const items = [
    { track_type: 'video', timeline_start_ms: 0, timeline_end_ms: 2000 },
    { track_type: 'dialogue', timeline_start_ms: 500, timeline_end_ms: 1200 },
  ]
  assert.equal(activeItemsAt(items, 800).length, 2)
})

test('render failure preserves the approved current timeline', () => {
  assert.deepEqual(renderOutcome('approved_v3', 'draft_v4', 'failed'), {
    current_timeline_id: 'approved_v3', draft_state: 'render_failed',
  })
  assert.equal(renderOutcome('approved_v3', 'draft_v4', 'succeeded').current_timeline_id, 'draft_v4')
})

test('subtitle preview keeps safe-area positioning and style', () => {
  assert.deepEqual(subtitlePreviewConfig({
    subtitle_text: '安全区字幕',
    transform_config: { position_x_pct: 45, position_y_pct: 80, font_size_px: 32, safe_area_enabled: true },
    effect_config: { subtitle_style: 'outline', color: '#ff0' },
  }), {
    text: '安全区字幕', x: 45, y: 80, fontSize: 32, color: '#ff0',
    background: 'rgba(0,0,0,.64)', fontWeight: 700, safeArea: true, style: 'outline',
  })
  assert.equal(subtitlePreviewConfig({
    transform_config: { position_x_pct: 2, position_y_pct: 98, safe_area_enabled: true },
  }).x, 10)
  assert.equal(subtitlePreviewConfig({
    transform_config: { position_x_pct: 2, position_y_pct: 98, safe_area_enabled: true },
  }).y, 90)
  assert.equal(subtitlePreviewConfig({
    transform_config: { position_x_pct: 2, safe_area_enabled: false },
  }).x, 2)
})
