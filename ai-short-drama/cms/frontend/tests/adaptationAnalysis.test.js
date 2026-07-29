import test from 'node:test'
import assert from 'node:assert/strict'
import { buildSpecFromDiagnostic, curvePoints, normalizeBeatEdits, severityLabel } from '../src/services/adaptationAnalysis.js'

test('节奏曲线限制在可视区域', () => {
  assert.equal(curvePoints([{ conflict_intensity: -1 }, { conflict_intensity: 0.5 }, { conflict_intensity: 2 }], 'conflict_intensity', 100, 100), '0,100 50,50 100,0')
  assert.equal(curvePoints([], 'conflict_intensity'), '')
})

test('节拍编辑只提交可修改字段并转为数字', () => {
  assert.deepEqual(normalizeBeatEdits([{ beat_key: 'b1', episode_number: '2', beat_ordinal: '3', estimated_duration_seconds: '15', title: '忽略' }]), [
    { beat_key: 'b1', episode_number: 2, beat_ordinal: 3, estimated_duration_seconds: 15 },
  ])
})

test('诊断建议生成待确认 Adaptation Spec 草稿', () => {
  const value = buildSpecFromDiagnostic(
    { version_number: 2, source_version_id: 'sv1', ir_revision_id: 'ir1', nodes: [{ chapter_id: 'c1' }], target_audience: { primary: '女性成长用户' }, emotional_value: ['逆袭'] },
    { episodes: [{}, {}], total_duration_seconds: 180 },
  )
  assert.equal(value.target_episode_count, 2)
  assert.equal(value.episode_duration_seconds, 90)
  assert.equal(value.audience_profile.description, '女性成长用户')
  assert.equal(value.source_version_id, 'sv1')
  assert.deepEqual(value.scope.chapter_ids, ['c1'])
  assert.equal(severityLabel('high'), '严重')
})
