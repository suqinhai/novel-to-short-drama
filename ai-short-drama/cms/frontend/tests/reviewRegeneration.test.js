import test from 'node:test'
import assert from 'node:assert/strict'
import {
  getRegeneratedSuccessor,
  getRegenerationSourceLabel,
  getVisualRegenerationAction,
  isRegeneratedVisualReview,
  regenerationNeedsPrompt,
} from '../src/services/reviewRegeneration.js'

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

test('a rejected visual review with a successful successor becomes read-only history', () => {
  const regenerated = {
    ...visual,
    review_status: 'rejected',
    regenerated_by_review_id: 'review_successor',
    regenerated_by_entity_id: 'asset_successor',
  }
  assert.equal(isRegeneratedVisualReview(regenerated), true)
  assert.equal(getVisualRegenerationAction(regenerated), null)
})

test('new visual versions explain which asset they were regenerated from', () => {
  assert.equal(getRegenerationSourceLabel({
    ...visual,
    regenerated_from_asset_id: 'asset_fd352fc40a7fbb9f0958',
    generation_version: 5,
  }), '由 asset_fd352fc40a7fbb9f0958 重新生成 · 版本 v5')
  assert.equal(getRegenerationSourceLabel(visual), '')
})

test('old visual records resolve their successor even when filters hide it', () => {
  const oldReview = {
    ...visual,
    entity_id: 'asset_old',
    review_status: 'rejected',
    regenerated_by_review_id: 'review_new',
    regenerated_by_entity_id: 'asset_new',
    regenerated_by_generation_version: 5,
  }
  assert.deepEqual(getRegeneratedSuccessor(oldReview), {
    ...oldReview,
    review_id: 'review_new',
    entity_id: 'asset_new',
    review_status: 'pending',
    reviewed_at: null,
    regenerated_from_asset_id: 'asset_old',
    generation_version: 5,
    regenerated_by_review_id: null,
    regenerated_by_entity_id: null,
    regenerated_by_generation_version: null,
  })

  const loaded = { review_id: 'review_new', entity_id: 'asset_loaded' }
  assert.equal(getRegeneratedSuccessor(oldReview, [loaded]), loaded)
})

test('only approved variants require an explicit prompt adjustment', () => {
  assert.equal(regenerationNeedsPrompt('variant'), true)
  assert.equal(regenerationNeedsPrompt('replace'), false)
  assert.equal(getVisualRegenerationAction({ stage: 'shot_video', entity_type: 'shot_video', review_status: 'rejected' }), null)
})
