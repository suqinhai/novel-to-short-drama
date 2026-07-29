import assert from 'node:assert/strict'
import test from 'node:test'
import {
  getAdjacentReview,
  getReviewTabCount,
  getReviewTaskTitle,
  groupReviewItems,
} from '../src/services/reviewWorkspace.js'

const items = [
  { review_id: 'r1', project_id: 'p1', novel_name: '项目一', stage: 'dialogue_audio', entity_type: 'dialogue_audio', review_status: 'pending' },
  { review_id: 'r2', project_id: 'p1', novel_name: '项目一', stage: 'dialogue_audio', entity_type: 'dialogue_audio', review_status: 'approved' },
  { review_id: 'r3', project_id: 'p1', novel_name: '项目一', stage: 'shot_video', entity_type: 'shot_video', review_status: 'pending' },
]

test('groups review tasks by project and production stage', () => {
  const groups = groupReviewItems(items)
  assert.equal(groups.length, 2)
  assert.equal(groups[0].items.length, 2)
  assert.equal(groups[0].pendingCount, 1)
  assert.equal(groups[1].stage, 'shot_video')
})

test('builds operational status counts from the global summary', () => {
  const summary = { total: 12, pending: 3, approved: 7, rejected: 2 }
  assert.equal(getReviewTabCount('pending', summary), 3)
  assert.equal(getReviewTabCount('processed', summary), 9)
  assert.equal(getReviewTabCount('', summary), 12)
})

test('provides human-readable task titles and adjacent navigation', () => {
  assert.equal(getReviewTaskTitle(items[0], '对白音频'), '对白音频任务')
  assert.equal(getAdjacentReview(items, 'r1', 1)?.review_id, 'r2')
  assert.equal(getAdjacentReview(items, 'r1', -1), null)
})
