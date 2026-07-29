import test from 'node:test'
import assert from 'node:assert/strict'
import {
  canRecoverMediaAsset,
  getMediaAssetSourceLabel,
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

test('a new version explains which asset it was regenerated from', () => {
  assert.equal(getMediaAssetSourceLabel({
    asset_id: 'asset_v5',
    predecessor_asset_id: 'asset_fd352fc40a7fbb9f0958',
    generation_version: 5,
    provider: 'generic_openai_images',
  }), '由 asset_fd352fc40a7fbb9f0958 重新生成 · 版本 v5')
})

test('manual replacement lineage uses the replacement wording', () => {
  assert.equal(getMediaAssetSourceLabel({
    predecessor_asset_id: 'video_v1',
    generation_version: 2,
    provider: 'manual_upload',
  }), '由 video_v1 上传替换 · 版本 v2')
})
