import test from 'node:test'
import assert from 'node:assert/strict'
import {
  defaultResolverStage, effectiveInputStateLabel, summarizeEffectiveInputs,
} from '../src/services/effectiveInputs.js'

test('effective input summary keeps requirement and blocking semantics separate', () => {
  const summary = summarizeEffectiveInputs({
    status: 'blocked',
    items: [
      { kind: 'narrative_ir', requirement: 'required', state: 'resolved', blocks: false },
      { kind: 'candidate_selection', requirement: 'optional', state: 'needs_review', blocks: true },
      { kind: 'timeline', requirement: 'required', state: 'missing', blocks: true },
    ],
  })
  assert.deepEqual(summary, {
    ready: false, executable: false, compatibilityMode: false,
    required: 2, optional: 1, resolved: 1, blocked: 2, missing: ['timeline'],
  })
  assert.equal(effectiveInputStateLabel('stale'), '已过期')
  assert.equal(effectiveInputStateLabel('needs_review'), '待确认')
})

test('legacy compatibility diagnostics do not claim generation is blocked', () => {
  const summary = summarizeEffectiveInputs({
    mode: 'legacy',
    status: 'blocked',
    items: [
      { kind: 'pacing_plan', requirement: 'required', state: 'missing', blocks: true },
    ],
  })
  assert.equal(summary.ready, false)
  assert.equal(summary.executable, true)
  assert.equal(summary.compatibilityMode, true)
  assert.equal(summary.blocked, 1)
})

test('project stages map to the matching resolver consumer', () => {
  assert.equal(defaultResolverStage('episode_script_review'), '05')
  assert.equal(defaultResolverStage('storyboard_images_approved'), '08')
  assert.equal(defaultResolverStage('video_processing'), '09')
  assert.equal(defaultResolverStage('tts_processing'), '10')
  assert.equal(defaultResolverStage('rendering'), '17')
})
