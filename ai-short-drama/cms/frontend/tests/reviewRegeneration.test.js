import test from 'node:test'
import assert from 'node:assert/strict'
import { getVisualRegenerationAction, regenerationNeedsPrompt } from '../src/services/reviewRegeneration.js'

const visual = { stage: 'visual_asset', entity_type: 'generated_asset' }

test('pending visual assets can be rejected and regenerated in one flow', () => {
  assert.deepEqual(getVisualRegenerationAction({ ...visual, review_status: 'pending' }), {
    operation: 'reject_regenerate', mode: 'replace', label: '退回重做',
  })
})

test('reviewed visual assets expose state-specific regeneration actions', () => {
  assert.equal(getVisualRegenerationAction({ ...visual, review_status: 'rejected' }).label, '按意见重新生成')
  assert.deepEqual(getVisualRegenerationAction({ ...visual, review_status: 'approved' }), {
    operation: 'regenerate', mode: 'variant', label: '生成新变体',
  })
})

test('only approved variants require an explicit prompt adjustment', () => {
  assert.equal(regenerationNeedsPrompt('variant'), true)
  assert.equal(regenerationNeedsPrompt('replace'), false)
  assert.equal(getVisualRegenerationAction({ stage: 'shot_video', entity_type: 'shot_video', review_status: 'rejected' }), null)
})
