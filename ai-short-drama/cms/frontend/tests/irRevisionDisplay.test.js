import assert from 'node:assert/strict'
import test from 'node:test'
import {
  getIRProgressSummary,
  getIRRevisionDisplayStatus,
  getIRScopeSummary,
} from '../src/services/irRevisionDisplay.js'

test('uses the live operation status until the revision is published', () => {
  assert.equal(getIRRevisionDisplayStatus({ status: 'staging', operation_status: 'running' }), 'running')
  assert.equal(getIRRevisionDisplayStatus({ status: 'rejected', operation_status: 'failed' }), 'failed')
  assert.equal(getIRRevisionDisplayStatus({ status: 'published', operation_status: 'completed' }), 'published')
})

test('describes full extraction chapters without calling them changed chapters', () => {
  assert.equal(getIRScopeSummary({ revision_scope: 'full', changed_chapter_ids: ['a', 'b'] }), '本次提取 2 章')
  assert.equal(getIRScopeSummary({ revision_scope: 'incremental', changed_chapter_ids: ['a'] }), '增量修订 · 1 个变更章节')
})

test('shows retry stage and terminal error details', () => {
  assert.equal(
    getIRProgressSummary({ operation_status: 'running', retry_count: 1, checkpoint_stage: 'retry_queued' }),
    '第 1 次重试 · 等待重试',
  )
  assert.equal(
    getIRProgressSummary({ operation_status: 'failed', operation_error_message: '模型请求超时' }),
    '模型请求超时',
  )
})
