import test from 'node:test'
import assert from 'node:assert/strict'
import {
  canRecoverMediaAsset,
  getMediaAssetSuccessorState,
  hasMediaAssetSuccessor,
} from '../src/services/mediaAssetVersions.js'

test('a failed asset remains recoverable until a successor exists', () => {
  assert.equal(canRecoverMediaAsset({ status: 'failed', media_url: null }), true)
})

test('a successful generated successor makes the old asset read-only history', () => {
  const item = {
    status: 'failed',
    successor_asset_id: 'video_v2',
    successor_generation_version: 2,
    successor_status: 'succeeded',
    successor_provider: 'generic_async_video',
  }
  assert.equal(hasMediaAssetSuccessor(item), true)
  assert.equal(canRecoverMediaAsset(item), false)
  assert.deepEqual(getMediaAssetSuccessorState(item), {
    badgeStatus: 'regenerated',
    label: '已重新生成',
    detail: '后继版本 v2 已可用',
  })
})

test('manual replacements and in-flight successors have distinct states', () => {
  assert.equal(getMediaAssetSuccessorState({
    successor_asset_id: 'manual_v2',
    successor_generation_version: 2,
    successor_status: 'succeeded',
    successor_provider: 'manual_upload',
  }).badgeStatus, 'replaced')
  assert.equal(getMediaAssetSuccessorState({
    successor_asset_id: 'video_v2',
    successor_generation_version: 2,
    successor_status: 'processing',
  }).badgeStatus, 'regenerating')
})
