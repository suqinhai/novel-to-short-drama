import test from 'node:test'
import assert from 'node:assert/strict'
import {
  cloneBibleAsVersion, continuitySummary, fieldLockState, handoffActionLabel,
  issueLocator, sortedIssues,
} from '../src/services/performanceContinuity.js'

test('locked fields remain non-editable while explicitly allowed fields can vary', () => {
  const bible = {
    performance_bible_id: 'pb_lin_v3',
    character_id: 'char_lin',
    character_version: 'adult',
    locked_fields: ['appearance.face_shape', 'speech.voice_identity'],
    allowed_fields: ['appearance.hairstyle'],
    speech: { rate_wpm: 150 },
    acting: {},
    appearance: { face_shape: 'oval', hairstyle: 'low ponytail' },
  }
  assert.deepEqual(fieldLockState(bible, 'appearance.face_shape'), { locked: true, allowed: false, editable: false })
  assert.deepEqual(fieldLockState(bible, 'appearance.hairstyle'), { locked: false, allowed: true, editable: true })
  const version = cloneBibleAsVersion(bible, { appearance: { ...bible.appearance, hairstyle: 'loose hair' } }, 'episode 6 injury')
  assert.equal(version.parent_performance_bible_id, 'pb_lin_v3')
  assert.equal(version.change_reason, 'episode 6 injury')
  assert.equal(version.appearance.face_shape, 'oval')
})

test('continuity timeline exposes character position, costume, held props and environment', () => {
  const summary = continuitySummary({
    output_state: {
      characters: {
        char_lin: { position: 'left', facing: 'right', costume: 'coat_v1', held_props: ['letter'], emotion: 'angry' },
      },
      props: { letter: { visible: true } },
      environment: { location_id: 'room', time: 'night', weather: 'rain', lighting: 'blue-left' },
      axis: 'lin-left_to_zhou-right',
    },
  })
  assert.equal(summary.characters[0].held, 'letter')
  assert.equal(summary.environment.weather, 'rain')
  assert.equal(summary.axis, 'lin-left_to_zhou-right')
})

test('QC issues sort by severity then exact frame locator', () => {
  const issues = sortedIssues([
    { visual_qc_issue_id: 'minor', severity: 'minor', episode_id: 'ep1', scene_id: 'sc1', shot_id: 'sh1', timecode_ms: 100, frame_number: 3 },
    { visual_qc_issue_id: 'critical-late', severity: 'critical', episode_id: 'ep1', scene_id: 'sc1', shot_id: 'sh2', timecode_ms: 40, frame_number: 1 },
    { visual_qc_issue_id: 'critical-early', severity: 'critical', episode_id: 'ep1', scene_id: 'sc1', shot_id: 'sh1', timecode_ms: 1900, frame_number: 48 },
  ])
  assert.deepEqual(issues.map((item) => item.visual_qc_issue_id), ['critical-early', 'critical-late', 'minor'])
  assert.equal(issueLocator(issues[0]), 'ep1 / sc1 / sh1 · 1.900s · F48')
})

test('handoff labels explicit action relay', () => {
  assert.equal(handoffActionLabel({ from_action_phase: '抬手开始', to_action_phase: '完成挥掌' }), '抬手开始 → 完成挥掌')
})
